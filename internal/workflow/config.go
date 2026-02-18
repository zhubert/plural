// Package workflow provides configurable workflow definitions for the Plural agent daemon.
// Workflows are defined in .plural/workflow.yaml per repository.
package workflow

import (
	"fmt"
	"time"
)

// Config is the top-level workflow configuration.
type Config struct {
	Source   SourceConfig   `yaml:"source"`
	Workflow WorkflowConfig `yaml:"workflow"`
}

// SourceConfig defines where issues come from.
type SourceConfig struct {
	Provider string       `yaml:"provider"`
	Filter   FilterConfig `yaml:"filter"`
}

// FilterConfig holds provider-specific filter parameters.
type FilterConfig struct {
	Label   string `yaml:"label"`   // GitHub: issue label to poll
	Project string `yaml:"project"` // Asana: project GID
	Team    string `yaml:"team"`    // Linear: team ID
}

// WorkflowConfig holds per-step configuration.
type WorkflowConfig struct {
	Coding CodingConfig `yaml:"coding"`
	PR     PRConfig     `yaml:"pr"`
	Review ReviewConfig `yaml:"review"`
	CI     CIConfig     `yaml:"ci"`
	Merge  MergeConfig  `yaml:"merge"`
}

// CodingConfig controls the coding phase.
type CodingConfig struct {
	MaxTurns      *int       `yaml:"max_turns"`
	MaxDuration   *Duration  `yaml:"max_duration"`
	Containerized *bool      `yaml:"containerized"`
	Supervisor    *bool      `yaml:"supervisor"`
	SystemPrompt  string     `yaml:"system_prompt"`
	After         []HookConfig `yaml:"after"`
}

// PRConfig controls PR creation.
type PRConfig struct {
	Draft     *bool        `yaml:"draft"`
	LinkIssue *bool        `yaml:"link_issue"`
	Template  string       `yaml:"template"`
	After     []HookConfig `yaml:"after"`
}

// ReviewConfig controls the review feedback cycle.
type ReviewConfig struct {
	AutoAddress       *bool        `yaml:"auto_address"`
	MaxFeedbackRounds *int         `yaml:"max_feedback_rounds"`
	SystemPrompt      string       `yaml:"system_prompt"`
	After             []HookConfig `yaml:"after"`
}

// CIConfig controls CI handling.
type CIConfig struct {
	Timeout   *Duration `yaml:"timeout"`
	OnFailure string    `yaml:"on_failure"`
}

// MergeConfig controls the merge step.
type MergeConfig struct {
	Method  string       `yaml:"method"`
	Cleanup *bool        `yaml:"cleanup"`
	After   []HookConfig `yaml:"after"`
}

// HookConfig defines a hook to run after a workflow step.
type HookConfig struct {
	Run string `yaml:"run"`
}

// Duration is a wrapper around time.Duration that implements YAML unmarshaling
// from human-readable strings like "30m", "2h".
type Duration struct {
	time.Duration
}

// UnmarshalYAML implements yaml.Unmarshaler for Duration.
func (d *Duration) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = parsed
	return nil
}

// MarshalYAML implements yaml.Marshaler for Duration.
func (d Duration) MarshalYAML() (any, error) {
	return d.Duration.String(), nil
}
