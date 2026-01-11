package logger

import (
	"fmt"
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

var (
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
func Init(path string) {
	mu.Lock()
	defer mu.Unlock()

	if initDone {
		return
	}

	logPath = path
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err == nil {
		logFile = f
		initDone = true
		writeLog(LevelInfo, "Logger initialized: %s", path)
	}
}

func ensureInit() {
	if !initDone {
		once.Do(func() {
			logPath = DefaultLogPath
			f, err := os.OpenFile(DefaultLogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err == nil {
				logFile = f
				initDone = true
				writeLog(LevelInfo, "Logger initialized: %s", DefaultLogPath)
			}
		})
	}
}

func writeLog(level LogLevel, format string, args ...interface{}) {
	if logFile == nil {
		return
	}
	if level < currentLevel {
		return
	}
	timestamp := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(logFile, "[%s] [%s] %s\n", timestamp, level.String(), msg)
	logFile.Sync()
}

// Debug writes a debug message to the log file (only if level is LevelDebug)
func Debug(format string, args ...interface{}) {
	mu.Lock()
	defer mu.Unlock()

	ensureInit()
	writeLog(LevelDebug, format, args...)
}

// Info writes an info message to the log file
func Info(format string, args ...interface{}) {
	mu.Lock()
	defer mu.Unlock()

	ensureInit()
	writeLog(LevelInfo, format, args...)
}

// Warn writes a warning message to the log file
func Warn(format string, args ...interface{}) {
	mu.Lock()
	defer mu.Unlock()

	ensureInit()
	writeLog(LevelWarn, format, args...)
}

// Error writes an error message to the log file
func Error(format string, args ...interface{}) {
	mu.Lock()
	defer mu.Unlock()

	ensureInit()
	writeLog(LevelError, format, args...)
}

// Log writes a debug message to the log file (legacy function, use Debug/Info/Warn/Error instead)
func Log(format string, args ...interface{}) {
	mu.Lock()
	defer mu.Unlock()

	ensureInit()
	writeLog(LevelDebug, format, args...)
}

// Close closes the log file
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
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
