package workflow

import "time"

// DefaultConfig returns a Config matching the current hardcoded daemon behavior.
func DefaultConfig() *Config {
	maxTurns := 50
	maxDuration := Duration{30 * time.Minute}
	containerized := true
	supervisor := true
	draft := false
	linkIssue := true
	autoAddress := true
	maxFeedbackRounds := 3
	ciTimeout := Duration{2 * time.Hour}
	cleanup := true

	return &Config{
		Source: SourceConfig{
			Provider: "github",
			Filter: FilterConfig{
				Label: "queued",
			},
		},
		Workflow: WorkflowConfig{
			Coding: CodingConfig{
				MaxTurns:      &maxTurns,
				MaxDuration:   &maxDuration,
				Containerized: &containerized,
				Supervisor:    &supervisor,
			},
			PR: PRConfig{
				Draft:     &draft,
				LinkIssue: &linkIssue,
			},
			Review: ReviewConfig{
				AutoAddress:       &autoAddress,
				MaxFeedbackRounds: &maxFeedbackRounds,
			},
			CI: CIConfig{
				Timeout:   &ciTimeout,
				OnFailure: "retry",
			},
			Merge: MergeConfig{
				Method:  "rebase",
				Cleanup: &cleanup,
			},
		},
	}
}

// Merge fills in missing values in partial from defaults.
// partial takes precedence; defaults fill gaps.
// Hook slices (After) are not merged — if partial defines hooks, they fully replace defaults.
func Merge(partial, defaults *Config) *Config {
	result := *partial

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

	// Coding
	if result.Workflow.Coding.MaxTurns == nil {
		result.Workflow.Coding.MaxTurns = defaults.Workflow.Coding.MaxTurns
	}
	if result.Workflow.Coding.MaxDuration == nil {
		result.Workflow.Coding.MaxDuration = defaults.Workflow.Coding.MaxDuration
	}
	if result.Workflow.Coding.Containerized == nil {
		result.Workflow.Coding.Containerized = defaults.Workflow.Coding.Containerized
	}
	if result.Workflow.Coding.Supervisor == nil {
		result.Workflow.Coding.Supervisor = defaults.Workflow.Coding.Supervisor
	}

	// PR
	if result.Workflow.PR.Draft == nil {
		result.Workflow.PR.Draft = defaults.Workflow.PR.Draft
	}
	if result.Workflow.PR.LinkIssue == nil {
		result.Workflow.PR.LinkIssue = defaults.Workflow.PR.LinkIssue
	}

	// Review
	if result.Workflow.Review.AutoAddress == nil {
		result.Workflow.Review.AutoAddress = defaults.Workflow.Review.AutoAddress
	}
	if result.Workflow.Review.MaxFeedbackRounds == nil {
		result.Workflow.Review.MaxFeedbackRounds = defaults.Workflow.Review.MaxFeedbackRounds
	}

	// CI
	if result.Workflow.CI.Timeout == nil {
		result.Workflow.CI.Timeout = defaults.Workflow.CI.Timeout
	}
	if result.Workflow.CI.OnFailure == "" {
		result.Workflow.CI.OnFailure = defaults.Workflow.CI.OnFailure
	}

	// Merge
	if result.Workflow.Merge.Method == "" {
		result.Workflow.Merge.Method = defaults.Workflow.Merge.Method
	}
	if result.Workflow.Merge.Cleanup == nil {
		result.Workflow.Merge.Cleanup = defaults.Workflow.Merge.Cleanup
	}

	return &result
}
