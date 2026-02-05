package git

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/logger"
)

// Result represents output from a git operation
type Result struct {
	Output          string
	Error           error
	Done            bool
	ConflictedFiles []string // Files with merge conflicts (only set on conflict)
	RepoPath        string   // Path to the repo where conflict occurred
}

// MergeToMain merges a branch into the default branch
// worktreePath is where Claude made changes - we commit any uncommitted changes first
// If commitMsg is provided and non-empty, it will be used directly instead of generating one
func (s *GitService) MergeToMain(ctx context.Context, repoPath, worktreePath, branch, commitMsg string) <-chan Result {
	ch := make(chan Result)

	go func() {
		defer close(ch)

		log := logger.WithComponent("git")
		defaultBranch := s.GetDefaultBranch(ctx, repoPath)
		log.Info("merging branch into default", "branch", branch, "defaultBranch", defaultBranch, "repoPath", repoPath, "worktree", worktreePath)

		// First, check for uncommitted changes in the worktree and commit them
		if !s.EnsureCommitted(ctx, ch, worktreePath, commitMsg) {
			return
		}

		// Checkout the default branch
		ch <- Result{Output: fmt.Sprintf("Checking out %s...\n", defaultBranch)}
		output, err := s.executor.CombinedOutput(ctx, repoPath, "git", "checkout", defaultBranch)
		if err != nil {
			ch <- Result{Output: string(output), Error: fmt.Errorf("failed to checkout %s: %w", defaultBranch, err), Done: true}
			return
		}
		ch <- Result{Output: string(output)}

		// Check if we need to sync with remote before merging
		// Use programmatic checks instead of string matching on error messages
		if s.HasRemoteOrigin(ctx, repoPath) {
			remoteBranch := fmt.Sprintf("origin/%s", defaultBranch)

			// Fetch to update remote refs
			ch <- Result{Output: "Fetching from origin...\n"}
			output, err = s.executor.CombinedOutput(ctx, repoPath, "git", "fetch", "origin", defaultBranch)
			if err != nil {
				// Fetch failed - check if remote branch exists
				if !s.RemoteBranchExists(ctx, repoPath, remoteBranch) {
					ch <- Result{Output: "Remote branch not found, continuing with local merge...\n"}
				} else {
					ch <- Result{Output: string(output), Error: fmt.Errorf("failed to fetch from origin: %w", err), Done: true}
					return
				}
			} else {
				ch <- Result{Output: string(output)}

				// Check for divergence using programmatic git commands
				divergence, divErr := s.GetBranchDivergence(ctx, repoPath, defaultBranch, remoteBranch)
				if divErr != nil {
					log.Warn("could not check divergence", "error", divErr)
				} else if divergence.IsDiverged() {
					// Local branch has diverged from remote - this is dangerous
					hint := fmt.Sprintf(`
Your local %s branch has diverged from origin/%s.
Local is %d commit(s) ahead and %d commit(s) behind.
This can cause commits to be lost if we merge now.

To fix this, sync your local %s branch first:
  cd %s
  git checkout %s
  git pull --rebase   # or: git reset --hard origin/%s

Then try merging again.
`, defaultBranch, defaultBranch, divergence.Ahead, divergence.Behind, defaultBranch, repoPath, defaultBranch, defaultBranch)
					ch <- Result{
						Output: hint,
						Error:  fmt.Errorf("local %s has diverged from origin (%d ahead, %d behind) - sync required before merge", defaultBranch, divergence.Ahead, divergence.Behind),
						Done:   true,
					}
					return
				} else if divergence.Behind > 0 {
					// Local is behind, can fast-forward - pull the changes
					ch <- Result{Output: fmt.Sprintf("Pulling %d commit(s) from origin...\n", divergence.Behind)}
					output, err = s.executor.CombinedOutput(ctx, repoPath, "git", "pull", "--ff-only")
					if err != nil {
						ch <- Result{Output: string(output), Error: fmt.Errorf("failed to pull: %w", err), Done: true}
						return
					}
					ch <- Result{Output: string(output)}
				} else {
					ch <- Result{Output: "Already up to date with origin.\n"}
				}
			}
		} else if !s.HasTrackingBranch(ctx, repoPath, defaultBranch) {
			ch <- Result{Output: "No remote configured, continuing with local merge...\n"}
		}

		// Merge the branch
		ch <- Result{Output: fmt.Sprintf("Merging %s...\n", branch)}
		output, err = s.executor.CombinedOutput(ctx, repoPath, "git", "merge", branch, "--no-edit")
		if err != nil {
			// Check if this is a merge conflict
			conflictedFiles, conflictErr := s.GetConflictedFiles(ctx, repoPath)
			if conflictErr == nil && len(conflictedFiles) > 0 {
				// This is a merge conflict - include the conflicted files in the result
				ch <- Result{
					Output:          string(output),
					Error:           fmt.Errorf("merge conflict"),
					Done:            true,
					ConflictedFiles: conflictedFiles,
					RepoPath:        repoPath,
				}
				return
			}

			// Not a conflict, some other error
			hint := fmt.Sprintf(`

To resolve this merge issue:
  1. cd %s
  2. Check git status for details
  3. Fix the issue and try again

Or abort the merge with: git merge --abort
`, repoPath)
			ch <- Result{Output: string(output) + hint, Error: fmt.Errorf("merge failed: %w", err), Done: true}
			return
		}
		ch <- Result{Output: string(output)}

		ch <- Result{Output: fmt.Sprintf("\nSuccessfully merged %s into %s\n", branch, defaultBranch), Done: true}
	}()

	return ch
}

// MergeToParent merges a child session's branch into its parent session's branch.
// This operates on the parent's worktree, merging the child's changes into it.
// childWorktreePath is where the child's changes are - we commit uncommitted changes first
// parentWorktreePath is where we perform the merge
// If commitMsg is provided and non-empty, it will be used directly instead of generating one
func (s *GitService) MergeToParent(ctx context.Context, childWorktreePath, childBranch, parentWorktreePath, parentBranch, commitMsg string) <-chan Result {
	ch := make(chan Result)

	go func() {
		defer close(ch)

		logger.WithComponent("git").Info("merging child into parent",
			"childBranch", childBranch,
			"parentBranch", parentBranch,
			"childWorktree", childWorktreePath,
			"parentWorktree", parentWorktreePath)

		// First, check for uncommitted changes in the child worktree and commit them
		if !s.EnsureCommitted(ctx, ch, childWorktreePath, commitMsg) {
			return
		}

		// Now merge the child branch into the parent worktree
		// The parent worktree should already be on the parent branch
		ch <- Result{Output: fmt.Sprintf("Merging %s into parent...\n", childBranch)}
		output, err := s.executor.CombinedOutput(ctx, parentWorktreePath, "git", "merge", childBranch, "--no-edit")
		if err != nil {
			// Check if this is a merge conflict
			conflictedFiles, conflictErr := s.GetConflictedFiles(ctx, parentWorktreePath)
			if conflictErr == nil && len(conflictedFiles) > 0 {
				// This is a merge conflict - include the conflicted files in the result
				ch <- Result{
					Output:          string(output),
					Error:           fmt.Errorf("merge conflict"),
					Done:            true,
					ConflictedFiles: conflictedFiles,
					RepoPath:        parentWorktreePath,
				}
				return
			}

			// Not a conflict, some other error
			hint := fmt.Sprintf(`

To resolve this merge issue:
  1. cd %s
  2. Check git status for details
  3. Fix the issue and try again

Or abort the merge with: git merge --abort
`, parentWorktreePath)
			ch <- Result{Output: string(output) + hint, Error: fmt.Errorf("merge failed: %w", err), Done: true}
			return
		}
		ch <- Result{Output: string(output)}

		ch <- Result{Output: fmt.Sprintf("\nSuccessfully merged %s into %s\n", childBranch, parentBranch), Done: true}
	}()

	return ch
}

// AbortMerge aborts an in-progress merge
func (s *GitService) AbortMerge(ctx context.Context, repoPath string) error {
	output, err := s.executor.CombinedOutput(ctx, repoPath, "git", "merge", "--abort")
	if err != nil {
		return fmt.Errorf("failed to abort merge: %s - %w", string(output), err)
	}
	return nil
}

// CreatePR pushes the branch and creates a pull request using gh CLI
// worktreePath is where Claude made changes - we commit any uncommitted changes first
// If commitMsg is provided and non-empty, it will be used directly instead of generating one
// If issueRef is provided, appropriate link text will be added to the PR body based on the source.
func (s *GitService) CreatePR(ctx context.Context, repoPath, worktreePath, branch, commitMsg string, issueRef *config.IssueRef) <-chan Result {
	ch := make(chan Result)

	go func() {
		defer close(ch)

		log := logger.WithComponent("git")
		defaultBranch := s.GetDefaultBranch(ctx, repoPath)
		log.Info("creating PR", "branch", branch, "defaultBranch", defaultBranch, "repoPath", repoPath, "worktree", worktreePath)

		// Check if gh CLI is available
		if _, err := exec.LookPath("gh"); err != nil {
			ch <- Result{Error: fmt.Errorf("gh CLI not found - install from https://cli.github.com"), Done: true}
			return
		}

		// First, check for uncommitted changes in the worktree and commit them
		if !s.EnsureCommitted(ctx, ch, worktreePath, commitMsg) {
			return
		}

		// Push the branch
		ch <- Result{Output: fmt.Sprintf("Pushing %s to origin...\n", branch)}
		output, err := s.executor.CombinedOutput(ctx, repoPath, "git", "push", "-u", "origin", branch)
		if err != nil {
			ch <- Result{Output: string(output), Error: fmt.Errorf("failed to push: %w", err), Done: true}
			return
		}
		ch <- Result{Output: string(output)}

		// Generate PR title and body with Claude
		ch <- Result{Output: "\nGenerating PR description with Claude...\n"}
		prTitle, prBody, err := s.GeneratePRTitleAndBodyWithIssueRef(ctx, repoPath, branch, issueRef)
		var ghArgs []string
		if err != nil {
			log.Warn("Claude PR generation failed, using --fill", "error", err)
			ch <- Result{Output: "Claude unavailable, using commit info for PR...\n"}
			// Fall back to --fill which uses commit info
			ghArgs = []string{"pr", "create", "--base", defaultBranch, "--head", branch, "--fill"}
		} else {
			ch <- Result{Output: fmt.Sprintf("PR title: %s\n", prTitle)}
			// Create PR with Claude-generated title and body
			ghArgs = []string{"pr", "create", "--base", defaultBranch, "--head", branch, "--title", prTitle, "--body", prBody}
		}

		// Run gh pr create using the executor
		handle, err := s.executor.Start(ctx, repoPath, "gh", ghArgs...)
		if err != nil {
			ch <- Result{Error: fmt.Errorf("failed to start gh: %w", err), Done: true}
			return
		}

		stdout, stderr, err := handle.Wait()
		if len(stdout) > 0 {
			ch <- Result{Output: string(stdout)}
		}
		if err != nil {
			errMsg := string(stderr)
			if errMsg == "" {
				errMsg = err.Error()
			}
			ch <- Result{Error: fmt.Errorf("PR creation failed: %s", errMsg), Done: true}
			return
		}

		ch <- Result{Output: "\nPull request created successfully!\n", Done: true}
	}()

	return ch
}

// SquashMergeToMain squashes all commits from a branch into a single commit when merging to main.
// worktreePath is where Claude made changes - we commit any uncommitted changes first.
// commitMsg is required and will be used as the commit message for the squashed commit.
func (s *GitService) SquashMergeToMain(ctx context.Context, repoPath, worktreePath, branch, commitMsg string) <-chan Result {
	ch := make(chan Result)

	go func() {
		defer close(ch)

		log := logger.WithComponent("git")
		defaultBranch := s.GetDefaultBranch(ctx, repoPath)
		log.Info("squash merging branch into default", "branch", branch, "defaultBranch", defaultBranch, "repoPath", repoPath, "worktree", worktreePath)

		// First, check for uncommitted changes in the worktree and commit them
		if !s.EnsureCommitted(ctx, ch, worktreePath, commitMsg) {
			return
		}

		// Checkout the default branch
		ch <- Result{Output: fmt.Sprintf("Checking out %s...\n", defaultBranch)}
		output, err := s.executor.CombinedOutput(ctx, repoPath, "git", "checkout", defaultBranch)
		if err != nil {
			ch <- Result{Output: string(output), Error: fmt.Errorf("failed to checkout %s: %w", defaultBranch, err), Done: true}
			return
		}
		ch <- Result{Output: string(output)}

		// Check if we need to sync with remote before merging (same logic as MergeToMain)
		if s.HasRemoteOrigin(ctx, repoPath) {
			remoteBranch := fmt.Sprintf("origin/%s", defaultBranch)

			// Fetch to update remote refs
			ch <- Result{Output: "Fetching from origin...\n"}
			output, err = s.executor.CombinedOutput(ctx, repoPath, "git", "fetch", "origin", defaultBranch)
			if err != nil {
				// Fetch failed - check if remote branch exists
				if !s.RemoteBranchExists(ctx, repoPath, remoteBranch) {
					ch <- Result{Output: "Remote branch not found, continuing with local merge...\n"}
				} else {
					ch <- Result{Output: string(output), Error: fmt.Errorf("failed to fetch from origin: %w", err), Done: true}
					return
				}
			} else {
				ch <- Result{Output: string(output)}

				// Check for divergence using programmatic git commands
				divergence, divErr := s.GetBranchDivergence(ctx, repoPath, defaultBranch, remoteBranch)
				if divErr != nil {
					log.Warn("could not check divergence", "error", divErr)
				} else if divergence.IsDiverged() {
					// Local branch has diverged from remote - this is dangerous
					hint := fmt.Sprintf(`
Your local %s branch has diverged from origin/%s.
Local is %d commit(s) ahead and %d commit(s) behind.
This can cause commits to be lost if we merge now.

To fix this, sync your local %s branch first:
  cd %s
  git checkout %s
  git pull --rebase   # or: git reset --hard origin/%s

Then try merging again.
`, defaultBranch, defaultBranch, divergence.Ahead, divergence.Behind, defaultBranch, repoPath, defaultBranch, defaultBranch)
					ch <- Result{
						Output: hint,
						Error:  fmt.Errorf("local %s has diverged from origin (%d ahead, %d behind) - sync required before merge", defaultBranch, divergence.Ahead, divergence.Behind),
						Done:   true,
					}
					return
				} else if divergence.Behind > 0 {
					// Local is behind, can fast-forward - pull the changes
					ch <- Result{Output: fmt.Sprintf("Pulling %d commit(s) from origin...\n", divergence.Behind)}
					output, err = s.executor.CombinedOutput(ctx, repoPath, "git", "pull", "--ff-only")
					if err != nil {
						ch <- Result{Output: string(output), Error: fmt.Errorf("failed to pull: %w", err), Done: true}
						return
					}
					ch <- Result{Output: string(output)}
				} else {
					ch <- Result{Output: "Already up to date with origin.\n"}
				}
			}
		} else if !s.HasTrackingBranch(ctx, repoPath, defaultBranch) {
			ch <- Result{Output: "No remote configured, continuing with local merge...\n"}
		}

		// Squash merge the branch (stages all changes but doesn't commit)
		ch <- Result{Output: fmt.Sprintf("Squash merging %s...\n", branch)}
		output, err = s.executor.CombinedOutput(ctx, repoPath, "git", "merge", "--squash", branch)
		if err != nil {
			// Check if this is a merge conflict
			conflictedFiles, conflictErr := s.GetConflictedFiles(ctx, repoPath)
			if conflictErr == nil && len(conflictedFiles) > 0 {
				// This is a merge conflict - include the conflicted files in the result
				ch <- Result{
					Output:          string(output),
					Error:           fmt.Errorf("merge conflict"),
					Done:            true,
					ConflictedFiles: conflictedFiles,
					RepoPath:        repoPath,
				}
				return
			}

			// Not a conflict, some other error
			hint := fmt.Sprintf(`

To resolve this merge issue:
  1. cd %s
  2. Check git status for details
  3. Fix the issue and try again

Or abort the merge with: git merge --abort
`, repoPath)
			ch <- Result{Output: string(output) + hint, Error: fmt.Errorf("squash merge failed: %w", err), Done: true}
			return
		}
		ch <- Result{Output: string(output)}

		// Commit the squashed changes with the provided message
		ch <- Result{Output: "Committing squashed changes...\n"}
		output, err = s.executor.CombinedOutput(ctx, repoPath, "git", "commit", "-m", commitMsg)
		if err != nil {
			ch <- Result{Output: string(output), Error: fmt.Errorf("failed to commit squashed changes: %w", err), Done: true}
			return
		}
		ch <- Result{Output: string(output)}

		ch <- Result{Output: fmt.Sprintf("\nSuccessfully squash merged %s into %s\n", branch, defaultBranch), Done: true}
	}()

	return ch
}

// PushUpdates commits any uncommitted changes and pushes to the remote branch.
// This is used after a PR has been created to push additional commits based on feedback.
// If commitMsg is provided and non-empty, it will be used directly instead of generating one.
func (s *GitService) PushUpdates(ctx context.Context, repoPath, worktreePath, branch, commitMsg string) <-chan Result {
	ch := make(chan Result)

	go func() {
		defer close(ch)

		logger.WithComponent("git").Info("pushing updates", "branch", branch, "worktree", worktreePath)

		// First, check for uncommitted changes in the worktree and commit them
		if !s.EnsureCommitted(ctx, ch, worktreePath, commitMsg) {
			return
		}

		// Push the updates to the existing remote branch
		ch <- Result{Output: fmt.Sprintf("Pushing updates to %s...\n", branch)}
		output, err := s.executor.CombinedOutput(ctx, repoPath, "git", "push", "origin", branch)
		if err != nil {
			ch <- Result{Output: string(output), Error: fmt.Errorf("failed to push: %w", err), Done: true}
			return
		}
		ch <- Result{Output: string(output)}

		ch <- Result{Output: "\nUpdates pushed successfully! The PR will be updated automatically.\n", Done: true}
	}()

	return ch
}
