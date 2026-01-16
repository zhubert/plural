package ui

import (
	"fmt"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/zhubert/plural/internal/git"
)

// EnterViewChangesMode enters the temporary diff view overlay with file navigation
func (c *Chat) EnterViewChangesMode(files []git.FileDiff) {
	c.viewChangesMode = true
	c.viewChangesFiles = files
	c.viewChangesFileIndex = 0

	// Create a fresh viewport for the diff content
	c.viewChangesViewport = viewport.New()
	c.viewChangesViewport.MouseWheelEnabled = true
	c.viewChangesViewport.MouseWheelDelta = 3
	c.viewChangesViewport.SoftWrap = true

	// Size it - will be adjusted in render, but set initial size
	c.viewChangesViewport.SetWidth(c.viewport.Width() * 2 / 3)
	c.viewChangesViewport.SetHeight(c.viewport.Height())

	// Load the first file's diff
	c.updateViewChangesDiff()
}

// updateViewChangesDiff updates the diff viewport with the currently selected file's diff
func (c *Chat) updateViewChangesDiff() {
	if len(c.viewChangesFiles) == 0 {
		c.viewChangesViewport.SetContent("No files to display")
		return
	}
	if c.viewChangesFileIndex >= len(c.viewChangesFiles) {
		c.viewChangesFileIndex = len(c.viewChangesFiles) - 1
	}
	file := c.viewChangesFiles[c.viewChangesFileIndex]
	content := HighlightDiff(file.Diff)
	c.viewChangesViewport.SetContent(content)
	c.viewChangesViewport.GotoTop()
}

// ExitViewChangesMode exits the diff view overlay and returns to chat
func (c *Chat) ExitViewChangesMode() {
	c.viewChangesMode = false
	c.viewChangesFiles = nil
	c.viewChangesFileIndex = 0
}

// IsInViewChangesMode returns whether we're currently showing the diff overlay
func (c *Chat) IsInViewChangesMode() bool {
	return c.viewChangesMode
}

// GetSelectedFileIndex returns the currently selected file index in view changes mode.
// Used for testing navigation.
func (c *Chat) GetSelectedFileIndex() int {
	return c.viewChangesFileIndex
}

// renderViewChangesMode renders the diff overlay view with a compact file navigation bar
func (c *Chat) renderViewChangesMode(panelStyle lipgloss.Style) string {
	// Calculate dimensions
	innerWidth := c.width - 2 // Account for panel border
	innerHeight := c.height - 2

	// Build the compact navigation bar
	navBar := c.renderFileNavBar(innerWidth)
	navBarHeight := 1 // Single line navigation

	// Diff viewport gets remaining height
	diffHeight := innerHeight - navBarHeight

	// Update diff viewport size to use full width
	c.viewChangesViewport.SetWidth(innerWidth)
	c.viewChangesViewport.SetHeight(diffHeight)

	// Get viewport content and constrain to max height to prevent layout overflow
	diffContent := lipgloss.NewStyle().
		MaxHeight(diffHeight).
		Render(c.viewChangesViewport.View())

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
	if len(c.viewChangesFiles) == 0 {
		return lipgloss.NewStyle().
			Width(width).
			Foreground(ColorTextMuted).
			Render("No files to display")
	}

	currentFile := c.viewChangesFiles[c.viewChangesFileIndex]

	// Build: "← [M] src/file.go (3 of 7) →"
	// Left arrow (show if not first file)
	leftArrow := "  "
	if c.viewChangesFileIndex > 0 {
		leftArrow = "← "
	}

	// Right arrow (show if not last file)
	rightArrow := "  "
	if c.viewChangesFileIndex < len(c.viewChangesFiles)-1 {
		rightArrow = " →"
	}

	// Status code in brackets
	statusStyle := lipgloss.NewStyle().Foreground(ColorInfo).Bold(true)
	status := statusStyle.Render(fmt.Sprintf("[%s]", currentFile.Status))

	// File counter
	counterStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	counter := counterStyle.Render(fmt.Sprintf("(%d of %d)", c.viewChangesFileIndex+1, len(c.viewChangesFiles)))

	// Arrow styles
	arrowStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)

	// Calculate available width for filename
	// Format: "← [M] filename (3 of 7) →"
	fixedWidth := len(leftArrow) + 4 + 1 + len(counter) + len(rightArrow) + 2 // arrows, status, spaces, counter
	maxFilenameWidth := width - fixedWidth
	if maxFilenameWidth < 10 {
		maxFilenameWidth = 10
	}

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
