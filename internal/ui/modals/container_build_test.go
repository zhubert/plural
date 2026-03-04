package modals

import (
	"runtime"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
)

// =============================================================================
// ContainerCommandState tests (CLI not installed, system not running)
// =============================================================================

func TestContainerCommandState_CLINotInstalled(t *testing.T) {
	s := NewContainerCLINotInstalledState()

	if s.Title() != "Docker Not Found" {
		t.Errorf("Expected title 'Docker Not Found', got %q", s.Title())
	}

	// Platform-specific: macOS gets brew command, Linux gets install script, others get URL
	switch runtime.GOOS {
	case "darwin":
		if s.GetCommand() != "brew install --cask docker" {
			t.Errorf("Expected brew install command on macOS, got %q", s.GetCommand())
		}
	case "linux":
		if s.GetCommand() != "sudo apt-get install docker-ce docker-ce-cli containerd.io" {
			t.Errorf("Expected Docker apt install command on Linux, got %q", s.GetCommand())
		}
	default:
		if s.GetCommand() != "https://docs.docker.com/get-docker/" {
			t.Errorf("Expected Docker install URL, got %q", s.GetCommand())
		}
	}

	rendered := s.Render()
	if !strings.Contains(rendered, "Docker is required") {
		t.Error("Rendered output should explain that Docker is required")
	}
}

func TestContainerCommandState_SystemNotRunning(t *testing.T) {
	s := NewContainerSystemNotRunningState()

	if s.Title() != "Docker Not Running" {
		t.Errorf("Expected title 'Docker Not Running', got %q", s.Title())
	}

	// Platform-specific: macOS gets open -a Docker, Linux gets systemctl, others get docker info
	switch runtime.GOOS {
	case "darwin":
		if s.GetCommand() != "open -a Docker" {
			t.Errorf("Expected 'open -a Docker' on macOS, got %q", s.GetCommand())
		}
	case "linux":
		if s.GetCommand() != "sudo systemctl start docker" {
			t.Errorf("Expected 'sudo systemctl start docker' on Linux, got %q", s.GetCommand())
		}
	default:
		if s.GetCommand() != "docker info" {
			t.Errorf("Expected 'docker info' on other platforms, got %q", s.GetCommand())
		}
	}

	rendered := s.Render()
	if !strings.Contains(rendered, "not running") {
		t.Error("Rendered output should explain that Docker is not running")
	}
}

func TestContainerCommandState_Help_BeforeCopy(t *testing.T) {
	s := NewContainerCLINotInstalledState()
	help := s.Help()

	if !strings.Contains(help, "copy to clipboard") {
		t.Errorf("Help before copy should mention clipboard, got %q", help)
	}
}

func TestContainerCommandState_Help_AfterCopy(t *testing.T) {
	s := NewContainerCLINotInstalledState()
	s.Copied = true
	help := s.Help()

	if !strings.Contains(help, "Copied") {
		t.Errorf("Help after copy should say Copied, got %q", help)
	}
}

func TestContainerCommandState_Render_ShowsCopiedMessage(t *testing.T) {
	s := NewContainerSystemNotRunningState()
	s.Copied = true
	rendered := s.Render()

	if !strings.Contains(rendered, "Copied to clipboard") {
		t.Error("Rendered output should show 'Copied to clipboard' after copy")
	}
}

// =============================================================================
// ValidateContainerImage tests
// =============================================================================

func TestValidateContainerImage(t *testing.T) {
	tests := []struct {
		name  string
		image string
		valid bool
	}{
		{"valid simple name", "plural-claude", true},
		{"valid with dots", "my.image.name", true},
		{"valid with underscores", "my_image", true},
		{"valid with tag", "plural-claude:latest", true},
		{"valid with namespace", "registry/image:v1", true},
		{"valid ghcr.io image", "ghcr.io/zhubert/plural-claude", true},
		{"valid ghcr.io with tag", "ghcr.io/zhubert/plural-claude:v1.2.3", true},
		{"valid uppercase", "MyImage", true},
		{"empty string", "", false},
		{"shell injection semicolon", "image; rm -rf /", false},
		{"shell injection backtick", "image`whoami`", false},
		{"shell injection dollar", "image$(whoami)", false},
		{"shell injection pipe", "image|cat /etc/passwd", false},
		{"starts with hyphen", "-image", false},
		{"starts with dot", ".image", false},
		{"spaces", "my image", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidateContainerImage(tt.image); got != tt.valid {
				t.Errorf("ValidateContainerImage(%q) = %v, want %v", tt.image, got, tt.valid)
			}
		})
	}
}

// =============================================================================
// ContainerBuildingState tests
// =============================================================================

func TestContainerBuildingState_New(t *testing.T) {
	langs := []string{"go", "python"}
	state := NewContainerBuildingState(langs)

	if len(state.Languages) != 2 {
		t.Errorf("expected 2 languages, got %d", len(state.Languages))
	}
	if state.Languages[0] != "go" || state.Languages[1] != "python" {
		t.Errorf("unexpected languages: %v", state.Languages)
	}
	// Spinner should be initialized with MiniDot frames
	if len(state.Spinner.Spinner.Frames) != len(spinner.MiniDot.Frames) {
		t.Errorf("expected MiniDot spinner, got %d frames", len(state.Spinner.Spinner.Frames))
	}
}

func TestContainerBuildingState_Title(t *testing.T) {
	state := NewContainerBuildingState(nil)
	if state.Title() != "Building Container Image" {
		t.Errorf("unexpected title: %q", state.Title())
	}
}

func TestContainerBuildingState_Help(t *testing.T) {
	state := NewContainerBuildingState(nil)
	help := state.Help()
	if !strings.Contains(help, "Esc") {
		t.Errorf("help should mention Esc key: %q", help)
	}
}

func TestContainerBuildingState_Render_WithLanguages(t *testing.T) {
	state := NewContainerBuildingState([]string{"go", "python"})
	rendered := state.Render()

	if !strings.Contains(rendered, "Building Container Image") {
		t.Error("rendered should contain title")
	}
	if !strings.Contains(rendered, "go, python") {
		t.Error("rendered should list detected languages")
	}
	if !strings.Contains(rendered, "Building image") {
		t.Error("rendered should contain building message")
	}
}

func TestContainerBuildingState_Render_NoLanguages(t *testing.T) {
	state := NewContainerBuildingState(nil)
	rendered := state.Render()

	if !strings.Contains(rendered, "Building image") {
		t.Error("rendered should contain building message even with no languages")
	}
	// Should NOT contain "Detected:" when no languages
	if strings.Contains(rendered, "Detected:") {
		t.Error("rendered should not show 'Detected:' when no languages provided")
	}
}

func TestContainerBuildingState_Update(t *testing.T) {
	state := NewContainerBuildingState([]string{"go"})
	newState, cmd := state.Update(nil)
	if newState != state {
		t.Error("update should return same state")
	}
	if cmd != nil {
		t.Error("update should return nil cmd")
	}
}

func TestContainerBuildingState_AdvanceSpinner(t *testing.T) {
	state := NewContainerBuildingState([]string{"go"})
	initialView := state.Spinner.View()
	if initialView == "" {
		t.Error("expected non-empty spinner view")
	}
	// AdvanceSpinner should accept a tick and return a command
	tick := spinner.TickMsg{ID: state.Spinner.ID()}
	cmd := state.AdvanceSpinner(tick)
	// The command may be nil (ID mismatch) or non-nil, both are valid
	_ = cmd
}

func TestContainerBuildingState_EmptyLanguages(t *testing.T) {
	state := NewContainerBuildingState([]string{})
	if len(state.Languages) != 0 {
		t.Errorf("expected 0 languages, got %d", len(state.Languages))
	}
}
