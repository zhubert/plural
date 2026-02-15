package modals

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/keys"
)

func makeRepos(n int) []string {
	repos := make([]string, n)
	for i := 0; i < n; i++ {
		repos[i] = fmt.Sprintf("/path/to/repo%d", i)
	}
	return repos
}

func TestNewSessionState_ScrollOffsetAdjustsOnNavigateDown(t *testing.T) {
	repos := makeRepos(15)
	state := NewNewSessionState(repos, false, false)

	// Navigate down past visible area
	for i := 0; i < 12; i++ {
		state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Down})
	}

	if state.RepoIndex != 12 {
		t.Errorf("expected RepoIndex 12, got %d", state.RepoIndex)
	}
	// ScrollOffset should have adjusted so index 12 is visible
	// visible range: [ScrollOffset, ScrollOffset+10)
	// 12 >= ScrollOffset+10 triggers adjustment
	expectedOffset := 12 - NewSessionMaxVisibleRepos + 1 // 3
	if state.ScrollOffset != expectedOffset {
		t.Errorf("expected ScrollOffset %d, got %d", expectedOffset, state.ScrollOffset)
	}
}

func TestNewSessionState_ScrollOffsetAdjustsOnNavigateUp(t *testing.T) {
	repos := makeRepos(15)
	state := NewNewSessionState(repos, false, false)

	// Navigate down to index 12
	for i := 0; i < 12; i++ {
		state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Down})
	}

	// Now navigate back up past the visible area
	for i := 0; i < 12; i++ {
		state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Up})
	}

	if state.RepoIndex != 0 {
		t.Errorf("expected RepoIndex 0, got %d", state.RepoIndex)
	}
	if state.ScrollOffset != 0 {
		t.Errorf("expected ScrollOffset 0, got %d", state.ScrollOffset)
	}
}

func TestNewSessionState_NoScrollIndicatorsWhenFewRepos(t *testing.T) {
	initTestStyles()

	repos := makeRepos(5)
	state := NewNewSessionState(repos, false, false)

	rendered := state.Render()
	if strings.Contains(rendered, "more above") {
		t.Error("should not show 'more above' indicator with only 5 repos")
	}
	if strings.Contains(rendered, "more below") {
		t.Error("should not show 'more below' indicator with only 5 repos")
	}
}

func TestNewSessionState_ScrollIndicatorsWhenManyRepos(t *testing.T) {
	initTestStyles()

	repos := makeRepos(15)
	state := NewNewSessionState(repos, false, false)

	// At start, should have "more below" but not "more above"
	rendered := state.Render()
	if strings.Contains(rendered, "more above") {
		t.Error("should not show 'more above' at top of list")
	}
	if !strings.Contains(rendered, "more below") {
		t.Error("should show 'more below' when repos exceed visible area")
	}

	// Navigate to middle
	for i := 0; i < 12; i++ {
		state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Down})
	}

	rendered = state.Render()
	if !strings.Contains(rendered, "more above") {
		t.Error("should show 'more above' when scrolled down")
	}
	if !strings.Contains(rendered, "more below") {
		t.Error("should show 'more below' when not at bottom")
	}

	// Navigate to bottom
	for i := 12; i < 14; i++ {
		state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Down})
	}

	rendered = state.Render()
	if !strings.Contains(rendered, "more above") {
		t.Error("should show 'more above' when at bottom")
	}
	if strings.Contains(rendered, "more below") {
		t.Error("should not show 'more below' when at bottom of list")
	}
}

func TestNewSessionState_RenderContainsVisibleReposOnly(t *testing.T) {
	initTestStyles()

	repos := makeRepos(15)
	state := NewNewSessionState(repos, false, false)

	rendered := state.Render()

	// Visible repos (0-9) should be in rendered output
	for i := 0; i < NewSessionMaxVisibleRepos; i++ {
		if !strings.Contains(rendered, repos[i]) {
			t.Errorf("expected visible repo %q in rendered output", repos[i])
		}
	}
	// Off-screen repos (10-14) should NOT be in rendered output
	for i := NewSessionMaxVisibleRepos; i < 15; i++ {
		if strings.Contains(rendered, repos[i]) {
			t.Errorf("did not expect off-screen repo %q in rendered output", repos[i])
		}
	}
}

func TestNewSessionState_ExactlyMaxVisibleRepos(t *testing.T) {
	initTestStyles()

	repos := makeRepos(NewSessionMaxVisibleRepos)
	state := NewNewSessionState(repos, false, false)

	rendered := state.Render()

	// All repos should be visible
	for _, repo := range repos {
		if !strings.Contains(rendered, repo) {
			t.Errorf("expected repo %q in rendered output", repo)
		}
	}

	// No scroll indicators
	if strings.Contains(rendered, "more above") {
		t.Error("should not show 'more above' with exactly max visible repos")
	}
	if strings.Contains(rendered, "more below") {
		t.Error("should not show 'more below' with exactly max visible repos")
	}

	// ScrollOffset should stay 0
	if state.ScrollOffset != 0 {
		t.Errorf("expected ScrollOffset 0, got %d", state.ScrollOffset)
	}
}

func TestNewSessionState_ScrollOffsetInitializedToZero(t *testing.T) {
	state := NewNewSessionState(makeRepos(20), false, false)
	if state.ScrollOffset != 0 {
		t.Errorf("expected initial ScrollOffset 0, got %d", state.ScrollOffset)
	}
}

func TestNewSessionState_NavigateDownDoesNotExceedBounds(t *testing.T) {
	repos := makeRepos(3)
	state := NewNewSessionState(repos, false, false)

	// Navigate down more times than there are items
	for i := 0; i < 10; i++ {
		state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Down})
	}

	if state.RepoIndex != 2 {
		t.Errorf("expected RepoIndex 2 (last item), got %d", state.RepoIndex)
	}
	if state.ScrollOffset != 0 {
		t.Errorf("expected ScrollOffset 0 (no scrolling needed), got %d", state.ScrollOffset)
	}
}

func TestNewSessionState_NavigateUpDoesNotGoBelowZero(t *testing.T) {
	repos := makeRepos(3)
	state := NewNewSessionState(repos, false, false)

	// Navigate up from index 0
	for i := 0; i < 5; i++ {
		state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Up})
	}

	if state.RepoIndex != 0 {
		t.Errorf("expected RepoIndex 0, got %d", state.RepoIndex)
	}
	if state.ScrollOffset != 0 {
		t.Errorf("expected ScrollOffset 0, got %d", state.ScrollOffset)
	}
}

func TestNewSessionState_AutonomousDisablesBranchAndContainer(t *testing.T) {
	initTestStyles()

	state := NewNewSessionState(makeRepos(3), true, true)

	// Tab to autonomous (index 2) and enable it
	state.Focus = 2
	state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Space})

	if !state.Autonomous {
		t.Fatal("expected autonomous to be enabled")
	}
	if !state.UseContainers {
		t.Fatal("expected containers to be auto-enabled by autonomous")
	}

	// Branch input should have been cleared
	if state.BranchInput.Value() != "" {
		t.Errorf("expected branch input to be cleared, got %q", state.BranchInput.Value())
	}

	// Render should show disabled message for branch
	rendered := state.Render()
	if !strings.Contains(rendered, "disabled in autonomous mode") {
		t.Error("expected 'disabled in autonomous mode' text for branch input")
	}

	// Container description should mention autonomous
	if !strings.Contains(rendered, "autonomous") {
		t.Errorf("expected container description to mention autonomous, rendered:\n%s", rendered)
	}
}

func TestNewSessionState_AutonomousSkipsFocusOnTab(t *testing.T) {
	state := NewNewSessionState(makeRepos(3), true, true)

	// Enable autonomous mode
	state.Autonomous = true
	state.UseContainers = true

	// Start at focus 1 (base selection), tab forward
	state.Focus = 1
	state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Tab})

	// Should land on 2 (autonomous), skipping 3 (branch) and 4 (containers)
	if state.Focus != 2 {
		t.Errorf("expected focus to land on 2 (autonomous), got %d", state.Focus)
	}

	// Tab again should wrap to 0 (skipping 3 and 4)
	state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Tab})
	if state.Focus != 0 {
		t.Errorf("expected focus to wrap to 0, got %d", state.Focus)
	}
}

func TestNewSessionState_AutonomousSkipsFocusOnShiftTab(t *testing.T) {
	state := NewNewSessionState(makeRepos(3), true, true)

	// Enable autonomous mode
	state.Autonomous = true
	state.UseContainers = true

	// Start at focus 2 (autonomous), shift-tab backward
	state.Focus = 2
	state.Update(tea.KeyPressMsg{Code: -1, Text: keys.ShiftTab})

	// Should land on 1 (base selection)
	if state.Focus != 1 {
		t.Errorf("expected focus to land on 1 (base selection), got %d", state.Focus)
	}
}

func TestNewSessionState_AutonomousContainerToggleIgnored(t *testing.T) {
	state := NewNewSessionState(makeRepos(3), true, true)

	// Enable autonomous mode
	state.Autonomous = true
	state.UseContainers = true

	// Try to toggle containers off while autonomous is on (shouldn't work)
	state.Focus = 4
	state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Space})

	if !state.UseContainers {
		t.Error("expected containers to remain enabled when autonomous is on")
	}
}

func TestNewSessionState_AutonomousBranchInputIgnored(t *testing.T) {
	state := NewNewSessionState(makeRepos(3), true, true)

	// Enable autonomous mode
	state.Autonomous = true
	state.UseContainers = true

	// Force focus to branch input and try typing
	state.Focus = 3
	state.Update(tea.KeyPressMsg{Code: -1, Text: "a"})

	if state.BranchInput.Value() != "" {
		t.Errorf("expected branch input to remain empty in autonomous mode, got %q", state.BranchInput.Value())
	}
}

func TestNewSessionState_ScrollOnlyAffectsRepoFocus(t *testing.T) {
	repos := makeRepos(15)
	state := NewNewSessionState(repos, false, false)

	// Switch focus to base selection
	state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Tab})

	// Navigate down in base selection
	state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Down})

	// ScrollOffset should not change for base selection
	if state.ScrollOffset != 0 {
		t.Errorf("expected ScrollOffset 0 when navigating base options, got %d", state.ScrollOffset)
	}
	if state.BaseIndex != 1 {
		t.Errorf("expected BaseIndex 1, got %d", state.BaseIndex)
	}
}

func TestNewSessionState_LockedRepo_Title(t *testing.T) {
	state := NewNewSessionState([]string{"/home/user/myrepo"}, false, false)
	state.LockedRepo = "/home/user/myrepo"
	if state.Title() != "New Session in myrepo" {
		t.Errorf("Expected 'New Session in myrepo', got %q", state.Title())
	}
}

func TestNewSessionState_LockedRepo_HidesRepoSelector(t *testing.T) {
	state := NewNewSessionState([]string{"/repo1", "/repo2"}, false, false)
	state.LockedRepo = "/repo1"
	state.Focus = 1 // Start on base branch

	rendered := state.Render()
	if strings.Contains(rendered, "Repository:") {
		t.Error("Locked repo should not show 'Repository:' section")
	}
	if strings.Contains(rendered, "repo2") {
		t.Error("Locked repo should not show other repos")
	}
}

func TestNewSessionState_LockedRepo_GetSelectedRepo(t *testing.T) {
	state := NewNewSessionState([]string{"/repo1"}, false, false)
	state.LockedRepo = "/locked/repo"
	if state.GetSelectedRepo() != "/locked/repo" {
		t.Errorf("Expected locked repo, got %q", state.GetSelectedRepo())
	}
}

func TestNewSessionState_LockedRepo_TabSkipsRepoFocus(t *testing.T) {
	state := NewNewSessionState([]string{"/repo1"}, false, false)
	state.LockedRepo = "/repo1"
	state.Focus = 1 // Start on base branch

	// Without containers, numFields=3: 0=repo(skip), 1=base, 2=branch
	// Tab should go: 1 (base) -> 2 (branch) -> 1 (base) — skipping 0
	state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Tab})
	if state.Focus != 2 {
		t.Errorf("Expected focus 2 (branch), got %d", state.Focus)
	}
	state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Tab})
	if state.Focus != 1 {
		t.Errorf("Expected focus 1 (base, wrapped), got %d", state.Focus)
	}
}

func TestNewSessionState_LockedRepo_TabSkipsRepoFocus_WithContainers(t *testing.T) {
	state := NewNewSessionState([]string{"/repo1"}, true, false)
	state.LockedRepo = "/repo1"
	state.Focus = 1 // Start on base branch

	// With containers, numFields=5: 0=repo(skip), 1=base, 2=autonomous, 3=branch, 4=containers
	// Tab: 1 (base) -> 2 (autonomous) -> 3 (branch) -> 4 (container) -> 1 (base)
	state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Tab})
	if state.Focus != 2 {
		t.Errorf("Expected focus 2 (autonomous), got %d", state.Focus)
	}
	state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Tab})
	if state.Focus != 3 {
		t.Errorf("Expected focus 3 (branch), got %d", state.Focus)
	}
	state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Tab})
	if state.Focus != 4 {
		t.Errorf("Expected focus 4 (container), got %d", state.Focus)
	}
	state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Tab})
	if state.Focus != 1 {
		t.Errorf("Expected focus 1 (wrapped, skipping 0), got %d", state.Focus)
	}
}

func TestSessionSettingsState_Title(t *testing.T) {
	state := NewSessionSettingsState("s1", "my-session", "feature-branch", "main", false, false)
	if state.Title() != "Session Settings" {
		t.Errorf("expected 'Session Settings', got %q", state.Title())
	}
}

func TestSessionSettingsState_AutonomousDefault(t *testing.T) {
	state := NewSessionSettingsState("s1", "my-session", "feature-branch", "main", false, false)
	if state.Autonomous {
		t.Error("expected autonomous to be false by default")
	}

	state2 := NewSessionSettingsState("s1", "my-session", "feature-branch", "main", true, false)
	if !state2.Autonomous {
		t.Error("expected autonomous to be true when initialized as true")
	}
}

func TestSessionSettingsState_GetNewName(t *testing.T) {
	state := NewSessionSettingsState("s1", "my-session", "feature-branch", "main", false, false)
	if state.GetNewName() != "my-session" {
		t.Errorf("expected 'my-session', got %q", state.GetNewName())
	}
}

func TestSessionSettingsState_Render(t *testing.T) {
	state := NewSessionSettingsState("s1", "my-session", "feature-branch", "main", true, true)
	rendered := state.Render()

	// Check info section and form structure
	checks := []string{"Session Settings", "feature-branch", "main", "yes", "Options"}
	for _, check := range checks {
		if !strings.Contains(rendered, check) {
			t.Errorf("expected render to contain %q\nFull render:\n%s", check, rendered)
		}
	}
}

func TestSessionSettingsState_Help(t *testing.T) {
	state := NewSessionSettingsState("s1", "my-session", "feature-branch", "main", false, false)

	help := state.Help()
	if !strings.Contains(help, "Enter: save") {
		t.Error("expected help to contain 'Enter: save'")
	}
	if !strings.Contains(help, "Esc: cancel") {
		t.Error("expected help to contain 'Esc: cancel'")
	}
}
