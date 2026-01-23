package modals

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// =============================================================================
// PreviewActiveState - State for the preview mode warning modal
// =============================================================================

// PreviewActiveState displays a warning when preview mode is active,
// reminding the user that the main repository is checked out to a session's branch.
type PreviewActiveState struct {
	SessionName string
	BranchName  string
}

func (*PreviewActiveState) modalState() {}

func (s *PreviewActiveState) Title() string { return "Preview Mode Active" }

func (s *PreviewActiveState) Help() string {
	return "p: end preview  Esc: dismiss"
}

func (s *PreviewActiveState) Render() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorWarning).
		MarginBottom(1).
		Render(s.Title())

	warningIcon := lipgloss.NewStyle().
		Foreground(ColorWarning).
		Bold(true).
		Render("⚠")

	message := lipgloss.NewStyle().
		Foreground(ColorText).
		Width(55).
		Render("You are previewing session \"" + s.SessionName + "\" in the main repository.")

	branchInfo := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginTop(1).
		Render("Branch: " + s.BranchName)

	warning := lipgloss.NewStyle().
		Foreground(ColorText).
		Width(55).
		MarginTop(1).
		Render("Avoid making changes in the main repo while preview is active.")

	instruction := lipgloss.NewStyle().
		Foreground(ColorWarning).
		MarginTop(1).
		Render("Press 'p' again to end preview and restore the previous branch.")

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Center, warningIcon, " ", title),
		message,
		branchInfo,
		warning,
		instruction,
		help,
	)
}

func (s *PreviewActiveState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	return s, nil
}

// NewPreviewActiveState creates a new PreviewActiveState with the session and branch info.
func NewPreviewActiveState(sessionName, branchName string) *PreviewActiveState {
	return &PreviewActiveState{
		SessionName: sessionName,
		BranchName:  branchName,
	}
}
