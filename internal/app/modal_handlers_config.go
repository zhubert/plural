package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/google/uuid"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/clipboard"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/keys"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/ui"
)

// handleMCPServersModal handles key events for the MCP Servers modal.
func (m *Model) handleMCPServersModal(key string, msg tea.KeyPressMsg, state *ui.MCPServersState) (tea.Model, tea.Cmd) {
	switch key {
	case keys.Escape:
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
			if err := m.config.Save(); err != nil {
				logger.Get().Error("failed to save config after MCP server deletion", "error", err)
				m.modal.Hide()
				return m, m.ShowFlashError("Failed to save configuration")
			}
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
	case keys.Escape:
		m.showMCPServersModal() // Go back to list
		return m, nil
	case keys.Enter:
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
		if err := m.config.Save(); err != nil {
			logger.Get().Error("failed to save MCP server config", "error", err)
			m.modal.Hide()
			return m, m.ShowFlashError("Failed to save MCP server configuration")
		}
		m.modal.Hide()
		return m, nil
	}
	// Forward other keys to the modal for text input handling
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// handlePluginsModal handles key events for the Plugins modal.
func (m *Model) handlePluginsModal(key string, msg tea.KeyPressMsg, state *ui.PluginsState) (tea.Model, tea.Cmd) {
	// Store current tab to preserve it after refresh
	currentTab := state.ActiveTab

	switch key {
	case keys.Escape:
		// If search is focused, let the modal handle it (exits search mode)
		if state.SearchFocused {
			modal, cmd := m.modal.Update(msg)
			m.modal = modal
			return m, cmd
		}
		m.modal.Hide()
		return m, nil

	// Marketplaces tab actions
	case "a":
		if state.ActiveTab == 0 { // Marketplaces tab
			m.modal.Show(ui.NewAddMarketplaceState())
		}
		return m, nil

	case "d":
		if state.ActiveTab == 0 { // Marketplaces tab
			if mp := state.GetSelectedMarketplace(); mp != nil {
				if err := claude.RemoveMarketplace(mp.Name); err != nil {
					state.SetError(err.Error())
				} else {
					m.showPluginsModalOnTab(currentTab) // Refresh and stay on tab
				}
			}
		}
		return m, nil

	case "u":
		if state.ActiveTab == 0 { // Marketplaces - update
			if mp := state.GetSelectedMarketplace(); mp != nil {
				if err := claude.UpdateMarketplace(mp.Name); err != nil {
					state.SetError(err.Error())
				} else {
					m.showPluginsModalOnTab(currentTab) // Refresh and stay on tab
				}
			}
		} else if state.ActiveTab == 1 { // Installed - uninstall
			if plugin := state.GetSelectedInstalledPlugin(); plugin != nil {
				if err := claude.UninstallPlugin(plugin.FullName); err != nil {
					state.SetError(err.Error())
				} else {
					m.showPluginsModalOnTab(currentTab) // Refresh and stay on tab
				}
			}
		}
		return m, nil

	case "e":
		if state.ActiveTab == 1 { // Installed - enable/disable
			if plugin := state.GetSelectedInstalledPlugin(); plugin != nil {
				var err error
				if plugin.Status == "enabled" {
					err = claude.DisablePlugin(plugin.FullName)
				} else {
					err = claude.EnablePlugin(plugin.FullName)
				}
				if err != nil {
					state.SetError(err.Error())
				} else {
					m.showPluginsModalOnTab(currentTab) // Refresh and stay on tab
				}
			}
		}
		return m, nil

	case "i", "enter":
		if state.ActiveTab == 2 { // Discover - install
			if plugin := state.GetSelectedAvailablePlugin(); plugin != nil {
				if err := claude.InstallPlugin(plugin.FullName); err != nil {
					state.SetError(err.Error())
				} else {
					m.showPluginsModalOnTab(1) // Go to Installed tab after install
				}
			}
		}
		return m, nil
	}

	// Forward other keys to the modal for navigation handling
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// handleAddMarketplaceModal handles key events for the Add Marketplace modal.
func (m *Model) handleAddMarketplaceModal(key string, msg tea.KeyPressMsg, state *ui.AddMarketplaceState) (tea.Model, tea.Cmd) {
	switch key {
	case keys.Escape:
		m.showPluginsModal() // Go back to plugins modal
		return m, nil
	case keys.Enter:
		source := state.GetValue()
		if source == "" {
			return m, nil
		}
		if err := claude.AddMarketplace(source); err != nil {
			m.modal.SetError(err.Error())
		} else {
			m.showPluginsModal() // Return to plugins modal and refresh
		}
		return m, nil
	}
	// Forward other keys to the modal for text input handling
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// showWorkspaceListModal opens the workspace list modal with current data.
func (m *Model) showWorkspaceListModal() {
	workspaces := m.config.GetWorkspaces()
	activeWS := m.config.GetActiveWorkspaceID()

	// Count sessions per workspace
	sessions := m.config.GetSessions()
	counts := make(map[string]int)
	for _, s := range sessions {
		if s.WorkspaceID != "" {
			counts[s.WorkspaceID]++
		}
	}

	m.modal.Show(ui.NewWorkspaceListState(workspaces, counts, activeWS))
}

// handleWorkspaceListModal handles key events for the Workspace List modal.
func (m *Model) handleWorkspaceListModal(key string, msg tea.KeyPressMsg, state *ui.WorkspaceListState) (tea.Model, tea.Cmd) {
	switch key {
	case keys.Escape:
		m.modal.Hide()
		return m, nil
	case keys.Enter:
		// Switch active workspace
		selectedID := state.GetSelectedWorkspaceID()
		m.config.SetActiveWorkspaceID(selectedID)
		if err := m.config.Save(); err != nil {
			logger.Get().Error("failed to save workspace selection", "error", err)
		}
		m.sidebar.SetSessions(m.getFilteredSessions())
		m.header.SetWorkspaceName(m.getActiveWorkspaceName())
		m.modal.Hide()
		return m, nil
	case "n":
		// Create new workspace
		m.modal.Show(ui.NewNewWorkspaceState())
		return m, nil
	case "d":
		// Delete selected workspace (not "All Sessions")
		if !state.IsAllSessionsSelected() {
			wsID := state.GetSelectedWorkspaceID()
			if wsID != "" {
				// Count affected sessions before deletion
				affectedSessions := m.config.GetSessionsByWorkspace(wsID)
				m.config.RemoveWorkspace(wsID)
				if err := m.config.Save(); err != nil {
					logger.Get().Error("failed to save after workspace deletion", "error", err)
				}
				m.sidebar.SetSessions(m.getFilteredSessions())
				m.header.SetWorkspaceName(m.getActiveWorkspaceName())
				m.showWorkspaceListModal() // Refresh
				if len(affectedSessions) > 0 {
					return m, m.ShowFlashInfo(fmt.Sprintf("%d session(s) moved to All Sessions", len(affectedSessions)))
				}
			}
		}
		return m, nil
	case "r":
		// Rename selected workspace (not "All Sessions")
		if !state.IsAllSessionsSelected() {
			wsID := state.GetSelectedWorkspaceID()
			wsName := state.GetSelectedWorkspaceName()
			if wsID != "" {
				m.modal.Show(ui.NewRenameWorkspaceState(wsID, wsName))
			}
		}
		return m, nil
	}
	// Forward navigation keys to modal
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// handleNewWorkspaceModal handles key events for the New/Rename Workspace modal.
func (m *Model) handleNewWorkspaceModal(key string, msg tea.KeyPressMsg, state *ui.NewWorkspaceState) (tea.Model, tea.Cmd) {
	switch key {
	case keys.Escape:
		m.showWorkspaceListModal() // Go back to list
		return m, nil
	case "enter":
		name := strings.TrimSpace(state.GetName())
		if name == "" {
			return m, nil
		}

		if state.IsRename {
			if !m.config.RenameWorkspace(state.WorkspaceID, name) {
				m.modal.SetError("Failed to rename workspace")
				return m, nil
			}
		} else {
			ws := config.Workspace{
				ID:   uuid.New().String(),
				Name: name,
			}
			if !m.config.AddWorkspace(ws) {
				m.modal.SetError("A workspace with that name already exists")
				return m, nil
			}
		}

		if err := m.config.Save(); err != nil {
			logger.Get().Error("failed to save workspace", "error", err)
			m.modal.SetError("Failed to save: " + err.Error())
			return m, nil
		}
		m.header.SetWorkspaceName(m.getActiveWorkspaceName())
		m.showWorkspaceListModal() // Return to list
		return m, nil
	}
	// Forward other keys for text input handling
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// handleSettingsModal handles key events for the Settings modal.
func (m *Model) handleSettingsModal(key string, msg tea.KeyPressMsg, state *ui.SettingsState) (tea.Model, tea.Cmd) {
	switch key {
	case keys.Escape:
		m.modal.Hide()
		return m, nil
	case keys.Enter:
		// Save all settings
		branchPrefix := state.GetBranchPrefix()
		m.config.SetDefaultBranchPrefix(branchPrefix)
		m.config.SetNotificationsEnabled(state.GetNotificationsEnabled())
		// Apply theme if changed
		if state.ThemeChanged() {
			selectedTheme := ui.GetSelectedSettingsTheme(state)
			ui.SetTheme(selectedTheme)
			m.config.SetTheme(string(selectedTheme))
			m.chat.RefreshStyles()
		}
		// Save per-repo settings for all repos
		for repo, gid := range state.GetAllAsanaProjects() {
			m.config.SetAsanaProject(repo, gid)
		}
		if err := m.config.Save(); err != nil {
			logger.Get().Error("failed to save settings", "error", err)
			m.modal.SetError("Failed to save: " + err.Error())
			return m, nil
		}
		m.modal.Hide()
		return m, nil
	}
	// Forward other keys to modal for text input handling
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

// handleContainerBuildModal handles key events for the Container Build modal.
func (m *Model) handleContainerBuildModal(key string, _ tea.KeyPressMsg, state *ui.ContainerBuildState) (tea.Model, tea.Cmd) {
	switch key {
	case keys.Escape:
		m.modal.Hide()
		return m, nil
	case keys.Enter:
		if err := clipboard.WriteText(state.GetBuildCommand()); err != nil {
			logger.Get().Error("failed to copy to clipboard", "error", err)
			return m, m.ShowFlashError("Failed to copy to clipboard")
		}
		state.Copied = true
		return m, nil
	}
	return m, nil
}
