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

	// With containers, numFields=4: 0=repo(skip), 1=base, 2=branch, 3=containers
	// Tab: 1 (base) -> 2 (branch) -> 3 (container) -> 1 (base)
	state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Tab})
	if state.Focus != 2 {
		t.Errorf("Expected focus 2 (branch), got %d", state.Focus)
	}
	state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Tab})
	if state.Focus != 3 {
		t.Errorf("Expected focus 3 (container), got %d", state.Focus)
	}
	state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Tab})
	if state.Focus != 1 {
		t.Errorf("Expected focus 1 (wrapped, skipping 0), got %d", state.Focus)
	}
}

func TestNewSessionState_DockerHintWhenContainersNotSupported(t *testing.T) {
	initTestStyles()

	state := NewNewSessionState(makeRepos(3), false, false)
	rendered := state.Render()

	if !strings.Contains(rendered, "Install Docker to enable container mode") {
		t.Error("expected Docker hint when containers not supported")
	}
}

func TestNewSessionState_NoDockerHintWhenContainersSupported(t *testing.T) {
	initTestStyles()

	state := NewNewSessionState(makeRepos(3), true, true)
	rendered := state.Render()

	if strings.Contains(rendered, "Install Docker to enable container mode") {
		t.Error("should not show Docker hint when containers are supported")
	}
}

func TestForkSessionState_DockerHintWhenContainersNotSupported(t *testing.T) {
	initTestStyles()

	state := NewForkSessionState("parent", "parent-id", "/repo", false, false, false)
	rendered := state.Render()

	if !strings.Contains(rendered, "Install Docker to enable container mode") {
		t.Error("expected Docker hint in fork modal when containers not supported")
	}
}

func TestForkSessionState_NoDockerHintWhenContainersSupported(t *testing.T) {
	initTestStyles()

	state := NewForkSessionState("parent", "parent-id", "/repo", false, true, true)
	rendered := state.Render()

	if strings.Contains(rendered, "Install Docker to enable container mode") {
		t.Error("should not show Docker hint in fork modal when containers are supported")
	}
}

func TestSessionSettingsState_Title(t *testing.T) {
	state := NewSessionSettingsState("s1", "my-session", "feature-branch", "main", false, "/repo", false, "", false, "")
	if state.Title() != "Session Settings" {
		t.Errorf("expected 'Session Settings', got %q", state.Title())
	}
}

func TestSessionSettingsState_GetNewName(t *testing.T) {
	state := NewSessionSettingsState("s1", "my-session", "feature-branch", "main", false, "/repo", false, "", false, "")
	if state.GetNewName() != "my-session" {
		t.Errorf("expected 'my-session', got %q", state.GetNewName())
	}
}

func TestSessionSettingsState_Render(t *testing.T) {
	state := NewSessionSettingsState("s1", "my-session", "feature-branch", "main", true, "/repo", false, "", false, "")
	rendered := state.Render()

	// Check info section and form structure
	checks := []string{"Session Settings", "feature-branch", "main", "yes", "Name"}
	for _, check := range checks {
		if !strings.Contains(rendered, check) {
			t.Errorf("expected render to contain %q\nFull render:\n%s", check, rendered)
		}
	}
}

func TestSessionSettingsState_Help(t *testing.T) {
	state := NewSessionSettingsState("s1", "my-session", "feature-branch", "main", false, "/repo", false, "", false, "")

	help := state.Help()
	if !strings.Contains(help, "Enter: save") {
		t.Error("expected help to contain 'Enter: save'")
	}
	if !strings.Contains(help, "Esc: cancel") {
		t.Error("expected help to contain 'Esc: cancel'")
	}
}

// =============================================================================
// SessionSettingsState - Asana/Linear integration tests
// =============================================================================

func TestSessionSettingsState_PreferredWidth_NoProviders(t *testing.T) {
	state := NewSessionSettingsState("s1", "name", "branch", "main", false, "/repo", false, "", false, "")
	// Without providers, should not implement PreferredWidth (default modal width)
	if state.AsanaPATSet || state.LinearAPIKeySet {
		t.Error("expected no providers set")
	}
}

func TestSessionSettingsState_PreferredWidth_WithAsana(t *testing.T) {
	state := NewSessionSettingsState("s1", "name", "branch", "main", false, "/repo", true, "", false, "")
	if w := state.PreferredWidth(); w != ModalWidthWide {
		t.Errorf("expected preferred width %d with Asana, got %d", ModalWidthWide, w)
	}
}

func TestSessionSettingsState_PreferredWidth_WithLinear(t *testing.T) {
	state := NewSessionSettingsState("s1", "name", "branch", "main", false, "/repo", false, "", true, "")
	if w := state.PreferredWidth(); w != ModalWidthWide {
		t.Errorf("expected preferred width %d with Linear, got %d", ModalWidthWide, w)
	}
}

func TestSessionSettingsState_AsanaLoading(t *testing.T) {
	state := NewSessionSettingsState("s1", "name", "branch", "main", false, "/repo", true, "", false, "")
	if !state.AsanaLoading {
		t.Error("expected AsanaLoading to be true initially when PAT set")
	}

	rendered := state.Render()
	if !strings.Contains(rendered, "Fetching Asana projects") {
		t.Error("should show loading state for Asana")
	}
}

func TestSessionSettingsState_SetAsanaProjects(t *testing.T) {
	state := NewSessionSettingsState("s1", "name", "branch", "main", false, "/repo", true, "", false, "")

	options := []AsanaProjectOption{
		{GID: "", Name: "(none)"},
		{GID: "p1", Name: "Project Alpha"},
	}
	state.SetAsanaProjects(options)

	if state.AsanaLoading {
		t.Error("expected AsanaLoading to be false after SetAsanaProjects")
	}
	if state.AsanaLoadError != "" {
		t.Errorf("expected no error, got %q", state.AsanaLoadError)
	}
}

func TestSessionSettingsState_SetAsanaProjectsError(t *testing.T) {
	state := NewSessionSettingsState("s1", "name", "branch", "main", false, "/repo", true, "", false, "")

	state.SetAsanaProjectsError("connection failed")

	if state.AsanaLoading {
		t.Error("expected AsanaLoading to be false after error")
	}
	if state.AsanaLoadError != "connection failed" {
		t.Errorf("expected error 'connection failed', got %q", state.AsanaLoadError)
	}

	rendered := state.Render()
	if !strings.Contains(rendered, "connection failed") {
		t.Error("should show error message in render")
	}
}

func TestSessionSettingsState_GetAsanaProject(t *testing.T) {
	state := NewSessionSettingsState("s1", "name", "branch", "main", false, "/repo", true, "p1", false, "")
	if state.GetAsanaProject() != "p1" {
		t.Errorf("expected 'p1', got %q", state.GetAsanaProject())
	}
}

func TestSessionSettingsState_LinearLoading(t *testing.T) {
	state := NewSessionSettingsState("s1", "name", "branch", "main", false, "/repo", false, "", true, "")
	if !state.LinearLoading {
		t.Error("expected LinearLoading to be true initially when API key set")
	}

	rendered := state.Render()
	if !strings.Contains(rendered, "Fetching Linear teams") {
		t.Error("should show loading state for Linear")
	}
}

func TestSessionSettingsState_SetLinearTeams(t *testing.T) {
	state := NewSessionSettingsState("s1", "name", "branch", "main", false, "/repo", false, "", true, "")

	options := []LinearTeamOption{
		{ID: "", Name: "(none)"},
		{ID: "t1", Name: "Engineering"},
	}
	state.SetLinearTeams(options)

	if state.LinearLoading {
		t.Error("expected LinearLoading to be false after SetLinearTeams")
	}
	if state.LinearLoadError != "" {
		t.Errorf("expected no error, got %q", state.LinearLoadError)
	}
}

func TestSessionSettingsState_SetLinearTeamsError(t *testing.T) {
	state := NewSessionSettingsState("s1", "name", "branch", "main", false, "/repo", false, "", true, "")
	state.SetLinearTeamsError("network error")

	if state.LinearLoading {
		t.Error("expected LinearLoading to be false after error")
	}
	if state.LinearLoadError != "network error" {
		t.Errorf("expected error 'network error', got %q", state.LinearLoadError)
	}
}

func TestSessionSettingsState_GetLinearTeam(t *testing.T) {
	state := NewSessionSettingsState("s1", "name", "branch", "main", false, "/repo", false, "", true, "team-123")
	if state.GetLinearTeam() != "team-123" {
		t.Errorf("expected 'team-123', got %q", state.GetLinearTeam())
	}
}

func TestSessionSettingsState_Render_NoProvidersOmitsRepoSection(t *testing.T) {
	state := NewSessionSettingsState("s1", "name", "branch", "main", false, "/repo", false, "", false, "")
	rendered := state.Render()

	if strings.Contains(rendered, "Repo Settings") {
		t.Error("should not show Repo Settings section when no providers configured")
	}
}

func TestSessionSettingsState_Render_BothProviders(t *testing.T) {
	state := NewSessionSettingsState("s1", "name", "branch", "main", false, "/repo", true, "p1", true, "t1")

	state.SetAsanaProjects([]AsanaProjectOption{
		{GID: "", Name: "(none)"},
		{GID: "p1", Name: "My Project"},
	})
	state.SetLinearTeams([]LinearTeamOption{
		{ID: "", Name: "(none)"},
		{ID: "t1", Name: "Engineering"},
	})

	rendered := state.Render()
	if !strings.Contains(rendered, "Asana project") {
		t.Error("should show Asana project section")
	}
	if !strings.Contains(rendered, "Linear team") {
		t.Error("should show Linear team section")
	}
}
