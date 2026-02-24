package claudeconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zhubert/plural-core/logger"
)

type claudeJSON struct {
	Projects map[string]projectConfig `json:"projects"`
}

type projectConfig struct {
	MCPServers map[string]json.RawMessage `json:"mcpServers"`
}

// DiscoverMCPToolPatterns reads the user's Claude Code MCP configuration
// from ~/.claude.json and <repoPath>/.mcp.json, and returns tool approval
// patterns (e.g., "mcp__myserver__*") for each discovered server name.
// All errors are logged but swallowed (best-effort).
func DiscoverMCPToolPatterns(repoPath string) []string {
	return discoverMCPToolPatterns(repoPath, "")
}

// discoverMCPToolPatterns is the internal implementation that accepts a homeDir
// override for testing. If homeDir is empty, os.UserHomeDir() is used.
func discoverMCPToolPatterns(repoPath, homeDir string) []string {
	log := logger.Get()
	seen := make(map[string]bool)
	var patterns []string

	// Resolve symlinks for repoPath to match against ~/.claude.json keys
	resolvedRepo := repoPath
	if resolved, err := filepath.EvalSymlinks(repoPath); err == nil {
		resolvedRepo = resolved
	}

	// Source 1: ~/.claude.json -> projects[repoPath].mcpServers
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			log.Debug("failed to get home dir for MCP discovery", "error", err)
			return nil
		}
	}

	claudeJSONPath := filepath.Join(homeDir, ".claude.json")
	if data, err := os.ReadFile(claudeJSONPath); err == nil {
		var cfg claudeJSON
		if err := json.Unmarshal(data, &cfg); err != nil {
			log.Debug("failed to parse ~/.claude.json for MCP discovery", "error", err)
		} else {
			// Try both the original and resolved repo path
			for _, key := range []string{resolvedRepo, repoPath} {
				if proj, ok := cfg.Projects[key]; ok {
					for name := range proj.MCPServers {
						if !seen[name] {
							seen[name] = true
							patterns = append(patterns, fmt.Sprintf("mcp__%s__*", name))
						}
					}
				}
			}
		}
	}

	// Source 2: <repoPath>/.mcp.json (top-level keys are server names)
	mcpJSONPath := filepath.Join(repoPath, ".mcp.json")
	if data, err := os.ReadFile(mcpJSONPath); err == nil {
		var servers map[string]json.RawMessage
		if err := json.Unmarshal(data, &servers); err != nil {
			log.Debug("failed to parse .mcp.json for MCP discovery", "error", err)
		} else {
			for name := range servers {
				if !seen[name] {
					seen[name] = true
					patterns = append(patterns, fmt.Sprintf("mcp__%s__*", name))
				}
			}
		}
	}

	if len(patterns) > 0 {
		log.Info("discovered MCP tool patterns for auto-approval", "patterns", patterns)
	}

	return patterns
}
