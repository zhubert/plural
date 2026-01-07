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
