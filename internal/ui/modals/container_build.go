package modals

import (
	"regexp"
	"runtime"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// validContainerImage matches valid container image names: lowercase alphanumeric,
// dots, hyphens, underscores, slashes (for namespaced images), and colons (for tags).
var validContainerImage = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._\-/:]*$`)

// =============================================================================
// ContainerCommandState - Generic "run this command" modal
// =============================================================================

// ContainerCommandState shows the user a command to run, with copy-to-clipboard support.
// Used for CLI not installed, system not running, etc.
type ContainerCommandState struct {
	ModalTitle string // e.g., "Container CLI Not Found"
	Message    string // Explanatory text
	Command    string // The command to display and copy
	Copied     bool   // Whether the command was copied to clipboard
}

func (*ContainerCommandState) modalState() {}

func (s *ContainerCommandState) Title() string { return s.ModalTitle }

func (s *ContainerCommandState) Help() string {
	if s.Copied {
		return "Copied! Press Esc to dismiss"
	}
	return "Enter: copy to clipboard  Esc: dismiss"
}

func (s *ContainerCommandState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	message := lipgloss.NewStyle().
		Foreground(ColorText).
		Width(55).
		MarginBottom(1).
		Render(s.Message)

	cmdStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Background(lipgloss.Color("#1a1a2e")).
		Padding(0, 1).
		MarginBottom(1)

	cmd := cmdStyle.Render(s.Command)

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

func (s *ContainerCommandState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	return s, nil
}

// GetCommand returns the command for clipboard copying.
func (s *ContainerCommandState) GetCommand() string {
	return s.Command
}

// NewContainerCLINotInstalledState creates a modal for when Docker is not installed.
// Shows a platform-appropriate install command or URL.
func NewContainerCLINotInstalledState() *ContainerCommandState {
	switch runtime.GOOS {
	case "darwin":
		return &ContainerCommandState{
			ModalTitle: "Docker Not Found",
			Message:    "Docker is required for container mode. Install Docker Desktop with Homebrew:",
			Command:    "brew install --cask docker",
		}
	case "linux":
		return &ContainerCommandState{
			ModalTitle: "Docker Not Found",
			Message:    "Docker is required for container mode. Install via https://docs.docker.com/engine/install/ — for Debian/Ubuntu:",
			Command:    "sudo apt-get install docker-ce docker-ce-cli containerd.io",
		}
	default:
		return &ContainerCommandState{
			ModalTitle: "Docker Not Found",
			Message:    "Docker is required for container mode. Visit https://docs.docker.com/get-docker/ to install.",
			Command:    "https://docs.docker.com/get-docker/",
		}
	}
}

// NewContainerSystemNotRunningState creates a modal for when the Docker daemon is not running.
// Shows a platform-appropriate start command.
func NewContainerSystemNotRunningState() *ContainerCommandState {
	switch runtime.GOOS {
	case "darwin":
		return &ContainerCommandState{
			ModalTitle: "Docker Not Running",
			Message:    "The Docker daemon is not running. Start Docker Desktop:",
			Command:    "open -a Docker",
		}
	case "linux":
		return &ContainerCommandState{
			ModalTitle: "Docker Not Running",
			Message:    "The Docker daemon is not running. Start it with:",
			Command:    "sudo systemctl start docker",
		}
	default:
		return &ContainerCommandState{
			ModalTitle: "Docker Not Running",
			Message:    "The Docker daemon is not running. Start Docker Desktop from your applications.",
			Command:    "docker info",
		}
	}
}

// =============================================================================
// ContainerBuildState - Container image not found
// =============================================================================

// ContainerBuildState shows the user how to pull the container image.
type ContainerBuildState struct {
	Image  string // Image name (e.g., "ghcr.io/zhubert/plural-claude")
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
		Render("The container image '" + s.Image + "' was not found. Pull it with:")

	cmdStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Background(lipgloss.Color("#1a1a2e")).
		Padding(0, 1).
		MarginBottom(1)

	cmd := cmdStyle.Render("docker pull " + s.Image)

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

// GetPullCommand returns the pull command for clipboard copying.
// The image name is validated before inclusion to prevent shell injection.
func (s *ContainerBuildState) GetPullCommand() string {
	image := s.Image
	if !validContainerImage.MatchString(image) {
		image = "ghcr.io/zhubert/plural-claude" // fall back to default for invalid names
	}
	return "docker pull " + image
}

// ValidateContainerImage checks if the given image name is safe.
func ValidateContainerImage(image string) bool {
	return validContainerImage.MatchString(image)
}

// NewContainerBuildState creates a new ContainerBuildState.
// Invalid image names are replaced with the default to prevent shell injection.
func NewContainerBuildState(image string) *ContainerBuildState {
	if !validContainerImage.MatchString(image) {
		image = "ghcr.io/zhubert/plural-claude"
	}
	return &ContainerBuildState{
		Image: image,
	}
}
