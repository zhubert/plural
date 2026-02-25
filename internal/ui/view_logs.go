package ui

import (
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/zhubert/plural/internal/logger"
)

// GetLogFiles returns a list of available log files for viewing.
// It finds the main debug log and any session-specific MCP/stream logs.
// Always returns a non-nil slice (may be empty if no log files found).
func GetLogFiles(currentSessionID string) []LogFile {
	files := []LogFile{}

	// Main debug log
	if defaultPath, err := logger.DefaultLogPath(); err == nil {
		if _, err := os.Stat(defaultPath); err == nil {
			files = append(files, LogFile{
				Name: "Debug Log",
				Path: defaultPath,
			})
		}
	}

	// Session-specific logs for current session only
	if currentSessionID != "" {
		if mcpPath, err := logger.MCPLogPath(currentSessionID); err == nil {
			if _, err := os.Stat(mcpPath); err == nil {
				files = append(files, LogFile{
					Name: fmt.Sprintf("MCP (%s)", truncateSessionID(currentSessionID)),
					Path: mcpPath,
				})
			}
		}

		if streamPath, err := logger.StreamLogPath(currentSessionID); err == nil {
			if _, err := os.Stat(streamPath); err == nil {
				files = append(files, LogFile{
					Name: fmt.Sprintf("Stream (%s)", truncateSessionID(currentSessionID)),
					Path: streamPath,
				})
			}
		}
	}

	return files
}

// truncateSessionID returns the first 8 characters of a session ID for display.
func truncateSessionID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// EnterLogViewerMode enters the log viewer overlay with available log files.
func (c *Chat) EnterLogViewerMode(files []LogFile) {
	c.logViewer = &LogViewerState{
		Files:      files,
		FileIndex:  0,
		Viewport:   viewport.New(),
		FollowTail: true, // Default to following tail
	}

	// Configure viewport
	c.logViewer.Viewport.MouseWheelEnabled = true
	c.logViewer.Viewport.MouseWheelDelta = 3
	c.logViewer.Viewport.SoftWrap = true

	// Size it - will be adjusted in render, but set initial size
	c.logViewer.Viewport.SetWidth(c.viewport.Width())
	c.logViewer.Viewport.SetHeight(c.viewport.Height())

	// Load the first file's content
	c.updateLogViewerContent()
}

// updateLogViewerContent updates the log viewport with the currently selected file's content.
func (c *Chat) updateLogViewerContent() {
	if c.logViewer == nil || len(c.logViewer.Files) == 0 {
		if c.logViewer != nil {
			c.logViewer.Viewport.SetContent("No log files found")
		}
		return
	}
	if c.logViewer.FileIndex >= len(c.logViewer.Files) {
		c.logViewer.FileIndex = len(c.logViewer.Files) - 1
	}

	file := &c.logViewer.Files[c.logViewer.FileIndex]

	// Read the file content
	content, err := os.ReadFile(file.Path)
	if err != nil {
		c.logViewer.Viewport.SetContent(fmt.Sprintf("Error reading log file: %v", err))
		return
	}

	file.Content = string(content)

	// Apply syntax highlighting for log format
	highlighted := highlightLogContent(file.Content)
	c.logViewer.Viewport.SetContent(highlighted)

	// Go to bottom if following tail, otherwise go to top
	if c.logViewer.FollowTail {
		c.logViewer.Viewport.GotoBottom()
	} else {
		c.logViewer.Viewport.GotoTop()
	}
}

// highlightLogContent applies syntax highlighting to log content.
func highlightLogContent(content string) string {
	var sb strings.Builder
	lines := strings.SplitSeq(content, "\n")

	for line := range lines {
		highlighted := highlightLogLine(line)
		sb.WriteString(highlighted)
		sb.WriteString("\n")
	}

	return sb.String()
}

// highlightLogLine applies syntax highlighting to a single log line.
func highlightLogLine(line string) string {
	if line == "" {
		return line
	}

	// Define styles
	levelErrorStyle := lipgloss.NewStyle().Foreground(ColorError).Bold(true)
	levelWarnStyle := lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
	levelInfoStyle := lipgloss.NewStyle().Foreground(ColorInfo)
	levelDebugStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	keyStyle := lipgloss.NewStyle().Foreground(ColorPrimary)
	valueStyle := lipgloss.NewStyle().Foreground(ColorText)

	// Check for level indicator and apply appropriate style
	if strings.Contains(line, "level=ERROR") {
		line = strings.Replace(line, "level=ERROR", levelErrorStyle.Render("level=ERROR"), 1)
	} else if strings.Contains(line, "level=WARN") {
		line = strings.Replace(line, "level=WARN", levelWarnStyle.Render("level=WARN"), 1)
	} else if strings.Contains(line, "level=INFO") {
		line = strings.Replace(line, "level=INFO", levelInfoStyle.Render("level=INFO"), 1)
	} else if strings.Contains(line, "level=DEBUG") {
		line = strings.Replace(line, "level=DEBUG", levelDebugStyle.Render("level=DEBUG"), 1)
	}

	// Highlight msg= values
	if idx := strings.Index(line, "msg="); idx >= 0 {
		before := line[:idx]
		rest := line[idx:]

		// Find the msg value (could be quoted or unquoted)
		if len(rest) > 4 && rest[4] == '"' {
			// Quoted message - find closing quote
			endIdx := strings.Index(rest[5:], "\"")
			if endIdx >= 0 {
				msgKey := keyStyle.Render("msg=")
				msgValue := valueStyle.Render(rest[4 : 5+endIdx+1])
				line = before + msgKey + msgValue + rest[5+endIdx+1:]
			}
		}
	}

	return line
}

// ExitLogViewerMode exits the log viewer overlay and returns to chat.
func (c *Chat) ExitLogViewerMode() {
	c.logViewer = nil
}

// IsInLogViewerMode returns whether we're currently showing the log viewer overlay.
func (c *Chat) IsInLogViewerMode() bool {
	return c.logViewer != nil
}

// RefreshLogViewer reloads the current log file content.
func (c *Chat) RefreshLogViewer() {
	if c.logViewer != nil {
		c.updateLogViewerContent()
	}
}

// ToggleLogViewerFollowTail toggles the follow tail mode.
func (c *Chat) ToggleLogViewerFollowTail() {
	if c.logViewer != nil {
		c.logViewer.FollowTail = !c.logViewer.FollowTail
		if c.logViewer.FollowTail {
			c.logViewer.Viewport.GotoBottom()
		}
	}
}

// GetLogViewerFollowTail returns whether follow tail mode is enabled.
func (c *Chat) GetLogViewerFollowTail() bool {
	if c.logViewer == nil {
		return false
	}
	return c.logViewer.FollowTail
}

// renderLogViewerMode renders the log viewer overlay with file navigation bar.
func (c *Chat) renderLogViewerMode(panelStyle lipgloss.Style) string {
	if c.logViewer == nil {
		return ""
	}

	// Calculate dimensions
	innerWidth := c.width - 2 // Account for panel border
	innerHeight := c.height - 2

	// Build the navigation bar
	navBar := c.renderLogNavBar(innerWidth)
	navBarHeight := 1 // Single line navigation

	// Log viewport gets remaining height
	logHeight := innerHeight - navBarHeight

	// Update log viewport size
	c.logViewer.Viewport.SetWidth(innerWidth)
	c.logViewer.Viewport.SetHeight(logHeight)

	// Get viewport content and constrain to max height
	logContent := lipgloss.NewStyle().
		MaxHeight(logHeight).
		Render(c.logViewer.Viewport.View())

	// Join navigation bar and log content vertically
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		navBar,
		logContent,
	)

	return panelStyle.Width(c.width).Height(c.height).Render(content)
}

// renderLogNavBar renders the navigation bar for the log viewer.
func (c *Chat) renderLogNavBar(width int) string {
	if c.logViewer == nil || len(c.logViewer.Files) == 0 {
		return lipgloss.NewStyle().
			Width(width).
			Foreground(ColorTextMuted).
			Render("No log files found")
	}

	currentFile := c.logViewer.Files[c.logViewer.FileIndex]

	// Build: "← Debug Log (1 of 3) → [F]ollow"
	// Left arrow (show if not first file)
	leftArrow := "  "
	if c.logViewer.FileIndex > 0 {
		leftArrow = "← "
	}

	// Right arrow (show if not last file)
	rightArrow := "  "
	if c.logViewer.FileIndex < len(c.logViewer.Files)-1 {
		rightArrow = " →"
	}

	// File counter
	counterStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	counter := counterStyle.Render(fmt.Sprintf("(%d of %d)", c.logViewer.FileIndex+1, len(c.logViewer.Files)))

	// Arrow styles
	arrowStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)

	// Follow indicator
	followIndicator := ""
	if c.logViewer.FollowTail {
		followStyle := lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)
		followIndicator = " " + followStyle.Render("[Follow]")
	} else {
		followStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		followIndicator = " " + followStyle.Render("[f: follow]")
	}

	// Refresh hint
	refreshStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	refreshHint := " " + refreshStyle.Render("[r: refresh]")

	// Calculate available width for filename
	fixedWidth := lipgloss.Width(leftArrow) + lipgloss.Width(counter) + lipgloss.Width(rightArrow) + lipgloss.Width(followIndicator) + lipgloss.Width(refreshHint) + 1 // arrows, counter, follow/refresh indicators, space
	maxFilenameWidth := max(width-fixedWidth, 10)

	// Truncate filename if needed
	filename := currentFile.Name
	if len(filename) > maxFilenameWidth {
		filename = filename[:maxFilenameWidth-1] + "…"
	}
	filenameStyle := lipgloss.NewStyle().Foreground(ColorText).Bold(true)

	// Assemble the navigation bar
	navContent := arrowStyle.Render(leftArrow) +
		filenameStyle.Render(filename) + " " +
		counter +
		arrowStyle.Render(rightArrow) +
		followIndicator +
		refreshHint

	// Style the whole bar
	barStyle := lipgloss.NewStyle().
		Width(width)

	return barStyle.Render(navContent)
}
