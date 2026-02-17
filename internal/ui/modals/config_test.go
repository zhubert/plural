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
		branchPrefix, notifs, false, "", false)
}

// newTestSettingsStateWithContainers is like newTestSettingsState but with container support.
func newTestSettingsStateWithContainers(branchPrefix string, notifs bool,
	containersSupported bool, containerImage string) *SettingsState {
	return NewSettingsState(testThemes, testThemeNames, testCurrentTheme,
		branchPrefix, notifs, containersSupported, containerImage, false)
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

// =============================================================================
// RepoSettingsState (per-repo settings) tests
// =============================================================================

func newTestRepoSettingsState(asanaPATSet bool) *RepoSettingsState {
	return NewRepoSettingsState("/path/to/myrepo", asanaPATSet, "")
}

func TestRepoSettingsState_Title(t *testing.T) {
	s := newTestRepoSettingsState(false)
	if s.Title() != "Repo Settings: myrepo" {
		t.Errorf("Expected title 'Repo Settings: myrepo', got %q", s.Title())
	}
}

func TestRepoSettingsState_Render_LoadingState(t *testing.T) {
	s := newTestRepoSettingsState(true)
	rendered := s.Render()

	if !strings.Contains(rendered, "Fetching Asana projects") {
		t.Error("Should show loading state initially when PAT set")
	}
}

func TestRepoSettingsState_Render_NoAsanaWithoutPAT(t *testing.T) {
	s := newTestRepoSettingsState(false)
	rendered := s.Render()

	if !strings.Contains(rendered, "No per-repo settings") {
		t.Error("Should show no-settings message when PAT not set")
	}
}

func TestRepoSettingsState_SetAsanaProjects(t *testing.T) {
	s := newTestRepoSettingsState(true)

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
	if s.form == nil {
		t.Error("Expected form to be created after SetAsanaProjects")
	}
}

func TestRepoSettingsState_SetAsanaProjectsError(t *testing.T) {
	s := newTestRepoSettingsState(true)

	s.SetAsanaProjectsError("connection failed")

	if s.AsanaLoading {
		t.Error("Expected AsanaLoading to be false after error")
	}
	if s.AsanaLoadError != "connection failed" {
		t.Errorf("Expected error 'connection failed', got %q", s.AsanaLoadError)
	}
}

func TestRepoSettingsState_Render_AsanaError(t *testing.T) {
	s := newTestRepoSettingsState(true)

	s.SetAsanaProjectsError("timeout")

	rendered := s.Render()
	if !strings.Contains(rendered, "timeout") {
		t.Error("Should show error message")
	}
}

func TestRepoSettingsState_Render_AsanaProjectList(t *testing.T) {
	s := NewRepoSettingsState("/repo/a", true, "p1")

	options := []AsanaProjectOption{
		{GID: "", Name: "(none)"},
		{GID: "p1", Name: "My Project"},
	}
	s.SetAsanaProjects(options)

	rendered := s.Render()
	if !strings.Contains(rendered, "My Project") {
		t.Error("Should show project name in rendered output")
	}
	if !strings.Contains(rendered, "Asana project") {
		t.Error("Should show Asana project title")
	}
}

func TestRepoSettingsState_GetAsanaProject(t *testing.T) {
	s := NewRepoSettingsState("/repo/a", true, "p1")
	if s.GetAsanaProject() != "p1" {
		t.Errorf("Expected 'p1', got %q", s.GetAsanaProject())
	}
}

func TestRepoSettingsState_SelectUpdatesValue(t *testing.T) {
	s := newTestRepoSettingsState(true)

	options := []AsanaProjectOption{
		{GID: "", Name: "(none)"},
		{GID: "p1", Name: "Project Alpha"},
		{GID: "p2", Name: "Project Beta"},
	}
	s.SetAsanaProjects(options)

	// Navigate down to select "Project Alpha" — huh Select updates bound value on navigation
	s.Update(tea.KeyPressMsg{Code: tea.KeyDown})

	if s.AsanaSelectedGID != "p1" {
		t.Errorf("Expected selected GID 'p1' after navigating down, got %q", s.AsanaSelectedGID)
	}
}

func TestRepoSettingsState_Help_WithAsana(t *testing.T) {
	s := newTestRepoSettingsState(true)

	options := []AsanaProjectOption{
		{GID: "", Name: "(none)"},
	}
	s.SetAsanaProjects(options)

	help := s.Help()
	if !strings.Contains(help, "Up/Down") {
		t.Errorf("Help should mention Up/Down, got %q", help)
	}
}

func TestRepoSettingsState_Help_NoAsana(t *testing.T) {
	s := newTestRepoSettingsState(false)
	help := s.Help()
	if !strings.Contains(help, "Esc") {
		t.Errorf("Help should mention Esc, got %q", help)
	}
}

func TestRepoSettingsState_PreferredWidth(t *testing.T) {
	s := newTestRepoSettingsState(false)
	if w := s.PreferredWidth(); w != ModalWidthWide {
		t.Errorf("Expected preferred width %d, got %d", ModalWidthWide, w)
	}
}

// =============================================================================
// NewSessionState tests (unchanged)
// =============================================================================

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
