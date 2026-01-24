package demo

import (
	"testing"
	"time"

	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/mcp"
	"github.com/zhubert/plural/internal/ui"
)

func TestScenarioValidate(t *testing.T) {
	tests := []struct {
		name      string
		scenario  *Scenario
		wantErr   bool
		errField  string
		wantWidth int
	}{
		{
			name: "valid scenario",
			scenario: &Scenario{
				Name:        "test",
				Description: "Test scenario",
				Width:       100,
				Height:      30,
				Setup:       DefaultSetup(),
			},
			wantErr:   false,
			wantWidth: 100,
		},
		{
			name: "missing name",
			scenario: &Scenario{
				Description: "Test scenario",
			},
			wantErr:  true,
			errField: "Name",
		},
		{
			name: "default width and height",
			scenario: &Scenario{
				Name:        "test",
				Description: "Test scenario",
			},
			wantErr:   false,
			wantWidth: 120, // Default
		},
		{
			name: "default setup",
			scenario: &Scenario{
				Name: "test",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.scenario.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil {
				if ve, ok := err.(*ValidationError); ok {
					if ve.Field != tt.errField {
						t.Errorf("Validate() error field = %v, want %v", ve.Field, tt.errField)
					}
				}
			}
			if !tt.wantErr && tt.wantWidth > 0 {
				if tt.scenario.Width != tt.wantWidth {
					t.Errorf("Width = %v, want %v", tt.scenario.Width, tt.wantWidth)
				}
			}
		})
	}
}

func TestStepBuilders(t *testing.T) {
	t.Run("Wait", func(t *testing.T) {
		step := Wait(500 * time.Millisecond)
		if step.Type != StepWait {
			t.Errorf("Type = %v, want StepWait", step.Type)
		}
		if step.Duration != 500*time.Millisecond {
			t.Errorf("Duration = %v, want 500ms", step.Duration)
		}
	})

	t.Run("Key", func(t *testing.T) {
		step := Key("enter")
		if step.Type != StepKey {
			t.Errorf("Type = %v, want StepKey", step.Type)
		}
		if step.Key != "enter" {
			t.Errorf("Key = %v, want enter", step.Key)
		}
	})

	t.Run("KeyWithDesc", func(t *testing.T) {
		step := KeyWithDesc("enter", "Submit the form")
		if step.Type != StepKey {
			t.Errorf("Type = %v, want StepKey", step.Type)
		}
		if step.Description != "Submit the form" {
			t.Errorf("Description = %v, want 'Submit the form'", step.Description)
		}
	})

	t.Run("Type", func(t *testing.T) {
		step := Type("hello world")
		if step.Type != StepTypeText {
			t.Errorf("Type = %v, want StepTypeText", step.Type)
		}
		if step.Text != "hello world" {
			t.Errorf("Text = %v, want 'hello world'", step.Text)
		}
	})

	t.Run("StreamingTextResponse", func(t *testing.T) {
		step := StreamingTextResponse("Hello World", 5)
		if step.Type != StepResponse {
			t.Errorf("Type = %v, want StepResponse", step.Type)
		}
		// "Hello World" (11 chars) with chunk size 5 should be 3 chunks + 1 done
		// "Hello", " Worl", "d", done
		if len(step.Chunks) != 4 {
			t.Errorf("Chunks length = %v, want 4, chunks: %+v", len(step.Chunks), step.Chunks)
		}
		if step.Chunks[0].Content != "Hello" {
			t.Errorf("First chunk = %v, want 'Hello'", step.Chunks[0].Content)
		}
	})

	t.Run("Capture", func(t *testing.T) {
		step := Capture()
		if step.Type != StepCapture {
			t.Errorf("Type = %v, want StepCapture", step.Type)
		}
	})

	t.Run("Annotate", func(t *testing.T) {
		step := Annotate("This is a caption")
		if step.Type != StepAnnotate {
			t.Errorf("Type = %v, want StepAnnotate", step.Type)
		}
		if step.Annotation != "This is a caption" {
			t.Errorf("Annotation = %v, want 'This is a caption'", step.Annotation)
		}
	})

	t.Run("Permission", func(t *testing.T) {
		step := Permission("Bash", "run tests")
		if step.Type != StepPermission {
			t.Errorf("Type = %v, want StepPermission", step.Type)
		}
		if step.PermissionTool != "Bash" {
			t.Errorf("PermissionTool = %v, want 'Bash'", step.PermissionTool)
		}
		if step.PermissionDescription != "run tests" {
			t.Errorf("PermissionDescription = %v, want 'run tests'", step.PermissionDescription)
		}
	})

	t.Run("Question", func(t *testing.T) {
		questions := []mcp.Question{
			{Question: "Pick one", Header: "Choice", Options: []mcp.QuestionOption{{Label: "A"}, {Label: "B"}}},
		}
		step := Question(questions...)
		if step.Type != StepQuestion {
			t.Errorf("Type = %v, want StepQuestion", step.Type)
		}
		if len(step.Questions) != 1 {
			t.Errorf("Questions length = %v, want 1", len(step.Questions))
		}
		if step.Questions[0].Question != "Pick one" {
			t.Errorf("Question = %v, want 'Pick one'", step.Questions[0].Question)
		}
	})

	t.Run("PlanApproval", func(t *testing.T) {
		step := PlanApproval("## Plan\n1. Do stuff", mcp.AllowedPrompt{Tool: "Bash", Prompt: "test"})
		if step.Type != StepPlanApproval {
			t.Errorf("Type = %v, want StepPlanApproval", step.Type)
		}
		if step.Plan != "## Plan\n1. Do stuff" {
			t.Errorf("Plan = %v, want '## Plan\\n1. Do stuff'", step.Plan)
		}
		if len(step.AllowedPrompts) != 1 {
			t.Errorf("AllowedPrompts length = %v, want 1", len(step.AllowedPrompts))
		}
	})

	t.Run("TodoList", func(t *testing.T) {
		items := []claude.TodoItem{
			{Content: "Task 1", Status: claude.TodoStatusPending},
			{Content: "Task 2", Status: claude.TodoStatusInProgress},
		}
		step := TodoList(items...)
		if step.Type != StepTodoList {
			t.Errorf("Type = %v, want StepTodoList", step.Type)
		}
		if len(step.TodoItems) != 2 {
			t.Errorf("TodoItems length = %v, want 2", len(step.TodoItems))
		}
	})

	t.Run("Flash", func(t *testing.T) {
		step := Flash("Test message", ui.FlashSuccess)
		if step.Type != StepFlash {
			t.Errorf("Type = %v, want StepFlash", step.Type)
		}
		if step.FlashText != "Test message" {
			t.Errorf("FlashText = %v, want 'Test message'", step.FlashText)
		}
		if step.FlashType != ui.FlashSuccess {
			t.Errorf("FlashType = %v, want FlashSuccess", step.FlashType)
		}
	})

	t.Run("FlashSuccess", func(t *testing.T) {
		step := FlashSuccess("Success!")
		if step.Type != StepFlash {
			t.Errorf("Type = %v, want StepFlash", step.Type)
		}
		if step.FlashType != ui.FlashSuccess {
			t.Errorf("FlashType = %v, want FlashSuccess", step.FlashType)
		}
	})

	t.Run("FlashError", func(t *testing.T) {
		step := FlashError("Error!")
		if step.FlashType != ui.FlashError {
			t.Errorf("FlashType = %v, want FlashError", step.FlashType)
		}
	})

	t.Run("FlashInfo", func(t *testing.T) {
		step := FlashInfo("Info")
		if step.FlashType != ui.FlashInfo {
			t.Errorf("FlashType = %v, want FlashInfo", step.FlashType)
		}
	})

	t.Run("FlashWarning", func(t *testing.T) {
		step := FlashWarning("Warning")
		if step.FlashType != ui.FlashWarning {
			t.Errorf("FlashType = %v, want FlashWarning", step.FlashType)
		}
	})

	t.Run("ToolUse", func(t *testing.T) {
		step := ToolUse("Read", "path/to/file.go")
		if step.Type != StepToolUse {
			t.Errorf("Type = %v, want StepToolUse", step.Type)
		}
		if step.ToolName != "Read" {
			t.Errorf("ToolName = %v, want 'Read'", step.ToolName)
		}
		if step.ToolInput != "path/to/file.go" {
			t.Errorf("ToolInput = %v, want 'path/to/file.go'", step.ToolInput)
		}
	})

	t.Run("CommitMessage", func(t *testing.T) {
		step := CommitMessage("Add new feature\n\nDetailed description here")
		if step.Type != StepCommitMessage {
			t.Errorf("Type = %v, want StepCommitMessage", step.Type)
		}
		if step.CommitMessage != "Add new feature\n\nDetailed description here" {
			t.Errorf("CommitMessage = %v, want 'Add new feature\\n\\nDetailed description here'", step.CommitMessage)
		}
	})
}

func TestDefaultSetup(t *testing.T) {
	setup := DefaultSetup()

	if len(setup.Repos) != 1 {
		t.Errorf("Repos length = %v, want 1", len(setup.Repos))
	}

	if len(setup.Sessions) != 1 {
		t.Errorf("Sessions length = %v, want 1", len(setup.Sessions))
	}

	if setup.Focus != "sidebar" {
		t.Errorf("Focus = %v, want 'sidebar'", setup.Focus)
	}
}

func TestValidationError(t *testing.T) {
	err := &ValidationError{
		Field:   "Name",
		Message: "is required",
	}

	expected := "validation error: Name: is required"
	if err.Error() != expected {
		t.Errorf("Error() = %v, want %v", err.Error(), expected)
	}
}
