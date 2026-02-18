package workflow

import (
	"strings"
	"testing"
)

func TestGenerateMermaid_Default(t *testing.T) {
	cfg := DefaultConfig()
	out := GenerateMermaid(cfg)

	// Should contain basic states
	mustContain := []string{
		"stateDiagram-v2",
		"[*] --> Polling",
		"Polling --> Queued",
		"Queued --> Coding",
		"Coding --> PRCreation",
		"PRCreation --> AwaitingReview",
		"AwaitingReview --> AddressingFeedback",
		"AddressingFeedback --> AwaitingReview",
		"AwaitingReview --> AwaitingCI",
		"AwaitingCI --> Merging",
		"Merging --> Completed",
		"Completed --> [*]",
		"rebase merge",
	}

	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("output missing %q\n\nFull output:\n%s", s, out)
		}
	}
}

func TestGenerateMermaid_WithHooks(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workflow.Coding.After = []HookConfig{{Run: "echo test"}}
	cfg.Workflow.PR.After = []HookConfig{{Run: "echo pr"}}
	cfg.Workflow.Merge.After = []HookConfig{{Run: "echo merge"}}

	out := GenerateMermaid(cfg)

	hookStates := []string{
		"CodingHooks",
		"PRHooks",
		"MergeHooks",
	}

	for _, s := range hookStates {
		if !strings.Contains(out, s) {
			t.Errorf("output missing hook state %q\n\nFull output:\n%s", s, out)
		}
	}
}

func TestGenerateMermaid_AbandonOnFailure(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workflow.CI.OnFailure = "abandon"

	out := GenerateMermaid(cfg)

	if !strings.Contains(out, "Abandoned : CI failed") {
		t.Errorf("expected abandon transition\n\nFull output:\n%s", out)
	}
}

func TestGenerateMermaid_NotifyOnFailure(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workflow.CI.OnFailure = "notify"

	out := GenerateMermaid(cfg)

	if !strings.Contains(out, "Failed : CI failed (notify)") {
		t.Errorf("expected notify transition\n\nFull output:\n%s", out)
	}
}

func TestGenerateMermaid_SquashMerge(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workflow.Merge.Method = "squash"

	out := GenerateMermaid(cfg)

	if !strings.Contains(out, "squash merge") {
		t.Errorf("expected squash merge\n\nFull output:\n%s", out)
	}
}

func TestGenerateMermaid_CustomProvider(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Source.Provider = "linear"

	out := GenerateMermaid(cfg)

	if !strings.Contains(out, "linear issue found") {
		t.Errorf("expected linear provider label\n\nFull output:\n%s", out)
	}
}

func TestGenerateMermaid_MaxFeedbackRounds(t *testing.T) {
	cfg := DefaultConfig()
	rounds := 5
	cfg.Workflow.Review.MaxFeedbackRounds = &rounds

	out := GenerateMermaid(cfg)

	if !strings.Contains(out, "max 5 rounds") {
		t.Errorf("expected max 5 rounds\n\nFull output:\n%s", out)
	}
}
