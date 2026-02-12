package cmd

import (
	"testing"
	"time"
)

func TestExtractSessionID(t *testing.T) {
	tests := []struct {
		name       string
		socketPath string
		expected   string
	}{
		{
			name:       "standard socket path",
			socketPath: "/tmp/pl-abc123de.sock",
			expected:   "abc123de",
		},
		{
			name:       "abbreviated UUID socket path",
			socketPath: "/tmp/pl-550e8400.sock",
			expected:   "550e8400",
		},
		{
			name:       "no pl- prefix",
			socketPath: "/tmp/other-abc123.sock",
			expected:   "",
		},
		{
			name:       "no .sock extension",
			socketPath: "/tmp/pl-abc123",
			expected:   "abc123",
		},
		{
			name:       "empty path",
			socketPath: "",
			expected:   "",
		},
		{
			name:       "just pl- prefix",
			socketPath: "/tmp/pl-.sock",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSessionID(tt.socketPath)
			if got != tt.expected {
				t.Errorf("extractSessionID(%q) = %q, want %q", tt.socketPath, got, tt.expected)
			}
		})
	}
}

func TestChannelCloseUnblocksRange(t *testing.T) {
	// Verify that closing a channel causes a range loop to exit.
	// This validates the pattern used in runMCPServer to clean up goroutines.
	ch := make(chan string)
	done := make(chan struct{})

	go func() {
		for range ch {
			// consume
		}
		close(done)
	}()

	close(ch)

	// Wait for goroutine to finish with a timeout
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-done:
		// goroutine exited as expected
	case <-timer.C:
		t.Error("goroutine did not exit after channel was closed")
	}
}
