package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteTemplate_CreatesFile(t *testing.T) {
	dir := t.TempDir()

	fp, err := WriteTemplate(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := filepath.Join(dir, ".plural", "workflow.yaml")
	if fp != expected {
		t.Errorf("expected path %s, got %s", expected, fp)
	}

	data, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}

	content := string(data)

	// Verify required sections are present
	if !strings.Contains(content, "source:") {
		t.Error("template should contain source section")
	}
	if !strings.Contains(content, "provider: github") {
		t.Error("template should contain provider: github")
	}
	if !strings.Contains(content, "states:") {
		t.Error("template should contain states section")
	}
	if !strings.Contains(content, "start: coding") {
		t.Error("template should contain start: coding")
	}

	// Verify state types are present
	for _, stateType := range []string{
		"type: task",
		"type: wait",
		"type: succeed",
		"type: fail",
	} {
		if !strings.Contains(content, stateType) {
			t.Errorf("template should contain %q", stateType)
		}
	}

	// Verify action names
	for _, action := range []string{
		"action: ai.code",
		"action: github.create_pr",
		"action: github.merge",
	} {
		if !strings.Contains(content, action) {
			t.Errorf("template should contain %q", action)
		}
	}

	// Verify event names
	for _, event := range []string{
		"event: pr.reviewed",
		"event: ci.complete",
	} {
		if !strings.Contains(content, event) {
			t.Errorf("template should contain %q", event)
		}
	}
}

func TestWriteTemplate_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()

	_, err := WriteTemplate(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, ".plural"))
	if err != nil {
		t.Fatalf("expected .plural directory to exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected .plural to be a directory")
	}
}

func TestWriteTemplate_ErrorsIfFileExists(t *testing.T) {
	dir := t.TempDir()

	// Create the file first
	pluralDir := filepath.Join(dir, ".plural")
	if err := os.MkdirAll(pluralDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluralDir, "workflow.yaml"), []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := WriteTemplate(dir)
	if err == nil {
		t.Fatal("expected error when file already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists': %v", err)
	}

	// Verify original content was not overwritten
	data, err := os.ReadFile(filepath.Join(pluralDir, "workflow.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "existing" {
		t.Error("existing file should not be overwritten")
	}
}

func TestWriteTemplate_ValidYAML(t *testing.T) {
	// The uncommented portions of the template should produce a valid config
	dir := t.TempDir()

	_, err := WriteTemplate(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("template should produce parseable YAML: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	if cfg.Source.Provider != "github" {
		t.Errorf("expected provider github, got %q", cfg.Source.Provider)
	}
	if cfg.Source.Filter.Label != "queued" {
		t.Errorf("expected label queued, got %q", cfg.Source.Filter.Label)
	}
	if cfg.Start != "coding" {
		t.Errorf("expected start coding, got %q", cfg.Start)
	}

	errs := Validate(cfg)
	if len(errs) > 0 {
		t.Errorf("template should produce valid config, got errors: %v", errs)
	}
}
