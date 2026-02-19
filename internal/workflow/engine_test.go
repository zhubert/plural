package workflow

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

// mockAction is a test action that returns a preset result.
type mockAction struct {
	result ActionResult
}

func (a *mockAction) Execute(ctx context.Context, ac *ActionContext) ActionResult {
	return a.result
}

// mockEventChecker is a test event checker.
type mockEventChecker struct {
	fired bool
	data  map[string]any
	err   error
}

func (c *mockEventChecker) CheckEvent(ctx context.Context, event string, params *ParamHelper, item *WorkItemView) (bool, map[string]any, error) {
	return c.fired, c.data, c.err
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestEngine_ProcessStep_TerminalSucceed(t *testing.T) {
	cfg := &Config{
		Start: "done",
		States: map[string]*State{
			"done": {Type: StateTypeSucceed},
		},
	}
	engine := NewEngine(cfg, NewActionRegistry(), nil, testLogger())

	view := &WorkItemView{CurrentStep: "done"}
	result, err := engine.ProcessStep(context.Background(), view)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Terminal {
		t.Error("expected terminal")
	}
	if !result.TerminalOK {
		t.Error("expected terminal OK (succeed)")
	}
}

func TestEngine_ProcessStep_TerminalFail(t *testing.T) {
	cfg := &Config{
		Start: "failed",
		States: map[string]*State{
			"failed": {Type: StateTypeFail},
		},
	}
	engine := NewEngine(cfg, NewActionRegistry(), nil, testLogger())

	view := &WorkItemView{CurrentStep: "failed"}
	result, err := engine.ProcessStep(context.Background(), view)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Terminal {
		t.Error("expected terminal")
	}
	if result.TerminalOK {
		t.Error("expected terminal NOT OK (fail)")
	}
}

func TestEngine_ProcessStep_TaskSync(t *testing.T) {
	registry := NewActionRegistry()
	registry.Register("test.action", &mockAction{
		result: ActionResult{Success: true, Data: map[string]any{"key": "val"}},
	})

	cfg := &Config{
		Start: "step1",
		States: map[string]*State{
			"step1":  {Type: StateTypeTask, Action: "test.action", Next: "done", Error: "failed"},
			"done":   {Type: StateTypeSucceed},
			"failed": {Type: StateTypeFail},
		},
	}
	engine := NewEngine(cfg, registry, nil, testLogger())

	view := &WorkItemView{CurrentStep: "step1"}
	result, err := engine.ProcessStep(context.Background(), view)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NewStep != "done" {
		t.Errorf("expected next step 'done', got %q", result.NewStep)
	}
	if result.NewPhase != "idle" {
		t.Errorf("expected phase 'idle', got %q", result.NewPhase)
	}
	if result.Data["key"] != "val" {
		t.Error("expected data to be passed through")
	}
}

func TestEngine_ProcessStep_TaskAsync(t *testing.T) {
	registry := NewActionRegistry()
	registry.Register("async.action", &mockAction{
		result: ActionResult{Success: true, Async: true},
	})

	cfg := &Config{
		Start: "step1",
		States: map[string]*State{
			"step1": {Type: StateTypeTask, Action: "async.action", Next: "done"},
			"done":  {Type: StateTypeSucceed},
		},
	}
	engine := NewEngine(cfg, registry, nil, testLogger())

	view := &WorkItemView{CurrentStep: "step1"}
	result, err := engine.ProcessStep(context.Background(), view)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NewStep != "step1" {
		t.Errorf("expected to stay on step1 (async), got %q", result.NewStep)
	}
	if result.NewPhase != "async_pending" {
		t.Errorf("expected phase async_pending, got %q", result.NewPhase)
	}
}

func TestEngine_ProcessStep_TaskFailure(t *testing.T) {
	registry := NewActionRegistry()
	registry.Register("fail.action", &mockAction{
		result: ActionResult{Success: false, Error: nil},
	})

	cfg := &Config{
		Start: "step1",
		States: map[string]*State{
			"step1":  {Type: StateTypeTask, Action: "fail.action", Next: "done", Error: "failed"},
			"done":   {Type: StateTypeSucceed},
			"failed": {Type: StateTypeFail},
		},
	}
	engine := NewEngine(cfg, registry, nil, testLogger())

	view := &WorkItemView{CurrentStep: "step1"}
	result, err := engine.ProcessStep(context.Background(), view)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NewStep != "failed" {
		t.Errorf("expected error step 'failed', got %q", result.NewStep)
	}
}

func TestEngine_ProcessStep_WaitFired(t *testing.T) {
	checker := &mockEventChecker{fired: true, data: map[string]any{"approved": true}}

	cfg := &Config{
		Start: "wait",
		States: map[string]*State{
			"wait": {Type: StateTypeWait, Event: "pr.reviewed", Next: "done"},
			"done": {Type: StateTypeSucceed},
		},
	}
	engine := NewEngine(cfg, NewActionRegistry(), checker, testLogger())

	view := &WorkItemView{CurrentStep: "wait", Phase: "idle"}
	result, err := engine.ProcessStep(context.Background(), view)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NewStep != "done" {
		t.Errorf("expected next step 'done', got %q", result.NewStep)
	}
}

func TestEngine_ProcessStep_WaitNotFired(t *testing.T) {
	checker := &mockEventChecker{fired: false}

	cfg := &Config{
		Start: "wait",
		States: map[string]*State{
			"wait": {Type: StateTypeWait, Event: "ci.complete", Next: "done"},
			"done": {Type: StateTypeSucceed},
		},
	}
	engine := NewEngine(cfg, NewActionRegistry(), checker, testLogger())

	view := &WorkItemView{CurrentStep: "wait", Phase: "idle"}
	result, err := engine.ProcessStep(context.Background(), view)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should stay in current step
	if result.NewStep != "wait" {
		t.Errorf("expected to stay on 'wait', got %q", result.NewStep)
	}
}

func TestEngine_AdvanceAfterAsync_Success(t *testing.T) {
	cfg := &Config{
		Start: "coding",
		States: map[string]*State{
			"coding": {Type: StateTypeTask, Action: "ai.code", Next: "open_pr", Error: "failed"},
			"open_pr": {Type: StateTypeTask, Action: "github.create_pr", Next: "done"},
			"done":   {Type: StateTypeSucceed},
			"failed": {Type: StateTypeFail},
		},
	}
	engine := NewEngine(cfg, NewActionRegistry(), nil, testLogger())

	view := &WorkItemView{CurrentStep: "coding", Phase: "async_pending"}
	result, err := engine.AdvanceAfterAsync(view, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NewStep != "open_pr" {
		t.Errorf("expected open_pr, got %q", result.NewStep)
	}
}

func TestEngine_AdvanceAfterAsync_Failure(t *testing.T) {
	cfg := &Config{
		Start: "coding",
		States: map[string]*State{
			"coding": {Type: StateTypeTask, Action: "ai.code", Next: "done", Error: "failed"},
			"done":   {Type: StateTypeSucceed},
			"failed": {Type: StateTypeFail},
		},
	}
	engine := NewEngine(cfg, NewActionRegistry(), nil, testLogger())

	view := &WorkItemView{CurrentStep: "coding", Phase: "async_pending"}
	result, err := engine.AdvanceAfterAsync(view, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NewStep != "failed" {
		t.Errorf("expected failed, got %q", result.NewStep)
	}
}

func TestEngine_GetStartState(t *testing.T) {
	cfg := &Config{Start: "my_start"}
	engine := NewEngine(cfg, NewActionRegistry(), nil, testLogger())
	if engine.GetStartState() != "my_start" {
		t.Errorf("expected my_start, got %s", engine.GetStartState())
	}
}

func TestEngine_IsTerminalState(t *testing.T) {
	cfg := &Config{
		States: map[string]*State{
			"coding": {Type: StateTypeTask},
			"done":   {Type: StateTypeSucceed},
			"failed": {Type: StateTypeFail},
		},
	}
	engine := NewEngine(cfg, NewActionRegistry(), nil, testLogger())

	if engine.IsTerminalState("coding") {
		t.Error("coding should not be terminal")
	}
	if !engine.IsTerminalState("done") {
		t.Error("done should be terminal")
	}
	if !engine.IsTerminalState("failed") {
		t.Error("failed should be terminal")
	}
	if engine.IsTerminalState("nonexistent") {
		t.Error("nonexistent should not be terminal")
	}
}

func TestEngine_FullTraversal(t *testing.T) {
	// Test a full workflow traversal with sync actions and event checks
	registry := NewActionRegistry()
	registry.Register("ai.code", &mockAction{result: ActionResult{Success: true, Async: true}})
	registry.Register("github.create_pr", &mockAction{result: ActionResult{Success: true, Data: map[string]any{"pr_url": "https://github.com/test/pr/1"}}})
	registry.Register("github.merge", &mockAction{result: ActionResult{Success: true}})

	cfg := DefaultWorkflowConfig()

	// Phase 1: coding (async)
	engine := NewEngine(cfg, registry, nil, testLogger())
	view := &WorkItemView{CurrentStep: "coding", Phase: "idle"}
	result, err := engine.ProcessStep(context.Background(), view)
	if err != nil {
		t.Fatalf("coding step error: %v", err)
	}
	if result.NewPhase != "async_pending" {
		t.Errorf("expected async_pending, got %q", result.NewPhase)
	}

	// Phase 2: after async, advance to open_pr
	view.Phase = "async_pending"
	result, err = engine.AdvanceAfterAsync(view, true)
	if err != nil {
		t.Fatalf("advance error: %v", err)
	}
	if result.NewStep != "open_pr" {
		t.Errorf("expected open_pr, got %q", result.NewStep)
	}

	// Phase 3: open_pr (sync)
	view.CurrentStep = "open_pr"
	view.Phase = "idle"
	result, err = engine.ProcessStep(context.Background(), view)
	if err != nil {
		t.Fatalf("open_pr step error: %v", err)
	}
	if result.NewStep != "await_review" {
		t.Errorf("expected await_review, got %q", result.NewStep)
	}

	// Phase 4: await_review (event fired)
	checker := &mockEventChecker{fired: true, data: map[string]any{"approved": true}}
	engine2 := NewEngine(cfg, registry, checker, testLogger())
	view.CurrentStep = "await_review"
	view.Phase = "idle"
	result, err = engine2.ProcessStep(context.Background(), view)
	if err != nil {
		t.Fatalf("await_review step error: %v", err)
	}
	if result.NewStep != "await_ci" {
		t.Errorf("expected await_ci, got %q", result.NewStep)
	}
}
