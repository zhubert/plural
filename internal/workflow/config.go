// Package workflow provides configurable workflow definitions for the Plural agent daemon.
// Workflows are defined in .plural/workflow.yaml per repository.
package workflow

import (
	"fmt"
	"time"
)

// StateType represents the kind of state in the workflow graph.
type StateType string

const (
	StateTypeTask    StateType = "task"
	StateTypeWait    StateType = "wait"
	StateTypeSucceed StateType = "succeed"
	StateTypeFail    StateType = "fail"
)

// Config is the top-level workflow configuration.
type Config struct {
	Workflow string            `yaml:"workflow"`
	Start    string            `yaml:"start"`
	Source   SourceConfig      `yaml:"source"`
	States   map[string]*State `yaml:"states"`
}

// State represents a single node in the workflow graph.
type State struct {
	Type    StateType      `yaml:"type"`
	Action  string         `yaml:"action,omitempty"`
	Event   string         `yaml:"event,omitempty"`
	Params  map[string]any `yaml:"params,omitempty"`
	Next    string         `yaml:"next,omitempty"`
	Error   string         `yaml:"error,omitempty"`
	Timeout *Duration      `yaml:"timeout,omitempty"`
	After   []HookConfig   `yaml:"after,omitempty"`
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

// ValidActions is the set of recognized action names for task states.
var ValidActions = map[string]bool{
	"ai.code":               true,
	"github.create_pr":      true,
	"github.push":           true,
	"github.merge":          true,
	"github.comment_issue":  true,
}

// ValidEvents is the set of recognized event names for wait states.
var ValidEvents = map[string]bool{
	"pr.reviewed": true,
	"ci.complete": true,
}

// ValidStateTypes is the set of recognized state types.
var ValidStateTypes = map[StateType]bool{
	StateTypeTask:    true,
	StateTypeWait:    true,
	StateTypeSucceed: true,
	StateTypeFail:    true,
}
