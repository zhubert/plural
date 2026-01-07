// Package process provides utilities for managing Claude CLI processes.
// It helps detect and clean up orphaned Claude processes that may block
// session resumption.
package process

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/zhubert/plural/internal/logger"
)

// ClaudeProcess represents a running Claude CLI process
type ClaudeProcess struct {
	PID       int
	SessionID string
	Command   string
}

// FindClaudeProcesses finds all running Claude CLI processes that are using
// the specified session ID. It looks for processes with --session-id or --resume
// arguments matching the given session ID.
func FindClaudeProcesses(sessionID string) ([]ClaudeProcess, error) {
	// Use pgrep with -f to search full command lines for "claude"
	// We intentionally get all claude processes and filter ourselves for accuracy
	cmd := exec.Command("pgrep", "-f", "claude")
	output, err := cmd.Output()
	if err != nil {
		// pgrep returns exit code 1 when no processes found - that's not an error for us
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find claude processes: %w", err)
	}

	var processes []ClaudeProcess
	outputStr := strings.TrimSpace(string(output))

	for pidStr := range strings.SplitSeq(outputStr, "\n") {
		pidStr = strings.TrimSpace(pidStr)
		if pidStr == "" {
			continue
		}

		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		// Get the full command line for this PID
		cmdLine, err := getProcessCommandLine(pid)
		if err != nil {
			continue
		}

		// Check if this process is using our session ID
		if containsSessionID(cmdLine, sessionID) {
			processes = append(processes, ClaudeProcess{
				PID:       pid,
				SessionID: sessionID,
				Command:   cmdLine,
			})
		}
	}

	return processes, nil
}

// KillClaudeProcesses kills all Claude CLI processes using the specified session ID.
// Returns the number of processes killed and any error encountered.
func KillClaudeProcesses(sessionID string) (int, error) {
	processes, err := FindClaudeProcesses(sessionID)
	if err != nil {
		return 0, err
	}

	if len(processes) == 0 {
		return 0, nil
	}

	killed := 0
	var lastErr error

	for _, proc := range processes {
		logger.Log("Process: Killing orphaned Claude process PID=%d for session %s", proc.PID, sessionID)

		p, err := os.FindProcess(proc.PID)
		if err != nil {
			lastErr = err
			continue
		}

		// Send SIGTERM first (graceful shutdown)
		if err := p.Signal(os.Interrupt); err != nil {
			// Process might have already exited, try SIGKILL
			if err := p.Kill(); err != nil {
				lastErr = err
				continue
			}
		}
		killed++
	}

	if killed == 0 && lastErr != nil {
		return 0, lastErr
	}

	return killed, nil
}

// IsSessionInUseError checks if an error message indicates that a Claude session
// is already in use by another process.
func IsSessionInUseError(errMsg string) bool {
	errLower := strings.ToLower(errMsg)
	// Claude CLI may use various phrasings for this error
	return strings.Contains(errLower, "session") &&
		(strings.Contains(errLower, "in use") ||
			strings.Contains(errLower, "already") ||
			strings.Contains(errLower, "locked") ||
			strings.Contains(errLower, "busy"))
}

// getProcessCommandLine returns the full command line for a process by PID.
func getProcessCommandLine(pid int) (string, error) {
	// On macOS/BSD, use ps to get the command line
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// containsSessionID checks if a command line contains references to the given session ID.
func containsSessionID(cmdLine, sessionID string) bool {
	// Look for --session-id <sessionID> or --resume <sessionID>
	return strings.Contains(cmdLine, "--session-id "+sessionID) ||
		strings.Contains(cmdLine, "--resume "+sessionID) ||
		strings.Contains(cmdLine, "--session-id="+sessionID) ||
		strings.Contains(cmdLine, "--resume="+sessionID)
}
