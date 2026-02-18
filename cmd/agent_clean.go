package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/zhubert/plural/internal/agent"
)

var agentCleanSkipConfirm bool

var agentCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove agent daemon state and lock files",
	Long: `Clears daemon state (work item tracking) and removes lock files.

This is useful when the daemon state becomes stale or corrupted,
or when a lock file is left behind after an unclean shutdown.

It will prompt for confirmation before proceeding unless the --yes flag is used.`,
	RunE: runAgentClean,
}

func init() {
	agentCleanCmd.Flags().BoolVarP(&agentCleanSkipConfirm, "yes", "y", false, "Skip confirmation prompt")
	agentCmd.AddCommand(agentCleanCmd)
}

func runAgentClean(cmd *cobra.Command, args []string) error {
	return runAgentCleanWithReader(os.Stdin)
}

func runAgentCleanWithReader(input io.Reader) error {
	// Check what exists
	stateExists := agent.DaemonStateExists()
	lockFiles, err := agent.FindDaemonLocks()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: error finding lock files: %v\n", err)
	}

	if !stateExists && len(lockFiles) == 0 {
		fmt.Println("Nothing to clean.")
		return nil
	}

	// Print summary
	fmt.Println("This will clean:")
	if stateExists {
		fmt.Println("  - Daemon state file (daemon-state.json)")
	}
	if len(lockFiles) > 0 {
		fmt.Printf("  - %d daemon lock file(s)\n", len(lockFiles))
		for _, lf := range lockFiles {
			fmt.Printf("      %s\n", lf)
		}
		fmt.Println()
		fmt.Println("  Warning: lock files indicate a daemon may be running.")
		fmt.Println("  Cleaning while a daemon is active can cause issues.")
	}

	// Confirm
	if !agentCleanSkipConfirm {
		if !confirm(input, "Continue?") {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Clean state file
	var stateRemoved bool
	if stateExists {
		if err := agent.ClearDaemonState(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: error removing state file: %v\n", err)
		} else {
			stateRemoved = true
		}
	}

	// Clean lock files
	locksRemoved, err := agent.ClearDaemonLocks()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: error removing lock files: %v\n", err)
	}

	// Print results
	fmt.Println()
	fmt.Println("Cleaned:")
	if stateRemoved {
		fmt.Println("  - Daemon state file removed")
	}
	if locksRemoved > 0 {
		fmt.Printf("  - %d lock file(s) removed\n", locksRemoved)
	}

	return nil
}
