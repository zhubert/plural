package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/zhubert/plural/internal/agent"
	"github.com/zhubert/plural/internal/cli"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/issues"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/session"
)

var (
	agentOnce                  bool
	agentRepo                  string
	agentMaxConcurrent         int
	agentMaxTurns              int
	agentMaxDuration           int
	agentAutoAddressPRComments bool
	agentAutoBroadcastPR       bool
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Run headless autonomous agent",
	Long: `Polls for issues and works them autonomously using containerized Claude sessions.

The agent runs headless (no TUI) and is suitable for CI/servers/background workers.
It uses the same configuration as the TUI (repos, allowed tools, auto-merge settings).

All sessions are containerized (container = sandbox).

Examples:
  plural agent                          # Continuous polling mode
  plural agent --once                   # Process available issues and exit
  plural agent --repo owner/repo        # Limit to specific repo (owner/repo or path)
  plural agent --max-turns 100          # Override max autonomous turns
  plural agent --max-duration 60        # Override max duration (minutes)`,
	RunE: runAgent,
}

func init() {
	agentCmd.Flags().BoolVar(&agentOnce, "once", false, "Process available issues and exit (vs continuous polling)")
	agentCmd.Flags().StringVar(&agentRepo, "repo", "", "Limit to specific repo (owner/repo or filesystem path)")
	agentCmd.Flags().IntVar(&agentMaxConcurrent, "max-concurrent", 0, "Override max concurrent sessions (0 = use config)")
	agentCmd.Flags().IntVar(&agentMaxTurns, "max-turns", 0, "Override max autonomous turns per session (0 = use config default of 50)")
	agentCmd.Flags().IntVar(&agentMaxDuration, "max-duration", 0, "Override max autonomous duration in minutes (0 = use config default of 30)")
	agentCmd.Flags().BoolVar(&agentAutoAddressPRComments, "auto-address-pr-comments", false, "Auto-address PR review comments")
	agentCmd.Flags().BoolVar(&agentAutoBroadcastPR, "auto-broadcast-pr", false, "Auto-create PRs when broadcast group completes")
	rootCmd.AddCommand(agentCmd)
}

func runAgent(cmd *cobra.Command, args []string) error {
	// Validate prerequisites
	prereqs := cli.DefaultPrerequisites()
	if err := cli.ValidateRequired(prereqs); err != nil {
		return fmt.Errorf("%v\n\nInstall required tools and try again", err)
	}

	// Check that docker is available (required for agent mode)
	dockerCheck := cli.Check(cli.Prerequisite{
		Name:        "docker",
		Required:    true,
		Description: "Docker (required for agent mode)",
		InstallURL:  "https://docs.docker.com/get-docker/",
	})
	if !dockerCheck.Found {
		return fmt.Errorf("docker is required for agent mode.\nInstall: https://docs.docker.com/get-docker/")
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	// Enable debug logging for agent mode (always on for headless autonomous operation)
	logger.SetDebug(true)

	// Ensure logger is closed on exit
	defer logger.Close()

	// Create structured logger for agent output (always debug level)
	agentLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create services
	gitSvc := git.NewGitService()
	sessSvc := session.NewSessionService()

	// Initialize issue providers
	githubProvider := issues.NewGitHubProvider(gitSvc)
	asanaProvider := issues.NewAsanaProvider(cfg)
	linearProvider := issues.NewLinearProvider(cfg)
	issueRegistry := issues.NewProviderRegistry(githubProvider, asanaProvider, linearProvider)

	// Build agent options
	var opts []agent.Option
	if agentOnce {
		opts = append(opts, agent.WithOnce(true))
	}
	if agentRepo != "" {
		opts = append(opts, agent.WithRepoFilter(agentRepo))
	}
	if agentMaxConcurrent > 0 {
		opts = append(opts, agent.WithMaxConcurrent(agentMaxConcurrent))
	}
	if agentMaxTurns > 0 {
		opts = append(opts, agent.WithMaxTurns(agentMaxTurns))
	}
	if agentMaxDuration > 0 {
		opts = append(opts, agent.WithMaxDuration(agentMaxDuration))
	}
	if agentAutoAddressPRComments {
		opts = append(opts, agent.WithAutoAddressPRComments(true))
	}
	if agentAutoBroadcastPR {
		opts = append(opts, agent.WithAutoBroadcastPR(true))
	}

	// Create agent
	a := agent.New(cfg, gitSvc, sessSvc, issueRegistry, agentLogger, opts...)

	// Set up signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		agentLogger.Info("received signal, shutting down gracefully", "signal", sig)
		cancel()
		// On second signal, force exit
		sig = <-sigCh
		agentLogger.Warn("received second signal, force exiting", "signal", sig)
		os.Exit(1)
	}()

	// Run agent
	return a.Run(ctx)
}
