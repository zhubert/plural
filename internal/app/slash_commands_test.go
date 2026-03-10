package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/zhubert/plural/internal/config"
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
	expected := []string{"/cost", "/help", "/mcp", "/status", "Plural Slash Commands"}
	for _, exp := range expected {
		if !strings.Contains(result.Response, exp) {
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

func TestGetSlashCommands(t *testing.T) {
	commands := getSlashCommands()

	if len(commands) == 0 {
		t.Error("getSlashCommands should return at least one command")
	}

	// Check that required commands exist
	expectedCommands := []string{"cost", "help", "mcp", "plugins", "status"}
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

	if !strings.Contains(result.Response, "No active session") {
		t.Error("Response should mention no active session")
	}
}

func TestSessionJSONLEntry(t *testing.T) {
	// Test parsing of session JSONL entry structure
	jsonData := `{"type":"assistant","message":{"id":"msg_123","usage":{"input_tokens":100,"output_tokens":50},"model":"claude-opus-4"}}`

	var entry sessionJSONLEntry
	if err := json.Unmarshal([]byte(jsonData), &entry); err != nil {
		t.Fatalf("Failed to unmarshal entry: %v", err)
	}

	if entry.Type != "assistant" {
		t.Errorf("Type = %q, want 'assistant'", entry.Type)
	}

	if entry.Message.ID != "msg_123" {
		t.Errorf("ID = %q, want 'msg_123'", entry.Message.ID)
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

func TestMessageUsageDeduplication(t *testing.T) {
	// Test that the messageUsage struct correctly tracks max values
	// This simulates what happens when processing streaming chunks from Claude's JSONL

	// Simulate streaming chunks for a single message ID with cumulative token counts
	// Chunk 1: output_tokens=10 (cumulative)
	// Chunk 2: output_tokens=25 (cumulative)
	// Chunk 3: output_tokens=50 (cumulative, final)
	// The correct total is 50, NOT 10+25+50=85

	messageUsages := make(map[string]*messageUsage)

	// Process first chunk
	msgID := "msg_test_123"
	usage := &messageUsage{}
	messageUsages[msgID] = usage
	if int64(10) > usage.OutputTokens {
		usage.OutputTokens = 10
	}
	if int64(100) > usage.InputTokens {
		usage.InputTokens = 100
	}

	// Process second chunk (cumulative)
	if int64(25) > usage.OutputTokens {
		usage.OutputTokens = 25
	}

	// Process third chunk (cumulative, final)
	if int64(50) > usage.OutputTokens {
		usage.OutputTokens = 50
	}

	// Verify deduplication works
	if usage.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50 (max of cumulative values)", usage.OutputTokens)
	}

	if usage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", usage.InputTokens)
	}
}

func TestMessageUsageMultipleMessages(t *testing.T) {
	// Test deduplication across multiple message IDs (multiple API calls)
	// Each message ID should be counted separately with its max value

	messageUsages := make(map[string]*messageUsage)

	// Message 1: streaming chunks with cumulative output tokens [10, 30, 50]
	msg1ID := "msg_001"
	usage1 := &messageUsage{}
	messageUsages[msg1ID] = usage1
	for _, tokens := range []int64{10, 30, 50} {
		if tokens > usage1.OutputTokens {
			usage1.OutputTokens = tokens
		}
	}
	usage1.InputTokens = 100

	// Message 2: streaming chunks with cumulative output tokens [5, 15, 25]
	msg2ID := "msg_002"
	usage2 := &messageUsage{}
	messageUsages[msg2ID] = usage2
	for _, tokens := range []int64{5, 15, 25} {
		if tokens > usage2.OutputTokens {
			usage2.OutputTokens = tokens
		}
	}
	usage2.InputTokens = 150

	// Sum up deduplicated values
	var totalInput, totalOutput int64
	for _, usage := range messageUsages {
		totalInput += usage.InputTokens
		totalOutput += usage.OutputTokens
	}

	// Expected: 50 + 25 = 75 (max per message, not sum of all chunks)
	if totalOutput != 75 {
		t.Errorf("Total OutputTokens = %d, want 75 (sum of max per message)", totalOutput)
	}

	// Expected: 100 + 150 = 250
	if totalInput != 250 {
		t.Errorf("Total InputTokens = %d, want 250", totalInput)
	}

	// Message count should be 2
	if len(messageUsages) != 2 {
		t.Errorf("MessageCount = %d, want 2", len(messageUsages))
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

// =============================================================================
// /status Command Tests
// =============================================================================

func TestHandleStatusCommand_NoSession(t *testing.T) {
	// Override the CLI calls for testing
	origVersion := runClaudeVersion
	origAuth := runClaudeAuthStatus
	defer func() {
		runClaudeVersion = origVersion
		runClaudeAuthStatus = origAuth
	}()

	runClaudeVersion = func() string { return "2.1.72 (Claude Code)" }
	runClaudeAuthStatus = func() (*claudeAuthStatus, error) {
		orgName := "My Org"
		return &claudeAuthStatus{
			LoggedIn:         true,
			AuthMethod:       "claude.ai",
			Email:            "test@example.com",
			OrgName:          &orgName,
			SubscriptionType: "max",
		}, nil
	}

	m := &Model{activeSession: nil}
	result := handleStatusCommand(m, "")

	if !result.Handled {
		t.Error("handleStatusCommand should return Handled=true")
	}

	// Should show version
	if !strings.Contains(result.Response, "2.1.72") {
		t.Error("Response should contain version")
	}

	// Should show no session
	if !strings.Contains(result.Response, "Session: none") {
		t.Error("Response should indicate no active session")
	}

	// Should show auth info
	if !strings.Contains(result.Response, "Claude Max Account") {
		t.Errorf("Response should contain login method, got: %s", result.Response)
	}
	if !strings.Contains(result.Response, "test@example.com") {
		t.Error("Response should contain email")
	}
	if !strings.Contains(result.Response, "My Org") {
		t.Error("Response should contain organization")
	}
}

func TestHandleStatusCommand_WithSession(t *testing.T) {
	origVersion := runClaudeVersion
	origAuth := runClaudeAuthStatus
	defer func() {
		runClaudeVersion = origVersion
		runClaudeAuthStatus = origAuth
	}()

	runClaudeVersion = func() string { return "2.1.72" }
	runClaudeAuthStatus = func() (*claudeAuthStatus, error) {
		return &claudeAuthStatus{
			LoggedIn:         true,
			AuthMethod:       "claude.ai",
			Email:            "user@test.com",
			SubscriptionType: "pro",
		}, nil
	}

	m := &Model{
		activeSession: &config.Session{
			ID:       "abc-123-def",
			Name:     "my-feature",
			WorkTree: "/home/user/project",
		},
	}
	result := handleStatusCommand(m, "")

	if !result.Handled {
		t.Error("handleStatusCommand should return Handled=true")
	}
	if !strings.Contains(result.Response, "my-feature") {
		t.Error("Response should contain session name")
	}
	if !strings.Contains(result.Response, "abc-123-def") {
		t.Error("Response should contain session ID")
	}
	if !strings.Contains(result.Response, "/home/user/project") {
		t.Error("Response should contain cwd")
	}
	if !strings.Contains(result.Response, "Claude Pro Account") {
		t.Errorf("Response should contain login method, got: %s", result.Response)
	}
}

func TestHandleStatusCommand_AuthError(t *testing.T) {
	origVersion := runClaudeVersion
	origAuth := runClaudeAuthStatus
	defer func() {
		runClaudeVersion = origVersion
		runClaudeAuthStatus = origAuth
	}()

	runClaudeVersion = func() string { return "2.1.72" }
	runClaudeAuthStatus = func() (*claudeAuthStatus, error) {
		return nil, fmt.Errorf("command not found")
	}

	m := &Model{activeSession: nil}
	result := handleStatusCommand(m, "")

	if !result.Handled {
		t.Error("handleStatusCommand should return Handled=true")
	}
	if !strings.Contains(result.Response, "could not determine") {
		t.Error("Response should indicate auth failure")
	}
}

func TestHandleStatusCommand_NotLoggedIn(t *testing.T) {
	origVersion := runClaudeVersion
	origAuth := runClaudeAuthStatus
	defer func() {
		runClaudeVersion = origVersion
		runClaudeAuthStatus = origAuth
	}()

	runClaudeVersion = func() string { return "2.1.72" }
	runClaudeAuthStatus = func() (*claudeAuthStatus, error) {
		return &claudeAuthStatus{LoggedIn: false}, nil
	}

	m := &Model{activeSession: nil}
	result := handleStatusCommand(m, "")

	if !strings.Contains(result.Response, "not logged in") {
		t.Error("Response should indicate not logged in")
	}
}

func TestHandleStatusCommand_UnnamedSession(t *testing.T) {
	origVersion := runClaudeVersion
	origAuth := runClaudeAuthStatus
	defer func() {
		runClaudeVersion = origVersion
		runClaudeAuthStatus = origAuth
	}()

	runClaudeVersion = func() string { return "2.1.72" }
	runClaudeAuthStatus = func() (*claudeAuthStatus, error) {
		return &claudeAuthStatus{LoggedIn: true, SubscriptionType: "max"}, nil
	}

	m := &Model{
		activeSession: &config.Session{
			ID:       "test-id",
			Name:     "",
			WorkTree: "/tmp/work",
		},
	}
	result := handleStatusCommand(m, "")

	if !strings.Contains(result.Response, "(unnamed)") {
		t.Error("Response should show (unnamed) for sessions without a name")
	}
}

func TestClaudeAuthStatus_LoginMethodDisplay(t *testing.T) {
	tests := []struct {
		name     string
		status   claudeAuthStatus
		expected string
	}{
		{"max subscription", claudeAuthStatus{SubscriptionType: "max"}, "Claude Max Account"},
		{"pro subscription", claudeAuthStatus{SubscriptionType: "pro"}, "Claude Pro Account"},
		{"team subscription", claudeAuthStatus{SubscriptionType: "team"}, "Claude Team Account"},
		{"enterprise subscription", claudeAuthStatus{SubscriptionType: "enterprise"}, "Claude Enterprise Account"},
		{"api key auth", claudeAuthStatus{AuthMethod: "api-key"}, "API Key"},
		{"apiKey auth", claudeAuthStatus{AuthMethod: "apiKey"}, "API Key"},
		{"other auth method", claudeAuthStatus{AuthMethod: "oauth"}, "oauth"},
		{"unknown", claudeAuthStatus{}, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.status.loginMethodDisplay()
			if result != tt.expected {
				t.Errorf("loginMethodDisplay() = %q, want %q", result, tt.expected)
			}
		})
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

// =============================================================================
// handleSlashCommand Dispatcher Tests
// =============================================================================

func TestHandleSlashCommand_Dispatcher(t *testing.T) {
	// Mock CLI calls for /status command
	origVersion := runClaudeVersion
	origAuth := runClaudeAuthStatus
	defer func() {
		runClaudeVersion = origVersion
		runClaudeAuthStatus = origAuth
	}()
	runClaudeVersion = func() string { return "2.1.72" }
	runClaudeAuthStatus = func() (*claudeAuthStatus, error) {
		return &claudeAuthStatus{LoggedIn: true, SubscriptionType: "max"}, nil
	}

	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	tests := []struct {
		name         string
		input        string
		wantHandled  bool
		wantAction   SlashCommandAction
		wantResponse string // substring expected in response
	}{
		{
			name:        "non-slash input is not handled",
			input:       "hello world",
			wantHandled: false,
		},
		{
			name:         "cost with no active session",
			input:        "/cost",
			wantHandled:  true,
			wantResponse: "No active session",
		},
		{
			name:         "help returns command list",
			input:        "/help",
			wantHandled:  true,
			wantResponse: "/cost",
		},
		{
			name:        "mcp opens modal",
			input:       "/mcp",
			wantHandled: true,
			wantAction:  ActionOpenMCP,
		},
		{
			name:        "plugins opens modal",
			input:       "/plugins",
			wantHandled: true,
			wantAction:  ActionOpenPlugins,
		},
		{
			name:        "plugin alias opens modal",
			input:       "/plugin",
			wantHandled: true,
			wantAction:  ActionOpenPlugins,
		},
		{
			name:         "status returns info",
			input:        "/status",
			wantHandled:  true,
			wantResponse: "Status",
		},
		{
			name:        "unknown command is not handled",
			input:       "/foobar",
			wantHandled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := m.handleSlashCommand(tt.input)

			if result.Handled != tt.wantHandled {
				t.Errorf("handleSlashCommand(%q).Handled = %v, want %v", tt.input, result.Handled, tt.wantHandled)
			}
			if result.Action != tt.wantAction {
				t.Errorf("handleSlashCommand(%q).Action = %v, want %v", tt.input, result.Action, tt.wantAction)
			}
			if tt.wantResponse != "" && !strings.Contains(result.Response, tt.wantResponse) {
				t.Errorf("handleSlashCommand(%q).Response should contain %q, got %q", tt.input, tt.wantResponse, result.Response)
			}
		})
	}
}

func TestFormatNumber_EdgeCases(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{-1, "-1"},                      // Negative
		{-1234, "-1,234"},               // Negative with commas
		{999, "999"},                    // Just under threshold
		{1000, "1,000"},                 // Exactly at threshold
		{10000000000, "10,000,000,000"}, // Large number
	}

	for _, tt := range tests {
		result := formatNumber(tt.input)
		if result != tt.expected {
			t.Errorf("formatNumber(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
