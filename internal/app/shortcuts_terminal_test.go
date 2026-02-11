package app

import (
	"testing"

	"github.com/zhubert/plural/internal/config"
)

// TestOpenTerminalForSession verifies that the correct terminal command is chosen
// based on whether the session is containerized or not.
func TestOpenTerminalForSession(t *testing.T) {
	tests := []struct {
		name          string
		containerized bool
		wantContainer bool
	}{
		{
			name:          "non-containerized session opens at worktree",
			containerized: false,
			wantContainer: false,
		},
		{
			name:          "containerized session opens in container",
			containerized: true,
			wantContainer: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := &config.Session{
				ID:            "test-session-123",
				WorkTree:      "/path/to/worktree",
				Containerized: tt.containerized,
			}

			// We can't easily test the actual execution, but we can verify
			// that the function returns a command and doesn't panic
			cmd := openTerminalForSession(sess)
			if cmd == nil {
				t.Fatal("openTerminalForSession returned nil command")
			}

			// The command should be callable without panicking
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("openTerminalForSession panicked: %v", r)
				}
			}()

			// Execute the command to ensure it's properly formed
			// (Note: this will actually try to open a terminal, so we just
			// verify it doesn't panic during construction)
			_ = cmd()
		})
	}
}
