package modals

import (
	"strings"
	"testing"
)

func TestContainerBuildState_Title(t *testing.T) {
	s := NewContainerBuildState("plural-claude")
	if s.Title() != "Container Image Not Found" {
		t.Errorf("Expected title 'Container Image Not Found', got %q", s.Title())
	}
}

func TestContainerBuildState_GetBuildCommand(t *testing.T) {
	s := NewContainerBuildState("plural-claude")
	expected := "brew install container && container system start && container build -t plural-claude ."
	if cmd := s.GetBuildCommand(); cmd != expected {
		t.Errorf("Expected build command %q, got %q", expected, cmd)
	}
}

func TestContainerBuildState_GetBuildCommand_CustomImage(t *testing.T) {
	s := NewContainerBuildState("my-image")
	expected := "brew install container && container system start && container build -t my-image ."
	if cmd := s.GetBuildCommand(); cmd != expected {
		t.Errorf("Expected build command %q, got %q", expected, cmd)
	}
}

func TestContainerBuildState_Render_ShowsBuildCommand(t *testing.T) {
	s := NewContainerBuildState("plural-claude")
	rendered := s.Render()

	if !strings.Contains(rendered, "container build -t plural-claude .") {
		t.Error("Rendered output should contain the build command")
	}
}

func TestContainerBuildState_Render_ShowsBrewInstall(t *testing.T) {
	s := NewContainerBuildState("plural-claude")
	rendered := s.Render()

	if !strings.Contains(rendered, "brew install container") {
		t.Error("Rendered output should contain the brew install command")
	}
}

func TestContainerBuildState_Render_ShowsSystemStart(t *testing.T) {
	s := NewContainerBuildState("plural-claude")
	rendered := s.Render()

	if !strings.Contains(rendered, "container system start") {
		t.Error("Rendered output should contain the system start command")
	}
}

func TestContainerBuildState_Render_ShowsImageName(t *testing.T) {
	s := NewContainerBuildState("plural-claude")
	rendered := s.Render()

	if !strings.Contains(rendered, "plural-claude") {
		t.Error("Rendered output should contain the image name")
	}
}

func TestContainerBuildState_Help_BeforeCopy(t *testing.T) {
	s := NewContainerBuildState("plural-claude")
	help := s.Help()

	if !strings.Contains(help, "copy to clipboard") {
		t.Errorf("Help before copy should mention clipboard, got %q", help)
	}
}

func TestContainerBuildState_Help_AfterCopy(t *testing.T) {
	s := NewContainerBuildState("plural-claude")
	s.Copied = true
	help := s.Help()

	if !strings.Contains(help, "Copied") {
		t.Errorf("Help after copy should say Copied, got %q", help)
	}
}

func TestContainerBuildState_Render_ShowsCopiedMessage(t *testing.T) {
	s := NewContainerBuildState("plural-claude")
	s.Copied = true
	rendered := s.Render()

	if !strings.Contains(rendered, "Copied to clipboard") {
		t.Error("Rendered output should show 'Copied to clipboard' after copy")
	}
}

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
	if s.Image != "plural-claude" {
		t.Errorf("Invalid image name should be replaced with default, got %q", s.Image)
	}
}

func TestGetBuildCommand_SanitizesInvalidImage(t *testing.T) {
	s := &ContainerBuildState{Image: "; rm -rf /"}
	cmd := s.GetBuildCommand()
	if strings.Contains(cmd, "rm -rf") {
		t.Error("GetBuildCommand should not include shell injection in output")
	}
	if !strings.Contains(cmd, "plural-claude") {
		t.Error("GetBuildCommand should fall back to default image for invalid names")
	}
}
