package process

import (
	"encoding/json"
	"runtime"
	"strconv"
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

func TestContainersSupported(t *testing.T) {
	// ContainersSupported should return a boolean without panicking.
	// On darwin/arm64 it returns true; on all other platforms it returns false.
	result := ContainersSupported()
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		if !result {
			t.Error("Expected ContainersSupported() to return true on darwin/arm64")
		}
	} else {
		if result {
			t.Error("Expected ContainersSupported() to return false on non-darwin/arm64")
		}
	}
}

func TestContainerCLIInstalled_NoPath(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")

	if ContainerCLIInstalled() {
		t.Error("Expected false when container CLI not on PATH")
	}
}

func TestContainerSystemRunning_NoCLI(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")

	if ContainerSystemRunning() {
		t.Error("Expected false when container CLI not installed")
	}
}

func TestContainerImageExists_NoContainerCLI(t *testing.T) {
	// With container CLI unavailable, should return false
	t.Setenv("PATH", "/nonexistent")

	if ContainerImageExists("plural-claude") {
		t.Error("Expected false when container CLI not found")
	}
}

func TestCheckContainerPrerequisites_NoCLI(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")

	result := CheckContainerPrerequisites("plural-claude", func() bool { return true })

	if result.CLIInstalled {
		t.Error("Expected CLIInstalled to be false")
	}
	if result.SystemRunning {
		t.Error("Expected SystemRunning to be false (short-circuited)")
	}
	if result.ImageExists {
		t.Error("Expected ImageExists to be false (short-circuited)")
	}
	if result.AuthAvailable {
		t.Error("Expected AuthAvailable to be false (short-circuited)")
	}
}

func TestCheckContainerPrerequisites_AuthCheckerNotCalledWhenCLIMissing(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")

	authCalled := false
	result := CheckContainerPrerequisites("plural-claude", func() bool {
		authCalled = true
		return true
	})

	if authCalled {
		t.Error("authChecker should not be called when CLI is not installed")
	}
	if result.AuthAvailable {
		t.Error("AuthAvailable should be false when short-circuited")
	}
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

func TestListContainerNamesJSON(t *testing.T) {
	// NOTE: This test duplicates the JSON parsing logic from ListContainerNames
	// rather than testing the function directly, because it depends on exec.Command.
	tests := []struct {
		name     string
		json     string
		wantIDs  []string
		wantErr  bool
	}{
		{
			name: "single container",
			json: `[{"configuration":{"id":"buildkit"}}]`,
			wantIDs: []string{"buildkit"},
			wantErr: false,
		},
		{
			name: "multiple containers",
			json: `[{"configuration":{"id":"plural-abc123"}},{"configuration":{"id":"plural-def456"}}]`,
			wantIDs: []string{"plural-abc123", "plural-def456"},
			wantErr: false,
		},
		{
			name: "mixed containers",
			json: `[{"configuration":{"id":"buildkit"}},{"configuration":{"id":"plural-test"}}]`,
			wantIDs: []string{"buildkit", "plural-test"},
			wantErr: false,
		},
		{
			name: "empty array",
			json: `[]`,
			wantIDs: nil, // Consistent with actual implementation returning nil for empty
			wantErr: false,
		},
		{
			name: "missing id field",
			json: `[{"configuration":{"other":"value"}}]`,
			wantIDs: nil,
			wantErr: false,
		},
		{
			name: "invalid json",
			json: `{invalid}`,
			wantIDs: nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the JSON parsing logic directly
			var containers []map[string]interface{}
			err := json.Unmarshal([]byte(tt.json), &containers)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			var names []string
			for _, container := range containers {
				if config, ok := container["configuration"].(map[string]interface{}); ok {
					if id, ok := config["id"].(string); ok && id != "" {
						names = append(names, id)
					}
				}
			}

			if len(names) != len(tt.wantIDs) {
				t.Errorf("Got %d names, want %d", len(names), len(tt.wantIDs))
			}

			for i, id := range names {
				if i >= len(tt.wantIDs) || id != tt.wantIDs[i] {
					t.Errorf("Name[%d] = %q, want %q", i, id, tt.wantIDs[i])
				}
			}
		})
	}
}

func TestWindowsProcessParsing_WMIC(t *testing.T) {
	// Test parsing of wmic output format (CSV with Node,CommandLine,ProcessId)
	wmicOutput := `Node,CommandLine,ProcessId
DESKTOP,C:\Program Files\Claude\claude.exe --session-id abc123 --verbose,1234
DESKTOP,C:\Program Files\Claude\claude.exe --session-id def456,5678
DESKTOP,C:\Program Files\OtherApp\other.exe,9999
`

	lines := strings.Split(wmicOutput, "\n")
	var processes []ClaudeProcess

	for i, line := range lines {
		// Skip header row and empty lines
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Split(line, ",")
		if len(fields) >= 2 {
			var pidStr, cmdLine string

			if len(fields) >= 3 {
				// wmic format - last field is PID
				pidStr = strings.Trim(strings.TrimSpace(fields[len(fields)-1]), "\"")
				// CommandLine is all fields except first (node) and last (PID)
				cmdLine = strings.Trim(strings.Join(fields[1:len(fields)-1], ","), "\"")
			}

			pid, err := strconv.Atoi(pidStr)
			if err != nil {
				continue
			}

			// Only include processes that contain --session-id
			if !strings.Contains(cmdLine, "--session-id") {
				continue
			}

			processes = append(processes, ClaudeProcess{
				PID:     pid,
				Command: cmdLine,
			})
		}
	}

	if len(processes) != 2 {
		t.Errorf("Expected 2 processes, got %d", len(processes))
	}

	if len(processes) > 0 {
		if processes[0].PID != 1234 {
			t.Errorf("Expected first PID 1234, got %d", processes[0].PID)
		}
		if !strings.Contains(processes[0].Command, "--session-id abc123") {
			t.Errorf("Expected command to contain --session-id abc123, got %q", processes[0].Command)
		}
	}

	if len(processes) > 1 {
		if processes[1].PID != 5678 {
			t.Errorf("Expected second PID 5678, got %d", processes[1].PID)
		}
		if !strings.Contains(processes[1].Command, "--session-id def456") {
			t.Errorf("Expected command to contain --session-id def456, got %q", processes[1].Command)
		}
	}
}

func TestWindowsProcessParsing_PowerShell(t *testing.T) {
	// Test parsing of PowerShell Get-CimInstance output format (CSV with ProcessId,CommandLine)
	// Get-CimInstance Win32_Process provides full command line, unlike Get-Process
	psOutput := `"ProcessId","CommandLine"
"1234","C:\Program Files\Claude\claude.exe --session-id abc123 --verbose"
"5678","C:\Program Files\Claude\claude.exe --session-id def456"
`

	lines := strings.Split(psOutput, "\n")
	var processes []ClaudeProcess

	for i, line := range lines {
		// Skip header row and empty lines
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Split(line, ",")
		if len(fields) >= 2 {
			var pidStr, cmdLine string

			// PowerShell Get-CimInstance format: ProcessId,CommandLine (2 fields)
			if len(fields) >= 3 {
				// wmic format - last field is PID
				pidStr = strings.Trim(strings.TrimSpace(fields[len(fields)-1]), "\"")
				cmdLine = strings.Trim(strings.Join(fields[1:len(fields)-1], ","), "\"")
			} else {
				// PowerShell format - first field is PID, second is CommandLine
				pidStr = strings.Trim(strings.TrimSpace(fields[0]), "\"")
				cmdLine = strings.Trim(strings.TrimSpace(fields[1]), "\"")
			}

			pid, err := strconv.Atoi(pidStr)
			if err != nil {
				continue
			}

			// Only include processes that contain --session-id
			if !strings.Contains(cmdLine, "--session-id") {
				continue
			}

			processes = append(processes, ClaudeProcess{
				PID:     pid,
				Command: cmdLine,
			})
		}
	}

	if len(processes) != 2 {
		t.Errorf("Expected 2 processes, got %d", len(processes))
	}

	if len(processes) > 0 {
		if processes[0].PID != 1234 {
			t.Errorf("Expected first PID 1234, got %d", processes[0].PID)
		}
		if !strings.Contains(processes[0].Command, "--session-id abc123") {
			t.Errorf("Expected command to contain --session-id abc123, got %q", processes[0].Command)
		}
	}

	if len(processes) > 1 {
		if processes[1].PID != 5678 {
			t.Errorf("Expected second PID 5678, got %d", processes[1].PID)
		}
		if !strings.Contains(processes[1].Command, "--session-id def456") {
			t.Errorf("Expected command to contain --session-id def456, got %q", processes[1].Command)
		}
	}
}
