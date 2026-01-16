package app

import "testing"

// Note: TestMergeType_String is in session_state_test.go

func TestMergeType_Push(t *testing.T) {
	// Test the MergeTypePush constant which isn't covered in session_state_test.go
	if MergeTypePush.String() != "push" {
		t.Errorf("MergeTypePush.String() = %q, want 'push'", MergeTypePush.String())
	}
}

func TestMergeTypeConstants(t *testing.T) {
	// Verify constants have expected values (iota order)
	if MergeTypeNone != 0 {
		t.Errorf("MergeTypeNone = %d, want 0", MergeTypeNone)
	}
	if MergeTypeMerge != 1 {
		t.Errorf("MergeTypeMerge = %d, want 1", MergeTypeMerge)
	}
	if MergeTypePR != 2 {
		t.Errorf("MergeTypePR = %d, want 2", MergeTypePR)
	}
	if MergeTypeParent != 3 {
		t.Errorf("MergeTypeParent = %d, want 3", MergeTypeParent)
	}
	if MergeTypePush != 4 {
		t.Errorf("MergeTypePush = %d, want 4", MergeTypePush)
	}
}

func TestAppState_String(t *testing.T) {
	tests := []struct {
		state AppState
		want  string
	}{
		{StateIdle, "Idle"},
		{StateStreamingClaude, "StreamingClaude"},
		{AppState(99), "Unknown"}, // Unknown state
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.want {
				t.Errorf("AppState(%d).String() = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestAppStateConstants(t *testing.T) {
	// Verify constants have expected values (iota order)
	if StateIdle != 0 {
		t.Errorf("StateIdle = %d, want 0", StateIdle)
	}
	if StateStreamingClaude != 1 {
		t.Errorf("StateStreamingClaude = %d, want 1", StateStreamingClaude)
	}
}

func TestFocusConstants(t *testing.T) {
	// Verify focus constants
	if FocusSidebar != 0 {
		t.Errorf("FocusSidebar = %d, want 0", FocusSidebar)
	}
	if FocusChat != 1 {
		t.Errorf("FocusChat = %d, want 1", FocusChat)
	}
}

func TestSlashCommandAction(t *testing.T) {
	// Verify SlashCommandAction constants
	if ActionNone != 0 {
		t.Errorf("ActionNone = %d, want 0", ActionNone)
	}
	if ActionOpenMCP != 1 {
		t.Errorf("ActionOpenMCP = %d, want 1", ActionOpenMCP)
	}
	if ActionOpenPlugins != 2 {
		t.Errorf("ActionOpenPlugins = %d, want 2", ActionOpenPlugins)
	}
}

func TestSlashCommandResult(t *testing.T) {
	// Test result struct
	result := SlashCommandResult{
		Handled:  true,
		Response: "test response",
		Action:   ActionOpenMCP,
	}

	if !result.Handled {
		t.Error("Expected Handled=true")
	}
	if result.Response != "test response" {
		t.Errorf("Response = %q, want 'test response'", result.Response)
	}
	if result.Action != ActionOpenMCP {
		t.Errorf("Action = %v, want ActionOpenMCP", result.Action)
	}
}

func TestUsageStats(t *testing.T) {
	stats := UsageStats{
		InputTokens:              1000,
		OutputTokens:             500,
		CacheCreationInputTokens: 200,
		CacheReadInputTokens:     100,
		TotalTokens:              1800,
		EstimatedCostUSD:         0.05,
		MessageCount:             10,
	}

	if stats.InputTokens != 1000 {
		t.Errorf("InputTokens = %d, want 1000", stats.InputTokens)
	}
	if stats.OutputTokens != 500 {
		t.Errorf("OutputTokens = %d, want 500", stats.OutputTokens)
	}
	if stats.TotalTokens != 1800 {
		t.Errorf("TotalTokens = %d, want 1800", stats.TotalTokens)
	}
	if stats.MessageCount != 10 {
		t.Errorf("MessageCount = %d, want 10", stats.MessageCount)
	}
}
