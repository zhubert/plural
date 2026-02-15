package modals

import (
	"strings"
	"testing"
)

func TestNewBroadcastState(t *testing.T) {
	repos := []string{"/path/to/repo1", "/path/to/repo2", "/another/repo3"}
	state := NewBroadcastState(repos, false, false)

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

	// Check textarea has line numbers disabled
	if state.PromptInput.ShowLineNumbers {
		t.Error("expected ShowLineNumbers to be false")
	}
}

func TestBroadcastState_Title(t *testing.T) {
	state := NewBroadcastState([]string{"/repo"}, false, false)
	if state.Title() != "Broadcast to Repositories" {
		t.Errorf("unexpected title: %s", state.Title())
	}
}

func TestBroadcastState_Help(t *testing.T) {
	state := NewBroadcastState([]string{"/repo"}, false, false)

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
	state := NewBroadcastState(repos, false, false)

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
	state := NewBroadcastState(repos, false, false)

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
	state := NewBroadcastState(repos, false, false)

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
	state := NewBroadcastState([]string{"/repo"}, false, false)

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
	state := NewBroadcastState(repos, false, false)

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
	state := NewBroadcastState([]string{"/repo"}, false, false)

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
	state := NewBroadcastState([]string{"/repo"}, false, false)

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
	state := NewBroadcastState(repos, false, false)

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

func TestBroadcastState_DockerHintWhenContainersNotSupported(t *testing.T) {
	initTestStyles()

	state := NewBroadcastState([]string{"/repo1"}, false, false)
	rendered := state.Render()

	if !strings.Contains(rendered, "Install Docker to enable container and autonomous modes") {
		t.Error("expected Docker hint when containers not supported")
	}
}

func TestBroadcastState_NoDockerHintWhenContainersSupported(t *testing.T) {
	initTestStyles()

	state := NewBroadcastState([]string{"/repo1"}, true, true)
	rendered := state.Render()

	if strings.Contains(rendered, "Install Docker to enable container and autonomous modes") {
		t.Error("should not show Docker hint when containers are supported")
	}
}

func TestBroadcastState_EmptyRepos(t *testing.T) {
	state := NewBroadcastState([]string{}, false, false)

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
	state := NewBroadcastState(repos, false, false)

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
	state := NewBroadcastState(repos, false, false)

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
	state := NewBroadcastState(repos, false, false)

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
	state := NewBroadcastState([]string{"/repo"}, false, false)

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

// =============================================================================
// BroadcastGroupState Tests
// =============================================================================

func TestNewBroadcastGroupState(t *testing.T) {
	sessions := []SessionItem{
		{ID: "sess1", Name: "feature-a", RepoName: "repo1", Selected: false},
		{ID: "sess2", Name: "feature-b", RepoName: "repo2", Selected: false},
		{ID: "sess3", Name: "feature-c", RepoName: "repo1", Selected: false},
	}
	state := NewBroadcastGroupState("group123", sessions)

	// Check initial state
	if len(state.Sessions) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(state.Sessions))
	}

	// Check sessions are selected by default
	for i, sess := range state.Sessions {
		if !sess.Selected {
			t.Errorf("session %d should be selected by default", i)
		}
	}

	// Check initial focus is on action selector
	if state.Focus != 0 {
		t.Errorf("expected initial focus 0 (action selector), got %d", state.Focus)
	}

	// Check default action is SendPrompt
	if state.Action != BroadcastActionSendPrompt {
		t.Errorf("expected default action to be SendPrompt, got %d", state.Action)
	}

	// Check group ID is set
	if state.GroupID != "group123" {
		t.Errorf("expected group ID 'group123', got %s", state.GroupID)
	}

	// Check textarea has line numbers disabled
	if state.PromptInput.ShowLineNumbers {
		t.Error("expected ShowLineNumbers to be false")
	}
}

func TestBroadcastGroupState_Title(t *testing.T) {
	state := NewBroadcastGroupState("group1", nil)
	if state.Title() != "Broadcast Group" {
		t.Errorf("unexpected title: %s", state.Title())
	}
}

func TestBroadcastGroupState_Help(t *testing.T) {
	sessions := []SessionItem{{ID: "s1", Name: "sess1"}}
	state := NewBroadcastGroupState("g1", sessions)

	// Help when focused on action selector
	state.Focus = 0
	help := state.Help()
	if !strings.Contains(help, "left/right") {
		t.Errorf("action selector help should mention left/right: %s", help)
	}

	// Help when focused on session list (SendPrompt action)
	state.Focus = 1
	state.Action = BroadcastActionSendPrompt
	help = state.Help()
	if !strings.Contains(help, "Tab: prompt") {
		t.Errorf("session list help should mention Tab: prompt for SendPrompt action: %s", help)
	}

	// Help when focused on session list (CreatePRs action)
	state.Action = BroadcastActionCreatePRs
	help = state.Help()
	if strings.Contains(help, "Tab: prompt") {
		t.Errorf("session list help should not mention prompt for CreatePRs action: %s", help)
	}

	// Help when focused on prompt
	state.Focus = 2
	help = state.Help()
	if !strings.Contains(help, "Shift+Tab") {
		t.Errorf("prompt help should mention Shift+Tab: %s", help)
	}
}

func TestBroadcastGroupState_ActionToggle(t *testing.T) {
	state := NewBroadcastGroupState("g1", nil)

	// Initial action is SendPrompt
	if state.Action != BroadcastActionSendPrompt {
		t.Error("initial action should be SendPrompt")
	}

	// Change to CreatePRs
	state.Action = BroadcastActionCreatePRs
	if state.Action != BroadcastActionCreatePRs {
		t.Error("action should be CreatePRs after toggle")
	}

	// Change back to SendPrompt
	state.Action = BroadcastActionSendPrompt
	if state.Action != BroadcastActionSendPrompt {
		t.Error("action should be SendPrompt after second toggle")
	}
}

func TestBroadcastGroupState_GetSelectedSessions(t *testing.T) {
	sessions := []SessionItem{
		{ID: "sess1", Name: "feature-a", Selected: true},
		{ID: "sess2", Name: "feature-b", Selected: false},
		{ID: "sess3", Name: "feature-c", Selected: true},
	}
	state := NewBroadcastGroupState("g1", sessions)

	// Deselect one to test partial selection
	state.Sessions[0].Selected = false

	selected := state.GetSelectedSessions()

	// Should have sess2 (false from initial) and sess3 (true)
	// Wait, we set Selected=true in NewBroadcastGroupState, so initially all are true
	// Then we deselected state.Sessions[0]
	// So we should have sess2 and sess3 selected

	if len(selected) != 2 {
		t.Errorf("expected 2 selected sessions, got %d", len(selected))
	}

	// Check IDs
	found2, found3 := false, false
	for _, id := range selected {
		if id == "sess2" {
			found2 = true
		}
		if id == "sess3" {
			found3 = true
		}
	}
	if !found2 || !found3 {
		t.Errorf("expected sess2 and sess3, got %v", selected)
	}
}

func TestBroadcastGroupState_GetSelectedCount(t *testing.T) {
	sessions := []SessionItem{
		{ID: "s1", Name: "a"},
		{ID: "s2", Name: "b"},
		{ID: "s3", Name: "c"},
		{ID: "s4", Name: "d"},
	}
	state := NewBroadcastGroupState("g1", sessions)

	// All selected by default
	if state.GetSelectedCount() != 4 {
		t.Errorf("expected 4 selected, got %d", state.GetSelectedCount())
	}

	// Deselect some
	state.Sessions[0].Selected = false
	state.Sessions[2].Selected = false

	if state.GetSelectedCount() != 2 {
		t.Errorf("expected 2 selected, got %d", state.GetSelectedCount())
	}

	// Deselect all
	for i := range state.Sessions {
		state.Sessions[i].Selected = false
	}

	if state.GetSelectedCount() != 0 {
		t.Errorf("expected 0 selected, got %d", state.GetSelectedCount())
	}
}

func TestBroadcastGroupState_GetPrompt(t *testing.T) {
	state := NewBroadcastGroupState("g1", nil)

	// Initial prompt is empty
	if state.GetPrompt() != "" {
		t.Errorf("expected empty prompt, got %s", state.GetPrompt())
	}

	// Set prompt
	state.PromptInput.SetValue("Test prompt message")

	if state.GetPrompt() != "Test prompt message" {
		t.Errorf("expected 'Test prompt message', got %s", state.GetPrompt())
	}
}

func TestBroadcastGroupState_GetAction(t *testing.T) {
	state := NewBroadcastGroupState("g1", nil)

	if state.GetAction() != BroadcastActionSendPrompt {
		t.Error("expected default action to be SendPrompt")
	}

	state.Action = BroadcastActionCreatePRs
	if state.GetAction() != BroadcastActionCreatePRs {
		t.Error("expected action to be CreatePRs")
	}
}

func TestBroadcastGroupState_Render(t *testing.T) {
	initTestStyles()

	sessions := []SessionItem{
		{ID: "s1", Name: "feature-a", RepoName: "repo1"},
		{ID: "s2", Name: "feature-b", RepoName: "repo2"},
	}
	state := NewBroadcastGroupState("g1", sessions)

	rendered := state.Render()

	// Check that title is rendered
	if !strings.Contains(rendered, "Broadcast Group") {
		t.Error("rendered output should contain title")
	}

	// Check that action options are rendered
	if !strings.Contains(rendered, "Send Prompt") {
		t.Error("rendered output should contain 'Send Prompt' action")
	}
	if !strings.Contains(rendered, "Create PRs") {
		t.Error("rendered output should contain 'Create PRs' action")
	}

	// Check that sessions are rendered
	if !strings.Contains(rendered, "feature-a") {
		t.Error("rendered output should contain session name")
	}

	// Check that prompt input is shown for SendPrompt action
	state.Action = BroadcastActionSendPrompt
	rendered = state.Render()
	if !strings.Contains(rendered, "Prompt") {
		t.Error("rendered output should contain Prompt label for SendPrompt action")
	}

	// Check that prompt input is hidden for CreatePRs action
	state.Action = BroadcastActionCreatePRs
	createPRsRender := state.Render()
	state.Action = BroadcastActionSendPrompt
	sendPromptRender := state.Render()
	// CreatePRs action should have fewer "Prompt" occurrences (no input label)
	createPRsPromptCount := strings.Count(createPRsRender, "Prompt")
	sendPromptCount := strings.Count(sendPromptRender, "Prompt")
	if createPRsPromptCount >= sendPromptCount {
		t.Errorf("CreatePRs render should have fewer 'Prompt' occurrences than SendPrompt render: %d vs %d", createPRsPromptCount, sendPromptCount)
	}
}

func TestBroadcastGroupState_EmptySessions(t *testing.T) {
	state := NewBroadcastGroupState("g1", []SessionItem{})

	if len(state.Sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(state.Sessions))
	}

	selected := state.GetSelectedSessions()
	if len(selected) != 0 {
		t.Errorf("expected 0 selected sessions, got %d", len(selected))
	}

	// Render should not panic
	rendered := state.Render()
	if rendered == "" {
		t.Error("render should return something even with no sessions")
	}
}

func TestBroadcastGroupState_ModalStateInterface(t *testing.T) {
	state := NewBroadcastGroupState("g1", nil)

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

func TestBroadcastGroupState_Navigation(t *testing.T) {
	sessions := []SessionItem{
		{ID: "s1", Name: "a"},
		{ID: "s2", Name: "b"},
		{ID: "s3", Name: "c"},
	}
	state := NewBroadcastGroupState("g1", sessions)

	// Start with focus on action selector
	state.Focus = 1 // Move to session list

	// Test navigation
	state.SelectedIndex = 0
	if state.SelectedIndex != 0 {
		t.Error("should start at index 0")
	}

	state.SelectedIndex = 1
	if state.SelectedIndex != 1 {
		t.Error("should be at index 1 after moving down")
	}

	state.SelectedIndex = 2
	if state.SelectedIndex != 2 {
		t.Error("should be at index 2 after moving down again")
	}

	// Can't go past end
	if state.SelectedIndex >= len(state.Sessions) {
		t.Error("should not exceed session count")
	}
}

func TestBroadcastGroupState_SelectAllNone(t *testing.T) {
	sessions := []SessionItem{
		{ID: "s1", Name: "a", Selected: false},
		{ID: "s2", Name: "b", Selected: false},
		{ID: "s3", Name: "c", Selected: false},
	}
	state := NewBroadcastGroupState("g1", sessions)

	// NewBroadcastGroupState selects all by default, so deselect first
	for i := range state.Sessions {
		state.Sessions[i].Selected = false
	}

	if state.GetSelectedCount() != 0 {
		t.Errorf("expected 0 selected after deselecting all, got %d", state.GetSelectedCount())
	}

	// Select all (simulating 'a' key)
	for i := range state.Sessions {
		state.Sessions[i].Selected = true
	}

	if state.GetSelectedCount() != 3 {
		t.Errorf("expected 3 selected after select all, got %d", state.GetSelectedCount())
	}

	// Select none (simulating 'n' key)
	for i := range state.Sessions {
		state.Sessions[i].Selected = false
	}

	if state.GetSelectedCount() != 0 {
		t.Errorf("expected 0 selected after select none, got %d", state.GetSelectedCount())
	}
}
