package app

import (
	"context"
	"fmt"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/google/uuid"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/keys"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/session"
	"github.com/zhubert/plural/internal/ui"
)

// handleAddRepoModal handles key events for the Add Repository modal.
func (m *Model) handleAddRepoModal(key string, msg tea.KeyPressMsg, state *ui.AddRepoState) (tea.Model, tea.Cmd) {
	// If showing completion options, forward Enter to the modal to select the option
	if state.IsShowingOptions() && key == keys.Enter {
		modal, cmd := m.modal.Update(msg)
		m.modal = modal
		return m, cmd
	}

	switch key {
	case keys.Escape:
		m.modal.Hide()
		return m, nil
	case keys.Enter:
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
// Validation checks are parallelized for better performance with many directories.
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

	// Parallelize validation checks
	type validationResult struct {
		dir   string
		valid bool
	}

	results := make(chan validationResult, len(dirs))
	var wg sync.WaitGroup

	for _, dir := range dirs {
		wg.Add(1)
		go func(dir string) {
			defer wg.Done()
			err := m.sessionService.ValidateRepo(ctx, dir)
			results <- validationResult{dir: dir, valid: err == nil}
		}(dir)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect valid repos
	var validDirs []string
	skipped := 0
	for result := range results {
		if result.valid {
			validDirs = append(validDirs, result.dir)
		} else {
			skipped++
		}
	}

	// Sequentially add valid repos to config
	var added, alreadyAdded int
	for _, dir := range validDirs {
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
	case keys.Escape:
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
	case keys.Enter:
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
		// Set containerized flag if container mode is enabled for this repo
		if m.config.GetUseContainers(repoPath) {
			sess.Containerized = true
		}
		// Auto-assign to active workspace
		if activeWS := m.config.GetActiveWorkspaceID(); activeWS != "" {
			sess.WorkspaceID = activeWS
		}
		m.config.AddSession(*sess)
		if err := m.config.Save(); err != nil {
			logger.Get().Error("failed to save config", "error", err)
			m.modal.SetError("Failed to save: " + err.Error())
			return m, nil
		}
		m.sidebar.SetSessions(m.getFilteredSessions())
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
	case keys.Escape:
		m.modal.Hide()
		return m, nil
	case keys.Enter:
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
			m.config.ClearOrphanedParentIDs([]string{sess.ID})
			m.config.Save()
			config.DeleteSessionMessages(sess.ID)
			m.sidebar.SetSessions(m.getFilteredSessions())
			// Clean up runner and all per-session state via SessionManager
			deletedRunner := m.sessionMgr.DeleteSession(sess.ID)
			m.sidebar.SetPendingPermission(sess.ID, false)
			m.sidebar.SetPendingQuestion(sess.ID, false)
			m.sidebar.SetIdleWithResponse(sess.ID, false)
			m.sidebar.SetUncommittedChanges(sess.ID, false)
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
	case keys.Up, keys.Down, "j", "k":
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
	case keys.Escape:
		m.modal.Hide()
		return m, nil
	case keys.Enter:
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
		// Set containerized flag if container mode is enabled for this repo
		if m.config.GetUseContainers(state.RepoPath) {
			sess.Containerized = true
		}
		// Auto-assign to active workspace
		if activeWS := m.config.GetActiveWorkspaceID(); activeWS != "" {
			sess.WorkspaceID = activeWS
		}

		log.Info("forked session created", "name", sess.Name, "parentID", sess.ParentID)
		m.config.AddSession(*sess)
		if err := m.config.Save(); err != nil {
			log.Error("failed to save config", "error", err)
			m.modal.SetError("Failed to save: " + err.Error())
			return m, nil
		}
		m.sidebar.SetSessions(m.getFilteredSessions())
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
	case keys.Escape:
		m.modal.Hide()
		return m, nil
	case keys.Enter:
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
		m.sidebar.SetSessions(m.getFilteredSessions())
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
	case keys.Escape:
		// Go back to the new session modal
		m.modal.Show(ui.NewNewSessionState(m.config.GetRepos()))
		return m, nil
	case keys.Enter:
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
	case keys.Escape:
		m.modal.Hide()
		return m, nil
	case keys.Enter:
		if state.ShouldExit() {
			logger.Get().Info("user confirmed exit with active sessions")
			return m, tea.Quit
		}
		// Cancel selected
		m.modal.Hide()
		return m, nil
	case keys.Up, keys.Down, "j", "k":
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
	case keys.Escape, keys.Enter:
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
	case keys.Escape:
		m.modal.Hide()
		return m, nil
	case keys.Enter:
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
// Sessions are created in parallel for better performance with many repos.
func (m *Model) createBroadcastSessions(repoPaths []string, prompt string, sessionName string) (tea.Model, tea.Cmd) {
	log := logger.Get()
	log.Info("creating broadcast sessions", "repoCount", len(repoPaths), "sessionName", sessionName)

	// Generate a broadcast group ID for this batch
	groupID := uuid.New().String()
	branchPrefix := m.config.GetDefaultBranchPrefix()

	// Use a semaphore to limit concurrent session creation (avoid overwhelming git/network)
	const maxConcurrent = 10
	sem := make(chan struct{}, maxConcurrent)

	// Thread-safe collection of results
	var mu sync.Mutex
	var createdSessions []*config.Session
	var failedRepos []string

	// Create sessions in parallel
	var wg sync.WaitGroup
	for _, repoPath := range repoPaths {
		wg.Add(1)
		go func(repoPath string) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			ctx := context.Background()
			sess, err := m.sessionService.Create(ctx, repoPath, sessionName, branchPrefix, session.BasePointOrigin)
			if err != nil {
				log.Error("failed to create session for broadcast", "repo", repoPath, "error", err)
				mu.Lock()
				failedRepos = append(failedRepos, repoPath)
				mu.Unlock()
				return
			}

			// Set the broadcast group ID
			sess.BroadcastGroupID = groupID

			// Set containerized flag if container mode is enabled for this repo
			if m.config.GetUseContainers(repoPath) {
				sess.Containerized = true
			}

			mu.Lock()
			createdSessions = append(createdSessions, sess)
			mu.Unlock()

			logger.WithSession(sess.ID).Info("created broadcast session", "repo", repoPath, "groupID", groupID)
		}(repoPath)
	}

	// Wait for all session creations to complete
	wg.Wait()

	// Add all sessions to config (after parallel creation completes)
	activeWS := m.config.GetActiveWorkspaceID()
	for _, sess := range createdSessions {
		if activeWS != "" {
			sess.WorkspaceID = activeWS
		}
		m.config.AddSession(*sess)
	}

	// Save config after creating all sessions
	if err := m.config.Save(); err != nil {
		log.Error("failed to save config after broadcast session creation", "error", err)
	}

	// Update sidebar with new sessions
	m.sidebar.SetSessions(m.getFilteredSessions())

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
// Runner retrieval is parallelized for better performance with many sessions.
func (m *Model) broadcastToSessions(sessions []config.Session, prompt string) (tea.Model, tea.Cmd) {
	log := logger.Get()
	log.Info("broadcasting to existing sessions", "count", len(sessions))

	// Build content blocks for the prompt
	content := []claude.ContentBlock{{
		Type: claude.ContentTypeText,
		Text: prompt,
	}}

	// First pass: parallelize getting/creating runners for all sessions
	type runnerResult struct {
		sess   config.Session
		runner claude.RunnerInterface
	}

	results := make(chan runnerResult, len(sessions))
	var wg sync.WaitGroup

	for _, sess := range sessions {
		wg.Add(1)
		go func(sess config.Session) {
			defer wg.Done()
			runner := m.sessionMgr.GetOrCreateRunner(&sess)
			results <- runnerResult{sess: sess, runner: runner}
		}(sess)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect runners
	var sessionsWithRunners []runnerResult
	for result := range results {
		if result.runner == nil {
			log.Error("failed to get runner for broadcast session", "sessionID", result.sess.ID)
			continue
		}
		sessionsWithRunners = append(sessionsWithRunners, result)
	}

	// Second pass: sequentially set up streaming and send content (modifies app state)
	var cmds []tea.Cmd
	sentCount := 0

	for _, result := range sessionsWithRunners {
		sessionID := result.sess.ID
		runner := result.runner

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
	case keys.Escape:
		m.modal.Hide()
		return m, nil
	case keys.Enter:
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
// Worktree status checks are parallelized for better performance with many sessions.
func (m *Model) createPRsForSessions(sessions []config.Session) (tea.Model, tea.Cmd) {
	log := logger.Get()
	log.Info("creating PRs for multiple sessions", "count", len(sessions))

	var cmds []tea.Cmd
	startedCount := 0
	skippedCount := 0

	// First pass: quick in-memory filtering to find candidates
	var candidates []config.Session
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

		candidates = append(candidates, sess)
	}

	// Second pass: parallel worktree status checks
	type statusResult struct {
		sess       config.Session
		hasChanges bool
		err        error
	}

	const maxConcurrent = 10
	sem := make(chan struct{}, maxConcurrent)
	results := make(chan statusResult, len(candidates))

	var wg sync.WaitGroup
	for _, sess := range candidates {
		wg.Add(1)
		go func(sess config.Session) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ctx := context.Background()
			status, err := m.gitService.GetWorktreeStatus(ctx, sess.WorkTree)
			results <- statusResult{sess: sess, hasChanges: status != nil && status.HasChanges, err: err}
		}(sess)
	}

	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect eligible sessions
	var eligible []config.Session
	for result := range results {
		sessionLog := logger.WithSession(result.sess.ID)
		if result.err != nil {
			sessionLog.Warn("failed to check worktree status", "error", result.err)
			skippedCount++
			continue
		}
		if result.hasChanges {
			sessionLog.Debug("skipping PR creation - has uncommitted changes")
			skippedCount++
			continue
		}
		eligible = append(eligible, result.sess)
	}

	// Third pass: start PR creation for eligible sessions (sequential - modifies app state)
	for _, sess := range eligible {
		sessionLog := logger.WithSession(sess.ID)
		sessionLog.Info("starting PR creation")
		mergeCtx, cancel := context.WithCancel(context.Background())
		m.sessionState().StartMerge(sess.ID, m.gitService.CreatePR(mergeCtx, sess.RepoPath, sess.WorkTree, sess.Branch, "", sess.GetIssueRef()), cancel, MergeTypePR)

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

// handleBulkActionModal handles key events for the Bulk Action modal.
func (m *Model) handleBulkActionModal(key string, msg tea.KeyPressMsg, state *ui.BulkActionState) (tea.Model, tea.Cmd) {
	switch key {
	case keys.Escape:
		m.modal.Hide()
		return m, nil
	case keys.Enter:
		switch state.GetAction() {
		case ui.BulkActionDelete:
			return m.executeBulkDelete(state.SessionIDs)
		case ui.BulkActionMoveToWorkspace:
			wsID := state.GetSelectedWorkspaceID()
			if wsID == "" {
				return m, nil
			}
			return m.executeBulkMove(state.SessionIDs, wsID)
		}
		return m, nil
	}
	// Forward other keys for navigation
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// executeBulkDelete deletes multiple sessions
func (m *Model) executeBulkDelete(sessionIDs []string) (tea.Model, tea.Cmd) {
	log := logger.Get()
	ctx := context.Background()

	// Delete worktrees in parallel using bounded concurrency
	const maxConcurrent = 10
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	for _, id := range sessionIDs {
		sess := m.config.GetSession(id)
		if sess == nil {
			continue
		}
		wg.Add(1)
		go func(s *config.Session) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := m.sessionService.Delete(ctx, s); err != nil {
				log.Warn("failed to delete worktree during bulk delete", "session", s.ID, "error", err)
			}
		}(sess)
	}
	wg.Wait()

	// Clean up state for each session (must be sequential - UI operations)
	for _, id := range sessionIDs {
		config.DeleteSessionMessages(id)
		m.sessionMgr.DeleteSession(id)
		m.sidebar.SetPendingPermission(id, false)
		m.sidebar.SetPendingQuestion(id, false)
		m.sidebar.SetIdleWithResponse(id, false)
		m.sidebar.SetUncommittedChanges(id, false)

		// Clear active session if deleted
		if m.activeSession != nil && m.activeSession.ID == id {
			m.activeSession = nil
			m.claudeRunner = nil
			m.chat.ClearSession()
			m.header.SetSessionName("")
			m.header.SetBaseBranch("")
			m.header.SetDiffStats(nil)
		}
	}

	// Batch remove all sessions from config and clean up orphaned parent refs
	deleted := m.config.RemoveSessions(sessionIDs)
	m.config.ClearOrphanedParentIDs(sessionIDs)

	if err := m.config.Save(); err != nil {
		log.Error("failed to save config after bulk delete", "error", err)
	}

	// Exit multi-select mode and update sidebar
	m.sidebar.ExitMultiSelect()
	m.sidebar.SetSessions(m.getFilteredSessions())
	m.modal.Hide()

	return m, m.ShowFlashSuccess(fmt.Sprintf("Deleted %d session(s)", deleted))
}

// executeBulkMove moves multiple sessions to a workspace
func (m *Model) executeBulkMove(sessionIDs []string, workspaceID string) (tea.Model, tea.Cmd) {
	count := m.config.SetSessionsWorkspace(sessionIDs, workspaceID)
	if err := m.config.Save(); err != nil {
		logger.Get().Error("failed to save config after bulk move", "error", err)
	}

	// Exit multi-select mode and update sidebar
	m.sidebar.ExitMultiSelect()
	m.sidebar.SetSessions(m.getFilteredSessions())
	m.modal.Hide()

	// Find workspace name for flash message
	var wsName string
	for _, ws := range m.config.GetWorkspaces() {
		if ws.ID == workspaceID {
			wsName = ws.Name
			break
		}
	}

	return m, m.ShowFlashSuccess(fmt.Sprintf("Moved %d session(s) to \"%s\"", count, wsName))
}
