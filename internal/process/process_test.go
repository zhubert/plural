package process

import (
	"encoding/json"
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
	// ContainersSupported delegates to ContainerCLIInstalled (docker).
	// With PATH cleared, should return false.
	t.Setenv("PATH", "/nonexistent")
	if ContainersSupported() {
		t.Error("Expected ContainersSupported() to return false when docker is not on PATH")
	}
}

func TestContainerCLIInstalled_NoPath(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")

	if ContainerCLIInstalled() {
		t.Error("Expected false when docker CLI not on PATH")
	}
}

func TestContainerSystemRunning_NoCLI(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")

	if ContainerSystemRunning() {
		t.Error("Expected false when docker CLI not installed")
	}
}

func TestContainerImageExists_NoContainerCLI(t *testing.T) {
	// With docker CLI unavailable, should return false
	t.Setenv("PATH", "/nonexistent")

	if ContainerImageExists("ghcr.io/zhubert/plural-claude") {
		t.Error("Expected false when docker CLI not found")
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

func TestCheckContainerImageUpdate_NoCLI(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")

	needsUpdate, err := CheckContainerImageUpdate("ghcr.io/zhubert/plural-claude")
	if err == nil {
		t.Error("Expected error when Docker CLI not installed")
	}
	if needsUpdate {
		t.Error("Expected needsUpdate to be false when CLI not installed")
	}
}

func TestCheckContainerImageUpdate_NoLocalImage(t *testing.T) {
	// If docker is available but image doesn't exist, should return error
	// We can't reliably test this without docker, so just verify the no-CLI path
	t.Setenv("PATH", "/nonexistent")

	needsUpdate, err := CheckContainerImageUpdate("nonexistent-image:latest")
	if err == nil {
		t.Error("Expected error for nonexistent image")
	}
	if needsUpdate {
		t.Error("Expected needsUpdate to be false")
	}
}

func TestManifestResponseParsing_MultiPlatform(t *testing.T) {
	rawJSON := `{
		"manifests": [
			{
				"digest": "sha256:abc123",
				"platform": {
					"architecture": "amd64",
					"os": "linux"
				}
			},
			{
				"digest": "sha256:def456",
				"platform": {
					"architecture": "arm64",
					"os": "linux"
				}
			}
		]
	}`

	var mr manifestResponse
	if err := json.Unmarshal([]byte(rawJSON), &mr); err != nil {
		t.Fatalf("Failed to parse manifest response: %v", err)
	}

	if len(mr.Manifests) != 2 {
		t.Fatalf("Expected 2 manifests, got %d", len(mr.Manifests))
	}

	if mr.Manifests[0].Digest != "sha256:abc123" {
		t.Errorf("Expected digest 'sha256:abc123', got %q", mr.Manifests[0].Digest)
	}

	if mr.Manifests[0].Platform.Architecture != "amd64" {
		t.Errorf("Expected architecture 'amd64', got %q", mr.Manifests[0].Platform.Architecture)
	}

	if mr.Manifests[1].Platform.OS != "linux" {
		t.Errorf("Expected OS 'linux', got %q", mr.Manifests[1].Platform.OS)
	}
}

func TestManifestResponseParsing_SinglePlatform(t *testing.T) {
	// Single-platform manifest has a top-level digest, no manifests array
	rawJSON := `{
		"digest": "sha256:singleplatform789"
	}`

	var mr manifestResponse
	if err := json.Unmarshal([]byte(rawJSON), &mr); err != nil {
		t.Fatalf("Failed to parse manifest response: %v", err)
	}

	if len(mr.Manifests) != 0 {
		t.Errorf("Expected 0 manifests for single-platform, got %d", len(mr.Manifests))
	}

	if mr.Digest != "sha256:singleplatform789" {
		t.Errorf("Expected top-level digest 'sha256:singleplatform789', got %q", mr.Digest)
	}
}

func TestImageInspectParsing(t *testing.T) {
	// Test parsing of docker image inspect JSON output
	tests := []struct {
		name       string
		json       string
		wantDigest string
		wantErr    bool
	}{
		{
			name:       "normal image with repo digests",
			json:       `[{"RepoDigests": ["ghcr.io/zhubert/plural-claude@sha256:abc123"]}]`,
			wantDigest: "sha256:abc123",
		},
		{
			name:    "empty repo digests (locally built)",
			json:    `[{"RepoDigests": []}]`,
			wantErr: true,
		},
		{
			name:    "null repo digests",
			json:    `[{"RepoDigests": null}]`,
			wantErr: true,
		},
		{
			name:    "empty array",
			json:    `[]`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var inspects []imageInspect
			if err := json.Unmarshal([]byte(tt.json), &inspects); err != nil {
				t.Fatalf("Failed to parse JSON: %v", err)
			}

			if len(inspects) == 0 || len(inspects[0].RepoDigests) == 0 {
				if !tt.wantErr {
					t.Error("Expected repo digests but got none")
				}
				return
			}

			repoDigest := inspects[0].RepoDigests[0]
			if _, after, ok := strings.Cut(repoDigest, "@"); ok {
				got := after
				if got != tt.wantDigest {
					t.Errorf("Got digest %q, want %q", got, tt.wantDigest)
				}
			} else if !tt.wantErr {
				t.Error("Expected @ in repo digest")
			}
		})
	}
}

func TestRepoDigestParsing(t *testing.T) {
	// Test the digest extraction logic used in getLocalImageDigest
	tests := []struct {
		name       string
		repoDigest string
		wantDigest string
		wantErr    bool
	}{
		{
			name:       "standard repo digest",
			repoDigest: "ghcr.io/zhubert/plural-claude@sha256:abc123def456",
			wantDigest: "sha256:abc123def456",
		},
		{
			name:       "no @ separator",
			repoDigest: "ghcr.io/zhubert/plural-claude:latest",
			wantDigest: "",
			wantErr:    true,
		},
		{
			name:       "empty string",
			repoDigest: "",
			wantDigest: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if idx := strings.Index(tt.repoDigest, "@"); idx != -1 {
				got := tt.repoDigest[idx+1:]
				if got != tt.wantDigest {
					t.Errorf("Got digest %q, want %q", got, tt.wantDigest)
				}
			} else if !tt.wantErr {
				t.Error("Expected to find @ in repo digest")
			}
		})
	}
}

func TestListContainerNamesParsing(t *testing.T) {
	// NOTE: This test duplicates the line-based parsing logic from ListContainerNames
	// rather than testing the function directly, because it depends on exec.Command.
	// Docker outputs one container name per line with `docker ps -a --format {{.Names}}`.
	tests := []struct {
		name      string
		output    string
		wantNames []string
	}{
		{
			name:      "single container",
			output:    "buildkit\n",
			wantNames: []string{"buildkit"},
		},
		{
			name:      "multiple containers",
			output:    "plural-abc123\nplural-def456\n",
			wantNames: []string{"plural-abc123", "plural-def456"},
		},
		{
			name:      "mixed containers",
			output:    "buildkit\nplural-test\n",
			wantNames: []string{"buildkit", "plural-test"},
		},
		{
			name:      "empty output",
			output:    "",
			wantNames: nil,
		},
		{
			name:      "whitespace only",
			output:    "  \n  \n",
			wantNames: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Replicate the parsing logic from ListContainerNames
			var names []string
			for line := range strings.SplitSeq(strings.TrimSpace(tt.output), "\n") {
				name := strings.TrimSpace(line)
				if name != "" {
					names = append(names, name)
				}
			}

			if len(names) != len(tt.wantNames) {
				t.Errorf("Got %d names, want %d", len(names), len(tt.wantNames))
				return
			}

			for i, name := range names {
				if i >= len(tt.wantNames) || name != tt.wantNames[i] {
					t.Errorf("Name[%d] = %q, want %q", i, name, tt.wantNames[i])
				}
			}
		})
	}
}
