package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/mcp"
)

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

	cmds := []tea.Cmd{
		m.listenForSessionResponse(sessionID, ch),
		m.listenForSessionPermission(sessionID, runner),
		m.listenForSessionQuestion(sessionID, runner),
		m.listenForSessionPlanApproval(sessionID, runner),
	}

	// Add supervisor tool listeners if this runner has supervisor channels
	if runner.CreateChildRequestChan() != nil {
		cmds = append(cmds,
			m.listenForCreateChildRequest(sessionID, runner),
			m.listenForListChildrenRequest(sessionID, runner),
			m.listenForMergeChildRequest(sessionID, runner),
		)
	}

	// Add host tool listeners if this runner has host tool channels
	if runner.CreatePRRequestChan() != nil {
		cmds = append(cmds,
			m.listenForCreatePRRequest(sessionID, runner),
			m.listenForPushBranchRequest(sessionID, runner),
			m.listenForGetReviewCommentsRequest(sessionID, runner),
		)
	}

	return cmds
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

// listenForCreateChildRequest creates a command to listen for create child requests from a supervisor session
func (m *Model) listenForCreateChildRequest(sessionID string, runner claude.RunnerInterface) tea.Cmd {
	ch := runner.CreateChildRequestChan()
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		req, ok := <-ch
		if !ok {
			return nil
		}
		return CreateChildRequestMsg{SessionID: sessionID, Request: req}
	}
}

// listenForListChildrenRequest creates a command to listen for list children requests from a supervisor session
func (m *Model) listenForListChildrenRequest(sessionID string, runner claude.RunnerInterface) tea.Cmd {
	ch := runner.ListChildrenRequestChan()
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		req, ok := <-ch
		if !ok {
			return nil
		}
		return ListChildrenRequestMsg{SessionID: sessionID, Request: req}
	}
}

// listenForMergeChildRequest creates a command to listen for merge child requests from a supervisor session
func (m *Model) listenForMergeChildRequest(sessionID string, runner claude.RunnerInterface) tea.Cmd {
	ch := runner.MergeChildRequestChan()
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		req, ok := <-ch
		if !ok {
			return nil
		}
		return MergeChildRequestMsg{SessionID: sessionID, Request: req}
	}
}

// listenForCreatePRRequest creates a command to listen for create PR requests from an automated supervisor
func (m *Model) listenForCreatePRRequest(sessionID string, runner claude.RunnerInterface) tea.Cmd {
	ch := runner.CreatePRRequestChan()
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		req, ok := <-ch
		if !ok {
			return nil
		}
		return CreatePRRequestMsg{SessionID: sessionID, Request: req}
	}
}

// listenForPushBranchRequest creates a command to listen for push branch requests from an automated supervisor
func (m *Model) listenForPushBranchRequest(sessionID string, runner claude.RunnerInterface) tea.Cmd {
	ch := runner.PushBranchRequestChan()
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		req, ok := <-ch
		if !ok {
			return nil
		}
		return PushBranchRequestMsg{SessionID: sessionID, Request: req}
	}
}

// listenForGetReviewCommentsRequest creates a command to listen for get review comments requests from an automated supervisor
func (m *Model) listenForGetReviewCommentsRequest(sessionID string, runner claude.RunnerInterface) tea.Cmd {
	ch := runner.GetReviewCommentsRequestChan()
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		req, ok := <-ch
		if !ok {
			return nil
		}
		return GetReviewCommentsRequestMsg{SessionID: sessionID, Request: req}
	}
}

// listenForMergeResult creates a command to listen for merge operation results
func (m *Model) listenForMergeResult(sessionID string) tea.Cmd {
	state := m.sessionState().GetIfExists(sessionID)
	if state == nil {
		return nil
	}
	ch := state.GetMergeChan()
	if ch == nil {
		return nil
	}

	return func() tea.Msg {
		result, ok := <-ch
		if !ok {
			return MergeResultMsg{SessionID: sessionID, Result: git.Result{Done: true}}
		}
		return MergeResultMsg{SessionID: sessionID, Result: result}
	}
}

// handlePermissionResponse handles y/n/a key presses for permission prompts
func (m *Model) handlePermissionResponse(key string, sessionID string, req *mcp.PermissionRequest) (tea.Model, tea.Cmd) {
	runner := m.sessionMgr.GetRunner(sessionID)
	if runner == nil {
		logger.WithSession(sessionID).Warn("permission response for unknown session")
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

	logger.WithSession(sessionID).Debug("permission response", "key", key, "allowed", allowed, "always", always)

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
	if state := m.sessionState().GetIfExists(sessionID); state != nil {
		state.SetPendingPermission(nil)
	}
	m.sidebar.SetPendingPermission(sessionID, false)
	m.chat.ClearPendingPermission()

	// Continue listening for session events
	return m, tea.Batch(m.sessionListeners(sessionID, runner, nil)...)
}

// submitQuestionResponse sends the collected question answers back to Claude
func (m *Model) submitQuestionResponse(sessionID string) (tea.Model, tea.Cmd) {
	log := logger.WithSession(sessionID)
	runner := m.sessionMgr.GetRunner(sessionID)
	if runner == nil {
		log.Warn("question response for unknown session")
		return m, nil
	}

	state := m.sessionState().GetIfExists(sessionID)
	if state == nil {
		log.Warn("no pending question for session")
		return m, nil
	}
	req := state.GetPendingQuestion()
	if req == nil {
		log.Warn("no pending question for session")
		return m, nil
	}

	// Get answers from chat
	answers := m.chat.GetQuestionAnswers()
	log.Debug("question response", "answerCount", len(answers))

	// Build response
	resp := mcp.QuestionResponse{
		ID:      req.ID,
		Answers: answers,
	}

	// Send response
	runner.SendQuestionResponse(resp)

	// Clear pending question
	state.SetPendingQuestion(nil)
	m.sidebar.SetPendingPermission(sessionID, false)
	m.sidebar.SetPendingQuestion(sessionID, false)
	m.chat.ClearPendingQuestion()

	// Continue listening for session events
	return m, tea.Batch(m.sessionListeners(sessionID, runner, nil)...)
}

// submitPlanApprovalResponse sends the plan approval response back to Claude
func (m *Model) submitPlanApprovalResponse(sessionID string, approved bool) (tea.Model, tea.Cmd) {
	log := logger.WithSession(sessionID)
	runner := m.sessionMgr.GetRunner(sessionID)
	if runner == nil {
		log.Warn("plan approval response for unknown session")
		return m, nil
	}

	state := m.sessionState().GetIfExists(sessionID)
	if state == nil {
		log.Warn("no pending plan approval for session")
		return m, nil
	}
	req := state.GetPendingPlanApproval()
	if req == nil {
		log.Warn("no pending plan approval for session")
		return m, nil
	}

	log.Debug("plan approval response", "approved", approved)

	// Build response
	resp := mcp.PlanApprovalResponse{
		ID:       req.ID,
		Approved: approved,
	}

	// Send response
	runner.SendPlanApprovalResponse(resp)

	// Clear pending plan approval
	state.SetPendingPlanApproval(nil)
	m.sidebar.SetPendingPermission(sessionID, false)
	m.chat.ClearPendingPlanApproval()

	// Continue listening for session events
	return m, tea.Batch(m.sessionListeners(sessionID, runner, nil)...)
}
