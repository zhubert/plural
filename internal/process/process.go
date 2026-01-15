// Package process provides utilities for managing and cleaning up Claude CLI processes.
package process

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/zhubert/plural/internal/logger"
)

// ClaudeProcess represents a running Claude CLI process found on the system.
type ClaudeProcess struct {
	PID     int    // Process ID
	Command string // Full command line
}

// FindClaudeProcesses finds all running Claude CLI processes on the system.
// This is useful for detecting orphaned processes that may have been left behind
// after a crash.
func FindClaudeProcesses() ([]ClaudeProcess, error) {
	var processes []ClaudeProcess

	switch runtime.GOOS {
	case "darwin", "linux":
		// Use pgrep to find claude processes
		cmd := exec.Command("pgrep", "-f", "claude.*--session-id")
		output, err := cmd.Output()
		if err != nil {
			// pgrep returns exit code 1 if no processes found
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
				return processes, nil
			}
			return nil, err
		}

		pids := strings.Fields(string(output))
		for _, pidStr := range pids {
			pid, err := strconv.Atoi(strings.TrimSpace(pidStr))
			if err != nil {
				continue
			}

			// Get the full command line for this PID
			psCmd := exec.Command("ps", "-p", pidStr, "-o", "args=")
			psOutput, err := psCmd.Output()
			if err != nil {
				continue
			}

			processes = append(processes, ClaudeProcess{
				PID:     pid,
				Command: strings.TrimSpace(string(psOutput)),
			})
		}

	case "windows":
		// Use tasklist on Windows
		cmd := exec.Command("tasklist", "/FI", "IMAGENAME eq claude*", "/FO", "CSV", "/NH")
		output, err := cmd.Output()
		if err != nil {
			return nil, err
		}

		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			fields := strings.Split(line, ",")
			if len(fields) >= 2 {
				// Remove quotes from PID field
				pidStr := strings.Trim(strings.TrimSpace(fields[1]), "\"")
				pid, err := strconv.Atoi(pidStr)
				if err != nil {
					continue
				}
				processes = append(processes, ClaudeProcess{
					PID:     pid,
					Command: strings.Trim(fields[0], "\""),
				})
			}
		}
	}

	logger.Log("Process: Found %d Claude processes", len(processes))
	return processes, nil
}

// KillProcess kills a process by PID.
func KillProcess(pid int) error {
	switch runtime.GOOS {
	case "darwin", "linux":
		cmd := exec.Command("kill", "-9", strconv.Itoa(pid))
		return cmd.Run()
	case "windows":
		cmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
		return cmd.Run()
	}
	return nil
}

// FindOrphanedClaudeProcesses finds Claude processes that have specific session IDs
// that aren't in the provided list of known session IDs.
func FindOrphanedClaudeProcesses(knownSessionIDs map[string]bool) ([]ClaudeProcess, error) {
	allProcesses, err := FindClaudeProcesses()
	if err != nil {
		return nil, err
	}

	var orphans []ClaudeProcess
	for _, proc := range allProcesses {
		sessionID := extractSessionID(proc.Command)
		if sessionID != "" && !knownSessionIDs[sessionID] {
			orphans = append(orphans, proc)
			logger.Log("Process: Found orphaned Claude process: PID=%d, sessionID=%s", proc.PID, sessionID)
		}
	}

	return orphans, nil
}

// extractSessionID extracts the session ID from a Claude CLI command line.
func extractSessionID(cmdLine string) string {
	// Look for --session-id or --resume followed by the ID
	patterns := []string{"--session-id", "--resume"}
	for _, pattern := range patterns {
		idx := strings.Index(cmdLine, pattern)
		if idx == -1 {
			continue
		}

		// Get the part after the flag
		rest := cmdLine[idx+len(pattern):]
		rest = strings.TrimLeft(rest, " =")

		// Extract the session ID (first space-separated token)
		fields := strings.Fields(rest)
		if len(fields) > 0 {
			return fields[0]
		}
	}
	return ""
}

// CleanupOrphanedProcesses kills all Claude processes that don't match known session IDs.
// Returns the number of processes killed.
func CleanupOrphanedProcesses(knownSessionIDs map[string]bool) (int, error) {
	orphans, err := FindOrphanedClaudeProcesses(knownSessionIDs)
	if err != nil {
		return 0, err
	}

	killed := 0
	for _, proc := range orphans {
		logger.Log("Process: Killing orphaned Claude process: PID=%d", proc.PID)
		if err := KillProcess(proc.PID); err != nil {
			logger.Error("Process: Failed to kill PID %d: %v", proc.PID, err)
			continue
		}
		killed++
	}

	return killed, nil
}
