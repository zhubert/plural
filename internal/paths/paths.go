// Package paths provides centralized path resolution for Plural's data directories.
//
// Plural supports the XDG Base Directory Specification for organizing files:
//
//   - Config (XDG_CONFIG_HOME): config.json — user settings worth syncing
//   - Data (XDG_DATA_HOME): sessions/*.json — local session history
//   - State (XDG_STATE_HOME): logs/ — transient log files
//
// Resolution order:
//  1. If ~/.plural/ exists → use legacy flat layout (all paths under ~/.plural/)
//  2. If XDG env vars are set → use XDG layout with proper separation
//  3. Fresh install, no XDG vars → default to ~/.plural/
package paths

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	mu       sync.Mutex
	resolved *resolvedPaths
)

type resolvedPaths struct {
	configDir string
	dataDir   string
	stateDir  string
	legacy    bool
}

// resolve computes the path layout once and caches it.
func resolve() (*resolvedPaths, error) {
	mu.Lock()
	defer mu.Unlock()

	if resolved != nil {
		return resolved, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	legacyDir := filepath.Join(home, ".plural")

	// 1. If ~/.plural/ exists, use legacy layout
	if info, err := os.Stat(legacyDir); err == nil && info.IsDir() {
		resolved = &resolvedPaths{
			configDir: legacyDir,
			dataDir:   legacyDir,
			stateDir:  legacyDir,
			legacy:    true,
		}
		return resolved, nil
	}

	// 2. Check XDG env vars
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	xdgData := os.Getenv("XDG_DATA_HOME")
	xdgState := os.Getenv("XDG_STATE_HOME")

	if xdgConfig != "" || xdgData != "" || xdgState != "" {
		// Use XDG layout — fill in defaults for unset vars
		if xdgConfig == "" {
			xdgConfig = filepath.Join(home, ".config")
		}
		if xdgData == "" {
			xdgData = filepath.Join(home, ".local", "share")
		}
		if xdgState == "" {
			xdgState = filepath.Join(home, ".local", "state")
		}
		resolved = &resolvedPaths{
			configDir: filepath.Join(xdgConfig, "plural"),
			dataDir:   filepath.Join(xdgData, "plural"),
			stateDir:  filepath.Join(xdgState, "plural"),
			legacy:    false,
		}
		return resolved, nil
	}

	// 3. Fresh install, no XDG — default to legacy
	resolved = &resolvedPaths{
		configDir: legacyDir,
		dataDir:   legacyDir,
		stateDir:  legacyDir,
		legacy:    true,
	}
	return resolved, nil
}

// ConfigDir returns the directory for configuration files (config.json).
func ConfigDir() (string, error) {
	r, err := resolve()
	if err != nil {
		return "", err
	}
	return r.configDir, nil
}

// DataDir returns the directory for persistent data files.
func DataDir() (string, error) {
	r, err := resolve()
	if err != nil {
		return "", err
	}
	return r.dataDir, nil
}

// StateDir returns the directory for runtime state and logs.
func StateDir() (string, error) {
	r, err := resolve()
	if err != nil {
		return "", err
	}
	return r.stateDir, nil
}

// ConfigFilePath returns the full path to config.json.
func ConfigFilePath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// SessionsDir returns the directory for session message files.
func SessionsDir() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sessions"), nil
}

// LogsDir returns the directory for log files.
func LogsDir() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "logs"), nil
}

// WorktreesDir returns the directory for centralized git worktrees.
func WorktreesDir() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "worktrees"), nil
}

// IsLegacyLayout returns true if using the ~/.plural/ flat layout.
func IsLegacyLayout() bool {
	r, err := resolve()
	if err != nil {
		return true // assume legacy on error
	}
	return r.legacy
}

// Reset clears the cached path resolution. This is intended for testing only.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	resolved = nil
}
