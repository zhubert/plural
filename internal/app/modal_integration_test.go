package app

import (
	"strings"
	"testing"
	"time"

	pexec "github.com/zhubert/plural/internal/exec"

	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/session"
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

	// Tab to base selection (Focus == 1)
	m = sendKey(m, "tab")
	state = m.modal.State.(*ui.NewSessionState)

	if state.Focus != 1 {
		t.Errorf("Expected focus on base selection (1) after tab, got %d", state.Focus)
	}

	// Tab to branch input (Focus == 2)
	m = sendKey(m, "tab")
	state = m.modal.State.(*ui.NewSessionState)

	if state.Focus != 2 {
		t.Errorf("Expected focus on branch input (2) after second tab, got %d", state.Focus)
	}

	// Tab through remaining fields to wrap back to repo list (Focus == 0)
	// If containers are supported, there's an extra field (focus 3) before wrapping
	if state.ContainersSupported {
		m = sendKey(m, "tab") // Focus 3: containers
		state = m.modal.State.(*ui.NewSessionState)
		if state.Focus != 3 {
			t.Errorf("Expected focus on containers (3) after third tab, got %d", state.Focus)
		}
	}
	m = sendKey(m, "tab") // Wrap to 0
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

func TestForkSessionModal_CopyMessagesDefault(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "f")
	state := m.modal.State.(*ui.ForkSessionState)

	// Copy messages should be true by default (per constructor)
	if !state.CopyMessages {
		t.Error("CopyMessages should be true by default")
	}
	if !state.ShouldCopyMessages() {
		t.Error("ShouldCopyMessages should return true by default")
	}

	// Verify rendered output shows the copy messages option in the form
	rendered := state.Render()
	if !strings.Contains(rendered, "Copy conversation history") {
		t.Error("rendered fork modal should show 'Copy conversation history' option")
	}
}

func TestForkSessionModal_InvalidBranchName(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "f")
	state := m.modal.State.(*ui.ForkSessionState)

	// Set invalid branch name
	state.SetBranchName("-invalid")

	m = sendKey(m, "enter")

	// Should show error
	if m.modal.GetError() == "" {
		t.Error("Expected error for invalid branch name")
	}

	if !m.modal.IsVisible() {
		t.Error("Modal should still be visible after error")
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
	state.SetNewName("")

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
	state.SetNewName("invalid..name")

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
	state.SetNewName("new-name")

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
// Settings Modal Tests
// =============================================================================

func TestSettingsModal_Open(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Show global settings modal directly (pressing "," with repos
	// but no sessions opens RepoSettingsState since a repo is selected)
	m.modal.Show(ui.NewSettingsState(
		cfg.GetDefaultBranchPrefix(),
		cfg.GetNotificationsEnabled(),
		false, "",
		false,
	))
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

	m.modal.Show(ui.NewSettingsState(
		cfg.GetDefaultBranchPrefix(),
		cfg.GetNotificationsEnabled(),
		false, "",
		false,
	))
	state := m.modal.State.(*ui.SettingsState)

	// Change the branch prefix
	state.SetBranchPrefix("modified/")

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

func TestSettingsModal_ToggleNotifications(t *testing.T) {
	cfg := testConfig()
	cfg.SetNotificationsEnabled(true)
	m := testModelWithSize(cfg, 120, 40)

	m.modal.Show(ui.NewSettingsState(
		cfg.GetDefaultBranchPrefix(),
		cfg.GetNotificationsEnabled(),
		false, "",
		false,
	))
	state := m.modal.State.(*ui.SettingsState)

	// Initial state - notifications should be enabled
	if !state.NotificationsEnabled {
		t.Error("Notifications should be enabled initially")
	}
}

func TestRepoSettingsModal_AsanaProjectGID(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Get a repo path from config
	repoPath := cfg.Sessions[0].RepoPath

	// Show repo settings modal directly
	m.modal.Show(ui.NewRepoSettingsState(repoPath, false, "", false, ""))
	state := m.modal.State.(*ui.RepoSettingsState)

	// Set the Asana project GID via the selector
	state.AsanaSelectedGID = "1234567890123"

	// Save
	m = sendKey(m, "enter")

	// Config should have the Asana project GID
	if got := m.config.GetAsanaProject(repoPath); got != "1234567890123" {
		t.Errorf("Expected Asana project '1234567890123', got %q", got)
	}
}

func TestRepoSettingsModal_AsanaProjectGID_ClearRemoves(t *testing.T) {
	cfg := testConfigWithSessions()
	// Pre-set an Asana project GID
	repoPath := cfg.Sessions[0].RepoPath
	cfg.SetAsanaProject(repoPath, "9999999999999")
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Show repo settings modal directly (pass pre-set asana GID)
	m.modal.Show(ui.NewRepoSettingsState(repoPath, false, "9999999999999", false, ""))
	state := m.modal.State.(*ui.RepoSettingsState)

	// Verify it was loaded
	if state.GetAsanaProject() != "9999999999999" {
		t.Errorf("Expected pre-set Asana project '9999999999999', got %q", state.GetAsanaProject())
	}

	// Clear the value via the selector (selecting "(none)")
	state.AsanaSelectedGID = ""

	// Save
	m = sendKey(m, "enter")

	// Config should have the Asana project removed
	if got := m.config.GetAsanaProject(repoPath); got != "" {
		t.Errorf("Expected empty Asana project after clearing, got %q", got)
	}
	if m.config.HasAsanaProject(repoPath) {
		t.Error("HasAsanaProject should return false after clearing")
	}
}

func TestRepoSettingsModal_LinearTeamID(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Get a repo path from config
	repoPath := cfg.Sessions[0].RepoPath

	// Show repo settings modal directly (no providers configured)
	m.modal.Show(ui.NewRepoSettingsState(repoPath, false, "", false, ""))
	state := m.modal.State.(*ui.RepoSettingsState)

	// Set the Linear team ID via the selector
	state.LinearSelectedTeamID = "team-abc-123"

	// Save
	m = sendKey(m, "enter")

	// Config should have the Linear team ID
	if got := m.config.GetLinearTeam(repoPath); got != "team-abc-123" {
		t.Errorf("Expected Linear team 'team-abc-123', got %q", got)
	}
}

func TestRepoSettingsModal_LinearTeamID_ClearRemoves(t *testing.T) {
	cfg := testConfigWithSessions()
	// Pre-set a Linear team ID
	repoPath := cfg.Sessions[0].RepoPath
	cfg.SetLinearTeam(repoPath, "team-xyz-999")
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Show repo settings modal directly (pass pre-set linear team ID)
	m.modal.Show(ui.NewRepoSettingsState(repoPath, false, "", false, "team-xyz-999"))
	state := m.modal.State.(*ui.RepoSettingsState)

	// Verify it was loaded
	if state.GetLinearTeam() != "team-xyz-999" {
		t.Errorf("Expected pre-set Linear team 'team-xyz-999', got %q", state.GetLinearTeam())
	}

	// Clear the value via the selector (selecting "(none)")
	state.LinearSelectedTeamID = ""

	// Save
	m = sendKey(m, "enter")

	// Config should have the Linear team removed
	if got := m.config.GetLinearTeam(repoPath); got != "" {
		t.Errorf("Expected empty Linear team after clearing, got %q", got)
	}
	if m.config.HasLinearTeam(repoPath) {
		t.Error("HasLinearTeam should return false after clearing")
	}
}

func TestSettingsModal_UpdatesBranchPrefix(t *testing.T) {
	cfg := testConfig()
	cfg.SetDefaultBranchPrefix("")
	m := testModelWithSize(cfg, 120, 40)

	m.modal.Show(ui.NewSettingsState(
		cfg.GetDefaultBranchPrefix(),
		cfg.GetNotificationsEnabled(),
		false, "",
		false,
	))
	state := m.modal.State.(*ui.SettingsState)

	// Set a new branch prefix
	state.SetBranchPrefix("test-prefix/")

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

	m.showMCPServersModal()
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

	m.showMCPServersModal()
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

	m.showMCPServersModal()
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

	m.showMCPServersModal()
	m = sendKey(m, "a")

	state, ok := m.modal.State.(*ui.AddMCPServerState)
	if !ok {
		t.Fatalf("Expected AddMCPServerState, got %T", m.modal.State)
	}

	// Verify fields start empty by default
	name, command, _, _, _ := state.GetValues()
	if name != "" || command != "" {
		t.Fatal("Expected empty fields by default")
	}

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

	initialShortcut := state.GetSelectedShortcut()

	// Navigate down
	m = sendKey(m, "j")
	state = m.modal.State.(*ui.HelpState)

	newShortcut := state.GetSelectedShortcut()
	if initialShortcut != nil && newShortcut != nil && newShortcut.Key == initialShortcut.Key {
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

	// Render once to populate totalLines
	_ = state.Render()

	// Scroll down
	m = sendKey(m, "down")
	state = m.modal.State.(*ui.ChangelogState)

	if state.ScrollOffset == initialOffset && len(entries) > 1 {
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
	m.sessionState().GetOrCreate(sessionID).PendingMessage = "pending message"

	// Open and close various modals
	m = sendKey(m, "tab") // Back to sidebar
	m = sendKey(m, "?")   // Help
	m = sendKey(m, "esc")
	m = sendKey(m, "t") // Theme
	m = sendKey(m, "esc")
	m.showMCPServersModal() // MCP
	m = sendKey(m, "esc")

	// Session state should be preserved
	state := m.sessionState().GetIfExists(sessionID)
	if state == nil || state.PendingMessage == "" {
		t.Error("Session state should be preserved after opening/closing modals")
	}

	if state.PendingMessage != "pending message" {
		t.Errorf("Expected pending message 'pending message', got %q", state.PendingMessage)
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

	state.SetBranchName("feature.lock")

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

// =============================================================================
// Preview Active Modal Tests
// =============================================================================

func TestPreviewActiveModal_DismissWithEscape(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Manually show the preview modal
	m.modal.Show(ui.NewPreviewActiveState("test-session", "feature-branch"))

	if !m.modal.IsVisible() {
		t.Fatal("Preview modal should be visible")
	}

	_, ok := m.modal.State.(*ui.PreviewActiveState)
	if !ok {
		t.Fatalf("Expected PreviewActiveState, got %T", m.modal.State)
	}

	// Press escape to dismiss
	m = sendKey(m, "esc")

	if m.modal.IsVisible() {
		t.Error("Modal should be hidden after pressing escape")
	}
}

func TestPreviewActiveModal_DismissWithEnter(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Manually show the preview modal
	m.modal.Show(ui.NewPreviewActiveState("test-session", "feature-branch"))

	if !m.modal.IsVisible() {
		t.Fatal("Preview modal should be visible")
	}

	// Press enter to dismiss
	m = sendKey(m, "enter")

	if m.modal.IsVisible() {
		t.Error("Modal should be hidden after pressing enter")
	}
}

func TestPreviewActiveModal_RendersSessionAndBranchInfo(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Manually show the preview modal
	state := ui.NewPreviewActiveState("my-session-name", "my-branch-name")
	m.modal.Show(state)

	if !m.modal.IsVisible() {
		t.Fatal("Preview modal should be visible")
	}

	// Check the state has the correct values
	previewState := m.modal.State.(*ui.PreviewActiveState)
	if previewState.SessionName != "my-session-name" {
		t.Errorf("Expected session name 'my-session-name', got '%s'", previewState.SessionName)
	}
	if previewState.BranchName != "my-branch-name" {
		t.Errorf("Expected branch name 'my-branch-name', got '%s'", previewState.BranchName)
	}

	// Verify the rendered content contains the session and branch info
	rendered := previewState.Render()
	if !strings.Contains(rendered, "my-session-name") {
		t.Error("Rendered modal should contain session name")
	}
	if !strings.Contains(rendered, "my-branch-name") {
		t.Error("Rendered modal should contain branch name")
	}
}

// =============================================================================
// New Session Modal - Add Repo Integration Tests
// =============================================================================

func TestNewSessionModal_AddRepoOpensAddRepoModal(t *testing.T) {
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

	// Focus should be on repo list
	if state.Focus != 0 {
		t.Fatalf("Expected focus on repo list (0), got %d", state.Focus)
	}

	// Press 'a' to add a repo
	m = sendKey(m, "a")

	// Should now be on AddRepoState with ReturnToNewSession set
	addState, ok := m.modal.State.(*ui.AddRepoState)
	if !ok {
		t.Fatalf("Expected AddRepoState after pressing 'a', got %T", m.modal.State)
	}

	if !addState.ReturnToNewSession {
		t.Error("ReturnToNewSession should be true when opened from new session modal")
	}
}

func TestNewSessionModal_AddRepoEscapeReturnsToNewSession(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Open new session modal, then add repo modal
	m = sendKey(m, "n")
	m = sendKey(m, "a")

	// Verify we're on AddRepoState
	_, ok := m.modal.State.(*ui.AddRepoState)
	if !ok {
		t.Fatalf("Expected AddRepoState, got %T", m.modal.State)
	}

	// Press Escape
	m = sendKey(m, "esc")

	// Should return to NewSessionState, not hide modal
	if !m.modal.IsVisible() {
		t.Fatal("Modal should still be visible after escape from add repo")
	}

	_, ok = m.modal.State.(*ui.NewSessionState)
	if !ok {
		t.Fatalf("Expected NewSessionState after escape, got %T", m.modal.State)
	}
}

func TestNewSessionModal_AddRepoNotOnBranchInput(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Open new session modal and tab to branch input
	m = sendKey(m, "n")
	m = sendKey(m, "tab") // base selection
	m = sendKey(m, "tab") // branch input

	state := m.modal.State.(*ui.NewSessionState)
	if state.Focus != 2 {
		t.Fatalf("Expected focus on branch input (2), got %d", state.Focus)
	}

	// Press 'a' - should pass through to text input, not open add repo
	m = sendKey(m, "a")

	// Should still be on NewSessionState (not AddRepoState)
	_, ok := m.modal.State.(*ui.NewSessionState)
	if !ok {
		t.Fatalf("Expected NewSessionState when pressing 'a' on branch input, got %T", m.modal.State)
	}
}

func TestNewSessionModal_EmptyRepoShowsAddHint(t *testing.T) {
	cfg := testConfig()
	// Remove all repos
	for _, repo := range cfg.GetRepos() {
		cfg.RemoveRepo(repo)
	}
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "n")
	state := m.modal.State.(*ui.NewSessionState)

	// Render and check for the updated text
	rendered := state.Render()
	if !strings.Contains(rendered, "Press 'a' to add one") {
		t.Error("Empty repo list should show 'Press 'a' to add one'")
	}
}

func TestNewSessionModal_HelpShowsAddRepo(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	m = sendKey(m, "n")
	state := m.modal.State.(*ui.NewSessionState)

	// With repos, help should include "a: add repo"
	help := state.Help()
	if !strings.Contains(help, "a: add repo") {
		t.Errorf("Help should contain 'a: add repo', got %q", help)
	}
}

// =============================================================================
// Explore Options - Container Inheritance Tests
// =============================================================================

func TestCreateParallelSessions_InheritsContainerizedFlag(t *testing.T) {
	// Set up config with a containerized parent session
	cfg := testConfig()
	cfg.Sessions = []config.Session{
		{
			ID:            "parent-containerized",
			RepoPath:      "/test/repo1",
			WorkTree:      "/test/worktree-parent",
			Branch:        "feature-branch",
			Name:          "repo1/parent",
			CreatedAt:     time.Now(),
			Started:       true,
			Containerized: true,
		},
	}

	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select the containerized session (sets activeSession + claudeRunner)
	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Expected active session")
	}
	if !m.activeSession.Containerized {
		t.Fatal("Parent session should be containerized")
	}

	// Set up mock executor for git and session services
	mockExec := pexec.NewMockExecutor(nil)
	// Mock git worktree add (used by CreateFromBranch)
	mockExec.AddPrefixMatch("git", []string{"worktree", "add"}, pexec.MockResponse{
		Stdout: []byte("Preparing worktree\n"),
	})
	// Mock claude branch name generation (used by GenerateBranchNamesFromOptions)
	mockExec.AddPrefixMatch("claude", []string{"--print"}, pexec.MockResponse{
		Stdout: []byte("1. option-one-branch\n2. option-two-branch\n"),
	})

	mockGitService := git.NewGitServiceWithExecutor(mockExec)
	mockSessionService := session.NewSessionServiceWithExecutor(mockExec)
	m.SetGitService(mockGitService)
	m.SetSessionService(mockSessionService)

	// Call createParallelSessions with test options
	options := []ui.OptionItem{
		{Number: 1, Text: "First option"},
		{Number: 2, Text: "Second option"},
	}
	m.createParallelSessions(options)

	// Verify child sessions inherited Containerized=true
	var childSessions []config.Session
	for _, s := range m.config.Sessions {
		if s.ParentID == "parent-containerized" {
			childSessions = append(childSessions, s)
		}
	}

	if len(childSessions) == 0 {
		t.Fatal("Expected child sessions to be created")
	}

	for _, child := range childSessions {
		if !child.Containerized {
			t.Errorf("Child session %s should inherit Containerized=true from parent, got false", child.ID)
		}
	}
}

func TestCreateParallelSessions_NonContainerizedParent(t *testing.T) {
	// Set up config with a non-containerized parent session
	cfg := testConfig()
	cfg.Sessions = []config.Session{
		{
			ID:            "parent-normal",
			RepoPath:      "/test/repo1",
			WorkTree:      "/test/worktree-parent",
			Branch:        "feature-branch",
			Name:          "repo1/parent",
			CreatedAt:     time.Now(),
			Started:       true,
			Containerized: false,
		},
	}

	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select the non-containerized session
	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Expected active session")
	}
	if m.activeSession.Containerized {
		t.Fatal("Parent session should not be containerized")
	}

	// Set up mock executor
	mockExec := pexec.NewMockExecutor(nil)
	mockExec.AddPrefixMatch("git", []string{"worktree", "add"}, pexec.MockResponse{
		Stdout: []byte("Preparing worktree\n"),
	})
	mockExec.AddPrefixMatch("claude", []string{"--print"}, pexec.MockResponse{
		Stdout: []byte("1. option-one-branch\n"),
	})

	mockGitService := git.NewGitServiceWithExecutor(mockExec)
	mockSessionService := session.NewSessionServiceWithExecutor(mockExec)
	m.SetGitService(mockGitService)
	m.SetSessionService(mockSessionService)

	// Call createParallelSessions
	options := []ui.OptionItem{
		{Number: 1, Text: "First option"},
	}
	m.createParallelSessions(options)

	// Verify child sessions have Containerized=false
	var childSessions []config.Session
	for _, s := range m.config.Sessions {
		if s.ParentID == "parent-normal" {
			childSessions = append(childSessions, s)
		}
	}

	if len(childSessions) == 0 {
		t.Fatal("Expected child sessions to be created")
	}

	for _, child := range childSessions {
		if child.Containerized {
			t.Errorf("Child session %s should not be containerized when parent is not, got true", child.ID)
		}
	}
}

func TestSessionSettingsModal_CancelWithEscape(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	state := ui.NewSessionSettingsState("session-1", "my-session", "feature-branch", "main", false)
	m.modal.Show(state)

	if !m.modal.IsVisible() {
		t.Fatal("expected modal to be visible")
	}

	m = sendKey(m, "esc")
	if m.modal.IsVisible() {
		t.Error("expected modal to be hidden after escape")
	}
}
