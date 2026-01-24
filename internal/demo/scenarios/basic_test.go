package scenarios

import (
	"testing"

	"github.com/zhubert/plural/internal/demo"
)

func TestAll(t *testing.T) {
	scenarios := All()

	if len(scenarios) != 1 {
		t.Errorf("All() should return 1 scenario, got %d", len(scenarios))
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
		{"overview", true},
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

func TestOverviewScenario(t *testing.T) {
	scenario := Overview

	if scenario.Name != "overview" {
		t.Errorf("Name = %v, want 'overview'", scenario.Name)
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

	// Should have multiple repos (3 for webapp, api-service, mobile-app)
	if len(scenario.Setup.Repos) < 3 {
		t.Errorf("Overview scenario should have at least 3 repos, got %d", len(scenario.Setup.Repos))
	}

	// Should have multiple sessions (6 across repos)
	if len(scenario.Setup.Sessions) < 5 {
		t.Errorf("Overview scenario should have at least 5 sessions, got %d", len(scenario.Setup.Sessions))
	}

	// Check for variety of step types that demonstrate all features
	stepTypes := make(map[demo.StepType]bool)
	for _, step := range scenario.Steps {
		stepTypes[step.Type] = true
	}

	// Should have PlanApproval step
	if !stepTypes[demo.StepPlanApproval] {
		t.Error("Overview scenario should have a PlanApproval step")
	}

	// Should have TodoList step
	if !stepTypes[demo.StepTodoList] {
		t.Error("Overview scenario should have a TodoList step")
	}

	// Should have ToolUse step
	if !stepTypes[demo.StepToolUse] {
		t.Error("Overview scenario should have a ToolUse step")
	}

	// Should have Question step
	if !stepTypes[demo.StepQuestion] {
		t.Error("Overview scenario should have a Question step")
	}

	// Should have Permission step
	if !stepTypes[demo.StepPermission] {
		t.Error("Overview scenario should have a Permission step")
	}

	// Should have streaming response step
	if !stepTypes[demo.StepResponse] {
		t.Error("Overview scenario should have a streaming Response step")
	}

	// Should have type steps (for user typing messages)
	if !stepTypes[demo.StepTypeText] {
		t.Error("Overview scenario should have a Type step for user input")
	}

	// Should have a reasonable number of steps for a comprehensive demo
	if len(scenario.Steps) < 40 {
		t.Errorf("Overview scenario should have at least 40 steps for comprehensive coverage, got %d", len(scenario.Steps))
	}
}
