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

	// Provider validation
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

	// CI on_failure
	if cfg.Workflow.CI.OnFailure != "" {
		switch cfg.Workflow.CI.OnFailure {
		case "abandon", "retry", "notify":
			// valid
		default:
			errs = append(errs, ValidationError{
				Field:   "workflow.ci.on_failure",
				Message: fmt.Sprintf("unknown on_failure policy %q (must be abandon, retry, or notify)", cfg.Workflow.CI.OnFailure),
			})
		}
	}

	// Merge method
	if cfg.Workflow.Merge.Method != "" {
		switch cfg.Workflow.Merge.Method {
		case "rebase", "squash", "merge":
			// valid
		default:
			errs = append(errs, ValidationError{
				Field:   "workflow.merge.method",
				Message: fmt.Sprintf("unknown merge method %q (must be rebase, squash, or merge)", cfg.Workflow.Merge.Method),
			})
		}
	}

	// System prompt file paths must not escape repo root
	errs = append(errs, validatePromptPath("workflow.coding.system_prompt", cfg.Workflow.Coding.SystemPrompt)...)
	errs = append(errs, validatePromptPath("workflow.review.system_prompt", cfg.Workflow.Review.SystemPrompt)...)
	errs = append(errs, validatePromptPath("workflow.pr.template", cfg.Workflow.PR.Template)...)

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
