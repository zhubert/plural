package ui

import (
	"regexp"
	"strings"
	"testing"
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
