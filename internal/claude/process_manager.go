package claude

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/zhubert/plural/internal/paths"
)

// errChannelFull is returned when the response channel is full for too long.
var errChannelFull = fmt.Errorf("channel full")

// containerMCPConfigPath is where the MCP config is mounted inside the container.
const containerMCPConfigPath = "/tmp/mcp.json"

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
	SessionID              string
	WorkingDir             string
	RepoPath               string // Main repository path (for containerized worktree support)
	SessionStarted         bool
	AllowedTools           []string
	MCPConfigPath          string
	ForkFromSessionID      string // When set, uses --resume <parentID> --fork-session to inherit parent conversation
	Containerized          bool   // When true, wraps Claude CLI in a container
	ContainerImage         string // Container image name (e.g., "ghcr.io/zhubert/plural-claude")
	Supervisor             bool   // When true, appends supervisor instructions to system prompt
	DisableStreamingChunks bool   // When true, omits --include-partial-messages for less verbose output (useful for agent mode)
}

// ProcessCallbacks defines callbacks that the ProcessManager invokes during operation.
// This allows the Runner to respond to process events without tight coupling.
//
// Callback Threading Model:
// All callbacks are invoked from the ProcessManager's internal goroutines.
// Implementations should be thread-safe and avoid blocking operations that
// could delay process management.
//
// Callback Invocation Order:
// 1. OnLine: Called repeatedly as stdout produces lines
// 2. OnProcessExit: Called when process exits, return value determines restart
// 3. If restarting:
//   - OnRestartAttempt: Called before each restart attempt
//   - OnRestartFailed: Called if restart fails
//   - OnFatalError: Called when max restarts exceeded
//
// Example implementation:
//
//	callbacks := ProcessCallbacks{
//	    OnLine: func(line string) {
//	        // Parse JSON and route to response channels
//	        chunks := parseStreamMessage(line)
//	        for _, chunk := range chunks {
//	            responseCh <- chunk
//	        }
//	    },
//	    OnProcessExit: func(err error, stderr string) bool {
//	        // Return true to allow restart, false to prevent
//	        return !userInterrupted && !responseComplete
//	    },
//	    OnFatalError: func(err error) {
//	        // Send error to user via response channel
//	        responseCh <- ResponseChunk{Error: err, Done: true}
//	    },
//	    OnContainerReady: func() {
//	        // Signal that container initialization is complete
//	        stateManager.StopContainerInit(sessionID)
//	    },
//	}
type ProcessCallbacks struct {
	// OnLine is called for each line read from stdout.
	// The line includes the trailing newline.
	// This callback is called synchronously from the output reader goroutine.
	OnLine func(line string)

	// OnContainerReady is called when a containerized session receives its init message.
	// This signals that the container is fully initialized and ready to accept user messages.
	// Not called for non-containerized sessions.
	OnContainerReady func()

	// OnProcessExit is called when the process exits unexpectedly.
	// The error parameter contains the exit reason (may be nil for clean exit).
	// The stderrContent contains any stderr output from the process.
	// Returns true if the process should be restarted, false to prevent restart.
	// Returning false is appropriate when:
	//   - The user interrupted the operation (e.g., pressed Escape)
	//   - The response was already complete (result message received)
	//   - The ProcessManager was explicitly stopped
	OnProcessExit func(err error, stderrContent string) bool

	// OnRestartAttempt is called when a restart is being attempted.
	// attemptNum is 1-indexed (1, 2, 3, ...).
	// This is called before the actual restart attempt.
	OnRestartAttempt func(attemptNum int)

	// OnRestartFailed is called when a restart attempt fails.
	// This is followed by OnFatalError if max attempts are exceeded.
	OnRestartFailed func(err error)

	// OnFatalError is called when max restarts exceeded or unrecoverable error.
	// After this callback, the ProcessManager will not attempt further restarts.
	// The Runner should clean up and report the error to the user.
	OnFatalError func(err error)
}

// ProcessManager manages the lifecycle of a Claude CLI process.
// It handles starting, stopping, monitoring, and auto-recovery of the process.
type ProcessManager struct {
	config    ProcessConfig
	callbacks ProcessCallbacks
	log       *slog.Logger

	// Process state (protected by mu)
	mu              sync.Mutex
	cmd             *exec.Cmd
	stdin           io.WriteCloser
	stdout          *bufio.Reader
	stderr          io.ReadCloser
	stderrContent   string        // Captured stderr content (read by drainStderr goroutine)
	stderrDone      chan struct{} // Signals when stderr has been fully read
	running         bool
	interrupted     bool
	restartAttempts int
	lastRestartTime time.Time

	// waitDone is closed by monitorExit when cmd.Wait() completes.
	// Stop() selects on this channel instead of calling cmd.Wait() again,
	// preventing undefined behavior from double Wait().
	waitDone chan struct{}

	// Context for process goroutines
	ctx    context.Context
	cancel context.CancelFunc

	// Goroutine lifecycle management
	wg sync.WaitGroup

	// Container startup watchdog
	containerReady chan struct{} // closed when MarkSessionStarted is called
	containerTimeout bool       // set by watchdog before killing
	containerLogs    string     // captured docker logs on timeout
}

// NewProcessManager creates a new ProcessManager with the given configuration and callbacks.
func NewProcessManager(config ProcessConfig, callbacks ProcessCallbacks, log *slog.Logger) *ProcessManager {
	return &ProcessManager{
		config:    config,
		callbacks: callbacks,
		log:       log,
	}
}

// BuildCommandArgs builds the command line arguments for the Claude CLI based on the config.
// This is exported for testing purposes to verify correct argument construction.
func BuildCommandArgs(config ProcessConfig) []string {
	var args []string
	if config.SessionStarted && !config.Containerized {
		// Session already started - resume our own session
		// (Skip resume in container mode: each container run is a fresh environment
		// with no prior session data, so --resume would fail with "No conversation found")
		args = []string{
			"--print",
			"--output-format", "stream-json",
			"--input-format", "stream-json",
			"--verbose",
			"--resume", config.SessionID,
		}
		// Add streaming chunks flag unless disabled (e.g., for agent mode)
		if !config.DisableStreamingChunks {
			args = append(args, "--include-partial-messages")
		}
	} else if config.ForkFromSessionID != "" && !config.Containerized {
		// Forked session - resume parent and fork to inherit conversation history
		// We must pass --session-id to ensure Claude uses our UUID for the forked session,
		// otherwise Claude generates its own ID and we can't resume later.
		// Skip in container mode: each container is a fresh environment with no parent
		// session data, so --resume would fail with "No conversation found".
		args = []string{
			"--print",
			"--output-format", "stream-json",
			"--input-format", "stream-json",
			"--verbose",
			"--resume", config.ForkFromSessionID,
			"--fork-session",
			"--session-id", config.SessionID,
		}
		// Add streaming chunks flag unless disabled (e.g., for agent mode)
		if !config.DisableStreamingChunks {
			args = append(args, "--include-partial-messages")
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
		// Add streaming chunks flag unless disabled (e.g., for agent mode)
		if !config.DisableStreamingChunks {
			args = append(args, "--include-partial-messages")
		}
	}

	// Build system prompt: base options prompt + supervisor instructions if applicable
	systemPrompt := OptionsSystemPrompt
	if config.Supervisor {
		systemPrompt = OptionsSystemPrompt + "\n\n" + SupervisorSystemPrompt
	}

	if config.Containerized {
		// Container IS the sandbox. When MCP config is available, use --permission-prompt-tool
		// with a wildcard MCP server (--auto-approve) that auto-approves all regular permissions
		// while routing AskUserQuestion and ExitPlanMode through the TUI.
		// Note: --dangerously-skip-permissions and --permission-prompt-tool conflict in Claude CLI,
		// so we use one or the other — never both.
		if config.MCPConfigPath != "" {
			args = append(args,
				"--mcp-config", containerMCPConfigPath,
				"--permission-prompt-tool", "mcp__plural__permission",
			)
		} else {
			// Fallback if MCP server didn't start — use dangerously-skip-permissions
			args = append(args, "--dangerously-skip-permissions")
		}
		args = append(args, "--append-system-prompt", systemPrompt)

		// Pre-authorize all tools — the container is the sandbox
		for _, tool := range ContainerAllowedTools {
			args = append(args, "--allowedTools", tool)
		}
	} else {
		// Add MCP config and permission prompt tool
		args = append(args,
			"--mcp-config", config.MCPConfigPath,
			"--permission-prompt-tool", "mcp__plural__permission",
			"--append-system-prompt", systemPrompt,
		)

		// Add pre-allowed tools
		for _, tool := range config.AllowedTools {
			args = append(args, "--allowedTools", tool)
		}
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

	pm.log.Info("starting process")
	startTime := time.Now()

	// Container mode requires credentials (short-lived OAuth tokens rotate and would become invalid)
	if pm.config.Containerized && !ContainerAuthAvailable() {
		return fmt.Errorf("container mode requires authentication: set ANTHROPIC_API_KEY, CLAUDE_CODE_OAUTH_TOKEN, run 'claude login', or add 'anthropic_api_key' to macOS keychain")
	}

	// Build command arguments
	args := BuildCommandArgs(pm.config)

	// Log fork operation if applicable
	if pm.config.ForkFromSessionID != "" {
		pm.log.Debug("forking session from parent", "parentSessionID", pm.config.ForkFromSessionID)
	}

	var cmd *exec.Cmd
	if pm.config.Containerized {
		// Remove any stale container with the same name from a previous crash.
		// docker run --rm only cleans up on clean exit, so a crashed container
		// may still be lingering and block the new docker run.
		containerName := "plural-" + pm.config.SessionID
		rmCmd := exec.Command("docker", "rm", "-f", containerName)
		if rmOut, rmErr := rmCmd.CombinedOutput(); rmErr != nil {
			pm.log.Debug("pre-start container cleanup (may not exist)", "name", containerName, "output", strings.TrimSpace(string(rmOut)))
		} else {
			pm.log.Info("removed stale container before start", "name", containerName)
		}

		result, err := buildContainerRunArgs(pm.config, args)
		if err != nil {
			return err
		}
		if result.AuthSource != "" {
			pm.log.Info("container auth credential source", "source", result.AuthSource)
		} else {
			pm.log.Warn("no auth credentials found for container")
		}
		pm.log.Debug("starting containerized process", "command", "docker "+strings.Join(result.Args, " "))
		cmd = exec.Command("docker", result.Args...)
		// Don't set cmd.Dir — the container's -w flag handles the working directory
	} else {
		pm.log.Debug("starting process", "command", "claude "+strings.Join(args, " "))
		cmd = exec.Command("claude", args...)
		cmd.Dir = pm.config.WorkingDir
	}

	// Get stdin pipe for writing messages
	stdin, err := cmd.StdinPipe()
	if err != nil {
		pm.log.Error("failed to get stdin pipe", "error", err)
		return fmt.Errorf("failed to get stdin pipe: %v", err)
	}

	// Get stdout pipe for reading responses
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		pm.log.Error("failed to get stdout pipe", "error", err)
		return fmt.Errorf("failed to get stdout pipe: %v", err)
	}

	// Get stderr pipe for error messages
	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		pm.log.Error("failed to get stderr pipe", "error", err)
		return fmt.Errorf("failed to get stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		pm.log.Error("failed to start process", "error", err)
		if pm.config.Containerized {
			return fmt.Errorf("failed to start container: %v (is Docker running?)", err)
		}
		return fmt.Errorf("failed to start process: %v", err)
	}

	pm.cmd = cmd
	pm.stdin = stdin
	pm.stdout = bufio.NewReader(stdout)
	pm.stderr = stderr
	pm.stderrContent = ""
	pm.stderrDone = make(chan struct{})
	pm.waitDone = make(chan struct{})
	pm.running = true

	// Cancel any previous context to prevent goroutine leaks from prior runs
	if pm.cancel != nil {
		pm.cancel()
	}
	// Create context for process goroutines
	pm.ctx, pm.cancel = context.WithCancel(context.Background())

	// Initialize container watchdog state for containerized sessions
	if pm.config.Containerized {
		pm.containerReady = make(chan struct{})
		pm.containerTimeout = false
		pm.containerLogs = ""
	}

	pm.log.Info("process started", "elapsed", time.Since(startTime), "pid", cmd.Process.Pid)

	// Start goroutines to read output, drain stderr, and monitor process
	// Track them with WaitGroup for proper cleanup on Stop()
	goroutines := 3
	if pm.config.Containerized {
		goroutines = 4 // +1 for watchdog
	}
	pm.wg.Add(goroutines)
	go func() {
		defer pm.wg.Done()
		pm.readOutput()
	}()
	go func() {
		defer pm.wg.Done()
		pm.drainStderr()
	}()
	go func() {
		defer pm.wg.Done()
		pm.monitorExit()
	}()
	if pm.config.Containerized {
		go func() {
			defer pm.wg.Done()
			pm.containerStartupWatchdog()
		}()
	}

	return nil
}

// Stop stops the persistent process gracefully.
// It waits for all goroutines (readOutput, monitorExit) to complete before returning.
// Safe to call multiple times — subsequent calls are no-ops.
func (pm *ProcessManager) Stop() {
	pm.mu.Lock()
	wasRunning := pm.running

	// Cancel context first to signal goroutines to exit
	if pm.cancel != nil {
		pm.cancel()
		pm.cancel = nil
	}

	if !wasRunning {
		pm.mu.Unlock()
		return
	}

	pm.log.Debug("stopping process")

	// Mark as not running immediately to prevent concurrent Stop() from
	// doing duplicate cleanup
	pm.running = false

	// Close stdin to signal EOF to the process
	if pm.stdin != nil {
		pm.stdin.Close()
		pm.stdin = nil
	}

	cmd := pm.cmd
	waitDone := pm.waitDone
	pm.mu.Unlock()

	// Wait for the process to exit using the waitDone channel from monitorExit.
	// monitorExit is the sole caller of cmd.Wait(), and signals waitDone when
	// it completes. This avoids calling cmd.Wait() twice (undefined behavior).
	if cmd != nil && cmd.Process != nil && waitDone != nil {
		select {
		case <-waitDone:
			pm.log.Debug("process exited gracefully")
		case <-time.After(2 * time.Second):
			pm.log.Debug("force killing process")
			cmd.Process.Kill()
			// Wait for monitorExit's cmd.Wait() to finish after kill
			<-waitDone
		}
	}

	// Defense-in-depth: force remove the container if we were running in container mode
	if pm.config.Containerized {
		containerName := "plural-" + pm.config.SessionID
		pm.log.Debug("removing container", "name", containerName)
		rmCmd := exec.Command("docker", "rm", "-f", containerName)
		if err := rmCmd.Run(); err != nil {
			pm.log.Debug("container rm failed (may already be removed)", "error", err)
		}

		// Clean up the auth secrets file from the host
		if authFile := containerAuthFilePath(pm.config.SessionID); authFile != "" {
			os.Remove(authFile)
		}
	}

	// Wait for goroutines (readOutput, monitorExit) to complete
	// This prevents resource leaks when process is started/stopped quickly
	pm.log.Debug("waiting for goroutines to complete")
	pm.wg.Wait()
	pm.log.Debug("all goroutines completed")

	pm.mu.Lock()
	if pm.stderr != nil {
		pm.stderr.Close()
		pm.stderr = nil
	}
	pm.cmd = nil
	pm.stdout = nil
	pm.mu.Unlock()
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
		pm.log.Debug("interrupt called but process not running")
		return nil
	}

	pm.log.Info("sending SIGINT", "pid", pm.cmd.Process.Pid)

	if err := pm.cmd.Process.Signal(syscall.SIGINT); err != nil {
		pm.log.Error("failed to send SIGINT", "error", err)
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
	wasContainerized := pm.config.Containerized
	callback := pm.callbacks.OnContainerReady
	pm.config.SessionStarted = true

	// Signal the watchdog that container startup succeeded
	if pm.containerReady != nil {
		select {
		case <-pm.containerReady:
			// Already closed
		default:
			close(pm.containerReady)
		}
	}
	pm.mu.Unlock()

	// Notify that container is ready (if this was a containerized session)
	if wasContainerized && callback != nil {
		callback()
	}
}

// readOutput continuously reads from stdout and invokes callbacks.
func (pm *ProcessManager) readOutput() {
	pm.log.Debug("output reader started")

	for {
		// Check for cancellation first
		select {
		case <-pm.ctx.Done():
			pm.log.Debug("output reader exiting - context cancelled")
			return
		default:
		}

		pm.mu.Lock()
		running := pm.running
		reader := pm.stdout
		pm.mu.Unlock()

		if !running || reader == nil {
			pm.log.Debug("output reader exiting - process not running")
			return
		}

		line, err := pm.readLine(reader)
		if err != nil {
			// Check if we were cancelled during the read
			select {
			case <-pm.ctx.Done():
				pm.log.Debug("output reader exiting - context cancelled during read")
				return
			default:
			}

			if err == io.EOF {
				pm.log.Debug("EOF on stdout - process exited")
			} else {
				pm.log.Debug("error reading stdout", "error", err)
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

// readLine reads a line from the reader, blocking until data is available.
//
// IMPORTANT: The spawned goroutine doing ReadString() cannot be cancelled
// (Go's blocking I/O limitation). However, this is acceptable because:
// 1. On context cancel, stdin is closed by Stop(), which unblocks the read with EOF
// 2. The goroutine will exit once the read completes (success or EOF)
//
// The channel is buffered (size 1) so the goroutine can always send its result
// even if we've already returned due to cancel, preventing a goroutine leak.
func (pm *ProcessManager) readLine(reader *bufio.Reader) (string, error) {
	resultCh := make(chan readResult, 1)

	go func() {
		line, err := reader.ReadString('\n')
		// Non-blocking send - channel is buffered so this always succeeds
		// even if the main function has returned due to cancel
		resultCh <- readResult{line: line, err: err}
	}()

	select {
	case <-pm.ctx.Done():
		// Context cancelled - the read goroutine will exit when stdin is closed
		// or process is killed, which happens in Stop()
		return "", pm.ctx.Err()
	case result := <-resultCh:
		return result.line, result.err
	}
}

// drainStderr reads all stderr content and stores it for later retrieval.
// This must run concurrently with the process so stderr is captured before
// cmd.Wait() closes the pipe.
func (pm *ProcessManager) drainStderr() {
	defer close(pm.stderrDone)

	pm.mu.Lock()
	stderr := pm.stderr
	pm.mu.Unlock()

	if stderr == nil {
		return
	}

	stderrBytes, err := io.ReadAll(stderr)
	if err != nil {
		pm.log.Debug("error reading stderr", "error", err)
		return
	}

	if len(stderrBytes) > 0 {
		pm.mu.Lock()
		pm.stderrContent = strings.TrimSpace(string(stderrBytes))
		pm.mu.Unlock()
		pm.log.Debug("captured stderr", "content", pm.stderrContent)
	}
}

// monitorExit waits for the process to exit and handles cleanup.
// It is the sole caller of cmd.Wait() — Stop() coordinates via the
// waitDone channel instead of calling cmd.Wait() itself, preventing
// undefined behavior from double Wait().
func (pm *ProcessManager) monitorExit() {
	pm.mu.Lock()
	cmd := pm.cmd
	waitDone := pm.waitDone
	pm.mu.Unlock()

	if cmd == nil {
		if waitDone != nil {
			close(waitDone)
		}
		return
	}

	// Wait for cmd.Wait() in a goroutine so we can also select on context.
	// The goroutine's result is always consumed — either for handleExit
	// or just to ensure cmd.Wait() completes before signaling waitDone.
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// Wait for either process exit or context cancellation
	select {
	case err := <-done:
		pm.log.Debug("process exited", "error", err)
		// Signal that cmd.Wait() has completed before handling exit,
		// so Stop() can proceed while handleExit runs
		if waitDone != nil {
			close(waitDone)
		}
		pm.handleExit(err)
	case <-pm.ctx.Done():
		pm.log.Debug("process monitor - context cancelled, waiting for cmd.Wait()")
		// Context was cancelled (Stop() called). We must still consume
		// cmd.Wait() to avoid a goroutine leak and ensure proper cleanup.
		// Stop() closes stdin and may kill the process, which unblocks Wait().
		<-done
		if waitDone != nil {
			close(waitDone)
		}
	}
}

// handleExit handles cleanup and potential restart when the process exits.
func (pm *ProcessManager) handleExit(err error) {
	pm.mu.Lock()

	if !pm.running {
		pm.mu.Unlock()
		return
	}

	pm.log.Debug("handling process exit")

	wasInterrupted := pm.interrupted
	pm.interrupted = false // Reset for next operation
	restartAttempts := pm.restartAttempts
	stderrDone := pm.stderrDone

	// Check if context was cancelled (Stop() was called)
	ctxCancelled := pm.ctx != nil && pm.ctx.Err() != nil
	pm.mu.Unlock()

	// Wait for stderr to be fully drained (drainStderr goroutine reads it
	// concurrently before cmd.Wait() closes the pipe)
	if stderrDone != nil {
		<-stderrDone
	}

	pm.mu.Lock()
	stderrContent := pm.stderrContent
	if stderrContent != "" {
		pm.log.Debug("stderr output", "content", stderrContent)
	}

	// Clean up pipes
	pm.cleanupLocked()
	pm.mu.Unlock()

	// If user interrupted or Stop() was called, don't attempt restart
	if wasInterrupted || ctxCancelled {
		pm.log.Debug("process exit due to user interrupt or stop, not restarting")
		if pm.callbacks.OnProcessExit != nil {
			pm.callbacks.OnProcessExit(err, stderrContent)
		}
		return
	}

	// If container startup timed out, report fatal error without retrying.
	// The watchdog already killed the process; retrying won't help since the
	// root cause is typically a broken/outdated container image.
	pm.mu.Lock()
	wasContainerTimeout := pm.containerTimeout
	containerLogs := pm.containerLogs
	pm.mu.Unlock()

	if wasContainerTimeout {
		pm.log.Error("container startup timed out", "timeout", ContainerStartupTimeout)

		// Clean up auth credentials file
		if pm.config.Containerized {
			if authFile := containerAuthFilePath(pm.config.SessionID); authFile != "" {
				if removeErr := os.Remove(authFile); removeErr == nil {
					pm.log.Debug("cleaned up auth file on container timeout", "path", authFile)
				}
			}
		}

		errMsg := fmt.Sprintf("container failed to start within %s — Claude CLI produced no output", ContainerStartupTimeout)
		if containerLogs != "" {
			errMsg += fmt.Sprintf("\n\nContainer logs:\n%s", containerLogs)
		}
		errMsg += "\n\nThis usually means the container image is outdated. Try pulling the latest image."

		if pm.callbacks.OnFatalError != nil {
			pm.callbacks.OnFatalError(fmt.Errorf("%s", errMsg))
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

		pm.log.Warn("process crashed, attempting restart",
			"attempt", restartAttempts+1,
			"maxAttempts", MaxProcessRestartAttempts)

		// Notify about restart attempt
		if pm.callbacks.OnRestartAttempt != nil {
			pm.callbacks.OnRestartAttempt(restartAttempts + 1)
		}

		// Wait before restart attempt
		time.Sleep(ProcessRestartDelay)

		// Attempt restart
		if err := pm.Start(); err != nil {
			pm.log.Error("failed to restart process", "error", err)
			if pm.callbacks.OnRestartFailed != nil {
				pm.callbacks.OnRestartFailed(err)
			}
			// Clean up auth credentials file on fatal restart failure
			if pm.config.Containerized {
				if authFile := containerAuthFilePath(pm.config.SessionID); authFile != "" {
					if removeErr := os.Remove(authFile); removeErr == nil {
						pm.log.Debug("cleaned up auth file on restart failure", "path", authFile)
					}
				}
			}
			// Report fatal error
			exitErr := fmt.Errorf("process crashed and restart failed: %v", err)
			if pm.callbacks.OnFatalError != nil {
				pm.callbacks.OnFatalError(exitErr)
			}
		} else {
			pm.log.Info("process restarted successfully")
		}
		return
	}

	// Max restarts exceeded - report fatal error
	pm.log.Error("max restart attempts exceeded", "maxAttempts", MaxProcessRestartAttempts)

	// Clean up auth credentials file that would otherwise persist on disk
	if pm.config.Containerized {
		if authFile := containerAuthFilePath(pm.config.SessionID); authFile != "" {
			if err := os.Remove(authFile); err == nil {
				pm.log.Debug("cleaned up auth file on fatal error", "path", authFile)
			}
		}
	}

	var exitErr error
	if stderrContent != "" {
		friendly := friendlyContainerError(stderrContent, pm.config.Containerized)
		exitErr = fmt.Errorf("process crashed repeatedly (max %d restarts): %s", MaxProcessRestartAttempts, friendly)
	} else if err != nil {
		exitErr = fmt.Errorf("process crashed repeatedly (max %d restarts): %v", MaxProcessRestartAttempts, err)
	} else {
		exitErr = fmt.Errorf("process crashed repeatedly (max %d restarts exceeded)", MaxProcessRestartAttempts)
	}

	if pm.callbacks.OnFatalError != nil {
		pm.callbacks.OnFatalError(exitErr)
	}
}

// containerRunResult holds the result of building container run arguments.
type containerRunResult struct {
	Args       []string // Arguments for `docker run`
	AuthSource string   // Credential source used (empty if none)
}

// buildContainerRunArgs constructs the arguments for `docker run` that wraps
// the Claude CLI process inside a Docker container.
func buildContainerRunArgs(config ProcessConfig, claudeArgs []string) (containerRunResult, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return containerRunResult{}, fmt.Errorf("failed to determine home directory: %w", err)
	}

	containerName := "plural-" + config.SessionID
	image := config.ContainerImage
	if image == "" {
		image = "ghcr.io/zhubert/plural-claude"
	}

	args := []string{
		"run", "-i", "--rm",
		"--name", containerName,
		"-v", config.WorkingDir + ":/workspace",
		"-v", homeDir + "/.claude:/home/claude/.claude-host:ro",
		"-w", "/workspace",
	}

	// On Linux, explicitly map host.docker.internal to the host gateway IP.
	// On macOS/Windows, Docker Desktop and Colima both provide native
	// host.docker.internal resolution. Passing --add-host here would override
	// Colima's correct mapping (which points to the macOS host) with the Docker
	// bridge gateway (which points to the Lima VM), breaking container→host
	// communication.
	if runtime.GOOS == "linux" {
		args = append(args, "--add-host", "host.docker.internal:host-gateway")
	}

	// Pass auth credentials via --env-file.
	// On macOS, Claude Code stores auth in the system keychain which isn't
	// accessible inside a Linux container. We write the key to a temp file
	// (0600 permissions) and pass it via --env-file, which sets the env var
	// directly in the container process. This is safer than -e which would
	// expose the key in `ps` output on the host.
	auth := writeContainerAuthFile(config.SessionID)
	if auth.Path != "" {
		args = append(args, "--env-file", auth.Path)
	} else if credentialsFileExists() {
		// No env var or keychain credentials, but .credentials.json exists on the host.
		// The entrypoint copies it into the container's ~/.claude/, so Claude CLI
		// will find it and handle token refresh natively. No --env-file needed.
		auth.Source = "~/.claude/.credentials.json (OAuth via claude login)"
	}

	// Mount MCP config for AskUserQuestion/ExitPlanMode support.
	// The MCP subprocess inside the container connects to the host via TCP
	// (Unix sockets don't work across the Docker container boundary).
	if config.MCPConfigPath != "" {
		args = append(args, "-v", config.MCPConfigPath+":"+containerMCPConfigPath+":ro")
	}

	// Mount main repository for git worktree support.
	// Git worktrees have a .git file pointing to /path/to/repo/.git/worktrees/<id>.
	// We mount the repo at its original absolute path so these references work transparently.
	// Note: Must be read-write because git needs to update .git/worktrees/<id>/ when committing.
	if config.RepoPath != "" {
		args = append(args, "-v", config.RepoPath+":"+config.RepoPath)
	}

	args = append(args, image)
	args = append(args, claudeArgs...)
	return containerRunResult{Args: args, AuthSource: auth.Source}, nil
}

// containerAuthDir returns the directory for storing container auth files.
// Uses the config directory which is user-private, unlike /tmp which is world-readable.
// Returns empty string if the config directory cannot be determined (credentials
// will not be written rather than falling back to an insecure location).
func containerAuthDir() string {
	dir, err := paths.ConfigDir()
	if err != nil {
		return ""
	}
	os.MkdirAll(dir, 0700)
	return dir
}

// containerAuthFilePath returns the path for a session's container auth file.
// Returns empty string if the auth directory cannot be determined.
func containerAuthFilePath(sessionID string) string {
	dir := containerAuthDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, fmt.Sprintf("plural-auth-%s", sessionID))
}

// ContainerAuthAvailable checks whether credentials are available for
// container mode. Returns true if any of the following are set:
//   - ANTHROPIC_API_KEY environment variable
//   - CLAUDE_CODE_OAUTH_TOKEN environment variable (long-lived token from "claude setup-token")
//   - "anthropic_api_key" or "Claude Code" macOS keychain entry
//   - ~/.claude/.credentials.json file (from "claude login" interactive OAuth)
func ContainerAuthAvailable() bool {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return true
	}
	if os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") != "" {
		return true
	}
	if readKeychainAPIKey() != "" {
		return true
	}
	if credentialsFileExists() {
		return true
	}
	return false
}

// readKeychainAPIKey reads an API key from the macOS keychain, checking
// both the legacy "anthropic_api_key" service and the "Claude Code" service
// (used by `claude login` on macOS).
func readKeychainAPIKey() string {
	if key := readKeychainPassword("anthropic_api_key"); key != "" {
		return key
	}
	return readKeychainPassword("Claude Code")
}

// credentialsFileExists checks whether ~/.claude/.credentials.json exists.
// This file is created by "claude login" (interactive OAuth) and contains
// refresh tokens that Claude CLI can use to obtain access tokens.
func credentialsFileExists() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".claude", ".credentials.json"))
	return err == nil
}

// containerAuthResult holds the result of writing a container auth file.
type containerAuthResult struct {
	Path   string // File path, empty if no credentials available
	Source string // Credential source description for logging
}

// writeContainerAuthFile writes credentials to a file in ~/.plural/ with
// restricted permissions (0600) and returns the file path and source.
// The file is passed to Docker via --env-file, which sets the env var
// directly in the container process.
//
// File format: ENV_VAR_NAME=value (Docker env-file format, no quotes)
//
// Credential sources (in priority order):
//  1. ANTHROPIC_API_KEY from environment
//  2. CLAUDE_CODE_OAUTH_TOKEN from environment (long-lived token from "claude setup-token")
//  3. "anthropic_api_key" macOS keychain entry
//
// Note: The short-lived OAuth access token from the macOS keychain (rotated
// every ~8-12 hours by the native CLI) is NOT supported — it would become
// invalid inside the container. Use "claude setup-token" to generate a
// long-lived CLAUDE_CODE_OAUTH_TOKEN instead.
//
// Returns empty path if no credentials are available.
func writeContainerAuthFile(sessionID string) containerAuthResult {
	var content string
	var source string

	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		content = "ANTHROPIC_API_KEY=" + apiKey
		source = "ANTHROPIC_API_KEY env var"
	} else if oauthToken := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"); oauthToken != "" {
		// Claude CLI recognizes CLAUDE_CODE_OAUTH_TOKEN directly as an environment variable
		content = "CLAUDE_CODE_OAUTH_TOKEN=" + oauthToken
		source = "CLAUDE_CODE_OAUTH_TOKEN env var"
	} else if apiKey := readKeychainAPIKey(); apiKey != "" {
		content = "ANTHROPIC_API_KEY=" + apiKey
		source = "macOS keychain"
	}

	if content == "" {
		return containerAuthResult{}
	}

	// Validate credential value has no newlines that would break Docker env-file format
	// (Docker env-file doesn't support multiline values)
	parts := strings.SplitN(content, "=", 2)
	if len(parts) == 2 && strings.ContainsAny(parts[1], "\n\r") {
		return containerAuthResult{}
	}

	path := containerAuthFilePath(sessionID)
	if path == "" {
		return containerAuthResult{}
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return containerAuthResult{}
	}
	return containerAuthResult{Path: path, Source: source}
}

// readKeychainPassword reads a password from the macOS keychain.
// Returns empty string if not found, on error, or on non-macOS platforms.
func readKeychainPassword(service string) string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	out, err := exec.Command("security", "find-generic-password", "-s", service, "-w").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
	pm.stderrContent = ""
	pm.stderrDone = nil
	pm.waitDone = nil
	pm.running = false

	// Close containerReady if still open to unblock the watchdog goroutine.
	// This handles the case where the process crashes before the init message
	// is received (non-timeout crash).
	if pm.containerReady != nil {
		select {
		case <-pm.containerReady:
			// Already closed
		default:
			close(pm.containerReady)
		}
	}
}

// containerStartupWatchdog monitors containerized session startup and kills the
// process if it doesn't produce output within ContainerStartupTimeout.
// This prevents the UI from hanging forever when Claude CLI inside the container
// hangs during initialization (e.g., MCP server init with an outdated image).
func (pm *ProcessManager) containerStartupWatchdog() {
	pm.log.Debug("container startup watchdog started", "timeout", ContainerStartupTimeout)

	pm.mu.Lock()
	ready := pm.containerReady
	pm.mu.Unlock()

	if ready == nil {
		pm.log.Debug("container startup watchdog exiting - no containerReady channel")
		return
	}

	select {
	case <-time.After(ContainerStartupTimeout):
		pm.log.Error("container startup watchdog fired - killing process", "timeout", ContainerStartupTimeout)

		// Capture docker logs before killing the process for diagnostics
		containerName := "plural-" + pm.config.SessionID
		logCmd := exec.Command("docker", "logs", "--tail", "50", containerName)
		logOutput, logErr := logCmd.CombinedOutput()
		var logs string
		if logErr == nil && len(logOutput) > 0 {
			logs = strings.TrimSpace(string(logOutput))
		}

		pm.mu.Lock()
		pm.containerTimeout = true
		pm.containerLogs = logs
		cmd := pm.cmd
		pm.mu.Unlock()

		// Kill the process — this will trigger monitorExit → handleExit
		// which checks containerTimeout and reports the fatal error
		if cmd != nil && cmd.Process != nil {
			cmd.Process.Kill()
		}

	case <-ready:
		pm.log.Debug("container startup watchdog exiting - session started successfully")

	case <-pm.ctx.Done():
		pm.log.Debug("container startup watchdog exiting - context cancelled")
	}
}

// friendlyContainerError translates known container stderr patterns into
// user-friendly error messages. Returns the original message if no pattern matches.
func friendlyContainerError(stderr string, containerized bool) string {
	if !containerized {
		return stderr
	}

	if strings.Contains(stderr, "MCP tool") && strings.Contains(stderr, "not found") {
		return "The container image is outdated and missing required features. " +
			"Please update the container image (rebuild or pull the latest version)."
	}

	if strings.Contains(stderr, "container name") && strings.Contains(stderr, "already in use") {
		return "A stale container could not be cleaned up automatically. " +
			"Run 'plural clean' to remove orphaned containers."
	}

	return stderr
}

// Ensure ProcessManager implements ProcessManagerInterface at compile time.
var _ ProcessManagerInterface = (*ProcessManager)(nil)
