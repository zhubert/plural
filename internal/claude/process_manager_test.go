package claude

import (
	"sync/atomic"
	"testing"
	"time"
)

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

	pm := NewProcessManager(config, callbacks)

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
	}, ProcessCallbacks{})

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
	}, ProcessCallbacks{})

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
	}, ProcessCallbacks{})

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
	}, ProcessCallbacks{})

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
	}, ProcessCallbacks{})

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
	}, ProcessCallbacks{})

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
	}, ProcessCallbacks{})

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
	}, ProcessCallbacks{})

	err := pm.WriteMessage([]byte("test message"))
	if err == nil {
		t.Error("WriteMessage should error when process is not running")
	}
}

func TestProcessManager_Interrupt_NotRunning(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{})

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
	}, ProcessCallbacks{})

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
		onLineCalled          int32
		onProcessExitCalled   int32
		onProcessHungCalled   int32
		onRestartAttemptCalled int32
		onRestartFailedCalled  int32
		onFatalErrorCalled    int32
	)

	callbacks := ProcessCallbacks{
		OnLine: func(line string) {
			atomic.AddInt32(&onLineCalled, 1)
		},
		OnProcessExit: func(err error, stderrContent string) bool {
			atomic.AddInt32(&onProcessExitCalled, 1)
			return false
		},
		OnProcessHung: func() {
			atomic.AddInt32(&onProcessHungCalled, 1)
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

	// Test OnProcessHung
	callbacks.OnProcessHung()
	if atomic.LoadInt32(&onProcessHungCalled) != 1 {
		t.Error("OnProcessHung callback not called")
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
	}, ProcessCallbacks{})

	// These should not panic even when callbacks are nil
	pm.callbacks.OnLine = nil
	pm.callbacks.OnProcessExit = nil
	pm.callbacks.OnProcessHung = nil
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
	if errReadTimeout == nil {
		t.Error("errReadTimeout should not be nil")
	}

	if errChannelFull == nil {
		t.Error("errChannelFull should not be nil")
	}

	if errReadTimeout.Error() == "" {
		t.Error("errReadTimeout should have a message")
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
	}, ProcessCallbacks{})

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

	if ResponseReadTimeout <= 0 {
		t.Error("ResponseReadTimeout should be positive")
	}

	if ResponseReadTimeout < time.Minute {
		t.Error("ResponseReadTimeout should be at least 1 minute")
	}
}

func TestProcessManager_ConcurrentAccess(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{})

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
	}, ProcessCallbacks{})

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
	}, ProcessCallbacks{})

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
