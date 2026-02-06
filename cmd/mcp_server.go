package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/mcp"
)

var socketPath string

var mcpServerCmd = &cobra.Command{
	Use:    "mcp-server",
	Short:  "Run MCP server for permission prompts (internal use)",
	Hidden: true,
	RunE:   runMCPServer,
}

func init() {
	mcpServerCmd.Flags().StringVar(&socketPath, "socket", "", "Unix socket path for TUI communication")
	mcpServerCmd.MarkFlagRequired("socket")
	rootCmd.AddCommand(mcpServerCmd)
}

func runMCPServer(cmd *cobra.Command, args []string) error {
	// Extract session ID from socket path (e.g., /tmp/plural-<session-id>.sock)
	sessionID := extractSessionID(socketPath)
	if sessionID != "" {
		if err := logger.Init(logger.MCPLogPath(sessionID)); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}
	defer logger.Close()

	// Connect to TUI socket
	client, err := mcp.NewSocketClient(socketPath)
	if err != nil {
		return fmt.Errorf("error connecting to TUI socket: %w", err)
	}
	defer client.Close()

	// Create channels for MCP server communication
	reqChan := make(chan mcp.PermissionRequest)
	respChan := make(chan mcp.PermissionResponse)
	questionChan := make(chan mcp.QuestionRequest)
	answerChan := make(chan mcp.QuestionResponse)
	planApprovalChan := make(chan mcp.PlanApprovalRequest)
	planResponseChan := make(chan mcp.PlanApprovalResponse)

	// Start goroutine to handle permission requests via socket
	go func() {
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

	// Start goroutine to handle question requests via socket
	go func() {
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

	// Start goroutine to handle plan approval requests via socket
	go func() {
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

	// Run MCP server on stdin/stdout
	server := mcp.NewServer(os.Stdin, os.Stdout, reqChan, respChan, questionChan, answerChan, planApprovalChan, planResponseChan, nil, sessionID)
	err = server.Run()

	// Close request channels so the forwarding goroutines exit their range loops
	close(reqChan)
	close(questionChan)
	close(planApprovalChan)

	if err != nil {
		return fmt.Errorf("MCP server error: %w", err)
	}
	return nil
}

// extractSessionID extracts the session ID from a socket path like /tmp/plural-<session-id>.sock
func extractSessionID(socketPath string) string {
	base := filepath.Base(socketPath)
	// Remove .sock extension
	base = strings.TrimSuffix(base, ".sock")
	// Remove plural- prefix
	if strings.HasPrefix(base, "plural-") {
		return strings.TrimPrefix(base, "plural-")
	}
	return ""
}
