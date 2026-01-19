package app

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/mcp"
	"github.com/zhubert/plural/internal/ui"
)

// Update handles messages. This is the core Bubble Tea update function that routes
// all messages to appropriate handlers.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateSizes()

	case tea.FocusMsg:
		m.windowFocused = true
		logger.Log("App: Window focused")

	case tea.BlurMsg:
		m.windowFocused = false
		logger.Log("App: Window blurred")

	case tea.PasteStartMsg:
		return m.handlePasteStart()

	case tea.PasteMsg:
		return m.handlePaste(msg)

	case tea.KeyPressMsg:
		if result, cmd := m.handleKeyPress(msg); result != nil {
			return result, cmd
		}
		// Key not handled by handleKeyPress, let it fall through to focused panel

	case ClaudeResponseMsg:
		return m.handleClaudeResponseMsg(msg)

	case PermissionRequestMsg:
		return m.handlePermissionRequestMsg(msg)

	case QuestionRequestMsg:
		return m.handleQuestionRequestMsg(msg)

	case PlanApprovalRequestMsg:
		return m.handlePlanApprovalRequestMsg(msg)

	case CommitMessageGeneratedMsg:
		return m.handleCommitMessageGenerated(msg)

	case SendPendingMessageMsg:
		return m.handleSendPendingMessageMsg(msg)

	case MergeResultMsg:
		return m.handleMergeResultMsg(msg)

	case GitHubIssuesFetchedMsg:
		return m.handleGitHubIssuesFetchedMsg(msg)

	case ChangelogFetchedMsg:
		return m.handleChangelogFetchedMsg(msg)

	case StartupModalMsg:
		return m.handleStartupModals()

	case ui.HelpShortcutTriggeredMsg:
		return m.handleHelpShortcutTrigger(msg.Key)

	case TerminalErrorMsg:
		return m.handleTerminalError(msg)
	}

	// Update modal
	if m.modal.IsVisible() {
		modal, cmd := m.modal.Update(msg)
		m.modal = modal
		cmds = append(cmds, cmd)
	}

	// Handle tick messages - both panels need these regardless of focus
	if cmd := m.handleTickMessages(msg); cmd != nil {
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	// Route scroll/mouse events to appropriate panel
	if cmd := m.routeScrollAndMouseEvents(msg); cmd != nil {
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	// Update focused panel for other messages
	if m.focus == FocusSidebar {
		sidebar, cmd := m.sidebar.Update(msg)
		m.sidebar = sidebar
		cmds = append(cmds, cmd)
	} else {
		chat, cmd := m.chat.Update(msg)
		m.chat = chat
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// handlePasteStart handles paste events - check for images in clipboard when paste starts
func (m *Model) handlePasteStart() (tea.Model, tea.Cmd) {
	logger.Log("App: PasteStartMsg received, focus=%v, hasActiveSession=%v", m.focus, m.activeSession != nil)
	if m.focus == FocusChat && m.activeSession != nil {
		model, cmd := m.handleImagePaste()
		if m.chat.HasPendingImage() {
			// Image was attached, don't process text paste
			return model, cmd
		}
		// No image found, let text paste proceed normally
	}
	return m, nil
}

// handlePaste logs paste content for debugging
func (m *Model) handlePaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	content := msg.Content
	preview := content
	if len(preview) > 100 {
		preview = preview[:100] + "..."
	}
	logger.Log("App: PasteMsg received: len=%d, preview=%q", len(content), preview)
	return m, nil
}

// handleKeyPress handles all keyboard input.
// Returns (model, cmd) if the key was handled, or (nil, nil) if it should fall through
// to the focused panel for handling.
func (m *Model) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	logger.Log("App: KeyPressMsg received: key=%q, focus=%v, modalVisible=%v", msg.String(), m.focus, m.modal.IsVisible())

	// Handle modal first if visible
	if m.modal.IsVisible() {
		return m.handleModalKey(msg)
	}

	// Handle Escape for various exit scenarios
	if msg.String() == "esc" {
		if result, cmd, handled := m.handleEscapeKey(); handled {
			return result, cmd
		}
	}

	// Handle chat-focused keys when chat is focused with an active session
	if m.focus == FocusChat && m.activeSession != nil {
		if result, cmd, handled := m.handleChatFocusedKeys(msg); handled {
			return result, cmd
		}
	}

	// Global keys
	key := msg.String()

	// Handle ctrl+c specially - always quits
	if key == "ctrl+c" {
		return m, tea.Quit
	}

	// Try executing from shortcut registry
	if result, cmd, handled := m.ExecuteShortcut(key); handled {
		return result, cmd
	}

	// Handle enter key
	if key == "enter" {
		return m.handleEnterKey()
	}

	// Key not handled - return nil to signal it should fall through to focused panel
	return nil, nil
}

// handleEscapeKey handles escape key for exiting search mode, view changes, or interrupting streaming
func (m *Model) handleEscapeKey() (tea.Model, tea.Cmd, bool) {
	// First check if sidebar is in search mode
	if m.sidebar.IsSearchMode() {
		m.sidebar.ExitSearchMode()
		return m, nil, true
	}
	// Check if view changes mode is active (regardless of focus)
	if m.chat.IsInViewChangesMode() {
		m.chat.ExitViewChangesMode()
		return m, nil, true
	}
	// Then check for streaming interruption
	if m.activeSession != nil {
		if cancel := m.sessionState().GetStreamCancel(m.activeSession.ID); cancel != nil {
			return m.interruptStreaming()
		}
	}
	return m, nil, false
}

// interruptStreaming interrupts the current streaming session
func (m *Model) interruptStreaming() (tea.Model, tea.Cmd, bool) {
	logger.Log("App: Interrupting streaming for session %s", m.activeSession.ID)
	cancel := m.sessionState().GetStreamCancel(m.activeSession.ID)
	if cancel != nil {
		cancel()
	}
	// Send SIGINT to interrupt the Claude process (handles sub-agent work)
	if m.claudeRunner != nil {
		if err := m.claudeRunner.Interrupt(); err != nil {
			logger.Error("App: Failed to interrupt Claude: %v", err)
		}
	}
	m.sessionState().StopWaiting(m.activeSession.ID)
	m.sidebar.SetStreaming(m.activeSession.ID, false)
	m.chat.SetWaiting(false)
	// Save partial response to runner before finishing
	if content := m.chat.GetStreaming(); content != "" {
		m.claudeRunner.AddAssistantMessage(content + "\n[Interrupted]")
		m.sessionMgr.SaveRunnerMessages(m.activeSession.ID, m.claudeRunner)
	}
	m.chat.AppendStreaming("\n[Interrupted]\n")
	m.chat.FinishStreaming()
	// Check if any sessions are still streaming
	if !m.hasAnyStreamingSessions() {
		m.setState(StateIdle)
	}
	return m, nil, true
}

// handleChatFocusedKeys handles keys when chat panel is focused
func (m *Model) handleChatFocusedKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd, bool) {
	key := msg.String()

	// Permission response
	if req := m.sessionState().GetPendingPermission(m.activeSession.ID); req != nil {
		switch key {
		case "y", "Y", "n", "N", "a", "A":
			result, cmd := m.handlePermissionResponse(key, m.activeSession.ID, req)
			return result, cmd, true
		}
	}

	// Question response
	if m.sessionState().GetPendingQuestion(m.activeSession.ID) != nil {
		if result, cmd, handled := m.handleQuestionKeys(key); handled {
			return result, cmd, true
		}
	}

	// Plan approval response
	if m.sessionState().GetPendingPlanApproval(m.activeSession.ID) != nil {
		if result, cmd, handled := m.handlePlanApprovalKeys(key); handled {
			return result, cmd, true
		}
	}

	// Ctrl+V for image pasting (fallback for terminals that send raw key presses)
	if key == "ctrl+v" {
		result, cmd := m.handleImagePaste()
		return result, cmd, true
	}

	// Ctrl+P for parallel option exploration
	if key == "ctrl+p" && m.sessionState().HasDetectedOptions(m.activeSession.ID) {
		result, cmd := m.showExploreOptionsModal()
		return result, cmd, true
	}

	// Backspace to remove pending image when input is empty
	if key == "backspace" && m.chat.HasPendingImage() && m.chat.GetInput() == "" {
		m.chat.ClearImage()
		return m, nil, true
	}

	return m, nil, false
}

// handleQuestionKeys handles keys for question responses
func (m *Model) handleQuestionKeys(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "1", "2", "3", "4", "5":
		num := int(key[0] - '0')
		if m.chat.SelectOptionByNumber(num) {
			result, cmd := m.submitQuestionResponse(m.activeSession.ID)
			return result, cmd, true
		}
		return m, nil, true
	case "up", "k":
		m.chat.MoveQuestionSelection(-1)
		return m, nil, true
	case "down", "j":
		m.chat.MoveQuestionSelection(1)
		return m, nil, true
	case "enter":
		if m.chat.SelectCurrentOption() {
			result, cmd := m.submitQuestionResponse(m.activeSession.ID)
			return result, cmd, true
		}
		return m, nil, true
	}
	return m, nil, false
}

// handlePlanApprovalKeys handles keys for plan approval responses
func (m *Model) handlePlanApprovalKeys(key string) (tea.Model, tea.Cmd, bool) {
	switch key {
	case "y", "Y":
		result, cmd := m.submitPlanApprovalResponse(m.activeSession.ID, true)
		return result, cmd, true
	case "n", "N":
		result, cmd := m.submitPlanApprovalResponse(m.activeSession.ID, false)
		return result, cmd, true
	case "up", "k":
		m.chat.ScrollPlan(-3)
		return m, nil, true
	case "down", "j":
		m.chat.ScrollPlan(3)
		return m, nil, true
	}
	return m, nil, false
}

// handleEnterKey handles the enter key press
func (m *Model) handleEnterKey() (tea.Model, tea.Cmd) {
	switch m.focus {
	case FocusSidebar:
		// Select session
		if sess := m.sidebar.SelectedSession(); sess != nil {
			m.selectSession(sess)
			// Check if this session has an unsent initial message (from issue import)
			if initialMsg := m.sessionState().GetInitialMessage(sess.ID); initialMsg != "" {
				m.sessionState().SetPendingMessage(sess.ID, initialMsg)
				return m, func() tea.Msg {
					return SendPendingMessageMsg{SessionID: sess.ID}
				}
			}
			return m, nil
		}
	case FocusChat:
		if m.CanSendMessage() {
			// Send message immediately
			return m.sendMessage()
		} else if m.activeSession != nil && m.sessionState().IsWaiting(m.activeSession.ID) {
			// Queue message to be sent when streaming completes
			input := m.chat.GetInput()
			if input != "" {
				m.sessionState().SetPendingMessage(m.activeSession.ID, input)
				m.chat.ClearInput()
				m.chat.SetQueuedMessage(input)
				logger.Log("App: Queued message for session %s while streaming", m.activeSession.ID)
			}
		}
	}
	return m, nil
}

// handleCommitMessageGenerated handles commit message generation completion
func (m *Model) handleCommitMessageGenerated(msg CommitMessageGeneratedMsg) (tea.Model, tea.Cmd) {
	if msg.Error != nil {
		logger.Log("App: Commit message generation failed: %v", msg.Error)
		m.chat.AppendStreaming(fmt.Sprintf("Failed to generate commit message: %v\n", msg.Error))
		m.pendingCommitSession = ""
		m.pendingCommitType = MergeTypeNone
		return m, nil
	}

	// Show the edit commit modal with the generated message
	m.modal.Show(ui.NewEditCommitState(msg.Message, m.pendingCommitType.String()))
	return m, nil
}

// handleTerminalError shows terminal error to user in chat
func (m *Model) handleTerminalError(msg TerminalErrorMsg) (tea.Model, tea.Cmd) {
	if m.activeSession != nil {
		m.chat.AppendStreaming(fmt.Sprintf("\n[%s]\n", msg.Error))
		m.chat.FinishStreaming()
	}
	return m, nil
}

// handleTickMessages handles various tick messages for animations and timers
func (m *Model) handleTickMessages(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case ui.SidebarTickMsg:
		sidebar, cmd := m.sidebar.Update(msg)
		m.sidebar = sidebar
		return cmd
	case ui.StopwatchTickMsg, ui.SelectionCopyMsg:
		chat, cmd := m.chat.Update(msg)
		m.chat = chat
		return cmd
	case ui.FlashTickMsg:
		// Check if flash message has expired
		if m.footer.ClearIfExpired() {
			// Flash cleared, no need to continue ticking
			return nil
		}
		// Flash still active, continue ticking
		if m.footer.HasFlash() {
			return ui.FlashTick()
		}
		return nil
	case ui.ClipboardErrorMsg:
		// Show error message when clipboard write fails
		m.footer.SetFlash("Failed to copy to clipboard", ui.FlashError)
		return ui.FlashTick()
	}
	return nil
}

// handlePermissionResponse handles y/n/a key presses for permission prompts
func (m *Model) handlePermissionResponse(key string, sessionID string, req *mcp.PermissionRequest) (tea.Model, tea.Cmd) {
	runner := m.sessionMgr.GetRunner(sessionID)
	if runner == nil {
		logger.Log("App: Permission response for unknown session %s", sessionID)
		return m, nil
	}

	var allowed, always bool
	switch key {
	case "y", "Y":
		allowed = true
	case "a", "A":
		allowed = true
		always = true
	case "n", "N":
		allowed = false
	}

	logger.Log("App: Permission response for session %s: key=%s, allowed=%v, always=%v", sessionID, key, allowed, always)

	// Build response
	resp := mcp.PermissionResponse{
		ID:      req.ID,
		Allowed: allowed,
		Always:  always,
	}
	if !allowed {
		resp.Message = "User denied permission"
	}

	// If always, save the tool to per-repo allowed list
	if always {
		m.sessionMgr.AddAllowedTool(sessionID, req.Tool)
	}

	// Send response
	runner.SendPermissionResponse(resp)

	// Clear pending permission
	m.sessionState().ClearPendingPermission(sessionID)
	m.sidebar.SetPendingPermission(sessionID, false)
	m.chat.ClearPendingPermission()

	// Continue listening for session events
	return m, tea.Batch(m.sessionListeners(sessionID, runner, nil)...)
}

// submitQuestionResponse sends the collected question answers back to Claude
func (m *Model) submitQuestionResponse(sessionID string) (tea.Model, tea.Cmd) {
	runner := m.sessionMgr.GetRunner(sessionID)
	if runner == nil {
		logger.Log("App: Question response for unknown session %s", sessionID)
		return m, nil
	}

	req := m.sessionState().GetPendingQuestion(sessionID)
	if req == nil {
		logger.Log("App: No pending question for session %s", sessionID)
		return m, nil
	}

	// Get answers from chat
	answers := m.chat.GetQuestionAnswers()
	logger.Log("App: Question response for session %s: %d answers", sessionID, len(answers))

	// Build response
	resp := mcp.QuestionResponse{
		ID:      req.ID,
		Answers: answers,
	}

	// Send response
	runner.SendQuestionResponse(resp)

	// Clear pending question
	m.sessionState().ClearPendingQuestion(sessionID)
	m.sidebar.SetPendingPermission(sessionID, false)
	m.chat.ClearPendingQuestion()

	// Continue listening for session events
	return m, tea.Batch(m.sessionListeners(sessionID, runner, nil)...)
}

// submitPlanApprovalResponse sends the plan approval response back to Claude
func (m *Model) submitPlanApprovalResponse(sessionID string, approved bool) (tea.Model, tea.Cmd) {
	runner := m.sessionMgr.GetRunner(sessionID)
	if runner == nil {
		logger.Log("App: Plan approval response for unknown session %s", sessionID)
		return m, nil
	}

	req := m.sessionState().GetPendingPlanApproval(sessionID)
	if req == nil {
		logger.Log("App: No pending plan approval for session %s", sessionID)
		return m, nil
	}

	logger.Log("App: Plan approval response for session %s: approved=%v", sessionID, approved)

	// Build response
	resp := mcp.PlanApprovalResponse{
		ID:       req.ID,
		Approved: approved,
	}

	// Send response
	runner.SendPlanApprovalResponse(resp)

	// Clear pending plan approval
	m.sessionState().ClearPendingPlanApproval(sessionID)
	m.sidebar.SetPendingPermission(sessionID, false)
	m.chat.ClearPendingPlanApproval()

	// Continue listening for session events
	return m, tea.Batch(m.sessionListeners(sessionID, runner, nil)...)
}

// sessionListeners returns all the listener commands for a session.
// This bundles response, permission, question, and plan approval listeners together
// so adding a new listener type only requires changing this one function.
// If responseChan is provided, it will be used instead of runner.GetResponseChan().
func (m *Model) sessionListeners(sessionID string, runner claude.RunnerInterface, responseChan <-chan claude.ResponseChunk) []tea.Cmd {
	if runner == nil {
		return nil
	}

	ch := responseChan
	if ch == nil {
		ch = runner.GetResponseChan()
	}

	return []tea.Cmd{
		m.listenForSessionResponse(sessionID, ch),
		m.listenForSessionPermission(sessionID, runner),
		m.listenForSessionQuestion(sessionID, runner),
		m.listenForSessionPlanApproval(sessionID, runner),
	}
}

// listenForSessionResponse creates a command to listen for responses from a specific session
func (m *Model) listenForSessionResponse(sessionID string, ch <-chan claude.ResponseChunk) tea.Cmd {
	if ch == nil {
		return nil
	}

	return func() tea.Msg {
		chunk, ok := <-ch
		if !ok {
			return ClaudeResponseMsg{SessionID: sessionID, Chunk: claude.ResponseChunk{Done: true}}
		}
		return ClaudeResponseMsg{SessionID: sessionID, Chunk: chunk}
	}
}

// listenForSessionPermission creates a command to listen for permission requests from a specific session
func (m *Model) listenForSessionPermission(sessionID string, runner claude.RunnerInterface) tea.Cmd {
	if runner == nil {
		return nil
	}

	ch := runner.PermissionRequestChan()
	if ch == nil {
		// Runner has been stopped, don't create a goroutine that would block forever
		return nil
	}
	return func() tea.Msg {
		req, ok := <-ch
		if !ok {
			return nil
		}
		return PermissionRequestMsg{SessionID: sessionID, Request: req}
	}
}

// listenForSessionQuestion creates a command to listen for question requests from a specific session
func (m *Model) listenForSessionQuestion(sessionID string, runner claude.RunnerInterface) tea.Cmd {
	if runner == nil {
		return nil
	}

	ch := runner.QuestionRequestChan()
	if ch == nil {
		// Runner has been stopped, don't create a goroutine that would block forever
		return nil
	}
	return func() tea.Msg {
		req, ok := <-ch
		if !ok {
			return nil
		}
		return QuestionRequestMsg{SessionID: sessionID, Request: req}
	}
}

// listenForSessionPlanApproval creates a command that waits for plan approval requests
func (m *Model) listenForSessionPlanApproval(sessionID string, runner claude.RunnerInterface) tea.Cmd {
	if runner == nil {
		return nil
	}

	ch := runner.PlanApprovalRequestChan()
	if ch == nil {
		// Runner has been stopped, don't create a goroutine that would block forever
		return nil
	}
	return func() tea.Msg {
		req, ok := <-ch
		if !ok {
			return nil
		}
		return PlanApprovalRequestMsg{SessionID: sessionID, Request: req}
	}
}
