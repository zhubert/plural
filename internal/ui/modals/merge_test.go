package modals

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestUnwrapCommitMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "summary only",
			input:    "Add new feature",
			expected: "Add new feature",
		},
		{
			name:     "summary with body on same line (no blank)",
			input:    "Add new feature\nThis is the body",
			expected: "Add new feature\nThis is the body",
		},
		{
			name:     "properly formatted with unwrapped body",
			input:    "Add new feature\n\nThis is the body of the commit message.",
			expected: "Add new feature\n\nThis is the body of the commit message.",
		},
		{
			name: "body wrapped at 72 chars",
			input: `Add file-by-file navigation to view changes mode

The single diff view is replaced with a two-pane layout: file list on
the left, diff content on the right. Users can navigate between files
with arrow keys and switch panes to scroll through individual diffs.
This makes reviewing changes in sessions with many modified files
significantly easier.`,
			expected: `Add file-by-file navigation to view changes mode

The single diff view is replaced with a two-pane layout: file list on the left, diff content on the right. Users can navigate between files with arrow keys and switch panes to scroll through individual diffs. This makes reviewing changes in sessions with many modified files significantly easier.`,
		},
		{
			name: "multiple paragraphs",
			input: `Add new feature

First paragraph that was wrapped at some
arbitrary column width.

Second paragraph also wrapped
at some width.`,
			expected: `Add new feature

First paragraph that was wrapped at some arbitrary column width.

Second paragraph also wrapped at some width.`,
		},
		{
			name:     "empty message",
			input:    "",
			expected: "",
		},
		{
			name: "body with extra spaces after unwrapping",
			input: `Summary line

Word1  word2
word3`,
			expected: `Summary line

Word1 word2 word3`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := unwrapCommitMessage(tt.input)
			if result != tt.expected {
				t.Errorf("\nInput:\n%s\n\nExpected:\n%s\n\nGot:\n%s", tt.input, tt.expected, result)
			}
		})
	}
}

func TestLoadingCommitState(t *testing.T) {
	t.Run("new state has correct defaults", func(t *testing.T) {
		state := NewLoadingCommitState("pr")
		if state.MergeType != "pr" {
			t.Errorf("expected MergeType 'pr', got %q", state.MergeType)
		}
		if state.SpinnerFrame != 0 {
			t.Errorf("expected SpinnerFrame 0, got %d", state.SpinnerFrame)
		}
	})

	t.Run("title is correct", func(t *testing.T) {
		state := NewLoadingCommitState("merge")
		if state.Title() != "Generating Commit Message" {
			t.Errorf("unexpected title: %q", state.Title())
		}
	})

	t.Run("help text shows cancel option", func(t *testing.T) {
		state := NewLoadingCommitState("merge")
		help := state.Help()
		if !strings.Contains(help, "Esc") {
			t.Errorf("help should mention Esc key: %q", help)
		}
	})

	t.Run("render shows operation label for each merge type", func(t *testing.T) {
		tests := []struct {
			mergeType     string
			expectedLabel string
		}{
			{"pr", "Create PR"},
			{"push", "Push updates to PR"},
			{"parent", "Merge to parent"},
			{"merge", "Merge to main"},
		}

		for _, tt := range tests {
			state := NewLoadingCommitState(tt.mergeType)
			rendered := state.Render()
			if !strings.Contains(rendered, tt.expectedLabel) {
				t.Errorf("render for %q should contain %q, got:\n%s", tt.mergeType, tt.expectedLabel, rendered)
			}
		}
	})

	t.Run("render shows waiting message", func(t *testing.T) {
		state := NewLoadingCommitState("merge")
		rendered := state.Render()
		if !strings.Contains(rendered, "Waiting for Claude") {
			t.Errorf("render should contain 'Waiting for Claude', got:\n%s", rendered)
		}
	})

	t.Run("advance spinner increments frame", func(t *testing.T) {
		state := NewLoadingCommitState("merge")
		if state.SpinnerFrame != 0 {
			t.Errorf("expected initial frame 0, got %d", state.SpinnerFrame)
		}
		state.AdvanceSpinner()
		if state.SpinnerFrame != 1 {
			t.Errorf("expected frame 1 after advance, got %d", state.SpinnerFrame)
		}
	})

	t.Run("spinner wraps around", func(t *testing.T) {
		state := NewLoadingCommitState("merge")
		// Advance through all frames
		for i := 0; i < len(spinnerFrames); i++ {
			state.AdvanceSpinner()
		}
		// Should have wrapped to 0
		if state.SpinnerFrame != 0 {
			t.Errorf("expected frame to wrap to 0, got %d", state.SpinnerFrame)
		}
	})

	t.Run("update returns self unchanged", func(t *testing.T) {
		state := NewLoadingCommitState("merge")
		newState, cmd := state.Update(nil)
		if newState != state {
			t.Error("update should return same state")
		}
		if cmd != nil {
			t.Error("update should return nil cmd")
		}
	})
}

func initTestStyles() {
	// Initialize styles with minimal values for testing
	ModalTitleStyle = lipgloss.NewStyle().Bold(true)
	ModalHelpStyle = lipgloss.NewStyle().Italic(true)
	SidebarItemStyle = lipgloss.NewStyle()
	SidebarSelectedStyle = lipgloss.NewStyle().Reverse(true)

	ColorPrimary = color.RGBA{R: 100, G: 100, B: 255, A: 255}
	ColorSecondary = color.RGBA{R: 100, G: 255, B: 100, A: 255}
	ColorText = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	ColorTextMuted = color.RGBA{R: 128, G: 128, B: 128, A: 255}

	ModalWidth = 60
}

func TestMergeConflictState(t *testing.T) {
	initTestStyles()

	t.Run("new state has correct defaults", func(t *testing.T) {
		state := NewMergeConflictState("session-123", "test-session", []string{"file1.go", "file2.go"}, "/repo/path")
		if state.SessionID != "session-123" {
			t.Errorf("expected SessionID 'session-123', got %q", state.SessionID)
		}
		if state.SessionName != "test-session" {
			t.Errorf("expected SessionName 'test-session', got %q", state.SessionName)
		}
		if len(state.ConflictedFiles) != 2 {
			t.Errorf("expected 2 conflicted files, got %d", len(state.ConflictedFiles))
		}
		if state.RepoPath != "/repo/path" {
			t.Errorf("expected RepoPath '/repo/path', got %q", state.RepoPath)
		}
		if len(state.Options) != 3 {
			t.Errorf("expected 3 options, got %d", len(state.Options))
		}
		if state.SelectedIndex != 0 {
			t.Errorf("expected SelectedIndex 0, got %d", state.SelectedIndex)
		}
	})

	t.Run("title is correct", func(t *testing.T) {
		state := NewMergeConflictState("s", "n", nil, "/p")
		if state.Title() != "Merge Conflict" {
			t.Errorf("unexpected title: %q", state.Title())
		}
	})

	t.Run("help text shows navigation instructions", func(t *testing.T) {
		state := NewMergeConflictState("s", "n", nil, "/p")
		help := state.Help()
		if !strings.Contains(help, "up/down") {
			t.Errorf("help should mention navigation keys: %q", help)
		}
		if !strings.Contains(help, "Enter") {
			t.Errorf("help should mention Enter key: %q", help)
		}
	})

	t.Run("render contains session name", func(t *testing.T) {
		state := NewMergeConflictState("s", "my-session", []string{"file.go"}, "/p")
		rendered := state.Render()
		if !strings.Contains(rendered, "my-session") {
			t.Errorf("render should contain session name, got:\n%s", rendered)
		}
	})

	t.Run("render contains conflicted files label", func(t *testing.T) {
		state := NewMergeConflictState("s", "n", []string{"file.go"}, "/p")
		rendered := state.Render()
		if !strings.Contains(rendered, "Conflicted files:") {
			t.Errorf("render should contain 'Conflicted files:', got:\n%s", rendered)
		}
	})

	t.Run("render shows conflicted files", func(t *testing.T) {
		state := NewMergeConflictState("s", "n", []string{"file1.go", "file2.go"}, "/p")
		rendered := state.Render()
		if !strings.Contains(rendered, "file1.go") {
			t.Errorf("render should contain file1.go, got:\n%s", rendered)
		}
		if !strings.Contains(rendered, "file2.go") {
			t.Errorf("render should contain file2.go, got:\n%s", rendered)
		}
	})

	t.Run("render truncates to max 5 files", func(t *testing.T) {
		files := []string{"f1.go", "f2.go", "f3.go", "f4.go", "f5.go", "f6.go", "f7.go"}
		state := NewMergeConflictState("s", "n", files, "/p")
		rendered := state.Render()
		// Should show first 5 files
		if !strings.Contains(rendered, "f1.go") {
			t.Errorf("render should contain f1.go")
		}
		if !strings.Contains(rendered, "f5.go") {
			t.Errorf("render should contain f5.go")
		}
		// Should NOT show f6.go or f7.go
		if strings.Contains(rendered, "f6.go") {
			t.Errorf("render should NOT contain f6.go")
		}
		// Should show "and 2 more"
		if !strings.Contains(rendered, "2 more") {
			t.Errorf("render should contain '2 more', got:\n%s", rendered)
		}
	})

	t.Run("render shows all 3 options", func(t *testing.T) {
		state := NewMergeConflictState("s", "n", []string{"f.go"}, "/p")
		rendered := state.Render()
		if !strings.Contains(rendered, "Have Claude resolve") {
			t.Errorf("render should contain 'Have Claude resolve'")
		}
		if !strings.Contains(rendered, "Abort merge") {
			t.Errorf("render should contain 'Abort merge'")
		}
		if !strings.Contains(rendered, "Resolve manually") {
			t.Errorf("render should contain 'Resolve manually'")
		}
	})

	t.Run("render does not have extra blank lines between files", func(t *testing.T) {
		state := NewMergeConflictState("s", "n", []string{"file1.go", "file2.go", "file3.go"}, "/p")
		rendered := state.Render()
		// Check that file lines are consecutive (no double newlines between them)
		if strings.Contains(rendered, "file1.go\n\n") {
			t.Errorf("render should not have double newlines after file1.go, got:\n%s", rendered)
		}
		if strings.Contains(rendered, "file2.go\n\n") {
			t.Errorf("render should not have double newlines after file2.go, got:\n%s", rendered)
		}
	})

	t.Run("get selected option returns correct index", func(t *testing.T) {
		state := NewMergeConflictState("s", "n", []string{"f.go"}, "/p")
		if state.GetSelectedOption() != 0 {
			t.Errorf("expected selected option 0, got %d", state.GetSelectedOption())
		}
		state.SelectedIndex = 2
		if state.GetSelectedOption() != 2 {
			t.Errorf("expected selected option 2, got %d", state.GetSelectedOption())
		}
	})
}
