package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/mcp"
)

func createTestConfig() *config.Config {
	return &config.Config{
		Repos: []string{"/test/repo"},
		Sessions: []config.Session{
			{
				ID:        "session-1",
				RepoPath:  "/test/repo",
				WorkTree:  "/test/worktree",
				Branch:    "plural-test",
				Name:      "repo/session1",
				CreatedAt: time.Now(),
				Started:   false,
			},
			{
				ID:        "session-2",
				RepoPath:  "/test/repo",
				WorkTree:  "/test/worktree2",
				Branch:    "custom-branch",
				Name:      "repo/session2",
				CreatedAt: time.Now(),
				Started:   true,
			},
		},
		AllowedTools:     []string{},
		RepoAllowedTools: make(map[string][]string),
	}
}

func TestNewSessionManager(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	if sm == nil {
		t.Fatal("NewSessionManager returned nil")
	}

	if sm.config != cfg {
		t.Error("SessionManager config reference mismatch")
	}

	if sm.stateManager == nil {
		t.Error("SessionManager stateManager should be initialized")
	}

	if sm.runners == nil {
		t.Error("SessionManager runners map should be initialized")
	}
}

func TestSessionManager_StateManager(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	stateManager := sm.StateManager()
	if stateManager == nil {
		t.Error("StateManager() should return non-nil")
	}

	if stateManager != sm.stateManager {
		t.Error("StateManager() should return the same instance")
	}
}

func TestSessionManager_GetRunner(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	// No runner initially
	runner := sm.GetRunner("session-1")
	if runner != nil {
		t.Error("GetRunner should return nil for non-existent runner")
	}

	// Set a runner
	testRunner := claude.New("session-1", "/test", false, nil)
	sm.runners["session-1"] = testRunner

	runner = sm.GetRunner("session-1")
	if runner != testRunner {
		t.Error("GetRunner should return the set runner")
	}
}

func TestSessionManager_GetRunners(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	runners := sm.GetRunners()
	if runners == nil {
		t.Error("GetRunners should return non-nil map")
	}

	if len(runners) != 0 {
		t.Errorf("Expected 0 runners initially, got %d", len(runners))
	}

	// Add runners
	sm.runners["session-1"] = claude.New("session-1", "/test", false, nil)
	sm.runners["session-2"] = claude.New("session-2", "/test", false, nil)

	runners = sm.GetRunners()
	if len(runners) != 2 {
		t.Errorf("Expected 2 runners, got %d", len(runners))
	}
}

func TestSessionManager_HasActiveStreaming(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	// No runners - no streaming
	if sm.HasActiveStreaming() {
		t.Error("Should not have active streaming with no runners")
	}

	// Add non-streaming runner
	runner := claude.New("session-1", "/test", false, nil)
	sm.runners["session-1"] = runner

	if sm.HasActiveStreaming() {
		t.Error("Should not have active streaming when runner is not streaming")
	}

	// Note: Cannot easily test streaming state without actually sending a message
	// The IsStreaming() method checks internal state that's set by Send()
}

func TestSessionManager_GetSession(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	// Get existing session
	sess := sm.GetSession("session-1")
	if sess == nil {
		t.Fatal("GetSession should return session for existing ID")
	}

	if sess.ID != "session-1" {
		t.Errorf("Expected session ID 'session-1', got %q", sess.ID)
	}

	// Get non-existent session
	sess = sm.GetSession("nonexistent")
	if sess != nil {
		t.Error("GetSession should return nil for non-existent ID")
	}
}

func TestSessionManager_Select_Nil(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	result := sm.Select(nil, "", "", "")
	if result != nil {
		t.Error("Select(nil) should return nil")
	}
}

func TestSessionManager_Select_SavesPreviousState(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	// Select a session with previous state to save
	sess := sm.GetSession("session-1")
	sm.Select(sess, "prev-session", "saved input", "saved streaming")

	// Verify state was saved
	state := sm.stateManager.GetIfExists("prev-session")
	if state == nil {
		t.Fatal("Expected state for prev-session")
	}
	if state.InputText != "saved input" {
		t.Errorf("Expected saved input 'saved input', got %q", state.InputText)
	}

	if state.StreamingContent != "saved streaming" {
		t.Errorf("Expected saved streaming 'saved streaming', got %q", state.StreamingContent)
	}
}

func TestSessionManager_Select_CreatesRunner(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	sess := sm.GetSession("session-1")
	result := sm.Select(sess, "", "", "")

	if result == nil {
		t.Fatal("Select should return non-nil result")
	}

	if result.Runner == nil {
		t.Error("Result should include runner")
	}

	// Runner should be cached
	cachedRunner := sm.GetRunner("session-1")
	if cachedRunner != result.Runner {
		t.Error("Runner should be cached after Select")
	}
}

func TestSessionManager_Select_ReusesRunner(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	// Pre-create a runner
	existingRunner := claude.New("session-1", "/test", false, nil)
	sm.runners["session-1"] = existingRunner

	sess := sm.GetSession("session-1")
	result := sm.Select(sess, "", "", "")

	if result.Runner != existingRunner {
		t.Error("Select should reuse existing runner")
	}
}

func TestSessionManager_Select_HeaderName(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	// Session with auto-generated branch (plural-)
	sess := sm.GetSession("session-1")
	result := sm.Select(sess, "", "", "")

	if result.HeaderName != sess.Name {
		t.Errorf("Expected header name %q, got %q", sess.Name, result.HeaderName)
	}

	// Session with custom branch
	sess = sm.GetSession("session-2")
	result = sm.Select(sess, "", "", "")

	if result.HeaderName != "custom-branch" {
		t.Errorf("Expected header name 'custom-branch', got %q", result.HeaderName)
	}
}

func TestSessionManager_Select_RestoresState(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	sess := sm.GetSession("session-1")

	// Set up state to restore
	sm.stateManager.StartWaiting(sess.ID, nil)
	permReq := &mcp.PermissionRequest{Tool: "Bash", Description: "test"}
	state := sm.stateManager.GetOrCreate(sess.ID)
	state.PendingPermission = permReq
	state.StreamingContent = "streaming content"
	state.InputText = "saved input text"

	result := sm.Select(sess, "", "", "")

	if !result.IsWaiting {
		t.Error("Expected IsWaiting to be restored")
	}

	if result.Permission == nil {
		t.Error("Expected permission to be restored")
	}

	if result.Streaming != "streaming content" {
		t.Errorf("Expected streaming content, got %q", result.Streaming)
	}

	if result.SavedInput != "saved input text" {
		t.Errorf("Expected saved input, got %q", result.SavedInput)
	}
}

func TestSessionManager_DeleteSession(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	// Create runner and state
	runner := claude.New("session-1", "/test", false, nil)
	sm.runners["session-1"] = runner
	sm.stateManager.GetOrCreate("session-1").InputText = "test input"

	// Delete session
	deletedRunner := sm.DeleteSession("session-1")

	if deletedRunner != runner {
		t.Error("DeleteSession should return the deleted runner")
	}

	// Runner should be removed
	if sm.GetRunner("session-1") != nil {
		t.Error("Runner should be removed after delete")
	}

	// State should be cleaned up
	state := sm.stateManager.GetIfExists("session-1")
	if state != nil {
		t.Error("State should be cleaned up after delete")
	}
}

func TestSessionManager_DeleteSession_NoRunner(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	// Delete session with no runner
	deletedRunner := sm.DeleteSession("session-1")

	if deletedRunner != nil {
		t.Error("DeleteSession should return nil when no runner exists")
	}
}

func TestSessionManager_SetRunner(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	runner := claude.New("session-1", "/test", false, nil)
	sm.SetRunner("session-1", runner)

	if sm.GetRunner("session-1") != runner {
		t.Error("SetRunner should set the runner")
	}
}

func TestSessionManager_AddAllowedTool(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	// Create runner first
	runner := claude.New("session-1", "/test", false, nil)
	sm.runners["session-1"] = runner

	// Add tool
	sm.AddAllowedTool("session-1", "Bash(git:*)")

	// Tool should be saved to config
	repoTools := cfg.GetAllowedToolsForRepo("/test/repo")
	found := false
	for _, tool := range repoTools {
		if tool == "Bash(git:*)" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Tool should be saved to config")
	}
}

func TestSessionManager_AddAllowedTool_NoSession(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	// Should not panic with non-existent session
	sm.AddAllowedTool("nonexistent", "Bash(git:*)")
}

func TestSessionManager_AddAllowedTool_NoRunner(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	// Should not panic when session exists but runner doesn't
	sm.AddAllowedTool("session-1", "Bash(git:*)")
}

// trackingMockRunner tracks SetForkFromSession calls for testing
type trackingMockRunner struct {
	*claude.MockRunner
	forkFromSessionID string
}

func newTrackingMockRunner(sessionID string, sessionStarted bool, msgs []claude.Message) *trackingMockRunner {
	return &trackingMockRunner{
		MockRunner: claude.NewMockRunner(sessionID, sessionStarted, msgs),
	}
}

func (m *trackingMockRunner) SetForkFromSession(parentSessionID string) {
	m.forkFromSessionID = parentSessionID
}

func TestSessionManager_Select_ForkedSession(t *testing.T) {
	// Create temp directories to simulate worktrees
	tempDir := t.TempDir()
	parentWorktree := filepath.Join(tempDir, "worktree1")
	childWorktree := filepath.Join(tempDir, "worktree2")
	os.MkdirAll(parentWorktree, 0755)
	os.MkdirAll(childWorktree, 0755)

	// Create the Claude session file in the parent's project directory
	homeDir, _ := os.UserHomeDir()
	escapePath := func(path string) string {
		escaped := strings.ReplaceAll(path, "/", "-")
		return strings.ReplaceAll(escaped, ".", "-")
	}
	parentProjectDir := filepath.Join(homeDir, ".claude", "projects", escapePath(parentWorktree))
	os.MkdirAll(parentProjectDir, 0700)
	sessionFile := filepath.Join(parentProjectDir, "parent-session.jsonl")
	os.WriteFile(sessionFile, []byte(`{"type":"test"}`+"\n"), 0600)
	defer os.RemoveAll(parentProjectDir)

	// Also clean up child project dir after test
	childProjectDir := filepath.Join(homeDir, ".claude", "projects", escapePath(childWorktree))
	defer os.RemoveAll(childProjectDir)

	// Create config with a forked child session
	cfg := &config.Config{
		Repos: []string{"/test/repo"},
		Sessions: []config.Session{
			{
				ID:       "parent-session",
				RepoPath: "/test/repo",
				WorkTree: parentWorktree,
				Branch:   "plural-parent",
				Name:     "repo/parent",
				Started:  true,
			},
			{
				ID:       "child-session",
				RepoPath: "/test/repo",
				WorkTree: childWorktree,
				Branch:   "plural-child",
				Name:     "repo/child",
				Started:  false, // Not started yet
				ParentID: "parent-session",
			},
		},
	}
	sm := NewSessionManager(cfg, git.NewGitService())

	sm.SetRunnerFactory(func(sessionID, workingDir string, sessionStarted bool, initialMessages []claude.Message) claude.RunnerInterface {
		return newTrackingMockRunner(sessionID, sessionStarted, initialMessages)
	})

	childSess := sm.GetSession("child-session")
	result := sm.Select(childSess, "", "", "")

	trackingRunner, ok := result.Runner.(*trackingMockRunner)
	if !ok {
		t.Fatal("Expected trackingMockRunner")
	}

	// SetForkFromSession should have been called with parent ID
	if trackingRunner.forkFromSessionID != "parent-session" {
		t.Errorf("Expected SetForkFromSession called with 'parent-session', got %q", trackingRunner.forkFromSessionID)
	}

	// Verify the session file was copied to child's project dir
	copiedFile := filepath.Join(childProjectDir, "parent-session.jsonl")
	if _, err := os.Stat(copiedFile); os.IsNotExist(err) {
		t.Error("Expected Claude session file to be copied to child's project directory")
	}
}

func TestSessionManager_Select_ForkedSession_AlreadyStarted(t *testing.T) {
	// If session already started, don't set fork (would use resume instead)
	cfg := &config.Config{
		Repos: []string{"/test/repo"},
		Sessions: []config.Session{
			{
				ID:       "parent-session",
				RepoPath: "/test/repo",
				WorkTree: "/test/worktree1",
				Branch:   "plural-parent",
				Name:     "repo/parent",
				Started:  true,
			},
			{
				ID:       "child-session",
				RepoPath: "/test/repo",
				WorkTree: "/test/worktree2",
				Branch:   "plural-child",
				Name:     "repo/child",
				Started:  true, // Already started
				ParentID: "parent-session",
			},
		},
	}
	sm := NewSessionManager(cfg, git.NewGitService())

	sm.SetRunnerFactory(func(sessionID, workingDir string, sessionStarted bool, initialMessages []claude.Message) claude.RunnerInterface {
		return newTrackingMockRunner(sessionID, sessionStarted, initialMessages)
	})

	childSess := sm.GetSession("child-session")
	result := sm.Select(childSess, "", "", "")

	trackingRunner, ok := result.Runner.(*trackingMockRunner)
	if !ok {
		t.Fatal("Expected trackingMockRunner")
	}

	// SetForkFromSession should NOT have been called (session already started)
	if trackingRunner.forkFromSessionID != "" {
		t.Errorf("Expected SetForkFromSession NOT called for started session, got %q", trackingRunner.forkFromSessionID)
	}
}

func TestSessionManager_Select_NonForkedSession(t *testing.T) {
	// Session without parent should not set fork
	cfg := &config.Config{
		Repos: []string{"/test/repo"},
		Sessions: []config.Session{
			{
				ID:       "session-1",
				RepoPath: "/test/repo",
				WorkTree: "/test/worktree1",
				Branch:   "plural-test",
				Name:     "repo/session1",
				Started:  false,
				ParentID: "", // No parent
			},
		},
	}
	sm := NewSessionManager(cfg, git.NewGitService())

	sm.SetRunnerFactory(func(sessionID, workingDir string, sessionStarted bool, initialMessages []claude.Message) claude.RunnerInterface {
		return newTrackingMockRunner(sessionID, sessionStarted, initialMessages)
	})

	sess := sm.GetSession("session-1")
	result := sm.Select(sess, "", "", "")

	trackingRunner, ok := result.Runner.(*trackingMockRunner)
	if !ok {
		t.Fatal("Expected trackingMockRunner")
	}

	// SetForkFromSession should NOT have been called (no parent)
	if trackingRunner.forkFromSessionID != "" {
		t.Errorf("Expected SetForkFromSession NOT called for non-forked session, got %q", trackingRunner.forkFromSessionID)
	}
}

func TestSessionManager_Select_ForkedSession_ParentNotStarted(t *testing.T) {
	// If parent session hasn't been started yet (no Claude session to fork from),
	// we should NOT try to fork - just start as a new session
	cfg := &config.Config{
		Repos: []string{"/test/repo"},
		Sessions: []config.Session{
			{
				ID:       "parent-session",
				RepoPath: "/test/repo",
				WorkTree: "/test/worktree1",
				Branch:   "plural-parent",
				Name:     "repo/parent",
				Started:  false, // Parent NOT started - no Claude session to fork from
			},
			{
				ID:       "child-session",
				RepoPath: "/test/repo",
				WorkTree: "/test/worktree2",
				Branch:   "plural-child",
				Name:     "repo/child",
				Started:  false,
				ParentID: "parent-session",
			},
		},
	}
	sm := NewSessionManager(cfg, git.NewGitService())

	sm.SetRunnerFactory(func(sessionID, workingDir string, sessionStarted bool, initialMessages []claude.Message) claude.RunnerInterface {
		return newTrackingMockRunner(sessionID, sessionStarted, initialMessages)
	})

	childSess := sm.GetSession("child-session")
	result := sm.Select(childSess, "", "", "")

	trackingRunner, ok := result.Runner.(*trackingMockRunner)
	if !ok {
		t.Fatal("Expected trackingMockRunner")
	}

	// SetForkFromSession should NOT have been called (parent not started)
	if trackingRunner.forkFromSessionID != "" {
		t.Errorf("Expected SetForkFromSession NOT called when parent not started, got %q", trackingRunner.forkFromSessionID)
	}
}

func TestSessionManager_Select_ForkedSession_ParentNotFound(t *testing.T) {
	// If parent session doesn't exist at all, we should NOT try to fork
	cfg := &config.Config{
		Repos: []string{"/test/repo"},
		Sessions: []config.Session{
			// Note: no parent session in config
			{
				ID:       "child-session",
				RepoPath: "/test/repo",
				WorkTree: "/test/worktree2",
				Branch:   "plural-child",
				Name:     "repo/child",
				Started:  false,
				ParentID: "nonexistent-parent", // Parent doesn't exist
			},
		},
	}
	sm := NewSessionManager(cfg, git.NewGitService())

	sm.SetRunnerFactory(func(sessionID, workingDir string, sessionStarted bool, initialMessages []claude.Message) claude.RunnerInterface {
		return newTrackingMockRunner(sessionID, sessionStarted, initialMessages)
	})

	childSess := sm.GetSession("child-session")
	result := sm.Select(childSess, "", "", "")

	trackingRunner, ok := result.Runner.(*trackingMockRunner)
	if !ok {
		t.Fatal("Expected trackingMockRunner")
	}

	// SetForkFromSession should NOT have been called (parent not found)
	if trackingRunner.forkFromSessionID != "" {
		t.Errorf("Expected SetForkFromSession NOT called when parent not found, got %q", trackingRunner.forkFromSessionID)
	}
}

func TestCopyClaudeSessionForFork(t *testing.T) {
	// Create temp directories to simulate worktrees
	tempDir := t.TempDir()
	parentWorktree := filepath.Join(tempDir, "parent-worktree")
	childWorktree := filepath.Join(tempDir, "child-worktree")
	os.MkdirAll(parentWorktree, 0755)
	os.MkdirAll(childWorktree, 0755)

	// Helper to escape paths like Claude does
	escapePath := func(path string) string {
		escaped := strings.ReplaceAll(path, "/", "-")
		return strings.ReplaceAll(escaped, ".", "-")
	}

	homeDir, _ := os.UserHomeDir()
	parentProjectDir := filepath.Join(homeDir, ".claude", "projects", escapePath(parentWorktree))
	childProjectDir := filepath.Join(homeDir, ".claude", "projects", escapePath(childWorktree))

	// Clean up after test
	defer os.RemoveAll(parentProjectDir)
	defer os.RemoveAll(childProjectDir)

	// Create parent's Claude session file
	os.MkdirAll(parentProjectDir, 0700)
	sessionContent := `{"type":"assistant","message":{"usage":{"input_tokens":100}}}` + "\n"
	sessionFile := filepath.Join(parentProjectDir, "test-parent-id.jsonl")
	if err := os.WriteFile(sessionFile, []byte(sessionContent), 0600); err != nil {
		t.Fatalf("Failed to create test session file: %v", err)
	}

	// Test the copy function
	err := copyClaudeSessionForFork("test-parent-id", parentWorktree, childWorktree)
	if err != nil {
		t.Fatalf("copyClaudeSessionForFork failed: %v", err)
	}

	// Verify the file was copied
	copiedFile := filepath.Join(childProjectDir, "test-parent-id.jsonl")
	copiedContent, err := os.ReadFile(copiedFile)
	if err != nil {
		t.Fatalf("Failed to read copied file: %v", err)
	}

	if string(copiedContent) != sessionContent {
		t.Errorf("Copied content mismatch: got %q, want %q", string(copiedContent), sessionContent)
	}
}

func TestCopyClaudeSessionForFork_SourceNotFound(t *testing.T) {
	tempDir := t.TempDir()
	parentWorktree := filepath.Join(tempDir, "nonexistent-parent")
	childWorktree := filepath.Join(tempDir, "child")

	// Don't create the parent session file - it should fail
	err := copyClaudeSessionForFork("nonexistent-session", parentWorktree, childWorktree)
	if err == nil {
		t.Error("Expected error when source file doesn't exist")
	}
	if !os.IsNotExist(err) {
		t.Errorf("Expected os.IsNotExist error, got: %v", err)
	}
}

func TestCopyClaudeSessionForFork_NoSessionFileCopyFallback(t *testing.T) {
	// Test that when Claude session file copy fails, we create a synthetic session
	// if there are saved messages, or fall back to starting fresh if there are no messages.

	// Set up a temporary home directory to avoid polluting ~/.claude/ during tests
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	// Test case 1: No saved messages - should NOT call SetForkFromSession
	t.Run("no_messages", func(t *testing.T) {
		cfg := &config.Config{
			Repos: []string{"/test/repo"},
			Sessions: []config.Session{
				{
					ID:       "parent-session-1",
					RepoPath: "/test/repo",
					WorkTree: "/nonexistent/parent/worktree",
					Branch:   "plural-parent",
					Name:     "repo/parent",
					Started:  true,
				},
				{
					ID:       "child-session-1",
					RepoPath: "/test/repo",
					WorkTree: "/nonexistent/child/worktree",
					Branch:   "plural-child",
					Name:     "repo/child",
					Started:  false,
					ParentID: "parent-session-1",
				},
			},
		}
		sm := NewSessionManager(cfg, git.NewGitService())

		sm.SetRunnerFactory(func(sessionID, workingDir string, sessionStarted bool, initialMessages []claude.Message) claude.RunnerInterface {
			return newTrackingMockRunner(sessionID, sessionStarted, initialMessages)
		})

		childSess := sm.GetSession("child-session-1")
		result := sm.Select(childSess, "", "", "")

		trackingRunner, ok := result.Runner.(*trackingMockRunner)
		if !ok {
			t.Fatal("Expected trackingMockRunner")
		}

		// SetForkFromSession should NOT have been called because there are no messages
		if trackingRunner.forkFromSessionID != "" {
			t.Errorf("Expected SetForkFromSession NOT called when no messages exist, got %q", trackingRunner.forkFromSessionID)
		}
	})

	// Test case 2: Has saved messages - SHOULD call SetForkFromSession with synthetic session
	t.Run("with_messages", func(t *testing.T) {
		cfg := &config.Config{
			Repos: []string{"/test/repo"},
			Sessions: []config.Session{
				{
					ID:       "parent-session-2",
					RepoPath: "/test/repo",
					WorkTree: "/nonexistent/parent/worktree2",
					Branch:   "plural-parent",
					Name:     "repo/parent",
					Started:  true,
				},
				{
					ID:       "child-session-2",
					RepoPath: "/test/repo",
					WorkTree: "/nonexistent/child/worktree2",
					Branch:   "plural-child",
					Name:     "repo/child",
					Started:  false,
					ParentID: "parent-session-2",
				},
			},
		}

		// Save some messages for the parent session
		parentMsgs := []config.Message{
			{Role: "user", Content: "Test message 1"},
			{Role: "assistant", Content: "Test response 1"},
		}
		if err := config.SaveSessionMessages("parent-session-2", parentMsgs, config.MaxSessionMessageLines); err != nil {
			t.Fatalf("Failed to save parent messages: %v", err)
		}

		sm := NewSessionManager(cfg, git.NewGitService())

		sm.SetRunnerFactory(func(sessionID, workingDir string, sessionStarted bool, initialMessages []claude.Message) claude.RunnerInterface {
			return newTrackingMockRunner(sessionID, sessionStarted, initialMessages)
		})

		childSess := sm.GetSession("child-session-2")
		result := sm.Select(childSess, "", "", "")

		trackingRunner, ok := result.Runner.(*trackingMockRunner)
		if !ok {
			t.Fatal("Expected trackingMockRunner")
		}

		// SetForkFromSession SHOULD have been called because we created a synthetic session
		if trackingRunner.forkFromSessionID != "parent-session-2" {
			t.Errorf("Expected SetForkFromSession called with 'parent-session-2' (synthetic session), got %q", trackingRunner.forkFromSessionID)
		}
	})
}

func TestCopyClaudeSessionForFork_CleansUpOnFailure(t *testing.T) {
	// Test that partial destination file is cleaned up when copy fails
	// We test this by making the destination directory read-only after creating it,
	// which will cause os.Create to fail
	tempDir := t.TempDir()
	parentWorktree := filepath.Join(tempDir, "parent-worktree")
	childWorktree := filepath.Join(tempDir, "child-worktree")
	os.MkdirAll(parentWorktree, 0755)
	os.MkdirAll(childWorktree, 0755)

	escapePath := func(path string) string {
		escaped := strings.ReplaceAll(path, "/", "-")
		return strings.ReplaceAll(escaped, ".", "-")
	}

	homeDir, _ := os.UserHomeDir()
	parentProjectDir := filepath.Join(homeDir, ".claude", "projects", escapePath(parentWorktree))
	childProjectDir := filepath.Join(homeDir, ".claude", "projects", escapePath(childWorktree))

	defer os.RemoveAll(parentProjectDir)
	defer os.RemoveAll(childProjectDir)

	// Create parent's Claude session file
	os.MkdirAll(parentProjectDir, 0700)
	sessionFile := filepath.Join(parentProjectDir, "test-session.jsonl")
	if err := os.WriteFile(sessionFile, []byte("test content\n"), 0600); err != nil {
		t.Fatalf("Failed to create test session file: %v", err)
	}

	// Make the child project directory read-only to force an error
	os.MkdirAll(childProjectDir, 0500) // Read + execute only, no write
	defer os.Chmod(childProjectDir, 0700) // Restore permissions for cleanup

	err := copyClaudeSessionForFork("test-session", parentWorktree, childWorktree)
	if err == nil {
		t.Error("Expected error when destination directory is read-only")
	}

	// Verify no partial file was left behind
	copiedFile := filepath.Join(childProjectDir, "test-session.jsonl")
	if _, err := os.Stat(copiedFile); err == nil {
		t.Error("Expected partial file to be cleaned up on error")
	}
}

func TestCopyClaudeSessionForFork_SuccessPath(t *testing.T) {
	// Test the happy path: successful copy with proper content integrity
	// This verifies that the refactored error handling doesn't break the normal case
	//
	// Note: Testing Close() errors specifically requires filesystem mocking infrastructure
	// that doesn't exist in this codebase. The fix ensures Close() errors are caught and
	// partial files are cleaned up, but we can't easily simulate disk-full scenarios in tests.
	// The code review verified the logic is correct by inspection.

	tempDir := t.TempDir()
	parentWorktree := filepath.Join(tempDir, "parent-worktree")
	childWorktree := filepath.Join(tempDir, "child-worktree")
	os.MkdirAll(parentWorktree, 0755)
	os.MkdirAll(childWorktree, 0755)

	escapePath := func(path string) string {
		escaped := strings.ReplaceAll(path, "/", "-")
		return strings.ReplaceAll(escaped, ".", "-")
	}

	homeDir, _ := os.UserHomeDir()
	parentProjectDir := filepath.Join(homeDir, ".claude", "projects", escapePath(parentWorktree))
	childProjectDir := filepath.Join(homeDir, ".claude", "projects", escapePath(childWorktree))

	defer os.RemoveAll(parentProjectDir)
	defer os.RemoveAll(childProjectDir)

	// Create parent's Claude session file with content
	os.MkdirAll(parentProjectDir, 0700)
	sessionContent := "test content\n"
	sessionFile := filepath.Join(parentProjectDir, "test-session.jsonl")
	if err := os.WriteFile(sessionFile, []byte(sessionContent), 0600); err != nil {
		t.Fatalf("Failed to create test session file: %v", err)
	}

	// Copy should succeed
	err := copyClaudeSessionForFork("test-session", parentWorktree, childWorktree)
	if err != nil {
		t.Fatalf("Expected successful copy, got error: %v", err)
	}

	// Verify the file was copied correctly
	copiedFile := filepath.Join(childProjectDir, "test-session.jsonl")
	copiedContent, err := os.ReadFile(copiedFile)
	if err != nil {
		t.Fatalf("Failed to read copied file: %v", err)
	}

	if string(copiedContent) != sessionContent {
		t.Errorf("Copied content mismatch: got %q, want %q", string(copiedContent), sessionContent)
	}
}

func TestSessionManager_SaveMessages_NoRunner(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	// Should not error when runner doesn't exist
	err := sm.SaveMessages("nonexistent-session")
	if err != nil {
		t.Errorf("Expected nil error when runner doesn't exist, got: %v", err)
	}
}

func TestSessionManager_SaveMessages_Success(t *testing.T) {
	// Set up temp home directory to avoid polluting ~/.plural/ during tests
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	// Create a runner with messages
	initialMsgs := []claude.Message{
		{Role: "user", Content: "Test message"},
		{Role: "assistant", Content: "Test response"},
	}
	runner := claude.New("session-1", "/test", false, initialMsgs)
	sm.runners["session-1"] = runner

	// Should save successfully
	err := sm.SaveMessages("session-1")
	if err != nil {
		t.Fatalf("Expected successful save, got error: %v", err)
	}

	// Verify messages were saved by loading them back
	loadedMsgs, err := config.LoadSessionMessages("session-1")
	if err != nil {
		t.Fatalf("Failed to load saved messages: %v", err)
	}

	if len(loadedMsgs) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(loadedMsgs))
	}
}

func TestSessionManager_SaveMessages_Error(t *testing.T) {
	// Skip if running as root since root can write to read-only directories
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user")
	}
	// Test error propagation by making the sessions directory read-only
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	// Create a runner with messages
	initialMsgs := []claude.Message{
		{Role: "user", Content: "Test message"},
	}
	runner := claude.New("session-1", "/test", false, initialMsgs)
	sm.runners["session-1"] = runner

	// Create the sessions directory and make it read-only
	sessionsDir := filepath.Join(tempHome, ".plural", "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatalf("Failed to create sessions directory: %v", err)
	}
	if err := os.Chmod(sessionsDir, 0444); err != nil {
		t.Fatalf("Failed to make sessions directory read-only: %v", err)
	}
	defer os.Chmod(sessionsDir, 0755) // Restore permissions for cleanup

	// Should return error
	err := sm.SaveMessages("session-1")
	if err == nil {
		t.Error("Expected error when sessions directory is read-only")
	}
}

func TestSessionManager_SaveRunnerMessages_NilRunner(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	// Should not error when runner is nil
	err := sm.SaveRunnerMessages("session-1", nil)
	if err != nil {
		t.Errorf("Expected nil error when runner is nil, got: %v", err)
	}
}

func TestSessionManager_SaveRunnerMessages_Success(t *testing.T) {
	// Set up temp home directory to avoid polluting ~/.plural/ during tests
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	// Create a runner with messages
	initialMsgs := []claude.Message{
		{Role: "user", Content: "Test user message"},
		{Role: "assistant", Content: "Test assistant response"},
	}
	runner := claude.New("session-1", "/test", false, initialMsgs)

	// Should save successfully
	err := sm.SaveRunnerMessages("session-1", runner)
	if err != nil {
		t.Fatalf("Expected successful save, got error: %v", err)
	}

	// Verify messages were saved by loading them back
	loadedMsgs, err := config.LoadSessionMessages("session-1")
	if err != nil {
		t.Fatalf("Failed to load saved messages: %v", err)
	}

	if len(loadedMsgs) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(loadedMsgs))
	}

	if loadedMsgs[0].Role != "user" || loadedMsgs[0].Content != "Test user message" {
		t.Errorf("First message mismatch: got role=%q content=%q", loadedMsgs[0].Role, loadedMsgs[0].Content)
	}

	if loadedMsgs[1].Role != "assistant" || loadedMsgs[1].Content != "Test assistant response" {
		t.Errorf("Second message mismatch: got role=%q content=%q", loadedMsgs[1].Role, loadedMsgs[1].Content)
	}
}

func TestSessionManager_SaveRunnerMessages_Error(t *testing.T) {
	// Skip if running as root since root can write to read-only directories
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user")
	}
	// Test error propagation by making the sessions directory read-only
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	cfg := createTestConfig()
	sm := NewSessionManager(cfg, git.NewGitService())

	// Create a runner with messages
	initialMsgs := []claude.Message{
		{Role: "user", Content: "Test message"},
	}
	runner := claude.New("session-1", "/test", false, initialMsgs)

	// Create the sessions directory and make it read-only
	sessionsDir := filepath.Join(tempHome, ".plural", "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatalf("Failed to create sessions directory: %v", err)
	}
	if err := os.Chmod(sessionsDir, 0444); err != nil {
		t.Fatalf("Failed to make sessions directory read-only: %v", err)
	}
	defer os.Chmod(sessionsDir, 0755) // Restore permissions for cleanup

	// Should return error
	err := sm.SaveRunnerMessages("session-1", runner)
	if err == nil {
		t.Error("Expected error when sessions directory is read-only")
	}
}

