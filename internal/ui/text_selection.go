// Package ui provides terminal user interface components for Plural.
//
// # Text Selection Coordinate System
//
// The text selection system uses a coordinate system relative to the chat viewport:
//
//	┌─────────────────────────────────────────────┐
//	│ ← 1px border                                │
//	│  ┌─────────────────────────────────────────┐│
//	│  │ (0,0)   Viewport content area           ││
//	│  │                                         ││
//	│  │    Selection coordinates are            ││
//	│  │    relative to this inner area          ││
//	│  │                                         ││
//	│  │                             (width, height)
//	│  └─────────────────────────────────────────┘│
//	│                                 1px border → │
//	└─────────────────────────────────────────────┘
//
// Mouse events from Bubble Tea arrive in terminal coordinates (0,0 = top-left of terminal).
// The Chat component receives events pre-adjusted to panel coordinates (0,0 = top-left of
// chat panel). The text selection code then subtracts 1 from both X and Y to account for
// the panel border, yielding viewport-relative coordinates.
//
// This adjustment happens in chat.go's Update() method for MouseClickMsg, MouseMotionMsg,
// and MouseReleaseMsg events:
//
//	x := msg.X - 1  // Subtract border width
//	y := msg.Y - 1  // Subtract border height
//
// Selection coordinates (selectionStartCol, selectionStartLine, etc.) are stored in
// viewport-relative coordinates. When rendering the selection highlight, these coordinates
// are used directly with the ultraviolet screen buffer which also operates in
// viewport-relative coordinates.
//
// When extracting selected text, the coordinates are used to index into the viewport's
// content lines. ANSI escape codes are stripped before text extraction to ensure
// coordinates align with visible character positions.
package ui

import (
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
	"github.com/zhubert/plural/internal/clipboard"
	"github.com/zhubert/plural/internal/logger"
)

// SelectionCopyMsg is sent after a delay to handle copying selected text
type SelectionCopyMsg struct {
	clickCount   int
	endSelection bool
	x, y         int
}

// ClipboardErrorMsg is sent when clipboard operations fail
type ClipboardErrorMsg struct {
	Error error
}

const (
	doubleClickThreshold = 500 * time.Millisecond
	clickTolerance       = 2 // pixels
)

// StartSelection begins a text selection at the given coordinates
func (c *Chat) StartSelection(col, line int) {
	c.selectionStartCol = col
	c.selectionStartLine = line
	c.selectionEndCol = col
	c.selectionEndLine = line
	c.selectionActive = true
}

// EndSelection updates the end position of the selection during drag
func (c *Chat) EndSelection(col, line int) {
	if !c.selectionActive {
		return
	}
	c.selectionEndCol = col
	c.selectionEndLine = line
}

// SelectionStop ends the drag but keeps the selection visible
func (c *Chat) SelectionStop() {
	c.selectionActive = false
}

// SelectionClear clears the selection entirely
func (c *Chat) SelectionClear() {
	c.selectionStartCol = -1
	c.selectionStartLine = -1
	c.selectionEndCol = -1
	c.selectionEndLine = -1
	c.selectionActive = false
}

// HasTextSelection returns true if there is an active or completed selection
func (c *Chat) HasTextSelection() bool {
	return c.selectionStartCol >= 0 && c.selectionStartLine >= 0 &&
		(c.selectionEndCol != c.selectionStartCol || c.selectionEndLine != c.selectionStartLine)
}

// handleMouseClick handles mouse click events and detects double/triple clicks
func (c *Chat) handleMouseClick(x, y int) tea.Cmd {
	now := time.Now()

	// Check if this is a potential multi-click
	if now.Sub(c.lastClickTime) <= doubleClickThreshold &&
		abs(x-c.lastClickX) <= clickTolerance &&
		abs(y-c.lastClickY) <= clickTolerance {
		c.clickCount++
	} else {
		c.clickCount = 1
	}

	c.lastClickTime = now
	c.lastClickX = x
	c.lastClickY = y

	switch c.clickCount {
	case 1:
		// Single click - start selection
		c.StartSelection(x, y)
	case 2:
		// Double click - select word and copy immediately
		c.SelectWord(x, y)
		return c.CopySelectedText()
	case 3:
		// Triple click - select line/paragraph and copy immediately
		c.SelectParagraph(x, y)
		c.clickCount = 0 // Reset after triple click
		return c.CopySelectedText()
	}

	return nil
}

// abs returns the absolute value of an integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// SelectWord selects the word at the given position
func (c *Chat) SelectWord(col, line int) {
	// Get the content from the viewport
	content := c.viewport.View()
	lines := strings.Split(content, "\n")

	if line < 0 || line >= len(lines) {
		return
	}

	currentLine := ansi.Strip(lines[line])
	if col < 0 || col >= len(currentLine) {
		return
	}

	// Find word boundaries using uniseg
	startCol := col
	endCol := col

	// Search backward for word start
	gr := uniseg.NewGraphemes(currentLine[:col])
	pos := 0
	lastBoundary := 0
	for gr.Next() {
		if gr.IsWordBoundary() {
			lastBoundary = pos
		}
		pos += len(gr.Str())
	}
	startCol = lastBoundary

	// Search forward for word end
	gr = uniseg.NewGraphemes(currentLine[col:])
	pos = col
	for gr.Next() {
		if gr.IsWordBoundary() {
			endCol = pos
			break
		}
		pos += len(gr.Str())
	}
	if endCol <= col {
		endCol = len(currentLine)
	}

	c.selectionStartCol = startCol
	c.selectionStartLine = line
	c.selectionEndCol = endCol
	c.selectionEndLine = line
	c.selectionActive = false
}

// SelectParagraph selects the paragraph/line at the given position
func (c *Chat) SelectParagraph(col, line int) {
	// Get the content from the viewport
	content := c.viewport.View()
	lines := strings.Split(content, "\n")

	if line < 0 || line >= len(lines) {
		return
	}

	// Find paragraph boundaries (search for empty lines)
	startLine := line
	endLine := line

	// Search backward for paragraph start
	for startLine > 0 {
		prevLine := ansi.Strip(lines[startLine-1])
		if strings.TrimSpace(prevLine) == "" {
			break
		}
		startLine--
	}

	// Search forward for paragraph end
	for endLine < len(lines)-1 {
		nextLine := ansi.Strip(lines[endLine+1])
		if strings.TrimSpace(nextLine) == "" {
			break
		}
		endLine++
	}

	// Get the width of the last line in the paragraph
	lastLineWidth := len(ansi.Strip(lines[endLine]))

	c.selectionStartCol = 0
	c.selectionStartLine = startLine
	c.selectionEndCol = lastLineWidth
	c.selectionEndLine = endLine
	c.selectionActive = false
}

// selectionArea returns the normalized selection area (start < end).
//
// Selection can happen in any direction - the user might drag from bottom-right
// to top-left. This function normalizes the coordinates so that (startCol, startLine)
// is always before (endCol, endLine) in reading order.
//
// The normalization handles two cases:
//  1. Multi-line backward selection: startLine > endLine - swap both lines and columns
//  2. Same-line backward selection: startLine == endLine && startCol > endCol - swap columns
//
// This ensures text extraction and rendering always process from start to end.
func (c *Chat) selectionArea() (startCol, startLine, endCol, endLine int) {
	startCol = c.selectionStartCol
	startLine = c.selectionStartLine
	endCol = c.selectionEndCol
	endLine = c.selectionEndLine

	// Normalize so start is before end in reading order (top-to-bottom, left-to-right)
	if startLine > endLine || (startLine == endLine && startCol > endCol) {
		startCol, endCol = endCol, startCol
		startLine, endLine = endLine, startLine
	}

	return
}

// GetSelectedText returns the currently selected text.
//
// The text extraction process:
//  1. Get the viewport's rendered content (which contains ANSI escape codes)
//  2. Split into lines
//  3. For each line in the selection range, strip ANSI codes before extracting substring
//  4. Join lines with newlines
//
// ANSI codes are stripped because selection coordinates correspond to visible character
// positions, not raw string positions. For example, a bold "Hello" might be stored as
// "\x1b[1mHello\x1b[0m" (15 bytes) but displays as 5 characters. When the user selects
// characters 0-5, they expect "Hello", not a partial escape sequence.
func (c *Chat) GetSelectedText() string {
	if !c.HasTextSelection() {
		return ""
	}

	content := c.viewport.View()
	lines := strings.Split(content, "\n")

	startCol, startLine, endCol, endLine := c.selectionArea()

	var result strings.Builder

	for y := startLine; y <= endLine && y < len(lines); y++ {
		line := ansi.Strip(lines[y])

		var lineStart, lineEnd int
		if y == startLine {
			lineStart = startCol
		} else {
			lineStart = 0
		}
		if y == endLine {
			lineEnd = endCol
		} else {
			lineEnd = len(line)
		}

		// Ensure bounds are valid
		if lineStart < 0 {
			lineStart = 0
		}
		if lineEnd > len(line) {
			lineEnd = len(line)
		}
		if lineStart > lineEnd {
			lineStart = lineEnd
		}

		if lineStart < len(line) {
			result.WriteString(line[lineStart:lineEnd])
		}
		if y < endLine {
			result.WriteString("\n")
		}
	}

	return strings.TrimSpace(result.String())
}

// CopySelectedText copies the selected text to the clipboard and starts flash animation
func (c *Chat) CopySelectedText() tea.Cmd {
	if !c.HasTextSelection() {
		return nil
	}

	selectedText := c.GetSelectedText()
	if selectedText == "" {
		return nil
	}

	// Start the selection flash animation
	c.selectionFlashFrame = 0

	return tea.Batch(
		// OSC 52 escape sequence (works in modern terminals)
		tea.SetClipboard(selectedText),
		// Native clipboard fallback - returns error message if it fails
		func() tea.Msg {
			if err := clipboard.WriteText(selectedText); err != nil {
				logger.Log("Failed to write to clipboard: %v", err)
				return ClipboardErrorMsg{Error: err}
			}
			return nil
		},
		// Start flash animation timer
		SelectionFlashTick(),
	)
}

// selectionView applies selection highlighting to the rendered view using ultraviolet
func (c *Chat) selectionView(view string) string {
	if !c.HasTextSelection() {
		return view
	}

	width := c.viewport.Width()
	height := c.viewport.Height()
	if width <= 0 || height <= 0 {
		return view
	}

	// Create screen buffer from the rendered view
	area := uv.Rect(0, 0, width, height)
	scr := uv.NewScreenBuffer(area.Dx(), area.Dy())
	uv.NewStyledString(view).Draw(scr, area)

	// Get normalized selection coordinates
	startCol, startLine, endCol, endLine := c.selectionArea()

	// Get selection style colors - use flash style during copy animation
	var selBg, selFg color.Color
	if c.selectionFlashFrame == 0 {
		// Flash frame - use bright green to indicate successful copy
		selBg = TextSelectionFlashStyle.GetBackground()
		selFg = TextSelectionFlashStyle.GetForeground()
	} else {
		// Normal selection
		selBg = TextSelectionStyle.GetBackground()
		selFg = TextSelectionStyle.GetForeground()
	}

	// Apply selection highlighting
	for y := startLine; y <= endLine && y < height; y++ {
		var xStart, xEnd int
		if y == startLine && y == endLine {
			// Single line selection
			xStart = startCol
			xEnd = endCol
		} else if y == startLine {
			// First line of multi-line selection
			xStart = startCol
			xEnd = width
		} else if y == endLine {
			// Last line of multi-line selection
			xStart = 0
			xEnd = endCol
		} else {
			// Middle lines
			xStart = 0
			xEnd = width
		}

		for x := xStart; x < xEnd && x < width; x++ {
			cell := scr.CellAt(x, y)
			if cell != nil {
				cell = cell.Clone()
				cell.Style.Bg = selBg
				cell.Style.Fg = selFg
				scr.SetCell(x, y, cell)
			}
		}
	}

	return scr.Render()
}
