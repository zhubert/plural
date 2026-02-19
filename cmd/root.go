package cmd

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"github.com/zhubert/plural/internal/app"
	"github.com/zhubert/plural-core/cli"
	"github.com/zhubert/plural-core/config"
	"github.com/zhubert/plural-core/logger"
)

var (
	debugMode             bool
	quietMode             bool
	version, commit, date string
)

// SetVersionInfo sets version information from ldflags
func SetVersionInfo(v, c, d string) {
	version, commit, date = v, c, d
}

var rootCmd = &cobra.Command{
	Use:   "plural",
	Short: "TUI for managing multiple concurrent Claude Code sessions",
	Long: `Plural is a TUI application for managing multiple concurrent Claude Code sessions.
Each session runs in its own git worktree, allowing isolated Claude conversations
on the same codebase.`,
	RunE:          runTUI,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", true, "Enable debug logging (on by default)")
	rootCmd.PersistentFlags().BoolVarP(&quietMode, "quiet", "q", false, "Reduce logging to info level only")
}

func initConfig() {
	if quietMode {
		logger.SetDebug(false)
	} else if debugMode {
		logger.SetDebug(true)
	}
}

// Execute runs the root command
func Execute() error {
	// Set version dynamically
	rootCmd.Version = version
	rootCmd.SetVersionTemplate(versionTemplate())
	return rootCmd.Execute()
}

func versionTemplate() string {
	if commit != "none" && commit != "" {
		return fmt.Sprintf("plural %s\n  commit: %s\n  built:  %s\n", version, commit, date)
	}
	return fmt.Sprintf("plural %s\n", version)
}

func runTUI(cmd *cobra.Command, args []string) error {
	// Validate prerequisites
	prereqs := cli.DefaultPrerequisites()
	if err := cli.ValidateRequired(prereqs); err != nil {
		return fmt.Errorf("%v\n\nInstall required tools and try again", err)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	// Ensure logger is closed on exit
	defer logger.Close()

	// Create and run the app
	m := app.New(cfg, version)
	defer m.Close()
	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running app: %w", err)
	}
	return nil
}
