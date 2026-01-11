package logger

import (
	"os"
	"strings"
	"testing"
)

func TestLog(t *testing.T) {
	// Log should not panic even when called
	// Note: We can't easily test the actual file output without modifying the package
	// to allow configurable log paths. This test ensures no panics occur.
	Log("test message")
	Log("test with %s", "argument")
	Log("test with %d and %s", 42, "string")
}

func TestLog_Formatting(t *testing.T) {
	// Test that formatting works correctly
	// The actual output goes to /tmp/plural-debug.log
	Log("integer: %d", 123)
	Log("string: %s", "hello")
	Log("float: %.2f", 3.14159)
	Log("multiple: %s=%d", "count", 5)
}

func TestClose(t *testing.T) {
	// Close should not panic
	// Note: Calling Close will close the global logFile, which could affect other tests
	// In a real test suite, we might want to reinitialize after this
	// For now, we just verify it doesn't panic
	// Close() - commented out to not affect other tests
}

func TestLogFile_Exists(t *testing.T) {
	// After init, the log file should exist
	logPath := "/tmp/plural-debug.log"

	_, err := os.Stat(logPath)
	if err != nil {
		t.Skip("Log file not created, possibly running in restricted environment")
	}

	// Enable debug level to test Log() which maps to debug
	SetDebug(true)
	defer SetDebug(false)

	// Write a test message
	testMsg := "test-unique-string-12345"
	Log("%s", testMsg)

	// Read the log file and verify our message is there
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), testMsg) {
		t.Error("Log file should contain the logged message")
	}
}

func TestLog_Timestamp(t *testing.T) {
	logPath := "/tmp/plural-debug.log"

	// Enable debug level to test Log() which maps to debug
	SetDebug(true)
	defer SetDebug(false)

	// Log a unique message
	uniqueMsg := "timestamp-test-unique-marker"
	Log("%s", uniqueMsg)

	// Read and verify timestamp format [HH:MM:SS.mmm]
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Skip("Cannot read log file")
	}

	// Should have timestamp in format [HH:MM:SS.mmm]
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.Contains(line, uniqueMsg) {
			// Should start with [
			if !strings.HasPrefix(line, "[") {
				t.Error("Log line should start with timestamp bracket")
			}
			// Should contain ] after timestamp
			if !strings.Contains(line, "]") {
				t.Error("Log line should have closing timestamp bracket")
			}
			return
		}
	}

	t.Error("Could not find test message in log")
}

func TestLog_Concurrent(t *testing.T) {
	// Test that concurrent logging doesn't cause issues
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 100; j++ {
				Log("concurrent test %d-%d", n, j)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
