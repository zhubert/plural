package ui

import (
	"strings"
	"testing"

	"github.com/rivo/uniseg"
)

func newTestChat() *Chat {
	c := NewChat()
	c.SetSize(80, 24)
	return c
}

// =============================================================================
// StartSelection / EndSelection / SelectionStop / SelectionClear
// =============================================================================

func TestStartSelection(t *testing.T) {
	c := newTestChat()
	c.StartSelection(5, 10)

	if c.selection.StartCol != 5 || c.selection.StartLine != 10 {
		t.Errorf("start position wrong: got (%d, %d)", c.selection.StartCol, c.selection.StartLine)
	}
	if c.selection.EndCol != 5 || c.selection.EndLine != 10 {
		t.Errorf("end position should match start: got (%d, %d)", c.selection.EndCol, c.selection.EndLine)
	}
	if !c.selection.Active {
		t.Error("expected Active=true after StartSelection")
	}
}

func TestEndSelection(t *testing.T) {
	c := newTestChat()
	c.StartSelection(5, 10)
	c.EndSelection(20, 12)

	if c.selection.EndCol != 20 || c.selection.EndLine != 12 {
		t.Errorf("end position wrong: got (%d, %d)", c.selection.EndCol, c.selection.EndLine)
	}
	if !c.selection.Active {
		t.Error("expected Active=true during drag")
	}
}

func TestEndSelection_InactiveIsNoop(t *testing.T) {
	c := newTestChat()
	// Don't start selection
	c.EndSelection(20, 12)

	// Should remain at zero values
	if c.selection.EndCol != 0 || c.selection.EndLine != 0 {
		t.Errorf("expected no change when inactive, got (%d, %d)", c.selection.EndCol, c.selection.EndLine)
	}
}

func TestSelectionStop(t *testing.T) {
	c := newTestChat()
	c.StartSelection(5, 10)
	c.EndSelection(20, 12)
	c.SelectionStop()

	if c.selection.Active {
		t.Error("expected Active=false after SelectionStop")
	}
	// Positions should be preserved
	if c.selection.StartCol != 5 || c.selection.EndCol != 20 {
		t.Error("positions should be preserved after SelectionStop")
	}
}

func TestSelectionClear(t *testing.T) {
	c := newTestChat()
	c.StartSelection(5, 10)
	c.EndSelection(20, 12)
	c.SelectionClear()

	if c.selection.Active {
		t.Error("expected Active=false after SelectionClear")
	}
	if c.selection.StartCol != -1 || c.selection.StartLine != -1 {
		t.Error("start should be (-1, -1) after clear")
	}
	if c.selection.EndCol != -1 || c.selection.EndLine != -1 {
		t.Error("end should be (-1, -1) after clear")
	}
}

// =============================================================================
// HasTextSelection
// =============================================================================

func TestHasTextSelection(t *testing.T) {
	tests := []struct {
		name                               string
		startCol, startLine, endCol, endLine int
		want                               bool
	}{
		{"no selection (default)", -1, -1, -1, -1, false},
		{"same point", 5, 5, 5, 5, false},
		{"different column same line", 5, 5, 10, 5, true},
		{"different line", 5, 5, 5, 6, true},
		{"full range", 0, 0, 20, 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestChat()
			c.selection.StartCol = tt.startCol
			c.selection.StartLine = tt.startLine
			c.selection.EndCol = tt.endCol
			c.selection.EndLine = tt.endLine
			got := c.HasTextSelection()
			if got != tt.want {
				t.Errorf("HasTextSelection() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// selectionArea (normalization)
// =============================================================================

func TestSelectionArea_NormalizesForwardSelection(t *testing.T) {
	c := newTestChat()
	c.selection.StartCol = 5
	c.selection.StartLine = 2
	c.selection.EndCol = 15
	c.selection.EndLine = 4

	startCol, startLine, endCol, endLine := c.selectionArea()
	if startCol != 5 || startLine != 2 || endCol != 15 || endLine != 4 {
		t.Errorf("forward selection should be unchanged: got (%d,%d)-(%d,%d)",
			startCol, startLine, endCol, endLine)
	}
}

func TestSelectionArea_NormalizesBackwardSelection(t *testing.T) {
	c := newTestChat()
	// Drag from bottom to top
	c.selection.StartCol = 15
	c.selection.StartLine = 4
	c.selection.EndCol = 5
	c.selection.EndLine = 2

	startCol, startLine, endCol, endLine := c.selectionArea()
	if startCol != 5 || startLine != 2 || endCol != 15 || endLine != 4 {
		t.Errorf("backward selection should be normalized: got (%d,%d)-(%d,%d)",
			startCol, startLine, endCol, endLine)
	}
}

func TestSelectionArea_NormalizesSameLineBackward(t *testing.T) {
	c := newTestChat()
	c.selection.StartCol = 20
	c.selection.StartLine = 5
	c.selection.EndCol = 3
	c.selection.EndLine = 5

	startCol, startLine, endCol, endLine := c.selectionArea()
	if startCol != 3 || endCol != 20 || startLine != 5 || endLine != 5 {
		t.Errorf("same-line backward should swap columns: got (%d,%d)-(%d,%d)",
			startCol, startLine, endCol, endLine)
	}
}

// =============================================================================
// GetSelectedText
// =============================================================================

func TestGetSelectedText_NoSelection(t *testing.T) {
	c := newTestChat()
	text := c.GetSelectedText()
	if text != "" {
		t.Errorf("expected empty string, got %q", text)
	}
}

// =============================================================================
// handleMouseClick (click counting)
// =============================================================================

func TestHandleMouseClick_SingleClick(t *testing.T) {
	c := newTestChat()
	c.handleMouseClick(5, 3)

	if c.selection.ClickCount != 1 {
		t.Errorf("expected ClickCount=1, got %d", c.selection.ClickCount)
	}
	if !c.selection.Active {
		t.Error("expected Active=true after single click")
	}
}

func TestHandleMouseClick_ResetOnDistantClick(t *testing.T) {
	c := newTestChat()
	c.handleMouseClick(5, 3)

	// Click far away - should reset count
	c.handleMouseClick(50, 20)

	if c.selection.ClickCount != 1 {
		t.Errorf("expected ClickCount=1 after distant click, got %d", c.selection.ClickCount)
	}
}

// =============================================================================
// abs helper
// =============================================================================

func TestAbsHelper(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0, 0},
		{5, 5},
		{-5, 5},
		{-1, 1},
		{1, 1},
	}

	for _, tt := range tests {
		got := abs(tt.input)
		if got != tt.want {
			t.Errorf("abs(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// =============================================================================
// SelectWord edge cases
// =============================================================================

func TestSelectWord_OutOfBounds(t *testing.T) {
	c := newTestChat()
	// Selecting word at negative coords should be a no-op
	c.SelectWord(-1, -1)
	if c.selection.Active {
		t.Error("expected no selection on out-of-bounds")
	}
}

// =============================================================================
// SelectParagraph edge cases
// =============================================================================

func TestSelectParagraph_OutOfBounds(t *testing.T) {
	c := newTestChat()
	c.SelectParagraph(0, -1)
	// Should be a no-op for out of bounds line
	if c.selection.Active {
		t.Error("expected no selection on out-of-bounds")
	}
}

// =============================================================================
// CopySelectedText with no selection
// =============================================================================

func TestCopySelectedText_NoSelection(t *testing.T) {
	c := newTestChat()
	cmd := c.CopySelectedText()
	if cmd != nil {
		t.Error("expected nil cmd when no selection")
	}
}

// =============================================================================
// Unicode handling tests
// =============================================================================

func TestSelectWord_UnicodeEmoji(t *testing.T) {
	c := newTestChat()

	// Test with emoji (4 bytes, 2 visual columns wide)
	testLine := "Hello 👋 world"
	c.viewport.SetContent(testLine)

	// Click on the emoji (column 6)
	c.SelectWord(6, 0)

	selected := c.GetSelectedText()
	if selected != "👋" {
		t.Errorf("expected emoji to be selected, got %q", selected)
	}
}

func TestSelectWord_UnicodeAccents(t *testing.T) {
	c := newTestChat()

	// Test with accented characters
	testLine := "Héllo wörld"
	c.viewport.SetContent(testLine)

	// Click on the second word (column 6)
	c.SelectWord(6, 0)

	selected := c.GetSelectedText()
	if selected != "wörld" {
		t.Errorf("expected 'wörld', got %q", selected)
	}
}

func TestSelectWord_UnicodeCJK(t *testing.T) {
	c := newTestChat()

	// Test with CJK characters (wide characters - 2 columns each)
	// Note: Unicode word boundaries treat each CJK character as a separate word
	// because CJK languages don't use spaces between words. This is correct per UAX#29.
	testLine := "Hello 世界 world"
	c.viewport.SetContent(testLine)

	// Click on the first CJK character (column 6)
	c.SelectWord(6, 0)

	selected := c.GetSelectedText()
	// Each CJK character is treated as a separate word per Unicode spec
	if selected != "世" {
		t.Errorf("expected '世', got %q", selected)
	}

	// Click on the second CJK character (column 8)
	c.SelectWord(8, 0)

	selected = c.GetSelectedText()
	if selected != "界" {
		t.Errorf("expected '界', got %q", selected)
	}
}

func TestGetSelectedText_UnicodeMultiByteChars(t *testing.T) {
	c := newTestChat()

	// Text with various multi-byte characters
	testLine := "Café ☕ is nice"
	c.viewport.SetContent(testLine)

	// Select "Café" (columns 0-4)
	c.selection.StartCol = 0
	c.selection.StartLine = 0
	c.selection.EndCol = 4
	c.selection.EndLine = 0

	selected := c.GetSelectedText()
	if selected != "Café" {
		t.Errorf("expected 'Café', got %q", selected)
	}
}

func TestGetSelectedText_UnicodeWideChars(t *testing.T) {
	c := newTestChat()

	// Text with wide characters (each takes 2 visual columns)
	testLine := "日本語"
	c.viewport.SetContent(testLine)

	// Wide chars: 日(0-1), 本(2-3), 語(4-5)
	// Select "本" (columns 2-4)
	c.selection.StartCol = 2
	c.selection.StartLine = 0
	c.selection.EndCol = 4
	c.selection.EndLine = 0

	selected := c.GetSelectedText()
	if selected != "本" {
		t.Errorf("expected '本', got %q", selected)
	}
}

// =============================================================================
// Utility function tests (columnToByteOffset, byteOffsetToColumn)
// =============================================================================

func TestColumnToByteOffset(t *testing.T) {
	tests := []struct {
		name   string
		str    string
		col    int
		want   int
		wantAt string // character at the returned offset
	}{
		{"empty string", "", 0, 0, ""},
		{"ascii start", "Hello", 0, 0, "H"},
		{"ascii middle", "Hello", 3, 3, "l"},
		{"ascii end", "Hello", 5, 5, ""},
		{"ascii beyond end", "Hello", 10, 5, ""},
		{"emoji start", "👋Hello", 0, 0, "👋"},
		{"emoji after", "👋Hello", 2, 4, "H"},
		{"wide char start", "世界", 0, 0, "世"},
		{"wide char middle", "世界", 2, 3, "界"},
		{"wide char end", "世界", 4, 6, ""},
		{"mixed unicode", "Café☕", 4, 5, "☕"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := columnToByteOffset(tt.str, tt.col)
			if got != tt.want {
				t.Errorf("columnToByteOffset(%q, %d) = %d, want %d", tt.str, tt.col, got, tt.want)
			}
			if tt.wantAt != "" && got < len(tt.str) {
				// Verify we're at the correct character
				rest := tt.str[got:]
				if !strings.HasPrefix(rest, tt.wantAt) {
					t.Errorf("at offset %d, got %q, want %q", got, string(rest[0]), tt.wantAt)
				}
			}
		})
	}
}

func TestByteOffsetToColumn(t *testing.T) {
	tests := []struct {
		name   string
		str    string
		offset int
		want   int
	}{
		{"empty string", "", 0, 0},
		{"ascii start", "Hello", 0, 0},
		{"ascii middle", "Hello", 3, 3},
		{"ascii end", "Hello", 5, 5},
		{"ascii beyond end", "Hello", 10, 5},
		{"emoji start", "👋Hello", 0, 0},
		{"emoji middle", "👋Hello", 2, 2},
		{"emoji after", "👋Hello", 4, 2},
		{"wide char start", "世界", 0, 0},
		{"wide char after first", "世界", 3, 2},
		{"wide char end", "世界", 6, 4},
		{"mixed unicode", "Café☕", 5, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := byteOffsetToColumn(tt.str, tt.offset)
			if got != tt.want {
				t.Errorf("byteOffsetToColumn(%q, %d) = %d, want %d", tt.str, tt.offset, got, tt.want)
			}
		})
	}
}

func TestColumnByteRoundTrip(t *testing.T) {
	// Note: Round-tripping is only guaranteed for ASCII and narrow characters.
	// Wide characters (emoji, CJK) can span multiple columns, so column positions
	// that fall "inside" a wide character will snap to the character's start position.
	// For example, if an emoji at byte 6 spans columns 6-7, asking for column 7
	// will return byte 6 (the emoji's start), and converting back gives column 6.
	//
	// This test verifies round-tripping works for ASCII where each byte = 1 column.

	tests := []struct {
		name string
		str  string
	}{
		{"ascii", "Hello world"},
		{"accents", "Café crème"}, // Multi-byte but single-width chars
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For each column position, convert to byte offset and back
			width := uniseg.StringWidth(tt.str)
			for col := 0; col <= width; col++ {
				byteOff := columnToByteOffset(tt.str, col)
				colBack := byteOffsetToColumn(tt.str, byteOff)
				if colBack != col {
					t.Errorf("round trip failed at col %d: col->byte->col = %d->%d->%d",
						col, col, byteOff, colBack)
				}
			}
		})
	}
}
