package claudeconfig

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestDiscoverMCPToolPatterns(t *testing.T) {
	tests := []struct {
		name         string
		claudeJSON   string   // contents for ~/.claude.json (empty = don't create)
		mcpJSON      string   // contents for <repo>/.mcp.json (empty = don't create)
		useSymlink   bool     // pass a symlink path as repoPath
		wantPatterns []string // expected patterns (sorted)
	}{
		{
			name:         "no config files",
			wantPatterns: nil,
		},
		{
			name: "claude.json with project mcpServers",
			claudeJSON: `{
				"projects": {
					"REPO_PATH": {
						"mcpServers": {
							"filesystem": {"command": "npx", "args": ["fs-server"]},
							"github": {"command": "gh", "args": ["mcp"]}
						}
					}
				}
			}`,
			wantPatterns: []string{"mcp__filesystem__*", "mcp__github__*"},
		},
		{
			name: "mcp.json with servers",
			mcpJSON: `{
				"sqlite": {"command": "npx", "args": ["sqlite-mcp"]},
				"postgres": {"command": "pg-mcp"}
			}`,
			wantPatterns: []string{"mcp__postgres__*", "mcp__sqlite__*"},
		},
		{
			name: "both sources merged and deduplicated",
			claudeJSON: `{
				"projects": {
					"REPO_PATH": {
						"mcpServers": {
							"shared-server": {"command": "shared"},
							"claude-only": {"command": "only-in-claude"}
						}
					}
				}
			}`,
			mcpJSON: `{
				"shared-server": {"command": "shared"},
				"repo-only": {"command": "only-in-repo"}
			}`,
			wantPatterns: []string{"mcp__claude-only__*", "mcp__repo-only__*", "mcp__shared-server__*"},
		},
		{
			name:         "malformed claude.json",
			claudeJSON:   `{not valid json`,
			wantPatterns: nil,
		},
		{
			name:         "malformed mcp.json",
			mcpJSON:      `[1, 2, 3]`,
			wantPatterns: nil,
		},
		{
			name: "repo path not found in projects",
			claudeJSON: `{
				"projects": {
					"/some/other/repo": {
						"mcpServers": {
							"other": {"command": "other"}
						}
					}
				}
			}`,
			wantPatterns: nil,
		},
		{
			name: "claude.json project with no mcpServers key",
			claudeJSON: `{
				"projects": {
					"REPO_PATH": {
						"allowedTools": ["tool1"]
					}
				}
			}`,
			wantPatterns: nil,
		},
		{
			name: "symlinked repo path matches claude.json",
			claudeJSON: `{
				"projects": {
					"RESOLVED_PATH": {
						"mcpServers": {
							"symlink-server": {"command": "test"}
						}
					}
				}
			}`,
			useSymlink:   true,
			wantPatterns: []string{"mcp__symlink-server__*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp home dir
			homeDir := t.TempDir()

			// Create temp repo dir
			repoDir := t.TempDir()

			// Write ~/.claude.json if specified
			if tt.claudeJSON != "" {
				// Replace REPO_PATH placeholder with actual repo path
				content := tt.claudeJSON

				if tt.useSymlink {
					// For symlink tests, use a symlink as repoPath but
					// register the fully resolved path in claude.json
					// (on macOS, /tmp is a symlink to /private/tmp)
					resolvedPath, err := filepath.EvalSymlinks(repoDir)
					if err != nil {
						t.Fatalf("failed to resolve repo dir: %v", err)
					}
					content = replaceAll(content, "RESOLVED_PATH", resolvedPath)

					// Create a symlink to the repo dir
					symlinkDir := t.TempDir()
					symlinkPath := filepath.Join(symlinkDir, "linked-repo")
					if err := os.Symlink(repoDir, symlinkPath); err != nil {
						t.Fatalf("failed to create symlink: %v", err)
					}
					repoDir = symlinkPath
				} else {
					content = replaceAll(content, "REPO_PATH", repoDir)
				}

				if err := os.WriteFile(filepath.Join(homeDir, ".claude.json"), []byte(content), 0644); err != nil {
					t.Fatalf("failed to write claude.json: %v", err)
				}
			}

			// Write <repo>/.mcp.json if specified
			if tt.mcpJSON != "" {
				if err := os.WriteFile(filepath.Join(repoDir, ".mcp.json"), []byte(tt.mcpJSON), 0644); err != nil {
					t.Fatalf("failed to write .mcp.json: %v", err)
				}
			}

			got := discoverMCPToolPatterns(repoDir, homeDir)

			// Sort for stable comparison
			sort.Strings(got)
			sort.Strings(tt.wantPatterns)

			if len(got) != len(tt.wantPatterns) {
				t.Fatalf("got %d patterns %v, want %d patterns %v", len(got), got, len(tt.wantPatterns), tt.wantPatterns)
			}

			for i := range got {
				if got[i] != tt.wantPatterns[i] {
					t.Errorf("pattern[%d] = %q, want %q", i, got[i], tt.wantPatterns[i])
				}
			}
		})
	}
}

func replaceAll(s, old, new string) string {
	for {
		i := indexOf(s, old)
		if i < 0 {
			return s
		}
		s = s[:i] + new + s[i+len(old):]
	}
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
