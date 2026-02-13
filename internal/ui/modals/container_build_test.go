package modals

import (
	"runtime"
	"strings"
	"testing"
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
		if s.GetCommand() != "curl -fsSL https://get.docker.com | sh" {
			t.Errorf("Expected Docker install script on Linux, got %q", s.GetCommand())
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
// ContainerBuildState tests
// =============================================================================

func TestContainerBuildState_Title(t *testing.T) {
	s := NewContainerBuildState("ghcr.io/zhubert/plural-claude")
	if s.Title() != "Container Image Not Found" {
		t.Errorf("Expected title 'Container Image Not Found', got %q", s.Title())
	}
}

func TestContainerBuildState_GetPullCommand(t *testing.T) {
	s := NewContainerBuildState("ghcr.io/zhubert/plural-claude")
	expected := "docker pull ghcr.io/zhubert/plural-claude"
	if cmd := s.GetPullCommand(); cmd != expected {
		t.Errorf("Expected pull command %q, got %q", expected, cmd)
	}
}

func TestContainerBuildState_GetPullCommand_CustomImage(t *testing.T) {
	s := NewContainerBuildState("my-image")
	expected := "docker pull my-image"
	if cmd := s.GetPullCommand(); cmd != expected {
		t.Errorf("Expected pull command %q, got %q", expected, cmd)
	}
}

func TestContainerBuildState_Render_ShowsPullCommand(t *testing.T) {
	s := NewContainerBuildState("ghcr.io/zhubert/plural-claude")
	rendered := s.Render()

	if !strings.Contains(rendered, "docker pull ghcr.io/zhubert/plural-claude") {
		t.Error("Rendered output should contain the pull command")
	}
}

func TestContainerBuildState_Render_DoesNotShowBrewInstall(t *testing.T) {
	s := NewContainerBuildState("ghcr.io/zhubert/plural-claude")
	rendered := s.Render()

	if strings.Contains(rendered, "brew install") {
		t.Error("Pull modal should not contain brew install command")
	}
}

func TestContainerBuildState_Render_DoesNotShowSystemStart(t *testing.T) {
	s := NewContainerBuildState("ghcr.io/zhubert/plural-claude")
	rendered := s.Render()

	if strings.Contains(rendered, "systemctl start") {
		t.Error("Pull modal should not contain systemctl start command")
	}
}

func TestContainerBuildState_Help_BeforeCopy(t *testing.T) {
	s := NewContainerBuildState("ghcr.io/zhubert/plural-claude")
	help := s.Help()

	if !strings.Contains(help, "copy to clipboard") {
		t.Errorf("Help before copy should mention clipboard, got %q", help)
	}
}

func TestContainerBuildState_Help_AfterCopy(t *testing.T) {
	s := NewContainerBuildState("ghcr.io/zhubert/plural-claude")
	s.Copied = true
	help := s.Help()

	if !strings.Contains(help, "Copied") {
		t.Errorf("Help after copy should say Copied, got %q", help)
	}
}

func TestContainerBuildState_Render_ShowsCopiedMessage(t *testing.T) {
	s := NewContainerBuildState("ghcr.io/zhubert/plural-claude")
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

func TestNewContainerBuildState_SanitizesInvalidImage(t *testing.T) {
	s := NewContainerBuildState("; rm -rf /")
	if s.Image != "ghcr.io/zhubert/plural-claude" {
		t.Errorf("Invalid image name should be replaced with default, got %q", s.Image)
	}
}

func TestGetPullCommand_SanitizesInvalidImage(t *testing.T) {
	s := &ContainerBuildState{Image: "; rm -rf /"}
	cmd := s.GetPullCommand()
	if strings.Contains(cmd, "rm -rf") {
		t.Error("GetPullCommand should not include shell injection in output")
	}
	if !strings.Contains(cmd, "ghcr.io/zhubert/plural-claude") {
		t.Error("GetPullCommand should fall back to default image for invalid names")
	}
}
