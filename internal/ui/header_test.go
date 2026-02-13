package ui

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// stripANSI removes ANSI escape codes from a string for testing
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

func TestNewHeader(t *testing.T) {
	header := NewHeader()

	if header == nil {
		t.Fatal("NewHeader() returned nil")
	}

	if header.sessionName != "" {
		t.Error("Expected empty session name initially")
	}

	if header.baseBranch != "" {
		t.Error("Expected empty base branch initially")
	}
}

func TestHeader_SetWidth(t *testing.T) {
	header := NewHeader()

	header.SetWidth(120)

	if header.width != 120 {
		t.Errorf("Expected width 120, got %d", header.width)
	}
}

func TestHeader_SetSessionName(t *testing.T) {
	header := NewHeader()

	header.SetSessionName("test-session")

	if header.sessionName != "test-session" {
		t.Errorf("Expected session name 'test-session', got %q", header.sessionName)
	}
}

func TestHeader_SetBaseBranch(t *testing.T) {
	header := NewHeader()

	header.SetBaseBranch("main")

	if header.baseBranch != "main" {
		t.Errorf("Expected base branch 'main', got %q", header.baseBranch)
	}
}

func TestHeader_View_NoSession(t *testing.T) {
	header := NewHeader()
	header.SetWidth(80)

	view := stripANSI(header.View())

	if !strings.Contains(view, "plural") {
		t.Errorf("Header should contain 'plural' title, got: %q", view)
	}
}

func TestHeader_View_WithSession(t *testing.T) {
	header := NewHeader()
	header.SetWidth(120)
	header.SetSessionName("feature-branch")

	view := stripANSI(header.View())

	if !strings.Contains(view, "plural") {
		t.Error("Header should contain 'plural' title")
	}

	if !strings.Contains(view, "feature-branch") {
		t.Errorf("Header should contain session name, got: %q", view)
	}
}

func TestHeader_View_WithBaseBranch(t *testing.T) {
	header := NewHeader()
	header.SetWidth(120)
	header.SetSessionName("feature-branch")
	header.SetBaseBranch("main")

	view := stripANSI(header.View())

	if !strings.Contains(view, "feature-branch") {
		t.Error("Header should contain session name")
	}

	if !strings.Contains(view, "(main)") {
		t.Errorf("Header should contain base branch indicator, got: %q", view)
	}
}

func TestHeader_View_WithBaseBranch_Muted(t *testing.T) {
	header := NewHeader()
	header.SetWidth(120)
	header.SetSessionName("feature-branch")
	header.SetBaseBranch("main")

	view := stripANSI(header.View())

	// The base branch portion should be present
	// We can't easily test the muted styling directly, but we can verify
	// the text content is there
	if !strings.Contains(view, "(main)") {
		t.Error("Header should contain base branch in parentheses")
	}
}

func TestHeader_View_NoBaseBranch(t *testing.T) {
	header := NewHeader()
	header.SetWidth(120)
	header.SetSessionName("feature-branch")
	// Don't set base branch

	view := stripANSI(header.View())

	if strings.Contains(view, "(from") {
		t.Error("Header should not contain base branch indicator when not set")
	}
}

func TestHeader_ClearBaseBranch(t *testing.T) {
	header := NewHeader()
	header.SetWidth(120)
	header.SetSessionName("feature-branch")
	header.SetBaseBranch("main")

	// Clear the base branch
	header.SetBaseBranch("")

	view := stripANSI(header.View())

	if strings.Contains(view, "(from") {
		t.Error("Header should not show base branch after clearing")
	}
}

func TestHeader_SetDiffStats(t *testing.T) {
	header := NewHeader()
	header.SetDiffStats(&DiffStats{
		FilesChanged: 3,
		Additions:    157,
		Deletions:    42,
	})

	if header.diffStats == nil {
		t.Fatal("Expected diffStats to be set")
	}

	if header.diffStats.FilesChanged != 3 {
		t.Errorf("Expected FilesChanged 3, got %d", header.diffStats.FilesChanged)
	}

	if header.diffStats.Additions != 157 {
		t.Errorf("Expected Additions 157, got %d", header.diffStats.Additions)
	}

	if header.diffStats.Deletions != 42 {
		t.Errorf("Expected Deletions 42, got %d", header.diffStats.Deletions)
	}
}

func TestHeader_SetDiffStats_Nil(t *testing.T) {
	header := NewHeader()
	header.SetDiffStats(&DiffStats{FilesChanged: 3})
	header.SetDiffStats(nil)

	if header.diffStats != nil {
		t.Error("Expected diffStats to be nil after clearing")
	}
}

func TestHeader_View_WithDiffStats(t *testing.T) {
	header := NewHeader()
	header.SetWidth(120)
	header.SetSessionName("feature-branch")
	header.SetDiffStats(&DiffStats{
		FilesChanged: 3,
		Additions:    157,
		Deletions:    5,
	})

	view := stripANSI(header.View())

	if !strings.Contains(view, "3 files") {
		t.Errorf("Header should contain file count, got: %q", view)
	}

	if !strings.Contains(view, "+157") {
		t.Errorf("Header should contain additions, got: %q", view)
	}

	if !strings.Contains(view, "-5") {
		t.Errorf("Header should contain deletions, got: %q", view)
	}
}

func TestHeader_View_WithDiffStats_SingleFile(t *testing.T) {
	header := NewHeader()
	header.SetWidth(120)
	header.SetSessionName("feature-branch")
	header.SetDiffStats(&DiffStats{
		FilesChanged: 1,
		Additions:    10,
		Deletions:    2,
	})

	view := stripANSI(header.View())

	// Should use singular "file" not "files"
	if !strings.Contains(view, "1 file,") {
		t.Errorf("Header should contain singular 'file', got: %q", view)
	}
}

func TestHeader_View_NoDiffStats_NoChanges(t *testing.T) {
	header := NewHeader()
	header.SetWidth(120)
	header.SetSessionName("feature-branch")
	header.SetDiffStats(&DiffStats{
		FilesChanged: 0,
		Additions:    0,
		Deletions:    0,
	})

	view := stripANSI(header.View())

	// Should not show diff stats when no changes
	if strings.Contains(view, "0 file") {
		t.Errorf("Header should not show diff stats with zero changes, got: %q", view)
	}
}

func TestHeader_View_WithDiffStatsAndBaseBranch(t *testing.T) {
	header := NewHeader()
	header.SetWidth(150)
	header.SetSessionName("feature-branch")
	header.SetBaseBranch("main")
	header.SetDiffStats(&DiffStats{
		FilesChanged: 2,
		Additions:    50,
		Deletions:    10,
	})

	view := stripANSI(header.View())

	// All elements should be present
	if !strings.Contains(view, "2 files") {
		t.Error("Header should contain file count")
	}

	if !strings.Contains(view, "+50") {
		t.Error("Header should contain additions")
	}

	if !strings.Contains(view, "-10") {
		t.Error("Header should contain deletions")
	}

	if !strings.Contains(view, "feature-branch") {
		t.Error("Header should contain session name")
	}

	if !strings.Contains(view, "(main)") {
		t.Error("Header should contain base branch")
	}
}

func TestHeader_View_UnicodeSessionName(t *testing.T) {
	header := NewHeader()
	header.SetWidth(80)
	// Session name with multi-byte Unicode characters (Japanese: "test")
	header.SetSessionName("テスト")

	view := stripANSI(header.View())

	if !strings.Contains(view, "plural") {
		t.Error("Header should contain 'plural' title")
	}

	if !strings.Contains(view, "テスト") {
		t.Errorf("Header should contain Unicode session name, got: %q", view)
	}

	// The rendered width in runes should match the header width
	runeCount := utf8.RuneCountInString(view)
	if runeCount != 80 {
		t.Errorf("Header rune width should be 80, got %d", runeCount)
	}
}

func TestHeader_View_UnicodeSessionName_WithBaseBranch(t *testing.T) {
	header := NewHeader()
	header.SetWidth(80)
	header.SetSessionName("功能分支")
	header.SetBaseBranch("main")

	view := stripANSI(header.View())

	if !strings.Contains(view, "功能分支") {
		t.Errorf("Header should contain Unicode session name, got: %q", view)
	}

	if !strings.Contains(view, "(main)") {
		t.Errorf("Header should contain base branch, got: %q", view)
	}

	runeCount := utf8.RuneCountInString(view)
	if runeCount != 80 {
		t.Errorf("Header rune width should be 80, got %d", runeCount)
	}
}

func TestHeader_View_UnicodeWithDiffStats(t *testing.T) {
	header := NewHeader()
	header.SetWidth(120)
	header.SetSessionName("ブランチ名")
	header.SetDiffStats(&DiffStats{
		FilesChanged: 2,
		Additions:    30,
		Deletions:    10,
	})

	view := stripANSI(header.View())

	if !strings.Contains(view, "ブランチ名") {
		t.Errorf("Header should contain Unicode session name, got: %q", view)
	}

	if !strings.Contains(view, "+30") {
		t.Errorf("Header should contain additions, got: %q", view)
	}

	if !strings.Contains(view, "-10") {
		t.Errorf("Header should contain deletions, got: %q", view)
	}

	runeCount := utf8.RuneCountInString(view)
	if runeCount != 120 {
		t.Errorf("Header rune width should be 120, got %d", runeCount)
	}
}

func TestHeader_View_MixedASCIIAndUnicode(t *testing.T) {
	header := NewHeader()
	header.SetWidth(100)
	// Mix of ASCII and multi-byte characters
	header.SetSessionName("feature-café-résumé")

	view := stripANSI(header.View())

	if !strings.Contains(view, "feature-café-résumé") {
		t.Errorf("Header should contain mixed session name, got: %q", view)
	}

	runeCount := utf8.RuneCountInString(view)
	if runeCount != 100 {
		t.Errorf("Header rune width should be 100, got %d", runeCount)
	}
}
