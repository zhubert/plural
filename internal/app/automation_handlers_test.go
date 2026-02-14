package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
)

// =============================================================================
// TestHandleSessionCompletedMsg
// =============================================================================

func TestHandleSessionCompletedMsg(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func() *config.Config
		sessionID   string
		wantCmd     bool
		description string
	}{
		{
			name: "with test command triggers test run",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.SetRepoTestCommand("/test/repo1", "go test ./...")
				return cfg
			},
			sessionID:   "session-1",
			wantCmd:     true,
			description: "should return a command to run tests",
		},
		{
			name: "without test command emits pipeline complete",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				return cfg
			},
			sessionID:   "session-1",
			wantCmd:     true,
			description: "should return a command that produces SessionPipelineCompleteMsg",
		},
		{
			name: "nil session returns nil cmd",
			setupConfig: func() *config.Config {
				return testConfigWithSessions()
			},
			sessionID:   "nonexistent-session",
			wantCmd:     false,
			description: "should return nil when session not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setupConfig()
			m, _ := testModelWithMocks(cfg, 120, 40)
			m.sidebar.SetSessions(cfg.Sessions)

			msg := SessionCompletedMsg{SessionID: tt.sessionID}
			_, cmd := m.handleSessionCompletedMsg(msg)

			if tt.wantCmd && cmd == nil {
				t.Errorf("%s: expected non-nil cmd", tt.description)
			}
			if !tt.wantCmd && cmd != nil {
				t.Errorf("%s: expected nil cmd", tt.description)
			}
		})
	}
}

func TestHandleSessionCompletedMsg_PipelineCompleteMsgContent(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	// No test command configured
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	msg := SessionCompletedMsg{SessionID: "session-1"}
	_, cmd := m.handleSessionCompletedMsg(msg)

	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	// Execute the command to get the message
	resultMsg := cmd()
	pipelineMsg, ok := resultMsg.(SessionPipelineCompleteMsg)
	if !ok {
		t.Fatalf("expected SessionPipelineCompleteMsg, got %T", resultMsg)
	}
	if pipelineMsg.SessionID != "session-1" {
		t.Errorf("expected session ID 'session-1', got %q", pipelineMsg.SessionID)
	}
	if !pipelineMsg.TestsPassed {
		t.Error("expected TestsPassed=true when no test command configured")
	}
}

// =============================================================================
// TestHandleTestRunResultMsg
// =============================================================================

func TestHandleTestRunResultMsg(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func() *config.Config
		msg         TestRunResultMsg
		wantCmd     bool
		wantMsgType string // "pipeline_complete", "send_pending", or ""
	}{
		{
			name: "exit code 0 emits pipeline complete with tests passed",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				return cfg
			},
			msg: TestRunResultMsg{
				SessionID: "session-1",
				ExitCode:  0,
				Iteration: 1,
				Output:    "PASS",
			},
			wantCmd:     true,
			wantMsgType: "pipeline_complete",
		},
		{
			name: "failed tests under max retries queues pending message",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.SetRepoTestMaxRetries("/test/repo1", 3)
				return cfg
			},
			msg: TestRunResultMsg{
				SessionID: "session-1",
				ExitCode:  1,
				Iteration: 1,
				Output:    "FAIL: some test",
			},
			wantCmd:     true,
			wantMsgType: "send_pending",
		},
		{
			name: "failed tests at max retries emits pipeline complete with tests failed",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.SetRepoTestMaxRetries("/test/repo1", 3)
				return cfg
			},
			msg: TestRunResultMsg{
				SessionID: "session-1",
				ExitCode:  1,
				Iteration: 3,
				Output:    "FAIL: some test",
			},
			wantCmd:     true,
			wantMsgType: "pipeline_complete",
		},
		{
			name: "non-autonomous session does not auto retry",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				// session-1 is not autonomous by default
				cfg.SetRepoTestMaxRetries("/test/repo1", 3)
				return cfg
			},
			msg: TestRunResultMsg{
				SessionID: "session-1",
				ExitCode:  1,
				Iteration: 1,
				Output:    "FAIL: some test",
			},
			wantCmd:     false,
			wantMsgType: "",
		},
		{
			name: "nil session returns nil",
			setupConfig: func() *config.Config {
				return testConfigWithSessions()
			},
			msg: TestRunResultMsg{
				SessionID: "nonexistent",
				ExitCode:  1,
				Iteration: 1,
			},
			wantCmd: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setupConfig()
			m, _ := testModelWithMocks(cfg, 120, 40)
			m.sidebar.SetSessions(cfg.Sessions)

			_, cmd := m.handleTestRunResultMsg(tt.msg)

			if tt.wantCmd && cmd == nil {
				t.Error("expected non-nil cmd")
			}
			if !tt.wantCmd && cmd != nil {
				t.Error("expected nil cmd")
			}

			if cmd != nil && tt.wantMsgType != "" {
				resultMsg := cmd()
				switch tt.wantMsgType {
				case "pipeline_complete":
					if _, ok := resultMsg.(SessionPipelineCompleteMsg); !ok {
						t.Errorf("expected SessionPipelineCompleteMsg, got %T", resultMsg)
					}
				case "send_pending":
					if _, ok := resultMsg.(SendPendingMessageMsg); !ok {
						t.Errorf("expected SendPendingMessageMsg, got %T", resultMsg)
					}
				}
			}
		})
	}
}

func TestHandleTestRunResultMsg_PassedTestsPipelineMsg(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	msg := TestRunResultMsg{
		SessionID: "session-1",
		ExitCode:  0,
		Iteration: 2,
		Output:    "ok  all tests passed",
	}
	_, cmd := m.handleTestRunResultMsg(msg)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	resultMsg := cmd().(SessionPipelineCompleteMsg)
	if !resultMsg.TestsPassed {
		t.Error("expected TestsPassed=true")
	}
	if resultMsg.SessionID != "session-1" {
		t.Errorf("expected session ID 'session-1', got %q", resultMsg.SessionID)
	}
}

func TestHandleTestRunResultMsg_MaxRetriesExhaustedPipelineMsg(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	cfg.SetRepoTestMaxRetries("/test/repo1", 2)
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	msg := TestRunResultMsg{
		SessionID: "session-1",
		ExitCode:  1,
		Iteration: 2,
		Output:    "FAIL",
	}
	_, cmd := m.handleTestRunResultMsg(msg)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	resultMsg := cmd().(SessionPipelineCompleteMsg)
	if resultMsg.TestsPassed {
		t.Error("expected TestsPassed=false")
	}
}

func TestHandleTestRunResultMsg_RetryQueuesPendingMsg(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	cfg.SetRepoTestMaxRetries("/test/repo1", 3)
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	msg := TestRunResultMsg{
		SessionID: "session-1",
		ExitCode:  1,
		Iteration: 1,
		Output:    "FAIL: TestSomething",
	}
	_, cmd := m.handleTestRunResultMsg(msg)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	// Verify pending message was queued
	state := m.sessionState().GetIfExists("session-1")
	if state == nil {
		t.Fatal("expected session state")
	}
	pendingMsg := state.GetPendingMsg()
	if pendingMsg == "" {
		t.Error("expected pending message to be set")
	}
}

// =============================================================================
// TestHandleSessionPipelineCompleteMsg
// =============================================================================

func TestHandleSessionPipelineCompleteMsg(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func() *config.Config
		msg         SessionPipelineCompleteMsg
		wantCmd     bool
	}{
		{
			name: "nil session returns nil",
			setupConfig: func() *config.Config {
				return testConfigWithSessions()
			},
			msg:     SessionPipelineCompleteMsg{SessionID: "nonexistent", TestsPassed: true},
			wantCmd: false,
		},
		{
			name: "non-autonomous session with no special flags returns nil",
			setupConfig: func() *config.Config {
				return testConfigWithSessions()
			},
			msg:     SessionPipelineCompleteMsg{SessionID: "session-1", TestsPassed: true},
			wantCmd: false,
		},
		{
			name: "standalone autonomous session with no PR triggers auto PR creation",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.Sessions[0].PRCreated = false
				return cfg
			},
			msg:     SessionPipelineCompleteMsg{SessionID: "session-1", TestsPassed: true},
			wantCmd: true,
		},
		{
			name: "autonomous session with tests failed does not trigger PR",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.Sessions[0].PRCreated = false
				return cfg
			},
			msg:     SessionPipelineCompleteMsg{SessionID: "session-1", TestsPassed: false},
			wantCmd: false,
		},
		{
			name: "autonomous session with PR already created does not re-create",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.Sessions[0].PRCreated = true
				return cfg
			},
			msg:     SessionPipelineCompleteMsg{SessionID: "session-1", TestsPassed: true},
			wantCmd: false,
		},
		{
			name: "supervisor session does not auto-create PR",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.Sessions[0].IsSupervisor = true
				return cfg
			},
			msg:     SessionPipelineCompleteMsg{SessionID: "session-1", TestsPassed: true},
			wantCmd: false,
		},
		{
			name: "child session does not auto-create PR",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.Sessions[0].SupervisorID = "session-3"
				return cfg
			},
			msg:     SessionPipelineCompleteMsg{SessionID: "session-1", TestsPassed: true},
			wantCmd: true, // returns cmd for supervisor notification (Phase 5B)
		},
		{
			name: "session with supervisor ID triggers notification",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].SupervisorID = "session-3"
				return cfg
			},
			msg:     SessionPipelineCompleteMsg{SessionID: "session-1", TestsPassed: true},
			wantCmd: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setupConfig()
			m, _ := testModelWithMocks(cfg, 120, 40)
			m.sidebar.SetSessions(cfg.Sessions)

			_, cmd := m.handleSessionPipelineCompleteMsg(tt.msg)

			if tt.wantCmd && cmd == nil {
				t.Error("expected non-nil cmd")
			}
			if !tt.wantCmd && cmd != nil {
				t.Error("expected nil cmd")
			}
		})
	}
}

// =============================================================================
// TestHandleAutonomousLimitReachedMsg
// =============================================================================

func TestHandleAutonomousLimitReachedMsg(t *testing.T) {
	tests := []struct {
		name     string
		reason   string
		wantText string
	}{
		{
			name:     "turn limit reason",
			reason:   "turn_limit",
			wantText: "AUTONOMOUS LIMIT",
		},
		{
			name:     "duration limit reason",
			reason:   "duration_limit",
			wantText: "AUTONOMOUS LIMIT",
		},
		{
			name:     "unknown reason",
			reason:   "custom_reason",
			wantText: "custom_reason",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfigWithSessions()
			cfg.Sessions[0].Autonomous = true
			m, _ := testModelWithMocks(cfg, 120, 40)
			m.sidebar.SetSessions(cfg.Sessions)

			msg := AutonomousLimitReachedMsg{
				SessionID: "session-1",
				Reason:    tt.reason,
			}
			_, cmd := m.handleAutonomousLimitReachedMsg(msg)

			// Should always return a flash warning command
			if cmd == nil {
				t.Error("expected non-nil cmd (flash warning)")
			}

			// Verify autonomous mode was disabled
			sess := m.config.GetSession("session-1")
			if sess != nil && sess.Autonomous {
				t.Error("expected autonomous mode to be disabled")
			}
		})
	}
}

func TestHandleAutonomousLimitReachedMsg_NilSession(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)

	msg := AutonomousLimitReachedMsg{
		SessionID: "nonexistent",
		Reason:    "turn_limit",
	}
	_, cmd := m.handleAutonomousLimitReachedMsg(msg)
	if cmd != nil {
		t.Error("expected nil cmd for nonexistent session")
	}
}

func TestHandleAutonomousLimitReachedMsg_ActiveSession(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select session-1 to make it active
	m = sendKey(m, "enter")
	if m.activeSession == nil || m.activeSession.ID != "session-1" {
		t.Fatal("expected session-1 to be active")
	}

	msg := AutonomousLimitReachedMsg{
		SessionID: "session-1",
		Reason:    "turn_limit",
	}
	_, cmd := m.handleAutonomousLimitReachedMsg(msg)
	if cmd == nil {
		t.Error("expected non-nil cmd")
	}
}

// =============================================================================
// TestAllBroadcastSessionsComplete
// =============================================================================

func TestAllBroadcastSessionsComplete(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Model, *config.Config)
		groupID string
		want    bool
	}{
		{
			name: "no sessions in group returns true",
			setup: func(m *Model, cfg *config.Config) {
				// No sessions have BroadcastGroupID set
			},
			groupID: "group-1",
			want:    true,
		},
		{
			name: "all sessions idle returns true",
			setup: func(m *Model, cfg *config.Config) {
				cfg.SetSessionBroadcastGroup("session-1", "group-1")
				cfg.SetSessionBroadcastGroup("session-3", "group-1")
			},
			groupID: "group-1",
			want:    true,
		},
		{
			name: "session waiting returns false",
			setup: func(m *Model, cfg *config.Config) {
				cfg.SetSessionBroadcastGroup("session-1", "group-1")
				cfg.SetSessionBroadcastGroup("session-3", "group-1")
				m.sessionState().GetOrCreate("session-1").IsWaiting = true
			},
			groupID: "group-1",
			want:    false,
		},
		{
			name: "session merging returns false",
			setup: func(m *Model, cfg *config.Config) {
				cfg.SetSessionBroadcastGroup("session-1", "group-1")
				ch := make(chan git.Result)
				m.sessionState().StartMerge("session-1", ch, func() {}, MergeTypePR)
			},
			groupID: "group-1",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfigWithSessions()
			m, _ := testModelWithMocks(cfg, 120, 40)
			m.sidebar.SetSessions(cfg.Sessions)
			tt.setup(m, cfg)

			got := m.allBroadcastSessionsComplete(tt.groupID)
			if got != tt.want {
				t.Errorf("allBroadcastSessionsComplete(%q) = %v, want %v", tt.groupID, got, tt.want)
			}
		})
	}
}

// =============================================================================
// TestFormatPRCommentsPrompt
// =============================================================================

func TestFormatPRCommentsPrompt(t *testing.T) {
	tests := []struct {
		name         string
		comments     []git.PRReviewComment
		wantContains []string
	}{
		{
			name: "single comment with all fields",
			comments: []git.PRReviewComment{
				{
					Author: "reviewer1",
					Body:   "Please fix this bug",
					Path:   "main.go",
					Line:   42,
				},
			},
			wantContains: []string{
				"1 comment(s)",
				"@reviewer1",
				"Please fix this bug",
				"main.go:42",
				"Comment 1",
			},
		},
		{
			name: "comment without path or line",
			comments: []git.PRReviewComment{
				{
					Author: "reviewer2",
					Body:   "Looks good overall",
				},
			},
			wantContains: []string{
				"@reviewer2",
				"Looks good overall",
			},
		},
		{
			name: "comment with path but no line",
			comments: []git.PRReviewComment{
				{
					Author: "reviewer3",
					Body:   "This file needs refactoring",
					Path:   "utils.go",
				},
			},
			wantContains: []string{
				"File: utils.go",
			},
		},
		{
			name: "comment without author",
			comments: []git.PRReviewComment{
				{
					Body: "Anonymous comment",
					Path: "test.go",
					Line: 10,
				},
			},
			wantContains: []string{
				"Comment 1 ---",
				"Anonymous comment",
				"test.go:10",
			},
		},
		{
			name: "multiple comments",
			comments: []git.PRReviewComment{
				{Author: "alice", Body: "Fix typo"},
				{Author: "bob", Body: "Add tests"},
				{Author: "charlie", Body: "Update docs"},
			},
			wantContains: []string{
				"3 comment(s)",
				"Comment 1",
				"Comment 2",
				"Comment 3",
				"@alice",
				"@bob",
				"@charlie",
				"Please address each of these review comments",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatPRCommentsPrompt(tt.comments)

			for _, want := range tt.wantContains {
				if !containsStr(result, want) {
					t.Errorf("expected result to contain %q, got:\n%s", want, result)
				}
			}
		})
	}
}

// =============================================================================
// TestHandleAutoPRCommentsFetchedMsg
// =============================================================================

func TestHandleAutoPRCommentsFetchedMsg(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		prompt    string
		wantCmd   bool
	}{
		{
			name:      "valid session queues pending message",
			sessionID: "session-1",
			prompt:    "Please fix the review comments",
			wantCmd:   true,
		},
		{
			name:      "nonexistent session returns nil",
			sessionID: "nonexistent",
			prompt:    "Some prompt",
			wantCmd:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfigWithSessions()
			m, _ := testModelWithMocks(cfg, 120, 40)
			m.sidebar.SetSessions(cfg.Sessions)

			msg := AutoPRCommentsFetchedMsg{
				SessionID: tt.sessionID,
				Prompt:    tt.prompt,
			}
			_, cmd := m.handleAutoPRCommentsFetchedMsg(msg)

			if tt.wantCmd && cmd == nil {
				t.Error("expected non-nil cmd")
			}
			if !tt.wantCmd && cmd != nil {
				t.Error("expected nil cmd")
			}
		})
	}
}

func TestHandleAutoPRCommentsFetchedMsg_SetsPendingMsg(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	prompt := "Please address the review comments about error handling"
	msg := AutoPRCommentsFetchedMsg{
		SessionID: "session-1",
		Prompt:    prompt,
	}
	_, cmd := m.handleAutoPRCommentsFetchedMsg(msg)

	// Verify pending message was set
	state := m.sessionState().GetIfExists("session-1")
	if state == nil {
		t.Fatal("expected session state")
	}
	if state.GetPendingMsg() != prompt {
		t.Errorf("expected pending msg %q, got %q", prompt, state.GetPendingMsg())
	}

	// Verify the command produces a SendPendingMessageMsg
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	resultMsg := cmd()
	sendMsg, ok := resultMsg.(SendPendingMessageMsg)
	if !ok {
		t.Fatalf("expected SendPendingMessageMsg, got %T", resultMsg)
	}
	if sendMsg.SessionID != "session-1" {
		t.Errorf("expected session ID 'session-1', got %q", sendMsg.SessionID)
	}
}

func TestHandleAutoPRCommentsFetchedMsg_ActiveSession(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select session-1 to make it active
	m = sendKey(m, "enter")
	if m.activeSession == nil || m.activeSession.ID != "session-1" {
		t.Fatal("expected session-1 to be active")
	}

	msg := AutoPRCommentsFetchedMsg{
		SessionID: "session-1",
		Prompt:    "Fix these comments",
	}
	_, cmd := m.handleAutoPRCommentsFetchedMsg(msg)
	if cmd == nil {
		t.Error("expected non-nil cmd for active session")
	}
}

// =============================================================================
// Helpers
// =============================================================================

// containsStr is a simple helper to check substring presence without importing strings.
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// testConfigWithAutonomousSession creates a config with an autonomous session for testing.
func testConfigWithAutonomousSession() *config.Config {
	cfg := testConfig()
	cfg.Sessions = []config.Session{
		{
			ID:         "auto-session",
			RepoPath:   "/test/repo1",
			WorkTree:   "/test/worktree-auto",
			Branch:     "auto-branch",
			Name:       "repo1/auto",
			CreatedAt:  time.Now(),
			Started:    true,
			Autonomous: true,
		},
	}
	return cfg
}

// verifyCmd is a helper to check that a tea.Cmd produces a specific message type.
func verifyCmd(t *testing.T, cmd tea.Cmd, wantType string) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	switch wantType {
	case "SessionPipelineCompleteMsg":
		if _, ok := msg.(SessionPipelineCompleteMsg); !ok {
			t.Errorf("expected SessionPipelineCompleteMsg, got %T", msg)
		}
	case "SendPendingMessageMsg":
		if _, ok := msg.(SendPendingMessageMsg); !ok {
			t.Errorf("expected SendPendingMessageMsg, got %T", msg)
		}
	}
}
