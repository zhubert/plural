package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/changelog"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/issues"
	"github.com/zhubert/plural/internal/mcp"
	"github.com/zhubert/plural/internal/session"
	"github.com/zhubert/plural/internal/ui"
)

// =============================================================================
// Group A: formatPermissionDenialsText (pure function)
// =============================================================================

func TestFormatPermissionDenialsText_EmptyInput(t *testing.T) {
	result := formatPermissionDenialsText(nil)
	if result != "" {
		t.Errorf("expected empty string for nil input, got %q", result)
	}

	result = formatPermissionDenialsText([]claude.PermissionDenial{})
	if result != "" {
		t.Errorf("expected empty string for empty slice, got %q", result)
	}
}

func TestFormatPermissionDenialsText_SingleDenialFull(t *testing.T) {
	denials := []claude.PermissionDenial{
		{Tool: "Bash", Description: "rm -rf /tmp", Reason: "destructive operation"},
	}
	result := formatPermissionDenialsText(denials)

	if !strings.Contains(result, "Bash") {
		t.Error("expected tool name in output")
	}
	if !strings.Contains(result, "rm -rf /tmp") {
		t.Error("expected description in output")
	}
	if !strings.Contains(result, "destructive operation") {
		t.Error("expected reason in output")
	}
	if !strings.Contains(result, "[Permission Denials]") {
		t.Error("expected header in output")
	}
}

func TestFormatPermissionDenialsText_ToolOnly(t *testing.T) {
	denials := []claude.PermissionDenial{
		{Tool: "Edit"},
	}
	result := formatPermissionDenialsText(denials)

	if !strings.Contains(result, "Edit") {
		t.Error("expected tool name in output")
	}
	// Should not contain ": " separator for description since description is empty
	if strings.Contains(result, "Edit:") {
		t.Error("should not have colon after tool name when no description")
	}
	// Should not contain parentheses for reason since reason is empty
	if strings.Contains(result, "(") {
		t.Error("should not have parentheses when no reason")
	}
}

func TestFormatPermissionDenialsText_ToolAndDescription(t *testing.T) {
	denials := []claude.PermissionDenial{
		{Tool: "Write", Description: "create /tmp/file.txt"},
	}
	result := formatPermissionDenialsText(denials)

	if !strings.Contains(result, "Write: create /tmp/file.txt") {
		t.Errorf("expected 'Write: create /tmp/file.txt' in output, got %q", result)
	}
	if strings.Contains(result, "(") {
		t.Error("should not have parentheses when no reason")
	}
}

func TestFormatPermissionDenialsText_MultipleDenials(t *testing.T) {
	denials := []claude.PermissionDenial{
		{Tool: "Bash", Description: "apt install", Reason: "needs sudo"},
		{Tool: "Edit", Description: "/etc/hosts"},
		{Tool: "Write"},
	}
	result := formatPermissionDenialsText(denials)

	lines := strings.Split(strings.TrimSpace(result), "\n")
	// Header + 3 denial lines
	if len(lines) < 4 {
		t.Errorf("expected at least 4 lines, got %d: %q", len(lines), result)
	}
	if !strings.Contains(result, "Bash") {
		t.Error("expected Bash in output")
	}
	if !strings.Contains(result, "Edit") {
		t.Error("expected Edit in output")
	}
	if !strings.Contains(result, "Write") {
		t.Error("expected Write in output")
	}
}

// =============================================================================
// Group B: handleNonActiveSessionStreaming
// =============================================================================

func TestNonActiveSessionStreaming_TextChunk(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select session-1 (the active session)
	m = sendKey(m, "enter")
	if m.activeSession == nil || m.activeSession.ID != "session-1" {
		t.Fatal("expected session-1 to be active")
	}

	// Pre-register a runner for session-3 so handleClaudeResponseMsg doesn't bail
	m.sessionMgr.GetOrCreateRunner(&cfg.Sessions[2])

	// Simulate a text chunk for session-3 (non-active, but started)
	m = simulateClaudeResponse(m, "session-3", claude.ResponseChunk{
		Type:    claude.ChunkTypeText,
		Content: "Hello from session 3",
	})

	state := m.sessionState().GetIfExists("session-3")
	if state == nil {
		t.Fatal("expected session state for session-3")
	}
	if !strings.Contains(state.GetStreamingContent(), "Hello from session 3") {
		t.Errorf("expected streaming content to contain text, got %q", state.GetStreamingContent())
	}
}

func TestNonActiveSessionStreaming_ToolUseChunk(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	m.sessionMgr.GetOrCreateRunner(&cfg.Sessions[2])

	// Simulate tool use for non-active session
	m = simulateClaudeResponse(m, "session-3", claude.ResponseChunk{
		Type:      claude.ChunkTypeToolUse,
		ToolName:  "Read",
		ToolInput: "main.go",
		ToolUseID: "tool-1",
	})

	state := m.sessionState().GetIfExists("session-3")
	if state == nil {
		t.Fatal("expected session state for session-3")
	}

	rollup := state.GetToolUseRollup()
	if rollup == nil || len(rollup.Items) != 1 {
		t.Fatal("expected 1 tool use in rollup")
	}
	if rollup.Items[0].ToolName != "Read" {
		t.Errorf("expected tool name 'Read', got %q", rollup.Items[0].ToolName)
	}
	if rollup.Items[0].Complete {
		t.Error("tool use should not be complete yet")
	}
}

func TestNonActiveSessionStreaming_ToolResultChunk(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	m.sessionMgr.GetOrCreateRunner(&cfg.Sessions[2])

	// Add a tool use first
	m = simulateClaudeResponse(m, "session-3", claude.ResponseChunk{
		Type:      claude.ChunkTypeToolUse,
		ToolName:  "Read",
		ToolInput: "main.go",
		ToolUseID: "tool-1",
	})

	// Then mark it complete
	m = simulateClaudeResponse(m, "session-3", claude.ResponseChunk{
		Type:      claude.ChunkTypeToolResult,
		ToolUseID: "tool-1",
	})

	state := m.sessionState().GetIfExists("session-3")
	if state == nil {
		t.Fatal("expected session state for session-3")
	}

	rollup := state.GetToolUseRollup()
	if rollup == nil || len(rollup.Items) != 1 {
		t.Fatal("expected 1 tool use in rollup")
	}
	if !rollup.Items[0].Complete {
		t.Error("tool use should be marked complete")
	}
}

func TestNonActiveSessionStreaming_TextAfterToolUse(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	m.sessionMgr.GetOrCreateRunner(&cfg.Sessions[2])

	// Add a tool use
	m = simulateClaudeResponse(m, "session-3", claude.ResponseChunk{
		Type:      claude.ChunkTypeToolUse,
		ToolName:  "Bash",
		ToolInput: "ls",
		ToolUseID: "tool-2",
	})

	// Then send text - should flush rollup first
	m = simulateClaudeResponse(m, "session-3", claude.ResponseChunk{
		Type:    claude.ChunkTypeText,
		Content: "Here are the results",
	})

	state := m.sessionState().GetIfExists("session-3")
	if state == nil {
		t.Fatal("expected session state for session-3")
	}

	// Rollup should be flushed (nil)
	if state.GetToolUseRollup() != nil {
		t.Error("expected rollup to be flushed after text arrives")
	}

	// Streaming content should contain both the flushed tool info and the text
	content := state.GetStreamingContent()
	if !strings.Contains(content, "Bash") {
		t.Error("expected flushed tool info in streaming content")
	}
	if !strings.Contains(content, "Here are the results") {
		t.Error("expected text in streaming content")
	}
}

func TestNonActiveSessionStreaming_TodoUpdate(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	m.sessionMgr.GetOrCreateRunner(&cfg.Sessions[2])

	todoList := &claude.TodoList{
		Items: []claude.TodoItem{
			{Content: "Task 1", Status: "pending"},
			{Content: "Task 2", Status: "in_progress"},
		},
	}

	m = simulateClaudeResponse(m, "session-3", claude.ResponseChunk{
		Type:     claude.ChunkTypeTodoUpdate,
		TodoList: todoList,
	})

	state := m.sessionState().GetIfExists("session-3")
	if state == nil {
		t.Fatal("expected session state for session-3")
	}

	storedList := state.GetCurrentTodoList()
	if storedList == nil || len(storedList.Items) != 2 {
		t.Error("expected todo list with 2 items to be stored")
	}
}

func TestNonActiveSessionStreaming_SubagentStatus(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	m.sessionMgr.GetOrCreateRunner(&cfg.Sessions[2])

	m = simulateClaudeResponse(m, "session-3", claude.ResponseChunk{
		Type:          claude.ChunkTypeSubagentStatus,
		SubagentModel: "claude-haiku-4-5",
	})

	state := m.sessionState().GetIfExists("session-3")
	if state == nil {
		t.Fatal("expected session state for session-3")
	}

	if state.GetSubagentModel() != "claude-haiku-4-5" {
		t.Errorf("expected subagent model 'claude-haiku-4-5', got %q", state.GetSubagentModel())
	}
}

func TestNonActiveSessionStreaming_PermissionDenials(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	m.sessionMgr.GetOrCreateRunner(&cfg.Sessions[2])

	m = simulateClaudeResponse(m, "session-3", claude.ResponseChunk{
		Type: claude.ChunkTypePermissionDenials,
		PermissionDenials: []claude.PermissionDenial{
			{Tool: "Bash", Description: "rm -rf /", Reason: "dangerous"},
		},
	})

	state := m.sessionState().GetIfExists("session-3")
	if state == nil {
		t.Fatal("expected session state for session-3")
	}

	content := state.GetStreamingContent()
	if !strings.Contains(content, "Permission Denials") {
		t.Error("expected permission denials header in streaming content")
	}
	if !strings.Contains(content, "Bash") {
		t.Error("expected tool name in streaming content")
	}
}

// =============================================================================
// Group C: handleSendPendingMessageMsg
// =============================================================================

func TestHandleSendPendingMessageMsg_NoPendingMessage(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select session to create runner
	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	// No pending message set
	msg := SendPendingMessageMsg{SessionID: sessionID}
	result, cmd := m.Update(msg)

	if cmd != nil {
		t.Error("expected nil cmd when no pending message")
	}
	_ = result
}

func TestHandleSendPendingMessageMsg_InvalidSession(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Set up a pending message for a non-existent session
	m.sessionState().GetOrCreate("nonexistent").SetPendingMsg("test message")

	msg := SendPendingMessageMsg{SessionID: "nonexistent"}
	_, cmd := m.Update(msg)

	if cmd != nil {
		t.Error("expected nil cmd for invalid session")
	}
}

func TestHandleSendPendingMessageMsg_MergedSession(t *testing.T) {
	cfg := testConfigWithSessions()
	// Mark session-1 as merged to parent
	cfg.Sessions[0].MergedToParent = true
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	m.sessionState().GetOrCreate(sessionID).SetPendingMsg("test message")

	msg := SendPendingMessageMsg{SessionID: sessionID}
	_, cmd := m.Update(msg)

	if cmd != nil {
		t.Error("expected nil cmd for merged session")
	}
}

func TestHandleSendPendingMessageMsg_SessionBusy(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	// Set pending message and mark as waiting
	m.sessionState().GetOrCreate(sessionID).SetPendingMsg("test message")
	m.sessionState().StartWaiting(sessionID, func() {})

	msg := SendPendingMessageMsg{SessionID: sessionID}
	_, _ = m.Update(msg)

	// Message should still be pending (re-queued, not consumed)
	state := m.sessionState().GetIfExists(sessionID)
	if state == nil || state.GetPendingMsg() == "" {
		t.Error("expected message to remain pending when session is busy")
	}
}

// =============================================================================
// Group D: Plan Approval Flow
// =============================================================================

func TestHandlePlanApprovalRequest_ActiveSession(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	m = simulatePlanApprovalRequest(m, sessionID, "# My Plan\n\nDo the thing.", nil)

	// Verify plan approval is stored in session state
	state := m.sessionState().GetIfExists(sessionID)
	if state == nil {
		t.Fatal("expected session state to exist")
	}
	req := state.GetPendingPlanApproval()
	if req == nil {
		t.Fatal("expected pending plan approval request")
	}
	if req.Plan != "# My Plan\n\nDo the thing." {
		t.Errorf("expected plan text, got %q", req.Plan)
	}
}

func TestHandlePlanApprovalRequest_UnknownSession(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Don't select any session, just send for a non-existent session
	msg := PlanApprovalRequestMsg{
		SessionID: "unknown-session",
		Request: mcp.PlanApprovalRequest{
			Plan: "some plan",
		},
	}
	_, cmd := m.Update(msg)

	// Should return nil cmd since session is unknown (no runner)
	if cmd != nil {
		t.Error("expected nil cmd for unknown session")
	}
}

func TestSubmitPlanApprovalResponse_Approved(t *testing.T) {
	cfg := testConfigWithSessions()
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	mock := factory.GetMock(sessionID)
	if mock == nil {
		t.Fatal("no mock runner")
	}

	var capturedResp mcp.PlanApprovalResponse
	mock.OnPlanApprovalResp = func(resp mcp.PlanApprovalResponse) {
		capturedResp = resp
	}

	// Send plan approval request
	m = simulatePlanApprovalRequest(m, sessionID, "# Plan", []mcp.AllowedPrompt{
		{Tool: "Bash", Prompt: "run tests"},
	})

	// Approve with 'y'
	m = sendKey(m, "y")

	// Verify the response was sent as approved
	if !capturedResp.Approved {
		t.Error("expected plan to be approved")
	}

	// Verify state is cleared
	state := m.sessionState().GetIfExists(sessionID)
	if state != nil && state.GetPendingPlanApproval() != nil {
		t.Error("expected pending plan approval to be cleared")
	}
}

func TestSubmitPlanApprovalResponse_Denied(t *testing.T) {
	cfg := testConfigWithSessions()
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	mock := factory.GetMock(sessionID)
	if mock == nil {
		t.Fatal("no mock runner")
	}

	var capturedResp mcp.PlanApprovalResponse
	mock.OnPlanApprovalResp = func(resp mcp.PlanApprovalResponse) {
		capturedResp = resp
	}

	m = simulatePlanApprovalRequest(m, sessionID, "# Plan", nil)

	// Deny with 'n'
	m = sendKey(m, "n")

	if capturedResp.Approved {
		t.Error("expected plan to be denied")
	}
}

// =============================================================================
// Group E: Issue Fetch Handlers
// =============================================================================

func TestHandleGitHubIssuesFetchedMsg_WithIssues(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Show import issues modal first
	m.modal.Show(ui.NewImportIssuesState("/test/repo1", "repo1"))

	msg := GitHubIssuesFetchedMsg{
		RepoPath: "/test/repo1",
		Issues: []git.GitHubIssue{
			{Number: 1, Title: "Bug report", Body: "Something broke", URL: "https://github.com/test/1"},
			{Number: 2, Title: "Feature request", Body: "Add feature", URL: "https://github.com/test/2"},
		},
	}
	m.Update(msg)

	// Verify issues were set on modal
	state, ok := m.modal.State.(*ui.ImportIssuesState)
	if !ok {
		t.Fatal("expected ImportIssuesState modal")
	}
	if len(state.Issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(state.Issues))
	}
}

func TestHandleGitHubIssuesFetchedMsg_WithError(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Show import issues modal first
	m.modal.Show(ui.NewImportIssuesState("/test/repo1", "repo1"))

	msg := GitHubIssuesFetchedMsg{
		RepoPath: "/test/repo1",
		Error:    errors.New("failed to fetch issues"),
	}
	m.Update(msg)

	// Verify error was set on modal
	state, ok := m.modal.State.(*ui.ImportIssuesState)
	if !ok {
		t.Fatal("expected ImportIssuesState modal")
	}
	if state.LoadError == "" {
		t.Error("expected error to be set on modal state")
	}
}

func TestHandleGitHubIssuesFetchedMsg_NoModal(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)

	// Don't show any modal - should be a no-op
	msg := GitHubIssuesFetchedMsg{
		RepoPath: "/test/repo1",
		Error:    errors.New("some error"),
	}
	_, cmd := m.Update(msg)

	// Should return nil cmd (no-op)
	if cmd != nil {
		t.Error("expected nil cmd when no modal is shown")
	}
}

func TestHandleIssuesFetchedMsg_WithIssues(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Show import issues modal
	m.modal.Show(ui.NewImportIssuesState("/test/repo1", "repo1"))

	msg := IssuesFetchedMsg{
		RepoPath: "/test/repo1",
		Source:   "github",
		Issues: []issues.Issue{
			{ID: "1", Title: "Bug fix", Source: "github", URL: "https://github.com/test/1"},
			{ID: "2", Title: "New feature", Source: "github", URL: "https://github.com/test/2"},
		},
	}
	m.Update(msg)

	// Verify issues were set on modal
	state, ok := m.modal.State.(*ui.ImportIssuesState)
	if !ok {
		t.Fatal("expected ImportIssuesState modal")
	}
	if len(state.Issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(state.Issues))
	}
}

func TestHandleIssuesFetchedMsg_WithError(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m.modal.Show(ui.NewImportIssuesState("/test/repo1", "repo1"))

	msg := IssuesFetchedMsg{
		RepoPath: "/test/repo1",
		Source:   "asana",
		Error:    errors.New("authentication failed"),
	}
	m.Update(msg)

	state, ok := m.modal.State.(*ui.ImportIssuesState)
	if !ok {
		t.Fatal("expected ImportIssuesState modal")
	}
	if state.LoadError == "" {
		t.Error("expected error to be set on modal state")
	}
	if !strings.Contains(state.LoadError, "authentication failed") {
		t.Errorf("expected error message, got %q", state.LoadError)
	}
}

// =============================================================================
// Group F: Changelog handler
// =============================================================================

func TestHandleChangelogFetchedMsg_WithError(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)

	msg := ChangelogFetchedMsg{
		Error: errors.New("network error"),
	}
	result, _ := m.Update(msg)
	updated := result.(*Model)

	// Modal should not be shown on error
	if updated.modal.IsVisible() {
		t.Error("expected no modal on changelog error")
	}
}

func TestHandleChangelogFetchedMsg_WithEntries(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)

	msg := ChangelogFetchedMsg{
		ShowAll: true,
		Entries: []changelog.Entry{
			{Version: "v1.0.0", Date: "2025-01-01", Changes: []string{"New feature"}},
			{Version: "v0.9.0", Date: "2024-12-01", Changes: []string{"Bug fix"}},
		},
	}
	result, _ := m.Update(msg)
	updated := result.(*Model)

	if !updated.modal.IsVisible() {
		t.Error("expected changelog modal to be shown")
	}

	_, ok := updated.modal.State.(*ui.ChangelogState)
	if !ok {
		t.Error("expected ChangelogState modal")
	}
}

func TestHandleChangelogFetchedMsg_NoEntries(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)

	msg := ChangelogFetchedMsg{
		ShowAll: true,
		Entries: []changelog.Entry{},
	}
	result, _ := m.Update(msg)
	updated := result.(*Model)

	if updated.modal.IsVisible() {
		t.Error("expected no modal when no changelog entries")
	}
}

// =============================================================================
// Group G: Accessor and utility function tests
// =============================================================================

func TestFlashMessages(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	tests := []struct {
		name string
		fn   func(string) tea.Cmd
	}{
		{"ShowFlashWarning", func(s string) tea.Cmd { return m.ShowFlashWarning(s) }},
		{"ShowFlashInfo", func(s string) tea.Cmd { return m.ShowFlashInfo(s) }},
		{"ShowFlashSuccess", func(s string) tea.Cmd { return m.ShowFlashSuccess(s) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.fn("test message")
			if cmd == nil {
				t.Errorf("%s should return a non-nil command", tt.name)
			}
		})
	}
}

func TestActiveSession_NoSession(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	if m.ActiveSession() != nil {
		t.Error("expected nil active session")
	}
}

func TestSessionMgr_NotNil(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	if m.SessionMgr() == nil {
		t.Error("expected non-nil session manager")
	}
}

func TestSetSessionService(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	svc := session.NewSessionService()
	m.SetSessionService(svc)
	// Should not panic
}

func TestHasActiveStreaming_NoStreaming(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	if m.HasActiveStreaming() {
		t.Error("expected no active streaming")
	}
}

func TestClose_NoPanic(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Close should not panic even with no active sessions
	m.Close()
}

// =============================================================================
// Group H: Supervisor Completion Deferral
// =============================================================================

// testConfigWithSupervisor creates a config with a supervisor and child sessions.
func testConfigWithSupervisor(childCount int) *config.Config {
	cfg := testConfig()
	cfg.Sessions = []config.Session{
		{
			ID:           "supervisor-1",
			RepoPath:     "/test/repo1",
			WorkTree:     "/test/worktree-supervisor",
			Branch:       "feature-supervisor",
			Name:         "repo1/supervisor",
			CreatedAt:    time.Now(),
			Started:      true,
			Autonomous:   true,
			IsSupervisor: true,
		},
	}
	for i := 0; i < childCount; i++ {
		cfg.Sessions = append(cfg.Sessions, config.Session{
			ID:           "child-" + string(rune('a'+i)),
			RepoPath:     "/test/repo1",
			WorkTree:     "/test/worktree-child-" + string(rune('a'+i)),
			Branch:       "feature-child-" + string(rune('a'+i)),
			Name:         "repo1/child-" + string(rune('a'+i)),
			CreatedAt:    time.Now(),
			Started:      true,
			SupervisorID: "supervisor-1",
		})
	}
	return cfg
}

func TestSupervisorDeferral_ActiveStreamingChild(t *testing.T) {
	cfg := testConfigWithSupervisor(2)
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select the supervisor session
	m = sendKey(m, "enter")
	if m.activeSession == nil || m.activeSession.ID != "supervisor-1" {
		t.Fatal("expected supervisor-1 to be active")
	}

	// Create runners for children
	m.sessionMgr.GetOrCreateRunner(&cfg.Sessions[1])
	m.sessionMgr.GetOrCreateRunner(&cfg.Sessions[2])

	// Mark one child as streaming
	childMock := factory.GetMock("child-a")
	if childMock == nil {
		t.Fatal("no mock runner for child-a")
	}
	childMock.SetStreaming(true)

	// Simulate supervisor Done — should NOT produce SessionCompletedMsg
	msg := ClaudeResponseMsg{
		SessionID: "supervisor-1",
		Chunk:     doneChunk(),
	}
	_, cmd := m.Update(msg)

	// The cmd should NOT contain a SessionCompletedMsg
	if cmd != nil {
		// Execute the command to check what messages it produces
		resultMsg := cmd()
		if _, ok := resultMsg.(SessionCompletedMsg); ok {
			t.Error("supervisor should NOT emit SessionCompletedMsg while children are streaming")
		}
	}
}

func TestSupervisorDeferral_ChildWaiting(t *testing.T) {
	cfg := testConfigWithSupervisor(1)
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")

	// Create runner for child
	m.sessionMgr.GetOrCreateRunner(&cfg.Sessions[1])

	// Mark child as waiting (e.g., waiting for user input or permission)
	// Use StartWaiting for thread-safe access to IsWaiting
	m.sessionState().StartWaiting("child-a", func() {})

	msg := ClaudeResponseMsg{
		SessionID: "supervisor-1",
		Chunk:     doneChunk(),
	}
	_, cmd := m.Update(msg)

	if cmd != nil {
		resultMsg := cmd()
		if _, ok := resultMsg.(SessionCompletedMsg); ok {
			t.Error("supervisor should NOT emit SessionCompletedMsg while child is waiting")
		}
	}
}

func TestSupervisorDeferral_ChildMerging(t *testing.T) {
	cfg := testConfigWithSupervisor(1)
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")

	// Create runner for child
	m.sessionMgr.GetOrCreateRunner(&cfg.Sessions[1])

	// Mark child as merging
	mergeCh := make(chan git.Result, 1)
	m.sessionState().StartMerge("child-a", mergeCh, nil, MergeTypeParent)

	msg := ClaudeResponseMsg{
		SessionID: "supervisor-1",
		Chunk:     doneChunk(),
	}
	_, cmd := m.Update(msg)

	if cmd != nil {
		resultMsg := cmd()
		if _, ok := resultMsg.(SessionCompletedMsg); ok {
			t.Error("supervisor should NOT emit SessionCompletedMsg while child is merging")
		}
	}
}

func TestSupervisorCompletion_AllChildrenDone(t *testing.T) {
	cfg := testConfigWithSupervisor(2)
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")

	// Create runners for children — not streaming
	m.sessionMgr.GetOrCreateRunner(&cfg.Sessions[1])
	m.sessionMgr.GetOrCreateRunner(&cfg.Sessions[2])

	childA := factory.GetMock("child-a")
	childB := factory.GetMock("child-b")
	childA.SetStreaming(false)
	childB.SetStreaming(false)

	// Simulate supervisor Done — should produce SessionCompletedMsg
	msg := ClaudeResponseMsg{
		SessionID: "supervisor-1",
		Chunk:     doneChunk(),
	}
	_, cmd := m.Update(msg)

	// Should have a command that produces SessionCompletedMsg
	if cmd == nil {
		t.Fatal("expected a command when supervisor completes with no active children")
	}

	// Execute the batch to find the SessionCompletedMsg
	found := false
	resultMsg := cmd()
	if _, ok := resultMsg.(SessionCompletedMsg); ok {
		found = true
	}
	// The cmd might be a batch — if the direct result isn't SessionCompletedMsg,
	// that's OK because batch returns a slice. The key test is that it's NOT nil.
	if !found {
		// Check if it's a tea.BatchMsg containing SessionCompletedMsg
		if batch, ok := resultMsg.(tea.BatchMsg); ok {
			for _, innerCmd := range batch {
				if innerMsg := innerCmd(); innerMsg != nil {
					if _, ok := innerMsg.(SessionCompletedMsg); ok {
						found = true
						break
					}
				}
			}
		}
	}
	if !found {
		t.Error("expected SessionCompletedMsg when all children are done")
	}
}

func TestSupervisorDeferral_NoChildren(t *testing.T) {
	// Supervisor with no children should complete normally
	cfg := testConfigWithSupervisor(0)
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")

	msg := ClaudeResponseMsg{
		SessionID: "supervisor-1",
		Chunk:     doneChunk(),
	}
	_, cmd := m.Update(msg)

	if cmd == nil {
		t.Fatal("expected a command when supervisor completes with no children")
	}

	found := false
	resultMsg := cmd()
	if _, ok := resultMsg.(SessionCompletedMsg); ok {
		found = true
	}
	if !found {
		if batch, ok := resultMsg.(tea.BatchMsg); ok {
			for _, innerCmd := range batch {
				if innerMsg := innerCmd(); innerMsg != nil {
					if _, ok := innerMsg.(SessionCompletedMsg); ok {
						found = true
						break
					}
				}
			}
		}
	}
	if !found {
		t.Error("expected SessionCompletedMsg when supervisor has no children")
	}
}
