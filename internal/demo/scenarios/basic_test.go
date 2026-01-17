package scenarios

import (
	"testing"

	"github.com/zhubert/plural/internal/demo"
)

func TestAll(t *testing.T) {
	scenarios := All()

	if len(scenarios) == 0 {
		t.Error("All() should return at least one scenario")
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
		{"parallel", true},
		{"permission", true},
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
}

func TestParallelScenario(t *testing.T) {
	scenario := Parallel

	if scenario.Name != "parallel" {
		t.Errorf("Name = %v, want 'parallel'", scenario.Name)
	}

	// Should have multiple sessions for parallel demo
	if len(scenario.Setup.Sessions) < 2 {
		t.Errorf("Parallel scenario should have at least 2 sessions, got %d", len(scenario.Setup.Sessions))
	}
}

func TestPermissionScenario(t *testing.T) {
	scenario := Permission

	if scenario.Name != "permission" {
		t.Errorf("Name = %v, want 'permission'", scenario.Name)
	}

	// Should have a permission step
	hasPermission := false
	for _, step := range scenario.Steps {
		if step.Type == 4 { // StepPermission
			hasPermission = true
			break
		}
	}

	if !hasPermission {
		t.Error("Permission scenario should have a permission step")
	}
}

func TestComprehensiveScenario(t *testing.T) {
	scenario := Comprehensive

	if scenario.Name != "comprehensive" {
		t.Errorf("Name = %v, want 'comprehensive'", scenario.Name)
	}

	// Should have multiple sessions
	if len(scenario.Setup.Sessions) < 2 {
		t.Errorf("Comprehensive scenario should have at least 2 sessions, got %d", len(scenario.Setup.Sessions))
	}

	// Should have a StartStreaming step (to demonstrate parallel work)
	hasStartStreaming := false
	for _, step := range scenario.Steps {
		if step.Type == demo.StepStartStreaming {
			hasStartStreaming = true
		}
	}

	if !hasStartStreaming {
		t.Error("Comprehensive scenario should have a StartStreaming step to demonstrate parallel work")
	}
}

func TestGetComprehensive(t *testing.T) {
	scenario := Get("comprehensive")
	if scenario == nil {
		t.Error("Get('comprehensive') should return a scenario")
	}
}
