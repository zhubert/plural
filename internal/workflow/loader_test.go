package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_FileNotExists(t *testing.T) {
	cfg, err := Load("/nonexistent/path")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil config for missing file")
	}
}

func TestLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()
	pluralDir := filepath.Join(dir, ".plural")
	if err := os.MkdirAll(pluralDir, 0o755); err != nil {
		t.Fatal(err)
	}

	yamlContent := `
source:
  provider: github
  filter:
    label: "ready"
workflow:
  coding:
    max_turns: 25
  merge:
    method: squash
`
	if err := os.WriteFile(filepath.Join(pluralDir, "workflow.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Source.Provider != "github" {
		t.Errorf("provider: got %q, want github", cfg.Source.Provider)
	}
	if cfg.Source.Filter.Label != "ready" {
		t.Errorf("label: got %q, want ready", cfg.Source.Filter.Label)
	}
	if cfg.Workflow.Coding.MaxTurns == nil || *cfg.Workflow.Coding.MaxTurns != 25 {
		t.Error("max_turns: expected 25")
	}
	if cfg.Workflow.Merge.Method != "squash" {
		t.Errorf("merge method: got %q, want squash", cfg.Workflow.Merge.Method)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	pluralDir := filepath.Join(dir, ".plural")
	if err := os.MkdirAll(pluralDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(pluralDir, "workflow.yaml"), []byte("{{invalid yaml"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadAndMerge_NoFile(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadAndMerge(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected default config")
	}
	if cfg.Source.Provider != "github" {
		t.Errorf("expected default provider github, got %q", cfg.Source.Provider)
	}
}

func TestLoadAndMerge_PartialFile(t *testing.T) {
	dir := t.TempDir()
	pluralDir := filepath.Join(dir, ".plural")
	if err := os.MkdirAll(pluralDir, 0o755); err != nil {
		t.Fatal(err)
	}

	yamlContent := `
source:
  provider: linear
  filter:
    team: "my-team"
workflow:
  merge:
    method: squash
`
	if err := os.WriteFile(filepath.Join(pluralDir, "workflow.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadAndMerge(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Explicit values preserved
	if cfg.Source.Provider != "linear" {
		t.Errorf("provider: got %q, want linear", cfg.Source.Provider)
	}
	if cfg.Workflow.Merge.Method != "squash" {
		t.Errorf("merge method: got %q, want squash", cfg.Workflow.Merge.Method)
	}

	// Defaults filled in
	if cfg.Workflow.Coding.MaxTurns == nil || *cfg.Workflow.Coding.MaxTurns != 50 {
		t.Error("expected default max_turns of 50")
	}
	if cfg.Workflow.CI.OnFailure != "retry" {
		t.Errorf("expected default on_failure retry, got %q", cfg.Workflow.CI.OnFailure)
	}
}
