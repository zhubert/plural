package workflow

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	validDefault := DefaultWorkflowConfig()

	tests := []struct {
		name       string
		cfg        *Config
		wantFields []string // expected error fields (empty = no errors)
	}{
		{
			name:       "valid default config",
			cfg:        validDefault,
			wantFields: nil,
		},
		{
			name: "valid github config",
			cfg: &Config{
				Start: "coding",
				Source: SourceConfig{
					Provider: "github",
					Filter:   FilterConfig{Label: "queued"},
				},
				States: map[string]*State{
					"coding": {Type: StateTypeTask, Action: "ai.code", Next: "done", Error: "failed"},
					"done":   {Type: StateTypeSucceed},
					"failed": {Type: StateTypeFail},
				},
			},
			wantFields: nil,
		},
		{
			name: "valid asana config",
			cfg: &Config{
				Start: "coding",
				Source: SourceConfig{
					Provider: "asana",
					Filter:   FilterConfig{Project: "12345"},
				},
				States: map[string]*State{
					"coding": {Type: StateTypeTask, Action: "ai.code", Next: "done"},
					"done":   {Type: StateTypeSucceed},
				},
			},
			wantFields: nil,
		},
		{
			name: "valid linear config",
			cfg: &Config{
				Start: "coding",
				Source: SourceConfig{
					Provider: "linear",
					Filter:   FilterConfig{Team: "my-team"},
				},
				States: map[string]*State{
					"coding": {Type: StateTypeTask, Action: "ai.code", Next: "done"},
					"done":   {Type: StateTypeSucceed},
				},
			},
			wantFields: nil,
		},
		{
			name:       "empty provider",
			cfg:        &Config{Start: "s", States: map[string]*State{"s": {Type: StateTypeSucceed}}},
			wantFields: []string{"source.provider"},
		},
		{
			name: "unknown provider",
			cfg: &Config{
				Start:  "s",
				Source: SourceConfig{Provider: "jira"},
				States: map[string]*State{"s": {Type: StateTypeSucceed}},
			},
			wantFields: []string{"source.provider"},
		},
		{
			name: "github missing label",
			cfg: &Config{
				Start:  "s",
				Source: SourceConfig{Provider: "github"},
				States: map[string]*State{"s": {Type: StateTypeSucceed}},
			},
			wantFields: []string{"source.filter.label"},
		},
		{
			name: "asana missing project",
			cfg: &Config{
				Start:  "s",
				Source: SourceConfig{Provider: "asana"},
				States: map[string]*State{"s": {Type: StateTypeSucceed}},
			},
			wantFields: []string{"source.filter.project"},
		},
		{
			name: "linear missing team",
			cfg: &Config{
				Start:  "s",
				Source: SourceConfig{Provider: "linear"},
				States: map[string]*State{"s": {Type: StateTypeSucceed}},
			},
			wantFields: []string{"source.filter.team"},
		},
		{
			name:       "missing start",
			cfg:        &Config{States: map[string]*State{"s": {Type: StateTypeSucceed}}, Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}}},
			wantFields: []string{"start"},
		},
		{
			name: "start references non-existent state",
			cfg: &Config{
				Start:  "nonexistent",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{"coding": {Type: StateTypeTask, Action: "ai.code", Next: "done"}, "done": {Type: StateTypeSucceed}},
			},
			wantFields: []string{"start"},
		},
		{
			name: "no states",
			cfg: &Config{
				Start:  "coding",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
			},
			wantFields: []string{"states"},
		},
		{
			name: "task missing action",
			cfg: &Config{
				Start:  "t",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"t":    {Type: StateTypeTask, Next: "done"},
					"done": {Type: StateTypeSucceed},
				},
			},
			wantFields: []string{"states.t.action"},
		},
		{
			name: "task unknown action",
			cfg: &Config{
				Start:  "t",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"t":    {Type: StateTypeTask, Action: "unknown.action", Next: "done"},
					"done": {Type: StateTypeSucceed},
				},
			},
			wantFields: []string{"states.t.action"},
		},
		{
			name: "task missing next",
			cfg: &Config{
				Start:  "t",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"t": {Type: StateTypeTask, Action: "ai.code"},
				},
			},
			wantFields: []string{"states.t.next"},
		},
		{
			name: "wait missing event",
			cfg: &Config{
				Start:  "w",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"w":    {Type: StateTypeWait, Next: "done"},
					"done": {Type: StateTypeSucceed},
				},
			},
			wantFields: []string{"states.w.event"},
		},
		{
			name: "wait unknown event",
			cfg: &Config{
				Start:  "w",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"w":    {Type: StateTypeWait, Event: "unknown.event", Next: "done"},
					"done": {Type: StateTypeSucceed},
				},
			},
			wantFields: []string{"states.w.event"},
		},
		{
			name: "terminal state with next",
			cfg: &Config{
				Start:  "done",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"done": {Type: StateTypeSucceed, Next: "other"},
				},
			},
			wantFields: []string{"states.done.next"},
		},
		{
			name: "next references non-existent state",
			cfg: &Config{
				Start:  "t",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"t": {Type: StateTypeTask, Action: "ai.code", Next: "nonexistent"},
				},
			},
			wantFields: []string{"states.t.next"},
		},
		{
			name: "error references non-existent state",
			cfg: &Config{
				Start:  "t",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"t":    {Type: StateTypeTask, Action: "ai.code", Next: "done", Error: "nonexistent"},
					"done": {Type: StateTypeSucceed},
				},
			},
			wantFields: []string{"states.t.error"},
		},
		{
			name: "invalid merge method in params",
			cfg: &Config{
				Start:  "m",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"m":    {Type: StateTypeTask, Action: "github.merge", Params: map[string]any{"method": "yolo"}, Next: "done"},
					"done": {Type: StateTypeSucceed},
				},
			},
			wantFields: []string{"states.m.params.method"},
		},
		{
			name: "invalid on_failure in ci params",
			cfg: &Config{
				Start:  "ci",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"ci":   {Type: StateTypeWait, Event: "ci.complete", Params: map[string]any{"on_failure": "explode"}, Next: "done"},
					"done": {Type: StateTypeSucceed},
				},
			},
			wantFields: []string{"states.ci.params.on_failure"},
		},
		{
			name: "system prompt absolute path in coding params",
			cfg: &Config{
				Start:  "c",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"c":    {Type: StateTypeTask, Action: "ai.code", Params: map[string]any{"system_prompt": "file:/etc/passwd"}, Next: "done"},
					"done": {Type: StateTypeSucceed},
				},
			},
			wantFields: []string{"states.c.params.system_prompt"},
		},
		{
			name: "system prompt path traversal",
			cfg: &Config{
				Start:  "c",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"c":    {Type: StateTypeTask, Action: "ai.code", Params: map[string]any{"system_prompt": "file:../../etc/passwd"}, Next: "done"},
					"done": {Type: StateTypeSucceed},
				},
			},
			wantFields: []string{"states.c.params.system_prompt"},
		},
		{
			name: "valid system prompt file path",
			cfg: &Config{
				Start:  "c",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"c":    {Type: StateTypeTask, Action: "ai.code", Params: map[string]any{"system_prompt": "file:./prompts/coding.md"}, Next: "done"},
					"done": {Type: StateTypeSucceed},
				},
			},
			wantFields: nil,
		},
		{
			name: "unknown state type",
			cfg: &Config{
				Start:  "x",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"x": {Type: "bogus"},
				},
			},
			wantFields: []string{"states.x.type"},
		},
		{
			name: "github.comment_issue missing params",
			cfg: &Config{
				Start:  "c",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"c":    {Type: StateTypeTask, Action: "github.comment_issue", Next: "done"},
					"done": {Type: StateTypeSucceed},
				},
			},
			wantFields: []string{"states.c.params.body"},
		},
		{
			name: "github.comment_issue missing body param",
			cfg: &Config{
				Start:  "c",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"c":    {Type: StateTypeTask, Action: "github.comment_issue", Params: map[string]any{"other": "value"}, Next: "done"},
					"done": {Type: StateTypeSucceed},
				},
			},
			wantFields: []string{"states.c.params.body"},
		},
		{
			name: "github.comment_issue empty body",
			cfg: &Config{
				Start:  "c",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"c":    {Type: StateTypeTask, Action: "github.comment_issue", Params: map[string]any{"body": ""}, Next: "done"},
					"done": {Type: StateTypeSucceed},
				},
			},
			wantFields: []string{"states.c.params.body"},
		},
		{
			name: "github.comment_issue valid body",
			cfg: &Config{
				Start:  "c",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"c":    {Type: StateTypeTask, Action: "github.comment_issue", Params: map[string]any{"body": "Starting work on this issue!"}, Next: "done"},
					"done": {Type: StateTypeSucceed},
				},
			},
			wantFields: nil,
		},
		{
			name: "github.comment_issue valid file body path",
			cfg: &Config{
				Start:  "c",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"c":    {Type: StateTypeTask, Action: "github.comment_issue", Params: map[string]any{"body": "file:templates/comment.md"}, Next: "done"},
					"done": {Type: StateTypeSucceed},
				},
			},
			wantFields: nil,
		},
		{
			name: "github.comment_issue body absolute path",
			cfg: &Config{
				Start:  "c",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"c":    {Type: StateTypeTask, Action: "github.comment_issue", Params: map[string]any{"body": "file:/etc/passwd"}, Next: "done"},
					"done": {Type: StateTypeSucceed},
				},
			},
			wantFields: []string{"states.c.params.body"},
		},
		{
			name: "github.comment_issue body path traversal",
			cfg: &Config{
				Start:  "c",
				Source: SourceConfig{Provider: "github", Filter: FilterConfig{Label: "q"}},
				States: map[string]*State{
					"c":    {Type: StateTypeTask, Action: "github.comment_issue", Params: map[string]any{"body": "file:../../etc/passwd"}, Next: "done"},
					"done": {Type: StateTypeSucceed},
				},
			},
			wantFields: []string{"states.c.params.body"},
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
