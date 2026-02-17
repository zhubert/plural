package agent

import (
	"strings"
	"testing"

	"github.com/zhubert/plural/internal/git"
)

func TestTrimURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid URL",
			input:    "https://github.com/user/repo/pull/42",
			expected: "https://github.com/user/repo/pull/42",
		},
		{
			name:     "URL with whitespace",
			input:    "  https://github.com/user/repo/pull/42  ",
			expected: "https://github.com/user/repo/pull/42",
		},
		{
			name:     "non-URL text",
			input:    "Creating pull request...",
			expected: "",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "http prefix not supported",
			input:    "http://github.com/user/repo",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimURL(tt.input)
			if got != tt.expected {
				t.Errorf("trimURL(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFormatPRCommentsPrompt(t *testing.T) {
	t.Run("single comment", func(t *testing.T) {
		comments := []git.PRReviewComment{
			{Author: "alice", Body: "Fix this typo", Path: "main.go", Line: 42},
		}

		result := formatPRCommentsPrompt(comments)

		if !strings.Contains(result, "1 comment(s)") {
			t.Error("expected comment count in prompt")
		}
		if !strings.Contains(result, "@alice") {
			t.Error("expected author in prompt")
		}
		if !strings.Contains(result, "Fix this typo") {
			t.Error("expected comment body in prompt")
		}
		if !strings.Contains(result, "main.go:42") {
			t.Error("expected file path and line in prompt")
		}
	})

	t.Run("multiple comments", func(t *testing.T) {
		comments := []git.PRReviewComment{
			{Author: "alice", Body: "Fix this", Path: "a.go", Line: 10},
			{Author: "bob", Body: "Add tests", Path: "b.go"},
		}

		result := formatPRCommentsPrompt(comments)

		if !strings.Contains(result, "2 comment(s)") {
			t.Error("expected comment count")
		}
		if !strings.Contains(result, "Comment 1") {
			t.Error("expected first comment header")
		}
		if !strings.Contains(result, "Comment 2") {
			t.Error("expected second comment header")
		}
		if !strings.Contains(result, "a.go:10") {
			t.Error("expected file:line for comment with line number")
		}
		// Second comment has no line number
		if !strings.Contains(result, "File: b.go\n") {
			t.Error("expected file without line number for second comment")
		}
	})

	t.Run("comment without path", func(t *testing.T) {
		comments := []git.PRReviewComment{
			{Author: "reviewer", Body: "General feedback"},
		}

		result := formatPRCommentsPrompt(comments)

		if strings.Contains(result, "File:") {
			t.Error("should not contain File: for comment without path")
		}
		if !strings.Contains(result, "General feedback") {
			t.Error("expected comment body")
		}
	})

	t.Run("comment without author", func(t *testing.T) {
		comments := []git.PRReviewComment{
			{Body: "Anonymous comment", Path: "main.go"},
		}

		result := formatPRCommentsPrompt(comments)

		if strings.Contains(result, "by @") {
			t.Error("should not contain author for comment without author")
		}
	})
}

func TestFormatOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncated", "hello world", 5, "hello..."},
		{"whitespace trimmed", "  hello  ", 10, "hello"},
		{"empty string", "", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatOutput(tt.input, tt.maxLen)
			if got != tt.expected {
				t.Errorf("formatOutput(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.expected)
			}
		})
	}
}
