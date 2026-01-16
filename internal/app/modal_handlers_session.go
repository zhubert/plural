package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
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
		if err := session.ValidateRepo(path); err != nil {
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
		if fullBranchName != "" && session.BranchExists(repoPath, fullBranchName) {
			m.modal.SetError("Branch already exists: " + fullBranchName)
			return m, nil
		}
		logger.Log("App: Creating new session for repo=%s, branch=%q, prefix=%q", repoPath, branchName, branchPrefix)
		sess, err := session.Create(repoPath, branchName, branchPrefix)
		if err != nil {
			logger.Log("App: Failed to create session: %v", err)
			m.modal.SetError(err.Error())
			return m, nil
		}
		logger.Log("App: Session created: id=%s, name=%s", sess.ID, sess.Name)
		m.config.AddSession(*sess)
		if err := m.config.Save(); err != nil {
			logger.Log("App: Failed to save config: %v", err)
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
			deleteWorktree := state.ShouldDeleteWorktree()
			logger.Log("App: Deleting session: id=%s, name=%s, deleteWorktree=%v", sess.ID, sess.Name, deleteWorktree)

			// Delete worktree if requested
			if deleteWorktree {
				if err := session.Delete(sess); err != nil {
					logger.Log("App: Failed to delete worktree: %v", err)
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
			logger.Log("App: Checking if active session should be cleared: activeSession=%v, activeSessionID=%s, deletedSessionID=%s",
				m.activeSession != nil,
				func() string {
					if m.activeSession != nil {
						return m.activeSession.ID
					} else {
						return "<nil>"
					}
				}(),
				sess.ID)
			if m.activeSession != nil && m.activeSession.ID == sess.ID {
				logger.Log("App: Clearing active session and chat")
				m.activeSession = nil
				m.claudeRunner = nil
				m.chat.ClearSession()
				m.header.SetSessionName("")
			} else {
				logger.Log("App: Not clearing chat - deleted session was not the active session")
			}
			if deletedRunner != nil {
				logger.Log("App: Session deleted successfully (runner stopped): %s", sess.ID)
			} else {
				logger.Log("App: Session deleted successfully: %s", sess.ID)
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
		if fullBranchName != "" && session.BranchExists(state.RepoPath, fullBranchName) {
			m.modal.SetError("Branch already exists: " + fullBranchName)
			return m, nil
		}

		logger.Log("App: Forking session %s, copyMessages=%v, branch=%q, prefix=%q", state.ParentSessionID, state.CopyMessages, branchName, branchPrefix)

		// Create new session
		sess, err := session.Create(state.RepoPath, branchName, branchPrefix)
		if err != nil {
			logger.Log("App: Failed to create forked session: %v", err)
			m.modal.SetError(err.Error())
			return m, nil
		}

		// Copy messages if requested
		if state.CopyMessages {
			parentMsgs, err := config.LoadSessionMessages(state.ParentSessionID)
			if err != nil {
				logger.Log("App: Warning - failed to load parent session messages: %v", err)
			} else if len(parentMsgs) > 0 {
				if err := config.SaveSessionMessages(sess.ID, parentMsgs, config.MaxSessionMessageLines); err != nil {
					logger.Log("App: Warning - failed to save forked session messages: %v", err)
				} else {
					logger.Log("App: Copied %d messages from parent session", len(parentMsgs))
				}
			}
		}

		// Set parent ID to track fork relationship
		sess.ParentID = state.ParentSessionID

		logger.Log("App: Forked session created: id=%s, name=%s, parentID=%s", sess.ID, sess.Name, sess.ParentID)
		m.config.AddSession(*sess)
		if err := m.config.Save(); err != nil {
			logger.Log("App: Failed to save config: %v", err)
			m.modal.SetError("Failed to save: " + err.Error())
			return m, nil
		}
		m.sidebar.SetSessions(m.config.GetSessions())
		m.sidebar.SelectSession(sess.ID)
		m.selectSession(sess)
		m.modal.Hide()
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
		if newBranch != oldBranch && session.BranchExists(sess.RepoPath, newBranch) {
			m.modal.SetError("Branch already exists: " + newBranch)
			return m, nil
		}

		// Rename the git branch
		if newBranch != oldBranch {
			if err := git.RenameBranch(sess.WorkTree, oldBranch, newBranch); err != nil {
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
		logger.Log("App: Renamed session %s: branch=%q", state.SessionID, newBranch)

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
