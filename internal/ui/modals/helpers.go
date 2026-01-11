package modals

import "strings"

// TruncatePath truncates a path from the beginning with ellipsis
func TruncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}

// TruncateString truncates a string from the end with ellipsis
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// SessionDisplayName returns the display name for a session based on branch and name.
// If the branch is custom (not starting with "plural-"), it returns the branch name.
// Otherwise, it extracts a short ID from the name.
func SessionDisplayName(branch, name string) string {
	if branch != "" && !strings.HasPrefix(branch, "plural-") {
		return branch
	}
	if parts := strings.Split(name, "/"); len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return name
}
