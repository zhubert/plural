package claude

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/zhubert/plural/internal/logger"
)

// errReadTimeout is returned when a read operation times out.
var errReadTimeout = fmt.Errorf("read timeout")

// errChannelFull is returned when the response channel is full for too long.
var errChannelFull = fmt.Errorf("channel full")

// readResult holds the result of a read operation for timeout handling.
type readResult struct {
	line string
	err  error
}

// ProcessManagerInterface defines the contract for managing Claude CLI processes.
// This interface enables dependency injection and testing.
type ProcessManagerInterface interface {
	// Start starts the persistent Claude CLI process.
	// Returns an error if the process is already running or fails to start.
	Start() error

	// Stop stops the persistent process gracefully.
	// If the process doesn't exit gracefully within the timeout, it's force-killed.
	Stop()

	// IsRunning returns whether the process is currently running.
	IsRunning() bool

	// WriteMessage writes a message to the process stdin.
	// Returns an error if the process is not running or write fails.
	WriteMessage(data []byte) error

	// Interrupt sends SIGINT to the process to interrupt the current operation.
	// Returns an error if the process is not running.
	Interrupt() error

	// SetInterrupted marks the current operation as interrupted by the user.
	// This prevents the process manager from reporting errors on expected exit.
	SetInterrupted(interrupted bool)

	// GetRestartAttempts returns the number of restart attempts since last success.
	GetRestartAttempts() int

	// ResetRestartAttempts resets the restart attempt counter (on successful response).
	ResetRestartAttempts()
}

// ProcessConfig holds the configuration for starting a Claude CLI process.
type ProcessConfig struct {
	SessionID         string
	WorkingDir        string
	SessionStarted    bool
	AllowedTools      []string
	MCPConfigPath     string
	ForkFromSessionID string // When set, uses --resume <parentID> --fork-session to inherit parent conversation
}

// ProcessCallbacks defines callbacks that the ProcessManager invokes during operation.
// This allows the Runner to respond to process events without tight coupling.
type ProcessCallbacks struct {
	// OnLine is called for each line read from stdout.
	OnLine func(line string)

	// OnProcessExit is called when the process exits unexpectedly.
	// The error parameter contains the exit reason (may be nil for clean exit).
	// The stderrContent contains any stderr output from the process.
	// Returns true if the process should be restarted.
	OnProcessExit func(err error, stderrContent string) bool

	// OnProcessHung is called when the process appears to be hung (no output for timeout).
	OnProcessHung func()

	// OnRestartAttempt is called when a restart is being attempted.
	// attemptNum is 1-indexed (1, 2, 3, ...).
	OnRestartAttempt func(attemptNum int)

	// OnRestartFailed is called when restart fails.
	OnRestartFailed func(err error)

	// OnFatalError is called when max restarts exceeded or unrecoverable error.
	OnFatalError func(err error)
}

// ProcessManager manages the lifecycle of a Claude CLI process.
// It handles starting, stopping, monitoring, and auto-recovery of the process.
type ProcessManager struct {
	config    ProcessConfig
	callbacks ProcessCallbacks

	// Process state (protected by mu)
	mu              sync.Mutex
	cmd             *exec.Cmd
	stdin           io.WriteCloser
	stdout          *bufio.Reader
	stderr          io.ReadCloser
	running         bool
	interrupted     bool
	restartAttempts int
	lastRestartTime time.Time

	// Context for process goroutines
	ctx    context.Context
	cancel context.CancelFunc

	// Ensures Stop is idempotent
	stopOnce sync.Once
	stopped  bool
}

// NewProcessManager creates a new ProcessManager with the given configuration and callbacks.
func NewProcessManager(config ProcessConfig, callbacks ProcessCallbacks) *ProcessManager {
	return &ProcessManager{
		config:    config,
		callbacks: callbacks,
	}
}

// BuildCommandArgs builds the command line arguments for the Claude CLI based on the config.
// This is exported for testing purposes to verify correct argument construction.
func BuildCommandArgs(config ProcessConfig) []string {
	var args []string
	if config.SessionStarted {
		// Session already started - resume our own session
		args = []string{
			"--print",
			"--output-format", "stream-json",
			"--input-format", "stream-json",
			"--verbose",
			"--resume", config.SessionID,
		}
	} else if config.ForkFromSessionID != "" {
		// Forked session - resume parent and fork to inherit conversation history
		// We must pass --session-id to ensure Claude uses our UUID for the forked session,
		// otherwise Claude generates its own ID and we can't resume later.
		args = []string{
			"--print",
			"--output-format", "stream-json",
			"--input-format", "stream-json",
			"--verbose",
			"--resume", config.ForkFromSessionID,
			"--fork-session",
			"--session-id", config.SessionID,
		}
	} else {
		// New session
		args = []string{
			"--print",
			"--output-format", "stream-json",
			"--input-format", "stream-json",
			"--verbose",
			"--session-id", config.SessionID,
		}
	}

	// Add MCP config and permission prompt tool
	args = append(args,
		"--mcp-config", config.MCPConfigPath,
		"--permission-prompt-tool", "mcp__plural__permission",
		"--append-system-prompt", OptionsSystemPrompt,
	)

	// Add pre-allowed tools
	for _, tool := range config.AllowedTools {
		args = append(args, "--allowedTools", tool)
	}

	return args
}

// Start starts the persistent Claude CLI process.
func (pm *ProcessManager) Start() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.running {
		return nil
	}

	if pm.stopped {
		return fmt.Errorf("process manager has been stopped")
	}

	logger.Info("ProcessManager: Starting process for session %s", pm.config.SessionID)
	startTime := time.Now()

	// Build command arguments
	args := BuildCommandArgs(pm.config)

	// Log fork operation if applicable
	if pm.config.ForkFromSessionID != "" {
		logger.Log("ProcessManager: Forking session from parent %s to new session %s", pm.config.ForkFromSessionID, pm.config.SessionID)
	}

	logger.Log("ProcessManager: Starting process: claude %s", strings.Join(args, " "))

	cmd := exec.Command("claude", args...)
	cmd.Dir = pm.config.WorkingDir

	// Get stdin pipe for writing messages
	stdin, err := cmd.StdinPipe()
	if err != nil {
		logger.Error("ProcessManager: Failed to get stdin pipe: %v", err)
		return fmt.Errorf("failed to get stdin pipe: %v", err)
	}

	// Get stdout pipe for reading responses
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		logger.Error("ProcessManager: Failed to get stdout pipe: %v", err)
		return fmt.Errorf("failed to get stdout pipe: %v", err)
	}

	// Get stderr pipe for error messages
	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		logger.Error("ProcessManager: Failed to get stderr pipe: %v", err)
		return fmt.Errorf("failed to get stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		logger.Error("ProcessManager: Failed to start process: %v", err)
		return fmt.Errorf("failed to start process: %v", err)
	}

	pm.cmd = cmd
	pm.stdin = stdin
	pm.stdout = bufio.NewReader(stdout)
	pm.stderr = stderr
	pm.running = true

	// Create context for process goroutines
	pm.ctx, pm.cancel = context.WithCancel(context.Background())

	logger.Info("ProcessManager: Process started in %v, pid=%d", time.Since(startTime), cmd.Process.Pid)

	// Start goroutines to read output and monitor process
	go pm.readOutput()
	go pm.monitorExit()

	return nil
}

// Stop stops the persistent process gracefully.
func (pm *ProcessManager) Stop() {
	pm.stopOnce.Do(func() {
		pm.mu.Lock()
		pm.stopped = true

		// Cancel context first to signal goroutines to exit
		if pm.cancel != nil {
			pm.cancel()
			pm.cancel = nil
		}

		if !pm.running {
			pm.mu.Unlock()
			return
		}

		logger.Log("ProcessManager: Stopping process for session %s", pm.config.SessionID)

		// Close stdin to signal EOF to the process
		if pm.stdin != nil {
			pm.stdin.Close()
			pm.stdin = nil
		}

		cmd := pm.cmd
		pm.mu.Unlock()

		// Kill the process if it doesn't exit gracefully
		if cmd != nil && cmd.Process != nil {
			done := make(chan struct{})
			go func() {
				cmd.Wait()
				close(done)
			}()

			select {
			case <-done:
				logger.Log("ProcessManager: Process exited gracefully")
			case <-time.After(2 * time.Second):
				logger.Log("ProcessManager: Force killing process")
				cmd.Process.Kill()
			}
		}

		pm.mu.Lock()
		if pm.stderr != nil {
			pm.stderr.Close()
			pm.stderr = nil
		}
		pm.cmd = nil
		pm.stdout = nil
		pm.running = false
		pm.mu.Unlock()
	})
}

// IsRunning returns whether the process is currently running.
func (pm *ProcessManager) IsRunning() bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.running
}

// WriteMessage writes a message to the process stdin.
func (pm *ProcessManager) WriteMessage(data []byte) error {
	pm.mu.Lock()
	stdin := pm.stdin
	running := pm.running
	pm.mu.Unlock()

	if !running || stdin == nil {
		return fmt.Errorf("process not running")
	}

	if _, err := stdin.Write(data); err != nil {
		return fmt.Errorf("failed to write to process: %v", err)
	}

	return nil
}

// Interrupt sends SIGINT to the process to interrupt the current operation.
func (pm *ProcessManager) Interrupt() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if !pm.running || pm.cmd == nil || pm.cmd.Process == nil {
		logger.Log("ProcessManager: Interrupt called but process not running")
		return nil
	}

	logger.Info("ProcessManager: Sending SIGINT to session %s (pid=%d)", pm.config.SessionID, pm.cmd.Process.Pid)

	if err := pm.cmd.Process.Signal(syscall.SIGINT); err != nil {
		logger.Error("ProcessManager: Failed to send SIGINT: %v", err)
		return fmt.Errorf("failed to send interrupt signal: %w", err)
	}

	return nil
}

// SetInterrupted marks the current operation as interrupted by the user.
func (pm *ProcessManager) SetInterrupted(interrupted bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.interrupted = interrupted
}

// GetRestartAttempts returns the number of restart attempts since last success.
func (pm *ProcessManager) GetRestartAttempts() int {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.restartAttempts
}

// ResetRestartAttempts resets the restart attempt counter.
func (pm *ProcessManager) ResetRestartAttempts() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.restartAttempts = 0
}

// UpdateConfig updates the process configuration.
// This should be called before Start() if the configuration changes.
func (pm *ProcessManager) UpdateConfig(config ProcessConfig) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.config = config
}

// MarkSessionStarted marks the session as started (for --resume flag on restart).
func (pm *ProcessManager) MarkSessionStarted() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.config.SessionStarted = true
}

// readOutput continuously reads from stdout and invokes callbacks.
func (pm *ProcessManager) readOutput() {
	logger.Log("ProcessManager: Output reader started for session %s", pm.config.SessionID)

	for {
		// Check for cancellation first
		select {
		case <-pm.ctx.Done():
			logger.Log("ProcessManager: Output reader exiting - context cancelled")
			return
		default:
		}

		pm.mu.Lock()
		running := pm.running
		reader := pm.stdout
		pm.mu.Unlock()

		if !running || reader == nil {
			logger.Log("ProcessManager: Output reader exiting - process not running")
			return
		}

		// Read with timeout to detect hung processes
		line, err := pm.readLineWithTimeout(reader)
		if err != nil {
			// Check if we were cancelled during the read
			select {
			case <-pm.ctx.Done():
				logger.Log("ProcessManager: Output reader exiting - context cancelled during read")
				return
			default:
			}

			if err == errReadTimeout {
				logger.Error("ProcessManager: Read timeout - process may be hung")
				pm.handleHung()
				return
			}

			if err == io.EOF {
				logger.Log("ProcessManager: EOF on stdout - process exited")
			} else {
				logger.Log("ProcessManager: Error reading stdout: %v", err)
			}
			// Process exit is handled by monitorExit goroutine
			return
		}

		if len(line) == 0 {
			continue
		}

		// Invoke callback for each line
		if pm.callbacks.OnLine != nil {
			pm.callbacks.OnLine(line)
		}
	}
}

// readLineWithTimeout reads a line with a timeout to detect hung processes.
func (pm *ProcessManager) readLineWithTimeout(reader *bufio.Reader) (string, error) {
	resultCh := make(chan readResult, 1)

	go func() {
		line, err := reader.ReadString('\n')
		resultCh <- readResult{line: line, err: err}
	}()

	select {
	case <-pm.ctx.Done():
		return "", pm.ctx.Err()
	case result := <-resultCh:
		return result.line, result.err
	case <-time.After(ResponseReadTimeout):
		return "", errReadTimeout
	}
}

// monitorExit waits for the process to exit and handles cleanup.
func (pm *ProcessManager) monitorExit() {
	pm.mu.Lock()
	cmd := pm.cmd
	pm.mu.Unlock()

	if cmd == nil {
		return
	}

	// Use a channel to detect when Wait() completes
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// Wait for either process exit or context cancellation
	select {
	case err := <-done:
		logger.Log("ProcessManager: Process exited: %v", err)
		pm.handleExit(err)
	case <-pm.ctx.Done():
		logger.Log("ProcessManager: Process monitor exiting - context cancelled")
	}
}

// handleHung handles the case when the process appears to be hung.
func (pm *ProcessManager) handleHung() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if !pm.running {
		return
	}

	logger.Error("ProcessManager: Process appears hung for session %s, killing", pm.config.SessionID)

	// Kill the hung process
	if pm.cmd != nil && pm.cmd.Process != nil {
		pm.cmd.Process.Kill()
	}

	// Invoke callback
	if pm.callbacks.OnProcessHung != nil {
		pm.callbacks.OnProcessHung()
	}

	// Clean up
	pm.cleanupLocked()
}

// handleExit handles cleanup and potential restart when the process exits.
func (pm *ProcessManager) handleExit(err error) {
	pm.mu.Lock()

	if !pm.running {
		pm.mu.Unlock()
		return
	}

	logger.Log("ProcessManager: Handling process exit for session %s", pm.config.SessionID)

	wasInterrupted := pm.interrupted
	pm.interrupted = false // Reset for next operation
	restartAttempts := pm.restartAttempts
	stopped := pm.stopped

	// Read stderr to get actual error message before closing
	var stderrContent string
	if pm.stderr != nil {
		stderrBytes, readErr := io.ReadAll(pm.stderr)
		if readErr != nil {
			logger.Log("ProcessManager: Failed to read stderr: %v", readErr)
		} else if len(stderrBytes) > 0 {
			stderrContent = strings.TrimSpace(string(stderrBytes))
			logger.Log("ProcessManager: Stderr output: %s", stderrContent)
		}
	}

	// Clean up pipes
	pm.cleanupLocked()
	pm.mu.Unlock()

	// If user interrupted or manager is stopped, don't attempt restart
	if wasInterrupted || stopped {
		logger.Log("ProcessManager: Process exit due to user interrupt or stop, not restarting")
		if pm.callbacks.OnProcessExit != nil {
			pm.callbacks.OnProcessExit(err, stderrContent)
		}
		return
	}

	// Check with callback if we should restart
	shouldRestart := true
	if pm.callbacks.OnProcessExit != nil {
		shouldRestart = pm.callbacks.OnProcessExit(err, stderrContent)
	}

	if !shouldRestart {
		return
	}

	// Check if we should attempt restart
	if restartAttempts < MaxProcessRestartAttempts {
		pm.mu.Lock()
		pm.restartAttempts = restartAttempts + 1
		pm.lastRestartTime = time.Now()
		pm.mu.Unlock()

		logger.Warn("ProcessManager: Process crashed, attempting restart %d/%d for session %s",
			restartAttempts+1, MaxProcessRestartAttempts, pm.config.SessionID)

		// Notify about restart attempt
		if pm.callbacks.OnRestartAttempt != nil {
			pm.callbacks.OnRestartAttempt(restartAttempts + 1)
		}

		// Wait before restart attempt
		time.Sleep(ProcessRestartDelay)

		// Attempt restart
		if err := pm.Start(); err != nil {
			logger.Error("ProcessManager: Failed to restart process: %v", err)
			if pm.callbacks.OnRestartFailed != nil {
				pm.callbacks.OnRestartFailed(err)
			}
			// Report fatal error
			exitErr := fmt.Errorf("process crashed and restart failed: %v", err)
			if pm.callbacks.OnFatalError != nil {
				pm.callbacks.OnFatalError(exitErr)
			}
		} else {
			logger.Info("ProcessManager: Process restarted successfully for session %s", pm.config.SessionID)
		}
		return
	}

	// Max restarts exceeded - report fatal error
	logger.Error("ProcessManager: Max restart attempts (%d) exceeded for session %s", MaxProcessRestartAttempts, pm.config.SessionID)
	var exitErr error
	if stderrContent != "" {
		exitErr = fmt.Errorf("process crashed repeatedly (max %d restarts): %s", MaxProcessRestartAttempts, stderrContent)
	} else if err != nil {
		exitErr = fmt.Errorf("process crashed repeatedly (max %d restarts): %v", MaxProcessRestartAttempts, err)
	} else {
		exitErr = fmt.Errorf("process crashed repeatedly (max %d restarts exceeded)", MaxProcessRestartAttempts)
	}

	if pm.callbacks.OnFatalError != nil {
		pm.callbacks.OnFatalError(exitErr)
	}
}

// cleanupLocked cleans up process resources. Must be called with mu held.
func (pm *ProcessManager) cleanupLocked() {
	if pm.stdin != nil {
		pm.stdin.Close()
		pm.stdin = nil
	}
	if pm.stderr != nil {
		pm.stderr.Close()
		pm.stderr = nil
	}
	pm.cmd = nil
	pm.stdout = nil
	pm.running = false
}

// Ensure ProcessManager implements ProcessManagerInterface at compile time.
var _ ProcessManagerInterface = (*ProcessManager)(nil)
