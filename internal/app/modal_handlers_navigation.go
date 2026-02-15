package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/keys"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/ui"
)

// handleWelcomeModal handles key events for the Welcome modal.
func (m *Model) handleWelcomeModal(key string, msg tea.KeyPressMsg, state *ui.WelcomeState) (tea.Model, tea.Cmd) {
	switch key {
	case keys.Enter, keys.Escape:
		// Mark welcome as shown and save
		m.config.MarkWelcomeShown()
		if err := m.config.Save(); err != nil {
			logger.Get().Warn("failed to save welcome-shown flag", "error", err)
		}
		m.modal.Hide()
		// Check if we should also show changelog
		return m.handleStartupModals()
	}
	return m, nil
}

// handleChangelogModal handles key events for the Changelog modal.
func (m *Model) handleChangelogModal(key string, msg tea.KeyPressMsg, state *ui.ChangelogState) (tea.Model, tea.Cmd) {
	switch key {
	case keys.Enter, keys.Escape:
		// Update last seen version and save
		m.config.SetLastSeenVersion(m.version)
		if err := m.config.Save(); err != nil {
			logger.Get().Warn("failed to save last-seen version", "error", err)
		}
		m.modal.Hide()
		return m, nil
	case keys.Up, "k", keys.Down, "j":
		// Forward scroll keys to modal
		modal, cmd := m.modal.Update(msg)
		m.modal = modal
		return m, cmd
	}
	return m, nil
}

// handleHelpModal handles key events for the Help modal.
func (m *Model) handleHelpModal(key string, msg tea.KeyPressMsg, state *ui.HelpState) (tea.Model, tea.Cmd) {
	// While filtering, forward all keys to the list (Esc cancels filter, Enter applies)
	if state.IsFiltering() {
		modal, cmd := m.modal.Update(msg)
		m.modal = modal
		return m, cmd
	}

	switch key {
	case keys.Escape, "?", "q":
		m.modal.Hide()
		return m, nil
	case keys.Enter:
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
// It normalizes display keys and delegates to the shortcut registry.
func (m *Model) handleHelpShortcutTrigger(key string) (tea.Model, tea.Cmd) {
	// Normalize display keys to actual key values
	normalizedKey := normalizeHelpDisplayKey(key)
	if normalizedKey == "" {
		return m, nil // Display-only shortcut, no action
	}

	// Execute through the shortcut registry
	result, cmd, _ := m.ExecuteShortcut(normalizedKey)
	return result, cmd
}

// normalizeHelpDisplayKey converts help modal display keys to actual key values.
// Returns empty string for display-only shortcuts that shouldn't be executed.
func normalizeHelpDisplayKey(displayKey string) string {
	switch displayKey {
	// Display-only shortcuts (informational, no action)
	case "↑/↓ or j/k", "PgUp/PgDn", "Enter", "Esc":
		return ""
	// Chat-context only shortcuts (not executable from help modal)
	case "Ctrl+V", "Ctrl+P", "ctrl+/":
		return ""
	// Permission shortcuts (context-sensitive)
	case "y", "n", "a":
		return ""
	// Normalize capitalized display names
	case "Tab":
		return "tab"
	default:
		return strings.ToLower(displayKey)
	}
}

// handleSearchMessagesModal handles key events for the Search Messages modal.
func (m *Model) handleSearchMessagesModal(key string, msg tea.KeyPressMsg, state *ui.SearchMessagesState) (tea.Model, tea.Cmd) {
	switch key {
	case keys.Escape:
		m.modal.Hide()
		return m, nil
	case keys.Enter:
		// Go to the selected search result
		result := state.GetSelectedResult()
		if result != nil {
			// Close modal first
			m.modal.Hide()
			// Scroll to message - for now we just close the modal
			// Future enhancement: could scroll the chat viewport to the message
			logger.Get().Debug("search - selected message", "index", result.MessageIndex+1, "role", result.Role)
		}
		return m, nil
	}
	// Forward other keys to the modal for text input and navigation
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}
