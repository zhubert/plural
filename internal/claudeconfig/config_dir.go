package claudeconfig

import (
	"os"
	"path/filepath"
)

// GetClaudeConfigDir returns the Claude Code configuration directory.
// If the CLAUDE_CONFIG_DIR environment variable is set, it is used as-is.
// Otherwise, falls back to ~/.claude.
func GetClaudeConfigDir() (string, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude"), nil
}
