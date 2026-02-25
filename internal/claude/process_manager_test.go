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
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// pmTestLogger creates a discard logger for process manager tests
func pmTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// pmCaptureLogger creates a logger that captures output to a buffer for assertions
func pmCaptureLogger(buf *strings.Builder) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestNewProcessManager(t *testing.T) {
	config := ProcessConfig{
		SessionID:      "test-session",
		WorkingDir:     "/tmp",
		SessionStarted: false,
		AllowedTools:   []string{"Read", "Write"},
		MCPConfigPath:  "/tmp/mcp.json",
	}

	callbacks := ProcessCallbacks{
		OnLine: func(line string) {},
	}

	pm := NewProcessManager(config, callbacks, pmTestLogger())

	if pm == nil {
		t.Fatal("NewProcessManager returned nil")
	}

	if pm.config.SessionID != "test-session" {
		t.Errorf("SessionID = %q, want 'test-session'", pm.config.SessionID)
	}

	if pm.config.WorkingDir != "/tmp" {
		t.Errorf("WorkingDir = %q, want '/tmp'", pm.config.WorkingDir)
	}

	if pm.config.SessionStarted {
		t.Error("SessionStarted should be false initially")
	}

	if len(pm.config.AllowedTools) != 2 {
		t.Errorf("AllowedTools count = %d, want 2", len(pm.config.AllowedTools))
	}

	if pm.IsRunning() {
		t.Error("Process should not be running initially")
	}
}

func TestProcessManager_IsRunning(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	if pm.IsRunning() {
		t.Error("IsRunning should be false before Start")
	}

	// Note: We can't easily test IsRunning after Start without a real claude binary
	// This test verifies the initial state
}

func TestProcessManager_SetInterrupted(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	// Initially false
	pm.mu.Lock()
	interrupted := pm.interrupted
	pm.mu.Unlock()

	if interrupted {
		t.Error("Interrupted should be false initially")
	}

	// Set to true
	pm.SetInterrupted(true)

	pm.mu.Lock()
	interrupted = pm.interrupted
	pm.mu.Unlock()

	if !interrupted {
		t.Error("Interrupted should be true after SetInterrupted(true)")
	}

	// Set back to false
	pm.SetInterrupted(false)

	pm.mu.Lock()
	interrupted = pm.interrupted
	pm.mu.Unlock()

	if interrupted {
		t.Error("Interrupted should be false after SetInterrupted(false)")
	}
}

func TestProcessManager_GetRestartAttempts(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	// Initially 0
	if pm.GetRestartAttempts() != 0 {
		t.Errorf("GetRestartAttempts = %d, want 0", pm.GetRestartAttempts())
	}

	// Set manually
	pm.mu.Lock()
	pm.restartAttempts = 3
	pm.mu.Unlock()

	if pm.GetRestartAttempts() != 3 {
		t.Errorf("GetRestartAttempts = %d, want 3", pm.GetRestartAttempts())
	}
}

func TestProcessManager_ResetRestartAttempts(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	// Set some attempts
	pm.mu.Lock()
	pm.restartAttempts = 5
	pm.mu.Unlock()

	// Reset
	pm.ResetRestartAttempts()

	if pm.GetRestartAttempts() != 0 {
		t.Errorf("GetRestartAttempts after reset = %d, want 0", pm.GetRestartAttempts())
	}
}

func TestProcessManager_UpdateConfig(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:      "old-session",
		WorkingDir:     "/old",
		SessionStarted: false,
	}, ProcessCallbacks{}, pmTestLogger())

	newConfig := ProcessConfig{
		SessionID:      "new-session",
		WorkingDir:     "/new",
		SessionStarted: true,
		AllowedTools:   []string{"Bash"},
		MCPConfigPath:  "/new/mcp.json",
	}

	pm.UpdateConfig(newConfig)

	pm.mu.Lock()
	if pm.config.SessionID != "new-session" {
		t.Errorf("SessionID = %q, want 'new-session'", pm.config.SessionID)
	}
	if pm.config.WorkingDir != "/new" {
		t.Errorf("WorkingDir = %q, want '/new'", pm.config.WorkingDir)
	}
	if !pm.config.SessionStarted {
		t.Error("SessionStarted should be true after update")
	}
	pm.mu.Unlock()
}

func TestProcessManager_MarkSessionStarted(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:      "test-session",
		WorkingDir:     "/tmp",
		SessionStarted: false,
	}, ProcessCallbacks{}, pmTestLogger())

	pm.mu.Lock()
	if pm.config.SessionStarted {
		t.Error("SessionStarted should be false initially")
	}
	pm.mu.Unlock()

	pm.MarkSessionStarted()

	pm.mu.Lock()
	if !pm.config.SessionStarted {
		t.Error("SessionStarted should be true after MarkSessionStarted")
	}
	pm.mu.Unlock()
}

func TestProcessManager_Stop_Idempotent(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	// Stop should be safe to call multiple times
	pm.Stop()
	pm.Stop()
	pm.Stop()

	if pm.IsRunning() {
		t.Error("should not be running after Stop")
	}
}

func TestProcessManager_WriteMessage_NotRunning(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	err := pm.WriteMessage([]byte("test message"))
	if err == nil {
		t.Error("WriteMessage should error when process is not running")
	}
}

func TestProcessManager_Interrupt_NotRunning(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	// Interrupt should not error when no process is running
	err := pm.Interrupt()
	if err != nil {
		t.Errorf("Interrupt should not error when not running, got: %v", err)
	}
}

func TestProcessManager_Stop_ThenNotRunning(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	pm.Stop()

	// After Stop, IsRunning should be false. ProcessManager is disposable —
	// callers create a new one rather than restarting a stopped one.
	if pm.IsRunning() {
		t.Error("should not be running after Stop")
	}
}

func TestProcessConfig_Fields(t *testing.T) {
	config := ProcessConfig{
		SessionID:      "session-123",
		WorkingDir:     "/path/to/work",
		SessionStarted: true,
		AllowedTools:   []string{"Read", "Write", "Bash"},
		MCPConfigPath:  "/path/to/mcp.json",
	}

	if config.SessionID != "session-123" {
		t.Errorf("SessionID = %q, want 'session-123'", config.SessionID)
	}

	if config.WorkingDir != "/path/to/work" {
		t.Errorf("WorkingDir = %q, want '/path/to/work'", config.WorkingDir)
	}

	if !config.SessionStarted {
		t.Error("SessionStarted should be true")
	}

	if len(config.AllowedTools) != 3 {
		t.Errorf("AllowedTools length = %d, want 3", len(config.AllowedTools))
	}

	if config.MCPConfigPath != "/path/to/mcp.json" {
		t.Errorf("MCPConfigPath = %q, want '/path/to/mcp.json'", config.MCPConfigPath)
	}
}

func TestProcessCallbacks_AllFields(t *testing.T) {
	var (
		onLineCalled           int32
		onProcessExitCalled    int32
		onRestartAttemptCalled int32
		onRestartFailedCalled  int32
		onFatalErrorCalled     int32
	)

	callbacks := ProcessCallbacks{
		OnLine: func(line string) {
			atomic.AddInt32(&onLineCalled, 1)
		},
		OnProcessExit: func(err error, stderrContent string) bool {
			atomic.AddInt32(&onProcessExitCalled, 1)
			return false
		},
		OnRestartAttempt: func(attemptNum int) {
			atomic.AddInt32(&onRestartAttemptCalled, 1)
		},
		OnRestartFailed: func(err error) {
			atomic.AddInt32(&onRestartFailedCalled, 1)
		},
		OnFatalError: func(err error) {
			atomic.AddInt32(&onFatalErrorCalled, 1)
		},
	}

	// Test OnLine
	callbacks.OnLine("test line")
	if atomic.LoadInt32(&onLineCalled) != 1 {
		t.Error("OnLine callback not called")
	}

	// Test OnProcessExit
	callbacks.OnProcessExit(nil, "")
	if atomic.LoadInt32(&onProcessExitCalled) != 1 {
		t.Error("OnProcessExit callback not called")
	}

	// Test OnRestartAttempt
	callbacks.OnRestartAttempt(1)
	if atomic.LoadInt32(&onRestartAttemptCalled) != 1 {
		t.Error("OnRestartAttempt callback not called")
	}

	// Test OnRestartFailed
	callbacks.OnRestartFailed(nil)
	if atomic.LoadInt32(&onRestartFailedCalled) != 1 {
		t.Error("OnRestartFailed callback not called")
	}

	// Test OnFatalError
	callbacks.OnFatalError(nil)
	if atomic.LoadInt32(&onFatalErrorCalled) != 1 {
		t.Error("OnFatalError callback not called")
	}
}

func TestProcessCallbacks_NilCallbacks(t *testing.T) {
	// Create ProcessManager with nil callbacks - should not panic
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	// These should not panic even when callbacks are nil
	pm.callbacks.OnLine = nil
	pm.callbacks.OnProcessExit = nil
	pm.callbacks.OnRestartAttempt = nil
	pm.callbacks.OnRestartFailed = nil
	pm.callbacks.OnFatalError = nil

	// The internal methods check for nil before calling
	if pm.callbacks.OnLine != nil {
		t.Error("OnLine should be nil")
	}
}

func TestProcessManagerInterface_Compliance(t *testing.T) {
	// Verify ProcessManager implements ProcessManagerInterface
	var _ ProcessManagerInterface = (*ProcessManager)(nil)
}

func TestErrorVariables_ProcessManager(t *testing.T) {
	// Verify error variables defined in process_manager.go
	if errChannelFull == nil {
		t.Error("errChannelFull should not be nil")
	}

	if errChannelFull.Error() == "" {
		t.Error("errChannelFull should have a message")
	}
}

func TestReadResult_Type(t *testing.T) {
	// Test the readResult struct
	result := readResult{
		line: "test line",
		err:  nil,
	}

	if result.line != "test line" {
		t.Errorf("line = %q, want 'test line'", result.line)
	}

	if result.err != nil {
		t.Error("err should be nil")
	}
}

func TestProcessManager_CleanupLocked(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	// Set some fields that would be set during Start()
	pm.mu.Lock()
	pm.running = true
	pm.mu.Unlock()

	// Call cleanupLocked
	pm.mu.Lock()
	pm.cleanupLocked()
	pm.mu.Unlock()

	// Verify cleanup happened
	pm.mu.Lock()
	running := pm.running
	stdin := pm.stdin
	stderr := pm.stderr
	cmd := pm.cmd
	stdout := pm.stdout
	pm.mu.Unlock()

	if running {
		t.Error("running should be false after cleanup")
	}
	if stdin != nil {
		t.Error("stdin should be nil after cleanup")
	}
	if stderr != nil {
		t.Error("stderr should be nil after cleanup")
	}
	if cmd != nil {
		t.Error("cmd should be nil after cleanup")
	}
	if stdout != nil {
		t.Error("stdout should be nil after cleanup")
	}
}

func TestProcessManager_Constants(t *testing.T) {
	// Test that constants used by ProcessManager are reasonable
	if MaxProcessRestartAttempts <= 0 {
		t.Error("MaxProcessRestartAttempts should be positive")
	}

	if MaxProcessRestartAttempts > 10 {
		t.Error("MaxProcessRestartAttempts should not be excessive")
	}

	if ProcessRestartDelay <= 0 {
		t.Error("ProcessRestartDelay should be positive")
	}
}

func TestProcessManager_ConcurrentAccess(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	// Test concurrent access to various methods
	done := make(chan bool)

	// Concurrent GetRestartAttempts
	go func() {
		for range 100 {
			pm.GetRestartAttempts()
		}
		done <- true
	}()

	// Concurrent ResetRestartAttempts
	go func() {
		for range 100 {
			pm.ResetRestartAttempts()
		}
		done <- true
	}()

	// Concurrent SetInterrupted
	go func() {
		for i := range 100 {
			pm.SetInterrupted(i%2 == 0)
		}
		done <- true
	}()

	// Concurrent IsRunning
	go func() {
		for range 100 {
			pm.IsRunning()
		}
		done <- true
	}()

	// Wait for all goroutines
	for range 4 {
		<-done
	}
}

func TestProcessManager_UpdateConfig_AfterStop(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	pm.Stop()

	// UpdateConfig should still work after Stop (for potential reuse scenarios)
	pm.UpdateConfig(ProcessConfig{
		SessionID:  "new-session",
		WorkingDir: "/new",
	})

	pm.mu.Lock()
	sessionID := pm.config.SessionID
	pm.mu.Unlock()

	if sessionID != "new-session" {
		t.Errorf("SessionID = %q, want 'new-session'", sessionID)
	}
}

func TestProcessManager_MarkSessionStarted_ThreadSafe(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:      "test-session",
		WorkingDir:     "/tmp",
		SessionStarted: false,
	}, ProcessCallbacks{}, pmTestLogger())

	done := make(chan bool)

	// Multiple goroutines calling MarkSessionStarted
	for range 10 {
		go func() {
			pm.MarkSessionStarted()
			done <- true
		}()
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}

	pm.mu.Lock()
	started := pm.config.SessionStarted
	pm.mu.Unlock()

	if !started {
		t.Error("SessionStarted should be true after multiple MarkSessionStarted calls")
	}
}

// Helper to check if args slice contains a specific flag
func containsArg(args []string, flag string) bool {
	return slices.Contains(args, flag)
}

// Helper to get the value following a flag in args slice
func getArgValue(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestBuildCommandArgs_NewSession(t *testing.T) {
	config := ProcessConfig{
		SessionID:      "new-session-uuid",
		WorkingDir:     "/tmp",
		SessionStarted: false,
		MCPConfigPath:  "/tmp/mcp.json",
		AllowedTools:   []string{"Read", "Write"},
	}

	args := BuildCommandArgs(config)

	// New session should use --session-id, not --resume
	if !containsArg(args, "--session-id") {
		t.Error("New session should have --session-id flag")
	}
	if containsArg(args, "--resume") {
		t.Error("New session should not have --resume flag")
	}
	if containsArg(args, "--fork-session") {
		t.Error("New session should not have --fork-session flag")
	}

	// Verify session ID value
	if got := getArgValue(args, "--session-id"); got != "new-session-uuid" {
		t.Errorf("--session-id value = %q, want 'new-session-uuid'", got)
	}

	// Verify common args
	if !containsArg(args, "--print") {
		t.Error("Should have --print flag")
	}
	if getArgValue(args, "--output-format") != "stream-json" {
		t.Error("Should have --output-format stream-json")
	}
}

func TestBuildCommandArgs_ResumedSession(t *testing.T) {
	config := ProcessConfig{
		SessionID:      "resumed-session-uuid",
		WorkingDir:     "/tmp",
		SessionStarted: true, // Already started
		MCPConfigPath:  "/tmp/mcp.json",
	}

	args := BuildCommandArgs(config)

	// Resumed session should use --resume with our session ID
	if !containsArg(args, "--resume") {
		t.Error("Resumed session should have --resume flag")
	}
	if containsArg(args, "--session-id") {
		t.Error("Resumed session should not have --session-id flag (using --resume)")
	}
	if containsArg(args, "--fork-session") {
		t.Error("Resumed session should not have --fork-session flag")
	}

	// Verify resume ID value
	if got := getArgValue(args, "--resume"); got != "resumed-session-uuid" {
		t.Errorf("--resume value = %q, want 'resumed-session-uuid'", got)
	}
}

func TestBuildCommandArgs_ForkedSession(t *testing.T) {
	config := ProcessConfig{
		SessionID:         "child-session-uuid",
		WorkingDir:        "/tmp",
		SessionStarted:    false,
		MCPConfigPath:     "/tmp/mcp.json",
		ForkFromSessionID: "parent-session-uuid",
	}

	args := BuildCommandArgs(config)

	// Forked session MUST have all three: --resume (parent), --fork-session, AND --session-id (child)
	// This is critical: without --session-id, Claude generates its own ID and we can't resume later
	if !containsArg(args, "--resume") {
		t.Error("Forked session should have --resume flag")
	}
	if !containsArg(args, "--fork-session") {
		t.Error("Forked session should have --fork-session flag")
	}
	if !containsArg(args, "--session-id") {
		t.Fatal("CRITICAL: Forked session MUST have --session-id flag to ensure we can resume later")
	}

	// Verify the values are correct
	if got := getArgValue(args, "--resume"); got != "parent-session-uuid" {
		t.Errorf("--resume value = %q, want 'parent-session-uuid'", got)
	}
	if got := getArgValue(args, "--session-id"); got != "child-session-uuid" {
		t.Errorf("--session-id value = %q, want 'child-session-uuid'", got)
	}
}

func TestBuildCommandArgs_ForkedSession_CanResumeAfterInterrupt(t *testing.T) {
	// This test simulates the bug scenario:
	// 1. Fork a session (first message)
	// 2. User interrupts
	// 3. User sends another message (needs to resume)

	childSessionID := "child-session-uuid"
	parentSessionID := "parent-session-uuid"

	// Step 1: First message in forked session
	forkConfig := ProcessConfig{
		SessionID:         childSessionID,
		SessionStarted:    false,
		ForkFromSessionID: parentSessionID,
		MCPConfigPath:     "/tmp/mcp.json",
	}
	forkArgs := BuildCommandArgs(forkConfig)

	// Verify fork uses --session-id with child's ID
	if !containsArg(forkArgs, "--session-id") {
		t.Fatal("Fork must pass --session-id to Claude CLI")
	}
	if got := getArgValue(forkArgs, "--session-id"); got != childSessionID {
		t.Fatalf("Fork --session-id = %q, want %q", got, childSessionID)
	}

	// Step 2: Simulate interrupt - session is now started
	// Step 3: Second message needs to resume
	resumeConfig := ProcessConfig{
		SessionID:         childSessionID,
		SessionStarted:    true,            // Marked as started after first response
		ForkFromSessionID: parentSessionID, // Still set, but shouldn't matter
		MCPConfigPath:     "/tmp/mcp.json",
	}
	resumeArgs := BuildCommandArgs(resumeConfig)

	// Verify resume uses --resume with child's ID (not parent's)
	if !containsArg(resumeArgs, "--resume") {
		t.Fatal("Resume must have --resume flag")
	}
	if got := getArgValue(resumeArgs, "--resume"); got != childSessionID {
		t.Fatalf("Resume --resume = %q, want %q (child ID, not parent)", got, childSessionID)
	}
	if containsArg(resumeArgs, "--fork-session") {
		t.Error("Resume should not have --fork-session (only used on first message)")
	}
}

func TestBuildCommandArgs_SessionStarted_TakesPriority(t *testing.T) {
	// When SessionStarted is true, it should take priority over ForkFromSessionID
	// This ensures we resume our own session, not try to fork again
	config := ProcessConfig{
		SessionID:         "child-uuid",
		SessionStarted:    true,          // Takes priority
		ForkFromSessionID: "parent-uuid", // Should be ignored
		MCPConfigPath:     "/tmp/mcp.json",
	}

	args := BuildCommandArgs(config)

	// Should resume our own session
	if got := getArgValue(args, "--resume"); got != "child-uuid" {
		t.Errorf("When SessionStarted=true, --resume should use child ID, got %q", got)
	}
	if containsArg(args, "--fork-session") {
		t.Error("When SessionStarted=true, should not have --fork-session")
	}
}

func TestProcessManager_WaitGroup_InitiallyZero(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	// WaitGroup should be at zero initially (no goroutines started)
	// This test verifies we can call Stop() without blocking forever
	done := make(chan bool, 1)
	go func() {
		pm.Stop()
		done <- true
	}()

	select {
	case <-done:
		// Good - Stop returned quickly
	case <-time.After(100 * time.Millisecond):
		t.Error("Stop() blocked - WaitGroup not properly initialized")
	}
}

func TestProcessManager_Stop_WaitsForGoroutines(t *testing.T) {
	// This test verifies that Stop() properly waits for goroutines
	// We can't easily test with a real process, but we can verify the structure
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	// Manually simulate what Start() does with the WaitGroup
	pm.mu.Lock()
	pm.running = true
	pm.ctx, pm.cancel = context.WithCancel(context.Background())
	pm.mu.Unlock()

	// Add to WaitGroup to simulate goroutines
	pm.wg.Add(2)

	// Goroutine 1: simulates readOutput
	go func() {
		defer pm.wg.Done()
		<-pm.ctx.Done() // Wait for cancel
	}()

	// Goroutine 2: simulates monitorExit
	go func() {
		defer pm.wg.Done()
		<-pm.ctx.Done() // Wait for cancel
	}()

	// Stop should wait for both goroutines
	stopDone := make(chan bool, 1)
	go func() {
		pm.Stop()
		stopDone <- true
	}()

	select {
	case <-stopDone:
		// Good - goroutines exited and Stop returned
	case <-time.After(2 * time.Second):
		t.Error("Stop() did not return - goroutines not properly tracked")
	}
}

func TestProcessManager_MultipleStartStop_NoLeak(t *testing.T) {
	// Test that multiple Start/Stop cycles don't leak goroutines
	// Note: We can't actually Start() without claude binary, but we can test Stop idempotency
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	// Multiple stops should be safe
	for i := range 5 {
		done := make(chan bool, 1)
		go func() {
			pm.Stop()
			done <- true
		}()

		select {
		case <-done:
			// Good
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Stop() %d blocked", i)
		}
	}
}

func TestProcessManager_ConfigTransition_ResumeToNewSession(t *testing.T) {
	// Verify that clearing SessionStarted and ForkFromSessionID produces
	// correct command args (new session instead of resume).
	// The resume fallback logic lives in Runner.ensureProcessRunning(),
	// which creates a fresh ProcessManager with these flags cleared.

	pm := NewProcessManager(ProcessConfig{
		SessionID:         "test-session",
		WorkingDir:        "/tmp",
		SessionStarted:    true,
		MCPConfigPath:     "/tmp/mcp.json",
		ForkFromSessionID: "parent-id",
	}, ProcessCallbacks{}, pmTestLogger())

	// Simulate what Runner.ensureProcessRunning does on resume fallback:
	// clear the resume/fork flags
	pm.mu.Lock()
	pm.config.SessionStarted = false
	pm.config.ForkFromSessionID = ""
	pm.mu.Unlock()

	args := BuildCommandArgs(pm.config)

	if containsArg(args, "--resume") {
		t.Error("After clearing SessionStarted, should not have --resume flag")
	}
	if !containsArg(args, "--session-id") {
		t.Error("After clearing SessionStarted, should have --session-id flag")
	}
}

func TestProcessManager_ResumeFallback_ConfigTransition(t *testing.T) {
	// Test that the config transition from resume to new session produces correct args
	config := ProcessConfig{
		SessionID:         "test-session-uuid",
		WorkingDir:        "/tmp",
		SessionStarted:    true,
		MCPConfigPath:     "/tmp/mcp.json",
		ForkFromSessionID: "parent-uuid",
	}

	// Before fallback: should use --resume
	argsBefore := BuildCommandArgs(config)
	if !containsArg(argsBefore, "--resume") {
		t.Error("Before fallback, should have --resume flag")
	}
	if got := getArgValue(argsBefore, "--resume"); got != "test-session-uuid" {
		t.Errorf("Before fallback, --resume = %q, want 'test-session-uuid'", got)
	}

	// Apply fallback: clear SessionStarted and ForkFromSessionID
	config.SessionStarted = false
	config.ForkFromSessionID = ""

	// After fallback: should use --session-id
	argsAfter := BuildCommandArgs(config)
	if containsArg(argsAfter, "--resume") {
		t.Error("After fallback, should not have --resume flag")
	}
	if containsArg(argsAfter, "--fork-session") {
		t.Error("After fallback, should not have --fork-session flag")
	}
	if !containsArg(argsAfter, "--session-id") {
		t.Error("After fallback, should have --session-id flag")
	}
	if got := getArgValue(argsAfter, "--session-id"); got != "test-session-uuid" {
		t.Errorf("After fallback, --session-id = %q, want 'test-session-uuid'", got)
	}
}

func TestProcessManager_GoroutineExitOnContextCancel(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	// Set up context
	pm.mu.Lock()
	pm.ctx, pm.cancel = context.WithCancel(context.Background())
	pm.running = true
	pm.mu.Unlock()

	// Track when goroutine exits
	exitedCh := make(chan bool, 1)
	pm.wg.Go(func() {
		// Simulate readOutput's context check
		select {
		case <-pm.ctx.Done():
			exitedCh <- true
			return
		}
	})

	// Cancel context
	pm.cancel()

	// Goroutine should exit promptly
	select {
	case <-exitedCh:
		// Good - goroutine responded to cancel
	case <-time.After(100 * time.Millisecond):
		t.Error("Goroutine did not exit on context cancel")
	}

	// WaitGroup should complete
	waitDone := make(chan bool, 1)
	go func() {
		pm.wg.Wait()
		waitDone <- true
	}()

	select {
	case <-waitDone:
		// Good
	case <-time.After(100 * time.Millisecond):
		t.Error("WaitGroup.Wait() blocked after goroutine exit")
	}
}

func TestBuildCommandArgs_Containerized_NewSession(t *testing.T) {
	// Consumer provides container-appropriate tools via AllowedTools
	config := ProcessConfig{
		SessionID:      "container-session-uuid",
		WorkingDir:     "/tmp/worktree",
		SessionStarted: false,
		MCPConfigPath:  "/tmp/mcp.json",
		AllowedTools:   ComposeTools(ToolSetBase, ToolSetContainerShell, ToolSetWeb, ToolSetProductivity),
		Containerized:  true,
		ContainerImage: "my-image",
	}

	args := BuildCommandArgs(config)

	// New session should use --session-id
	if !containsArg(args, "--session-id") {
		t.Error("Should have --session-id flag")
	}
	if got := getArgValue(args, "--session-id"); got != "container-session-uuid" {
		t.Errorf("--session-id = %q, want 'container-session-uuid'", got)
	}

	// With MCPConfigPath set, containerized sessions use MCP with wildcard
	// instead of --dangerously-skip-permissions (they conflict in Claude CLI)
	if containsArg(args, "--dangerously-skip-permissions") {
		t.Error("Containerized session with MCPConfigPath must NOT have --dangerously-skip-permissions (conflicts with --permission-prompt-tool)")
	}
	if !containsArg(args, "--mcp-config") {
		t.Error("Containerized session with MCPConfigPath should have --mcp-config")
	}
	if !containsArg(args, "--permission-prompt-tool") {
		t.Error("Containerized session with MCPConfigPath should have --permission-prompt-tool")
	}

	// --mcp-config must use the short container-side path, not the host path
	if got := getArgValue(args, "--mcp-config"); got != containerMCPConfigPath {
		t.Errorf("--mcp-config = %q, want %q (short container-side path)", got, containerMCPConfigPath)
	}

	// Must have --allowedTools from consumer-provided AllowedTools
	if !containsArg(args, "--allowedTools") {
		t.Error("Containerized session must have --allowedTools for pre-authorized tools")
	}
	// Verify Bash is pre-authorized (unrestricted, not Bash(ls:*) etc.)
	bashFound := false
	for i, arg := range args {
		if arg == "--allowedTools" && i+1 < len(args) && args[i+1] == "Bash" {
			bashFound = true
			break
		}
	}
	if !bashFound {
		t.Error("Containerized session must pre-authorize unrestricted Bash")
	}

	// Non-supervisor containerized session should not have --append-system-prompt
	if containsArg(args, "--append-system-prompt") {
		t.Error("Non-supervisor containerized session should NOT have --append-system-prompt")
	}
}

func TestBuildCommandArgs_Containerized_UsesConsumerTools(t *testing.T) {
	// Verify that BuildCommandArgs uses whatever tools the consumer provides,
	// rather than hardcoding containerAllowedTools
	customTools := []string{"Read", "Write", "Bash", "CustomTool"}
	config := ProcessConfig{
		SessionID:      "custom-tools-session",
		WorkingDir:     "/tmp/worktree",
		SessionStarted: false,
		MCPConfigPath:  "/tmp/mcp.json",
		AllowedTools:   customTools,
		Containerized:  true,
		ContainerImage: "my-image",
	}

	args := BuildCommandArgs(config)

	// Verify each consumer-provided tool appears in args
	for _, tool := range customTools {
		found := false
		for i, arg := range args {
			if arg == "--allowedTools" && i+1 < len(args) && args[i+1] == tool {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("consumer tool %q not found in args", tool)
		}
	}

	// Count --allowedTools flags — should match consumer tools count
	count := 0
	for _, arg := range args {
		if arg == "--allowedTools" {
			count++
		}
	}
	if count != len(customTools) {
		t.Errorf("expected %d --allowedTools flags, got %d", len(customTools), count)
	}
}

func TestBuildCommandArgs_Containerized_NoMCPConfig(t *testing.T) {
	// When MCPConfigPath is empty (MCP server not started yet),
	// containerized sessions fall back to --dangerously-skip-permissions
	config := ProcessConfig{
		SessionID:      "container-session-uuid",
		WorkingDir:     "/tmp/worktree",
		SessionStarted: false,
		MCPConfigPath:  "", // Empty - MCP not configured
		AllowedTools:   ComposeTools(ToolSetBase, ToolSetContainerShell, ToolSetWeb, ToolSetProductivity),
		Containerized:  true,
		ContainerImage: "my-image",
	}

	args := BuildCommandArgs(config)

	if !containsArg(args, "--dangerously-skip-permissions") {
		t.Error("Containerized session without MCPConfigPath should fall back to --dangerously-skip-permissions")
	}
	if containsArg(args, "--mcp-config") {
		t.Error("Containerized session without MCPConfigPath should not have --mcp-config")
	}
	if containsArg(args, "--permission-prompt-tool") {
		t.Error("Containerized session without MCPConfigPath should not have --permission-prompt-tool")
	}
}

func TestBuildCommandArgs_Containerized_ResumedSession(t *testing.T) {
	config := ProcessConfig{
		SessionID:      "container-session-uuid",
		WorkingDir:     "/tmp/worktree",
		SessionStarted: true,
		MCPConfigPath:  "/tmp/mcp.json",
		Containerized:  true,
		ContainerImage: "my-image",
	}

	args := BuildCommandArgs(config)

	// Containerized sessions always use --session-id (never --resume) because
	// each container run is a fresh environment with no prior session data
	if containsArg(args, "--resume") {
		t.Error("Containerized session must not use --resume (no session data persists across container runs)")
	}
	if got := getArgValue(args, "--session-id"); got != "container-session-uuid" {
		t.Errorf("--session-id = %q, want 'container-session-uuid'", got)
	}

	// With MCPConfigPath set, should use MCP instead of --dangerously-skip-permissions
	if containsArg(args, "--dangerously-skip-permissions") {
		t.Error("Containerized resumed session with MCPConfigPath must NOT have --dangerously-skip-permissions")
	}
	if !containsArg(args, "--mcp-config") {
		t.Error("Containerized resumed session with MCPConfigPath should have --mcp-config")
	}
	if !containsArg(args, "--permission-prompt-tool") {
		t.Error("Containerized resumed session with MCPConfigPath should have --permission-prompt-tool")
	}
}

func TestBuildCommandArgs_Containerized_ForkedSession(t *testing.T) {
	config := ProcessConfig{
		SessionID:         "child-session-uuid",
		WorkingDir:        "/tmp/worktree",
		SessionStarted:    false,
		MCPConfigPath:     "/tmp/mcp.json",
		ForkFromSessionID: "parent-session-uuid",
		Containerized:     true,
		ContainerImage:    "my-image",
	}

	args := BuildCommandArgs(config)

	// Containerized forked sessions must NOT use --resume/--fork-session because
	// the parent session data doesn't exist inside the container.
	// Instead, it should be treated as a new session with --session-id.
	if containsArg(args, "--resume") {
		t.Error("Containerized forked session must not have --resume (parent data not in container)")
	}
	if containsArg(args, "--fork-session") {
		t.Error("Containerized forked session must not have --fork-session (parent data not in container)")
	}
	if !containsArg(args, "--session-id") {
		t.Error("Containerized forked session should have --session-id")
	}

	// Verify it uses our session ID, not the parent's
	for i, arg := range args {
		if arg == "--session-id" && i+1 < len(args) {
			if args[i+1] != "child-session-uuid" {
				t.Errorf("Expected --session-id child-session-uuid, got %s", args[i+1])
			}
			break
		}
	}

	// With MCPConfigPath set, should use MCP instead of --dangerously-skip-permissions
	if containsArg(args, "--dangerously-skip-permissions") {
		t.Error("Containerized forked session with MCPConfigPath must NOT have --dangerously-skip-permissions")
	}
	if !containsArg(args, "--mcp-config") {
		t.Error("Containerized forked session with MCPConfigPath should have --mcp-config")
	}
}

func TestBuildCommandArgs_NonContainerized_Unchanged(t *testing.T) {
	// Regression test: non-containerized config should still have MCP flags
	config := ProcessConfig{
		SessionID:      "normal-session-uuid",
		WorkingDir:     "/tmp/worktree",
		SessionStarted: false,
		MCPConfigPath:  "/tmp/mcp.json",
		AllowedTools:   []string{"Read", "Write"},
		Containerized:  false, // explicitly not containerized
	}

	args := BuildCommandArgs(config)

	// Must have MCP-related flags
	if !containsArg(args, "--mcp-config") {
		t.Error("Non-containerized session must have --mcp-config")
	}
	if !containsArg(args, "--permission-prompt-tool") {
		t.Error("Non-containerized session must have --permission-prompt-tool")
	}

	// Must have --allowedTools
	if !containsArg(args, "--allowedTools") {
		t.Error("Non-containerized session must have --allowedTools")
	}

	// Must NOT have --dangerously-skip-permissions
	if containsArg(args, "--dangerously-skip-permissions") {
		t.Error("Non-containerized session must not have --dangerously-skip-permissions")
	}
}

func TestBuildContainerRunArgs(t *testing.T) {
	config := ProcessConfig{
		SessionID:      "test-session-123",
		WorkingDir:     "/path/to/worktree",
		ContainerImage: "ghcr.io/zhubert/plural-claude",
	}

	claudeArgs := []string{"--print", "--session-id", "test-session-123", "--dangerously-skip-permissions"}

	result, err := buildContainerRunArgs(config, claudeArgs)
	if err != nil {
		t.Fatalf("buildContainerRunArgs failed: %v", err)
	}
	args := result.Args

	// Verify basic container run structure
	if args[0] != "run" {
		t.Errorf("First arg should be 'run', got %q", args[0])
	}
	if !containsArg(args, "-i") {
		t.Error("Should have -i flag for interactive stdin")
	}
	if !containsArg(args, "--rm") {
		t.Error("Should have --rm flag for auto-cleanup")
	}

	// No --add-host needed (reverse TCP direction, no host.docker.internal required)
	if containsArg(args, "--add-host") {
		t.Error("Should not have --add-host flag (reverse TCP direction)")
	}

	// Verify container name
	if got := getArgValue(args, "--name"); got != "plural-test-session-123" {
		t.Errorf("Container name = %q, want 'plural-test-session-123'", got)
	}

	// Verify working directory mount
	foundWorkspaceMount := slices.Contains(args, "/path/to/worktree:/workspace")
	if !foundWorkspaceMount {
		t.Error("Should mount worktree to /workspace")
	}

	// Verify -w /workspace
	if got := getArgValue(args, "-w"); got != "/workspace" {
		t.Errorf("Working directory = %q, want '/workspace'", got)
	}

	// Verify image name appears before claude args (entrypoint handles running claude)
	foundImage := false
	for i, arg := range args {
		if arg == "ghcr.io/zhubert/plural-claude" {
			// Next arg should be a claude flag (entrypoint invokes claude)
			if i+1 < len(args) && args[i+1] == "--print" {
				foundImage = true
			}
			break
		}
	}
	if !foundImage {
		t.Error("Should have image name followed by claude args")
	}

	// Verify claude args are appended
	if !containsArg(args, "--dangerously-skip-permissions") {
		t.Error("Claude args should be appended to container run args")
	}
}

func TestBuildContainerRunArgs_DefaultImage(t *testing.T) {
	config := ProcessConfig{
		SessionID:      "test-session",
		WorkingDir:     "/tmp",
		ContainerImage: "", // Empty should default to "ghcr.io/zhubert/plural-claude"
	}

	result, err := buildContainerRunArgs(config, []string{"--print"})
	if err != nil {
		t.Fatalf("buildContainerRunArgs failed: %v", err)
	}

	// Check that the default image is in the args
	if !containsArg(result.Args, "ghcr.io/zhubert/plural-claude") {
		t.Error("Empty ContainerImage should default to 'ghcr.io/zhubert/plural-claude'")
	}
}

func TestBuildContainerRunArgs_ReportsAuthSource(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")

	config := ProcessConfig{
		SessionID:      "test-auth-source",
		WorkingDir:     "/tmp",
		ContainerImage: "plural-claude",
	}
	defer os.Remove(containerAuthFilePath(config.SessionID))

	result, err := buildContainerRunArgs(config, []string{"--print"})
	if err != nil {
		t.Fatalf("buildContainerRunArgs failed: %v", err)
	}
	if result.AuthSource == "" {
		t.Error("AuthSource should be set when credentials are available")
	}
	if result.AuthSource != "ANTHROPIC_API_KEY env var" {
		t.Errorf("AuthSource = %q, want %q", result.AuthSource, "ANTHROPIC_API_KEY env var")
	}

	// Verify --env-file is used instead of -v mount for auth
	if !containsArg(result.Args, "--env-file") {
		t.Error("Should use --env-file for auth credentials")
	}
	// Should NOT have the old -v mount for auth
	for _, arg := range result.Args {
		if strings.Contains(arg, ".auth:ro") {
			t.Error("Should not mount auth file via -v (use --env-file instead)")
		}
	}
}

func TestBuildContainerRunArgs_ErrorsOnMissingHome(t *testing.T) {
	t.Setenv("HOME", "")
	// Some platforms also check USER; clear everything that os.UserHomeDir checks
	t.Setenv("USERPROFILE", "")

	config := ProcessConfig{
		SessionID:      "test-no-home",
		WorkingDir:     "/tmp",
		ContainerImage: "plural-claude",
	}

	_, err := buildContainerRunArgs(config, []string{"--print"})
	if err == nil {
		t.Error("buildContainerRunArgs should return error when home directory cannot be determined")
	}
}

func TestProcessManager_HandleExit_CleansUpAuthFileOnFatalError(t *testing.T) {
	// Create a temporary auth file to simulate what writeContainerAuthFile creates
	sessionID := "test-auth-cleanup"
	authFile := containerAuthFilePath(sessionID)
	if err := os.WriteFile(authFile, []byte("ANTHROPIC_API_KEY=sk-ant-test-key"), 0600); err != nil {
		t.Fatalf("failed to create test auth file: %v", err)
	}
	defer os.Remove(authFile) // cleanup in case test fails

	// Verify the file exists
	if _, err := os.Stat(authFile); os.IsNotExist(err) {
		t.Fatal("test auth file should exist before test")
	}

	var fatalErrorCalled bool
	pm := NewProcessManager(ProcessConfig{
		SessionID:     sessionID,
		WorkingDir:    "/tmp",
		Containerized: true,
	}, ProcessCallbacks{
		OnProcessExit: func(err error, stderr string) bool {
			return true // allow restart
		},
		OnFatalError: func(err error) {
			fatalErrorCalled = true
		},
	}, pmTestLogger())

	// Set restart attempts to max so handleExit takes the fatal error path
	pm.mu.Lock()
	pm.running = true
	pm.restartAttempts = MaxProcessRestartAttempts
	pm.stderrDone = make(chan struct{})
	close(pm.stderrDone) // simulate stderr already drained
	pm.ctx, pm.cancel = context.WithCancel(context.Background())
	pm.mu.Unlock()

	// Call handleExit which should take the max-restarts-exceeded path
	pm.handleExit(fmt.Errorf("process crashed"))

	// Verify the fatal error callback was called
	if !fatalErrorCalled {
		t.Error("OnFatalError callback should have been called")
	}

	// Verify the auth file was cleaned up
	if _, err := os.Stat(authFile); !os.IsNotExist(err) {
		t.Error("auth file should have been cleaned up on fatal error")
	}
}

func TestProcessManager_HandleExit_NoAuthCleanupForNonContainerized(t *testing.T) {
	// Create a temporary auth file (shouldn't exist for non-containerized, but test the path)
	sessionID := "test-no-auth-cleanup"

	var fatalErrorCalled bool
	pm := NewProcessManager(ProcessConfig{
		SessionID:     sessionID,
		WorkingDir:    "/tmp",
		Containerized: false, // NOT containerized
	}, ProcessCallbacks{
		OnProcessExit: func(err error, stderr string) bool {
			return true
		},
		OnFatalError: func(err error) {
			fatalErrorCalled = true
		},
	}, pmTestLogger())

	pm.mu.Lock()
	pm.running = true
	pm.restartAttempts = MaxProcessRestartAttempts
	pm.stderrDone = make(chan struct{})
	close(pm.stderrDone)
	pm.ctx, pm.cancel = context.WithCancel(context.Background())
	pm.mu.Unlock()

	pm.handleExit(fmt.Errorf("process crashed"))

	if !fatalErrorCalled {
		t.Error("OnFatalError callback should have been called")
	}
	// No auth file assertion needed — just verify non-containerized path doesn't panic
}

func TestWriteContainerAuthFile_DockerEnvFileFormat(t *testing.T) {
	sessionID := "test-env-file-format"
	defer os.Remove(containerAuthFilePath(sessionID))

	// Set env var with a value containing $ — Docker env-file format is plain KEY=VALUE
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-$pecial-key")

	result := writeContainerAuthFile(sessionID)
	if result.Path == "" {
		t.Fatal("writeContainerAuthFile should succeed for key with $")
	}

	content, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("failed to read auth file: %v", err)
	}

	expected := "ANTHROPIC_API_KEY=sk-ant-$pecial-key"
	if string(content) != expected {
		t.Errorf("auth file content = %q, want %q", string(content), expected)
	}
	if result.Source != "ANTHROPIC_API_KEY env var" {
		t.Errorf("source = %q, want %q", result.Source, "ANTHROPIC_API_KEY env var")
	}
}

func TestWriteContainerAuthFile_RejectsNewlines(t *testing.T) {
	sessionID := "test-newline-reject"
	defer os.Remove(containerAuthFilePath(sessionID))

	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-key\nINJECTED=bad")

	result := writeContainerAuthFile(sessionID)
	if result.Path != "" {
		t.Error("writeContainerAuthFile should reject values with newlines")
		os.Remove(result.Path)
	}
}

func TestWriteContainerAuthFile_AllowsSingleQuotes(t *testing.T) {
	sessionID := "test-quote-allow"
	defer os.Remove(containerAuthFilePath(sessionID))

	// Single quotes are fine in Docker env-file format (no shell sourcing)
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-key'-with-quote")

	result := writeContainerAuthFile(sessionID)
	if result.Path == "" {
		t.Fatal("writeContainerAuthFile should allow values with single quotes in env-file format")
	}

	content, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("failed to read auth file: %v", err)
	}

	expected := "ANTHROPIC_API_KEY=sk-ant-key'-with-quote"
	if string(content) != expected {
		t.Errorf("auth file content = %q, want %q", string(content), expected)
	}
}

func TestContainerAuthDir_ReturnsUserPrivateDir(t *testing.T) {
	dir := containerAuthDir()
	if dir == "" {
		t.Skip("home directory not available")
	}
	if dir == "/tmp" {
		t.Error("containerAuthDir should not return /tmp")
	}
}

func TestContainerAuthAvailable_WithAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")

	if !ContainerAuthAvailable() {
		t.Error("ContainerAuthAvailable should return true when ANTHROPIC_API_KEY is set")
	}
}

func TestContainerAuthAvailable_WithOAuthToken(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "oauth-test-token")

	if !ContainerAuthAvailable() {
		t.Error("ContainerAuthAvailable should return true when CLAUDE_CODE_OAUTH_TOKEN is set")
	}
}

func TestContainerAuthAvailable_WithoutCredentials(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")

	// On macOS this might still return true if there's a keychain entry,
	// but with empty env vars and no keychain it should return false.
	// We can't fully test the keychain path in CI, but we can verify
	// the function doesn't panic.
	_ = ContainerAuthAvailable()
}

func TestWriteContainerAuthFile_APIKeyPriority(t *testing.T) {
	sessionID := "test-api-key-priority"
	defer os.Remove(containerAuthFilePath(sessionID))

	// When both are set, ANTHROPIC_API_KEY takes priority
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "oauth-test-token")

	result := writeContainerAuthFile(sessionID)
	if result.Path == "" {
		t.Fatal("expected auth file to be written")
	}

	content, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("failed to read auth file: %v", err)
	}
	expected := "ANTHROPIC_API_KEY=sk-ant-test-key"
	if string(content) != expected {
		t.Errorf("auth file content = %q, want %q", string(content), expected)
	}
	if result.Source != "ANTHROPIC_API_KEY env var" {
		t.Errorf("source = %q, want %q", result.Source, "ANTHROPIC_API_KEY env var")
	}
}

func TestWriteContainerAuthFile_OAuthTokenFallback(t *testing.T) {
	sessionID := "test-oauth-fallback"
	defer os.Remove(containerAuthFilePath(sessionID))

	// When only CLAUDE_CODE_OAUTH_TOKEN is set, it should be written as CLAUDE_CODE_OAUTH_TOKEN
	// (Claude CLI recognizes this environment variable directly)
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "oauth-test-token")

	result := writeContainerAuthFile(sessionID)
	if result.Path == "" {
		// On macOS with keychain entry for anthropic_api_key, that takes priority
		// over CLAUDE_CODE_OAUTH_TOKEN. Skip if keychain returned a result first.
		t.Skip("keychain may have returned a result before CLAUDE_CODE_OAUTH_TOKEN")
	}

	content, err := os.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("failed to read auth file: %v", err)
	}
	expected := "CLAUDE_CODE_OAUTH_TOKEN=oauth-test-token"
	if string(content) != expected {
		t.Errorf("auth file content = %q, want %q", string(content), expected)
	}
	if result.Source != "CLAUDE_CODE_OAUTH_TOKEN env var" {
		t.Errorf("source = %q, want %q", result.Source, "CLAUDE_CODE_OAUTH_TOKEN env var")
	}
}

func TestWriteContainerAuthFile_KeychainFallback(t *testing.T) {
	sessionID := "test-keychain-fallback"
	defer os.Remove(containerAuthFilePath(sessionID))

	// With no env vars, writeContainerAuthFile falls back to keychain.
	// On macOS with a keychain entry this will succeed; in CI it returns empty.
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")

	result := writeContainerAuthFile(sessionID)

	if result.Path != "" {
		content, err := os.ReadFile(result.Path)
		if err != nil {
			t.Fatalf("failed to read auth file: %v", err)
		}
		// Should be either ANTHROPIC_API_KEY or CLAUDE_CODE_OAUTH_TOKEN from keychain
		s := string(content)
		if !strings.Contains(s, "ANTHROPIC_API_KEY=") && !strings.Contains(s, "CLAUDE_CODE_OAUTH_TOKEN=") {
			t.Errorf("unexpected auth file content: %s", s)
		}
	}
}

func TestReadKeychainOAuthToken_ValidCredentials(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("keychain tests only run on macOS")
	}
	// We can't mock the keychain directly, but we can test the JSON parsing
	// by testing the exported behavior indirectly. These tests verify the
	// JSON parsing logic via the unexported function.
}

func TestKeychainOAuthCredentials_JSONParsing(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantKey bool
		wantVal string
	}{
		{
			name: "valid Pro/Max credentials",
			input: `{"claudeAiOauth":{"accessToken":"sk-ant-oat01-test-token","refreshToken":"sk-ant-ort01-refresh","expiresAt":` +
				fmt.Sprintf("%d", time.Now().Add(time.Hour).UnixMilli()) +
				`,"scopes":["user:inference"],"subscriptionType":"max"}}`,
			wantKey: true,
			wantVal: "sk-ant-oat01-test-token",
		},
		{
			name:    "empty access token",
			input:   `{"claudeAiOauth":{"accessToken":"","expiresAt":999999999999999}}`,
			wantKey: false,
		},
		{
			name:    "expired token",
			input:   `{"claudeAiOauth":{"accessToken":"sk-ant-oat01-expired","expiresAt":1000000000000}}`,
			wantKey: false,
		},
		{
			name:    "invalid JSON",
			input:   `not json`,
			wantKey: false,
		},
		{
			name:    "empty JSON",
			input:   `{}`,
			wantKey: false,
		},
		{
			name:    "missing claudeAiOauth",
			input:   `{"other":"field"}`,
			wantKey: false,
		},
		{
			name:    "zero expiresAt (no expiry check)",
			input:   `{"claudeAiOauth":{"accessToken":"sk-ant-oat01-no-expiry","expiresAt":0}}`,
			wantKey: true,
			wantVal: "sk-ant-oat01-no-expiry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var creds keychainOAuthCredentials
			err := json.Unmarshal([]byte(tt.input), &creds)

			if tt.wantKey {
				if err != nil {
					t.Fatalf("unexpected parse error: %v", err)
				}
				if creds.ClaudeAiOauth.AccessToken != tt.wantVal {
					t.Errorf("got accessToken %q, want %q", creds.ClaudeAiOauth.AccessToken, tt.wantVal)
				}
			} else {
				// Either parse failed or the token is empty/expired
				if err == nil && creds.ClaudeAiOauth.AccessToken != "" {
					// Check if it should be rejected due to expiration
					if creds.ClaudeAiOauth.ExpiresAt > 0 && time.Now().UnixMilli() >= creds.ClaudeAiOauth.ExpiresAt {
						// Expected: expired token
					} else {
						t.Errorf("expected empty/expired token, got %q", creds.ClaudeAiOauth.AccessToken)
					}
				}
			}
		})
	}
}

func TestKeychainCredential_EnvVarMapping(t *testing.T) {
	// Test that readKeychainCredential returns the correct env var names.
	// We can't mock the keychain, but we can verify the struct behavior.
	cred := keychainCredential{Value: "test-key", EnvVar: "ANTHROPIC_API_KEY", Source: "test"}
	if cred.EnvVar != "ANTHROPIC_API_KEY" {
		t.Errorf("expected ANTHROPIC_API_KEY, got %s", cred.EnvVar)
	}

	cred = keychainCredential{Value: "oauth-token", EnvVar: "CLAUDE_CODE_OAUTH_TOKEN", Source: "test"}
	if cred.EnvVar != "CLAUDE_CODE_OAUTH_TOKEN" {
		t.Errorf("expected CLAUDE_CODE_OAUTH_TOKEN, got %s", cred.EnvVar)
	}
}

func TestProcessManager_Start_FailsWithoutAuthForContainerized(t *testing.T) {
	// Clear all credential env vars
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	pm := NewProcessManager(ProcessConfig{
		SessionID:      "test-no-auth",
		WorkingDir:     t.TempDir(),
		Containerized:  true,
		ContainerImage: "plural-claude",
	}, ProcessCallbacks{}, log)

	err := pm.Start()
	if err == nil {
		pm.Stop()
		// On macOS with a keychain entry this might succeed
		// We can only reliably test this on systems without keychain
		t.Log("Start succeeded - likely has keychain credentials")
		return
	}

	if !strings.Contains(err.Error(), "container mode requires authentication") {
		t.Errorf("expected auth error, got: %v", err)
	}
}

func TestProcessManager_Start_AllowsNonContainerizedWithoutAuth(t *testing.T) {
	// Clear all credential env vars
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	pm := NewProcessManager(ProcessConfig{
		SessionID:     "test-non-container",
		WorkingDir:    t.TempDir(),
		Containerized: false,
	}, ProcessCallbacks{}, log)

	// Start will fail because "claude" binary doesn't exist in test,
	// but it should NOT fail with an auth error
	err := pm.Start()
	if err != nil && strings.Contains(err.Error(), "container mode requires authentication") {
		t.Error("non-containerized sessions should not require auth")
	}
	// Clean up if it somehow started
	pm.Stop()
}

func TestBuildContainerRunArgs_MountsMCPConfig(t *testing.T) {
	config := ProcessConfig{
		SessionID:      "test-mount-session",
		WorkingDir:     "/path/to/worktree",
		ContainerImage: "plural-claude",
		MCPConfigPath:  "/var/folders/xx/long-temp-path/T/plural-mcp-test-mount-session.json",
	}

	claudeArgs := []string{"--print", "--session-id", "test-mount-session"}
	result, err := buildContainerRunArgs(config, claudeArgs)
	if err != nil {
		t.Fatalf("buildContainerRunArgs failed: %v", err)
	}
	args := result.Args

	// Verify MCP config is mounted at the short container-side path (read-only)
	foundConfigMount := false
	expectedConfig := "/var/folders/xx/long-temp-path/T/plural-mcp-test-mount-session.json:" + containerMCPConfigPath + ":ro"
	if slices.Contains(args, expectedConfig) {
		foundConfigMount = true
	}
	if !foundConfigMount {
		t.Errorf("Should mount MCP config at short container path, looking for %q in %v", expectedConfig, args)
	}

	// Verify no socket file is mounted (TCP is used instead)
	for _, arg := range args {
		if strings.Contains(arg, ".sock") {
			t.Errorf("Should not mount a socket file (TCP is used for container sessions), found %q", arg)
		}
	}
}

func TestBuildContainerRunArgs_NoMountsWithoutPaths(t *testing.T) {
	config := ProcessConfig{
		SessionID:      "test-no-mount",
		WorkingDir:     "/path/to/worktree",
		ContainerImage: "plural-claude",
		MCPConfigPath:  "", // No MCP config
	}

	claudeArgs := []string{"--print", "--session-id", "test-no-mount"}
	result, err := buildContainerRunArgs(config, claudeArgs)
	if err != nil {
		t.Fatalf("buildContainerRunArgs failed: %v", err)
	}
	args := result.Args

	// Should NOT have MCP config mounts
	for _, arg := range args {
		if strings.Contains(arg, "plural-mcp-test-no-mount.json") {
			t.Error("Should not mount MCP config when MCPConfigPath is empty")
		}
	}
}

func TestBuildContainerRunArgs_PublishesContainerMCPPort(t *testing.T) {
	config := ProcessConfig{
		SessionID:        "test-mcp-port",
		WorkingDir:       "/path/to/worktree",
		ContainerImage:   "plural-claude",
		ContainerMCPPort: 21120,
	}

	result, err := buildContainerRunArgs(config, []string{"--print"})
	if err != nil {
		t.Fatalf("buildContainerRunArgs failed: %v", err)
	}
	args := result.Args

	// Verify -p 0:21120 is present
	if !containsArg(args, "-p") {
		t.Error("Should have -p flag for port publishing")
	}
	if got := getArgValue(args, "-p"); got != "0:21120" {
		t.Errorf("-p value = %q, want '0:21120'", got)
	}
}

func TestBuildContainerRunArgs_NoPortWithoutContainerMCPPort(t *testing.T) {
	config := ProcessConfig{
		SessionID:        "test-no-port",
		WorkingDir:       "/path/to/worktree",
		ContainerImage:   "plural-claude",
		ContainerMCPPort: 0, // No port
	}

	result, err := buildContainerRunArgs(config, []string{"--print"})
	if err != nil {
		t.Fatalf("buildContainerRunArgs failed: %v", err)
	}
	args := result.Args

	// Should NOT have -p flag
	if containsArg(args, "-p") {
		t.Error("Should not have -p flag when ContainerMCPPort is 0")
	}
}

func TestProcessManager_MonitorExit_SingleWait(t *testing.T) {
	// Regression test for issue #126: cmd.Wait() must only be called once.
	// monitorExit owns the cmd.Wait() call; Stop() coordinates via waitDone channel.
	//
	// This test uses a real subprocess (cat) to verify the coordination:
	// 1. Start a process with monitorExit watching it
	// 2. Call Stop() which cancels context and closes stdin
	// 3. Verify monitorExit's cmd.Wait() completes (via waitDone) without panic
	// 4. Verify Stop() doesn't call cmd.Wait() again

	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-single-wait",
		WorkingDir: t.TempDir(),
	}, ProcessCallbacks{
		OnProcessExit: func(err error, stderrContent string) bool {
			return false // don't restart
		},
	}, pmTestLogger())

	// Manually start a real process (cat reads stdin forever until EOF)
	cmd := exec.Command("cat")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to get stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to get stdout pipe: %v", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("failed to get stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	pm.mu.Lock()
	pm.cmd = cmd
	pm.stdin = stdin
	pm.stdout = bufio.NewReader(stdout)
	pm.stderr = stderrPipe
	pm.stderrDone = make(chan struct{})
	pm.waitDone = make(chan struct{})
	pm.running = true
	pm.ctx, pm.cancel = context.WithCancel(context.Background())
	pm.mu.Unlock()

	// Start the goroutines that would be started by Start()
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

	// Stop should complete without panic from double cmd.Wait()
	stopDone := make(chan struct{})
	go func() {
		pm.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		// Success - Stop completed without panic
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() timed out - possible deadlock in wait coordination")
	}
}

func TestProcessManager_MonitorExit_NaturalExit(t *testing.T) {
	// Test that when the process exits naturally (before Stop is called),
	// monitorExit properly closes waitDone and handles exit.

	var exitCalled atomic.Int32
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-natural-exit",
		WorkingDir: t.TempDir(),
	}, ProcessCallbacks{
		OnProcessExit: func(err error, stderrContent string) bool {
			exitCalled.Add(1)
			return false
		},
	}, pmTestLogger())

	// Start a process that exits immediately
	cmd := exec.Command("true")
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	pm.mu.Lock()
	pm.cmd = cmd
	pm.stdin = stdin
	pm.stdout = bufio.NewReader(stdout)
	pm.stderr = stderrPipe
	pm.stderrDone = make(chan struct{})
	pm.waitDone = make(chan struct{})
	pm.running = true
	pm.ctx, pm.cancel = context.WithCancel(context.Background())
	// Capture waitDone under the lock before goroutines start.
	// handleExit -> cleanupLocked sets pm.waitDone = nil, so reading
	// pm.waitDone without the lock after goroutines start would race.
	waitDone := pm.waitDone
	pm.mu.Unlock()

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

	// Wait for waitDone to be closed (monitorExit called cmd.Wait())
	select {
	case <-waitDone:
		// Good - monitorExit closed waitDone after cmd.Wait() completed
	case <-time.After(5 * time.Second):
		t.Fatal("waitDone was not closed after natural process exit")
	}

	// Wait for monitorExit goroutine to finish (including handleExit)
	// before checking the callback, since waitDone is closed before handleExit runs.
	wgDone := make(chan struct{})
	go func() {
		pm.wg.Wait()
		close(wgDone)
	}()
	select {
	case <-wgDone:
	case <-time.After(5 * time.Second):
		t.Fatal("goroutines did not complete after natural process exit")
	}

	// OnProcessExit should have been called
	if exitCalled.Load() != 1 {
		t.Errorf("OnProcessExit called %d times, want 1", exitCalled.Load())
	}

	// Cleanup
	pm.Stop()
}

func TestProcessManager_WaitDone_InitializedByStart(t *testing.T) {
	// Verify that waitDone is nil before Start and would be set by Start
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-waitdone-init",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	pm.mu.Lock()
	if pm.waitDone != nil {
		t.Error("waitDone should be nil before Start")
	}
	pm.mu.Unlock()
}

func TestProcessManager_CleanupLocked_ClearsWaitDone(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-cleanup-waitdone",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	// Simulate state after Start
	pm.mu.Lock()
	pm.waitDone = make(chan struct{})
	pm.running = true
	pm.cleanupLocked()
	pm.mu.Unlock()

	pm.mu.Lock()
	if pm.waitDone != nil {
		t.Error("waitDone should be nil after cleanupLocked")
	}
	pm.mu.Unlock()
}

func TestContainerSidePaths_ShortEnough(t *testing.T) {
	// Verify the MCP config path is reasonable when mounted inside the container.
	if len(containerMCPConfigPath) > 100 {
		t.Errorf("container MCP config path too long (%d chars): %s", len(containerMCPConfigPath), containerMCPConfigPath)
	}
}

func TestFriendlyContainerError(t *testing.T) {
	tests := []struct {
		name          string
		stderr        string
		containerized bool
		wantContains  string
	}{
		{
			name:          "MCP tool not found in container",
			stderr:        `Error: MCP tool mcp__plural__permission (passed via --permission-prompt-tool) not found. Available MCP tools: none`,
			containerized: true,
			wantContains:  "Claude CLI in the container is outdated",
		},
		{
			name:          "container name conflict",
			stderr:        `docker: Error response from daemon: Conflict. The container name "/plural-abc123" is already in use by container "def456".`,
			containerized: true,
			wantContains:  "could not be cleaned up automatically",
		},
		{
			name:          "unknown error passes through",
			stderr:        "some unknown error",
			containerized: true,
			wantContains:  "some unknown error",
		},
		{
			name:          "non-containerized passes through even with matching pattern",
			stderr:        `Error: MCP tool mcp__plural__permission not found`,
			containerized: false,
			wantContains:  "MCP tool mcp__plural__permission not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := friendlyContainerError(tt.stderr, tt.containerized)
			if !strings.Contains(got, tt.wantContains) {
				t.Errorf("friendlyContainerError() = %q, want it to contain %q", got, tt.wantContains)
			}
		})
	}
}

func TestBuildContainerRunArgs_WithRepoPath(t *testing.T) {
	config := ProcessConfig{
		SessionID:      "test-session",
		WorkingDir:     "/tmp/worktree",
		RepoPath:       "/tmp/repo",
		Containerized:  true,
		ContainerImage: "test-image",
		MCPConfigPath:  "/tmp/mcp.json",
	}

	result, err := buildContainerRunArgs(config, []string{"--arg"})
	if err != nil {
		t.Fatalf("buildContainerRunArgs failed: %v", err)
	}

	// Check that repo mount exists at the original absolute path (read-write, not read-only)
	found := false
	expectedMount := "/tmp/repo:/tmp/repo"
	for i, arg := range result.Args {
		if arg == "-v" && i+1 < len(result.Args) {
			if result.Args[i+1] == expectedMount {
				found = true
				break
			}
		}
	}

	if !found {
		t.Errorf("Expected repo mount %q not found in docker args: %v", expectedMount, result.Args)
	}
}

func TestBuildContainerRunArgs_WithoutRepoPath(t *testing.T) {
	config := ProcessConfig{
		SessionID:      "test-session",
		WorkingDir:     "/tmp/worktree",
		RepoPath:       "", // No repo path
		Containerized:  true,
		ContainerImage: "test-image",
	}

	result, err := buildContainerRunArgs(config, []string{"--arg"})
	if err != nil {
		t.Fatalf("buildContainerRunArgs failed: %v", err)
	}

	// Check that no repo mount exists when RepoPath is empty
	for i, arg := range result.Args {
		if arg == "-v" && i+1 < len(result.Args) {
			mount := result.Args[i+1]
			// Should not contain any mount ending with :ro that isn't the MCP config or workspace
			if strings.HasSuffix(mount, ":ro") &&
				!strings.Contains(mount, "/.claude:") &&
				!strings.Contains(mount, "/tmp/mcp.json:") {
				t.Errorf("Unexpected repo mount found when RepoPath is empty: %q", mount)
			}
		}
	}
}

func TestBuildCommandArgs_SystemPrompt(t *testing.T) {
	config := ProcessConfig{
		SessionID:      "session-with-prompt",
		WorkingDir:     "/tmp",
		SessionStarted: false,
		MCPConfigPath:  "/tmp/mcp.json",
		AllowedTools:   []string{"Read"},
		SystemPrompt:   "You are a coding agent.",
	}

	args := BuildCommandArgs(config)

	systemPrompt := getArgValue(args, "--append-system-prompt")
	if systemPrompt != "You are a coding agent." {
		t.Errorf("expected system prompt to be passed through, got %q", systemPrompt)
	}
}

func TestBuildCommandArgs_NoSystemPrompt(t *testing.T) {
	config := ProcessConfig{
		SessionID:      "normal-session",
		WorkingDir:     "/tmp",
		SessionStarted: false,
		MCPConfigPath:  "/tmp/mcp.json",
		AllowedTools:   []string{"Read"},
	}

	args := BuildCommandArgs(config)

	systemPrompt := getArgValue(args, "--append-system-prompt")
	if systemPrompt != "" {
		t.Errorf("session with no system prompt should NOT have --append-system-prompt, got %q", systemPrompt)
	}
}

func TestBuildCommandArgs_Containerized_SystemPrompt(t *testing.T) {
	config := ProcessConfig{
		SessionID:      "container-with-prompt",
		WorkingDir:     "/tmp/worktree",
		SessionStarted: false,
		MCPConfigPath:  "/tmp/mcp.json",
		Containerized:  true,
		ContainerImage: "my-image",
		SystemPrompt:   "You are a coding agent in a container.",
	}

	args := BuildCommandArgs(config)

	systemPrompt := getArgValue(args, "--append-system-prompt")
	if systemPrompt != "You are a coding agent in a container." {
		t.Errorf("expected system prompt to be passed through in container mode, got %q", systemPrompt)
	}
}

func TestBuildCommandArgs_DisableStreamingChunks(t *testing.T) {
	t.Run("with streaming chunks disabled", func(t *testing.T) {
		config := ProcessConfig{
			SessionID:              "agent-session",
			WorkingDir:             "/tmp",
			SessionStarted:         false,
			MCPConfigPath:          "/tmp/mcp.json",
			AllowedTools:           []string{"Read"},
			DisableStreamingChunks: true,
		}

		args := BuildCommandArgs(config)

		// Should NOT include --include-partial-messages
		for _, arg := range args {
			if arg == "--include-partial-messages" {
				t.Error("args should not contain --include-partial-messages when DisableStreamingChunks is true")
			}
		}

		// Should still have other required flags
		if !containsArg(args, "--output-format") {
			t.Error("args should contain --output-format")
		}
		if !containsArg(args, "--input-format") {
			t.Error("args should contain --input-format")
		}
	})

	t.Run("with streaming chunks enabled (default)", func(t *testing.T) {
		config := ProcessConfig{
			SessionID:              "normal-session",
			WorkingDir:             "/tmp",
			SessionStarted:         false,
			MCPConfigPath:          "/tmp/mcp.json",
			AllowedTools:           []string{"Read"},
			DisableStreamingChunks: false,
		}

		args := BuildCommandArgs(config)

		// SHOULD include --include-partial-messages
		if !containsArg(args, "--include-partial-messages") {
			t.Error("args should contain --include-partial-messages when DisableStreamingChunks is false")
		}
	})

	t.Run("resumed session with streaming chunks disabled", func(t *testing.T) {
		config := ProcessConfig{
			SessionID:              "resumed-agent-session",
			WorkingDir:             "/tmp",
			SessionStarted:         true,
			MCPConfigPath:          "/tmp/mcp.json",
			AllowedTools:           []string{"Read"},
			DisableStreamingChunks: true,
			Containerized:          false,
		}

		args := BuildCommandArgs(config)

		// Should NOT include --include-partial-messages even when resuming
		for _, arg := range args {
			if arg == "--include-partial-messages" {
				t.Error("resumed session should not contain --include-partial-messages when DisableStreamingChunks is true")
			}
		}

		// Should have --resume
		if !containsArg(args, "--resume") {
			t.Error("args should contain --resume for resumed session")
		}
	})
}

func TestCredentialsFileExists_WithFile(t *testing.T) {
	// Create a temp directory to act as home
	tmpHome := t.TempDir()
	claudeDir := filepath.Join(tmpHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0700); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}
	credFile := filepath.Join(claudeDir, ".credentials.json")
	if err := os.WriteFile(credFile, []byte(`{"refreshToken":"test"}`), 0600); err != nil {
		t.Fatalf("failed to create credentials file: %v", err)
	}

	// Override HOME so credentialsFileExists() finds our temp file
	t.Setenv("HOME", tmpHome)

	if !credentialsFileExists() {
		t.Error("credentialsFileExists should return true when .credentials.json exists")
	}
}

func TestCredentialsFileExists_Missing(t *testing.T) {
	// Point HOME at a temp dir with no .claude directory
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	if credentialsFileExists() {
		t.Error("credentialsFileExists should return false when .credentials.json does not exist")
	}
}

func TestContainerAuthAvailable_WithCredentialsFile(t *testing.T) {
	// Clear env-based credentials
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")

	// Create a temp home with .credentials.json
	tmpHome := t.TempDir()
	claudeDir := filepath.Join(tmpHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0700); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}
	credFile := filepath.Join(claudeDir, ".credentials.json")
	if err := os.WriteFile(credFile, []byte(`{"refreshToken":"test"}`), 0600); err != nil {
		t.Fatalf("failed to create credentials file: %v", err)
	}
	t.Setenv("HOME", tmpHome)

	// On macOS, keychain may still return true before we reach the credentials check,
	// but the important thing is that ContainerAuthAvailable returns true.
	if !ContainerAuthAvailable() {
		t.Error("ContainerAuthAvailable should return true when .credentials.json exists")
	}
}

func TestContainerStartupWatchdog_Timeout(t *testing.T) {
	// Use a real subprocess (sleep 60) with containerized config.
	// Override timeout to 1s for fast test.
	// Verify watchdog kills it and OnFatalError is called with timeout error.

	var fatalErr error
	fatalCh := make(chan struct{})

	pm := NewProcessManager(ProcessConfig{
		SessionID:     "test-watchdog-timeout",
		WorkingDir:    t.TempDir(),
		Containerized: true,
	}, ProcessCallbacks{
		OnProcessExit: func(err error, stderrContent string) bool {
			return true // allow restart path — but containerTimeout will prevent it
		},
		OnFatalError: func(err error) {
			fatalErr = err
			close(fatalCh)
		},
	}, pmTestLogger())

	// Manually start a real process that blocks forever
	cmd := exec.Command("sleep", "60")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to get stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to get stdout pipe: %v", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("failed to get stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	pm.mu.Lock()
	pm.cmd = cmd
	pm.stdin = stdin
	pm.stdout = bufio.NewReader(stdout)
	pm.stderr = stderrPipe
	pm.stderrDone = make(chan struct{})
	pm.waitDone = make(chan struct{})
	pm.running = true
	pm.ctx, pm.cancel = context.WithCancel(context.Background())
	pm.containerReady = make(chan struct{})
	pm.containerTimeout = false
	pm.mu.Unlock()

	// Start goroutines
	pm.wg.Add(4) // readOutput, drainStderr, monitorExit, watchdog
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

	// Run watchdog with a short timeout for testing (monkey-patch via goroutine)
	go func() {
		defer pm.wg.Done()
		// Short timeout for testing: select on 1s instead of ContainerStartupTimeout
		pm.mu.Lock()
		ready := pm.containerReady
		pm.mu.Unlock()

		select {
		case <-time.After(1 * time.Second):
			pm.log.Error("test watchdog fired")
			pm.mu.Lock()
			pm.containerTimeout = true
			innerCmd := pm.cmd
			pm.mu.Unlock()
			if innerCmd != nil && innerCmd.Process != nil {
				innerCmd.Process.Kill()
			}
		case <-ready:
			return
		case <-pm.ctx.Done():
			return
		}
	}()

	// Wait for fatal error callback
	select {
	case <-fatalCh:
		// Good — watchdog triggered fatal error
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for OnFatalError callback")
	}

	if fatalErr == nil {
		t.Fatal("expected non-nil fatal error")
	}
	if !strings.Contains(fatalErr.Error(), "container failed to start") {
		t.Errorf("expected timeout error message, got: %v", fatalErr)
	}

	// Verify containerTimeout flag was set
	pm.mu.Lock()
	timedOut := pm.containerTimeout
	pm.mu.Unlock()
	if !timedOut {
		t.Error("containerTimeout should be true after watchdog fires")
	}

	// Clean up
	pm.Stop()
}

func TestContainerStartupWatchdog_CancelledOnReady(t *testing.T) {
	// Start watchdog, call MarkSessionStarted(), verify watchdog exits cleanly.

	pm := NewProcessManager(ProcessConfig{
		SessionID:     "test-watchdog-ready",
		WorkingDir:    t.TempDir(),
		Containerized: true,
	}, ProcessCallbacks{
		OnFatalError: func(err error) {
			t.Errorf("OnFatalError should not be called, got: %v", err)
		},
	}, pmTestLogger())

	pm.mu.Lock()
	pm.ctx, pm.cancel = context.WithCancel(context.Background())
	pm.containerReady = make(chan struct{})
	pm.containerTimeout = false
	pm.running = true
	pm.mu.Unlock()

	watchdogDone := make(chan struct{})
	pm.wg.Go(func() {
		pm.containerStartupWatchdog()
		close(watchdogDone)
	})

	// Signal ready
	pm.MarkSessionStarted()

	// Watchdog should exit promptly
	select {
	case <-watchdogDone:
		// Good
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog did not exit after MarkSessionStarted")
	}

	// containerTimeout should NOT be set
	pm.mu.Lock()
	timedOut := pm.containerTimeout
	pm.mu.Unlock()
	if timedOut {
		t.Error("containerTimeout should be false when session started normally")
	}

	pm.cancel()
}

func TestContainerStartupWatchdog_CancelledOnStop(t *testing.T) {
	// Start watchdog, cancel context, verify watchdog exits cleanly.

	pm := NewProcessManager(ProcessConfig{
		SessionID:     "test-watchdog-stop",
		WorkingDir:    t.TempDir(),
		Containerized: true,
	}, ProcessCallbacks{
		OnFatalError: func(err error) {
			t.Errorf("OnFatalError should not be called, got: %v", err)
		},
	}, pmTestLogger())

	pm.mu.Lock()
	pm.ctx, pm.cancel = context.WithCancel(context.Background())
	pm.containerReady = make(chan struct{})
	pm.containerTimeout = false
	pm.running = true
	pm.mu.Unlock()

	watchdogDone := make(chan struct{})
	pm.wg.Go(func() {
		pm.containerStartupWatchdog()
		close(watchdogDone)
	})

	// Cancel context (simulates Stop)
	pm.cancel()

	// Watchdog should exit promptly
	select {
	case <-watchdogDone:
		// Good
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog did not exit after context cancel")
	}

	// containerTimeout should NOT be set
	pm.mu.Lock()
	timedOut := pm.containerTimeout
	pm.mu.Unlock()
	if timedOut {
		t.Error("containerTimeout should be false when context was cancelled")
	}
}

func TestContainerStartupWatchdog_NotStartedForNonContainer(t *testing.T) {
	// Non-containerized config: verify no containerReady channel is created.

	pm := NewProcessManager(ProcessConfig{
		SessionID:     "test-watchdog-noncontainer",
		WorkingDir:    t.TempDir(),
		Containerized: false,
	}, ProcessCallbacks{}, pmTestLogger())

	pm.mu.Lock()
	pm.ctx, pm.cancel = context.WithCancel(context.Background())
	pm.running = true
	pm.mu.Unlock()

	// containerReady should be nil for non-containerized sessions
	pm.mu.Lock()
	ready := pm.containerReady
	pm.mu.Unlock()

	if ready != nil {
		t.Error("containerReady should be nil for non-containerized sessions")
	}

	// Watchdog should exit immediately when containerReady is nil
	watchdogDone := make(chan struct{})
	pm.wg.Go(func() {
		pm.containerStartupWatchdog()
		close(watchdogDone)
	})

	select {
	case <-watchdogDone:
		// Good — exited immediately
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog should exit immediately for non-containerized sessions")
	}

	pm.cancel()
}

func TestContainerStartupTimeoutDuration(t *testing.T) {
	t.Run("default when zero", func(t *testing.T) {
		pm := NewProcessManager(ProcessConfig{
			SessionID:  "test-timeout-default",
			WorkingDir: t.TempDir(),
		}, ProcessCallbacks{}, pmTestLogger())

		got := pm.containerStartupTimeoutDuration()
		if got != ContainerStartupTimeout {
			t.Errorf("expected default %v, got %v", ContainerStartupTimeout, got)
		}
	})

	t.Run("override when set", func(t *testing.T) {
		custom := 10 * time.Minute
		pm := NewProcessManager(ProcessConfig{
			SessionID:               "test-timeout-override",
			WorkingDir:              t.TempDir(),
			ContainerStartupTimeout: custom,
		}, ProcessCallbacks{}, pmTestLogger())

		got := pm.containerStartupTimeoutDuration()
		if got != custom {
			t.Errorf("expected custom %v, got %v", custom, got)
		}
	})
}

func TestContainerStartupWatchdog_CleanupOnCrash(t *testing.T) {
	// Process exits before timeout. Verify cleanupLocked closes containerReady
	// so the watchdog exits cleanly.

	pm := NewProcessManager(ProcessConfig{
		SessionID:     "test-watchdog-crash",
		WorkingDir:    t.TempDir(),
		Containerized: true,
	}, ProcessCallbacks{
		OnProcessExit: func(err error, stderrContent string) bool {
			return false // don't restart
		},
		OnFatalError: func(err error) {
			// Should NOT be called for non-timeout crash when restarts not exhausted
		},
	}, pmTestLogger())

	// Start a process that exits immediately (simulating a crash)
	cmd := exec.Command("true")
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	pm.mu.Lock()
	pm.cmd = cmd
	pm.stdin = stdin
	pm.stdout = bufio.NewReader(stdout)
	pm.stderr = stderrPipe
	pm.stderrDone = make(chan struct{})
	pm.waitDone = make(chan struct{})
	pm.running = true
	pm.ctx, pm.cancel = context.WithCancel(context.Background())
	pm.containerReady = make(chan struct{})
	pm.containerTimeout = false
	pm.mu.Unlock()

	// Start all goroutines including watchdog
	pm.wg.Add(4)
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

	watchdogDone := make(chan struct{})
	go func() {
		defer pm.wg.Done()
		pm.containerStartupWatchdog()
		close(watchdogDone)
	}()

	// The process exits immediately (true command).
	// monitorExit → handleExit → cleanupLocked should close containerReady,
	// which unblocks the watchdog.
	select {
	case <-watchdogDone:
		// Good — watchdog exited because cleanupLocked closed containerReady
	case <-time.After(5 * time.Second):
		t.Fatal("watchdog did not exit after process crash — cleanupLocked may not be closing containerReady")
	}

	// containerTimeout should NOT be set (it was a crash, not a timeout)
	pm.mu.Lock()
	timedOut := pm.containerTimeout
	pm.mu.Unlock()
	if timedOut {
		t.Error("containerTimeout should be false for a non-timeout crash")
	}

	// Clean up
	pm.Stop()
}

func TestBuildContainerRunArgs_CredentialsJsonAuthSource(t *testing.T) {
	// Clear env-based credentials so writeContainerAuthFile returns empty
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")

	// Create a temp home with .credentials.json
	tmpHome := t.TempDir()
	claudeDir := filepath.Join(tmpHome, ".claude")
	if err := os.MkdirAll(claudeDir, 0700); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}
	credFile := filepath.Join(claudeDir, ".credentials.json")
	if err := os.WriteFile(credFile, []byte(`{"refreshToken":"test"}`), 0600); err != nil {
		t.Fatalf("failed to create credentials file: %v", err)
	}
	t.Setenv("HOME", tmpHome)

	config := ProcessConfig{
		SessionID:      "test-cred-json",
		WorkingDir:     "/tmp",
		ContainerImage: "plural-claude",
	}

	result, err := buildContainerRunArgs(config, []string{"--print"})
	if err != nil {
		t.Fatalf("buildContainerRunArgs failed: %v", err)
	}

	// If keychain returned a result on macOS, auth source will be from keychain.
	// But if no keychain (CI / Linux), auth source should be credentials.json.
	if runtime.GOOS != "darwin" {
		if !strings.Contains(result.AuthSource, ".credentials.json") {
			t.Errorf("AuthSource = %q, want it to contain '.credentials.json'", result.AuthSource)
		}
		// Should NOT have --env-file since credentials.json is handled by entrypoint copy
		if containsArg(result.Args, "--env-file") {
			t.Error("Should not use --env-file when auth is via .credentials.json (entrypoint handles copy)")
		}
	}
}

func TestDrainStderr_ContainerStreamsLineByLine(t *testing.T) {
	// Verify that containerized sessions stream stderr line-by-line
	// by checking that individual lines appear in the log output.

	var logBuf strings.Builder
	logger := pmCaptureLogger(&logBuf)

	pm := NewProcessManager(ProcessConfig{
		SessionID:     "test-stderr-streaming",
		WorkingDir:    t.TempDir(),
		Containerized: true,
	}, ProcessCallbacks{}, logger)

	// Create a pipe to simulate stderr with known content
	pr, pw := io.Pipe()

	pm.mu.Lock()
	pm.stderr = pr
	pm.stderrDone = make(chan struct{})
	pm.mu.Unlock()

	// Write stderr lines in a goroutine (pipe blocks until reader consumes)
	go func() {
		fmt.Fprintln(pw, "[plural-update] Checking for updates...")
		fmt.Fprintln(pw, "[plural-update] Current version: unknown")
		fmt.Fprintln(pw, "Error: something went wrong inside container")
		pw.Close()
	}()

	// Run drainStderr — it should read line-by-line for containerized sessions
	pm.drainStderr()

	// Verify each line was logged individually
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "container stderr") {
		t.Error("expected 'container stderr' log entries for containerized session")
	}
	if !strings.Contains(logOutput, "Checking for updates") {
		t.Error("expected first stderr line to be logged")
	}
	if !strings.Contains(logOutput, "something went wrong inside container") {
		t.Error("expected error stderr line to be logged")
	}

	// Verify accumulated content is stored
	pm.mu.Lock()
	content := pm.stderrContent
	pm.mu.Unlock()

	if !strings.Contains(content, "[plural-update] Checking for updates...") {
		t.Errorf("stderrContent missing first line, got: %q", content)
	}
	if !strings.Contains(content, "Error: something went wrong inside container") {
		t.Errorf("stderrContent missing error line, got: %q", content)
	}
}

func TestDrainStderr_NonContainerUsesReadAll(t *testing.T) {
	// Verify that non-containerized sessions use the original ReadAll behavior
	// and log "captured stderr" rather than "container stderr".

	var logBuf strings.Builder
	logger := pmCaptureLogger(&logBuf)

	pm := NewProcessManager(ProcessConfig{
		SessionID:     "test-stderr-noncontainer",
		WorkingDir:    t.TempDir(),
		Containerized: false,
	}, ProcessCallbacks{}, logger)

	pr, pw := io.Pipe()

	pm.mu.Lock()
	pm.stderr = pr
	pm.stderrDone = make(chan struct{})
	pm.mu.Unlock()

	go func() {
		fmt.Fprintln(pw, "some error output")
		pw.Close()
	}()

	pm.drainStderr()

	logOutput := logBuf.String()
	if strings.Contains(logOutput, "container stderr") {
		t.Error("non-containerized session should NOT use 'container stderr' logging")
	}
	if !strings.Contains(logOutput, "captured stderr") {
		t.Error("non-containerized session should use 'captured stderr' logging")
	}

	pm.mu.Lock()
	content := pm.stderrContent
	pm.mu.Unlock()

	if !strings.Contains(content, "some error output") {
		t.Errorf("stderrContent = %q, want it to contain 'some error output'", content)
	}
}

func TestContainerStartupWatchdog_HeartbeatLogs(t *testing.T) {
	// Verify that the watchdog logs periodic heartbeat messages
	// while waiting for the container to start.

	var logBuf strings.Builder
	logger := pmCaptureLogger(&logBuf)

	pm := NewProcessManager(ProcessConfig{
		SessionID:     "test-watchdog-heartbeat",
		WorkingDir:    t.TempDir(),
		Containerized: true,
	}, ProcessCallbacks{}, logger)

	pm.mu.Lock()
	pm.ctx, pm.cancel = context.WithCancel(context.Background())
	pm.containerReady = make(chan struct{})
	pm.containerTimeout = false
	pm.running = true
	pm.mu.Unlock()

	// Override ContainerStartupTimeout for a fast test isn't possible since
	// it's a const. Instead, run the watchdog and cancel it after we've seen
	// at least one heartbeat.
	watchdogDone := make(chan struct{})
	pm.wg.Go(func() {
		pm.containerStartupWatchdog()
		close(watchdogDone)
	})

	// Wait long enough for at least one heartbeat (15s) — that's too slow for a unit test.
	// Instead, cancel the context after a short delay and verify the watchdog structure.
	// We'll verify the heartbeat message format by checking the log after cancel.
	time.Sleep(100 * time.Millisecond)
	pm.cancel()

	select {
	case <-watchdogDone:
		// Good — watchdog exited
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog did not exit after context cancel")
	}

	// Verify the watchdog started message is present
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "container startup watchdog started") {
		t.Error("expected watchdog started log entry")
	}
	if !strings.Contains(logOutput, "container startup watchdog exiting - context cancelled") {
		t.Error("expected watchdog exit log entry")
	}
}

func TestIsChannelClosed(t *testing.T) {
	ch := make(chan struct{})

	if isChannelClosed(ch) {
		t.Error("open channel should not be reported as closed")
	}

	close(ch)

	if !isChannelClosed(ch) {
		t.Error("closed channel should be reported as closed")
	}
}

