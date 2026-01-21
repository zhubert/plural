package logger

import (
	"context"
	"log/slog"
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

	// Reset sets level back to Info, so use Info() instead of Log() (which is Debug level)
	Info("message to log2")

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

func TestLogLevels(t *testing.T) {
	logPath, cleanup := setupTestLogger(t)
	defer cleanup()

	// Test each log level
	SetLevel(LevelDebug)

	Debug("debug message")
	Info("info message")
	Warn("warn message")
	Error("error message")

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	contentStr := string(content)

	// All messages should be present at debug level
	if !strings.Contains(contentStr, "debug message") {
		t.Error("Should contain debug message")
	}
	if !strings.Contains(contentStr, "info message") {
		t.Error("Should contain info message")
	}
	if !strings.Contains(contentStr, "warn message") {
		t.Error("Should contain warn message")
	}
	if !strings.Contains(contentStr, "error message") {
		t.Error("Should contain error message")
	}

	// Verify level strings appear in output
	if !strings.Contains(contentStr, "[DEBUG]") {
		t.Error("Should contain [DEBUG] level marker")
	}
	if !strings.Contains(contentStr, "[INFO]") {
		t.Error("Should contain [INFO] level marker")
	}
	if !strings.Contains(contentStr, "[WARN]") {
		t.Error("Should contain [WARN] level marker")
	}
	if !strings.Contains(contentStr, "[ERROR]") {
		t.Error("Should contain [ERROR] level marker")
	}
}

func TestLogLevel_Filtering(t *testing.T) {
	logPath, cleanup := setupTestLogger(t)
	defer cleanup()

	// Set to Info level - Debug should be filtered
	SetLevel(LevelInfo)

	Debug("debug-filtered")
	Info("info-visible")

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	contentStr := string(content)

	// Debug should be filtered out
	if strings.Contains(contentStr, "debug-filtered") {
		t.Error("Debug message should be filtered at Info level")
	}

	// Info should be visible
	if !strings.Contains(contentStr, "info-visible") {
		t.Error("Info message should be visible at Info level")
	}
}

func TestComponentLogger(t *testing.T) {
	logPath, cleanup := setupTestLogger(t)
	defer cleanup()

	// Get a component logger
	claudeLog := ComponentLogger("Claude")

	// Log a message with the component logger
	claudeLog.Info("Runner created", "sessionID", "abc123")

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	contentStr := string(content)

	// Should contain the message
	if !strings.Contains(contentStr, "Runner created") {
		t.Error("Should contain 'Runner created' message")
	}

	// Should contain the component attribute
	if !strings.Contains(contentStr, "component=Claude") {
		t.Error("Should contain 'component=Claude' attribute")
	}

	// Should contain the sessionID attribute
	if !strings.Contains(contentStr, "sessionID=abc123") {
		t.Error("Should contain 'sessionID=abc123' attribute")
	}
}

func TestWithSession(t *testing.T) {
	logPath, cleanup := setupTestLogger(t)
	defer cleanup()

	// Get a session logger
	sessionLog := WithSession("session-xyz")

	// Log a message with the session logger
	sessionLog.Info("Operation started")

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	contentStr := string(content)

	// Should contain the message
	if !strings.Contains(contentStr, "Operation started") {
		t.Error("Should contain 'Operation started' message")
	}

	// Should contain the sessionID attribute
	if !strings.Contains(contentStr, "sessionID=session-xyz") {
		t.Error("Should contain 'sessionID=session-xyz' attribute")
	}
}

func TestPluralHandler_Enabled(t *testing.T) {
	handler := NewPluralHandler(os.Stdout, slog.LevelInfo)

	// Should be enabled for Info and above
	if !handler.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Should be enabled for Info level")
	}
	if !handler.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("Should be enabled for Warn level")
	}
	if !handler.Enabled(context.Background(), slog.LevelError) {
		t.Error("Should be enabled for Error level")
	}

	// Should not be enabled for Debug
	if handler.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Should not be enabled for Debug level when set to Info")
	}
}

func TestPluralHandler_WithAttrs(t *testing.T) {
	logPath, cleanup := setupTestLogger(t)
	defer cleanup()

	// Create a logger with pre-attached attributes
	log := Logger().With("requestID", "req-123", "userID", "user-456")

	log.Info("Request processed")

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	contentStr := string(content)

	// Should contain all pre-attached attributes
	if !strings.Contains(contentStr, "requestID=req-123") {
		t.Error("Should contain 'requestID=req-123' attribute")
	}
	if !strings.Contains(contentStr, "userID=user-456") {
		t.Error("Should contain 'userID=user-456' attribute")
	}
}

func TestLogLevelString(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LogLevel(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if got := tt.level.String(); got != tt.expected {
			t.Errorf("LogLevel(%d).String() = %q, want %q", tt.level, got, tt.expected)
		}
	}
}

func TestMCPLogPath(t *testing.T) {
	sessionID := "test-session-123"
	expected := "/tmp/plural-mcp-test-session-123.log"

	if got := MCPLogPath(sessionID); got != expected {
		t.Errorf("MCPLogPath(%q) = %q, want %q", sessionID, got, expected)
	}
}
