package logger

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

var (
	root     *slog.Logger
	levelVar = new(slog.LevelVar)
	logFile  *os.File
	mu       sync.Mutex
	logPath  string
	initDone bool
)

// DefaultLogPath is the default log file for the main process
const DefaultLogPath = "/tmp/plural-debug.log"

// MCPLogPath returns the log path for an MCP session
func MCPLogPath(sessionID string) string {
	return fmt.Sprintf("/tmp/plural-mcp-%s.log", sessionID)
}

// StreamLogPath returns the log path for Claude stream messages
func StreamLogPath(sessionID string) string {
	return fmt.Sprintf("/tmp/plural-stream-%s.log", sessionID)
}

// SetDebug enables or disables debug level logging
func SetDebug(enabled bool) {
	if enabled {
		levelVar.Set(slog.LevelDebug)
	} else {
		levelVar.Set(slog.LevelInfo)
	}
}

// Init initializes the logger with a custom path. Must be called before logging.
// If not called, the default path will be used on first log call.
// Returns an error if the log file cannot be opened.
func Init(path string) error {
	mu.Lock()
	defer mu.Unlock()

	if initDone {
		return nil
	}

	logPath = path
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file %s: %w", path, err)
	}
	logFile = f
	handler := slog.NewTextHandler(f, &slog.HandlerOptions{Level: levelVar})
	root = slog.New(handler)
	initDone = true

	root.Info("logger initialized", "path", path)
	return nil
}

// ensureInit initializes the logger with default settings if not already initialized.
// Caller must hold mu.
func ensureInit() {
	if initDone {
		return
	}

	logPath = DefaultLogPath
	f, err := os.OpenFile(DefaultLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to open log file %s: %v\n", DefaultLogPath, err)
		return
	}
	logFile = f
	handler := slog.NewTextHandler(f, &slog.HandlerOptions{Level: levelVar})
	root = slog.New(handler)
	initDone = true

	root.Info("logger initialized", "path", DefaultLogPath)
}

// Get returns the root logger instance.
// Use this when you don't have session context.
func Get() *slog.Logger {
	mu.Lock()
	defer mu.Unlock()

	ensureInit()

	if root == nil {
		return slog.Default()
	}
	return root
}

// WithSession returns a logger with the session ID attached.
// All log entries from this logger will include sessionID as a structured field.
//
// Example:
//
//	log := logger.WithSession(sess.ID)
//	log.Info("runner created", "workDir", dir)
//	// Output: level=INFO msg="runner created" sessionID=abc123 workDir=/path
func WithSession(sessionID string) *slog.Logger {
	mu.Lock()
	defer mu.Unlock()

	ensureInit()

	if root == nil {
		return slog.Default().With("sessionID", sessionID)
	}
	return root.With("sessionID", sessionID)
}

// WithComponent returns a logger with the component name attached.
// Useful for non-session-scoped logging where you want to identify the source.
//
// Example:
//
//	log := logger.WithComponent("git")
//	log.Info("commit created", "hash", hash)
//	// Output: level=INFO msg="commit created" component=git hash=abc123
func WithComponent(component string) *slog.Logger {
	mu.Lock()
	defer mu.Unlock()

	ensureInit()

	if root == nil {
		return slog.Default().With("component", component)
	}
	return root.With("component", component)
}

// Close closes the log file
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
	root = nil
}

// Reset resets the logger state, allowing reinitialization.
// This is primarily for testing purposes.
func Reset() {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
	initDone = false
	logPath = ""
	root = nil
	levelVar = new(slog.LevelVar)
}

// ClearLogs removes all plural log files from /tmp
func ClearLogs() (int, error) {
	count := 0

	// Remove main debug log
	if err := os.Remove(DefaultLogPath); err == nil {
		count++
	} else if !os.IsNotExist(err) {
		return count, err
	}

	// Remove MCP session logs using glob pattern
	mcpLogs, err := filepath.Glob("/tmp/plural-mcp-*.log")
	if err != nil {
		return count, err
	}

	for _, logPath := range mcpLogs {
		if err := os.Remove(logPath); err == nil {
			count++
		} else if !os.IsNotExist(err) {
			return count, err
		}
	}

	// Remove stream session logs using glob pattern
	streamLogs, err := filepath.Glob("/tmp/plural-stream-*.log")
	if err != nil {
		return count, err
	}

	for _, logPath := range streamLogs {
		if err := os.Remove(logPath); err == nil {
			count++
		} else if !os.IsNotExist(err) {
			return count, err
		}
	}

	return count, nil
}
