package modals

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/keys"
)

// =============================================================================
// Container Mode Tests
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

func TestImportIssuesState_ContainerMode_DefaultsFalse(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	if state.UseContainers {
		t.Error("expected UseContainers to default to false")
	}
}

func TestImportIssuesState_ContainerMode_Toggle(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.SetIssues([]IssueItem{
		{ID: "1", Title: "Issue 1", Source: "github"},
	})

	// Move focus to container checkbox (focus 1)
	tabMsg := tea.KeyPressMsg{Code: 0, Text: keys.Tab}
	state.Update(tabMsg)
	if state.Focus != 1 {
		t.Errorf("expected focus 1 (containers), got %d", state.Focus)
	}

	// Toggle container mode on
	spaceMsg := tea.KeyPressMsg{Code: 0, Text: keys.Space}
	state.Update(spaceMsg)
	if !state.UseContainers {
		t.Error("expected UseContainers to be true after toggle")
	}

	// Toggle container mode off
	state.Update(spaceMsg)
	if state.UseContainers {
		t.Error("expected UseContainers to be false after second toggle")
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

	// Tab to containers (focus 1)
	tabMsg := tea.KeyPressMsg{Code: 0, Text: keys.Tab}
	state.Update(tabMsg)
	if state.Focus != 1 {
		t.Errorf("expected focus 1, got %d", state.Focus)
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

	// Initially at focus 0, can navigate issue list
	downMsg := tea.KeyPressMsg{Code: 0, Text: keys.Down}
	state.Update(downMsg)
	if state.SelectedIndex != 1 {
		t.Errorf("expected selected index 1, got %d", state.SelectedIndex)
	}

	// Move focus to container checkbox
	tabMsg := tea.KeyPressMsg{Code: 0, Text: keys.Tab}
	state.Update(tabMsg) // Focus 1 (containers)

	// Down arrow when focused on checkbox wraps to issue list
	state.Update(downMsg)
	if state.Focus != 0 {
		t.Errorf("expected focus to wrap to 0 (issue list), got %d", state.Focus)
	}
}

func TestImportIssuesState_GetMethods(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)

	// Initially false
	if state.GetUseContainers() {
		t.Error("expected GetUseContainers to return false initially")
	}

	// Enable containers
	state.UseContainers = true
	if !state.GetUseContainers() {
		t.Error("expected GetUseContainers to return true when UseContainers is true")
	}
}

func TestImportIssuesState_ArrowNavigationBetweenCheckboxes(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.SetIssues([]IssueItem{
		{ID: "1", Title: "Issue 1", Source: "github"},
		{ID: "2", Title: "Issue 2", Source: "github"},
	})

	// Tab to container checkbox (focus 1)
	tabMsg := tea.KeyPressMsg{Code: 0, Text: keys.Tab}
	state.Update(tabMsg)
	if state.Focus != 1 {
		t.Fatalf("expected focus 1, got %d", state.Focus)
	}

	// Up arrow should move to issue list (focus 0)
	upMsg := tea.KeyPressMsg{Code: 0, Text: keys.Up}
	state.Update(upMsg)
	if state.Focus != 0 {
		t.Errorf("expected up arrow to move from containers to issue list (focus 0), got %d", state.Focus)
	}

	// Down arrow from containers should wrap to issue list (focus 0)
	state.Focus = 1
	downMsg := tea.KeyPressMsg{Code: 0, Text: keys.Down}
	state.Update(downMsg)
	if state.Focus != 0 {
		t.Errorf("expected down arrow from containers to wrap to issue list (focus 0), got %d", state.Focus)
	}
}

func TestImportIssuesState_WithoutContainerSupport(t *testing.T) {
	// When containers are not supported, the checkboxes should not appear
	state := NewImportIssuesState("/repo/path", "test-repo", false, false)
	state.SetIssues([]IssueItem{
		{ID: "1", Title: "Issue 1", Source: "github"},
	})

	// Tab should not cycle through container checkboxes
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
}
