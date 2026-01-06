package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/app"
	"github.com/zhubert/plural/internal/cli"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/mcp"
)

func main() {
	// Check for subcommand
	if len(os.Args) > 1 && os.Args[1] == "mcp-server" {
		runMCPServer()
		return
	}

	clearSessions := flag.Bool("clear", false, "Remove all sessions and exit")
	checkPrereqs := flag.Bool("check-prereqs", false, "Check CLI prerequisites and exit")
	flag.Parse()

	// Check prerequisites
	prereqs := cli.DefaultPrerequisites()

	if *checkPrereqs {
		results := cli.CheckAll(prereqs)
		fmt.Print(cli.FormatCheckResults(results))
		os.Exit(0)
	}

	// Validate required prerequisites before starting
	if err := cli.ValidateRequired(prereqs); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "\nRun 'plural --check-prereqs' to see all prerequisites.\n")
		os.Exit(1)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Handle clear flag
	if *clearSessions {
		cfg.ClearSessions()
		if err := cfg.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("All sessions cleared.")
		return
	}

	// Ensure logger is closed on exit
	defer logger.Close()

	// Create and run the app
	m := app.New(cfg)
	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running app: %v\n", err)
		os.Exit(1)
	}
}

// runMCPServer runs the MCP server subprocess for handling permission prompts
func runMCPServer() {
	mcpCmd := flag.NewFlagSet("mcp-server", flag.ExitOnError)
	socketPath := mcpCmd.String("socket", "", "Unix socket path for TUI communication")
	mcpCmd.Parse(os.Args[2:])

	if *socketPath == "" {
		fmt.Fprintf(os.Stderr, "Error: --socket is required\n")
		os.Exit(1)
	}

	// Extract session ID from socket path (e.g., /tmp/plural-<session-id>.sock)
	sessionID := extractSessionID(*socketPath)
	if sessionID != "" {
		logger.Init(logger.MCPLogPath(sessionID))
	}
	defer logger.Close()

	// Connect to TUI socket
	client, err := mcp.NewSocketClient(*socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to TUI socket: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// Create channels for MCP server communication
	reqChan := make(chan mcp.PermissionRequest)
	respChan := make(chan mcp.PermissionResponse)

	// Start goroutine to handle permission requests via socket
	go func() {
		for req := range reqChan {
			resp, err := client.SendRequest(req)
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

	// Run MCP server on stdin/stdout
	server := mcp.NewServer(os.Stdin, os.Stdout, reqChan, respChan, nil)
	if err := server.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
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
