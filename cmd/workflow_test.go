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
start: coding
source:
  provider: github
  filter:
    label: "queued"
states:
  coding:
    type: task
    action: ai.code
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

	buf := new(bytes.Buffer)
	workflowVisualizeCmd.SetOut(buf)
	workflowVisualizeCmd.SetErr(buf)

	err := workflowVisualizeCmd.RunE(workflowVisualizeCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "stateDiagram-v2") {
		t.Errorf("output should contain stateDiagram-v2, got: %s", output)
	}
	if !strings.Contains(output, "coding") {
		t.Errorf("output should contain coding state, got: %s", output)
	}
}

func TestWorkflowVisualizeCmd_WithFile(t *testing.T) {
	dir := t.TempDir()
	pluralDir := filepath.Join(dir, ".plural")
	if err := os.MkdirAll(pluralDir, 0o755); err != nil {
		t.Fatal(err)
	}

	yamlContent := `
start: coding
source:
  provider: github
  filter:
    label: "queued"
states:
  coding:
    type: task
    action: ai.code
    params:
      max_turns: 25
    next: merge
    error: failed
    after:
      - run: "echo done"
  merge:
    type: task
    action: github.merge
    params:
      method: squash
    next: done
  done:
    type: succeed
  failed:
    type: fail
`
	if err := os.WriteFile(filepath.Join(pluralDir, "workflow.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	workflowRepoPath = dir

	buf := new(bytes.Buffer)
	workflowVisualizeCmd.SetOut(buf)
	workflowVisualizeCmd.SetErr(buf)

	err := workflowVisualizeCmd.RunE(workflowVisualizeCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "coding") {
		t.Errorf("output should contain coding state, got: %s", output)
	}
	if !strings.Contains(output, "merge") {
		t.Errorf("output should contain merge state, got: %s", output)
	}
	if !strings.Contains(output, "github.merge") {
		t.Errorf("output should contain github.merge action, got: %s", output)
	}
}
