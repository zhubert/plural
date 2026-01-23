package modals

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// =============================================================================
// PreviewActiveState Tests
// =============================================================================

func TestNewPreviewActiveState(t *testing.T) {
	state := NewPreviewActiveState("my-session", "feature-branch")

	if state.SessionName != "my-session" {
		t.Errorf("expected session name 'my-session', got '%s'", state.SessionName)
	}
	if state.BranchName != "feature-branch" {
		t.Errorf("expected branch name 'feature-branch', got '%s'", state.BranchName)
	}
}

func TestPreviewActiveState_Title(t *testing.T) {
	state := NewPreviewActiveState("session", "branch")

	title := state.Title()
	if title != "Preview Mode Active" {
		t.Errorf("expected title 'Preview Mode Active', got '%s'", title)
	}
}

func TestPreviewActiveState_Help(t *testing.T) {
	state := NewPreviewActiveState("session", "branch")

	help := state.Help()
	if help == "" {
		t.Error("expected non-empty help text")
	}
	if !strings.Contains(help, "p") {
		t.Error("expected help to mention 'p' key for ending preview")
	}
	if !strings.Contains(help, "Esc") {
		t.Error("expected help to mention 'Esc' key for dismissing")
	}
}

func TestPreviewActiveState_Render(t *testing.T) {
	state := NewPreviewActiveState("test-session", "my-feature-branch")

	rendered := state.Render()
	if rendered == "" {
		t.Error("expected non-empty rendered output")
	}

	// Check that session name appears in rendered output
	if !strings.Contains(rendered, "test-session") {
		t.Error("expected rendered output to contain session name")
	}

	// Check that branch name appears in rendered output
	if !strings.Contains(rendered, "my-feature-branch") {
		t.Error("expected rendered output to contain branch name")
	}
}

func TestPreviewActiveState_Update(t *testing.T) {
	state := NewPreviewActiveState("session", "branch")

	// Update should be a no-op for this simple acknowledgment modal
	keyMsg := tea.KeyPressMsg{Code: 0, Text: "x"}
	newState, cmd := state.Update(keyMsg)

	// State should be unchanged (same instance)
	if newState != state {
		t.Error("expected Update to return same state instance")
	}
	if cmd != nil {
		t.Error("expected Update to return nil cmd")
	}
}

func TestPreviewActiveState_ModalStateInterface(t *testing.T) {
	// Compile-time check that PreviewActiveState implements ModalState
	var _ ModalState = (*PreviewActiveState)(nil)
}
