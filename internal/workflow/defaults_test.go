package workflow

import (
	"testing"
	"time"
)

func TestDefaultWorkflowConfig(t *testing.T) {
	cfg := DefaultWorkflowConfig()

	if cfg.Workflow != "issue-to-merge" {
		t.Errorf("default workflow: got %q", cfg.Workflow)
	}
	if cfg.Start != "coding" {
		t.Errorf("default start: got %q", cfg.Start)
	}
	if cfg.Source.Provider != "github" {
		t.Errorf("default provider: got %q", cfg.Source.Provider)
	}
	if cfg.Source.Filter.Label != "queued" {
		t.Errorf("default label: got %q", cfg.Source.Filter.Label)
	}

	// Verify expected states exist
	expectedStates := []string{"coding", "open_pr", "await_review", "await_ci", "merge", "done", "failed"}
	for _, name := range expectedStates {
		if _, ok := cfg.States[name]; !ok {
			t.Errorf("expected state %q to exist", name)
		}
	}

	// Coding params
	coding := cfg.States["coding"]
	p := NewParamHelper(coding.Params)
	if p.Int("max_turns", 0) != 50 {
		t.Error("coding max_turns: expected 50")
	}
	if p.Duration("max_duration", 0) != 30*time.Minute {
		t.Error("coding max_duration: expected 30m")
	}
	if !p.Bool("containerized", false) {
		t.Error("coding containerized: expected true")
	}
	if !p.Bool("supervisor", false) {
		t.Error("coding supervisor: expected true")
	}

	// Review params
	review := cfg.States["await_review"]
	rp := NewParamHelper(review.Params)
	if rp.Int("max_feedback_rounds", 0) != 3 {
		t.Error("review max_feedback_rounds: expected 3")
	}
	if !rp.Bool("auto_address", false) {
		t.Error("review auto_address: expected true")
	}

	// CI params
	ci := cfg.States["await_ci"]
	cp := NewParamHelper(ci.Params)
	if cp.String("on_failure", "") != "retry" {
		t.Errorf("ci on_failure: got %q", cp.String("on_failure", ""))
	}

	// Merge params
	merge := cfg.States["merge"]
	mp := NewParamHelper(merge.Params)
	if mp.String("method", "") != "rebase" {
		t.Errorf("merge method: got %q", mp.String("method", ""))
	}

	// Default should pass validation
	errs := Validate(cfg)
	if len(errs) > 0 {
		t.Errorf("default config should be valid, got errors: %v", errs)
	}
}

func TestMerge(t *testing.T) {
	t.Run("empty partial gets all defaults", func(t *testing.T) {
		partial := &Config{}
		defaults := DefaultWorkflowConfig()
		result := Merge(partial, defaults)

		if result.Source.Provider != "github" {
			t.Errorf("provider: got %q", result.Source.Provider)
		}
		if result.Start != "coding" {
			t.Errorf("start: got %q", result.Start)
		}
		if len(result.States) != len(defaults.States) {
			t.Errorf("expected %d states, got %d", len(defaults.States), len(result.States))
		}
	})

	t.Run("partial provider preserved", func(t *testing.T) {
		partial := &Config{
			Source: SourceConfig{Provider: "asana"},
		}
		defaults := DefaultWorkflowConfig()
		result := Merge(partial, defaults)

		if result.Source.Provider != "asana" {
			t.Errorf("provider: got %q", result.Source.Provider)
		}
	})

	t.Run("partial state replaces default entirely", func(t *testing.T) {
		partial := &Config{
			States: map[string]*State{
				"coding": {
					Type:   StateTypeTask,
					Action: "ai.code",
					Params: map[string]any{
						"max_turns": 100,
					},
					Next:  "open_pr",
					Error: "failed",
				},
			},
		}
		defaults := DefaultWorkflowConfig()
		result := Merge(partial, defaults)

		coding := result.States["coding"]
		p := NewParamHelper(coding.Params)
		if p.Int("max_turns", 0) != 100 {
			t.Error("expected overridden max_turns of 100")
		}
		// Since we replaced the entire state, max_duration should not be set
		if p.Has("max_duration") {
			t.Error("expected max_duration to be absent (full state replacement)")
		}

		// Other states from defaults should still exist
		if _, ok := result.States["open_pr"]; !ok {
			t.Error("expected open_pr state from defaults")
		}
	})

	t.Run("partial merge method preserved", func(t *testing.T) {
		partial := &Config{
			States: map[string]*State{
				"merge": {
					Type:   StateTypeTask,
					Action: "github.merge",
					Params: map[string]any{
						"method": "squash",
					},
					Next: "done",
				},
			},
		}
		defaults := DefaultWorkflowConfig()
		result := Merge(partial, defaults)

		mp := NewParamHelper(result.States["merge"].Params)
		if mp.String("method", "") != "squash" {
			t.Errorf("merge method: got %q", mp.String("method", ""))
		}
	})

	t.Run("default states preserved when not overridden", func(t *testing.T) {
		partial := &Config{}
		defaults := DefaultWorkflowConfig()
		result := Merge(partial, defaults)

		// Verify coding params from defaults are preserved
		coding := result.States["coding"]
		p := NewParamHelper(coding.Params)
		if p.Int("max_turns", 0) != 50 {
			t.Error("expected default max_turns of 50")
		}
	})
}
