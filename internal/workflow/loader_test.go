package workflow

import (
	"os"
	"path/filepath"
	"strings"
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
workflow: test-flow
start: coding

source:
  provider: github
  filter:
    label: "ready"

states:
  coding:
    type: task
    action: ai.code
    params:
      max_turns: 25
    next: done
    error: failed
  done:
    type: succeed
  failed:
    type: fail
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
	if cfg.Start != "coding" {
		t.Errorf("start: got %q", cfg.Start)
	}
	coding := cfg.States["coding"]
	if coding == nil {
		t.Fatal("expected coding state")
	}
	p := NewParamHelper(coding.Params)
	if p.Int("max_turns", 0) != 25 {
		t.Error("max_turns: expected 25")
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

func TestLoad_OldFormatDetection(t *testing.T) {
	dir := t.TempDir()
	pluralDir := filepath.Join(dir, ".plural")
	if err := os.MkdirAll(pluralDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Old format has workflow as a nested struct
	oldYAML := `
source:
  provider: github
  filter:
    label: "queued"
workflow:
  coding:
    max_turns: 50
  merge:
    method: rebase
`
	if err := os.WriteFile(filepath.Join(pluralDir, "workflow.yaml"), []byte(oldYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for old format")
	}
	if !strings.Contains(err.Error(), "old flat format") {
		t.Errorf("expected old format error message, got: %v", err)
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
	if cfg.Start != "coding" {
		t.Errorf("expected default start coding, got %q", cfg.Start)
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

states:
  merge:
    type: task
    action: github.merge
    params:
      method: squash
    next: done
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
	mp := NewParamHelper(cfg.States["merge"].Params)
	if mp.String("method", "") != "squash" {
		t.Errorf("merge method: got %q", mp.String("method", ""))
	}

	// Defaults filled in — coding state from defaults
	coding := cfg.States["coding"]
	if coding == nil {
		t.Fatal("expected coding state from defaults")
	}
	cp := NewParamHelper(coding.Params)
	if cp.Int("max_turns", 0) != 50 {
		t.Error("expected default max_turns of 50")
	}
}
