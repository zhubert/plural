package workflow

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidationError describes a single validation problem.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validate checks a Config for errors and returns all problems found.
func Validate(cfg *Config) []ValidationError {
	var errs []ValidationError

	// Start state must exist
	if cfg.Start == "" {
		errs = append(errs, ValidationError{
			Field:   "start",
			Message: "start state is required",
		})
	} else if cfg.States != nil {
		if _, ok := cfg.States[cfg.Start]; !ok {
			errs = append(errs, ValidationError{
				Field:   "start",
				Message: fmt.Sprintf("start state %q does not exist", cfg.Start),
			})
		}
	}

	// States must exist
	if len(cfg.States) == 0 {
		errs = append(errs, ValidationError{
			Field:   "states",
			Message: "at least one state is required",
		})
	}

	// Validate each state
	for name, state := range cfg.States {
		errs = append(errs, validateState(name, state, cfg.States)...)
	}

	// Provider validation
	errs = append(errs, validateSource(cfg)...)

	return errs
}

// validateState validates a single state definition.
func validateState(name string, state *State, allStates map[string]*State) []ValidationError {
	var errs []ValidationError
	prefix := fmt.Sprintf("states.%s", name)

	// Type validation
	if !ValidStateTypes[state.Type] {
		errs = append(errs, ValidationError{
			Field:   prefix + ".type",
			Message: fmt.Sprintf("unknown state type %q (must be task, wait, succeed, or fail)", state.Type),
		})
		return errs // Can't validate further without valid type
	}

	switch state.Type {
	case StateTypeTask:
		// Task states require action
		if state.Action == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".action",
				Message: "action is required for task states",
			})
		} else if !ValidActions[state.Action] {
			errs = append(errs, ValidationError{
				Field:   prefix + ".action",
				Message: fmt.Sprintf("unknown action %q", state.Action),
			})
		}

		// Non-terminal states must have next
		if state.Next == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".next",
				Message: "next is required for task states",
			})
		}

		// Validate params for ai.code action
		if state.Action == "ai.code" {
			errs = append(errs, validateCodingParams(prefix, state.Params)...)
		}

		// Validate params for github.merge action
		if state.Action == "github.merge" {
			errs = append(errs, validateMergeParams(prefix, state.Params)...)
		}

		// Validate params for github.comment_issue action
		if state.Action == "github.comment_issue" {
			errs = append(errs, validateCommentIssueParams(prefix, state.Params)...)
		}

	case StateTypeWait:
		// Wait states require event
		if state.Event == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".event",
				Message: "event is required for wait states",
			})
		} else if !ValidEvents[state.Event] {
			errs = append(errs, ValidationError{
				Field:   prefix + ".event",
				Message: fmt.Sprintf("unknown event %q", state.Event),
			})
		}

		// Non-terminal states must have next
		if state.Next == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".next",
				Message: "next is required for wait states",
			})
		}

		// Validate params for ci.complete event
		if state.Event == "ci.complete" {
			errs = append(errs, validateCIParams(prefix, state.Params)...)
		}

	case StateTypeSucceed, StateTypeFail:
		// Terminal states must not have next
		if state.Next != "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".next",
				Message: "terminal states must not have next",
			})
		}
	}

	// Validate next/error references exist
	if state.Next != "" {
		if _, ok := allStates[state.Next]; !ok {
			errs = append(errs, ValidationError{
				Field:   prefix + ".next",
				Message: fmt.Sprintf("references non-existent state %q", state.Next),
			})
		}
	}
	if state.Error != "" {
		if _, ok := allStates[state.Error]; !ok {
			errs = append(errs, ValidationError{
				Field:   prefix + ".error",
				Message: fmt.Sprintf("references non-existent state %q", state.Error),
			})
		}
	}

	return errs
}

// validateCodingParams validates params for ai.code actions.
func validateCodingParams(prefix string, params map[string]any) []ValidationError {
	var errs []ValidationError
	if params == nil {
		return errs
	}

	// Validate system_prompt path if present
	if sp, ok := params["system_prompt"]; ok {
		if s, ok := sp.(string); ok {
			errs = append(errs, validatePromptPath(prefix+".params.system_prompt", s)...)
		}
	}

	return errs
}

// validateMergeParams validates params for github.merge actions.
func validateMergeParams(prefix string, params map[string]any) []ValidationError {
	var errs []ValidationError
	if params == nil {
		return errs
	}

	if method, ok := params["method"]; ok {
		if s, ok := method.(string); ok {
			switch s {
			case "rebase", "squash", "merge":
				// valid
			default:
				errs = append(errs, ValidationError{
					Field:   prefix + ".params.method",
					Message: fmt.Sprintf("unknown merge method %q (must be rebase, squash, or merge)", s),
				})
			}
		}
	}

	return errs
}

// validateCommentIssueParams validates params for github.comment_issue actions.
func validateCommentIssueParams(prefix string, params map[string]any) []ValidationError {
	var errs []ValidationError

	if params == nil {
		errs = append(errs, ValidationError{
			Field:   prefix + ".params.body",
			Message: "body is required for github.comment_issue action",
		})
		return errs
	}

	body, ok := params["body"]
	if !ok || body == nil {
		errs = append(errs, ValidationError{
			Field:   prefix + ".params.body",
			Message: "body is required for github.comment_issue action",
		})
		return errs
	}

	if s, ok := body.(string); ok {
		if s == "" {
			errs = append(errs, ValidationError{
				Field:   prefix + ".params.body",
				Message: "body must not be empty",
			})
		} else {
			errs = append(errs, validatePromptPath(prefix+".params.body", s)...)
		}
	}

	return errs
}

// validateCIParams validates params for ci.complete events.
func validateCIParams(prefix string, params map[string]any) []ValidationError {
	var errs []ValidationError
	if params == nil {
		return errs
	}

	if onFailure, ok := params["on_failure"]; ok {
		if s, ok := onFailure.(string); ok {
			switch s {
			case "abandon", "retry", "notify":
				// valid
			default:
				errs = append(errs, ValidationError{
					Field:   prefix + ".params.on_failure",
					Message: fmt.Sprintf("unknown on_failure policy %q (must be abandon, retry, or notify)", s),
				})
			}
		}
	}

	return errs
}

// validateSource validates the source configuration.
func validateSource(cfg *Config) []ValidationError {
	var errs []ValidationError

	switch cfg.Source.Provider {
	case "github", "asana", "linear":
		// valid
	case "":
		errs = append(errs, ValidationError{
			Field:   "source.provider",
			Message: "provider is required",
		})
	default:
		errs = append(errs, ValidationError{
			Field:   "source.provider",
			Message: fmt.Sprintf("unknown provider %q (must be github, asana, or linear)", cfg.Source.Provider),
		})
	}

	// Provider-specific filter requirements
	switch cfg.Source.Provider {
	case "github":
		if cfg.Source.Filter.Label == "" {
			errs = append(errs, ValidationError{
				Field:   "source.filter.label",
				Message: "label is required for github provider",
			})
		}
	case "asana":
		if cfg.Source.Filter.Project == "" {
			errs = append(errs, ValidationError{
				Field:   "source.filter.project",
				Message: "project is required for asana provider",
			})
		}
	case "linear":
		if cfg.Source.Filter.Team == "" {
			errs = append(errs, ValidationError{
				Field:   "source.filter.team",
				Message: "team is required for linear provider",
			})
		}
	}

	return errs
}

// validatePromptPath checks that a file: path doesn't escape the repo root.
func validatePromptPath(field, value string) []ValidationError {
	if value == "" || !strings.HasPrefix(value, "file:") {
		return nil
	}

	path := strings.TrimPrefix(value, "file:")
	cleaned := filepath.Clean(path)

	if filepath.IsAbs(cleaned) {
		return []ValidationError{{
			Field:   field,
			Message: "file path must be relative (not absolute)",
		}}
	}

	if strings.HasPrefix(cleaned, "..") {
		return []ValidationError{{
			Field:   field,
			Message: "file path must not escape the repository root",
		}}
	}

	return nil
}
