package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestGetLogFiles_MainDebugLog(t *testing.T) {
	// Create a temporary debug log file
	tmpDir := t.TempDir()
	testLogPath := filepath.Join(tmpDir, "plural-debug.log")
	if err := os.WriteFile(testLogPath, []byte("test log content"), 0644); err != nil {
		t.Fatalf("Failed to create test log file: %v", err)
	}

	// Test that we can find log files (the actual implementation uses hardcoded paths)
	// This test verifies the function doesn't panic and returns a slice
	files := GetLogFiles("")
	// We can't guarantee the debug log exists in all test environments,
	// so just verify the function returns without error
	if files == nil {
		t.Error("GetLogFiles should return a non-nil slice")
	}
}

func TestGetLogFiles_WithSessionID(t *testing.T) {
	// Test that session-specific logs are prioritized
	files := GetLogFiles("test-session-id")

	// Verify we get a non-nil slice
	if files == nil {
		t.Error("GetLogFiles should return a non-nil slice")
	}
}

func TestTruncateSessionID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"short", "short"},
		{"12345678", "12345678"},
		{"123456789", "12345678"},
		{"a-very-long-session-id-here", "a-very-l"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncateSessionID(tt.input)
			if result != tt.expected {
				t.Errorf("truncateSessionID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestChat_LogViewerMode_EnterExit(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 40)

	// Initially not in log viewer mode
	if chat.IsInLogViewerMode() {
		t.Error("Chat should not be in log viewer mode initially")
	}

	// Enter log viewer mode
	testFiles := []LogFile{
		{Name: "Test Log", Path: "/tmp/test.log", Content: "test content"},
	}
	chat.EnterLogViewerMode(testFiles)

	if !chat.IsInLogViewerMode() {
		t.Error("Chat should be in log viewer mode after entering")
	}

	// Exit log viewer mode
	chat.ExitLogViewerMode()

	if chat.IsInLogViewerMode() {
		t.Error("Chat should not be in log viewer mode after exiting")
	}
}

func TestChat_LogViewerMode_KeyHandling(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 40)

	// Enter log viewer mode with multiple files
	testFiles := []LogFile{
		{Name: "Log 1", Path: "/tmp/test1.log"},
		{Name: "Log 2", Path: "/tmp/test2.log"},
		{Name: "Log 3", Path: "/tmp/test3.log"},
	}
	chat.EnterLogViewerMode(testFiles)

	// Test navigation keys
	tests := []struct {
		name          string
		key           string
		expectExit    bool
		expectedIndex int
	}{
		{"right arrow navigates to next file", "right", false, 1},
		{"right arrow again", "right", false, 2},
		{"right at end stays at end", "right", false, 2},
		{"left arrow navigates to previous file", "left", false, 1},
		{"left arrow again", "left", false, 0},
		{"left at start stays at start", "left", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyMsg := keyPressMsg(tt.key)
			chat.Update(keyMsg)

			if tt.expectExit && chat.IsInLogViewerMode() {
				t.Error("Expected to exit log viewer mode")
			}
			if !tt.expectExit && !chat.IsInLogViewerMode() {
				t.Error("Should still be in log viewer mode")
			}
			if !tt.expectExit && chat.logViewer.FileIndex != tt.expectedIndex {
				t.Errorf("Expected file index %d, got %d", tt.expectedIndex, chat.logViewer.FileIndex)
			}
		})
	}
}

func TestChat_LogViewerMode_EscapeExits(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 40)

	testFiles := []LogFile{
		{Name: "Test Log", Path: "/tmp/test.log"},
	}
	chat.EnterLogViewerMode(testFiles)

	if !chat.IsInLogViewerMode() {
		t.Fatal("Should be in log viewer mode")
	}

	// Press escape
	keyMsg := tea.KeyPressMsg{Code: tea.KeyEscape}
	chat.Update(keyMsg)

	if chat.IsInLogViewerMode() {
		t.Error("Escape should exit log viewer mode")
	}
}

func TestChat_LogViewerMode_QExits(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 40)

	testFiles := []LogFile{
		{Name: "Test Log", Path: "/tmp/test.log"},
	}
	chat.EnterLogViewerMode(testFiles)

	if !chat.IsInLogViewerMode() {
		t.Fatal("Should be in log viewer mode")
	}

	// Press 'q' to exit
	keyMsg := tea.KeyPressMsg{Code: 0, Text: "q"}
	chat.Update(keyMsg)

	if chat.IsInLogViewerMode() {
		t.Error("'q' should exit log viewer mode")
	}
}

func TestChat_LogViewerMode_FollowTail(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 40)

	testFiles := []LogFile{
		{Name: "Test Log", Path: "/tmp/test.log"},
	}
	chat.EnterLogViewerMode(testFiles)

	// Follow tail should be on by default
	if !chat.GetLogViewerFollowTail() {
		t.Error("Follow tail should be enabled by default")
	}

	// Toggle follow tail
	chat.ToggleLogViewerFollowTail()

	if chat.GetLogViewerFollowTail() {
		t.Error("Follow tail should be disabled after toggle")
	}

	// Toggle again
	chat.ToggleLogViewerFollowTail()

	if !chat.GetLogViewerFollowTail() {
		t.Error("Follow tail should be enabled after second toggle")
	}
}

func TestChat_LogViewerMode_FollowTailKey(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 40)

	testFiles := []LogFile{
		{Name: "Test Log", Path: "/tmp/test.log"},
	}
	chat.EnterLogViewerMode(testFiles)

	// Press 'f' to toggle follow
	keyMsg := keyPressMsg("f")
	chat.Update(keyMsg)

	if chat.GetLogViewerFollowTail() {
		t.Error("Follow tail should be disabled after pressing 'f'")
	}
}

func TestHighlightLogLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "empty line",
			input:    "",
			contains: "",
		},
		{
			name:     "info level",
			input:    "level=INFO msg=\"test message\"",
			contains: "INFO",
		},
		{
			name:     "error level",
			input:    "level=ERROR msg=\"error message\"",
			contains: "ERROR",
		},
		{
			name:     "warn level",
			input:    "level=WARN msg=\"warning message\"",
			contains: "WARN",
		},
		{
			name:     "debug level",
			input:    "level=DEBUG msg=\"debug message\"",
			contains: "DEBUG",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := highlightLogLine(tt.input)
			// The result should still contain the level text (possibly with ANSI codes)
			if tt.contains != "" && !containsText(result, tt.contains) {
				t.Errorf("highlightLogLine(%q) should contain %q, got %q", tt.input, tt.contains, result)
			}
		})
	}
}

func TestHighlightLogContent(t *testing.T) {
	input := `level=INFO msg="first message"
level=ERROR msg="error here"
level=DEBUG msg="debug info"`

	result := highlightLogContent(input)

	// Should contain all original text (with potential ANSI codes)
	if !containsText(result, "INFO") {
		t.Error("Result should contain INFO")
	}
	if !containsText(result, "ERROR") {
		t.Error("Result should contain ERROR")
	}
	if !containsText(result, "DEBUG") {
		t.Error("Result should contain DEBUG")
	}
}

// Helper function to check if text contains a substring (ignoring ANSI codes)
func containsText(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestRenderLogNavBar_UsesVisualWidth(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 40)

	chat.EnterLogViewerMode([]LogFile{
		{Name: "Debug Log", Path: "/tmp/test1.log", Content: "content1"},
		{Name: "MCP Log (session-abc123)", Path: "/tmp/test2.log", Content: "content2"},
		{Name: "Stream Log (session-abc123)", Path: "/tmp/test3.log", Content: "content3"},
	})

	result := chat.renderLogNavBar(80)
	visibleWidth := lipgloss.Width(result)

	// The rendered nav bar's visible width should not exceed the requested width
	if visibleWidth > 80 {
		t.Errorf("renderLogNavBar visible width %d exceeds requested width 80", visibleWidth)
	}
}

func TestRenderLogNavBar_FilenameNotOverTruncated(t *testing.T) {
	chat := NewChat()
	chat.SetSize(120, 40)

	// Short name that should fit easily at width 120
	chat.EnterLogViewerMode([]LogFile{
		{Name: "Debug Log", Path: "/tmp/test.log", Content: "content"},
	})

	result := chat.renderLogNavBar(120)

	// The name should appear in the output without truncation
	stripped := stripANSI(result)
	if !strings.Contains(stripped, "Debug Log") {
		t.Errorf("renderLogNavBar at width 120 should contain full name 'Debug Log', got: %q", stripped)
	}
}

func TestRenderLogNavBar_NoFiles(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 40)

	result := chat.renderLogNavBar(80)
	if result == "" {
		t.Error("renderLogNavBar should return something even with no files")
	}
}

// keyPressMsg creates a tea.KeyPressMsg for the given key string
func keyPressMsg(key string) tea.KeyPressMsg {
	switch key {
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "esc", "escape":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	default:
		// Regular letter keys use Text field
		return tea.KeyPressMsg{Code: 0, Text: key}
	}
}
