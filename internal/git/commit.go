package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/zhubert/plural/internal/logger"
)

// Configuration constants for commit operations
const (
	// MaxDiffSize is the maximum number of characters to include in a diff.
	// This prevents excessive memory usage when Claude is analyzing changes.
	// Claude's context window can handle much more, but large diffs slow down
	// commit message generation and rarely provide additional value.
	// 50KB is enough to capture meaningful changes while staying responsive.
	MaxDiffSize = 50000
)

// CommitAll stages all changes and commits them with the given message
func (s *GitService) CommitAll(ctx context.Context, worktreePath, message string) error {
	logger.WithComponent("git").Info("committing all changes", "worktree", worktreePath)

	// Stage all changes
	if output, err := s.executor.CombinedOutput(ctx, worktreePath, "git", "add", "-A"); err != nil {
		return fmt.Errorf("git add failed: %s - %w", string(output), err)
	}

	// Commit
	if output, err := s.executor.CombinedOutput(ctx, worktreePath, "git", "commit", "-m", message); err != nil {
		return fmt.Errorf("git commit failed: %s - %w", string(output), err)
	}

	return nil
}

// GenerateCommitMessage creates a commit message based on the changes (simple fallback)
func (s *GitService) GenerateCommitMessage(ctx context.Context, worktreePath string) (string, error) {
	status, err := s.GetWorktreeStatus(ctx, worktreePath)
	if err != nil {
		return "", err
	}

	if !status.HasChanges {
		return "", fmt.Errorf("no changes to commit")
	}

	// Get the diff stats for a better message (use --no-ext-diff to ensure output goes to stdout)
	statOutput, err := s.executor.Output(ctx, worktreePath, "git", "diff", "--no-ext-diff", "--stat", "HEAD")
	if err != nil {
		logger.WithComponent("git").Warn("git diff --stat failed", "error", err, "worktree", worktreePath)
	}

	// Create a simple but descriptive message
	var message strings.Builder
	message.WriteString(fmt.Sprintf("Plural session changes\n\n%s\n\nFiles:\n", status.Summary))
	for _, file := range status.Files {
		message.WriteString(fmt.Sprintf("- %s\n", file))
	}

	if len(statOutput) > 0 {
		message.WriteString(fmt.Sprintf("\nStats:\n%s", string(statOutput)))
	}

	return message.String(), nil
}

// GenerateCommitMessageWithClaude uses Claude to generate a commit message from the diff
func (s *GitService) GenerateCommitMessageWithClaude(ctx context.Context, worktreePath string) (string, error) {
	log := logger.WithComponent("git")
	log.Info("generating commit message with Claude", "worktree", worktreePath)

	status, err := s.GetWorktreeStatus(ctx, worktreePath)
	if err != nil {
		return "", err
	}

	if !status.HasChanges {
		return "", fmt.Errorf("no changes to commit")
	}

	// Get the full diff for Claude to analyze (use --no-ext-diff to ensure output goes to stdout)
	diffOutput, err := s.executor.Output(ctx, worktreePath, "git", "diff", "--no-ext-diff", "HEAD")
	if err != nil {
		// Try without HEAD for new repos
		log.Debug("diff HEAD failed, trying without HEAD", "error", err, "worktree", worktreePath)
		diffOutput, err = s.executor.Output(ctx, worktreePath, "git", "diff", "--no-ext-diff")
		if err != nil {
			log.Warn("git diff failed", "error", err, "worktree", worktreePath)
		}
	}

	// Also get staged changes
	cachedOutput, err := s.executor.Output(ctx, worktreePath, "git", "diff", "--no-ext-diff", "--cached")
	if err != nil {
		log.Warn("git diff --cached failed", "error", err, "worktree", worktreePath)
	}

	fullDiff := string(diffOutput) + string(cachedOutput)

	// Truncate diff if too large (Claude has context limits)
	maxDiffSize := MaxDiffSize
	if len(fullDiff) > maxDiffSize {
		fullDiff = fullDiff[:maxDiffSize] + "\n... (diff truncated)"
	}

	// Build the prompt for Claude
	prompt := fmt.Sprintf(`Generate a git commit message for the following changes. Follow these rules:
1. First line: Short summary (max 72 chars) in imperative mood (e.g., "Add feature", "Fix bug", "Update config")
2. Blank line after summary
3. Optional body: Explain the "why" not the "what" (the diff shows what changed)
4. Focus on the purpose and impact of the changes
5. Be concise - only add body if the changes are complex enough to warrant explanation
6. Do NOT include any preamble like "Here's a commit message:" - just output the commit message directly

Changed files: %s

Diff:
%s`, strings.Join(status.Files, ", "), fullDiff)

	// Call Claude CLI directly with --print for a simple response
	output, err := s.executor.Output(ctx, worktreePath, "claude", "--print", "-p", prompt)
	if err != nil {
		log.Error("Claude commit message generation failed", "error", err)
		return "", fmt.Errorf("failed to generate commit message with Claude: %w", err)
	}

	commitMsg := strings.TrimSpace(string(output))
	if commitMsg == "" {
		return "", fmt.Errorf("Claude returned empty commit message")
	}

	log.Info("generated commit message", "title", strings.Split(commitMsg, "\n")[0])
	return commitMsg, nil
}

// CommitConflictResolution stages all changes and commits with the given message.
// This is used after resolving merge conflicts to complete the merge.
func (s *GitService) CommitConflictResolution(ctx context.Context, repoPath, message string) error {
	logger.WithComponent("git").Info("committing conflict resolution", "repoPath", repoPath)

	// Stage all changes
	if output, err := s.executor.CombinedOutput(ctx, repoPath, "git", "add", "-A"); err != nil {
		return fmt.Errorf("git add failed: %s - %w", string(output), err)
	}

	// Commit
	if output, err := s.executor.CombinedOutput(ctx, repoPath, "git", "commit", "-m", message); err != nil {
		return fmt.Errorf("git commit failed: %s - %w", string(output), err)
	}

	return nil
}

// EnsureCommitted checks for uncommitted changes and commits them if present.
// If commitMsg is empty, it generates a commit message using Claude (with fallback).
// Returns true if the operation succeeded (either committed or no changes needed),
// false if there was an error (error is sent to the result channel).
func (s *GitService) EnsureCommitted(ctx context.Context, ch chan<- Result, worktreePath, commitMsg string) bool {
	log := logger.WithComponent("git")

	status, err := s.GetWorktreeStatus(ctx, worktreePath)
	if err != nil {
		ch <- Result{Error: fmt.Errorf("failed to check worktree status: %w", err), Done: true}
		return false
	}

	if !status.HasChanges {
		log.Debug("no uncommitted changes in worktree", "worktree", worktreePath)
		ch <- Result{Output: "No uncommitted changes in worktree\n\n"}
		return true
	}

	// Report that we found uncommitted changes
	ch <- Result{Output: fmt.Sprintf("Found uncommitted changes (%s)\n", status.Summary)}

	// Generate commit message if not provided
	if commitMsg == "" {
		ch <- Result{Output: "Generating commit message with Claude...\n"}
		commitMsg, err = s.GenerateCommitMessageWithClaude(ctx, worktreePath)
		if err != nil {
			log.Warn("Claude commit message generation failed, using fallback", "error", err)
			commitMsg, err = s.GenerateCommitMessage(ctx, worktreePath)
			if err != nil {
				ch <- Result{Error: fmt.Errorf("failed to generate commit message: %w", err), Done: true}
				return false
			}
		}
	}

	// Commit the changes
	ch <- Result{Output: fmt.Sprintf("Committing changes in worktree...\n")}
	if err := s.CommitAll(ctx, worktreePath, commitMsg); err != nil {
		ch <- Result{Error: fmt.Errorf("failed to commit changes: %w", err), Done: true}
		return false
	}
	ch <- Result{Output: "Changes committed.\n"}

	return true
}
