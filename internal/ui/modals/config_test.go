package modals

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSettingsState_NumFields_NoRepo(t *testing.T) {
	s := NewSettingsState("", false, false, "", "")
	if n := s.numFields(); n != 2 {
		t.Errorf("Expected 2 fields with no repo, got %d", n)
	}
}

func TestSettingsState_NumFields_WithRepo(t *testing.T) {
	s := NewSettingsState("", false, false, "/some/repo", "")
	if n := s.numFields(); n != 4 {
		t.Errorf("Expected 4 fields with repo, got %d", n)
	}
}

func TestSettingsState_AsanaFocusIndex(t *testing.T) {
	s := NewSettingsState("", false, false, "/some/repo", "")
	if idx := s.asanaFocusIndex(); idx != 3 {
		t.Errorf("Expected asana focus index 3, got %d", idx)
	}
}

func TestSettingsState_TabCycle_WithRepo(t *testing.T) {
	s := NewSettingsState("", false, false, "/some/repo", "")

	// Start at 0 (branch prefix)
	if s.Focus != 0 {
		t.Fatalf("Expected initial focus 0, got %d", s.Focus)
	}

	// Tab through: 0 -> 1 -> 2 -> 3 -> 0 (4 fields with repo)
	expectedFoci := []int{1, 2, 3, 0}
	for i, expected := range expectedFoci {
		s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if s.Focus != expected {
			t.Errorf("After tab %d: expected focus %d, got %d", i+1, expected, s.Focus)
		}
	}
}

func TestSettingsState_Render_NoContainerSection(t *testing.T) {
	// Container settings should no longer appear in settings modal
	s := NewSettingsState("", false, false, "/some/repo", "")
	rendered := s.Render()

	if strings.Contains(rendered, "Run sessions in containers") {
		t.Error("Container checkbox should not appear in settings modal")
	}
	if strings.Contains(rendered, "defense in depth") {
		t.Error("Container warning should not appear in settings modal")
	}
}

func TestNewSessionState_ContainerCheckbox_WhenSupported(t *testing.T) {
	s := NewNewSessionState([]string{"/repo"}, true)

	if s.numFields() != 4 {
		t.Errorf("Expected 4 fields with containers supported, got %d", s.numFields())
	}

	// Tab to container checkbox (focus 3)
	s.Focus = 3
	s.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !s.UseContainers {
		t.Error("Space at focus 3 should toggle container checkbox when supported")
	}

	// Toggle back
	s.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if s.UseContainers {
		t.Error("Space again should toggle container checkbox off")
	}
}

func TestNewSessionState_ContainerCheckbox_WhenUnsupported(t *testing.T) {
	s := NewNewSessionState([]string{"/repo"}, false)

	if s.numFields() != 3 {
		t.Errorf("Expected 3 fields with containers unsupported, got %d", s.numFields())
	}

	// Container checkbox should not be rendered
	rendered := s.Render()
	if strings.Contains(rendered, "Run in container") {
		t.Error("Container checkbox should not appear when unsupported")
	}
}

func TestNewSessionState_ContainerCheckbox_Render(t *testing.T) {
	s := NewNewSessionState([]string{"/repo"}, true)
	rendered := s.Render()

	if !strings.Contains(rendered, "Run in container") {
		t.Error("Container checkbox should appear when supported")
	}
	if !strings.Contains(rendered, "defense in depth") {
		t.Error("Container warning should appear when supported")
	}
}

func TestNewSessionState_GetUseContainers(t *testing.T) {
	s := NewNewSessionState([]string{"/repo"}, true)

	if s.GetUseContainers() {
		t.Error("GetUseContainers should return false initially")
	}

	s.UseContainers = true
	if !s.GetUseContainers() {
		t.Error("GetUseContainers should return true after setting")
	}
}

func TestForkSessionState_ContainerCheckbox_WhenSupported(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, true)

	if s.numFields() != 3 {
		t.Errorf("Expected 3 fields with containers supported, got %d", s.numFields())
	}

	// Tab to container checkbox (focus 2)
	s.Focus = 2
	s.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !s.UseContainers {
		t.Error("Space at focus 2 should toggle container checkbox when supported")
	}
}

func TestForkSessionState_ContainerCheckbox_WhenUnsupported(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, false)

	if s.numFields() != 2 {
		t.Errorf("Expected 2 fields with containers unsupported, got %d", s.numFields())
	}

	rendered := s.Render()
	if strings.Contains(rendered, "Run in container") {
		t.Error("Container checkbox should not appear when unsupported")
	}
}

func TestForkSessionState_InheritsParentContainerized(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", true, true)

	if !s.UseContainers {
		t.Error("Fork should default to parent's containerized state (true)")
	}

	s2 := NewForkSessionState("parent", "parent-id", "/repo", false, true)
	if s2.UseContainers {
		t.Error("Fork should default to parent's containerized state (false)")
	}
}

func TestForkSessionState_ContainerCheckbox_Render(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, true)
	rendered := s.Render()

	if !strings.Contains(rendered, "Run in container") {
		t.Error("Container checkbox should appear when supported")
	}
}

func TestForkSessionState_UpDownNavigation_CyclesToContainerCheckbox(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, true)

	// Start at focus 0
	if s.Focus != 0 {
		t.Fatalf("Expected focus 0, got %d", s.Focus)
	}

	// Press down should go to focus 1
	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.Focus != 1 {
		t.Errorf("After Down from 0, expected focus 1, got %d", s.Focus)
	}

	// Press down should go to focus 2 (container checkbox)
	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.Focus != 2 {
		t.Errorf("After Down from 1, expected focus 2 (container checkbox), got %d", s.Focus)
	}

	// Press down should wrap to focus 0
	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.Focus != 0 {
		t.Errorf("After Down from 2, expected focus 0 (wrap), got %d", s.Focus)
	}

	// Press up should go to focus 2
	s.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if s.Focus != 2 {
		t.Errorf("After Up from 0, expected focus 2 (wrap), got %d", s.Focus)
	}
}

func TestForkSessionState_UpDownNavigation_WithoutContainers(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, false)

	// Only 2 fields: toggle between 0 and 1
	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.Focus != 1 {
		t.Errorf("After Down from 0, expected focus 1, got %d", s.Focus)
	}

	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.Focus != 0 {
		t.Errorf("After Down from 1, expected focus 0 (wrap), got %d", s.Focus)
	}
}

func TestForkSessionState_HelpText_ContainerFocused(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, true)
	s.Focus = 2

	help := s.Help()
	if !strings.Contains(help, "Space: toggle") {
		t.Errorf("Help at container focus should mention Space: toggle, got %q", help)
	}
}

func TestBroadcastState_ContainerCheckbox_WhenSupported(t *testing.T) {
	s := NewBroadcastState([]string{"/repo"}, true)

	if !s.ContainersSupported {
		t.Error("ContainersSupported should be true")
	}

	// Tab to container checkbox (focus 3)
	s.Focus = 3
	s.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !s.UseContainers {
		t.Error("Space at focus 3 should toggle container checkbox")
	}

	if !s.GetUseContainers() {
		t.Error("GetUseContainers should return true after toggle")
	}
}

func TestBroadcastState_ContainerCheckbox_WhenUnsupported(t *testing.T) {
	s := NewBroadcastState([]string{"/repo"}, false)

	rendered := s.Render()
	if strings.Contains(rendered, "Run in containers") {
		t.Error("Container checkbox should not appear when unsupported")
	}
}

func TestBroadcastState_ContainerCheckbox_Render(t *testing.T) {
	s := NewBroadcastState([]string{"/repo"}, true)
	rendered := s.Render()

	if !strings.Contains(rendered, "Run in containers") {
		t.Error("Container checkbox should appear when supported")
	}
}
