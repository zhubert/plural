package modals

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ContainerBuildState shows the user how to build the container image.
type ContainerBuildState struct {
	Image  string // Image name (e.g., "plural-claude")
	Copied bool   // Whether the command was copied to clipboard
}

func (*ContainerBuildState) modalState() {}

func (s *ContainerBuildState) Title() string { return "Container Image Not Found" }

func (s *ContainerBuildState) Help() string {
	if s.Copied {
		return "Copied! Press Esc to dismiss"
	}
	return "Enter: copy to clipboard  Esc: dismiss"
}

func (s *ContainerBuildState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	message := lipgloss.NewStyle().
		Foreground(ColorText).
		Width(55).
		MarginBottom(1).
		Render("The container image '" + s.Image + "' was not found. Run these commands to set up containers:")

	cmdStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Background(lipgloss.Color("#1a1a2e")).
		Padding(0, 1)

	stepLabelStyle := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginTop(1)

	step1Label := stepLabelStyle.Render("1. Install Apple's container CLI:")
	step1Cmd := cmdStyle.Render("brew install container")

	step2Label := stepLabelStyle.Render("2. Build the image (from Plural repo root):")
	step2Cmd := cmdStyle.MarginBottom(1).Render("container build -t " + s.Image + " .")

	var statusView string
	if s.Copied {
		statusView = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			Render("Copied to clipboard!")
	}

	help := ModalHelpStyle.Render(s.Help())

	parts := []string{title, message, step1Label, step1Cmd, step2Label, step2Cmd}
	if statusView != "" {
		parts = append(parts, statusView)
	}
	parts = append(parts, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (s *ContainerBuildState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	return s, nil
}

// GetBuildCommand returns the setup commands for clipboard copying.
func (s *ContainerBuildState) GetBuildCommand() string {
	return "brew install container && container build -t " + s.Image + " ."
}

// NewContainerBuildState creates a new ContainerBuildState.
func NewContainerBuildState(image string) *ContainerBuildState {
	return &ContainerBuildState{
		Image: image,
	}
}
