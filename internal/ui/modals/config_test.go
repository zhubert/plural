package modals

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// testThemes and testThemeNames are used across settings tests.
var (
	testThemes       = []string{"dark-purple", "nord", "dracula"}
	testThemeNames   = []string{"Dark Purple", "Nord", "Dracula"}
	testCurrentTheme = "dark-purple"
)

// newTestSettingsState is a helper that prepends theme data to NewSettingsState calls.
func newTestSettingsState(branchPrefix string, notifs bool) *SettingsState {
	return NewSettingsState(testThemes, testThemeNames, testCurrentTheme,
		branchPrefix, notifs, false, "") // no containers by default
}

// newTestSettingsStateWithContainers is like newTestSettingsState but with container support.
func newTestSettingsStateWithContainers(branchPrefix string, notifs bool,
	containersSupported bool, containerImage string) *SettingsState {
	return NewSettingsState(testThemes, testThemeNames, testCurrentTheme,
		branchPrefix, notifs, containersSupported, containerImage)
}

// =============================================================================
// SettingsState (global settings) tests
// =============================================================================

func TestSettingsState_NumFields_NoContainers(t *testing.T) {
	s := newTestSettingsState("", false)
	if n := s.numFields(); n != 5 {
		t.Errorf("Expected 5 fields without containers, got %d", n)
	}
}

func TestSettingsState_NumFields_WithContainers(t *testing.T) {
	// theme + branch + notifs + cleanup + broadcast + container + 4 autonomous global = 10
	s := newTestSettingsStateWithContainers("", false, true, "")
	if n := s.numFields(); n != 10 {
		t.Errorf("Expected 10 fields with containers supported, got %d", n)
	}
}

func TestSettingsState_TabCycle_NoContainers(t *testing.T) {
	s := newTestSettingsState("", false)

	if s.Focus != 0 {
		t.Fatalf("Expected initial focus 0, got %d", s.Focus)
	}

	// Tab through: 0 -> 1 -> 2 -> 3 -> 4 -> 0 (5 fields: theme, branch, notifs, cleanup, broadcast)
	expectedFoci := []int{1, 2, 3, 4, 0}
	for i, expected := range expectedFoci {
		s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if s.Focus != expected {
			t.Errorf("After tab %d: expected focus %d, got %d", i+1, expected, s.Focus)
		}
	}
}

func TestSettingsState_TabCycle_WithContainers(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, true, "plural-claude")

	// 10 fields: theme(0) branch(1) notifs(2) cleanup(3) broadcast(4) container(5)
	// autoAddress(6) maxTurns(7) maxDuration(8) maxConcurrent(9)
	expectedFoci := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}
	for i, expected := range expectedFoci {
		s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if s.Focus != expected {
			t.Errorf("After tab %d: expected focus %d, got %d", i+1, expected, s.Focus)
		}
	}
}

func TestSettingsState_Render_NoContainerSection_WhenUnsupported(t *testing.T) {
	s := newTestSettingsState("", false)
	rendered := s.Render()

	if strings.Contains(rendered, "Container image") {
		t.Error("Container image field should not appear when containers unsupported")
	}
}

func TestSettingsState_Render_ContainerSection_WhenSupported(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, true, "plural-claude")
	rendered := s.Render()

	if !strings.Contains(rendered, "Container image") {
		t.Error("Container image field should appear when containers supported")
	}
}

func TestSettingsState_PreferredWidth(t *testing.T) {
	s := newTestSettingsState("", false)
	if w := s.PreferredWidth(); w != ModalWidthWide {
		t.Errorf("Expected preferred width %d, got %d", ModalWidthWide, w)
	}
}

func TestSettingsState_ThemeSelector(t *testing.T) {
	s := newTestSettingsState("", false)

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
	s := newTestSettingsState("", false)

	// Focus on branch prefix (focus 1)
	s.Focus = 1

	// Left/Right should NOT change theme
	s.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if s.SelectedThemeIndex != 0 {
		t.Errorf("Theme should not change when not focused, got index %d", s.SelectedThemeIndex)
	}
}

func TestSettingsState_Render_ContainsThemeSection(t *testing.T) {
	s := newTestSettingsState("", false)
	rendered := s.Render()

	if !strings.Contains(rendered, "Theme:") {
		t.Error("Rendered settings should contain 'Theme:' label")
	}
	if !strings.Contains(rendered, "Dark Purple") {
		t.Error("Rendered settings should contain the selected theme display name")
	}
}

func TestSettingsState_HelpChangesOnThemeFocus(t *testing.T) {
	s := newTestSettingsState("", false)

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

func TestSettingsState_ContainerImageFocusIndex(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, true, "")
	if idx := s.containerImageFocusIndex(); idx != 5 {
		t.Errorf("Expected container image focus index 5, got %d", idx)
	}
}

func TestSettingsState_GetContainerImage(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, true, "my-image")
	if img := s.GetContainerImage(); img != "my-image" {
		t.Errorf("Expected container image 'my-image', got %q", img)
	}
}

func TestSettingsState_GetContainerImage_Default(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, true, "")
	if img := s.GetContainerImage(); img != "" {
		t.Errorf("Expected empty container image, got %q", img)
	}
}

func TestSettingsState_ContainerImageInput_WhenFocused(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, true, "plural-claude")

	s.Focus = s.containerImageFocusIndex()
	s.updateInputFocus()

	if !s.ContainerImageInput.Focused() {
		t.Error("Container image input should be focused when focus is on container image index")
	}
	if s.BranchPrefixInput.Focused() {
		t.Error("Branch prefix input should not be focused when container image is focused")
	}
}

func TestSettingsState_Render_ContainerImageValue(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, true, "custom-image")
	rendered := s.Render()

	if !strings.Contains(rendered, "Container image") {
		t.Error("Should show container image label")
	}
}

// --- Autonomous global settings tests ---

func TestSettingsState_AutonomousSection_HiddenWithoutContainers(t *testing.T) {
	s := newTestSettingsState("", false)
	rendered := s.Render()

	if strings.Contains(rendered, "Autonomous:") {
		t.Error("Autonomous section should not appear when containers unsupported")
	}
}

func TestSettingsState_AutonomousSection_ShownWithContainers(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, true, "")
	rendered := s.Render()

	if !strings.Contains(rendered, "Autonomous:") {
		t.Error("Autonomous section should appear when containers supported")
	}
	if !strings.Contains(rendered, "Auto-address PR comments") {
		t.Error("Auto-address PR comments field should appear")
	}
	if !strings.Contains(rendered, "Max autonomous turns") {
		t.Error("Max autonomous turns field should appear")
	}
	if !strings.Contains(rendered, "Max autonomous duration") {
		t.Error("Max autonomous duration field should appear")
	}
	if !strings.Contains(rendered, "Max concurrent auto-sessions") {
		t.Error("Max concurrent auto-sessions field should appear")
	}
}

func TestSettingsState_AutonomousFocusIndices(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, true, "")

	if idx := s.autoAddressFocusIndex(); idx != 6 {
		t.Errorf("Expected autoAddress focus index 6, got %d", idx)
	}
	if idx := s.autoMaxTurnsFocusIndex(); idx != 7 {
		t.Errorf("Expected autoMaxTurns focus index 7, got %d", idx)
	}
	if idx := s.autoMaxDurationFocusIndex(); idx != 8 {
		t.Errorf("Expected autoMaxDuration focus index 8, got %d", idx)
	}
	if idx := s.issueMaxConcurrentFocusIndex(); idx != 9 {
		t.Errorf("Expected issueMaxConcurrent focus index 9, got %d", idx)
	}
}

func TestSettingsState_AutonomousCheckboxToggle(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, true, "")

	// Toggle auto-address PR comments
	s.Focus = s.autoAddressFocusIndex()
	s.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !s.AutoAddressPRComments {
		t.Error("Space should toggle AutoAddressPRComments to true")
	}
	s.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if s.AutoAddressPRComments {
		t.Error("Space again should toggle AutoAddressPRComments to false")
	}
}

func TestSettingsState_AutonomousInputFocus(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, true, "")

	// Focus on max turns input
	s.Focus = s.autoMaxTurnsFocusIndex()
	s.updateInputFocus()
	if !s.AutoMaxTurnsInput.Focused() {
		t.Error("AutoMaxTurnsInput should be focused")
	}
	if s.AutoMaxDurationInput.Focused() {
		t.Error("AutoMaxDurationInput should not be focused")
	}
}

// Global settings should NOT contain per-repo fields
func TestSettingsState_Render_NoPerRepoSection(t *testing.T) {
	s := newTestSettingsState("", false)
	rendered := s.Render()

	if strings.Contains(rendered, "Per-repo settings") {
		t.Error("Global settings should not contain per-repo section")
	}
	if strings.Contains(rendered, "Issue polling") {
		t.Error("Global settings should not contain issue polling")
	}
	if strings.Contains(rendered, "Asana project") {
		t.Error("Global settings should not contain Asana project")
	}
}

// =============================================================================
// RepoSettingsState (per-repo settings) tests
// =============================================================================

func newTestRepoSettingsState(containersSupported bool, asanaPATSet bool) *RepoSettingsState {
	return NewRepoSettingsState("/path/to/myrepo", containersSupported, asanaPATSet,
		false, "", false, "")
}

func TestRepoSettingsState_Title(t *testing.T) {
	s := newTestRepoSettingsState(true, false)
	if s.Title() != "Repo Settings: myrepo" {
		t.Errorf("Expected title 'Repo Settings: myrepo', got %q", s.Title())
	}
}

func TestRepoSettingsState_NumFields_ContainersAndAsana(t *testing.T) {
	s := newTestRepoSettingsState(true, true)
	if n := s.numFields(); n != 4 {
		t.Errorf("Expected 4 fields (polling, label, merge, asana), got %d", n)
	}
}

func TestRepoSettingsState_NumFields_ContainersOnly(t *testing.T) {
	s := newTestRepoSettingsState(true, false)
	if n := s.numFields(); n != 3 {
		t.Errorf("Expected 3 fields (polling, label, merge), got %d", n)
	}
}

func TestRepoSettingsState_NumFields_AsanaOnly(t *testing.T) {
	s := newTestRepoSettingsState(false, true)
	if n := s.numFields(); n != 1 {
		t.Errorf("Expected 1 field (asana), got %d", n)
	}
}

func TestRepoSettingsState_NumFields_NoFields(t *testing.T) {
	s := newTestRepoSettingsState(false, false)
	if n := s.numFields(); n != 0 {
		t.Errorf("Expected 0 fields, got %d", n)
	}
}

func TestRepoSettingsState_TabCycle_WithContainersAndAsana(t *testing.T) {
	s := newTestRepoSettingsState(true, true)

	// 4 fields: issuePolling(0) issueLabel(1) autoMerge(2) asana(3)
	expectedFoci := []int{1, 2, 3, 0}
	for i, expected := range expectedFoci {
		s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if s.Focus != expected {
			t.Errorf("After tab %d: expected focus %d, got %d", i+1, expected, s.Focus)
		}
	}
}

func TestRepoSettingsState_CheckboxToggle(t *testing.T) {
	s := newTestRepoSettingsState(true, false)

	// Toggle issue polling
	s.Focus = s.issuePollingFocusIndex()
	s.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !s.IssuePolling {
		t.Error("Space should toggle IssuePolling to true")
	}
	s.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if s.IssuePolling {
		t.Error("Space again should toggle IssuePolling to false")
	}

	// Toggle auto-merge
	s.Focus = s.autoMergeFocusIndex()
	s.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !s.AutoMerge {
		t.Error("Space should toggle AutoMerge to true")
	}
}

func TestRepoSettingsState_Render_WithContainers(t *testing.T) {
	s := newTestRepoSettingsState(true, false)
	rendered := s.Render()

	if !strings.Contains(rendered, "Issue polling") {
		t.Error("Should show Issue polling field")
	}
	if !strings.Contains(rendered, "Issue filter label") {
		t.Error("Should show Issue filter label field")
	}
	if !strings.Contains(rendered, "Auto-merge after CI") {
		t.Error("Should show Auto-merge field")
	}
}

func TestRepoSettingsState_Render_WithAsana(t *testing.T) {
	s := newTestRepoSettingsState(false, true)
	rendered := s.Render()

	if !strings.Contains(rendered, "Asana project") {
		t.Error("Should show Asana project field when PAT set")
	}
	if !strings.Contains(rendered, "Fetching Asana projects") {
		t.Error("Should show loading state initially")
	}
}

func TestRepoSettingsState_Render_NoAsanaWithoutPAT(t *testing.T) {
	s := newTestRepoSettingsState(true, false)
	rendered := s.Render()

	if strings.Contains(rendered, "Asana project") {
		t.Error("Asana field should not appear when PAT not set")
	}
}

func TestRepoSettingsState_SetAsanaProjects(t *testing.T) {
	s := newTestRepoSettingsState(false, true)

	if !s.AsanaLoading {
		t.Error("Expected AsanaLoading to be true initially when PAT set")
	}

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
}

func TestRepoSettingsState_SetAsanaProjectsError(t *testing.T) {
	s := newTestRepoSettingsState(false, true)

	s.SetAsanaProjectsError("connection failed")

	if s.AsanaLoading {
		t.Error("Expected AsanaLoading to be false after error")
	}
	if s.AsanaLoadError != "connection failed" {
		t.Errorf("Expected error 'connection failed', got %q", s.AsanaLoadError)
	}
}

func TestRepoSettingsState_AsanaSearchFiltering(t *testing.T) {
	s := newTestRepoSettingsState(false, true)

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

func TestRepoSettingsState_AsanaNavigation(t *testing.T) {
	s := newTestRepoSettingsState(false, true)

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
}

func TestRepoSettingsState_AsanaSelectProject(t *testing.T) {
	s := newTestRepoSettingsState(false, true)

	options := []AsanaProjectOption{
		{GID: "", Name: "(none)"},
		{GID: "p1", Name: "Project Alpha"},
		{GID: "p2", Name: "Project Beta"},
	}
	s.SetAsanaProjects(options)

	s.Focus = s.asanaFocusIndex()
	s.updateInputFocus()

	// Navigate to "Project Alpha" (index 1)
	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})

	// Press Enter to select
	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if s.AsanaSelectedGID != "p1" {
		t.Errorf("Expected selected GID 'p1', got %q", s.AsanaSelectedGID)
	}

	if s.GetAsanaProject() != "p1" {
		t.Errorf("GetAsanaProject should return 'p1', got %q", s.GetAsanaProject())
	}
}

func TestRepoSettingsState_AsanaSelectNone(t *testing.T) {
	s := NewRepoSettingsState("/repo/a", false, true,
		false, "", false, "existing-gid")

	options := []AsanaProjectOption{
		{GID: "", Name: "(none)"},
		{GID: "p1", Name: "Project Alpha"},
	}
	s.SetAsanaProjects(options)

	s.Focus = s.asanaFocusIndex()
	s.updateInputFocus()

	// Cursor is at index 0 ((none)), press Enter
	s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if s.AsanaSelectedGID != "" {
		t.Errorf("Expected empty GID after selecting (none), got %q", s.AsanaSelectedGID)
	}
}

func TestRepoSettingsState_IsAsanaFocused(t *testing.T) {
	s := newTestRepoSettingsState(true, true)

	s.Focus = 0
	if s.IsAsanaFocused() {
		t.Error("Should not be Asana-focused when Focus is 0")
	}

	s.Focus = s.asanaFocusIndex()
	if !s.IsAsanaFocused() {
		t.Error("Should be Asana-focused when Focus is asanaFocusIndex")
	}

	// PAT not set: even at correct focus index, should not be Asana-focused
	s2 := newTestRepoSettingsState(true, false)
	s2.Focus = 0
	if s2.IsAsanaFocused() {
		t.Error("Should not be Asana-focused when PAT is not set")
	}
}

func TestRepoSettingsState_HelpChangesOnAsanaFocus(t *testing.T) {
	s := newTestRepoSettingsState(true, true)

	s.Focus = s.asanaFocusIndex()
	help := s.Help()
	if !strings.Contains(help, "Up/Down: navigate") {
		t.Errorf("Help at Asana focus should mention Up/Down: navigate, got %q", help)
	}
}

func TestRepoSettingsState_Render_AsanaLoading(t *testing.T) {
	s := newTestRepoSettingsState(false, true)

	rendered := s.Render()
	if !strings.Contains(rendered, "Fetching Asana projects") {
		t.Error("Should show loading message when AsanaLoading is true")
	}
}

func TestRepoSettingsState_Render_AsanaError(t *testing.T) {
	s := newTestRepoSettingsState(false, true)

	s.SetAsanaProjectsError("timeout")

	rendered := s.Render()
	if !strings.Contains(rendered, "timeout") {
		t.Error("Should show error message")
	}
}

func TestRepoSettingsState_Render_AsanaProjectList(t *testing.T) {
	s := NewRepoSettingsState("/repo/a", false, true,
		false, "", false, "p1")

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

func TestRepoSettingsState_Render_AsanaCurrentNone(t *testing.T) {
	s := newTestRepoSettingsState(false, true)

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

func TestRepoSettingsState_GetIssueLabel(t *testing.T) {
	s := NewRepoSettingsState("/repo", true, false,
		false, "my-label", false, "")

	if label := s.GetIssueLabel(); label != "my-label" {
		t.Errorf("Expected issue label 'my-label', got %q", label)
	}
}

func TestRepoSettingsState_InputFocus(t *testing.T) {
	s := newTestRepoSettingsState(true, true)

	// Focus on issue label input
	s.Focus = s.issueLabelFocusIndex()
	s.updateInputFocus()
	if !s.IssueLabelInput.Focused() {
		t.Error("IssueLabelInput should be focused")
	}
	if s.AsanaSearchInput.Focused() {
		t.Error("AsanaSearchInput should not be focused")
	}

	// Focus on asana
	s.Focus = s.asanaFocusIndex()
	s.updateInputFocus()
	if !s.AsanaSearchInput.Focused() {
		t.Error("AsanaSearchInput should be focused")
	}
	if s.IssueLabelInput.Focused() {
		t.Error("IssueLabelInput should not be focused")
	}
}

func TestRepoSettingsState_FocusIndices_ContainersAndAsana(t *testing.T) {
	s := newTestRepoSettingsState(true, true)

	if idx := s.issuePollingFocusIndex(); idx != 0 {
		t.Errorf("Expected issuePolling focus index 0, got %d", idx)
	}
	if idx := s.issueLabelFocusIndex(); idx != 1 {
		t.Errorf("Expected issueLabel focus index 1, got %d", idx)
	}
	if idx := s.autoMergeFocusIndex(); idx != 2 {
		t.Errorf("Expected autoMerge focus index 2, got %d", idx)
	}
	if idx := s.asanaFocusIndex(); idx != 3 {
		t.Errorf("Expected asana focus index 3, got %d", idx)
	}
}

func TestRepoSettingsState_FocusIndices_AsanaOnly(t *testing.T) {
	s := newTestRepoSettingsState(false, true)

	if idx := s.asanaFocusIndex(); idx != 0 {
		t.Errorf("Expected asana focus index 0 without containers, got %d", idx)
	}
}

// =============================================================================
// NewSessionState tests (unchanged)
// =============================================================================

func TestNewSessionState_ContainerCheckbox_WhenSupported(t *testing.T) {
	s := NewNewSessionState([]string{"/repo"}, true, false)

	if s.numFields() != 5 {
		t.Errorf("Expected 5 fields with containers supported, got %d", s.numFields())
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

func TestContainerAuthHelp_Content(t *testing.T) {
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
	s := NewNewSessionState([]string{"/repo"}, true, false)
	s.UseContainers = true
	rendered := s.Render()

	if !strings.Contains(rendered, "keychain") {
		t.Error("Auth warning should mention keychain")
	}
}
