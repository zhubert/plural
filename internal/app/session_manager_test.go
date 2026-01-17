package app

import (
	"testing"
	"time"

	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
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
	sm := NewSessionManager(cfg)

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
	sm := NewSessionManager(cfg)

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
	sm := NewSessionManager(cfg)

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
	sm := NewSessionManager(cfg)

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
	sm := NewSessionManager(cfg)

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
	sm := NewSessionManager(cfg)

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
	sm := NewSessionManager(cfg)

	result := sm.Select(nil, "", "", "")
	if result != nil {
		t.Error("Select(nil) should return nil")
	}
}

func TestSessionManager_Select_SavesPreviousState(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg)

	// Select a session with previous state to save
	sess := sm.GetSession("session-1")
	sm.Select(sess, "prev-session", "saved input", "saved streaming")

	// Verify state was saved
	savedInput := sm.stateManager.GetInput("prev-session")
	if savedInput != "saved input" {
		t.Errorf("Expected saved input 'saved input', got %q", savedInput)
	}

	savedStreaming := sm.stateManager.GetStreaming("prev-session")
	if savedStreaming != "saved streaming" {
		t.Errorf("Expected saved streaming 'saved streaming', got %q", savedStreaming)
	}
}

func TestSessionManager_Select_CreatesRunner(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg)

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
	sm := NewSessionManager(cfg)

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
	sm := NewSessionManager(cfg)

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
	sm := NewSessionManager(cfg)

	sess := sm.GetSession("session-1")

	// Set up state to restore
	sm.stateManager.StartWaiting(sess.ID, nil)
	permReq := &mcp.PermissionRequest{Tool: "Bash", Description: "test"}
	sm.stateManager.SetPendingPermission(sess.ID, permReq)
	sm.stateManager.SaveStreaming(sess.ID, "streaming content")
	sm.stateManager.SaveInput(sess.ID, "saved input text")

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
	sm := NewSessionManager(cfg)

	// Create runner and state
	runner := claude.New("session-1", "/test", false, nil)
	sm.runners["session-1"] = runner
	sm.stateManager.SaveInput("session-1", "test input")

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
	if sm.stateManager.GetInput("session-1") != "" {
		t.Error("State should be cleaned up after delete")
	}
}

func TestSessionManager_DeleteSession_NoRunner(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg)

	// Delete session with no runner
	deletedRunner := sm.DeleteSession("session-1")

	if deletedRunner != nil {
		t.Error("DeleteSession should return nil when no runner exists")
	}
}

func TestSessionManager_SetRunner(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg)

	runner := claude.New("session-1", "/test", false, nil)
	sm.SetRunner("session-1", runner)

	if sm.GetRunner("session-1") != runner {
		t.Error("SetRunner should set the runner")
	}
}

func TestSessionManager_AddAllowedTool(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg)

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
	sm := NewSessionManager(cfg)

	// Should not panic with non-existent session
	sm.AddAllowedTool("nonexistent", "Bash(git:*)")
}

func TestSessionManager_AddAllowedTool_NoRunner(t *testing.T) {
	cfg := createTestConfig()
	sm := NewSessionManager(cfg)

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
	// Create config with a forked child session
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
				Started:  false, // Not started yet
				ParentID: "parent-session",
			},
		},
	}
	sm := NewSessionManager(cfg)

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
	sm := NewSessionManager(cfg)

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
	sm := NewSessionManager(cfg)

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
	sm := NewSessionManager(cfg)

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
	sm := NewSessionManager(cfg)

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

