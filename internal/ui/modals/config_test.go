package modals

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSettingsState_NumFields_NoRepo(t *testing.T) {
	s := NewSettingsState("", false, false, false, true, "", "")
	if n := s.numFields(); n != 2 {
		t.Errorf("Expected 2 fields with no repo, got %d", n)
	}
}

func TestSettingsState_NumFields_WithRepo_ContainersSupported(t *testing.T) {
	s := NewSettingsState("", false, false, false, true, "/some/repo", "")
	if n := s.numFields(); n != 5 {
		t.Errorf("Expected 5 fields with repo and containers supported, got %d", n)
	}
}

func TestSettingsState_NumFields_WithRepo_ContainersUnsupported(t *testing.T) {
	s := NewSettingsState("", false, false, false, false, "/some/repo", "")
	if n := s.numFields(); n != 4 {
		t.Errorf("Expected 4 fields with repo and containers unsupported, got %d", n)
	}
}

func TestSettingsState_AsanaFocusIndex_ContainersSupported(t *testing.T) {
	s := NewSettingsState("", false, false, false, true, "/some/repo", "")
	if idx := s.asanaFocusIndex(); idx != 4 {
		t.Errorf("Expected asana focus index 4 with containers supported, got %d", idx)
	}
}

func TestSettingsState_AsanaFocusIndex_ContainersUnsupported(t *testing.T) {
	s := NewSettingsState("", false, false, false, false, "/some/repo", "")
	if idx := s.asanaFocusIndex(); idx != 3 {
		t.Errorf("Expected asana focus index 3 with containers unsupported, got %d", idx)
	}
}

func TestSettingsState_TabCycle_ContainersUnsupported(t *testing.T) {
	s := NewSettingsState("", false, false, false, false, "/some/repo", "")

	// Start at 0 (branch prefix)
	if s.Focus != 0 {
		t.Fatalf("Expected initial focus 0, got %d", s.Focus)
	}

	// Tab through: 0 -> 1 -> 2 -> 3 -> 0 (4 fields when containers unsupported)
	expectedFoci := []int{1, 2, 3, 0}
	for i, expected := range expectedFoci {
		s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if s.Focus != expected {
			t.Errorf("After tab %d: expected focus %d, got %d", i+1, expected, s.Focus)
		}
	}
}

func TestSettingsState_ContainerToggle_OnlyWhenSupported(t *testing.T) {
	// With containers unsupported, toggling at focus 3 should NOT toggle containers
	s := NewSettingsState("", false, false, false, false, "/some/repo", "")
	s.Focus = 3 // This is the Asana field when containers unsupported

	s.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if s.UseContainers {
		t.Error("Space at focus 3 should not toggle containers when unsupported")
	}
}

func TestSettingsState_ContainerToggle_WhenSupported(t *testing.T) {
	s := NewSettingsState("", false, false, false, true, "/some/repo", "")
	s.Focus = 3 // Containers field when supported

	s.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !s.UseContainers {
		t.Error("Space at focus 3 should toggle containers when supported")
	}
}

func TestSettingsState_Render_ContainersHidden_WhenUnsupported(t *testing.T) {
	s := NewSettingsState("", false, false, false, false, "/some/repo", "")
	rendered := s.Render()

	if strings.Contains(rendered, "Run sessions in containers") {
		t.Error("Container checkbox should not appear when containers unsupported")
	}
}

func TestSettingsState_Render_ContainersShown_WhenSupported(t *testing.T) {
	s := NewSettingsState("", false, false, false, true, "/some/repo", "")
	rendered := s.Render()

	if !strings.Contains(rendered, "Run sessions in containers") {
		t.Error("Container checkbox should appear when containers supported")
	}
}

func TestSettingsState_Render_WarningShown_WhenSupported(t *testing.T) {
	s := NewSettingsState("", false, false, false, true, "/some/repo", "")
	rendered := s.Render()

	if !strings.Contains(rendered, "defense in depth") {
		t.Error("Warning about defense in depth should appear when containers shown")
	}
}

func TestSettingsState_Render_NoWarning_WhenUnsupported(t *testing.T) {
	s := NewSettingsState("", false, false, false, false, "/some/repo", "")
	rendered := s.Render()

	if strings.Contains(rendered, "defense in depth") {
		t.Error("Warning should not appear when containers unsupported")
	}
}
