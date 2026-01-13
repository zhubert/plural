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
	"github.com/zhubert/plural/internal/session"
)

// Version information set via ldflags at build time
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Check for subcommand
	if len(os.Args) > 1 && os.Args[1] == "mcp-server" {
		runMCPServer()
		return
	}

	// Custom usage function for standard help format
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `plural - TUI for managing multiple concurrent Claude Code sessions

Usage: plural [options]

Options:
  -v, --version        Print version information and exit
  -h, --help           Show this help message
      --debug          Enable debug logging (verbose output to /tmp/plural-debug.log)
      --clear          Remove all sessions and log files, then exit
      --check-prereqs  Check CLI prerequisites and exit
      --prune          Remove orphaned worktrees (worktrees without matching sessions)

For more information, visit: https://github.com/zhubert/plural
`)
	}

	showVersion := flag.Bool("version", false, "Print version information and exit")
	flag.BoolVar(showVersion, "v", false, "Print version information and exit")
	debugMode := flag.Bool("debug", false, "Enable debug logging")
	clearSessions := flag.Bool("clear", false, "Remove all sessions and exit")
	checkPrereqs := flag.Bool("check-prereqs", false, "Check CLI prerequisites and exit")
	pruneWorktrees := flag.Bool("prune", false, "Remove orphaned worktrees (worktrees without matching sessions)")
	flag.Parse()

	// Set debug logging level if requested
	if *debugMode {
		logger.SetDebug(true)
	}

	// Handle version flag
	if *showVersion {
		fmt.Printf("plural %s\n", version)
		if commit != "none" {
			fmt.Printf("  commit: %s\n", commit)
			fmt.Printf("  built:  %s\n", date)
		}
		os.Exit(0)
	}

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
		logsCleared, err := logger.ClearLogs()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: error clearing logs: %v\n", err)
		}
		messagesCleared, err := config.ClearAllSessionMessages()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: error clearing session messages: %v\n", err)
		}
		fmt.Println("All sessions cleared.")
		if logsCleared > 0 {
			fmt.Printf("Removed %d log file(s).\n", logsCleared)
		}
		if messagesCleared > 0 {
			fmt.Printf("Removed %d session message file(s).\n", messagesCleared)
		}
		return
	}

	// Handle prune flag
	if *pruneWorktrees {
		orphanWorktrees, err := session.FindOrphanedWorktrees(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error finding orphaned worktrees: %v\n", err)
			os.Exit(1)
		}

		orphanMessages, err := config.FindOrphanedSessionMessages(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error finding orphaned session messages: %v\n", err)
			os.Exit(1)
		}

		if len(orphanWorktrees) == 0 && len(orphanMessages) == 0 {
			fmt.Println("No orphaned worktrees or session files found.")
			return
		}

		if len(orphanWorktrees) > 0 {
			fmt.Printf("Found %d orphaned worktree(s):\n", len(orphanWorktrees))
			for _, orphan := range orphanWorktrees {
				fmt.Printf("  - %s\n", orphan.Path)
			}
		}

		if len(orphanMessages) > 0 {
			fmt.Printf("Found %d orphaned session message file(s):\n", len(orphanMessages))
			for _, sessionID := range orphanMessages {
				fmt.Printf("  - %s.json\n", sessionID)
			}
		}

		prunedWorktrees, err := session.PruneOrphanedWorktrees(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error pruning worktrees: %v\n", err)
			os.Exit(1)
		}

		prunedMessages, err := config.PruneOrphanedSessionMessages(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error pruning session messages: %v\n", err)
			os.Exit(1)
		}

		if prunedWorktrees > 0 {
			fmt.Printf("Pruned %d worktree(s).\n", prunedWorktrees)
		}
		if prunedMessages > 0 {
			fmt.Printf("Pruned %d session message file(s).\n", prunedMessages)
		}
		return
	}

	// Ensure logger is closed on exit
	defer logger.Close()

	// Create and run the app
	m := app.New(cfg, version)
	defer m.Close() // Gracefully shut down all Claude sessions on exit
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
		if err := logger.Init(logger.MCPLogPath(sessionID)); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
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
	questionChan := make(chan mcp.QuestionRequest)
	answerChan := make(chan mcp.QuestionResponse)

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

	// Run MCP server on stdin/stdout
	server := mcp.NewServer(os.Stdin, os.Stdout, reqChan, respChan, questionChan, answerChan, nil)
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
