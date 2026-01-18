package modals

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// =============================================================================
// HelpState Tests
// =============================================================================

func TestNewHelpStateFromSections(t *testing.T) {
	sections := []HelpSection{
		{
			Title: "Navigation",
			Shortcuts: []HelpShortcut{
				{Key: "tab", Desc: "switch pane"},
				{Key: "up/down", Desc: "navigate"},
			},
		},
		{
			Title: "Actions",
			Shortcuts: []HelpShortcut{
				{Key: "enter", Desc: "confirm"},
				{Key: "esc", Desc: "cancel"},
			},
		},
	}

	state := NewHelpStateFromSections(sections)

	if len(state.Sections) != 2 {
		t.Errorf("expected 2 sections, got %d", len(state.Sections))
	}

	if len(state.FlatShortcuts) != 4 {
		t.Errorf("expected 4 flattened shortcuts, got %d", len(state.FlatShortcuts))
	}

	if state.SelectedIndex != 0 {
		t.Errorf("expected initial selected index to be 0, got %d", state.SelectedIndex)
	}

	if state.ScrollOffset != 0 {
		t.Errorf("expected initial scroll offset to be 0, got %d", state.ScrollOffset)
	}
}

func TestHelpState_Title(t *testing.T) {
	state := &HelpState{}
	if state.Title() != "Keyboard Shortcuts" {
		t.Errorf("expected title 'Keyboard Shortcuts', got '%s'", state.Title())
	}
}

func TestHelpState_Help(t *testing.T) {
	state := &HelpState{}
	help := state.Help()
	if help == "" {
		t.Error("expected non-empty help text")
	}
}

func TestHelpState_Update_Navigation(t *testing.T) {
	sections := []HelpSection{
		{
			Title: "Test",
			Shortcuts: []HelpShortcut{
				{Key: "a", Desc: "action a"},
				{Key: "b", Desc: "action b"},
				{Key: "c", Desc: "action c"},
			},
		},
	}
	state := NewHelpStateFromSections(sections)

	// Test down navigation
	keyDownMsg := tea.KeyPressMsg{Code: 0, Text: "down"}
	newState, _ := state.Update(keyDownMsg)
	if s, ok := newState.(*HelpState); ok {
		if s.SelectedIndex != 1 {
			t.Errorf("expected selected index 1 after down, got %d", s.SelectedIndex)
		}
	}

	// Test up navigation
	keyUpMsg := tea.KeyPressMsg{Code: 0, Text: "up"}
	newState, _ = state.Update(keyUpMsg)
	if s, ok := newState.(*HelpState); ok {
		if s.SelectedIndex != 0 {
			t.Errorf("expected selected index 0 after up, got %d", s.SelectedIndex)
		}
	}

	// Test up at start (should stay at 0)
	newState, _ = state.Update(keyUpMsg)
	if s, ok := newState.(*HelpState); ok {
		if s.SelectedIndex != 0 {
			t.Errorf("expected selected index to stay 0 when at start, got %d", s.SelectedIndex)
		}
	}
}

func TestHelpState_Update_NavigationBounds(t *testing.T) {
	sections := []HelpSection{
		{
			Title: "Test",
			Shortcuts: []HelpShortcut{
				{Key: "a", Desc: "action a"},
				{Key: "b", Desc: "action b"},
			},
		},
	}
	state := NewHelpStateFromSections(sections)

	// Navigate to the end
	keyDownMsg := tea.KeyPressMsg{Code: 0, Text: "down"}
	state.Update(keyDownMsg) // Now at 1

	// Try to go past the end
	state.Update(keyDownMsg)
	if state.SelectedIndex != 1 {
		t.Errorf("expected selected index to stay at 1 when at end, got %d", state.SelectedIndex)
	}
}

func TestHelpState_GetSelectedShortcut(t *testing.T) {
	sections := []HelpSection{
		{
			Title: "Test",
			Shortcuts: []HelpShortcut{
				{Key: "a", Desc: "action a"},
				{Key: "b", Desc: "action b"},
			},
		},
	}
	state := NewHelpStateFromSections(sections)

	shortcut := state.GetSelectedShortcut()
	if shortcut == nil {
		t.Fatal("expected non-nil shortcut")
	}
	if shortcut.Key != "a" {
		t.Errorf("expected key 'a', got '%s'", shortcut.Key)
	}

	// Navigate and check again
	keyDownMsg := tea.KeyPressMsg{Code: 0, Text: "down"}
	state.Update(keyDownMsg)

	shortcut = state.GetSelectedShortcut()
	if shortcut == nil {
		t.Fatal("expected non-nil shortcut after navigation")
	}
	if shortcut.Key != "b" {
		t.Errorf("expected key 'b', got '%s'", shortcut.Key)
	}
}

func TestHelpState_GetSelectedShortcut_Empty(t *testing.T) {
	state := &HelpState{
		FlatShortcuts: []HelpShortcut{},
		SelectedIndex: 0,
	}

	shortcut := state.GetSelectedShortcut()
	if shortcut != nil {
		t.Error("expected nil shortcut for empty list")
	}
}

func TestHelpState_Render(t *testing.T) {
	sections := []HelpSection{
		{
			Title: "Navigation",
			Shortcuts: []HelpShortcut{
				{Key: "tab", Desc: "switch pane"},
			},
		},
	}
	state := NewHelpStateFromSections(sections)

	rendered := state.Render()
	if rendered == "" {
		t.Error("expected non-empty rendered output")
	}
}

// =============================================================================
// AddRepoState Tests
// =============================================================================

func TestNewAddRepoState_WithSuggestion(t *testing.T) {
	state := NewAddRepoState("/some/path")

	if state.SuggestedRepo != "/some/path" {
		t.Errorf("expected suggested repo '/some/path', got '%s'", state.SuggestedRepo)
	}
	if !state.UseSuggested {
		t.Error("expected UseSuggested to be true when suggestion provided")
	}
}

func TestNewAddRepoState_NoSuggestion(t *testing.T) {
	state := NewAddRepoState("")

	if state.SuggestedRepo != "" {
		t.Errorf("expected empty suggested repo, got '%s'", state.SuggestedRepo)
	}
	if state.UseSuggested {
		t.Error("expected UseSuggested to be false when no suggestion")
	}
}

func TestAddRepoState_Title(t *testing.T) {
	state := NewAddRepoState("")
	if state.Title() != "Add Repository" {
		t.Errorf("expected title 'Add Repository', got '%s'", state.Title())
	}
}

func TestAddRepoState_GetPath_UseSuggested(t *testing.T) {
	state := NewAddRepoState("/suggested/path")
	state.UseSuggested = true

	path := state.GetPath()
	if path != "/suggested/path" {
		t.Errorf("expected path '/suggested/path', got '%s'", path)
	}
}

func TestAddRepoState_GetPath_UseInput(t *testing.T) {
	state := NewAddRepoState("/suggested/path")
	state.UseSuggested = false
	state.Input.SetValue("/custom/path")

	path := state.GetPath()
	if path != "/custom/path" {
		t.Errorf("expected path '/custom/path', got '%s'", path)
	}
}

func TestAddRepoState_Update_ToggleSuggestion(t *testing.T) {
	state := NewAddRepoState("/suggested/path")
	state.UseSuggested = true

	// Press down to toggle
	keyDownMsg := tea.KeyPressMsg{Code: 0, Text: "down"}
	state.Update(keyDownMsg)

	if state.UseSuggested {
		t.Error("expected UseSuggested to toggle to false")
	}

	// Press up to toggle back
	keyUpMsg := tea.KeyPressMsg{Code: 0, Text: "up"}
	state.Update(keyUpMsg)

	if !state.UseSuggested {
		t.Error("expected UseSuggested to toggle back to true")
	}
}

func TestAddRepoState_Render(t *testing.T) {
	state := NewAddRepoState("/test/path")
	rendered := state.Render()

	if rendered == "" {
		t.Error("expected non-empty rendered output")
	}
}

func TestAddRepoState_IsShowingOptions(t *testing.T) {
	state := NewAddRepoState("")

	if state.IsShowingOptions() {
		t.Error("expected IsShowingOptions to be false initially")
	}

	state.showingOptions = true
	if !state.IsShowingOptions() {
		t.Error("expected IsShowingOptions to be true after setting")
	}
}

// =============================================================================
// SelectRepoForIssuesState Tests
// =============================================================================

func TestNewSelectRepoForIssuesState(t *testing.T) {
	repos := []string{"/repo1", "/repo2", "/repo3"}
	state := NewSelectRepoForIssuesState(repos)

	if len(state.RepoOptions) != 3 {
		t.Errorf("expected 3 repos, got %d", len(state.RepoOptions))
	}
	if state.RepoIndex != 0 {
		t.Errorf("expected initial index 0, got %d", state.RepoIndex)
	}
}

func TestSelectRepoForIssuesState_Title(t *testing.T) {
	state := NewSelectRepoForIssuesState(nil)
	if state.Title() != "Select Repository" {
		t.Errorf("expected title 'Select Repository', got '%s'", state.Title())
	}
}

func TestSelectRepoForIssuesState_GetSelectedRepo(t *testing.T) {
	repos := []string{"/repo1", "/repo2"}
	state := NewSelectRepoForIssuesState(repos)

	selected := state.GetSelectedRepo()
	if selected != "/repo1" {
		t.Errorf("expected '/repo1', got '%s'", selected)
	}

	state.RepoIndex = 1
	selected = state.GetSelectedRepo()
	if selected != "/repo2" {
		t.Errorf("expected '/repo2', got '%s'", selected)
	}
}

func TestSelectRepoForIssuesState_GetSelectedRepo_Empty(t *testing.T) {
	state := NewSelectRepoForIssuesState([]string{})

	selected := state.GetSelectedRepo()
	if selected != "" {
		t.Errorf("expected empty string for empty list, got '%s'", selected)
	}
}

func TestSelectRepoForIssuesState_Update_Navigation(t *testing.T) {
	repos := []string{"/repo1", "/repo2", "/repo3"}
	state := NewSelectRepoForIssuesState(repos)

	// Navigate down
	keyDownMsg := tea.KeyPressMsg{Code: 0, Text: "down"}
	state.Update(keyDownMsg)
	if state.RepoIndex != 1 {
		t.Errorf("expected index 1 after down, got %d", state.RepoIndex)
	}

	// Navigate up
	keyUpMsg := tea.KeyPressMsg{Code: 0, Text: "up"}
	state.Update(keyUpMsg)
	if state.RepoIndex != 0 {
		t.Errorf("expected index 0 after up, got %d", state.RepoIndex)
	}

	// Up at start should stay at 0
	state.Update(keyUpMsg)
	if state.RepoIndex != 0 {
		t.Errorf("expected index to stay 0 at start, got %d", state.RepoIndex)
	}
}

func TestSelectRepoForIssuesState_Render(t *testing.T) {
	repos := []string{"/repo1", "/repo2"}
	state := NewSelectRepoForIssuesState(repos)

	rendered := state.Render()
	if rendered == "" {
		t.Error("expected non-empty rendered output")
	}
}

func TestSelectRepoForIssuesState_Render_Empty(t *testing.T) {
	state := NewSelectRepoForIssuesState([]string{})

	rendered := state.Render()
	if rendered == "" {
		t.Error("expected non-empty rendered output even with no repos")
	}
}

// =============================================================================
// Helper function tests
// =============================================================================

func TestFormatInt(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
		{9999, "9999"},
	}

	for _, tt := range tests {
		result := formatInt(tt.input)
		if result != tt.expected {
			t.Errorf("formatInt(%d) = %s, expected %s", tt.input, result, tt.expected)
		}
	}
}

// =============================================================================
// Type assertion tests - ensure all modal states implement ModalState
// =============================================================================

func TestModalStateInterface(t *testing.T) {
	// These compile-time checks verify interface implementation
	var _ ModalState = (*HelpState)(nil)
	var _ ModalState = (*AddRepoState)(nil)
	var _ ModalState = (*SelectRepoForIssuesState)(nil)
}
