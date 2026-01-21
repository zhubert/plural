package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	// LevelDebug is for verbose debugging information
	LevelDebug LogLevel = iota
	// LevelInfo is for general operational information
	LevelInfo
	// LevelWarn is for warning conditions
	LevelWarn
	// LevelError is for error conditions
	LevelError
)

// toSlogLevel converts our LogLevel to slog.Level
func (l LogLevel) toSlogLevel() slog.Level {
	switch l {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

var (
	slogLogger   *slog.Logger
	levelVar     = new(slog.LevelVar) // Allows dynamic level changes
	logFile      *os.File
	mu           sync.Mutex
	once         sync.Once
	logPath      string
	initDone     bool
	currentLevel LogLevel = LevelInfo // Default to Info level
)

// DefaultLogPath is the default log file for the main process
const DefaultLogPath = "/tmp/plural-debug.log"

// MCPLogPath returns the log path for an MCP session
func MCPLogPath(sessionID string) string {
	return fmt.Sprintf("/tmp/plural-mcp-%s.log", sessionID)
}

// SetLevel sets the minimum log level to output
func SetLevel(level LogLevel) {
	mu.Lock()
	defer mu.Unlock()
	currentLevel = level
	levelVar.Set(level.toSlogLevel())
}

// SetDebug enables debug level logging
func SetDebug(enabled bool) {
	if enabled {
		SetLevel(LevelDebug)
	} else {
		SetLevel(LevelInfo)
	}
}

// Init initializes the logger with a custom path. Must be called before Log().
// If not called, the default path will be used on first Log() call.
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
	levelVar.Set(currentLevel.toSlogLevel())
	handler := slog.NewTextHandler(f, &slog.HandlerOptions{Level: levelVar})
	slogLogger = slog.New(handler)
	initDone = true

	slogLogger.Info("Logger initialized", "path", path)
	return nil
}

func ensureInit() {
	if !initDone {
		once.Do(func() {
			logPath = DefaultLogPath
			f, err := os.OpenFile(DefaultLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				// Print to stderr since we can't log
				fmt.Fprintf(os.Stderr, "Warning: failed to open log file %s: %v\n", DefaultLogPath, err)
				return
			}
			logFile = f
			levelVar.Set(currentLevel.toSlogLevel())
			handler := slog.NewTextHandler(f, &slog.HandlerOptions{Level: levelVar})
			slogLogger = slog.New(handler)
			initDone = true

			slogLogger.Info("Logger initialized", "path", DefaultLogPath)
		})
	}
}

// logWithLevel logs a message at the given level using printf-style formatting
func logWithLevel(level slog.Level, format string, args ...interface{}) {
	mu.Lock()
	defer mu.Unlock()

	ensureInit()

	if slogLogger == nil {
		return
	}

	// Check if enabled before formatting (optimization)
	if !slogLogger.Enabled(context.Background(), level) {
		return
	}

	// Format the message using printf-style
	msg := fmt.Sprintf(format, args...)
	slogLogger.Log(context.Background(), level, msg)
}

// Debug writes a debug message to the log file (only if level is LevelDebug)
func Debug(format string, args ...interface{}) {
	logWithLevel(slog.LevelDebug, format, args...)
}

// Info writes an info message to the log file
func Info(format string, args ...interface{}) {
	logWithLevel(slog.LevelInfo, format, args...)
}

// Warn writes a warning message to the log file
func Warn(format string, args ...interface{}) {
	logWithLevel(slog.LevelWarn, format, args...)
}

// Error writes an error message to the log file
func Error(format string, args ...interface{}) {
	logWithLevel(slog.LevelError, format, args...)
}

// Log writes a debug message to the log file (legacy function, use Debug/Info/Warn/Error instead)
// Deprecated: Use Debug() for debug-level logging
func Log(format string, args ...interface{}) {
	logWithLevel(slog.LevelDebug, format, args...)
}

// Close closes the log file
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
	slogLogger = nil
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
	once = sync.Once{}
	logPath = ""
	slogLogger = nil
	currentLevel = LevelInfo
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

	return count, nil
}

// ComponentLogger returns a slog.Logger with the component attribute pre-attached.
// This enables efficient structured logging with the With() pattern.
//
// Example:
//
//	log := logger.ComponentLogger("Claude")
//	log.Info("Runner created", "sessionID", sessID, "workDir", dir)
func ComponentLogger(component string) *slog.Logger {
	mu.Lock()
	defer mu.Unlock()

	ensureInit()

	if slogLogger == nil {
		return slog.Default()
	}
	return slogLogger.With(slog.String("component", component))
}

// WithSession returns a slog.Logger with the session ID pre-attached.
// Useful for session-scoped logging where session ID is repeated.
//
// Example:
//
//	sessionLog := logger.WithSession(sess.ID)
//	sessionLog.Debug("Starting operation")
func WithSession(sessionID string) *slog.Logger {
	mu.Lock()
	defer mu.Unlock()

	ensureInit()

	if slogLogger == nil {
		return slog.Default()
	}
	return slogLogger.With(slog.String("sessionID", sessionID))
}

// Logger returns the underlying slog.Logger for advanced use cases.
// Returns nil if the logger is not initialized.
func Logger() *slog.Logger {
	mu.Lock()
	defer mu.Unlock()

	ensureInit()
	return slogLogger
}
