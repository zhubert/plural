package git

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/zhubert/plural/internal/logger"
)

// Configuration constants for git operations
const (
	// MaxDiffSize is the maximum number of characters to include in a diff.
	// This prevents excessive memory usage when Claude is analyzing changes.
	// Claude's context window can handle much more, but large diffs slow down
	// commit message generation and rarely provide additional value.
	// 50KB is enough to capture meaningful changes while staying responsive.
	MaxDiffSize = 50000
)

// Result represents output from a git operation
type Result struct {
	Output          string
	Error           error
	Done            bool
	ConflictedFiles []string // Files with merge conflicts (only set on conflict)
	RepoPath        string   // Path to the repo where conflict occurred
}

// WorktreeStatus represents the status of changes in a worktree
type WorktreeStatus struct {
	HasChanges bool
	Summary    string // Short summary like "3 files changed"
	Files      []string // List of changed files
	Diff       string // Full diff output
}

// GetWorktreeStatus returns the status of uncommitted changes in a worktree
func GetWorktreeStatus(worktreePath string) (*WorktreeStatus, error) {
	status := &WorktreeStatus{}

	// Get list of changed files using git status --porcelain
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = worktreePath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}

	// Only trim trailing whitespace - leading space is significant in porcelain format
	// (e.g., " M file.go" means modified in worktree, the leading space is part of status)
	lines := strings.Split(strings.TrimRight(string(output), "\n\r\t "), "\n")
	if len(lines) == 1 && lines[0] == "" {
		// No changes
		status.HasChanges = false
		status.Summary = "No changes"
		return status, nil
	}

	status.HasChanges = true
	for _, line := range lines {
		if len(line) > 3 {
			status.Files = append(status.Files, strings.TrimSpace(line[3:]))
		}
	}

	// Generate summary
	fileCount := len(status.Files)
	if fileCount == 1 {
		status.Summary = "1 file changed"
	} else {
		status.Summary = fmt.Sprintf("%d files changed", fileCount)
	}

	// Get diff
	cmd = exec.Command("git", "diff", "HEAD")
	cmd.Dir = worktreePath
	diffOutput, err := cmd.Output()
	if err != nil {
		// If HEAD doesn't exist (new repo), try diff without HEAD
		logger.Log("Git: diff HEAD failed (may be new repo), trying without HEAD: %v", err)
		cmd = exec.Command("git", "diff")
		cmd.Dir = worktreePath
		diffOutput, err = cmd.Output()
		if err != nil {
			logger.Log("Git: Warning - git diff failed (best-effort): %v", err)
		}
	}

	// Also include staged changes in diff-like format
	cmd = exec.Command("git", "diff", "--cached")
	cmd.Dir = worktreePath
	cachedDiff, err := cmd.Output()
	if err != nil {
		logger.Log("Git: Warning - git diff --cached failed (best-effort): %v", err)
	}

	status.Diff = string(diffOutput) + string(cachedDiff)

	return status, nil
}

// CommitAll stages all changes and commits them with the given message
func CommitAll(worktreePath, message string) error {
	logger.Log("Git: Committing all changes in %s", worktreePath)

	// Stage all changes
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = worktreePath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %s - %w", string(output), err)
	}

	// Commit
	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Dir = worktreePath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit failed: %s - %w", string(output), err)
	}

	return nil
}

// GenerateCommitMessage creates a commit message based on the changes (simple fallback)
func GenerateCommitMessage(worktreePath string) (string, error) {
	status, err := GetWorktreeStatus(worktreePath)
	if err != nil {
		return "", err
	}

	if !status.HasChanges {
		return "", fmt.Errorf("no changes to commit")
	}

	// Get the diff stats for a better message
	cmd := exec.Command("git", "diff", "--stat", "HEAD")
	cmd.Dir = worktreePath
	statOutput, err := cmd.Output()
	if err != nil {
		logger.Log("Git: Warning - git diff --stat failed (best-effort): %v", err)
	}

	// Create a simple but descriptive message
	message := fmt.Sprintf("Plural session changes\n\n%s\n\nFiles:\n", status.Summary)
	for _, file := range status.Files {
		message += fmt.Sprintf("- %s\n", file)
	}

	if len(statOutput) > 0 {
		message += fmt.Sprintf("\nStats:\n%s", string(statOutput))
	}

	return message, nil
}

// GenerateCommitMessageWithClaude uses Claude to generate a commit message from the diff
func GenerateCommitMessageWithClaude(ctx context.Context, worktreePath string) (string, error) {
	logger.Log("Git: Generating commit message with Claude for %s", worktreePath)

	status, err := GetWorktreeStatus(worktreePath)
	if err != nil {
		return "", err
	}

	if !status.HasChanges {
		return "", fmt.Errorf("no changes to commit")
	}

	// Get the full diff for Claude to analyze
	cmd := exec.CommandContext(ctx, "git", "diff", "HEAD")
	cmd.Dir = worktreePath
	diffOutput, err := cmd.Output()
	if err != nil {
		// Try without HEAD for new repos
		logger.Log("Git: diff HEAD failed (may be new repo), trying without HEAD: %v", err)
		cmd = exec.CommandContext(ctx, "git", "diff")
		cmd.Dir = worktreePath
		diffOutput, err = cmd.Output()
		if err != nil {
			logger.Log("Git: Warning - git diff failed (best-effort): %v", err)
		}
	}

	// Also get staged changes
	cmd = exec.CommandContext(ctx, "git", "diff", "--cached")
	cmd.Dir = worktreePath
	cachedOutput, err := cmd.Output()
	if err != nil {
		logger.Log("Git: Warning - git diff --cached failed (best-effort): %v", err)
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
	args := []string{"--print", "-p", prompt}

	cmd = exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = worktreePath

	output, err := cmd.Output()
	if err != nil {
		logger.Log("Git: Claude commit message generation failed: %v", err)
		return "", fmt.Errorf("failed to generate commit message with Claude: %w", err)
	}

	commitMsg := strings.TrimSpace(string(output))
	if commitMsg == "" {
		return "", fmt.Errorf("Claude returned empty commit message")
	}

	logger.Log("Git: Generated commit message: %s", strings.Split(commitMsg, "\n")[0])
	return commitMsg, nil
}

// GeneratePRTitleAndBody uses Claude to generate a PR title and body from the branch changes
func GeneratePRTitleAndBody(ctx context.Context, repoPath, branch string) (title, body string, err error) {
	logger.Log("Git: Generating PR title and body with Claude for branch %s", branch)

	defaultBranch := GetDefaultBranch(repoPath)

	// Get the commit log for this branch
	cmd := exec.CommandContext(ctx, "git", "log", fmt.Sprintf("%s..%s", defaultBranch, branch), "--oneline")
	cmd.Dir = repoPath
	commitLog, err := cmd.Output()
	if err != nil {
		logger.Log("Git: Failed to get commit log: %v", err)
		return "", "", fmt.Errorf("failed to get commit log: %w", err)
	}

	// Get the diff from base branch
	cmd = exec.CommandContext(ctx, "git", "diff", fmt.Sprintf("%s...%s", defaultBranch, branch))
	cmd.Dir = repoPath
	diffOutput, err := cmd.Output()
	if err != nil {
		logger.Log("Git: Failed to get diff: %v", err)
		return "", "", fmt.Errorf("failed to get diff: %w", err)
	}

	fullDiff := string(diffOutput)

	// Truncate diff if too large
	maxDiffSize := MaxDiffSize
	if len(fullDiff) > maxDiffSize {
		fullDiff = fullDiff[:maxDiffSize] + "\n... (diff truncated)"
	}

	// Build the prompt for Claude
	prompt := fmt.Sprintf(`Generate a GitHub pull request title and body for the following changes.

Output format (use exactly this format with the markers):
---TITLE---
Your PR title here (max 72 chars, imperative mood)
---BODY---
## Summary
Brief description of what this PR does

## Changes
- Bullet points of key changes

## Test plan
- How to test these changes

Rules:
1. Title should be concise and descriptive (max 72 chars)
2. Body should explain the purpose and changes clearly
3. Include a test plan section
4. Do NOT include any preamble - start directly with ---TITLE---

Commits in this branch:
%s

Diff:
%s`, string(commitLog), fullDiff)

	// Call Claude CLI
	args := []string{"--print", "-p", prompt}

	cmd = exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		logger.Log("Git: Claude PR generation failed: %v", err)
		return "", "", fmt.Errorf("failed to generate PR with Claude: %w", err)
	}

	result := strings.TrimSpace(string(output))

	// Parse the output
	titleMarker := "---TITLE---"
	bodyMarker := "---BODY---"

	titleStart := strings.Index(result, titleMarker)
	bodyStart := strings.Index(result, bodyMarker)

	if titleStart == -1 || bodyStart == -1 {
		// Fallback: use first line as title, rest as body
		lines := strings.SplitN(result, "\n", 2)
		title = strings.TrimSpace(lines[0])
		if len(lines) > 1 {
			body = strings.TrimSpace(lines[1])
		}
	} else {
		title = strings.TrimSpace(result[titleStart+len(titleMarker) : bodyStart])
		body = strings.TrimSpace(result[bodyStart+len(bodyMarker):])
	}

	if title == "" {
		return "", "", fmt.Errorf("Claude returned empty PR title")
	}

	logger.Log("Git: Generated PR title: %s", title)
	return title, body, nil
}

// HasRemoteOrigin checks if the repository has a remote named "origin"
func HasRemoteOrigin(repoPath string) bool {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoPath
	err := cmd.Run()
	return err == nil
}

// GetDefaultBranch returns the default branch name (main or master)
func GetDefaultBranch(repoPath string) string {
	// Try to get the default branch from origin
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err == nil {
		// Output is like "refs/remotes/origin/main"
		ref := strings.TrimSpace(string(output))
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	// Fallback: check if main exists, otherwise use master
	cmd = exec.Command("git", "rev-parse", "--verify", "main")
	cmd.Dir = repoPath
	if err := cmd.Run(); err == nil {
		return "main"
	}

	return "master"
}

// GetConflictedFiles returns the list of files with merge conflicts in a repo
func GetConflictedFiles(repoPath string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get conflicted files: %w", err)
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		return nil, nil
	}

	files := strings.Split(outputStr, "\n")
	return files, nil
}

// AbortMerge aborts an in-progress merge
func AbortMerge(repoPath string) error {
	cmd := exec.Command("git", "merge", "--abort")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to abort merge: %s - %w", string(output), err)
	}
	return nil
}

// CommitConflictResolution stages all changes and commits with the given message.
// This is used after resolving merge conflicts to complete the merge.
func CommitConflictResolution(repoPath, message string) error {
	logger.Log("Git: Committing conflict resolution in %s", repoPath)

	// Stage all changes
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add failed: %s - %w", string(output), err)
	}

	// Commit
	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit failed: %s - %w", string(output), err)
	}

	return nil
}

// MergeToMain merges a branch into the default branch
// worktreePath is where Claude made changes - we commit any uncommitted changes first
// If commitMsg is provided and non-empty, it will be used directly instead of generating one
func MergeToMain(ctx context.Context, repoPath, worktreePath, branch, commitMsg string) <-chan Result {
	ch := make(chan Result)

	go func() {
		defer close(ch)

		defaultBranch := GetDefaultBranch(repoPath)
		logger.Log("Git: Merging %s into %s in %s (worktree: %s)", branch, defaultBranch, repoPath, worktreePath)

		// First, check for uncommitted changes in the worktree and commit them
		status, err := GetWorktreeStatus(worktreePath)
		if err != nil {
			ch <- Result{Error: fmt.Errorf("failed to get worktree status: %w", err), Done: true}
			return
		}

		if status.HasChanges {
			ch <- Result{Output: fmt.Sprintf("Found uncommitted changes (%s)\n", status.Summary)}

			// Use provided commit message or generate one
			if commitMsg == "" {
				ch <- Result{Output: "Generating commit message with Claude...\n"}

				// Try to generate commit message with Claude, fall back to simple message
				commitMsg, err = GenerateCommitMessageWithClaude(ctx, worktreePath)
				if err != nil {
					logger.Log("Git: Claude commit message failed, using fallback: %v", err)
					ch <- Result{Output: "Claude unavailable, using fallback message...\n"}
					commitMsg, err = GenerateCommitMessage(worktreePath)
					if err != nil {
						ch <- Result{Error: fmt.Errorf("failed to generate commit message: %w", err), Done: true}
						return
					}
				} else {
					// Show the generated commit message
					firstLine := strings.Split(commitMsg, "\n")[0]
					ch <- Result{Output: fmt.Sprintf("Commit message: %s\n", firstLine)}
				}
			} else {
				// Show the user-provided commit message
				firstLine := strings.Split(commitMsg, "\n")[0]
				ch <- Result{Output: fmt.Sprintf("Commit message: %s\n", firstLine)}
			}

			ch <- Result{Output: "Committing changes...\n"}
			if err := CommitAll(worktreePath, commitMsg); err != nil {
				ch <- Result{Error: fmt.Errorf("failed to commit changes: %w", err), Done: true}
				return
			}
			ch <- Result{Output: "Changes committed successfully\n\n"}
		} else {
			ch <- Result{Output: "No uncommitted changes in worktree\n\n"}
		}

		// Checkout the default branch
		ch <- Result{Output: fmt.Sprintf("Checking out %s...\n", defaultBranch)}
		cmd := exec.CommandContext(ctx, "git", "checkout", defaultBranch)
		cmd.Dir = repoPath
		output, err := cmd.CombinedOutput()
		if err != nil {
			ch <- Result{Output: string(output), Error: fmt.Errorf("failed to checkout %s: %w", defaultBranch, err), Done: true}
			return
		}
		ch <- Result{Output: string(output)}

		// Merge the branch
		ch <- Result{Output: fmt.Sprintf("Merging %s...\n", branch)}
		cmd = exec.CommandContext(ctx, "git", "merge", branch, "--no-edit")
		cmd.Dir = repoPath
		output, err = cmd.CombinedOutput()
		if err != nil {
			// Check if this is a merge conflict
			conflictedFiles, conflictErr := GetConflictedFiles(repoPath)
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

// CreatePR pushes the branch and creates a pull request using gh CLI
// worktreePath is where Claude made changes - we commit any uncommitted changes first
// If commitMsg is provided and non-empty, it will be used directly instead of generating one
func CreatePR(ctx context.Context, repoPath, worktreePath, branch, commitMsg string) <-chan Result {
	ch := make(chan Result)

	go func() {
		defer close(ch)

		defaultBranch := GetDefaultBranch(repoPath)
		logger.Log("Git: Creating PR for %s -> %s in %s (worktree: %s)", branch, defaultBranch, repoPath, worktreePath)

		// Check if gh CLI is available
		if _, err := exec.LookPath("gh"); err != nil {
			ch <- Result{Error: fmt.Errorf("gh CLI not found - install from https://cli.github.com"), Done: true}
			return
		}

		// First, check for uncommitted changes in the worktree and commit them
		status, err := GetWorktreeStatus(worktreePath)
		if err != nil {
			ch <- Result{Error: fmt.Errorf("failed to get worktree status: %w", err), Done: true}
			return
		}

		if status.HasChanges {
			ch <- Result{Output: fmt.Sprintf("Found uncommitted changes (%s)\n", status.Summary)}

			// Use provided commit message or generate one
			if commitMsg == "" {
				ch <- Result{Output: "Generating commit message with Claude...\n"}

				// Try to generate commit message with Claude, fall back to simple message
				commitMsg, err = GenerateCommitMessageWithClaude(ctx, worktreePath)
				if err != nil {
					logger.Log("Git: Claude commit message failed, using fallback: %v", err)
					ch <- Result{Output: "Claude unavailable, using fallback message...\n"}
					commitMsg, err = GenerateCommitMessage(worktreePath)
					if err != nil {
						ch <- Result{Error: fmt.Errorf("failed to generate commit message: %w", err), Done: true}
						return
					}
				} else {
					// Show the generated commit message
					firstLine := strings.Split(commitMsg, "\n")[0]
					ch <- Result{Output: fmt.Sprintf("Commit message: %s\n", firstLine)}
				}
			} else {
				// Show the user-provided commit message
				firstLine := strings.Split(commitMsg, "\n")[0]
				ch <- Result{Output: fmt.Sprintf("Commit message: %s\n", firstLine)}
			}

			ch <- Result{Output: "Committing changes...\n"}
			if err := CommitAll(worktreePath, commitMsg); err != nil {
				ch <- Result{Error: fmt.Errorf("failed to commit changes: %w", err), Done: true}
				return
			}
			ch <- Result{Output: "Changes committed successfully\n\n"}
		} else {
			ch <- Result{Output: "No uncommitted changes in worktree\n\n"}
		}

		// Push the branch
		ch <- Result{Output: fmt.Sprintf("Pushing %s to origin...\n", branch)}
		cmd := exec.CommandContext(ctx, "git", "push", "-u", "origin", branch)
		cmd.Dir = repoPath
		output, err := cmd.CombinedOutput()
		if err != nil {
			ch <- Result{Output: string(output), Error: fmt.Errorf("failed to push: %w", err), Done: true}
			return
		}
		ch <- Result{Output: string(output)}

		// Generate PR title and body with Claude
		ch <- Result{Output: "\nGenerating PR description with Claude...\n"}
		prTitle, prBody, err := GeneratePRTitleAndBody(ctx, repoPath, branch)
		if err != nil {
			logger.Log("Git: Claude PR generation failed, using --fill: %v", err)
			ch <- Result{Output: "Claude unavailable, using commit info for PR...\n"}
			// Fall back to --fill which uses commit info
			cmd = exec.CommandContext(ctx, "gh", "pr", "create",
				"--base", defaultBranch,
				"--head", branch,
				"--fill",
			)
		} else {
			ch <- Result{Output: fmt.Sprintf("PR title: %s\n", prTitle)}
			// Create PR with Claude-generated title and body
			cmd = exec.CommandContext(ctx, "gh", "pr", "create",
				"--base", defaultBranch,
				"--head", branch,
				"--title", prTitle,
				"--body", prBody,
			)
		}
		cmd.Dir = repoPath

		// Stream the output
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			ch <- Result{Error: fmt.Errorf("failed to create stdout pipe: %w", err), Done: true}
			return
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			ch <- Result{Error: fmt.Errorf("failed to create stderr pipe: %w", err), Done: true}
			return
		}

		if err := cmd.Start(); err != nil {
			ch <- Result{Error: fmt.Errorf("failed to start gh: %w", err), Done: true}
			return
		}

		// Read stdout
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			ch <- Result{Output: scanner.Text() + "\n"}
		}

		// Read any stderr
		stderrScanner := bufio.NewScanner(stderr)
		var stderrOutput strings.Builder
		for stderrScanner.Scan() {
			stderrOutput.WriteString(stderrScanner.Text() + "\n")
		}

		if err := cmd.Wait(); err != nil {
			errMsg := stderrOutput.String()
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

// MergeToParent merges a child session's branch into its parent session's branch.
// This operates on the parent's worktree, merging the child's changes into it.
// childWorktreePath is where the child's changes are - we commit uncommitted changes first
// parentWorktreePath is where we perform the merge
// If commitMsg is provided and non-empty, it will be used directly instead of generating one
func MergeToParent(ctx context.Context, childWorktreePath, childBranch, parentWorktreePath, parentBranch, commitMsg string) <-chan Result {
	ch := make(chan Result)

	go func() {
		defer close(ch)

		logger.Log("Git: Merging child %s into parent %s (child worktree: %s, parent worktree: %s)",
			childBranch, parentBranch, childWorktreePath, parentWorktreePath)

		// First, check for uncommitted changes in the child worktree and commit them
		status, err := GetWorktreeStatus(childWorktreePath)
		if err != nil {
			ch <- Result{Error: fmt.Errorf("failed to get child worktree status: %w", err), Done: true}
			return
		}

		if status.HasChanges {
			ch <- Result{Output: fmt.Sprintf("Found uncommitted changes in child (%s)\n", status.Summary)}

			// Use provided commit message or generate one
			if commitMsg == "" {
				ch <- Result{Output: "Generating commit message with Claude...\n"}

				// Try to generate commit message with Claude, fall back to simple message
				commitMsg, err = GenerateCommitMessageWithClaude(ctx, childWorktreePath)
				if err != nil {
					logger.Log("Git: Claude commit message failed, using fallback: %v", err)
					ch <- Result{Output: "Claude unavailable, using fallback message...\n"}
					commitMsg, err = GenerateCommitMessage(childWorktreePath)
					if err != nil {
						ch <- Result{Error: fmt.Errorf("failed to generate commit message: %w", err), Done: true}
						return
					}
				} else {
					// Show the generated commit message
					firstLine := strings.Split(commitMsg, "\n")[0]
					ch <- Result{Output: fmt.Sprintf("Commit message: %s\n", firstLine)}
				}
			} else {
				// Show the user-provided commit message
				firstLine := strings.Split(commitMsg, "\n")[0]
				ch <- Result{Output: fmt.Sprintf("Commit message: %s\n", firstLine)}
			}

			ch <- Result{Output: "Committing changes in child...\n"}
			if err := CommitAll(childWorktreePath, commitMsg); err != nil {
				ch <- Result{Error: fmt.Errorf("failed to commit changes: %w", err), Done: true}
				return
			}
			ch <- Result{Output: "Changes committed successfully\n\n"}
		} else {
			ch <- Result{Output: "No uncommitted changes in child worktree\n\n"}
		}

		// Now merge the child branch into the parent worktree
		// The parent worktree should already be on the parent branch
		ch <- Result{Output: fmt.Sprintf("Merging %s into parent...\n", childBranch)}
		cmd := exec.CommandContext(ctx, "git", "merge", childBranch, "--no-edit")
		cmd.Dir = parentWorktreePath
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Check if this is a merge conflict
			conflictedFiles, conflictErr := GetConflictedFiles(parentWorktreePath)
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

// GitHubIssue represents a GitHub issue fetched via the gh CLI
type GitHubIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	URL    string `json:"url"`
}

// FetchGitHubIssues fetches open issues from a GitHub repository using the gh CLI.
// The repoPath is used as the working directory to determine which repo to query.
func FetchGitHubIssues(repoPath string) ([]GitHubIssue, error) {
	cmd := exec.Command("gh", "issue", "list",
		"--json", "number,title,body,url",
		"--state", "open",
	)
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh issue list failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("gh issue list failed: %w", err)
	}

	var issues []GitHubIssue
	if err := json.Unmarshal(output, &issues); err != nil {
		return nil, fmt.Errorf("failed to parse issues: %w", err)
	}

	return issues, nil
}
