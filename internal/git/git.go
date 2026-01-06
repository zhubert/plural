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
func MergeToMain(ctx context.Context, repoPath, branch string) <-chan Result {
	ch := make(chan Result)

	go func() {
		defer close(ch)

		defaultBranch := GetDefaultBranch(repoPath)
		logger.Log("Git: Merging %s into %s in %s", branch, defaultBranch, repoPath)

		// First, checkout the default branch
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
func CreatePR(ctx context.Context, repoPath, branch string) <-chan Result {
	ch := make(chan Result)

	go func() {
		defer close(ch)

		defaultBranch := GetDefaultBranch(repoPath)
		logger.Log("Git: Creating PR for %s -> %s in %s", branch, defaultBranch, repoPath)

		// Check if gh CLI is available
		if _, err := exec.LookPath("gh"); err != nil {
			ch <- Result{Error: fmt.Errorf("gh CLI not found - install from https://cli.github.com"), Done: true}
			return
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
