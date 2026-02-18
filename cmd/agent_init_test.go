package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentInitCmd_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	agentInitRepo = dir

	err := agentInitCmd.RunE(agentInitCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fp := filepath.Join(dir, ".plural", "workflow.yaml")
	if _, err := os.Stat(fp); os.IsNotExist(err) {
		t.Fatal("expected workflow.yaml to be created")
	}
}

func TestAgentInitCmd_ErrorsIfExists(t *testing.T) {
	dir := t.TempDir()
	pluralDir := filepath.Join(dir, ".plural")
	if err := os.MkdirAll(pluralDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluralDir, "workflow.yaml"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	agentInitRepo = dir

	err := agentInitCmd.RunE(agentInitCmd, []string{})
	if err == nil {
		t.Fatal("expected error when file already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists': %v", err)
	}
}
