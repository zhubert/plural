package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
