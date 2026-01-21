package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/zhubert/plural/internal/logger"
)

// CommitResult represents the result of a commit operation
type CommitResult struct {
	// CommitMsg contains the final commit message used (may be generated or provided)
	CommitMsg string
	// Error is set if the commit operation failed
	Error error
	// HadChanges indicates whether there were changes to commit
	HadChanges bool
}

// CommitWithMessage commits any uncommitted changes using the provided message
// or generates one using Claude (with fallback to simple message generation).
//
// This helper consolidates the commit message generation/application pattern
// used across MergeToMain, CreatePR, MergeToParent, and PushUpdates.
//
// Returns:
//   - CommitResult with the commit message used and any error
//   - The callback function is called with progress messages
func (s *GitService) CommitWithMessage(
	ctx context.Context,
	worktreePath string,
	commitMsg string,
	progressFn func(msg string),
	errorFn func(err error),
) CommitResult {
	result := CommitResult{}

	// Check for uncommitted changes
	status, err := s.GetWorktreeStatus(ctx, worktreePath)
	if err != nil {
		result.Error = fmt.Errorf("failed to get worktree status: %w", err)
		return result
	}

	if !status.HasChanges {
		progressFn("No uncommitted changes in worktree\n\n")
		result.HadChanges = false
		return result
	}

	result.HadChanges = true
	progressFn(fmt.Sprintf("Found uncommitted changes (%s)\n", status.Summary))

	// Use provided commit message or generate one
	if commitMsg == "" {
		progressFn("Generating commit message with Claude...\n")

		// Try to generate commit message with Claude, fall back to simple message
		commitMsg, err = s.GenerateCommitMessageWithClaude(ctx, worktreePath)
		if err != nil {
			logger.Log("Git: Claude commit message failed, using fallback: %v", err)
			progressFn("Claude unavailable, using fallback message...\n")
			commitMsg, err = s.GenerateCommitMessage(ctx, worktreePath)
			if err != nil {
				result.Error = fmt.Errorf("failed to generate commit message: %w", err)
				return result
			}
		} else {
			// Show the generated commit message
			firstLine := strings.Split(commitMsg, "\n")[0]
			progressFn(fmt.Sprintf("Commit message: %s\n", firstLine))
		}
	} else {
		// Show the user-provided commit message
		firstLine := strings.Split(commitMsg, "\n")[0]
		progressFn(fmt.Sprintf("Commit message: %s\n", firstLine))
	}

	progressFn("Committing changes...\n")
	if err := s.CommitAll(ctx, worktreePath, commitMsg); err != nil {
		result.Error = fmt.Errorf("failed to commit changes: %w", err)
		return result
	}
	progressFn("Changes committed successfully\n\n")

	result.CommitMsg = commitMsg
	return result
}

// EnsureCommitted checks for uncommitted changes and commits them if present.
// This is a channel-based version that sends progress to a Result channel.
//
// Returns true if the commit was successful or no changes were present,
// false if there was an error (error is sent to channel with Done=true).
func (s *GitService) EnsureCommitted(
	ctx context.Context,
	ch chan<- Result,
	worktreePath string,
	commitMsg string,
) bool {
	result := s.CommitWithMessage(
		ctx,
		worktreePath,
		commitMsg,
		func(msg string) {
			ch <- Result{Output: msg}
		},
		func(err error) {
			ch <- Result{Error: err, Done: true}
		},
	)

	if result.Error != nil {
		ch <- Result{Error: result.Error, Done: true}
		return false
	}

	return true
}
