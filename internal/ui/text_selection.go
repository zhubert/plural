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
//
// # Unicode and Multi-Byte Character Handling
//
// Selection coordinates represent VISUAL column positions, not byte offsets or rune indices.
// This distinction is critical for correctly handling Unicode text:
//
//   - Multi-byte characters (e.g., "é" = 2 bytes, "👋" = 4 bytes) may take 1 visual column
//   - Wide characters (e.g., CJK "世" = 3 bytes) take 2 visual columns each
//   - Combining characters and grapheme clusters must be treated as single units
//
// To handle this correctly, the text selection system uses two key utility functions:
//
//   - columnToByteOffset(): Converts visual column position → byte offset in string
//   - byteOffsetToColumn(): Converts byte offset in string → visual column position
//
// These functions use the uniseg library to iterate over grapheme clusters (user-perceived
// characters) and account for their visual width using uniseg.StringWidth().
//
// Example: For the string "Hello 👋 world" (where 👋 is 4 bytes but 2 visual columns):
//   - Column 0 = byte 0 ('H')
//   - Column 6 = byte 6 (start of 👋 emoji)
//   - Column 8 = byte 10 (space after emoji)
//
// When selecting text, mouse coordinates arrive as visual column positions. To extract
// the selected substring, we must:
//  1. Convert column positions to byte offsets (to know where to slice the string)
//  2. Extract the substring using byte-based string slicing
//
// Similarly, when finding word boundaries, uniseg operates on byte positions, so we
// convert the click column to a byte offset, find word boundaries in bytes, then
// convert back to visual columns for storage.
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
	c.selection.StartCol = col
	c.selection.StartLine = line
	c.selection.EndCol = col
	c.selection.EndLine = line
	c.selection.Active = true
}

// EndSelection updates the end position of the selection during drag
func (c *Chat) EndSelection(col, line int) {
	if !c.selection.Active {
		return
	}
	c.selection.EndCol = col
	c.selection.EndLine = line
}

// SelectionStop ends the drag but keeps the selection visible
func (c *Chat) SelectionStop() {
	c.selection.Active = false
}

// SelectionClear clears the selection entirely
func (c *Chat) SelectionClear() {
	c.selection.StartCol = -1
	c.selection.StartLine = -1
	c.selection.EndCol = -1
	c.selection.EndLine = -1
	c.selection.Active = false
}

// HasTextSelection returns true if there is an active or completed selection
func (c *Chat) HasTextSelection() bool {
	return c.selection.StartCol >= 0 && c.selection.StartLine >= 0 &&
		(c.selection.EndCol != c.selection.StartCol || c.selection.EndLine != c.selection.StartLine)
}

// handleMouseClick handles mouse click events and detects double/triple clicks
func (c *Chat) handleMouseClick(x, y int) tea.Cmd {
	now := time.Now()

	// Check if this is a potential multi-click
	if now.Sub(c.selection.LastClickTime) <= doubleClickThreshold &&
		abs(x-c.selection.LastClickX) <= clickTolerance &&
		abs(y-c.selection.LastClickY) <= clickTolerance {
		c.selection.ClickCount++
	} else {
		c.selection.ClickCount = 1
	}

	c.selection.LastClickTime = now
	c.selection.LastClickX = x
	c.selection.LastClickY = y

	switch c.selection.ClickCount {
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
		c.selection.ClickCount = 0 // Reset after triple click
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

// columnToByteOffset converts a visual column position to a byte offset in the string.
// This handles multi-byte characters and wide characters (e.g., CJK) correctly.
// Returns the byte offset, or len(s) if col is beyond the end of the string.
func columnToByteOffset(s string, col int) int {
	if col <= 0 {
		return 0
	}

	gr := uniseg.NewGraphemes(s)
	currentCol := 0
	byteOffset := 0

	for gr.Next() {
		grapheme := gr.Str()
		graphemeWidth := uniseg.StringWidth(grapheme)

		if currentCol+graphemeWidth > col {
			return byteOffset
		}

		currentCol += graphemeWidth
		byteOffset += len(grapheme)
	}

	return byteOffset
}

// byteOffsetToColumn converts a byte offset in a string to a visual column position.
// This handles multi-byte characters and wide characters (e.g., CJK) correctly.
// Returns the visual column position.
func byteOffsetToColumn(s string, offset int) int {
	if offset <= 0 {
		return 0
	}
	if offset >= len(s) {
		return uniseg.StringWidth(s)
	}

	gr := uniseg.NewGraphemes(s)
	currentCol := 0
	bytePos := 0

	for gr.Next() {
		grapheme := gr.Str()
		if bytePos >= offset {
			return currentCol
		}
		currentCol += uniseg.StringWidth(grapheme)
		bytePos += len(grapheme)
	}

	return currentCol
}

// SelectWord selects the word at the given position.
// The col parameter is a visual column position (0-based).
func (c *Chat) SelectWord(col, line int) {
	// Get the content from the viewport
	content := c.viewport.View()
	lines := strings.Split(content, "\n")

	if line < 0 || line >= len(lines) {
		return
	}

	currentLine := ansi.Strip(lines[line])
	lineWidth := uniseg.StringWidth(currentLine)
	if col < 0 || col >= lineWidth {
		return
	}

	// Convert column position to byte offset for grapheme iteration
	clickByteOffset := columnToByteOffset(currentLine, col)

	// Find all word boundaries in the line
	// A word boundary marks the END of a word (the last character of the word)
	type boundary struct {
		byteOffset int
		column     int
	}
	var boundaries []boundary

	gr := uniseg.NewGraphemes(currentLine)
	bytePos := 0
	currentCol := 0
	for gr.Next() {
		grapheme := gr.Str()
		graphemeWidth := uniseg.StringWidth(grapheme)

		if gr.IsWordBoundary() {
			// This grapheme is the end of a word
			boundaries = append(boundaries, boundary{
				byteOffset: bytePos + len(grapheme),
				column:     currentCol + graphemeWidth,
			})
		}

		bytePos += len(grapheme)
		currentCol += graphemeWidth
	}

	// Find the word containing the click position
	// Words are bounded by boundaries: [0, boundary[0]), [boundary[0], boundary[1]), ...
	startCol := 0
	endCol := lineWidth

	for _, b := range boundaries {
		if b.byteOffset > clickByteOffset {
			// The click is before this boundary, so it's in the previous word
			endCol = b.column
			break
		}
		// The click is after this boundary, so update the start for the next word
		startCol = b.column
	}

	c.selection.StartCol = startCol
	c.selection.StartLine = line
	c.selection.EndCol = endCol
	c.selection.EndLine = line
	c.selection.Active = false
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

	// Get the visual width of the last line in the paragraph
	lastLineWidth := uniseg.StringWidth(ansi.Strip(lines[endLine]))

	c.selection.StartCol = 0
	c.selection.StartLine = startLine
	c.selection.EndCol = lastLineWidth
	c.selection.EndLine = endLine
	c.selection.Active = false
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
	startCol = c.selection.StartCol
	startLine = c.selection.StartLine
	endCol = c.selection.EndCol
	endLine = c.selection.EndLine

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
//  4. Convert visual column positions to byte offsets for proper substring extraction
//  5. Join lines with newlines
//
// ANSI codes are stripped because selection coordinates correspond to visible character
// positions, not raw string positions. For example, a bold "Hello" might be stored as
// "\x1b[1mHello\x1b[0m" (15 bytes) but displays as 5 characters. When the user selects
// characters 0-5, they expect "Hello", not a partial escape sequence.
//
// Column-to-byte conversion is necessary because selection coordinates are visual column
// positions (accounting for wide characters like CJK), but string slicing requires byte
// offsets. Without this conversion, multi-byte characters cause incorrect text extraction.
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

		var lineStartCol, lineEndCol int
		if y == startLine {
			lineStartCol = startCol
		} else {
			lineStartCol = 0
		}
		if y == endLine {
			lineEndCol = endCol
		} else {
			lineEndCol = uniseg.StringWidth(line)
		}

		// Ensure column bounds are valid
		lineWidth := uniseg.StringWidth(line)
		if lineStartCol < 0 {
			lineStartCol = 0
		}
		if lineEndCol > lineWidth {
			lineEndCol = lineWidth
		}
		if lineStartCol > lineEndCol {
			lineStartCol = lineEndCol
		}

		// Convert visual column positions to byte offsets
		lineStartByte := columnToByteOffset(line, lineStartCol)
		lineEndByte := columnToByteOffset(line, lineEndCol)

		// Extract substring using byte offsets
		if lineStartByte < len(line) {
			result.WriteString(line[lineStartByte:lineEndByte])
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
	c.selection.FlashFrame = 0

	return tea.Batch(
		// OSC 52 escape sequence (works in modern terminals)
		tea.SetClipboard(selectedText),
		// Native clipboard fallback - returns error message if it fails
		func() tea.Msg {
			if err := clipboard.WriteText(selectedText); err != nil {
				logger.Get().Error("Failed to write to clipboard", "error", err)
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
	if c.selection.FlashFrame == 0 {
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
