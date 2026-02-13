package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/zhubert/plural/internal/git"
)

func TestRenderFileNavBar_UsesVisualWidth(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 40)

	longFilename := "internal/very/long/path/to/some/deeply/nested/file_with_long_name.go"

	chat.EnterViewChangesMode([]git.FileDiff{
		{Filename: longFilename, Status: "M", Diff: "test diff"},
		{Filename: "other.go", Status: "A", Diff: "other diff"},
	})

	result := chat.renderFileNavBar(80)
	visibleWidth := lipgloss.Width(result)

	// The rendered nav bar's visible width should not exceed the requested width
	if visibleWidth > 80 {
		t.Errorf("renderFileNavBar visible width %d exceeds requested width 80", visibleWidth)
	}
}

func TestRenderFileNavBar_NoFiles(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 40)

	// No viewChanges state at all
	result := chat.renderFileNavBar(80)
	if result == "" {
		t.Error("renderFileNavBar should return something even with no files")
	}
}

func TestRenderFileNavBar_FilenameNotOverTruncated(t *testing.T) {
	chat := NewChat()
	chat.SetSize(120, 40)

	// Use a filename that should fit at width 120 without truncation
	filename := "src/main.go"

	chat.EnterViewChangesMode([]git.FileDiff{
		{Filename: filename, Status: "M", Diff: "diff"},
	})

	result := chat.renderFileNavBar(120)

	// The filename should appear in the output without truncation (no ellipsis)
	stripped := stripANSI(result)
	if !strings.Contains(stripped, filename) {
		t.Errorf("renderFileNavBar at width 120 should contain full filename %q, got: %q", filename, stripped)
	}
}
