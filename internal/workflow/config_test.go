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
source:
  provider: github
  filter:
    label: "queued"

workflow:
  coding:
    max_turns: 100
    max_duration: 1h
    containerized: true
    supervisor: false
    system_prompt: "Be careful with tests"
    after:
      - run: "./scripts/post-code.sh"

  pr:
    draft: true
    link_issue: false
    template: "file:./pr-template.md"

  review:
    auto_address: true
    max_feedback_rounds: 5
    system_prompt: "file:./prompts/review.md"
    after:
      - run: "./scripts/post-review.sh"

  ci:
    timeout: 3h
    on_failure: abandon

  merge:
    method: squash
    cleanup: false
    after:
      - run: "./scripts/post-merge.sh"
`

	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlStr), &cfg); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	// Source
	if cfg.Source.Provider != "github" {
		t.Errorf("provider: got %q, want github", cfg.Source.Provider)
	}
	if cfg.Source.Filter.Label != "queued" {
		t.Errorf("label: got %q, want queued", cfg.Source.Filter.Label)
	}

	// Coding
	if cfg.Workflow.Coding.MaxTurns == nil || *cfg.Workflow.Coding.MaxTurns != 100 {
		t.Error("coding max_turns: expected 100")
	}
	if cfg.Workflow.Coding.MaxDuration == nil || cfg.Workflow.Coding.MaxDuration.Duration != time.Hour {
		t.Error("coding max_duration: expected 1h")
	}
	if cfg.Workflow.Coding.Containerized == nil || !*cfg.Workflow.Coding.Containerized {
		t.Error("coding containerized: expected true")
	}
	if cfg.Workflow.Coding.Supervisor == nil || *cfg.Workflow.Coding.Supervisor {
		t.Error("coding supervisor: expected false")
	}
	if cfg.Workflow.Coding.SystemPrompt != "Be careful with tests" {
		t.Errorf("coding system_prompt: got %q", cfg.Workflow.Coding.SystemPrompt)
	}
	if len(cfg.Workflow.Coding.After) != 1 || cfg.Workflow.Coding.After[0].Run != "./scripts/post-code.sh" {
		t.Error("coding after hooks: unexpected value")
	}

	// PR
	if cfg.Workflow.PR.Draft == nil || !*cfg.Workflow.PR.Draft {
		t.Error("pr draft: expected true")
	}
	if cfg.Workflow.PR.LinkIssue == nil || *cfg.Workflow.PR.LinkIssue {
		t.Error("pr link_issue: expected false")
	}
	if cfg.Workflow.PR.Template != "file:./pr-template.md" {
		t.Errorf("pr template: got %q", cfg.Workflow.PR.Template)
	}

	// Review
	if cfg.Workflow.Review.MaxFeedbackRounds == nil || *cfg.Workflow.Review.MaxFeedbackRounds != 5 {
		t.Error("review max_feedback_rounds: expected 5")
	}

	// CI
	if cfg.Workflow.CI.Timeout == nil || cfg.Workflow.CI.Timeout.Duration != 3*time.Hour {
		t.Error("ci timeout: expected 3h")
	}
	if cfg.Workflow.CI.OnFailure != "abandon" {
		t.Errorf("ci on_failure: got %q, want abandon", cfg.Workflow.CI.OnFailure)
	}

	// Merge
	if cfg.Workflow.Merge.Method != "squash" {
		t.Errorf("merge method: got %q, want squash", cfg.Workflow.Merge.Method)
	}
	if cfg.Workflow.Merge.Cleanup == nil || *cfg.Workflow.Merge.Cleanup {
		t.Error("merge cleanup: expected false")
	}
}

func TestConfigPartialParse(t *testing.T) {
	// Only source section, everything else should be zero values
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

	// Workflow fields should be zero/nil
	if cfg.Workflow.Coding.MaxTurns != nil {
		t.Error("coding max_turns should be nil")
	}
	if cfg.Workflow.Merge.Method != "" {
		t.Error("merge method should be empty")
	}
}
