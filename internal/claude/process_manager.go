package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

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
	Containerized     bool   // When true, wraps Claude CLI in a container
	ContainerImage    string // Container image name (e.g., "plural-claude")
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
//	}
type ProcessCallbacks struct {
	// OnLine is called for each line read from stdout.
	// The line includes the trailing newline.
	// This callback is called synchronously from the output reader goroutine.
	OnLine func(line string)

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

	// Context for process goroutines
	ctx    context.Context
	cancel context.CancelFunc

	// Goroutine lifecycle management
	wg sync.WaitGroup
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
			"--include-partial-messages",
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
			"--include-partial-messages",
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
			"--include-partial-messages",
			"--verbose",
			"--session-id", config.SessionID,
		}
	}

	if config.Containerized {
		// Container IS the sandbox — skip MCP permission system entirely
		args = append(args,
			"--dangerously-skip-permissions",
			"--append-system-prompt", OptionsSystemPrompt,
		)
	} else {
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

	// Build command arguments
	args := BuildCommandArgs(pm.config)

	// Log fork operation if applicable
	if pm.config.ForkFromSessionID != "" {
		pm.log.Debug("forking session from parent", "parentSessionID", pm.config.ForkFromSessionID)
	}

	var cmd *exec.Cmd
	if pm.config.Containerized {
		containerArgs := buildContainerRunArgs(pm.config, args)
		pm.log.Debug("starting containerized process", "command", "container "+strings.Join(containerArgs, " "))
		cmd = exec.Command("container", containerArgs...)
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
		return fmt.Errorf("failed to start process: %v", err)
	}

	pm.cmd = cmd
	pm.stdin = stdin
	pm.stdout = bufio.NewReader(stdout)
	pm.stderr = stderr
	pm.stderrContent = ""
	pm.stderrDone = make(chan struct{})
	pm.running = true

	// Create context for process goroutines
	pm.ctx, pm.cancel = context.WithCancel(context.Background())

	pm.log.Info("process started", "elapsed", time.Since(startTime), "pid", cmd.Process.Pid)

	// Start goroutines to read output, drain stderr, and monitor process
	// Track them with WaitGroup for proper cleanup on Stop()
	pm.wg.Add(3)
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
			pm.log.Debug("process exited gracefully")
		case <-time.After(2 * time.Second):
			pm.log.Debug("force killing process")
			cmd.Process.Kill()
		}
	}

	// Defense-in-depth: force remove the container if we were running in container mode
	if pm.config.Containerized {
		containerName := "plural-" + pm.config.SessionID
		pm.log.Debug("removing container", "name", containerName)
		rmCmd := exec.Command("container", "rm", "-f", containerName)
		if err := rmCmd.Run(); err != nil {
			pm.log.Debug("container rm failed (may already be removed)", "error", err)
		}

		// Clean up the auth secrets file from the host
		authFile := fmt.Sprintf("/tmp/plural-auth-%s", pm.config.SessionID)
		os.Remove(authFile)
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
	defer pm.mu.Unlock()
	pm.config.SessionStarted = true
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
		pm.log.Debug("process exited", "error", err)
		pm.handleExit(err)
	case <-pm.ctx.Done():
		pm.log.Debug("process monitor exiting - context cancelled")
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

// buildContainerRunArgs constructs the arguments for `container run` that wraps
// the Claude CLI process inside an Apple container.
func buildContainerRunArgs(config ProcessConfig, claudeArgs []string) []string {
	homeDir, _ := os.UserHomeDir()

	containerName := "plural-" + config.SessionID
	image := config.ContainerImage
	if image == "" {
		image = "plural-claude"
	}

	args := []string{
		"run", "-i", "--rm",
		"--name", containerName,
		"-v", config.WorkingDir + ":/workspace",
		"-v", homeDir + "/.claude:/home/claude/.claude-host:ro",
		"-w", "/workspace",
	}

	// Mount auth credentials file into the container.
	// On macOS, Claude Code stores auth in the system keychain which isn't
	// accessible inside a Linux container. We write the key to a temp file
	// (0600 permissions) and mount it read-only, rather than passing via -e
	// which would expose the key in `ps` output.
	if authFile := writeContainerAuthFile(config.SessionID); authFile != "" {
		args = append(args, "-v", authFile+":/home/claude/.auth:ro")
	}

	args = append(args, image)
	args = append(args, claudeArgs...)
	return args
}

// writeContainerAuthFile writes credentials to a temporary file with
// restricted permissions (0600) and returns the file path. The entrypoint
// script reads this file and exports the appropriate env var.
//
// File format: ENV_VAR_NAME=value
//
// Credential priority:
//  1. ANTHROPIC_API_KEY from environment (explicit user override, API billing)
//  2. OAuth access token from "Claude Code-credentials" keychain (subscription billing)
//  3. API key from "anthropic_api_key" keychain entry (API billing)
//
// Returns empty string if no credentials are available.
func writeContainerAuthFile(sessionID string) string {
	var content string

	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		content = "ANTHROPIC_API_KEY=" + apiKey
	} else if oauthToken := readOAuthAccessToken(); oauthToken != "" {
		content = "CLAUDE_CODE_OAUTH_TOKEN=" + oauthToken
	} else if apiKey := readKeychainPassword("anthropic_api_key"); apiKey != "" {
		content = "ANTHROPIC_API_KEY=" + apiKey
	}

	if content == "" {
		return ""
	}

	path := fmt.Sprintf("/tmp/plural-auth-%s", sessionID)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return ""
	}
	return path
}

// readOAuthAccessToken extracts the OAuth access token from the
// "Claude Code-credentials" macOS keychain entry. This is used for
// subscription billing (Claude Pro/Team/Enterprise).
func readOAuthAccessToken() string {
	credsJSON := readKeychainPassword("Claude Code-credentials")
	if credsJSON == "" {
		return ""
	}

	var creds struct {
		ClaudeAiOauth struct {
			AccessToken string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal([]byte(credsJSON), &creds); err != nil {
		return ""
	}
	return creds.ClaudeAiOauth.AccessToken
}

// readKeychainPassword reads a password from the macOS keychain.
// Returns empty string if not found or on error.
func readKeychainPassword(service string) string {
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
	pm.running = false
}

// Ensure ProcessManager implements ProcessManagerInterface at compile time.
var _ ProcessManagerInterface = (*ProcessManager)(nil)
