package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/clipboard"
	"github.com/zhubert/plural-core/config"
	"github.com/zhubert/plural/internal/keys"
	"github.com/zhubert/plural-core/logger"
	"github.com/zhubert/plural/internal/plugins"
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
				if err := plugins.RemoveMarketplace(mp.Name); err != nil {
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
				if err := plugins.UpdateMarketplace(mp.Name); err != nil {
					state.SetError(err.Error())
				} else {
					m.showPluginsModalOnTab(currentTab) // Refresh and stay on tab
				}
			}
		} else if state.ActiveTab == 1 { // Installed - uninstall
			if plugin := state.GetSelectedInstalledPlugin(); plugin != nil {
				if err := plugins.UninstallPlugin(plugin.FullName); err != nil {
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
					err = plugins.DisablePlugin(plugin.FullName)
				} else {
					err = plugins.EnablePlugin(plugin.FullName)
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
				if err := plugins.InstallPlugin(plugin.FullName); err != nil {
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
		if err := plugins.AddMarketplace(source); err != nil {
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

// handleSettingsModal handles key events for the global Settings modal.
func (m *Model) handleSettingsModal(key string, msg tea.KeyPressMsg, state *ui.SettingsState) (tea.Model, tea.Cmd) {
	switch key {
	case keys.Escape:
		m.modal.Hide()
		return m, nil
	case keys.Enter:
		// Save all global settings
		m.config.SetDefaultBranchPrefix(state.GetBranchPrefix())
		m.config.SetNotificationsEnabled(state.GetNotificationsEnabled())
		m.config.SetAutoCleanupMerged(state.AutoCleanupMerged)
		// Save container image if containers are supported.
		if state.ContainersSupported {
			containerImage := state.GetContainerImage()
			if containerImage != "" && !ui.ValidateContainerImage(containerImage) {
				m.modal.SetError("Invalid container image name")
				return m, nil
			}
			m.config.SetContainerImage(containerImage)
		}
		// Apply theme if changed
		if state.ThemeChanged() {
			selectedTheme := ui.GetSelectedSettingsTheme(state)
			ui.SetTheme(selectedTheme)
			m.config.SetTheme(string(selectedTheme))
			m.chat.RefreshStyles()
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
	return m.handleCopyCommandModal(key, state.GetPullCommand, func() { state.Copied = true })
}

// handleContainerCommandModal handles key events for container command modals
// (CLI not installed, system not running).
func (m *Model) handleContainerCommandModal(key string, state *ui.ContainerCommandState) (tea.Model, tea.Cmd) {
	return m.handleCopyCommandModal(key, state.GetCommand, func() { state.Copied = true })
}

// handleCopyCommandModal is a shared handler for modals that copy a command to clipboard.
func (m *Model) handleCopyCommandModal(key string, getCmd func() string, setCopied func()) (tea.Model, tea.Cmd) {
	switch key {
	case keys.Escape:
		m.modal.Hide()
		return m, nil
	case keys.Enter:
		if err := clipboard.WriteText(getCmd()); err != nil {
			logger.Get().Error("failed to copy to clipboard", "error", err)
			return m, m.ShowFlashError("Failed to copy to clipboard")
		}
		setCopied()
		return m, nil
	}
	return m, nil
}
