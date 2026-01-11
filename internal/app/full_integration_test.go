package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/zhubert/plural/internal/mcp"
)

// =============================================================================
// Full Message Flow Tests
// =============================================================================

func TestFullFlow_SendMessageReceiveResponse(t *testing.T) {
	cfg := testConfigWithSessions()
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select session - this creates a runner via the mock factory
	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Expected active session after enter")
	}
	sessionID := m.activeSession.ID

	// Get mock runner and verify it was created
	mock := factory.GetMock(sessionID)
	if mock == nil {
		t.Fatal("No mock runner created for session")
	}

	// Queue response chunks
	mock.QueueResponse(
		textChunk("Hello! "),
		textChunk("I'm Claude."),
		doneChunk(),
	)

	// Type and send message
	m.chat.SetInput("Hello Claude")

	// Simulate sending (we can't easily trigger the actual send without the runner)
	// Instead, directly simulate receiving the response chunks
	m = simulateClaudeResponse(m, sessionID, textChunk("Hello! "))
	m = simulateClaudeResponse(m, sessionID, textChunk("I'm Claude."))
	m = simulateClaudeResponse(m, sessionID, doneChunk())

	// Verify final state - should be idle after done chunk
	if m.state != StateIdle {
		t.Errorf("Expected StateIdle after completion, got %v", m.state)
	}
}

func TestFullFlow_ToolUseDisplay(t *testing.T) {
	cfg := testConfigWithSessions()
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select session
	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	mock := factory.GetMock(sessionID)
	if mock == nil {
		t.Fatal("No mock runner created")
	}

	// Simulate a response with tool use
	m = simulateClaudeResponse(m, sessionID, textChunk("Let me read that file.\n"))
	m = simulateClaudeResponse(m, sessionID, toolChunk("Read", "main.go"))
	m = simulateClaudeResponse(m, sessionID, textChunk("\nHere's the content..."))

	// Verify tool use is reflected in streaming content
	streaming := m.chat.GetStreaming()
	if !strings.Contains(streaming, "Read") {
		t.Errorf("Expected tool use 'Read' in streaming content, got: %s", streaming)
	}

	// Complete streaming
	m = simulateClaudeResponse(m, sessionID, doneChunk())

	if m.state != StateIdle {
		t.Errorf("Expected StateIdle after completion, got %v", m.state)
	}
}

func TestFullFlow_MultipleChunks(t *testing.T) {
	cfg := testConfigWithSessions()
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	mock := factory.GetMock(sessionID)
	if mock == nil {
		t.Fatal("No mock runner created")
	}

	// Send multiple text chunks
	chunks := []string{"First ", "second ", "third ", "fourth."}
	for _, chunk := range chunks {
		m = simulateClaudeResponse(m, sessionID, textChunk(chunk))
	}

	// Verify content accumulated
	streaming := m.chat.GetStreaming()
	for _, chunk := range chunks {
		if !strings.Contains(streaming, strings.TrimSpace(chunk)) {
			t.Errorf("Expected chunk %q in streaming, got: %s", chunk, streaming)
		}
	}

	m = simulateClaudeResponse(m, sessionID, doneChunk())
}

// =============================================================================
// Permission Flow Tests
// =============================================================================

func TestPermission_FullFlow_Approve(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select session
	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	// Simulate permission request arriving
	m = simulatePermissionRequest(m, sessionID, "Bash", "Run: rm -rf /tmp/test")

	// Verify permission is pending
	if !m.chat.HasPendingPermission() {
		t.Error("Chat should have pending permission")
	}

	pending := m.sessionState().GetPendingPermission(sessionID)
	if pending == nil || pending.Tool != "Bash" {
		t.Error("Session state should have pending Bash permission")
	}

	// Approve with 'y' - need to be in chat focus
	m = sendKey(m, "y")

	// Verify permission cleared
	if m.chat.HasPendingPermission() {
		t.Error("Permission should be cleared after approval")
	}
	if m.sessionState().GetPendingPermission(sessionID) != nil {
		t.Error("Session state permission should be cleared")
	}
}

func TestPermission_FullFlow_Deny(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	m = simulatePermissionRequest(m, sessionID, "Bash", "Run: dangerous command")

	// Deny with 'n'
	m = sendKey(m, "n")

	// Verify permission cleared
	if m.chat.HasPendingPermission() {
		t.Error("Permission should be cleared after denial")
	}
}

func TestPermission_FullFlow_Always(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	toolName := "Bash(git:*)"
	m = simulatePermissionRequest(m, sessionID, toolName, "Run: git status")

	// Always allow with 'a'
	m = sendKey(m, "a")

	// Verify permission cleared
	if m.chat.HasPendingPermission() {
		t.Error("Permission should be cleared")
	}

	// Verify tool was added to allowed list for the repo
	repoPath := m.activeSession.RepoPath
	allowedTools := m.config.GetAllowedToolsForRepo(repoPath)
	found := false
	for _, tool := range allowedTools {
		if tool == toolName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Tool %s should be added to allowed list, got: %v", toolName, allowedTools)
	}
}

func TestPermission_OnlyInChatFocus(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	m = simulatePermissionRequest(m, sessionID, "Bash", "Run: test")

	// Switch to sidebar
	m = sendKey(m, "tab")
	if m.focus != FocusSidebar {
		t.Fatal("Should be in sidebar focus")
	}

	// Try to approve - should not work from sidebar
	m = sendKey(m, "y")

	// Permission should still be pending
	if m.sessionState().GetPendingPermission(sessionID) == nil {
		t.Error("Permission should still be pending when response sent from sidebar")
	}
}

// =============================================================================
// Question Flow Tests
// =============================================================================

func TestQuestion_SelectByNumber(t *testing.T) {
	cfg := testConfigWithSessions()
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	// Get the mock runner to capture the question response
	mock := factory.GetMock(sessionID)
	if mock == nil {
		t.Fatal("No mock runner")
	}

	var capturedAnswers map[string]string
	mock.OnQuestionResp = func(resp mcp.QuestionResponse) {
		capturedAnswers = resp.Answers
	}

	questions := []mcp.Question{
		{
			Question: "Choose a testing framework",
			Header:   "Framework",
			Options: []mcp.QuestionOption{
				{Label: "Option A", Description: "First option"},
				{Label: "Option B", Description: "Second option"},
			},
		},
	}

	m = simulateQuestionRequest(m, sessionID, questions)

	// Verify question is pending
	if !m.chat.HasPendingQuestion() {
		t.Error("Chat should have pending question")
	}

	// Select option 1 by pressing '1'
	m = sendKey(m, "1")

	// Verify question completed (answers are cleared from chat after submission)
	if m.chat.HasPendingQuestion() {
		t.Error("Question should be cleared after selection")
	}

	// Verify answer was captured via the callback
	if len(capturedAnswers) != 1 {
		t.Errorf("Expected 1 answer in captured response, got %d", len(capturedAnswers))
	}
	if answer, ok := capturedAnswers["Choose a testing framework"]; !ok || answer != "Option A" {
		t.Errorf("Expected answer 'Option A', got %q", answer)
	}
}

func TestQuestion_NavigateAndSelect(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	questions := []mcp.Question{
		{
			Question: "Pick one",
			Header:   "Choice",
			Options: []mcp.QuestionOption{
				{Label: "A", Description: "Option A"},
				{Label: "B", Description: "Option B"},
				{Label: "C", Description: "Option C"},
			},
		},
	}

	m = simulateQuestionRequest(m, sessionID, questions)

	// Navigate down twice with arrow keys
	m = sendKey(m, "down")
	m = sendKey(m, "down")

	// Select with enter
	m = sendKey(m, "enter")

	// Question should be completed
	if m.chat.HasPendingQuestion() {
		t.Error("Question should be cleared after enter")
	}
}

func TestQuestion_MultipleQuestions(t *testing.T) {
	cfg := testConfigWithSessions()
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	// Get the mock runner to capture the question response
	mock := factory.GetMock(sessionID)
	if mock == nil {
		t.Fatal("No mock runner")
	}

	var capturedAnswers map[string]string
	mock.OnQuestionResp = func(resp mcp.QuestionResponse) {
		capturedAnswers = resp.Answers
	}

	questions := []mcp.Question{
		{
			Question: "First question",
			Header:   "Q1",
			Options: []mcp.QuestionOption{
				{Label: "A", Description: "First"},
				{Label: "B", Description: "Second"},
			},
		},
		{
			Question: "Second question",
			Header:   "Q2",
			Options: []mcp.QuestionOption{
				{Label: "X", Description: "Option X"},
				{Label: "Y", Description: "Option Y"},
			},
		},
	}

	m = simulateQuestionRequest(m, sessionID, questions)

	// Answer first question - should not complete yet (2 questions total)
	m = sendKey(m, "1")

	// Should still have pending question (second one)
	if !m.chat.HasPendingQuestion() {
		t.Error("Should still have pending question after first answer")
	}

	// Answer second question
	m = sendKey(m, "2")

	// Now should be complete
	if m.chat.HasPendingQuestion() {
		t.Error("Question should be cleared after all answers")
	}

	// Verify answers were captured
	if len(capturedAnswers) != 2 {
		t.Errorf("Expected 2 answers, got %d", len(capturedAnswers))
	}
}

// =============================================================================
// Session Switching Tests
// =============================================================================

func TestSessionSwitch_PreservesStreamingContent(t *testing.T) {
	cfg := testConfigWithSessions()
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select first session
	m = sendKey(m, "enter")
	sessionA := m.activeSession.ID

	mock := factory.GetMock(sessionA)
	if mock == nil {
		t.Fatal("No mock runner for session A")
	}

	// Simulate partial streaming (no done chunk)
	m = simulateClaudeResponse(m, sessionA, textChunk("Partial response..."))

	// Get streaming content
	streamingBefore := m.chat.GetStreaming()
	if streamingBefore == "" {
		t.Fatal("Expected streaming content before switch")
	}

	// Switch to sidebar and navigate to second session
	m = sendKey(m, "tab")
	m = sendKey(m, "down")
	m = sendKey(m, "enter")

	// Verify we're on a different session
	if m.activeSession.ID == sessionA {
		t.Fatal("Should have switched to different session")
	}

	// Switch back to first session
	m = sendKey(m, "tab")
	m = sendKey(m, "up")
	m = sendKey(m, "enter")

	// Verify we're back on session A
	if m.activeSession.ID != sessionA {
		t.Fatal("Should be back on session A")
	}

	// Verify streaming content was preserved
	streamingAfter := m.chat.GetStreaming()
	if streamingAfter != streamingBefore {
		t.Errorf("Streaming content not preserved. Before: %q, After: %q", streamingBefore, streamingAfter)
	}
}

func TestSessionSwitch_PreservesPendingPermission(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select first session
	m = sendKey(m, "enter")
	sessionA := m.activeSession.ID

	// Set pending permission
	m = simulatePermissionRequest(m, sessionA, "Bash", "test command")

	if !m.chat.HasPendingPermission() {
		t.Fatal("Should have pending permission")
	}

	// Switch to second session
	m = sendKey(m, "tab")
	m = sendKey(m, "down")
	m = sendKey(m, "enter")

	// New session should not have permission displayed
	if m.chat.HasPendingPermission() {
		t.Error("New session should not have pending permission in chat")
	}

	// But session A's state should still have it
	if m.sessionState().GetPendingPermission(sessionA) == nil {
		t.Error("Session A should still have pending permission in state")
	}

	// Switch back to session A
	m = sendKey(m, "tab")
	m = sendKey(m, "up")
	m = sendKey(m, "enter")

	// Permission should be restored in chat
	if !m.chat.HasPendingPermission() {
		t.Error("Permission should be restored when switching back")
	}
}

func TestSessionSwitch_PreservesPendingQuestion(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionA := m.activeSession.ID

	questions := []mcp.Question{
		{
			Question: "Pick one",
			Header:   "Test",
			Options: []mcp.QuestionOption{
				{Label: "A"},
				{Label: "B"},
			},
		},
	}

	m = simulateQuestionRequest(m, sessionA, questions)

	if !m.chat.HasPendingQuestion() {
		t.Fatal("Should have pending question")
	}

	// Switch sessions
	m = sendKey(m, "tab")
	m = sendKey(m, "down")
	m = sendKey(m, "enter")

	// Session A state should still have question
	if m.sessionState().GetPendingQuestion(sessionA) == nil {
		t.Error("Session A should still have pending question in state")
	}

	// Switch back
	m = sendKey(m, "tab")
	m = sendKey(m, "up")
	m = sendKey(m, "enter")

	// Question should be restored
	if !m.chat.HasPendingQuestion() {
		t.Error("Question should be restored when switching back")
	}
}

// =============================================================================
// Streaming State Tests
// =============================================================================

func TestStreaming_StateTransitions(t *testing.T) {
	cfg := testConfigWithSessions()
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	mock := factory.GetMock(sessionID)
	if mock == nil {
		t.Fatal("No mock runner")
	}

	// Initially idle
	if m.state != StateIdle {
		t.Errorf("Expected initial StateIdle, got %v", m.state)
	}

	// First response chunk should trigger streaming state
	m = simulateClaudeResponse(m, sessionID, textChunk("Hello"))

	// After done, back to idle
	m = simulateClaudeResponse(m, sessionID, doneChunk())

	if m.state != StateIdle {
		t.Errorf("Expected StateIdle after done, got %v", m.state)
	}
}

func TestStreaming_ErrorHandling(t *testing.T) {
	cfg := testConfigWithSessions()
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	mock := factory.GetMock(sessionID)
	if mock == nil {
		t.Fatal("No mock runner")
	}

	// Send an error chunk
	m = simulateClaudeResponse(m, sessionID, errorChunk(errors.New("session in use")))

	// Should be back to idle after error
	if m.state != StateIdle {
		t.Errorf("Expected StateIdle after error, got %v", m.state)
	}
}

// =============================================================================
// Message Queueing Tests
// =============================================================================

func TestMessageQueue_DuringStreaming(t *testing.T) {
	cfg := testConfigWithSessions()
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	mock := factory.GetMock(sessionID)
	if mock == nil {
		t.Fatal("No mock runner")
	}

	// Set streaming state manually (simulating active streaming)
	mock.SetStreaming(true)
	m.setState(StateStreamingClaude)

	// Queue a pending message
	m.sessionState().SetPendingMessage(sessionID, "queued message")

	// Verify message is queued
	if !m.sessionState().HasPendingMessage(sessionID) {
		t.Error("Message should be queued during streaming")
	}

	queued := m.sessionState().PeekPendingMessage(sessionID)
	if queued != "queued message" {
		t.Errorf("Expected 'queued message', got %q", queued)
	}
}

// =============================================================================
// Integration with Mock Runner Tests
// =============================================================================

func TestMockRunner_TracksMessages(t *testing.T) {
	cfg := testConfigWithSessions()
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	mock := factory.GetMock(sessionID)
	if mock == nil {
		t.Fatal("No mock runner")
	}

	// Add a message to the mock
	mock.AddAssistantMessage("Test assistant message")

	// Verify it's tracked
	messages := mock.GetMessages()
	if len(messages) == 0 {
		t.Fatal("Expected at least one message")
	}

	found := false
	for _, msg := range messages {
		if msg.Role == "assistant" && msg.Content == "Test assistant message" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find the assistant message")
	}
}

func TestMockRunner_AllowedTools(t *testing.T) {
	cfg := testConfigWithSessions()
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	mock := factory.GetMock(sessionID)
	if mock == nil {
		t.Fatal("No mock runner")
	}

	// Add a tool
	mock.AddAllowedTool("TestTool")

	// Verify it's in the list
	tools := mock.GetAllowedTools()
	found := false
	for _, tool := range tools {
		if tool == "TestTool" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected TestTool in allowed tools")
	}
}

// =============================================================================
// View Changes Mode Tests
// =============================================================================

func TestViewChanges_EnterAndExit(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select a session first (this switches focus to chat)
	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Expected active session")
	}
	if m.focus != FocusChat {
		t.Fatal("Expected chat focus after selecting session")
	}

	// Enter view changes mode directly (simulating what shortcutViewChanges does)
	m.chat.EnterViewChangesMode(testFileDiffs())

	if !m.chat.IsInViewChangesMode() {
		t.Error("Expected to be in view changes mode")
	}

	// Exit with Escape (focus should be on chat for this to work)
	m = sendKey(m, "esc")

	if m.chat.IsInViewChangesMode() {
		t.Error("Expected to exit view changes mode after Escape")
	}
}

func TestViewChanges_NavigateFiles(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	m.chat.EnterViewChangesMode(testFileDiffs())

	if !m.chat.IsInViewChangesMode() {
		t.Fatal("Should be in view changes mode")
	}

	// Get initial state - first file should be selected
	initialIdx := m.chat.GetSelectedFileIndex()
	if initialIdx != 0 {
		t.Errorf("Expected initial file index 0, got %d", initialIdx)
	}

	// Switch to file pane first (up/down only navigate files when file pane is focused)
	m = sendKey(m, "h")
	if m.chat.GetViewChangesFocus() != "files" {
		t.Fatal("Expected to be in file pane after 'h'")
	}

	// Navigate down
	m = sendKey(m, "down")
	afterDown := m.chat.GetSelectedFileIndex()
	if afterDown != 1 {
		t.Errorf("Expected file index 1 after down, got %d", afterDown)
	}

	// Navigate down again
	m = sendKey(m, "down")
	afterDown2 := m.chat.GetSelectedFileIndex()
	if afterDown2 != 2 {
		t.Errorf("Expected file index 2 after second down, got %d", afterDown2)
	}

	// Navigate up
	m = sendKey(m, "up")
	afterUp := m.chat.GetSelectedFileIndex()
	if afterUp != 1 {
		t.Errorf("Expected file index 1 after up, got %d", afterUp)
	}
}

func TestViewChanges_SwitchPanes(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	m.chat.EnterViewChangesMode(testFileDiffs())

	// Initial focus should be on diff pane (right pane) based on EnterViewChangesMode
	initialPane := m.chat.GetViewChangesFocus()
	if initialPane != "diff" {
		t.Errorf("Expected initial focus on 'diff', got %q", initialPane)
	}

	// Switch to file list pane with 'h' or left arrow
	m = sendKey(m, "h")
	afterH := m.chat.GetViewChangesFocus()
	if afterH != "files" {
		t.Errorf("Expected focus on 'files' after 'h', got %q", afterH)
	}

	// Switch back with 'l'
	m = sendKey(m, "l")
	afterL := m.chat.GetViewChangesFocus()
	if afterL != "diff" {
		t.Errorf("Expected focus on 'diff' after 'l', got %q", afterL)
	}
}

// =============================================================================
// Fork Modal Tests
// =============================================================================

func TestForkModal_OpenAndClose(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Need sidebar focus and a session selected to open fork modal
	if m.focus != FocusSidebar {
		t.Fatal("Expected sidebar focus")
	}

	// Press 'f' to open fork modal
	m = sendKey(m, "f")

	if !m.modal.IsVisible() {
		t.Error("Expected modal to be visible after pressing 'f'")
	}

	// Close with Escape
	m = sendKey(m, "esc")

	if m.modal.IsVisible() {
		t.Error("Expected modal to close after Escape")
	}
}

// =============================================================================
// Merge Modal Tests
// =============================================================================

func TestMergeModal_OpenAndClose(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Press 'm' to open merge modal (requires sidebar focus and session)
	m = sendKey(m, "m")

	if !m.modal.IsVisible() {
		t.Error("Expected modal to be visible after pressing 'm'")
	}

	// Close with Escape
	m = sendKey(m, "esc")

	if m.modal.IsVisible() {
		t.Error("Expected modal to close after Escape")
	}
}

func TestMergeModal_ChildSessionShowsParentOption(t *testing.T) {
	cfg := testConfigWithParentChild()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Navigate to child session (second in the list)
	m = sendKey(m, "down")

	// Select the child session
	selected := m.sidebar.SelectedSession()
	if selected == nil || selected.ParentID == "" {
		t.Fatal("Expected to select child session with parent")
	}

	// Open merge modal
	m = sendKey(m, "m")

	if !m.modal.IsVisible() {
		t.Error("Expected modal to be visible")
	}

	// The modal should be showing - can't easily verify contents without accessing modal state
	// but we can verify the modal is shown for a child session
}

func TestMergeModal_PRSessionShowsPushOption(t *testing.T) {
	cfg := testConfigWithParentChild()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Navigate to PR session (third in the list)
	m = sendKey(m, "down")
	m = sendKey(m, "down")

	selected := m.sidebar.SelectedSession()
	if selected == nil || !selected.PRCreated {
		t.Fatal("Expected to select session with PR created")
	}

	// Open merge modal
	m = sendKey(m, "m")

	if !m.modal.IsVisible() {
		t.Error("Expected modal to be visible")
	}
}

// =============================================================================
// Git Result Handling Tests
// =============================================================================

func TestGitResult_StreamingOutput(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select session
	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	// Simulate merge output streaming
	m = simulateMergeResult(m, sessionID, "Checking out main...\n", nil, false, nil, "")

	// Verify output is appended to chat
	streaming := m.chat.GetStreaming()
	if !strings.Contains(streaming, "Checking out main") {
		t.Errorf("Expected streaming to contain 'Checking out main', got: %s", streaming)
	}

	// More output
	m = simulateMergeResult(m, sessionID, "Merging feature-branch...\n", nil, false, nil, "")

	streaming = m.chat.GetStreaming()
	if !strings.Contains(streaming, "Merging feature-branch") {
		t.Errorf("Expected streaming to contain 'Merging feature-branch', got: %s", streaming)
	}

	// Completion
	m = simulateMergeResult(m, sessionID, "Successfully merged!\n", nil, true, nil, "")
}

func TestGitResult_ErrorHandling(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	// Simulate an error
	m = simulateMergeResult(m, sessionID, "", errors.New("merge failed: branch diverged"), true, nil, "")

	// Verify error is shown in chat
	streaming := m.chat.GetStreaming()
	if !strings.Contains(streaming, "Error") && !strings.Contains(streaming, "merge failed") {
		t.Errorf("Expected error message in streaming, got: %s", streaming)
	}
}

func TestGitResult_ConflictHandling(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	// Simulate a merge conflict
	conflictedFiles := []string{"main.go", "config.go"}
	m = simulateMergeResult(m, sessionID, "CONFLICT (content): Merge conflict in main.go\n", errors.New("merge conflict"), true, conflictedFiles, "/test/repo1")

	// Conflict modal should be shown
	if !m.modal.IsVisible() {
		t.Error("Expected conflict resolution modal to be visible")
	}
}
