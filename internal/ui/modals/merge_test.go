package modals

import (
	"strings"
	"testing"
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
