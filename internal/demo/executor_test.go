package demo

import (
	"testing"
	"time"

	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/mcp"
	"github.com/zhubert/plural/internal/ui"
)

func TestExecutorDefaultConfig(t *testing.T) {
	cfg := DefaultExecutorConfig()

	if cfg.CaptureEveryStep {
		t.Error("CaptureEveryStep should be false by default")
	}

	if cfg.TypeDelay != 50*time.Millisecond {
		t.Errorf("TypeDelay = %v, want 50ms", cfg.TypeDelay)
	}

	if cfg.KeyDelay != 100*time.Millisecond {
		t.Errorf("KeyDelay = %v, want 100ms", cfg.KeyDelay)
	}

	if cfg.ResponseChunkDelay != 30*time.Millisecond {
		t.Errorf("ResponseChunkDelay = %v, want 30ms", cfg.ResponseChunkDelay)
	}
}

func TestExecutorRun(t *testing.T) {
	scenario := &Scenario{
		Name:        "test",
		Description: "Test scenario",
		Width:       80,
		Height:      24,
		Setup: &ScenarioSetup{
			Repos: []string{"/test/repo"},
			Sessions: []config.Session{
				{
					ID:        "test-session",
					RepoPath:  "/test/repo",
					WorkTree:  "/test/.worktrees/test-session",
					Branch:    "test-branch",
					Name:      "test/session",
					CreatedAt: time.Now(),
					Started:   true,
				},
			},
			Focus: "sidebar",
		},
		Steps: []Step{
			Wait(100 * time.Millisecond),
			Key("enter"),
			Wait(100 * time.Millisecond),
		},
	}

	cfg := DefaultExecutorConfig()
	cfg.CaptureEveryStep = true

	executor := NewExecutor(cfg)
	frames, err := executor.Run(scenario)

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should have at least the initial frame + frames from steps
	if len(frames) < 3 {
		t.Errorf("Expected at least 3 frames, got %d", len(frames))
	}

	// First frame should have initial delay
	if frames[0].Delay != 500*time.Millisecond {
		t.Errorf("First frame delay = %v, want 500ms", frames[0].Delay)
	}
}

func TestExecutorRunInvalidScenario(t *testing.T) {
	scenario := &Scenario{
		// Missing Name - should fail validation
		Description: "Invalid",
	}

	executor := NewExecutor(DefaultExecutorConfig())
	_, err := executor.Run(scenario)

	if err == nil {
		t.Error("Run() should return error for invalid scenario")
	}
}

func TestExecutorNoCaptureEveryStep(t *testing.T) {
	// Note: CaptureEveryStep only affects Key steps, not Type steps.
	// Type steps always capture frames for character-by-character animation.
	scenario := &Scenario{
		Name:   "minimal",
		Width:  80,
		Height: 24,
		Setup: &ScenarioSetup{
			Repos: []string{"/test/repo"},
			Sessions: []config.Session{
				{
					ID:        "test-session",
					RepoPath:  "/test/repo",
					WorkTree:  "/test/.worktrees/test-session",
					Branch:    "test-branch",
					Name:      "test/session",
					CreatedAt: time.Now(),
					Started:   true,
				},
			},
		},
		Steps: []Step{
			Key("enter"),
			Key("down"),
			Key("up"),
			Wait(100 * time.Millisecond),
		},
	}

	// With CaptureEveryStep = true
	cfg := DefaultExecutorConfig()
	cfg.CaptureEveryStep = true
	executor := NewExecutor(cfg)
	framesWithCapture, _ := executor.Run(scenario)

	// With CaptureEveryStep = false
	cfg.CaptureEveryStep = false
	executor2 := NewExecutor(cfg)
	framesWithoutCapture, _ := executor2.Run(scenario)

	// Should have fewer frames when not capturing every step (3 fewer for the 3 key presses)
	if len(framesWithoutCapture) >= len(framesWithCapture) {
		t.Errorf("Expected fewer frames without capture every step: with=%d, without=%d",
			len(framesWithCapture), len(framesWithoutCapture))
	}
}

func TestKeyPress(t *testing.T) {
	tests := []struct {
		key      string
		wantCode any
	}{
		{"enter", nil},   // Just checking it doesn't panic
		{"tab", nil},
		{"escape", nil},
		{"esc", nil},
		{"backspace", nil},
		{"up", nil},
		{"down", nil},
		{"left", nil},
		{"right", nil},
		{"home", nil},
		{"end", nil},
		{"pgup", nil},
		{"pgdown", nil},
		{"space", nil},
		{"ctrl+c", nil},
		{"ctrl+v", nil},
		{"ctrl+s", nil},
		{"ctrl+o", nil},
		{"ctrl+p", nil},
		{"shift+tab", nil},
		{"a", nil},
		{"1", nil},
		{"/", nil},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			// Just verify it doesn't panic
			msg := keyPress(tt.key)
			_ = msg
		})
	}
}

func TestExecutorNewStepTypes(t *testing.T) {
	// Create a scenario that uses all the new step types
	scenario := &Scenario{
		Name:        "new-steps-test",
		Description: "Test new step types",
		Width:       80,
		Height:      24,
		Setup: &ScenarioSetup{
			Repos: []string{"/test/repo"},
			Sessions: []config.Session{
				{
					ID:        "test-session",
					RepoPath:  "/test/repo",
					WorkTree:  "/test/.worktrees/test-session",
					Branch:    "test-branch",
					Name:      "test/session",
					CreatedAt: time.Now(),
					Started:   true,
				},
			},
			Focus: "sidebar",
		},
		Steps: []Step{
			// Select the session first
			Key("enter"),
			Wait(100 * time.Millisecond),

			// Test Question step
			Question(mcp.Question{
				Question: "Pick one",
				Header:   "Choice",
				Options: []mcp.QuestionOption{
					{Label: "Option A", Description: "First option"},
					{Label: "Option B", Description: "Second option"},
				},
			}),

			// Dismiss question
			Key("enter"),
			Wait(100 * time.Millisecond),

			// Test PlanApproval step
			PlanApproval("## Plan\n1. Do stuff",
				mcp.AllowedPrompt{Tool: "Bash", Prompt: "run tests"},
			),

			// Dismiss plan approval
			Key("y"),
			Wait(100 * time.Millisecond),

			// Test TodoList step
			TodoList(
				claude.TodoItem{Content: "Task 1", Status: claude.TodoStatusPending},
				claude.TodoItem{Content: "Task 2", Status: claude.TodoStatusInProgress, ActiveForm: "Working on task 2"},
			),

			// Test ToolUse step
			ToolUse("Read", "file.go"),
			ToolUse("Edit", "file.go"),

			// Test Flash step
			FlashSuccess("Operation completed"),

			// Test Permission step (already exists but include for completeness)
			Permission("Bash", "ls -la"),

			// Dismiss permission
			Key("y"),

			// Annotate step
			Annotate("This is an annotation"),
			Capture(),
		},
	}

	cfg := DefaultExecutorConfig()
	cfg.CaptureEveryStep = true

	executor := NewExecutor(cfg)
	frames, err := executor.Run(scenario)

	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should have captured multiple frames
	if len(frames) < 10 {
		t.Errorf("Expected at least 10 frames, got %d", len(frames))
	}

	// Verify the annotated frame has the annotation
	foundAnnotation := false
	for _, frame := range frames {
		if frame.Annotation == "This is an annotation" {
			foundAnnotation = true
			break
		}
	}
	if !foundAnnotation {
		t.Error("Did not find frame with annotation 'This is an annotation'")
	}
}

func TestExecutorStepWithoutSession(t *testing.T) {
	// Test that step types requiring a session fail gracefully when no session is selected
	tests := []struct {
		name     string
		step     Step
		wantErr  bool
		errMatch string
	}{
		{
			name:     "Question without session",
			step:     Question(mcp.Question{Question: "test"}),
			wantErr:  true,
			errMatch: "no active session",
		},
		{
			name:     "PlanApproval without session",
			step:     PlanApproval("plan"),
			wantErr:  true,
			errMatch: "no active session",
		},
		{
			name:     "TodoList without session",
			step:     TodoList(claude.TodoItem{Content: "task"}),
			wantErr:  true,
			errMatch: "no active session",
		},
		{
			name:     "ToolUse without session",
			step:     ToolUse("Read", "file"),
			wantErr:  true,
			errMatch: "no active session",
		},
		{
			name:     "Flash without session (should work)",
			step:     Flash("message", ui.FlashInfo),
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scenario := &Scenario{
				Name:   tt.name,
				Width:  80,
				Height: 24,
				Setup: &ScenarioSetup{
					Repos: []string{"/test/repo"},
					Sessions: []config.Session{
						{
							ID:        "test-session",
							RepoPath:  "/test/repo",
							WorkTree:  "/test/.worktrees/test-session",
							Branch:    "test-branch",
							Name:      "test/session",
							CreatedAt: time.Now(),
							Started:   true,
						},
					},
					Focus: "sidebar",
				},
				// Don't select a session - go straight to the step
				Steps: []Step{tt.step},
			}

			executor := NewExecutor(DefaultExecutorConfig())
			_, err := executor.Run(scenario)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Run() expected error containing %q, got nil", tt.errMatch)
				}
			} else {
				if err != nil {
					t.Errorf("Run() unexpected error: %v", err)
				}
			}
		})
	}
}
