package workflow

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDurationUnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{name: "30 minutes", input: "30m", want: 30 * time.Minute},
		{name: "2 hours", input: "2h", want: 2 * time.Hour},
		{name: "1h30m", input: "1h30m", want: 90 * time.Minute},
		{name: "45s", input: "45s", want: 45 * time.Second},
		{name: "invalid", input: "bogus", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			yamlStr := "duration: " + tt.input
			var out struct {
				Duration Duration `yaml:"duration"`
			}
			err := yaml.Unmarshal([]byte(yamlStr), &out)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for input %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.Duration.Duration != tt.want {
				t.Errorf("got %v, want %v", out.Duration.Duration, tt.want)
			}
		})
	}
}

func TestDurationMarshalYAML(t *testing.T) {
	d := Duration{30 * time.Minute}
	val, err := d.MarshalYAML()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "30m0s" {
		t.Errorf("got %v, want 30m0s", val)
	}
}

func TestFullConfigParse(t *testing.T) {
	yamlStr := `
workflow: custom-flow
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
      max_turns: 100
      max_duration: "1h"
      containerized: true
      supervisor: false
      system_prompt: "Be careful with tests"
    after:
      - run: "./scripts/post-code.sh"
    next: open_pr
    error: failed

  open_pr:
    type: task
    action: github.create_pr
    params:
      link_issue: false
      template: "file:./pr-template.md"
    next: await_review
    error: failed

  await_review:
    type: wait
    event: pr.reviewed
    params:
      auto_address: true
      max_feedback_rounds: 5
      system_prompt: "file:./prompts/review.md"
    after:
      - run: "./scripts/post-review.sh"
    next: await_ci
    error: failed

  await_ci:
    type: wait
    event: ci.complete
    timeout: 3h
    params:
      on_failure: abandon
    next: merge
    error: failed

  merge:
    type: task
    action: github.merge
    params:
      method: squash
      cleanup: false
    after:
      - run: "./scripts/post-merge.sh"
    next: done

  done:
    type: succeed

  failed:
    type: fail
`

	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlStr), &cfg); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	// Top-level
	if cfg.Workflow != "custom-flow" {
		t.Errorf("workflow: got %q, want custom-flow", cfg.Workflow)
	}
	if cfg.Start != "coding" {
		t.Errorf("start: got %q, want coding", cfg.Start)
	}

	// Source
	if cfg.Source.Provider != "github" {
		t.Errorf("provider: got %q, want github", cfg.Source.Provider)
	}
	if cfg.Source.Filter.Label != "queued" {
		t.Errorf("label: got %q, want queued", cfg.Source.Filter.Label)
	}

	// States count
	if len(cfg.States) != 7 {
		t.Errorf("expected 7 states, got %d", len(cfg.States))
	}

	// Coding state
	coding := cfg.States["coding"]
	if coding.Type != StateTypeTask {
		t.Errorf("coding type: got %q, want task", coding.Type)
	}
	if coding.Action != "ai.code" {
		t.Errorf("coding action: got %q", coding.Action)
	}
	p := NewParamHelper(coding.Params)
	if p.Int("max_turns", 0) != 100 {
		t.Error("coding max_turns: expected 100")
	}
	if p.Bool("containerized", false) != true {
		t.Error("coding containerized: expected true")
	}
	if p.Bool("supervisor", true) != false {
		t.Error("coding supervisor: expected false")
	}
	if p.String("system_prompt", "") != "Be careful with tests" {
		t.Errorf("coding system_prompt: got %q", p.String("system_prompt", ""))
	}
	if len(coding.After) != 1 || coding.After[0].Run != "./scripts/post-code.sh" {
		t.Error("coding after hooks: unexpected value")
	}
	if coding.Next != "open_pr" {
		t.Errorf("coding next: got %q", coding.Next)
	}
	if coding.Error != "failed" {
		t.Errorf("coding error: got %q", coding.Error)
	}

	// CI state
	ci := cfg.States["await_ci"]
	if ci.Timeout == nil || ci.Timeout.Duration != 3*time.Hour {
		t.Error("ci timeout: expected 3h")
	}
	ciP := NewParamHelper(ci.Params)
	if ciP.String("on_failure", "") != "abandon" {
		t.Errorf("ci on_failure: got %q", ciP.String("on_failure", ""))
	}

	// Merge state
	merge := cfg.States["merge"]
	mP := NewParamHelper(merge.Params)
	if mP.String("method", "") != "squash" {
		t.Errorf("merge method: got %q", mP.String("method", ""))
	}
	if mP.Bool("cleanup", true) != false {
		t.Error("merge cleanup: expected false")
	}

	// Terminal states
	done := cfg.States["done"]
	if done.Type != StateTypeSucceed {
		t.Errorf("done type: got %q", done.Type)
	}
	failed := cfg.States["failed"]
	if failed.Type != StateTypeFail {
		t.Errorf("failed type: got %q", failed.Type)
	}
}

func TestConfigPartialParse(t *testing.T) {
	// Only source section, no states
	yamlStr := `
source:
  provider: asana
  filter:
    project: "12345"
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlStr), &cfg); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if cfg.Source.Provider != "asana" {
		t.Errorf("provider: got %q, want asana", cfg.Source.Provider)
	}
	if cfg.Source.Filter.Project != "12345" {
		t.Errorf("project: got %q, want 12345", cfg.Source.Filter.Project)
	}

	// States should be nil
	if cfg.States != nil {
		t.Error("states should be nil for partial config")
	}
}
