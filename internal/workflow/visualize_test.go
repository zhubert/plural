package workflow

import (
	"strings"
	"testing"
)

func TestGenerateMermaid_Default(t *testing.T) {
	cfg := DefaultWorkflowConfig()
	out := GenerateMermaid(cfg)

	// Should contain basic transitions
	mustContain := []string{
		"stateDiagram-v2",
		"[*] --> coding",
		"coding -->",
		"open_pr -->",
		"await_review -->",
		"await_ci -->",
		"merge -->",
		"done --> [*]",
		"failed --> [*]",
	}

	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("output missing %q\n\nFull output:\n%s", s, out)
		}
	}
}

func TestGenerateMermaid_WithHooks(t *testing.T) {
	cfg := DefaultWorkflowConfig()
	cfg.States["coding"].After = []HookConfig{{Run: "echo test"}}

	out := GenerateMermaid(cfg)

	if !strings.Contains(out, "coding_hooks") {
		t.Errorf("output missing hook state\n\nFull output:\n%s", out)
	}
}

func TestGenerateMermaid_ErrorEdges(t *testing.T) {
	cfg := DefaultWorkflowConfig()
	out := GenerateMermaid(cfg)

	// Coding should have error edge to failed
	if !strings.Contains(out, "coding --> failed : error") {
		t.Errorf("output missing error edge from coding\n\nFull output:\n%s", out)
	}
}

func TestGenerateMermaid_WaitTimeout(t *testing.T) {
	cfg := DefaultWorkflowConfig()
	out := GenerateMermaid(cfg)

	// await_ci should show timeout
	if !strings.Contains(out, "timeout:") {
		t.Errorf("output missing timeout label\n\nFull output:\n%s", out)
	}
}

func TestGenerateMermaid_CustomProvider(t *testing.T) {
	cfg := DefaultWorkflowConfig()
	cfg.Source.Provider = "linear"

	out := GenerateMermaid(cfg)
	// Just verify it still generates valid output
	if !strings.Contains(out, "stateDiagram-v2") {
		t.Errorf("expected valid mermaid output\n\nFull output:\n%s", out)
	}
}
