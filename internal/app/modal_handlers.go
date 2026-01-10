package app

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/claude"
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
	case *ui.ThemeState:
		return m.handleThemeModal(key, msg, s)
	case *ui.MergeConflictState:
		return m.handleMergeConflictModal(key, msg, s)
	case *ui.ExploreOptionsState:
		return m.handleExploreOptionsModal(key, msg, s)
	case *ui.ForkSessionState:
		return m.handleForkSessionModal(key, msg, s)
	case *ui.HelpState:
		return m.handleHelpModal(key, msg, s)
	case *ui.ImportIssuesState:
		return m.handleImportIssuesModal(key, msg, s)
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

		// Determine merge type
		var mergeType MergeType
		switch option {
		case "Merge to parent":
			mergeType = MergeTypeParent
		case "Create PR":
			mergeType = MergeTypePR
		case "Push updates to PR":
			mergeType = MergeTypePush
		default:
			mergeType = MergeTypeMerge
		}

		// For merge-to-parent, validate parent exists
		var parentSess *config.Session
		if mergeType == MergeTypeParent {
			if sess.ParentID == "" {
				m.chat.AppendStreaming("Error: Session has no parent to merge to\n")
				return m, nil
			}
			parentSess = m.config.GetSession(sess.ParentID)
			if parentSess == nil {
				m.chat.AppendStreaming("Error: Parent session not found\n")
				return m, nil
			}
			m.pendingParentSession = parentSess.ID
		}

		if status.HasChanges {
			// Generate commit message and show edit modal
			m.chat.AppendStreaming("Generating commit message with Claude...\n")
			m.pendingCommitSession = sess.ID
			m.pendingCommitType = mergeType
			return m, m.generateCommitMessage(sess.ID, sess.WorkTree)
		}

		// No changes - proceed directly with merge/PR/push
		ctx, cancel := context.WithCancel(context.Background())
		switch mergeType {
		case MergeTypePR:
			logger.Log("App: Creating PR for branch %s (no uncommitted changes)", sess.Branch)
			m.chat.AppendStreaming("Creating PR for " + sess.Branch + "...\n\n")
			m.sessionState().StartMerge(sess.ID, git.CreatePR(ctx, sess.RepoPath, sess.WorkTree, sess.Branch, ""), cancel, MergeTypePR)
		case MergeTypePush:
			logger.Log("App: Pushing updates for branch %s (no uncommitted changes)", sess.Branch)
			m.chat.AppendStreaming("Pushing updates to " + sess.Branch + "...\n\n")
			m.sessionState().StartMerge(sess.ID, git.PushUpdates(ctx, sess.RepoPath, sess.WorkTree, sess.Branch, ""), cancel, MergeTypePush)
		case MergeTypeParent:
			logger.Log("App: Merging branch %s to parent %s (no uncommitted changes)", sess.Branch, parentSess.Branch)
			m.chat.AppendStreaming("Merging " + sess.Branch + " to parent " + parentSess.Branch + "...\n\n")
			m.sessionState().StartMerge(sess.ID, git.MergeToParent(ctx, sess.WorkTree, sess.Branch, parentSess.WorkTree, parentSess.Branch, ""), cancel, MergeTypeParent)
		default:
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
		if state.MergeType == "conflict" {
			// Don't clear conflict state on cancel - user might want to try again
			m.chat.AppendStreaming("Commit cancelled. Press 'c' to try again.\n")
		} else {
			m.pendingCommitSession = ""
			m.pendingCommitType = MergeTypeNone
			m.chat.AppendStreaming("Cancelled.\n")
		}
		return m, nil
	case "ctrl+s":
		// Confirm commit
		commitMsg := state.GetMessage()
		if commitMsg == "" {
			return m, nil // Don't allow empty commit messages
		}
		m.modal.Hide()

		// Handle conflict resolution commit
		if state.MergeType == "conflict" {
			return m.commitConflictResolution(commitMsg)
		}

		// Handle normal merge/PR commit
		sess := m.config.GetSession(m.pendingCommitSession)
		if sess == nil {
			m.pendingCommitSession = ""
			m.pendingCommitType = MergeTypeNone
			return m, nil
		}

		mergeType := m.pendingCommitType
		parentSessionID := m.pendingParentSession
		m.pendingCommitSession = ""
		m.pendingCommitType = MergeTypeNone
		m.pendingParentSession = ""

		// Proceed with merge/PR/push using the edited commit message
		ctx, cancel := context.WithCancel(context.Background())
		switch mergeType {
		case MergeTypePR:
			logger.Log("App: Creating PR for branch %s with user-edited commit message", sess.Branch)
			m.chat.AppendStreaming("Creating PR for " + sess.Branch + "...\n\n")
			m.sessionState().StartMerge(sess.ID, git.CreatePR(ctx, sess.RepoPath, sess.WorkTree, sess.Branch, commitMsg), cancel, MergeTypePR)
		case MergeTypePush:
			logger.Log("App: Pushing updates for branch %s with user-edited commit message", sess.Branch)
			m.chat.AppendStreaming("Pushing updates to " + sess.Branch + "...\n\n")
			m.sessionState().StartMerge(sess.ID, git.PushUpdates(ctx, sess.RepoPath, sess.WorkTree, sess.Branch, commitMsg), cancel, MergeTypePush)
		case MergeTypeParent:
			parentSess := m.config.GetSession(parentSessionID)
			if parentSess == nil {
				m.chat.AppendStreaming("Error: Parent session not found\n")
				cancel()
				return m, nil
			}
			logger.Log("App: Merging branch %s to parent %s with user-edited commit message", sess.Branch, parentSess.Branch)
			m.chat.AppendStreaming("Merging " + sess.Branch + " to parent " + parentSess.Branch + "...\n\n")
			m.sessionState().StartMerge(sess.ID, git.MergeToParent(ctx, sess.WorkTree, sess.Branch, parentSess.WorkTree, parentSess.Branch, commitMsg), cancel, MergeTypeParent)
		default:
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

// commitConflictResolution commits the resolved merge conflicts.
func (m *Model) commitConflictResolution(commitMsg string) (tea.Model, tea.Cmd) {
	if m.pendingConflictRepoPath == "" {
		m.chat.AppendStreaming("[Error: No pending conflict resolution]\n")
		return m, nil
	}

	logger.Log("App: Committing conflict resolution in %s", m.pendingConflictRepoPath)
	err := git.CommitConflictResolution(m.pendingConflictRepoPath, commitMsg)
	if err != nil {
		m.chat.AppendStreaming(fmt.Sprintf("[Error committing: %v]\n", err))
		return m, nil
	}

	m.chat.AppendStreaming("Merge conflicts resolved and committed successfully!\n")

	// Mark the session as merged
	if m.pendingConflictSessionID != "" {
		m.config.MarkSessionMerged(m.pendingConflictSessionID)
		m.config.Save()
		m.sidebar.SetSessions(m.config.GetSessions())
		logger.Log("App: Marked session %s as merged after conflict resolution", m.pendingConflictSessionID)
	}

	// Clear pending conflict state
	m.pendingConflictRepoPath = ""
	m.pendingConflictSessionID = ""

	return m, nil
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

// handleThemeModal handles key events for the Theme picker modal.
func (m *Model) handleThemeModal(key string, msg tea.KeyPressMsg, state *ui.ThemeState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.modal.Hide()
		return m, nil
	case "enter":
		selectedTheme := state.GetSelectedTheme()
		ui.SetTheme(selectedTheme)
		m.config.SetTheme(string(selectedTheme))
		m.config.Save()
		m.modal.Hide()
		return m, nil
	case "up", "k", "down", "j":
		// Forward navigation keys to modal
		modal, cmd := m.modal.Update(msg)
		m.modal = modal
		return m, cmd
	}
	return m, nil
}

// handleMergeConflictModal handles key events for the Merge Conflict modal.
func (m *Model) handleMergeConflictModal(key string, msg tea.KeyPressMsg, state *ui.MergeConflictState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.modal.Hide()
		return m, nil
	case "enter":
		option := state.GetSelectedOption()
		m.modal.Hide()

		switch option {
		case 0: // "Have Claude resolve"
			return m.handleClaudeResolveConflict(state)
		case 1: // "Abort merge"
			return m.handleAbortMerge(state)
		case 2: // "Resolve manually"
			return m.handleManualResolve(state)
		}
		return m, nil
	case "up", "k", "down", "j":
		// Forward navigation keys to modal
		modal, cmd := m.modal.Update(msg)
		m.modal = modal
		return m, cmd
	}
	return m, nil
}

// handleClaudeResolveConflict sends a prompt to Claude to resolve merge conflicts.
func (m *Model) handleClaudeResolveConflict(state *ui.MergeConflictState) (tea.Model, tea.Cmd) {
	sess := m.config.GetSession(state.SessionID)
	if sess == nil {
		m.chat.AppendStreaming("[Error: Session not found]\n")
		return m, nil
	}

	// Make sure this session is active
	if m.activeSession == nil || m.activeSession.ID != sess.ID {
		m.selectSession(sess)
	}

	// Build the list of conflicted files with full paths
	var filesList strings.Builder
	for _, file := range state.ConflictedFiles {
		filesList.WriteString(fmt.Sprintf("  %s/%s\n", state.RepoPath, file))
	}

	prompt := fmt.Sprintf(`The merge to main encountered conflicts in these files:
%s
Please resolve these merge conflicts by:
1. Reading each conflicted file
2. Understanding both versions (between <<<<<<< and >>>>>>> markers)
3. Editing the file to combine the changes appropriately
4. Removing the conflict markers

Do NOT run git add or git commit - I will handle that after reviewing your changes.`, filesList.String())

	logger.Log("App: Sending conflict resolution prompt to Claude for session %s", sess.ID)
	m.chat.AddUserMessage(prompt)

	// Store conflict info for later commit
	m.pendingConflictRepoPath = state.RepoPath
	m.pendingConflictSessionID = state.SessionID

	// Get runner
	runner := m.sessionMgr.GetRunner(sess.ID)
	if runner == nil {
		m.chat.AppendStreaming("[Error: Could not get Claude runner]\n")
		return m, nil
	}

	m.claudeRunner = runner

	// Create context for this request
	ctx, cancel := context.WithCancel(context.Background())
	m.sessionState().StartWaiting(sess.ID, cancel)
	startTime, _ := m.sessionState().GetWaitStart(sess.ID)
	m.chat.SetWaitingWithStart(true, startTime)
	m.sidebar.SetStreaming(sess.ID, true)
	m.setState(StateStreamingClaude)

	// Send to Claude
	content := []claude.ContentBlock{{Type: claude.ContentTypeText, Text: prompt}}
	responseChan := runner.SendContent(ctx, content)

	return m, tea.Batch(
		m.listenForSessionResponse(sess.ID, responseChan),
		m.listenForSessionPermission(sess.ID, runner),
		m.listenForSessionQuestion(sess.ID, runner),
		ui.SidebarTick(),
		ui.StopwatchTick(),
	)
}

// handleAbortMerge aborts the in-progress merge.
func (m *Model) handleAbortMerge(state *ui.MergeConflictState) (tea.Model, tea.Cmd) {
	err := git.AbortMerge(state.RepoPath)
	if err != nil {
		m.chat.AppendStreaming(fmt.Sprintf("[Error aborting merge: %v]\n", err))
	} else {
		m.chat.AppendStreaming("Merge aborted successfully.\n")
	}
	return m, nil
}

// handleManualResolve shows info for manual conflict resolution.
func (m *Model) handleManualResolve(state *ui.MergeConflictState) (tea.Model, tea.Cmd) {
	var msg strings.Builder
	msg.WriteString("To resolve conflicts manually:\n\n")
	msg.WriteString(fmt.Sprintf("  cd %s\n\n", state.RepoPath))
	msg.WriteString("Conflicted files:\n")
	for _, file := range state.ConflictedFiles {
		msg.WriteString(fmt.Sprintf("  %s\n", file))
	}
	msg.WriteString("\nAfter resolving:\n")
	msg.WriteString("  git add <files>\n")
	msg.WriteString("  git commit\n\n")
	msg.WriteString("Or abort with: git merge --abort\n")

	m.chat.AppendStreaming(msg.String())
	return m, nil
}

// handleExploreOptionsModal handles key events for the Explore Options modal.
func (m *Model) handleExploreOptionsModal(key string, msg tea.KeyPressMsg, state *ui.ExploreOptionsState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.modal.Hide()
		return m, nil
	case "enter":
		selected := state.GetSelectedOptions()
		if len(selected) == 0 {
			return m, nil
		}
		m.modal.Hide()
		return m.createParallelSessions(selected)
	case "up", "k", "down", "j", "space":
		// Forward navigation and space (toggle) keys to modal
		modal, cmd := m.modal.Update(msg)
		m.modal = modal
		return m, cmd
	}
	return m, nil
}

// handleImportIssuesModal handles key events for the Import Issues modal.
func (m *Model) handleImportIssuesModal(key string, msg tea.KeyPressMsg, state *ui.ImportIssuesState) (tea.Model, tea.Cmd) {
	// Don't handle keys while loading
	if state.Loading {
		return m, nil
	}

	switch key {
	case "esc":
		m.modal.Hide()
		return m, nil
	case "enter":
		selected := state.GetSelectedIssues()
		if len(selected) == 0 {
			return m, nil
		}
		m.modal.Hide()
		return m.createSessionsFromIssues(state.RepoPath, selected)
	case "up", "k", "down", "j", "space":
		// Forward navigation and space (toggle) keys to modal
		modal, cmd := m.modal.Update(msg)
		m.modal = modal
		return m, cmd
	}
	return m, nil
}

// createSessionsFromIssues creates new sessions for each selected GitHub issue.
func (m *Model) createSessionsFromIssues(repoPath string, issues []ui.IssueItem) (tea.Model, tea.Cmd) {
	var firstSession *config.Session

	for _, issue := range issues {
		// Create branch name from issue number
		branchName := fmt.Sprintf("issue-%d", issue.Number)

		// Check if branch already exists and skip if so
		if session.BranchExists(repoPath, branchName) {
			logger.Log("App: Skipping issue #%d, branch %s already exists", issue.Number, branchName)
			continue
		}

		// Create new session
		sess, err := session.Create(repoPath, branchName)
		if err != nil {
			logger.Log("App: Failed to create session for issue #%d: %v", issue.Number, err)
			continue
		}

		// Create initial message with issue context
		initialMsg := fmt.Sprintf("GitHub Issue #%d: %s\n\n%s\n\n---\nPlease help me work on this issue.",
			issue.Number, issue.Title, issue.Body)

		// Save initial message as user message
		messages := []config.Message{
			{Role: "user", Content: initialMsg},
		}
		if err := config.SaveSessionMessages(sess.ID, messages, config.MaxSessionMessageLines); err != nil {
			logger.Log("App: Warning - failed to save initial message for issue #%d: %v", issue.Number, err)
		}

		// No parent ID - these are top-level sessions
		logger.Log("App: Created session for issue #%d: id=%s, name=%s", issue.Number, sess.ID, sess.Name)

		m.config.AddSession(*sess)
		if firstSession == nil {
			firstSession = sess
		}
	}

	// Save config and update sidebar
	if err := m.config.Save(); err != nil {
		logger.Log("App: Failed to save config: %v", err)
	}
	m.sidebar.SetSessions(m.config.GetSessions())

	// Select the first created session
	if firstSession != nil {
		m.sidebar.SelectSession(firstSession.ID)
		m.selectSession(firstSession)
	}

	return m, nil
}

// parallelSessionInfo holds info needed to start a session after creation
type parallelSessionInfo struct {
	Session      *config.Session
	OptionPrompt string
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
		// Check if branch already exists
		if branchName != "" && session.BranchExists(state.RepoPath, branchName) {
			m.modal.SetError("Branch already exists: " + branchName)
			return m, nil
		}

		logger.Log("App: Forking session %s, copyMessages=%v, branch=%q", state.ParentSessionID, state.CopyMessages, branchName)

		// Create new session
		sess, err := session.Create(state.RepoPath, branchName)
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

// createParallelSessions creates new sessions for each selected option, pre-populated with history.
func (m *Model) createParallelSessions(selectedOptions []ui.OptionItem) (tea.Model, tea.Cmd) {
	if m.activeSession == nil || m.claudeRunner == nil {
		return m, nil
	}

	parentSession := m.activeSession
	parentMessages := m.claudeRunner.GetMessages()

	logger.Log("App: Creating %d parallel sessions from session %s", len(selectedOptions), parentSession.ID)

	var cmds []tea.Cmd
	var createdSessions []parallelSessionInfo
	var firstSession *config.Session

	for _, opt := range selectedOptions {
		// Create a branch name based on the option
		branchName := fmt.Sprintf("option-%d", opt.Number)

		// Create new session
		sess, err := session.Create(parentSession.RepoPath, branchName)
		if err != nil {
			logger.Log("App: Failed to create parallel session for option %d: %v", opt.Number, err)
			m.chat.AppendStreaming(fmt.Sprintf("[Error creating session for option %d: %v]\n", opt.Number, err))
			continue
		}

		logger.Log("App: Created parallel session %s for option %d", sess.ID, opt.Number)

		// Build message history: parent messages only (option prompt will be added by SendContent)
		var messages []config.Message
		for _, msg := range parentMessages {
			messages = append(messages, config.Message{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}

		// Option prompt to send (will be added to history by SendContent)
		optionPrompt := fmt.Sprintf("Let's go with option %d: %s", opt.Number, opt.Text)

		// Save parent messages to disk for this new session
		if err := config.SaveSessionMessages(sess.ID, messages, config.MaxSessionMessageLines); err != nil {
			logger.Log("App: Failed to save messages for parallel session %s: %v", sess.ID, err)
		}

		// Set parent ID to track fork relationship
		sess.ParentID = parentSession.ID

		// Add session to config
		m.config.AddSession(*sess)
		createdSessions = append(createdSessions, parallelSessionInfo{
			Session:      sess,
			OptionPrompt: optionPrompt,
		})

		if firstSession == nil {
			firstSession = sess
		}
	}

	// Save config
	if err := m.config.Save(); err != nil {
		logger.Log("App: Failed to save config after creating parallel sessions: %v", err)
	}

	// Update sidebar
	m.sidebar.SetSessions(m.config.GetSessions())

	// Clear detected options since we've acted on them
	m.sessionState().ClearDetectedOptions(parentSession.ID)

	// Start all sessions in parallel
	if len(createdSessions) > 0 {
		m.chat.AppendStreaming(fmt.Sprintf("\nCreated %d parallel session(s) to explore options.\n", len(createdSessions)))

		// Start each session
		for _, info := range createdSessions {
			sess := info.Session
			optionPrompt := info.OptionPrompt

			// Get or create runner for this session (this loads pre-populated messages)
			result := m.sessionMgr.Select(sess, "", "", "")
			if result == nil || result.Runner == nil {
				logger.Log("App: Failed to get runner for parallel session %s", sess.ID)
				continue
			}

			runner := result.Runner

			// Start streaming for this session
			ctx, cancel := context.WithCancel(context.Background())
			m.sessionState().StartWaiting(sess.ID, cancel)
			m.sidebar.SetStreaming(sess.ID, true)

			logger.Log("App: Auto-starting parallel session %s with prompt: %s", sess.ID, optionPrompt)

			// Send the option choice to Claude
			content := []claude.ContentBlock{{Type: claude.ContentTypeText, Text: optionPrompt}}
			responseChan := runner.SendContent(ctx, content)

			// Add listeners for this session
			cmds = append(cmds,
				m.listenForSessionResponse(sess.ID, responseChan),
				m.listenForSessionPermission(sess.ID, runner),
				m.listenForSessionQuestion(sess.ID, runner),
			)
		}

		// Switch to the first session's UI
		if firstSession != nil {
			m.sidebar.SelectSession(firstSession.ID)
			m.selectSession(firstSession)

			// Update UI for the active session
			if m.claudeRunner != nil {
				startTime, _ := m.sessionState().GetWaitStart(firstSession.ID)
				m.chat.SetWaitingWithStart(true, startTime)
				m.chat.AddUserMessage(createdSessions[0].OptionPrompt)
			}
		}

		m.setState(StateStreamingClaude)
		cmds = append(cmds, ui.SidebarTick(), ui.StopwatchTick())
	}

	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// handleHelpModal handles key events for the Help modal.
func (m *Model) handleHelpModal(key string, msg tea.KeyPressMsg, state *ui.HelpState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "?", "q":
		m.modal.Hide()
		return m, nil
	case "enter":
		// Trigger the selected shortcut
		shortcut := state.GetSelectedShortcut()
		if shortcut != nil {
			m.modal.Hide()
			// Return a command that sends a HelpShortcutTriggeredMsg
			return m, func() tea.Msg {
				return ui.HelpShortcutTriggeredMsg{Key: shortcut.Key}
			}
		}
		return m, nil
	}
	// Forward navigation keys to the modal
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// handleHelpShortcutTrigger handles shortcuts triggered from the help modal.
// It maps display keys to actual actions.
func (m *Model) handleHelpShortcutTrigger(key string) (tea.Model, tea.Cmd) {
	// Normalize key names from help display to actual key values
	normalizedKey := strings.ToLower(key)

	// Handle special display-format keys
	switch key {
	case "Tab":
		normalizedKey = "tab"
	case "↑/↓ or j/k":
		// Navigation keys - just toggle focus as a demonstration
		normalizedKey = "tab"
	case "PgUp/PgDn":
		// Page keys - no direct action from modal
		return m, nil
	case "Enter":
		normalizedKey = "enter"
	case "/":
		normalizedKey = "/"
	case "Esc":
		// Escape - no action from modal
		return m, nil
	case "Ctrl+V":
		// Image paste - only works in chat context
		return m, nil
	case "Ctrl+P":
		// Fork options - only works in chat context with detected options
		return m, nil
	case "ctrl+f":
		// Force resume - only works with session in use
		return m, nil
	}

	// Now handle the normalized key
	switch normalizedKey {
	case "tab":
		m.toggleFocus()
		return m, nil
	case "n":
		m.modal.Show(ui.NewNewSessionState(m.config.GetRepos()))
		return m, nil
	case "a":
		currentRepo := session.GetCurrentDirGitRoot()
		if currentRepo != "" {
			for _, repo := range m.config.GetRepos() {
				if repo == currentRepo {
					currentRepo = ""
					break
				}
			}
		}
		m.modal.Show(ui.NewAddRepoState(currentRepo))
		return m, nil
	case "d":
		if m.sidebar.SelectedSession() != nil {
			sess := m.sidebar.SelectedSession()
			displayName := ui.SessionDisplayName(sess.Branch, sess.Name)
			m.modal.Show(ui.NewConfirmDeleteState(displayName))
		}
		return m, nil
	case "v":
		if m.sidebar.SelectedSession() != nil {
			sess := m.sidebar.SelectedSession()
			if m.activeSession == nil || m.activeSession.ID != sess.ID {
				m.selectSession(sess)
			}
			status, err := git.GetWorktreeStatus(sess.WorkTree)
			var content string
			if err != nil {
				content = fmt.Sprintf("[Error getting status: %v]\n", err)
			} else if !status.HasChanges {
				content = "No uncommitted changes in this session."
			} else {
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("📝 Uncommitted changes (%s):\n\n", status.Summary))
				for _, file := range status.Files {
					sb.WriteString(fmt.Sprintf("  • %s\n", file))
				}
				if status.Diff != "" {
					sb.WriteString("\n--- Diff ---\n")
					sb.WriteString(ui.HighlightDiff(status.Diff))
				}
				content = sb.String()
			}
			m.chat.EnterViewChangesMode(content)
		}
		return m, nil
	case "m":
		if m.sidebar.SelectedSession() != nil {
			sess := m.sidebar.SelectedSession()
			hasRemote := git.HasRemoteOrigin(sess.RepoPath)
			var changesSummary string
			if status, err := git.GetWorktreeStatus(sess.WorkTree); err == nil && status.HasChanges {
				changesSummary = status.Summary
				if len(status.Files) <= 5 {
					changesSummary += ": " + strings.Join(status.Files, ", ")
				}
			}
			displayName := ui.SessionDisplayName(sess.Branch, sess.Name)
			var parentName string
			if sess.ParentID != "" {
				if parent := m.config.GetSession(sess.ParentID); parent != nil {
					parentName = ui.SessionDisplayName(parent.Branch, parent.Name)
				}
			}
			m.modal.Show(ui.NewMergeState(displayName, hasRemote, changesSummary, parentName, sess.PRCreated))
		}
		return m, nil
	case "f":
		if m.sidebar.SelectedSession() != nil {
			sess := m.sidebar.SelectedSession()
			displayName := ui.SessionDisplayName(sess.Branch, sess.Name)
			m.modal.Show(ui.NewForkSessionState(displayName, sess.ID, sess.RepoPath))
		}
		return m, nil
	case "c":
		if m.pendingConflictRepoPath != "" {
			return m.showCommitConflictModal()
		}
		return m, nil
	case "s":
		m.showMCPServersModal()
		return m, nil
	case "/":
		if !m.sidebar.IsSearchMode() {
			m.sidebar.EnterSearchMode()
		}
		return m, nil
	case "t":
		m.modal.Show(ui.NewThemeState(ui.CurrentThemeName()))
		return m, nil
	case "?":
		m.modal.Show(ui.NewHelpState())
		return m, nil
	case "q":
		return m, tea.Quit
	case "y":
		// Permission responses - only work in permission context
		return m, nil
	}

	return m, nil
}

