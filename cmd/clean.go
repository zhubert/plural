package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/process"
	"github.com/zhubert/plural/internal/session"
)

var skipConfirm bool

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove all sessions, logs, orphaned worktrees, and containers",
	Long: `Clears all session data, removes log files, prunes orphaned worktrees,
kills any orphaned Claude processes, and removes orphaned containers.

This command combines the functionality of the former --clear and --prune flags.
It will prompt for confirmation before proceeding unless the --yes flag is used.`,
	RunE: runClean,
}

func init() {
	cleanCmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip confirmation prompt")
	rootCmd.AddCommand(cleanCmd)
}

func runClean(cmd *cobra.Command, args []string) error {
	return runCleanWithReader(os.Stdin)
}

// runCleanWithReader allows injecting a reader for testing
func runCleanWithReader(input io.Reader) error {
	// Load config to show what will be cleaned
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	// Gather statistics about what will be cleaned
	sessionCount := len(cfg.GetSessions())

	// Create session service for orphan detection
	sessionService := session.NewSessionService()
	ctx := context.Background()

	orphanWorktrees, err := sessionService.FindOrphanedWorktrees(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: error finding orphaned worktrees: %v\n", err)
	}

	orphanMessages, err := config.FindOrphanedSessionMessages(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: error finding orphaned session messages: %v\n", err)
	}

	// Build set of known session IDs for process cleanup
	knownSessions := make(map[string]bool)
	for _, sess := range cfg.GetSessions() {
		knownSessions[sess.ID] = true
	}

	orphanProcesses, err := process.FindOrphanedClaudeProcesses(knownSessions)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: error finding orphaned processes: %v\n", err)
	}

	orphanContainers, err := process.FindOrphanedContainers(knownSessions)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: error finding orphaned containers: %v\n", err)
	}

	// Check if there's anything to clean
	if sessionCount == 0 && len(orphanWorktrees) == 0 && len(orphanMessages) == 0 && len(orphanProcesses) == 0 && len(orphanContainers) == 0 {
		fmt.Println("Nothing to clean.")
		return nil
	}

	// Print summary of what will be cleaned
	fmt.Println("This will clean:")
	if sessionCount > 0 {
		fmt.Printf("  - %d session(s)\n", sessionCount)
	}
	if len(orphanWorktrees) > 0 {
		fmt.Printf("  - %d orphaned worktree(s)\n", len(orphanWorktrees))
		for _, orphan := range orphanWorktrees {
			fmt.Printf("      %s\n", orphan.Path)
		}
	}
	if len(orphanMessages) > 0 {
		fmt.Printf("  - %d orphaned session file(s)\n", len(orphanMessages))
	}
	if len(orphanProcesses) > 0 {
		fmt.Printf("  - %d orphaned process(es)\n", len(orphanProcesses))
		for _, proc := range orphanProcesses {
			fmt.Printf("      PID %d\n", proc.PID)
		}
	}
	if len(orphanContainers) > 0 {
		fmt.Printf("  - %d orphaned container(s)\n", len(orphanContainers))
		for _, c := range orphanContainers {
			fmt.Printf("      %s\n", c.Name)
		}
	}
	fmt.Println("  - All log files in ~/.plural/logs")

	// Confirm unless --yes flag is set
	if !skipConfirm {
		if !confirm(input, "Continue?") {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Clear sessions (from --clear)
	cfg.ClearSessions()
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("error saving config: %w", err)
	}

	logsCleared, err := logger.ClearLogs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: error clearing logs: %v\n", err)
	}

	messagesCleared, err := config.ClearAllSessionMessages()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: error clearing session messages: %v\n", err)
	}

	// Prune orphans in parallel (from --prune)

	var prunedWorktrees, prunedMessages, prunedProcesses, prunedContainers int
	var worktreesErr, messagesErr, processesErr, containersErr error

	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		prunedWorktrees, worktreesErr = sessionService.PruneOrphanedWorktrees(ctx, cfg)
	}()

	go func() {
		defer wg.Done()
		prunedMessages, messagesErr = config.PruneOrphanedSessionMessages(cfg)
	}()

	go func() {
		defer wg.Done()
		prunedProcesses, processesErr = process.CleanupOrphanedProcesses(knownSessions)
	}()

	go func() {
		defer wg.Done()
		prunedContainers, containersErr = process.CleanupOrphanedContainers(knownSessions)
	}()

	wg.Wait()

	// Report any errors
	if worktreesErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: error pruning worktrees: %v\n", worktreesErr)
	}
	if messagesErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: error pruning session messages: %v\n", messagesErr)
	}
	if processesErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: error killing orphaned processes: %v\n", processesErr)
	}
	if containersErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: error removing orphaned containers: %v\n", containersErr)
	}

	// Print results
	fmt.Println()
	fmt.Println("Cleaned:")
	if sessionCount > 0 {
		fmt.Printf("  - %d session(s) cleared\n", sessionCount)
	}
	if logsCleared > 0 {
		fmt.Printf("  - %d log file(s) removed\n", logsCleared)
	}
	if messagesCleared > 0 {
		fmt.Printf("  - %d session message file(s) removed\n", messagesCleared)
	}
	if prunedWorktrees > 0 {
		fmt.Printf("  - %d orphaned worktree(s) pruned\n", prunedWorktrees)
	}
	if prunedMessages > 0 {
		fmt.Printf("  - %d orphaned session file(s) pruned\n", prunedMessages)
	}
	if prunedProcesses > 0 {
		fmt.Printf("  - %d orphaned process(es) killed\n", prunedProcesses)
	}
	if prunedContainers > 0 {
		fmt.Printf("  - %d orphaned container(s) removed\n", prunedContainers)
	}

	return nil
}

// confirm prompts the user for y/n confirmation
func confirm(input io.Reader, prompt string) bool {
	reader := bufio.NewReader(input)
	fmt.Printf("%s [y/N]: ", prompt)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes"
}
