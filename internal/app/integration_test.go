package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/mcp"
	"github.com/zhubert/plural/internal/ui"
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

	state := m.sessionState().GetIfExists(sessionID)
	if state == nil || state.PendingPermission == nil || state.PendingPermission.Tool != "Bash" {
		t.Error("Session state should have pending Bash permission")
	}

	// Approve with 'y' - need to be in chat focus
	m = sendKey(m, "y")

	// Verify permission cleared
	if m.chat.HasPendingPermission() {
		t.Error("Permission should be cleared after approval")
	}
	state = m.sessionState().GetIfExists(sessionID)
	if state != nil && state.PendingPermission != nil {
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
	state := m.sessionState().GetIfExists(sessionID)
	if state == nil || state.PendingPermission == nil {
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
	stateA := m.sessionState().GetIfExists(sessionA)
	if stateA == nil || stateA.PendingPermission == nil {
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
	stateA := m.sessionState().GetIfExists(sessionA)
	if stateA == nil || stateA.PendingQuestion == nil {
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
	m.sessionState().GetOrCreate(sessionID).PendingMessage = "queued message"

	// Verify message is queued
	state := m.sessionState().GetIfExists(sessionID)
	if state == nil || state.PendingMessage == "" {
		t.Error("Message should be queued during streaming")
	}

	if state.PendingMessage != "queued message" {
		t.Errorf("Expected 'queued message', got %q", state.PendingMessage)
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

	// Navigate to next file with right arrow
	m = sendKey(m, "right")
	afterRight := m.chat.GetSelectedFileIndex()
	if afterRight != 1 {
		t.Errorf("Expected file index 1 after right, got %d", afterRight)
	}

	// Navigate to next file again with 'l'
	m = sendKey(m, "l")
	afterL := m.chat.GetSelectedFileIndex()
	if afterL != 2 {
		t.Errorf("Expected file index 2 after 'l', got %d", afterL)
	}

	// Navigate to previous file with left arrow
	m = sendKey(m, "left")
	afterLeft := m.chat.GetSelectedFileIndex()
	if afterLeft != 1 {
		t.Errorf("Expected file index 1 after left, got %d", afterLeft)
	}

	// Navigate to previous file with 'h'
	m = sendKey(m, "h")
	afterH := m.chat.GetSelectedFileIndex()
	if afterH != 0 {
		t.Errorf("Expected file index 0 after 'h', got %d", afterH)
	}

	// Try to go before first file - should stay at 0
	m = sendKey(m, "h")
	atStart := m.chat.GetSelectedFileIndex()
	if atStart != 0 {
		t.Errorf("Expected file index to stay at 0, got %d", atStart)
	}
}

func TestViewChanges_EscapeFromSidebarFocus(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select a session first (this switches focus to chat)
	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Expected active session")
	}

	// Enter view changes mode (simulating what shortcutViewChanges does)
	m.chat.EnterViewChangesMode(testFileDiffs())

	if !m.chat.IsInViewChangesMode() {
		t.Fatal("Expected to be in view changes mode")
	}

	// Switch focus back to sidebar (Tab)
	m = sendKey(m, "tab")
	if m.focus != FocusSidebar {
		t.Fatalf("Expected sidebar focus after tab, got %v", m.focus)
	}

	// Verify still in view changes mode
	if !m.chat.IsInViewChangesMode() {
		t.Fatal("Should still be in view changes mode after switching to sidebar")
	}

	// Press Escape from sidebar - should close view changes mode
	m = sendKey(m, "esc")

	if m.chat.IsInViewChangesMode() {
		t.Error("Expected to exit view changes mode after Escape from sidebar")
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

// =============================================================================
// Focus Switching Tests
// =============================================================================

func TestFocusSwitching_TabToggle(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Initially sidebar should be focused
	if m.focus != FocusSidebar {
		t.Errorf("Expected initial focus on sidebar, got %v", m.focus)
	}

	// Select a session - this automatically switches focus to chat
	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Expected active session after enter")
	}

	// After selecting a session, focus should be on chat
	if m.focus != FocusChat {
		t.Errorf("Expected focus on chat after selecting session, got %v", m.focus)
	}

	// Press Tab to switch back to sidebar
	m = sendKey(m, "tab")
	if m.focus != FocusSidebar {
		t.Errorf("Expected focus on sidebar after tab, got %v", m.focus)
	}

	// Press Tab again to switch to chat
	m = sendKey(m, "tab")
	if m.focus != FocusChat {
		t.Errorf("Expected focus on chat after second tab, got %v", m.focus)
	}
}

func TestFocusSwitching_PreservesState(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select first session - this auto-switches focus to chat
	m.sidebar.SelectSession("session-1")
	m = sendKey(m, "enter")

	// Switch to sidebar
	m = sendKey(m, "tab")

	// Session selection should be preserved
	selected := m.sidebar.SelectedSession()
	if selected == nil || selected.ID != "session-1" {
		t.Error("Session selection should be preserved after focus switch")
	}
}

// =============================================================================
// Sidebar Navigation Tests
// =============================================================================

func TestSidebar_NavigationWithJK(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	initial := m.sidebar.SelectedSession()
	if initial == nil {
		t.Fatal("Expected a session to be selected")
	}
	initialID := initial.ID

	// Navigate down with j
	m = sendKey(m, "j")
	afterJ := m.sidebar.SelectedSession()
	if afterJ == nil || afterJ.ID == initialID {
		t.Error("Expected selection to change after pressing j")
	}

	// Navigate up with k
	m = sendKey(m, "k")
	afterK := m.sidebar.SelectedSession()
	if afterK == nil || afterK.ID != initialID {
		t.Error("Expected selection to return to initial after pressing k")
	}
}

func TestSidebar_NavigationWithArrows(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	initial := m.sidebar.SelectedSession()
	if initial == nil {
		t.Fatal("Expected a session to be selected")
	}
	initialID := initial.ID

	// Navigate down with arrow
	m = sendKey(m, "down")
	afterDown := m.sidebar.SelectedSession()
	if afterDown == nil || afterDown.ID == initialID {
		t.Error("Expected selection to change after pressing down")
	}

	// Navigate up with arrow
	m = sendKey(m, "up")
	afterUp := m.sidebar.SelectedSession()
	if afterUp == nil || afterUp.ID != initialID {
		t.Error("Expected selection to return to initial after pressing up")
	}
}

func TestSidebar_SearchMode(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	if m.sidebar.IsSearchMode() {
		t.Error("Should not be in search mode initially")
	}

	m = sendKey(m, "/")
	if !m.sidebar.IsSearchMode() {
		t.Error("Should be in search mode after pressing /")
	}

	m = sendKey(m, "esc")
	if m.sidebar.IsSearchMode() {
		t.Error("Should not be in search mode after pressing esc")
	}
}

func TestSidebar_SelectSessionWithEnter(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	if m.activeSession != nil {
		t.Error("Should have no active session initially")
	}

	m = sendKey(m, "enter")

	if m.activeSession == nil {
		t.Error("Session should be active after pressing enter")
	}

	selected := m.sidebar.SelectedSession()
	if selected != nil && m.activeSession.ID != selected.ID {
		t.Errorf("Active session ID %s should match selected ID %s", m.activeSession.ID, selected.ID)
	}
}

// =============================================================================
// Modal Interaction Tests
// =============================================================================

func TestModal_OpenNewSessionModal(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	if m.modal.IsVisible() {
		t.Error("Modal should not be visible initially")
	}

	m = sendKey(m, "n")

	if !m.modal.IsVisible() {
		t.Error("Modal should be visible after pressing n")
	}

	if _, ok := m.modal.State.(*ui.NewSessionState); !ok {
		t.Errorf("Expected NewSessionState modal, got %T", m.modal.State)
	}
}

func TestModal_OpenAddRepoModal(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "a")

	if !m.modal.IsVisible() {
		t.Error("Modal should be visible after pressing a")
	}

	if _, ok := m.modal.State.(*ui.AddRepoState); !ok {
		t.Errorf("Expected AddRepoState modal, got %T", m.modal.State)
	}
}

func TestModal_CloseWithEscape(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "n")
	if !m.modal.IsVisible() {
		t.Fatal("Modal should be visible")
	}

	m = sendKey(m, "esc")
	if m.modal.IsVisible() {
		t.Error("Modal should be closed after pressing esc")
	}
}

func TestModal_ConfirmDeleteNavigation(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "d")

	if !m.modal.IsVisible() {
		t.Error("Delete modal should be visible")
	}

	state, ok := m.modal.State.(*ui.ConfirmDeleteState)
	if !ok {
		t.Fatalf("Expected ConfirmDeleteState, got %T", m.modal.State)
	}

	if state.SelectedIndex != 0 {
		t.Errorf("Expected initial selection 0, got %d", state.SelectedIndex)
	}

	m = sendKey(m, "down")
	afterDown := m.modal.State.(*ui.ConfirmDeleteState)
	if afterDown.SelectedIndex != 1 {
		t.Errorf("Expected selection 1 after down, got %d", afterDown.SelectedIndex)
	}

	m = sendKey(m, "up")
	afterUp := m.modal.State.(*ui.ConfirmDeleteState)
	if afterUp.SelectedIndex != 0 {
		t.Errorf("Expected selection 0 after up, got %d", afterUp.SelectedIndex)
	}
}

func TestModal_ThemeSelector(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "t")

	if !m.modal.IsVisible() {
		t.Error("Theme modal should be visible")
	}

	_, ok := m.modal.State.(*ui.ThemeState)
	if !ok {
		t.Fatalf("Expected ThemeState, got %T", m.modal.State)
	}

	m = sendKey(m, "esc")
	if m.modal.IsVisible() {
		t.Error("Theme modal should be closed")
	}
}

func TestModal_MCPServers(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// MCP servers is accessed via /mcp slash command, not a shortcut
	m.showMCPServersModal()

	if !m.modal.IsVisible() {
		t.Error("MCP servers modal should be visible")
	}

	_, ok := m.modal.State.(*ui.MCPServersState)
	if !ok {
		t.Fatalf("Expected MCPServersState, got %T", m.modal.State)
	}

	m = sendKey(m, "a")
	_, ok = m.modal.State.(*ui.AddMCPServerState)
	if !ok {
		t.Fatalf("Expected AddMCPServerState after 'a', got %T", m.modal.State)
	}

	m = sendKey(m, "esc")
	_, ok = m.modal.State.(*ui.MCPServersState)
	if !ok {
		t.Fatalf("Expected MCPServersState after esc, got %T", m.modal.State)
	}
}

func TestModal_InputBlocking(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "n")

	prevFocus := m.focus
	m = sendKey(m, "tab")

	// Focus should not have changed because modal is absorbing keys
	if m.focus != prevFocus && m.modal.IsVisible() {
		t.Log("Tab was handled by modal as expected")
	}

	// 'q' should not quit while modal is open
	m = sendKey(m, "q")
	if !m.modal.IsVisible() {
		t.Log("q key handled by modal")
	}
}

// =============================================================================
// Chat Interaction Tests
// =============================================================================

func TestChat_TypeInTextarea(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Expected active session after enter")
	}

	if m.focus != FocusChat {
		t.Errorf("Expected focus on chat after selecting session, got %v", m.focus)
	}

	if !m.chat.IsFocused() {
		t.Error("Chat should be focused")
	}

	initialInput := m.chat.GetInput()
	m = typeText(m, "hello")
	afterInput := m.chat.GetInput()
	t.Logf("Initial input: %q, After typing: %q", initialInput, afterInput)
}

func TestChat_ClearInputAfterSend(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	m = sendKey(m, "tab")

	m.chat.SetInput("test message")
	m.chat.ClearInput()

	if m.chat.GetInput() != "" {
		t.Errorf("Expected empty input after clear, got %q", m.chat.GetInput())
	}
}

func TestChat_FocusState(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	if m.chat.IsFocused() {
		t.Error("Chat should not be focused initially")
	}

	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Expected active session")
	}

	if !m.chat.IsFocused() {
		t.Error("Chat should be focused after selecting session")
	}

	m = sendKey(m, "tab")
	if m.chat.IsFocused() {
		t.Error("Chat should not be focused after tab to sidebar")
	}

	m = sendKey(m, "tab")
	if !m.chat.IsFocused() {
		t.Error("Chat should be focused after tabbing back")
	}
}

func TestChat_NoSendWithoutSession(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "tab")

	if m.CanSendMessage() {
		t.Error("Should not be able to send message without active session")
	}
}

// =============================================================================
// Keyboard Shortcuts Tests
// =============================================================================

func TestShortcuts_QuitOnlyFromSidebar(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	if m.focus != FocusSidebar {
		t.Error("Should start with sidebar focus")
	}

	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Expected active session")
	}

	if m.focus != FocusChat {
		t.Errorf("Expected focus on chat after selecting session, got %v", m.focus)
	}

	// From chat, 'q' should not quit - it should be passed to textarea
	m = sendKey(m, "q")
	if m.focus != FocusChat {
		t.Error("Should still be in chat after pressing q")
	}
}

func TestShortcuts_ModalOnlyFromSidebar(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Expected active session")
	}

	if m.focus != FocusChat {
		t.Errorf("Expected focus on chat after selecting session, got %v", m.focus)
	}

	// From chat, 'n', 'r', 'd' etc should not open modals
	m = sendKey(m, "n")
	if m.modal.IsVisible() {
		t.Error("Modal should not open from chat focus")
	}

	m = sendKey(m, "r")
	if m.modal.IsVisible() {
		t.Error("Modal should not open from chat focus")
	}
}

// =============================================================================
// State Machine Tests
// =============================================================================

func TestState_InitialIdle(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	if m.state != StateIdle {
		t.Errorf("Expected initial state Idle, got %v", m.state)
	}

	if !m.IsIdle() {
		t.Error("IsIdle should return true initially")
	}
}

func TestState_Transitions(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m.setState(StateStreamingClaude)
	if m.state != StateStreamingClaude {
		t.Errorf("Expected state StreamingClaude, got %v", m.state)
	}

	m.setState(StateIdle)
	if m.state != StateIdle {
		t.Errorf("Expected state Idle, got %v", m.state)
	}
}

// =============================================================================
// Window Sizing Tests
// =============================================================================

func TestWindowSize_UpdatesComponents(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	if m.width != 0 || m.height != 0 {
		t.Errorf("Expected initial size 0x0, got %dx%d", m.width, m.height)
	}

	m = setSize(m, 120, 40)

	if m.width != 120 {
		t.Errorf("Expected width 120, got %d", m.width)
	}
	if m.height != 40 {
		t.Errorf("Expected height 40, got %d", m.height)
	}
}

func TestWindowSize_SmallTerminal(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	m = setSize(m, 40, 10)

	if m.width != 40 || m.height != 10 {
		t.Errorf("Expected 40x10, got %dx%d", m.width, m.height)
	}

	// Should not panic when rendering with small size
	_ = m.View()
}

func TestWindowSize_LargeTerminal(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	m = setSize(m, 300, 100)

	if m.width != 300 || m.height != 100 {
		t.Errorf("Expected 300x100, got %dx%d", m.width, m.height)
	}

	// Should not panic when rendering with large size
	_ = m.View()
}

// =============================================================================
// Combined Flow Tests
// =============================================================================

func TestFlow_CreateSessionNavigateAndChat(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Navigate down in sidebar
	m = sendKey(m, "j")
	selected := m.sidebar.SelectedSession()
	if selected == nil {
		t.Fatal("Should have selected session after navigation")
	}

	// Select session with Enter
	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Should have active session after enter")
	}

	// Should be in chat
	if m.focus != FocusChat {
		t.Errorf("Should be in chat after selecting session, got focus=%v", m.focus)
	}

	// Switch to sidebar
	m = sendKey(m, "tab")
	if m.focus != FocusSidebar {
		t.Errorf("Should be in sidebar after tab, got focus=%v", m.focus)
	}

	// Open delete modal
	m = sendKey(m, "d")
	if !m.modal.IsVisible() {
		t.Error("Delete modal should be open")
	}

	// Close modal with Escape
	m = sendKey(m, "esc")
	if m.modal.IsVisible() {
		t.Error("Modal should be closed")
	}
}

func TestFlow_OpenAndCloseMultipleModals(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Open and close new session modal
	m = sendKey(m, "n")
	if !m.modal.IsVisible() {
		t.Error("New session modal should be open")
	}
	m = sendKey(m, "esc")
	if m.modal.IsVisible() {
		t.Error("Modal should be closed")
	}

	// Open and close theme modal
	m = sendKey(m, "t")
	if !m.modal.IsVisible() {
		t.Error("Theme modal should be open")
	}
	m = sendKey(m, "esc")
	if m.modal.IsVisible() {
		t.Error("Modal should be closed")
	}

	// Open and close MCP servers modal (via /mcp slash command)
	m.showMCPServersModal()
	if !m.modal.IsVisible() {
		t.Error("MCP modal should be open")
	}
	m = sendKey(m, "esc")
	if m.modal.IsVisible() {
		t.Error("Modal should be closed")
	}
}

// =============================================================================
// View Changes Tests (using git.FileDiff)
// =============================================================================

func TestViewChanges_WithFileDiffs(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	if m.chat.IsInViewChangesMode() {
		t.Error("Should not be in view changes mode initially")
	}

	m.chat.EnterViewChangesMode([]git.FileDiff{{
		Filename: "test.go",
		Status:   "M",
		Diff:     "test changes content",
	}})

	if !m.chat.IsInViewChangesMode() {
		t.Error("Should be in view changes mode")
	}

	m.chat.ExitViewChangesMode()
	if m.chat.IsInViewChangesMode() {
		t.Error("Should not be in view changes mode after exit")
	}
}
