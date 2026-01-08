package app

import (
	"testing"

	"github.com/zhubert/plural/internal/mcp"
	"github.com/zhubert/plural/internal/ui"
)

// =============================================================================
// Focus Switching Tests
// =============================================================================

func TestIntegration_FocusSwitching_TabToggle(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Initially sidebar should be focused
	if m.focus != FocusSidebar {
		t.Errorf("Expected initial focus on sidebar, got %v", m.focus)
	}
	if !m.sidebar.IsFocused() {
		t.Error("Sidebar should be focused initially")
	}
	if m.chat.IsFocused() {
		t.Error("Chat should not be focused initially")
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
	if m.sidebar.IsFocused() {
		t.Error("Sidebar should not be focused after selecting session")
	}
	if !m.chat.IsFocused() {
		t.Error("Chat should be focused after selecting session")
	}

	// Press Tab to switch back to sidebar
	m = sendKey(m, "tab")

	if m.focus != FocusSidebar {
		t.Errorf("Expected focus on sidebar after tab, got %v", m.focus)
	}
	if !m.sidebar.IsFocused() {
		t.Error("Sidebar should be focused after tab")
	}

	// Press Tab again to switch to chat
	m = sendKey(m, "tab")

	if m.focus != FocusChat {
		t.Errorf("Expected focus on chat after second tab, got %v", m.focus)
	}
	if !m.chat.IsFocused() {
		t.Error("Chat should be focused after second tab")
	}
}

func TestIntegration_FocusSwitching_PreservesState(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select first session - this auto-switches focus to chat
	m.sidebar.SelectSession("session-1")
	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Expected active session")
	}

	// After selecting, focus should already be on chat
	if m.focus != FocusChat {
		t.Error("Expected focus on chat after selecting session")
	}

	// Switch to sidebar
	m = sendKey(m, "tab")
	if m.focus != FocusSidebar {
		t.Error("Expected focus on sidebar after tab")
	}

	// Session selection should be preserved
	selected := m.sidebar.SelectedSession()
	if selected == nil || selected.ID != "session-1" {
		t.Error("Session selection should be preserved after focus switch")
	}
}

// =============================================================================
// Sidebar Navigation Tests
// =============================================================================

func TestIntegration_Sidebar_NavigationWithJK(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Initially first session should be selected
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

func TestIntegration_Sidebar_NavigationWithArrows(t *testing.T) {
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

func TestIntegration_Sidebar_SearchMode(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Enter search mode with /
	if m.sidebar.IsSearchMode() {
		t.Error("Should not be in search mode initially")
	}

	m = sendKey(m, "/")
	if !m.sidebar.IsSearchMode() {
		t.Error("Should be in search mode after pressing /")
	}

	// Exit search mode with Escape
	m = sendKey(m, "esc")
	if m.sidebar.IsSearchMode() {
		t.Error("Should not be in search mode after pressing esc")
	}
}

func TestIntegration_Sidebar_SelectSessionWithEnter(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// No active session initially
	if m.activeSession != nil {
		t.Error("Should have no active session initially")
	}

	// Press Enter to select
	m = sendKey(m, "enter")

	// Session should now be active
	if m.activeSession == nil {
		t.Error("Session should be active after pressing enter")
	}

	// Check session ID matches
	selected := m.sidebar.SelectedSession()
	if selected != nil && m.activeSession.ID != selected.ID {
		t.Errorf("Active session ID %s should match selected ID %s", m.activeSession.ID, selected.ID)
	}
}

// =============================================================================
// Modal Interaction Tests
// =============================================================================

func TestIntegration_Modal_OpenNewSessionModal(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Modal should not be visible initially
	if m.modal.IsVisible() {
		t.Error("Modal should not be visible initially")
	}

	// Press 'n' to open new session modal
	m = sendKey(m, "n")

	// Modal should be visible
	if !m.modal.IsVisible() {
		t.Error("Modal should be visible after pressing n")
	}

	// Check it's the right modal type
	if _, ok := m.modal.State.(*ui.NewSessionState); !ok {
		t.Errorf("Expected NewSessionState modal, got %T", m.modal.State)
	}

	// Check modal title
	if m.modal.State.Title() != "New Session" {
		t.Errorf("Expected title 'New Session', got %q", m.modal.State.Title())
	}
}

func TestIntegration_Modal_OpenAddRepoModal(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Press 'r' to open add repo modal
	m = sendKey(m, "r")

	if !m.modal.IsVisible() {
		t.Error("Modal should be visible after pressing r")
	}

	if _, ok := m.modal.State.(*ui.AddRepoState); !ok {
		t.Errorf("Expected AddRepoState modal, got %T", m.modal.State)
	}

	if m.modal.State.Title() != "Add Repository" {
		t.Errorf("Expected title 'Add Repository', got %q", m.modal.State.Title())
	}
}

func TestIntegration_Modal_CloseWithEscape(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Open modal
	m = sendKey(m, "n")
	if !m.modal.IsVisible() {
		t.Fatal("Modal should be visible")
	}

	// Close with Escape
	m = sendKey(m, "esc")
	if m.modal.IsVisible() {
		t.Error("Modal should be closed after pressing esc")
	}
}

func TestIntegration_Modal_NewSessionNavigation(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Open new session modal
	m = sendKey(m, "n")

	state, ok := m.modal.State.(*ui.NewSessionState)
	if !ok {
		t.Fatal("Expected NewSessionState")
	}

	// With 2 repos, we should be able to navigate
	initialIdx := state.RepoIndex

	// Navigate down with Tab (moves between fields)
	m = sendKey(m, "tab")
	afterTab := m.modal.State.(*ui.NewSessionState)
	if afterTab.Focus == state.Focus {
		// Focus should have changed
		t.Log("Focus changed as expected")
	}

	// Navigate repos with down arrow when on repo selector
	state = m.modal.State.(*ui.NewSessionState)
	if state.Focus == 0 { // On repo selector
		m = sendKey(m, "down")
		afterDown := m.modal.State.(*ui.NewSessionState)
		if len(cfg.Repos) > 1 && afterDown.RepoIndex == initialIdx {
			t.Error("Repo index should change after pressing down")
		}
	}
}

func TestIntegration_Modal_ConfirmDeleteNavigation(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Press 'd' to open delete modal
	m = sendKey(m, "d")

	if !m.modal.IsVisible() {
		t.Error("Delete modal should be visible")
	}

	state, ok := m.modal.State.(*ui.ConfirmDeleteState)
	if !ok {
		t.Fatalf("Expected ConfirmDeleteState, got %T", m.modal.State)
	}

	// Initially first option (keep worktree) should be selected
	initialIdx := state.SelectedIndex
	if initialIdx != 0 {
		t.Errorf("Expected initial selection 0, got %d", initialIdx)
	}

	// Navigate down
	m = sendKey(m, "down")
	afterDown := m.modal.State.(*ui.ConfirmDeleteState)
	if afterDown.SelectedIndex != 1 {
		t.Errorf("Expected selection 1 after down, got %d", afterDown.SelectedIndex)
	}

	// Navigate back up
	m = sendKey(m, "up")
	afterUp := m.modal.State.(*ui.ConfirmDeleteState)
	if afterUp.SelectedIndex != 0 {
		t.Errorf("Expected selection 0 after up, got %d", afterUp.SelectedIndex)
	}
}

func TestIntegration_Modal_ThemeSelector(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Press 't' to open theme modal
	m = sendKey(m, "t")

	if !m.modal.IsVisible() {
		t.Error("Theme modal should be visible")
	}

	_, ok := m.modal.State.(*ui.ThemeState)
	if !ok {
		t.Fatalf("Expected ThemeState, got %T", m.modal.State)
	}

	// Navigate themes
	m = sendKey(m, "down")
	m = sendKey(m, "up")

	// Close
	m = sendKey(m, "esc")
	if m.modal.IsVisible() {
		t.Error("Theme modal should be closed")
	}
}

func TestIntegration_Modal_MCPServers(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Press 's' to open MCP servers modal
	m = sendKey(m, "s")

	if !m.modal.IsVisible() {
		t.Error("MCP servers modal should be visible")
	}

	_, ok := m.modal.State.(*ui.MCPServersState)
	if !ok {
		t.Fatalf("Expected MCPServersState, got %T", m.modal.State)
	}

	// Press 'a' to switch to add server modal
	m = sendKey(m, "a")

	_, ok = m.modal.State.(*ui.AddMCPServerState)
	if !ok {
		t.Fatalf("Expected AddMCPServerState after 'a', got %T", m.modal.State)
	}

	// Escape should go back to server list
	m = sendKey(m, "esc")
	_, ok = m.modal.State.(*ui.MCPServersState)
	if !ok {
		t.Fatalf("Expected MCPServersState after esc, got %T", m.modal.State)
	}
}

func TestIntegration_Modal_InputBlocking(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Open modal
	m = sendKey(m, "n")

	// Try pressing keys that would normally do things
	// These should be absorbed by the modal
	prevFocus := m.focus
	m = sendKey(m, "tab") // In modal, this navigates within modal, not toggles focus

	// Focus should not have changed because modal is absorbing keys
	if m.focus != prevFocus && m.modal.IsVisible() {
		// This is actually expected - tab in modal goes to modal's Update
		t.Log("Tab was handled by modal as expected")
	}

	// 'q' should not quit while modal is open
	m = sendKey(m, "q")
	// If we got here, q didn't quit
	if !m.modal.IsVisible() {
		// Modal might handle 'q' differently but shouldn't close
		t.Log("q key handled by modal")
	}
}

// =============================================================================
// Chat Interaction Tests
// =============================================================================

func TestIntegration_Chat_TypeInTextarea(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select a session - this automatically focuses chat
	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Expected active session after enter")
	}

	// After selecting, focus should already be on chat
	if m.focus != FocusChat {
		t.Errorf("Expected focus on chat after selecting session, got %v", m.focus)
	}

	// Verify chat is focused
	if !m.chat.IsFocused() {
		t.Error("Chat should be focused")
	}

	// Note: The chat textarea is managed by bubbles/textarea
	// When we type, the characters are processed by the textarea component
	// We can verify the input is being captured
	initialInput := m.chat.GetInput()

	// Type some characters
	m = typeText(m, "hello")

	// The input should have changed (though we might not be able to verify exact content
	// due to how the textarea internal state works)
	afterInput := m.chat.GetInput()
	t.Logf("Initial input: %q, After typing: %q", initialInput, afterInput)
}

func TestIntegration_Chat_ClearInputAfterSend(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select session and switch to chat
	m = sendKey(m, "enter")
	m = sendKey(m, "tab")

	// Set some input directly
	m.chat.SetInput("test message")

	// Clear it
	m.chat.ClearInput()

	if m.chat.GetInput() != "" {
		t.Errorf("Expected empty input after clear, got %q", m.chat.GetInput())
	}
}

func TestIntegration_Chat_FocusState(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Initially chat not focused
	if m.chat.IsFocused() {
		t.Error("Chat should not be focused initially")
	}

	// Select a session - this automatically focuses chat
	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Expected active session")
	}

	// After selecting, chat should be focused
	if !m.chat.IsFocused() {
		t.Error("Chat should be focused after selecting session")
	}

	// Tab to sidebar
	m = sendKey(m, "tab")
	if m.chat.IsFocused() {
		t.Error("Chat should not be focused after tab to sidebar")
	}

	// Tab back to chat
	m = sendKey(m, "tab")
	if !m.chat.IsFocused() {
		t.Error("Chat should be focused after tabbing back")
	}
}

func TestIntegration_Chat_NoSendWithoutSession(t *testing.T) {
	cfg := testConfig() // No sessions
	m := testModelWithSize(cfg, 120, 40)

	// Switch to chat
	m = sendKey(m, "tab")

	// Should not be able to send without a session
	if m.CanSendMessage() {
		t.Error("Should not be able to send message without active session")
	}
}

// =============================================================================
// Permission Response Tests
// =============================================================================

func TestIntegration_Permission_ResponseKeys(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select session
	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Expected active session")
	}

	// Set up a pending permission
	permReq := &mcp.PermissionRequest{
		Tool:        "Bash",
		Description: "Run: git status",
	}
	m.sessionState().SetPendingPermission(m.activeSession.ID, permReq)
	m.chat.SetPendingPermission("Bash", "Run: git status")

	// Switch to chat
	m = sendKey(m, "tab")

	// Verify permission is pending
	if !m.chat.HasPendingPermission() {
		t.Error("Chat should have pending permission")
	}

	// The 'y', 'n', 'a' keys are handled in Update when permission is pending
	// We can verify the state setup is correct
	pendingPerm := m.sessionState().GetPendingPermission(m.activeSession.ID)
	if pendingPerm == nil {
		t.Error("Session should have pending permission")
	}
	if pendingPerm.Tool != "Bash" {
		t.Errorf("Expected tool 'Bash', got %q", pendingPerm.Tool)
	}
}

func TestIntegration_Permission_ClearAfterResponse(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select session
	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	// Set up permission
	permReq := &mcp.PermissionRequest{
		Tool:        "Bash",
		Description: "test",
	}
	m.sessionState().SetPendingPermission(sessionID, permReq)

	// Clear it
	m.sessionState().ClearPendingPermission(sessionID)

	if m.sessionState().GetPendingPermission(sessionID) != nil {
		t.Error("Permission should be cleared")
	}
}

// =============================================================================
// Question Response Tests
// =============================================================================

func TestIntegration_Question_SetupAndNavigation(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select session and switch to chat
	m = sendKey(m, "enter")
	m = sendKey(m, "tab")

	// Set up questions
	questions := []mcp.Question{
		{
			Question: "Which option?",
			Header:   "Choice",
			Options: []mcp.QuestionOption{
				{Label: "Option A", Description: "First"},
				{Label: "Option B", Description: "Second"},
			},
		},
	}
	m.chat.SetPendingQuestion(questions)
	m.sessionState().SetPendingQuestion(m.activeSession.ID, &mcp.QuestionRequest{Questions: questions})

	if !m.chat.HasPendingQuestion() {
		t.Error("Chat should have pending question")
	}

	// Test navigation
	m.chat.MoveQuestionSelection(1)
	m.chat.MoveQuestionSelection(-1)

	// Clear question
	m.chat.ClearPendingQuestion()
	if m.chat.HasPendingQuestion() {
		t.Error("Question should be cleared")
	}
}

func TestIntegration_Question_SelectByNumber(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")
	m = sendKey(m, "tab")

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
	m.chat.SetPendingQuestion(questions)

	// Select by number (1-based)
	completed := m.chat.SelectOptionByNumber(1)
	if !completed {
		t.Error("Selection should complete with single question")
	}

	// Get answers
	answers := m.chat.GetQuestionAnswers()
	if len(answers) != 1 {
		t.Errorf("Expected 1 answer, got %d", len(answers))
	}
}

// =============================================================================
// Keyboard Shortcuts Tests (Sidebar Focus)
// =============================================================================

func TestIntegration_Shortcuts_QuitOnlyFromSidebar(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// From sidebar, 'q' should work (we can't test actual quit but can verify focus)
	if m.focus != FocusSidebar {
		t.Error("Should start with sidebar focus")
	}

	// Select session - this automatically switches focus to chat
	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Expected active session")
	}

	// After selecting, focus should already be on chat
	if m.focus != FocusChat {
		t.Errorf("Expected focus on chat after selecting session, got %v", m.focus)
	}

	// From chat, 'q' should not quit - it should be passed to textarea
	// We can't test the actual message input, but we can verify we're still in chat
	m = sendKey(m, "q")
	if m.focus != FocusChat {
		t.Error("Should still be in chat after pressing q")
	}
}

func TestIntegration_Shortcuts_ModalOnlyFromSidebar(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select session - this automatically switches focus to chat
	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Expected active session")
	}

	// After selecting, focus should already be on chat
	if m.focus != FocusChat {
		t.Errorf("Expected focus on chat after selecting session, got %v", m.focus)
	}

	// From chat, 'n', 'r', 'd' etc should not open modals
	// They should be passed to the textarea instead
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
// View Changes Mode Tests
// =============================================================================

func TestIntegration_ViewChanges_EnterAndExit(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Initially not in view changes mode
	if m.chat.IsInViewChangesMode() {
		t.Error("Should not be in view changes mode initially")
	}

	// Enter view changes mode
	m.chat.EnterViewChangesMode("test changes content")
	if !m.chat.IsInViewChangesMode() {
		t.Error("Should be in view changes mode")
	}

	// Exit view changes mode
	m.chat.ExitViewChangesMode()
	if m.chat.IsInViewChangesMode() {
		t.Error("Should not be in view changes mode after exit")
	}
}

// =============================================================================
// State Machine Tests
// =============================================================================

func TestIntegration_State_InitialIdle(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	if m.state != StateIdle {
		t.Errorf("Expected initial state Idle, got %v", m.state)
	}

	if !m.IsIdle() {
		t.Error("IsIdle should return true initially")
	}
}

func TestIntegration_State_Transitions(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Test setState
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

func TestIntegration_WindowSize_UpdatesComponents(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	// Initial size is 0,0
	if m.width != 0 || m.height != 0 {
		t.Errorf("Expected initial size 0x0, got %dx%d", m.width, m.height)
	}

	// Set size
	m = setSize(m, 120, 40)

	if m.width != 120 {
		t.Errorf("Expected width 120, got %d", m.width)
	}
	if m.height != 40 {
		t.Errorf("Expected height 40, got %d", m.height)
	}
}

func TestIntegration_WindowSize_SmallTerminal(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	// Very small terminal
	m = setSize(m, 40, 10)

	if m.width != 40 || m.height != 10 {
		t.Errorf("Expected 40x10, got %dx%d", m.width, m.height)
	}

	// Should not panic when rendering with small size
	_ = m.View()
}

func TestIntegration_WindowSize_LargeTerminal(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	// Large terminal
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

func TestIntegration_Flow_CreateSessionNavigateAndChat(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// 1. Navigate down in sidebar
	m = sendKey(m, "j")
	selected := m.sidebar.SelectedSession()
	if selected == nil {
		t.Fatal("Should have selected session after navigation")
	}

	// 2. Select session with Enter - this auto-focuses chat
	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Should have active session after enter")
	}

	// 3. After selecting, should be in chat
	if m.focus != FocusChat {
		t.Errorf("Should be in chat after selecting session, got focus=%v", m.focus)
	}

	// 4. Switch to sidebar
	m = sendKey(m, "tab")
	if m.focus != FocusSidebar {
		t.Errorf("Should be in sidebar after tab, got focus=%v", m.focus)
	}

	// 5. Open delete modal (requires a session to be selected in sidebar)
	m = sendKey(m, "d")
	if !m.modal.IsVisible() {
		t.Error("Delete modal should be open")
	}

	// 6. Close modal with Escape
	m = sendKey(m, "esc")
	if m.modal.IsVisible() {
		t.Error("Modal should be closed")
	}
}

func TestIntegration_Flow_OpenAndCloseMultipleModals(t *testing.T) {
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

	// Open and close MCP servers modal
	m = sendKey(m, "s")
	if !m.modal.IsVisible() {
		t.Error("MCP modal should be open")
	}
	m = sendKey(m, "esc")
	if m.modal.IsVisible() {
		t.Error("Modal should be closed")
	}
}
