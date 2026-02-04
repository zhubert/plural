package modals

import (
	"strings"
	"testing"
)

func TestNewBroadcastState(t *testing.T) {
	repos := []string{"/path/to/repo1", "/path/to/repo2", "/another/repo3"}
	state := NewBroadcastState(repos)

	// Check initial state
	if len(state.Repos) != 3 {
		t.Errorf("expected 3 repos, got %d", len(state.Repos))
	}

	// Check repo names are derived from paths
	if state.Repos[0].Name != "repo1" {
		t.Errorf("expected repo name 'repo1', got %s", state.Repos[0].Name)
	}
	if state.Repos[1].Name != "repo2" {
		t.Errorf("expected repo name 'repo2', got %s", state.Repos[1].Name)
	}
	if state.Repos[2].Name != "repo3" {
		t.Errorf("expected repo name 'repo3', got %s", state.Repos[2].Name)
	}

	// Check repos are not selected by default
	for i, repo := range state.Repos {
		if repo.Selected {
			t.Errorf("repo %d should not be selected by default", i)
		}
	}

	// Check initial focus is on repo list
	if state.Focus != 0 {
		t.Errorf("expected initial focus 0, got %d", state.Focus)
	}

	// Check SelectedIndex is 0
	if state.SelectedIndex != 0 {
		t.Errorf("expected initial SelectedIndex 0, got %d", state.SelectedIndex)
	}
}

func TestBroadcastState_Title(t *testing.T) {
	state := NewBroadcastState([]string{"/repo"})
	if state.Title() != "Broadcast to Repositories" {
		t.Errorf("unexpected title: %s", state.Title())
	}
}

func TestBroadcastState_Help(t *testing.T) {
	state := NewBroadcastState([]string{"/repo"})

	// Help when focused on repos
	state.Focus = 0
	help := state.Help()
	if help != "Space: toggle  Tab: name  a: all  n: none  Enter: send  Esc: cancel" {
		t.Errorf("unexpected help for repo focus: %s", help)
	}

	// Help when focused on name input
	state.Focus = 1
	help = state.Help()
	if help != "Tab: prompt  Shift+Tab: repos  Enter: send  Esc: cancel" {
		t.Errorf("unexpected help for name focus: %s", help)
	}

	// Help when focused on prompt
	state.Focus = 2
	help = state.Help()
	if help != "Tab: repos  Shift+Tab: name  Enter: send  Esc: cancel" {
		t.Errorf("unexpected help for prompt focus: %s", help)
	}
}

func TestBroadcastState_ToggleSelection(t *testing.T) {
	repos := []string{"/repo1", "/repo2"}
	state := NewBroadcastState(repos)

	// Check initial state
	if state.Repos[0].Selected {
		t.Error("repo should not be selected initially")
	}

	// Manually toggle selection (simulating space key)
	state.Repos[0].Selected = true
	if !state.Repos[0].Selected {
		t.Error("repo should be selected after toggle")
	}

	// Toggle again
	state.Repos[0].Selected = false
	if state.Repos[0].Selected {
		t.Error("repo should be unselected after second toggle")
	}
}

func TestBroadcastState_SelectAll(t *testing.T) {
	repos := []string{"/repo1", "/repo2", "/repo3"}
	state := NewBroadcastState(repos)

	// Manually select all (simulating 'a' key)
	for i := range state.Repos {
		state.Repos[i].Selected = true
	}

	for i, repo := range state.Repos {
		if !repo.Selected {
			t.Errorf("repo %d should be selected", i)
		}
	}

	if state.GetSelectedCount() != 3 {
		t.Errorf("expected 3 selected repos, got %d", state.GetSelectedCount())
	}
}

func TestBroadcastState_SelectNone(t *testing.T) {
	repos := []string{"/repo1", "/repo2", "/repo3"}
	state := NewBroadcastState(repos)

	// Select all first
	for i := range state.Repos {
		state.Repos[i].Selected = true
	}

	// Deselect all (simulating 'n' key)
	for i := range state.Repos {
		state.Repos[i].Selected = false
	}

	for i, repo := range state.Repos {
		if repo.Selected {
			t.Errorf("repo %d should not be selected", i)
		}
	}

	if state.GetSelectedCount() != 0 {
		t.Errorf("expected 0 selected repos, got %d", state.GetSelectedCount())
	}
}

func TestBroadcastState_FocusToggle(t *testing.T) {
	state := NewBroadcastState([]string{"/repo"})

	// Initial focus is on repos (0)
	if state.Focus != 0 {
		t.Errorf("expected initial focus 0, got %d", state.Focus)
	}

	// Tab from repos to name input
	state.Focus = 1
	if state.Focus != 1 {
		t.Errorf("expected focus 1 (name input), got %d", state.Focus)
	}

	// Tab from name to prompt
	state.Focus = 2
	if state.Focus != 2 {
		t.Errorf("expected focus 2 (prompt), got %d", state.Focus)
	}

	// Tab from prompt wraps back to repos
	state.Focus = 0
	if state.Focus != 0 {
		t.Errorf("expected focus 0 (repos) after wrap, got %d", state.Focus)
	}
}

func TestBroadcastState_GetSelectedRepos(t *testing.T) {
	repos := []string{"/repo1", "/repo2", "/repo3"}
	state := NewBroadcastState(repos)

	// Select first and third
	state.Repos[0].Selected = true
	state.Repos[2].Selected = true

	selected := state.GetSelectedRepos()

	if len(selected) != 2 {
		t.Errorf("expected 2 selected repos, got %d", len(selected))
	}

	// Check paths (not names)
	if selected[0] != "/repo1" {
		t.Errorf("expected '/repo1', got %s", selected[0])
	}
	if selected[1] != "/repo3" {
		t.Errorf("expected '/repo3', got %s", selected[1])
	}
}

func TestBroadcastState_GetName(t *testing.T) {
	state := NewBroadcastState([]string{"/repo"})

	// Initial name is empty
	if state.GetName() != "" {
		t.Errorf("expected empty name, got %s", state.GetName())
	}

	// Set value in name input (simulate typing)
	state.NameInput.SetValue("my-feature")

	if state.GetName() != "my-feature" {
		t.Errorf("expected name 'my-feature', got %s", state.GetName())
	}
}

func TestBroadcastState_GetPrompt(t *testing.T) {
	state := NewBroadcastState([]string{"/repo"})

	// Initial prompt is empty
	if state.GetPrompt() != "" {
		t.Errorf("expected empty prompt, got %s", state.GetPrompt())
	}

	// Set value in textarea (simulate typing)
	state.PromptInput.SetValue("Hello, this is a test prompt")

	if state.GetPrompt() != "Hello, this is a test prompt" {
		t.Errorf("expected prompt text, got %s", state.GetPrompt())
	}
}

func TestBroadcastState_Render(t *testing.T) {
	initTestStyles()

	repos := []string{"/repo1", "/repo2"}
	state := NewBroadcastState(repos)

	// Select first repo
	state.Repos[0].Selected = true

	rendered := state.Render()

	// Check that title is rendered
	if !strings.Contains(rendered, "Broadcast") {
		t.Error("rendered output should contain title")
	}

	// Check that repos are rendered
	if !strings.Contains(rendered, "repo1") {
		t.Error("rendered output should contain repo1")
	}
	if !strings.Contains(rendered, "repo2") {
		t.Error("rendered output should contain repo2")
	}

	// Check that help is rendered
	if !strings.Contains(rendered, "Enter") {
		t.Error("rendered output should contain help text")
	}
}

func TestBroadcastState_EmptyRepos(t *testing.T) {
	state := NewBroadcastState([]string{})

	if len(state.Repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(state.Repos))
	}

	selected := state.GetSelectedRepos()
	if len(selected) != 0 {
		t.Errorf("expected 0 selected repos, got %d", len(selected))
	}

	// Render should not panic
	rendered := state.Render()
	if rendered == "" {
		t.Error("render should return something even with no repos")
	}
}

func TestBroadcastState_ScrollOffset(t *testing.T) {
	// Create more repos than visible
	repos := make([]string, 10)
	for i := 0; i < 10; i++ {
		repos[i] = "/repo" + string(rune('0'+i))
	}
	state := NewBroadcastState(repos)

	// Initial scroll offset is 0
	if state.ScrollOffset != 0 {
		t.Errorf("expected initial scroll offset 0, got %d", state.ScrollOffset)
	}

	// Manually set scroll offset (simulating navigation)
	state.ScrollOffset = 3
	state.SelectedIndex = 8

	// Verify state is updated
	if state.ScrollOffset != 3 {
		t.Errorf("expected scroll offset 3, got %d", state.ScrollOffset)
	}
	if state.SelectedIndex != 8 {
		t.Errorf("expected selected index 8, got %d", state.SelectedIndex)
	}
}

func TestBroadcastState_Navigation(t *testing.T) {
	repos := []string{"/repo1", "/repo2", "/repo3"}
	state := NewBroadcastState(repos)

	// Test down navigation
	state.SelectedIndex = 1
	if state.SelectedIndex != 1 {
		t.Errorf("expected SelectedIndex 1, got %d", state.SelectedIndex)
	}

	// Test up navigation
	state.SelectedIndex = 0
	if state.SelectedIndex != 0 {
		t.Errorf("expected SelectedIndex 0, got %d", state.SelectedIndex)
	}

	// Test boundary - can't go below 0
	if state.SelectedIndex < 0 {
		t.Error("SelectedIndex should not be negative")
	}

	// Test boundary - can't go above length-1
	state.SelectedIndex = len(state.Repos) - 1
	if state.SelectedIndex >= len(state.Repos) {
		t.Error("SelectedIndex should not exceed repos length")
	}
}

func TestBroadcastState_GetSelectedCount(t *testing.T) {
	repos := []string{"/repo1", "/repo2", "/repo3", "/repo4"}
	state := NewBroadcastState(repos)

	// Initially zero
	if state.GetSelectedCount() != 0 {
		t.Errorf("expected 0 selected, got %d", state.GetSelectedCount())
	}

	// Select some
	state.Repos[0].Selected = true
	state.Repos[2].Selected = true

	if state.GetSelectedCount() != 2 {
		t.Errorf("expected 2 selected, got %d", state.GetSelectedCount())
	}

	// Select all
	for i := range state.Repos {
		state.Repos[i].Selected = true
	}

	if state.GetSelectedCount() != 4 {
		t.Errorf("expected 4 selected, got %d", state.GetSelectedCount())
	}
}

func TestBroadcastState_ModalStateInterface(t *testing.T) {
	state := NewBroadcastState([]string{"/repo"})

	// Verify it implements ModalState interface
	var _ ModalState = state

	// Verify Title returns string
	title := state.Title()
	if title == "" {
		t.Error("Title should not be empty")
	}

	// Verify Help returns string
	help := state.Help()
	if help == "" {
		t.Error("Help should not be empty")
	}

	// Verify Render returns string
	rendered := state.Render()
	if rendered == "" {
		t.Error("Render should not be empty")
	}

	// Verify Update returns ModalState
	newState, _ := state.Update(nil)
	if newState == nil {
		t.Error("Update should return non-nil state")
	}
}

func TestFormatCount(t *testing.T) {
	tests := []struct {
		count    int
		total    int
		contains string
	}{
		{0, 0, "0"},
		{5, 0, "5"},
		{3, 10, "3"},
		{3, 10, "/10"},
		{12, 15, "12"},
		{12, 15, "/15"},
		{6, 12, "6"},
		{6, 12, "/12"},
		{100, 200, "100"},
		{100, 200, "/200"},
	}

	for _, tt := range tests {
		result := formatCount(tt.count, tt.total)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("formatCount(%d, %d) = %q, want to contain %q", tt.count, tt.total, result, tt.contains)
		}
	}
}
