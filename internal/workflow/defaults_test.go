package workflow

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Source.Provider != "github" {
		t.Errorf("default provider: got %q, want github", cfg.Source.Provider)
	}
	if cfg.Source.Filter.Label != "queued" {
		t.Errorf("default label: got %q, want queued", cfg.Source.Filter.Label)
	}
	if cfg.Workflow.Coding.MaxTurns == nil || *cfg.Workflow.Coding.MaxTurns != 50 {
		t.Error("default max_turns: expected 50")
	}
	if cfg.Workflow.Coding.MaxDuration == nil || cfg.Workflow.Coding.MaxDuration.Duration != 30*time.Minute {
		t.Error("default max_duration: expected 30m")
	}
	if cfg.Workflow.Coding.Containerized == nil || !*cfg.Workflow.Coding.Containerized {
		t.Error("default containerized: expected true")
	}
	if cfg.Workflow.Coding.Supervisor == nil || !*cfg.Workflow.Coding.Supervisor {
		t.Error("default supervisor: expected true")
	}
	if cfg.Workflow.Review.MaxFeedbackRounds == nil || *cfg.Workflow.Review.MaxFeedbackRounds != 3 {
		t.Error("default max_feedback_rounds: expected 3")
	}
	if cfg.Workflow.CI.OnFailure != "retry" {
		t.Errorf("default on_failure: got %q, want retry", cfg.Workflow.CI.OnFailure)
	}
	if cfg.Workflow.Merge.Method != "rebase" {
		t.Errorf("default merge method: got %q, want rebase", cfg.Workflow.Merge.Method)
	}
}

func TestMerge(t *testing.T) {
	tests := []struct {
		name     string
		partial  *Config
		check    func(t *testing.T, result *Config)
	}{
		{
			name:    "empty partial gets all defaults",
			partial: &Config{},
			check: func(t *testing.T, result *Config) {
				if result.Source.Provider != "github" {
					t.Errorf("provider: got %q, want github", result.Source.Provider)
				}
				if result.Workflow.Coding.MaxTurns == nil || *result.Workflow.Coding.MaxTurns != 50 {
					t.Error("expected default max_turns")
				}
			},
		},
		{
			name: "partial provider preserved",
			partial: &Config{
				Source: SourceConfig{Provider: "asana"},
			},
			check: func(t *testing.T, result *Config) {
				if result.Source.Provider != "asana" {
					t.Errorf("provider: got %q, want asana", result.Source.Provider)
				}
			},
		},
		{
			name: "partial coding overrides preserved",
			partial: func() *Config {
				maxTurns := 100
				return &Config{
					Workflow: WorkflowConfig{
						Coding: CodingConfig{MaxTurns: &maxTurns},
					},
				}
			}(),
			check: func(t *testing.T, result *Config) {
				if result.Workflow.Coding.MaxTurns == nil || *result.Workflow.Coding.MaxTurns != 100 {
					t.Error("expected overridden max_turns of 100")
				}
				// Other defaults should still be filled
				if result.Workflow.Coding.MaxDuration == nil || result.Workflow.Coding.MaxDuration.Duration != 30*time.Minute {
					t.Error("expected default max_duration")
				}
			},
		},
		{
			name: "partial merge method preserved",
			partial: &Config{
				Workflow: WorkflowConfig{
					Merge: MergeConfig{Method: "squash"},
				},
			},
			check: func(t *testing.T, result *Config) {
				if result.Workflow.Merge.Method != "squash" {
					t.Errorf("merge method: got %q, want squash", result.Workflow.Merge.Method)
				}
			},
		},
		{
			name: "boolean false preserved (not overwritten by default true)",
			partial: func() *Config {
				f := false
				return &Config{
					Workflow: WorkflowConfig{
						Coding: CodingConfig{Containerized: &f},
					},
				}
			}(),
			check: func(t *testing.T, result *Config) {
				if result.Workflow.Coding.Containerized == nil || *result.Workflow.Coding.Containerized {
					t.Error("expected containerized to be false (overridden)")
				}
			},
		},
	}

	defaults := DefaultConfig()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Merge(tt.partial, defaults)
			tt.check(t, result)
		})
	}
}
