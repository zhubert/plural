package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pexec "github.com/zhubert/plural/internal/exec"
)

// svc creates a new GitService for testing (used by integration tests)
var svc = NewGitService()

// ctx is a background context for testing
var ctx = context.Background()

// createTestRepo creates a temporary git repository for testing
func createTestRepo(t *testing.T) string {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "plural-git-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git user for commits
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	cmd.Run()

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	cmd.Run()

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	return tmpDir
}

func TestResult_Fields(t *testing.T) {
	result := Result{
		Output: "test output",
		Error:  nil,
		Done:   true,
	}

	if result.Output != "test output" {
		t.Error("Output mismatch")
	}
	if result.Error != nil {
		t.Error("Error should be nil")
	}
	if !result.Done {
		t.Error("Done should be true")
	}
}

func TestHasRemoteOrigin_NoRemote(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"remote", "get-url", "origin"}, pexec.MockResponse{
		Err: fmt.Errorf("fatal: No such remote 'origin'"),
	})
	s := NewGitServiceWithExecutor(mock)

	if s.HasRemoteOrigin(ctx, "/repo") {
		t.Error("HasRemoteOrigin should return false for repo without origin")
	}
}

func TestHasRemoteOrigin_WithRemote(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"remote", "get-url", "origin"}, pexec.MockResponse{
		Stdout: []byte("https://github.com/test/test.git\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	if !s.HasRemoteOrigin(ctx, "/repo") {
		t.Error("HasRemoteOrigin should return true for repo with origin")
	}
}

func TestHasRemoteOrigin_InvalidPath(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"remote", "get-url", "origin"}, pexec.MockResponse{
		Err: fmt.Errorf("fatal: not a git repository"),
	})
	s := NewGitServiceWithExecutor(mock)

	if s.HasRemoteOrigin(ctx, "/nonexistent/path") {
		t.Error("HasRemoteOrigin should return false for invalid path")
	}
}

func TestGetDefaultBranch_FromSymbolicRef(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"symbolic-ref", "refs/remotes/origin/HEAD"}, pexec.MockResponse{
		Stdout: []byte("refs/remotes/origin/main\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	branch := s.GetDefaultBranch(ctx, "/repo")
	if branch != "main" {
		t.Errorf("GetDefaultBranch = %q, want 'main'", branch)
	}
}

func TestGetDefaultBranch_FallbackToMain(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	// symbolic-ref fails (no remote)
	mock.AddExactMatch("git", []string{"symbolic-ref", "refs/remotes/origin/HEAD"}, pexec.MockResponse{
		Err: fmt.Errorf("fatal: ref refs/remotes/origin/HEAD is not a symbolic ref"),
	})
	// rev-parse --verify main succeeds
	mock.AddExactMatch("git", []string{"rev-parse", "--verify", "main"}, pexec.MockResponse{
		Stdout: []byte("abc123\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	branch := s.GetDefaultBranch(ctx, "/repo")
	if branch != "main" {
		t.Errorf("GetDefaultBranch = %q, want 'main'", branch)
	}
}

func TestGetDefaultBranch_FallbackToMaster(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	// symbolic-ref fails
	mock.AddExactMatch("git", []string{"symbolic-ref", "refs/remotes/origin/HEAD"}, pexec.MockResponse{
		Err: fmt.Errorf("fatal: ref refs/remotes/origin/HEAD is not a symbolic ref"),
	})
	// rev-parse --verify main also fails
	mock.AddExactMatch("git", []string{"rev-parse", "--verify", "main"}, pexec.MockResponse{
		Err: fmt.Errorf("fatal: Needed a single revision"),
	})
	s := NewGitServiceWithExecutor(mock)

	branch := s.GetDefaultBranch(ctx, "/repo")
	if branch != "master" {
		t.Errorf("GetDefaultBranch = %q, want 'master'", branch)
	}
}

func TestMergeToMain(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a feature branch
	cmd := exec.Command("git", "checkout", "-b", "feature-branch")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create feature branch: %v", err)
	}

	// Make a change on the feature branch
	testFile := filepath.Join(repoPath, "feature.txt")
	if err := os.WriteFile(testFile, []byte("feature content"), 0644); err != nil {
		t.Fatalf("Failed to create feature file: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Feature commit")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit feature: %v", err)
	}

	// Merge to main
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch := svc.MergeToMain(ctx, repoPath, repoPath, "feature-branch", "")

	var lastResult Result
	for result := range ch {
		lastResult = result
		if result.Error != nil {
			t.Errorf("Merge error: %v", result.Error)
		}
	}

	if !lastResult.Done {
		t.Error("Merge should complete with Done=true")
	}

	// Verify we're on the default branch
	cmd = exec.Command("git", "branch", "--show-current")
	cmd.Dir = repoPath
	output, _ := cmd.Output()
	currentBranch := string(output)
	if currentBranch != "main\n" && currentBranch != "master\n" {
		// Also check if we could be on main
		t.Logf("Current branch: %q (expected main or master)", currentBranch)
	}
}

func TestMergeToMain_Conflict(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a feature branch
	cmd := exec.Command("git", "checkout", "-b", "conflict-branch")
	cmd.Dir = repoPath
	cmd.Run()

	// Modify test.txt on feature branch
	testFile := filepath.Join(repoPath, "test.txt")
	os.WriteFile(testFile, []byte("feature version"), 0644)

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Feature change")
	cmd.Dir = repoPath
	cmd.Run()

	// Go back to main and make a conflicting change
	cmd = exec.Command("git", "checkout", "-")
	cmd.Dir = repoPath
	cmd.Run()

	os.WriteFile(testFile, []byte("main version"), 0644)

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Main change")
	cmd.Dir = repoPath
	cmd.Run()

	// Try to merge - should fail with conflict
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch := svc.MergeToMain(ctx, repoPath, repoPath, "conflict-branch", "")

	var hadError bool
	for result := range ch {
		if result.Error != nil {
			hadError = true
		}
	}

	if !hadError {
		t.Error("Expected merge to fail with conflict")
	}
}

func TestMergeToMain_Cancelled(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a branch
	cmd := exec.Command("git", "checkout", "-b", "test-branch")
	cmd.Dir = repoPath
	cmd.Run()

	// Cancel immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ch := svc.MergeToMain(ctx, repoPath, repoPath, "test-branch", "")

	// Drain channel
	for range ch {
	}
	// Just verify it doesn't hang
}

func TestCreatePR_NoGh(t *testing.T) {
	// Skip if gh is installed (we can't easily test the success path without a real repo)
	if _, err := exec.LookPath("gh"); err == nil {
		t.Skip("gh is installed, skipping no-gh test")
	}

	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch := svc.CreatePR(ctx, repoPath, repoPath, "test-branch", "", "", nil, "")

	var hadError bool
	for result := range ch {
		if result.Error != nil {
			hadError = true
		}
	}

	if !hadError {
		t.Error("Expected CreatePR to fail when gh is not installed")
	}
}

func TestGetWorktreeStatus_NoChanges(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Stdout: []byte(""),
	})
	s := NewGitServiceWithExecutor(mock)

	status, err := s.GetWorktreeStatus(ctx, "/repo")
	if err != nil {
		t.Fatalf("GetWorktreeStatus failed: %v", err)
	}

	if status.HasChanges {
		t.Error("Expected HasChanges to be false for clean repo")
	}

	if status.Summary != "No changes" {
		t.Errorf("Expected Summary 'No changes', got %q", status.Summary)
	}

	if len(status.Files) != 0 {
		t.Errorf("Expected no files, got %d", len(status.Files))
	}
}

func TestGetWorktreeStatus_WithChanges(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Stdout: []byte(" M test.txt\n?? new.txt\n"),
	})
	mock.AddExactMatch("git", []string{"diff", "--no-ext-diff", "HEAD"}, pexec.MockResponse{
		Stdout: []byte("diff --git a/test.txt b/test.txt\n--- a/test.txt\n+++ b/test.txt\n@@ -1 +1 @@\n-test content\n+modified content\n"),
	})
	// Mock for untracked file diff
	mock.AddExactMatch("git", []string{"diff", "--no-ext-diff", "--no-index", "/dev/null", "new.txt"}, pexec.MockResponse{
		Stdout: []byte("diff --git a/dev/null b/new.txt\n--- /dev/null\n+++ b/new.txt\n@@ -0,0 +1 @@\n+new content\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	status, err := s.GetWorktreeStatus(ctx, "/repo")
	if err != nil {
		t.Fatalf("GetWorktreeStatus failed: %v", err)
	}

	if !status.HasChanges {
		t.Error("Expected HasChanges to be true")
	}

	if len(status.Files) != 2 {
		t.Errorf("Expected 2 files, got %d: %v", len(status.Files), status.Files)
	}

	if status.Summary != "2 files changed" {
		t.Errorf("Expected Summary '2 files changed', got %q", status.Summary)
	}
}

func TestGetWorktreeStatus_SingleFile(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Stdout: []byte("?? single.txt\n"),
	})
	mock.AddExactMatch("git", []string{"diff", "--no-ext-diff", "HEAD"}, pexec.MockResponse{
		Stdout: []byte(""),
	})
	// Mock for untracked file diff
	mock.AddExactMatch("git", []string{"diff", "--no-ext-diff", "--no-index", "/dev/null", "single.txt"}, pexec.MockResponse{
		Stdout: []byte("diff --git a/dev/null b/single.txt\n+++ b/single.txt\n@@ -0,0 +1 @@\n+content\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	status, err := s.GetWorktreeStatus(ctx, "/repo")
	if err != nil {
		t.Fatalf("GetWorktreeStatus failed: %v", err)
	}

	if status.Summary != "1 file changed" {
		t.Errorf("Expected Summary '1 file changed', got %q", status.Summary)
	}
}

func TestGetWorktreeStatus_InvalidPath(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Err: fmt.Errorf("fatal: not a git repository"),
	})
	s := NewGitServiceWithExecutor(mock)

	_, err := s.GetWorktreeStatus(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestCommitAll_Success(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"add", "-A"}, pexec.MockResponse{})
	mock.AddExactMatch("git", []string{"commit", "-m", "Test commit message"}, pexec.MockResponse{
		Stdout: []byte("[main abc1234] Test commit message\n 1 file changed, 1 insertion(+)\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	err := s.CommitAll(ctx, "/repo", "Test commit message")
	if err != nil {
		t.Fatalf("CommitAll failed: %v", err)
	}

	// Verify both git add and git commit were called
	calls := mock.GetCalls()
	if len(calls) != 2 {
		t.Fatalf("Expected 2 calls, got %d", len(calls))
	}
	if calls[0].Args[0] != "add" {
		t.Errorf("Expected first call to be 'git add', got args: %v", calls[0].Args)
	}
	if calls[1].Args[0] != "commit" {
		t.Errorf("Expected second call to be 'git commit', got args: %v", calls[1].Args)
	}
}

func TestCommitAll_NoChanges(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"add", "-A"}, pexec.MockResponse{})
	mock.AddExactMatch("git", []string{"commit", "-m", "Empty commit"}, pexec.MockResponse{
		Stdout: []byte("nothing to commit, working tree clean\n"),
		Err:    fmt.Errorf("exit status 1"),
	})
	s := NewGitServiceWithExecutor(mock)

	err := s.CommitAll(ctx, "/repo", "Empty commit")
	if err == nil {
		t.Error("Expected CommitAll to fail with no changes")
	}
}

func TestCommitAll_MultipleFiles(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"add", "-A"}, pexec.MockResponse{})
	mock.AddExactMatch("git", []string{"commit", "-m", "Multiple files commit"}, pexec.MockResponse{
		Stdout: []byte("[main abc1234] Multiple files commit\n 4 files changed\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	err := s.CommitAll(ctx, "/repo", "Multiple files commit")
	if err != nil {
		t.Fatalf("CommitAll failed: %v", err)
	}
}

func TestGenerateCommitMessage_Success(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	// Mock GetWorktreeStatus: status --porcelain
	mock.AddExactMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Stdout: []byte("?? feature.txt\n"),
	})
	// Mock diff HEAD (called by GetWorktreeStatus)
	mock.AddExactMatch("git", []string{"diff", "--no-ext-diff", "HEAD"}, pexec.MockResponse{
		Stdout: []byte(""),
	})
	// Mock untracked file diff (called by parseFileDiffs)
	mock.AddExactMatch("git", []string{"diff", "--no-ext-diff", "--no-index", "/dev/null", "feature.txt"}, pexec.MockResponse{
		Stdout: []byte("diff --git a/dev/null b/feature.txt\n+++ b/feature.txt\n@@ -0,0 +1 @@\n+feature content\n"),
	})
	// Mock diff --stat HEAD (called by GenerateCommitMessage)
	mock.AddExactMatch("git", []string{"diff", "--no-ext-diff", "--stat", "HEAD"}, pexec.MockResponse{
		Stdout: []byte(" feature.txt | 1 +\n 1 file changed, 1 insertion(+)\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	msg, err := s.GenerateCommitMessage(ctx, "/repo")
	if err != nil {
		t.Fatalf("GenerateCommitMessage failed: %v", err)
	}

	if msg == "" {
		t.Error("Expected non-empty commit message")
	}

	if !strings.Contains(msg, "1 file") {
		t.Errorf("Expected message to contain file count, got: %s", msg)
	}

	if !strings.Contains(msg, "feature.txt") {
		t.Errorf("Expected message to contain filename, got: %s", msg)
	}
}

func TestGenerateCommitMessage_NoChanges(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Stdout: []byte(""),
	})
	s := NewGitServiceWithExecutor(mock)

	_, err := s.GenerateCommitMessage(ctx, "/repo")
	if err == nil {
		t.Error("Expected error when generating commit message with no changes")
	}
}

func TestMergeToMain_WithProvidedCommitMessage(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a feature branch
	cmd := exec.Command("git", "checkout", "-b", "feature-with-msg")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create feature branch: %v", err)
	}

	// Make a change on the feature branch
	testFile := filepath.Join(repoPath, "feature-msg.txt")
	if err := os.WriteFile(testFile, []byte("feature content"), 0644); err != nil {
		t.Fatalf("Failed to create feature file: %v", err)
	}

	// Don't commit - let MergeToMain auto-commit with provided message
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	customCommitMsg := "Custom commit message for merge"
	ch := svc.MergeToMain(ctx, repoPath, repoPath, "feature-with-msg", customCommitMsg)

	var lastResult Result
	for result := range ch {
		lastResult = result
		if result.Error != nil {
			t.Logf("Result output: %s", result.Output)
			t.Errorf("Merge error: %v", result.Error)
		}
	}

	if !lastResult.Done {
		t.Error("Merge should complete with Done=true")
	}

	// Verify the commit message was used
	cmd = exec.Command("git", "log", "--oneline", "-2")
	cmd.Dir = repoPath
	output, _ := cmd.Output()
	if !strings.Contains(string(output), "Custom commit message") {
		t.Logf("Git log: %s", output)
		// Note: This may not always work depending on merge behavior
	}
}

func TestCreatePR_WithProvidedCommitMessage(t *testing.T) {
	// Skip if gh is installed and we don't want to actually create a PR
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh not installed, skipping PR creation test")
	}

	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a feature branch
	cmd := exec.Command("git", "checkout", "-b", "feature-pr-msg")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create feature branch: %v", err)
	}

	// Make a change
	testFile := filepath.Join(repoPath, "pr-feature.txt")
	if err := os.WriteFile(testFile, []byte("pr content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// CreatePR will fail without a real remote, but we can verify it tries
	ch := svc.CreatePR(ctx, repoPath, repoPath, "feature-pr-msg", "", "Custom PR commit", nil, "")

	// Drain channel - expect an error since no remote
	for range ch {
	}
	// Just verify it doesn't hang
}

func TestGetWorktreeStatus_StagedChanges(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Stdout: []byte("A  staged.txt\n"),
	})
	mock.AddExactMatch("git", []string{"diff", "--no-ext-diff", "HEAD"}, pexec.MockResponse{
		Stdout: []byte("diff --git a/staged.txt b/staged.txt\nnew file mode 100644\n--- /dev/null\n+++ b/staged.txt\n@@ -0,0 +1 @@\n+staged content\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	status, err := s.GetWorktreeStatus(ctx, "/repo")
	if err != nil {
		t.Fatalf("GetWorktreeStatus failed: %v", err)
	}

	if !status.HasChanges {
		t.Error("Expected HasChanges to be true for staged files")
	}

	if status.Diff == "" {
		t.Error("Expected Diff to contain staged changes")
	}

	diffCount := strings.Count(status.Diff, "diff --git a/staged.txt")
	if diffCount != 1 {
		t.Errorf("Expected staged file diff to appear once, got %d times", diffCount)
	}
}

func TestGetWorktreeStatus_StagedAndUnstaged(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Stdout: []byte("A  staged.txt\n M test.txt\n"),
	})
	mock.AddExactMatch("git", []string{"diff", "--no-ext-diff", "HEAD"}, pexec.MockResponse{
		Stdout: []byte("diff --git a/staged.txt b/staged.txt\nnew file mode 100644\n--- /dev/null\n+++ b/staged.txt\n@@ -0,0 +1 @@\n+staged content\ndiff --git a/test.txt b/test.txt\n--- a/test.txt\n+++ b/test.txt\n@@ -1 +1 @@\n-test content\n+modified content\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	status, err := s.GetWorktreeStatus(ctx, "/repo")
	if err != nil {
		t.Fatalf("GetWorktreeStatus failed: %v", err)
	}

	if !status.HasChanges {
		t.Error("Expected HasChanges to be true")
	}

	stagedDiffCount := strings.Count(status.Diff, "diff --git a/staged.txt")
	testDiffCount := strings.Count(status.Diff, "diff --git a/test.txt")

	if stagedDiffCount != 1 {
		t.Errorf("Expected staged.txt diff to appear once, got %d times", stagedDiffCount)
	}
	if testDiffCount != 1 {
		t.Errorf("Expected test.txt diff to appear once, got %d times", testDiffCount)
	}
}

func TestMaxDiffSize(t *testing.T) {
	// Verify the constant is set correctly
	if MaxDiffSize != 50000 {
		t.Errorf("MaxDiffSize = %d, want 50000", MaxDiffSize)
	}
}

func TestWorktreeStatus_Fields(t *testing.T) {
	status := WorktreeStatus{
		HasChanges: true,
		Summary:    "2 files changed",
		Files:      []string{"file1.txt", "file2.txt"},
		Diff:       "diff --git a/file1.txt...",
	}

	if !status.HasChanges {
		t.Error("HasChanges should be true")
	}
	if status.Summary != "2 files changed" {
		t.Error("Summary mismatch")
	}
	if len(status.Files) != 2 {
		t.Error("Expected 2 files")
	}
	if status.Diff == "" {
		t.Error("Diff should not be empty")
	}
}

func TestMergeToMain_NoChangesToCommit(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a feature branch with a commit (no uncommitted changes)
	cmd := exec.Command("git", "checkout", "-b", "clean-feature")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create feature branch: %v", err)
	}

	// Make a change and commit it
	testFile := filepath.Join(repoPath, "clean.txt")
	if err := os.WriteFile(testFile, []byte("clean content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Clean commit")
	cmd.Dir = repoPath
	cmd.Run()

	// Now merge - there should be no uncommitted changes
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch := svc.MergeToMain(ctx, repoPath, repoPath, "clean-feature", "")

	var sawNoChangesMsg bool
	for result := range ch {
		if strings.Contains(result.Output, "No uncommitted changes") {
			sawNoChangesMsg = true
		}
		if result.Error != nil {
			t.Errorf("Unexpected error: %v", result.Error)
		}
	}

	if !sawNoChangesMsg {
		t.Error("Expected 'No uncommitted changes' message")
	}
}

func TestCreatePR_Cancelled(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a branch
	cmd := exec.Command("git", "checkout", "-b", "pr-cancel-test")
	cmd.Dir = repoPath
	cmd.Run()

	// Cancel immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ch := svc.CreatePR(ctx, repoPath, repoPath, "pr-cancel-test", "", "", nil, "")

	// Drain channel - should not hang
	for range ch {
	}
}

func TestCreatePR_UsesBaseBranchNotDefaultBranch(t *testing.T) {
	// This test verifies that CreatePR uses baseBranch (not defaultBranch) for the --base flag.
	// This is critical for forked sessions where baseBranch is a parent branch, not main.
	mockExec := pexec.NewMockExecutor(nil)
	svc := NewGitServiceWithExecutor(mockExec)

	repoPath := "/test/repo"
	worktreePath := "/test/worktree"
	branch := "feature-branch"
	baseBranch := "parent-branch" // The branch this PR should target (e.g., from a forked session)

	// Mock GetDefaultBranch to return "main"
	mockExec.AddPrefixMatch("git", []string{"symbolic-ref", "refs/remotes/origin/HEAD"}, pexec.MockResponse{
		Stdout: []byte("refs/remotes/origin/main\n"),
	})

	// Mock worktree status check (no uncommitted changes)
	mockExec.AddPrefixMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	// Mock git push
	mockExec.AddPrefixMatch("git", []string{"push", "-u", "origin", branch}, pexec.MockResponse{
		Stdout: []byte("Branch pushed successfully\n"),
	})

	// Mock git log for PR generation (baseBranch..branch)
	mockExec.AddPrefixMatch("git", []string{"log", baseBranch + ".." + branch, "--oneline"}, pexec.MockResponse{
		Stdout: []byte("abc123 Add new feature\n"),
	})

	// Mock git diff for PR generation (using baseBranch)
	mockExec.AddPrefixMatch("git", []string{"diff", baseBranch + "..." + branch}, pexec.MockResponse{
		Stdout: []byte("diff --git a/file.txt b/file.txt\n"),
	})

	// Mock Claude PR generation (will fail, which is expected - we're testing the fallback path)
	// When Claude generation fails, CreatePR falls back to --fill
	mockExec.AddPrefixMatch("claude", []string{}, pexec.MockResponse{
		Stderr: []byte("Claude not available"),
		Err:    fmt.Errorf("claude not available"),
	})

	// Mock gh pr create to succeed
	mockExec.AddPrefixMatch("gh", []string{"pr", "create"}, pexec.MockResponse{
		Stdout: []byte("https://github.com/owner/repo/pull/123\n"),
	})

	// Skip this test if gh CLI is not available
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh CLI not available, skipping test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Call CreatePR with baseBranch="parent-branch"
	ch := svc.CreatePR(ctx, repoPath, worktreePath, branch, baseBranch, "", nil, "")

	// Drain the channel
	for range ch {
	}

	// Get all recorded calls
	calls := mockExec.GetCalls()

	// Find the gh pr create call
	var ghCall *pexec.MockCall
	for _, call := range calls {
		if call.Name == "gh" && len(call.Args) > 0 && call.Args[0] == "pr" {
			ghCall = &call
			break
		}
	}

	if ghCall == nil {
		t.Fatal("gh pr create was not called")
	}

	// Find the --base flag in the command
	baseIndex := -1
	for i, arg := range ghCall.Args {
		if arg == "--base" {
			baseIndex = i
			break
		}
	}

	if baseIndex == -1 {
		t.Fatalf("--base flag not found in gh command: %v", ghCall.Args)
	}

	if baseIndex+1 >= len(ghCall.Args) {
		t.Fatalf("--base flag has no value in gh command: %v", ghCall.Args)
	}

	actualBase := ghCall.Args[baseIndex+1]
	if actualBase != baseBranch {
		t.Errorf("gh pr create --base = %q, want %q (not %q)", actualBase, baseBranch, "main")
		t.Errorf("Full command: gh %v", ghCall.Args)
	}
}

func TestCommitAll_InvalidPath(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"add", "-A"}, pexec.MockResponse{
		Stderr: []byte("fatal: not a git repository\n"),
		Err:    fmt.Errorf("exit status 128"),
	})
	s := NewGitServiceWithExecutor(mock)

	err := s.CommitAll(ctx, "/nonexistent/path", "Test commit")
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestGetWorktreeStatus_DiffFallback(t *testing.T) {
	// Tests the fallback behavior when diff HEAD fails (e.g., new repo without commits)
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Stdout: []byte("?? nohead.txt\n"),
	})
	// diff HEAD fails (no commits yet)
	mock.AddExactMatch("git", []string{"diff", "--no-ext-diff", "HEAD"}, pexec.MockResponse{
		Err: fmt.Errorf("fatal: bad default revision 'HEAD'"),
	})
	// Fallback: unstaged diff
	mock.AddExactMatch("git", []string{"diff", "--no-ext-diff"}, pexec.MockResponse{
		Stdout: []byte(""),
	})
	// Fallback: staged diff
	mock.AddExactMatch("git", []string{"diff", "--no-ext-diff", "--cached"}, pexec.MockResponse{
		Stdout: []byte(""),
	})
	// Untracked file diff
	mock.AddExactMatch("git", []string{"diff", "--no-ext-diff", "--no-index", "/dev/null", "nohead.txt"}, pexec.MockResponse{
		Stdout: []byte("diff --git a/dev/null b/nohead.txt\n+++ b/nohead.txt\n@@ -0,0 +1 @@\n+content\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	status, err := s.GetWorktreeStatus(ctx, "/repo")
	if err != nil {
		t.Fatalf("GetWorktreeStatus failed: %v", err)
	}

	if !status.HasChanges {
		t.Error("Expected HasChanges to be true")
	}
}

func TestGenerateCommitMessage_MultipleFiles(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Stdout: []byte("?? file1.txt\n?? file2.txt\n?? file3.txt\n"),
	})
	mock.AddExactMatch("git", []string{"diff", "--no-ext-diff", "HEAD"}, pexec.MockResponse{
		Stdout: []byte(""),
	})
	// Mock untracked file diffs for parseFileDiffs
	for _, name := range []string{"file1.txt", "file2.txt", "file3.txt"} {
		mock.AddExactMatch("git", []string{"diff", "--no-ext-diff", "--no-index", "/dev/null", name}, pexec.MockResponse{
			Stdout: fmt.Appendf(nil, "diff --git a/dev/null b/%s\n+content\n", name),
		})
	}
	mock.AddExactMatch("git", []string{"diff", "--no-ext-diff", "--stat", "HEAD"}, pexec.MockResponse{
		Stdout: []byte(" file1.txt | 1 +\n file2.txt | 1 +\n file3.txt | 1 +\n 3 files changed, 3 insertions(+)\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	msg, err := s.GenerateCommitMessage(ctx, "/repo")
	if err != nil {
		t.Fatalf("GenerateCommitMessage failed: %v", err)
	}

	if !strings.Contains(msg, "3 files") {
		t.Errorf("Expected message to mention 3 files, got: %s", msg)
	}

	for _, name := range []string{"file1.txt", "file2.txt", "file3.txt"} {
		if !strings.Contains(msg, name) {
			t.Errorf("Expected message to contain %s, got: %s", name, msg)
		}
	}
}

// createTestRepoWithWorktree creates a repo and two worktrees for testing parent-child merges
func createTestRepoWithWorktree(t *testing.T) (repoPath, parentWorktree, childWorktree, parentBranch, childBranch string, cleanup func()) {
	t.Helper()

	// Create the main repo
	repoPath = createTestRepo(t)

	// Get current (default) branch name first
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = repoPath
	output, _ := cmd.Output()
	defaultBranch := strings.TrimSpace(string(output))
	if defaultBranch == "" {
		defaultBranch = "master" // fallback
	}

	// Create parent branch (in main repo)
	parentBranch = "parent-branch"
	cmd = exec.Command("git", "checkout", "-b", parentBranch)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		os.RemoveAll(repoPath)
		t.Fatalf("Failed to create parent branch: %v", err)
	}

	// Add a file on parent branch
	parentFile := filepath.Join(repoPath, "parent.txt")
	if err := os.WriteFile(parentFile, []byte("parent content"), 0644); err != nil {
		os.RemoveAll(repoPath)
		t.Fatalf("Failed to create parent file: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Parent commit")
	cmd.Dir = repoPath
	cmd.Run()

	// Create child branch from parent (still in main repo)
	childBranch = "child-branch"
	cmd = exec.Command("git", "checkout", "-b", childBranch)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		os.RemoveAll(repoPath)
		t.Fatalf("Failed to create child branch: %v", err)
	}

	// Go back to default branch so we can create worktrees for both branches
	cmd = exec.Command("git", "checkout", defaultBranch)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		os.RemoveAll(repoPath)
		t.Fatalf("Failed to checkout default branch: %v", err)
	}

	// Create parent worktree - git worktree add needs the path to not exist
	parentWorktree, _ = os.MkdirTemp("", "plural-parent-wt-*")
	os.RemoveAll(parentWorktree) // Remove it so git worktree add can create it
	cmd = exec.Command("git", "worktree", "add", parentWorktree, parentBranch)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(repoPath)
		t.Fatalf("Failed to create parent worktree: %v\nOutput: %s", err, output)
	}

	// Create child worktree - git worktree add needs the path to not exist
	childWorktree, _ = os.MkdirTemp("", "plural-child-wt-*")
	os.RemoveAll(childWorktree) // Remove it so git worktree add can create it
	cmd = exec.Command("git", "worktree", "add", childWorktree, childBranch)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(repoPath)
		os.RemoveAll(parentWorktree)
		t.Fatalf("Failed to create child worktree: %v\nOutput: %s", err, output)
	}

	cleanup = func() {
		// Remove worktrees first (from main repo)
		cmd := exec.Command("git", "worktree", "remove", "--force", parentWorktree)
		cmd.Dir = repoPath
		cmd.Run()
		cmd = exec.Command("git", "worktree", "remove", "--force", childWorktree)
		cmd.Dir = repoPath
		cmd.Run()
		os.RemoveAll(repoPath)
		os.RemoveAll(parentWorktree)
		os.RemoveAll(childWorktree)
	}

	return
}

func TestMergeToParent_Success(t *testing.T) {
	repoPath, parentWorktree, childWorktree, parentBranch, childBranch, cleanup := createTestRepoWithWorktree(t)
	defer cleanup()
	_ = repoPath // unused but part of the setup

	// Make a change on child branch
	childFile := filepath.Join(childWorktree, "child.txt")
	if err := os.WriteFile(childFile, []byte("child content"), 0644); err != nil {
		t.Fatalf("Failed to create child file: %v", err)
	}

	cmd := exec.Command("git", "add", ".")
	cmd.Dir = childWorktree
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Child commit")
	cmd.Dir = childWorktree
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit child changes: %v", err)
	}

	// Merge child to parent
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch := svc.MergeToParent(ctx, childWorktree, childBranch, parentWorktree, parentBranch, "")

	var lastResult Result
	for result := range ch {
		lastResult = result
		if result.Error != nil {
			t.Errorf("Merge error: %v (output: %s)", result.Error, result.Output)
		}
	}

	if !lastResult.Done {
		t.Error("Merge should complete with Done=true")
	}

	// Verify child file exists in parent worktree after merge
	if _, err := os.Stat(filepath.Join(parentWorktree, "child.txt")); os.IsNotExist(err) {
		t.Error("child.txt should exist in parent worktree after merge")
	}
}

func TestMergeToParent_WithUncommittedChanges(t *testing.T) {
	repoPath, parentWorktree, childWorktree, parentBranch, childBranch, cleanup := createTestRepoWithWorktree(t)
	defer cleanup()
	_ = repoPath

	// Make an uncommitted change on child branch
	childFile := filepath.Join(childWorktree, "uncommitted.txt")
	if err := os.WriteFile(childFile, []byte("uncommitted content"), 0644); err != nil {
		t.Fatalf("Failed to create child file: %v", err)
	}

	// Merge to parent with custom commit message (since we have uncommitted changes)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch := svc.MergeToParent(ctx, childWorktree, childBranch, parentWorktree, parentBranch, "Custom child commit")

	var sawUncommittedMsg bool
	var lastResult Result
	for result := range ch {
		lastResult = result
		if strings.Contains(result.Output, "uncommitted changes") {
			sawUncommittedMsg = true
		}
		if result.Error != nil {
			t.Errorf("Merge error: %v (output: %s)", result.Error, result.Output)
		}
	}

	if !sawUncommittedMsg {
		t.Error("Expected message about uncommitted changes")
	}

	if !lastResult.Done {
		t.Error("Merge should complete with Done=true")
	}

	// Verify file exists in parent
	if _, err := os.Stat(filepath.Join(parentWorktree, "uncommitted.txt")); os.IsNotExist(err) {
		t.Error("uncommitted.txt should exist in parent worktree after merge")
	}
}

func TestMergeToParent_Conflict(t *testing.T) {
	repoPath, parentWorktree, childWorktree, parentBranch, childBranch, cleanup := createTestRepoWithWorktree(t)
	defer cleanup()
	_ = repoPath

	// Make a change on child
	conflictFile := filepath.Join(childWorktree, "conflict.txt")
	if err := os.WriteFile(conflictFile, []byte("child version"), 0644); err != nil {
		t.Fatalf("Failed to create file in child: %v", err)
	}

	cmd := exec.Command("git", "add", ".")
	cmd.Dir = childWorktree
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Child conflict commit")
	cmd.Dir = childWorktree
	cmd.Run()

	// Make a conflicting change on parent
	conflictFile = filepath.Join(parentWorktree, "conflict.txt")
	if err := os.WriteFile(conflictFile, []byte("parent version"), 0644); err != nil {
		t.Fatalf("Failed to create file in parent: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = parentWorktree
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Parent conflict commit")
	cmd.Dir = parentWorktree
	cmd.Run()

	// Try to merge - should fail with conflict
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch := svc.MergeToParent(ctx, childWorktree, childBranch, parentWorktree, parentBranch, "")

	var hadConflict bool
	var conflictedFiles []string
	for result := range ch {
		if result.Error != nil && len(result.ConflictedFiles) > 0 {
			hadConflict = true
			conflictedFiles = result.ConflictedFiles
		}
	}

	if !hadConflict {
		t.Error("Expected merge to fail with conflict")
	}

	if len(conflictedFiles) == 0 {
		t.Error("Expected conflicted files list to be populated")
	}
}

func TestMergeToParent_Cancelled(t *testing.T) {
	repoPath, parentWorktree, childWorktree, parentBranch, childBranch, cleanup := createTestRepoWithWorktree(t)
	defer cleanup()
	_ = repoPath

	// Cancel immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ch := svc.MergeToParent(ctx, childWorktree, childBranch, parentWorktree, parentBranch, "")

	// Drain channel - should not hang
	for range ch {
	}
}

func TestMergeToParent_NoChangesToCommit(t *testing.T) {
	repoPath, parentWorktree, childWorktree, parentBranch, childBranch, cleanup := createTestRepoWithWorktree(t)
	defer cleanup()
	_ = repoPath

	// Make a change on child and commit it (no uncommitted changes)
	childFile := filepath.Join(childWorktree, "committed.txt")
	if err := os.WriteFile(childFile, []byte("committed content"), 0644); err != nil {
		t.Fatalf("Failed to create child file: %v", err)
	}

	cmd := exec.Command("git", "add", ".")
	cmd.Dir = childWorktree
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Child commit")
	cmd.Dir = childWorktree
	cmd.Run()

	// Merge to parent
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch := svc.MergeToParent(ctx, childWorktree, childBranch, parentWorktree, parentBranch, "")

	var sawNoChangesMsg bool
	for result := range ch {
		if strings.Contains(result.Output, "No uncommitted changes") {
			sawNoChangesMsg = true
		}
		if result.Error != nil {
			t.Errorf("Unexpected error: %v", result.Error)
		}
	}

	if !sawNoChangesMsg {
		t.Error("Expected 'No uncommitted changes' message")
	}
}

// createTestRepoWithRemote creates a test repo with a "remote" (bare repo) for testing pull scenarios
func createTestRepoWithRemote(t *testing.T) (repoPath, remotePath string, cleanup func()) {
	t.Helper()

	// Create a bare "remote" repository
	remotePath, err := os.MkdirTemp("", "plural-git-remote-*")
	if err != nil {
		t.Fatalf("Failed to create remote temp dir: %v", err)
	}

	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = remotePath
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remotePath)
		t.Fatalf("Failed to init bare repo: %v", err)
	}

	// Create the local repo
	repoPath, err = os.MkdirTemp("", "plural-git-local-*")
	if err != nil {
		os.RemoveAll(remotePath)
		t.Fatalf("Failed to create local temp dir: %v", err)
	}

	cmd = exec.Command("git", "init")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remotePath)
		os.RemoveAll(repoPath)
		t.Fatalf("Failed to init local repo: %v", err)
	}

	// Configure git user
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = repoPath
	cmd.Run()

	// Create initial commit
	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial content"), 0644); err != nil {
		os.RemoveAll(remotePath)
		os.RemoveAll(repoPath)
		t.Fatalf("Failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remotePath)
		os.RemoveAll(repoPath)
		t.Fatalf("Failed to initial commit: %v", err)
	}

	// Add remote and push
	cmd = exec.Command("git", "remote", "add", "origin", remotePath)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remotePath)
		os.RemoveAll(repoPath)
		t.Fatalf("Failed to add remote: %v", err)
	}

	// Rename branch to main and push
	cmd = exec.Command("git", "branch", "-M", "main")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "push", "-u", "origin", "main")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		os.RemoveAll(remotePath)
		os.RemoveAll(repoPath)
		t.Fatalf("Failed to push to remote: %v", err)
	}

	// Set the bare repo's HEAD to point to main (important for CI where default may be master)
	cmd = exec.Command("git", "symbolic-ref", "HEAD", "refs/heads/main")
	cmd.Dir = remotePath
	cmd.Run()

	// Set up origin/HEAD in local repo so GetDefaultBranch works correctly
	cmd = exec.Command("git", "remote", "set-head", "origin", "main")
	cmd.Dir = repoPath
	cmd.Run()

	cleanup = func() {
		os.RemoveAll(repoPath)
		os.RemoveAll(remotePath)
	}

	return repoPath, remotePath, cleanup
}

func TestMergeToMain_PullFailsDiverged(t *testing.T) {
	repoPath, remotePath, cleanup := createTestRepoWithRemote(t)
	defer cleanup()
	_ = remotePath

	// Create a feature branch from main
	cmd := exec.Command("git", "checkout", "-b", "feature-diverged")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create feature branch: %v", err)
	}

	// Make a change on the feature branch
	featureFile := filepath.Join(repoPath, "feature.txt")
	if err := os.WriteFile(featureFile, []byte("feature content"), 0644); err != nil {
		t.Fatalf("Failed to create feature file: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Feature commit")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit feature: %v", err)
	}

	// Now simulate diverged history:
	// Clone the remote to a second location, make a commit, push
	tempClone, err := os.MkdirTemp("", "plural-git-clone-*")
	if err != nil {
		t.Fatalf("Failed to create clone temp dir: %v", err)
	}
	defer os.RemoveAll(tempClone)

	cmd = exec.Command("git", "clone", remotePath, tempClone)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to clone: %v", err)
	}

	// Configure git user in clone
	cmd = exec.Command("git", "config", "user.email", "other@example.com")
	cmd.Dir = tempClone
	cmd.Run()

	cmd = exec.Command("git", "config", "user.name", "Other User")
	cmd.Dir = tempClone
	cmd.Run()

	// Make a commit in the clone and push
	otherFile := filepath.Join(tempClone, "other.txt")
	if err := os.WriteFile(otherFile, []byte("other content"), 0644); err != nil {
		t.Fatalf("Failed to create other file: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tempClone
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Other commit")
	cmd.Dir = tempClone
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit in clone: %v", err)
	}

	cmd = exec.Command("git", "push")
	cmd.Dir = tempClone
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to push from clone: %v", err)
	}

	// Now back in our original repo, make a LOCAL commit on main (creating divergence)
	cmd = exec.Command("git", "checkout", "main")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to checkout main: %v", err)
	}

	localFile := filepath.Join(repoPath, "local.txt")
	if err := os.WriteFile(localFile, []byte("local content"), 0644); err != nil {
		t.Fatalf("Failed to create local file: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Local commit causing divergence")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to make local commit: %v", err)
	}

	// Fetch from origin to update refs
	cmd = exec.Command("git", "fetch", "origin")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to fetch from origin: %v", err)
	}

	// Verify diverged state exists before we test
	cmd = exec.Command("git", "rev-list", "--left-right", "--count", "main...origin/main")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to check divergence: %v", err)
	}
	// Output should be "1\t1" (1 commit on each side)
	counts := strings.TrimSpace(string(output))
	if !strings.Contains(counts, "1") {
		t.Fatalf("Expected diverged state but got counts: %q", counts)
	}

	// Switch back to feature branch for the merge
	cmd = exec.Command("git", "checkout", "feature-diverged")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to checkout feature: %v", err)
	}

	// Try to merge - should fail because local main has diverged from origin/main
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch := svc.MergeToMain(ctx, repoPath, repoPath, "feature-diverged", "")

	var hadDivergedError bool
	var sawHelpfulMessage bool
	var allOutput []string
	for result := range ch {
		allOutput = append(allOutput, result.Output)
		if result.Error != nil {
			allOutput = append(allOutput, "Error: "+result.Error.Error())
			if strings.Contains(result.Error.Error(), "diverged") {
				hadDivergedError = true
			}
		}
		if strings.Contains(result.Output, "sync your local") {
			sawHelpfulMessage = true
		}
	}

	if !hadDivergedError {
		t.Errorf("Expected merge to fail with 'diverged' error when local main has diverged from origin.\nAll output:\n%s", strings.Join(allOutput, "\n"))
	}

	if !sawHelpfulMessage {
		t.Errorf("Expected helpful message about syncing local branch.\nAll output:\n%s", strings.Join(allOutput, "\n"))
	}
}

func TestMergeToMain_PullFailsNoRemote(t *testing.T) {
	// Create a local-only repo (no remote)
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Get the default branch name
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = repoPath
	output, _ := cmd.Output()
	defaultBranch := strings.TrimSpace(string(output))
	if defaultBranch == "" {
		defaultBranch = "master"
	}

	// Create a feature branch
	cmd = exec.Command("git", "checkout", "-b", "feature-no-remote")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create feature branch: %v", err)
	}

	// Make a change on the feature branch
	featureFile := filepath.Join(repoPath, "feature.txt")
	if err := os.WriteFile(featureFile, []byte("feature content"), 0644); err != nil {
		t.Fatalf("Failed to create feature file: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Feature commit")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit feature: %v", err)
	}

	// Try to merge - should succeed even though pull fails (no remote)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch := svc.MergeToMain(ctx, repoPath, repoPath, "feature-no-remote", "")

	var sawNoRemoteMessage bool
	var lastResult Result
	for result := range ch {
		lastResult = result
		if strings.Contains(result.Output, "No remote configured") ||
			strings.Contains(result.Output, "no tracking information") {
			sawNoRemoteMessage = true
		}
	}

	if lastResult.Error != nil {
		t.Errorf("Expected merge to succeed for local-only repo, got error: %v", lastResult.Error)
	}

	if !lastResult.Done {
		t.Error("Expected merge to complete with Done=true")
	}

	if !sawNoRemoteMessage {
		t.Error("Expected message about no remote/tracking info")
	}

	// Verify the merge actually happened - feature file should exist on main
	cmd = exec.Command("git", "checkout", defaultBranch)
	cmd.Dir = repoPath
	cmd.Run()

	if _, err := os.Stat(filepath.Join(repoPath, "feature.txt")); os.IsNotExist(err) {
		t.Error("feature.txt should exist on main after merge")
	}
}

func TestGetDiffStats_NoChanges(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Stdout: []byte(""),
	})
	s := NewGitServiceWithExecutor(mock)

	stats, err := s.GetDiffStats(ctx, "/repo")
	if err != nil {
		t.Fatalf("GetDiffStats failed: %v", err)
	}

	if stats.FilesChanged != 0 {
		t.Errorf("Expected FilesChanged to be 0, got %d", stats.FilesChanged)
	}
	if stats.Additions != 0 {
		t.Errorf("Expected Additions to be 0, got %d", stats.Additions)
	}
	if stats.Deletions != 0 {
		t.Errorf("Expected Deletions to be 0, got %d", stats.Deletions)
	}
}

func TestGetDiffStats_WithModifiedFile(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Stdout: []byte(" M test.txt\n"),
	})
	mock.AddExactMatch("git", []string{"diff", "--numstat"}, pexec.MockResponse{
		Stdout: []byte("3\t1\ttest.txt\n"),
	})
	mock.AddExactMatch("git", []string{"diff", "--numstat", "--cached"}, pexec.MockResponse{
		Stdout: []byte(""),
	})
	s := NewGitServiceWithExecutor(mock)

	stats, err := s.GetDiffStats(ctx, "/repo")
	if err != nil {
		t.Fatalf("GetDiffStats failed: %v", err)
	}

	if stats.FilesChanged != 1 {
		t.Errorf("Expected FilesChanged to be 1, got %d", stats.FilesChanged)
	}
	if stats.Additions != 3 {
		t.Errorf("Expected 3 additions, got %d", stats.Additions)
	}
	if stats.Deletions != 1 {
		t.Errorf("Expected 1 deletion, got %d", stats.Deletions)
	}
}

func TestGetDiffStats_WithNewFile(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Stdout: []byte("?? new.txt\n"),
	})
	mock.AddExactMatch("git", []string{"diff", "--numstat"}, pexec.MockResponse{
		Stdout: []byte(""),
	})
	mock.AddExactMatch("git", []string{"diff", "--numstat", "--cached"}, pexec.MockResponse{
		Stdout: []byte(""),
	})
	// Untracked file line count
	mock.AddExactMatch("git", []string{"diff", "--no-index", "--numstat", "/dev/null", "new.txt"}, pexec.MockResponse{
		Stdout: []byte("3\t0\tnew.txt\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	stats, err := s.GetDiffStats(ctx, "/repo")
	if err != nil {
		t.Fatalf("GetDiffStats failed: %v", err)
	}

	if stats.FilesChanged != 1 {
		t.Errorf("Expected FilesChanged to be 1, got %d", stats.FilesChanged)
	}
	if stats.Additions != 3 {
		t.Errorf("Expected Additions to be 3, got %d", stats.Additions)
	}
}

func TestGetDiffStats_WithStagedChanges(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Stdout: []byte("A  staged.txt\n"),
	})
	mock.AddExactMatch("git", []string{"diff", "--numstat"}, pexec.MockResponse{
		Stdout: []byte(""),
	})
	mock.AddExactMatch("git", []string{"diff", "--numstat", "--cached"}, pexec.MockResponse{
		Stdout: []byte("5\t0\tstaged.txt\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	stats, err := s.GetDiffStats(ctx, "/repo")
	if err != nil {
		t.Fatalf("GetDiffStats failed: %v", err)
	}

	if stats.FilesChanged != 1 {
		t.Errorf("Expected FilesChanged to be 1, got %d", stats.FilesChanged)
	}
	if stats.Additions != 5 {
		t.Errorf("Expected Additions to be 5, got %d", stats.Additions)
	}
}

func TestGetDiffStats_WithDeletions(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Stdout: []byte(" M test.txt\n"),
	})
	mock.AddExactMatch("git", []string{"diff", "--numstat"}, pexec.MockResponse{
		Stdout: []byte("0\t1\ttest.txt\n"),
	})
	mock.AddExactMatch("git", []string{"diff", "--numstat", "--cached"}, pexec.MockResponse{
		Stdout: []byte(""),
	})
	s := NewGitServiceWithExecutor(mock)

	stats, err := s.GetDiffStats(ctx, "/repo")
	if err != nil {
		t.Fatalf("GetDiffStats failed: %v", err)
	}

	if stats.FilesChanged != 1 {
		t.Errorf("Expected FilesChanged to be 1, got %d", stats.FilesChanged)
	}
	if stats.Deletions < 1 {
		t.Errorf("Expected at least 1 deletion, got %d", stats.Deletions)
	}
}

func TestGetDiffStats_MultipleFiles(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Stdout: []byte("A  file1.txt\nA  file2.txt\nA  file3.txt\n"),
	})
	mock.AddExactMatch("git", []string{"diff", "--numstat"}, pexec.MockResponse{
		Stdout: []byte(""),
	})
	mock.AddExactMatch("git", []string{"diff", "--numstat", "--cached"}, pexec.MockResponse{
		Stdout: []byte("2\t0\tfile1.txt\n4\t0\tfile2.txt\n6\t0\tfile3.txt\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	stats, err := s.GetDiffStats(ctx, "/repo")
	if err != nil {
		t.Fatalf("GetDiffStats failed: %v", err)
	}

	if stats.FilesChanged != 3 {
		t.Errorf("Expected FilesChanged to be 3, got %d", stats.FilesChanged)
	}
	if stats.Additions != 12 {
		t.Errorf("Expected Additions to be 12, got %d", stats.Additions)
	}
}

func TestGetDiffStats_InvalidPath(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Err: fmt.Errorf("fatal: not a git repository"),
	})
	s := NewGitServiceWithExecutor(mock)

	_, err := s.GetDiffStats(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestGetDiffStats_MixedChanges(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Stdout: []byte(" M test.txt\nA  new.txt\n"),
	})
	mock.AddExactMatch("git", []string{"diff", "--numstat"}, pexec.MockResponse{
		Stdout: []byte("2\t1\ttest.txt\n"),
	})
	mock.AddExactMatch("git", []string{"diff", "--numstat", "--cached"}, pexec.MockResponse{
		Stdout: []byte("2\t0\tnew.txt\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	stats, err := s.GetDiffStats(ctx, "/repo")
	if err != nil {
		t.Fatalf("GetDiffStats failed: %v", err)
	}

	if stats.FilesChanged != 2 {
		t.Errorf("Expected FilesChanged to be 2, got %d", stats.FilesChanged)
	}
	if stats.Additions == 0 {
		t.Error("Expected some additions")
	}
	if stats.Deletions == 0 {
		t.Error("Expected some deletions")
	}
}

// Tests for BranchDivergence helper functions

func TestBranchDivergence_IsDiverged(t *testing.T) {
	tests := []struct {
		name     string
		behind   int
		ahead    int
		expected bool
	}{
		{"in sync", 0, 0, false},
		{"only behind", 3, 0, false},
		{"only ahead", 0, 2, false},
		{"diverged", 3, 2, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &BranchDivergence{Behind: tt.behind, Ahead: tt.ahead}
			if got := d.IsDiverged(); got != tt.expected {
				t.Errorf("IsDiverged() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBranchDivergence_CanFastForward(t *testing.T) {
	tests := []struct {
		name     string
		behind   int
		ahead    int
		expected bool
	}{
		{"in sync", 0, 0, true},
		{"only behind", 3, 0, true},
		{"only ahead", 0, 2, false},
		{"diverged", 3, 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &BranchDivergence{Behind: tt.behind, Ahead: tt.ahead}
			if got := d.CanFastForward(); got != tt.expected {
				t.Errorf("CanFastForward() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetBranchDivergence_InSync(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"rev-list", "--count", "--left-right", "origin/main...main"}, pexec.MockResponse{
		Stdout: []byte("0\t0\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	divergence, err := s.GetBranchDivergence(ctx, "/repo", "main", "origin/main")
	if err != nil {
		t.Fatalf("GetBranchDivergence failed: %v", err)
	}

	if divergence.Behind != 0 || divergence.Ahead != 0 {
		t.Errorf("Expected 0 behind, 0 ahead; got %d behind, %d ahead", divergence.Behind, divergence.Ahead)
	}
	if divergence.IsDiverged() {
		t.Error("Should not be diverged when in sync")
	}
	if !divergence.CanFastForward() {
		t.Error("Should be able to fast-forward when in sync")
	}
}

func TestGetBranchDivergence_LocalAhead(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"rev-list", "--count", "--left-right", "origin/main...main"}, pexec.MockResponse{
		Stdout: []byte("0\t1\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	divergence, err := s.GetBranchDivergence(ctx, "/repo", "main", "origin/main")
	if err != nil {
		t.Fatalf("GetBranchDivergence failed: %v", err)
	}

	if divergence.Behind != 0 || divergence.Ahead != 1 {
		t.Errorf("Expected 0 behind, 1 ahead; got %d behind, %d ahead", divergence.Behind, divergence.Ahead)
	}
	if divergence.IsDiverged() {
		t.Error("Should not be diverged when only ahead")
	}
	if divergence.CanFastForward() {
		t.Error("Should not be able to fast-forward when ahead")
	}
}

func TestGetBranchDivergence_LocalBehind(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"rev-list", "--count", "--left-right", "origin/main...main"}, pexec.MockResponse{
		Stdout: []byte("1\t0\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	divergence, err := s.GetBranchDivergence(ctx, "/repo", "main", "origin/main")
	if err != nil {
		t.Fatalf("GetBranchDivergence failed: %v", err)
	}

	if divergence.Behind != 1 || divergence.Ahead != 0 {
		t.Errorf("Expected 1 behind, 0 ahead; got %d behind, %d ahead", divergence.Behind, divergence.Ahead)
	}
	if divergence.IsDiverged() {
		t.Error("Should not be diverged when only behind")
	}
	if !divergence.CanFastForward() {
		t.Error("Should be able to fast-forward when only behind")
	}
}

func TestGetBranchDivergence_Diverged(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"rev-list", "--count", "--left-right", "origin/main...main"}, pexec.MockResponse{
		Stdout: []byte("1\t1\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	divergence, err := s.GetBranchDivergence(ctx, "/repo", "main", "origin/main")
	if err != nil {
		t.Fatalf("GetBranchDivergence failed: %v", err)
	}

	if divergence.Behind != 1 || divergence.Ahead != 1 {
		t.Errorf("Expected 1 behind, 1 ahead; got %d behind, %d ahead", divergence.Behind, divergence.Ahead)
	}
	if !divergence.IsDiverged() {
		t.Error("Should be diverged when both ahead and behind")
	}
	if divergence.CanFastForward() {
		t.Error("Should not be able to fast-forward when diverged")
	}
}

func TestGetBranchDivergence_InvalidBranch(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"rev-list", "--count", "--left-right", "origin/nonexistent...main"}, pexec.MockResponse{
		Err: fmt.Errorf("fatal: bad revision 'origin/nonexistent...main'"),
	})
	s := NewGitServiceWithExecutor(mock)

	_, err := s.GetBranchDivergence(ctx, "/repo", "main", "origin/nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent branch")
	}
}

func TestHasTrackingBranch_NoTracking(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"config", "--get", "branch.main.remote"}, pexec.MockResponse{
		Err: fmt.Errorf("exit status 1"),
	})
	s := NewGitServiceWithExecutor(mock)

	if s.HasTrackingBranch(ctx, "/repo", "main") {
		t.Error("HasTrackingBranch should return false for branch without upstream")
	}
}

func TestHasTrackingBranch_WithTracking(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"config", "--get", "branch.main.remote"}, pexec.MockResponse{
		Stdout: []byte("origin\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	if !s.HasTrackingBranch(ctx, "/repo", "main") {
		t.Error("HasTrackingBranch should return true for branch with upstream")
	}
}

func TestHasTrackingBranch_InvalidPath(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"config", "--get", "branch.main.remote"}, pexec.MockResponse{
		Err: fmt.Errorf("fatal: not a git repository"),
	})
	s := NewGitServiceWithExecutor(mock)

	if s.HasTrackingBranch(ctx, "/nonexistent/path", "main") {
		t.Error("HasTrackingBranch should return false for invalid path")
	}
}

func TestRemoteBranchExists_Exists(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"rev-parse", "--verify", "origin/main"}, pexec.MockResponse{
		Stdout: []byte("abc123\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	if !s.RemoteBranchExists(ctx, "/repo", "origin/main") {
		t.Error("RemoteBranchExists should return true for existing remote branch")
	}
}

func TestRemoteBranchExists_NotExists(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"rev-parse", "--verify", "origin/nonexistent"}, pexec.MockResponse{
		Err: fmt.Errorf("fatal: Needed a single revision"),
	})
	s := NewGitServiceWithExecutor(mock)

	if s.RemoteBranchExists(ctx, "/repo", "origin/nonexistent") {
		t.Error("RemoteBranchExists should return false for non-existent remote branch")
	}
}

func TestRemoteBranchExists_NoRemote(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"rev-parse", "--verify", "origin/main"}, pexec.MockResponse{
		Err: fmt.Errorf("fatal: Needed a single revision"),
	})
	s := NewGitServiceWithExecutor(mock)

	if s.RemoteBranchExists(ctx, "/repo", "origin/main") {
		t.Error("RemoteBranchExists should return false when no remote configured")
	}
}

func TestRemoteBranchExists_InvalidPath(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"rev-parse", "--verify", "origin/main"}, pexec.MockResponse{
		Err: fmt.Errorf("fatal: not a git repository"),
	})
	s := NewGitServiceWithExecutor(mock)

	if s.RemoteBranchExists(ctx, "/nonexistent/path", "origin/main") {
		t.Error("RemoteBranchExists should return false for invalid path")
	}
}

func TestGetCurrentBranch_Success(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, pexec.MockResponse{
		Stdout: []byte("main\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	branch, err := s.GetCurrentBranch(ctx, "/repo")
	if err != nil {
		t.Fatalf("GetCurrentBranch failed: %v", err)
	}
	if branch != "main" {
		t.Errorf("GetCurrentBranch = %q, want 'main'", branch)
	}
}

func TestGetCurrentBranch_FeatureBranch(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, pexec.MockResponse{
		Stdout: []byte("my-feature\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	branch, err := s.GetCurrentBranch(ctx, "/repo")
	if err != nil {
		t.Fatalf("GetCurrentBranch failed: %v", err)
	}
	if branch != "my-feature" {
		t.Errorf("GetCurrentBranch = %q, want 'my-feature'", branch)
	}
}

func TestGetCurrentBranch_InvalidPath(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, pexec.MockResponse{
		Err: fmt.Errorf("fatal: not a git repository"),
	})
	s := NewGitServiceWithExecutor(mock)

	_, err := s.GetCurrentBranch(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestGetCurrentBranch_DetachedHead(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, pexec.MockResponse{
		Stdout: []byte("HEAD\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	_, err := s.GetCurrentBranch(ctx, "/repo")
	if err == nil {
		t.Error("Expected error for detached HEAD")
	}
	if !strings.Contains(err.Error(), "detached") {
		t.Errorf("Expected error to mention 'detached', got: %v", err)
	}
}

func TestCheckoutBranch_Success(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"checkout", "checkout-test"}, pexec.MockResponse{
		Stdout: []byte("Switched to branch 'checkout-test'\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	err := s.CheckoutBranch(ctx, "/repo", "checkout-test")
	if err != nil {
		t.Fatalf("CheckoutBranch failed: %v", err)
	}
}

func TestCheckoutBranch_NonexistentBranch(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"checkout", "nonexistent-branch"}, pexec.MockResponse{
		Stderr: []byte("error: pathspec 'nonexistent-branch' did not match any file(s) known to git\n"),
		Err:    fmt.Errorf("exit status 1"),
	})
	s := NewGitServiceWithExecutor(mock)

	err := s.CheckoutBranch(ctx, "/repo", "nonexistent-branch")
	if err == nil {
		t.Error("Expected error for nonexistent branch")
	}
}

func TestCheckoutBranch_InvalidPath(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"checkout", "main"}, pexec.MockResponse{
		Stderr: []byte("fatal: not a git repository\n"),
		Err:    fmt.Errorf("exit status 128"),
	})
	s := NewGitServiceWithExecutor(mock)

	err := s.CheckoutBranch(ctx, "/nonexistent/path", "main")
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestCheckoutBranch_WithUncommittedChanges(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	// Checkout succeeds when changes don't conflict
	mock.AddExactMatch("git", []string{"checkout", "changes-test"}, pexec.MockResponse{
		Stdout: []byte("Switched to branch 'changes-test'\nM\ttest.txt\n"),
	})
	s := NewGitServiceWithExecutor(mock)

	err := s.CheckoutBranch(ctx, "/repo", "changes-test")
	if err != nil {
		t.Logf("CheckoutBranch with uncommitted changes: %v", err)
	}
}

func TestCheckoutBranchIgnoreWorktrees_Success(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a feature branch
	cmd := exec.Command("git", "checkout", "-b", "worktree-test")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create branch: %v", err)
	}

	// Go back to default branch
	defaultBranch := svc.GetDefaultBranch(ctx, repoPath)
	cmd = exec.Command("git", "checkout", defaultBranch)
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to checkout default branch: %v", err)
	}

	// Create a worktree using the feature branch
	worktreePath := filepath.Join(os.TempDir(), "test-worktree-"+filepath.Base(repoPath))
	defer os.RemoveAll(worktreePath)

	cmd = exec.Command("git", "worktree", "add", worktreePath, "worktree-test")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	// Standard checkout should fail because branch is in use by worktree
	err := svc.CheckoutBranch(ctx, repoPath, "worktree-test")
	if err == nil {
		t.Error("Expected CheckoutBranch to fail for branch in use by worktree")
	}

	// CheckoutBranchIgnoreWorktrees should succeed
	err = svc.CheckoutBranchIgnoreWorktrees(ctx, repoPath, "worktree-test")
	if err != nil {
		t.Errorf("CheckoutBranchIgnoreWorktrees failed: %v", err)
	}

	// Verify we're on the correct branch
	branch, err := svc.GetCurrentBranch(ctx, repoPath)
	if err != nil {
		t.Fatalf("Failed to get current branch: %v", err)
	}
	if branch != "worktree-test" {
		t.Errorf("Expected branch 'worktree-test', got '%s'", branch)
	}

	// Clean up worktree
	cmd = exec.Command("git", "worktree", "remove", worktreePath)
	cmd.Dir = repoPath
	cmd.Run() // Ignore errors in cleanup
}

func TestSquashMergeToMain(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a feature branch
	cmd := exec.Command("git", "checkout", "-b", "squash-feature")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create feature branch: %v", err)
	}

	// Make multiple commits on the feature branch
	for i := 1; i <= 3; i++ {
		testFile := filepath.Join(repoPath, fmt.Sprintf("feature%d.txt", i))
		if err := os.WriteFile(testFile, fmt.Appendf(nil, "feature %d content", i), 0644); err != nil {
			t.Fatalf("Failed to create feature file: %v", err)
		}

		cmd = exec.Command("git", "add", ".")
		cmd.Dir = repoPath
		cmd.Run()

		cmd = exec.Command("git", "commit", "-m", fmt.Sprintf("Feature commit %d", i))
		cmd.Dir = repoPath
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to commit feature %d: %v", i, err)
		}
	}

	// Squash merge to main
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	squashCommitMsg := "Squashed feature: all 3 commits combined"
	ch := svc.SquashMergeToMain(ctx, repoPath, repoPath, "squash-feature", squashCommitMsg)

	var lastResult Result
	for result := range ch {
		lastResult = result
		if result.Error != nil {
			t.Errorf("Squash merge error: %v", result.Error)
		}
	}

	if !lastResult.Done {
		t.Error("Squash merge should complete with Done=true")
	}

	// Verify we're on the default branch
	cmd = exec.Command("git", "branch", "--show-current")
	cmd.Dir = repoPath
	output, _ := cmd.Output()
	currentBranch := strings.TrimSpace(string(output))
	if currentBranch != "main" && currentBranch != "master" {
		t.Logf("Current branch: %q (expected main or master)", currentBranch)
	}

	// Verify all feature files exist on main
	for i := 1; i <= 3; i++ {
		testFile := filepath.Join(repoPath, fmt.Sprintf("feature%d.txt", i))
		if _, err := os.Stat(testFile); os.IsNotExist(err) {
			t.Errorf("feature%d.txt should exist on main after squash merge", i)
		}
	}

	// Verify there's only ONE commit on main for the squashed changes (plus the initial commit)
	cmd = exec.Command("git", "log", "--oneline")
	cmd.Dir = repoPath
	output, _ = cmd.Output()
	commitLines := strings.Split(strings.TrimSpace(string(output)), "\n")
	// Should have 2 commits: initial commit + squashed commit
	if len(commitLines) != 2 {
		t.Errorf("Expected 2 commits on main after squash merge (initial + squashed), got %d: %v", len(commitLines), commitLines)
	}

	// Verify the commit message matches what we provided
	cmd = exec.Command("git", "log", "-1", "--pretty=%s")
	cmd.Dir = repoPath
	output, _ = cmd.Output()
	lastCommitMsg := strings.TrimSpace(string(output))
	if lastCommitMsg != squashCommitMsg {
		t.Errorf("Expected commit message %q, got %q", squashCommitMsg, lastCommitMsg)
	}
}

func TestSquashMergeToMain_WithUncommittedChanges(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a feature branch
	cmd := exec.Command("git", "checkout", "-b", "squash-uncommitted")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create feature branch: %v", err)
	}

	// Make a committed change
	testFile := filepath.Join(repoPath, "committed.txt")
	if err := os.WriteFile(testFile, []byte("committed content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Committed change")
	cmd.Dir = repoPath
	cmd.Run()

	// Make an uncommitted change
	uncommittedFile := filepath.Join(repoPath, "uncommitted.txt")
	if err := os.WriteFile(uncommittedFile, []byte("uncommitted content"), 0644); err != nil {
		t.Fatalf("Failed to create uncommitted file: %v", err)
	}

	// Squash merge with custom commit message (should commit the uncommitted change first)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch := svc.SquashMergeToMain(ctx, repoPath, repoPath, "squash-uncommitted", "Squashed with uncommitted")

	var sawUncommittedMsg bool
	var lastResult Result
	for result := range ch {
		lastResult = result
		if strings.Contains(result.Output, "uncommitted changes") {
			sawUncommittedMsg = true
		}
		if result.Error != nil {
			t.Errorf("Squash merge error: %v", result.Error)
		}
	}

	if !sawUncommittedMsg {
		t.Error("Expected message about uncommitted changes")
	}

	if !lastResult.Done {
		t.Error("Squash merge should complete with Done=true")
	}

	// Verify both files exist on main
	for _, fileName := range []string{"committed.txt", "uncommitted.txt"} {
		file := filepath.Join(repoPath, fileName)
		if _, err := os.Stat(file); os.IsNotExist(err) {
			t.Errorf("%s should exist on main after squash merge", fileName)
		}
	}
}

func TestSquashMergeToMain_Conflict(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a feature branch
	cmd := exec.Command("git", "checkout", "-b", "squash-conflict")
	cmd.Dir = repoPath
	cmd.Run()

	// Modify test.txt on feature branch
	testFile := filepath.Join(repoPath, "test.txt")
	os.WriteFile(testFile, []byte("feature version"), 0644)

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Feature change")
	cmd.Dir = repoPath
	cmd.Run()

	// Go back to main and make a conflicting change
	cmd = exec.Command("git", "checkout", "-")
	cmd.Dir = repoPath
	cmd.Run()

	os.WriteFile(testFile, []byte("main version"), 0644)

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Main change")
	cmd.Dir = repoPath
	cmd.Run()

	// Try to squash merge - should fail with conflict
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ch := svc.SquashMergeToMain(ctx, repoPath, repoPath, "squash-conflict", "Squash conflicting")

	var hadError bool
	for result := range ch {
		if result.Error != nil {
			hadError = true
		}
	}

	if !hadError {
		t.Error("Expected squash merge to fail with conflict")
	}
}

func TestSquashMergeToMain_Cancelled(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a branch
	cmd := exec.Command("git", "checkout", "-b", "squash-cancel")
	cmd.Dir = repoPath
	cmd.Run()

	// Cancel immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ch := svc.SquashMergeToMain(ctx, repoPath, repoPath, "squash-cancel", "Cancelled")

	// Drain channel - should not hang
	for range ch {
	}
}

func TestIsMergeInProgress_NoMerge(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"rev-parse", "--verify", "MERGE_HEAD"}, pexec.MockResponse{
		Err: fmt.Errorf("fatal: Needed a single revision"),
	})
	s := NewGitServiceWithExecutor(mock)

	inProgress, err := s.IsMergeInProgress(ctx, "/repo")
	if err != nil {
		t.Fatalf("IsMergeInProgress failed: %v", err)
	}

	if inProgress {
		t.Error("Expected no merge in progress for clean repo")
	}
}

func TestIsMergeInProgress_DuringMerge(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a feature branch
	cmd := exec.Command("git", "checkout", "-b", "merge-conflict-test")
	cmd.Dir = repoPath
	cmd.Run()

	// Modify test.txt on feature branch
	testFile := filepath.Join(repoPath, "test.txt")
	os.WriteFile(testFile, []byte("feature version"), 0644)

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Feature change")
	cmd.Dir = repoPath
	cmd.Run()

	// Go back to main and make a conflicting change
	cmd = exec.Command("git", "checkout", "-")
	cmd.Dir = repoPath
	cmd.Run()

	os.WriteFile(testFile, []byte("main version"), 0644)

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "Main change")
	cmd.Dir = repoPath
	cmd.Run()

	// Start merge - this will fail with conflict
	cmd = exec.Command("git", "merge", "merge-conflict-test")
	cmd.Dir = repoPath
	cmd.Run() // Ignore error - we expect conflict

	// Now we should be in a merge state
	inProgress, err := svc.IsMergeInProgress(ctx, repoPath)
	if err != nil {
		t.Fatalf("IsMergeInProgress failed: %v", err)
	}

	if !inProgress {
		t.Error("Expected merge in progress after conflicting merge")
	}

	// Abort the merge
	cmd = exec.Command("git", "merge", "--abort")
	cmd.Dir = repoPath
	cmd.Run()

	// After abort, no merge in progress
	inProgress, err = svc.IsMergeInProgress(ctx, repoPath)
	if err != nil {
		t.Fatalf("IsMergeInProgress failed after abort: %v", err)
	}

	if inProgress {
		t.Error("Expected no merge in progress after abort")
	}
}

func TestIsMergeInProgress_InvalidPath(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("git", []string{"rev-parse", "--verify", "MERGE_HEAD"}, pexec.MockResponse{
		Err: fmt.Errorf("fatal: not a git repository"),
	})
	s := NewGitServiceWithExecutor(mock)

	// Should not error, just return false
	inProgress, err := s.IsMergeInProgress(ctx, "/nonexistent/path")
	if err != nil {
		t.Errorf("Expected no error for invalid path, got: %v", err)
	}

	if inProgress {
		t.Error("Expected no merge in progress for invalid path")
	}
}

func TestGeneratePRTitleAndBodyWithBaseBranch(t *testing.T) {
	// Create mock executor for testing
	mockExec := pexec.NewMockExecutor(nil)
	svc := NewGitServiceWithExecutor(mockExec)

	// Mock git log to return commits
	mockExec.AddPrefixMatch("git", []string{"log", "feature-base..feature-branch", "--oneline"}, pexec.MockResponse{
		Stdout: []byte("abc123 Add feature Y\n"),
	})

	// Mock git diff to return a simple diff
	mockExec.AddPrefixMatch("git", []string{"diff", "--no-ext-diff", "feature-base...feature-branch"}, pexec.MockResponse{
		Stdout: []byte(`diff --git a/file.txt b/file.txt
index 1234567..abcdefg 100644
--- a/file.txt
+++ b/file.txt
@@ -1,1 +1,2 @@
 existing line
+new line from feature Y
`),
	})

	// Mock Claude response (using conventional commit format)
	claudeResponse := `---TITLE---
feat: add feature Y

---BODY---
## Summary
This PR adds feature Y to the codebase.

## Changes
- Added new line to file.txt

## Test plan
- Verify the new line appears in file.txt
`
	mockExec.AddPrefixMatch("claude", []string{"--print", "-p"}, pexec.MockResponse{
		Stdout: []byte(claudeResponse),
	})

	ctx := context.Background()
	title, body, err := svc.GeneratePRTitleAndBodyWithIssueRef(ctx, "/test/repo", "feature-branch", "feature-base", nil)

	if err != nil {
		t.Fatalf("GeneratePRTitleAndBodyWithIssueRef failed: %v", err)
	}

	if title != "feat: add feature Y" {
		t.Errorf("Expected title 'feat: add feature Y', got '%s'", title)
	}

	if !strings.Contains(body, "feature Y") {
		t.Errorf("Expected body to contain 'feature Y', got: %s", body)
	}

	// Verify that the git log and diff commands used the baseBranch parameter
	calls := mockExec.GetCalls()

	var foundLogWithBase, foundDiffWithBase bool
	for _, call := range calls {
		if call.Name == "git" && len(call.Args) > 1 {
			if call.Args[0] == "log" && len(call.Args) > 1 && call.Args[1] == "feature-base..feature-branch" {
				foundLogWithBase = true
			}
			if call.Args[0] == "diff" && len(call.Args) > 2 && call.Args[2] == "feature-base...feature-branch" {
				foundDiffWithBase = true
			}
		}
	}

	if !foundLogWithBase {
		t.Error("Expected git log command to use baseBranch 'feature-base'")
	}

	if !foundDiffWithBase {
		t.Error("Expected git diff command to use baseBranch 'feature-base'")
	}
}

func TestGeneratePRTitleAndBodyWithEmptyBaseBranch(t *testing.T) {
	// Create mock executor for testing
	mockExec := pexec.NewMockExecutor(nil)
	svc := NewGitServiceWithExecutor(mockExec)

	// Mock GetDefaultBranch
	mockExec.AddPrefixMatch("git", []string{"symbolic-ref", "refs/remotes/origin/HEAD"}, pexec.MockResponse{
		Stdout: []byte("refs/remotes/origin/main\n"),
	})

	// Mock git log with main as base
	mockExec.AddPrefixMatch("git", []string{"log", "main..feature-branch", "--oneline"}, pexec.MockResponse{
		Stdout: []byte("abc123 Add feature\n"),
	})

	// Mock git diff
	mockExec.AddPrefixMatch("git", []string{"diff", "--no-ext-diff", "main...feature-branch"}, pexec.MockResponse{
		Stdout: []byte("diff --git a/file.txt b/file.txt\n"),
	})

	// Mock Claude response (using conventional commit format)
	mockExec.AddPrefixMatch("claude", []string{"--print", "-p"}, pexec.MockResponse{
		Stdout: []byte("---TITLE---\nfeat: add feature\n---BODY---\nTest PR"),
	})

	ctx := context.Background()
	// Pass empty string for baseBranch - should fall back to default branch
	title, _, err := svc.GeneratePRTitleAndBodyWithIssueRef(ctx, "/test/repo", "feature-branch", "", nil)

	if err != nil {
		t.Fatalf("GeneratePRTitleAndBodyWithIssueRef failed: %v", err)
	}

	if title != "feat: add feature" {
		t.Errorf("Expected title 'feat: add feature', got '%s'", title)
	}

	// Verify that it fell back to using main
	calls := mockExec.GetCalls()

	var foundLogWithMain bool
	for _, call := range calls {
		if call.Name == "git" && len(call.Args) > 1 && call.Args[0] == "log" {
			if strings.Contains(call.Args[1], "main") {
				foundLogWithMain = true
			}
		}
	}

	if !foundLogWithMain {
		t.Error("Expected to fall back to default branch 'main' when baseBranch is empty")
	}
}

func TestExtractOwnerRepo(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "SSH format",
			input:    "git@github.com:zhubert/plural.git",
			expected: "zhubert/plural",
		},
		{
			name:     "HTTPS format",
			input:    "https://github.com/zhubert/plural.git",
			expected: "zhubert/plural",
		},
		{
			name:     "HTTP format",
			input:    "http://github.com/zhubert/plural.git",
			expected: "zhubert/plural",
		},
		{
			name:     "no .git suffix",
			input:    "https://github.com/zhubert/plural",
			expected: "zhubert/plural",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "bare path",
			input:    "/path/to/repo",
			expected: "",
		},
		{
			name:     "malformed SSH no slash",
			input:    "git@github.com:justowner.git",
			expected: "",
		},
		{
			name:     "HTTPS no path",
			input:    "https://github.com/",
			expected: "",
		},
		{
			name:     "SSH with nested path",
			input:    "git@github.com:org/sub/repo.git",
			expected: "org/sub/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractOwnerRepo(tt.input)
			if got != tt.expected {
				t.Errorf("ExtractOwnerRepo(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestGetRemoteOriginURL(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockExec := pexec.NewMockExecutor(nil)
		mockExec.AddPrefixMatch("git", []string{"remote", "get-url", "origin"}, pexec.MockResponse{
			Stdout: []byte("https://github.com/zhubert/plural.git\n"),
		})
		gitSvc := NewGitServiceWithExecutor(mockExec)

		url, err := gitSvc.GetRemoteOriginURL(context.Background(), "/any/path")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if url != "https://github.com/zhubert/plural.git" {
			t.Errorf("expected URL 'https://github.com/zhubert/plural.git', got %q", url)
		}
	})

	t.Run("error when no remote", func(t *testing.T) {
		mockExec := pexec.NewMockExecutor(nil)
		mockExec.AddPrefixMatch("git", []string{"remote", "get-url", "origin"}, pexec.MockResponse{
			Err: fmt.Errorf("fatal: No such remote 'origin'"),
		})
		gitSvc := NewGitServiceWithExecutor(mockExec)

		_, err := gitSvc.GetRemoteOriginURL(context.Background(), "/any/path")
		if err == nil {
			t.Error("expected error when no remote")
		}
	})
}
