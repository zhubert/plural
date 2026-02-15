package modals

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/keys"
)

// =============================================================================
// Autonomous and Container Mode Tests
// =============================================================================

func TestImportIssuesState_ContainerSupport(t *testing.T) {
	// Test with containers supported
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	if !state.ContainersSupported {
		t.Error("expected ContainersSupported to be true")
	}
	if !state.ContainerAuthAvailable {
		t.Error("expected ContainerAuthAvailable to be true")
	}

	// Test without containers supported
	state = NewImportIssuesState("/repo/path", "test-repo", false, false)
	if state.ContainersSupported {
		t.Error("expected ContainersSupported to be false")
	}
	if state.ContainerAuthAvailable {
		t.Error("expected ContainerAuthAvailable to be false")
	}
}

func TestImportIssuesState_AutonomousMode_DefaultsFalse(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	if state.Autonomous {
		t.Error("expected Autonomous to default to false")
	}
	if state.UseContainers {
		t.Error("expected UseContainers to default to false")
	}
}

func TestImportIssuesState_AutonomousMode_Toggle(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.SetIssues([]IssueItem{
		{ID: "1", Title: "Issue 1", Source: "github"},
	})

	// Move focus to autonomous checkbox
	tabMsg := tea.KeyPressMsg{Code: 0, Text: keys.Tab}
	state.Update(tabMsg)
	if state.Focus != 1 {
		t.Errorf("expected focus 1 (autonomous), got %d", state.Focus)
	}

	// Toggle autonomous mode on
	spaceMsg := tea.KeyPressMsg{Code: 0, Text: keys.Space}
	state.Update(spaceMsg)
	if !state.Autonomous {
		t.Error("expected Autonomous to be true after toggle")
	}
	if !state.UseContainers {
		t.Error("expected UseContainers to be enabled when Autonomous is enabled")
	}

	// Toggle autonomous mode off
	state.Update(spaceMsg)
	if state.Autonomous {
		t.Error("expected Autonomous to be false after second toggle")
	}
	// UseContainers should remain true (not auto-disabled when autonomous is disabled)
	if !state.UseContainers {
		t.Error("expected UseContainers to remain true after disabling autonomous")
	}
}

func TestImportIssuesState_ContainerMode_Toggle(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.SetIssues([]IssueItem{
		{ID: "1", Title: "Issue 1", Source: "github"},
	})

	// Move focus to container checkbox (focus 2)
	tabMsg := tea.KeyPressMsg{Code: 0, Text: keys.Tab}
	state.Update(tabMsg) // Focus 1 (autonomous)
	state.Update(tabMsg) // Focus 2 (containers)
	if state.Focus != 2 {
		t.Errorf("expected focus 2 (containers), got %d", state.Focus)
	}

	// Toggle container mode on
	spaceMsg := tea.KeyPressMsg{Code: 0, Text: keys.Space}
	state.Update(spaceMsg)
	if !state.UseContainers {
		t.Error("expected UseContainers to be true after toggle")
	}
	if state.Autonomous {
		t.Error("expected Autonomous to remain false")
	}
}

func TestImportIssuesState_ContainerMode_DisabledWhenAutonomous(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.SetIssues([]IssueItem{
		{ID: "1", Title: "Issue 1", Source: "github"},
	})

	// Enable autonomous mode first
	tabMsg := tea.KeyPressMsg{Code: 0, Text: keys.Tab}
	state.Update(tabMsg) // Focus 1 (autonomous)
	spaceMsg := tea.KeyPressMsg{Code: 0, Text: keys.Space}
	state.Update(spaceMsg) // Toggle autonomous on

	// Now try to toggle containers (should not work when autonomous)
	state.Update(tabMsg) // Focus 2 (containers)
	state.Update(spaceMsg) // Try to toggle containers off
	if !state.UseContainers {
		t.Error("expected UseContainers to remain true when Autonomous is enabled")
	}
}

func TestImportIssuesState_FocusCycle(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.SetIssues([]IssueItem{
		{ID: "1", Title: "Issue 1", Source: "github"},
	})

	// Start at focus 0 (issue list)
	if state.Focus != 0 {
		t.Errorf("expected initial focus 0, got %d", state.Focus)
	}

	// Tab to autonomous (focus 1)
	tabMsg := tea.KeyPressMsg{Code: 0, Text: keys.Tab}
	state.Update(tabMsg)
	if state.Focus != 1 {
		t.Errorf("expected focus 1, got %d", state.Focus)
	}

	// Tab to containers (focus 2)
	state.Update(tabMsg)
	if state.Focus != 2 {
		t.Errorf("expected focus 2, got %d", state.Focus)
	}

	// Tab back to issue list (focus 0)
	state.Update(tabMsg)
	if state.Focus != 0 {
		t.Errorf("expected focus 0 after cycling, got %d", state.Focus)
	}
}

func TestImportIssuesState_NavigationOnlyWhenFocusedOnList(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.SetIssues([]IssueItem{
		{ID: "1", Title: "Issue 1", Source: "github"},
		{ID: "2", Title: "Issue 2", Source: "github"},
	})

	// Initially at focus 0, can navigate
	downMsg := tea.KeyPressMsg{Code: 0, Text: keys.Down}
	state.Update(downMsg)
	if state.SelectedIndex != 1 {
		t.Errorf("expected selected index 1, got %d", state.SelectedIndex)
	}

	// Move focus to autonomous checkbox
	tabMsg := tea.KeyPressMsg{Code: 0, Text: keys.Tab}
	state.Update(tabMsg) // Focus 1 (autonomous)

	// Try to navigate down - should not work
	state.Update(downMsg)
	if state.SelectedIndex != 1 {
		t.Errorf("expected selected index to remain 1 when not focused on list, got %d", state.SelectedIndex)
	}

	// Move focus back to list
	state.Update(tabMsg) // Focus 2 (containers)
	state.Update(tabMsg) // Focus 0 (list)

	// Now navigation should work again
	state.Update(downMsg)
	if state.SelectedIndex != 1 {
		t.Errorf("expected selected index to stay at 1 (already at max), got %d", state.SelectedIndex)
	}
	upMsg := tea.KeyPressMsg{Code: 0, Text: keys.Up}
	state.Update(upMsg)
	if state.SelectedIndex != 0 {
		t.Errorf("expected selected index 0 after navigating up, got %d", state.SelectedIndex)
	}
}

func TestImportIssuesState_GetMethods(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)

	// Initially both false
	if state.GetUseContainers() {
		t.Error("expected GetUseContainers to return false initially")
	}
	if state.GetAutonomous() {
		t.Error("expected GetAutonomous to return false initially")
	}

	// Enable autonomous
	state.Autonomous = true
	if !state.GetAutonomous() {
		t.Error("expected GetAutonomous to return true")
	}
	// GetUseContainers should return true when autonomous is true
	if !state.GetUseContainers() {
		t.Error("expected GetUseContainers to return true when autonomous is true")
	}

	// Disable autonomous, enable containers manually
	state.Autonomous = false
	state.UseContainers = true
	if state.GetAutonomous() {
		t.Error("expected GetAutonomous to return false")
	}
	if !state.GetUseContainers() {
		t.Error("expected GetUseContainers to return true when UseContainers is true")
	}
}

func TestImportIssuesState_WithoutContainerSupport(t *testing.T) {
	// When containers are not supported, the checkboxes should not appear
	state := NewImportIssuesState("/repo/path", "test-repo", false, false)
	state.SetIssues([]IssueItem{
		{ID: "1", Title: "Issue 1", Source: "github"},
	})

	// Tab should not cycle through autonomous/container checkboxes
	tabMsg := tea.KeyPressMsg{Code: 0, Text: keys.Tab}
	state.Update(tabMsg)
	// Focus should still be 0 since there are no other focus targets
	if state.Focus != 0 {
		t.Errorf("expected focus to remain 0 when containers not supported, got %d", state.Focus)
	}

	// Space should only toggle issue selection
	spaceMsg := tea.KeyPressMsg{Code: 0, Text: keys.Space}
	state.Update(spaceMsg)
	if !state.Issues[0].Selected {
		t.Error("expected issue to be selected after space")
	}
	if state.Autonomous {
		t.Error("autonomous should not be toggled when containers not supported")
	}
}
