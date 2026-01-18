package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestLogger creates a temp log file and initializes the logger with it.
// Returns the path to the temp file and a cleanup function.
func setupTestLogger(t *testing.T) (string, func()) {
	t.Helper()
	Reset()

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test-debug.log")
	if err := Init(logPath); err != nil {
		t.Fatalf("Failed to init logger: %v", err)
	}

	return logPath, func() {
		Reset()
	}
}

func TestLog(t *testing.T) {
	_, cleanup := setupTestLogger(t)
	defer cleanup()

	// Log should not panic
	Log("test message")
	Log("test with %s", "argument")
	Log("test with %d and %s", 42, "string")
}

func TestLog_Formatting(t *testing.T) {
	_, cleanup := setupTestLogger(t)
	defer cleanup()

	// Test that formatting works correctly
	Log("integer: %d", 123)
	Log("string: %s", "hello")
	Log("float: %.2f", 3.14159)
	Log("multiple: %s=%d", "count", 5)
}

func TestClose(t *testing.T) {
	_, cleanup := setupTestLogger(t)
	defer cleanup()

	// Close should not panic
	Close()
}

func TestLogFile_Exists(t *testing.T) {
	logPath, cleanup := setupTestLogger(t)
	defer cleanup()

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
	logPath, cleanup := setupTestLogger(t)
	defer cleanup()

	// Enable debug level to test Log() which maps to debug
	SetDebug(true)
	defer SetDebug(false)

	// Log a unique message
	uniqueMsg := "timestamp-test-unique-marker"
	Log("%s", uniqueMsg)

	// Read and verify timestamp format [HH:MM:SS.mmm]
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
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
	_, cleanup := setupTestLogger(t)
	defer cleanup()

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

func TestReset(t *testing.T) {
	// First initialization
	tmpDir := t.TempDir()
	logPath1 := filepath.Join(tmpDir, "log1.log")
	if err := Init(logPath1); err != nil {
		t.Fatalf("Failed to init logger: %v", err)
	}

	SetDebug(true)
	Log("message to log1")

	// Reset and reinitialize to a different path
	Reset()

	logPath2 := filepath.Join(tmpDir, "log2.log")
	if err := Init(logPath2); err != nil {
		t.Fatalf("Failed to reinit logger: %v", err)
	}

	Log("message to log2")

	// Verify log1 has the first message but not the second
	content1, err := os.ReadFile(logPath1)
	if err != nil {
		t.Fatalf("Failed to read log1: %v", err)
	}
	if !strings.Contains(string(content1), "message to log1") {
		t.Error("log1 should contain 'message to log1'")
	}
	if strings.Contains(string(content1), "message to log2") {
		t.Error("log1 should NOT contain 'message to log2'")
	}

	// Verify log2 has the second message but not the first
	content2, err := os.ReadFile(logPath2)
	if err != nil {
		t.Fatalf("Failed to read log2: %v", err)
	}
	if !strings.Contains(string(content2), "message to log2") {
		t.Error("log2 should contain 'message to log2'")
	}
	if strings.Contains(string(content2), "message to log1") {
		t.Error("log2 should NOT contain 'message to log1'")
	}

	Reset()
	SetDebug(false)
}
