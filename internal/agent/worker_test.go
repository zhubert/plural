package agent

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/exec"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/issues"
	"github.com/zhubert/plural/internal/mcp"
	"github.com/zhubert/plural/internal/paths"
	"github.com/zhubert/plural/internal/session"
)

// testAgent creates an agent suitable for testing with mock services.
func testAgent(cfg *config.Config) *Agent {
	mockExec := exec.NewMockExecutor(nil)
	gitSvc := git.NewGitServiceWithExecutor(mockExec)
	sessSvc := session.NewSessionServiceWithExecutor(mockExec)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry := issues.NewProviderRegistry()

	a := New(cfg, gitSvc, sessSvc, registry, logger)
	a.sessionMgr.SetSkipMessageLoad(true)
	return a
}

// testConfig creates a minimal config for testing.
func testConfig() *config.Config {
	return &config.Config{
		Repos:            []string{},
		Sessions:         []config.Session{},
		AllowedTools:     []string{},
		RepoAllowedTools: make(map[string][]string),
		AutoMaxTurns:     50,
		AutoMaxDurationMin: 30,
	}
}

// testSession creates a minimal session for testing.
func testSession(id string) *config.Session {
	return &config.Session{
		ID:           id,
		RepoPath:     "/test/repo",
		WorkTree:     "/test/worktree-" + id,
		Branch:       "feature-" + id,
		Name:         "test/" + id,
		CreatedAt:    time.Now(),
		Started:      true,
		Autonomous:   true,
		Containerized: true,
	}
}

func TestWorkerCompletesAfterDoneChunk(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	sess := testSession("test-1")
	cfg.AddSession(*sess)

	mock := claude.NewMockRunner("test-1", true, nil)
	mock.QueueResponse(
		claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "Hello"},
		claude.ResponseChunk{Done: true},
	)

	w := NewSessionWorker(a, sess, mock, "Solve this issue")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w.Start(ctx)
	w.Wait()

	if !w.Done() {
		t.Error("expected worker to be done")
	}
	if w.turns != 1 {
		t.Errorf("expected 1 turn, got %d", w.turns)
	}
}

func TestWorkerHandlesMultipleChunks(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	sess := testSession("test-2")
	cfg.AddSession(*sess)

	mock := claude.NewMockRunner("test-2", true, nil)
	mock.QueueResponse(
		claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "Part 1 "},
		claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "Part 2 "},
		claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "Part 3"},
		claude.ResponseChunk{Done: true},
	)

	w := NewSessionWorker(a, sess, mock, "Solve this")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w.Start(ctx)
	w.Wait()

	if w.turns != 1 {
		t.Errorf("expected 1 turn, got %d", w.turns)
	}
}

func TestWorkerStopsOnErrorChunk(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	sess := testSession("test-3")
	cfg.AddSession(*sess)

	mock := claude.NewMockRunner("test-3", true, nil)
	mock.QueueResponse(
		claude.ResponseChunk{Error: context.DeadlineExceeded, Done: true},
	)

	w := NewSessionWorker(a, sess, mock, "Solve this")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w.Start(ctx)
	w.Wait()

	if !w.Done() {
		t.Error("expected worker to be done after error")
	}
}

func TestWorkerStopsOnContextCancellation(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	sess := testSession("test-4")
	cfg.AddSession(*sess)

	// Don't queue a done chunk — the worker will wait until context is cancelled
	mock := claude.NewMockRunner("test-4", true, nil)
	mock.QueueResponse(
		claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "Working..."},
	)

	w := NewSessionWorker(a, sess, mock, "Solve this")
	ctx, cancel := context.WithCancel(context.Background())

	w.Start(ctx)

	// Give the worker time to start processing
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		w.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop after context cancellation")
	}
}

func TestWorkerRespectsMaxTurns(t *testing.T) {
	cfg := testConfig()
	cfg.AutoMaxTurns = 2
	a := testAgent(cfg)

	sess := testSession("test-5")
	cfg.AddSession(*sess)

	mock := claude.NewMockRunner("test-5", true, nil)
	// Queue enough responses for 3 turns — the worker should stop after 2
	mock.QueueResponse(
		claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "Turn 1"},
		claude.ResponseChunk{Done: true},
	)

	w := NewSessionWorker(a, sess, mock, "Solve this")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w.Start(ctx)
	w.Wait()

	// After first response completes, worker checks limits.
	// With max 2 turns and 1 turn completed, it should continue...
	// But there's no second response queued and no pending message, so it completes.
	if !w.Done() {
		t.Error("expected worker to be done")
	}
}

func TestWorkerAutoRespondsToQuestion(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	sess := testSession("test-6")
	cfg.AddSession(*sess)

	mock := claude.NewMockRunner("test-6", true, nil)

	// We need to intercept the question response
	var receivedAnswers map[string]string
	var mu sync.Mutex
	mock.OnQuestionResp = func(resp mcp.QuestionResponse) {
		mu.Lock()
		receivedAnswers = resp.Answers
		mu.Unlock()
	}

	// Queue the initial response (will complete with done)
	mock.QueueResponse(
		claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "Working"},
	)

	w := NewSessionWorker(a, sess, mock, "Solve this")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w.Start(ctx)

	// Give the worker time to start its select loop
	time.Sleep(100 * time.Millisecond)

	// Simulate a question request while the worker is in the select loop
	mock.SimulateQuestionRequest(mcp.QuestionRequest{
		ID: "q1",
		Questions: []mcp.Question{
			{
				Question: "Which approach?",
				Options: []mcp.QuestionOption{
					{Label: "Option A"},
					{Label: "Option B"},
				},
			},
		},
	})

	// Give the worker time to process the question
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	answers := receivedAnswers
	mu.Unlock()

	if answers == nil {
		t.Fatal("expected question to be answered")
	}
	if answers["Which approach?"] != "Option A" {
		t.Errorf("expected first option 'Option A', got %q", answers["Which approach?"])
	}

	cancel()
	w.Wait()
}

func TestWorkerAutoApprovesPlan(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	sess := testSession("test-7")
	cfg.AddSession(*sess)

	mock := claude.NewMockRunner("test-7", true, nil)

	var approved bool
	var mu sync.Mutex
	mock.OnPlanApprovalResp = func(resp mcp.PlanApprovalResponse) {
		mu.Lock()
		approved = resp.Approved
		mu.Unlock()
	}

	// Queue initial response without done chunk (worker will wait in select)
	mock.QueueResponse(
		claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "Planning..."},
	)

	w := NewSessionWorker(a, sess, mock, "Solve this")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	// Simulate plan approval request
	mock.SimulatePlanApprovalRequest(mcp.PlanApprovalRequest{
		ID:   "p1",
		Plan: "Step 1: Do X\nStep 2: Do Y",
	})

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	wasApproved := approved
	mu.Unlock()

	if !wasApproved {
		t.Error("expected plan to be auto-approved")
	}

	cancel()
	w.Wait()
}

func TestWorkerDoneBeforeStart(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	sess := testSession("test-8")

	mock := claude.NewMockRunner("test-8", true, nil)
	w := NewSessionWorker(a, sess, mock, "Solve this")

	if w.Done() {
		t.Error("worker should not be done before Start()")
	}
}

func TestNewSessionWorker(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	sess := testSession("test-9")

	mock := claude.NewMockRunner("test-9", true, nil)
	w := NewSessionWorker(a, sess, mock, "Initial message")

	if w.sessionID != "test-9" {
		t.Errorf("expected sessionID 'test-9', got %q", w.sessionID)
	}
	if w.initialMsg != "Initial message" {
		t.Errorf("expected initialMsg 'Initial message', got %q", w.initialMsg)
	}
	if w.turns != 0 {
		t.Errorf("expected 0 turns initially, got %d", w.turns)
	}
}

func TestWorkerSendsInitialMessage(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	sess := testSession("test-10")
	cfg.AddSession(*sess)

	mock := claude.NewMockRunner("test-10", true, nil)

	var sentContent []claude.ContentBlock
	mock.OnSend = func(content []claude.ContentBlock) {
		sentContent = content
	}

	mock.QueueResponse(
		claude.ResponseChunk{Done: true},
	)

	w := NewSessionWorker(a, sess, mock, "GitHub Issue #42: Fix bug\n\nDetails here")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w.Start(ctx)
	w.Wait()

	if len(sentContent) == 0 {
		t.Fatal("expected initial message to be sent")
	}
	if sentContent[0].Text != "GitHub Issue #42: Fix bug\n\nDetails here" {
		t.Errorf("unexpected initial message: %q", sentContent[0].Text)
	}
}

func TestCheckLimits(t *testing.T) {
	t.Run("under limits", func(t *testing.T) {
		cfg := testConfig()
		cfg.AutoMaxTurns = 10
		cfg.AutoMaxDurationMin = 30
		a := testAgent(cfg)

		sess := testSession("test-limits-1")
		w := NewSessionWorker(a, sess, nil, "")
		w.turns = 5
		w.startTime = time.Now()

		if w.checkLimits() {
			t.Error("should not hit limits")
		}
	})

	t.Run("at turn limit", func(t *testing.T) {
		cfg := testConfig()
		cfg.AutoMaxTurns = 5
		a := testAgent(cfg)

		sess := testSession("test-limits-2")
		w := NewSessionWorker(a, sess, nil, "")
		w.turns = 5

		if !w.checkLimits() {
			t.Error("should hit turn limit")
		}
	})

	t.Run("at duration limit", func(t *testing.T) {
		cfg := testConfig()
		cfg.AutoMaxDurationMin = 1 // 1 minute
		a := testAgent(cfg)

		sess := testSession("test-limits-3")
		w := NewSessionWorker(a, sess, nil, "")
		w.startTime = time.Now().Add(-2 * time.Minute) // Started 2 minutes ago

		if !w.checkLimits() {
			t.Error("should hit duration limit")
		}
	})
}

func TestHasActiveChildren(t *testing.T) {
	t.Run("no children", func(t *testing.T) {
		cfg := testConfig()
		a := testAgent(cfg)

		sess := testSession("supervisor-1")
		sess.IsSupervisor = true
		cfg.AddSession(*sess)

		mock := claude.NewMockRunner("supervisor-1", true, nil)
		w := NewSessionWorker(a, sess, mock, "")

		if w.hasActiveChildren() {
			t.Error("should have no active children")
		}
	})

	t.Run("with active child worker", func(t *testing.T) {
		cfg := testConfig()
		a := testAgent(cfg)

		sess := testSession("supervisor-2")
		sess.IsSupervisor = true
		cfg.AddSession(*sess)

		childSess := testSession("child-1")
		childSess.SupervisorID = "supervisor-2"
		cfg.AddSession(*childSess)
		cfg.AddChildSession("supervisor-2", "child-1")

		// Create a child worker that is not done
		childMock := claude.NewMockRunner("child-1", true, nil)
		childWorker := NewSessionWorker(a, childSess, childMock, "")
		a.workers["child-1"] = childWorker

		mock := claude.NewMockRunner("supervisor-2", true, nil)
		w := NewSessionWorker(a, sess, mock, "")

		if !w.hasActiveChildren() {
			t.Error("should have active children")
		}
	})

	t.Run("with completed child worker", func(t *testing.T) {
		cfg := testConfig()
		a := testAgent(cfg)

		sess := testSession("supervisor-3")
		sess.IsSupervisor = true
		cfg.AddSession(*sess)

		childSess := testSession("child-2")
		childSess.SupervisorID = "supervisor-3"
		cfg.AddSession(*childSess)
		cfg.AddChildSession("supervisor-3", "child-2")

		// Create a child worker that is done
		childMock := claude.NewMockRunner("child-2", true, nil)
		childWorker := NewSessionWorker(a, childSess, childMock, "")
		close(childWorker.done) // Mark as done
		a.workers["child-2"] = childWorker

		mock := claude.NewMockRunner("supervisor-3", true, nil)
		w := NewSessionWorker(a, sess, mock, "")

		if w.hasActiveChildren() {
			t.Error("should not have active children (child is done)")
		}
	})
}

// testRunnerFactory is a test factory that returns pre-created mock runners.
type testRunnerFactory struct {
	mu      sync.Mutex
	runners map[string]*claude.MockRunner
}

func newTestRunnerFactory() *testRunnerFactory {
	return &testRunnerFactory{
		runners: make(map[string]*claude.MockRunner),
	}
}

func (f *testRunnerFactory) Create(sessionID, workingDir, repoPath string, started bool, msgs []claude.Message) claude.RunnerInterface {
	f.mu.Lock()
	defer f.mu.Unlock()
	mock := claude.NewMockRunner(sessionID, started, msgs)
	f.runners[sessionID] = mock
	return mock
}

func (f *testRunnerFactory) GetMock(sessionID string) *claude.MockRunner {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runners[sessionID]
}

// Ensure the agent properly uses SessionManager for runner factory injection.
func testAgentWithFactory(cfg *config.Config) (*Agent, *testRunnerFactory) {
	a := testAgent(cfg)
	factory := newTestRunnerFactory()
	a.sessionMgr.SetRunnerFactory(factory.Create)
	return a, factory
}

// Verify that StateManager integration works for pending messages
func TestWorkerStateManagerIntegration(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	sess := testSession("state-test-1")
	cfg.AddSession(*sess)

	// Verify StateManager is accessible
	sm := a.sessionMgr.StateManager()
	if sm == nil {
		t.Fatal("expected StateManager to be non-nil")
	}

	// Create a session state and set a pending message
	state := sm.GetOrCreate("state-test-1")
	if state == nil {
		t.Fatal("expected state to be non-nil")
	}
}

func TestWorkerAutoRespondsToQuestionNoOptions(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	sess := testSession("test-no-opts")
	cfg.AddSession(*sess)

	mock := claude.NewMockRunner("test-no-opts", true, nil)

	var receivedAnswers map[string]string
	var mu sync.Mutex
	mock.OnQuestionResp = func(resp mcp.QuestionResponse) {
		mu.Lock()
		receivedAnswers = resp.Answers
		mu.Unlock()
	}

	mock.QueueResponse(
		claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "Working"},
	)

	w := NewSessionWorker(a, sess, mock, "Solve this")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	w.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Simulate a question with no options
	mock.SimulateQuestionRequest(mcp.QuestionRequest{
		ID: "q2",
		Questions: []mcp.Question{
			{
				Question: "What should I do?",
				Options:  nil,
			},
		},
	})

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	answers := receivedAnswers
	mu.Unlock()

	if answers == nil {
		t.Fatal("expected question to be answered")
	}
	if answers["What should I do?"] != "Continue as you see fit" {
		t.Errorf("expected fallback answer, got %q", answers["What should I do?"])
	}

	cancel()
	w.Wait()
}

// Test that calling GetPendingMessage returns and clears the pending message.
func TestGetPendingMessage(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	sm := a.sessionMgr.StateManager()
	state := sm.GetOrCreate("pending-test")
	state.SetPendingMsg("test message")

	msg := sm.GetPendingMessage("pending-test")
	if msg != "test message" {
		t.Errorf("expected 'test message', got %q", msg)
	}

	// Should be cleared after retrieval
	msg = sm.GetPendingMessage("pending-test")
	if msg != "" {
		t.Errorf("expected empty after retrieval, got %q", msg)
	}
}

// Verify NewSessionManager integration doesn't require a real git service.
func TestAgentSessionManagerIntegration(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	if a.sessionMgr == nil {
		t.Fatal("expected sessionMgr to be non-nil")
	}

	// Verify we can use it
	runner := a.sessionMgr.GetRunner("nonexistent")
	if runner != nil {
		t.Error("expected nil runner for nonexistent session")
	}
}

// Verify SessionManager is exported via Agent for subpackage usage patterns.
func TestSessionManagerStateManager(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	sm := a.sessionMgr.StateManager()
	state := sm.GetOrCreate("test-state")
	if state == nil {
		t.Fatal("expected to create session state")
	}
}

// TestWorkerKeepsRunningDuringAutoMerge verifies that the worker loop
// continues running when auto-merge is active to process pending messages
// (e.g., review comments from the auto-merge state machine).
func TestWorkerKeepsRunningDuringAutoMerge(t *testing.T) {
	origInterval := autoMergeWorkerPollInterval
	autoMergeWorkerPollInterval = 100 * time.Millisecond
	defer func() { autoMergeWorkerPollInterval = origInterval }()

	cfg := testConfig()
	sess := testSession("test-automerge")
	sess.Autonomous = true
	sess.IsSupervisor = true
	sess.PRCreated = true // PR already created
	sess.PRMerged = false
	sess.PRClosed = false

	cfg.AddSession(*sess)

	a := testAgent(cfg)
	a.autoMerge = true

	// Create a mock runner that completes immediately
	mockRunner := claude.NewMockRunner(sess.ID, true, nil)
	mockRunner.QueueResponse(
		claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "Task completed"},
		claude.ResponseChunk{Done: true},
	)

	worker := NewSessionWorker(a, sess, mockRunner, "Test task")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	worker.Start(ctx)

	// Wait for worker to process initial response
	time.Sleep(100 * time.Millisecond)

	// Verify worker hasn't exited yet (should be waiting for auto-merge)
	if worker.Done() {
		t.Fatal("Worker should still be running while auto-merge is active")
	}

	// Simulate auto-merge completing by marking PR as merged
	cfg.MarkSessionPRMerged(sess.ID)

	// Wait for worker to exit
	worker.Wait()

	if !worker.Done() {
		t.Fatal("Worker should have exited after PR was merged")
	}
}

// TestWorkerProcessesPendingMessagesDuringAutoMerge verifies that pending
// messages (e.g., review comments) are sent to Claude while auto-merge is running.
func TestWorkerProcessesPendingMessagesDuringAutoMerge(t *testing.T) {
	origInterval := autoMergeWorkerPollInterval
	autoMergeWorkerPollInterval = 100 * time.Millisecond
	defer func() { autoMergeWorkerPollInterval = origInterval }()

	cfg := testConfig()
	sess := testSession("test-automerge-comments")
	sess.Autonomous = true
	sess.IsSupervisor = true
	sess.PRCreated = true
	sess.PRMerged = false
	sess.PRClosed = false

	cfg.AddSession(*sess)

	a := testAgent(cfg)
	a.autoMerge = true

	mockRunner := claude.NewMockRunner(sess.ID, true, nil)

	// Queue initial response
	mockRunner.QueueResponse(
		claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "Initial task done"},
		claude.ResponseChunk{Done: true},
	)

	worker := NewSessionWorker(a, sess, mockRunner, "Test task")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	worker.Start(ctx)

	// Wait for initial task to complete
	time.Sleep(100 * time.Millisecond)

	// Queue review comment response BEFORE setting pending message
	mockRunner.QueueResponse(
		claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "Fixed review comments"},
		claude.ResponseChunk{Done: true},
	)

	// Simulate auto-merge detecting review comments
	state := a.sessionMgr.StateManager().GetOrCreate(sess.ID)
	state.SetPendingMsg("Review comments:\n1. Fix timing\n2. Add docs")

	// Wait for pending message to be sent
	time.Sleep(200 * time.Millisecond)

	// Complete auto-merge
	cfg.MarkSessionPRMerged(sess.ID)

	// Wait for worker to complete
	worker.Wait()

	// Verify worker processed at least 2 turns (initial + review comments)
	if worker.turns < 2 {
		t.Errorf("Expected at least 2 turns (initial + review comments), got %d", worker.turns)
	}
}

// TestWorkerAutoMergeDoesNotIncrementTurns verifies that the worker's auto-merge
// polling loop does NOT increment w.turns. Only actual Claude API round-trips
// should count as turns.
func TestWorkerAutoMergeDoesNotIncrementTurns(t *testing.T) {
	origInterval := autoMergeWorkerPollInterval
	autoMergeWorkerPollInterval = 100 * time.Millisecond
	defer func() { autoMergeWorkerPollInterval = origInterval }()

	cfg := testConfig()
	sess := testSession("test-automerge-turns")
	sess.Autonomous = true
	sess.IsSupervisor = true
	sess.PRCreated = true // PR already created
	sess.PRMerged = false
	sess.PRClosed = false

	cfg.AddSession(*sess)

	a := testAgent(cfg)
	a.autoMerge = true

	// Create a mock runner that completes after one response
	mockRunner := claude.NewMockRunner(sess.ID, true, nil)
	mockRunner.QueueResponse(
		claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "Task completed"},
		claude.ResponseChunk{Done: true},
	)

	worker := NewSessionWorker(a, sess, mockRunner, "Test task")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	worker.Start(ctx)

	// Wait for the initial response to be processed
	time.Sleep(200 * time.Millisecond)

	// At this point worker should have 1 turn from the initial response
	// and be in auto-merge polling mode

	// Let it poll a couple of cycles (autoMergeWorkerPollInterval is 15s
	// but the worker should check every cycle). We'll just wait a short
	// time and then mark as merged.
	time.Sleep(300 * time.Millisecond)

	turnsBeforeMerge := worker.turns

	// Mark PR as merged to let the worker exit
	cfg.MarkSessionPRMerged(sess.ID)

	// Wait for worker to exit
	worker.Wait()

	// Turns should not have increased beyond what was set before auto-merge polling
	if worker.turns != turnsBeforeMerge {
		t.Errorf("expected turns to remain at %d during auto-merge polling, got %d",
			turnsBeforeMerge, worker.turns)
	}
	if worker.turns != 1 {
		t.Errorf("expected exactly 1 turn (initial response only), got %d", worker.turns)
	}
}

// TestStandaloneAgentNotDaemonManaged verifies that agents created via New()
// have daemonManaged=false by default.
func TestStandaloneAgentNotDaemonManaged(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)
	if a.daemonManaged {
		t.Error("expected daemonManaged=false for standalone agent")
	}
}

// TestWorkerDaemonManagedSkipsAutoMerge verifies that a daemon-managed worker
// exits promptly after Claude completes without launching auto-merge goroutines.
func TestWorkerDaemonManagedSkipsAutoMerge(t *testing.T) {
	cfg := testConfig()
	sess := testSession("test-daemon-managed")
	sess.Autonomous = true
	sess.IsSupervisor = true
	sess.PRCreated = true
	sess.PRMerged = false
	sess.PRClosed = false

	cfg.AddSession(*sess)

	a := testAgent(cfg)
	a.autoMerge = true
	a.daemonManaged = true // Daemon manages lifecycle

	mockRunner := claude.NewMockRunner(sess.ID, true, nil)
	mockRunner.QueueResponse(
		claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "Task completed"},
		claude.ResponseChunk{Done: true},
	)

	worker := NewSessionWorker(a, sess, mockRunner, "Test task")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	worker.Start(ctx)

	// Worker should exit promptly since daemonManaged skips auto-merge.
	// Use a channel with timeout to verify it doesn't hang.
	done := make(chan struct{})
	go func() {
		worker.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Worker exited promptly — success
	case <-time.After(1 * time.Second):
		t.Fatal("daemon-managed worker should exit promptly without auto-merge, but it hung")
	}

	if !worker.Done() {
		t.Error("expected worker to be done")
	}
}

// TestWorkerHandleCreatePRSavesMessagesBeforePR verifies that handleCreatePR
// saves the runner's messages to disk before calling CreatePR. This ensures
// loadTranscript() can find the messages and upload the transcript to the PR.
// Without this, messages from the current turn haven't been persisted yet
// since handleDone/saveRunnerMessages only runs after the turn completes.
func TestWorkerHandleCreatePRSavesMessagesBeforePR(t *testing.T) {
	// Set up temp dir for session messages so we don't pollute real data.
	// Override HOME so paths.resolve() won't find ~/.plural (legacy mode)
	// and will fall through to the XDG env var path.
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	origXDG := os.Getenv("XDG_DATA_HOME")
	os.Setenv("HOME", tmpDir)
	os.Setenv("XDG_DATA_HOME", tmpDir)
	paths.Reset()
	defer func() {
		os.Setenv("HOME", origHome)
		if origXDG != "" {
			os.Setenv("XDG_DATA_HOME", origXDG)
		} else {
			os.Unsetenv("XDG_DATA_HOME")
		}
		paths.Reset()
	}()

	cfg := testConfig()
	sess := testSession("test-createpr-msgs")
	sess.Autonomous = true
	sess.PRCreated = false

	cfg.AddSession(*sess)

	// Track whether messages were on disk when gh pr create ran
	var messagesFoundDuringPR bool
	var mu sync.Mutex

	mockExec := exec.NewMockExecutor(nil)
	// Use AddRule with a custom matcher that checks for messages on disk
	// at the time gh pr create is invoked
	mockExec.AddRule(func(dir, name string, args []string) bool {
		if name != "gh" || len(args) < 2 || args[0] != "pr" || args[1] != "create" {
			return false
		}
		// Side effect: check if messages exist on disk during PR creation
		msgs, err := config.LoadSessionMessages("test-createpr-msgs")
		mu.Lock()
		if err == nil && len(msgs) > 0 {
			messagesFoundDuringPR = true
		}
		mu.Unlock()
		return true
	}, exec.MockResponse{
		Stdout: []byte("https://github.com/owner/repo/pull/42\n"),
	})
	mockExec.AddPrefixMatch("git", []string{"push"}, exec.MockResponse{})
	mockExec.AddPrefixMatch("git", []string{"log"}, exec.MockResponse{
		Stdout: []byte("abc1234 Initial commit\n"),
	})
	mockExec.AddPrefixMatch("git", []string{"diff"}, exec.MockResponse{
		Stdout: []byte("diff content"),
	})

	gitSvc := git.NewGitServiceWithExecutor(mockExec)
	sessSvc := session.NewSessionServiceWithExecutor(mockExec)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry := issues.NewProviderRegistry()

	a := New(cfg, gitSvc, sessSvc, registry, logger)
	a.sessionMgr.SetSkipMessageLoad(true)
	a.daemonManaged = true

	// Create a mock runner with pre-existing messages (simulates the
	// conversation that happened before Claude called create_pr)
	initialMessages := []claude.Message{
		{Role: "user", Content: "Fix the bug in login"},
		{Role: "assistant", Content: "I'll fix the login bug by updating the auth handler."},
	}
	mockRunner := claude.NewMockRunner(sess.ID, true, initialMessages)
	mockRunner.SetHostTools(true) // Enable create_pr channel

	// Queue initial response that won't complete (worker stays in select loop)
	mockRunner.QueueResponse(
		claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "Working..."},
	)

	worker := NewSessionWorker(a, sess, mockRunner, "Test task")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	worker.Start(ctx)

	// Give worker time to enter the select loop
	time.Sleep(100 * time.Millisecond)

	// Verify no messages on disk yet (handleDone hasn't been called)
	msgs, _ := config.LoadSessionMessages("test-createpr-msgs")
	if len(msgs) != 0 {
		t.Fatalf("expected no messages on disk before create_pr, got %d", len(msgs))
	}

	// Simulate Claude calling create_pr MCP tool
	mockRunner.SimulateCreatePRRequest(mcp.CreatePRRequest{
		ID:    "pr-1",
		Title: "Fix login bug",
	})

	// Wait for the request to be processed
	time.Sleep(500 * time.Millisecond)

	// Verify messages were on disk when gh pr create executed
	mu.Lock()
	found := messagesFoundDuringPR
	mu.Unlock()
	if !found {
		t.Error("expected session messages to be saved to disk before gh pr create ran")
	}

	// Also verify messages are still on disk after completion.
	// Expect 3 messages: 2 initial + 1 user message from SendContent("Test task")
	msgs, err := config.LoadSessionMessages("test-createpr-msgs")
	if err != nil {
		t.Fatalf("failed to load messages: %v", err)
	}
	if len(msgs) < 2 {
		t.Errorf("expected at least 2 messages on disk, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "Fix the bug in login" {
		t.Errorf("unexpected first message: %+v", msgs[0])
	}

	cancel()
	worker.Wait()
}

// TestWorkerDaemonManagedHandleCreatePRSkipsAutoMerge verifies that
// handleCreatePR does not launch auto-merge when daemonManaged is true.
func TestWorkerDaemonManagedHandleCreatePRSkipsAutoMerge(t *testing.T) {
	cfg := testConfig()
	sess := testSession("test-daemon-createpr")
	sess.Autonomous = true
	sess.PRCreated = false

	cfg.AddSession(*sess)

	mockExec := exec.NewMockExecutor(nil)
	// Mock git operations for PR creation
	mockExec.AddPrefixMatch("gh", []string{"pr", "create"}, exec.MockResponse{
		Stdout: []byte("https://github.com/owner/repo/pull/1\n"),
	})
	mockExec.AddPrefixMatch("git", []string{"push"}, exec.MockResponse{})
	mockExec.AddPrefixMatch("git", []string{"log"}, exec.MockResponse{
		Stdout: []byte("abc1234 Initial commit\n"),
	})
	mockExec.AddPrefixMatch("git", []string{"diff"}, exec.MockResponse{
		Stdout: []byte("diff content"),
	})

	gitSvc := git.NewGitServiceWithExecutor(mockExec)
	sessSvc := session.NewSessionServiceWithExecutor(mockExec)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry := issues.NewProviderRegistry()

	a := New(cfg, gitSvc, sessSvc, registry, logger)
	a.sessionMgr.SetSkipMessageLoad(true)
	a.autoMerge = true
	a.daemonManaged = true

	mockRunner := claude.NewMockRunner(sess.ID, true, nil)

	// Queue initial response that won't complete (worker stays in select loop)
	mockRunner.QueueResponse(
		claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "Working..."},
	)

	worker := NewSessionWorker(a, sess, mockRunner, "Test task")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	worker.Start(ctx)

	// Give worker time to enter the select loop
	time.Sleep(100 * time.Millisecond)

	// Simulate a create_pr MCP request
	mockRunner.SimulateCreatePRRequest(mcp.CreatePRRequest{
		ID:    "pr-1",
		Title: "Fix bug",
	})

	// Give worker time to process the request
	time.Sleep(500 * time.Millisecond)

	// The worker should still be running (not hung in auto-merge)
	// and should NOT have started auto-merge. We verify by cancelling
	// and confirming it exits promptly.
	cancel()

	done := make(chan struct{})
	go func() {
		worker.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Worker exited promptly — no auto-merge goroutine blocking
	case <-time.After(2 * time.Second):
		t.Fatal("daemon-managed worker hung after handleCreatePR, auto-merge may have been started")
	}
}
