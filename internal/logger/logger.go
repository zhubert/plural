package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
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

func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

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

// PluralHandler is a custom slog.Handler that produces human-readable output
// in the format: [HH:MM:SS.mmm] [LEVEL] message key=value...
type PluralHandler struct {
	w       io.Writer
	level   slog.Level
	mu      *sync.Mutex
	attrs   []slog.Attr
	groups  []string
}

// NewPluralHandler creates a handler with Plural's timestamp format
func NewPluralHandler(w io.Writer, level slog.Level) *PluralHandler {
	return &PluralHandler{
		w:     w,
		level: level,
		mu:    &sync.Mutex{},
	}
}

// Enabled reports whether the handler handles records at the given level
func (h *PluralHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle writes the log record to the output
func (h *PluralHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Format: [HH:MM:SS.mmm] [LEVEL] message key=value...
	timestamp := r.Time.Format("15:04:05.000")
	levelStr := levelString(r.Level)

	// Start building the log line
	line := fmt.Sprintf("[%s] [%s] %s", timestamp, levelStr, r.Message)

	// Append pre-set attributes from WithAttrs
	for _, a := range h.attrs {
		line += fmt.Sprintf(" %s=%v", a.Key, formatValue(a.Value))
	}

	// Append attributes from this record
	r.Attrs(func(a slog.Attr) bool {
		line += fmt.Sprintf(" %s=%v", a.Key, formatValue(a.Value))
		return true
	})

	fmt.Fprintln(h.w, line)

	// Sync to disk for durability
	if f, ok := h.w.(*os.File); ok {
		f.Sync()
	}
	return nil
}

// WithAttrs returns a new handler with the given attributes pre-attached
func (h *PluralHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &PluralHandler{
		w:      h.w,
		level:  h.level,
		mu:     h.mu,
		attrs:  newAttrs,
		groups: h.groups,
	}
}

// WithGroup returns a new handler with the given group name
func (h *PluralHandler) WithGroup(name string) slog.Handler {
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name
	return &PluralHandler{
		w:      h.w,
		level:  h.level,
		mu:     h.mu,
		attrs:  h.attrs,
		groups: newGroups,
	}
}

// levelString converts slog.Level to our string format
func levelString(level slog.Level) string {
	switch {
	case level < slog.LevelInfo:
		return "DEBUG"
	case level < slog.LevelWarn:
		return "INFO"
	case level < slog.LevelError:
		return "WARN"
	default:
		return "ERROR"
	}
}

// formatValue formats a slog.Value for output
func formatValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindTime:
		return v.Time().Format(time.RFC3339)
	case slog.KindDuration:
		return v.Duration().String()
	default:
		return fmt.Sprintf("%v", v.Any())
	}
}

// SetLevel sets the minimum log level to output
func SetLevel(level LogLevel) {
	mu.Lock()
	defer mu.Unlock()
	currentLevel = level

	// Recreate the logger with the new level if initialized
	if logFile != nil {
		handler := NewPluralHandler(logFile, level.toSlogLevel())
		slogLogger = slog.New(handler)
	}
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
	handler := NewPluralHandler(f, currentLevel.toSlogLevel())
	slogLogger = slog.New(handler)
	initDone = true

	// Log initialization message
	slogLogger.Info("Logger initialized: " + path)
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
			handler := NewPluralHandler(f, currentLevel.toSlogLevel())
			slogLogger = slog.New(handler)
			initDone = true

			// Log initialization message
			slogLogger.Info("Logger initialized: " + DefaultLogPath)
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
//
// Output: [14:05:23.456] [INFO] Runner created component=Claude sessionID=abc123 workDir=/path
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
//
// Output: [14:05:23.456] [DEBUG] Starting operation sessionID=abc123
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
