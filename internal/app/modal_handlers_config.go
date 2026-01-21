package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/ui"
)

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
	case "escape":
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
	case "esc":
		m.showPluginsModal() // Go back to plugins modal
		return m, nil
	case "enter":
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

// handleThemeModal handles key events for the Theme picker modal.
func (m *Model) handleThemeModal(key string, msg tea.KeyPressMsg, state *ui.ThemeState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.modal.Hide()
		return m, nil
	case "enter":
		selectedTheme := ui.GetSelectedThemeAsThemeName(state)
		ui.SetTheme(selectedTheme)
		m.config.SetTheme(string(selectedTheme))
		m.config.Save()
		m.chat.RefreshStyles()
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

// handleSettingsModal handles key events for the Settings modal.
func (m *Model) handleSettingsModal(key string, msg tea.KeyPressMsg, state *ui.SettingsState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.modal.Hide()
		return m, nil
	case "enter":
		// Save all settings
		branchPrefix := state.GetBranchPrefix()
		m.config.SetDefaultBranchPrefix(branchPrefix)
		m.config.SetNotificationsEnabled(state.GetNotificationsEnabled())
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
