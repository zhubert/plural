package app

import (
	"testing"

	"github.com/zhubert/plural-core/claude"
	"github.com/zhubert/plural-core/git"
	"github.com/zhubert/plural-core/mcp"
)

// =============================================================================
// sessionListeners Tests
// =============================================================================

func TestSessionListeners_NilRunner(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	cmds := m.sessionListeners("session-1", nil, nil)
	if cmds != nil {
		t.Error("expected nil commands for nil runner")
	}
}

func TestSessionListeners_BasicRunner(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)

	// Provide a custom response channel since the runner doesn't have one until SendContent is called
	responseChan := make(chan claude.ResponseChunk, 1)
	cmds := m.sessionListeners("session-1", runner, responseChan)

	// Basic runner should have 4 listeners:
	// response, permission, question, plan approval
	if len(cmds) != 4 {
		t.Errorf("expected 4 commands for basic runner, got %d", len(cmds))
	}

	// All commands should be non-nil
	for i, cmd := range cmds {
		if cmd == nil {
			t.Errorf("command %d is nil", i)
		}
	}
}

func TestSessionListeners_SupervisorRunner(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)
	runner.SetSupervisor(true)

	// Provide a custom response channel
	responseChan := make(chan claude.ResponseChunk, 1)
	cmds := m.sessionListeners("session-1", runner, responseChan)

	// Supervisor runner should have 7 listeners:
	// 4 basic + 3 supervisor (createChild, listChildren, mergeChild)
	if len(cmds) != 7 {
		t.Errorf("expected 7 commands for supervisor runner, got %d", len(cmds))
	}

	for i, cmd := range cmds {
		if cmd == nil {
			t.Errorf("command %d is nil", i)
		}
	}
}

func TestSessionListeners_HostToolsRunner(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)
	runner.SetHostTools(true)

	// Provide a custom response channel
	responseChan := make(chan claude.ResponseChunk, 1)
	cmds := m.sessionListeners("session-1", runner, responseChan)

	// Host tools runner should have 7 listeners:
	// 4 basic + 3 host tools (createPR, pushBranch, getReviewComments)
	if len(cmds) != 7 {
		t.Errorf("expected 7 commands for host tools runner, got %d", len(cmds))
	}

	for i, cmd := range cmds {
		if cmd == nil {
			t.Errorf("command %d is nil", i)
		}
	}
}

func TestSessionListeners_AllFeatures(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)
	runner.SetSupervisor(true)
	runner.SetHostTools(true)

	// Provide a custom response channel
	responseChan := make(chan claude.ResponseChunk, 1)
	cmds := m.sessionListeners("session-1", runner, responseChan)

	// Full-featured runner should have 10 listeners:
	// 4 basic + 3 supervisor + 3 host tools
	if len(cmds) != 10 {
		t.Errorf("expected 10 commands for full-featured runner, got %d", len(cmds))
	}

	for i, cmd := range cmds {
		if cmd == nil {
			t.Errorf("command %d is nil", i)
		}
	}
}

func TestSessionListeners_CustomResponseChan(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)
	customChan := make(chan claude.ResponseChunk, 2)

	cmds := m.sessionListeners("session-1", runner, customChan)

	// Execute the first command (should be response listener)
	if len(cmds) == 0 {
		t.Fatal("expected at least one command")
	}

	// Test continuous listening by sending two chunks
	testChunk1 := claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "test1"}
	testChunk2 := claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "test2"}
	customChan <- testChunk1
	customChan <- testChunk2

	// First call should receive first chunk
	msg1 := cmds[0]()
	respMsg1, ok := msg1.(ClaudeResponseMsg)
	if !ok {
		t.Fatalf("expected ClaudeResponseMsg, got %T", msg1)
	}
	if respMsg1.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", respMsg1.SessionID)
	}
	if respMsg1.Chunk.Content != "test1" {
		t.Errorf("expected test1 content, got %s", respMsg1.Chunk.Content)
	}

	// Second call should receive second chunk, verifying continuous listening
	msg2 := cmds[0]()
	respMsg2, ok := msg2.(ClaudeResponseMsg)
	if !ok {
		t.Fatalf("expected ClaudeResponseMsg, got %T", msg2)
	}
	if respMsg2.Chunk.Content != "test2" {
		t.Errorf("expected test2 content, got %s", respMsg2.Chunk.Content)
	}
}

// =============================================================================
// listenForSessionResponse Tests
// =============================================================================

func TestListenForSessionResponse_NilChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	cmd := m.listenForSessionResponse("session-1", nil)
	if cmd != nil {
		t.Error("expected nil command for nil channel")
	}
}

func TestListenForSessionResponse_ReceiveChunk(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	ch := make(chan claude.ResponseChunk, 1)
	testChunk := claude.ResponseChunk{
		Type:    claude.ChunkTypeText,
		Content: "Hello from Claude",
	}
	ch <- testChunk

	cmd := m.listenForSessionResponse("session-1", ch)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	msg := cmd()
	respMsg, ok := msg.(ClaudeResponseMsg)
	if !ok {
		t.Fatalf("expected ClaudeResponseMsg, got %T", msg)
	}

	if respMsg.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", respMsg.SessionID)
	}
	if respMsg.Chunk.Type != claude.ChunkTypeText {
		t.Errorf("expected ChunkTypeText, got %v", respMsg.Chunk.Type)
	}
	if respMsg.Chunk.Content != "Hello from Claude" {
		t.Errorf("expected 'Hello from Claude', got %s", respMsg.Chunk.Content)
	}
}

func TestListenForSessionResponse_ClosedChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	ch := make(chan claude.ResponseChunk)
	close(ch)

	cmd := m.listenForSessionResponse("session-1", ch)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	msg := cmd()
	respMsg, ok := msg.(ClaudeResponseMsg)
	if !ok {
		t.Fatalf("expected ClaudeResponseMsg, got %T", msg)
	}

	// NOTE: Response listeners return Done=true for closed channels to signal stream completion.
	// MCP listeners (permission, question, plan approval) return nil for closed channels since
	// they don't need to signal "done" - they just stop listening when the runner is stopped.
	if !respMsg.Chunk.Done {
		t.Error("expected Done=true for closed channel")
	}
	if respMsg.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", respMsg.SessionID)
	}
}

// =============================================================================
// listenForSessionPermission Tests
// =============================================================================

func TestListenForSessionPermission_NilRunner(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	cmd := m.listenForSessionPermission("session-1", nil)
	if cmd != nil {
		t.Error("expected nil command for nil runner")
	}
}

func TestListenForSessionPermission_NilChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	// Create a stopped runner (has nil channels)
	runner := claude.NewMockRunner("session-1", true, nil)
	runner.Stop()

	cmd := m.listenForSessionPermission("session-1", runner)
	if cmd != nil {
		t.Error("expected nil command when permission channel is nil")
	}
}

func TestListenForSessionPermission_ReceiveRequest(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)

	cmd := m.listenForSessionPermission("session-1", runner)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	testReq := mcp.PermissionRequest{
		ID:          "perm-1",
		Tool:        "Bash",
		Description: "ls -la",
	}

	// Send to buffered channel first, then read with cmd()
	runner.SimulatePermissionRequest(testReq)
	msg := cmd()

	permMsg, ok := msg.(PermissionRequestMsg)
	if !ok {
		t.Fatalf("expected PermissionRequestMsg, got %T", msg)
	}
	if permMsg.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", permMsg.SessionID)
	}
	if permMsg.Request.Tool != "Bash" {
		t.Errorf("expected Bash, got %s", permMsg.Request.Tool)
	}
	if permMsg.Request.Description != "ls -la" {
		t.Errorf("expected 'ls -la', got %s", permMsg.Request.Description)
	}
}

func TestListenForSessionPermission_ClosedChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)

	cmd := m.listenForSessionPermission("session-1", runner)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	// Stop runner to close channels
	runner.Stop()

	msg := cmd()
	if msg != nil {
		t.Errorf("expected nil message for closed channel, got %T", msg)
	}
}

// =============================================================================
// listenForSessionQuestion Tests
// =============================================================================

func TestListenForSessionQuestion_NilRunner(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	cmd := m.listenForSessionQuestion("session-1", nil)
	if cmd != nil {
		t.Error("expected nil command for nil runner")
	}
}

func TestListenForSessionQuestion_NilChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)
	runner.Stop()

	cmd := m.listenForSessionQuestion("session-1", runner)
	if cmd != nil {
		t.Error("expected nil command when question channel is nil")
	}
}

func TestListenForSessionQuestion_ReceiveRequest(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)

	cmd := m.listenForSessionQuestion("session-1", runner)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	testReq := mcp.QuestionRequest{
		ID: "q-1",
		Questions: []mcp.Question{
			{
				Question: "Which approach?",
				Header:   "Approach",
				Options: []mcp.QuestionOption{
					{Label: "A", Description: "First"},
					{Label: "B", Description: "Second"},
				},
			},
		},
	}

	// Send to buffered channel first, then read with cmd()
	runner.SimulateQuestionRequest(testReq)
	msg := cmd()

	qMsg, ok := msg.(QuestionRequestMsg)
	if !ok {
		t.Fatalf("expected QuestionRequestMsg, got %T", msg)
	}
	if qMsg.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", qMsg.SessionID)
	}
	if len(qMsg.Request.Questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(qMsg.Request.Questions))
	}
	if qMsg.Request.Questions[0].Question != "Which approach?" {
		t.Errorf("unexpected question: %s", qMsg.Request.Questions[0].Question)
	}
}

func TestListenForSessionQuestion_ClosedChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)

	cmd := m.listenForSessionQuestion("session-1", runner)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	runner.Stop()

	msg := cmd()
	if msg != nil {
		t.Errorf("expected nil message for closed channel, got %T", msg)
	}
}

// =============================================================================
// listenForSessionPlanApproval Tests
// =============================================================================

func TestListenForSessionPlanApproval_NilRunner(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	cmd := m.listenForSessionPlanApproval("session-1", nil)
	if cmd != nil {
		t.Error("expected nil command for nil runner")
	}
}

func TestListenForSessionPlanApproval_NilChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)
	runner.Stop()

	cmd := m.listenForSessionPlanApproval("session-1", runner)
	if cmd != nil {
		t.Error("expected nil command when plan approval channel is nil")
	}
}

func TestListenForSessionPlanApproval_ReceiveRequest(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)

	cmd := m.listenForSessionPlanApproval("session-1", runner)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	testReq := mcp.PlanApprovalRequest{
		ID:   "plan-1",
		Plan: "## Implementation Plan\n1. Do this\n2. Do that",
		AllowedPrompts: []mcp.AllowedPrompt{
			{Tool: "Bash", Prompt: "run tests"},
		},
	}

	// Send to buffered channel first, then read with cmd()
	runner.SimulatePlanApprovalRequest(testReq)
	msg := cmd()

	planMsg, ok := msg.(PlanApprovalRequestMsg)
	if !ok {
		t.Fatalf("expected PlanApprovalRequestMsg, got %T", msg)
	}
	if planMsg.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", planMsg.SessionID)
	}
	if planMsg.Request.Plan != testReq.Plan {
		t.Errorf("unexpected plan content")
	}
	if len(planMsg.Request.AllowedPrompts) != 1 {
		t.Errorf("expected 1 allowed prompt, got %d", len(planMsg.Request.AllowedPrompts))
	}
}

func TestListenForSessionPlanApproval_ClosedChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)

	cmd := m.listenForSessionPlanApproval("session-1", runner)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	runner.Stop()

	msg := cmd()
	if msg != nil {
		t.Errorf("expected nil message for closed channel, got %T", msg)
	}
}

// =============================================================================
// listenForCreateChildRequest Tests
// =============================================================================

func TestListenForCreateChildRequest_NilChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	// Basic runner without supervisor channels
	runner := claude.NewMockRunner("session-1", true, nil)

	cmd := m.listenForCreateChildRequest("session-1", runner)
	if cmd != nil {
		t.Error("expected nil command when create child channel is nil")
	}
}

func TestListenForCreateChildRequest_ReceiveRequest(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)
	runner.SetSupervisor(true)

	cmd := m.listenForCreateChildRequest("session-1", runner)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	testReq := mcp.CreateChildRequest{
		ID:   "create-1",
		Task: "Implement feature X",
	}

	// Send to buffered channel first, then read with cmd()
	runner.SimulateCreateChildRequest(testReq)
	msg := cmd()

	createMsg, ok := msg.(CreateChildRequestMsg)
	if !ok {
		t.Fatalf("expected CreateChildRequestMsg, got %T", msg)
	}
	if createMsg.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", createMsg.SessionID)
	}
	if createMsg.Request.Task != "Implement feature X" {
		t.Errorf("unexpected task: %s", createMsg.Request.Task)
	}
}

func TestListenForCreateChildRequest_ClosedChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)
	runner.SetSupervisor(true)

	cmd := m.listenForCreateChildRequest("session-1", runner)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	runner.Stop()

	msg := cmd()
	if msg != nil {
		t.Errorf("expected nil message for closed channel, got %T", msg)
	}
}

// =============================================================================
// listenForListChildrenRequest Tests
// =============================================================================

func TestListenForListChildrenRequest_NilChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)

	cmd := m.listenForListChildrenRequest("session-1", runner)
	if cmd != nil {
		t.Error("expected nil command when list children channel is nil")
	}
}

func TestListenForListChildrenRequest_ReceiveRequest(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)
	runner.SetSupervisor(true)

	cmd := m.listenForListChildrenRequest("session-1", runner)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	testReq := mcp.ListChildrenRequest{
		ID: "list-1",
	}

	// Send to buffered channel first, then read with cmd()
	runner.SimulateListChildrenRequest(testReq)
	msg := cmd()

	listMsg, ok := msg.(ListChildrenRequestMsg)
	if !ok {
		t.Fatalf("expected ListChildrenRequestMsg, got %T", msg)
	}
	if listMsg.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", listMsg.SessionID)
	}
	if listMsg.Request.ID != "list-1" {
		t.Errorf("unexpected ID: %s", listMsg.Request.ID)
	}
}

func TestListenForListChildrenRequest_ClosedChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)
	runner.SetSupervisor(true)

	cmd := m.listenForListChildrenRequest("session-1", runner)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	runner.Stop()

	msg := cmd()
	if msg != nil {
		t.Errorf("expected nil message for closed channel, got %T", msg)
	}
}

// =============================================================================
// listenForMergeChildRequest Tests
// =============================================================================

func TestListenForMergeChildRequest_NilChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)

	cmd := m.listenForMergeChildRequest("session-1", runner)
	if cmd != nil {
		t.Error("expected nil command when merge child channel is nil")
	}
}

func TestListenForMergeChildRequest_ReceiveRequest(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)
	runner.SetSupervisor(true)

	cmd := m.listenForMergeChildRequest("session-1", runner)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	testReq := mcp.MergeChildRequest{
		ID:             "merge-1",
		ChildSessionID: "child-123",
	}

	// Send to buffered channel first, then read with cmd()
	runner.SimulateMergeChildRequest(testReq)
	msg := cmd()

	mergeMsg, ok := msg.(MergeChildRequestMsg)
	if !ok {
		t.Fatalf("expected MergeChildRequestMsg, got %T", msg)
	}
	if mergeMsg.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", mergeMsg.SessionID)
	}
	if mergeMsg.Request.ChildSessionID != "child-123" {
		t.Errorf("unexpected child session ID: %s", mergeMsg.Request.ChildSessionID)
	}
}

func TestListenForMergeChildRequest_ClosedChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)
	runner.SetSupervisor(true)

	cmd := m.listenForMergeChildRequest("session-1", runner)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	runner.Stop()

	msg := cmd()
	if msg != nil {
		t.Errorf("expected nil message for closed channel, got %T", msg)
	}
}

// =============================================================================
// listenForCreatePRRequest Tests
// =============================================================================

func TestListenForCreatePRRequest_NilChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)

	cmd := m.listenForCreatePRRequest("session-1", runner)
	if cmd != nil {
		t.Error("expected nil command when create PR channel is nil")
	}
}

func TestListenForCreatePRRequest_ReceiveRequest(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)
	runner.SetHostTools(true)

	cmd := m.listenForCreatePRRequest("session-1", runner)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	testReq := mcp.CreatePRRequest{
		ID:    "pr-1",
		Title: "Add new feature",
	}

	// Send to buffered channel first, then read with cmd()
	runner.SimulateCreatePRRequest(testReq)
	msg := cmd()

	prMsg, ok := msg.(CreatePRRequestMsg)
	if !ok {
		t.Fatalf("expected CreatePRRequestMsg, got %T", msg)
	}
	if prMsg.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", prMsg.SessionID)
	}
	if prMsg.Request.Title != "Add new feature" {
		t.Errorf("unexpected title: %s", prMsg.Request.Title)
	}
}

func TestListenForCreatePRRequest_ClosedChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)
	runner.SetHostTools(true)

	cmd := m.listenForCreatePRRequest("session-1", runner)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	runner.Stop()

	msg := cmd()
	if msg != nil {
		t.Errorf("expected nil message for closed channel, got %T", msg)
	}
}

// =============================================================================
// listenForPushBranchRequest Tests
// =============================================================================

func TestListenForPushBranchRequest_NilChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)

	cmd := m.listenForPushBranchRequest("session-1", runner)
	if cmd != nil {
		t.Error("expected nil command when push branch channel is nil")
	}
}

func TestListenForPushBranchRequest_ReceiveRequest(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)
	runner.SetHostTools(true)

	cmd := m.listenForPushBranchRequest("session-1", runner)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	testReq := mcp.PushBranchRequest{
		ID:            "push-1",
		CommitMessage: "feat: add new feature",
	}

	// Send to buffered channel first, then read with cmd()
	runner.SimulatePushBranchRequest(testReq)
	msg := cmd()

	pushMsg, ok := msg.(PushBranchRequestMsg)
	if !ok {
		t.Fatalf("expected PushBranchRequestMsg, got %T", msg)
	}
	if pushMsg.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", pushMsg.SessionID)
	}
	if pushMsg.Request.CommitMessage != "feat: add new feature" {
		t.Errorf("unexpected commit message: %s", pushMsg.Request.CommitMessage)
	}
}

func TestListenForPushBranchRequest_ClosedChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)
	runner.SetHostTools(true)

	cmd := m.listenForPushBranchRequest("session-1", runner)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	runner.Stop()

	msg := cmd()
	if msg != nil {
		t.Errorf("expected nil message for closed channel, got %T", msg)
	}
}

// =============================================================================
// listenForGetReviewCommentsRequest Tests
// =============================================================================

func TestListenForGetReviewCommentsRequest_NilChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)

	cmd := m.listenForGetReviewCommentsRequest("session-1", runner)
	if cmd != nil {
		t.Error("expected nil command when get review comments channel is nil")
	}
}

func TestListenForGetReviewCommentsRequest_ReceiveRequest(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)
	runner.SetHostTools(true)

	cmd := m.listenForGetReviewCommentsRequest("session-1", runner)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	testReq := mcp.GetReviewCommentsRequest{
		ID: "review-1",
	}

	// Send to buffered channel first, then read with cmd()
	runner.SimulateGetReviewCommentsRequest(testReq)
	msg := cmd()

	reviewMsg, ok := msg.(GetReviewCommentsRequestMsg)
	if !ok {
		t.Fatalf("expected GetReviewCommentsRequestMsg, got %T", msg)
	}
	if reviewMsg.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", reviewMsg.SessionID)
	}
	if reviewMsg.Request.ID != "review-1" {
		t.Errorf("unexpected ID: %s", reviewMsg.Request.ID)
	}
}

func TestListenForGetReviewCommentsRequest_ClosedChannel(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	runner := claude.NewMockRunner("session-1", true, nil)
	runner.SetHostTools(true)

	cmd := m.listenForGetReviewCommentsRequest("session-1", runner)
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	runner.Stop()

	msg := cmd()
	if msg != nil {
		t.Errorf("expected nil message for closed channel, got %T", msg)
	}
}

// =============================================================================
// listenForMergeResult Tests
// =============================================================================

func TestListenForMergeResult_NoSessionState(t *testing.T) {
	cfg := testConfig()
	m := testModel(cfg)

	cmd := m.listenForMergeResult("nonexistent-session")
	if cmd != nil {
		t.Error("expected nil command for nonexistent session")
	}
}

func TestListenForMergeResult_NilChannel(t *testing.T) {
	cfg := testConfig()
	m, _ := testModelWithMocks(cfg, 100, 40)

	// Create session state without merge channel
	state := m.sessionState().GetOrCreate("session-1")
	state.WithLock(func(s *SessionState) {
		s.MergeChan = nil
	})

	cmd := m.listenForMergeResult("session-1")
	if cmd != nil {
		t.Error("expected nil command when merge channel is nil")
	}
}

func TestListenForMergeResult_ReceiveResult(t *testing.T) {
	cfg := testConfig()
	m, _ := testModelWithMocks(cfg, 100, 40)

	// Create session state with merge channel
	mergeChan := make(chan git.Result, 1)
	state := m.sessionState().GetOrCreate("session-1")
	state.WithLock(func(s *SessionState) {
		s.MergeChan = mergeChan
	})

	cmd := m.listenForMergeResult("session-1")
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	testResult := git.Result{
		Output: "Merge successful",
		Done:   true,
	}
	mergeChan <- testResult

	msg := cmd()
	mergeMsg, ok := msg.(MergeResultMsg)
	if !ok {
		t.Fatalf("expected MergeResultMsg, got %T", msg)
	}

	if mergeMsg.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", mergeMsg.SessionID)
	}
	if mergeMsg.Result.Output != "Merge successful" {
		t.Errorf("unexpected output: %s", mergeMsg.Result.Output)
	}
	if !mergeMsg.Result.Done {
		t.Error("expected Done=true")
	}
}

func TestListenForMergeResult_ClosedChannel(t *testing.T) {
	cfg := testConfig()
	m, _ := testModelWithMocks(cfg, 100, 40)

	// Create session state with closed merge channel
	mergeChan := make(chan git.Result)
	close(mergeChan)
	state := m.sessionState().GetOrCreate("session-1")
	state.WithLock(func(s *SessionState) {
		s.MergeChan = mergeChan
	})

	cmd := m.listenForMergeResult("session-1")
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	msg := cmd()
	mergeMsg, ok := msg.(MergeResultMsg)
	if !ok {
		t.Fatalf("expected MergeResultMsg, got %T", msg)
	}

	// Closed channel should return Done: true
	if !mergeMsg.Result.Done {
		t.Error("expected Done=true for closed channel")
	}
	if mergeMsg.SessionID != "session-1" {
		t.Errorf("expected session-1, got %s", mergeMsg.SessionID)
	}
}

func TestListenForMergeResult_ConflictedFiles(t *testing.T) {
	cfg := testConfig()
	m, _ := testModelWithMocks(cfg, 100, 40)

	mergeChan := make(chan git.Result, 1)
	state := m.sessionState().GetOrCreate("session-1")
	state.WithLock(func(s *SessionState) {
		s.MergeChan = mergeChan
	})

	cmd := m.listenForMergeResult("session-1")
	if cmd == nil {
		t.Fatal("expected non-nil command")
	}

	testResult := git.Result{
		Output:          "CONFLICT (content): Merge conflict in main.go",
		Done:            true,
		ConflictedFiles: []string{"main.go", "config.go"},
		RepoPath:        "/test/repo1",
	}
	mergeChan <- testResult

	msg := cmd()
	mergeMsg, ok := msg.(MergeResultMsg)
	if !ok {
		t.Fatalf("expected MergeResultMsg, got %T", msg)
	}

	if len(mergeMsg.Result.ConflictedFiles) != 2 {
		t.Errorf("expected 2 conflicted files, got %d", len(mergeMsg.Result.ConflictedFiles))
	}
	if mergeMsg.Result.ConflictedFiles[0] != "main.go" {
		t.Errorf("unexpected first conflicted file: %s", mergeMsg.Result.ConflictedFiles[0])
	}
	if mergeMsg.Result.RepoPath != "/test/repo1" {
		t.Errorf("unexpected repo path: %s", mergeMsg.Result.RepoPath)
	}
}
