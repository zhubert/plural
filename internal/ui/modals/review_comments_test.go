package modals

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// =============================================================================
// ReviewCommentsState Tests
// =============================================================================

func TestNewReviewCommentsState(t *testing.T) {
	state := NewReviewCommentsState("session-123", "feature-branch")

	if state.SessionID != "session-123" {
		t.Errorf("expected session ID 'session-123', got '%s'", state.SessionID)
	}
	if state.Branch != "feature-branch" {
		t.Errorf("expected branch 'feature-branch', got '%s'", state.Branch)
	}
	if !state.Loading {
		t.Error("expected Loading to be true initially")
	}
	if state.SelectedIndex != 0 {
		t.Errorf("expected initial selected index 0, got %d", state.SelectedIndex)
	}
	if state.LoadError != "" {
		t.Errorf("expected empty load error, got '%s'", state.LoadError)
	}
}

func TestReviewCommentsState_Title(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	if state.Title() != "PR Review Comments" {
		t.Errorf("expected title 'PR Review Comments', got '%s'", state.Title())
	}
}

func TestReviewCommentsState_PreferredWidth(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	width := state.PreferredWidth()

	if width != ModalWidthWide {
		t.Errorf("expected preferred width %d, got %d", ModalWidthWide, width)
	}

	if width <= ModalWidth {
		t.Errorf("expected wide modal width (%d) to be greater than default modal width (%d)", width, ModalWidth)
	}
}

func TestReviewCommentsState_ImplementsModalWithPreferredWidth(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	_, ok := interface{}(state).(ModalWithPreferredWidth)
	if !ok {
		t.Error("ReviewCommentsState should implement ModalWithPreferredWidth")
	}
}

func TestReviewCommentsState_ImplementsModalWithSize(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	_, ok := interface{}(state).(ModalWithSize)
	if !ok {
		t.Error("ReviewCommentsState should implement ModalWithSize")
	}
}

func TestReviewCommentsState_ImplementsModalState(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	var _ ModalState = state // Compile-time check
}

func TestReviewCommentsState_Help_Loading(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	help := state.Help()
	if !strings.Contains(help, "Loading") {
		t.Errorf("expected loading help text, got '%s'", help)
	}
}

func TestReviewCommentsState_Help_Error(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	state.SetError("some error")
	help := state.Help()
	if !strings.Contains(help, "Esc") {
		t.Errorf("expected error help to mention Esc, got '%s'", help)
	}
}

func TestReviewCommentsState_Help_Normal(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	state.SetComments([]ReviewCommentItem{{Body: "test"}})
	help := state.Help()
	if !strings.Contains(help, "select all") {
		t.Errorf("expected normal help to mention 'select all', got '%s'", help)
	}
	if !strings.Contains(help, "Enter") {
		t.Errorf("expected normal help to mention 'Enter', got '%s'", help)
	}
}

func TestReviewCommentsState_SetComments(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	comments := []ReviewCommentItem{
		{Author: "user1", Body: "Fix this"},
		{Author: "user2", Body: "And this"},
	}
	state.SetComments(comments)

	if state.Loading {
		t.Error("expected Loading to be false after SetComments")
	}
	if state.LoadError != "" {
		t.Errorf("expected empty error after SetComments, got '%s'", state.LoadError)
	}
	if len(state.Comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(state.Comments))
	}
}

func TestReviewCommentsState_SetError(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	state.SetError("gh pr view failed")

	if state.Loading {
		t.Error("expected Loading to be false after SetError")
	}
	if state.LoadError != "gh pr view failed" {
		t.Errorf("expected error 'gh pr view failed', got '%s'", state.LoadError)
	}
}

func TestReviewCommentsState_Update_Navigation(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	state.SetComments([]ReviewCommentItem{
		{Body: "first"},
		{Body: "second"},
		{Body: "third"},
	})

	// Move down
	state.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if state.SelectedIndex != 1 {
		t.Errorf("expected index 1 after down, got %d", state.SelectedIndex)
	}

	// Move down again
	state.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if state.SelectedIndex != 2 {
		t.Errorf("expected index 2 after second down, got %d", state.SelectedIndex)
	}

	// Move down at bottom - should stay
	state.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if state.SelectedIndex != 2 {
		t.Errorf("expected index 2 (clamped), got %d", state.SelectedIndex)
	}

	// Move up
	state.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if state.SelectedIndex != 1 {
		t.Errorf("expected index 1 after up, got %d", state.SelectedIndex)
	}

	// Move up to top
	state.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if state.SelectedIndex != 0 {
		t.Errorf("expected index 0 after up to top, got %d", state.SelectedIndex)
	}

	// Move up at top - should stay
	state.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if state.SelectedIndex != 0 {
		t.Errorf("expected index 0 (clamped), got %d", state.SelectedIndex)
	}
}

func TestReviewCommentsState_Update_ToggleSelection(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	state.SetComments([]ReviewCommentItem{
		{Body: "first", Selected: false},
		{Body: "second", Selected: false},
	})

	// Toggle first item
	state.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !state.Comments[0].Selected {
		t.Error("expected first comment to be selected after space")
	}

	// Toggle again to deselect
	state.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if state.Comments[0].Selected {
		t.Error("expected first comment to be deselected after second space")
	}
}

func TestReviewCommentsState_Update_SelectAll(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	state.SetComments([]ReviewCommentItem{
		{Body: "first", Selected: false},
		{Body: "second", Selected: false},
		{Body: "third", Selected: true},
	})

	// Press 'a' - not all selected, so select all
	state.Update(tea.KeyPressMsg{Code: 'a'})
	for i, c := range state.Comments {
		if !c.Selected {
			t.Errorf("expected comment %d to be selected after 'a'", i)
		}
	}

	// Press 'a' again - all selected, so deselect all
	state.Update(tea.KeyPressMsg{Code: 'a'})
	for i, c := range state.Comments {
		if c.Selected {
			t.Errorf("expected comment %d to be deselected after second 'a'", i)
		}
	}
}

func TestReviewCommentsState_GetSelectedComments(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	state.SetComments([]ReviewCommentItem{
		{Body: "first", Selected: true},
		{Body: "second", Selected: false},
		{Body: "third", Selected: true},
	})

	selected := state.GetSelectedComments()
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected, got %d", len(selected))
	}
	if selected[0].Body != "first" {
		t.Errorf("expected first selected body 'first', got '%s'", selected[0].Body)
	}
	if selected[1].Body != "third" {
		t.Errorf("expected second selected body 'third', got '%s'", selected[1].Body)
	}
}

func TestReviewCommentsState_GetSelectedComments_NoneSelected(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	state.SetComments([]ReviewCommentItem{
		{Body: "first", Selected: false},
		{Body: "second", Selected: false},
	})

	selected := state.GetSelectedComments()
	if len(selected) != 0 {
		t.Errorf("expected 0 selected, got %d", len(selected))
	}
}

func TestReviewCommentsState_Render_Loading(t *testing.T) {
	state := NewReviewCommentsState("s1", "feature-branch")
	rendered := state.Render()
	if !strings.Contains(rendered, "Fetching review comments") {
		t.Error("expected loading message in render")
	}
	if !strings.Contains(rendered, "feature-branch") {
		t.Error("expected branch name in render")
	}
}

func TestReviewCommentsState_Render_Error(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	state.SetError("no pull requests found")
	rendered := state.Render()
	if !strings.Contains(rendered, "no pull requests found") {
		t.Error("expected error message in render")
	}
}

func TestReviewCommentsState_Render_NoComments(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	state.SetComments(nil)
	rendered := state.Render()
	if !strings.Contains(rendered, "No review comments found") {
		t.Error("expected 'No review comments found' in render")
	}
}

func TestReviewCommentsState_Render_WithComments(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	state.SetComments([]ReviewCommentItem{
		{Author: "reviewer", Body: "Please fix this bug", Selected: true},
		{Author: "someone", Body: "What about X?", Selected: false},
	})
	rendered := state.Render()
	if !strings.Contains(rendered, "@reviewer") {
		t.Error("expected @reviewer in render")
	}
	if !strings.Contains(rendered, "@someone") {
		t.Error("expected @someone in render")
	}
	if !strings.Contains(rendered, "[x]") {
		t.Error("expected [x] checkbox for selected comment")
	}
	if !strings.Contains(rendered, "[ ]") {
		t.Error("expected [ ] checkbox for unselected comment")
	}
	if !strings.Contains(rendered, "2 of 2") || !strings.Contains(rendered, "1 of 2") {
		// One is selected out of two
		if !strings.Contains(rendered, "1 of 2") {
			t.Error("expected '1 of 2 comment(s) selected' in render")
		}
	}
}

func TestReviewCommentsState_Render_InlineComment(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	state.SetComments([]ReviewCommentItem{
		{Author: "reviewer", Body: "Use mutex", Path: "internal/app.go", Line: 42, Selected: true},
	})
	rendered := state.Render()
	if !strings.Contains(rendered, "internal/app.go:42") {
		t.Error("expected file path and line in render for inline comment")
	}
}

func TestReviewCommentsState_SetSize(t *testing.T) {
	state := NewReviewCommentsState("s1", "branch")
	state.SetSize(100, 50)
	if state.availableWidth != 100 {
		t.Errorf("expected availableWidth 100, got %d", state.availableWidth)
	}
}

