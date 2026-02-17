package agent

import (
	"fmt"
	"strings"

	"github.com/zhubert/plural/internal/git"
)

// trimURL extracts a URL from output text.
func trimURL(output string) string {
	trimmed := strings.TrimSpace(output)
	if strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	return ""
}

// formatPRCommentsPrompt formats PR review comments as a prompt string.
func formatPRCommentsPrompt(comments []git.PRReviewComment) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("New PR review comments need to be addressed (%d comment(s)):\n\n", len(comments)))

	for i, c := range comments {
		sb.WriteString(fmt.Sprintf("--- Comment %d", i+1))
		if c.Author != "" {
			sb.WriteString(fmt.Sprintf(" by @%s", c.Author))
		}
		sb.WriteString(" ---\n")
		if c.Path != "" {
			if c.Line > 0 {
				sb.WriteString(fmt.Sprintf("File: %s:%d\n", c.Path, c.Line))
			} else {
				sb.WriteString(fmt.Sprintf("File: %s\n", c.Path))
			}
		}
		sb.WriteString(c.Body)
		sb.WriteString("\n\n")
	}

	sb.WriteString("Please address each of these review comments. For code changes, make the necessary edits. For questions, provide a response and make any relevant code changes.")
	return sb.String()
}
