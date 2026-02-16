package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/mcp"
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
			name: "emits pipeline complete",
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
				m.sessionState().StartWaiting("session-1", func() {})
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
// TestHandleAutoMergePollResultMsg
// =============================================================================

func TestHandleAutoMergePollResultMsg(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func() *config.Config
		msg         AutoMergePollResultMsg
		wantCmd     bool
		description string
	}{
		{
			name: "nil session returns nil",
			setupConfig: func() *config.Config {
				return testConfigWithSessions()
			},
			msg:         AutoMergePollResultMsg{SessionID: "nonexistent"},
			wantCmd:     false,
			description: "should return nil for nonexistent session",
		},
		{
			name: "unaddressed comments triggers addressing (step 1)",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.Sessions[0].PRCreated = true
				cfg.Sessions[0].PRCommentsAddressedCount = 0
				return cfg
			},
			msg: AutoMergePollResultMsg{
				SessionID:      "session-1",
				ReviewDecision: git.ReviewChangesRequested,
				CommentCount:   3,
				CIStatus:       git.CIStatusPassing,
			},
			wantCmd:     true,
			description: "comments take priority over review state and CI",
		},
		{
			name: "changes requested with no new comments waits (step 2)",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.Sessions[0].PRCreated = true
				cfg.Sessions[0].PRCommentsAddressedCount = 3
				return cfg
			},
			msg: AutoMergePollResultMsg{
				SessionID:      "session-1",
				ReviewDecision: git.ReviewChangesRequested,
				CommentCount:   3,
				CIStatus:       git.CIStatusPassing,
				Attempt:        1,
			},
			wantCmd:     true,
			description: "should continue polling while changes are requested",
		},
		{
			name: "review required waits (step 2)",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.Sessions[0].PRCreated = true
				return cfg
			},
			msg: AutoMergePollResultMsg{
				SessionID:      "session-1",
				ReviewDecision: git.ReviewRequired,
				CIStatus:       git.CIStatusPassing,
				Attempt:        1,
			},
			wantCmd:     true,
			description: "should continue polling while waiting for review",
		},
		{
			name: "approved and CI passing merges (step 3+4)",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.Sessions[0].PRCreated = true
				return cfg
			},
			msg: AutoMergePollResultMsg{
				SessionID:      "session-1",
				ReviewDecision: git.ReviewApproved,
				CommentCount:   0,
				CIStatus:       git.CIStatusPassing,
			},
			wantCmd:     true,
			description: "should trigger merge",
		},
		{
			name: "no review yet waits for review (step 2)",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.Sessions[0].PRCreated = true
				return cfg
			},
			msg: AutoMergePollResultMsg{
				SessionID:      "session-1",
				ReviewDecision: git.ReviewNone,
				CommentCount:   0,
				CIStatus:       git.CIStatusPassing,
				Attempt:        1,
			},
			wantCmd:     true,
			description: "should wait for review when ReviewNone (no review submitted yet)",
		},
		{
			name: "approved but CI failing stops",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.Sessions[0].PRCreated = true
				return cfg
			},
			msg: AutoMergePollResultMsg{
				SessionID:      "session-1",
				ReviewDecision: git.ReviewApproved,
				CIStatus:       git.CIStatusFailing,
			},
			wantCmd:     false,
			description: "should stop when CI fails (no retry cmd)",
		},
		{
			name: "approved but CI pending continues polling",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.Sessions[0].PRCreated = true
				return cfg
			},
			msg: AutoMergePollResultMsg{
				SessionID:      "session-1",
				ReviewDecision: git.ReviewApproved,
				CIStatus:       git.CIStatusPending,
				Attempt:        1,
			},
			wantCmd:     true,
			description: "should continue polling while CI is pending",
		},
		{
			name: "approved and no CI configured merges",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.Sessions[0].PRCreated = true
				return cfg
			},
			msg: AutoMergePollResultMsg{
				SessionID:      "session-1",
				ReviewDecision: git.ReviewApproved,
				CIStatus:       git.CIStatusNone,
			},
			wantCmd:     true,
			description: "should merge when approved and no CI checks",
		},
		{
			name: "max attempts waiting for review gives up",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.Sessions[0].PRCreated = true
				return cfg
			},
			msg: AutoMergePollResultMsg{
				SessionID:      "session-1",
				ReviewDecision: git.ReviewRequired,
				CIStatus:       git.CIStatusPassing,
				Attempt:        maxAutoMergePollAttempts,
			},
			wantCmd:     true,
			description: "should give up after max attempts (flash warning cmd)",
		},
		{
			name: "max attempts waiting for CI gives up",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.Sessions[0].PRCreated = true
				return cfg
			},
			msg: AutoMergePollResultMsg{
				SessionID:      "session-1",
				ReviewDecision: git.ReviewApproved,
				CIStatus:       git.CIStatusPending,
				Attempt:        maxAutoMergePollAttempts,
			},
			wantCmd:     true,
			description: "should give up after max attempts (flash warning cmd)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setupConfig()
			m, _ := testModelWithMocks(cfg, 120, 40)
			m.sidebar.SetSessions(cfg.Sessions)

			_, cmd := m.handleAutoMergePollResultMsg(tt.msg)

			if tt.wantCmd && cmd == nil {
				t.Errorf("%s: expected non-nil cmd", tt.description)
			}
			if !tt.wantCmd && cmd != nil {
				t.Errorf("%s: expected nil cmd", tt.description)
			}
		})
	}
}

func TestHandleAutoMergePollResultMsg_CommentsUpdateAddressedCount(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	cfg.Sessions[0].PRCreated = true
	cfg.Sessions[0].PRCommentsAddressedCount = 2
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	msg := AutoMergePollResultMsg{
		SessionID:      "session-1",
		ReviewDecision: git.ReviewApproved,
		CommentCount:   5,
		CIStatus:       git.CIStatusPassing,
	}
	_, _ = m.handleAutoMergePollResultMsg(msg)

	// Verify addressed count was updated
	sess := m.config.GetSession("session-1")
	if sess.PRCommentsAddressedCount != 5 {
		t.Errorf("expected PRCommentsAddressedCount=5, got %d", sess.PRCommentsAddressedCount)
	}
}

func TestHandleAutoMergePollResultMsg_CommentsBlockMergeEvenWhenApproved(t *testing.T) {
	// Even if the PR is approved and CI passes, unaddressed comments must be addressed first
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	cfg.Sessions[0].PRCreated = true
	cfg.Sessions[0].PRCommentsAddressedCount = 0
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	msg := AutoMergePollResultMsg{
		SessionID:      "session-1",
		ReviewDecision: git.ReviewApproved,
		CommentCount:   3,
		CIStatus:       git.CIStatusPassing,
	}
	_, cmd := m.handleAutoMergePollResultMsg(msg)

	// Should address comments, not merge
	if cmd == nil {
		t.Error("expected non-nil cmd for comment addressing")
	}

	// The addressed count should be updated
	sess := m.config.GetSession("session-1")
	if sess.PRCommentsAddressedCount != 3 {
		t.Errorf("expected PRCommentsAddressedCount=3, got %d", sess.PRCommentsAddressedCount)
	}
}

// =============================================================================
// TestHandleAutoMergePollResultMsg_ReviewNoneTimesOut
// =============================================================================

func TestHandleAutoMergePollResultMsg_ReviewNoneTimesOut(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	cfg.Sessions[0].PRCreated = true
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	msg := AutoMergePollResultMsg{
		SessionID:      "session-1",
		ReviewDecision: git.ReviewNone,
		CommentCount:   0,
		CIStatus:       git.CIStatusPassing,
		Attempt:        maxAutoMergePollAttempts,
	}
	_, cmd := m.handleAutoMergePollResultMsg(msg)

	// Should give up with a flash warning, same as ReviewRequired timeout
	if cmd == nil {
		t.Error("expected non-nil cmd (flash warning for timeout)")
	}
}

// =============================================================================
// TestPollForAutoMerge_ConcurrencyGuard
// =============================================================================

func TestPollForAutoMerge_ConcurrencyGuard(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	cfg.Sessions[0].PRCreated = true
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// First call should succeed
	cmd1 := m.pollForAutoMerge("session-1")
	if cmd1 == nil {
		t.Error("expected non-nil cmd for first pollForAutoMerge call")
	}

	// Verify flag is set
	state := m.sessionState().GetIfExists("session-1")
	if state == nil {
		t.Fatal("expected session state to exist")
	}
	if !state.GetAutoMergePolling() {
		t.Error("expected AutoMergePolling to be true after first call")
	}

	// Second call should return nil (already polling)
	cmd2 := m.pollForAutoMerge("session-1")
	if cmd2 != nil {
		t.Error("expected nil cmd for duplicate pollForAutoMerge call")
	}

	// After clearing, should succeed again
	m.clearAutoMergePolling("session-1")
	if state.GetAutoMergePolling() {
		t.Error("expected AutoMergePolling to be false after clearing")
	}

	cmd3 := m.pollForAutoMerge("session-1")
	if cmd3 == nil {
		t.Error("expected non-nil cmd after clearing polling flag")
	}
}

func TestHandleAutoMergeResultMsg_ClearsPollingFlag(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	cfg.Sessions[0].PRCreated = true
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Set polling flag
	state := m.sessionState().GetOrCreate("session-1")
	state.SetAutoMergePolling(true)

	// Success case
	msg := AutoMergeResultMsg{SessionID: "session-1", Error: nil}
	_, _ = m.handleAutoMergeResultMsg(msg)

	if state.GetAutoMergePolling() {
		t.Error("expected AutoMergePolling to be cleared after successful merge")
	}
}

func TestHandleAutoMergeResultMsg_ClearsPollingFlagOnError(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	cfg.Sessions[0].PRCreated = true
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Set polling flag
	state := m.sessionState().GetOrCreate("session-1")
	state.SetAutoMergePolling(true)

	// Error case
	msg := AutoMergeResultMsg{SessionID: "session-1", Error: fmt.Errorf("merge conflict")}
	_, _ = m.handleAutoMergeResultMsg(msg)

	if state.GetAutoMergePolling() {
		t.Error("expected AutoMergePolling to be cleared after failed merge")
	}
}

func TestHandleAutoMergePollResultMsg_ClearsPollingOnCIFailure(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	cfg.Sessions[0].PRCreated = true
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Set polling flag
	state := m.sessionState().GetOrCreate("session-1")
	state.SetAutoMergePolling(true)

	msg := AutoMergePollResultMsg{
		SessionID:      "session-1",
		ReviewDecision: git.ReviewApproved,
		CIStatus:       git.CIStatusFailing,
	}
	_, _ = m.handleAutoMergePollResultMsg(msg)

	if state.GetAutoMergePolling() {
		t.Error("expected AutoMergePolling to be cleared after CI failure")
	}
}

func TestHandleAutoMergePollResultMsg_ClearsPollingOnCommentAddressing(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	cfg.Sessions[0].PRCreated = true
	cfg.Sessions[0].PRCommentsAddressedCount = 0
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Set polling flag
	state := m.sessionState().GetOrCreate("session-1")
	state.SetAutoMergePolling(true)

	msg := AutoMergePollResultMsg{
		SessionID:      "session-1",
		ReviewDecision: git.ReviewApproved,
		CommentCount:   3,
		CIStatus:       git.CIStatusPassing,
	}
	_, _ = m.handleAutoMergePollResultMsg(msg)

	// Polling flag should be cleared when redirecting to address comments
	// (it will be re-set when polling restarts after Claude finishes)
	if state.GetAutoMergePolling() {
		t.Error("expected AutoMergePolling to be cleared when addressing comments")
	}
}

// =============================================================================
// TestHandleSessionPipelineCompleteMsg_RestartsAutoMergePolling
// =============================================================================

func TestHandleSessionPipelineCompleteMsg_RestartsAutoMergePolling(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func() *config.Config
		msg         SessionPipelineCompleteMsg
		wantCmd     bool
	}{
		{
			name: "autonomous session with PR and auto-merge restarts polling",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.Sessions[0].PRCreated = true
				cfg.SetRepoAutoMerge("/test/repo1", true)
				return cfg
			},
			msg:     SessionPipelineCompleteMsg{SessionID: "session-1", TestsPassed: true},
			wantCmd: true,
		},
		{
			name: "autonomous session with PR but no auto-merge does not restart",
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
			name: "already merged PR does not restart",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].Autonomous = true
				cfg.Sessions[0].PRCreated = true
				cfg.Sessions[0].PRMerged = true
				cfg.SetRepoAutoMerge("/test/repo1", true)
				return cfg
			},
			msg:     SessionPipelineCompleteMsg{SessionID: "session-1", TestsPassed: true},
			wantCmd: false,
		},
		{
			name: "non-autonomous session does not restart",
			setupConfig: func() *config.Config {
				cfg := testConfigWithSessions()
				cfg.Sessions[0].PRCreated = true
				cfg.SetRepoAutoMerge("/test/repo1", true)
				return cfg
			},
			msg:     SessionPipelineCompleteMsg{SessionID: "session-1", TestsPassed: true},
			wantCmd: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setupConfig()
			m, _ := testModelWithMocks(cfg, 120, 40)
			m.sidebar.SetSessions(cfg.Sessions)

			_, cmd := m.handleSessionPipelineCompleteMsg(tt.msg)

			if tt.wantCmd && cmd == nil {
				t.Error("expected non-nil cmd (auto-merge polling restart)")
			}
			if !tt.wantCmd && cmd != nil {
				t.Error("expected nil cmd")
			}
		})
	}
}

// =============================================================================
// TestHandleCreatePRRequestMsg
// =============================================================================

func TestHandleCreatePRRequestMsg_NilRunner(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	cfg.Sessions[0].IsSupervisor = true
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// No runner registered for session-1
	msg := CreatePRRequestMsg{
		SessionID: "session-1",
		Request:   mcp.CreatePRRequest{ID: float64(1), Title: "Test PR"},
	}
	_, cmd := m.handleCreatePRRequestMsg(msg)
	if cmd != nil {
		t.Error("expected nil cmd when runner is nil")
	}
}

func TestHandleCreatePRRequestMsg_NilSession(t *testing.T) {
	cfg := testConfigWithSessions()
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Register a runner for session-1
	sess := cfg.GetSession("session-1")
	m.sessionMgr.Select(sess, "", "", "")
	mock := factory.GetMock("session-1")
	mock.SetHostTools(true)

	msg := CreatePRRequestMsg{
		SessionID: "nonexistent",
		Request:   mcp.CreatePRRequest{ID: float64(1)},
	}
	_, cmd := m.handleCreatePRRequestMsg(msg)
	// No runner for "nonexistent" so returns nil
	if cmd != nil {
		t.Error("expected nil cmd for nonexistent session")
	}
}

func TestHandleCreatePRRequestMsg_SessionNotFound(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	cfg.Sessions[0].IsSupervisor = true
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Register a runner for session-1
	sess := cfg.GetSession("session-1")
	m.sessionMgr.Select(sess, "", "", "")
	mock := factory.GetMock("session-1")
	mock.SetHostTools(true)

	// Remove the session from config so GetSession returns nil
	cfg.RemoveSession("session-1")

	msg := CreatePRRequestMsg{
		SessionID: "session-1",
		Request:   mcp.CreatePRRequest{ID: float64(1), Title: "Test PR"},
	}
	_, cmd := m.handleCreatePRRequestMsg(msg)
	// Should return a batch cmd (re-registered listeners + error response sent via SendCreatePRResponse)
	if cmd == nil {
		t.Error("expected non-nil cmd (should re-register listeners even on error)")
	}
	// The error response is sent via runner.SendCreatePRResponse which writes to a buffered channel
	_ = mock // runner received the error response internally
}

func TestHandleCreatePRRequestMsg_ValidSession(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	cfg.Sessions[0].IsSupervisor = true
	cfg.Sessions[0].BaseBranch = "main"
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Register a runner for session-1
	sess := cfg.GetSession("session-1")
	m.sessionMgr.Select(sess, "", "", "")
	mock := factory.GetMock("session-1")
	mock.SetHostTools(true)

	msg := CreatePRRequestMsg{
		SessionID: "session-1",
		Request:   mcp.CreatePRRequest{ID: float64(1), Title: "My PR Title"},
	}
	_, cmd := m.handleCreatePRRequestMsg(msg)
	// Should return a batch cmd (listeners + async PR creation)
	if cmd == nil {
		t.Error("expected non-nil cmd")
	}
}

// =============================================================================
// TestHandlePushBranchRequestMsg
// =============================================================================

func TestHandlePushBranchRequestMsg_NilRunner(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	cfg.Sessions[0].IsSupervisor = true
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	msg := PushBranchRequestMsg{
		SessionID: "session-1",
		Request:   mcp.PushBranchRequest{ID: float64(1), CommitMessage: "test commit"},
	}
	_, cmd := m.handlePushBranchRequestMsg(msg)
	if cmd != nil {
		t.Error("expected nil cmd when runner is nil")
	}
}

func TestHandlePushBranchRequestMsg_SessionNotFound(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	cfg.Sessions[0].IsSupervisor = true
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Register a runner
	sess := cfg.GetSession("session-1")
	m.sessionMgr.Select(sess, "", "", "")
	mock := factory.GetMock("session-1")
	mock.SetHostTools(true)

	// Remove session
	cfg.RemoveSession("session-1")

	msg := PushBranchRequestMsg{
		SessionID: "session-1",
		Request:   mcp.PushBranchRequest{ID: float64(1), CommitMessage: "test"},
	}
	_, cmd := m.handlePushBranchRequestMsg(msg)
	if cmd == nil {
		t.Error("expected non-nil cmd (re-register listeners)")
	}
	// The error response is sent via runner.SendPushBranchResponse which writes to a buffered channel
	_ = mock
}

func TestHandlePushBranchRequestMsg_ValidSession(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	cfg.Sessions[0].IsSupervisor = true
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Register a runner
	sess := cfg.GetSession("session-1")
	m.sessionMgr.Select(sess, "", "", "")
	mock := factory.GetMock("session-1")
	mock.SetHostTools(true)

	msg := PushBranchRequestMsg{
		SessionID: "session-1",
		Request:   mcp.PushBranchRequest{ID: float64(1), CommitMessage: "push changes"},
	}
	_, cmd := m.handlePushBranchRequestMsg(msg)
	if cmd == nil {
		t.Error("expected non-nil cmd")
	}
}

// =============================================================================
// TestHandlePRCreatedFromToolMsg
// =============================================================================

func TestHandlePRCreatedFromToolMsg(t *testing.T) {
	cfg := testConfigWithSessions()
	cfg.Sessions[0].Autonomous = true
	cfg.Sessions[0].IsSupervisor = true
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	msg := PRCreatedFromToolMsg{
		SessionID: "session-1",
		PRURL:     "https://github.com/test/repo/pull/42",
	}
	_, cmd := m.handlePRCreatedFromToolMsg(msg)

	// Should return nil cmd (just updates state)
	if cmd != nil {
		t.Error("expected nil cmd")
	}

	// Verify session was marked as PR created
	sess := cfg.GetSession("session-1")
	if sess == nil {
		t.Fatal("session should still exist")
	}
	if !sess.PRCreated {
		t.Error("expected session to be marked as PR created")
	}
}

// =============================================================================
// Helpers
// =============================================================================

// containsStr is a simple helper to check substring presence.
func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
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
