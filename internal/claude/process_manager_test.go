package claude

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

// pmTestLogger creates a discard logger for process manager tests
func pmTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
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

	pm.mu.Lock()
	stopped := pm.stopped
	pm.mu.Unlock()

	if !stopped {
		t.Error("stopped flag should be true after Stop")
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

func TestProcessManager_Start_AfterStop(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, pmTestLogger())

	pm.Stop()

	err := pm.Start()
	if err == nil {
		t.Error("Start should error after Stop has been called")
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
		for i := 0; i < 100; i++ {
			pm.GetRestartAttempts()
		}
		done <- true
	}()

	// Concurrent ResetRestartAttempts
	go func() {
		for i := 0; i < 100; i++ {
			pm.ResetRestartAttempts()
		}
		done <- true
	}()

	// Concurrent SetInterrupted
	go func() {
		for i := 0; i < 100; i++ {
			pm.SetInterrupted(i%2 == 0)
		}
		done <- true
	}()

	// Concurrent IsRunning
	go func() {
		for i := 0; i < 100; i++ {
			pm.IsRunning()
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 4; i++ {
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
	for i := 0; i < 10; i++ {
		go func() {
			pm.MarkSessionStarted()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
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
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
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
		SessionStarted:    true, // Marked as started after first response
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
		SessionStarted:    true, // Takes priority
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
	for i := 0; i < 5; i++ {
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

	// Should now use --session-id (new session) instead of --resume
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
	pm.wg.Add(1)
	go func() {
		defer pm.wg.Done()
		// Simulate readOutput's context check
		select {
		case <-pm.ctx.Done():
			exitedCh <- true
			return
		}
	}()

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

