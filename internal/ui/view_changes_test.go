package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zhubert/plural/internal/git"
)

// TestDiffViewerNavigationBarWidthCalculation verifies that the diff viewer
// navigation bar correctly calculates available width for filename display
// by using lipgloss.Width() instead of len() for styled strings.
func TestDiffViewerNavigationBarWidthCalculation(t *testing.T) {
	chat := NewChat()
	chat.SetSize(100, 40)

	// Create test diff files
	files := []git.FileDiff{
		{Filename: "very_long_filename_that_should_be_truncated_properly.go", Status: "M", Diff: "test diff"},
		{Filename: "short.go", Status: "A", Diff: "test diff"},
	}

	chat.EnterViewChangesMode(files)

	// Render the view
	view := chat.View()

	// Extract the navigation bar (should be near the top of the view)
	lines := strings.Split(view, "\n")

	// Find the navigation bar line (contains arrows and filename)
	var navBar string
	for _, line := range lines {
		stripped := ansi.Strip(line)
		if strings.Contains(stripped, "of 2") && (strings.Contains(stripped, "←") || strings.Contains(stripped, "→")) {
			navBar = line
			break
		}
	}

	if navBar == "" {
		t.Fatal("Could not find navigation bar in rendered view")
	}

	// The navigation bar should fit within terminal width
	// The box borders (│) are part of the rendered output, so the content line
	// should be exactly 100 chars (the terminal width)
	visibleNavBar := ansi.Strip(navBar)
	visibleWidth := lipgloss.Width(visibleNavBar)

	// The entire line including borders should be exactly the terminal width
	if visibleWidth != 100 {
		t.Errorf("Navigation bar line width should be exactly 100, got %d\nNav bar: %q",
			visibleWidth, visibleNavBar)
	}

	// Verify the filename is visible
	if !strings.Contains(visibleNavBar, "very_long_filename") {
		t.Errorf("Navigation bar should contain part of the filename, got: %q", visibleNavBar)
	}
}

// TestDiffViewerWidthCalculationWithStyledStrings is a more focused unit test
// that verifies the width calculation logic specifically for styled strings.
func TestDiffViewerWidthCalculationWithStyledStrings(t *testing.T) {
	// Create a styled string with ANSI codes
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true)
	styledText := style.Render("(1 of 3)")

	// The raw string length includes ANSI escape codes
	rawLen := len(styledText)

	// The visual width should be much smaller
	visualWidth := lipgloss.Width(styledText)

	// Visual width should be just the text content
	if visualWidth != 8 { // "(1 of 3)" is 8 characters
		t.Errorf("Expected visual width 8, got %d", visualWidth)
	}

	// Raw length should be larger due to ANSI codes
	if rawLen <= visualWidth {
		t.Errorf("Raw length (%d) should be greater than visual width (%d) due to ANSI codes", rawLen, visualWidth)
	}

	// This demonstrates the bug: using len() instead of lipgloss.Width()
	// would over-estimate the fixed width, causing filenames to be over-truncated
	if rawLen > 20 {
		// Typical ANSI codes add significant bytes
		t.Logf("ANSI codes added %d bytes to an 8-character string", rawLen-visualWidth)
	}
}

// TestLogViewerNavigationBarWidthCalculation verifies that the log viewer
// navigation bar correctly calculates available width for filename display.
func TestLogViewerNavigationBarWidthCalculation(t *testing.T) {
	chat := NewChat()
	chat.SetSize(100, 40)

	// Create test log files
	files := []LogFile{
		{Name: "very_long_log_filename_that_should_be_truncated_properly.log", Path: "/tmp/test1.log", Content: "log content"},
		{Name: "short.log", Path: "/tmp/test2.log", Content: "log content"},
	}

	chat.EnterLogViewerMode(files)

	// Render the view
	view := chat.View()

	// Extract the navigation bar
	lines := strings.Split(view, "\n")

	var navBar string
	for _, line := range lines {
		stripped := ansi.Strip(line)
		if strings.Contains(stripped, "of 2") && (strings.Contains(stripped, "←") || strings.Contains(stripped, "→")) {
			navBar = line
			break
		}
	}

	if navBar == "" {
		t.Fatal("Could not find navigation bar in rendered view")
	}

	// The navigation bar should fit within terminal width
	visibleNavBar := ansi.Strip(navBar)
	visibleWidth := lipgloss.Width(visibleNavBar)

	// The entire line including borders should be exactly the terminal width
	if visibleWidth != 100 {
		t.Errorf("Log viewer navigation bar line width should be exactly 100, got %d\nNav bar: %q",
			visibleWidth, visibleNavBar)
	}

	// Verify the filename is visible
	if !strings.Contains(visibleNavBar, "very_long_log") {
		t.Errorf("Navigation bar should contain part of the filename, got: %q", visibleNavBar)
	}
}

// TestStyledCounterWidth demonstrates the ANSI width bug with a minimal example.
func TestStyledCounterWidth(t *testing.T) {
	// Simulate the counter styling from the actual code
	counterStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	counter := counterStyle.Render("(3 of 7)")

	// Bug: using len(counter) includes ANSI escape codes
	buggyWidth := len(counter)

	// Fix: using lipgloss.Width() gets visible width only
	correctWidth := lipgloss.Width(counter)

	t.Logf("Styled counter: %q", counter)
	t.Logf("Buggy width (len): %d", buggyWidth)
	t.Logf("Correct width (lipgloss.Width): %d", correctWidth)

	// The correct width should be 8: "(3 of 7)"
	if correctWidth != 8 {
		t.Errorf("Expected correct width 8, got %d", correctWidth)
	}

	// The buggy width should be larger due to ANSI codes
	if buggyWidth <= correctWidth {
		t.Errorf("Buggy width (%d) should be larger than correct width (%d)", buggyWidth, correctWidth)
	}

	// If we use buggyWidth in fixedWidth calculation, we over-estimate
	// the space needed, causing maxFilenameWidth to be too small
	terminalWidth := 80
	leftArrow := 2
	rightArrow := 2
	statusAndSpaces := 7 // "[M] " + spaces

	// Buggy calculation (old code)
	buggyFixedWidth := leftArrow + statusAndSpaces + buggyWidth + rightArrow
	buggyMaxFilename := terminalWidth - buggyFixedWidth

	// Correct calculation (new code)
	correctFixedWidth := leftArrow + statusAndSpaces + correctWidth + rightArrow
	correctMaxFilename := terminalWidth - correctFixedWidth

	t.Logf("Buggy max filename width: %d", buggyMaxFilename)
	t.Logf("Correct max filename width: %d", correctMaxFilename)

	// The difference is how many extra characters we can show in the filename
	extraChars := correctMaxFilename - buggyMaxFilename
	t.Logf("Filenames are over-truncated by %d characters with the bug", extraChars)

	if extraChars <= 0 {
		t.Errorf("Expected correct calculation to allow more filename characters, got %d extra", extraChars)
	}
}
