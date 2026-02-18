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
	agentAutoMerge             bool
	agentNoAutoMerge           bool
	agentMergeMethod           string
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Run headless autonomous agent daemon",
	Long: `Persistent orchestrator daemon that manages the full lifecycle of work items:
picking up issues, coding, PR creation, review feedback cycles, and final merge.

The daemon is stoppable and restartable without losing track of in-flight work.
State is persisted to ~/.plural/daemon-state.json.

The --repo flag is required to specify which registered repo to poll.
Auto-merge is enabled by default; use --no-auto-merge to disable.

All sessions are containerized (container = sandbox).

Examples:
  plural agent --repo owner/repo              # Run daemon (long-running, auto-merge on)
  plural agent --repo owner/repo --once       # Process one tick and exit
  plural agent --repo /path/to/repo           # Use filesystem path instead
  plural agent --repo owner/repo --no-auto-merge  # Disable auto-merge
  plural agent --repo owner/repo --max-turns 100`,
	RunE: runAgent,
}

func init() {
	agentCmd.Flags().BoolVar(&agentOnce, "once", false, "Run one tick and exit (vs continuous daemon)")
	agentCmd.Flags().StringVar(&agentRepo, "repo", "", "Repo to poll (owner/repo or filesystem path)")
	agentCmd.Flags().IntVar(&agentMaxConcurrent, "max-concurrent", 0, "Override max concurrent sessions (0 = use config)")
	agentCmd.Flags().IntVar(&agentMaxTurns, "max-turns", 0, "Override max autonomous turns per session (0 = use config default of 50)")
	agentCmd.Flags().IntVar(&agentMaxDuration, "max-duration", 0, "Override max autonomous duration in minutes (0 = use config default of 30)")
	agentCmd.Flags().BoolVar(&agentAutoAddressPRComments, "auto-address-pr-comments", false, "Auto-address PR review comments")
	agentCmd.Flags().BoolVar(&agentAutoBroadcastPR, "auto-broadcast-pr", false, "Auto-create PRs when broadcast group completes")
	agentCmd.Flags().BoolVar(&agentAutoMerge, "auto-merge", false, "Auto-merge PRs after review approval and CI pass (default: true)")
	agentCmd.Flags().BoolVar(&agentNoAutoMerge, "no-auto-merge", false, "Disable auto-merge")
	agentCmd.Flags().StringVar(&agentMergeMethod, "merge-method", "", "Merge method: rebase, squash, or merge (default: rebase)")
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

	// --repo is required
	if agentRepo == "" {
		return fmt.Errorf("--repo is required: specify which repo to poll (owner/repo or filesystem path)")
	}

	// Build daemon options
	var opts []agent.DaemonOption
	if agentOnce {
		opts = append(opts, agent.WithDaemonOnce(true))
	}
	opts = append(opts, agent.WithDaemonRepoFilter(agentRepo))
	if agentMaxConcurrent > 0 {
		opts = append(opts, agent.WithDaemonMaxConcurrent(agentMaxConcurrent))
	}
	if agentMaxTurns > 0 {
		opts = append(opts, agent.WithDaemonMaxTurns(agentMaxTurns))
	}
	if agentMaxDuration > 0 {
		opts = append(opts, agent.WithDaemonMaxDuration(agentMaxDuration))
	}
	if agentAutoAddressPRComments {
		opts = append(opts, agent.WithDaemonAutoAddressPRComments(true))
	}
	if agentAutoBroadcastPR {
		opts = append(opts, agent.WithDaemonAutoBroadcastPR(true))
	}
	// Auto-merge is on by default for daemon; --no-auto-merge disables it
	if agentNoAutoMerge {
		opts = append(opts, agent.WithDaemonAutoMerge(false))
	}
	if agentMergeMethod != "" {
		opts = append(opts, agent.WithDaemonMergeMethod(agentMergeMethod))
	}

	// Create daemon
	d := agent.NewDaemon(cfg, gitSvc, sessSvc, issueRegistry, agentLogger, opts...)

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

	// Run daemon
	return d.Run(ctx)
}
