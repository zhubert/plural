package scenarios

import (
	"testing"

	"github.com/zhubert/plural/internal/demo"
)

func TestAll(t *testing.T) {
	scenarios := All()

	if len(scenarios) != 2 {
		t.Errorf("All() should return 2 scenarios, got %d", len(scenarios))
	}

	// Verify each scenario is valid
	for _, s := range scenarios {
		if err := s.Validate(); err != nil {
			t.Errorf("Scenario %q validation failed: %v", s.Name, err)
		}
	}
}

func TestGet(t *testing.T) {
	tests := []struct {
		name      string
		wantFound bool
	}{
		{"basic", true},
		{"comprehensive", true},
		{"nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scenario := Get(tt.name)
			found := scenario != nil

			if found != tt.wantFound {
				t.Errorf("Get(%q) found = %v, want %v", tt.name, found, tt.wantFound)
			}
		})
	}
}

func TestBasicScenario(t *testing.T) {
	scenario := Basic

	if scenario.Name != "basic" {
		t.Errorf("Name = %v, want 'basic'", scenario.Name)
	}

	if scenario.Width != 120 {
		t.Errorf("Width = %v, want 120", scenario.Width)
	}

	if len(scenario.Steps) == 0 {
		t.Error("Steps should not be empty")
	}

	if scenario.Setup == nil {
		t.Error("Setup should not be nil")
	}

	if len(scenario.Setup.Sessions) == 0 {
		t.Error("Setup.Sessions should not be empty")
	}

	// Should have multiple repos
	if len(scenario.Setup.Repos) < 2 {
		t.Errorf("Basic scenario should have multiple repos, got %d", len(scenario.Setup.Repos))
	}

	// Should have multiple sessions across repos
	if len(scenario.Setup.Sessions) < 3 {
		t.Errorf("Basic scenario should have at least 3 sessions, got %d", len(scenario.Setup.Sessions))
	}

	// Should have a streaming response step (for Claude's response)
	hasStreamingResponse := false
	for _, step := range scenario.Steps {
		if step.Type == demo.StepResponse && len(step.Chunks) > 0 {
			hasStreamingResponse = true
			break
		}
	}

	if !hasStreamingResponse {
		t.Error("Basic scenario should have a streaming response step")
	}

	// Should have type steps (for user typing message)
	hasTypeStep := false
	for _, step := range scenario.Steps {
		if step.Type == demo.StepTypeText {
			hasTypeStep = true
			break
		}
	}

	if !hasTypeStep {
		t.Error("Basic scenario should have a Type step for user input")
	}
}

func TestComprehensiveScenario(t *testing.T) {
	scenario := Comprehensive

	if scenario.Name != "comprehensive" {
		t.Errorf("Name = %v, want 'comprehensive'", scenario.Name)
	}

	if scenario.Width != 120 {
		t.Errorf("Width = %v, want 120", scenario.Width)
	}

	if len(scenario.Steps) == 0 {
		t.Error("Steps should not be empty")
	}

	if scenario.Setup == nil {
		t.Error("Setup should not be nil")
	}

	// Should have multiple repos
	if len(scenario.Setup.Repos) < 2 {
		t.Errorf("Comprehensive scenario should have multiple repos, got %d", len(scenario.Setup.Repos))
	}

	// Should have multiple sessions (at least 3 for variety)
	if len(scenario.Setup.Sessions) < 3 {
		t.Errorf("Comprehensive scenario should have at least 3 sessions, got %d", len(scenario.Setup.Sessions))
	}

	// Should have a streaming response step (for Claude's options response)
	hasStreamingResponse := false
	for _, step := range scenario.Steps {
		if step.Type == demo.StepResponse && len(step.Chunks) > 0 {
			hasStreamingResponse = true
			break
		}
	}

	if !hasStreamingResponse {
		t.Error("Comprehensive scenario should have a streaming response step")
	}

	// Should have type steps (for user typing question)
	hasTypeStep := false
	for _, step := range scenario.Steps {
		if step.Type == demo.StepTypeText {
			hasTypeStep = true
			break
		}
	}

	if !hasTypeStep {
		t.Error("Comprehensive scenario should have a Type step for user input")
	}
}
