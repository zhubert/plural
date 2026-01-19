package demo

import (
	"testing"
	"time"
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
