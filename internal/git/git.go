package git

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/zhubert/plural/internal/logger"
)

// Result represents output from a git operation
type Result struct {
	Output string
	Error  error
	Done   bool
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

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
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
		cmd = exec.Command("git", "diff")
		cmd.Dir = worktreePath
		diffOutput, _ = cmd.Output()
	}

	// Also include untracked files in diff-like format
	cmd = exec.Command("git", "diff", "--cached")
	cmd.Dir = worktreePath
	cachedDiff, _ := cmd.Output()

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

// GenerateCommitMessage creates a commit message based on the changes
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
	statOutput, _ := cmd.Output()

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

// MergeToMain merges a branch into the default branch
// worktreePath is where Claude made changes - we commit any uncommitted changes first
func MergeToMain(ctx context.Context, repoPath, worktreePath, branch string) <-chan Result {
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
			ch <- Result{Output: "Committing changes...\n"}

			commitMsg, err := GenerateCommitMessage(worktreePath)
			if err != nil {
				ch <- Result{Error: fmt.Errorf("failed to generate commit message: %w", err), Done: true}
				return
			}

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
			ch <- Result{Output: string(output), Error: fmt.Errorf("merge failed: %w", err), Done: true}
			return
		}
		ch <- Result{Output: string(output)}

		ch <- Result{Output: fmt.Sprintf("\nSuccessfully merged %s into %s\n", branch, defaultBranch), Done: true}
	}()

	return ch
}

// CreatePR pushes the branch and creates a pull request using gh CLI
// worktreePath is where Claude made changes - we commit any uncommitted changes first
func CreatePR(ctx context.Context, repoPath, worktreePath, branch string) <-chan Result {
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
			ch <- Result{Output: "Committing changes...\n"}

			commitMsg, err := GenerateCommitMessage(worktreePath)
			if err != nil {
				ch <- Result{Error: fmt.Errorf("failed to generate commit message: %w", err), Done: true}
				return
			}

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

		// Create PR using gh CLI
		ch <- Result{Output: "\nCreating pull request...\n"}
		cmd = exec.CommandContext(ctx, "gh", "pr", "create",
			"--base", defaultBranch,
			"--head", branch,
			"--fill", // Use commit info for title/body
		)
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
