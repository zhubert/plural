package modals

import (
	"strings"
)

// ListItemRenderer provides common list item rendering utilities.
// It handles the pattern of rendering selectable items with proper
// styling for selected vs unselected states.

// RenderSelectableList renders a simple list with selection highlighting.
// Returns the rendered list string. selectedIndex indicates which item is selected.
func RenderSelectableList(items []string, selectedIndex int) string {
	var result strings.Builder
	for i, item := range items {
		style := SidebarItemStyle
		prefix := "  "
		if i == selectedIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}
		result.WriteString(style.Render(prefix+item) + "\n")
	}
	return result.String()
}

// RenderSelectableListWithFocus renders a list where selection is only shown when focused.
// When focus is true, the selected item is highlighted; otherwise all items use the normal style.
// marker is shown next to the selected item when not focused (e.g., "* ")
func RenderSelectableListWithFocus(items []string, selectedIndex int, focused bool, marker string) string {
	var result strings.Builder
	for i, item := range items {
		style := SidebarItemStyle
		prefix := "  "
		if focused && i == selectedIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		} else if i == selectedIndex {
			prefix = marker
		}
		result.WriteString(style.Render(prefix+item) + "\n")
	}
	return result.String()
}

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
