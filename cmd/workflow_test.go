package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowValidateCmd_NoFile(t *testing.T) {
	dir := t.TempDir()

	cmd := workflowValidateCmd
	cmd.SetArgs([]string{})
	workflowRepoPath = dir

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
}

func TestWorkflowValidateCmd_ValidFile(t *testing.T) {
	dir := t.TempDir()
	pluralDir := filepath.Join(dir, ".plural")
	if err := os.MkdirAll(pluralDir, 0o755); err != nil {
		t.Fatal(err)
	}

	yamlContent := `
source:
  provider: github
  filter:
    label: "queued"
workflow:
  merge:
    method: squash
  ci:
    on_failure: retry
`
	if err := os.WriteFile(filepath.Join(pluralDir, "workflow.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	workflowRepoPath = dir
	err := workflowValidateCmd.RunE(workflowValidateCmd, []string{})
	if err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestWorkflowValidateCmd_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	pluralDir := filepath.Join(dir, ".plural")
	if err := os.MkdirAll(pluralDir, 0o755); err != nil {
		t.Fatal(err)
	}

	yamlContent := `
source:
  provider: jira
`
	if err := os.WriteFile(filepath.Join(pluralDir, "workflow.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	workflowRepoPath = dir
	err := workflowValidateCmd.RunE(workflowValidateCmd, []string{})
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
	if !strings.Contains(err.Error(), "source.provider") {
		t.Errorf("error should mention source.provider: %v", err)
	}
}

func TestWorkflowVisualizeCmd(t *testing.T) {
	dir := t.TempDir()

	// Use default config (no file)
	workflowRepoPath = dir

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := workflowVisualizeCmd.RunE(workflowVisualizeCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stdout = old

	output := buf.String()
	if !strings.Contains(output, "stateDiagram-v2") {
		t.Errorf("output should contain stateDiagram-v2, got: %s", output)
	}
	if !strings.Contains(output, "Coding") {
		t.Errorf("output should contain Coding state, got: %s", output)
	}
}

func TestWorkflowVisualizeCmd_WithFile(t *testing.T) {
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
  ci:
    on_failure: abandon
  coding:
    after:
      - run: "echo done"
`
	if err := os.WriteFile(filepath.Join(pluralDir, "workflow.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	workflowRepoPath = dir

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := workflowVisualizeCmd.RunE(workflowVisualizeCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stdout = old

	output := buf.String()
	if !strings.Contains(output, "linear") {
		t.Errorf("output should contain linear provider, got: %s", output)
	}
	if !strings.Contains(output, "squash merge") {
		t.Errorf("output should contain squash merge, got: %s", output)
	}
	if !strings.Contains(output, "Abandoned") {
		t.Errorf("output should contain Abandoned for abandon on_failure, got: %s", output)
	}
	if !strings.Contains(output, "CodingHooks") {
		t.Errorf("output should contain CodingHooks, got: %s", output)
	}
}
