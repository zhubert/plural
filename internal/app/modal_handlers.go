package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/ui"
)

// handleModalKey routes modal key events to the appropriate handler based on modal state type.
// This reduces the size of the main Update function by delegating modal handling.
//
// Modal handlers are organized by domain:
//   - modal_handlers_session.go: Session lifecycle (add repo, new/fork/rename/delete session)
//   - modal_handlers_git.go: Git operations (merge, commit, conflict resolution)
//   - modal_handlers_config.go: Configuration (MCP servers, plugins, themes, settings)
//   - modal_handlers_navigation.go: Navigation/info (help, welcome, changelog, search)
//   - modal_handlers_issues.go: Issue/task import (GitHub issues, Asana tasks, explore options)
func (m *Model) handleModalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch s := m.modal.State.(type) {
	// Session modals (modal_handlers_session.go)
	case *ui.AddRepoState:
		return m.handleAddRepoModal(key, msg, s)
	case *ui.NewSessionState:
		return m.handleNewSessionModal(key, msg, s)
	case *ui.ConfirmDeleteState:
		return m.handleConfirmDeleteModal(key, msg, s)
	case *ui.ConfirmDeleteRepoState:
		return m.handleConfirmDeleteRepoModal(key, msg, s)
	case *ui.ConfirmExitState:
		return m.handleConfirmExitModal(key, msg, s)
	case *ui.PreviewActiveState:
		return m.handlePreviewActiveModal(key, msg, s)
	case *ui.ForkSessionState:
		return m.handleForkSessionModal(key, msg, s)
	case *ui.RenameSessionState:
		return m.handleRenameSessionModal(key, msg, s)
	case *ui.SessionSettingsState:
		return m.handleSessionSettingsModal(key, msg, s)
	case *ui.BroadcastState:
		return m.handleBroadcastModal(key, msg, s)
	case *ui.BroadcastGroupState:
		return m.handleBroadcastGroupModal(key, msg, s)
	case *ui.BulkActionState:
		return m.handleBulkActionModal(key, msg, s)

	// Git modals (modal_handlers_git.go)
	case *ui.MergeState:
		return m.handleMergeModal(key, msg, s)
	case *ui.LoadingCommitState:
		return m.handleLoadingCommitModal(key, msg, s)
	case *ui.EditCommitState:
		return m.handleEditCommitModal(key, msg, s)
	case *ui.MergeConflictState:
		return m.handleMergeConflictModal(key, msg, s)
	case *ui.ReviewCommentsState:
		return m.handleReviewCommentsModal(key, msg, s)

	// Config modals (modal_handlers_config.go)
	case *ui.MCPServersState:
		return m.handleMCPServersModal(key, msg, s)
	case *ui.AddMCPServerState:
		return m.handleAddMCPServerModal(key, msg, s)
	case *ui.PluginsState:
		return m.handlePluginsModal(key, msg, s)
	case *ui.AddMarketplaceState:
		return m.handleAddMarketplaceModal(key, msg, s)
	case *ui.SettingsState:
		return m.handleSettingsModal(key, msg, s)
	case *ui.ContainerBuildState:
		return m.handleContainerBuildModal(key, msg, s)
	case *ui.ContainerCommandState:
		return m.handleContainerCommandModal(key, s)

	// Navigation modals (modal_handlers_navigation.go)
	case *ui.WelcomeState:
		return m.handleWelcomeModal(key, msg, s)
	case *ui.ChangelogState:
		return m.handleChangelogModal(key, msg, s)
	case *ui.HelpState:
		return m.handleHelpModal(key, msg, s)
	case *ui.SearchMessagesState:
		return m.handleSearchMessagesModal(key, msg, s)

	// Issue/task modals (modal_handlers_issues.go)
	case *ui.ExploreOptionsState:
		return m.handleExploreOptionsModal(key, msg, s)
	case *ui.SelectRepoForIssuesState:
		return m.handleSelectRepoForIssuesModal(key, msg, s)
	case *ui.SelectIssueSourceState:
		return m.handleSelectIssueSourceModal(key, msg, s)
	case *ui.ImportIssuesState:
		return m.handleImportIssuesModal(key, msg, s)
	}

	// Default: update modal input (for text-based modals)
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}
