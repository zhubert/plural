package modals

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/config"
)

func TestNewBulkActionState(t *testing.T) {
	ids := []string{"s1", "s2", "s3"}
	workspaces := []config.Workspace{
		{ID: "ws1", Name: "Feature Work"},
	}

	state := NewBulkActionState(ids, workspaces)

	if state.SessionCount != 3 {
		t.Errorf("expected session count 3, got %d", state.SessionCount)
	}
	if len(state.SessionIDs) != 3 {
		t.Errorf("expected 3 session IDs, got %d", len(state.SessionIDs))
	}
	if state.Action != BulkActionDelete {
		t.Errorf("expected default action to be Delete, got %d", state.Action)
	}
	if len(state.Workspaces) != 1 {
		t.Errorf("expected 1 workspace, got %d", len(state.Workspaces))
	}
}

func TestBulkActionState_SwitchAction(t *testing.T) {
	state := NewBulkActionState([]string{"s1"}, nil)

	// Start at Delete
	if state.Action != BulkActionDelete {
		t.Fatal("should start at Delete")
	}

	// Switch right to Move
	state.Update(tea.KeyPressMsg{Code: -1, Text: "l"})
	if state.Action != BulkActionMoveToWorkspace {
		t.Errorf("expected MoveToWorkspace, got %d", state.Action)
	}

	// Can't go further right
	state.Update(tea.KeyPressMsg{Code: -1, Text: "l"})
	if state.Action != BulkActionMoveToWorkspace {
		t.Errorf("should stay at MoveToWorkspace, got %d", state.Action)
	}

	// Switch back left to Delete
	state.Update(tea.KeyPressMsg{Code: -1, Text: "h"})
	if state.Action != BulkActionDelete {
		t.Errorf("expected Delete, got %d", state.Action)
	}

	// Can't go further left
	state.Update(tea.KeyPressMsg{Code: -1, Text: "h"})
	if state.Action != BulkActionDelete {
		t.Errorf("should stay at Delete, got %d", state.Action)
	}
}

func TestBulkActionState_WorkspaceNavigation(t *testing.T) {
	workspaces := []config.Workspace{
		{ID: "ws1", Name: "A"},
		{ID: "ws2", Name: "B"},
		{ID: "ws3", Name: "C"},
	}
	state := NewBulkActionState([]string{"s1"}, workspaces)

	// Switch to MoveToWorkspace
	state.Action = BulkActionMoveToWorkspace

	// Navigate down
	state.Update(tea.KeyPressMsg{Code: -1, Text: "j"})
	if state.SelectedWSIdx != 1 {
		t.Errorf("expected ws index 1, got %d", state.SelectedWSIdx)
	}

	state.Update(tea.KeyPressMsg{Code: -1, Text: "j"})
	if state.SelectedWSIdx != 2 {
		t.Errorf("expected ws index 2, got %d", state.SelectedWSIdx)
	}

	// Can't go past end
	state.Update(tea.KeyPressMsg{Code: -1, Text: "j"})
	if state.SelectedWSIdx != 2 {
		t.Errorf("should stay at 2, got %d", state.SelectedWSIdx)
	}

	// Navigate up
	state.Update(tea.KeyPressMsg{Code: -1, Text: "k"})
	if state.SelectedWSIdx != 1 {
		t.Errorf("expected ws index 1, got %d", state.SelectedWSIdx)
	}
}

func TestBulkActionState_WorkspaceNavigation_OnlyInMoveAction(t *testing.T) {
	workspaces := []config.Workspace{
		{ID: "ws1", Name: "A"},
	}
	state := NewBulkActionState([]string{"s1"}, workspaces)

	// In Delete action, up/down should not change workspace index
	state.Action = BulkActionDelete
	state.Update(tea.KeyPressMsg{Code: -1, Text: "j"})
	if state.SelectedWSIdx != 0 {
		t.Errorf("up/down should not work in Delete mode, ws index is %d", state.SelectedWSIdx)
	}
}

func TestBulkActionState_GetSelectedWorkspaceID(t *testing.T) {
	workspaces := []config.Workspace{
		{ID: "ws1", Name: "A"},
		{ID: "ws2", Name: "B"},
	}
	state := NewBulkActionState([]string{"s1"}, workspaces)

	if id := state.GetSelectedWorkspaceID(); id != "ws1" {
		t.Errorf("expected ws1, got %q", id)
	}

	state.SelectedWSIdx = 1
	if id := state.GetSelectedWorkspaceID(); id != "ws2" {
		t.Errorf("expected ws2, got %q", id)
	}
}

func TestBulkActionState_GetSelectedWorkspaceID_Empty(t *testing.T) {
	state := NewBulkActionState([]string{"s1"}, nil)

	if id := state.GetSelectedWorkspaceID(); id != "" {
		t.Errorf("expected empty ID with no workspaces, got %q", id)
	}
}

func TestBulkActionState_Render_Delete(t *testing.T) {
	state := NewBulkActionState([]string{"s1", "s2"}, nil)
	rendered := state.Render()

	if !strings.Contains(rendered, "Bulk Action (2 sessions)") {
		t.Error("should contain title with count")
	}
	if !strings.Contains(rendered, "Delete") {
		t.Error("should contain Delete action")
	}
	if !strings.Contains(rendered, "delete 2 session(s)") {
		t.Errorf("should contain delete confirmation message, got:\n%s", rendered)
	}
}

func TestBulkActionState_Render_MoveToWorkspace(t *testing.T) {
	workspaces := []config.Workspace{
		{ID: "ws1", Name: "Feature Work"},
	}
	state := NewBulkActionState([]string{"s1"}, workspaces)
	state.Action = BulkActionMoveToWorkspace

	rendered := state.Render()

	if !strings.Contains(rendered, "Move to Workspace") {
		t.Error("should contain 'Move to Workspace' action")
	}
	if !strings.Contains(rendered, "Feature Work") {
		t.Error("should show workspace name")
	}
	if !strings.Contains(rendered, "Select workspace:") {
		t.Error("should show workspace selection label")
	}
}

func TestBulkActionState_Render_NoWorkspaces(t *testing.T) {
	state := NewBulkActionState([]string{"s1"}, nil)
	state.Action = BulkActionMoveToWorkspace

	rendered := state.Render()

	if !strings.Contains(rendered, "No workspaces") {
		t.Error("should show 'No workspaces' message")
	}
}
