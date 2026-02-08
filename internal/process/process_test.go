package process

import (
	"strings"
	"testing"
)

func TestExtractSessionID(t *testing.T) {
	tests := []struct {
		name     string
		cmdLine  string
		expected string
	}{
		{
			name:     "session-id flag",
			cmdLine:  "claude --print --session-id abc123 --verbose",
			expected: "abc123",
		},
		{
			name:     "resume flag",
			cmdLine:  "claude --print --resume def456 --verbose",
			expected: "def456",
		},
		{
			name:     "session-id with equals",
			cmdLine:  "claude --session-id=xyz789",
			expected: "xyz789",
		},
		{
			name:     "resume with equals",
			cmdLine:  "claude --resume=session-001",
			expected: "session-001",
		},
		{
			name:     "full command line",
			cmdLine:  "/usr/local/bin/claude --print --output-format stream-json --input-format stream-json --verbose --session-id 550e8400-e29b-41d4-a716-446655440000 --mcp-config /tmp/plural-mcp.json",
			expected: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:     "no session flag",
			cmdLine:  "claude --print --verbose",
			expected: "",
		},
		{
			name:     "empty command",
			cmdLine:  "",
			expected: "",
		},
		{
			name:     "session-id at end",
			cmdLine:  "claude --verbose --session-id last-session",
			expected: "last-session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSessionID(tt.cmdLine)
			if result != tt.expected {
				t.Errorf("extractSessionID(%q) = %q, want %q", tt.cmdLine, result, tt.expected)
			}
		})
	}
}

func TestClaudeProcess_Fields(t *testing.T) {
	proc := ClaudeProcess{
		PID:     12345,
		Command: "claude --session-id test",
	}

	if proc.PID != 12345 {
		t.Errorf("Expected PID 12345, got %d", proc.PID)
	}

	if proc.Command != "claude --session-id test" {
		t.Errorf("Expected command 'claude --session-id test', got %q", proc.Command)
	}
}

func TestFindOrphanedClaudeProcesses_NoOrphans(t *testing.T) {
	// This test just verifies the function doesn't crash with empty input
	knownSessions := map[string]bool{
		"session-1": true,
		"session-2": true,
	}

	// The actual processes found will depend on the system state,
	// but we can verify the function works
	orphans, err := FindOrphanedClaudeProcesses(knownSessions)
	if err != nil {
		t.Fatalf("FindOrphanedClaudeProcesses failed: %v", err)
	}

	// Can't assert on count since it depends on system state,
	// but function should not error
	_ = orphans
}

func TestFindClaudeProcesses(t *testing.T) {
	// This test verifies the function works without crashing
	processes, err := FindClaudeProcesses()
	if err != nil {
		t.Fatalf("FindClaudeProcesses failed: %v", err)
	}

	// Can't assert on count since it depends on system state
	_ = processes
}

func TestOrphanedContainer_Fields(t *testing.T) {
	c := OrphanedContainer{
		Name: "plural-abc123",
	}

	if c.Name != "plural-abc123" {
		t.Errorf("Expected Name 'plural-abc123', got %q", c.Name)
	}
}

func TestExtractSessionIDFromContainerName(t *testing.T) {
	tests := []struct {
		name      string
		container string
		wantID    string
	}{
		{
			name:      "standard plural prefix",
			container: "plural-abc123",
			wantID:    "abc123",
		},
		{
			name:      "uuid session ID",
			container: "plural-550e8400-e29b-41d4-a716-446655440000",
			wantID:    "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:      "minimal name",
			container: "plural-x",
			wantID:    "x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.TrimPrefix(tt.container, "plural-")
			if got != tt.wantID {
				t.Errorf("TrimPrefix(%q, 'plural-') = %q, want %q", tt.container, got, tt.wantID)
			}
		})
	}
}

func TestFindOrphanedContainers_NoContainerCLI(t *testing.T) {
	// Set PATH to empty to ensure container CLI is not found
	// t.Setenv automatically restores the original value after the test
	t.Setenv("PATH", "/nonexistent")

	knownSessions := map[string]bool{
		"session-1": true,
	}

	containers, err := FindOrphanedContainers(knownSessions)
	if err != nil {
		t.Fatalf("Expected no error when container CLI not found, got: %v", err)
	}

	if len(containers) != 0 {
		t.Errorf("Expected empty list when container CLI not found, got %d containers", len(containers))
	}
}
