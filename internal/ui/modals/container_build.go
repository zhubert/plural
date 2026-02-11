package modals

import (
	"regexp"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// validContainerImage matches valid container image names: lowercase alphanumeric,
// dots, hyphens, underscores, slashes (for namespaced images), and colons (for tags).
var validContainerImage = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._\-/:]*$`)

// =============================================================================
// ContainerCLINotInstalledState - Container CLI not found
// =============================================================================

// ContainerCLINotInstalledState shows the user how to install the container CLI.
type ContainerCLINotInstalledState struct {
	Copied bool // Whether the command was copied to clipboard
}

func (*ContainerCLINotInstalledState) modalState() {}

func (s *ContainerCLINotInstalledState) Title() string { return "Container CLI Not Found" }

func (s *ContainerCLINotInstalledState) Help() string {
	if s.Copied {
		return "Copied! Press Esc to dismiss"
	}
	return "Enter: copy to clipboard  Esc: dismiss"
}

func (s *ContainerCLINotInstalledState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	message := lipgloss.NewStyle().
		Foreground(ColorText).
		Width(55).
		MarginBottom(1).
		Render("Apple's container CLI is required for container mode. Install it with Homebrew:")

	cmdStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Background(lipgloss.Color("#1a1a2e")).
		Padding(0, 1).
		MarginBottom(1)

	cmd := cmdStyle.Render("brew install container")

	var statusView string
	if s.Copied {
		statusView = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			Render("Copied to clipboard!")
	}

	help := ModalHelpStyle.Render(s.Help())

	parts := []string{title, message, cmd}
	if statusView != "" {
		parts = append(parts, statusView)
	}
	parts = append(parts, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (s *ContainerCLINotInstalledState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	return s, nil
}

// GetCommand returns the install command for clipboard copying.
func (s *ContainerCLINotInstalledState) GetCommand() string {
	return "brew install container"
}

// =============================================================================
// ContainerSystemNotRunningState - Container system not running
// =============================================================================

// ContainerSystemNotRunningState shows the user how to start the container system.
type ContainerSystemNotRunningState struct {
	Copied bool // Whether the command was copied to clipboard
}

func (*ContainerSystemNotRunningState) modalState() {}

func (s *ContainerSystemNotRunningState) Title() string { return "Container System Not Running" }

func (s *ContainerSystemNotRunningState) Help() string {
	if s.Copied {
		return "Copied! Press Esc to dismiss"
	}
	return "Enter: copy to clipboard  Esc: dismiss"
}

func (s *ContainerSystemNotRunningState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	message := lipgloss.NewStyle().
		Foreground(ColorText).
		Width(55).
		MarginBottom(1).
		Render("The container system service is not running. Start it with:")

	cmdStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Background(lipgloss.Color("#1a1a2e")).
		Padding(0, 1).
		MarginBottom(1)

	cmd := cmdStyle.Render("container system start")

	var statusView string
	if s.Copied {
		statusView = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			Render("Copied to clipboard!")
	}

	help := ModalHelpStyle.Render(s.Help())

	parts := []string{title, message, cmd}
	if statusView != "" {
		parts = append(parts, statusView)
	}
	parts = append(parts, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (s *ContainerSystemNotRunningState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	return s, nil
}

// GetCommand returns the start command for clipboard copying.
func (s *ContainerSystemNotRunningState) GetCommand() string {
	return "container system start"
}

// =============================================================================
// ContainerBuildState - Container image not built
// =============================================================================

// ContainerBuildState shows the user how to build the container image.
type ContainerBuildState struct {
	Image  string // Image name (e.g., "plural-claude")
	Copied bool   // Whether the command was copied to clipboard
}

func (*ContainerBuildState) modalState() {}

func (s *ContainerBuildState) Title() string { return "Container Image Not Built" }

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
		Render("The container image '" + s.Image + "' was not found. Build it from the Plural repo root:")

	cmdStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Background(lipgloss.Color("#1a1a2e")).
		Padding(0, 1).
		MarginBottom(1)

	cmd := cmdStyle.Render("container build -t " + s.Image + " .")

	var statusView string
	if s.Copied {
		statusView = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			Render("Copied to clipboard!")
	}

	help := ModalHelpStyle.Render(s.Help())

	parts := []string{title, message, cmd}
	if statusView != "" {
		parts = append(parts, statusView)
	}
	parts = append(parts, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (s *ContainerBuildState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	return s, nil
}

// GetBuildCommand returns the build command for clipboard copying.
// The image name is validated before inclusion to prevent shell injection.
func (s *ContainerBuildState) GetBuildCommand() string {
	image := s.Image
	if !validContainerImage.MatchString(image) {
		image = "plural-claude" // fall back to default for invalid names
	}
	return "container build -t " + image + " ."
}

// ValidateContainerImage checks if the given image name is safe.
func ValidateContainerImage(image string) bool {
	return validContainerImage.MatchString(image)
}

// NewContainerBuildState creates a new ContainerBuildState.
// Invalid image names are replaced with the default to prevent shell injection.
func NewContainerBuildState(image string) *ContainerBuildState {
	if !validContainerImage.MatchString(image) {
		image = "plural-claude"
	}
	return &ContainerBuildState{
		Image: image,
	}
}
