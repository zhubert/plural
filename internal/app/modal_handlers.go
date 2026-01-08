package app

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/session"
	"github.com/zhubert/plural/internal/ui"
)

// handleModalKey routes modal key events to the appropriate handler based on modal state type.
// This reduces the size of the main Update function by delegating modal handling.
func (m *Model) handleModalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch s := m.modal.State.(type) {
	case *ui.AddRepoState:
		return m.handleAddRepoModal(key, msg, s)
	case *ui.NewSessionState:
		return m.handleNewSessionModal(key, msg, s)
	case *ui.ConfirmDeleteState:
		return m.handleConfirmDeleteModal(key, msg, s)
	case *ui.MergeState:
		return m.handleMergeModal(key, msg, s)
	case *ui.MCPServersState:
		return m.handleMCPServersModal(key, msg, s)
	case *ui.AddMCPServerState:
		return m.handleAddMCPServerModal(key, msg, s)
	case *ui.EditCommitState:
		return m.handleEditCommitModal(key, msg, s)
	case *ui.WelcomeState:
		return m.handleWelcomeModal(key, msg, s)
	case *ui.ChangelogState:
		return m.handleChangelogModal(key, msg, s)
	}

	// Default: update modal input (for text-based modals)
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// handleAddRepoModal handles key events for the Add Repository modal.
func (m *Model) handleAddRepoModal(key string, msg tea.KeyPressMsg, state *ui.AddRepoState) (tea.Model, tea.Cmd) {
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
		// Check if branch already exists
		if branchName != "" && session.BranchExists(repoPath, branchName) {
			m.modal.SetError("Branch already exists: " + branchName)
			return m, nil
		}
		logger.Log("App: Creating new session for repo=%s, branch=%q", repoPath, branchName)
		sess, err := session.Create(repoPath, branchName)
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
			if m.activeSession != nil && m.activeSession.ID == sess.ID {
				m.activeSession = nil
				m.claudeRunner = nil
				m.chat.ClearSession()
				m.header.SetSessionName("")
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

// handleMergeModal handles key events for the Merge/PR modal.
func (m *Model) handleMergeModal(key string, msg tea.KeyPressMsg, state *ui.MergeState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.modal.Hide()
		return m, nil
	case "enter":
		option := state.GetSelectedOption()
		sess := m.sidebar.SelectedSession()
		if option == "" || sess == nil {
			return m, nil
		}
		// Check if this session already has a merge in progress
		if m.sessionState().IsMerging(sess.ID) {
			logger.Log("App: Merge already in progress for session %s", sess.ID)
			return m, nil
		}
		logger.Log("App: Starting merge operation: option=%q, session=%s, branch=%s, worktree=%s", option, sess.ID, sess.Branch, sess.WorkTree)
		m.modal.Hide()
		if m.activeSession == nil || m.activeSession.ID != sess.ID {
			m.selectSession(sess)
		}

		// Check for uncommitted changes
		status, err := git.GetWorktreeStatus(sess.WorkTree)
		if err != nil {
			m.chat.AppendStreaming(fmt.Sprintf("Error checking worktree status: %v\n", err))
			return m, nil
		}

		mergeType := MergeTypeMerge
		if option == "Create PR" {
			mergeType = MergeTypePR
		}

		if status.HasChanges {
			// Generate commit message and show edit modal
			m.chat.AppendStreaming("Generating commit message with Claude...\n")
			m.pendingCommitSession = sess.ID
			m.pendingCommitType = mergeType
			return m, m.generateCommitMessage(sess.ID, sess.WorkTree)
		}

		// No changes - proceed directly with merge/PR
		ctx, cancel := context.WithCancel(context.Background())
		if mergeType == MergeTypePR {
			logger.Log("App: Creating PR for branch %s (no uncommitted changes)", sess.Branch)
			m.chat.AppendStreaming("Creating PR for " + sess.Branch + "...\n\n")
			m.sessionState().StartMerge(sess.ID, git.CreatePR(ctx, sess.RepoPath, sess.WorkTree, sess.Branch, ""), cancel, MergeTypePR)
		} else {
			logger.Log("App: Merging branch %s to main (no uncommitted changes)", sess.Branch)
			m.chat.AppendStreaming("Merging " + sess.Branch + " to main...\n\n")
			m.sessionState().StartMerge(sess.ID, git.MergeToMain(ctx, sess.RepoPath, sess.WorkTree, sess.Branch, ""), cancel, MergeTypeMerge)
		}
		return m, m.listenForMergeResult(sess.ID)
	}
	// Forward other keys to the modal for navigation handling
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// handleMCPServersModal handles key events for the MCP Servers modal.
func (m *Model) handleMCPServersModal(key string, msg tea.KeyPressMsg, state *ui.MCPServersState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.modal.Hide()
		return m, nil
	case "a":
		m.modal.Show(ui.NewAddMCPServerState(m.config.GetRepos()))
		return m, nil
	case "d":
		if server := state.GetSelectedServer(); server != nil {
			if server.IsGlobal {
				m.config.RemoveGlobalMCPServer(server.Name)
			} else {
				m.config.RemoveRepoMCPServer(server.RepoPath, server.Name)
			}
			m.config.Save()
			m.showMCPServersModal() // Refresh the modal
		}
		return m, nil
	}
	// Forward other keys to the modal for navigation handling
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// handleAddMCPServerModal handles key events for the Add MCP Server modal.
func (m *Model) handleAddMCPServerModal(key string, msg tea.KeyPressMsg, state *ui.AddMCPServerState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.showMCPServersModal() // Go back to list
		return m, nil
	case "enter":
		name, command, args, repoPath, isGlobal := state.GetValues()
		if name == "" || command == "" {
			return m, nil
		}
		// Parse args (space-separated)
		var argsList []string
		if args != "" {
			argsList = strings.Fields(args)
		}
		server := config.MCPServer{
			Name:    name,
			Command: command,
			Args:    argsList,
		}
		if isGlobal {
			m.config.AddGlobalMCPServer(server)
		} else {
			m.config.AddRepoMCPServer(repoPath, server)
		}
		m.config.Save()
		m.modal.Hide()
		return m, nil
	}
	// Forward other keys to the modal for text input handling
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// handleEditCommitModal handles key events for the Edit Commit modal.
func (m *Model) handleEditCommitModal(key string, msg tea.KeyPressMsg, state *ui.EditCommitState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		// Cancel commit message editing
		m.modal.Hide()
		m.pendingCommitSession = ""
		m.pendingCommitType = MergeTypeNone
		m.chat.AppendStreaming("Cancelled.\n")
		return m, nil
	case "ctrl+s":
		// Confirm commit and proceed with merge/PR
		commitMsg := state.GetMessage()
		if commitMsg == "" {
			return m, nil // Don't allow empty commit messages
		}
		m.modal.Hide()

		sess := m.config.GetSession(m.pendingCommitSession)
		if sess == nil {
			m.pendingCommitSession = ""
			m.pendingCommitType = MergeTypeNone
			return m, nil
		}

		mergeType := m.pendingCommitType
		m.pendingCommitSession = ""
		m.pendingCommitType = MergeTypeNone

		// Proceed with merge/PR using the edited commit message
		ctx, cancel := context.WithCancel(context.Background())
		if mergeType == MergeTypePR {
			logger.Log("App: Creating PR for branch %s with user-edited commit message", sess.Branch)
			m.chat.AppendStreaming("Creating PR for " + sess.Branch + "...\n\n")
			m.sessionState().StartMerge(sess.ID, git.CreatePR(ctx, sess.RepoPath, sess.WorkTree, sess.Branch, commitMsg), cancel, MergeTypePR)
		} else {
			logger.Log("App: Merging branch %s to main with user-edited commit message", sess.Branch)
			m.chat.AppendStreaming("Merging " + sess.Branch + " to main...\n\n")
			m.sessionState().StartMerge(sess.ID, git.MergeToMain(ctx, sess.RepoPath, sess.WorkTree, sess.Branch, commitMsg), cancel, MergeTypeMerge)
		}
		return m, m.listenForMergeResult(sess.ID)
	}
	// Forward other keys to the modal for textarea handling
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// handleWelcomeModal handles key events for the Welcome modal.
func (m *Model) handleWelcomeModal(key string, msg tea.KeyPressMsg, state *ui.WelcomeState) (tea.Model, tea.Cmd) {
	switch key {
	case "enter", "esc":
		// Mark welcome as shown and save
		m.config.MarkWelcomeShown()
		m.config.Save()
		m.modal.Hide()
		// Check if we should also show changelog
		return m.handleStartupModals()
	}
	return m, nil
}

// handleChangelogModal handles key events for the Changelog modal.
func (m *Model) handleChangelogModal(key string, msg tea.KeyPressMsg, state *ui.ChangelogState) (tea.Model, tea.Cmd) {
	switch key {
	case "enter", "esc":
		// Update last seen version and save
		m.config.SetLastSeenVersion(m.version)
		m.config.Save()
		m.modal.Hide()
		return m, nil
	case "up", "k", "down", "j":
		// Forward scroll keys to modal
		modal, cmd := m.modal.Update(msg)
		m.modal = modal
		return m, cmd
	}
	return m, nil
}

