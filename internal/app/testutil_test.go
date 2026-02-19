package app

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural-core/claude"
	"github.com/zhubert/plural-core/config"
	"github.com/zhubert/plural-core/git"
	"github.com/zhubert/plural/internal/keys"
	"github.com/zhubert/plural-core/mcp"
)

// testConfig creates a minimal config for testing.
func testConfig() *config.Config {
	return &config.Config{
		Repos:            []string{"/test/repo1", "/test/repo2"},
		Sessions:         []config.Session{},
		AllowedTools:     []string{},
		RepoAllowedTools: make(map[string][]string),
		MCPServers:       []config.MCPServer{},
		RepoMCP:          make(map[string][]config.MCPServer),
		WelcomeShown:     true, // Skip welcome modal in tests
	}
}

// testConfigWithSessions creates a config with test sessions.
func testConfigWithSessions() *config.Config {
	cfg := testConfig()
	cfg.Sessions = []config.Session{
		{
			ID:        "session-1",
			RepoPath:  "/test/repo1",
			WorkTree:  "/test/worktree1",
			Branch:    "feature-branch",
			Name:      "repo1/session1",
			CreatedAt: time.Now(),
			Started:   true,
		},
		{
			ID:        "session-2",
			RepoPath:  "/test/repo1",
			WorkTree:  "/test/worktree2",
			Branch:    "plural-abc123",
			Name:      "repo1/abc123",
			CreatedAt: time.Now(),
			Started:   false,
		},
		{
			ID:        "session-3",
			RepoPath:  "/test/repo2",
			WorkTree:  "/test/worktree3",
			Branch:    "bugfix",
			Name:      "repo2/bugfix",
			CreatedAt: time.Now(),
			Started:   true,
		},
	}
	return cfg
}

// testModel creates a test Model with the given config.
func testModel(cfg *config.Config) *Model {
	return New(cfg, "0.0.0-test")
}

// testModelWithSize creates a test Model and sets its size.
func testModelWithSize(cfg *config.Config, width, height int) *Model {
	m := testModel(cfg)
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

// keyPress creates a tea.KeyPressMsg for the given key string.
// Examples: "a", "enter", "tab", "esc", "ctrl+c", "up", "down"
func keyPress(key string) tea.KeyPressMsg {
	switch key {
	case keys.Enter:
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case keys.Tab:
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case keys.Escape:
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case keys.Backspace:
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case keys.Up:
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case keys.Down:
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case keys.Left:
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case keys.Right:
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case keys.Home:
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case keys.End:
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case keys.PgUp:
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case keys.PgDown:
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case keys.Space:
		return tea.KeyPressMsg{Code: tea.KeySpace}
	case keys.CtrlC:
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case keys.CtrlV:
		return tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl}
	case keys.CtrlS:
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	case keys.ShiftTab:
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case keys.AltComma:
		return tea.KeyPressMsg{Code: ',', Mod: tea.ModAlt}
	default:
		// Regular character - for single characters, set both Code and Text
		if len(key) == 1 {
			return tea.KeyPressMsg{Code: rune(key[0]), Text: key}
		}
		// Fallback for unknown keys
		return tea.KeyPressMsg{Text: key}
	}
}

// sendKey sends a key press to the model and returns the updated model.
func sendKey(m *Model, key string) *Model {
	result, _ := m.Update(keyPress(key))
	return result.(*Model)
}

// typeText simulates typing a string by sending individual character key presses.
func typeText(m *Model, text string) *Model {
	for _, ch := range text {
		m = sendKey(m, string(ch))
	}
	return m
}

// setSize sends a window size message to the model.
func setSize(m *Model, width, height int) *Model {
	result, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return result.(*Model)
}

// =============================================================================
// Mock Runner Support
// =============================================================================

// testRunnerFactory creates mock runners and tracks them for test assertions.
type testRunnerFactory struct {
	runners map[string]*claude.MockRunner
}

// newTestRunnerFactory creates a new test runner factory.
func newTestRunnerFactory() *testRunnerFactory {
	return &testRunnerFactory{
		runners: make(map[string]*claude.MockRunner),
	}
}

// Create implements the RunnerFactory signature for creating mock runners.
func (f *testRunnerFactory) Create(sessionID, workingDir, repoPath string, started bool, msgs []claude.Message) claude.RunnerInterface {
	mock := claude.NewMockRunner(sessionID, started, msgs)
	f.runners[sessionID] = mock
	return mock
}

// GetMock returns the mock runner for a session, or nil if not found.
func (f *testRunnerFactory) GetMock(sessionID string) *claude.MockRunner {
	return f.runners[sessionID]
}

// testModelWithMocks creates a test model with mock runner injection.
// Returns both the model and the factory so tests can access mock runners.
func testModelWithMocks(cfg *config.Config, width, height int) (*Model, *testRunnerFactory) {
	m := New(cfg, "0.0.0-test")
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})

	factory := newTestRunnerFactory()
	m.sessionMgr.SetRunnerFactory(factory.Create)

	return m, factory
}

// =============================================================================
// Response Simulation Helpers
// =============================================================================

// simulateClaudeResponse injects a ClaudeResponseMsg into the model.
// This bypasses channels and directly tests the message handler.
func simulateClaudeResponse(m *Model, sessionID string, chunk claude.ResponseChunk) *Model {
	msg := ClaudeResponseMsg{
		SessionID: sessionID,
		Chunk:     chunk,
	}
	result, _ := m.Update(msg)
	return result.(*Model)
}

// simulatePermissionRequest injects a PermissionRequestMsg into the model.
func simulatePermissionRequest(m *Model, sessionID, tool, description string) *Model {
	msg := PermissionRequestMsg{
		SessionID: sessionID,
		Request: mcp.PermissionRequest{
			Tool:        tool,
			Description: description,
		},
	}
	result, _ := m.Update(msg)
	return result.(*Model)
}

// simulateQuestionRequest injects a QuestionRequestMsg into the model.
func simulateQuestionRequest(m *Model, sessionID string, questions []mcp.Question) *Model {
	msg := QuestionRequestMsg{
		SessionID: sessionID,
		Request: mcp.QuestionRequest{
			Questions: questions,
		},
	}
	result, _ := m.Update(msg)
	return result.(*Model)
}

// simulatePlanApprovalRequest injects a PlanApprovalRequestMsg into the model.
func simulatePlanApprovalRequest(m *Model, sessionID string, plan string, allowedPrompts []mcp.AllowedPrompt) *Model {
	msg := PlanApprovalRequestMsg{
		SessionID: sessionID,
		Request: mcp.PlanApprovalRequest{
			Plan:           plan,
			AllowedPrompts: allowedPrompts,
		},
	}
	result, _ := m.Update(msg)
	return result.(*Model)
}

// =============================================================================
// Response Building Helpers
// =============================================================================

// textChunk creates a text response chunk.
func textChunk(content string) claude.ResponseChunk {
	return claude.ResponseChunk{
		Type:    claude.ChunkTypeText,
		Content: content,
	}
}

// toolChunk creates a tool use response chunk.
func toolChunk(toolName, toolInput string) claude.ResponseChunk {
	return claude.ResponseChunk{
		Type:      claude.ChunkTypeToolUse,
		ToolName:  toolName,
		ToolInput: toolInput,
	}
}

// doneChunk creates a done response chunk.
func doneChunk() claude.ResponseChunk {
	return claude.ResponseChunk{
		Done: true,
	}
}

// errorChunk creates an error response chunk.
func errorChunk(err error) claude.ResponseChunk {
	return claude.ResponseChunk{
		Error: err,
		Done:  true,
	}
}

// =============================================================================
// Git Flow Test Helpers
// =============================================================================

// testConfigWithParentChild creates a config with a parent session and a child session.
func testConfigWithParentChild() *config.Config {
	cfg := testConfig()
	cfg.Sessions = []config.Session{
		{
			ID:        "parent-session",
			RepoPath:  "/test/repo1",
			WorkTree:  "/test/worktree-parent",
			Branch:    "feature-parent",
			Name:      "repo1/parent",
			CreatedAt: time.Now(),
			Started:   true,
		},
		{
			ID:        "child-session",
			RepoPath:  "/test/repo1",
			WorkTree:  "/test/worktree-child",
			Branch:    "feature-child",
			Name:      "repo1/child",
			CreatedAt: time.Now(),
			Started:   true,
			ParentID:  "parent-session",
		},
		{
			ID:        "session-with-pr",
			RepoPath:  "/test/repo1",
			WorkTree:  "/test/worktree-pr",
			Branch:    "pr-branch",
			Name:      "repo1/pr-session",
			CreatedAt: time.Now(),
			Started:   true,
			PRCreated: true,
		},
	}
	return cfg
}

// simulateMergeResult injects a MergeResultMsg into the model.
// This bypasses channels and directly tests the message handler.
func simulateMergeResult(m *Model, sessionID string, output string, err error, done bool, conflictedFiles []string, repoPath string) *Model {
	msg := MergeResultMsg{
		SessionID: sessionID,
		Result: git.Result{
			Output:          output,
			Error:           err,
			Done:            done,
			ConflictedFiles: conflictedFiles,
			RepoPath:        repoPath,
		},
	}
	result, _ := m.Update(msg)
	return result.(*Model)
}

// testFileDiffs creates test file diff data for view changes mode.
func testFileDiffs() []git.FileDiff {
	return []git.FileDiff{
		{
			Filename: "main.go",
			Status:   "M",
			Diff:     "diff --git a/main.go b/main.go\n@@ -1,3 +1,4 @@\n package main\n+// Added comment",
		},
		{
			Filename: "README.md",
			Status:   "A",
			Diff:     "diff --git a/README.md b/README.md\n@@ -0,0 +1,3 @@\n+# New Project\n+\n+Description here",
		},
		{
			Filename: "old.txt",
			Status:   "D",
			Diff:     "diff --git a/old.txt b/old.txt\ndeleted file mode 100644",
		},
	}
}

// =============================================================================
// Mouse Event Helpers
// =============================================================================

// mouseClick creates a tea.MouseClickMsg at the given coordinates.
func mouseClick(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{
		X:      x,
		Y:      y,
		Button: tea.MouseLeft,
	}
}

// mouseMotion creates a tea.MouseMotionMsg at the given coordinates.
func mouseMotion(x, y int) tea.MouseMotionMsg {
	return tea.MouseMotionMsg{
		X:      x,
		Y:      y,
		Button: tea.MouseLeft,
	}
}

// mouseRelease creates a tea.MouseReleaseMsg at the given coordinates.
func mouseRelease(x, y int) tea.MouseReleaseMsg {
	return tea.MouseReleaseMsg{
		X:      x,
		Y:      y,
		Button: tea.MouseLeft,
	}
}
