// Package process provides utilities for managing and cleaning up Claude CLI processes.
package process

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/zhubert/plural/internal/logger"
)

// ContainersSupported returns true if Docker is installed on the system.
func ContainersSupported() bool {
	return ContainerCLIInstalled()
}

// ContainerCLIInstalled returns true if the `docker` CLI is on the PATH.
func ContainerCLIInstalled() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

// containerCheckTimeout is the maximum time to wait for container CLI commands.
const containerCheckTimeout = 5 * time.Second

// ContainerSystemRunning returns true if the Docker daemon is active.
// Returns false if the CLI is not installed, the daemon is not running, or the
// check times out (5s deadline to avoid blocking the UI).
func ContainerSystemRunning() bool {
	if !ContainerCLIInstalled() {
		return false
	}
	return containerSystemRunning()
}

// ContainerImageExists checks if a container image exists locally.
// Returns false if the container CLI is not available or the image is not found.
func ContainerImageExists(image string) bool {
	if !ContainerCLIInstalled() {
		return false
	}
	return containerImageExists(image)
}

// containerSystemRunning checks if the Docker daemon is running.
// Caller must verify CLI is installed first.
func containerSystemRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), containerCheckTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "info")
	return cmd.Run() == nil
}

// containerImageExists checks if a container image exists locally.
// Caller must verify CLI is installed first.
func containerImageExists(image string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), containerCheckTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	return cmd.Run() == nil
}

// ContainerPrerequisites holds the results of all container prerequisite checks.
type ContainerPrerequisites struct {
	CLIInstalled  bool
	SystemRunning bool
	ImageExists   bool
	AuthAvailable bool
}

// CheckContainerPrerequisites runs all container prerequisite checks with short-circuiting.
// Later checks are skipped when earlier ones fail, since they depend on the previous step.
// authChecker is a function that returns whether auth credentials are available.
func CheckContainerPrerequisites(image string, authChecker func() bool) ContainerPrerequisites {
	result := ContainerPrerequisites{}

	result.CLIInstalled = ContainerCLIInstalled()
	if !result.CLIInstalled {
		return result
	}

	result.SystemRunning = containerSystemRunning()
	if !result.SystemRunning {
		return result
	}

	result.ImageExists = containerImageExists(image)
	if !result.ImageExists {
		return result
	}

	result.AuthAvailable = authChecker()
	return result
}

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
	log := logger.WithComponent("process")

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

	log.Debug("found Claude processes", "count", len(processes))
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

	log := logger.WithComponent("process")
	var orphans []ClaudeProcess
	for _, proc := range allProcesses {
		sessionID := extractSessionID(proc.Command)
		if sessionID != "" && !knownSessionIDs[sessionID] {
			orphans = append(orphans, proc)
			log.Info("found orphaned Claude process", "pid", proc.PID, "sessionID", sessionID)
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

	log := logger.WithComponent("process")
	killed := 0
	for _, proc := range orphans {
		log.Info("killing orphaned Claude process", "pid", proc.PID)
		if err := KillProcess(proc.PID); err != nil {
			log.Error("failed to kill process", "pid", proc.PID, "error", err)
			continue
		}
		killed++
	}

	return killed, nil
}

// OrphanedContainer represents a container found on the system that doesn't match any known session.
type OrphanedContainer struct {
	Name string // Container name (e.g., "plural-abc123")
}

// ListContainerNames returns a list of all container names using the Docker CLI.
// Docker outputs NDJSON (one JSON object per line) with a "Names" field.
func ListContainerNames() ([]string, error) {
	cmd := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			names = append(names, name)
		}
	}

	return names, nil
}

// FindOrphanedContainers finds containers named plural-* whose session ID is not in knownSessionIDs.
// Returns an empty list (not an error) if the container CLI is not available.
func FindOrphanedContainers(knownSessionIDs map[string]bool) ([]OrphanedContainer, error) {
	log := logger.WithComponent("process")

	// Check if container CLI is available
	if _, err := exec.LookPath("docker"); err != nil {
		log.Debug("docker CLI not found, skipping container orphan check")
		return nil, nil
	}

	// Get list of container names
	names, err := ListContainerNames()
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var orphans []OrphanedContainer
	for _, name := range names {
		if name == "" {
			continue
		}
		if !strings.HasPrefix(name, "plural-") {
			continue
		}
		sessionID := strings.TrimPrefix(name, "plural-")
		if !knownSessionIDs[sessionID] {
			orphans = append(orphans, OrphanedContainer{Name: name})
			log.Info("found orphaned container", "name", name, "sessionID", sessionID)
		}
	}

	log.Debug("found orphaned containers", "count", len(orphans))
	return orphans, nil
}

// CleanupOrphanedContainers removes all containers named plural-* that don't match known session IDs.
// Returns the number of containers removed.
func CleanupOrphanedContainers(knownSessionIDs map[string]bool) (int, error) {
	orphans, err := FindOrphanedContainers(knownSessionIDs)
	if err != nil {
		return 0, err
	}

	log := logger.WithComponent("process")
	removed := 0
	for _, container := range orphans {
		log.Info("removing orphaned container", "name", container.Name)
		cmd := exec.Command("docker", "rm", "-f", container.Name)
		if err := cmd.Run(); err != nil {
			log.Error("failed to remove container", "name", container.Name, "error", err)
			continue
		}
		removed++
	}

	return removed, nil
}
