package app

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/ui"
)

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
		log := logger.WithSession(sess.ID)
		// Check if this session already has a merge in progress
		if state := m.sessionState().GetIfExists(sess.ID); state != nil && state.IsMerging() {
			log.Debug("merge already in progress")
			return m, nil
		}
		// Check if there's already a pending commit message generation
		if m.pendingCommitSession == sess.ID {
			log.Debug("commit message generation already pending")
			return m, nil
		}
		log.Debug("starting merge operation", "option", option, "branch", sess.Branch, "worktree", sess.WorkTree)
		m.modal.Hide()
		if m.activeSession == nil || m.activeSession.ID != sess.ID {
			m.selectSession(sess)
		}

		// Check for uncommitted changes
		ctx := context.Background()
		status, err := m.gitService.GetWorktreeStatus(ctx, sess.WorkTree)
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
			// Finish any existing streaming before starting merge operation
			m.chat.FinishStreaming()
			// Show loading modal with spinner while generating commit message
			m.modal.Show(ui.NewLoadingCommitState(mergeType.String()))
			m.pendingCommitSession = sess.ID
			m.pendingCommitType = mergeType
			return m, tea.Batch(m.generateCommitMessage(sess.ID, sess.WorkTree), ui.StopwatchTick())
		}

		// No changes - proceed directly with merge/PR/push
		// Finish any existing streaming before starting merge operation
		m.chat.FinishStreaming()
		mergeCtx, cancel := context.WithCancel(context.Background())
		switch mergeType {
		case MergeTypePR:
			log.Info("creating PR (no uncommitted changes)")
			m.chat.AppendStreaming("Creating PR for " + sess.Branch + "...\n\n")
			m.sessionState().StartMerge(sess.ID, m.gitService.CreatePR(mergeCtx, sess.RepoPath, sess.WorkTree, sess.Branch, "", sess.IssueNumber), cancel, MergeTypePR)
		case MergeTypePush:
			log.Info("pushing updates (no uncommitted changes)")
			m.chat.AppendStreaming("Pushing updates to " + sess.Branch + "...\n\n")
			m.sessionState().StartMerge(sess.ID, m.gitService.PushUpdates(mergeCtx, sess.RepoPath, sess.WorkTree, sess.Branch, ""), cancel, MergeTypePush)
		case MergeTypeParent:
			log.Info("merging to parent (no uncommitted changes)", "parentBranch", parentSess.Branch)
			m.chat.AppendStreaming("Merging " + sess.Branch + " to parent " + parentSess.Branch + "...\n\n")
			m.sessionState().StartMerge(sess.ID, m.gitService.MergeToParent(mergeCtx, sess.WorkTree, sess.Branch, parentSess.WorkTree, parentSess.Branch, ""), cancel, MergeTypeParent)
		default:
			log.Info("merging to main (no uncommitted changes)")
			m.chat.AppendStreaming("Merging " + sess.Branch + " to main...\n\n")
			m.sessionState().StartMerge(sess.ID, m.gitService.MergeToMain(mergeCtx, sess.RepoPath, sess.WorkTree, sess.Branch, ""), cancel, MergeTypeMerge)
		}
		return m, m.listenForMergeResult(sess.ID)
	}
	// Forward other keys to the modal for navigation handling
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// handleLoadingCommitModal handles key events for the Loading Commit modal.
func (m *Model) handleLoadingCommitModal(key string, _ tea.KeyPressMsg, _ *ui.LoadingCommitState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		// Cancel commit message generation
		m.modal.Hide()
		m.pendingCommitSession = ""
		m.pendingCommitType = MergeTypeNone
		m.chat.AppendStreaming("Cancelled.\n")
		return m, nil
	}
	// No other keys handled while loading
	return m, nil
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
		// Finish any existing streaming before starting merge operation
		m.chat.FinishStreaming()
		log := logger.WithSession(sess.ID)
		mergeCtx, cancel := context.WithCancel(context.Background())
		switch mergeType {
		case MergeTypePR:
			log.Info("creating PR with user-edited commit message")
			m.chat.AppendStreaming("Creating PR for " + sess.Branch + "...\n\n")
			m.sessionState().StartMerge(sess.ID, m.gitService.CreatePR(mergeCtx, sess.RepoPath, sess.WorkTree, sess.Branch, commitMsg, sess.IssueNumber), cancel, MergeTypePR)
		case MergeTypePush:
			log.Info("pushing updates with user-edited commit message")
			m.chat.AppendStreaming("Pushing updates to " + sess.Branch + "...\n\n")
			m.sessionState().StartMerge(sess.ID, m.gitService.PushUpdates(mergeCtx, sess.RepoPath, sess.WorkTree, sess.Branch, commitMsg), cancel, MergeTypePush)
		case MergeTypeParent:
			parentSess := m.config.GetSession(parentSessionID)
			if parentSess == nil {
				m.chat.AppendStreaming("Error: Parent session not found\n")
				cancel()
				return m, nil
			}
			log.Info("merging to parent with user-edited commit message", "parentBranch", parentSess.Branch)
			m.chat.AppendStreaming("Merging " + sess.Branch + " to parent " + parentSess.Branch + "...\n\n")
			m.sessionState().StartMerge(sess.ID, m.gitService.MergeToParent(mergeCtx, sess.WorkTree, sess.Branch, parentSess.WorkTree, parentSess.Branch, commitMsg), cancel, MergeTypeParent)
		default:
			log.Info("merging to main with user-edited commit message")
			m.chat.AppendStreaming("Merging " + sess.Branch + " to main...\n\n")
			m.sessionState().StartMerge(sess.ID, m.gitService.MergeToMain(mergeCtx, sess.RepoPath, sess.WorkTree, sess.Branch, commitMsg), cancel, MergeTypeMerge)
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

	logger.Get().Debug("committing conflict resolution", "repoPath", m.pendingConflictRepoPath)
	ctx := context.Background()
	err := m.gitService.CommitConflictResolution(ctx, m.pendingConflictRepoPath, commitMsg)
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
		logger.WithSession(m.pendingConflictSessionID).Info("marked session as merged after conflict resolution")
	}

	// Clear pending conflict state
	m.pendingConflictRepoPath = ""
	m.pendingConflictSessionID = ""

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
5. Stage the resolved files with git add
6. Commit the merge with a descriptive commit message explaining the resolution`, filesList.String())

	logger.WithSession(sess.ID).Debug("sending conflict resolution prompt to Claude")
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

	cmds := append(m.sessionListeners(sess.ID, runner, responseChan),
		ui.SidebarTick(),
		ui.StopwatchTick(),
	)
	return m, tea.Batch(cmds...)
}

// handleAbortMerge aborts the in-progress merge.
func (m *Model) handleAbortMerge(state *ui.MergeConflictState) (tea.Model, tea.Cmd) {
	ctx := context.Background()
	err := m.gitService.AbortMerge(ctx, state.RepoPath)
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
