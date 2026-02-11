package modals

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// testThemes and testThemeNames are used across settings tests.
var (
	testThemes      = []string{"dark-purple", "nord", "dracula"}
	testThemeNames  = []string{"Dark Purple", "Nord", "Dracula"}
	testCurrentTheme = "dark-purple"
)

// newTestSettingsState is a helper that prepends theme data to NewSettingsState calls.
func newTestSettingsState(branchPrefix string, notifs bool, repos []string,
	asanaProjects map[string]string, defaultRepoIndex int, asanaPATSet bool) *SettingsState {
	return NewSettingsState(testThemes, testThemeNames, testCurrentTheme,
		branchPrefix, notifs, repos, asanaProjects, defaultRepoIndex, asanaPATSet,
		false, "") // no containers by default in tests
}

// newTestSettingsStateWithContainers is like newTestSettingsState but with container support.
func newTestSettingsStateWithContainers(branchPrefix string, notifs bool, repos []string,
	asanaProjects map[string]string, defaultRepoIndex int, asanaPATSet bool,
	containersSupported bool, containerImage string) *SettingsState {
	return NewSettingsState(testThemes, testThemeNames, testCurrentTheme,
		branchPrefix, notifs, repos, asanaProjects, defaultRepoIndex, asanaPATSet,
		containersSupported, containerImage)
}

func TestSettingsState_NumFields_NoRepo(t *testing.T) {
	s := newTestSettingsState("", false, nil, nil, 0, false)
	if n := s.numFields(); n != 3 {
		t.Errorf("Expected 3 fields with no repos, got %d", n)
	}
}

func TestSettingsState_NumFields_WithRepo_AsanaPAT(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": ""},
		0, true)
	if n := s.numFields(); n != 5 {
		t.Errorf("Expected 5 fields with repo and Asana PAT, got %d", n)
	}
}

func TestSettingsState_NumFields_WithRepo_NoAsanaPAT(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": ""},
		0, false)
	if n := s.numFields(); n != 3 {
		t.Errorf("Expected 3 fields with repo but no Asana PAT (per-repo section hidden), got %d", n)
	}
}

func TestSettingsState_AsanaFocusIndex(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": ""},
		0, true)
	if idx := s.asanaFocusIndex(); idx != 4 {
		t.Errorf("Expected asana focus index 4, got %d", idx)
	}
}

func TestSettingsState_TabCycle_WithRepo(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": ""},
		0, true)

	// Start at 0 (theme)
	if s.Focus != 0 {
		t.Fatalf("Expected initial focus 0, got %d", s.Focus)
	}

	// Tab through: 0 -> 1 -> 2 -> 3 -> 4 -> 0 (5 fields with repo + PAT)
	expectedFoci := []int{1, 2, 3, 4, 0}
	for i, expected := range expectedFoci {
		s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if s.Focus != expected {
			t.Errorf("After tab %d: expected focus %d, got %d", i+1, expected, s.Focus)
		}
	}
}

func TestSettingsState_TabCycle_WithRepo_NoPAT(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": ""},
		0, false)

	// Tab through: 0 -> 1 -> 2 -> 0 (3 fields, per-repo section hidden without PAT)
	expectedFoci := []int{1, 2, 0}
	for i, expected := range expectedFoci {
		s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if s.Focus != expected {
			t.Errorf("After tab %d: expected focus %d, got %d", i+1, expected, s.Focus)
		}
	}
}

func TestSettingsState_Render_NoContainerSection_WhenUnsupported(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": ""},
		0, true)
	rendered := s.Render()

	if strings.Contains(rendered, "Container image") {
		t.Error("Container image field should not appear when containers unsupported")
	}
}

func TestSettingsState_Render_ContainerSection_WhenSupported(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": ""},
		0, true, true, "plural-claude")
	rendered := s.Render()

	if !strings.Contains(rendered, "Container image") {
		t.Error("Container image field should appear when containers supported")
	}
}

func TestSettingsState_Render_AsanaHiddenWithoutPAT(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": "123"},
		0, false)
	rendered := s.Render()

	if strings.Contains(rendered, "Asana project") {
		t.Error("Asana field should not appear when ASANA_PAT is not set")
	}
}

func TestSettingsState_Render_AsanaShownWithPAT(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": "123"},
		0, true)
	rendered := s.Render()

	if !strings.Contains(rendered, "Asana project") {
		t.Error("Asana field should appear when ASANA_PAT is set")
	}
}

func TestSettingsState_RepoSelector_LeftRight(t *testing.T) {
	repos := []string{"/repo/a", "/repo/b", "/repo/c"}
	s := newTestSettingsState("", false, repos,
		map[string]string{"/repo/a": "111", "/repo/b": "222", "/repo/c": "333"},
		0, true)

	// Focus on repo selector (now index 3)
	s.Focus = 3

	// Initially at index 0
	if s.SelectedRepoIndex != 0 {
		t.Fatalf("Expected initial repo index 0, got %d", s.SelectedRepoIndex)
	}

	// Press right -> index 1
	s.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if s.SelectedRepoIndex != 1 {
		t.Errorf("After Right, expected repo index 1, got %d", s.SelectedRepoIndex)
	}

	// Press right -> index 2
	s.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if s.SelectedRepoIndex != 2 {
		t.Errorf("After Right, expected repo index 2, got %d", s.SelectedRepoIndex)
	}

	// Press right at max -> should clamp
	s.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if s.SelectedRepoIndex != 2 {
		t.Errorf("After Right at max, expected repo index 2 (clamped), got %d", s.SelectedRepoIndex)
	}

	// Press left -> index 1
	s.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if s.SelectedRepoIndex != 1 {
		t.Errorf("After Left, expected repo index 1, got %d", s.SelectedRepoIndex)
	}

	// Press left -> index 0
	s.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if s.SelectedRepoIndex != 0 {
		t.Errorf("After Left, expected repo index 0, got %d", s.SelectedRepoIndex)
	}

	// Press left at min -> should clamp
	s.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if s.SelectedRepoIndex != 0 {
		t.Errorf("After Left at min, expected repo index 0 (clamped), got %d", s.SelectedRepoIndex)
	}
}

func TestSettingsState_SetAsanaProjects(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/repo/a"},
		map[string]string{"/repo/a": ""},
		0, true)

	// Initially loading
	if !s.AsanaLoading {
		t.Error("Expected AsanaLoading to be true initially when PAT set")
	}

	// Set projects
	options := []AsanaProjectOption{
		{GID: "", Name: "(none)"},
		{GID: "p1", Name: "Project Alpha"},
		{GID: "p2", Name: "Project Beta"},
	}
	s.SetAsanaProjects(options)

	if s.AsanaLoading {
		t.Error("Expected AsanaLoading to be false after SetAsanaProjects")
	}
	if s.AsanaLoadError != "" {
		t.Errorf("Expected no error, got %q", s.AsanaLoadError)
	}
	if len(s.AsanaProjectOptions) != 3 {
		t.Errorf("Expected 3 options, got %d", len(s.AsanaProjectOptions))
	}
	if s.AsanaCursorIndex != 0 {
		t.Errorf("Expected cursor at 0, got %d", s.AsanaCursorIndex)
	}
}

func TestSettingsState_SetAsanaProjectsError(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/repo/a"},
		map[string]string{"/repo/a": ""},
		0, true)

	s.SetAsanaProjectsError("connection failed")

	if s.AsanaLoading {
		t.Error("Expected AsanaLoading to be false after error")
	}
	if s.AsanaLoadError != "connection failed" {
		t.Errorf("Expected error 'connection failed', got %q", s.AsanaLoadError)
	}
}

func TestSettingsState_AsanaSearchFiltering(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/repo/a"},
		map[string]string{"/repo/a": ""},
		0, true)

	options := []AsanaProjectOption{
		{GID: "", Name: "(none)"},
		{GID: "p1", Name: "Project Alpha"},
		{GID: "p2", Name: "Project Beta"},
		{GID: "p3", Name: "Other Gamma"},
	}
	s.SetAsanaProjects(options)

	// No filter: all shown
	filtered := s.getFilteredAsanaProjects()
	if len(filtered) != 4 {
		t.Errorf("Expected 4 results with no filter, got %d", len(filtered))
	}

	// Filter by "project"
	s.AsanaSearchInput.SetValue("project")
	filtered = s.getFilteredAsanaProjects()
	if len(filtered) != 2 {
		t.Errorf("Expected 2 results for 'project' filter, got %d", len(filtered))
	}

	// Filter by "gamma"
	s.AsanaSearchInput.SetValue("gamma")
	filtered = s.getFilteredAsanaProjects()
	if len(filtered) != 1 {
		t.Errorf("Expected 1 result for 'gamma' filter, got %d", len(filtered))
	}
	if filtered[0].GID != "p3" {
		t.Errorf("Expected GID 'p3', got %q", filtered[0].GID)
	}

	// Filter with no matches
	s.AsanaSearchInput.SetValue("nonexistent")
	filtered = s.getFilteredAsanaProjects()
	if len(filtered) != 0 {
		t.Errorf("Expected 0 results for 'nonexistent' filter, got %d", len(filtered))
	}
}

func TestSettingsState_AsanaNavigation(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/repo/a"},
		map[string]string{"/repo/a": ""},
		0, true)

	options := []AsanaProjectOption{
		{GID: "", Name: "(none)"},
		{GID: "p1", Name: "Project 1"},
		{GID: "p2", Name: "Project 2"},
	}
	s.SetAsanaProjects(options)

	// Focus on Asana field
	s.Focus = s.asanaFocusIndex()
	s.updateInputFocus()

	// Down
	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.AsanaCursorIndex != 1 {
		t.Errorf("After Down, expected cursor at 1, got %d", s.AsanaCursorIndex)
	}

	// Down again
	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.AsanaCursorIndex != 2 {
		t.Errorf("After Down, expected cursor at 2, got %d", s.AsanaCursorIndex)
	}

	// Down at end: clamp
	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.AsanaCursorIndex != 2 {
		t.Errorf("After Down at end, expected cursor at 2, got %d", s.AsanaCursorIndex)
	}

	// Up
	s.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if s.AsanaCursorIndex != 1 {
		t.Errorf("After Up, expected cursor at 1, got %d", s.AsanaCursorIndex)
	}

	// Up to top
	s.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if s.AsanaCursorIndex != 0 {
		t.Errorf("After Up, expected cursor at 0, got %d", s.AsanaCursorIndex)
	}

	// Up at top: clamp
	s.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if s.AsanaCursorIndex != 0 {
		t.Errorf("After Up at top, expected cursor at 0, got %d", s.AsanaCursorIndex)
	}
}

func TestSettingsState_AsanaSelectProject(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/repo/a"},
		map[string]string{"/repo/a": ""},
		0, true)

	options := []AsanaProjectOption{
		{GID: "", Name: "(none)"},
		{GID: "p1", Name: "Project Alpha"},
		{GID: "p2", Name: "Project Beta"},
	}
	s.SetAsanaProjects(options)

	// Focus on Asana field
	s.Focus = s.asanaFocusIndex()
	s.updateInputFocus()

	// Navigate to "Project Alpha" (index 1)
	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})

	// Press Enter to select
	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if s.AsanaSelectedGIDs["/repo/a"] != "p1" {
		t.Errorf("Expected selected GID 'p1', got %q", s.AsanaSelectedGIDs["/repo/a"])
	}

	// GetAsanaProject should also return the selected GID
	if s.GetAsanaProject() != "p1" {
		t.Errorf("GetAsanaProject should return 'p1', got %q", s.GetAsanaProject())
	}
}

func TestSettingsState_AsanaSelectNone(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/repo/a"},
		map[string]string{"/repo/a": "existing-gid"},
		0, true)

	options := []AsanaProjectOption{
		{GID: "", Name: "(none)"},
		{GID: "p1", Name: "Project Alpha"},
	}
	s.SetAsanaProjects(options)

	// Focus on Asana field
	s.Focus = s.asanaFocusIndex()
	s.updateInputFocus()

	// Cursor is at index 0 ((none)), press Enter
	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if s.AsanaSelectedGIDs["/repo/a"] != "" {
		t.Errorf("Expected empty GID after selecting (none), got %q", s.AsanaSelectedGIDs["/repo/a"])
	}
}

func TestSettingsState_AsanaPerRepoSelection(t *testing.T) {
	repos := []string{"/repo/a", "/repo/b"}
	s := newTestSettingsState("", false, repos,
		map[string]string{"/repo/a": "aaa", "/repo/b": "bbb"},
		0, true)

	options := []AsanaProjectOption{
		{GID: "", Name: "(none)"},
		{GID: "aaa", Name: "Project A"},
		{GID: "bbb", Name: "Project B"},
	}
	s.SetAsanaProjects(options)

	// Focus on Asana, select Project B for repo a
	s.Focus = s.asanaFocusIndex()
	s.updateInputFocus()
	s.AsanaCursorIndex = 2 // Project B
	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if s.AsanaSelectedGIDs["/repo/a"] != "bbb" {
		t.Errorf("Expected 'bbb' for repo a, got %q", s.AsanaSelectedGIDs["/repo/a"])
	}

	// Switch to repo b
	s.Focus = 3
	s.switchRepo(1)

	// Repo b should still have 'bbb'
	if s.AsanaSelectedGIDs["/repo/b"] != "bbb" {
		t.Errorf("Expected 'bbb' for repo b, got %q", s.AsanaSelectedGIDs["/repo/b"])
	}

	// Switch back to repo a
	s.switchRepo(-1)

	// Repo a should still have 'bbb' (what we set earlier)
	if s.AsanaSelectedGIDs["/repo/a"] != "bbb" {
		t.Errorf("Expected 'bbb' for repo a after switching back, got %q", s.AsanaSelectedGIDs["/repo/a"])
	}
}

func TestSettingsState_GetAllAsanaProjects(t *testing.T) {
	repos := []string{"/repo/a", "/repo/b"}
	s := newTestSettingsState("", false, repos,
		map[string]string{"/repo/a": "aaa", "/repo/b": "bbb"},
		0, true)

	// Modify via selector
	s.AsanaSelectedGIDs["/repo/a"] = "modified"

	projects := s.GetAllAsanaProjects()
	if projects["/repo/a"] != "modified" {
		t.Errorf("Expected GetAllAsanaProjects to return 'modified', got %q", projects["/repo/a"])
	}
	if projects["/repo/b"] != "bbb" {
		t.Errorf("Expected repo b to have 'bbb', got %q", projects["/repo/b"])
	}
}

func TestSettingsState_Render_WithRepoSelector(t *testing.T) {
	repos := []string{"/path/to/myrepo"}
	s := newTestSettingsState("", false, repos,
		map[string]string{"/path/to/myrepo": ""},
		0, true) // PAT must be set for per-repo section to appear

	rendered := s.Render()
	if !strings.Contains(rendered, "Per-repo settings") {
		t.Error("Expected per-repo section header")
	}
	if !strings.Contains(rendered, "myrepo") {
		t.Error("Expected repo name in rendered output")
	}
}

func TestSettingsState_Render_WithRepoButNoPAT(t *testing.T) {
	repos := []string{"/path/to/myrepo"}
	s := newTestSettingsState("", false, repos,
		map[string]string{"/path/to/myrepo": ""},
		0, false)

	rendered := s.Render()
	if strings.Contains(rendered, "Per-repo settings") {
		t.Error("Per-repo section should not appear without Asana PAT")
	}
}

func TestSettingsState_Render_NoRepos(t *testing.T) {
	s := newTestSettingsState("", false, nil, nil, 0, false)
	rendered := s.Render()

	if strings.Contains(rendered, "Per-repo settings") {
		t.Error("Per-repo section should not appear when no repos")
	}
}

func TestSettingsState_DefaultRepoIndex(t *testing.T) {
	repos := []string{"/repo/a", "/repo/b", "/repo/c"}
	s := newTestSettingsState("", false, repos,
		map[string]string{"/repo/a": "aaa", "/repo/b": "bbb", "/repo/c": "ccc"},
		1, true)

	if s.SelectedRepoIndex != 1 {
		t.Errorf("Expected default repo index 1, got %d", s.SelectedRepoIndex)
	}
	// Selected GID for the default repo should be 'bbb'
	if s.GetAsanaProject() != "bbb" {
		t.Errorf("Expected asana GID 'bbb' for default repo b, got %q", s.GetAsanaProject())
	}
}

func TestSettingsState_DefaultRepoIndex_OutOfBounds(t *testing.T) {
	repos := []string{"/repo/a"}
	s := newTestSettingsState("", false, repos,
		map[string]string{"/repo/a": "aaa"},
		99, true)

	if s.SelectedRepoIndex != 0 {
		t.Errorf("Expected clamped repo index 0, got %d", s.SelectedRepoIndex)
	}
}

func TestSettingsState_HelpChangesOnRepoFocus(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/repo"},
		map[string]string{"/repo": ""},
		0, true) // PAT must be set for per-repo section (and focus 3) to exist

	s.Focus = 3
	help := s.Help()
	if !strings.Contains(help, "Left/Right: switch repo") {
		t.Errorf("Help at repo selector focus should mention Left/Right: switch repo, got %q", help)
	}

	s.Focus = 1
	help = s.Help()
	if strings.Contains(help, "switch repo") {
		t.Errorf("Help at non-repo focus should not mention switch repo, got %q", help)
	}
}

func TestSettingsState_HelpChangesOnAsanaFocus(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/repo"},
		map[string]string{"/repo": ""},
		0, true)

	s.Focus = s.asanaFocusIndex()
	help := s.Help()
	if !strings.Contains(help, "Up/Down: navigate") {
		t.Errorf("Help at Asana focus should mention Up/Down: navigate, got %q", help)
	}
}

func TestSettingsState_PreferredWidth(t *testing.T) {
	s := newTestSettingsState("", false, nil, nil, 0, false)
	if w := s.PreferredWidth(); w != ModalWidthWide {
		t.Errorf("Expected preferred width %d, got %d", ModalWidthWide, w)
	}
}

func TestSettingsState_ThemeSelector(t *testing.T) {
	s := newTestSettingsState("", false, nil, nil, 0, false)

	// Initially focused on theme (focus 0) and current theme selected
	if s.Focus != 0 {
		t.Fatalf("Expected initial focus 0, got %d", s.Focus)
	}
	if s.SelectedThemeIndex != 0 {
		t.Fatalf("Expected initial theme index 0, got %d", s.SelectedThemeIndex)
	}
	if s.GetSelectedTheme() != "dark-purple" {
		t.Errorf("Expected selected theme 'dark-purple', got %q", s.GetSelectedTheme())
	}
	if s.ThemeChanged() {
		t.Error("Theme should not be changed initially")
	}

	// Press right -> next theme
	s.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if s.SelectedThemeIndex != 1 {
		t.Errorf("After Right, expected theme index 1, got %d", s.SelectedThemeIndex)
	}
	if s.GetSelectedTheme() != "nord" {
		t.Errorf("Expected selected theme 'nord', got %q", s.GetSelectedTheme())
	}
	if !s.ThemeChanged() {
		t.Error("Theme should be changed after switching")
	}

	// Press right -> next theme
	s.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if s.SelectedThemeIndex != 2 {
		t.Errorf("After Right, expected theme index 2, got %d", s.SelectedThemeIndex)
	}

	// Press right at max -> should clamp
	s.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if s.SelectedThemeIndex != 2 {
		t.Errorf("After Right at max, expected theme index 2 (clamped), got %d", s.SelectedThemeIndex)
	}

	// Press left -> back to index 1
	s.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if s.SelectedThemeIndex != 1 {
		t.Errorf("After Left, expected theme index 1, got %d", s.SelectedThemeIndex)
	}

	// Press left -> back to index 0
	s.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if s.SelectedThemeIndex != 0 {
		t.Errorf("After Left, expected theme index 0, got %d", s.SelectedThemeIndex)
	}

	// Press left at min -> should clamp
	s.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if s.SelectedThemeIndex != 0 {
		t.Errorf("After Left at min, expected theme index 0 (clamped), got %d", s.SelectedThemeIndex)
	}

	// Theme should no longer be changed (back to original)
	if s.ThemeChanged() {
		t.Error("Theme should not be changed after navigating back to original")
	}
}

func TestSettingsState_ThemeSelector_NotAffectedWhenNotFocused(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/repo"},
		map[string]string{"/repo": ""},
		0, false)

	// Focus on branch prefix (focus 1)
	s.Focus = 1

	// Left/Right should NOT change theme
	s.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if s.SelectedThemeIndex != 0 {
		t.Errorf("Theme should not change when not focused, got index %d", s.SelectedThemeIndex)
	}
}

func TestSettingsState_Render_ContainsThemeSection(t *testing.T) {
	s := newTestSettingsState("", false, nil, nil, 0, false)
	rendered := s.Render()

	if !strings.Contains(rendered, "Theme:") {
		t.Error("Rendered settings should contain 'Theme:' label")
	}
	if !strings.Contains(rendered, "Dark Purple") {
		t.Error("Rendered settings should contain the selected theme display name")
	}
}

func TestSettingsState_HelpChangesOnThemeFocus(t *testing.T) {
	s := newTestSettingsState("", false, nil, nil, 0, false)

	s.Focus = 0
	help := s.Help()
	if !strings.Contains(help, "change theme") {
		t.Errorf("Help at theme focus should mention change theme, got %q", help)
	}

	s.Focus = 1
	help = s.Help()
	if strings.Contains(help, "change theme") {
		t.Errorf("Help at non-theme focus should not mention change theme, got %q", help)
	}
}

func TestSettingsState_Render_AsanaLoading(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/repo/a"},
		map[string]string{"/repo/a": ""},
		0, true)

	rendered := s.Render()
	if !strings.Contains(rendered, "Fetching Asana projects") {
		t.Error("Should show loading message when AsanaLoading is true")
	}
}

func TestSettingsState_Render_AsanaError(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/repo/a"},
		map[string]string{"/repo/a": ""},
		0, true)

	s.SetAsanaProjectsError("timeout")

	rendered := s.Render()
	if !strings.Contains(rendered, "timeout") {
		t.Error("Should show error message")
	}
}

func TestSettingsState_Render_AsanaProjectList(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/repo/a"},
		map[string]string{"/repo/a": "p1"},
		0, true)

	options := []AsanaProjectOption{
		{GID: "", Name: "(none)"},
		{GID: "p1", Name: "My Project"},
	}
	s.SetAsanaProjects(options)

	rendered := s.Render()
	if !strings.Contains(rendered, "My Project") {
		t.Error("Should show project name in rendered output")
	}
	if !strings.Contains(rendered, "Current: My Project") {
		t.Error("Should show current selection label")
	}
}

func TestSettingsState_Render_AsanaCurrentNone(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/repo/a"},
		map[string]string{"/repo/a": ""},
		0, true)

	options := []AsanaProjectOption{
		{GID: "", Name: "(none)"},
		{GID: "p1", Name: "My Project"},
	}
	s.SetAsanaProjects(options)

	rendered := s.Render()
	if !strings.Contains(rendered, "Current: (none)") {
		t.Error("Should show 'Current: (none)' when no project selected")
	}
}

func TestSettingsState_IsAsanaFocused(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/repo/a"},
		map[string]string{"/repo/a": ""},
		0, true)

	// Not focused on Asana
	s.Focus = 0
	if s.IsAsanaFocused() {
		t.Error("Should not be Asana-focused when Focus is 0")
	}

	// Focused on Asana
	s.Focus = s.asanaFocusIndex()
	if !s.IsAsanaFocused() {
		t.Error("Should be Asana-focused when Focus is asanaFocusIndex")
	}

	// PAT not set: even at correct focus index, should not be Asana-focused
	s2 := newTestSettingsState("", false,
		[]string{"/repo/a"},
		map[string]string{"/repo/a": ""},
		0, false)
	s2.Focus = 4
	if s2.IsAsanaFocused() {
		t.Error("Should not be Asana-focused when PAT is not set")
	}
}

func TestNewSessionState_ContainerCheckbox_WhenSupported(t *testing.T) {
	s := NewNewSessionState([]string{"/repo"}, true, false)

	if s.numFields() != 4 {
		t.Errorf("Expected 4 fields with containers supported, got %d", s.numFields())
	}

	// Tab to container checkbox (focus 3)
	s.Focus = 3
	s.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !s.UseContainers {
		t.Error("Space at focus 3 should toggle container checkbox when supported")
	}

	// Toggle back
	s.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if s.UseContainers {
		t.Error("Space again should toggle container checkbox off")
	}
}

func TestNewSessionState_ContainerCheckbox_WhenUnsupported(t *testing.T) {
	s := NewNewSessionState([]string{"/repo"}, false, false)

	if s.numFields() != 3 {
		t.Errorf("Expected 3 fields with containers unsupported, got %d", s.numFields())
	}

	// Container checkbox should not be rendered
	rendered := s.Render()
	if strings.Contains(rendered, "Run in container") {
		t.Error("Container checkbox should not appear when unsupported")
	}
}

func TestNewSessionState_ContainerCheckbox_Render(t *testing.T) {
	s := NewNewSessionState([]string{"/repo"}, true, false)
	rendered := s.Render()

	if !strings.Contains(rendered, "Run in container") {
		t.Error("Container checkbox should appear when supported")
	}
	if !strings.Contains(rendered, "defense in depth") {
		t.Error("Container warning should appear when supported")
	}
}

func TestNewSessionState_GetUseContainers(t *testing.T) {
	s := NewNewSessionState([]string{"/repo"}, true, false)

	if s.GetUseContainers() {
		t.Error("GetUseContainers should return false initially")
	}

	s.UseContainers = true
	if !s.GetUseContainers() {
		t.Error("GetUseContainers should return true after setting")
	}
}

func TestForkSessionState_ContainerCheckbox_WhenSupported(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, true, false)

	if s.numFields() != 3 {
		t.Errorf("Expected 3 fields with containers supported, got %d", s.numFields())
	}

	// Tab to container checkbox (focus 2)
	s.Focus = 2
	s.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !s.UseContainers {
		t.Error("Space at focus 2 should toggle container checkbox when supported")
	}
}

func TestForkSessionState_ContainerCheckbox_WhenUnsupported(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, false, false)

	if s.numFields() != 2 {
		t.Errorf("Expected 2 fields with containers unsupported, got %d", s.numFields())
	}

	rendered := s.Render()
	if strings.Contains(rendered, "Run in container") {
		t.Error("Container checkbox should not appear when unsupported")
	}
}

func TestForkSessionState_InheritsParentContainerized(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", true, true, false)

	if !s.UseContainers {
		t.Error("Fork should default to parent's containerized state (true)")
	}

	s2 := NewForkSessionState("parent", "parent-id", "/repo", false, true, false)
	if s2.UseContainers {
		t.Error("Fork should default to parent's containerized state (false)")
	}
}

func TestForkSessionState_ContainerCheckbox_Render(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, true, false)
	rendered := s.Render()

	if !strings.Contains(rendered, "Run in container") {
		t.Error("Container checkbox should appear when supported")
	}
}

func TestForkSessionState_UpDownNavigation_CyclesToContainerCheckbox(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, true, false)

	if s.Focus != 0 {
		t.Fatalf("Expected focus 0, got %d", s.Focus)
	}

	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.Focus != 1 {
		t.Errorf("After Down from 0, expected focus 1, got %d", s.Focus)
	}

	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.Focus != 2 {
		t.Errorf("After Down from 1, expected focus 2 (container checkbox), got %d", s.Focus)
	}

	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.Focus != 0 {
		t.Errorf("After Down from 2, expected focus 0 (wrap), got %d", s.Focus)
	}

	s.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if s.Focus != 2 {
		t.Errorf("After Up from 0, expected focus 2 (wrap), got %d", s.Focus)
	}
}

func TestForkSessionState_UpDownNavigation_WithoutContainers(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, false, false)

	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.Focus != 1 {
		t.Errorf("After Down from 0, expected focus 1, got %d", s.Focus)
	}

	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.Focus != 0 {
		t.Errorf("After Down from 1, expected focus 0 (wrap), got %d", s.Focus)
	}
}

func TestForkSessionState_HelpText_ContainerFocused(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, true, false)
	s.Focus = 2

	help := s.Help()
	if !strings.Contains(help, "Space: toggle") {
		t.Errorf("Help at container focus should mention Space: toggle, got %q", help)
	}
}

func TestBroadcastState_ContainerCheckbox_WhenSupported(t *testing.T) {
	s := NewBroadcastState([]string{"/repo"}, true, false)

	if !s.ContainersSupported {
		t.Error("ContainersSupported should be true")
	}

	s.Focus = 3
	s.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !s.UseContainers {
		t.Error("Space at focus 3 should toggle container checkbox")
	}

	if !s.GetUseContainers() {
		t.Error("GetUseContainers should return true after toggle")
	}
}

func TestBroadcastState_ContainerCheckbox_WhenUnsupported(t *testing.T) {
	s := NewBroadcastState([]string{"/repo"}, false, false)

	rendered := s.Render()
	if strings.Contains(rendered, "Run in containers") {
		t.Error("Container checkbox should not appear when unsupported")
	}
}

func TestBroadcastState_ContainerCheckbox_Render(t *testing.T) {
	s := NewBroadcastState([]string{"/repo"}, true, false)
	rendered := s.Render()

	if !strings.Contains(rendered, "Run in containers") {
		t.Error("Container checkbox should appear when supported")
	}
}

func TestNewSessionState_AuthWarning_WhenNoAuth(t *testing.T) {
	s := NewNewSessionState([]string{"/repo"}, true, false)
	s.UseContainers = true
	rendered := s.Render()

	if !strings.Contains(rendered, "ANTHROPIC_API_KEY") {
		t.Error("Auth warning should appear when containers checked and no auth")
	}
}

func TestNewSessionState_AuthWarning_WhenAuthAvailable(t *testing.T) {
	s := NewNewSessionState([]string{"/repo"}, true, true)
	s.UseContainers = true
	rendered := s.Render()

	if strings.Contains(rendered, "Requires ANTHROPIC_API_KEY") {
		t.Error("Auth warning should not appear when auth is available")
	}
}

func TestNewSessionState_AuthWarning_WhenContainersNotChecked(t *testing.T) {
	s := NewNewSessionState([]string{"/repo"}, true, false)
	rendered := s.Render()

	if strings.Contains(rendered, "Requires ANTHROPIC_API_KEY") {
		t.Error("Auth warning should not appear when containers not checked")
	}
}

func TestForkSessionState_AuthWarning_WhenNoAuth(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", true, true, false)
	rendered := s.Render()

	if !strings.Contains(rendered, "ANTHROPIC_API_KEY") {
		t.Error("Auth warning should appear when containers checked and no auth (inherited from parent)")
	}
}

func TestForkSessionState_AuthWarning_WhenAuthAvailable(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", true, true, true)
	rendered := s.Render()

	if strings.Contains(rendered, "Requires ANTHROPIC_API_KEY") {
		t.Error("Auth warning should not appear when auth is available")
	}
}

func TestBroadcastState_AuthWarning_WhenNoAuth(t *testing.T) {
	s := NewBroadcastState([]string{"/repo"}, true, false)
	s.UseContainers = true
	rendered := s.Render()

	if !strings.Contains(rendered, "ANTHROPIC_API_KEY") {
		t.Error("Auth warning should appear when containers checked and no auth")
	}
}

func TestBroadcastState_AuthWarning_WhenAuthAvailable(t *testing.T) {
	s := NewBroadcastState([]string{"/repo"}, true, true)
	s.UseContainers = true
	rendered := s.Render()

	if strings.Contains(rendered, "Requires ANTHROPIC_API_KEY") {
		t.Error("Auth warning should not appear when auth is available")
	}
}

// =============================================================================
// Settings container image field tests
// =============================================================================

func TestSettingsState_NumFields_WithContainers(t *testing.T) {
	// No repos, no PAT, but containers supported: theme + branch + notifs + container = 4
	s := newTestSettingsStateWithContainers("", false, nil, nil, 0, false, true, "")
	if n := s.numFields(); n != 4 {
		t.Errorf("Expected 4 fields with containers supported and no repos, got %d", n)
	}
}

func TestSettingsState_NumFields_WithContainersAndRepoAndPAT(t *testing.T) {
	// Repos + PAT + containers: theme + branch + notifs + container + repo + asana = 6
	s := newTestSettingsStateWithContainers("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": ""},
		0, true, true, "")
	if n := s.numFields(); n != 6 {
		t.Errorf("Expected 6 fields with containers, repo and PAT, got %d", n)
	}
}

func TestSettingsState_ContainerImageFocusIndex(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, nil, nil, 0, false, true, "")
	if idx := s.containerImageFocusIndex(); idx != 3 {
		t.Errorf("Expected container image focus index 3, got %d", idx)
	}
}

func TestSettingsState_RepoSelectorFocusIndex_WithContainers(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": ""},
		0, true, true, "")
	if idx := s.repoSelectorFocusIndex(); idx != 4 {
		t.Errorf("Expected repo selector focus index 4 with containers, got %d", idx)
	}
}

func TestSettingsState_RepoSelectorFocusIndex_WithoutContainers(t *testing.T) {
	s := newTestSettingsState("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": ""},
		0, true)
	if idx := s.repoSelectorFocusIndex(); idx != 3 {
		t.Errorf("Expected repo selector focus index 3 without containers, got %d", idx)
	}
}

func TestSettingsState_AsanaFocusIndex_WithContainers(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": ""},
		0, true, true, "")
	if idx := s.asanaFocusIndex(); idx != 5 {
		t.Errorf("Expected asana focus index 5 with containers, got %d", idx)
	}
}

func TestSettingsState_TabCycle_WithContainers(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false,
		[]string{"/some/repo"},
		map[string]string{"/some/repo": ""},
		0, true, true, "plural-claude")

	// 6 fields: theme(0) branch(1) notifs(2) container(3) repo(4) asana(5)
	expectedFoci := []int{1, 2, 3, 4, 5, 0}
	for i, expected := range expectedFoci {
		s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if s.Focus != expected {
			t.Errorf("After tab %d: expected focus %d, got %d", i+1, expected, s.Focus)
		}
	}
}

func TestSettingsState_GetContainerImage(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, nil, nil, 0, false, true, "my-image")
	if img := s.GetContainerImage(); img != "my-image" {
		t.Errorf("Expected container image 'my-image', got %q", img)
	}
}

func TestSettingsState_GetContainerImage_Default(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, nil, nil, 0, false, true, "")
	if img := s.GetContainerImage(); img != "" {
		t.Errorf("Expected empty container image, got %q", img)
	}
}

func TestSettingsState_ContainerImageInput_WhenFocused(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, nil, nil, 0, false, true, "plural-claude")

	// Focus on container image field
	s.Focus = s.containerImageFocusIndex()
	s.updateInputFocus()

	// Container image input should be focused
	if !s.ContainerImageInput.Focused() {
		t.Error("Container image input should be focused when focus is on container image index")
	}

	// Branch prefix should not be focused
	if s.BranchPrefixInput.Focused() {
		t.Error("Branch prefix input should not be focused when container image is focused")
	}
}

func TestSettingsState_Render_ContainerImageValue(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, nil, nil, 0, false, true, "custom-image")
	rendered := s.Render()

	if !strings.Contains(rendered, "Container image") {
		t.Error("Should show container image label")
	}
}

func TestContainerAuthHelp_Content(t *testing.T) {
	// Verify the ContainerAuthHelp constant mentions all three auth methods
	if !strings.Contains(ContainerAuthHelp, "ANTHROPIC_API_KEY") {
		t.Error("ContainerAuthHelp should mention ANTHROPIC_API_KEY")
	}
	if !strings.Contains(ContainerAuthHelp, "setup-token") {
		t.Error("ContainerAuthHelp should mention setup-token")
	}
	if !strings.Contains(ContainerAuthHelp, "keychain") {
		t.Error("ContainerAuthHelp should mention keychain")
	}
}

func TestNewSessionState_AuthWarning_UsesContainerAuthHelp(t *testing.T) {
	// Verify the auth warning renders when containers checked and no auth
	s := NewNewSessionState([]string{"/repo"}, true, false)
	s.UseContainers = true
	rendered := s.Render()

	// Check for keychain mention (word won't be split by lipgloss wrapping)
	if !strings.Contains(rendered, "keychain") {
		t.Error("Auth warning should mention keychain")
	}
}
