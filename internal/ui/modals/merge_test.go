package modals

import "testing"

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
