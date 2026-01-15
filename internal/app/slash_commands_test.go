package app

import (
	"testing"
)

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{12, "12"},
		{123, "123"},
		{1234, "1,234"},
		{12345, "12,345"},
		{123456, "123,456"},
		{1234567, "1,234,567"},
		{12345678, "12,345,678"},
		{123456789, "123,456,789"},
		{1234567890, "1,234,567,890"},
	}

	for _, tt := range tests {
		result := formatNumber(tt.input)
		if result != tt.expected {
			t.Errorf("formatNumber(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestGetSlashCommandCompletions(t *testing.T) {
	tests := []struct {
		prefix   string
		expected []string
	}{
		{"", nil},           // No prefix
		{"hello", nil},      // Not a slash command
		{"/", []string{"/cost", "/help", "/mcp", "/plugins"}},
		{"/c", []string{"/cost"}},
		{"/co", []string{"/cost"}},
		{"/cost", []string{"/cost"}},
		{"/h", []string{"/help"}},
		{"/help", []string{"/help"}},
		{"/m", []string{"/mcp"}},
		{"/mcp", []string{"/mcp"}},
		{"/p", []string{"/plugins"}},
		{"/plugins", []string{"/plugins"}},
		{"/xyz", []string{}}, // No matches - returns empty slice
	}

	for _, tt := range tests {
		result := GetSlashCommandCompletions(tt.prefix)
		if tt.expected == nil {
			if result != nil {
				t.Errorf("GetSlashCommandCompletions(%q) = %v, want nil", tt.prefix, result)
			}
			continue
		}
		if len(result) != len(tt.expected) {
			t.Errorf("GetSlashCommandCompletions(%q) = %v, want %v", tt.prefix, result, tt.expected)
			continue
		}
		for i := range result {
			if result[i] != tt.expected[i] {
				t.Errorf("GetSlashCommandCompletions(%q)[%d] = %q, want %q", tt.prefix, i, result[i], tt.expected[i])
			}
		}
	}
}

func TestHandleHelpCommand(t *testing.T) {
	result := handleHelpCommand(nil, "")

	if !result.Handled {
		t.Error("handleHelpCommand should return Handled=true")
	}

	if result.Response == "" {
		t.Error("handleHelpCommand should return a non-empty response")
	}

	// Check that the response contains expected commands
	expected := []string{"/cost", "/help", "/mcp", "Plural Slash Commands"}
	for _, exp := range expected {
		if !containsString(result.Response, exp) {
			t.Errorf("handleHelpCommand response should contain %q", exp)
		}
	}
}

func TestHandleMCPCommand(t *testing.T) {
	result := handleMCPCommand(nil, "")

	if !result.Handled {
		t.Error("handleMCPCommand should return Handled=true")
	}

	if result.Action != ActionOpenMCP {
		t.Errorf("handleMCPCommand should return Action=ActionOpenMCP, got %v", result.Action)
	}
}

func TestHandlePluginsCommand(t *testing.T) {
	result := handlePluginsCommand(nil, "")

	if !result.Handled {
		t.Error("handlePluginsCommand should return Handled=true")
	}

	if result.Action != ActionOpenPlugins {
		t.Errorf("handlePluginsCommand should return Action=ActionOpenPlugins, got %v", result.Action)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
