package app

import (
	"context"

	tea "charm.land/bubbletea/v2"
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
