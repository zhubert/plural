package modals

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSettingsState_NumFields_NoRepo(t *testing.T) {
	s := NewSettingsState("", false, nil, nil, 0, false)
	if n := s.numFields(); n != 2 {
		t.Errorf("Expected 2 fields with no repos, got %d", n)
	}
}

func TestSettingsState_NumFields_WithRepo_AsanaPAT(t *testing.T) {
	s := NewSettingsState("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": ""},
		0, true)
	if n := s.numFields(); n != 4 {
		t.Errorf("Expected 4 fields with repo and Asana PAT, got %d", n)
	}
}

func TestSettingsState_NumFields_WithRepo_NoAsanaPAT(t *testing.T) {
	s := NewSettingsState("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": ""},
		0, false)
	if n := s.numFields(); n != 3 {
		t.Errorf("Expected 3 fields with repo but no Asana PAT, got %d", n)
	}
}

func TestSettingsState_AsanaFocusIndex(t *testing.T) {
	s := NewSettingsState("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": ""},
		0, true)
	if idx := s.asanaFocusIndex(); idx != 3 {
		t.Errorf("Expected asana focus index 3, got %d", idx)
	}
}

func TestSettingsState_TabCycle_WithRepo(t *testing.T) {
	s := NewSettingsState("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": ""},
		0, true)

	// Start at 0 (branch prefix)
	if s.Focus != 0 {
		t.Fatalf("Expected initial focus 0, got %d", s.Focus)
	}

	// Tab through: 0 -> 1 -> 2 -> 3 -> 0 (4 fields with repo + PAT)
	expectedFoci := []int{1, 2, 3, 0}
	for i, expected := range expectedFoci {
		s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if s.Focus != expected {
			t.Errorf("After tab %d: expected focus %d, got %d", i+1, expected, s.Focus)
		}
	}
}

func TestSettingsState_TabCycle_WithRepo_NoPAT(t *testing.T) {
	s := NewSettingsState("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": ""},
		0, false)

	// Tab through: 0 -> 1 -> 2 -> 0 (3 fields, no asana)
	expectedFoci := []int{1, 2, 0}
	for i, expected := range expectedFoci {
		s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if s.Focus != expected {
			t.Errorf("After tab %d: expected focus %d, got %d", i+1, expected, s.Focus)
		}
	}
}

func TestSettingsState_Render_NoContainerSection(t *testing.T) {
	s := NewSettingsState("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": ""},
		0, true)
	rendered := s.Render()

	if strings.Contains(rendered, "Run sessions in containers") {
		t.Error("Container checkbox should not appear in settings modal")
	}
	if strings.Contains(rendered, "defense in depth") {
		t.Error("Container warning should not appear in settings modal")
	}
}

func TestSettingsState_Render_AsanaHiddenWithoutPAT(t *testing.T) {
	s := NewSettingsState("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": "123"},
		0, false)
	rendered := s.Render()

	if strings.Contains(rendered, "Asana project GID") {
		t.Error("Asana field should not appear when ASANA_PAT is not set")
	}
}

func TestSettingsState_Render_AsanaShownWithPAT(t *testing.T) {
	s := NewSettingsState("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": "123"},
		0, true)
	rendered := s.Render()

	if !strings.Contains(rendered, "Asana project GID") {
		t.Error("Asana field should appear when ASANA_PAT is set")
	}
}

func TestSettingsState_RepoSelector_LeftRight(t *testing.T) {
	repos := []string{"/repo/a", "/repo/b", "/repo/c"}
	s := NewSettingsState("", false, repos,
		map[string]string{"/repo/a": "111", "/repo/b": "222", "/repo/c": "333"},
		0, true)

	// Focus on repo selector
	s.Focus = 2

	// Initially at index 0
	if s.SelectedRepoIndex != 0 {
		t.Fatalf("Expected initial repo index 0, got %d", s.SelectedRepoIndex)
	}

	// Press right -> index 1
	s.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if s.SelectedRepoIndex != 1 {
		t.Errorf("After Right, expected repo index 1, got %d", s.SelectedRepoIndex)
	}
	if s.AsanaProjectInput.Value() != "222" {
		t.Errorf("Expected asana GID '222', got %q", s.AsanaProjectInput.Value())
	}

	// Press right -> index 2
	s.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if s.SelectedRepoIndex != 2 {
		t.Errorf("After Right, expected repo index 2, got %d", s.SelectedRepoIndex)
	}
	if s.AsanaProjectInput.Value() != "333" {
		t.Errorf("Expected asana GID '333', got %q", s.AsanaProjectInput.Value())
	}

	// Press right at max -> should clamp
	s.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if s.SelectedRepoIndex != 2 {
		t.Errorf("After Right at max, expected repo index 2 (clamped), got %d", s.SelectedRepoIndex)
	}

	// Press left -> index 1
	s.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if s.SelectedRepoIndex != 1 {
		t.Errorf("After Left, expected repo index 1, got %d", s.SelectedRepoIndex)
	}

	// Press left -> index 0
	s.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if s.SelectedRepoIndex != 0 {
		t.Errorf("After Left, expected repo index 0, got %d", s.SelectedRepoIndex)
	}

	// Press left at min -> should clamp
	s.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if s.SelectedRepoIndex != 0 {
		t.Errorf("After Left at min, expected repo index 0 (clamped), got %d", s.SelectedRepoIndex)
	}
}

func TestSettingsState_ValuePersistenceAcrossSwitch(t *testing.T) {
	repos := []string{"/repo/a", "/repo/b"}
	s := NewSettingsState("", false, repos,
		map[string]string{"/repo/a": "aaa", "/repo/b": "bbb"},
		0, true)

	// Modify repo a's asana GID via the input
	s.Focus = s.asanaFocusIndex()
	s.updateInputFocus()
	s.AsanaProjectInput.SetValue("modified-aaa")

	// Switch repos
	s.Focus = 2
	s.switchRepo(1)

	// Repo b should show its original values
	if s.AsanaProjectInput.Value() != "bbb" {
		t.Errorf("Expected asana GID 'bbb' for repo b, got %q", s.AsanaProjectInput.Value())
	}

	// Switch back to repo a
	s.switchRepo(-1)

	// Modified value should be preserved
	if s.AsanaProjectInput.Value() != "modified-aaa" {
		t.Errorf("Expected modified asana GID 'modified-aaa' for repo a, got %q", s.AsanaProjectInput.Value())
	}
}

func TestSettingsState_GetAllAsanaProjects(t *testing.T) {
	repos := []string{"/repo/a", "/repo/b"}
	s := NewSettingsState("", false, repos,
		map[string]string{"/repo/a": "aaa", "/repo/b": "bbb"},
		0, true)

	// Modify current repo's value
	s.AsanaProjectInput.SetValue("modified")

	projects := s.GetAllAsanaProjects()
	if projects["/repo/a"] != "modified" {
		t.Errorf("Expected GetAllAsanaProjects to flush current value, got %q", projects["/repo/a"])
	}
	if projects["/repo/b"] != "bbb" {
		t.Errorf("Expected repo b to have 'bbb', got %q", projects["/repo/b"])
	}
}

func TestSettingsState_Render_WithRepoSelector(t *testing.T) {
	repos := []string{"/path/to/myrepo"}
	s := NewSettingsState("", false, repos,
		map[string]string{"/path/to/myrepo": ""},
		0, false)

	rendered := s.Render()
	if !strings.Contains(rendered, "Per-repo settings") {
		t.Error("Expected per-repo section header")
	}
	if !strings.Contains(rendered, "myrepo") {
		t.Error("Expected repo name in rendered output")
	}
}

func TestSettingsState_Render_NoRepos(t *testing.T) {
	s := NewSettingsState("", false, nil, nil, 0, false)
	rendered := s.Render()

	if strings.Contains(rendered, "Per-repo settings") {
		t.Error("Per-repo section should not appear when no repos")
	}
}

func TestSettingsState_DefaultRepoIndex(t *testing.T) {
	repos := []string{"/repo/a", "/repo/b", "/repo/c"}
	s := NewSettingsState("", false, repos,
		map[string]string{"/repo/a": "aaa", "/repo/b": "bbb", "/repo/c": "ccc"},
		1, true)

	if s.SelectedRepoIndex != 1 {
		t.Errorf("Expected default repo index 1, got %d", s.SelectedRepoIndex)
	}
	if s.AsanaProjectInput.Value() != "bbb" {
		t.Errorf("Expected asana GID 'bbb' for default repo b, got %q", s.AsanaProjectInput.Value())
	}
}

func TestSettingsState_DefaultRepoIndex_OutOfBounds(t *testing.T) {
	repos := []string{"/repo/a"}
	s := NewSettingsState("", false, repos,
		map[string]string{"/repo/a": "aaa"},
		99, true)

	if s.SelectedRepoIndex != 0 {
		t.Errorf("Expected clamped repo index 0, got %d", s.SelectedRepoIndex)
	}
}

func TestSettingsState_HelpChangesOnRepoFocus(t *testing.T) {
	s := NewSettingsState("", false,
		[]string{"/repo"},
		map[string]string{"/repo": ""},
		0, false)

	s.Focus = 2
	help := s.Help()
	if !strings.Contains(help, "Left/Right") {
		t.Errorf("Help at repo selector focus should mention Left/Right, got %q", help)
	}

	s.Focus = 0
	help = s.Help()
	if strings.Contains(help, "Left/Right") {
		t.Errorf("Help at non-repo focus should not mention Left/Right, got %q", help)
	}
}

func TestNewSessionState_ContainerCheckbox_WhenSupported(t *testing.T) {
	s := NewNewSessionState([]string{"/repo"}, true, false)

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
	s := NewNewSessionState([]string{"/repo"}, false, false)

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
	s := NewNewSessionState([]string{"/repo"}, true, false)
	rendered := s.Render()

	if !strings.Contains(rendered, "Run in container") {
		t.Error("Container checkbox should appear when supported")
	}
	if !strings.Contains(rendered, "defense in depth") {
		t.Error("Container warning should appear when supported")
	}
}

func TestNewSessionState_GetUseContainers(t *testing.T) {
	s := NewNewSessionState([]string{"/repo"}, true, false)

	if s.GetUseContainers() {
		t.Error("GetUseContainers should return false initially")
	}

	s.UseContainers = true
	if !s.GetUseContainers() {
		t.Error("GetUseContainers should return true after setting")
	}
}

func TestForkSessionState_ContainerCheckbox_WhenSupported(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, true, false)

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
	s := NewForkSessionState("parent", "parent-id", "/repo", false, false, false)

	if s.numFields() != 2 {
		t.Errorf("Expected 2 fields with containers unsupported, got %d", s.numFields())
	}

	rendered := s.Render()
	if strings.Contains(rendered, "Run in container") {
		t.Error("Container checkbox should not appear when unsupported")
	}
}

func TestForkSessionState_InheritsParentContainerized(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", true, true, false)

	if !s.UseContainers {
		t.Error("Fork should default to parent's containerized state (true)")
	}

	s2 := NewForkSessionState("parent", "parent-id", "/repo", false, true, false)
	if s2.UseContainers {
		t.Error("Fork should default to parent's containerized state (false)")
	}
}

func TestForkSessionState_ContainerCheckbox_Render(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, true, false)
	rendered := s.Render()

	if !strings.Contains(rendered, "Run in container") {
		t.Error("Container checkbox should appear when supported")
	}
}

func TestForkSessionState_UpDownNavigation_CyclesToContainerCheckbox(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, true, false)

	if s.Focus != 0 {
		t.Fatalf("Expected focus 0, got %d", s.Focus)
	}

	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.Focus != 1 {
		t.Errorf("After Down from 0, expected focus 1, got %d", s.Focus)
	}

	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.Focus != 2 {
		t.Errorf("After Down from 1, expected focus 2 (container checkbox), got %d", s.Focus)
	}

	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.Focus != 0 {
		t.Errorf("After Down from 2, expected focus 0 (wrap), got %d", s.Focus)
	}

	s.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if s.Focus != 2 {
		t.Errorf("After Up from 0, expected focus 2 (wrap), got %d", s.Focus)
	}
}

func TestForkSessionState_UpDownNavigation_WithoutContainers(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, false, false)

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
	s := NewForkSessionState("parent", "parent-id", "/repo", false, true, false)
	s.Focus = 2

	help := s.Help()
	if !strings.Contains(help, "Space: toggle") {
		t.Errorf("Help at container focus should mention Space: toggle, got %q", help)
	}
}

func TestBroadcastState_ContainerCheckbox_WhenSupported(t *testing.T) {
	s := NewBroadcastState([]string{"/repo"}, true, false)

	if !s.ContainersSupported {
		t.Error("ContainersSupported should be true")
	}

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
	s := NewBroadcastState([]string{"/repo"}, false, false)

	rendered := s.Render()
	if strings.Contains(rendered, "Run in containers") {
		t.Error("Container checkbox should not appear when unsupported")
	}
}

func TestBroadcastState_ContainerCheckbox_Render(t *testing.T) {
	s := NewBroadcastState([]string{"/repo"}, true, false)
	rendered := s.Render()

	if !strings.Contains(rendered, "Run in containers") {
		t.Error("Container checkbox should appear when supported")
	}
}

func TestNewSessionState_AuthWarning_WhenNoAuth(t *testing.T) {
	s := NewNewSessionState([]string{"/repo"}, true, false)
	s.UseContainers = true
	rendered := s.Render()

	if !strings.Contains(rendered, "ANTHROPIC_API_KEY") {
		t.Error("Auth warning should appear when containers checked and no auth")
	}
}

func TestNewSessionState_AuthWarning_WhenAuthAvailable(t *testing.T) {
	s := NewNewSessionState([]string{"/repo"}, true, true)
	s.UseContainers = true
	rendered := s.Render()

	if strings.Contains(rendered, "Requires ANTHROPIC_API_KEY") {
		t.Error("Auth warning should not appear when auth is available")
	}
}

func TestNewSessionState_AuthWarning_WhenContainersNotChecked(t *testing.T) {
	s := NewNewSessionState([]string{"/repo"}, true, false)
	rendered := s.Render()

	if strings.Contains(rendered, "Requires ANTHROPIC_API_KEY") {
		t.Error("Auth warning should not appear when containers not checked")
	}
}

func TestForkSessionState_AuthWarning_WhenNoAuth(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", true, true, false)
	rendered := s.Render()

	if !strings.Contains(rendered, "ANTHROPIC_API_KEY") {
		t.Error("Auth warning should appear when containers checked and no auth (inherited from parent)")
	}
}

func TestForkSessionState_AuthWarning_WhenAuthAvailable(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", true, true, true)
	rendered := s.Render()

	if strings.Contains(rendered, "Requires ANTHROPIC_API_KEY") {
		t.Error("Auth warning should not appear when auth is available")
	}
}

func TestBroadcastState_AuthWarning_WhenNoAuth(t *testing.T) {
	s := NewBroadcastState([]string{"/repo"}, true, false)
	s.UseContainers = true
	rendered := s.Render()

	if !strings.Contains(rendered, "ANTHROPIC_API_KEY") {
		t.Error("Auth warning should appear when containers checked and no auth")
	}
}

func TestBroadcastState_AuthWarning_WhenAuthAvailable(t *testing.T) {
	s := NewBroadcastState([]string{"/repo"}, true, true)
	s.UseContainers = true
	rendered := s.Render()

	if strings.Contains(rendered, "Requires ANTHROPIC_API_KEY") {
		t.Error("Auth warning should not appear when auth is available")
	}
}
