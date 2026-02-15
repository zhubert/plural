package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/mcp"
)

var socketPath string
var tcpAddr string
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
	mcpServerCmd.Flags().BoolVar(&autoApprove, "auto-approve", false, "Auto-approve all tool permissions (used in container mode)")
	mcpServerCmd.Flags().StringVar(&mcpSessionID, "session-id", "", "Session ID for logging")
	mcpServerCmd.Flags().BoolVar(&mcpSupervisor, "supervisor", false, "Enable supervisor tools (create/list/merge child sessions)")
	mcpServerCmd.Flags().BoolVar(&mcpHostTools, "host-tools", false, "Enable host operation tools (create_pr, push_branch)")
	rootCmd.AddCommand(mcpServerCmd)
}

func runMCPServer(cmd *cobra.Command, args []string) error {
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

	// Connect to TUI — via TCP (container mode) or Unix socket (host mode).
	// TCP connections retry because the container's network stack may not be ready
	// immediately on boot. Without retries, the MCP subprocess exits, causing
	// Claude CLI to exit, and the user's first prompt is lost.
	var client *mcp.SocketClient
	var err error
	if tcpAddr != "" {
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
		return fmt.Errorf("either --socket or --tcp must be specified")
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
	wg.Add(3)

	go func() {
		defer wg.Done()
		for req := range reqChan {
			resp, err := client.SendPermissionRequest(req)
			if err != nil {
				// On error, deny permission
				respChan <- mcp.PermissionResponse{
					ID:      req.ID,
					Allowed: false,
					Message: "Communication error with TUI",
				}
			} else {
				respChan <- resp
			}
		}
	}()

	go func() {
		defer wg.Done()
		for req := range questionChan {
			resp, err := client.SendQuestionRequest(req)
			if err != nil {
				// On error, return empty answers
				answerChan <- mcp.QuestionResponse{
					ID:      req.ID,
					Answers: map[string]string{},
				}
			} else {
				answerChan <- resp
			}
		}
	}()

	go func() {
		defer wg.Done()
		for req := range planApprovalChan {
			resp, err := client.SendPlanApprovalRequest(req)
			if err != nil {
				// On error, reject the plan
				planResponseChan <- mcp.PlanApprovalResponse{
					ID:       req.ID,
					Approved: false,
				}
			} else {
				planResponseChan <- resp
			}
		}
	}()

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

		wg.Add(3)

		go func() {
			defer wg.Done()
			for req := range createChildChan {
				resp, fwdErr := client.SendCreateChildRequest(req)
				if fwdErr != nil {
					createChildRespChan <- mcp.CreateChildResponse{ID: req.ID, Success: false, Error: "Communication error with TUI"}
				} else {
					createChildRespChan <- resp
				}
			}
		}()

		go func() {
			defer wg.Done()
			for req := range listChildrenChan {
				resp, fwdErr := client.SendListChildrenRequest(req)
				if fwdErr != nil {
					listChildrenRespChan <- mcp.ListChildrenResponse{ID: req.ID, Children: []mcp.ChildSessionInfo{}}
				} else {
					listChildrenRespChan <- resp
				}
			}
		}()

		go func() {
			defer wg.Done()
			for req := range mergeChildChan {
				resp, fwdErr := client.SendMergeChildRequest(req)
				if fwdErr != nil {
					mergeChildRespChan <- mcp.MergeChildResponse{ID: req.ID, Success: false, Error: "Communication error with TUI"}
				} else {
					mergeChildRespChan <- resp
				}
			}
		}()

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

		wg.Add(3)

		go func() {
			defer wg.Done()
			for req := range createPRChan {
				resp, fwdErr := client.SendCreatePRRequest(req)
				if fwdErr != nil {
					createPRRespChan <- mcp.CreatePRResponse{ID: req.ID, Success: false, Error: "Communication error with TUI"}
				} else {
					createPRRespChan <- resp
				}
			}
		}()

		go func() {
			defer wg.Done()
			for req := range pushBranchChan {
				resp, fwdErr := client.SendPushBranchRequest(req)
				if fwdErr != nil {
					pushBranchRespChan <- mcp.PushBranchResponse{ID: req.ID, Success: false, Error: "Communication error with TUI"}
				} else {
					pushBranchRespChan <- resp
				}
			}
		}()

		go func() {
			defer wg.Done()
			for req := range getReviewCommentsChan {
				resp, fwdErr := client.SendGetReviewCommentsRequest(req)
				if fwdErr != nil {
					getReviewCommentsRespChan <- mcp.GetReviewCommentsResponse{ID: req.ID, Success: false, Error: "Communication error with TUI"}
				} else {
					getReviewCommentsRespChan <- resp
				}
			}
		}()

		serverOpts = append(serverOpts, mcp.WithHostTools(
			createPRChan, createPRRespChan,
			pushBranchChan, pushBranchRespChan,
			getReviewCommentsChan, getReviewCommentsRespChan,
		))
	}

	// Run MCP server on stdin/stdout
	var allowedTools []string
	if autoApprove {
		allowedTools = []string{"*"}
	}
	server := mcp.NewServer(os.Stdin, os.Stdout, reqChan, respChan, questionChan, answerChan, planApprovalChan, planResponseChan, allowedTools, sessionID, serverOpts...)
	err = server.Run()

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
	if strings.HasPrefix(base, "pl-") {
		return strings.TrimPrefix(base, "pl-")
	}
	return ""
}
