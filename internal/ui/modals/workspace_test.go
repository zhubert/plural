package modals

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/config"
)

func TestNewWorkspaceListState(t *testing.T) {
	workspaces := []config.Workspace{
		{ID: "ws1", Name: "Feature Work"},
		{ID: "ws2", Name: "Bug Fixes"},
	}
	counts := map[string]int{
		"ws1": 3,
		"ws2": 1,
	}

	state := NewWorkspaceListState(workspaces, counts, "")

	if len(state.Workspaces) != 2 {
		t.Errorf("expected 2 workspaces, got %d", len(state.Workspaces))
	}

	// No active workspace -> selected index should be 0 ("All Sessions")
	if state.SelectedIndex != 0 {
		t.Errorf("expected selectedIndex 0, got %d", state.SelectedIndex)
	}

	if state.ActiveWorkspaceID != "" {
		t.Errorf("expected empty activeWorkspaceID, got %q", state.ActiveWorkspaceID)
	}
}

func TestWorkspaceListState_ActiveWorkspaceSelection(t *testing.T) {
	workspaces := []config.Workspace{
		{ID: "ws1", Name: "Feature Work"},
		{ID: "ws2", Name: "Bug Fixes"},
	}
	counts := map[string]int{}

	state := NewWorkspaceListState(workspaces, counts, "ws2")

	// Active workspace is ws2, which is index 2 (0=All, 1=ws1, 2=ws2)
	if state.SelectedIndex != 2 {
		t.Errorf("expected selectedIndex 2 for active ws2, got %d", state.SelectedIndex)
	}
}

func TestWorkspaceListState_GetSelectedWorkspaceID(t *testing.T) {
	workspaces := []config.Workspace{
		{ID: "ws1", Name: "Feature Work"},
		{ID: "ws2", Name: "Bug Fixes"},
	}
	counts := map[string]int{}

	state := NewWorkspaceListState(workspaces, counts, "")

	// Index 0 = "All Sessions" -> empty string
	state.SelectedIndex = 0
	if id := state.GetSelectedWorkspaceID(); id != "" {
		t.Errorf("All Sessions should return empty ID, got %q", id)
	}

	// Index 1 = ws1
	state.SelectedIndex = 1
	if id := state.GetSelectedWorkspaceID(); id != "ws1" {
		t.Errorf("expected ws1, got %q", id)
	}

	// Index 2 = ws2
	state.SelectedIndex = 2
	if id := state.GetSelectedWorkspaceID(); id != "ws2" {
		t.Errorf("expected ws2, got %q", id)
	}
}

func TestWorkspaceListState_IsAllSessionsSelected(t *testing.T) {
	state := NewWorkspaceListState(nil, nil, "")

	if !state.IsAllSessionsSelected() {
		t.Error("should be 'All Sessions' selected by default")
	}

	state.SelectedIndex = 1
	if state.IsAllSessionsSelected() {
		t.Error("should not be 'All Sessions' at index 1")
	}
}

func TestWorkspaceListState_Navigation(t *testing.T) {
	workspaces := []config.Workspace{
		{ID: "ws1", Name: "A"},
		{ID: "ws2", Name: "B"},
	}
	state := NewWorkspaceListState(workspaces, nil, "")

	// Navigate down
	state.Update(tea.KeyPressMsg{Code: -1, Text: "j"})
	if state.SelectedIndex != 1 {
		t.Errorf("expected index 1 after down, got %d", state.SelectedIndex)
	}

	state.Update(tea.KeyPressMsg{Code: -1, Text: "j"})
	if state.SelectedIndex != 2 {
		t.Errorf("expected index 2 after second down, got %d", state.SelectedIndex)
	}

	// Should not go past end
	state.Update(tea.KeyPressMsg{Code: -1, Text: "j"})
	if state.SelectedIndex != 2 {
		t.Errorf("should stay at 2 (max), got %d", state.SelectedIndex)
	}

	// Navigate up
	state.Update(tea.KeyPressMsg{Code: -1, Text: "k"})
	if state.SelectedIndex != 1 {
		t.Errorf("expected index 1 after up, got %d", state.SelectedIndex)
	}

	// Should not go past start
	state.SelectedIndex = 0
	state.Update(tea.KeyPressMsg{Code: -1, Text: "k"})
	if state.SelectedIndex != 0 {
		t.Errorf("should stay at 0 (min), got %d", state.SelectedIndex)
	}
}

func TestWorkspaceListState_Render(t *testing.T) {
	workspaces := []config.Workspace{
		{ID: "ws1", Name: "Feature Work"},
	}
	counts := map[string]int{"ws1": 5}

	state := NewWorkspaceListState(workspaces, counts, "ws1")

	rendered := state.Render()

	if !strings.Contains(rendered, "Workspaces") {
		t.Error("should contain title 'Workspaces'")
	}
	if !strings.Contains(rendered, "All Sessions") {
		t.Error("should contain 'All Sessions' entry")
	}
	if !strings.Contains(rendered, "Feature Work") {
		t.Error("should contain workspace name")
	}
	if !strings.Contains(rendered, "5 sessions") {
		t.Error("should contain session count")
	}
	if !strings.Contains(rendered, "(active)") {
		t.Error("should show active indicator for ws1")
	}
}

func TestWorkspaceListState_Render_NoWorkspaces(t *testing.T) {
	state := NewWorkspaceListState(nil, nil, "")
	rendered := state.Render()

	if !strings.Contains(rendered, "No workspaces") {
		t.Error("should show empty message")
	}
}

func TestNewNewWorkspaceState(t *testing.T) {
	state := NewNewWorkspaceState()

	if state.IsRename {
		t.Error("new workspace should not be rename mode")
	}
	if state.WorkspaceID != "" {
		t.Error("new workspace should have empty ID")
	}
	if state.Title() != "New Workspace" {
		t.Errorf("expected title 'New Workspace', got %q", state.Title())
	}
	if state.GetName() != "" {
		t.Error("name should start empty")
	}
}

func TestNewRenameWorkspaceState(t *testing.T) {
	state := NewRenameWorkspaceState("ws1", "Old Name")

	if !state.IsRename {
		t.Error("rename workspace should be in rename mode")
	}
	if state.WorkspaceID != "ws1" {
		t.Errorf("expected workspace ID 'ws1', got %q", state.WorkspaceID)
	}
	if state.Title() != "Rename Workspace" {
		t.Errorf("expected title 'Rename Workspace', got %q", state.Title())
	}
	if state.GetName() != "Old Name" {
		t.Errorf("expected initial name 'Old Name', got %q", state.GetName())
	}
}

func TestNewWorkspaceState_Render(t *testing.T) {
	state := NewNewWorkspaceState()
	rendered := state.Render()

	if !strings.Contains(rendered, "New Workspace") {
		t.Error("should contain title")
	}
	if !strings.Contains(rendered, "Name:") {
		t.Error("should contain name label")
	}
	if !strings.Contains(rendered, "Enter: save") {
		t.Error("should contain help text")
	}
}

func TestWorkspaceListState_GetSelectedWorkspaceName(t *testing.T) {
	workspaces := []config.Workspace{
		{ID: "ws1", Name: "Feature Work"},
		{ID: "ws2", Name: "Bug Fixes"},
	}

	state := NewWorkspaceListState(workspaces, nil, "")

	// "All Sessions" has no name
	state.SelectedIndex = 0
	if name := state.GetSelectedWorkspaceName(); name != "" {
		t.Errorf("All Sessions should have empty name, got %q", name)
	}

	// ws1
	state.SelectedIndex = 1
	if name := state.GetSelectedWorkspaceName(); name != "Feature Work" {
		t.Errorf("expected 'Feature Work', got %q", name)
	}

	// ws2
	state.SelectedIndex = 2
	if name := state.GetSelectedWorkspaceName(); name != "Bug Fixes" {
		t.Errorf("expected 'Bug Fixes', got %q", name)
	}
}
