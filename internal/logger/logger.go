package logger

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	logFile  *os.File
	mu       sync.Mutex
	once     sync.Once
	logPath  string
	initDone bool
)

// DefaultLogPath is the default log file for the main process
const DefaultLogPath = "/tmp/plural-debug.log"

// MCPLogPath returns the log path for an MCP session
func MCPLogPath(sessionID string) string {
	return fmt.Sprintf("/tmp/plural-mcp-%s.log", sessionID)
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
		writeLog("Logger initialized: %s", path)
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
				writeLog("Logger initialized: %s", DefaultLogPath)
			}
		})
	}
}

func writeLog(format string, args ...interface{}) {
	if logFile == nil {
		return
	}
	timestamp := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(logFile, "[%s] %s\n", timestamp, msg)
	logFile.Sync()
}

// Log writes a debug message to the log file
func Log(format string, args ...interface{}) {
	mu.Lock()
	defer mu.Unlock()

	ensureInit()
	writeLog(format, args...)
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
