package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/ui"
)

// ShowFlash displays a flash message in the footer and returns a command to start the auto-dismiss timer
func (m *Model) ShowFlash(text string, flashType ui.FlashType) tea.Cmd {
	m.footer.SetFlash(text, flashType)
	return ui.FlashTick()
}

// ShowFlashError displays an error flash message
func (m *Model) ShowFlashError(text string) tea.Cmd {
	return m.ShowFlash(text, ui.FlashError)
}

// ShowFlashWarning displays a warning flash message
func (m *Model) ShowFlashWarning(text string) tea.Cmd {
	return m.ShowFlash(text, ui.FlashWarning)
}

// ShowFlashInfo displays an info flash message
func (m *Model) ShowFlashInfo(text string) tea.Cmd {
	return m.ShowFlash(text, ui.FlashInfo)
}

// ShowFlashSuccess displays a success flash message
func (m *Model) ShowFlashSuccess(text string) tea.Cmd {
	return m.ShowFlash(text, ui.FlashSuccess)
}

// saveConfigOrFlash saves the config and shows a flash error if the save fails.
// It also logs the error for debugging. Returns a tea.Cmd (non-nil only on error).
func (m *Model) saveConfigOrFlash() tea.Cmd {
	if err := m.config.Save(); err != nil {
		logger.Get().Error("failed to save config", "error", err)
		return m.ShowFlashError("Failed to save configuration")
	}
	return nil
}
