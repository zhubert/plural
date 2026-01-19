package app

import (
	"encoding/json"
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

func TestGetSlashCommands(t *testing.T) {
	commands := getSlashCommands()

	if len(commands) == 0 {
		t.Error("getSlashCommands should return at least one command")
	}

	// Check that required commands exist
	expectedCommands := []string{"cost", "help", "mcp", "plugins"}
	for _, expected := range expectedCommands {
		found := false
		for _, cmd := range commands {
			if cmd.name == expected {
				found = true
				if cmd.description == "" {
					t.Errorf("Command %q has empty description", expected)
				}
				break
			}
		}
		if !found {
			t.Errorf("Expected command %q not found in getSlashCommands()", expected)
		}
	}
}

func TestHandleCostCommand_NoSession(t *testing.T) {
	// Test /cost with no active session
	// We need a Model with nil activeSession
	m := &Model{activeSession: nil}
	result := handleCostCommand(m, "")

	if !result.Handled {
		t.Error("handleCostCommand should return Handled=true")
	}

	if !containsString(result.Response, "No active session") {
		t.Error("Response should mention no active session")
	}
}

func TestSessionJSONLEntry(t *testing.T) {
	// Test parsing of session JSONL entry structure
	jsonData := `{"type":"assistant","message":{"usage":{"input_tokens":100,"output_tokens":50},"model":"claude-opus-4"}}`

	var entry sessionJSONLEntry
	if err := json.Unmarshal([]byte(jsonData), &entry); err != nil {
		t.Fatalf("Failed to unmarshal entry: %v", err)
	}

	if entry.Type != "assistant" {
		t.Errorf("Type = %q, want 'assistant'", entry.Type)
	}

	if entry.Message.Usage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", entry.Message.Usage.InputTokens)
	}

	if entry.Message.Usage.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", entry.Message.Usage.OutputTokens)
	}

	if entry.Message.Model != "claude-opus-4" {
		t.Errorf("Model = %q, want 'claude-opus-4'", entry.Message.Model)
	}
}

func TestSessionJSONLEntry_WithCache(t *testing.T) {
	jsonData := `{"type":"assistant","message":{"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":30,"cache_read_input_tokens":20,"cache_creation":{"ephemeral_5m_input_tokens":10,"ephemeral_1h_input_tokens":5}}}}`

	var entry sessionJSONLEntry
	if err := json.Unmarshal([]byte(jsonData), &entry); err != nil {
		t.Fatalf("Failed to unmarshal entry: %v", err)
	}

	if entry.Message.Usage.CacheCreationInputTokens != 30 {
		t.Errorf("CacheCreationInputTokens = %d, want 30", entry.Message.Usage.CacheCreationInputTokens)
	}

	if entry.Message.Usage.CacheReadInputTokens != 20 {
		t.Errorf("CacheReadInputTokens = %d, want 20", entry.Message.Usage.CacheReadInputTokens)
	}

	if entry.Message.Usage.CacheCreation.Ephemeral5mInputTokens != 10 {
		t.Errorf("Ephemeral5mInputTokens = %d, want 10", entry.Message.Usage.CacheCreation.Ephemeral5mInputTokens)
	}
}

func TestSlashCommandDef(t *testing.T) {
	cmd := slashCommandDef{
		name:        "test",
		description: "A test command",
	}

	if cmd.name != "test" {
		t.Errorf("name = %q, want 'test'", cmd.name)
	}

	if cmd.description != "A test command" {
		t.Errorf("description = %q, want 'A test command'", cmd.description)
	}
}

func TestFormatNumber_EdgeCases(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{-1, "-1"},                            // Negative
		{-1234, "-1,234"},                     // Negative with commas
		{999, "999"},                          // Just under threshold
		{1000, "1,000"},                       // Exactly at threshold
		{10000000000, "10,000,000,000"},       // Large number
	}

	for _, tt := range tests {
		result := formatNumber(tt.input)
		if result != tt.expected {
			t.Errorf("formatNumber(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
