package modals

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"

	"charm.land/bubbles/v2/spinner"
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

// ValidateContainerImage checks if the given image name is safe.
func ValidateContainerImage(image string) bool {
	return validContainerImage.MatchString(image)
}

// =============================================================================
// ContainerBuildingState - Shows progress while building a container image
// =============================================================================

// ContainerBuildingState displays a spinner and detected languages while
// a container image is being built in the background.
type ContainerBuildingState struct {
	Languages []string      // Display names of detected languages
	Spinner   spinner.Model // Animated spinner
}

func (*ContainerBuildingState) modalState() {}

func (s *ContainerBuildingState) Title() string { return "Building Container Image" }

func (s *ContainerBuildingState) Help() string {
	return "Esc: cancel"
}

func (s *ContainerBuildingState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	var langSection string
	if len(s.Languages) > 0 {
		langList := strings.Join(s.Languages, ", ")
		langSection = lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Width(55).
			MarginBottom(1).
			Render(fmt.Sprintf("Detected: %s", langList))
	}

	spinnerLine := s.Spinner.View() + " " + lipgloss.NewStyle().
		Foreground(ColorText).
		Render("Building image (this may take a few minutes)...")

	help := ModalHelpStyle.Render(s.Help())

	parts := []string{title}
	if langSection != "" {
		parts = append(parts, langSection)
	}
	parts = append(parts, "", spinnerLine, "", help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (s *ContainerBuildingState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	return s, nil
}

// AdvanceSpinner updates the spinner by forwarding a tick message.
func (s *ContainerBuildingState) AdvanceSpinner(msg spinner.TickMsg) tea.Cmd {
	var cmd tea.Cmd
	s.Spinner, cmd = s.Spinner.Update(msg)
	return cmd
}

// NewContainerBuildingState creates a new ContainerBuildingState with the given language names.
func NewContainerBuildingState(languages []string) *ContainerBuildingState {
	sp := spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(ColorUser).Bold(true)),
	)
	return &ContainerBuildingState{
		Languages: languages,
		Spinner:   sp,
	}
}
