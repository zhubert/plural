package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/mcp"
)

func TestSessionStateManager_GetCreatesState(t *testing.T) {
	m := NewSessionStateManager()

	state := m.Get("session-1")
	if state == nil {
		t.Fatal("expected non-nil state")
	}

	// Getting the same session should return the same state
	state2 := m.Get("session-1")
	if state != state2 {
		t.Error("expected same state object on second Get")
	}
}

func TestSessionStateManager_GetIfExistsDoesNotCreate(t *testing.T) {
	m := NewSessionStateManager()

	// Should return nil for non-existent session
	state := m.GetIfExists("session-1")
	if state != nil {
		t.Error("expected nil for non-existent session")
	}

	// Create the session
	m.Get("session-1")

	// Now it should exist
	state = m.GetIfExists("session-1")
	if state == nil {
		t.Error("expected non-nil state after Get")
	}
}

func TestSessionStateManager_Delete(t *testing.T) {
	m := NewSessionStateManager()

	// Create and then delete
	m.Get("session-1")
	m.Delete("session-1")

	// Should be gone
	state := m.GetIfExists("session-1")
	if state != nil {
		t.Error("expected nil after Delete")
	}
}

func TestSessionStateManager_PendingPermission(t *testing.T) {
	m := NewSessionStateManager()

	// Initially nil
	if m.GetPendingPermission("session-1") != nil {
		t.Error("expected nil initially")
	}

	// Set a permission request
	req := &mcp.PermissionRequest{
		ID:   "perm-1",
		Tool: "Read",
	}
	m.SetPendingPermission("session-1", req)

	// Should be retrievable
	got := m.GetPendingPermission("session-1")
	if got == nil {
		t.Fatal("expected non-nil permission")
	}
	if got.ID != "perm-1" {
		t.Errorf("expected ID 'perm-1', got %q", got.ID)
	}

	// Clear it
	m.ClearPendingPermission("session-1")
	if m.GetPendingPermission("session-1") != nil {
		t.Error("expected nil after clear")
	}
}

func TestSessionStateManager_PendingQuestion(t *testing.T) {
	m := NewSessionStateManager()

	// Initially nil
	if m.GetPendingQuestion("session-1") != nil {
		t.Error("expected nil initially")
	}

	// Set a question request
	req := &mcp.QuestionRequest{
		ID: "question-1",
	}
	m.SetPendingQuestion("session-1", req)

	// Should be retrievable
	got := m.GetPendingQuestion("session-1")
	if got == nil {
		t.Fatal("expected non-nil question")
	}
	if got.ID != "question-1" {
		t.Errorf("expected ID 'question-1', got %q", got.ID)
	}

	// Clear it
	m.ClearPendingQuestion("session-1")
	if m.GetPendingQuestion("session-1") != nil {
		t.Error("expected nil after clear")
	}
}

func TestSessionStateManager_Waiting(t *testing.T) {
	m := NewSessionStateManager()

	// Initially not waiting
	if m.IsWaiting("session-1") {
		t.Error("expected not waiting initially")
	}

	// Start waiting
	cancel := func() {}
	m.StartWaiting("session-1", cancel)

	// Now should be waiting
	if !m.IsWaiting("session-1") {
		t.Error("expected waiting after StartWaiting")
	}

	// Wait start time should be set
	startTime, ok := m.GetWaitStart("session-1")
	if !ok {
		t.Error("expected wait start to be set")
	}
	if time.Since(startTime) > time.Second {
		t.Error("wait start time seems wrong")
	}

	// Stop waiting
	m.StopWaiting("session-1")
	if m.IsWaiting("session-1") {
		t.Error("expected not waiting after StopWaiting")
	}
}

func TestSessionStateManager_Merge(t *testing.T) {
	m := NewSessionStateManager()

	// Initially not merging
	if m.IsMerging("session-1") {
		t.Error("expected not merging initially")
	}

	// Start merge
	ch := make(chan git.Result)
	_, cancel := context.WithCancel(context.Background())
	m.StartMerge("session-1", ch, cancel, MergeTypePR)

	// Now should be merging
	if !m.IsMerging("session-1") {
		t.Error("expected merging after StartMerge")
	}

	// Check merge type
	if m.GetMergeType("session-1") != MergeTypePR {
		t.Errorf("expected MergeTypePR, got %v", m.GetMergeType("session-1"))
	}

	// Stop merge
	m.StopMerge("session-1")
	if m.IsMerging("session-1") {
		t.Error("expected not merging after StopMerge")
	}
	if m.GetMergeType("session-1") != MergeTypeNone {
		t.Errorf("expected MergeTypeNone after StopMerge, got %v", m.GetMergeType("session-1"))
	}
}

func TestSessionStateManager_InputText(t *testing.T) {
	m := NewSessionStateManager()

	// Initially empty
	if m.GetInput("session-1") != "" {
		t.Error("expected empty input initially")
	}

	// Save input
	m.SaveInput("session-1", "Hello, world!")

	// Should be retrievable
	if m.GetInput("session-1") != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got %q", m.GetInput("session-1"))
	}

	// Clear input
	m.ClearInput("session-1")
	if m.GetInput("session-1") != "" {
		t.Error("expected empty input after clear")
	}
}

func TestSessionStateManager_Streaming(t *testing.T) {
	m := NewSessionStateManager()

	// Initially empty
	if m.GetStreaming("session-1") != "" {
		t.Error("expected empty streaming initially")
	}

	// Save streaming
	m.SaveStreaming("session-1", "First chunk")

	// Should be retrievable
	if m.GetStreaming("session-1") != "First chunk" {
		t.Errorf("expected 'First chunk', got %q", m.GetStreaming("session-1"))
	}

	// Append streaming
	m.AppendStreaming("session-1", " second chunk")
	if m.GetStreaming("session-1") != "First chunk second chunk" {
		t.Errorf("expected 'First chunk second chunk', got %q", m.GetStreaming("session-1"))
	}

	// Clear streaming
	m.ClearStreaming("session-1")
	if m.GetStreaming("session-1") != "" {
		t.Error("expected empty streaming after clear")
	}
}

func TestSessionStateManager_SessionInUseError(t *testing.T) {
	m := NewSessionStateManager()

	// Initially false
	if m.HasSessionInUseError("session-1") {
		t.Error("expected no error initially")
	}

	// Set error
	m.SetSessionInUseError("session-1", true)
	if !m.HasSessionInUseError("session-1") {
		t.Error("expected error after SetSessionInUseError(true)")
	}

	// Clear error
	m.SetSessionInUseError("session-1", false)
	if m.HasSessionInUseError("session-1") {
		t.Error("expected no error after SetSessionInUseError(false)")
	}
}

func TestSessionStateManager_DeleteCancelsOperations(t *testing.T) {
	m := NewSessionStateManager()

	// Set up state with cancel functions
	ctx, mergeCancel := context.WithCancel(context.Background())
	ch := make(chan git.Result)
	m.StartMerge("session-1", ch, mergeCancel, MergeTypeMerge)

	ctx2, streamCancel := context.WithCancel(context.Background())
	m.StartWaiting("session-1", streamCancel)

	// Delete should cancel both
	m.Delete("session-1")

	// Both contexts should be cancelled
	select {
	case <-ctx.Done():
		// Good
	default:
		t.Error("merge context should be cancelled")
	}

	select {
	case <-ctx2.Done():
		// Good
	default:
		t.Error("stream context should be cancelled")
	}
}

func TestSessionStateManager_ConcurrentAccess(t *testing.T) {
	m := NewSessionStateManager()

	// Run concurrent operations to detect race conditions
	var wg sync.WaitGroup
	const numGoroutines = 10
	const numOperations = 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sessionID := "session-1"

			for j := 0; j < numOperations; j++ {
				switch j % 6 {
				case 0:
					m.Get(sessionID)
				case 1:
					m.SetPendingPermission(sessionID, &mcp.PermissionRequest{ID: "perm"})
					m.ClearPendingPermission(sessionID)
				case 2:
					m.SaveInput(sessionID, "input")
					m.GetInput(sessionID)
				case 3:
					m.AppendStreaming(sessionID, "chunk")
					m.GetStreaming(sessionID)
				case 4:
					m.IsWaiting(sessionID)
				case 5:
					m.IsMerging(sessionID)
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestMergeType_String(t *testing.T) {
	tests := []struct {
		mt       MergeType
		expected string
	}{
		{MergeTypeNone, "none"},
		{MergeTypeMerge, "merge"},
		{MergeTypePR, "pr"},
		{MergeTypeParent, "parent"},
		{MergeType(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.mt.String(); got != tt.expected {
			t.Errorf("MergeType(%d).String() = %q, want %q", tt.mt, got, tt.expected)
		}
	}
}
