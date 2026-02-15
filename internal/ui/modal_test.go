package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestNewModal(t *testing.T) {
	modal := NewModal()

	if modal == nil {
		t.Fatal("NewModal() returned nil")
	}

	if modal.IsVisible() {
		t.Error("New modal should not be visible")
	}

	if modal.State != nil {
		t.Error("New modal should have nil state")
	}
}

func TestModal_ShowHide(t *testing.T) {
	modal := NewModal()

	// Create a simple state
	state := NewAddRepoState("")

	modal.Show(state)

	if !modal.IsVisible() {
		t.Error("Modal should be visible after Show")
	}

	if modal.State == nil {
		t.Error("Modal state should not be nil after Show")
	}

	modal.Hide()

	if modal.IsVisible() {
		t.Error("Modal should not be visible after Hide")
	}

	if modal.State != nil {
		t.Error("Modal state should be nil after Hide")
	}
}

func TestModal_Error(t *testing.T) {
	modal := NewModal()

	if modal.GetError() != "" {
		t.Error("New modal should have no error")
	}

	modal.SetError("Something went wrong")

	if modal.GetError() != "Something went wrong" {
		t.Errorf("Expected error message, got %q", modal.GetError())
	}

	// Show clears error
	modal.Show(NewAddRepoState(""))
	if modal.GetError() != "" {
		t.Error("Show should clear error")
	}

	modal.SetError("New error")

	// Hide clears error
	modal.Hide()
	if modal.GetError() != "" {
		t.Error("Hide should clear error")
	}
}

func TestModal_View(t *testing.T) {
	modal := NewModal()

	// No state - should return empty
	view := modal.View(80, 24)
	if view != "" {
		t.Error("View should return empty string when not visible")
	}

	// With state
	modal.Show(NewAddRepoState(""))
	view = modal.View(80, 24)
	if view == "" {
		t.Error("View should return non-empty string when visible")
	}

	// With error
	modal.SetError("Test error")
	view = modal.View(80, 24)
	if view == "" {
		t.Error("View should return non-empty string with error")
	}
}

func TestModal_View_WidthClamping(t *testing.T) {
	modal := NewModal()

	// Use ImportIssuesState which implements ModalWithPreferredWidth (120 chars)
	state := NewImportIssuesState("/repo/path", "test-repo")
	state.SetIssues([]IssueItem{
		{ID: "1", Title: "Test issue", Source: "github"},
	})
	modal.Show(state)

	// With a wide screen, modal should render fine
	view := modal.View(200, 40)
	if view == "" {
		t.Error("View should render with wide screen")
	}

	// With a narrow screen (e.g., 100 chars), the modal should still render
	// without exceeding the screen width. The modal has 6 chars horizontal
	// overhead (2 border + 4 padding), so at 100 chars screen the content
	// width should be clamped to 94.
	view = modal.View(100, 40)
	if view == "" {
		t.Error("View should render with narrow screen")
	}
	// Check that no line exceeds the screen width
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		// Use lipgloss.Width for visual width (handles ANSI codes)
		lineWidth := lipgloss.Width(line)
		if lineWidth > 100 {
			t.Errorf("line %d exceeds screen width: visual width %d > screen width 100", i, lineWidth)
		}
	}

	// With a very narrow screen, should still render (clamped to minimum)
	view = modal.View(50, 40)
	if view == "" {
		t.Error("View should render with very narrow screen")
	}
	lines = strings.Split(view, "\n")
	for i, line := range lines {
		lineWidth := lipgloss.Width(line)
		if lineWidth > 50 {
			t.Errorf("line %d exceeds screen width: visual width %d > screen width 50", i, lineWidth)
		}
	}
}

// AddRepoState tests

func TestNewAddRepoState(t *testing.T) {
	// Without suggestion
	state := NewAddRepoState("")

	if state.SuggestedRepo != "" {
		t.Error("SuggestedRepo should be empty")
	}

	if state.UseSuggested {
		t.Error("UseSuggested should be false when no suggestion")
	}

	if state.Title() != "Add Repository" {
		t.Errorf("Expected title 'Add Repository', got %q", state.Title())
	}

	// With suggestion
	state = NewAddRepoState("/path/to/repo")

	if state.SuggestedRepo != "/path/to/repo" {
		t.Errorf("Expected SuggestedRepo '/path/to/repo', got %q", state.SuggestedRepo)
	}

	if !state.UseSuggested {
		t.Error("UseSuggested should be true when suggestion provided")
	}
}

func TestAddRepoState_GetPath(t *testing.T) {
	// Using suggestion
	state := NewAddRepoState("/suggested/path")
	path := state.GetPath()
	if path != "/suggested/path" {
		t.Errorf("Expected suggested path, got %q", path)
	}

	// Switch to input
	state.UseSuggested = false
	state.Input.SetValue("/custom/path")
	path = state.GetPath()
	if path != "/custom/path" {
		t.Errorf("Expected custom path, got %q", path)
	}
}

func TestAddRepoState_Render(t *testing.T) {
	// Without suggestion
	state := NewAddRepoState("")
	render := state.Render()
	if render == "" {
		t.Error("Render should not be empty")
	}

	// With suggestion
	state = NewAddRepoState("/path/to/repo")
	render = state.Render()
	if render == "" {
		t.Error("Render with suggestion should not be empty")
	}
}

func TestAddRepoState_Help(t *testing.T) {
	// Without suggestion
	state := NewAddRepoState("")
	help := state.Help()
	if help == "" {
		t.Error("Help should not be empty")
	}

	// With suggestion (different help text)
	state = NewAddRepoState("/path/to/repo")
	helpWithSuggestion := state.Help()
	if helpWithSuggestion == "" {
		t.Error("Help with suggestion should not be empty")
	}
}

// NewSessionState tests

func TestNewNewSessionState(t *testing.T) {
	repos := []string{"/repo1", "/repo2"}
	state := NewNewSessionState(repos, false, false)

	if len(state.RepoOptions) != 2 {
		t.Errorf("Expected 2 repos, got %d", len(state.RepoOptions))
	}

	if state.RepoIndex != 0 {
		t.Errorf("Expected RepoIndex 0, got %d", state.RepoIndex)
	}

	if state.Focus != 0 {
		t.Errorf("Expected Focus 0, got %d", state.Focus)
	}

	if state.Title() != "New Session" {
		t.Errorf("Expected title 'New Session', got %q", state.Title())
	}
}

func TestNewSessionState_GetSelectedRepo(t *testing.T) {
	repos := []string{"/repo1", "/repo2", "/repo3"}
	state := NewNewSessionState(repos, false, false)

	// First repo selected
	if state.GetSelectedRepo() != "/repo1" {
		t.Errorf("Expected /repo1, got %q", state.GetSelectedRepo())
	}

	// Change selection
	state.RepoIndex = 2
	if state.GetSelectedRepo() != "/repo3" {
		t.Errorf("Expected /repo3, got %q", state.GetSelectedRepo())
	}

	// Empty repos
	state = NewNewSessionState([]string{}, false, false)
	if state.GetSelectedRepo() != "" {
		t.Errorf("Expected empty string for no repos, got %q", state.GetSelectedRepo())
	}

	// Out of bounds index
	state = NewNewSessionState(repos, false, false)
	state.RepoIndex = 10
	if state.GetSelectedRepo() != "" {
		t.Errorf("Expected empty string for out of bounds, got %q", state.GetSelectedRepo())
	}
}

func TestNewSessionState_GetBranchName(t *testing.T) {
	state := NewNewSessionState([]string{"/repo"}, false, false)

	// Initially empty
	if state.GetBranchName() != "" {
		t.Errorf("Expected empty branch name, got %q", state.GetBranchName())
	}

	// Set branch name
	state.BranchInput.SetValue("feature-branch")
	if state.GetBranchName() != "feature-branch" {
		t.Errorf("Expected 'feature-branch', got %q", state.GetBranchName())
	}
}

func TestNewSessionState_Render(t *testing.T) {
	// With repos
	state := NewNewSessionState([]string{"/repo1", "/repo2"}, false, false)
	render := state.Render()
	if render == "" {
		t.Error("Render should not be empty")
	}

	// Without repos
	state = NewNewSessionState([]string{}, false, false)
	render = state.Render()
	if render == "" {
		t.Error("Render without repos should not be empty")
	}
}

func TestNewSessionState_Help(t *testing.T) {
	// With repos, focused on repo list - should show add and delete hints
	state := NewNewSessionState([]string{"/repo1", "/repo2"}, false, false)
	state.Focus = 0
	help := state.Help()
	if help != "up/down: select  Tab: next field  a: add repo  d: delete repo  Enter: create" {
		t.Errorf("Expected help with add/delete hints when focused on repos, got %q", help)
	}

	// Without repos, focused on repo list - should show add hint
	state = NewNewSessionState([]string{}, false, false)
	state.Focus = 0
	help = state.Help()
	if help != "a: add repo  Esc: cancel" {
		t.Errorf("Expected help with add hint when no repos, got %q", help)
	}

	// With repos, focused on base selection - should not show delete hint
	state = NewNewSessionState([]string{"/repo1"}, false, false)
	state.Focus = 1
	help = state.Help()
	if help != "up/down: select  Tab: next field  Enter: create" {
		t.Errorf("Expected help without delete hint when focused on base, got %q", help)
	}

	// With repos, focused on branch input - should not show delete hint
	state = NewNewSessionState([]string{"/repo1"}, false, false)
	state.Focus = 2
	help = state.Help()
	if help != "up/down: select  Tab: next field  Enter: create" {
		t.Errorf("Expected help without delete hint when focused on input, got %q", help)
	}
}

// ConfirmDeleteState tests

func TestNewConfirmDeleteState(t *testing.T) {
	state := NewConfirmDeleteState("my-feature-branch")

	if state.SessionName != "my-feature-branch" {
		t.Errorf("Expected SessionName 'my-feature-branch', got %q", state.SessionName)
	}

	if len(state.Options) != 2 {
		t.Errorf("Expected 2 options, got %d", len(state.Options))
	}

	if state.SelectedIndex != 0 {
		t.Errorf("Expected SelectedIndex 0, got %d", state.SelectedIndex)
	}

	if state.Title() != "Delete Session?" {
		t.Errorf("Expected title 'Delete Session?', got %q", state.Title())
	}
}

func TestConfirmDeleteState_ShouldDeleteWorktree(t *testing.T) {
	state := NewConfirmDeleteState("test-session")

	// First option: Keep worktree
	if state.ShouldDeleteWorktree() {
		t.Error("Index 0 should not delete worktree")
	}

	// Second option: Delete worktree
	state.SelectedIndex = 1
	if !state.ShouldDeleteWorktree() {
		t.Error("Index 1 should delete worktree")
	}
}

func TestConfirmDeleteState_Render(t *testing.T) {
	state := NewConfirmDeleteState("test-session")
	render := state.Render()
	if render == "" {
		t.Error("Render should not be empty")
	}
}

// ConfirmDeleteRepoState tests

func TestNewConfirmDeleteRepoState(t *testing.T) {
	state := NewConfirmDeleteRepoState("/path/to/my-repo")

	if state.RepoPath != "/path/to/my-repo" {
		t.Errorf("Expected RepoPath '/path/to/my-repo', got %q", state.RepoPath)
	}

	if state.Title() != "Delete Repository?" {
		t.Errorf("Expected title 'Delete Repository?', got %q", state.Title())
	}

	if state.Help() != "Enter: confirm  Esc: cancel" {
		t.Errorf("Unexpected help text: %q", state.Help())
	}
}

func TestConfirmDeleteRepoState_GetRepoPath(t *testing.T) {
	state := NewConfirmDeleteRepoState("/some/repo/path")

	if state.GetRepoPath() != "/some/repo/path" {
		t.Errorf("Expected '/some/repo/path', got %q", state.GetRepoPath())
	}
}

func TestConfirmDeleteRepoState_Render(t *testing.T) {
	state := NewConfirmDeleteRepoState("/test/repo")
	render := state.Render()
	if render == "" {
		t.Error("Render should not be empty")
	}
}

// MergeState tests

func TestNewMergeState(t *testing.T) {
	// Without remote, without parent, no PR created
	state := NewMergeState("my-feature", false, "3 files changed", "", false)

	if state.SessionName != "my-feature" {
		t.Errorf("Expected SessionName 'my-feature', got %q", state.SessionName)
	}

	if len(state.Options) != 1 {
		t.Errorf("Expected 1 option without remote, got %d", len(state.Options))
	}

	if state.HasRemote {
		t.Error("HasRemote should be false")
	}

	if state.HasParent {
		t.Error("HasParent should be false")
	}

	if state.ChangesSummary != "3 files changed" {
		t.Errorf("Expected changes summary, got %q", state.ChangesSummary)
	}

	// With remote, without parent, no PR created
	state = NewMergeState("another-branch", true, "1 file changed", "", false)

	if len(state.Options) != 2 {
		t.Errorf("Expected 2 options with remote, got %d", len(state.Options))
	}

	if !state.HasRemote {
		t.Error("HasRemote should be true")
	}

	if state.Title() != "Merge/PR" {
		t.Errorf("Expected title 'Merge/PR', got %q", state.Title())
	}

	// With parent, with remote - should have 3 options
	state = NewMergeState("child-branch", true, "", "parent-branch", false)

	if len(state.Options) != 3 {
		t.Errorf("Expected 3 options with parent and remote, got %d", len(state.Options))
	}

	if !state.HasParent {
		t.Error("HasParent should be true")
	}

	if state.ParentName != "parent-branch" {
		t.Errorf("Expected ParentName 'parent-branch', got %q", state.ParentName)
	}

	// First option should be "Merge to parent" when parent exists
	if state.Options[0] != "Merge to parent" {
		t.Errorf("Expected first option 'Merge to parent', got %q", state.Options[0])
	}

	// With remote, PR already created - should show "Push updates to PR" instead of "Create PR"
	state = NewMergeState("pr-branch", true, "2 files changed", "", true)

	if len(state.Options) != 2 {
		t.Errorf("Expected 2 options with PR created, got %d", len(state.Options))
	}

	if state.Options[1] != "Push updates to PR" {
		t.Errorf("Expected 'Push updates to PR', got %q", state.Options[1])
	}

	if !state.PRCreated {
		t.Error("PRCreated should be true")
	}
}

func TestMergeState_GetSelectedOption(t *testing.T) {
	state := NewMergeState("test-session", true, "", "", false)

	// First option
	if state.GetSelectedOption() != "Merge to main" {
		t.Errorf("Expected 'Merge to main', got %q", state.GetSelectedOption())
	}

	// Second option
	state.SelectedIndex = 1
	if state.GetSelectedOption() != "Create PR" {
		t.Errorf("Expected 'Create PR', got %q", state.GetSelectedOption())
	}

	// Out of bounds
	state.SelectedIndex = 10
	if state.GetSelectedOption() != "" {
		t.Errorf("Expected empty for out of bounds, got %q", state.GetSelectedOption())
	}

	// Empty options
	state.Options = nil
	if state.GetSelectedOption() != "" {
		t.Errorf("Expected empty for nil options, got %q", state.GetSelectedOption())
	}
}

func TestMergeState_Render(t *testing.T) {
	// With changes summary
	state := NewMergeState("test-session", true, "5 files changed", "", false)
	render := state.Render()
	if render == "" {
		t.Error("Render should not be empty")
	}

	// Without changes summary
	state = NewMergeState("test-session", false, "", "", false)
	render = state.Render()
	if render == "" {
		t.Error("Render without changes should not be empty")
	}
}

// EditCommitState tests

func TestNewEditCommitState(t *testing.T) {
	state := NewEditCommitState("Initial commit message", "merge")

	if state.MergeType != "merge" {
		t.Errorf("Expected MergeType 'merge', got %q", state.MergeType)
	}

	if state.GetMessage() != "Initial commit message" {
		t.Errorf("Expected message, got %q", state.GetMessage())
	}

	if state.Title() != "Edit Commit Message" {
		t.Errorf("Expected title 'Edit Commit Message', got %q", state.Title())
	}

	// PR type
	state = NewEditCommitState("PR message", "pr")
	if state.MergeType != "pr" {
		t.Errorf("Expected MergeType 'pr', got %q", state.MergeType)
	}
}

func TestEditCommitState_Render(t *testing.T) {
	state := NewEditCommitState("Test message", "merge")
	render := state.Render()
	if render == "" {
		t.Error("Render should not be empty")
	}

	state = NewEditCommitState("Test message", "pr")
	render = state.Render()
	if render == "" {
		t.Error("Render for PR should not be empty")
	}
}

// MCPServersState tests

func TestNewMCPServersState(t *testing.T) {
	globalServers := []MCPServerDisplay{
		{Name: "github", Command: "npx", Args: "@mcp/github", IsGlobal: true},
	}
	perRepoServers := map[string][]MCPServerDisplay{
		"/repo1": {{Name: "postgres", Command: "npx", Args: "@mcp/postgres", IsGlobal: false, RepoPath: "/repo1"}},
	}
	repos := []string{"/repo1"}

	state := NewMCPServersState(globalServers, perRepoServers, repos)

	if len(state.Servers) != 2 {
		t.Errorf("Expected 2 servers total, got %d", len(state.Servers))
	}

	if state.SelectedIndex != 0 {
		t.Errorf("Expected SelectedIndex 0, got %d", state.SelectedIndex)
	}

	if state.Title() != "MCP Servers" {
		t.Errorf("Expected title 'MCP Servers', got %q", state.Title())
	}
}

func TestMCPServersState_GetSelectedServer(t *testing.T) {
	globalServers := []MCPServerDisplay{
		{Name: "github", Command: "npx", IsGlobal: true},
		{Name: "postgres", Command: "npx", IsGlobal: true},
	}

	state := NewMCPServersState(globalServers, nil, nil)

	// First server
	server := state.GetSelectedServer()
	if server == nil {
		t.Fatal("Expected server, got nil")
	}
	if server.Name != "github" {
		t.Errorf("Expected 'github', got %q", server.Name)
	}

	// Second server
	state.SelectedIndex = 1
	server = state.GetSelectedServer()
	if server.Name != "postgres" {
		t.Errorf("Expected 'postgres', got %q", server.Name)
	}

	// Empty state
	state = NewMCPServersState(nil, nil, nil)
	server = state.GetSelectedServer()
	if server != nil {
		t.Error("Expected nil for empty servers")
	}

	// Out of bounds
	state = NewMCPServersState(globalServers, nil, nil)
	state.SelectedIndex = 10
	server = state.GetSelectedServer()
	if server != nil {
		t.Error("Expected nil for out of bounds")
	}
}

func TestMCPServersState_Render(t *testing.T) {
	// With servers
	globalServers := []MCPServerDisplay{
		{Name: "github", Command: "npx", Args: "@mcp/github", IsGlobal: true},
	}
	state := NewMCPServersState(globalServers, nil, nil)
	render := state.Render()
	if render == "" {
		t.Error("Render should not be empty")
	}

	// Without servers
	state = NewMCPServersState(nil, nil, nil)
	render = state.Render()
	if render == "" {
		t.Error("Render without servers should not be empty")
	}
}

// AddMCPServerState tests

func TestNewAddMCPServerState(t *testing.T) {
	repos := []string{"/repo1", "/repo2"}
	state := NewAddMCPServerState(repos)

	if state.Title() != "Add MCP Server" {
		t.Errorf("Expected title 'Add MCP Server', got %q", state.Title())
	}

	// Default values should be empty strings and global scope
	name, command, args, repoPath, isGlobal := state.GetValues()
	if name != "" {
		t.Errorf("Expected empty name, got %q", name)
	}
	if command != "" {
		t.Errorf("Expected empty command, got %q", command)
	}
	if args != "" {
		t.Errorf("Expected empty args, got %q", args)
	}
	if !isGlobal {
		t.Error("Default should be global")
	}
	if repoPath != "" {
		t.Errorf("Expected empty repoPath for global, got %q", repoPath)
	}

	// Without repos — should still default to global with empty repoPath
	stateNoRepos := NewAddMCPServerState([]string{})
	_, _, _, repoPath, isGlobal = stateNoRepos.GetValues()
	if !isGlobal {
		t.Error("Expected isGlobal true with no repos")
	}
	if repoPath != "" {
		t.Errorf("Expected empty repoPath with no repos, got %q", repoPath)
	}
}

func TestAddMCPServerState_Render(t *testing.T) {
	repos := []string{"/repo1", "/repo2"}
	state := NewAddMCPServerState(repos)

	render := state.Render()
	if render == "" {
		t.Error("Render should not be empty")
	}
}

func TestAddMCPServerState_Help(t *testing.T) {
	state := NewAddMCPServerState([]string{"/repo1"})
	help := state.Help()
	if help == "" {
		t.Error("Help should not be empty")
	}
}

// Helper function tests

func TestTruncatePath(t *testing.T) {
	tests := []struct {
		path     string
		maxLen   int
		expected string
	}{
		{"/short", 20, "/short"},
		{"/very/long/path/to/somewhere", 15, "...to/somewhere"}, // ... + last 12 chars
		{"", 10, ""},
		{"/a/b/c/d/e/f/g", 10, "...d/e/f/g"}, // ... + last 7 chars
	}

	for _, tt := range tests {
		result := TruncatePath(tt.path, tt.maxLen)
		if result != tt.expected {
			t.Errorf("TruncatePath(%q, %d) = %q, want %q", tt.path, tt.maxLen, result, tt.expected)
		}
	}
}

func TestTruncateString_Modal(t *testing.T) {
	tests := []struct {
		s        string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "he..."},
		{"hello world", 8, "hello..."},
		{"", 10, ""},
		{"hi", 2, "hi"},
	}

	for _, tt := range tests {
		result := TruncateString(tt.s, tt.maxLen)
		if result != tt.expected {
			t.Errorf("TruncateString(%q, %d) = %q, want %q", tt.s, tt.maxLen, result, tt.expected)
		}
	}
}

func TestSessionDisplayName(t *testing.T) {
	tests := []struct {
		branch   string
		name     string
		expected string
	}{
		// Old-style names with slashes - extracts last part
		{"my-feature-branch", "repo/my-feature-branch", "my-feature-branch"},
		{"fix/bug-123", "repo/fix/bug-123", "bug-123"},
		{"plural-abc123", "repo/abc123", "abc123"},
		{"", "myrepo/short-id", "short-id"},
		{"", "repo/with/multiple/parts/final", "final"},

		// Simple names (new style after rename) - returns as-is
		{"my-feature-branch", "custom-name", "custom-name"},
		{"fix/bug-123", "my renamed session", "my renamed session"},
		{"", "simple-session", "simple-session"},

		// Edge case: empty name
		{"", "", ""},
		{"plural-", "", ""},
	}

	for _, tt := range tests {
		result := SessionDisplayName(tt.branch, tt.name)
		if result != tt.expected {
			t.Errorf("SessionDisplayName(%q, %q) = %q, want %q", tt.branch, tt.name, result, tt.expected)
		}
	}
}
