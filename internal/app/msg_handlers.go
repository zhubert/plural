package app

import (
	"context"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/mcp"
	"github.com/zhubert/plural/internal/notification"
	"github.com/zhubert/plural/internal/ui"
)

// handleClaudeResponseMsg handles streaming responses from Claude sessions.
func (m *Model) handleClaudeResponseMsg(msg ClaudeResponseMsg) (tea.Model, tea.Cmd) {
	// Get the runner for this session
	runner := m.sessionMgr.GetRunner(msg.SessionID)
	exists := runner != nil
	if !exists {
		logger.WithSession(msg.SessionID).Warn("received response for unknown session")
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
	logger.WithSession(sessionID).Error("error in session", "error", errMsg)
	m.sidebar.SetStreaming(sessionID, false)
	m.sessionState().StopWaiting(sessionID)

	if isActiveSession {
		m.chat.SetWaiting(false)
		m.chat.AppendStreaming("\n[Error: " + errMsg + "]")
	} else {
		// Store error for non-active session
		m.sessionState().GetOrCreate(sessionID).AppendStreamingContent("\n[Error: " + errMsg + "]")
	}

	// Check if any sessions are still streaming
	if !m.hasAnyStreamingSessions() {
		m.setState(StateIdle)
	}

	return m, nil
}

// handleClaudeDone handles completion of Claude streaming.
func (m *Model) handleClaudeDone(sessionID string, runner claude.RunnerInterface, isActiveSession bool) (tea.Model, tea.Cmd) {
	logger.WithSession(sessionID).Info("completed streaming")
	m.sidebar.SetStreaming(sessionID, false)
	m.sidebar.SetIdleWithResponse(sessionID, true)

	// Flush any pending tool uses, clear streaming content, and clear subagent indicator
	if state := m.sessionState().GetIfExists(sessionID); state != nil {
		state.FlushToolUseRollup(ui.GetToolIcon, ui.ToolUseInProgress, ui.ToolUseComplete)
		state.SetStreamingContent("")
		state.SetSubagentModel("")
	}
	m.sessionState().StopWaiting(sessionID)

	var completionCmd tea.Cmd
	if isActiveSession {
		m.chat.SetWaiting(false)
		m.chat.FinishStreaming()
		// Clear subagent indicator
		m.chat.ClearSubagentModel()
		// Start completion flash animation
		completionCmd = m.chat.StartCompletionFlash()

		// Refresh diff stats after Claude finishes (files may have changed)
		m.refreshDiffStats()
	}

	// Mark session as started and save messages
	sess := m.sessionMgr.GetSession(sessionID)
	if sess != nil && runner.SessionStarted() {
		if !sess.Started {
			m.config.MarkSessionStarted(sess.ID)
			sess.Started = true
			// Clear container init state now that session has started
			// The callback in SessionManager already cleared SessionStateManager state
			if isActiveSession {
				m.chat.SetContainerInitializing(false, time.Time{})
			}
			if err := m.config.Save(); err != nil {
				logger.WithSession(sess.ID).Error("failed to save config after marking session started", "error", err)
			}
		}
		// Save messages for this session
		if err := m.sessionMgr.SaveRunnerMessages(sessionID, runner); err != nil {
			flashCmd := m.ShowFlashError("Failed to save session messages")
			if completionCmd != nil {
				completionCmd = tea.Batch(completionCmd, flashCmd)
			} else {
				completionCmd = flashCmd
			}
		}
	}

	// Check if Claude resolved a pending merge conflict for this session
	m.checkConflictResolution(sessionID)

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
	if state := m.sessionState().GetIfExists(sessionID); state != nil && state.GetPendingMsg() != "" {
		if completionCmd != nil {
			return m, tea.Batch(completionCmd, func() tea.Msg {
				return SendPendingMessageMsg{SessionID: sessionID}
			})
		}
		return m, func() tea.Msg {
			return SendPendingMessageMsg{SessionID: sessionID}
		}
	}

	// Emit SessionCompletedMsg for autonomous sessions
	if sess != nil && sess.Autonomous {
		state := m.sessionState().GetOrCreate(sessionID)

		// Initialize autonomous start time if not already set
		// (e.g., for sessions created via issue poller or child sessions where
		// shortcutToggleAutonomous wasn't called)
		if state.GetAutonomousStartTime().IsZero() {
			state.SetAutonomousStartTime(m.sessionState().GetStreamingStartTimeOrNow(sessionID))
		}

		turns := state.IncrementAutonomousTurns()
		maxTurns := m.config.GetAutoMaxTurns()
		maxDuration := time.Duration(m.config.GetAutoMaxDurationMin()) * time.Minute
		elapsed := time.Since(state.GetAutonomousStartTime())

		// Check limits
		if turns >= maxTurns {
			logger.WithSession(sessionID).Warn("autonomous session hit turn limit", "turns", turns, "max", maxTurns)
			limitCmd := func() tea.Msg {
				return AutonomousLimitReachedMsg{SessionID: sessionID, Reason: "turn_limit"}
			}
			if completionCmd != nil {
				return m, tea.Batch(completionCmd, limitCmd)
			}
			return m, limitCmd
		}
		if elapsed >= maxDuration {
			logger.WithSession(sessionID).Warn("autonomous session hit duration limit", "elapsed", elapsed, "max", maxDuration)
			limitCmd := func() tea.Msg {
				return AutonomousLimitReachedMsg{SessionID: sessionID, Reason: "duration_limit"}
			}
			if completionCmd != nil {
				return m, tea.Batch(completionCmd, limitCmd)
			}
			return m, limitCmd
		}

		// Supervisor sessions with active children should not be considered
		// complete yet — children will notify the supervisor via
		// SendPendingMessageMsg when they finish, which triggers another
		// Claude response. The supervisor only truly completes when Claude
		// finishes a response with no active children remaining.
		if sess.IsSupervisor {
			children := m.config.GetChildSessions(sessionID)
			for _, child := range children {
				childRunner := m.sessionMgr.GetRunner(child.ID)
				if childRunner != nil && childRunner.IsStreaming() {
					logger.WithSession(sessionID).Debug("supervisor has active children, deferring completion")
					if completionCmd != nil {
						return m, completionCmd
					}
					return m, nil
				}
				if childState := m.sessionState().GetIfExists(child.ID); childState != nil {
					if childState.GetIsWaiting() || childState.IsMerging() {
						logger.WithSession(sessionID).Debug("supervisor has active children, deferring completion")
						if completionCmd != nil {
							return m, completionCmd
						}
						return m, nil
					}
				}
			}
		}

		// Emit completion event (no pending interactions)
		completedCmd := func() tea.Msg {
			return SessionCompletedMsg{SessionID: sessionID}
		}
		if completionCmd != nil {
			return m, tea.Batch(completionCmd, completedCmd)
		}
		return m, completedCmd
	}

	if completionCmd != nil {
		return m, completionCmd
	}
	return m, nil
}

// handleClaudeStreaming handles streaming content chunks from Claude.
func (m *Model) handleClaudeStreaming(sessionID string, chunk claude.ResponseChunk, runner claude.RunnerInterface, isActiveSession bool) (tea.Model, tea.Cmd) {
	// Streaming content - clear wait time since response has started
	if state := m.sessionState().GetIfExists(sessionID); state != nil {
		state.SetWaitStartTime(time.Time{})
	}

	if isActiveSession {
		m.chat.SetWaiting(false)
		// Handle different chunk types
		switch chunk.Type {
		case claude.ChunkTypeToolUse:
			// Append tool use to streaming content so it persists in history
			m.chat.AppendToolUse(chunk.ToolName, chunk.ToolInput, chunk.ToolUseID)
		case claude.ChunkTypeToolResult:
			// Tool completed, mark the tool use as complete by ID with result info
			m.chat.MarkToolUseComplete(chunk.ToolUseID, chunk.ResultInfo)
		case claude.ChunkTypeText:
			m.chat.AppendStreaming(chunk.Content)
		case claude.ChunkTypeTodoUpdate:
			// Update the todo list display
			if chunk.TodoList != nil {
				m.sessionState().GetOrCreate(sessionID).SetCurrentTodoList(chunk.TodoList)
				m.chat.SetTodoList(chunk.TodoList)
			}
		case claude.ChunkTypeStreamStats:
			// Update streaming statistics display
			if chunk.Stats != nil {
				m.chat.SetStreamStats(chunk.Stats)
			}
		case claude.ChunkTypeSubagentStatus:
			// Update subagent indicator (empty model means subagent ended)
			m.chat.SetSubagentModel(chunk.SubagentModel)
			m.sessionState().GetOrCreate(sessionID).SetSubagentModel(chunk.SubagentModel)
		case claude.ChunkTypePermissionDenials:
			// Append permission denials summary to chat
			if len(chunk.PermissionDenials) > 0 {
				m.chat.AppendPermissionDenials(chunk.PermissionDenials)
			}
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
	return m, tea.Batch(m.sessionListeners(sessionID, runner, nil)...)
}

// handleNonActiveSessionStreaming handles streaming content for non-active sessions.
func (m *Model) handleNonActiveSessionStreaming(sessionID string, chunk claude.ResponseChunk) {
	state := m.sessionState().GetOrCreate(sessionID)

	switch chunk.Type {
	case claude.ChunkTypeToolUse:
		// Add tool use to rollup for non-active session
		state.AddToolUse(chunk.ToolName, chunk.ToolInput, chunk.ToolUseID)

	case claude.ChunkTypeToolResult:
		// Mark the tool use as complete by ID for non-active session with result info
		state.MarkToolUseComplete(chunk.ToolUseID, chunk.ResultInfo)

	case claude.ChunkTypeText:
		// Flush any pending tool uses to streaming content before adding text
		state.FlushToolUseRollup(ui.GetToolIcon, ui.ToolUseInProgress, ui.ToolUseComplete)
		state.AppendStreamingContent(chunk.Content)

	case claude.ChunkTypeTodoUpdate:
		// Store todo list for non-active session
		if chunk.TodoList != nil {
			state.SetCurrentTodoList(chunk.TodoList)
		}

	case claude.ChunkTypeSubagentStatus:
		// Store subagent status for non-active session
		state.SetSubagentModel(chunk.SubagentModel)

	case claude.ChunkTypePermissionDenials:
		// Append permission denials to streaming content for non-active session
		if len(chunk.PermissionDenials) > 0 {
			denialText := formatPermissionDenialsText(chunk.PermissionDenials)
			state.AppendStreamingContent(denialText)
		}

	default:
		if chunk.Content != "" {
			// Flush any pending tool uses before adding other content
			state.FlushToolUseRollup(ui.GetToolIcon, ui.ToolUseInProgress, ui.ToolUseComplete)
			state.AppendStreamingContent(chunk.Content)
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
		m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent(msg.Result.Output)
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
		logger.WithSession(sessionID).Warn("merge conflict detected", "files", result.ConflictedFiles)
		m.modal.Show(ui.NewMergeConflictState(sessionID, sessionName, result.ConflictedFiles, result.RepoPath))
		// Clean up merge state
		m.sessionState().StopMerge(sessionID)
		return m, nil
	}

	// Regular error (not a conflict)
	if isActiveSession {
		m.chat.AppendStreaming("\n[Error: " + result.Error.Error() + "]\n")
	} else {
		m.sessionState().GetOrCreate(sessionID).AppendStreamingContent("\n[Error: " + result.Error.Error() + "]\n")
	}
	// Clean up merge state for this session
	m.sessionState().StopMerge(sessionID)

	return m, nil
}

// handleMergeDone handles successful completion of merge operations.
func (m *Model) handleMergeDone(sessionID string, isActiveSession bool) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if isActiveSession {
		m.chat.FinishStreaming()
	} else {
		// Store completed merge output as a message for when user switches back
		if state := m.sessionState().GetIfExists(sessionID); state != nil {
			content := state.GetStreamingContent()
			if content != "" {
				if runner := m.sessionMgr.GetRunner(sessionID); runner != nil {
					runner.AddAssistantMessage(content)
					if err := m.sessionMgr.SaveRunnerMessages(sessionID, runner); err != nil {
						cmds = append(cmds, m.ShowFlashError("Failed to save session messages"))
					}
				}
				state.SetStreamingContent("")
			}
		}
	}

	// Mark session as merged or PR created based on operation type
	log := logger.WithSession(sessionID)
	state := m.sessionState().GetIfExists(sessionID)
	mergeType := MergeTypeNone
	if state != nil {
		mergeType = state.GetMergeType()
	}
	switch mergeType {
	case MergeTypePR:
		m.config.MarkSessionPRCreated(sessionID)
		log.Info("marked session as PR created")
		// Phase 5A: Start auto-merge state machine (review → CI → merge)
		sess := m.config.GetSession(sessionID)
		if sess != nil && sess.Autonomous && m.config.GetRepoAutoMerge(sess.RepoPath) {
			log.Info("starting auto-merge polling", "branch", sess.Branch)
			cmds = append(cmds, m.pollForAutoMerge(sessionID))
		}
	case MergeTypeMerge:
		m.config.MarkSessionMerged(sessionID)
		log.Info("marked session as merged")
	case MergeTypeParent:
		// Get child session to find parent
		childSess := m.config.GetSession(sessionID)
		if childSess != nil && childSess.ParentID != "" {
			// Merge conversation history from child to parent
			if err := m.mergeConversationHistory(sessionID, childSess.ParentID); err != nil {
				log.Error("failed to merge conversation history", "error", err)
				cmds = append(cmds, m.ShowFlashWarning("Failed to merge conversation history"))
			} else {
				log.Info("merged conversation history to parent", "parentID", childSess.ParentID)
			}
		}
		m.config.MarkSessionMergedToParent(sessionID)
		log.Info("marked session as merged to parent")
	}

	if err := m.config.Save(); err != nil {
		log.Error("failed to save config after merge", "error", err)
		cmds = append(cmds, m.ShowFlashError("Failed to save session state"))
	}
	// Update sidebar with new session status
	m.sidebar.SetSessions(m.getFilteredSessions())
	// Clean up merge state for this session
	m.sessionState().StopMerge(sessionID)

	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// handleSendPendingMessageMsg processes queued messages submitted during streaming.
func (m *Model) handleSendPendingMessageMsg(msg SendPendingMessageMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	pendingMsg := m.sessionState().GetPendingMessage(msg.SessionID)
	if pendingMsg == "" {
		return m, nil
	}

	// Only send if this session is still valid and can accept messages
	sess := m.sessionMgr.GetSession(msg.SessionID)
	if sess == nil || sess.MergedToParent {
		log.Warn("cannot send pending message - session invalid or merged")
		return m, nil
	}

	// Check if session is currently busy (e.g., merge in progress or already streaming again)
	state := m.sessionState().GetIfExists(msg.SessionID)
	if state != nil && (state.GetIsWaiting() || state.IsMerging()) {
		// Re-queue the message to try again later
		state.SetPendingMsg(pendingMsg)
		return m, nil
	}

	// Get the runner for this session
	runner := m.sessionMgr.GetRunner(msg.SessionID)
	if runner == nil {
		log.Warn("no runner to send pending message")
		return m, nil
	}

	log.Debug("sending pending message", "message", pendingMsg)

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

	cmds := append(m.sessionListeners(msg.SessionID, runner, responseChan),
		m.sidebar.SidebarTick(),
		m.chat.SpinnerTick(),
	)
	return m, tea.Batch(cmds...)
}

// handlePermissionRequestMsg handles permission requests from Claude.
func (m *Model) handlePermissionRequestMsg(msg PermissionRequestMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	// Get the runner for this session
	runner := m.sessionMgr.GetRunner(msg.SessionID)
	exists := runner != nil
	if !exists {
		log.Warn("received permission request for unknown session")
		return m, nil
	}

	// Store permission request for this session (inline, not modal)
	log.Debug("permission request received", "tool", msg.Request.Tool)
	m.sessionState().GetOrCreate(msg.SessionID).SetPendingPermission(&msg.Request)
	m.sidebar.SetPendingPermission(msg.SessionID, true)

	// If this is the active session, show permission in chat
	if m.activeSession != nil && m.activeSession.ID == msg.SessionID {
		m.chat.SetPendingPermission(msg.Request.Tool, msg.Request.Description)
	}

	// Continue listening for session events
	return m, tea.Batch(m.sessionListeners(msg.SessionID, runner, nil)...)
}

// handleQuestionRequestMsg handles question requests from Claude.
func (m *Model) handleQuestionRequestMsg(msg QuestionRequestMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	// Get the runner for this session
	runner := m.sessionMgr.GetRunner(msg.SessionID)
	exists := runner != nil
	if !exists {
		log.Warn("received question request for unknown session")
		return m, nil
	}

	// Auto-respond for autonomous sessions
	sess := m.config.GetSession(msg.SessionID)
	if sess != nil && sess.Autonomous {
		log.Info("auto-responding to question (autonomous mode)")

		// Build auto-response: select first option for each question
		answers := make(map[string]string)
		for _, q := range msg.Request.Questions {
			if len(q.Options) > 0 {
				answers[q.Question] = q.Options[0].Label
			} else {
				answers[q.Question] = "Continue as you see fit"
			}
		}

		resp := mcp.QuestionResponse{
			ID:      msg.Request.ID,
			Answers: answers,
		}
		runner.SendQuestionResponse(resp)

		// Log auto-response in chat
		isActiveSession := m.activeSession != nil && m.activeSession.ID == msg.SessionID
		autoMsg := "[AUTO-RESPONSE] Answered question automatically"
		if isActiveSession {
			m.chat.AppendStreaming("\n" + autoMsg + "\n")
		} else {
			m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent("\n" + autoMsg + "\n")
		}

		return m, tea.Batch(m.sessionListeners(msg.SessionID, runner, nil)...)
	}

	// Store question request for this session
	log.Debug("question request received", "questionCount", len(msg.Request.Questions))
	m.sessionState().GetOrCreate(msg.SessionID).SetPendingQuestion(&msg.Request)
	m.sidebar.SetPendingPermission(msg.SessionID, true) // Reuse permission indicator for questions
	m.sidebar.SetPendingQuestion(msg.SessionID, true)

	// If this is the active session, show question in chat
	if m.activeSession != nil && m.activeSession.ID == msg.SessionID {
		m.chat.SetPendingQuestion(msg.Request.Questions)
	}

	// Continue listening for session events
	return m, tea.Batch(m.sessionListeners(msg.SessionID, runner, nil)...)
}

// handlePlanApprovalRequestMsg handles plan approval requests from Claude.
func (m *Model) handlePlanApprovalRequestMsg(msg PlanApprovalRequestMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	// Get the runner for this session
	runner := m.sessionMgr.GetRunner(msg.SessionID)
	exists := runner != nil
	if !exists {
		log.Warn("received plan approval request for unknown session")
		return m, nil
	}

	// Auto-approve for autonomous sessions
	sess := m.config.GetSession(msg.SessionID)
	if sess != nil && sess.Autonomous {
		log.Info("auto-approving plan (autonomous mode)")

		resp := mcp.PlanApprovalResponse{
			ID:       msg.Request.ID,
			Approved: true,
		}
		runner.SendPlanApprovalResponse(resp)

		// Log auto-response in chat
		isActiveSession := m.activeSession != nil && m.activeSession.ID == msg.SessionID
		autoMsg := "[AUTO-RESPONSE] Plan approved automatically"
		if isActiveSession {
			m.chat.AppendStreaming("\n" + autoMsg + "\n")
		} else {
			m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent("\n" + autoMsg + "\n")
		}

		return m, tea.Batch(m.sessionListeners(msg.SessionID, runner, nil)...)
	}

	// Store plan approval request for this session
	log.Debug("plan approval request received", "planChars", len(msg.Request.Plan), "allowedPrompts", len(msg.Request.AllowedPrompts))
	m.sessionState().GetOrCreate(msg.SessionID).SetPendingPlanApproval(&msg.Request)
	m.sidebar.SetPendingPermission(msg.SessionID, true) // Reuse permission indicator for plan approval

	// If this is the active session, show plan approval in chat
	if m.activeSession != nil && m.activeSession.ID == msg.SessionID {
		m.chat.SetPendingPlanApproval(msg.Request.Plan, msg.Request.AllowedPrompts)
	}

	// Continue listening for session events
	return m, tea.Batch(m.sessionListeners(msg.SessionID, runner, nil)...)
}

// handleGitHubIssuesFetchedMsg handles fetched GitHub issues.
// Deprecated: use handleIssuesFetchedMsg instead
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
					ID:     strconv.Itoa(issue.Number),
					Title:  issue.Title,
					Body:   issue.Body,
					URL:    issue.URL,
					Source: "github",
				}
			}
			state.SetIssues(items)
		}
	}
	return m, nil
}

// handleIssuesFetchedMsg handles fetched issues/tasks from any source.
func (m *Model) handleIssuesFetchedMsg(msg IssuesFetchedMsg) (tea.Model, tea.Cmd) {
	// Handle fetched issues - update the modal if it's still visible
	if state, ok := m.modal.State.(*ui.ImportIssuesState); ok {
		if msg.Error != nil {
			state.SetError(msg.Error.Error())
		} else {
			// Convert to UI issue items
			items := make([]ui.IssueItem, len(msg.Issues))
			for i, issue := range msg.Issues {
				items[i] = ui.IssueItem{
					ID:     issue.ID,
					Title:  issue.Title,
					Body:   issue.Body,
					URL:    issue.URL,
					Source: string(issue.Source),
				}
			}
			state.SetIssues(items)
		}
	}
	return m, nil
}

// handleReviewCommentsFetchedMsg handles fetched PR review comments.
func (m *Model) handleReviewCommentsFetchedMsg(msg ReviewCommentsFetchedMsg) (tea.Model, tea.Cmd) {
	if state, ok := m.modal.State.(*ui.ReviewCommentsState); ok {
		if msg.Error != nil {
			state.SetError(msg.Error.Error())
		} else {
			// Convert git.PRReviewComment to ui.ReviewCommentItem
			items := make([]ui.ReviewCommentItem, len(msg.Comments))
			for i, c := range msg.Comments {
				items[i] = ui.ReviewCommentItem{
					Author:   c.Author,
					Body:     c.Body,
					Path:     c.Path,
					Line:     c.Line,
					URL:      c.URL,
					Selected: true, // Pre-select all comments by default
				}
			}
			state.SetComments(items)

			// Update last-seen count and clear indicator (viewing = acknowledging)
			m.config.UpdateSessionPRCommentCount(msg.SessionID, len(msg.Comments))
			if err := m.config.Save(); err != nil {
				logger.WithSession(msg.SessionID).Error("failed to save comment count", "error", err)
			}
			m.sidebar.SetHasNewComments(msg.SessionID, false)
			m.sidebar.SetSessions(m.getFilteredSessions())
		}
	}
	return m, nil
}

// handlePRBatchStatusCheckMsg handles the batch result of checking all eligible sessions' PR states.
func (m *Model) handlePRBatchStatusCheckMsg(msg PRBatchStatusCheckMsg) (tea.Model, tea.Cmd) {
	if msg.Error != nil {
		logger.WithComponent("pr-poller").Debug("batch PR status check failed", "error", msg.Error)
		return m, nil
	}

	var cmds []tea.Cmd
	changed := false

	for _, result := range msg.Results {
		sess := m.config.GetSession(result.SessionID)
		if sess == nil {
			continue
		}

		log := logger.WithSession(result.SessionID)
		sessionName := ui.SessionDisplayName(sess.Branch, sess.Name)

		switch result.State {
		case git.PRStateMerged:
			log.Info("PR merged on GitHub", "session", sessionName)
			m.config.MarkSessionPRMerged(result.SessionID)
			changed = true

			if m.config.GetAutoCleanupMerged() {
				cleanupCmd := m.autoCleanupSession(result.SessionID, sessionName, "merged")
				if cleanupCmd != nil {
					cmds = append(cmds, cleanupCmd)
				}
			} else {
				cmds = append(cmds, m.ShowFlashSuccess("PR merged: "+sessionName))
			}

		case git.PRStateClosed:
			log.Info("PR closed on GitHub", "session", sessionName)
			m.config.MarkSessionPRClosed(result.SessionID)
			changed = true

			if m.config.GetAutoCleanupMerged() {
				cleanupCmd := m.autoCleanupSession(result.SessionID, sessionName, "closed")
				if cleanupCmd != nil {
					cmds = append(cmds, cleanupCmd)
				}
			} else {
				cmds = append(cmds, m.ShowFlashWarning("PR closed: "+sessionName))
			}

		case git.PRStateOpen:
			// Check for new comments on open PRs
			if result.CommentCount > sess.PRCommentCount {
				log.Info("new PR comments detected",
					"session", sessionName,
					"previous", sess.PRCommentCount,
					"current", result.CommentCount,
				)
				m.sidebar.SetHasNewComments(result.SessionID, true)

				// Update comment count
				m.config.UpdateSessionPRCommentCount(result.SessionID, result.CommentCount)
				changed = true

				// Auto-address comments for autonomous sessions
				if sess.Autonomous && m.config.GetAutoAddressPRComments() {
					log.Info("auto-fetching PR comments for autonomous session")
					cmds = append(cmds, m.autoFetchAndSendPRComments(result.SessionID))
				}
			}
		}
	}

	if changed {
		if err := m.config.Save(); err != nil {
			logger.WithComponent("pr-poller").Error("failed to save config after PR state change", "error", err)
		}
	}

	// Always refresh sidebar to pick up attention state changes (new comments indicator)
	m.sidebar.SetSessions(m.getFilteredSessions())

	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// formatPermissionDenialsText formats permission denials as a text block for display.
func formatPermissionDenialsText(denials []claude.PermissionDenial) string {
	if len(denials) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n[Permission Denials]\n")
	for _, d := range denials {
		sb.WriteString("  - ")
		sb.WriteString(d.Tool)
		if d.Description != "" {
			sb.WriteString(": ")
			sb.WriteString(d.Description)
		}
		if d.Reason != "" {
			sb.WriteString(" (")
			sb.WriteString(d.Reason)
			sb.WriteString(")")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
