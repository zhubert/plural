package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhubert/plural/internal/config"
)

// createTestRepo creates a temporary git repository for testing
func createTestRepo(t *testing.T) string {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "plural-session-test-*")
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

	// Create initial commit (required for worktree)
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

// cleanupWorktrees removes worktrees created during testing
func cleanupWorktrees(repoPath string) {
	worktreeDir := filepath.Join(filepath.Dir(repoPath), ".plural-worktrees")
	os.RemoveAll(worktreeDir)

	// Also prune the worktree references from git
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = repoPath
	cmd.Run()
}

func TestCreate(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)
	defer cleanupWorktrees(repoPath)

	session, err := Create(repoPath, "", "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify session fields
	if session.ID == "" {
		t.Error("Session ID should not be empty")
	}

	if session.RepoPath != repoPath {
		t.Errorf("RepoPath = %q, want %q", session.RepoPath, repoPath)
	}

	if session.WorkTree == "" {
		t.Error("WorkTree should not be empty")
	}

	if !strings.HasPrefix(session.Branch, "plural-") {
		t.Errorf("Branch = %q, should start with 'plural-'", session.Branch)
	}

	if session.Name == "" {
		t.Error("Name should not be empty")
	}

	if session.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}

	// Verify the worktree was created
	if _, err := os.Stat(session.WorkTree); os.IsNotExist(err) {
		t.Error("Worktree directory should exist")
	}

	// Verify it's a valid git directory
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = session.WorkTree
	if err := cmd.Run(); err != nil {
		t.Error("Worktree should be a valid git directory")
	}
}

func TestCreate_MultipleSessions(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)
	defer cleanupWorktrees(repoPath)

	// Create multiple sessions
	session1, err := Create(repoPath, "", "")
	if err != nil {
		t.Fatalf("Create session1 failed: %v", err)
	}

	session2, err := Create(repoPath, "", "")
	if err != nil {
		t.Fatalf("Create session2 failed: %v", err)
	}

	// They should have different IDs
	if session1.ID == session2.ID {
		t.Error("Sessions should have different IDs")
	}

	// They should have different worktrees
	if session1.WorkTree == session2.WorkTree {
		t.Error("Sessions should have different worktrees")
	}

	// They should have different branches
	if session1.Branch == session2.Branch {
		t.Error("Sessions should have different branches")
	}
}

func TestCreate_InvalidRepo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plural-session-invalid-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Try to create session in non-git directory
	_, err = Create(tmpDir, "", "")
	if err == nil {
		t.Error("Create should fail for non-git directory")
	}
}

func TestValidateRepo_Valid(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	err := ValidateRepo(repoPath)
	if err != nil {
		t.Errorf("ValidateRepo failed for valid repo: %v", err)
	}
}

func TestValidateRepo_Invalid(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plural-validate-invalid-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	err = ValidateRepo(tmpDir)
	if err == nil {
		t.Error("ValidateRepo should fail for non-git directory")
	}
}

func TestValidateRepo_TildePath(t *testing.T) {
	err := ValidateRepo("~/some/path")
	if err == nil {
		t.Error("ValidateRepo should reject ~ paths")
	}
	if !strings.Contains(err.Error(), "absolute path") {
		t.Errorf("Error should mention absolute path: %v", err)
	}
}

func TestValidateRepo_NonexistentPath(t *testing.T) {
	err := ValidateRepo("/nonexistent/path/to/repo")
	if err == nil {
		t.Error("ValidateRepo should fail for nonexistent path")
	}
}

func TestGetGitRoot_Valid(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	root := GetGitRoot(repoPath)

	// Resolve symlinks for comparison (macOS has /var -> /private/var)
	expectedPath, _ := filepath.EvalSymlinks(repoPath)
	actualPath, _ := filepath.EvalSymlinks(root)

	if actualPath != expectedPath {
		t.Errorf("GetGitRoot = %q, want %q", root, repoPath)
	}
}

func TestGetGitRoot_Subdirectory(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a subdirectory
	subDir := filepath.Join(repoPath, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	root := GetGitRoot(subDir)

	// Resolve symlinks for comparison (macOS has /var -> /private/var)
	expectedPath, _ := filepath.EvalSymlinks(repoPath)
	actualPath, _ := filepath.EvalSymlinks(root)

	if actualPath != expectedPath {
		t.Errorf("GetGitRoot from subdir = %q, want %q", root, repoPath)
	}
}

func TestGetGitRoot_Invalid(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plural-gitroot-invalid-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	root := GetGitRoot(tmpDir)
	if root != "" {
		t.Errorf("GetGitRoot for non-git dir = %q, want empty string", root)
	}
}

func TestGetGitRoot_Nonexistent(t *testing.T) {
	root := GetGitRoot("/nonexistent/path")
	if root != "" {
		t.Errorf("GetGitRoot for nonexistent path = %q, want empty string", root)
	}
}

func TestGetCurrentDirGitRoot(t *testing.T) {
	// Save current directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(origDir)

	// Create a test repo and cd into it
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	if err := os.Chdir(repoPath); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	root := GetCurrentDirGitRoot()

	// Resolve symlinks for comparison (macOS has /var -> /private/var)
	expectedPath, _ := filepath.EvalSymlinks(repoPath)
	actualPath, _ := filepath.EvalSymlinks(root)

	if actualPath != expectedPath {
		t.Errorf("GetCurrentDirGitRoot = %q, want %q", root, repoPath)
	}
}

func TestSessionName_Format(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)
	defer cleanupWorktrees(repoPath)

	session, err := Create(repoPath, "", "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Name should be in format "reponame/shortid"
	parts := strings.Split(session.Name, "/")
	if len(parts) != 2 {
		t.Errorf("Session name format incorrect: %q", session.Name)
	}

	repoName := filepath.Base(repoPath)
	if parts[0] != repoName {
		t.Errorf("Session name repo part = %q, want %q", parts[0], repoName)
	}

	// Short ID should be 8 characters
	if len(parts[1]) != 8 {
		t.Errorf("Session name short ID length = %d, want 8", len(parts[1]))
	}

	// Short ID should be prefix of full ID
	if !strings.HasPrefix(session.ID, parts[1]) {
		t.Errorf("Short ID %q should be prefix of full ID %q", parts[1], session.ID)
	}
}

func TestBranchName_Format(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)
	defer cleanupWorktrees(repoPath)

	session, err := Create(repoPath, "", "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Branch should be "plural-<UUID>"
	expectedPrefix := "plural-"
	if !strings.HasPrefix(session.Branch, expectedPrefix) {
		t.Errorf("Branch %q should start with %q", session.Branch, expectedPrefix)
	}

	// The rest should be the session ID
	branchID := strings.TrimPrefix(session.Branch, expectedPrefix)
	if branchID != session.ID {
		t.Errorf("Branch ID part = %q, want session ID %q", branchID, session.ID)
	}
}

func TestWorktreePath_Location(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)
	defer cleanupWorktrees(repoPath)

	session, err := Create(repoPath, "", "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Worktree should be in .plural-worktrees directory
	expectedDir := filepath.Join(filepath.Dir(repoPath), ".plural-worktrees")
	if !strings.HasPrefix(session.WorkTree, expectedDir) {
		t.Errorf("WorkTree %q should be in %q", session.WorkTree, expectedDir)
	}

	// Worktree directory name should be the session ID
	worktreeName := filepath.Base(session.WorkTree)
	if worktreeName != session.ID {
		t.Errorf("Worktree directory name = %q, want session ID %q", worktreeName, session.ID)
	}
}

func TestCreate_CustomBranch(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)
	defer cleanupWorktrees(repoPath)

	customBranch := "feature/my-cool-feature"
	session, err := Create(repoPath, customBranch, "")
	if err != nil {
		t.Fatalf("Create with custom branch failed: %v", err)
	}

	if session.Branch != customBranch {
		t.Errorf("Branch = %q, want %q", session.Branch, customBranch)
	}
}

func TestValidateBranchName(t *testing.T) {
	tests := []struct {
		name    string
		branch  string
		wantErr bool
	}{
		{"empty is allowed", "", false},
		{"simple name", "feature", false},
		{"with slash", "feature/my-branch", false},
		{"with underscore", "feature_test", false},
		{"with dash", "feature-test", false},
		{"with dots", "v1.2.3", false},
		{"complex valid", "feature/ABC-123_test.v2", false},
		{"starts with dash", "-invalid", true},
		{"ends with .lock", "branch.lock", true},
		{"contains ..", "branch..name", true},
		{"contains space", "branch name", true},
		{"contains tilde", "branch~name", true},
		{"contains caret", "branch^name", true},
		{"contains colon", "branch:name", true},
		{"too long", strings.Repeat("a", 101), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBranchName(tt.branch)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBranchName(%q) error = %v, wantErr %v", tt.branch, err, tt.wantErr)
			}
		})
	}
}

func TestBranchExists(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// The default branch should exist (main or master)
	// Check for main first, then master
	if !BranchExists(repoPath, "main") && !BranchExists(repoPath, "master") {
		t.Error("Expected default branch to exist")
	}

	// A random branch should not exist
	if BranchExists(repoPath, "nonexistent-branch-12345") {
		t.Error("Expected nonexistent branch to not exist")
	}
}

func TestDelete(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)
	defer cleanupWorktrees(repoPath)

	// Create a session first
	session, err := Create(repoPath, "", "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify worktree exists
	if _, err := os.Stat(session.WorkTree); os.IsNotExist(err) {
		t.Fatal("Worktree should exist before delete")
	}

	// Verify branch exists
	if !BranchExists(repoPath, session.Branch) {
		t.Fatal("Branch should exist before delete")
	}

	// Delete the session
	err = Delete(session)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify worktree no longer exists
	if _, err := os.Stat(session.WorkTree); !os.IsNotExist(err) {
		t.Error("Worktree should be deleted")
	}

	// Verify branch is deleted
	if BranchExists(repoPath, session.Branch) {
		t.Error("Branch should be deleted")
	}
}

func TestDelete_NonexistentWorktree(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a fake session pointing to nonexistent worktree
	session := &config.Session{
		ID:       "fake-session-id",
		RepoPath: repoPath,
		WorkTree: "/nonexistent/worktree/path",
		Branch:   "nonexistent-branch",
	}

	// Delete should return an error but not panic
	err := Delete(session)
	if err == nil {
		t.Error("Expected error when deleting nonexistent worktree")
	}
}

func TestDelete_AlreadyDeletedBranch(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)
	defer cleanupWorktrees(repoPath)

	// Create a session
	session, err := Create(repoPath, "", "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Manually delete the branch first
	cmd := exec.Command("git", "worktree", "remove", session.WorkTree, "--force")
	cmd.Dir = repoPath
	cmd.Run()

	cmd = exec.Command("git", "branch", "-D", session.Branch)
	cmd.Dir = repoPath
	cmd.Run()

	// Delete should handle this gracefully (branch deletion is best-effort)
	err = Delete(session)
	// Error is expected since worktree is already gone
	// But it shouldn't panic
}

func TestFindOrphanedWorktrees(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)
	defer cleanupWorktrees(repoPath)

	// Create a session
	session, err := Create(repoPath, "", "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Create a config that knows about this session
	cfg := &config.Config{
		Repos:    []string{repoPath},
		Sessions: []config.Session{*session},
	}

	// Find orphans - there should be none since the session is in config
	orphans, err := FindOrphanedWorktrees(cfg)
	if err != nil {
		t.Fatalf("FindOrphanedWorktrees failed: %v", err)
	}

	if len(orphans) != 0 {
		t.Errorf("Expected 0 orphans, got %d", len(orphans))
	}

	// Now create a config without this session (simulating orphan)
	emptyConfig := &config.Config{
		Repos:    []string{repoPath},
		Sessions: []config.Session{},
	}

	orphans, err = FindOrphanedWorktrees(emptyConfig)
	if err != nil {
		t.Fatalf("FindOrphanedWorktrees failed: %v", err)
	}

	if len(orphans) != 1 {
		t.Errorf("Expected 1 orphan, got %d", len(orphans))
	}

	if len(orphans) > 0 && orphans[0].ID != session.ID {
		t.Errorf("Orphan ID = %q, want %q", orphans[0].ID, session.ID)
	}
}

func TestFindOrphanedWorktrees_NoWorktrees(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	cfg := &config.Config{
		Repos:    []string{repoPath},
		Sessions: []config.Session{},
	}

	orphans, err := FindOrphanedWorktrees(cfg)
	if err != nil {
		t.Fatalf("FindOrphanedWorktrees failed: %v", err)
	}

	// No worktrees directory exists, so no orphans
	if len(orphans) != 0 {
		t.Errorf("Expected 0 orphans, got %d", len(orphans))
	}
}

func TestPruneOrphanedWorktrees(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)
	defer cleanupWorktrees(repoPath)

	// Create a session
	session, err := Create(repoPath, "", "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Create a config without this session (making it an orphan)
	cfg := &config.Config{
		Repos:    []string{repoPath},
		Sessions: []config.Session{},
	}

	// Verify worktree exists before prune
	if _, err := os.Stat(session.WorkTree); os.IsNotExist(err) {
		t.Fatal("Worktree should exist before prune")
	}

	// Prune orphans
	pruned, err := PruneOrphanedWorktrees(cfg)
	if err != nil {
		t.Fatalf("PruneOrphanedWorktrees failed: %v", err)
	}

	if pruned != 1 {
		t.Errorf("Expected 1 pruned, got %d", pruned)
	}

	// Verify worktree is gone
	if _, err := os.Stat(session.WorkTree); !os.IsNotExist(err) {
		t.Error("Worktree should be removed after prune")
	}
}

func TestOrphanedWorktree_Fields(t *testing.T) {
	orphan := OrphanedWorktree{
		Path:     "/path/to/worktree",
		RepoPath: "/path/to/repo",
		ID:       "session-id-123",
	}

	if orphan.Path != "/path/to/worktree" {
		t.Error("Path mismatch")
	}
	if orphan.RepoPath != "/path/to/repo" {
		t.Error("RepoPath mismatch")
	}
	if orphan.ID != "session-id-123" {
		t.Error("ID mismatch")
	}
}

func TestCreate_CustomBranchDisplayName(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)
	defer cleanupWorktrees(repoPath)

	customBranch := "feature/my-feature"
	session, err := Create(repoPath, customBranch, "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Name should include the custom branch, not the short UUID
	if !strings.Contains(session.Name, customBranch) {
		t.Errorf("Session name %q should contain branch name %q", session.Name, customBranch)
	}
}

func TestCreate_BranchPrefix(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)
	defer cleanupWorktrees(repoPath)

	branchPrefix := "zhubert/"
	session, err := Create(repoPath, "", branchPrefix)
	if err != nil {
		t.Fatalf("Create with branch prefix failed: %v", err)
	}

	// Branch should start with prefix
	if !strings.HasPrefix(session.Branch, branchPrefix) {
		t.Errorf("Branch %q should start with prefix %q", session.Branch, branchPrefix)
	}

	// Branch should still have plural- after prefix
	expectedPrefix := branchPrefix + "plural-"
	if !strings.HasPrefix(session.Branch, expectedPrefix) {
		t.Errorf("Branch %q should start with %q", session.Branch, expectedPrefix)
	}

	// Display name should include the prefix
	if !strings.Contains(session.Name, branchPrefix) {
		t.Errorf("Session name %q should contain prefix %q", session.Name, branchPrefix)
	}
}

func TestCreate_BranchPrefixWithCustomBranch(t *testing.T) {
	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)
	defer cleanupWorktrees(repoPath)

	branchPrefix := "zhubert/"
	customBranch := "issue-42"
	session, err := Create(repoPath, customBranch, branchPrefix)
	if err != nil {
		t.Fatalf("Create with branch prefix and custom branch failed: %v", err)
	}

	// Branch should be prefix + custom branch
	expectedBranch := branchPrefix + customBranch
	if session.Branch != expectedBranch {
		t.Errorf("Branch = %q, want %q", session.Branch, expectedBranch)
	}

	// Display name should include the full branch name with prefix
	if !strings.Contains(session.Name, expectedBranch) {
		t.Errorf("Session name %q should contain %q", session.Name, expectedBranch)
	}
}
