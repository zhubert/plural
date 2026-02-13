package ui

import (
	"testing"
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
// Regression: negative EndLine causing index out of range panic
// =============================================================================

func TestGetSelectedText_NegativeEndLine_NoPanic(t *testing.T) {
	c := newTestChat()
	// Simulate: valid start position but negative end position
	// This can happen when dragging onto the panel border (mouse Y=0, adjusted to -1)
	c.selection.StartCol = 5
	c.selection.StartLine = 0
	c.selection.EndCol = 0
	c.selection.EndLine = -1

	// HasTextSelection returns true because StartCol >= 0 && StartLine >= 0
	// and (EndCol != StartCol || EndLine != StartLine)
	if !c.HasTextSelection() {
		t.Fatal("expected HasTextSelection=true for this edge case")
	}

	// This should not panic (previously caused: index out of range [-1])
	text := c.GetSelectedText()
	_ = text
}

func TestSelectionView_NegativeEndLine_NoPanic(t *testing.T) {
	c := newTestChat()
	c.selection.StartCol = 5
	c.selection.StartLine = 0
	c.selection.EndCol = 0
	c.selection.EndLine = -1

	// Should not panic when rendering selection with negative coordinates
	view := c.selectionView("hello\nworld\n")
	_ = view
}
