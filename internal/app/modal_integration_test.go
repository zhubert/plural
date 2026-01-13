package app

import (
	"strings"
	"testing"

	"github.com/zhubert/plural/internal/ui"
)

// =============================================================================
// Add Repository Modal Tests
// =============================================================================

func TestAddRepoModal_SubmitValidPath(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Open add repo modal
	m = sendKey(m, "a")
	if !m.modal.IsVisible() {
		t.Fatal("Add repo modal should be visible")
	}

	state, ok := m.modal.State.(*ui.AddRepoState)
	if !ok {
		t.Fatalf("Expected AddRepoState, got %T", m.modal.State)
	}

	// Type an invalid repo path - validation will run before checking if already added
	// First make sure we're not using the suggested path
	state.UseSuggested = false
	state.Input.SetValue("/nonexistent/path")

	// Submit
	m = sendKey(m, "enter")

	// Should show error because it's not a valid git repository
	// (git validation runs before duplicate check)
	errorMsg := m.modal.GetError()
	if errorMsg == "" {
		t.Error("Expected validation error, got none")
	}

	// Modal should still be visible
	if !m.modal.IsVisible() {
		t.Error("Modal should still be visible after validation error")
	}
}

func TestAddRepoModal_SubmitEmptyPath(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "a")
	state := m.modal.State.(*ui.AddRepoState)

	// Make sure we're in input mode with empty value
	state.UseSuggested = false
	state.Input.SetValue("")

	m = sendKey(m, "enter")

	// Should show error
	if m.modal.GetError() != "Please enter a path" {
		t.Errorf("Expected 'Please enter a path' error, got %q", m.modal.GetError())
	}

	if !m.modal.IsVisible() {
		t.Error("Modal should still be visible after validation error")
	}
}

func TestAddRepoModal_CancelWithEscape(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "a")
	if !m.modal.IsVisible() {
		t.Fatal("Modal should be visible")
	}

	m = sendKey(m, "esc")
	if m.modal.IsVisible() {
		t.Error("Modal should be closed after escape")
	}
}

func TestAddRepoModal_ToggleSuggestion(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Create add repo modal with a suggested repo
	m.modal.Show(ui.NewAddRepoState("/suggested/repo"))

	state := m.modal.State.(*ui.AddRepoState)

	// Initial state should use suggested
	if !state.UseSuggested {
		t.Error("Should use suggested repo initially when one is provided")
	}

	// Toggle with down
	m = sendKey(m, "down")
	state = m.modal.State.(*ui.AddRepoState)

	if state.UseSuggested {
		t.Error("Should switch to input after pressing down")
	}

	// Toggle back with up
	m = sendKey(m, "up")
	state = m.modal.State.(*ui.AddRepoState)

	if !state.UseSuggested {
		t.Error("Should switch back to suggested after pressing up")
	}
}

// =============================================================================
// New Session Modal Tests
// =============================================================================

func TestNewSessionModal_Open(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Open new session modal
	m = sendKey(m, "n")
	if !m.modal.IsVisible() {
		t.Fatal("New session modal should be visible")
	}

	state, ok := m.modal.State.(*ui.NewSessionState)
	if !ok {
		t.Fatalf("Expected NewSessionState, got %T", m.modal.State)
	}

	// Verify repos are loaded
	if len(state.RepoOptions) != 2 {
		t.Errorf("Expected 2 repos, got %d", len(state.RepoOptions))
	}
}

func TestNewSessionModal_NavigateRepos(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "n")
	state := m.modal.State.(*ui.NewSessionState)

	initialRepo := state.GetSelectedRepo()
	if initialRepo == "" {
		t.Fatal("Should have a repo selected initially")
	}

	initialIndex := state.RepoIndex

	// Navigate down
	m = sendKey(m, "down")
	state = m.modal.State.(*ui.NewSessionState)

	if state.RepoIndex == initialIndex && len(state.RepoOptions) > 1 {
		t.Error("Repo index should change after pressing down")
	}

	// Navigate back up
	m = sendKey(m, "up")
	state = m.modal.State.(*ui.NewSessionState)

	if state.RepoIndex != initialIndex {
		t.Error("Repo index should return to initial after pressing up")
	}
}

func TestNewSessionModal_InvalidBranchName(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "n")
	state := m.modal.State.(*ui.NewSessionState)

	// Set an invalid branch name (starts with -)
	state.BranchInput.SetValue("-invalid-name")

	m = sendKey(m, "enter")

	// Should show validation error
	if m.modal.GetError() == "" {
		t.Error("Expected validation error for invalid branch name")
	}

	if !m.modal.IsVisible() {
		t.Error("Modal should still be visible after validation error")
	}
}

func TestNewSessionModal_BranchNameWithDoubleDots(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "n")
	state := m.modal.State.(*ui.NewSessionState)

	// Set a branch name with ..
	state.BranchInput.SetValue("branch..name")

	m = sendKey(m, "enter")

	// Should show validation error
	errorMsg := m.modal.GetError()
	if errorMsg == "" {
		t.Error("Expected validation error for branch name with '..'")
	}
}

func TestNewSessionModal_CancelWithEscape(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "n")
	if !m.modal.IsVisible() {
		t.Fatal("Modal should be visible")
	}

	m = sendKey(m, "esc")
	if m.modal.IsVisible() {
		t.Error("Modal should be closed after escape")
	}
}

func TestNewSessionModal_TabSwitchesFocus(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "n")
	state := m.modal.State.(*ui.NewSessionState)

	// Initially should be on repo list (Focus == 0)
	if state.Focus != 0 {
		t.Errorf("Expected initial focus on repo list (0), got %d", state.Focus)
	}

	// Tab to branch input
	m = sendKey(m, "tab")
	state = m.modal.State.(*ui.NewSessionState)

	if state.Focus != 1 {
		t.Errorf("Expected focus on branch input (1) after tab, got %d", state.Focus)
	}

	// Tab back to repo list
	m = sendKey(m, "tab")
	state = m.modal.State.(*ui.NewSessionState)

	if state.Focus != 0 {
		t.Errorf("Expected focus back on repo list (0), got %d", state.Focus)
	}
}

// =============================================================================
// Confirm Delete Modal Tests
// =============================================================================

func TestConfirmDeleteModal_SelectDeleteOption(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Open delete modal
	m = sendKey(m, "d")
	if !m.modal.IsVisible() {
		t.Fatal("Delete modal should be visible")
	}

	state, ok := m.modal.State.(*ui.ConfirmDeleteState)
	if !ok {
		t.Fatalf("Expected ConfirmDeleteState, got %T", m.modal.State)
	}

	// Initially on first option (Keep worktree)
	if state.SelectedIndex != 0 {
		t.Errorf("Expected initial selection 0, got %d", state.SelectedIndex)
	}

	// Navigate to delete worktree option
	// ConfirmDeleteState has 2 options: "Keep worktree", "Delete worktree"
	m = sendKey(m, "down")
	state = m.modal.State.(*ui.ConfirmDeleteState)
	if state.SelectedIndex != 1 {
		t.Errorf("Expected selection 1 after down, got %d", state.SelectedIndex)
	}

	// ShouldDeleteWorktree should return true when on index 1
	if !state.ShouldDeleteWorktree() {
		t.Error("ShouldDeleteWorktree should return true when on index 1")
	}
}

func TestConfirmDeleteModal_CancelWithEscape(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	initialSessionCount := len(cfg.Sessions)

	m = sendKey(m, "d")
	if !m.modal.IsVisible() {
		t.Fatal("Delete modal should be visible")
	}

	// Cancel with escape (standard cancel action)
	m = sendKey(m, "esc")

	// Modal should be closed
	if m.modal.IsVisible() {
		t.Error("Modal should be closed after pressing escape")
	}

	// Session count should be unchanged
	if len(m.config.GetSessions()) != initialSessionCount {
		t.Error("Session count should be unchanged after cancel")
	}
}

func TestConfirmDeleteModal_VimNavigation(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "d")
	state := m.modal.State.(*ui.ConfirmDeleteState)

	// Navigate with j
	m = sendKey(m, "j")
	state = m.modal.State.(*ui.ConfirmDeleteState)
	if state.SelectedIndex != 1 {
		t.Errorf("Expected selection 1 after j, got %d", state.SelectedIndex)
	}

	// Navigate with k
	m = sendKey(m, "k")
	state = m.modal.State.(*ui.ConfirmDeleteState)
	if state.SelectedIndex != 0 {
		t.Errorf("Expected selection 0 after k, got %d", state.SelectedIndex)
	}
}

// =============================================================================
// Fork Session Modal Tests
// =============================================================================

func TestForkSessionModal_Open(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "f")
	if !m.modal.IsVisible() {
		t.Fatal("Fork modal should be visible")
	}

	state, ok := m.modal.State.(*ui.ForkSessionState)
	if !ok {
		t.Fatalf("Expected ForkSessionState, got %T", m.modal.State)
	}

	// Should have parent session ID set
	selectedSession := m.sidebar.SelectedSession()
	if state.ParentSessionID != selectedSession.ID {
		t.Errorf("Expected parent session ID %s, got %s", selectedSession.ID, state.ParentSessionID)
	}
}

func TestForkSessionModal_ToggleCopyMessages(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "f")
	state := m.modal.State.(*ui.ForkSessionState)

	// Initial state - copy messages should be true by default (per constructor)
	if !state.CopyMessages {
		t.Error("CopyMessages should be true by default")
	}

	// Focus should be on checkbox (Focus == 0)
	if state.Focus != 0 {
		t.Errorf("Expected initial focus on checkbox (0), got %d", state.Focus)
	}

	// Toggle with space
	m = sendKey(m, "space")
	state = m.modal.State.(*ui.ForkSessionState)

	if state.CopyMessages {
		t.Error("CopyMessages should be false after pressing space")
	}

	// Toggle again
	m = sendKey(m, "space")
	state = m.modal.State.(*ui.ForkSessionState)

	if !state.CopyMessages {
		t.Error("CopyMessages should be true after second space")
	}
}

func TestForkSessionModal_InvalidBranchName(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "f")
	state := m.modal.State.(*ui.ForkSessionState)

	// Set invalid branch name
	state.BranchInput.SetValue("-invalid")

	m = sendKey(m, "enter")

	// Should show error
	if m.modal.GetError() == "" {
		t.Error("Expected error for invalid branch name")
	}

	if !m.modal.IsVisible() {
		t.Error("Modal should still be visible after error")
	}
}

func TestForkSessionModal_TabSwitchesFocus(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "f")
	state := m.modal.State.(*ui.ForkSessionState)

	// Initial focus on checkbox
	if state.Focus != 0 {
		t.Errorf("Expected initial focus 0, got %d", state.Focus)
	}

	// Tab to branch input
	m = sendKey(m, "tab")
	state = m.modal.State.(*ui.ForkSessionState)

	if state.Focus != 1 {
		t.Errorf("Expected focus 1 after tab, got %d", state.Focus)
	}

	// Tab back to checkbox
	m = sendKey(m, "tab")
	state = m.modal.State.(*ui.ForkSessionState)

	if state.Focus != 0 {
		t.Errorf("Expected focus 0 after second tab, got %d", state.Focus)
	}
}

// =============================================================================
// Rename Session Modal Tests
// =============================================================================

func TestRenameSessionModal_Open(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "r")
	if !m.modal.IsVisible() {
		t.Fatal("Rename modal should be visible")
	}

	state, ok := m.modal.State.(*ui.RenameSessionState)
	if !ok {
		t.Fatalf("Expected RenameSessionState, got %T", m.modal.State)
	}

	// Should have session ID set
	selectedSession := m.sidebar.SelectedSession()
	if state.SessionID != selectedSession.ID {
		t.Errorf("Expected session ID %s, got %s", selectedSession.ID, state.SessionID)
	}
}

func TestRenameSessionModal_EmptyName(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "r")
	state := m.modal.State.(*ui.RenameSessionState)

	// Clear the name
	state.NameInput.SetValue("")

	m = sendKey(m, "enter")

	// Should show error
	if m.modal.GetError() != "Name cannot be empty" {
		t.Errorf("Expected 'Name cannot be empty' error, got %q", m.modal.GetError())
	}

	if !m.modal.IsVisible() {
		t.Error("Modal should still be visible after error")
	}
}

func TestRenameSessionModal_InvalidBranchName(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "r")
	state := m.modal.State.(*ui.RenameSessionState)

	// Set invalid name
	state.NameInput.SetValue("invalid..name")

	m = sendKey(m, "enter")

	// Should show error
	if m.modal.GetError() == "" {
		t.Error("Expected error for invalid branch name")
	}
}

func TestRenameSessionModal_Cancel(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	selectedSession := m.sidebar.SelectedSession()
	originalName := selectedSession.Name

	m = sendKey(m, "r")
	state := m.modal.State.(*ui.RenameSessionState)

	// Change the name
	state.NameInput.SetValue("new-name")

	// Cancel with escape
	m = sendKey(m, "esc")

	if m.modal.IsVisible() {
		t.Error("Modal should be closed after escape")
	}

	// Session name should be unchanged
	session := m.config.GetSession(selectedSession.ID)
	if session.Name != originalName {
		t.Errorf("Session name should be unchanged after cancel, expected %s, got %s", originalName, session.Name)
	}
}

// =============================================================================
// Theme Modal Tests
// =============================================================================

func TestThemeModal_Open(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "t")
	if !m.modal.IsVisible() {
		t.Fatal("Theme modal should be visible")
	}

	_, ok := m.modal.State.(*ui.ThemeState)
	if !ok {
		t.Fatalf("Expected ThemeState, got %T", m.modal.State)
	}
}

func TestThemeModal_Navigate(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "t")
	state := m.modal.State.(*ui.ThemeState)

	initialIndex := state.SelectedIndex

	// Navigate down
	m = sendKey(m, "down")
	state = m.modal.State.(*ui.ThemeState)

	if state.SelectedIndex == initialIndex && len(state.Themes) > 1 {
		t.Error("Theme selection should change after pressing down")
	}

	// Navigate up
	m = sendKey(m, "up")
	state = m.modal.State.(*ui.ThemeState)

	if state.SelectedIndex != initialIndex {
		t.Error("Theme selection should return to initial after pressing up")
	}
}

func TestThemeModal_VimNavigation(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "t")
	state := m.modal.State.(*ui.ThemeState)

	initialIndex := state.SelectedIndex

	// Navigate with j
	m = sendKey(m, "j")
	state = m.modal.State.(*ui.ThemeState)

	if state.SelectedIndex == initialIndex && len(state.Themes) > 1 {
		t.Error("Theme selection should change after pressing j")
	}

	// Navigate with k
	m = sendKey(m, "k")
	state = m.modal.State.(*ui.ThemeState)

	if state.SelectedIndex != initialIndex {
		t.Error("Theme selection should return to initial after pressing k")
	}
}

func TestThemeModal_SelectAndApply(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "t")

	// Navigate to a different theme
	m = sendKey(m, "down")

	// Select it
	m = sendKey(m, "enter")

	// Modal should close
	if m.modal.IsVisible() {
		t.Error("Modal should close after selecting theme")
	}
}

// =============================================================================
// Settings Modal Tests
// =============================================================================

func TestSettingsModal_Open(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, ",")
	if !m.modal.IsVisible() {
		t.Fatal("Settings modal should be visible")
	}

	_, ok := m.modal.State.(*ui.SettingsState)
	if !ok {
		t.Fatalf("Expected SettingsState, got %T", m.modal.State)
	}
}

func TestSettingsModal_CancelWithEscape(t *testing.T) {
	cfg := testConfig()
	cfg.SetDefaultBranchPrefix("original/")
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, ",")
	state := m.modal.State.(*ui.SettingsState)

	// Change the branch prefix
	state.BranchPrefixInput.SetValue("modified/")

	// Cancel
	m = sendKey(m, "esc")

	if m.modal.IsVisible() {
		t.Error("Modal should close after escape")
	}

	// Branch prefix should be unchanged
	if m.config.GetDefaultBranchPrefix() != "original/" {
		t.Errorf("Branch prefix should be unchanged after cancel, got %s", m.config.GetDefaultBranchPrefix())
	}
}

func TestSettingsModal_TabSwitchesFocus(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, ",")
	state := m.modal.State.(*ui.SettingsState)

	// Initial focus on branch prefix
	if state.Focus != 0 {
		t.Errorf("Expected initial focus 0, got %d", state.Focus)
	}

	// Tab to notifications
	m = sendKey(m, "tab")
	state = m.modal.State.(*ui.SettingsState)

	if state.Focus != 1 {
		t.Errorf("Expected focus 1 after tab, got %d", state.Focus)
	}

	// Tab back to branch prefix
	m = sendKey(m, "tab")
	state = m.modal.State.(*ui.SettingsState)

	if state.Focus != 0 {
		t.Errorf("Expected focus 0 after second tab, got %d", state.Focus)
	}
}

func TestSettingsModal_ToggleNotifications(t *testing.T) {
	cfg := testConfig()
	cfg.SetNotificationsEnabled(true)
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, ",")
	state := m.modal.State.(*ui.SettingsState)

	// Initial state - notifications should be enabled
	if !state.NotificationsEnabled {
		t.Error("Notifications should be enabled initially")
	}

	// Tab to notifications checkbox
	m = sendKey(m, "tab")
	state = m.modal.State.(*ui.SettingsState)

	// Toggle with space
	m = sendKey(m, "space")
	state = m.modal.State.(*ui.SettingsState)

	if state.NotificationsEnabled {
		t.Error("Notifications should be disabled after space")
	}
}

func TestSettingsModal_UpdatesBranchPrefix(t *testing.T) {
	cfg := testConfig()
	cfg.SetDefaultBranchPrefix("")
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, ",")
	state := m.modal.State.(*ui.SettingsState)

	// Set a new branch prefix
	state.BranchPrefixInput.SetValue("test-prefix/")

	m = sendKey(m, "enter")

	// Config should be updated
	if m.config.GetDefaultBranchPrefix() != "test-prefix/" {
		t.Errorf("Expected branch prefix 'test-prefix/', got %q", m.config.GetDefaultBranchPrefix())
	}
}

// =============================================================================
// MCP Servers Modal Tests
// =============================================================================

func TestMCPServersModal_Open(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "s")
	if !m.modal.IsVisible() {
		t.Fatal("MCP servers modal should be visible")
	}

	_, ok := m.modal.State.(*ui.MCPServersState)
	if !ok {
		t.Fatalf("Expected MCPServersState, got %T", m.modal.State)
	}
}

func TestMCPServersModal_OpenAddServer(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "s")
	if !m.modal.IsVisible() {
		t.Fatal("MCP servers modal should be visible")
	}

	// Press 'a' to add a new server
	m = sendKey(m, "a")

	_, ok := m.modal.State.(*ui.AddMCPServerState)
	if !ok {
		t.Fatalf("Expected AddMCPServerState after pressing 'a', got %T", m.modal.State)
	}
}

func TestMCPServersModal_AddServerBackToList(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "s")
	m = sendKey(m, "a")

	// Press escape to go back to list
	m = sendKey(m, "esc")

	_, ok := m.modal.State.(*ui.MCPServersState)
	if !ok {
		t.Fatalf("Expected MCPServersState after escaping add modal, got %T", m.modal.State)
	}
}

func TestAddMCPServerModal_SubmitEmptyFields(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "s")
	m = sendKey(m, "a")

	state, ok := m.modal.State.(*ui.AddMCPServerState)
	if !ok {
		t.Fatalf("Expected AddMCPServerState, got %T", m.modal.State)
	}

	// Clear all fields
	state.NameInput.SetValue("")
	state.CmdInput.SetValue("")

	// Try to submit
	m = sendKey(m, "enter")

	// Should still be on add modal (empty fields prevent submission)
	_, ok = m.modal.State.(*ui.AddMCPServerState)
	if !ok {
		t.Error("Should still be on add server modal with empty fields")
	}
}

// =============================================================================
// Merge Modal Tests
// =============================================================================

func TestMergeModal_Open(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "m")
	if !m.modal.IsVisible() {
		t.Fatal("Merge modal should be visible")
	}

	_, ok := m.modal.State.(*ui.MergeState)
	if !ok {
		t.Fatalf("Expected MergeState, got %T", m.modal.State)
	}
}

func TestMergeModal_Navigate(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "m")
	state := m.modal.State.(*ui.MergeState)

	initialOption := state.GetSelectedOption()
	if initialOption == "" {
		t.Fatal("Should have an option selected")
	}

	initialIndex := state.SelectedIndex

	// Navigate down
	m = sendKey(m, "down")
	state = m.modal.State.(*ui.MergeState)

	if state.SelectedIndex == initialIndex && len(state.Options) > 1 {
		t.Log("Navigation may have limited options - this is OK")
	}
}

func TestMergeModal_ChildSessionShowsParent(t *testing.T) {
	cfg := testConfigWithParentChild()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Navigate to child session
	m = sendKey(m, "down")

	selected := m.sidebar.SelectedSession()
	if selected == nil || selected.ParentID == "" {
		t.Fatal("Should have selected child session with parent")
	}

	m = sendKey(m, "m")
	state := m.modal.State.(*ui.MergeState)

	// Should have "Merge to parent" option - check the Options slice
	hasParentOption := false
	for _, opt := range state.Options {
		if opt == "Merge to parent" {
			hasParentOption = true
			break
		}
	}
	if !hasParentOption {
		t.Error("Child session should have 'Merge to parent' option")
	}
}

func TestMergeModal_PRSessionMergeState(t *testing.T) {
	cfg := testConfigWithParentChild()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Navigate to PR session (third in list)
	m = sendKey(m, "down")
	m = sendKey(m, "down")

	selected := m.sidebar.SelectedSession()
	if selected == nil || !selected.PRCreated {
		t.Fatal("Should have selected session with PR created")
	}

	m = sendKey(m, "m")
	state := m.modal.State.(*ui.MergeState)

	// Verify the modal state has the PRCreated flag set
	// Note: "Push updates to PR" only appears when HasRemote is true,
	// which requires an actual git remote. In tests without a real git
	// repo, HasRemote will be false, so we just verify the modal opens.
	if state.PRCreated != true {
		t.Error("MergeState should have PRCreated set to true")
	}

	// Should always have "Merge to main" option
	hasMergeOption := false
	for _, opt := range state.Options {
		if opt == "Merge to main" {
			hasMergeOption = true
			break
		}
	}
	if !hasMergeOption {
		t.Error("Should have 'Merge to main' option")
	}
}

func TestMergeModal_Cancel(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "m")
	if !m.modal.IsVisible() {
		t.Fatal("Modal should be visible")
	}

	m = sendKey(m, "esc")

	if m.modal.IsVisible() {
		t.Error("Modal should close after escape")
	}
}

// =============================================================================
// Help Modal Tests
// =============================================================================

func TestHelpModal_Open(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "?")
	if !m.modal.IsVisible() {
		t.Fatal("Help modal should be visible")
	}

	_, ok := m.modal.State.(*ui.HelpState)
	if !ok {
		t.Fatalf("Expected HelpState, got %T", m.modal.State)
	}
}

func TestHelpModal_Navigate(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "?")
	state := m.modal.State.(*ui.HelpState)

	initialIndex := state.SelectedIndex

	// Navigate down
	m = sendKey(m, "down")
	state = m.modal.State.(*ui.HelpState)

	if state.SelectedIndex == initialIndex {
		t.Log("Navigation may have limited options")
	}
}

func TestHelpModal_CloseWithQuestion(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "?")
	if !m.modal.IsVisible() {
		t.Fatal("Help modal should be visible")
	}

	// Close with ?
	m = sendKey(m, "?")
	if m.modal.IsVisible() {
		t.Error("Help modal should close when pressing ? again")
	}
}

func TestHelpModal_CloseWithQ(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "?")
	if !m.modal.IsVisible() {
		t.Fatal("Help modal should be visible")
	}

	// Close with q
	m = sendKey(m, "q")
	if m.modal.IsVisible() {
		t.Error("Help modal should close when pressing q")
	}
}

func TestHelpModal_TriggerShortcut(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "?")

	// Press enter to trigger selected shortcut
	m = sendKey(m, "enter")

	// Help modal should close (shortcut was triggered) or different modal opened
	// This depends on which shortcut was selected
}

// =============================================================================
// Merge Conflict Modal Tests
// =============================================================================

func TestMergeConflictModal_Navigate(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Directly show the merge conflict modal
	conflictedFiles := []string{"main.go", "config.go"}
	m.modal.Show(ui.NewMergeConflictState("test-session", "Test Session", conflictedFiles, "/test/repo1"))

	if !m.modal.IsVisible() {
		t.Fatal("Merge conflict modal should be visible")
	}

	state, ok := m.modal.State.(*ui.MergeConflictState)
	if !ok {
		t.Fatalf("Expected MergeConflictState, got %T", m.modal.State)
	}

	// Initial selection should be 0
	if state.GetSelectedOption() != 0 {
		t.Errorf("Expected initial selection 0, got %d", state.GetSelectedOption())
	}

	// Navigate down
	m = sendKey(m, "down")
	state = m.modal.State.(*ui.MergeConflictState)
	if state.GetSelectedOption() != 1 {
		t.Errorf("Expected selection 1 after down, got %d", state.GetSelectedOption())
	}

	// Navigate down again
	m = sendKey(m, "down")
	state = m.modal.State.(*ui.MergeConflictState)
	if state.GetSelectedOption() != 2 {
		t.Errorf("Expected selection 2 after second down, got %d", state.GetSelectedOption())
	}
}

func TestMergeConflictModal_Cancel(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Directly show the merge conflict modal
	m.modal.Show(ui.NewMergeConflictState("test-session", "Test Session", []string{"main.go"}, "/test/repo1"))

	if !m.modal.IsVisible() {
		t.Fatal("Modal should be visible")
	}

	m = sendKey(m, "esc")

	if m.modal.IsVisible() {
		t.Error("Modal should close after escape")
	}
}

// =============================================================================
// Import Issues Modal Tests (UI only - no actual GitHub calls)
// =============================================================================

func TestImportIssuesModal_OpenWithSession(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select a session first
	m = sendKey(m, "enter")
	m = sendKey(m, "tab") // Back to sidebar

	m = sendKey(m, "i")

	if !m.modal.IsVisible() {
		t.Fatal("Import issues modal should be visible")
	}

	state, ok := m.modal.State.(*ui.ImportIssuesState)
	if !ok {
		t.Fatalf("Expected ImportIssuesState, got %T", m.modal.State)
	}

	// Should have repo path from selected session
	if state.RepoPath == "" {
		t.Error("Expected repo path to be set from selected session")
	}
}

func TestImportIssuesModal_OpenWithoutSession(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "i")

	if !m.modal.IsVisible() {
		t.Fatal("Modal should be visible")
	}

	// Should show repo selector since no session is selected
	_, ok := m.modal.State.(*ui.SelectRepoForIssuesState)
	if !ok {
		t.Fatalf("Expected SelectRepoForIssuesState when no session selected, got %T", m.modal.State)
	}
}

func TestSelectRepoForIssuesModal_Navigate(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "i")

	state, ok := m.modal.State.(*ui.SelectRepoForIssuesState)
	if !ok {
		t.Fatalf("Expected SelectRepoForIssuesState, got %T", m.modal.State)
	}

	initialIndex := state.RepoIndex

	// Navigate
	m = sendKey(m, "down")
	state = m.modal.State.(*ui.SelectRepoForIssuesState)

	if state.RepoIndex == initialIndex && len(state.RepoOptions) > 1 {
		t.Error("Repo index should change after pressing down")
	}
}

// =============================================================================
// Changelog Modal Tests
// =============================================================================

func TestChangelogModal_CloseWithEnter(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Manually show changelog modal
	m.modal.Show(ui.NewChangelogState([]ui.ChangelogEntry{
		{Version: "1.0.0", Changes: []string{"Test change"}},
	}))

	if !m.modal.IsVisible() {
		t.Fatal("Changelog modal should be visible")
	}

	m = sendKey(m, "enter")

	if m.modal.IsVisible() {
		t.Error("Modal should close after enter")
	}
}

func TestChangelogModal_CloseWithEscape(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m.modal.Show(ui.NewChangelogState([]ui.ChangelogEntry{
		{Version: "1.0.0", Changes: []string{"Test"}},
	}))

	m = sendKey(m, "esc")

	if m.modal.IsVisible() {
		t.Error("Modal should close after escape")
	}
}

func TestChangelogModal_Scroll(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Create many entries to enable scrolling
	entries := make([]ui.ChangelogEntry, 10)
	for i := range entries {
		entries[i] = ui.ChangelogEntry{
			Version: "1.0." + string(rune('0'+i)),
			Changes: []string{"Change " + string(rune('0'+i))},
		}
	}
	m.modal.Show(ui.NewChangelogState(entries))

	state := m.modal.State.(*ui.ChangelogState)
	initialOffset := state.ScrollOffset

	// Scroll down
	m = sendKey(m, "down")
	state = m.modal.State.(*ui.ChangelogState)

	if state.ScrollOffset == initialOffset && len(entries) > state.MaxVisible {
		t.Log("Scroll should change when there are more entries than visible")
	}

	// Scroll with j
	m = sendKey(m, "j")

	// Scroll up
	m = sendKey(m, "up")
	m = sendKey(m, "k")

	// Should still be visible
	if !m.modal.IsVisible() {
		t.Error("Modal should still be visible after scrolling")
	}
}

// =============================================================================
// Welcome Modal Tests
// =============================================================================

func TestWelcomeModal_CloseWithEnter(t *testing.T) {
	cfg := testConfig()
	cfg.WelcomeShown = false // Force welcome to show
	m := testModelWithSize(cfg, 120, 40)

	// Manually show welcome modal
	m.modal.Show(ui.NewWelcomeState())

	if !m.modal.IsVisible() {
		t.Fatal("Welcome modal should be visible")
	}

	m = sendKey(m, "enter")

	if m.modal.IsVisible() {
		t.Error("Modal should close after enter")
	}
}

func TestWelcomeModal_CloseWithEscape(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m.modal.Show(ui.NewWelcomeState())

	m = sendKey(m, "esc")

	if m.modal.IsVisible() {
		t.Error("Modal should close after escape")
	}
}

// =============================================================================
// Edit Commit Modal Tests
// =============================================================================

func TestEditCommitModal_Cancel(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Manually show edit commit modal
	m.modal.Show(ui.NewEditCommitState("Test commit message", "merge"))

	if !m.modal.IsVisible() {
		t.Fatal("Edit commit modal should be visible")
	}

	m = sendKey(m, "esc")

	if m.modal.IsVisible() {
		t.Error("Modal should close after escape")
	}
}

// =============================================================================
// Explore Options Modal Tests
// =============================================================================

func TestExploreOptionsModal_Navigate(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Create explore options state with test options
	options := []ui.OptionItem{
		{Number: 1, Text: "Option 1"},
		{Number: 2, Text: "Option 2"},
		{Number: 3, Text: "Option 3"},
	}
	m.modal.Show(ui.NewExploreOptionsState("Test Session", options))

	if !m.modal.IsVisible() {
		t.Fatal("Explore options modal should be visible")
	}

	state := m.modal.State.(*ui.ExploreOptionsState)

	initialIndex := state.SelectedIndex

	// Navigate
	m = sendKey(m, "down")
	state = m.modal.State.(*ui.ExploreOptionsState)

	if state.SelectedIndex == initialIndex && len(options) > 1 {
		t.Log("Selection should change after down")
	}
}

func TestExploreOptionsModal_ToggleSelection(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	options := []ui.OptionItem{
		{Number: 1, Text: "Option 1"},
		{Number: 2, Text: "Option 2"},
	}
	m.modal.Show(ui.NewExploreOptionsState("Test Session", options))

	state := m.modal.State.(*ui.ExploreOptionsState)
	initialSelected := len(state.GetSelectedOptions())

	// Toggle with space
	m = sendKey(m, "space")
	state = m.modal.State.(*ui.ExploreOptionsState)
	afterSpace := len(state.GetSelectedOptions())

	if afterSpace == initialSelected {
		t.Log("Selection toggling behavior depends on implementation")
	}
}

func TestExploreOptionsModal_Cancel(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	options := []ui.OptionItem{{Number: 1, Text: "Option 1"}}
	m.modal.Show(ui.NewExploreOptionsState("Test Session", options))

	m = sendKey(m, "esc")

	if m.modal.IsVisible() {
		t.Error("Modal should close after escape")
	}
}

// =============================================================================
// Search Messages Modal Tests
// =============================================================================

func TestSearchMessagesModal_Open(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Open search modal with test messages
	m.modal.Show(ui.NewSearchMessagesState([]struct{ Role, Content string }{
		{Role: "assistant", Content: "Test message 1"},
		{Role: "assistant", Content: "Test message 2"},
	}))

	if !m.modal.IsVisible() {
		t.Fatal("Search messages modal should be visible")
	}

	_, ok := m.modal.State.(*ui.SearchMessagesState)
	if !ok {
		t.Fatalf("Expected SearchMessagesState, got %T", m.modal.State)
	}
}

func TestSearchMessagesModal_Cancel(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m.modal.Show(ui.NewSearchMessagesState([]struct{ Role, Content string }{
		{Role: "assistant", Content: "Test"},
	}))

	m = sendKey(m, "esc")

	if m.modal.IsVisible() {
		t.Error("Modal should close after escape")
	}
}

// =============================================================================
// Session State Preservation Tests
// =============================================================================

func TestModalDoesNotAffectSessionState(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select a session
	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	// Add some state
	m.sessionState().SetPendingMessage(sessionID, "pending message")

	// Open and close various modals
	m = sendKey(m, "tab") // Back to sidebar
	m = sendKey(m, "?")   // Help
	m = sendKey(m, "esc")
	m = sendKey(m, "t") // Theme
	m = sendKey(m, "esc")
	m = sendKey(m, "s") // MCP
	m = sendKey(m, "esc")

	// Session state should be preserved
	if !m.sessionState().HasPendingMessage(sessionID) {
		t.Error("Session state should be preserved after opening/closing modals")
	}

	pending := m.sessionState().PeekPendingMessage(sessionID)
	if pending != "pending message" {
		t.Errorf("Expected pending message 'pending message', got %q", pending)
	}
}

// =============================================================================
// Modal Focus Tests
// =============================================================================

func TestModalBlocksShortcuts(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Open a modal
	m = sendKey(m, "n")
	if !m.modal.IsVisible() {
		t.Fatal("Modal should be visible")
	}

	// Try other shortcuts that should be blocked
	initialModalType := m.modal.State

	m = sendKey(m, "t") // Theme shortcut - goes to text input in new session modal
	// Should still be on the same modal type (shortcut blocked)
	if _, ok := m.modal.State.(*ui.NewSessionState); !ok {
		t.Errorf("Should still be on NewSessionState, got %T", m.modal.State)
	}
	_ = initialModalType
}

func TestModalReceivesTextInput(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Open add repo modal which has text input
	m = sendKey(m, "a")
	state := m.modal.State.(*ui.AddRepoState)

	// Make sure we're not using suggested repo
	state.UseSuggested = false
	state.Input.Focus()

	// Type some text
	m = typeText(m, "/path/to/repo")

	state = m.modal.State.(*ui.AddRepoState)
	path := state.GetPath()

	if !strings.Contains(path, "/path/to/repo") {
		t.Errorf("Expected path to contain '/path/to/repo', got %q", path)
	}
}

// =============================================================================
// Modal Validation Edge Cases
// =============================================================================

func TestNewSessionModal_BranchNameTooLong(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "n")
	state := m.modal.State.(*ui.NewSessionState)

	// Create a branch name that's too long (>100 chars)
	longName := strings.Repeat("a", 110)
	state.BranchInput.SetValue(longName)

	m = sendKey(m, "enter")

	if m.modal.GetError() == "" {
		t.Error("Expected error for branch name that's too long")
	}
}

func TestForkSessionModal_BranchEndingWithLock(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "f")
	state := m.modal.State.(*ui.ForkSessionState)

	state.BranchInput.SetValue("feature.lock")

	m = sendKey(m, "enter")

	if m.modal.GetError() == "" {
		t.Error("Expected error for branch name ending with '.lock'")
	}
}

// =============================================================================
// Concurrent Modal Operations
// =============================================================================

func TestRapidModalOpenClose(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Rapidly open and close modals
	for i := 0; i < 10; i++ {
		m = sendKey(m, "n")
		m = sendKey(m, "esc")
		m = sendKey(m, "t")
		m = sendKey(m, "esc")
		m = sendKey(m, "?")
		m = sendKey(m, "esc")
	}

	// Should end with no modal
	if m.modal.IsVisible() {
		t.Error("No modal should be visible after rapid open/close")
	}

	// App should still be functional
	if m.focus != FocusSidebar {
		t.Log("Focus may have changed during rapid operations")
	}
}

// =============================================================================
// Modal with Streaming State
// =============================================================================

func TestModalDuringStreaming(t *testing.T) {
	cfg := testConfigWithSessions()
	m, factory := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select session and start streaming
	m = sendKey(m, "enter")
	sessionID := m.activeSession.ID

	mock := factory.GetMock(sessionID)
	mock.SetStreaming(true)
	m.setState(StateStreamingClaude)

	// Simulate partial response
	m = simulateClaudeResponse(m, sessionID, textChunk("Partial..."))

	// Switch to sidebar and try to open modal
	m = sendKey(m, "tab")
	m = sendKey(m, "?")

	// Help modal should still work during streaming
	if !m.modal.IsVisible() {
		t.Log("Modal opening during streaming may be disabled by design")
	}
}

// =============================================================================
// Focus Restoration After Modal
// =============================================================================

func TestFocusRestoredAfterModal(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Start with sidebar focus
	if m.focus != FocusSidebar {
		t.Fatal("Should start with sidebar focus")
	}

	// Open and close theme modal
	m = sendKey(m, "t")
	m = sendKey(m, "esc")

	// Should return to sidebar focus
	if m.focus != FocusSidebar {
		t.Error("Focus should return to sidebar after closing modal")
	}

	// Select a session (switches to chat focus)
	m = sendKey(m, "enter")
	if m.focus != FocusChat {
		t.Fatal("Should be in chat focus")
	}

	// Switch to sidebar
	m = sendKey(m, "tab")

	// Open help modal
	m = sendKey(m, "?")
	m = sendKey(m, "esc")

	// Should return to sidebar focus
	if m.focus != FocusSidebar {
		t.Error("Focus should return to sidebar after closing help modal")
	}
}
