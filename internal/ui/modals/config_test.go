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
		branchPrefix, notifs, false, "", false, false, false)
}

// newTestSettingsStateWithContainers is like newTestSettingsState but with container support.
func newTestSettingsStateWithContainers(branchPrefix string, notifs bool,
	containersSupported bool, containerImage string) *SettingsState {
	return NewSettingsState(testThemes, testThemeNames, testCurrentTheme,
		branchPrefix, notifs, containersSupported, containerImage, false, false, false)
}

// =============================================================================
// SettingsState (global settings) tests
// =============================================================================

func TestSettingsState_Render_NoContainerSection_WhenUnsupported(t *testing.T) {
	s := newTestSettingsState("", false)

	if s.ContainersSupported {
		t.Error("ContainersSupported should be false")
	}

	// General settings should render correctly even without container support
	rendered := s.Render()
	for _, expected := range []string{"Settings", "Theme", "Default branch prefix", "Options"} {
		if !strings.Contains(rendered, expected) {
			t.Errorf("should contain %q in rendered output", expected)
		}
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

func TestSettingsState_InitialTheme(t *testing.T) {
	s := newTestSettingsState("", false)

	if s.GetSelectedTheme() != "dark-purple" {
		t.Errorf("Expected selected theme 'dark-purple', got %q", s.GetSelectedTheme())
	}
	if s.ThemeChanged() {
		t.Error("Theme should not be changed initially")
	}
	if s.OriginalTheme != "dark-purple" {
		t.Errorf("Expected original theme 'dark-purple', got %q", s.OriginalTheme)
	}
}

func TestSettingsState_Render_ContainsThemeSection(t *testing.T) {
	s := newTestSettingsState("", false)
	rendered := s.Render()

	if !strings.Contains(rendered, "Theme") {
		t.Error("Rendered settings should contain 'Theme' label")
	}
	if !strings.Contains(rendered, "Dark Purple") {
		t.Error("Rendered settings should contain the selected theme display name")
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

func TestSettingsState_Render_ContainerImageValue(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, true, "custom-image")
	rendered := s.Render()

	if !strings.Contains(rendered, "Container image") {
		t.Error("Should show container image label")
	}
}

func TestSettingsState_GetBranchPrefix(t *testing.T) {
	s := newTestSettingsState("my-prefix/", false)
	if prefix := s.GetBranchPrefix(); prefix != "my-prefix/" {
		t.Errorf("Expected branch prefix 'my-prefix/', got %q", prefix)
	}
}

func TestSettingsState_SetBranchPrefix(t *testing.T) {
	s := newTestSettingsState("", false)
	s.SetBranchPrefix("new-prefix/")
	if prefix := s.GetBranchPrefix(); prefix != "new-prefix/" {
		t.Errorf("Expected branch prefix 'new-prefix/', got %q", prefix)
	}
}

func TestSettingsState_GetNotificationsEnabled(t *testing.T) {
	s := newTestSettingsState("", true)
	if !s.GetNotificationsEnabled() {
		t.Error("Expected notifications to be enabled")
	}

	s2 := newTestSettingsState("", false)
	if s2.GetNotificationsEnabled() {
		t.Error("Expected notifications to be disabled")
	}
}

func TestSettingsState_GetAutoMaxTurns(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, true, "")
	s.SetAutoMaxTurns("100")
	if v := s.GetAutoMaxTurns(); v != "100" {
		t.Errorf("Expected auto max turns '100', got %q", v)
	}
}

func TestSettingsState_GetAutoMaxDuration(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, true, "")
	s.SetAutoMaxDuration("60")
	if v := s.GetAutoMaxDuration(); v != "60" {
		t.Errorf("Expected auto max duration '60', got %q", v)
	}
}

func TestSettingsState_GetIssueMaxConcurrent(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, true, "")
	s.SetIssueMaxConcurrent("5")
	if v := s.GetIssueMaxConcurrent(); v != "5" {
		t.Errorf("Expected issue max concurrent '5', got %q", v)
	}
}

func TestSettingsState_HelpText(t *testing.T) {
	s := newTestSettingsState("", false)
	help := s.Help()
	if !strings.Contains(help, "Tab") {
		t.Errorf("Help should mention Tab, got %q", help)
	}
	if !strings.Contains(help, "Enter") {
		t.Errorf("Help should mention Enter, got %q", help)
	}
	if !strings.Contains(help, "Esc") {
		t.Errorf("Help should mention Esc, got %q", help)
	}
}

func TestSettingsState_Title(t *testing.T) {
	s := newTestSettingsState("", false)
	if s.Title() != "Settings" {
		t.Errorf("Expected title 'Settings', got %q", s.Title())
	}
}

// --- Autonomous global settings tests ---

func TestSettingsState_AutonomousSection_HiddenWithoutContainers(t *testing.T) {
	s := newTestSettingsState("", false)

	if s.ContainersSupported {
		t.Error("ContainersSupported should be false")
	}

	// General settings should render correctly
	rendered := s.Render()
	if !strings.Contains(rendered, "Settings") {
		t.Error("should contain title")
	}

	// Autonomous-related getters should return defaults when containers not supported
	if s.GetAutoMaxTurns() != "" {
		t.Errorf("auto max turns should be empty without containers, got %q", s.GetAutoMaxTurns())
	}
	if s.GetAutoMaxDuration() != "" {
		t.Errorf("auto max duration should be empty without containers, got %q", s.GetAutoMaxDuration())
	}
	if s.GetIssueMaxConcurrent() != "" {
		t.Errorf("issue max concurrent should be empty without containers, got %q", s.GetIssueMaxConcurrent())
	}
	if s.AutoAddressPRComments {
		t.Error("AutoAddressPRComments should default to false without containers")
	}
}

func TestSettingsState_AutonomousSection_ShownWithContainers(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, true, "")
	rendered := s.Render()

	if !strings.Contains(rendered, "Autonomous options") {
		t.Error("Autonomous options section should appear")
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

func TestSettingsState_AutoAddressPRComments_Default(t *testing.T) {
	s := newTestSettingsStateWithContainers("", false, true, "")
	if s.AutoAddressPRComments {
		t.Error("AutoAddressPRComments should default to false")
	}
}

func TestSettingsState_ContainersSupported(t *testing.T) {
	s := newTestSettingsState("", false)
	if s.ContainersSupported {
		t.Error("ContainersSupported should be false without containers")
	}

	s2 := newTestSettingsStateWithContainers("", false, true, "")
	if !s2.ContainersSupported {
		t.Error("ContainersSupported should be true with containers")
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
		false, false, "")
}

func TestRepoSettingsState_Title(t *testing.T) {
	s := newTestRepoSettingsState(true, false)
	if s.Title() != "Repo Settings: myrepo" {
		t.Errorf("Expected title 'Repo Settings: myrepo', got %q", s.Title())
	}
}

func TestRepoSettingsState_NumFields_ContainersAndAsana(t *testing.T) {
	s := newTestRepoSettingsState(true, true)
	if n := s.numFields(); n != 3 {
		t.Errorf("Expected 3 fields (polling, merge, asana), got %d", n)
	}
}

func TestRepoSettingsState_NumFields_ContainersOnly(t *testing.T) {
	s := newTestRepoSettingsState(true, false)
	if n := s.numFields(); n != 2 {
		t.Errorf("Expected 2 fields (polling, merge), got %d", n)
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

	// 3 fields: issuePolling(0) autoMerge(1) asana(2)
	expectedFoci := []int{1, 2, 0}
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
		false, false, "existing-gid")

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
		false, false, "p1")

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

func TestRepoSettingsState_InputFocus(t *testing.T) {
	s := newTestRepoSettingsState(true, true)

	// Focus on asana
	s.Focus = s.asanaFocusIndex()
	s.updateInputFocus()
	if !s.AsanaSearchInput.Focused() {
		t.Error("AsanaSearchInput should be focused")
	}

	// Focus away from asana
	s.Focus = s.issuePollingFocusIndex()
	s.updateInputFocus()
	if s.AsanaSearchInput.Focused() {
		t.Error("AsanaSearchInput should not be focused when on issuePolling")
	}
}

func TestRepoSettingsState_FocusIndices_ContainersAndAsana(t *testing.T) {
	s := newTestRepoSettingsState(true, true)

	if idx := s.issuePollingFocusIndex(); idx != 0 {
		t.Errorf("Expected issuePolling focus index 0, got %d", idx)
	}
	if idx := s.autoMergeFocusIndex(); idx != 1 {
		t.Errorf("Expected autoMerge focus index 1, got %d", idx)
	}
	if idx := s.asanaFocusIndex(); idx != 2 {
		t.Errorf("Expected asana focus index 2, got %d", idx)
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

	// Tab to container checkbox (focus 4)
	s.Focus = 4
	s.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !s.UseContainers {
		t.Error("Space at focus 4 should toggle container checkbox when supported")
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

// =============================================================================
// ForkSessionState tests
// =============================================================================

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

func TestForkSessionState_ContainerOption_WhenSupported(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", true, true, false)

	// Container option should be enabled via MultiSelect when parent was containerized
	if !s.GetUseContainers() {
		t.Error("UseContainers should be true when inherited from containerized parent")
	}

	// Non-containerized parent
	s2 := NewForkSessionState("parent", "parent-id", "/repo", false, true, false)
	if s2.GetUseContainers() {
		t.Error("UseContainers should be false when parent was not containerized")
	}
}

func TestForkSessionState_ContainerOption_WhenUnsupported(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, false, false)
	rendered := s.Render()

	// Without containers supported, the container option should not be in the form
	if strings.Contains(rendered, "Run in container") {
		t.Error("Container option should not appear when unsupported")
	}
}

func TestForkSessionState_DefaultCopyMessages(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, true, false)
	if !s.ShouldCopyMessages() {
		t.Error("Fork should default to copying messages")
	}
}

func TestForkSessionState_GetBranchName(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, true, false)
	s.SetBranchName("my-branch")
	if name := s.GetBranchName(); name != "my-branch" {
		t.Errorf("Expected branch name 'my-branch', got %q", name)
	}
}

func TestForkSessionState_GetUseContainers(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", true, true, false)
	if !s.GetUseContainers() {
		t.Error("GetUseContainers should return true when inherited from parent")
	}

	s2 := NewForkSessionState("parent", "parent-id", "/repo", false, true, false)
	if s2.GetUseContainers() {
		t.Error("GetUseContainers should return false when parent was not containerized")
	}
}

func TestForkSessionState_HelpText(t *testing.T) {
	s := NewForkSessionState("parent", "parent-id", "/repo", false, true, false)
	help := s.Help()
	if !strings.Contains(help, "Tab") {
		t.Errorf("Help should mention Tab, got %q", help)
	}
	if !strings.Contains(help, "Enter") {
		t.Errorf("Help should mention Enter, got %q", help)
	}
	if !strings.Contains(help, "Esc") {
		t.Errorf("Help should mention Esc, got %q", help)
	}
}

func TestForkSessionState_Render_ShowsParentName(t *testing.T) {
	s := NewForkSessionState("my-parent-session", "parent-id", "/repo", false, true, false)
	rendered := s.Render()
	if !strings.Contains(rendered, "my-parent-session") {
		t.Error("Render should show parent session name")
	}
}

// =============================================================================
// BroadcastState tests (unchanged)
// =============================================================================

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
