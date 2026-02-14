package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/mcp"
	"github.com/zhubert/plural/internal/notification"
	"github.com/zhubert/plural/internal/ui"
)

// handleSessionCompletedMsg handles the completion of an autonomous session's response.
// This triggers the test loop (Phase 2) if a test command is configured,
// or emits SessionPipelineCompleteMsg directly.
func (m *Model) handleSessionCompletedMsg(msg SessionCompletedMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	sess := m.config.GetSession(msg.SessionID)
	if sess == nil {
		log.Warn("session completed but session not found")
		return m, nil
	}

	// Check if this repo has a test command configured
	testCmd := m.config.GetRepoTestCommand(sess.RepoPath)
	if testCmd != "" {
		log.Info("autonomous session completed, running tests", "testCmd", testCmd)
		return m, runTestsForSession(msg.SessionID, sess.WorkTree, testCmd, 1)
	}

	// No test command - pipeline is complete
	log.Info("autonomous session completed (no test command)")
	return m, func() tea.Msg {
		return SessionPipelineCompleteMsg{SessionID: msg.SessionID, TestsPassed: true}
	}
}

// handleTestRunResultMsg handles the result of a test run.
func (m *Model) handleTestRunResultMsg(msg TestRunResultMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	sess := m.config.GetSession(msg.SessionID)
	if sess == nil {
		return m, nil
	}

	isActiveSession := m.activeSession != nil && m.activeSession.ID == msg.SessionID

	if msg.ExitCode == 0 {
		// Tests passed
		log.Info("tests passed", "iteration", msg.Iteration)
		testMsg := fmt.Sprintf("[TESTS PASSED] (iteration %d)\n", msg.Iteration)
		if isActiveSession {
			m.chat.AppendStreaming("\n" + testMsg)
		} else {
			m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent("\n" + testMsg)
		}
		return m, func() tea.Msg {
			return SessionPipelineCompleteMsg{SessionID: msg.SessionID, TestsPassed: true}
		}
	}

	// Tests failed
	maxRetries := m.config.GetRepoTestMaxRetries(sess.RepoPath)
	log.Warn("tests failed", "iteration", msg.Iteration, "maxRetries", maxRetries, "exitCode", msg.ExitCode)

	if msg.Iteration >= maxRetries {
		// Max retries exhausted
		failMsg := fmt.Sprintf("[TESTS FAILED] Max retries (%d) exhausted\n", maxRetries)
		if isActiveSession {
			m.chat.AppendStreaming("\n" + failMsg)
		} else {
			m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent("\n" + failMsg)
		}

		// Send notification
		if m.config.GetNotificationsEnabled() {
			sessionName := ui.SessionDisplayName(sess.Branch, sess.Name)
			go notification.SessionCompleted(sessionName + " (tests failed)")
		}

		return m, func() tea.Msg {
			return SessionPipelineCompleteMsg{SessionID: msg.SessionID, TestsPassed: false}
		}
	}

	// Feed test output back to Claude for fixing
	if !sess.Autonomous {
		// Non-autonomous: just show the output, don't auto-retry
		testMsg := fmt.Sprintf("[TESTS FAILED] (iteration %d, exit code %d)\n%s\n", msg.Iteration, msg.ExitCode, msg.Output)
		if isActiveSession {
			m.chat.AppendStreaming("\n" + testMsg)
		} else {
			m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent("\n" + testMsg)
		}
		return m, nil
	}

	retryMsg := fmt.Sprintf("[TESTS FAILED] (iteration %d/%d) - sending output to Claude for fixing\n",
		msg.Iteration, maxRetries)
	if isActiveSession {
		m.chat.AppendStreaming("\n" + retryMsg)
	} else {
		m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent("\n" + retryMsg)
	}

	// Queue the test output as a pending message for Claude to fix
	// Truncate if extremely long
	output := msg.Output
	if len(output) > 10000 {
		output = output[:5000] + "\n\n... [truncated] ...\n\n" + output[len(output)-5000:]
	}
	pendingMsg := fmt.Sprintf("The tests failed (attempt %d/%d). Please fix the issues and try again.\n\nTest output:\n```\n%s\n```",
		msg.Iteration, maxRetries, output)

	state := m.sessionState().GetOrCreate(msg.SessionID)
	state.SetPendingMsg(pendingMsg)

	return m, func() tea.Msg {
		return SendPendingMessageMsg{SessionID: msg.SessionID}
	}
}

// handleSessionPipelineCompleteMsg handles the completion of a session's full pipeline.
// This triggers auto-PR creation, broadcast group completion, etc.
func (m *Model) handleSessionPipelineCompleteMsg(msg SessionPipelineCompleteMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	sess := m.config.GetSession(msg.SessionID)
	if sess == nil {
		return m, nil
	}

	var cmds []tea.Cmd

	// Phase 3B: Check broadcast group completion
	if sess.BroadcastGroupID != "" && m.config.GetAutoBroadcastPR() {
		if m.allBroadcastSessionsComplete(sess.BroadcastGroupID) {
			groupSessions := m.config.GetSessionsByBroadcastGroup(sess.BroadcastGroupID)
			log.Info("broadcast group complete, creating PRs", "groupID", sess.BroadcastGroupID, "sessions", len(groupSessions))
			cmds = append(cmds, m.ShowFlashInfo(fmt.Sprintf("Broadcast complete: creating PRs for %d sessions", len(groupSessions))))
			prCmds := m.createPRsForBroadcastGroup(groupSessions)
			cmds = append(cmds, prCmds...)
		}
	}

	// Phase 5A: Auto-create PR for autonomous sessions
	if sess.Autonomous && !sess.PRCreated && msg.TestsPassed {
		log.Info("autonomous session pipeline complete, auto-creating PR")
		prCmd := m.autoCreatePR(msg.SessionID)
		if prCmd != nil {
			cmds = append(cmds, prCmd)
		}
	}

	// Phase 5B: Notify supervisor if this is a child session
	if sess.SupervisorID != "" {
		supervisorSess := m.config.GetSession(sess.SupervisorID)
		if supervisorSess != nil {
			log.Info("notifying supervisor of child completion", "supervisorID", sess.SupervisorID)
			cmds = append(cmds, m.notifySupervisor(sess.SupervisorID, msg.SessionID, msg.TestsPassed))
		}
	}

	// Send notification for pipeline completion
	if m.config.GetNotificationsEnabled() {
		sessionName := ui.SessionDisplayName(sess.Branch, sess.Name)
		status := "completed"
		if !msg.TestsPassed {
			status = "completed (tests failed)"
		}
		go notification.SessionCompleted(sessionName + " " + status)
	}

	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// handleAutonomousLimitReachedMsg handles when an autonomous session hits its safety limits.
func (m *Model) handleAutonomousLimitReachedMsg(msg AutonomousLimitReachedMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	sess := m.config.GetSession(msg.SessionID)
	if sess == nil {
		return m, nil
	}

	var limitText string
	switch msg.Reason {
	case "turn_limit":
		maxTurns := m.config.GetAutoMaxTurns()
		limitText = fmt.Sprintf("[AUTONOMOUS LIMIT] Stopped after %d turns (max: %d)", maxTurns, maxTurns)
	case "duration_limit":
		maxDur := m.config.GetAutoMaxDurationMin()
		limitText = fmt.Sprintf("[AUTONOMOUS LIMIT] Stopped after %d minutes (max: %d)", maxDur, maxDur)
	default:
		limitText = "[AUTONOMOUS LIMIT] Stopped: " + msg.Reason
	}

	log.Warn("autonomous session stopped", "reason", msg.Reason)

	isActiveSession := m.activeSession != nil && m.activeSession.ID == msg.SessionID
	if isActiveSession {
		m.chat.AppendStreaming("\n" + limitText + "\n")
	} else {
		m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent("\n" + limitText + "\n")
	}

	// Disable autonomous mode for this session
	m.config.SetSessionAutonomous(msg.SessionID, false)
	if err := m.config.Save(); err != nil {
		log.Error("failed to save config after disabling autonomous mode", "error", err)
	}
	m.sidebar.SetSessions(m.getFilteredSessions())

	// Send notification
	if m.config.GetNotificationsEnabled() {
		sessionName := ui.SessionDisplayName(sess.Branch, sess.Name)
		go notification.SessionCompleted(sessionName + " (autonomous limit reached)")
	}

	return m, m.ShowFlashWarning(fmt.Sprintf("Autonomous session stopped: %s", msg.Reason))
}

// allBroadcastSessionsComplete checks if all sessions in a broadcast group have finished.
func (m *Model) allBroadcastSessionsComplete(groupID string) bool {
	groupSessions := m.config.GetSessionsByBroadcastGroup(groupID)
	for _, sess := range groupSessions {
		state := m.sessionState().GetIfExists(sess.ID)
		if state != nil {
			if state.GetIsWaiting() || state.IsMerging() {
				return false
			}
		}
		runner := m.sessionMgr.GetRunner(sess.ID)
		if runner != nil && runner.IsStreaming() {
			return false
		}
	}
	return true
}

// createPRsForBroadcastGroup creates PRs for all sessions in a broadcast group.
func (m *Model) createPRsForBroadcastGroup(sessions []config.Session) []tea.Cmd {
	var cmds []tea.Cmd
	for _, sess := range sessions {
		if sess.PRCreated || sess.Merged {
			continue
		}
		prCmd := m.autoCreatePR(sess.ID)
		if prCmd != nil {
			cmds = append(cmds, prCmd)
		}
	}
	return cmds
}

// autoCreatePR creates a PR for a session automatically using the existing merge flow.
func (m *Model) autoCreatePR(sessionID string) tea.Cmd {
	log := logger.WithSession(sessionID)
	sess := m.config.GetSession(sessionID)
	if sess == nil {
		return nil
	}

	// Check if already merging
	state := m.sessionState().GetIfExists(sessionID)
	if state != nil && state.IsMerging() {
		log.Debug("merge already in progress, skipping auto-PR")
		return nil
	}

	log.Info("auto-creating PR", "branch", sess.Branch)

	isActiveSession := m.activeSession != nil && m.activeSession.ID == sessionID
	autoMsg := "[AUTO] Creating PR for " + sess.Branch + "...\n"
	if isActiveSession {
		m.chat.AppendStreaming("\n" + autoMsg)
	} else {
		m.sessionState().GetOrCreate(sessionID).AppendStreamingContent("\n" + autoMsg)
	}

	// Use the existing CreatePR flow which handles commit, push, and PR creation
	mergeCtx, cancel := context.WithCancel(context.Background())
	m.sessionState().StartMerge(sessionID,
		m.gitService.CreatePR(mergeCtx, sess.RepoPath, sess.WorkTree, sess.Branch, sess.BaseBranch, "", sess.GetIssueRef()),
		cancel, MergeTypePR)

	return m.listenForMergeResult(sessionID)
}

// autoCleanupSession cleans up a session that has been merged or closed.
// It stops the runner, deletes the worktree, and removes the session from config.
func (m *Model) autoCleanupSession(sessionID, sessionName, reason string) tea.Cmd {
	log := logger.WithSession(sessionID)
	sess := m.config.GetSession(sessionID)
	if sess == nil {
		return nil
	}

	// If this is the active session, switch away first
	if m.activeSession != nil && m.activeSession.ID == sessionID {
		log.Info("auto-cleanup: clearing active session before cleanup")
		m.activeSession = nil
		m.claudeRunner = nil
		m.chat.ClearSession()
		m.header.SetSessionName("")
		m.header.SetBaseBranch("")
		m.header.SetDiffStats(nil)
	}

	// Delete worktree
	ctx := context.Background()
	if err := m.sessionService.Delete(ctx, sess); err != nil {
		log.Warn("auto-cleanup: failed to delete worktree", "error", err)
	}

	// Clean up runner and state
	m.sessionMgr.DeleteSession(sessionID)
	m.sidebar.SetPendingPermission(sessionID, false)
	m.sidebar.SetPendingQuestion(sessionID, false)
	m.sidebar.SetIdleWithResponse(sessionID, false)
	m.sidebar.SetUncommittedChanges(sessionID, false)
	m.sidebar.SetHasNewComments(sessionID, false)

	// Remove session from config
	m.config.RemoveSession(sessionID)
	m.config.ClearOrphanedParentIDs([]string{sessionID})
	config.DeleteSessionMessages(sessionID)

	// Save is handled by the caller (handlePRBatchStatusCheckMsg sets changed=true)

	m.sidebar.SetSessions(m.getFilteredSessions())

	log.Info("auto-cleaned session", "reason", reason)
	return m.ShowFlashInfo(fmt.Sprintf("Auto-cleaned: %s (PR %s)", sessionName, reason))
}

// handleAutoPRCommentsFetchedMsg handles fetched PR review comments by queuing them for Claude.
func (m *Model) handleAutoPRCommentsFetchedMsg(msg AutoPRCommentsFetchedMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	sess := m.config.GetSession(msg.SessionID)
	if sess == nil {
		return m, nil
	}

	log.Info("sending PR review comments to Claude", "sessionID", msg.SessionID)

	isActiveSession := m.activeSession != nil && m.activeSession.ID == msg.SessionID
	autoMsg := "[AUTO] Addressing PR review comments...\n"
	if isActiveSession {
		m.chat.AppendStreaming("\n" + autoMsg)
	} else {
		m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent("\n" + autoMsg)
	}

	state := m.sessionState().GetOrCreate(msg.SessionID)
	state.SetPendingMsg(msg.Prompt)

	return m, func() tea.Msg {
		return SendPendingMessageMsg{SessionID: msg.SessionID}
	}
}

// autoFetchAndSendPRComments fetches PR review comments and sends them to Claude for an autonomous session.
func (m *Model) autoFetchAndSendPRComments(sessionID string) tea.Cmd {
	sess := m.config.GetSession(sessionID)
	if sess == nil {
		return nil
	}

	repoPath := sess.RepoPath
	branch := sess.Branch
	gitSvc := m.gitService

	return func() tea.Msg {
		log := logger.WithSession(sessionID)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		comments, err := gitSvc.FetchPRReviewComments(ctx, repoPath, branch)
		if err != nil {
			log.Warn("failed to fetch PR review comments", "error", err)
			return nil
		}

		if len(comments) == 0 {
			return nil
		}

		// Format comments as a prompt
		prompt := formatPRCommentsPrompt(comments)
		return AutoPRCommentsFetchedMsg{
			SessionID: sessionID,
			Prompt:    prompt,
		}
	}
}

// AutoPRCommentsFetchedMsg carries fetched PR comments to be sent to Claude.
type AutoPRCommentsFetchedMsg struct {
	SessionID string
	Prompt    string
}

// formatPRCommentsPrompt formats PR review comments as a prompt string.
func formatPRCommentsPrompt(comments []git.PRReviewComment) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("New PR review comments need to be addressed (%d comment(s)):\n\n", len(comments)))

	for i, c := range comments {
		sb.WriteString(fmt.Sprintf("--- Comment %d", i+1))
		if c.Author != "" {
			sb.WriteString(fmt.Sprintf(" by @%s", c.Author))
		}
		sb.WriteString(" ---\n")
		if c.Path != "" {
			if c.Line > 0 {
				sb.WriteString(fmt.Sprintf("File: %s:%d\n", c.Path, c.Line))
			} else {
				sb.WriteString(fmt.Sprintf("File: %s\n", c.Path))
			}
		}
		sb.WriteString(c.Body)
		sb.WriteString("\n\n")
	}

	sb.WriteString("Please address each of these review comments. For code changes, make the necessary edits. For questions, provide a response and make any relevant code changes.")
	return sb.String()
}

// notifySupervisor sends a status update about a child session to the supervisor.
func (m *Model) notifySupervisor(supervisorID, childID string, testsPassed bool) tea.Cmd {
	childSess := m.config.GetSession(childID)
	if childSess == nil {
		return nil
	}

	sessionName := ui.SessionDisplayName(childSess.Branch, childSess.Name)
	status := "completed successfully"
	if !testsPassed {
		status = "completed (tests failed)"
	}

	// Check if all children are done
	allChildren := m.config.GetChildSessions(supervisorID)
	allDone := true
	completedCount := 0
	for _, child := range allChildren {
		childState := m.sessionState().GetIfExists(child.ID)
		runner := m.sessionMgr.GetRunner(child.ID)
		if runner != nil && runner.IsStreaming() {
			allDone = false
		} else if childState != nil && (childState.GetIsWaiting() || childState.IsMerging()) {
			allDone = false
		} else {
			completedCount++
		}
	}

	var prompt string
	if allDone {
		prompt = fmt.Sprintf("Child session '%s' %s.\n\nAll %d child sessions have completed. You may now review the results, merge children to parent, or create PRs.",
			sessionName, status, len(allChildren))
	} else {
		prompt = fmt.Sprintf("Child session '%s' %s. (%d/%d children completed)",
			sessionName, status, completedCount, len(allChildren))
	}

	state := m.sessionState().GetOrCreate(supervisorID)
	state.SetPendingMsg(prompt)

	return func() tea.Msg {
		return SendPendingMessageMsg{SessionID: supervisorID}
	}
}

// createChildSession creates an autonomous child session from a supervisor session.
func (m *Model) createChildSession(supervisorID, taskDescription string) tea.Cmd {
	log := logger.WithSession(supervisorID)
	supervisorSess := m.config.GetSession(supervisorID)
	if supervisorSess == nil {
		return nil
	}

	ctx := context.Background()
	branchPrefix := m.config.GetDefaultBranchPrefix()

	// Generate a branch name from the task
	branchName := fmt.Sprintf("child-%s", time.Now().Format("20060102-150405"))

	// Create from supervisor's branch
	sess, err := m.sessionService.CreateFromBranch(ctx, supervisorSess.RepoPath, supervisorSess.Branch, branchName, branchPrefix)
	if err != nil {
		log.Error("failed to create child session", "error", err)
		return m.ShowFlashError("Failed to create child session")
	}

	sess.Autonomous = true
	sess.Containerized = supervisorSess.Containerized
	sess.SupervisorID = supervisorID
	sess.ParentID = supervisorID

	// Auto-assign to active workspace
	if activeWS := m.config.GetActiveWorkspaceID(); activeWS != "" {
		sess.WorkspaceID = activeWS
	}

	m.config.AddSession(*sess)
	m.config.AddChildSession(supervisorID, sess.ID)

	if err := m.config.Save(); err != nil {
		log.Error("failed to save config after creating child session", "error", err)
	}
	m.sidebar.SetSessions(m.getFilteredSessions())

	log.Info("created child session", "childID", sess.ID, "branch", sess.Branch)

	// Start the child session
	result := m.sessionMgr.Select(sess, "", "", "")
	if result == nil || result.Runner == nil {
		logger.WithSession(sess.ID).Error("failed to get runner for child session")
		return m.ShowFlashError("Failed to start child session")
	}

	runner := result.Runner
	sendCtx, cancel := context.WithCancel(context.Background())
	m.sessionState().StartWaiting(sess.ID, cancel)
	m.sidebar.SetStreaming(sess.ID, true)

	initialMsg := fmt.Sprintf("You are a child session working on a specific task assigned by a supervisor session.\n\nTask: %s\n\nPlease complete this task. When you are done, make sure all changes are committed.", taskDescription)

	content := []claude.ContentBlock{{Type: claude.ContentTypeText, Text: initialMsg}}
	responseChan := runner.SendContent(sendCtx, content)

	var cmds []tea.Cmd
	cmds = append(cmds, m.sessionListeners(sess.ID, runner, responseChan)...)
	cmds = append(cmds, m.ShowFlashInfo(fmt.Sprintf("Created child session: %s", sess.Branch)))
	cmds = append(cmds, ui.SidebarTick(), ui.StopwatchTick())
	m.setState(StateStreamingClaude)

	return tea.Batch(cmds...)
}

// CIPollResultMsg carries the result of a CI status check for auto-merge.
type CIPollResultMsg struct {
	SessionID string
	Status    git.CIStatus
}

// pollCIForAutoMerge polls CI status for a session and returns the result.
func (m *Model) pollCIForAutoMerge(sessionID string) tea.Cmd {
	sess := m.config.GetSession(sessionID)
	if sess == nil {
		return nil
	}

	repoPath := sess.RepoPath
	branch := sess.Branch
	gitSvc := m.gitService

	return func() tea.Msg {
		// Wait before first check to give CI time to start
		time.Sleep(30 * time.Second)

		log := logger.WithSession(sessionID)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		status, err := gitSvc.CheckPRChecks(ctx, repoPath, branch)
		if err != nil {
			log.Warn("failed to check CI status", "error", err)
			return CIPollResultMsg{SessionID: sessionID, Status: git.CIStatusPending}
		}

		return CIPollResultMsg{SessionID: sessionID, Status: status}
	}
}

// handleCIPollResultMsg handles CI poll results for auto-merge.
func (m *Model) handleCIPollResultMsg(msg CIPollResultMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	sess := m.config.GetSession(msg.SessionID)
	if sess == nil {
		return m, nil
	}

	isActiveSession := m.activeSession != nil && m.activeSession.ID == msg.SessionID

	switch msg.Status {
	case git.CIStatusPassing:
		log.Info("CI checks passed, auto-merging PR", "branch", sess.Branch)
		autoMsg := "[AUTO] CI checks passed, merging PR...\n"
		if isActiveSession {
			m.chat.AppendStreaming("\n" + autoMsg)
		} else {
			m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent("\n" + autoMsg)
		}
		return m, m.autoMergePR(msg.SessionID)

	case git.CIStatusFailing:
		log.Warn("CI checks failed, skipping auto-merge", "branch", sess.Branch)
		failMsg := "[AUTO] CI checks failed - skipping auto-merge\n"
		if isActiveSession {
			m.chat.AppendStreaming("\n" + failMsg)
		} else {
			m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent("\n" + failMsg)
		}
		if m.config.GetNotificationsEnabled() {
			sessionName := ui.SessionDisplayName(sess.Branch, sess.Name)
			go notification.SessionCompleted(sessionName + " (CI failed)")
		}
		return m, nil

	case git.CIStatusPending:
		// Still pending, poll again
		log.Debug("CI checks still pending, will poll again", "branch", sess.Branch)
		return m, m.pollCIForAutoMerge(msg.SessionID)

	case git.CIStatusNone:
		// No checks configured, merge immediately
		log.Info("no CI checks configured, auto-merging PR", "branch", sess.Branch)
		autoMsg := "[AUTO] No CI checks configured, merging PR...\n"
		if isActiveSession {
			m.chat.AppendStreaming("\n" + autoMsg)
		} else {
			m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent("\n" + autoMsg)
		}
		return m, m.autoMergePR(msg.SessionID)
	}

	return m, nil
}

// AutoMergeResultMsg carries the result of an auto-merge attempt.
type AutoMergeResultMsg struct {
	SessionID string
	Error     error
}

// autoMergePR merges a PR automatically.
func (m *Model) autoMergePR(sessionID string) tea.Cmd {
	sess := m.config.GetSession(sessionID)
	if sess == nil {
		return nil
	}

	repoPath := sess.RepoPath
	branch := sess.Branch
	gitSvc := m.gitService

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err := gitSvc.MergePR(ctx, repoPath, branch)
		return AutoMergeResultMsg{SessionID: sessionID, Error: err}
	}
}

// handleAutoMergeResultMsg handles the result of an auto-merge attempt.
func (m *Model) handleAutoMergeResultMsg(msg AutoMergeResultMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	sess := m.config.GetSession(msg.SessionID)
	if sess == nil {
		return m, nil
	}

	isActiveSession := m.activeSession != nil && m.activeSession.ID == msg.SessionID

	if msg.Error != nil {
		log.Error("auto-merge failed", "error", msg.Error)
		errMsg := fmt.Sprintf("[AUTO] Merge failed: %s\n", msg.Error.Error())
		if isActiveSession {
			m.chat.AppendStreaming("\n" + errMsg)
		} else {
			m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent("\n" + errMsg)
		}
		return m, m.ShowFlashError(fmt.Sprintf("Auto-merge failed: %s", sess.Branch))
	}

	log.Info("auto-merge successful", "branch", sess.Branch)
	successMsg := "[AUTO] PR merged successfully!\n"
	if isActiveSession {
		m.chat.AppendStreaming("\n" + successMsg)
	} else {
		m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent("\n" + successMsg)
	}

	// Mark as merged
	m.config.MarkSessionPRMerged(msg.SessionID)
	if err := m.config.Save(); err != nil {
		log.Error("failed to save config after auto-merge", "error", err)
	}
	m.sidebar.SetSessions(m.getFilteredSessions())

	// Send notification
	if m.config.GetNotificationsEnabled() {
		sessionName := ui.SessionDisplayName(sess.Branch, sess.Name)
		go notification.SessionCompleted(sessionName + " (auto-merged)")
	}

	return m, m.ShowFlashSuccess(fmt.Sprintf("Auto-merged: %s", sess.Branch))
}

// contextWithTimeout creates a context with a timeout for async operations.
func contextWithTimeout(d time.Duration) context.Context {
	ctx, _ := context.WithTimeout(context.Background(), d)
	return ctx
}

// handleCreateChildRequestMsg handles a create_child_session MCP tool call from the supervisor.
func (m *Model) handleCreateChildRequestMsg(msg CreateChildRequestMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	runner := m.sessionMgr.GetRunner(msg.SessionID)
	if runner == nil {
		log.Warn("create child request for unknown session")
		return m, nil
	}

	sess := m.config.GetSession(msg.SessionID)
	if sess == nil || !sess.IsSupervisor {
		runner.SendCreateChildResponse(mcp.CreateChildResponse{
			ID:    msg.Request.ID,
			Error: "Session is not a supervisor",
		})
		return m, tea.Batch(m.sessionListeners(msg.SessionID, runner, nil)...)
	}

	// Create the child session synchronously
	ctx := context.Background()
	branchPrefix := m.config.GetDefaultBranchPrefix()
	branchName := fmt.Sprintf("child-%s", time.Now().Format("20060102-150405"))

	childSess, err := m.sessionService.CreateFromBranch(ctx, sess.RepoPath, sess.Branch, branchName, branchPrefix)
	if err != nil {
		log.Error("failed to create child session", "error", err)
		runner.SendCreateChildResponse(mcp.CreateChildResponse{
			ID:    msg.Request.ID,
			Error: fmt.Sprintf("Failed to create child session: %v", err),
		})
		return m, tea.Batch(m.sessionListeners(msg.SessionID, runner, nil)...)
	}

	childSess.Autonomous = true
	childSess.Containerized = sess.Containerized
	childSess.SupervisorID = msg.SessionID
	childSess.ParentID = msg.SessionID

	if activeWS := m.config.GetActiveWorkspaceID(); activeWS != "" {
		childSess.WorkspaceID = activeWS
	}

	m.config.AddSession(*childSess)
	m.config.AddChildSession(msg.SessionID, childSess.ID)

	if err := m.config.Save(); err != nil {
		log.Error("failed to save config after creating child session", "error", err)
	}
	m.sidebar.SetSessions(m.getFilteredSessions())

	log.Info("created child session via MCP tool", "childID", childSess.ID, "branch", childSess.Branch)

	// Send response immediately so the supervisor can continue
	runner.SendCreateChildResponse(mcp.CreateChildResponse{
		ID:      msg.Request.ID,
		Success: true,
		ChildID: childSess.ID,
		Branch:  childSess.Branch,
	})

	// Start the child session asynchronously
	childResult := m.sessionMgr.Select(childSess, "", "", "")
	if childResult == nil || childResult.Runner == nil {
		logger.WithSession(childSess.ID).Error("failed to get runner for child session")
		return m, tea.Batch(m.sessionListeners(msg.SessionID, runner, nil)...)
	}

	childRunner := childResult.Runner
	sendCtx, cancel := context.WithCancel(context.Background())
	m.sessionState().StartWaiting(childSess.ID, cancel)
	m.sidebar.SetStreaming(childSess.ID, true)

	initialMsg := fmt.Sprintf("You are a child session working on a specific task assigned by a supervisor session.\n\nTask: %s\n\nPlease complete this task. When you are done, make sure all changes are committed.", msg.Request.Task)
	content := []claude.ContentBlock{{Type: claude.ContentTypeText, Text: initialMsg}}
	responseChan := childRunner.SendContent(sendCtx, content)

	var cmds []tea.Cmd
	// Re-register supervisor listeners
	cmds = append(cmds, m.sessionListeners(msg.SessionID, runner, nil)...)
	// Register child listeners
	cmds = append(cmds, m.sessionListeners(childSess.ID, childRunner, responseChan)...)
	cmds = append(cmds, ui.SidebarTick(), ui.StopwatchTick())

	return m, tea.Batch(cmds...)
}

// handleListChildrenRequestMsg handles a list_child_sessions MCP tool call from the supervisor.
func (m *Model) handleListChildrenRequestMsg(msg ListChildrenRequestMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	runner := m.sessionMgr.GetRunner(msg.SessionID)
	if runner == nil {
		log.Warn("list children request for unknown session")
		return m, nil
	}

	children := m.config.GetChildSessions(msg.SessionID)
	var childInfos []mcp.ChildSessionInfo
	for _, child := range children {
		status := "idle"
		childRunner := m.sessionMgr.GetRunner(child.ID)
		if childRunner != nil && childRunner.IsStreaming() {
			status = "running"
		} else if child.MergedToParent {
			status = "merged"
		} else if child.PRCreated {
			status = "pr_created"
		} else {
			// Check if waiting/in-progress
			childState := m.sessionState().GetIfExists(child.ID)
			if childState != nil && childState.GetIsWaiting() {
				status = "running"
			}
		}
		childInfos = append(childInfos, mcp.ChildSessionInfo{
			ID:     child.ID,
			Branch: child.Branch,
			Status: status,
		})
	}

	runner.SendListChildrenResponse(mcp.ListChildrenResponse{
		ID:       msg.Request.ID,
		Children: childInfos,
	})

	return m, tea.Batch(m.sessionListeners(msg.SessionID, runner, nil)...)
}

// handleMergeChildRequestMsg handles a merge_child_to_parent MCP tool call from the supervisor.
func (m *Model) handleMergeChildRequestMsg(msg MergeChildRequestMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	runner := m.sessionMgr.GetRunner(msg.SessionID)
	if runner == nil {
		log.Warn("merge child request for unknown session")
		return m, nil
	}

	sess := m.config.GetSession(msg.SessionID)
	if sess == nil {
		runner.SendMergeChildResponse(mcp.MergeChildResponse{
			ID:    msg.Request.ID,
			Error: "Supervisor session not found",
		})
		return m, tea.Batch(m.sessionListeners(msg.SessionID, runner, nil)...)
	}

	childSess := m.config.GetSession(msg.Request.ChildSessionID)
	if childSess == nil {
		runner.SendMergeChildResponse(mcp.MergeChildResponse{
			ID:    msg.Request.ID,
			Error: "Child session not found",
		})
		return m, tea.Batch(m.sessionListeners(msg.SessionID, runner, nil)...)
	}

	if childSess.SupervisorID != msg.SessionID {
		runner.SendMergeChildResponse(mcp.MergeChildResponse{
			ID:    msg.Request.ID,
			Error: "Child session does not belong to this supervisor",
		})
		return m, tea.Batch(m.sessionListeners(msg.SessionID, runner, nil)...)
	}

	if childSess.MergedToParent {
		runner.SendMergeChildResponse(mcp.MergeChildResponse{
			ID:    msg.Request.ID,
			Error: "Child session already merged",
		})
		return m, tea.Batch(m.sessionListeners(msg.SessionID, runner, nil)...)
	}

	log.Info("merging child to parent via MCP tool", "childID", childSess.ID, "childBranch", childSess.Branch)

	// Capture values for closure
	supervisorID := msg.SessionID
	childID := childSess.ID
	childWorkTree := childSess.WorkTree
	childBranch := childSess.Branch
	supervisorWorkTree := sess.WorkTree
	supervisorBranch := sess.Branch
	requestID := msg.Request.ID
	gitSvc := m.gitService

	// Re-register supervisor listeners first
	cmds := m.sessionListeners(msg.SessionID, runner, nil)

	// Run merge asynchronously
	mergeCmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		resultCh := gitSvc.MergeToParent(ctx, childWorkTree, childBranch, supervisorWorkTree, supervisorBranch, "")

		var lastResult git.Result
		for result := range resultCh {
			lastResult = result
		}

		if lastResult.Error != nil {
			return MergeChildCompleteMsg{
				SessionID: supervisorID,
				ChildID:   childID,
				Error:     lastResult.Error,
			}
		}

		return MergeChildCompleteMsg{
			SessionID: supervisorID,
			ChildID:   childID,
			Success:   true,
			Message:   fmt.Sprintf("Successfully merged %s into %s", childBranch, supervisorBranch),
		}
	}

	// Store the request ID so the completion handler can send the response
	state := m.sessionState().GetOrCreate(supervisorID)
	state.SetPendingMergeChildRequestID(requestID)

	cmds = append(cmds, mergeCmd)
	return m, tea.Batch(cmds...)
}

// handleMergeChildCompleteMsg handles the completion of a child-to-parent merge.
func (m *Model) handleMergeChildCompleteMsg(msg MergeChildCompleteMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	runner := m.sessionMgr.GetRunner(msg.SessionID)
	if runner == nil {
		log.Warn("merge child complete for unknown session")
		return m, nil
	}

	// Get the stored request ID
	state := m.sessionState().GetIfExists(msg.SessionID)
	var requestID interface{}
	if state != nil {
		requestID = state.GetPendingMergeChildRequestID()
		state.SetPendingMergeChildRequestID(nil)
	}

	if msg.Error != nil {
		log.Error("merge child failed", "childID", msg.ChildID, "error", msg.Error)
		runner.SendMergeChildResponse(mcp.MergeChildResponse{
			ID:    requestID,
			Error: msg.Error.Error(),
		})
	} else {
		log.Info("merge child succeeded", "childID", msg.ChildID)
		// Mark child as merged
		m.config.MarkSessionMergedToParent(msg.ChildID)
		if err := m.config.Save(); err != nil {
			log.Error("failed to save config after merge", "error", err)
		}
		m.sidebar.SetSessions(m.getFilteredSessions())

		runner.SendMergeChildResponse(mcp.MergeChildResponse{
			ID:      requestID,
			Success: true,
			Message: msg.Message,
		})
	}

	return m, tea.Batch(m.sessionListeners(msg.SessionID, runner, nil)...)
}
