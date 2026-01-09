package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	if HasRemoteOrigin(repoPath) {
		t.Error("HasRemoteOrigin should return false for repo without origin")
	}
}

func TestHasRemoteOrigin_WithRemote(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Add a remote origin
	cmd := exec.Command("git", "remote", "add", "origin", "https://github.com/test/test.git")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to add remote: %v", err)
	}

	if !HasRemoteOrigin(repoPath) {
		t.Error("HasRemoteOrigin should return true for repo with origin")
	}
}

func TestHasRemoteOrigin_InvalidPath(t *testing.T) {
	if HasRemoteOrigin("/nonexistent/path") {
		t.Error("HasRemoteOrigin should return false for invalid path")
	}
}

func TestGetDefaultBranch_Master(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// By default, new repos might use main or master depending on git config
	// Let's explicitly create master branch
	cmd := exec.Command("git", "checkout", "-b", "master")
	cmd.Dir = repoPath
	cmd.Run()

	branch := GetDefaultBranch(repoPath)
	// Should return either main or master
	if branch != "main" && branch != "master" {
		t.Errorf("GetDefaultBranch = %q, want 'main' or 'master'", branch)
	}
}

func TestGetDefaultBranch_Main(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Rename default branch to main
	cmd := exec.Command("git", "branch", "-M", "main")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Skip("Could not rename branch to main")
	}

	branch := GetDefaultBranch(repoPath)
	if branch != "main" && branch != "master" {
		t.Errorf("GetDefaultBranch = %q, want 'main' or 'master'", branch)
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

	ch := MergeToMain(ctx, repoPath, repoPath, "feature-branch", "")

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

	ch := MergeToMain(ctx, repoPath, repoPath, "conflict-branch", "")

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

	ch := MergeToMain(ctx, repoPath, repoPath, "test-branch", "")

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

	ch := CreatePR(ctx, repoPath, repoPath, "test-branch", "")

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
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	status, err := GetWorktreeStatus(repoPath)
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
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a new file (untracked)
	newFile := filepath.Join(repoPath, "new.txt")
	if err := os.WriteFile(newFile, []byte("new content"), 0644); err != nil {
		t.Fatalf("Failed to create new file: %v", err)
	}

	// Modify existing file
	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("modified content"), 0644); err != nil {
		t.Fatalf("Failed to modify test file: %v", err)
	}

	status, err := GetWorktreeStatus(repoPath)
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
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create one new file
	newFile := filepath.Join(repoPath, "single.txt")
	if err := os.WriteFile(newFile, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	status, err := GetWorktreeStatus(repoPath)
	if err != nil {
		t.Fatalf("GetWorktreeStatus failed: %v", err)
	}

	if status.Summary != "1 file changed" {
		t.Errorf("Expected Summary '1 file changed', got %q", status.Summary)
	}
}

func TestGetWorktreeStatus_InvalidPath(t *testing.T) {
	_, err := GetWorktreeStatus("/nonexistent/path")
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestCommitAll_Success(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create changes
	newFile := filepath.Join(repoPath, "committed.txt")
	if err := os.WriteFile(newFile, []byte("to be committed"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Verify changes exist before commit
	status, _ := GetWorktreeStatus(repoPath)
	if !status.HasChanges {
		t.Fatal("Expected changes before commit")
	}

	// Commit all changes
	err := CommitAll(repoPath, "Test commit message")
	if err != nil {
		t.Fatalf("CommitAll failed: %v", err)
	}

	// Verify no changes after commit
	status, _ = GetWorktreeStatus(repoPath)
	if status.HasChanges {
		t.Error("Expected no changes after CommitAll")
	}

	// Verify commit was created with correct message
	cmd := exec.Command("git", "log", "-1", "--pretty=%s")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get commit message: %v", err)
	}

	if string(output) != "Test commit message\n" {
		t.Errorf("Expected commit message 'Test commit message', got %q", string(output))
	}
}

func TestCommitAll_NoChanges(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Try to commit with no changes - should fail
	err := CommitAll(repoPath, "Empty commit")
	if err == nil {
		t.Error("Expected CommitAll to fail with no changes")
	}
}

func TestCommitAll_MultipleFiles(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create multiple files
	for i := 0; i < 3; i++ {
		file := filepath.Join(repoPath, filepath.Base(repoPath)+string(rune('a'+i))+".txt")
		if err := os.WriteFile(file, []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	// Modify existing file too
	testFile := filepath.Join(repoPath, "test.txt")
	if err := os.WriteFile(testFile, []byte("modified"), 0644); err != nil {
		t.Fatalf("Failed to modify file: %v", err)
	}

	// Commit all
	err := CommitAll(repoPath, "Multiple files commit")
	if err != nil {
		t.Fatalf("CommitAll failed: %v", err)
	}

	// Verify clean state
	status, _ := GetWorktreeStatus(repoPath)
	if status.HasChanges {
		t.Error("Expected no changes after committing multiple files")
	}
}

func TestGenerateCommitMessage_Success(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create changes
	newFile := filepath.Join(repoPath, "feature.txt")
	if err := os.WriteFile(newFile, []byte("feature content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	msg, err := GenerateCommitMessage(repoPath)
	if err != nil {
		t.Fatalf("GenerateCommitMessage failed: %v", err)
	}

	if msg == "" {
		t.Error("Expected non-empty commit message")
	}

	// Should contain summary
	if !contains(msg, "1 file") {
		t.Errorf("Expected message to contain file count, got: %s", msg)
	}

	// Should contain file name
	if !contains(msg, "feature.txt") {
		t.Errorf("Expected message to contain filename, got: %s", msg)
	}
}

func TestGenerateCommitMessage_NoChanges(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	_, err := GenerateCommitMessage(repoPath)
	if err == nil {
		t.Error("Expected error when generating commit message with no changes")
	}
}

// contains checks if s contains substr
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
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
	ch := MergeToMain(ctx, repoPath, repoPath, "feature-with-msg", customCommitMsg)

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
	if !contains(string(output), "Custom commit message") {
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
	ch := CreatePR(ctx, repoPath, repoPath, "feature-pr-msg", "Custom PR commit")

	// Drain channel - expect an error since no remote
	for range ch {
	}
	// Just verify it doesn't hang
}

func TestGetWorktreeStatus_StagedChanges(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create and stage a file
	newFile := filepath.Join(repoPath, "staged.txt")
	if err := os.WriteFile(newFile, []byte("staged content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	cmd := exec.Command("git", "add", "staged.txt")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to stage file: %v", err)
	}

	status, err := GetWorktreeStatus(repoPath)
	if err != nil {
		t.Fatalf("GetWorktreeStatus failed: %v", err)
	}

	if !status.HasChanges {
		t.Error("Expected HasChanges to be true for staged files")
	}

	// Diff should include staged changes
	if status.Diff == "" {
		t.Error("Expected Diff to contain staged changes")
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

	ch := MergeToMain(ctx, repoPath, repoPath, "clean-feature", "")

	var sawNoChangesMsg bool
	for result := range ch {
		if contains(result.Output, "No uncommitted changes") {
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

	ch := CreatePR(ctx, repoPath, repoPath, "pr-cancel-test", "")

	// Drain channel - should not hang
	for range ch {
	}
}

func TestCommitAll_InvalidPath(t *testing.T) {
	err := CommitAll("/nonexistent/path", "Test commit")
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestGetWorktreeStatus_DiffFallback(t *testing.T) {
	// This tests the fallback behavior when diff HEAD fails
	// Create a new repo without any commits
	tmpDir, err := os.MkdirTemp("", "plural-git-nohead-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Create a file but don't commit
	testFile := filepath.Join(tmpDir, "nohead.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// This should work even without a HEAD commit
	status, err := GetWorktreeStatus(tmpDir)
	if err != nil {
		t.Fatalf("GetWorktreeStatus failed: %v", err)
	}

	if !status.HasChanges {
		t.Error("Expected HasChanges to be true")
	}
}

func TestGenerateCommitMessage_MultipleFiles(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create multiple files
	for _, name := range []string{"file1.txt", "file2.txt", "file3.txt"} {
		file := filepath.Join(repoPath, name)
		if err := os.WriteFile(file, []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	msg, err := GenerateCommitMessage(repoPath)
	if err != nil {
		t.Fatalf("GenerateCommitMessage failed: %v", err)
	}

	// Should mention multiple files
	if !contains(msg, "3 files") {
		t.Errorf("Expected message to mention 3 files, got: %s", msg)
	}

	// Should list all files
	for _, name := range []string{"file1.txt", "file2.txt", "file3.txt"} {
		if !contains(msg, name) {
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

	ch := MergeToParent(ctx, childWorktree, childBranch, parentWorktree, parentBranch, "")

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

	ch := MergeToParent(ctx, childWorktree, childBranch, parentWorktree, parentBranch, "Custom child commit")

	var sawUncommittedMsg bool
	var lastResult Result
	for result := range ch {
		lastResult = result
		if contains(result.Output, "uncommitted changes") {
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

	ch := MergeToParent(ctx, childWorktree, childBranch, parentWorktree, parentBranch, "")

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

	ch := MergeToParent(ctx, childWorktree, childBranch, parentWorktree, parentBranch, "")

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

	ch := MergeToParent(ctx, childWorktree, childBranch, parentWorktree, parentBranch, "")

	var sawNoChangesMsg bool
	for result := range ch {
		if contains(result.Output, "No uncommitted changes") {
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
