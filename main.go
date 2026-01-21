package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/app"
	"github.com/zhubert/plural/internal/cli"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/demo"
	"github.com/zhubert/plural/internal/demo/scenarios"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/mcp"
	"github.com/zhubert/plural/internal/process"
	"github.com/zhubert/plural/internal/session"
)

// Version information set via ldflags at build time
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Check for subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "mcp-server":
			runMCPServer()
			return
		case "demo":
			runDemo()
			return
		}
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

		// Build set of known session IDs
		knownSessions := make(map[string]bool)
		for _, sess := range cfg.GetSessions() {
			knownSessions[sess.ID] = true
		}

		// Find orphaned Claude processes
		orphanProcesses, err := process.FindOrphanedClaudeProcesses(knownSessions)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error finding orphaned processes: %v\n", err)
			// Non-fatal, continue with other cleanup
		}

		if len(orphanWorktrees) == 0 && len(orphanMessages) == 0 && len(orphanProcesses) == 0 {
			fmt.Println("No orphaned worktrees, session files, or processes found.")
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

		if len(orphanProcesses) > 0 {
			fmt.Printf("Found %d orphaned Claude process(es):\n", len(orphanProcesses))
			for _, proc := range orphanProcesses {
				fmt.Printf("  - PID %d\n", proc.PID)
			}
		}

		sessionSvc := session.NewSessionService()
		ctx := context.Background()
		prunedWorktrees, err := sessionSvc.PruneOrphanedWorktrees(ctx, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error pruning worktrees: %v\n", err)
			os.Exit(1)
		}

		prunedMessages, err := config.PruneOrphanedSessionMessages(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error pruning session messages: %v\n", err)
			os.Exit(1)
		}

		prunedProcesses, err := process.CleanupOrphanedProcesses(knownSessions)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error killing orphaned processes: %v\n", err)
			// Non-fatal, continue
		}

		if prunedWorktrees > 0 {
			fmt.Printf("Pruned %d worktree(s).\n", prunedWorktrees)
		}
		if prunedMessages > 0 {
			fmt.Printf("Pruned %d session message file(s).\n", prunedMessages)
		}
		if prunedProcesses > 0 {
			fmt.Printf("Killed %d orphaned process(es).\n", prunedProcesses)
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

// runDemo runs the demo generation subcommand
func runDemo() {
	demoCmd := flag.NewFlagSet("demo", flag.ExitOnError)
	demoCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, `plural demo - Generate demo recordings of Plural

Usage: plural demo <command> [options]

Commands:
  list                    List available demo scenarios
  run <scenario>          Run a scenario and output to stdout (for testing)
  generate <scenario>     Generate a VHS tape file for rendering
  cast <scenario>         Generate an asciinema cast file

Options:
  -o, --output <file>     Output file (default: demo.tape, demo.cast, etc.)
  -w, --width <int>       Terminal width (default: 120)
  -h, --height <int>      Terminal height (default: 40)
      --capture-all       Capture frame after every step (for debugging)

Examples:
  plural demo list
  plural demo generate basic -o demo.tape
  plural demo cast basic -o demo.cast

After generating a .tape file, render with VHS:
  vhs demo.tape

After generating a .cast file, play with asciinema:
  asciinema play demo.cast
`)
	}

	if len(os.Args) < 3 {
		demoCmd.Usage()
		os.Exit(1)
	}

	command := os.Args[2]

	// Parse flags - need to handle scenario name which may appear before or after flags
	// Go's flag package stops at first non-flag arg, so we extract scenario first
	output := demoCmd.String("output", "", "Output file")
	demoCmd.StringVar(output, "o", "", "Output file")
	width := demoCmd.Int("width", 120, "Terminal width")
	demoCmd.IntVar(width, "w", 120, "Terminal width")
	height := demoCmd.Int("height", 40, "Terminal height")
	demoCmd.IntVar(height, "h", 40, "Terminal height")
	captureAll := demoCmd.Bool("capture-all", false, "Capture frame after every step (for debugging)")

	// Separate scenario name from flags so flags can appear after scenario name
	// e.g., "plural demo cast basic -o foo.cast" should work
	remainingArgs := os.Args[3:]
	var scenarioName string
	var flagArgs []string
	for i := 0; i < len(remainingArgs); i++ {
		arg := remainingArgs[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			// If this flag takes a value, include the next arg too
			if i+1 < len(remainingArgs) && !strings.HasPrefix(remainingArgs[i+1], "-") &&
				(arg == "-o" || arg == "--output" || arg == "-w" || arg == "--width" || arg == "-h" || arg == "--height") {
				i++
				flagArgs = append(flagArgs, remainingArgs[i])
			}
		} else if scenarioName == "" {
			scenarioName = arg
		}
	}
	demoCmd.Parse(flagArgs)

	switch command {
	case "list":
		fmt.Println("Available demo scenarios:")
		fmt.Println()
		for _, s := range scenarios.All() {
			fmt.Printf("  %-15s %s\n", s.Name, s.Description)
		}
		return

	case "run", "generate", "cast":
		if scenarioName == "" {
			fmt.Fprintf(os.Stderr, "Error: scenario name required\n")
			fmt.Fprintf(os.Stderr, "Run 'plural demo list' to see available scenarios\n")
			os.Exit(1)
		}

		scenario := scenarios.Get(scenarioName)
		if scenario == nil {
			fmt.Fprintf(os.Stderr, "Error: unknown scenario %q\n", scenarioName)
			fmt.Fprintf(os.Stderr, "Run 'plural demo list' to see available scenarios\n")
			os.Exit(1)
		}

		// Override dimensions if specified
		if *width > 0 {
			scenario.Width = *width
		}
		if *height > 0 {
			scenario.Height = *height
		}

		// Configure executor
		execCfg := demo.DefaultExecutorConfig()
		execCfg.CaptureEveryStep = *captureAll

		// Run the scenario
		executor := demo.NewExecutor(execCfg)
		frames, err := executor.Run(scenario)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error running scenario: %v\n", err)
			os.Exit(1)
		}

		switch command {
		case "run":
			// Just print frames to stdout for testing
			fmt.Printf("Captured %d frames\n", len(frames))
			for i, f := range frames {
				fmt.Printf("\n=== Frame %d (delay: %v) ===\n", i, f.Delay)
				if f.Annotation != "" {
					fmt.Printf("Annotation: %s\n", f.Annotation)
				}
				fmt.Println(f.Content)
			}

		case "generate":
			// Generate VHS tape
			outputFile := *output
			if outputFile == "" {
				outputFile = scenarioName + ".tape"
			}

			f, err := os.Create(outputFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
				os.Exit(1)
			}
			defer f.Close()

			vhsCfg := demo.DefaultVHSConfig()
			vhsCfg.Output = strings.TrimSuffix(outputFile, ".tape") + ".gif"
			vhsCfg.Width = scenario.Width
			vhsCfg.Height = scenario.Height

			if err := demo.GenerateVHSTape(f, frames, vhsCfg); err != nil {
				fmt.Fprintf(os.Stderr, "Error generating VHS tape: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("Generated %s (%d frames)\n", outputFile, len(frames))
			fmt.Printf("Render with: vhs %s\n", outputFile)

		case "cast":
			// Generate asciinema cast
			outputFile := *output
			if outputFile == "" {
				outputFile = scenarioName + ".cast"
			}

			f, err := os.Create(outputFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
				os.Exit(1)
			}
			defer f.Close()

			if err := demo.GenerateASCIICast(f, frames, scenario.Width, scenario.Height); err != nil {
				fmt.Fprintf(os.Stderr, "Error generating cast file: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("Generated %s (%d frames)\n", outputFile, len(frames))
			fmt.Printf("Play with: asciinema play %s\n", outputFile)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown demo command: %s\n", command)
		demoCmd.Usage()
		os.Exit(1)
	}
}
