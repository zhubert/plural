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

func TestGet(t *testing.T) {
	_, cleanup := setupTestLogger(t)
	defer cleanup()

	log := Get()
	if log == nil {
		t.Fatal("Get() returned nil")
	}

	// Should not panic
	log.Info("test message")
	log.Debug("debug message", "key", "value")
	log.Warn("warning", "count", 42)
	log.Error("error occurred", "err", "something failed")
}

func TestGet_StructuredLogging(t *testing.T) {
	logPath, cleanup := setupTestLogger(t)
	defer cleanup()

	log := Get()
	log.Info("user action", "action", "login", "userID", 123)

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	contentStr := string(content)

	if !strings.Contains(contentStr, "user action") {
		t.Error("Should contain message")
	}
	if !strings.Contains(contentStr, "action=login") {
		t.Error("Should contain action=login")
	}
	if !strings.Contains(contentStr, "userID=123") {
		t.Error("Should contain userID=123")
	}
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

	// Write a test message
	testMsg := "test-unique-string-12345"
	Get().Info(testMsg)

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

	// Log a unique message
	uniqueMsg := "timestamp-test-unique-marker"
	Get().Info(uniqueMsg)

	// Read and verify timestamp exists (slog uses time= prefix)
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	lines := strings.SplitSeq(string(content), "\n")
	for line := range lines {
		if strings.Contains(line, uniqueMsg) {
			// slog TextHandler format includes time=
			if !strings.Contains(line, "time=") {
				t.Error("Log line should contain timestamp")
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

	for i := range 10 {
		go func(n int) {
			log := Get()
			for j := range 100 {
				log.Debug("concurrent test", "goroutine", n, "iteration", j)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for range 10 {
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

	Get().Info("message to log1")

	// Reset and reinitialize to a different path
	Reset()

	logPath2 := filepath.Join(tmpDir, "log2.log")
	if err := Init(logPath2); err != nil {
		t.Fatalf("Failed to reinit logger: %v", err)
	}

	Get().Info("message to log2")

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
}

func TestLogLevels(t *testing.T) {
	logPath, cleanup := setupTestLogger(t)
	defer cleanup()

	// Enable debug level
	SetDebug(true)
	defer SetDebug(false)

	log := Get()
	log.Debug("debug message")
	log.Info("info message")
	log.Warn("warn message")
	log.Error("error message")

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

	// Verify level strings appear in output (slog uses level=DEBUG format)
	if !strings.Contains(contentStr, "level=DEBUG") {
		t.Error("Should contain level=DEBUG marker")
	}
	if !strings.Contains(contentStr, "level=INFO") {
		t.Error("Should contain level=INFO marker")
	}
	if !strings.Contains(contentStr, "level=WARN") {
		t.Error("Should contain level=WARN marker")
	}
	if !strings.Contains(contentStr, "level=ERROR") {
		t.Error("Should contain level=ERROR marker")
	}
}

func TestLogLevel_Filtering(t *testing.T) {
	logPath, cleanup := setupTestLogger(t)
	defer cleanup()

	// Default is Info level - Debug should be filtered
	SetDebug(false)

	log := Get()
	log.Debug("debug-filtered")
	log.Info("info-visible")

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

func TestWithComponent(t *testing.T) {
	logPath, cleanup := setupTestLogger(t)
	defer cleanup()

	// Get a component logger
	claudeLog := WithComponent("Claude")

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

func TestWithSession_AdditionalAttrs(t *testing.T) {
	logPath, cleanup := setupTestLogger(t)
	defer cleanup()

	// Get a session logger and add more context
	sessionLog := WithSession("sess-123").With("component", "runner")

	sessionLog.Info("process started", "pid", 12345)

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	contentStr := string(content)

	// Should contain all attributes
	if !strings.Contains(contentStr, "sessionID=sess-123") {
		t.Error("Should contain sessionID")
	}
	if !strings.Contains(contentStr, "component=runner") {
		t.Error("Should contain component")
	}
	if !strings.Contains(contentStr, "pid=12345") {
		t.Error("Should contain pid")
	}
}

func TestLoggerWithAttrs(t *testing.T) {
	logPath, cleanup := setupTestLogger(t)
	defer cleanup()

	// Create a logger with pre-attached attributes
	log := Get().With("requestID", "req-123", "userID", "user-456")

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

func TestEnsureInit_DefaultPath(t *testing.T) {
	Reset()
	defer Reset()

	// Don't call Init - let ensureInit use the default path
	log := Get()
	if log == nil {
		t.Fatal("Get() returned nil without prior Init()")
	}

	// Should not panic
	log.Info("default path test")
}

func TestConcurrent_InitAndGet(t *testing.T) {
	// Test that concurrent Init and Get calls don't race
	for range 10 {
		Reset()

		tmpDir := t.TempDir()
		logPath := filepath.Join(tmpDir, "concurrent.log")

		done := make(chan bool, 20)

		// Race Init and Get/WithSession/WithComponent
		for range 5 {
			go func() {
				_ = Init(logPath)
				done <- true
			}()
			go func() {
				Get().Info("concurrent get")
				done <- true
			}()
			go func() {
				WithSession("sess").Info("concurrent session")
				done <- true
			}()
			go func() {
				WithComponent("comp").Info("concurrent component")
				done <- true
			}()
		}

		for range 20 {
			<-done
		}
	}
	Reset()
}

func TestMCPLogPath(t *testing.T) {
	sessionID := "test-session-123"

	got, err := MCPLogPath(sessionID)
	if err != nil {
		t.Fatalf("MCPLogPath(%q) returned error: %v", sessionID, err)
	}

	// Should contain the session ID in the filename
	if !strings.Contains(got, "mcp-test-session-123.log") {
		t.Errorf("MCPLogPath(%q) = %q, should contain 'mcp-test-session-123.log'", sessionID, got)
	}

	// Should be in a logs directory
	if !strings.Contains(got, "/logs/") {
		t.Errorf("MCPLogPath(%q) = %q, should be in a logs directory", sessionID, got)
	}
}

func TestStreamLogPath(t *testing.T) {
	sessionID := "test-session-456"

	got, err := StreamLogPath(sessionID)
	if err != nil {
		t.Fatalf("StreamLogPath(%q) returned error: %v", sessionID, err)
	}

	// Should contain the session ID in the filename
	if !strings.Contains(got, "stream-test-session-456.log") {
		t.Errorf("StreamLogPath(%q) = %q, should contain 'stream-test-session-456.log'", sessionID, got)
	}

	// Should be in a logs directory
	if !strings.Contains(got, "/logs/") {
		t.Errorf("StreamLogPath(%q) = %q, should be in a logs directory", sessionID, got)
	}
}
