package app

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/notification"
	"github.com/zhubert/plural/internal/ui"
)

// handleClaudeResponseMsg handles streaming responses from Claude sessions.
func (m *Model) handleClaudeResponseMsg(msg ClaudeResponseMsg) (tea.Model, tea.Cmd) {
	// Get the runner for this session
	runner := m.sessionMgr.GetRunner(msg.SessionID)
	exists := runner != nil
	if !exists {
		logger.Warn("App: Received response for unknown session %s", msg.SessionID)
		return m, nil
	}

	isActiveSession := m.activeSession != nil && m.activeSession.ID == msg.SessionID

	if msg.Chunk.Error != nil {
		return m.handleClaudeError(msg.SessionID, msg.Chunk.Error.Error(), isActiveSession)
	}

	if msg.Chunk.Done {
		return m.handleClaudeDone(msg.SessionID, runner, isActiveSession)
	}

	return m.handleClaudeStreaming(msg.SessionID, msg.Chunk, runner, isActiveSession)
}

// handleClaudeError handles error responses from Claude.
func (m *Model) handleClaudeError(sessionID string, errMsg string, isActiveSession bool) (tea.Model, tea.Cmd) {
	logger.Error("App: Error in session %s: %v", sessionID, errMsg)
	m.sidebar.SetStreaming(sessionID, false)
	m.sessionState().StopWaiting(sessionID)

	if isActiveSession {
		m.chat.SetWaiting(false)
		m.chat.AppendStreaming("\n[Error: " + errMsg + "]")
	} else {
		// Store error for non-active session
		m.sessionState().AppendStreaming(sessionID, "\n[Error: "+errMsg+"]")
	}

	// Check if any sessions are still streaming
	if !m.hasAnyStreamingSessions() {
		m.setState(StateIdle)
	}

	return m, nil
}

// handleClaudeDone handles completion of Claude streaming.
func (m *Model) handleClaudeDone(sessionID string, runner claude.RunnerInterface, isActiveSession bool) (tea.Model, tea.Cmd) {
	logger.Info("App: Session %s completed streaming", sessionID)
	m.sidebar.SetStreaming(sessionID, false)
	m.sessionState().StopWaiting(sessionID)

	var completionCmd tea.Cmd
	if isActiveSession {
		m.chat.SetWaiting(false)
		m.chat.FinishStreaming()
		// Start completion flash animation
		completionCmd = m.chat.StartCompletionFlash()
	} else {
		// For non-active session, just clear our saved streaming content
		// The runner already adds the assistant message when streaming completes (claude.go)
		m.sessionState().ClearStreaming(sessionID)
	}

	// Mark session as started and save messages
	sess := m.sessionMgr.GetSession(sessionID)
	if sess != nil && runner.SessionStarted() {
		if !sess.Started {
			m.config.MarkSessionStarted(sess.ID)
			sess.Started = true
			m.config.Save()
		}
		// Save messages for this session
		m.sessionMgr.SaveRunnerMessages(sessionID, runner)
	}

	// Detect options in the last assistant message for parallel exploration
	m.detectOptionsInSession(sessionID, runner)

	// Send desktop notification if window is not focused and notifications are enabled
	if !m.windowFocused && m.config.GetNotificationsEnabled() {
		sessionName := sessionID
		if sess != nil {
			sessionName = ui.SessionDisplayName(sess.Branch, sess.Name)
		}
		go notification.SessionCompleted(sessionName)
	}

	// Check if any sessions are still streaming
	if !m.hasAnyStreamingSessions() {
		m.setState(StateIdle)
	}

	// Check for pending message queued during streaming
	if m.sessionState().HasPendingMessage(sessionID) {
		if completionCmd != nil {
			return m, tea.Batch(completionCmd, func() tea.Msg {
				return SendPendingMessageMsg{SessionID: sessionID}
			})
		}
		return m, func() tea.Msg {
			return SendPendingMessageMsg{SessionID: sessionID}
		}
	}

	if completionCmd != nil {
		return m, completionCmd
	}
	return m, nil
}

// handleClaudeStreaming handles streaming content chunks from Claude.
func (m *Model) handleClaudeStreaming(sessionID string, chunk claude.ResponseChunk, runner claude.RunnerInterface, isActiveSession bool) (tea.Model, tea.Cmd) {
	// Streaming content - clear wait time since response has started
	m.sessionState().ClearWaitStart(sessionID)

	if isActiveSession {
		m.chat.SetWaiting(false)
		// Handle different chunk types
		switch chunk.Type {
		case claude.ChunkTypeToolUse:
			// Append tool use to streaming content so it persists in history
			m.chat.AppendToolUse(chunk.ToolName, chunk.ToolInput)
		case claude.ChunkTypeToolResult:
			// Tool completed, mark the tool use line as complete
			m.chat.MarkLastToolUseComplete()
		case claude.ChunkTypeText:
			m.chat.AppendStreaming(chunk.Content)
		default:
			// For backwards compatibility, treat unknown types as text
			if chunk.Content != "" {
				m.chat.AppendStreaming(chunk.Content)
			}
		}
	} else {
		// Store streaming content for non-active session
		m.handleNonActiveSessionStreaming(sessionID, chunk)
	}

	// Continue listening for more chunks from this session
	return m, tea.Batch(
		m.listenForSessionResponse(sessionID, runner.GetResponseChan()),
		m.listenForSessionPermission(sessionID, runner),
		m.listenForSessionQuestion(sessionID, runner),
	)
}

// handleNonActiveSessionStreaming handles streaming content for non-active sessions.
func (m *Model) handleNonActiveSessionStreaming(sessionID string, chunk claude.ResponseChunk) {
	switch chunk.Type {
	case claude.ChunkTypeToolUse:
		// Format tool use for non-active session
		icon := ui.GetToolIcon(chunk.ToolName)
		line := ui.ToolUseInProgress + " " + icon + "(" + chunk.ToolName
		if chunk.ToolInput != "" {
			line += ": " + chunk.ToolInput
		}
		line += ")\n"
		existing := m.sessionState().GetStreaming(sessionID)
		if existing != "" && !strings.HasSuffix(existing, "\n") {
			m.sessionState().AppendStreaming(sessionID, "\n")
		}
		// Track position where the marker starts
		m.sessionState().SetToolUsePos(sessionID, len(m.sessionState().GetStreaming(sessionID)))
		m.sessionState().AppendStreaming(sessionID, line)

	case claude.ChunkTypeToolResult:
		// Mark the tool use as complete for non-active session
		if pos, exists := m.sessionState().GetToolUsePos(sessionID); exists && pos >= 0 {
			m.sessionState().ReplaceToolUseMarker(sessionID, ui.ToolUseInProgress, ui.ToolUseComplete, pos)
			m.sessionState().ClearToolUsePos(sessionID)
		}

	case claude.ChunkTypeText:
		// Add extra newline after tool use for visual separation
		if pos, exists := m.sessionState().GetToolUsePos(sessionID); exists && pos >= 0 {
			streaming := m.sessionState().GetStreaming(sessionID)
			if strings.HasSuffix(streaming, "\n") && !strings.HasSuffix(streaming, "\n\n") {
				m.sessionState().AppendStreaming(sessionID, "\n")
			}
		}
		m.sessionState().AppendStreaming(sessionID, chunk.Content)

	default:
		if chunk.Content != "" {
			m.sessionState().AppendStreaming(sessionID, chunk.Content)
		}
	}
}

// handleMergeResultMsg handles merge operation results.
func (m *Model) handleMergeResultMsg(msg MergeResultMsg) (tea.Model, tea.Cmd) {
	isActiveSession := m.activeSession != nil && m.activeSession.ID == msg.SessionID

	if msg.Result.Error != nil {
		return m.handleMergeError(msg.SessionID, msg.Result, isActiveSession)
	}

	if msg.Result.Done {
		return m.handleMergeDone(msg.SessionID, isActiveSession)
	}

	// Still receiving merge output
	if isActiveSession {
		m.chat.AppendStreaming(msg.Result.Output)
	} else {
		m.sessionState().AppendStreaming(msg.SessionID, msg.Result.Output)
	}
	return m, m.listenForMergeResult(msg.SessionID)
}

// handleMergeError handles merge operation errors.
func (m *Model) handleMergeError(sessionID string, result git.Result, isActiveSession bool) (tea.Model, tea.Cmd) {
	// Check if this is a merge conflict with conflicted files
	if len(result.ConflictedFiles) > 0 {
		// Show conflict resolution modal
		sess := m.config.GetSession(sessionID)
		sessionName := sessionID
		if sess != nil {
			sessionName = ui.SessionDisplayName(sess.Branch, sess.Name)
		}
		logger.Log("App: Merge conflict detected for session %s, files: %v", sessionID, result.ConflictedFiles)
		m.modal.Show(ui.NewMergeConflictState(sessionID, sessionName, result.ConflictedFiles, result.RepoPath))
		// Clean up merge state
		m.sessionState().StopMerge(sessionID)
		return m, nil
	}

	// Regular error (not a conflict)
	if isActiveSession {
		m.chat.AppendStreaming("\n[Error: " + result.Error.Error() + "]\n")
	} else {
		m.sessionState().AppendStreaming(sessionID, "\n[Error: "+result.Error.Error()+"]\n")
	}
	// Clean up merge state for this session
	m.sessionState().StopMerge(sessionID)

	return m, nil
}

// handleMergeDone handles successful completion of merge operations.
func (m *Model) handleMergeDone(sessionID string, isActiveSession bool) (tea.Model, tea.Cmd) {
	if isActiveSession {
		m.chat.FinishStreaming()
	} else {
		// Store completed merge output as a message for when user switches back
		if content := m.sessionState().GetStreaming(sessionID); content != "" {
			if runner := m.sessionMgr.GetRunner(sessionID); runner != nil {
				runner.AddAssistantMessage(content)
				m.sessionMgr.SaveRunnerMessages(sessionID, runner)
			}
			m.sessionState().ClearStreaming(sessionID)
		}
	}

	// Mark session as merged or PR created based on operation type
	mergeType := m.sessionState().GetMergeType(sessionID)
	switch mergeType {
	case MergeTypePR:
		m.config.MarkSessionPRCreated(sessionID)
		logger.Log("App: Marked session %s as PR created", sessionID)
	case MergeTypeMerge:
		m.config.MarkSessionMerged(sessionID)
		logger.Log("App: Marked session %s as merged", sessionID)
	case MergeTypeParent:
		// Get child session to find parent
		childSess := m.config.GetSession(sessionID)
		if childSess != nil && childSess.ParentID != "" {
			// Merge conversation history from child to parent
			if err := m.mergeConversationHistory(sessionID, childSess.ParentID); err != nil {
				logger.Log("App: Failed to merge conversation history: %v", err)
				if isActiveSession {
					m.chat.AppendStreaming(fmt.Sprintf("\n[Warning: Failed to merge conversation history: %v]\n", err))
				}
			} else {
				logger.Log("App: Merged conversation history from %s to parent %s", sessionID, childSess.ParentID)
			}
		}
		m.config.MarkSessionMergedToParent(sessionID)
		logger.Log("App: Marked session %s as merged to parent", sessionID)
	}

	m.config.Save()
	// Update sidebar with new session status
	m.sidebar.SetSessions(m.config.GetSessions())
	// Clean up merge state for this session
	m.sessionState().StopMerge(sessionID)

	return m, nil
}

// handleSendPendingMessageMsg processes queued messages submitted during streaming.
func (m *Model) handleSendPendingMessageMsg(msg SendPendingMessageMsg) (tea.Model, tea.Cmd) {
	pendingMsg := m.sessionState().GetPendingMessage(msg.SessionID)
	if pendingMsg == "" {
		return m, nil
	}

	// Only send if this session is still valid and can accept messages
	sess := m.sessionMgr.GetSession(msg.SessionID)
	if sess == nil || sess.MergedToParent {
		logger.Log("App: Cannot send pending message for session %s (invalid or merged)", msg.SessionID)
		return m, nil
	}

	// Check if session is currently busy (e.g., merge in progress or already streaming again)
	if m.sessionState().IsWaiting(msg.SessionID) || m.sessionState().IsMerging(msg.SessionID) {
		// Re-queue the message to try again later
		m.sessionState().SetPendingMessage(msg.SessionID, pendingMsg)
		return m, nil
	}

	// Get the runner for this session
	runner := m.sessionMgr.GetRunner(msg.SessionID)
	if runner == nil {
		logger.Log("App: No runner for session %s to send pending message", msg.SessionID)
		return m, nil
	}

	logger.Log("App: Sending pending message for session %s: %s", msg.SessionID, pendingMsg)

	// If this is the active session, add to chat and clear queued display
	isActiveSession := m.activeSession != nil && m.activeSession.ID == msg.SessionID
	if isActiveSession {
		m.chat.ClearQueuedMessage()
		m.chat.AddUserMessage(pendingMsg)
	}

	// Create context and start streaming
	ctx, cancel := context.WithCancel(context.Background())
	m.sessionState().StartWaiting(msg.SessionID, cancel)
	startTime, _ := m.sessionState().GetWaitStart(msg.SessionID)
	if isActiveSession {
		m.chat.SetWaitingWithStart(true, startTime)
	}
	m.sidebar.SetStreaming(msg.SessionID, true)
	m.setState(StateStreamingClaude)

	// Send the message
	content := []claude.ContentBlock{{Type: claude.ContentTypeText, Text: pendingMsg}}
	responseChan := runner.SendContent(ctx, content)

	return m, tea.Batch(
		m.listenForSessionResponse(msg.SessionID, responseChan),
		m.listenForSessionPermission(msg.SessionID, runner),
		m.listenForSessionQuestion(msg.SessionID, runner),
		ui.SidebarTick(),
		ui.StopwatchTick(),
	)
}

// handlePermissionRequestMsg handles permission requests from Claude.
func (m *Model) handlePermissionRequestMsg(msg PermissionRequestMsg) (tea.Model, tea.Cmd) {
	// Get the runner for this session
	runner := m.sessionMgr.GetRunner(msg.SessionID)
	exists := runner != nil
	if !exists {
		logger.Log("App: Received permission request for unknown session %s", msg.SessionID)
		return m, nil
	}

	// Store permission request for this session (inline, not modal)
	logger.Log("App: Permission request for session %s: tool=%s", msg.SessionID, msg.Request.Tool)
	m.sessionState().SetPendingPermission(msg.SessionID, &msg.Request)
	m.sidebar.SetPendingPermission(msg.SessionID, true)

	// If this is the active session, show permission in chat
	if m.activeSession != nil && m.activeSession.ID == msg.SessionID {
		m.chat.SetPendingPermission(msg.Request.Tool, msg.Request.Description)
	}

	// Continue listening for more permission requests and responses
	return m, tea.Batch(
		m.listenForSessionResponse(msg.SessionID, runner.GetResponseChan()),
		m.listenForSessionPermission(msg.SessionID, runner),
		m.listenForSessionQuestion(msg.SessionID, runner),
	)
}

// handleQuestionRequestMsg handles question requests from Claude.
func (m *Model) handleQuestionRequestMsg(msg QuestionRequestMsg) (tea.Model, tea.Cmd) {
	// Get the runner for this session
	runner := m.sessionMgr.GetRunner(msg.SessionID)
	exists := runner != nil
	if !exists {
		logger.Log("App: Received question request for unknown session %s", msg.SessionID)
		return m, nil
	}

	// Store question request for this session
	logger.Log("App: Question request for session %s: %d questions", msg.SessionID, len(msg.Request.Questions))
	m.sessionState().SetPendingQuestion(msg.SessionID, &msg.Request)
	m.sidebar.SetPendingPermission(msg.SessionID, true) // Reuse permission indicator for questions

	// If this is the active session, show question in chat
	if m.activeSession != nil && m.activeSession.ID == msg.SessionID {
		m.chat.SetPendingQuestion(msg.Request.Questions)
	}

	// Continue listening for more requests and responses
	return m, tea.Batch(
		m.listenForSessionResponse(msg.SessionID, runner.GetResponseChan()),
		m.listenForSessionPermission(msg.SessionID, runner),
		m.listenForSessionQuestion(msg.SessionID, runner),
		m.listenForSessionPlanApproval(msg.SessionID, runner),
	)
}

// handlePlanApprovalRequestMsg handles plan approval requests from Claude.
func (m *Model) handlePlanApprovalRequestMsg(msg PlanApprovalRequestMsg) (tea.Model, tea.Cmd) {
	// Get the runner for this session
	runner := m.sessionMgr.GetRunner(msg.SessionID)
	exists := runner != nil
	if !exists {
		logger.Log("App: Received plan approval request for unknown session %s", msg.SessionID)
		return m, nil
	}

	// Store plan approval request for this session
	logger.Log("App: Plan approval request for session %s: plan %d chars, %d allowed prompts",
		msg.SessionID, len(msg.Request.Plan), len(msg.Request.AllowedPrompts))
	m.sessionState().SetPendingPlanApproval(msg.SessionID, &msg.Request)
	m.sidebar.SetPendingPermission(msg.SessionID, true) // Reuse permission indicator for plan approval

	// If this is the active session, show plan approval in chat
	if m.activeSession != nil && m.activeSession.ID == msg.SessionID {
		m.chat.SetPendingPlanApproval(msg.Request.Plan, msg.Request.AllowedPrompts)
	}

	// Continue listening for more requests and responses
	return m, tea.Batch(
		m.listenForSessionResponse(msg.SessionID, runner.GetResponseChan()),
		m.listenForSessionPermission(msg.SessionID, runner),
		m.listenForSessionQuestion(msg.SessionID, runner),
		m.listenForSessionPlanApproval(msg.SessionID, runner),
	)
}

// handleGitHubIssuesFetchedMsg handles fetched GitHub issues.
func (m *Model) handleGitHubIssuesFetchedMsg(msg GitHubIssuesFetchedMsg) (tea.Model, tea.Cmd) {
	// Handle fetched GitHub issues - update the modal if it's still visible
	if state, ok := m.modal.State.(*ui.ImportIssuesState); ok {
		if msg.Error != nil {
			state.SetError(msg.Error.Error())
		} else {
			// Convert to UI issue items
			items := make([]ui.IssueItem, len(msg.Issues))
			for i, issue := range msg.Issues {
				items[i] = ui.IssueItem{
					Number: issue.Number,
					Title:  issue.Title,
					Body:   issue.Body,
					URL:    issue.URL,
				}
			}
			state.SetIssues(items)
		}
	}
	return m, nil
}
