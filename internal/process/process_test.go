package process

import (
	"testing"
)

func TestIsSessionInUseError(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected bool
	}{
		{
			name:     "session in use",
			errMsg:   "Error: Session abc123 is already in use",
			expected: true,
		},
		{
			name:     "session locked",
			errMsg:   "Session is locked by another process",
			expected: true,
		},
		{
			name:     "session busy",
			errMsg:   "Session is busy, please try again",
			expected: true,
		},
		{
			name:     "session already running",
			errMsg:   "A session with this ID is already running",
			expected: true,
		},
		{
			name:     "mixed case",
			errMsg:   "SESSION IS ALREADY IN USE",
			expected: true,
		},
		{
			name:     "unrelated error",
			errMsg:   "Network connection failed",
			expected: false,
		},
		{
			name:     "session without in use",
			errMsg:   "Session created successfully",
			expected: false,
		},
		{
			name:     "empty string",
			errMsg:   "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSessionInUseError(tt.errMsg)
			if result != tt.expected {
				t.Errorf("IsSessionInUseError(%q) = %v, want %v", tt.errMsg, result, tt.expected)
			}
		})
	}
}

func TestContainsSessionID(t *testing.T) {
	sessionID := "abc123-def456"

	tests := []struct {
		name     string
		cmdLine  string
		expected bool
	}{
		{
			name:     "session-id with space",
			cmdLine:  "claude --print --session-id abc123-def456 hello",
			expected: true,
		},
		{
			name:     "resume with space",
			cmdLine:  "claude --print --resume abc123-def456 hello",
			expected: true,
		},
		{
			name:     "session-id with equals",
			cmdLine:  "claude --print --session-id=abc123-def456 hello",
			expected: true,
		},
		{
			name:     "resume with equals",
			cmdLine:  "claude --print --resume=abc123-def456 hello",
			expected: true,
		},
		{
			name:     "different session",
			cmdLine:  "claude --print --session-id xyz789 hello",
			expected: false,
		},
		{
			name:     "no session flag",
			cmdLine:  "claude --print hello",
			expected: false,
		},
		{
			name:     "partial match",
			cmdLine:  "claude --print --session-id abc123 hello",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsSessionID(tt.cmdLine, sessionID)
			if result != tt.expected {
				t.Errorf("containsSessionID(%q, %q) = %v, want %v", tt.cmdLine, sessionID, result, tt.expected)
			}
		})
	}
}

func TestFindClaudeProcesses_NoProcesses(t *testing.T) {
	// Test with a session ID that shouldn't exist
	processes, err := FindClaudeProcesses("nonexistent-session-id-12345")
	if err != nil {
		t.Errorf("FindClaudeProcesses() error = %v, want nil", err)
	}
	if len(processes) != 0 {
		t.Errorf("FindClaudeProcesses() returned %d processes, want 0", len(processes))
	}
}

func TestKillClaudeProcesses_NoProcesses(t *testing.T) {
	// Kill with no matching processes should return 0, nil
	killed, err := KillClaudeProcesses("nonexistent-session-id-67890")
	if err != nil {
		t.Errorf("KillClaudeProcesses() error = %v, want nil", err)
	}
	if killed != 0 {
		t.Errorf("KillClaudeProcesses() killed %d, want 0", killed)
	}
}

func TestClaudeProcess_Fields(t *testing.T) {
	proc := ClaudeProcess{
		PID:       12345,
		SessionID: "session-123",
		Command:   "claude --print --session-id session-123",
	}

	if proc.PID != 12345 {
		t.Errorf("PID = %d, want 12345", proc.PID)
	}
	if proc.SessionID != "session-123" {
		t.Errorf("SessionID = %q, want 'session-123'", proc.SessionID)
	}
	if proc.Command == "" {
		t.Error("Command should not be empty")
	}
}

func TestContainsSessionID_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		cmdLine   string
		sessionID string
		expected  bool
	}{
		{
			name:      "empty session ID matches (substring behavior)",
			cmdLine:   "claude --print --session-id abc",
			sessionID: "",
			expected:  true, // Empty string is contained in any string
		},
		{
			name:      "empty command line",
			cmdLine:   "",
			sessionID: "abc123",
			expected:  false,
		},
		{
			name:      "both empty",
			cmdLine:   "",
			sessionID: "",
			expected:  false, // Empty command line doesn't contain the patterns
		},
		{
			name:      "session ID as substring - matches",
			cmdLine:   "claude --session-id abc123-extended",
			sessionID: "abc123",
			expected:  true, // Simple substring match
		},
		{
			name:      "session ID with special chars",
			cmdLine:   "claude --session-id test-session-uuid",
			sessionID: "test-session-uuid",
			expected:  true,
		},
		{
			name:      "session ID not in flags",
			cmdLine:   "claude --print abc123",
			sessionID: "abc123",
			expected:  false, // abc123 appears but not after --session-id or --resume
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsSessionID(tt.cmdLine, tt.sessionID)
			if result != tt.expected {
				t.Errorf("containsSessionID(%q, %q) = %v, want %v", tt.cmdLine, tt.sessionID, result, tt.expected)
			}
		})
	}
}

func TestIsSessionInUseError_AdditionalCases(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected bool
	}{
		{
			name:     "just session word",
			errMsg:   "session",
			expected: false, // needs "in use", "already", "locked", or "busy"
		},
		{
			name:     "just in use",
			errMsg:   "in use",
			expected: false, // needs "session"
		},
		{
			name:     "session with already",
			errMsg:   "This session is already active",
			expected: true,
		},
		{
			name:     "whitespace only",
			errMsg:   "   ",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSessionInUseError(tt.errMsg)
			if result != tt.expected {
				t.Errorf("IsSessionInUseError(%q) = %v, want %v", tt.errMsg, result, tt.expected)
			}
		})
	}
}
