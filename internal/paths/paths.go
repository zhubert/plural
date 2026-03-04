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
//
// Test safety: when running under `go test`, if no test has overridden HOME
// or XDG env vars, resolve() automatically redirects to a temp directory.
// This prevents tests from accidentally writing to the real ~/.plural/ directory.
// Tests that explicitly set HOME (via t.Setenv) and call Reset() get normal
// resolution against their overridden paths.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var (
	mu       sync.Mutex
	resolved *resolvedPaths

	// origHome is captured at init time to detect whether a test has
	// overridden HOME. If HOME still equals origHome during a test,
	// resolve() auto-redirects to a temp dir.
	origHome string

	// testFallback is lazily created once per test binary for auto-redirect.
	testFallbackOnce sync.Once
	testFallbackDir  string
)

func init() {
	origHome, _ = os.UserHomeDir()
}

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

	// Safety net: when running under `go test`, if HOME hasn't been
	// overridden by the test, automatically redirect to a temp dir
	// to prevent accidental writes to the real ~/.plural/ directory.
	if testing.Testing() && home == origHome {
		return resolveTestFallback()
	}

	return resolveNormal(home)
}

// resolveTestFallback returns paths under an auto-created temp directory.
// Called when running under go test without explicit path isolation.
// Must be called with mu held.
func resolveTestFallback() (*resolvedPaths, error) {
	testFallbackOnce.Do(func() {
		testFallbackDir, _ = os.MkdirTemp("", "plural-test-paths-*")
	})
	if testFallbackDir == "" {
		return nil, fmt.Errorf("test safety net: failed to create temp directory")
	}
	resolved = &resolvedPaths{
		configDir: filepath.Join(testFallbackDir, "config", "plural"),
		dataDir:   filepath.Join(testFallbackDir, "data", "plural"),
		stateDir:  filepath.Join(testFallbackDir, "state", "plural"),
	}
	return resolved, nil
}

// resolveNormal applies the standard resolution logic.
// Must be called with mu held.
func resolveNormal(home string) (*resolvedPaths, error) {
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
