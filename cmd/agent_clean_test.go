package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhubert/plural/internal/paths"
)

// setupAgentCleanTest sets up isolated temp dirs for paths resolution.
// HOME is set to a temp dir (no ~/.plural/) so XDG vars are respected.
func setupAgentCleanTest(t *testing.T) (dataDir, stateDir string) {
	t.Helper()
	tmpDir := t.TempDir()

	// Set HOME to tmpDir so ~/.plural doesn't exist → XDG vars take effect
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmpDir, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmpDir, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "config"))
	paths.Reset()
	t.Cleanup(func() { paths.Reset() })

	dataDir = filepath.Join(tmpDir, "data", "plural")
	stateDir = filepath.Join(tmpDir, "state", "plural")
	os.MkdirAll(dataDir, 0o755)
	os.MkdirAll(stateDir, 0o755)
	return dataDir, stateDir
}

func TestRunAgentClean_NothingToClean(t *testing.T) {
	setupAgentCleanTest(t)

	agentCleanSkipConfirm = true
	defer func() { agentCleanSkipConfirm = false }()

	err := runAgentCleanWithReader(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunAgentClean_RemovesStateAndLocks(t *testing.T) {
	dataDir, stateDir := setupAgentCleanTest(t)

	// Create state file
	stateFile := filepath.Join(dataDir, "daemon-state.json")
	if err := os.WriteFile(stateFile, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatalf("failed to create state file: %v", err)
	}

	// Create lock files
	for _, name := range []string{"daemon-abc123.lock", "daemon-def456.lock"} {
		if err := os.WriteFile(filepath.Join(stateDir, name), []byte("12345"), 0o644); err != nil {
			t.Fatalf("failed to create lock file: %v", err)
		}
	}

	agentCleanSkipConfirm = true
	defer func() { agentCleanSkipConfirm = false }()

	err := runAgentCleanWithReader(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify state file removed
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Error("expected state file to be removed")
	}

	// Verify lock files removed
	locks, _ := filepath.Glob(filepath.Join(stateDir, "daemon-*.lock"))
	if len(locks) != 0 {
		t.Errorf("expected 0 lock files, got %d", len(locks))
	}
}

func TestRunAgentClean_AbortOnNo(t *testing.T) {
	dataDir, _ := setupAgentCleanTest(t)

	// Create state file
	stateFile := filepath.Join(dataDir, "daemon-state.json")
	if err := os.WriteFile(stateFile, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatalf("failed to create state file: %v", err)
	}

	agentCleanSkipConfirm = false

	err := runAgentCleanWithReader(strings.NewReader("n\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// State file should still exist
	if _, err := os.Stat(stateFile); err != nil {
		t.Error("expected state file to still exist after abort")
	}
}

func TestRunAgentClean_ConfirmYes(t *testing.T) {
	dataDir, _ := setupAgentCleanTest(t)

	// Create state file
	stateFile := filepath.Join(dataDir, "daemon-state.json")
	if err := os.WriteFile(stateFile, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatalf("failed to create state file: %v", err)
	}

	agentCleanSkipConfirm = false

	err := runAgentCleanWithReader(strings.NewReader("y\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// State file should be removed
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Error("expected state file to be removed after confirmation")
	}
}

func TestRunAgentClean_OnlyLocks(t *testing.T) {
	_, stateDir := setupAgentCleanTest(t)

	// Only create a lock file, no state file
	if err := os.WriteFile(filepath.Join(stateDir, "daemon-abc.lock"), []byte("99"), 0o644); err != nil {
		t.Fatalf("failed to create lock file: %v", err)
	}

	agentCleanSkipConfirm = true
	defer func() { agentCleanSkipConfirm = false }()

	err := runAgentCleanWithReader(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Lock file should be removed
	locks, _ := filepath.Glob(filepath.Join(stateDir, "daemon-*.lock"))
	if len(locks) != 0 {
		t.Errorf("expected 0 lock files, got %d", len(locks))
	}
}
