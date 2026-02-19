package app

import (
	"testing"

	"github.com/zhubert/plural-core/config"
	"github.com/zhubert/plural-core/exec"
	"github.com/zhubert/plural-core/git"
)

// TestCreatePRsForSessions_WithUncommittedChanges verifies that sessions with
// uncommitted changes are not skipped and are passed to CreatePR which will
// auto-commit them.
func TestCreatePRsForSessions_WithUncommittedChanges(t *testing.T) {
	// Create a mock executor
	mockExec := exec.NewMockExecutor(nil)

	// Mock git commands that might be called
	mockExec.AddPrefixMatch("git", []string{}, exec.MockResponse{})
	mockExec.AddPrefixMatch("gh", []string{}, exec.MockResponse{})

	// Create test config
	cfg := testConfig()

	// Create test model with mock executor
	m := testModel(cfg)
	mockGitService := git.NewGitServiceWithExecutor(mockExec)
	m.SetGitService(mockGitService)

	// Create test sessions - one with uncommitted changes
	sess1 := config.Session{
		ID:         "sess1",
		RepoPath:   "/test/repo",
		WorkTree:   "/test/worktree1",
		Branch:     "test-branch-1",
		BaseBranch: "main",
		PRCreated:  false,
		Merged:     false,
	}

	sess2 := config.Session{
		ID:         "sess2",
		RepoPath:   "/test/repo",
		WorkTree:   "/test/worktree2",
		Branch:     "test-branch-2",
		BaseBranch: "main",
		PRCreated:  true, // Already has PR
		Merged:     false,
	}

	sess3 := config.Session{
		ID:         "sess3",
		RepoPath:   "/test/repo",
		WorkTree:   "/test/worktree3",
		Branch:     "test-branch-3",
		BaseBranch: "main",
		PRCreated:  false,
		Merged:     true, // Already merged
	}

	sessions := []config.Session{sess1, sess2, sess3}

	// Call createPRsForSessions
	_, cmd := m.createPRsForSessions(sessions)

	// Verify that exactly one session got a PR creation started
	// (sess1 should proceed despite uncommitted changes, sess2 and sess3 should be skipped)
	sessionState1 := m.sessionState().GetIfExists(sess1.ID)
	sessionState2 := m.sessionState().GetIfExists(sess2.ID)
	sessionState3 := m.sessionState().GetIfExists(sess3.ID)

	// sess1 should have merge in progress (PR creation is a merge operation)
	if sessionState1 == nil || !sessionState1.IsMerging() {
		t.Error("Expected sess1 to have PR creation in progress")
	}

	// sess2 and sess3 should not have merge operations
	if sessionState2 != nil && sessionState2.IsMerging() {
		t.Error("Expected sess2 to be skipped (already has PR)")
	}

	if sessionState3 != nil && sessionState3.IsMerging() {
		t.Error("Expected sess3 to be skipped (already merged)")
	}

	// Verify we got a command batch back
	if cmd == nil {
		t.Error("Expected non-nil command from createPRsForSessions")
	}
}

// TestCreatePRsForSessions_AllAlreadyHavePRs verifies that when all sessions
// already have PRs or are merged, no PR creation is started.
func TestCreatePRsForSessions_AllAlreadyHavePRs(t *testing.T) {
	// Create test config
	cfg := testConfig()

	// Create test model
	m := testModel(cfg)

	// Create test sessions - all already have PRs or are merged
	sess1 := config.Session{
		ID:         "sess1",
		RepoPath:   "/test/repo",
		WorkTree:   "/test/worktree1",
		Branch:     "test-branch-1",
		BaseBranch: "main",
		PRCreated:  true,
		Merged:     false,
	}

	sess2 := config.Session{
		ID:         "sess2",
		RepoPath:   "/test/repo",
		WorkTree:   "/test/worktree2",
		Branch:     "test-branch-2",
		BaseBranch: "main",
		PRCreated:  false,
		Merged:     true,
	}

	sessions := []config.Session{sess1, sess2}

	// Call createPRsForSessions
	_, cmd := m.createPRsForSessions(sessions)

	// Verify that no sessions have merge operations
	sessionState1 := m.sessionState().GetIfExists(sess1.ID)
	sessionState2 := m.sessionState().GetIfExists(sess2.ID)

	if sessionState1 != nil && sessionState1.IsMerging() {
		t.Error("Expected sess1 to be skipped (already has PR)")
	}

	if sessionState2 != nil && sessionState2.IsMerging() {
		t.Error("Expected sess2 to be skipped (already merged)")
	}

	// We should still get a command (for the flash message)
	if cmd == nil {
		t.Error("Expected non-nil command from createPRsForSessions")
	}
}

// TestExecuteBulkCreatePRs verifies that the bulk create PRs action correctly
// converts session IDs to session objects and calls createPRsForSessions.
func TestExecuteBulkCreatePRs(t *testing.T) {
	// Create test config
	cfg := testConfig()

	// Create test model
	m := testModel(cfg)

	// Add test sessions to config
	sess1 := config.Session{
		ID:         "sess1",
		RepoPath:   "/test/repo",
		WorkTree:   "/test/worktree1",
		Branch:     "test-branch-1",
		BaseBranch: "main",
		PRCreated:  false,
		Merged:     false,
	}

	sess2 := config.Session{
		ID:         "sess2",
		RepoPath:   "/test/repo",
		WorkTree:   "/test/worktree2",
		Branch:     "test-branch-2",
		BaseBranch: "main",
		PRCreated:  false,
		Merged:     false,
	}

	cfg.AddSession(sess1)
	cfg.AddSession(sess2)

	// Call executeBulkCreatePRs
	sessionIDs := []string{"sess1", "sess2"}
	_, cmd := m.executeBulkCreatePRs(sessionIDs)

	// Verify modal is hidden and multi-select is exited
	if m.modal.IsVisible() {
		t.Error("Expected modal to be hidden")
	}

	// Verify we got a command
	if cmd == nil {
		t.Error("Expected non-nil command from executeBulkCreatePRs")
	}
}
