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
	expected := "container build -t plural-claude ."
	if cmd := s.GetBuildCommand(); cmd != expected {
		t.Errorf("Expected build command %q, got %q", expected, cmd)
	}
}

func TestContainerBuildState_GetBuildCommand_CustomImage(t *testing.T) {
	s := NewContainerBuildState("my-image")
	expected := "container build -t my-image ."
	if cmd := s.GetBuildCommand(); cmd != expected {
		t.Errorf("Expected build command %q, got %q", expected, cmd)
	}
}

func TestContainerBuildState_Render_ShowsCommand(t *testing.T) {
	s := NewContainerBuildState("plural-claude")
	rendered := s.Render()

	if !strings.Contains(rendered, "container build -t plural-claude .") {
		t.Error("Rendered output should contain the build command")
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
