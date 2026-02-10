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

// logsDir returns the path to the logs directory (~/.plural/logs)
func logsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".plural", "logs"), nil
}

// DefaultLogPath returns the default log file path for the main process
func DefaultLogPath() (string, error) {
	dir, err := logsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "plural.log"), nil
}

// MCPLogPath returns the log path for an MCP session
func MCPLogPath(sessionID string) (string, error) {
	dir, err := logsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("mcp-%s.log", sessionID)), nil
}

// StreamLogPath returns the log path for Claude stream messages
func StreamLogPath(sessionID string) (string, error) {
	dir, err := logsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fmt.Sprintf("stream-%s.log", sessionID)), nil
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

	// Ensure the directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory %s: %w", dir, err)
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

	defaultPath, err := DefaultLogPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to get default log path: %v\n", err)
		return
	}

	// Ensure the logs directory exists
	dir := filepath.Dir(defaultPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create log directory %s: %v\n", dir, err)
		return
	}

	logPath = defaultPath
	f, err := os.OpenFile(defaultPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to open log file %s: %v\n", defaultPath, err)
		return
	}
	logFile = f
	handler := slog.NewTextHandler(f, &slog.HandlerOptions{Level: levelVar})
	root = slog.New(handler)
	initDone = true

	root.Info("logger initialized", "path", defaultPath)
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

// ClearLogs removes all plural log files from ~/.plural/logs
func ClearLogs() (int, error) {
	count := 0

	dir, err := logsDir()
	if err != nil {
		return 0, fmt.Errorf("failed to get logs directory: %w", err)
	}

	// Remove main log
	defaultPath := filepath.Join(dir, "plural.log")
	if err := os.Remove(defaultPath); err == nil {
		count++
	} else if !os.IsNotExist(err) {
		return count, err
	}

	// Remove MCP session logs using glob pattern
	mcpPattern := filepath.Join(dir, "mcp-*.log")
	mcpLogs, err := filepath.Glob(mcpPattern)
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
	streamPattern := filepath.Join(dir, "stream-*.log")
	streamLogs, err := filepath.Glob(streamPattern)
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
