package modals

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/zhubert/plural/internal/keys"
)

// =============================================================================
// ImportIssuesState Tests
// =============================================================================

func TestNewImportIssuesState(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)

	if state.RepoPath != "/repo/path" {
		t.Errorf("expected repo path '/repo/path', got '%s'", state.RepoPath)
	}
	if state.RepoName != "test-repo" {
		t.Errorf("expected repo name 'test-repo', got '%s'", state.RepoName)
	}
	if !state.Loading {
		t.Error("expected Loading to be true initially")
	}
	if state.Source != "github" {
		t.Errorf("expected source 'github', got '%s'", state.Source)
	}
	if state.SelectedIndex != 0 {
		t.Errorf("expected initial selected index 0, got %d", state.SelectedIndex)
	}
}

func TestNewImportIssuesStateWithSource(t *testing.T) {
	state := NewImportIssuesStateWithSource("/repo/path", "test-repo", "asana", "project123", true, true)

	if state.Source != "asana" {
		t.Errorf("expected source 'asana', got '%s'", state.Source)
	}
	if state.ProjectID != "project123" {
		t.Errorf("expected project ID 'project123', got '%s'", state.ProjectID)
	}
}

func TestImportIssuesState_Title_GitHub(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	if state.Title() != "Import GitHub Issues" {
		t.Errorf("expected title 'Import GitHub Issues', got '%s'", state.Title())
	}
}

func TestImportIssuesState_Title_Asana(t *testing.T) {
	state := NewImportIssuesStateWithSource("/repo/path", "test-repo", "asana", "", true, true)
	if state.Title() != "Import Asana Tasks" {
		t.Errorf("expected title 'Import Asana Tasks', got '%s'", state.Title())
	}
}

func TestImportIssuesState_PreferredWidth(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	width := state.PreferredWidth()

	if width != ModalWidthWide {
		t.Errorf("expected preferred width %d, got %d", ModalWidthWide, width)
	}

	// Verify it's wider than the default modal width
	if width <= ModalWidth {
		t.Errorf("expected wide modal width (%d) to be greater than default modal width (%d)", width, ModalWidth)
	}
}

func TestImportIssuesState_ImplementsModalWithPreferredWidth(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)

	// Verify it implements the interface
	_, ok := interface{}(state).(ModalWithPreferredWidth)
	if !ok {
		t.Error("ImportIssuesState should implement ModalWithPreferredWidth interface")
	}
}

func TestImportIssuesState_Help_Loading(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.Loading = true

	help := state.Help()
	if help != "Loading issues..." {
		t.Errorf("expected loading help text, got '%s'", help)
	}
}

func TestImportIssuesState_Help_Error(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.SetError("Test error")

	help := state.Help()
	if help != "Esc: close" {
		t.Errorf("expected error help text, got '%s'", help)
	}
}

func TestImportIssuesState_Help_Normal(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.Loading = false

	help := state.Help()
	if !strings.Contains(help, "up/down navigate") {
		t.Errorf("expected normal help text to contain navigation instructions, got '%s'", help)
	}
}

func TestImportIssuesState_SetIssues(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.Loading = true

	issues := []IssueItem{
		{ID: "1", Title: "Issue 1", Source: "github"},
		{ID: "2", Title: "Issue 2", Source: "github"},
	}

	state.SetIssues(issues)

	if state.Loading {
		t.Error("expected Loading to be false after SetIssues")
	}
	if len(state.Issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(state.Issues))
	}
	if state.LoadError != "" {
		t.Errorf("expected empty error, got '%s'", state.LoadError)
	}
}

func TestImportIssuesState_SetError(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.Loading = true

	state.SetError("Failed to load")

	if state.Loading {
		t.Error("expected Loading to be false after SetError")
	}
	if state.LoadError != "Failed to load" {
		t.Errorf("expected error 'Failed to load', got '%s'", state.LoadError)
	}
}

func TestImportIssuesState_Update_Navigation(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.SetIssues([]IssueItem{
		{ID: "1", Title: "Issue 1", Source: "github"},
		{ID: "2", Title: "Issue 2", Source: "github"},
		{ID: "3", Title: "Issue 3", Source: "github"},
	})

	// Navigate down
	keyDownMsg := tea.KeyPressMsg{Code: 0, Text: keys.Down}
	state.Update(keyDownMsg)
	if state.SelectedIndex != 1 {
		t.Errorf("expected selected index 1 after down, got %d", state.SelectedIndex)
	}

	// Navigate down again
	state.Update(keyDownMsg)
	if state.SelectedIndex != 2 {
		t.Errorf("expected selected index 2 after second down, got %d", state.SelectedIndex)
	}

	// Navigate up
	keyUpMsg := tea.KeyPressMsg{Code: 0, Text: keys.Up}
	state.Update(keyUpMsg)
	if state.SelectedIndex != 1 {
		t.Errorf("expected selected index 1 after up, got %d", state.SelectedIndex)
	}

	// Navigate to start
	state.Update(keyUpMsg)
	if state.SelectedIndex != 0 {
		t.Errorf("expected selected index 0 after up, got %d", state.SelectedIndex)
	}

	// Up at start should stay at 0
	state.Update(keyUpMsg)
	if state.SelectedIndex != 0 {
		t.Errorf("expected selected index to stay 0 at start, got %d", state.SelectedIndex)
	}
}

func TestImportIssuesState_Update_NavigationWithVimKeys(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.SetIssues([]IssueItem{
		{ID: "1", Title: "Issue 1", Source: "github"},
		{ID: "2", Title: "Issue 2", Source: "github"},
	})

	// Navigate down with 'j'
	keyJMsg := tea.KeyPressMsg{Code: 0, Text: "j"}
	state.Update(keyJMsg)
	if state.SelectedIndex != 1 {
		t.Errorf("expected selected index 1 after 'j', got %d", state.SelectedIndex)
	}

	// Navigate up with 'k'
	keyKMsg := tea.KeyPressMsg{Code: 0, Text: "k"}
	state.Update(keyKMsg)
	if state.SelectedIndex != 0 {
		t.Errorf("expected selected index 0 after 'k', got %d", state.SelectedIndex)
	}
}

func TestImportIssuesState_Update_ToggleSelection(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.SetIssues([]IssueItem{
		{ID: "1", Title: "Issue 1", Source: "github"},
		{ID: "2", Title: "Issue 2", Source: "github"},
	})

	// Initially no issues are selected
	if state.Issues[0].Selected {
		t.Error("expected first issue to be unselected initially")
	}

	// Toggle selection with Space
	keySpaceMsg := tea.KeyPressMsg{Code: 0, Text: keys.Space}
	state.Update(keySpaceMsg)
	if !state.Issues[0].Selected {
		t.Error("expected first issue to be selected after space")
	}

	// Toggle again to deselect
	state.Update(keySpaceMsg)
	if state.Issues[0].Selected {
		t.Error("expected first issue to be deselected after second space")
	}
}

func TestImportIssuesState_Update_Scrolling(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)

	// Create more issues than maxVisible
	issues := make([]IssueItem, 15)
	for i := 0; i < 15; i++ {
		issues[i] = IssueItem{ID: formatInt(i + 1), Title: "Issue " + formatInt(i+1), Source: "github"}
	}
	state.SetIssues(issues)

	// Navigate down to the bottom
	keyDownMsg := tea.KeyPressMsg{Code: 0, Text: keys.Down}
	for i := 0; i < 14; i++ {
		state.Update(keyDownMsg)
	}

	// Check that we're at the last issue
	if state.SelectedIndex != 14 {
		t.Errorf("expected selected index 14, got %d", state.SelectedIndex)
	}

	// Check that scroll offset has been adjusted
	if state.ScrollOffset == 0 {
		t.Error("expected scroll offset to be adjusted for bottom items")
	}

	// Navigate back up
	keyUpMsg := tea.KeyPressMsg{Code: 0, Text: keys.Up}
	for i := 0; i < 14; i++ {
		state.Update(keyUpMsg)
	}

	// Should be back at top
	if state.SelectedIndex != 0 {
		t.Errorf("expected selected index 0 after navigating back, got %d", state.SelectedIndex)
	}
	if state.ScrollOffset != 0 {
		t.Errorf("expected scroll offset 0 at top, got %d", state.ScrollOffset)
	}
}

func TestImportIssuesState_GetSelectedIssues(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.SetIssues([]IssueItem{
		{ID: "1", Title: "Issue 1", Source: "github", Selected: false},
		{ID: "2", Title: "Issue 2", Source: "github", Selected: true},
		{ID: "3", Title: "Issue 3", Source: "github", Selected: false},
		{ID: "4", Title: "Issue 4", Source: "github", Selected: true},
	})

	selected := state.GetSelectedIssues()
	if len(selected) != 2 {
		t.Errorf("expected 2 selected issues, got %d", len(selected))
	}
	if selected[0].ID != "2" {
		t.Errorf("expected first selected issue ID '2', got '%s'", selected[0].ID)
	}
	if selected[1].ID != "4" {
		t.Errorf("expected second selected issue ID '4', got '%s'", selected[1].ID)
	}
}

func TestImportIssuesState_GetSelectedIssues_None(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.SetIssues([]IssueItem{
		{ID: "1", Title: "Issue 1", Source: "github", Selected: false},
		{ID: "2", Title: "Issue 2", Source: "github", Selected: false},
	})

	selected := state.GetSelectedIssues()
	if len(selected) != 0 {
		t.Errorf("expected 0 selected issues, got %d", len(selected))
	}
}

func TestImportIssuesState_Render_Loading(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.Loading = true

	rendered := state.Render()
	if !strings.Contains(rendered, "Fetching issues from GitHub") {
		t.Error("expected rendered output to contain loading message")
	}
}

func TestImportIssuesState_Render_Error(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.SetError("Network error")

	rendered := state.Render()
	if !strings.Contains(rendered, "Network error") {
		t.Error("expected rendered output to contain error message")
	}
}

func TestImportIssuesState_Render_NoIssues(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.SetIssues([]IssueItem{})

	rendered := state.Render()
	if !strings.Contains(rendered, "No open issues found") {
		t.Error("expected rendered output to contain 'no issues' message")
	}
}

func TestImportIssuesState_Render_WithIssues_GitHub(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.SetIssues([]IssueItem{
		{ID: "123", Title: "Fix bug", Source: "github"},
		{ID: "456", Title: "Add feature", Source: "github"},
	})

	rendered := state.Render()
	if !strings.Contains(rendered, "#123") {
		t.Error("expected rendered output to contain issue number")
	}
	if !strings.Contains(rendered, "Fix bug") {
		t.Error("expected rendered output to contain issue title")
	}
}

func TestImportIssuesState_Render_WithIssues_Asana(t *testing.T) {
	state := NewImportIssuesStateWithSource("/repo/path", "test-repo", "asana", "", true, true)
	state.SetIssues([]IssueItem{
		{ID: "1234567890", Title: "Complete task", Source: "asana"},
	})

	rendered := state.Render()
	// Asana issues shouldn't show the ID in the format "#123:"
	if strings.Contains(rendered, "#1234567890:") {
		t.Error("Asana tasks should not show issue number in GitHub format")
	}
	if !strings.Contains(rendered, "Complete task") {
		t.Error("expected rendered output to contain task title")
	}
}

func TestImportIssuesState_Render_TitleTruncation(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)

	// Create an issue with a very long title
	longTitle := strings.Repeat("a", 200)
	state.SetIssues([]IssueItem{
		{ID: "1", Title: longTitle, Source: "github"},
	})

	rendered := state.Render()

	// The rendered output should not contain the full long title
	// It should be truncated and end with "..."
	if strings.Contains(rendered, longTitle) {
		t.Error("expected long title to be truncated in rendered output")
	}
	if !strings.Contains(rendered, "...") {
		t.Error("expected truncated title to end with '...'")
	}
}

func TestImportIssuesState_Render_UnicodeTitleTruncation(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)

	// Create an issue with multi-byte Unicode characters
	// Use bullet points (•) which are 3 bytes in UTF-8
	unicodeTitle := strings.Repeat("Fix • bug • ", 20) // 240 characters with multi-byte chars
	state.SetIssues([]IssueItem{
		{ID: "1", Title: unicodeTitle, Source: "github"},
	})

	rendered := state.Render()

	// The rendered output should be truncated without breaking Unicode characters
	// Should not contain the full title but should be valid UTF-8
	if strings.Contains(rendered, unicodeTitle) {
		t.Error("expected long Unicode title to be truncated in rendered output")
	}
	if !strings.Contains(rendered, "...") {
		t.Error("expected truncated Unicode title to end with '...'")
	}

	// Verify the truncation didn't break Unicode by checking it contains valid bullet points
	// (if truncation broke mid-character, we'd have replacement characters or garbled output)
	if strings.Contains(rendered, "Fix • bug") {
		// Good - Unicode characters are intact
	} else {
		t.Error("expected truncated output to maintain valid Unicode characters")
	}
}

func TestImportIssuesState_Render_SelectedCount(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)
	state.SetIssues([]IssueItem{
		{ID: "1", Title: "Issue 1", Source: "github", Selected: true},
		{ID: "2", Title: "Issue 2", Source: "github", Selected: false},
		{ID: "3", Title: "Issue 3", Source: "github", Selected: true},
	})

	rendered := state.Render()

	// Should show "2 issue(s) selected - will create 2 session(s)"
	if !strings.Contains(rendered, "2 issue(s) selected") {
		t.Error("expected rendered output to show selected count")
	}
	if !strings.Contains(rendered, "will create 2 session(s)") {
		t.Error("expected rendered output to show session creation message")
	}
}

func TestImportIssuesState_Render_ScrollIndicators(t *testing.T) {
	state := NewImportIssuesState("/repo/path", "test-repo", true, true)

	// Create more issues than maxVisible
	issues := make([]IssueItem, 15)
	for i := 0; i < 15; i++ {
		issues[i] = IssueItem{ID: formatInt(i + 1), Title: "Issue " + formatInt(i+1), Source: "github"}
	}
	state.SetIssues(issues)

	// Scroll down a bit
	state.SelectedIndex = 5
	state.ScrollOffset = 2

	rendered := state.Render()

	// Should show scroll indicators (at least one should be present when scrolled)
	if !strings.Contains(rendered, "up more above") || !strings.Contains(rendered, "down more below") {
		t.Error("expected rendered output to show scroll indicators when scrolled")
	}
}

// =============================================================================
// Type assertion tests - ensure ImportIssuesState implements ModalState
// =============================================================================

func TestImportIssuesState_ImplementsModalState(t *testing.T) {
	var _ ModalState = (*ImportIssuesState)(nil)
}
