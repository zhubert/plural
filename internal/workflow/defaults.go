package workflow

import "time"

// DefaultWorkflowConfig returns a Config with the default state graph
// (coding → open_pr → await_review → await_ci → merge → done, with a failed terminal state).
func DefaultWorkflowConfig() *Config {
	return &Config{
		Workflow: "issue-to-merge",
		Start:   "coding",
		Source: SourceConfig{
			Provider: "github",
			Filter: FilterConfig{
				Label: "queued",
			},
		},
		States: map[string]*State{
			"coding": {
				Type:   StateTypeTask,
				Action: "ai.code",
				Params: map[string]any{
					"max_turns":     50,
					"max_duration":  "30m",
					"containerized": true,
					"supervisor":    true,
				},
				Next:  "open_pr",
				Error: "failed",
			},
			"open_pr": {
				Type:   StateTypeTask,
				Action: "github.create_pr",
				Params: map[string]any{
					"link_issue": true,
				},
				Next:  "await_review",
				Error: "failed",
			},
			"await_review": {
				Type:  StateTypeWait,
				Event: "pr.reviewed",
				Params: map[string]any{
					"auto_address":        true,
					"max_feedback_rounds": 3,
				},
				Next:  "await_ci",
				Error: "failed",
			},
			"await_ci": {
				Type:    StateTypeWait,
				Event:   "ci.complete",
				Timeout: &Duration{2 * time.Hour},
				Params: map[string]any{
					"on_failure": "retry",
				},
				Next:  "merge",
				Error: "failed",
			},
			"merge": {
				Type:   StateTypeTask,
				Action: "github.merge",
				Params: map[string]any{
					"method":  "rebase",
					"cleanup": true,
				},
				Next: "done",
			},
			"done": {
				Type: StateTypeSucceed,
			},
			"failed": {
				Type: StateTypeFail,
			},
		},
	}
}

// DefaultConfig returns the default config. Alias for DefaultWorkflowConfig.
func DefaultConfig() *Config {
	return DefaultWorkflowConfig()
}

// Merge overlays partial onto defaults. States present in partial replace the
// corresponding default state entirely. States in defaults but not in partial
// are preserved. Top-level fields (Workflow, Start) use partial if non-empty.
// Source fields use partial if non-empty.
func Merge(partial, defaults *Config) *Config {
	result := &Config{
		Workflow: partial.Workflow,
		Start:   partial.Start,
		Source:  partial.Source,
		States:  make(map[string]*State),
	}

	// Fill empty top-level fields from defaults
	if result.Workflow == "" {
		result.Workflow = defaults.Workflow
	}
	if result.Start == "" {
		result.Start = defaults.Start
	}

	// Source
	if result.Source.Provider == "" {
		result.Source.Provider = defaults.Source.Provider
	}
	if result.Source.Filter.Label == "" {
		result.Source.Filter.Label = defaults.Source.Filter.Label
	}
	if result.Source.Filter.Project == "" {
		result.Source.Filter.Project = defaults.Source.Filter.Project
	}
	if result.Source.Filter.Team == "" {
		result.Source.Filter.Team = defaults.Source.Filter.Team
	}

	// Copy defaults first
	for name, state := range defaults.States {
		s := *state
		if state.Params != nil {
			s.Params = make(map[string]any, len(state.Params))
			for k, v := range state.Params {
				s.Params[k] = v
			}
		}
		if state.After != nil {
			s.After = make([]HookConfig, len(state.After))
			copy(s.After, state.After)
		}
		result.States[name] = &s
	}

	// Overlay partial states (full replacement per state)
	for name, state := range partial.States {
		result.States[name] = state
	}

	return result
}
