package app

import (
	tea "charm.land/bubbletea/v2"
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
