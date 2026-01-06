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

	ch := MergeToMain(ctx, repoPath, "feature-branch")

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

	ch := MergeToMain(ctx, repoPath, "conflict-branch")

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

	ch := MergeToMain(ctx, repoPath, "test-branch")

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

	ch := CreatePR(ctx, repoPath, "test-branch")

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
