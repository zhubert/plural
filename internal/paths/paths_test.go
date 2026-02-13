package paths

import (
	"os"
	"path/filepath"
	"testing"
)

// setupTestHome creates a temp directory, sets HOME to it, and resets the path cache.
// Returns the temp home path and a cleanup function.
func setupTestHome(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	Reset()
	t.Cleanup(Reset)
	return tmpDir
}

func TestFreshInstallNoXDG(t *testing.T) {
	home := setupTestHome(t)
	// No ~/.plural/, no XDG vars → default to ~/.plural/
	expected := filepath.Join(home, ".plural")

	configDir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	if configDir != expected {
		t.Errorf("ConfigDir = %q, want %q", configDir, expected)
	}

	dataDir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	if dataDir != expected {
		t.Errorf("DataDir = %q, want %q", dataDir, expected)
	}

	stateDir, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir: %v", err)
	}
	if stateDir != expected {
		t.Errorf("StateDir = %q, want %q", stateDir, expected)
	}

	if !IsLegacyLayout() {
		t.Error("IsLegacyLayout should be true for fresh install without XDG")
	}
}

func TestLegacyDirExists(t *testing.T) {
	home := setupTestHome(t)
	legacyDir := filepath.Join(home, ".plural")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatal(err)
	}

	configDir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	if configDir != legacyDir {
		t.Errorf("ConfigDir = %q, want %q", configDir, legacyDir)
	}

	dataDir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	if dataDir != legacyDir {
		t.Errorf("DataDir = %q, want %q", dataDir, legacyDir)
	}

	stateDir, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir: %v", err)
	}
	if stateDir != legacyDir {
		t.Errorf("StateDir = %q, want %q", stateDir, legacyDir)
	}

	if !IsLegacyLayout() {
		t.Error("IsLegacyLayout should be true when ~/.plural/ exists")
	}
}

func TestLegacyTakesPrecedenceOverXDG(t *testing.T) {
	home := setupTestHome(t)
	legacyDir := filepath.Join(home, ".plural")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Set XDG vars — legacy should still win
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	Reset()

	configDir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	if configDir != legacyDir {
		t.Errorf("ConfigDir = %q, want %q (legacy should take precedence)", configDir, legacyDir)
	}

	if !IsLegacyLayout() {
		t.Error("IsLegacyLayout should be true when ~/.plural/ exists, even with XDG vars")
	}
}

func TestXDGAllVarsSet(t *testing.T) {
	home := setupTestHome(t)
	// No ~/.plural/ exists

	xdgConfig := filepath.Join(home, "my-config")
	xdgData := filepath.Join(home, "my-data")
	xdgState := filepath.Join(home, "my-state")

	t.Setenv("XDG_CONFIG_HOME", xdgConfig)
	t.Setenv("XDG_DATA_HOME", xdgData)
	t.Setenv("XDG_STATE_HOME", xdgState)
	Reset()

	configDir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	if want := filepath.Join(xdgConfig, "plural"); configDir != want {
		t.Errorf("ConfigDir = %q, want %q", configDir, want)
	}

	dataDir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	if want := filepath.Join(xdgData, "plural"); dataDir != want {
		t.Errorf("DataDir = %q, want %q", dataDir, want)
	}

	stateDir, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir: %v", err)
	}
	if want := filepath.Join(xdgState, "plural"); stateDir != want {
		t.Errorf("StateDir = %q, want %q", stateDir, want)
	}

	if IsLegacyLayout() {
		t.Error("IsLegacyLayout should be false when using XDG")
	}
}

func TestXDGPartialVars(t *testing.T) {
	home := setupTestHome(t)
	// No ~/.plural/ exists

	xdgConfig := filepath.Join(home, "my-config")
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)
	// XDG_DATA_HOME and XDG_STATE_HOME not set — should use XDG defaults
	Reset()

	configDir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	if want := filepath.Join(xdgConfig, "plural"); configDir != want {
		t.Errorf("ConfigDir = %q, want %q", configDir, want)
	}

	dataDir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}
	if want := filepath.Join(home, ".local", "share", "plural"); dataDir != want {
		t.Errorf("DataDir = %q, want %q", dataDir, want)
	}

	stateDir, err := StateDir()
	if err != nil {
		t.Fatalf("StateDir: %v", err)
	}
	if want := filepath.Join(home, ".local", "state", "plural"); stateDir != want {
		t.Errorf("StateDir = %q, want %q", stateDir, want)
	}

	if IsLegacyLayout() {
		t.Error("IsLegacyLayout should be false when using XDG")
	}
}

func TestDerivedPaths(t *testing.T) {
	home := setupTestHome(t)
	legacyDir := filepath.Join(home, ".plural")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatal(err)
	}

	t.Run("legacy layout", func(t *testing.T) {
		Reset()
		cfgPath, err := ConfigFilePath()
		if err != nil {
			t.Fatalf("ConfigFilePath: %v", err)
		}
		if want := filepath.Join(legacyDir, "config.json"); cfgPath != want {
			t.Errorf("ConfigFilePath = %q, want %q", cfgPath, want)
		}

		sessDir, err := SessionsDir()
		if err != nil {
			t.Fatalf("SessionsDir: %v", err)
		}
		if want := filepath.Join(legacyDir, "sessions"); sessDir != want {
			t.Errorf("SessionsDir = %q, want %q", sessDir, want)
		}

		logsDir, err := LogsDir()
		if err != nil {
			t.Fatalf("LogsDir: %v", err)
		}
		if want := filepath.Join(legacyDir, "logs"); logsDir != want {
			t.Errorf("LogsDir = %q, want %q", logsDir, want)
		}
	})

	t.Run("XDG layout", func(t *testing.T) {
		// Remove legacy dir so XDG kicks in
		os.RemoveAll(legacyDir)
		xdgConfig := filepath.Join(home, ".config")
		xdgData := filepath.Join(home, ".local", "share")
		xdgState := filepath.Join(home, ".local", "state")
		t.Setenv("XDG_CONFIG_HOME", xdgConfig)
		t.Setenv("XDG_DATA_HOME", xdgData)
		t.Setenv("XDG_STATE_HOME", xdgState)
		Reset()

		cfgPath, err := ConfigFilePath()
		if err != nil {
			t.Fatalf("ConfigFilePath: %v", err)
		}
		if want := filepath.Join(xdgConfig, "plural", "config.json"); cfgPath != want {
			t.Errorf("ConfigFilePath = %q, want %q", cfgPath, want)
		}

		sessDir, err := SessionsDir()
		if err != nil {
			t.Fatalf("SessionsDir: %v", err)
		}
		if want := filepath.Join(xdgData, "plural", "sessions"); sessDir != want {
			t.Errorf("SessionsDir = %q, want %q", sessDir, want)
		}

		logsDir, err := LogsDir()
		if err != nil {
			t.Fatalf("LogsDir: %v", err)
		}
		if want := filepath.Join(xdgState, "plural", "logs"); logsDir != want {
			t.Errorf("LogsDir = %q, want %q", logsDir, want)
		}
	})
}

func TestResetClearsCache(t *testing.T) {
	home := setupTestHome(t)

	// First resolve: no legacy, no XDG → defaults to ~/.plural/
	dir1, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	expectedLegacy := filepath.Join(home, ".plural")
	if dir1 != expectedLegacy {
		t.Errorf("ConfigDir = %q, want %q", dir1, expectedLegacy)
	}

	// Now set XDG and reset
	xdgConfig := filepath.Join(home, "new-config")
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)
	Reset()

	dir2, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir after reset: %v", err)
	}
	expectedXDG := filepath.Join(xdgConfig, "plural")
	if dir2 != expectedXDG {
		t.Errorf("ConfigDir after reset = %q, want %q", dir2, expectedXDG)
	}
}

func TestLegacyFileNotDir(t *testing.T) {
	home := setupTestHome(t)
	// Create ~/.plural as a file, not a directory — should NOT be treated as legacy
	legacyPath := filepath.Join(home, ".plural")
	if err := os.WriteFile(legacyPath, []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}

	xdgConfig := filepath.Join(home, ".config")
	t.Setenv("XDG_CONFIG_HOME", xdgConfig)
	Reset()

	configDir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	if want := filepath.Join(xdgConfig, "plural"); configDir != want {
		t.Errorf("ConfigDir = %q, want %q (file named .plural should not trigger legacy)", configDir, want)
	}
}
