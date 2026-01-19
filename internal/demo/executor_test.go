package demo

import (
	"testing"
	"time"

	"github.com/zhubert/plural/internal/config"
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
			Type("hello"),   // 5 characters typed
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

	// Should have fewer frames when not capturing every step
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
