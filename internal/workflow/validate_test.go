package workflow

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name       string
		cfg        *Config
		wantFields []string // expected error fields (empty = no errors)
	}{
		{
			name: "valid github config",
			cfg: &Config{
				Source: SourceConfig{
					Provider: "github",
					Filter:   FilterConfig{Label: "queued"},
				},
				Workflow: WorkflowConfig{
					CI:    CIConfig{OnFailure: "retry"},
					Merge: MergeConfig{Method: "rebase"},
				},
			},
			wantFields: nil,
		},
		{
			name: "valid asana config",
			cfg: &Config{
				Source: SourceConfig{
					Provider: "asana",
					Filter:   FilterConfig{Project: "12345"},
				},
			},
			wantFields: nil,
		},
		{
			name: "valid linear config",
			cfg: &Config{
				Source: SourceConfig{
					Provider: "linear",
					Filter:   FilterConfig{Team: "my-team"},
				},
			},
			wantFields: nil,
		},
		{
			name:       "empty provider",
			cfg:        &Config{},
			wantFields: []string{"source.provider"},
		},
		{
			name: "unknown provider",
			cfg: &Config{
				Source: SourceConfig{Provider: "jira"},
			},
			wantFields: []string{"source.provider"},
		},
		{
			name: "github missing label",
			cfg: &Config{
				Source: SourceConfig{Provider: "github"},
			},
			wantFields: []string{"source.filter.label"},
		},
		{
			name: "asana missing project",
			cfg: &Config{
				Source: SourceConfig{Provider: "asana"},
			},
			wantFields: []string{"source.filter.project"},
		},
		{
			name: "linear missing team",
			cfg: &Config{
				Source: SourceConfig{Provider: "linear"},
			},
			wantFields: []string{"source.filter.team"},
		},
		{
			name: "invalid on_failure",
			cfg: &Config{
				Source: SourceConfig{
					Provider: "github",
					Filter:   FilterConfig{Label: "queued"},
				},
				Workflow: WorkflowConfig{
					CI: CIConfig{OnFailure: "explode"},
				},
			},
			wantFields: []string{"workflow.ci.on_failure"},
		},
		{
			name: "invalid merge method",
			cfg: &Config{
				Source: SourceConfig{
					Provider: "github",
					Filter:   FilterConfig{Label: "queued"},
				},
				Workflow: WorkflowConfig{
					Merge: MergeConfig{Method: "yolo"},
				},
			},
			wantFields: []string{"workflow.merge.method"},
		},
		{
			name: "system prompt absolute path",
			cfg: &Config{
				Source: SourceConfig{
					Provider: "github",
					Filter:   FilterConfig{Label: "queued"},
				},
				Workflow: WorkflowConfig{
					Coding: CodingConfig{SystemPrompt: "file:/etc/passwd"},
				},
			},
			wantFields: []string{"workflow.coding.system_prompt"},
		},
		{
			name: "system prompt path traversal",
			cfg: &Config{
				Source: SourceConfig{
					Provider: "github",
					Filter:   FilterConfig{Label: "queued"},
				},
				Workflow: WorkflowConfig{
					Coding: CodingConfig{SystemPrompt: "file:../../etc/passwd"},
				},
			},
			wantFields: []string{"workflow.coding.system_prompt"},
		},
		{
			name: "inline system prompt is valid",
			cfg: &Config{
				Source: SourceConfig{
					Provider: "github",
					Filter:   FilterConfig{Label: "queued"},
				},
				Workflow: WorkflowConfig{
					Coding: CodingConfig{SystemPrompt: "Be careful with tests"},
				},
			},
			wantFields: nil,
		},
		{
			name: "valid file path",
			cfg: &Config{
				Source: SourceConfig{
					Provider: "github",
					Filter:   FilterConfig{Label: "queued"},
				},
				Workflow: WorkflowConfig{
					Coding: CodingConfig{SystemPrompt: "file:./prompts/coding.md"},
				},
			},
			wantFields: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := Validate(tt.cfg)

			if len(tt.wantFields) == 0 {
				if len(errs) > 0 {
					t.Errorf("expected no errors, got %d: %v", len(errs), errs)
				}
				return
			}

			errFields := make(map[string]bool)
			for _, e := range errs {
				errFields[e.Field] = true
			}

			for _, field := range tt.wantFields {
				if !errFields[field] {
					t.Errorf("expected error for field %q, got errors: %v", field, errs)
				}
			}
		})
	}
}

func TestValidationErrorString(t *testing.T) {
	e := ValidationError{Field: "source.provider", Message: "provider is required"}
	s := e.Error()
	if !strings.Contains(s, "source.provider") || !strings.Contains(s, "provider is required") {
		t.Errorf("unexpected error string: %q", s)
	}
}
