package ui

import (
	"fmt"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/zhubert/plural/internal/git"
)

// EnterViewChangesMode enters the temporary diff view overlay with file navigation
func (c *Chat) EnterViewChangesMode(files []git.FileDiff) {
	c.viewChanges = &ViewChangesState{
		Files:     files,
		FileIndex: 0,
		Viewport:  viewport.New(),
	}

	// Configure viewport
	c.viewChanges.Viewport.MouseWheelEnabled = true
	c.viewChanges.Viewport.MouseWheelDelta = 3
	c.viewChanges.Viewport.SoftWrap = true

	// Size it - will be adjusted in render, but set initial size
	c.viewChanges.Viewport.SetWidth(c.viewport.Width() * 2 / 3)
	c.viewChanges.Viewport.SetHeight(c.viewport.Height())

	// Load the first file's diff
	c.updateViewChangesDiff()
}

// updateViewChangesDiff updates the diff viewport with the currently selected file's diff
func (c *Chat) updateViewChangesDiff() {
	if c.viewChanges == nil || len(c.viewChanges.Files) == 0 {
		if c.viewChanges != nil {
			c.viewChanges.Viewport.SetContent("No files to display")
		}
		return
	}
	if c.viewChanges.FileIndex >= len(c.viewChanges.Files) {
		c.viewChanges.FileIndex = len(c.viewChanges.Files) - 1
	}
	file := c.viewChanges.Files[c.viewChanges.FileIndex]
	content := HighlightDiff(file.Diff)
	c.viewChanges.Viewport.SetContent(content)
	c.viewChanges.Viewport.GotoTop()
}

// ExitViewChangesMode exits the diff view overlay and returns to chat
func (c *Chat) ExitViewChangesMode() {
	c.viewChanges = nil
}

// IsInViewChangesMode returns whether we're currently showing the diff overlay
func (c *Chat) IsInViewChangesMode() bool {
	return c.viewChanges != nil
}

// GetSelectedFileIndex returns the currently selected file index in view changes mode.
// Used for testing navigation.
func (c *Chat) GetSelectedFileIndex() int {
	if c.viewChanges == nil {
		return 0
	}
	return c.viewChanges.FileIndex
}

// renderViewChangesMode renders the diff overlay view with a compact file navigation bar
func (c *Chat) renderViewChangesMode(panelStyle lipgloss.Style) string {
	if c.viewChanges == nil {
		return ""
	}

	// Calculate dimensions
	innerWidth := c.width - 2 // Account for panel border
	innerHeight := c.height - 2

	// Build the compact navigation bar
	navBar := c.renderFileNavBar(innerWidth)
	navBarHeight := 1 // Single line navigation

	// Diff viewport gets remaining height
	diffHeight := innerHeight - navBarHeight

	// Update diff viewport size to use full width
	c.viewChanges.Viewport.SetWidth(innerWidth)
	c.viewChanges.Viewport.SetHeight(diffHeight)

	// Get viewport content and constrain to max height to prevent layout overflow
	diffContent := lipgloss.NewStyle().
		MaxHeight(diffHeight).
		Render(c.viewChanges.Viewport.View())

	// Join navigation bar and diff vertically
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		navBar,
		diffContent,
	)

	return panelStyle.Width(c.width).Height(c.height).Render(content)
}

// renderFileNavBar renders the compact horizontal file navigation bar
func (c *Chat) renderFileNavBar(width int) string {
	if c.viewChanges == nil || len(c.viewChanges.Files) == 0 {
		return lipgloss.NewStyle().
			Width(width).
			Foreground(ColorTextMuted).
			Render("No files to display")
	}

	currentFile := c.viewChanges.Files[c.viewChanges.FileIndex]

	// Build: "← [M] src/file.go (3 of 7) →"
	// Left arrow (show if not first file)
	leftArrow := "  "
	if c.viewChanges.FileIndex > 0 {
		leftArrow = "← "
	}

	// Right arrow (show if not last file)
	rightArrow := "  "
	if c.viewChanges.FileIndex < len(c.viewChanges.Files)-1 {
		rightArrow = " →"
	}

	// Status code in brackets
	statusStyle := lipgloss.NewStyle().Foreground(ColorInfo).Bold(true)
	status := statusStyle.Render(fmt.Sprintf("[%s]", currentFile.Status))

	// File counter
	counterStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	counter := counterStyle.Render(fmt.Sprintf("(%d of %d)", c.viewChanges.FileIndex+1, len(c.viewChanges.Files)))

	// Arrow styles
	arrowStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)

	// Calculate available width for filename
	// Format: "← [M] filename (3 of 7) →"
	fixedWidth := lipgloss.Width(leftArrow) + 4 + 1 + lipgloss.Width(counter) + lipgloss.Width(rightArrow) + 2 // arrows, status, spaces, counter
	maxFilenameWidth := max(width-fixedWidth, 10)

	// Truncate filename if needed
	filename := currentFile.Filename
	if len(filename) > maxFilenameWidth {
		filename = "…" + filename[len(filename)-maxFilenameWidth+1:]
	}
	filenameStyle := lipgloss.NewStyle().Foreground(ColorText)

	// Assemble the navigation bar
	navContent := arrowStyle.Render(leftArrow) +
		status + " " +
		filenameStyle.Render(filename) + " " +
		counter +
		arrowStyle.Render(rightArrow)

	// Style the whole bar (no explicit background - let terminal's native background show through)
	barStyle := lipgloss.NewStyle().
		Width(width)

	return barStyle.Render(navContent)
}
