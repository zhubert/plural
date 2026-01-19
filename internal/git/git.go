package git

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	pexec "github.com/zhubert/plural/internal/exec"
	"github.com/zhubert/plural/internal/logger"
)

// executor is the command executor used by this package.
// It can be swapped for testing/demos via SetExecutor.
var executor pexec.CommandExecutor = pexec.NewRealExecutor()

// SetExecutor sets the command executor used by this package.
// This is primarily used for testing and demo generation.
func SetExecutor(e pexec.CommandExecutor) {
	executor = e
}

// GetExecutor returns the current command executor.
func GetExecutor() pexec.CommandExecutor {
	return executor
}

// Configuration constants for git operations
const (
	// MaxDiffSize is the maximum number of characters to include in a diff.
	// This prevents excessive memory usage when Claude is analyzing changes.
	// Claude's context window can handle much more, but large diffs slow down
	// commit message generation and rarely provide additional value.
	// 50KB is enough to capture meaningful changes while staying responsive.
	MaxDiffSize = 50000

	// MaxBranchNameLength is the maximum length for auto-generated branch names.
	// User-provided branch names can be longer (up to MaxBranchNameValidation).
	MaxBranchNameLength = 50
)

// Result represents output from a git operation
type Result struct {
	Output          string
	Error           error
	Done            bool
	ConflictedFiles []string // Files with merge conflicts (only set on conflict)
	RepoPath        string   // Path to the repo where conflict occurred
}

// FileDiff represents a single file's diff with its status
type FileDiff struct {
	Filename string // File path relative to repo root
	Status   string // Status code: M (modified), A (added), D (deleted), etc.
	Diff     string // Diff content for this file only
}

// WorktreeStatus represents the status of changes in a worktree
type WorktreeStatus struct {
	HasChanges bool
	Summary    string     // Short summary like "3 files changed"
	Files      []string   // List of changed files
	Diff       string     // Full diff output
	FileDiffs  []FileDiff // Per-file diffs for detailed viewing
}

// GetWorktreeStatus returns the status of uncommitted changes in a worktree
func GetWorktreeStatus(worktreePath string) (*WorktreeStatus, error) {
	status := &WorktreeStatus{}

	ctx := context.Background()

	// Get list of changed files using git status --porcelain
	output, err := executor.Output(ctx, worktreePath, "git", "status", "--porcelain")
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
	// Map to store status codes for each file
	fileStatuses := make(map[string]string)
	for _, line := range lines {
		if len(line) > 3 {
			filename := strings.TrimSpace(line[3:])
			status.Files = append(status.Files, filename)
			// Extract status code (first 2 chars: index status + worktree status)
			// Use the more significant status (prefer non-space)
			statusCode := strings.TrimSpace(line[:2])
			if statusCode == "" {
				statusCode = "M" // Default to modified
			} else if len(statusCode) == 2 {
				// Take the first non-space character
				if statusCode[0] != ' ' {
					statusCode = string(statusCode[0])
				} else {
					statusCode = string(statusCode[1])
				}
			}
			fileStatuses[filename] = statusCode
		}
	}

	// Generate summary
	fileCount := len(status.Files)
	if fileCount == 1 {
		status.Summary = "1 file changed"
	} else {
		status.Summary = fmt.Sprintf("%d files changed", fileCount)
	}

	// Get diff (use --no-ext-diff to ensure output goes to stdout even if external diff is configured)
	diffOutput, err := executor.Output(ctx, worktreePath, "git", "diff", "--no-ext-diff", "HEAD")
	if err != nil {
		// If HEAD doesn't exist (new repo), try diff without HEAD
		logger.Log("Git: diff HEAD failed (may be new repo), trying without HEAD: %v", err)
		diffOutput, err = executor.Output(ctx, worktreePath, "git", "diff", "--no-ext-diff")
		if err != nil {
			logger.Log("Git: Warning - git diff failed (best-effort): %v", err)
		}
	}

	// Also include staged changes in diff-like format
	cachedDiff, err := executor.Output(ctx, worktreePath, "git", "diff", "--no-ext-diff", "--cached")
	if err != nil {
		logger.Log("Git: Warning - git diff --cached failed (best-effort): %v", err)
	}

	status.Diff = string(diffOutput) + string(cachedDiff)

	// Parse per-file diffs for detailed viewing
	status.FileDiffs = parseFileDiffs(worktreePath, status.Diff, status.Files, fileStatuses)

	return status, nil
}

// parseFileDiffs splits a combined diff into per-file chunks
func parseFileDiffs(worktreePath, diff string, files []string, fileStatuses map[string]string) []FileDiff {
	if diff == "" {
		// No diff content from git diff - but untracked files need special handling
		result := make([]FileDiff, 0, len(files))
		for _, file := range files {
			status := fileStatuses[file]
			if status == "" {
				status = "M"
			}
			var diffContent string
			if status == "?" {
				diffContent = generateUntrackedFileDiff(worktreePath, file)
			} else {
				diffContent = "(no diff available)"
			}
			result = append(result, FileDiff{
				Filename: file,
				Status:   status,
				Diff:     diffContent,
			})
		}
		return result
	}

	// Split diff on "diff --git" markers
	chunks := strings.Split(diff, "diff --git ")
	fileDiffMap := make(map[string]string)

	for _, chunk := range chunks {
		if chunk == "" {
			continue
		}
		// Re-add the marker for proper formatting
		chunk = "diff --git " + chunk

		// Extract filename from "diff --git a/path b/path" line
		firstLine := strings.SplitN(chunk, "\n", 2)[0]
		// Parse "diff --git a/foo/bar.go b/foo/bar.go"
		parts := strings.Split(firstLine, " ")
		if len(parts) >= 4 {
			// Get the b/path part and strip the "b/" prefix
			bPath := parts[len(parts)-1]
			if strings.HasPrefix(bPath, "b/") {
				filename := bPath[2:]
				fileDiffMap[filename] = strings.TrimRight(chunk, "\n")
			}
		}
	}

	// Build result in the same order as files list
	result := make([]FileDiff, 0, len(files))
	for _, file := range files {
		status := fileStatuses[file]
		if status == "" {
			status = "M"
		}
		diffContent := fileDiffMap[file]
		if diffContent == "" {
			// For untracked files (status "?"), generate a diff showing the new file content
			if status == "?" {
				diffContent = generateUntrackedFileDiff(worktreePath, file)
			} else {
				diffContent = "(no diff available - file may be binary)"
			}
		}
		result = append(result, FileDiff{
			Filename: file,
			Status:   status,
			Diff:     diffContent,
		})
	}

	return result
}

// generateUntrackedFileDiff creates a diff-like output for an untracked file
func generateUntrackedFileDiff(worktreePath, file string) string {
	// Use git diff --no-index to compare /dev/null with the new file
	// This produces a proper diff format showing the file as new
	output, err := executor.Output(context.Background(), worktreePath, "git", "diff", "--no-ext-diff", "--no-index", "/dev/null", file)
	if err != nil {
		// git diff --no-index returns exit code 1 when files differ, which is expected
		// Only treat it as an error if there's no output
		if len(output) == 0 {
			logger.Log("Git: Warning - failed to generate diff for untracked file %s: %v", file, err)
			return "(no diff available - file may be binary)"
		}
	}
	return strings.TrimRight(string(output), "\n")
}

// CommitAll stages all changes and commits them with the given message
func CommitAll(worktreePath, message string) error {
	logger.Log("Git: Committing all changes in %s", worktreePath)

	ctx := context.Background()

	// Stage all changes
	if output, err := executor.CombinedOutput(ctx, worktreePath, "git", "add", "-A"); err != nil {
		return fmt.Errorf("git add failed: %s - %w", string(output), err)
	}

	// Commit
	if output, err := executor.CombinedOutput(ctx, worktreePath, "git", "commit", "-m", message); err != nil {
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

	// Get the diff stats for a better message (use --no-ext-diff to ensure output goes to stdout)
	statOutput, err := executor.Output(context.Background(), worktreePath, "git", "diff", "--no-ext-diff", "--stat", "HEAD")
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

	// Get the full diff for Claude to analyze (use --no-ext-diff to ensure output goes to stdout)
	diffOutput, err := executor.Output(ctx, worktreePath, "git", "diff", "--no-ext-diff", "HEAD")
	if err != nil {
		// Try without HEAD for new repos
		logger.Log("Git: diff HEAD failed (may be new repo), trying without HEAD: %v", err)
		diffOutput, err = executor.Output(ctx, worktreePath, "git", "diff", "--no-ext-diff")
		if err != nil {
			logger.Log("Git: Warning - git diff failed (best-effort): %v", err)
		}
	}

	// Also get staged changes
	cachedOutput, err := executor.Output(ctx, worktreePath, "git", "diff", "--no-ext-diff", "--cached")
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

	cmd := exec.CommandContext(ctx, "claude", args...)
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

// GeneratePRTitleAndBody uses Claude to generate a PR title and body from the branch changes.
// If issueNumber is provided (non-zero), it will be included as "Fixes #N" in the PR body.
func GeneratePRTitleAndBody(ctx context.Context, repoPath, branch string, issueNumber int) (title, body string, err error) {
	logger.Log("Git: Generating PR title and body with Claude for branch %s (issue: %d)", branch, issueNumber)

	defaultBranch := GetDefaultBranch(repoPath)

	// Get the commit log for this branch
	commitLog, err := executor.Output(ctx, repoPath, "git", "log", fmt.Sprintf("%s..%s", defaultBranch, branch), "--oneline")
	if err != nil {
		logger.Log("Git: Failed to get commit log: %v", err)
		return "", "", fmt.Errorf("failed to get commit log: %w", err)
	}

	// Get the diff from base branch (use --no-ext-diff to ensure output goes to stdout)
	diffOutput, err := executor.Output(ctx, repoPath, "git", "diff", "--no-ext-diff", fmt.Sprintf("%s...%s", defaultBranch, branch))
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

	cmd := exec.CommandContext(ctx, "claude", args...)
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

	// Add "Fixes #N" to the body if this PR is for a GitHub issue
	if issueNumber > 0 {
		fixesLine := fmt.Sprintf("\n\nFixes #%d", issueNumber)
		body = body + fixesLine
		logger.Log("Git: Added issue reference: Fixes #%d", issueNumber)
	}

	logger.Log("Git: Generated PR title: %s", title)
	return title, body, nil
}

// HasRemoteOrigin checks if the repository has a remote named "origin"
func HasRemoteOrigin(repoPath string) bool {
	_, _, err := executor.Run(context.Background(), repoPath, "git", "remote", "get-url", "origin")
	return err == nil
}

// GetDefaultBranch returns the default branch name (main or master)
func GetDefaultBranch(repoPath string) string {
	ctx := context.Background()

	// Try to get the default branch from origin
	output, err := executor.Output(ctx, repoPath, "git", "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		// Output is like "refs/remotes/origin/main"
		ref := strings.TrimSpace(string(output))
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	// Fallback: check if main exists, otherwise use master
	_, _, err = executor.Run(ctx, repoPath, "git", "rev-parse", "--verify", "main")
	if err == nil {
		return "main"
	}

	return "master"
}

// GetConflictedFiles returns the list of files with merge conflicts in a repo
func GetConflictedFiles(repoPath string) ([]string, error) {
	output, err := executor.Output(context.Background(), repoPath, "git", "diff", "--name-only", "--diff-filter=U")
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

// BranchDivergence represents the divergence between local and remote branches.
type BranchDivergence struct {
	Behind int // Number of commits local is behind remote
	Ahead  int // Number of commits local is ahead of remote
}

// IsDiverged returns true if the branches have diverged (both ahead and behind).
func (d *BranchDivergence) IsDiverged() bool {
	return d.Behind > 0 && d.Ahead > 0
}

// CanFastForward returns true if local can fast-forward to remote (not ahead).
func (d *BranchDivergence) CanFastForward() bool {
	return d.Ahead == 0
}

// GetBranchDivergence returns how many commits the local branch is behind and ahead
// of the remote branch. Uses git rev-list --count --left-right which outputs "behind\tahead".
// Returns an error if either branch doesn't exist or comparison fails.
func GetBranchDivergence(repoPath, localBranch, remoteBranch string) (*BranchDivergence, error) {
	ctx := context.Background()

	// git rev-list --count --left-right remoteBranch...localBranch
	// Output format: "behind<tab>ahead"
	output, err := executor.Output(ctx, repoPath, "git", "rev-list", "--count", "--left-right",
		fmt.Sprintf("%s...%s", remoteBranch, localBranch))
	if err != nil {
		return nil, fmt.Errorf("failed to get branch divergence: %w", err)
	}

	// Parse "behind\tahead" format
	parts := strings.Split(strings.TrimSpace(string(output)), "\t")
	if len(parts) != 2 {
		return nil, fmt.Errorf("unexpected rev-list output format: %q", string(output))
	}

	behind, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse behind count: %w", err)
	}

	ahead, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse ahead count: %w", err)
	}

	return &BranchDivergence{Behind: behind, Ahead: ahead}, nil
}

// HasTrackingBranch checks if the given branch has an upstream tracking branch configured.
// Uses git config to check for branch.<name>.remote which is set when tracking is configured.
func HasTrackingBranch(repoPath, branch string) bool {
	ctx := context.Background()
	_, err := executor.Output(ctx, repoPath, "git", "config", "--get", fmt.Sprintf("branch.%s.remote", branch))
	return err == nil
}

// RemoteBranchExists checks if a remote branch reference exists (e.g., "origin/main").
// Uses git rev-parse --verify which exits 0 if the ref exists, non-zero otherwise.
func RemoteBranchExists(repoPath, remoteBranch string) bool {
	ctx := context.Background()
	_, _, err := executor.Run(ctx, repoPath, "git", "rev-parse", "--verify", remoteBranch)
	return err == nil
}

// AbortMerge aborts an in-progress merge
func AbortMerge(repoPath string) error {
	output, err := executor.CombinedOutput(context.Background(), repoPath, "git", "merge", "--abort")
	if err != nil {
		return fmt.Errorf("failed to abort merge: %s - %w", string(output), err)
	}
	return nil
}

// CommitConflictResolution stages all changes and commits with the given message.
// This is used after resolving merge conflicts to complete the merge.
func CommitConflictResolution(repoPath, message string) error {
	logger.Log("Git: Committing conflict resolution in %s", repoPath)

	ctx := context.Background()

	// Stage all changes
	if output, err := executor.CombinedOutput(ctx, repoPath, "git", "add", "-A"); err != nil {
		return fmt.Errorf("git add failed: %s - %w", string(output), err)
	}

	// Commit
	if output, err := executor.CombinedOutput(ctx, repoPath, "git", "commit", "-m", message); err != nil {
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
		if !EnsureCommitted(ctx, ch, worktreePath, commitMsg) {
			return
		}

		// Checkout the default branch
		ch <- Result{Output: fmt.Sprintf("Checking out %s...\n", defaultBranch)}
		output, err := executor.CombinedOutput(ctx, repoPath, "git", "checkout", defaultBranch)
		if err != nil {
			ch <- Result{Output: string(output), Error: fmt.Errorf("failed to checkout %s: %w", defaultBranch, err), Done: true}
			return
		}
		ch <- Result{Output: string(output)}

		// Check if we need to sync with remote before merging
		// Use programmatic checks instead of string matching on error messages
		if HasRemoteOrigin(repoPath) {
			remoteBranch := fmt.Sprintf("origin/%s", defaultBranch)

			// Fetch to update remote refs
			ch <- Result{Output: "Fetching from origin...\n"}
			output, err = executor.CombinedOutput(ctx, repoPath, "git", "fetch", "origin", defaultBranch)
			if err != nil {
				// Fetch failed - check if remote branch exists
				if !RemoteBranchExists(repoPath, remoteBranch) {
					ch <- Result{Output: "Remote branch not found, continuing with local merge...\n"}
				} else {
					ch <- Result{Output: string(output), Error: fmt.Errorf("failed to fetch from origin: %w", err), Done: true}
					return
				}
			} else {
				ch <- Result{Output: string(output)}

				// Check for divergence using programmatic git commands
				divergence, divErr := GetBranchDivergence(repoPath, defaultBranch, remoteBranch)
				if divErr != nil {
					logger.Log("Git: Warning - could not check divergence: %v", divErr)
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
					output, err = executor.CombinedOutput(ctx, repoPath, "git", "pull", "--ff-only")
					if err != nil {
						ch <- Result{Output: string(output), Error: fmt.Errorf("failed to pull: %w", err), Done: true}
						return
					}
					ch <- Result{Output: string(output)}
				} else {
					ch <- Result{Output: "Already up to date with origin.\n"}
				}
			}
		} else if !HasTrackingBranch(repoPath, defaultBranch) {
			ch <- Result{Output: "No remote configured, continuing with local merge...\n"}
		}

		// Merge the branch
		ch <- Result{Output: fmt.Sprintf("Merging %s...\n", branch)}
		output, err = executor.CombinedOutput(ctx, repoPath, "git", "merge", branch, "--no-edit")
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
// If issueNumber is provided (non-zero), "Fixes #N" will be added to the PR body
func CreatePR(ctx context.Context, repoPath, worktreePath, branch, commitMsg string, issueNumber int) <-chan Result {
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
		if !EnsureCommitted(ctx, ch, worktreePath, commitMsg) {
			return
		}

		// Push the branch
		ch <- Result{Output: fmt.Sprintf("Pushing %s to origin...\n", branch)}
		output, err := executor.CombinedOutput(ctx, repoPath, "git", "push", "-u", "origin", branch)
		if err != nil {
			ch <- Result{Output: string(output), Error: fmt.Errorf("failed to push: %w", err), Done: true}
			return
		}
		ch <- Result{Output: string(output)}

		// Generate PR title and body with Claude
		ch <- Result{Output: "\nGenerating PR description with Claude...\n"}
		prTitle, prBody, err := GeneratePRTitleAndBody(ctx, repoPath, branch, issueNumber)
		var cmd *exec.Cmd
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
		if !EnsureCommitted(ctx, ch, childWorktreePath, commitMsg) {
			return
		}

		// Now merge the child branch into the parent worktree
		// The parent worktree should already be on the parent branch
		ch <- Result{Output: fmt.Sprintf("Merging %s into parent...\n", childBranch)}
		output, err := executor.CombinedOutput(ctx, parentWorktreePath, "git", "merge", childBranch, "--no-edit")
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

// PushUpdates commits any uncommitted changes and pushes to the remote branch.
// This is used after a PR has been created to push additional commits based on feedback.
// If commitMsg is provided and non-empty, it will be used directly instead of generating one.
func PushUpdates(ctx context.Context, repoPath, worktreePath, branch, commitMsg string) <-chan Result {
	ch := make(chan Result)

	go func() {
		defer close(ch)

		logger.Log("Git: Pushing updates for branch %s (worktree: %s)", branch, worktreePath)

		// First, check for uncommitted changes in the worktree and commit them
		if !EnsureCommitted(ctx, ch, worktreePath, commitMsg) {
			return
		}

		// Push the updates to the existing remote branch
		ch <- Result{Output: fmt.Sprintf("Pushing updates to %s...\n", branch)}
		output, err := executor.CombinedOutput(ctx, repoPath, "git", "push", "origin", branch)
		if err != nil {
			ch <- Result{Output: string(output), Error: fmt.Errorf("failed to push: %w", err), Done: true}
			return
		}
		ch <- Result{Output: string(output)}

		ch <- Result{Output: "\nUpdates pushed successfully! The PR will be updated automatically.\n", Done: true}
	}()

	return ch
}

// GenerateBranchNamesFromOptions uses Claude to generate short, descriptive git branch names
// from a list of option texts. Returns a map from option number to branch name.
// This batches all options into a single Claude call for efficiency.
func GenerateBranchNamesFromOptions(ctx context.Context, options []struct {
	Number int
	Text   string
}) (map[int]string, error) {
	if len(options) == 0 {
		return nil, nil
	}

	logger.Log("Git: Generating branch names for %d options", len(options))

	// Build the options list for the prompt
	var optionsList strings.Builder
	for _, opt := range options {
		fmt.Fprintf(&optionsList, "%d. %s\n", opt.Number, opt.Text)
	}

	prompt := fmt.Sprintf(`Generate short git branch names for each of these options/features:

%s
Rules:
1. Use lowercase letters and hyphens only (no spaces, underscores, or special characters)
2. Keep each name concise: 2-4 words maximum, under 30 characters total
3. Make each name descriptive of what that option/feature does
4. Output ONLY the branch names, one per line, in the format: "N. branch-name" where N is the option number
5. Do NOT include any preamble or explanation

Example output format:
1. add-dark-mode
2. use-redis-cache
3. sqlite-backend`, optionsList.String())

	args := []string{"--print", "-p", prompt}
	cmd := exec.CommandContext(ctx, "claude", args...)

	output, err := cmd.Output()
	if err != nil {
		logger.Log("Git: Claude branch name generation failed: %v", err)
		return nil, fmt.Errorf("failed to generate branch names with Claude: %w", err)
	}

	// Parse the output - expect lines like "1. branch-name"
	result := make(map[int]string)
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse "N. branch-name" or "N) branch-name"
		var num int
		var name string
		if _, err := fmt.Sscanf(line, "%d. %s", &num, &name); err != nil {
			if _, err := fmt.Sscanf(line, "%d) %s", &num, &name); err != nil {
				continue
			}
		}

		if num > 0 && name != "" {
			result[num] = sanitizeBranchName(name)
		}
	}

	logger.Log("Git: Generated %d branch names", len(result))
	return result, nil
}

// sanitizeBranchName ensures a branch name is valid for git
func sanitizeBranchName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)

	// Replace spaces and underscores with hyphens
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")

	// Remove any characters that aren't alphanumeric or hyphens
	var result strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			result.WriteRune(c)
		}
	}
	name = result.String()

	// Remove leading/trailing hyphens and collapse multiple hyphens
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	name = strings.Trim(name, "-")

	// Truncate if too long
	if len(name) > MaxBranchNameLength {
		name = name[:MaxBranchNameLength]
		// Don't end with a hyphen
		name = strings.TrimRight(name, "-")
	}

	return name
}

// DiffStats represents the statistics of changes in a worktree
type DiffStats struct {
	FilesChanged int // Number of files changed
	Additions    int // Number of lines added
	Deletions    int // Number of lines deleted
}

// GetDiffStats returns the diff statistics (files changed, additions, deletions)
// for uncommitted changes in the given worktree.
func GetDiffStats(worktreePath string) (*DiffStats, error) {
	ctx := context.Background()
	stats := &DiffStats{}

	// Get list of changed files using git status --porcelain
	output, err := executor.Output(ctx, worktreePath, "git", "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}

	// Count files from status output
	statusOutput := strings.TrimRight(string(output), "\n\r\t ")
	if statusOutput == "" {
		// No changes
		return stats, nil
	}

	lines := strings.Split(statusOutput, "\n")
	for _, line := range lines {
		if len(line) > 2 {
			stats.FilesChanged++
		}
	}

	// Get diff stats using git diff --numstat for unstaged changes
	// (without HEAD to get only working tree changes vs staged)
	numstatOutput, err := executor.Output(ctx, worktreePath, "git", "diff", "--numstat")
	if err != nil {
		logger.Log("Git: Warning - git diff --numstat failed: %v", err)
	}

	// Get staged changes separately
	cachedOutput, err := executor.Output(ctx, worktreePath, "git", "diff", "--numstat", "--cached")
	if err != nil {
		logger.Log("Git: Warning - git diff --numstat --cached failed: %v", err)
	}

	// Parse numstat output: each line is "additions<tab>deletions<tab>filename"
	parseNumstat := func(data []byte) {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) >= 2 {
				// Binary files show "-" for additions/deletions
				if parts[0] != "-" {
					var add int
					fmt.Sscanf(parts[0], "%d", &add)
					stats.Additions += add
				}
				if parts[1] != "-" {
					var del int
					fmt.Sscanf(parts[1], "%d", &del)
					stats.Deletions += del
				}
			}
		}
	}

	parseNumstat(numstatOutput)
	parseNumstat(cachedOutput)

	return stats, nil
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
	output, err := executor.Output(context.Background(), repoPath, "gh", "issue", "list",
		"--json", "number,title,body,url",
		"--state", "open",
	)
	if err != nil {
		return nil, fmt.Errorf("gh issue list failed: %w", err)
	}

	var issues []GitHubIssue
	if err := json.Unmarshal(output, &issues); err != nil {
		return nil, fmt.Errorf("failed to parse issues: %w", err)
	}

	return issues, nil
}

// RenameBranch renames a git branch in the given worktree.
// The worktree must have the branch checked out.
func RenameBranch(worktreePath, oldName, newName string) error {
	output, err := executor.CombinedOutput(context.Background(), worktreePath, "git", "branch", "-m", oldName, newName)
	if err != nil {
		return fmt.Errorf("git branch rename failed: %s: %w", string(output), err)
	}

	logger.Log("Git: Renamed branch %q to %q in %s", oldName, newName, worktreePath)
	return nil
}
