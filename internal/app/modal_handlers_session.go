package app

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/google/uuid"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/session"
	"github.com/zhubert/plural/internal/ui"
)

// handleAddRepoModal handles key events for the Add Repository modal.
func (m *Model) handleAddRepoModal(key string, msg tea.KeyPressMsg, state *ui.AddRepoState) (tea.Model, tea.Cmd) {
	// If showing completion options, forward Enter to the modal to select the option
	if state.IsShowingOptions() && key == "enter" {
		modal, cmd := m.modal.Update(msg)
		m.modal = modal
		return m, cmd
	}

	switch key {
	case "esc":
		m.modal.Hide()
		return m, nil
	case "enter":
		path := state.GetPath()
		if path == "" {
			m.modal.SetError("Please enter a path")
			return m, nil
		}

		ctx := context.Background()

		// Check if this is a glob pattern
		if ui.IsGlobPattern(path) {
			return m.handleAddReposFromGlob(ctx, path)
		}

		// Single path - validate and add
		if err := m.sessionService.ValidateRepo(ctx, path); err != nil {
			m.modal.SetError(err.Error())
			return m, nil
		}
		if !m.config.AddRepo(path) {
			m.modal.SetError("Repository already added")
			return m, nil
		}
		if err := m.config.Save(); err != nil {
			m.modal.SetError("Failed to save: " + err.Error())
			return m, nil
		}
		m.modal.Hide()
		return m, nil
	}
	// Forward other keys to the modal for text input handling
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// handleAddReposFromGlob expands a glob pattern and adds all matching git repositories.
func (m *Model) handleAddReposFromGlob(ctx context.Context, pattern string) (tea.Model, tea.Cmd) {
	// Expand the glob to directories
	dirs, err := ui.ExpandGlobToDirs(pattern)
	if err != nil {
		m.modal.SetError("Invalid glob pattern: " + err.Error())
		return m, nil
	}

	if len(dirs) == 0 {
		m.modal.SetError("No directories match the pattern")
		return m, nil
	}

	// Filter to valid git repos and add them
	var added, skipped, alreadyAdded int
	for _, dir := range dirs {
		if err := m.sessionService.ValidateRepo(ctx, dir); err != nil {
			skipped++
			continue
		}
		if !m.config.AddRepo(dir) {
			alreadyAdded++
			continue
		}
		added++
	}

	// Save if any were added
	if added > 0 {
		if err := m.config.Save(); err != nil {
			m.modal.SetError("Failed to save: " + err.Error())
			return m, nil
		}
	}

	m.modal.Hide()

	// Build status message
	if added == 0 {
		if alreadyAdded > 0 {
			return m, m.ShowFlashWarning(fmt.Sprintf("All %d repos already added", alreadyAdded))
		}
		return m, m.ShowFlashWarning("No git repositories found matching pattern")
	}

	msg := fmt.Sprintf("Added %d repo(s)", added)
	if skipped > 0 || alreadyAdded > 0 {
		msg += fmt.Sprintf(" (skipped: %d non-git, %d already added)", skipped, alreadyAdded)
	}
	return m, m.ShowFlashSuccess(msg)
}

// handleNewSessionModal handles key events for the New Session modal.
func (m *Model) handleNewSessionModal(key string, msg tea.KeyPressMsg, state *ui.NewSessionState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.modal.Hide()
		return m, nil
	case "d":
		// Only allow delete when focus is on repo list and there are repos
		if state.Focus == 0 && len(state.RepoOptions) > 0 {
			repoPath := state.GetSelectedRepo()
			if repoPath != "" {
				m.modal.Show(ui.NewConfirmDeleteRepoState(repoPath))
				return m, nil
			}
		}
		// If focused on branch input, let it pass through for text input
		if state.Focus == 2 {
			modal, cmd := m.modal.Update(msg)
			m.modal = modal
			return m, cmd
		}
		return m, nil
	case "enter":
		repoPath := state.GetSelectedRepo()
		if repoPath == "" {
			return m, nil
		}
		branchName := state.GetBranchName()
		// Validate branch name
		if err := session.ValidateBranchName(branchName); err != nil {
			m.modal.SetError(err.Error())
			return m, nil
		}
		// Get branch prefix and build full branch name for existence check
		branchPrefix := m.config.GetDefaultBranchPrefix()
		fullBranchName := branchPrefix + branchName
		if branchName == "" {
			fullBranchName = "" // Will be auto-generated
		}
		// Check if branch already exists
		ctx := context.Background()
		if fullBranchName != "" && m.sessionService.BranchExists(ctx, repoPath, fullBranchName) {
			m.modal.SetError("Branch already exists: " + fullBranchName)
			return m, nil
		}
		basePoint := session.BasePointOrigin
		if state.GetBaseIndex() == 1 {
			basePoint = session.BasePointHead
		}
		logger.Get().Debug("creating new session", "repo", repoPath, "branch", branchName, "prefix", branchPrefix, "basePoint", basePoint)
		sess, err := m.sessionService.Create(ctx, repoPath, branchName, branchPrefix, basePoint)
		if err != nil {
			logger.Get().Error("failed to create session", "error", err)
			m.modal.SetError(err.Error())
			return m, nil
		}
		logger.WithSession(sess.ID).Info("session created", "name", sess.Name)
		m.config.AddSession(*sess)
		if err := m.config.Save(); err != nil {
			logger.Get().Error("failed to save config", "error", err)
			m.modal.SetError("Failed to save: " + err.Error())
			return m, nil
		}
		m.sidebar.SetSessions(m.config.GetSessions())
		m.sidebar.SelectSession(sess.ID)
		m.selectSession(sess)
		m.modal.Hide()
		return m, nil
	}
	// Forward other keys (tab, shift+tab, up, down, etc.) to modal for handling
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// handleConfirmDeleteModal handles key events for the Confirm Delete modal.
func (m *Model) handleConfirmDeleteModal(key string, msg tea.KeyPressMsg, state *ui.ConfirmDeleteState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.modal.Hide()
		return m, nil
	case "enter":
		if sess := m.sidebar.SelectedSession(); sess != nil {
			log := logger.WithSession(sess.ID)
			deleteWorktree := state.ShouldDeleteWorktree()
			log.Debug("deleting session", "name", sess.Name, "deleteWorktree", deleteWorktree)

			// Delete worktree if requested
			if deleteWorktree {
				ctx := context.Background()
				if err := m.sessionService.Delete(ctx, sess); err != nil {
					log.Warn("failed to delete worktree", "error", err)
					// Continue with session removal even if worktree deletion fails
				}
			}

			m.config.RemoveSession(sess.ID)
			m.config.Save()
			config.DeleteSessionMessages(sess.ID)
			m.sidebar.SetSessions(m.config.GetSessions())
			// Clean up runner and all per-session state via SessionManager
			deletedRunner := m.sessionMgr.DeleteSession(sess.ID)
			m.sidebar.SetPendingPermission(sess.ID, false)
			activeSessionID := "<nil>"
			if m.activeSession != nil {
				activeSessionID = m.activeSession.ID
			}
			log.Debug("checking if active session should be cleared", "activeSessionExists", m.activeSession != nil, "activeSessionID", activeSessionID)
			if m.activeSession != nil && m.activeSession.ID == sess.ID {
				log.Debug("clearing active session and chat")
				m.activeSession = nil
				m.claudeRunner = nil
				m.chat.ClearSession()
				m.header.SetSessionName("")
				m.header.SetBaseBranch("")
				m.header.SetDiffStats(nil)
			} else {
				log.Debug("not clearing chat - deleted session was not the active session")
			}
			if deletedRunner != nil {
				log.Info("session deleted successfully (runner stopped)")
			} else {
				log.Info("session deleted successfully")
			}
		}
		m.modal.Hide()
		return m, nil
	case "up", "down", "j", "k":
		// Forward navigation keys to modal for option selection
		modal, cmd := m.modal.Update(msg)
		m.modal = modal
		return m, cmd
	}
	return m, nil
}

// handleForkSessionModal handles key events for the Fork Session modal.
func (m *Model) handleForkSessionModal(key string, msg tea.KeyPressMsg, state *ui.ForkSessionState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.modal.Hide()
		return m, nil
	case "enter":
		branchName := state.GetBranchName()
		// Validate branch name
		if err := session.ValidateBranchName(branchName); err != nil {
			m.modal.SetError(err.Error())
			return m, nil
		}
		// Get branch prefix and build full branch name for existence check
		branchPrefix := m.config.GetDefaultBranchPrefix()
		fullBranchName := branchPrefix + branchName
		if branchName == "" {
			fullBranchName = "" // Will be auto-generated
		}
		// Check if branch already exists
		ctx := context.Background()
		if fullBranchName != "" && m.sessionService.BranchExists(ctx, state.RepoPath, fullBranchName) {
			m.modal.SetError("Branch already exists: " + fullBranchName)
			return m, nil
		}

		// Get parent session to fork from its branch
		parentSess := m.config.GetSession(state.ParentSessionID)
		if parentSess == nil {
			m.modal.SetError("Parent session not found")
			return m, nil
		}

		logger.WithSession(state.ParentSessionID).Debug("forking session", "parentBranch", parentSess.Branch, "copyMessages", state.CopyMessages, "newBranch", branchName, "prefix", branchPrefix)

		// Create new session forked from parent's branch
		sess, err := m.sessionService.CreateFromBranch(ctx, state.RepoPath, parentSess.Branch, branchName, branchPrefix)
		if err != nil {
			logger.Get().Error("failed to create forked session", "error", err)
			m.modal.SetError(err.Error())
			return m, nil
		}

		log := logger.WithSession(sess.ID)

		// Copy messages if requested
		var messageCopyFailed bool
		if state.CopyMessages {
			parentMsgs, err := config.LoadSessionMessages(state.ParentSessionID)
			if err != nil {
				log.Warn("failed to load parent session messages", "error", err)
				messageCopyFailed = true
			} else if len(parentMsgs) > 0 {
				if err := config.SaveSessionMessages(sess.ID, parentMsgs, config.MaxSessionMessageLines); err != nil {
					log.Warn("failed to save forked session messages", "error", err)
					messageCopyFailed = true
				} else {
					log.Debug("copied messages from parent session", "count", len(parentMsgs))
				}
			}
		}

		// Set parent ID to track fork relationship
		sess.ParentID = state.ParentSessionID

		log.Info("forked session created", "name", sess.Name, "parentID", sess.ParentID)
		m.config.AddSession(*sess)
		if err := m.config.Save(); err != nil {
			log.Error("failed to save config", "error", err)
			m.modal.SetError("Failed to save: " + err.Error())
			return m, nil
		}
		m.sidebar.SetSessions(m.config.GetSessions())
		m.sidebar.SelectSession(sess.ID)
		m.selectSession(sess)
		m.modal.Hide()

		// Show warning if message copy failed (after modal is hidden)
		if messageCopyFailed {
			return m, m.ShowFlashWarning("Session created but conversation history could not be copied")
		}
		return m, nil
	}
	// Forward other keys (tab, shift+tab, space, up, down, etc.) to modal for handling
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// handleRenameSessionModal handles key events for the Rename Session modal.
func (m *Model) handleRenameSessionModal(key string, msg tea.KeyPressMsg, state *ui.RenameSessionState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.modal.Hide()
		return m, nil
	case "enter":
		newName := state.GetNewName()
		if newName == "" {
			m.modal.SetError("Name cannot be empty")
			return m, nil
		}

		// Get the session to access worktree path and old branch name
		sess := m.config.GetSession(state.SessionID)
		if sess == nil {
			m.modal.SetError("Session not found")
			return m, nil
		}

		oldBranch := sess.Branch

		// Apply branch prefix if configured
		branchPrefix := m.config.GetDefaultBranchPrefix()
		newBranch := branchPrefix + newName

		// Validate the new branch name
		if err := session.ValidateBranchName(newName); err != nil {
			m.modal.SetError(err.Error())
			return m, nil
		}

		// Check if new branch already exists (unless it's the same name)
		ctx := context.Background()
		if newBranch != oldBranch && m.sessionService.BranchExists(ctx, sess.RepoPath, newBranch) {
			m.modal.SetError("Branch already exists: " + newBranch)
			return m, nil
		}

		// Rename the git branch
		if newBranch != oldBranch {
			if err := m.gitService.RenameBranch(ctx, sess.WorkTree, oldBranch, newBranch); err != nil {
				m.modal.SetError("Failed to rename branch: " + err.Error())
				return m, nil
			}
		}

		// Update the session name and branch in config
		// Name stores the full branch name (same as branch) for display
		if !m.config.RenameSession(state.SessionID, newBranch, newBranch) {
			m.modal.SetError("Failed to rename session")
			return m, nil
		}
		if err := m.config.Save(); err != nil {
			m.modal.SetError("Failed to save: " + err.Error())
			return m, nil
		}
		logger.WithSession(state.SessionID).Info("renamed session", "branch", newBranch)

		// Update sidebar and header
		m.sidebar.SetSessions(m.config.GetSessions())
		if m.activeSession != nil && m.activeSession.ID == state.SessionID {
			m.activeSession.Name = newBranch
			m.activeSession.Branch = newBranch
			m.header.SetSessionName(newBranch)
		}
		m.modal.Hide()
		return m, nil
	}
	// Forward other keys to the modal for text input handling
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// handleConfirmDeleteRepoModal handles key events for the Confirm Delete Repo modal.
func (m *Model) handleConfirmDeleteRepoModal(key string, msg tea.KeyPressMsg, state *ui.ConfirmDeleteRepoState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		// Go back to the new session modal
		m.modal.Show(ui.NewNewSessionState(m.config.GetRepos()))
		return m, nil
	case "enter":
		repoPath := state.GetRepoPath()
		logger.Get().Debug("deleting repository", "path", repoPath)

		if !m.config.RemoveRepo(repoPath) {
			m.modal.SetError("Repository not found")
			return m, nil
		}
		if err := m.config.Save(); err != nil {
			m.modal.SetError("Failed to save: " + err.Error())
			return m, nil
		}
		logger.Get().Info("repository deleted successfully", "path", repoPath)

		// Return to new session modal with updated repo list
		m.modal.Show(ui.NewNewSessionState(m.config.GetRepos()))
		return m, nil
	}
	return m, nil
}

// handleConfirmExitModal handles key events for the Confirm Exit modal.
func (m *Model) handleConfirmExitModal(key string, msg tea.KeyPressMsg, state *ui.ConfirmExitState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.modal.Hide()
		return m, nil
	case "enter":
		if state.ShouldExit() {
			logger.Get().Info("user confirmed exit with active sessions")
			return m, tea.Quit
		}
		// Cancel selected
		m.modal.Hide()
		return m, nil
	case "up", "down", "j", "k":
		// Forward navigation keys to modal for option selection
		modal, cmd := m.modal.Update(msg)
		m.modal = modal
		return m, cmd
	}
	return m, nil
}

// handlePreviewActiveModal handles key events for the Preview Active warning modal.
func (m *Model) handlePreviewActiveModal(key string, msg tea.KeyPressMsg, state *ui.PreviewActiveState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "enter":
		m.modal.Hide()
		return m, nil
	case "p":
		// End preview and close modal
		m.modal.Hide()
		return m.endPreview()
	}
	return m, nil
}

// handleBroadcastModal handles key events for the Broadcast modal.
func (m *Model) handleBroadcastModal(key string, msg tea.KeyPressMsg, state *ui.BroadcastState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "escape":
		m.modal.Hide()
		return m, nil
	case "enter":
		selectedRepos := state.GetSelectedRepos()
		if len(selectedRepos) == 0 {
			m.modal.SetError("Select at least one repository")
			return m, nil
		}

		prompt := state.GetPrompt()
		if strings.TrimSpace(prompt) == "" {
			m.modal.SetError("Enter a prompt")
			return m, nil
		}

		// Get the optional session name
		sessionName := strings.TrimSpace(state.GetName())

		// Validate session name if provided
		if sessionName != "" {
			if err := session.ValidateBranchName(sessionName); err != nil {
				m.modal.SetError(err.Error())
				return m, nil
			}
		}

		m.modal.Hide()
		return m.createBroadcastSessions(selectedRepos, prompt, sessionName)
	}

	// Forward other keys to modal for navigation/selection
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// createBroadcastSessions creates sessions for each selected repo and sends the prompt to each.
// If sessionName is provided (non-empty), it will be used as the branch name for all sessions.
func (m *Model) createBroadcastSessions(repoPaths []string, prompt string, sessionName string) (tea.Model, tea.Cmd) {
	log := logger.Get()
	log.Info("creating broadcast sessions", "repoCount", len(repoPaths), "sessionName", sessionName)

	// Generate a broadcast group ID for this batch
	groupID := uuid.New().String()
	branchPrefix := m.config.GetDefaultBranchPrefix()

	var createdSessions []*config.Session
	var failedRepos []string

	ctx := context.Background()

	// Create a session for each repo
	for _, repoPath := range repoPaths {
		sess, err := m.sessionService.Create(ctx, repoPath, sessionName, branchPrefix, session.BasePointOrigin)
		if err != nil {
			log.Error("failed to create session for broadcast", "repo", repoPath, "error", err)
			failedRepos = append(failedRepos, repoPath)
			continue
		}

		// Set the broadcast group ID
		sess.BroadcastGroupID = groupID

		// Add session to config
		m.config.AddSession(*sess)
		createdSessions = append(createdSessions, sess)

		logger.WithSession(sess.ID).Info("created broadcast session", "repo", repoPath, "groupID", groupID)
	}

	// Save config after creating all sessions
	if err := m.config.Save(); err != nil {
		log.Error("failed to save config after broadcast session creation", "error", err)
	}

	// Update sidebar with new sessions
	m.sidebar.SetSessions(m.config.GetSessions())

	// If no sessions were created, show error
	if len(createdSessions) == 0 {
		return m, m.ShowFlashError("Failed to create any sessions")
	}

	// Select the first session
	firstSession := createdSessions[0]
	m.sidebar.SelectSession(firstSession.ID)
	m.selectSession(firstSession)

	// Build content blocks for the prompt
	content := []claude.ContentBlock{{
		Type: claude.ContentTypeText,
		Text: prompt,
	}}

	// Collect all commands for parallel execution
	var cmds []tea.Cmd

	// Send prompt to each created session
	for _, sess := range createdSessions {
		// Get or create the runner for this session
		result := m.sessionMgr.Select(sess, "", "", "")
		if result == nil || result.Runner == nil {
			log.Error("failed to get runner for broadcast session", "sessionID", sess.ID)
			continue
		}

		runner := result.Runner
		sessionID := sess.ID

		// Create context for this request
		reqCtx, cancel := context.WithCancel(context.Background())
		m.sessionState().StartWaiting(sessionID, cancel)
		m.sidebar.SetStreaming(sessionID, true)

		// Send the content
		responseChan := runner.SendContent(reqCtx, content)

		// Add listeners for this session
		cmds = append(cmds, m.sessionListeners(sessionID, runner, responseChan)...)
	}

	// Set the app state to streaming
	m.setState(StateStreamingClaude)

	// Add UI update ticks
	cmds = append(cmds, ui.SidebarTick(), ui.StopwatchTick())

	// Show status message
	msg := fmt.Sprintf("Broadcasting to %d repo(s)", len(createdSessions))
	if len(failedRepos) > 0 {
		msg += fmt.Sprintf(" (failed: %d)", len(failedRepos))
	}

	cmds = append(cmds, m.ShowFlashSuccess(msg))

	return m, tea.Batch(cmds...)
}

// broadcastToSessions sends a prompt to all sessions in a group.
func (m *Model) broadcastToSessions(sessions []config.Session, prompt string) (tea.Model, tea.Cmd) {
	log := logger.Get()
	log.Info("broadcasting to existing sessions", "count", len(sessions))

	// Build content blocks for the prompt
	content := []claude.ContentBlock{{
		Type: claude.ContentTypeText,
		Text: prompt,
	}}

	// Collect all commands for parallel execution
	var cmds []tea.Cmd
	sentCount := 0

	for _, sess := range sessions {
		// Get or create the runner for this session
		result := m.sessionMgr.Select(&sess, "", "", "")
		if result == nil || result.Runner == nil {
			log.Error("failed to get runner for broadcast session", "sessionID", sess.ID)
			continue
		}

		runner := result.Runner
		sessionID := sess.ID

		// Create context for this request
		reqCtx, cancel := context.WithCancel(context.Background())
		m.sessionState().StartWaiting(sessionID, cancel)
		m.sidebar.SetStreaming(sessionID, true)

		// Send the content
		responseChan := runner.SendContent(reqCtx, content)

		// Add listeners for this session
		cmds = append(cmds, m.sessionListeners(sessionID, runner, responseChan)...)
		sentCount++
	}

	// Set the app state to streaming
	m.setState(StateStreamingClaude)

	// Clear the chat input since we're sending it
	m.chat.ClearInput()

	// Add UI update ticks
	cmds = append(cmds, ui.SidebarTick(), ui.StopwatchTick())

	// Show status message
	cmds = append(cmds, m.ShowFlashSuccess(fmt.Sprintf("Sent to %d session(s)", sentCount)))

	return m, tea.Batch(cmds...)
}

// handleBroadcastGroupModal handles key events for the Broadcast Group modal.
func (m *Model) handleBroadcastGroupModal(key string, msg tea.KeyPressMsg, state *ui.BroadcastGroupState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "escape":
		m.modal.Hide()
		return m, nil
	case "enter":
		selectedIDs := state.GetSelectedSessions()
		if len(selectedIDs) == 0 {
			m.modal.SetError("Select at least one session")
			return m, nil
		}

		action := state.GetAction()
		m.modal.Hide()

		// Get the full session objects for selected sessions
		var selectedSessions []config.Session
		for _, id := range selectedIDs {
			if sess := m.config.GetSession(id); sess != nil {
				selectedSessions = append(selectedSessions, *sess)
			}
		}

		if len(selectedSessions) == 0 {
			return m, m.ShowFlashError("No valid sessions found")
		}

		switch action {
		case ui.BroadcastActionSendPrompt:
			prompt := state.GetPrompt()
			if strings.TrimSpace(prompt) == "" {
				m.modal.Show(state) // Re-show modal
				m.modal.SetError("Enter a prompt")
				return m, nil
			}
			return m.broadcastToSessions(selectedSessions, prompt)

		case ui.BroadcastActionCreatePRs:
			return m.createPRsForSessions(selectedSessions)
		}
	}

	// Forward other keys to modal for navigation/selection
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// createPRsForSessions triggers PR creation for multiple sessions.
func (m *Model) createPRsForSessions(sessions []config.Session) (tea.Model, tea.Cmd) {
	log := logger.Get()
	log.Info("creating PRs for multiple sessions", "count", len(sessions))

	ctx := context.Background()
	var cmds []tea.Cmd
	startedCount := 0
	skippedCount := 0

	for _, sess := range sessions {
		sessionLog := logger.WithSession(sess.ID)

		// Skip if PR already created
		if sess.PRCreated {
			sessionLog.Debug("skipping PR creation - PR already exists")
			skippedCount++
			continue
		}

		// Skip if session is merged
		if sess.Merged {
			sessionLog.Debug("skipping PR creation - session already merged")
			skippedCount++
			continue
		}

		// Check if there's already a merge in progress for this session
		if state := m.sessionState().GetIfExists(sess.ID); state != nil && state.IsMerging() {
			sessionLog.Debug("skipping PR creation - merge already in progress")
			skippedCount++
			continue
		}

		// Check for uncommitted changes
		status, err := m.gitService.GetWorktreeStatus(ctx, sess.WorkTree)
		if err != nil {
			sessionLog.Warn("failed to check worktree status", "error", err)
			skippedCount++
			continue
		}

		if status.HasChanges {
			// Need to commit first - this requires user interaction for commit message
			// For now, skip sessions with uncommitted changes
			sessionLog.Debug("skipping PR creation - has uncommitted changes")
			skippedCount++
			continue
		}

		// Start PR creation
		sessionLog.Info("starting PR creation")
		mergeCtx, cancel := context.WithCancel(context.Background())
		m.sessionState().StartMerge(sess.ID, m.gitService.CreatePR(mergeCtx, sess.RepoPath, sess.WorkTree, sess.Branch, "", sess.IssueNumber), cancel, MergeTypePR)

		// Add listener for merge result
		cmds = append(cmds, m.listenForMergeResult(sess.ID))
		startedCount++
	}

	// Show status message
	var msg string
	if startedCount > 0 {
		msg = fmt.Sprintf("Creating PRs for %d session(s)", startedCount)
		if skippedCount > 0 {
			msg += fmt.Sprintf(" (skipped %d)", skippedCount)
		}
		cmds = append(cmds, m.ShowFlashSuccess(msg))
	} else {
		msg = "No sessions eligible for PR creation"
		if skippedCount > 0 {
			msg += fmt.Sprintf(" (%d skipped - already have PRs, merged, or have uncommitted changes)", skippedCount)
		}
		cmds = append(cmds, m.ShowFlashWarning(msg))
	}

	return m, tea.Batch(cmds...)
}
