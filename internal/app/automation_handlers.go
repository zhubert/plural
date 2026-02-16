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
// Emits SessionPipelineCompleteMsg directly.
func (m *Model) handleSessionCompletedMsg(msg SessionCompletedMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	sess := m.config.GetSession(msg.SessionID)
	if sess == nil {
		log.Warn("session completed but session not found")
		return m, nil
	}

	log.Info("autonomous session completed")
	return m, func() tea.Msg {
		return SessionPipelineCompleteMsg{SessionID: msg.SessionID, TestsPassed: true}
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

	// Phase 5A: Auto-create PR for standalone autonomous sessions.
	// Supervisors decide whether to create a PR or stop for user input.
	// Child sessions notify their supervisor instead (Phase 5B).
	if sess.Autonomous && !sess.IsSupervisor && sess.SupervisorID == "" && !sess.PRCreated && msg.TestsPassed {
		log.Info("autonomous session pipeline complete, auto-creating PR")
		prCmd := m.autoCreatePR(msg.SessionID)
		if prCmd != nil {
			cmds = append(cmds, prCmd)
		}
	}

	// Restart auto-merge polling if session already has a PR and auto-merge is enabled.
	// This handles the case where the session just finished addressing PR review comments
	// and new commits may have triggered new CI runs that need to pass before merging.
	if sess.Autonomous && sess.PRCreated && !sess.PRMerged && !sess.PRClosed &&
		m.config.GetRepoAutoMerge(sess.RepoPath) {
		log.Info("restarting auto-merge polling after session completion", "branch", sess.Branch)
		cmds = append(cmds, m.pollForAutoMerge(msg.SessionID))
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

	// Stop any active streaming before cleanup. DeleteSession calls runner.Stop()
	// which cancels in-flight operations. In-flight messages referencing this session
	// will get nil session checks and return early.
	m.sidebar.SetStreaming(sessionID, false)

	// Delete worktree
	ctx := context.Background()
	if err := m.sessionService.Delete(ctx, sess); err != nil {
		log.Warn("auto-cleanup: failed to delete worktree", "error", err)
	}

	// Clean up runner and state (stops runner, cancels operations, deletes state)
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

	if err := m.config.Save(); err != nil {
		log.Error("failed to save config after auto-cleanup", "error", err)
	}

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
		prompt = fmt.Sprintf("Child session '%s' %s.\n\nAll %d child sessions have completed. You should now review the results, merge children to parent with `merge_child_to_parent`, and create a PR with `push_branch` and `create_pr`.",
			sessionName, status, len(allChildren))
	} else {
		prompt = fmt.Sprintf("Child session '%s' %s. (%d/%d children completed)\n\nWait for all children to complete before merging or creating PRs.",
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
	cmds = append(cmds, m.sidebar.SidebarTick(), m.chat.SpinnerTick())
	// Only transition to streaming state if not already there, to avoid
	// disrupting user interaction in the currently active session.
	if m.state != StateStreamingClaude {
		m.setState(StateStreamingClaude)
	}

	return tea.Batch(cmds...)
}

// maxAutoMergePollAttempts is the maximum number of auto-merge poll attempts before giving up.
const maxAutoMergePollAttempts = 60 // ~30 minutes at 30s intervals

// AutoMergePollResultMsg carries the result of checking review state + CI status for auto-merge.
// The auto-merge state machine proceeds in order:
//  1. Address any unaddressed review comments
//  2. Wait for review approval (if required)
//  3. Wait for CI to pass
//  4. Merge
type AutoMergePollResultMsg struct {
	SessionID      string
	ReviewDecision git.ReviewDecision
	CommentCount   int
	CIStatus       git.CIStatus
	Attempt        int
}

// pollForAutoMerge starts polling for auto-merge readiness.
// Returns nil if polling is already active for this session (prevents concurrent chains).
func (m *Model) pollForAutoMerge(sessionID string) tea.Cmd {
	log := logger.WithSession(sessionID)
	state := m.sessionState().GetOrCreate(sessionID)
	if state.GetAutoMergePolling() {
		log.Debug("auto-merge polling already active, skipping duplicate chain")
		return nil
	}
	state.SetAutoMergePolling(true)
	return m.pollForAutoMergeAttempt(sessionID, 1)
}

// clearAutoMergePolling clears the auto-merge polling flag for a session.
func (m *Model) clearAutoMergePolling(sessionID string) {
	if state := m.sessionState().GetIfExists(sessionID); state != nil {
		state.SetAutoMergePolling(false)
	}
}

// pollForAutoMergeAttempt polls review state and CI status for auto-merge.
func (m *Model) pollForAutoMergeAttempt(sessionID string, attempt int) tea.Cmd {
	sess := m.config.GetSession(sessionID)
	if sess == nil {
		return nil
	}

	repoPath := sess.RepoPath
	branch := sess.Branch
	gitSvc := m.gitService

	return func() tea.Msg {
		// Wait before check to give CI/reviews time
		time.Sleep(30 * time.Second)

		log := logger.WithSession(sessionID)

		// Fetch review decision
		reviewCtx, reviewCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer reviewCancel()
		reviewDecision, err := gitSvc.CheckPRReviewDecision(reviewCtx, repoPath, branch)
		if err != nil {
			log.Warn("failed to check PR review decision", "error", err)
		}

		// Fetch comment count
		commentCount := 0
		commentCtx, commentCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer commentCancel()
		results, err := gitSvc.GetBatchPRStatesWithComments(commentCtx, repoPath, []string{branch})
		if err != nil {
			log.Warn("failed to check PR comment count", "error", err)
		} else if result, ok := results[branch]; ok {
			commentCount = result.CommentCount
		}

		// Fetch CI status
		ciCtx, ciCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer ciCancel()
		ciStatus, err := gitSvc.CheckPRChecks(ciCtx, repoPath, branch)
		if err != nil {
			log.Warn("failed to check CI status", "error", err)
			ciStatus = git.CIStatusPending
		}

		return AutoMergePollResultMsg{
			SessionID:      sessionID,
			ReviewDecision: reviewDecision,
			CommentCount:   commentCount,
			CIStatus:       ciStatus,
			Attempt:        attempt,
		}
	}
}

// handleAutoMergePollResultMsg implements the auto-merge state machine.
// Priority order: address comments → wait for approval → wait for CI → merge.
func (m *Model) handleAutoMergePollResultMsg(msg AutoMergePollResultMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	sess := m.config.GetSession(msg.SessionID)
	if sess == nil {
		return m, nil
	}

	isActiveSession := m.activeSession != nil && m.activeSession.ID == msg.SessionID

	// Step 1: Check for unaddressed review comments (highest priority)
	if msg.CommentCount > sess.PRCommentsAddressedCount {
		log.Info("unaddressed review comments detected, addressing before merge",
			"branch", sess.Branch,
			"addressed", sess.PRCommentsAddressedCount,
			"current", msg.CommentCount,
		)
		autoMsg := "[AUTO] Review comments detected, addressing before merge...\n"
		if isActiveSession {
			m.chat.AppendStreaming("\n" + autoMsg)
		} else {
			m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent("\n" + autoMsg)
		}
		// Mark these comments as addressed (we're about to send them to Claude).
		// Polling stops here and restarts via handleSessionPipelineCompleteMsg
		// after Claude finishes, so this count won't be checked again until then.
		m.config.UpdateSessionPRCommentsAddressedCount(msg.SessionID, msg.CommentCount)
		m.sidebar.SetHasNewComments(msg.SessionID, true)
		// Clear polling flag — it will be re-set when polling restarts after Claude finishes
		m.clearAutoMergePolling(msg.SessionID)
		return m, m.autoFetchAndSendPRComments(msg.SessionID)
	}

	// Step 2: Check review approval
	switch msg.ReviewDecision {
	case git.ReviewChangesRequested:
		log.Info("changes requested, waiting for re-review", "branch", sess.Branch)
		autoMsg := "[AUTO] Changes requested by reviewer, waiting for re-review...\n"
		if isActiveSession {
			m.chat.AppendStreaming("\n" + autoMsg)
		} else {
			m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent("\n" + autoMsg)
		}
		return m, m.pollForAutoMergeAttempt(msg.SessionID, msg.Attempt+1)

	case git.ReviewNone:
		log.Debug("waiting for review", "branch", sess.Branch, "attempt", msg.Attempt, "decision", msg.ReviewDecision)
		// Only show message on first attempt to avoid spam
		if msg.Attempt == 1 {
			autoMsg := "[AUTO] Waiting for review...\n"
			if isActiveSession {
				m.chat.AppendStreaming("\n" + autoMsg)
			} else {
				m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent("\n" + autoMsg)
			}
		}
		if msg.Attempt >= maxAutoMergePollAttempts {
			log.Warn("auto-merge polling timed out waiting for review", "branch", sess.Branch)
			failMsg := fmt.Sprintf("[AUTO] Still waiting for review after %d attempts - giving up on auto-merge\n", msg.Attempt)
			if isActiveSession {
				m.chat.AppendStreaming("\n" + failMsg)
			} else {
				m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent("\n" + failMsg)
			}
			m.clearAutoMergePolling(msg.SessionID)
			return m, m.ShowFlashWarning(fmt.Sprintf("Auto-merge timed out (waiting for review): %s", sess.Branch))
		}
		return m, m.pollForAutoMergeAttempt(msg.SessionID, msg.Attempt+1)
	}

	// Step 3: Review is approved. Check CI.
	switch msg.CIStatus {
	case git.CIStatusPassing:
		log.Info("review approved and CI passed, merging PR", "branch", sess.Branch)
		autoMsg := "[AUTO] Review approved, CI checks passed. Merging PR...\n"
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
		m.clearAutoMergePolling(msg.SessionID)
		if m.config.GetNotificationsEnabled() {
			sessionName := ui.SessionDisplayName(sess.Branch, sess.Name)
			go notification.SessionCompleted(sessionName + " (CI failed)")
		}
		return m, nil

	case git.CIStatusPending:
		if msg.Attempt >= maxAutoMergePollAttempts {
			log.Warn("auto-merge polling timed out waiting for CI", "branch", sess.Branch)
			failMsg := fmt.Sprintf("[AUTO] CI checks still pending after %d attempts - giving up on auto-merge\n", msg.Attempt)
			if isActiveSession {
				m.chat.AppendStreaming("\n" + failMsg)
			} else {
				m.sessionState().GetOrCreate(msg.SessionID).AppendStreamingContent("\n" + failMsg)
			}
			m.clearAutoMergePolling(msg.SessionID)
			return m, m.ShowFlashWarning(fmt.Sprintf("Auto-merge timed out (CI pending): %s", sess.Branch))
		}
		log.Debug("CI checks still pending, will poll again", "branch", sess.Branch, "attempt", msg.Attempt)
		return m, m.pollForAutoMergeAttempt(msg.SessionID, msg.Attempt+1)

	case git.CIStatusNone:
		log.Info("review approved, no CI checks configured, merging PR", "branch", sess.Branch)
		autoMsg := "[AUTO] Review approved, no CI checks configured. Merging PR...\n"
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

		// Don't delete branch - it will be deleted during session cleanup
		err := gitSvc.MergePR(ctx, repoPath, branch, false)
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

	// Clear polling flag regardless of outcome
	m.clearAutoMergePolling(msg.SessionID)

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
	cmds = append(cmds, m.sidebar.SidebarTick(), m.chat.SpinnerTick())

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

	// Get the stored request ID for response correlation
	state := m.sessionState().GetIfExists(msg.SessionID)
	var requestID interface{}
	if state != nil {
		requestID = state.GetPendingMergeChildRequestID()
		state.SetPendingMergeChildRequestID(nil)
	}
	if requestID == nil {
		log.Warn("merge child complete but no stored request ID for response correlation")
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

// handleCreatePRRequestMsg handles a create_pr MCP tool call from an automated supervisor.
func (m *Model) handleCreatePRRequestMsg(msg CreatePRRequestMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	runner := m.sessionMgr.GetRunner(msg.SessionID)
	if runner == nil {
		log.Warn("create PR request for unknown session")
		return m, nil
	}

	sess := m.config.GetSession(msg.SessionID)
	if sess == nil {
		runner.SendCreatePRResponse(mcp.CreatePRResponse{
			ID:    msg.Request.ID,
			Error: "Session not found",
		})
		return m, tea.Batch(m.sessionListeners(msg.SessionID, runner, nil)...)
	}

	log.Info("create_pr called via MCP tool", "branch", sess.Branch, "title", msg.Request.Title)

	// Capture values for the goroutine
	sessionID := msg.SessionID
	requestID := msg.Request.ID
	repoPath := sess.RepoPath
	workTree := sess.WorkTree
	branch := sess.Branch
	baseBranch := sess.BaseBranch
	title := msg.Request.Title
	issueRef := sess.GetIssueRef()
	gitSvc := m.gitService

	// Re-register listeners first
	cmds := m.sessionListeners(msg.SessionID, runner, nil)

	// Run PR creation asynchronously
	prCmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		resultCh := gitSvc.CreatePR(ctx, repoPath, workTree, branch, baseBranch, title, issueRef)

		var lastErr error
		var prURL string
		for result := range resultCh {
			if result.Error != nil {
				lastErr = result.Error
			}
			// gh pr create outputs the PR URL to stdout
			if trimmed := strings.TrimSpace(result.Output); strings.HasPrefix(trimmed, "https://") {
				prURL = trimmed
			}
		}

		if lastErr != nil {
			runner.SendCreatePRResponse(mcp.CreatePRResponse{
				ID:    requestID,
				Error: fmt.Sprintf("Failed to create PR: %v", lastErr),
			})
			return nil
		}

		runner.SendCreatePRResponse(mcp.CreatePRResponse{
			ID:      requestID,
			Success: true,
			PRURL:   prURL,
		})

		return PRCreatedFromToolMsg{SessionID: sessionID, PRURL: prURL}
	}

	cmds = append(cmds, prCmd)
	return m, tea.Batch(cmds...)
}

// PRCreatedFromToolMsg is sent when a PR is created via the create_pr MCP tool.
type PRCreatedFromToolMsg struct {
	SessionID string
	PRURL     string
}

// handlePRCreatedFromToolMsg updates session state after a PR is created via the create_pr tool.
func (m *Model) handlePRCreatedFromToolMsg(msg PRCreatedFromToolMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	m.config.MarkSessionPRCreated(msg.SessionID)
	if err := m.config.Save(); err != nil {
		log.Error("failed to save config after PR creation", "error", err)
	}
	m.sidebar.SetSessions(m.getFilteredSessions())
	log.Info("marked session as PR created via tool", "prURL", msg.PRURL)

	// Start CI polling for auto-merge if enabled
	sess := m.config.GetSession(msg.SessionID)
	if sess != nil && sess.Autonomous && m.config.GetRepoAutoMerge(sess.RepoPath) {
		log.Info("starting CI polling for auto-merge", "branch", sess.Branch)
		return m, m.pollForAutoMerge(msg.SessionID)
	}

	return m, nil
}

// handlePushBranchRequestMsg handles a push_branch MCP tool call from an automated supervisor.
func (m *Model) handlePushBranchRequestMsg(msg PushBranchRequestMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	runner := m.sessionMgr.GetRunner(msg.SessionID)
	if runner == nil {
		log.Warn("push branch request for unknown session")
		return m, nil
	}

	sess := m.config.GetSession(msg.SessionID)
	if sess == nil {
		runner.SendPushBranchResponse(mcp.PushBranchResponse{
			ID:    msg.Request.ID,
			Error: "Session not found",
		})
		return m, tea.Batch(m.sessionListeners(msg.SessionID, runner, nil)...)
	}

	log.Info("push_branch called via MCP tool", "branch", sess.Branch, "commitMessage", msg.Request.CommitMessage)

	// Capture values for the goroutine
	requestID := msg.Request.ID
	repoPath := sess.RepoPath
	workTree := sess.WorkTree
	branch := sess.Branch
	commitMessage := msg.Request.CommitMessage
	gitSvc := m.gitService

	// Re-register listeners first
	cmds := m.sessionListeners(msg.SessionID, runner, nil)

	// Run push asynchronously
	pushCmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		resultCh := gitSvc.PushUpdates(ctx, repoPath, workTree, branch, commitMessage)

		var lastErr error
		for result := range resultCh {
			if result.Error != nil {
				lastErr = result.Error
			}
		}

		if lastErr != nil {
			runner.SendPushBranchResponse(mcp.PushBranchResponse{
				ID:    requestID,
				Error: fmt.Sprintf("Failed to push branch: %v", lastErr),
			})
			return nil
		}

		runner.SendPushBranchResponse(mcp.PushBranchResponse{
			ID:      requestID,
			Success: true,
		})
		return nil
	}

	cmds = append(cmds, pushCmd)
	return m, tea.Batch(cmds...)
}

// handleGetReviewCommentsRequestMsg handles a get_review_comments MCP tool call from an automated supervisor.
func (m *Model) handleGetReviewCommentsRequestMsg(msg GetReviewCommentsRequestMsg) (tea.Model, tea.Cmd) {
	log := logger.WithSession(msg.SessionID)
	runner := m.sessionMgr.GetRunner(msg.SessionID)
	if runner == nil {
		log.Warn("get review comments request for unknown session")
		return m, nil
	}

	sess := m.config.GetSession(msg.SessionID)
	if sess == nil {
		runner.SendGetReviewCommentsResponse(mcp.GetReviewCommentsResponse{
			ID:    msg.Request.ID,
			Error: "Session not found",
		})
		return m, tea.Batch(m.sessionListeners(msg.SessionID, runner, nil)...)
	}

	log.Info("get_review_comments called via MCP tool", "branch", sess.Branch)

	// Capture values for the goroutine
	requestID := msg.Request.ID
	repoPath := sess.RepoPath
	branch := sess.Branch
	gitSvc := m.gitService

	// Re-register listeners first
	cmds := m.sessionListeners(msg.SessionID, runner, nil)

	// Fetch review comments asynchronously
	fetchCmd := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		comments, err := gitSvc.FetchPRReviewComments(ctx, repoPath, branch)
		if err != nil {
			runner.SendGetReviewCommentsResponse(mcp.GetReviewCommentsResponse{
				ID:      requestID,
				Success: false,
				Error:   fmt.Sprintf("Failed to fetch review comments: %v", err),
			})
			return nil
		}

		// Convert git.PRReviewComment to mcp.ReviewComment
		mcpComments := make([]mcp.ReviewComment, len(comments))
		for i, c := range comments {
			mcpComments[i] = mcp.ReviewComment{
				Author: c.Author,
				Body:   c.Body,
				Path:   c.Path,
				Line:   c.Line,
				URL:    c.URL,
			}
		}

		runner.SendGetReviewCommentsResponse(mcp.GetReviewCommentsResponse{
			ID:       requestID,
			Success:  true,
			Comments: mcpComments,
		})
		return nil
	}

	cmds = append(cmds, fetchCmd)
	return m, tea.Batch(cmds...)
}
