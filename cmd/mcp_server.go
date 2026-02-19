package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/zhubert/plural-core/logger"
	"github.com/zhubert/plural-core/mcp"
)

var socketPath string
var tcpAddr string
var listenAddr string
var autoApprove bool
var mcpSessionID string
var mcpSupervisor bool
var mcpHostTools bool

var mcpServerCmd = &cobra.Command{
	Use:    "mcp-server",
	Short:  "Run MCP server for permission prompts (internal use)",
	Hidden: true,
	RunE:   runMCPServer,
}

func init() {
	mcpServerCmd.Flags().StringVar(&socketPath, "socket", "", "Unix socket path for TUI communication")
	mcpServerCmd.Flags().StringVar(&tcpAddr, "tcp", "", "TCP address for TUI communication (host:port)")
	mcpServerCmd.Flags().StringVar(&listenAddr, "listen", "", "Listen on TCP address and wait for host to connect (host:port)")
	mcpServerCmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "Auto-approve all tool permissions (used in container mode)")
	mcpServerCmd.Flags().StringVar(&mcpSessionID, "session-id", "", "Session ID for logging")
	mcpServerCmd.Flags().BoolVar(&mcpSupervisor, "supervisor", false, "Enable supervisor tools (create/list/merge child sessions)")
	mcpServerCmd.Flags().BoolVar(&mcpHostTools, "host-tools", false, "Enable host operation tools (create_pr, push_branch)")
	rootCmd.AddCommand(mcpServerCmd)
}

func runMCPServer(cmd *cobra.Command, args []string) error {
	// Diagnostic output to stderr — flows through Docker back to host stream log.
	// Prefixed with [mcp] so the host can identify MCP subprocess lifecycle events.
	mode := "socket"
	if listenAddr != "" {
		mode = "listen=" + listenAddr
	} else if tcpAddr != "" {
		mode = "tcp=" + tcpAddr
	} else if socketPath != "" {
		mode = "socket=" + socketPath
	}
	fmt.Fprintf(os.Stderr, "[mcp] starting (mode=%s auto-approve=%v supervisor=%v host-tools=%v)\n",
		mode, autoApprove, mcpSupervisor, mcpHostTools)

	// Determine session ID from flag or socket path
	sessionID := mcpSessionID
	if sessionID == "" {
		sessionID = extractSessionID(socketPath)
	}
	if sessionID != "" {
		if logPath, err := logger.MCPLogPath(sessionID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to get MCP log path: %v\n", err)
		} else if err := logger.Init(logPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}
	defer logger.Close()

	// Connect to TUI — via listen (container reverse-TCP mode), TCP (legacy), or Unix socket (host mode).
	//
	// In --listen mode (container sessions), the MCP subprocess listens on a port inside the
	// container and waits for the host to dial in. This reverses the TCP direction so that
	// macOS firewall rules (which block inbound connections to the host) are bypassed.
	//
	// In --tcp mode (legacy), the subprocess connects outward to the host.
	// TCP connections retry because the container's network stack may not be ready
	// immediately on boot. Without retries, the MCP subprocess exits, causing
	// Claude CLI to exit, and the user's first prompt is lost.
	var client *mcp.SocketClient
	var err error
	if listenAddr != "" {
		// Reverse TCP: listen and wait for the host to connect
		fmt.Fprintf(os.Stderr, "[mcp] listen: waiting for host connection on %s\n", listenAddr)
		client, err = mcp.NewListeningSocketClient(listenAddr)
		if err != nil {
			return fmt.Errorf("error listening for TUI connection on %s: %w", listenAddr, err)
		}
		fmt.Fprintf(os.Stderr, "[mcp] listen: host connected\n")
	} else if tcpAddr != "" {
		const maxRetries = 10
		const retryInterval = 500 * time.Millisecond
		for i := range maxRetries {
			client, err = mcp.NewTCPSocketClient(tcpAddr)
			if err == nil {
				break
			}
			if i < maxRetries-1 {
				fmt.Fprintf(os.Stderr, "TCP connect attempt %d/%d failed, retrying: %v\n", i+1, maxRetries, err)
				time.Sleep(retryInterval)
			}
		}
		if err != nil {
			return fmt.Errorf("error connecting to TUI via TCP (%s) after %d attempts: %w", tcpAddr, maxRetries, err)
		}
	} else if socketPath != "" {
		client, err = mcp.NewSocketClient(socketPath)
		if err != nil {
			return fmt.Errorf("error connecting to TUI socket: %w", err)
		}
	} else {
		return fmt.Errorf("either --socket, --tcp, or --listen must be specified")
	}
	defer client.Close()

	// Create channels for MCP server communication.
	// Response channels are buffered (1) so that if the server exits while a
	// forwarding goroutine is sending a response, the send completes without
	// blocking and the goroutine can exit its range loop when the request
	// channel is closed.
	reqChan := make(chan mcp.PermissionRequest)
	respChan := make(chan mcp.PermissionResponse, 1)
	questionChan := make(chan mcp.QuestionRequest)
	answerChan := make(chan mcp.QuestionResponse, 1)
	planApprovalChan := make(chan mcp.PlanApprovalRequest)
	planResponseChan := make(chan mcp.PlanApprovalResponse, 1)

	// Start goroutines to forward requests to the TUI via socket and return responses.
	// Each goroutine exits when its request channel is closed (range loop ends).
	var wg sync.WaitGroup

	mcp.ForwardRequests(&wg, reqChan, respChan,
		client.SendPermissionRequest,
		func(req mcp.PermissionRequest) mcp.PermissionResponse {
			return mcp.PermissionResponse{ID: req.ID, Allowed: false, Message: "Communication error with TUI"}
		})

	mcp.ForwardRequests(&wg, questionChan, answerChan,
		client.SendQuestionRequest,
		func(req mcp.QuestionRequest) mcp.QuestionResponse {
			return mcp.QuestionResponse{ID: req.ID, Answers: map[string]string{}}
		})

	mcp.ForwardRequests(&wg, planApprovalChan, planResponseChan,
		client.SendPlanApprovalRequest,
		func(req mcp.PlanApprovalRequest) mcp.PlanApprovalResponse {
			return mcp.PlanApprovalResponse{ID: req.ID, Approved: false}
		})

	// Supervisor channels and forwarding goroutines
	var serverOpts []mcp.ServerOption
	var createChildChan chan mcp.CreateChildRequest
	var createChildRespChan chan mcp.CreateChildResponse
	var listChildrenChan chan mcp.ListChildrenRequest
	var listChildrenRespChan chan mcp.ListChildrenResponse
	var mergeChildChan chan mcp.MergeChildRequest
	var mergeChildRespChan chan mcp.MergeChildResponse

	if mcpSupervisor {
		createChildChan = make(chan mcp.CreateChildRequest)
		createChildRespChan = make(chan mcp.CreateChildResponse, 1)
		listChildrenChan = make(chan mcp.ListChildrenRequest)
		listChildrenRespChan = make(chan mcp.ListChildrenResponse, 1)
		mergeChildChan = make(chan mcp.MergeChildRequest)
		mergeChildRespChan = make(chan mcp.MergeChildResponse, 1)

		mcp.ForwardRequests(&wg, createChildChan, createChildRespChan,
			client.SendCreateChildRequest,
			func(req mcp.CreateChildRequest) mcp.CreateChildResponse {
				return mcp.CreateChildResponse{ID: req.ID, Success: false, Error: "Communication error with TUI"}
			})

		mcp.ForwardRequests(&wg, listChildrenChan, listChildrenRespChan,
			client.SendListChildrenRequest,
			func(req mcp.ListChildrenRequest) mcp.ListChildrenResponse {
				return mcp.ListChildrenResponse{ID: req.ID, Children: []mcp.ChildSessionInfo{}}
			})

		mcp.ForwardRequests(&wg, mergeChildChan, mergeChildRespChan,
			client.SendMergeChildRequest,
			func(req mcp.MergeChildRequest) mcp.MergeChildResponse {
				return mcp.MergeChildResponse{ID: req.ID, Success: false, Error: "Communication error with TUI"}
			})

		serverOpts = append(serverOpts, mcp.WithSupervisor(
			createChildChan, createChildRespChan,
			listChildrenChan, listChildrenRespChan,
			mergeChildChan, mergeChildRespChan,
		))
	}

	// Host tools channels and forwarding goroutines
	var createPRChan chan mcp.CreatePRRequest
	var createPRRespChan chan mcp.CreatePRResponse
	var pushBranchChan chan mcp.PushBranchRequest
	var pushBranchRespChan chan mcp.PushBranchResponse
	var getReviewCommentsChan chan mcp.GetReviewCommentsRequest
	var getReviewCommentsRespChan chan mcp.GetReviewCommentsResponse

	if mcpHostTools {
		createPRChan = make(chan mcp.CreatePRRequest)
		createPRRespChan = make(chan mcp.CreatePRResponse, 1)
		pushBranchChan = make(chan mcp.PushBranchRequest)
		pushBranchRespChan = make(chan mcp.PushBranchResponse, 1)
		getReviewCommentsChan = make(chan mcp.GetReviewCommentsRequest)
		getReviewCommentsRespChan = make(chan mcp.GetReviewCommentsResponse, 1)

		mcp.ForwardRequests(&wg, createPRChan, createPRRespChan,
			client.SendCreatePRRequest,
			func(req mcp.CreatePRRequest) mcp.CreatePRResponse {
				return mcp.CreatePRResponse{ID: req.ID, Success: false, Error: "Communication error with TUI"}
			})

		mcp.ForwardRequests(&wg, pushBranchChan, pushBranchRespChan,
			client.SendPushBranchRequest,
			func(req mcp.PushBranchRequest) mcp.PushBranchResponse {
				return mcp.PushBranchResponse{ID: req.ID, Success: false, Error: "Communication error with TUI"}
			})

		mcp.ForwardRequests(&wg, getReviewCommentsChan, getReviewCommentsRespChan,
			client.SendGetReviewCommentsRequest,
			func(req mcp.GetReviewCommentsRequest) mcp.GetReviewCommentsResponse {
				return mcp.GetReviewCommentsResponse{ID: req.ID, Success: false, Error: "Communication error with TUI"}
			})

		serverOpts = append(serverOpts, mcp.WithHostTools(
			createPRChan, createPRRespChan,
			pushBranchChan, pushBranchRespChan,
			getReviewCommentsChan, getReviewCommentsRespChan,
		))
	}

	// Run MCP server on stdin/stdout
	fmt.Fprintf(os.Stderr, "[mcp] connected to TUI, starting JSONRPC server\n")
	var allowedTools []string
	if autoApprove {
		allowedTools = []string{"*"}
	}
	server := mcp.NewServer(os.Stdin, os.Stdout, reqChan, respChan, questionChan, answerChan, planApprovalChan, planResponseChan, allowedTools, sessionID, serverOpts...)
	err = server.Run()
	fmt.Fprintf(os.Stderr, "[mcp] JSONRPC server exited (err=%v)\n", err)

	// Close request channels so the forwarding goroutines exit their range loops,
	// then wait for them to finish before closing response channels.
	close(reqChan)
	close(questionChan)
	close(planApprovalChan)
	if mcpSupervisor {
		close(createChildChan)
		close(listChildrenChan)
		close(mergeChildChan)
	}
	if mcpHostTools {
		close(createPRChan)
		close(pushBranchChan)
		close(getReviewCommentsChan)
	}
	wg.Wait()
	close(respChan)
	close(answerChan)
	close(planResponseChan)
	if mcpSupervisor {
		close(createChildRespChan)
		close(listChildrenRespChan)
		close(mergeChildRespChan)
	}
	if mcpHostTools {
		close(createPRRespChan)
		close(pushBranchRespChan)
		close(getReviewCommentsRespChan)
	}

	if err != nil {
		return fmt.Errorf("MCP server error: %w", err)
	}
	return nil
}

// extractSessionID extracts the session ID from a socket path like /tmp/pl-<session-id>.sock
func extractSessionID(socketPath string) string {
	base := filepath.Base(socketPath)
	// Remove .sock extension
	base = strings.TrimSuffix(base, ".sock")
	// Remove pl- prefix (shortened from plural- to keep socket path under Unix limit)
	if after, ok := strings.CutPrefix(base, "pl-"); ok {
		return after
	}
	return ""
}
