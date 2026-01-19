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

	state := m.GetOrCreate("session-1")
	if state == nil {
		t.Fatal("expected non-nil state")
	}

	// Getting the same session should return the same state
	state2 := m.GetOrCreate("session-1")
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
	m.GetOrCreate("session-1")

	// Now it should exist
	state = m.GetIfExists("session-1")
	if state == nil {
		t.Error("expected non-nil state after Get")
	}
}

func TestSessionStateManager_Delete(t *testing.T) {
	m := NewSessionStateManager()

	// Create and then delete
	m.GetOrCreate("session-1")
	m.Delete("session-1")

	// Should be gone
	state := m.GetIfExists("session-1")
	if state != nil {
		t.Error("expected nil after Delete")
	}
}

func TestSessionStateManager_PendingPermission(t *testing.T) {
	m := NewSessionStateManager()

	// Initially nil (no state exists)
	if state := m.GetIfExists("session-1"); state != nil && state.PendingPermission != nil {
		t.Error("expected nil initially")
	}

	// Set a permission request
	req := &mcp.PermissionRequest{
		ID:   "perm-1",
		Tool: "Read",
	}
	m.GetOrCreate("session-1").PendingPermission = req

	// Should be retrievable
	state := m.GetIfExists("session-1")
	if state == nil || state.PendingPermission == nil {
		t.Fatal("expected non-nil permission")
	}
	if state.PendingPermission.ID != "perm-1" {
		t.Errorf("expected ID 'perm-1', got %q", state.PendingPermission.ID)
	}

	// Clear it
	state.PendingPermission = nil
	if m.GetIfExists("session-1").PendingPermission != nil {
		t.Error("expected nil after clear")
	}
}

func TestSessionStateManager_PendingQuestion(t *testing.T) {
	m := NewSessionStateManager()

	// Initially nil (no state exists)
	if state := m.GetIfExists("session-1"); state != nil && state.PendingQuestion != nil {
		t.Error("expected nil initially")
	}

	// Set a question request
	req := &mcp.QuestionRequest{
		ID: "question-1",
	}
	m.GetOrCreate("session-1").PendingQuestion = req

	// Should be retrievable
	state := m.GetIfExists("session-1")
	if state == nil || state.PendingQuestion == nil {
		t.Fatal("expected non-nil question")
	}
	if state.PendingQuestion.ID != "question-1" {
		t.Errorf("expected ID 'question-1', got %q", state.PendingQuestion.ID)
	}

	// Clear it
	state.PendingQuestion = nil
	if m.GetIfExists("session-1").PendingQuestion != nil {
		t.Error("expected nil after clear")
	}
}

func TestSessionStateManager_Waiting(t *testing.T) {
	m := NewSessionStateManager()

	// Initially not waiting (no state exists)
	if state := m.GetIfExists("session-1"); state != nil && state.IsWaiting {
		t.Error("expected not waiting initially")
	}

	// Start waiting
	cancel := func() {}
	m.StartWaiting("session-1", cancel)

	// Now should be waiting
	state := m.GetIfExists("session-1")
	if state == nil || !state.IsWaiting {
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
	state = m.GetIfExists("session-1")
	if state != nil && state.IsWaiting {
		t.Error("expected not waiting after StopWaiting")
	}
}

func TestSessionStateManager_Merge(t *testing.T) {
	m := NewSessionStateManager()

	// Initially not merging (no state exists)
	if state := m.GetIfExists("session-1"); state != nil && state.IsMerging() {
		t.Error("expected not merging initially")
	}

	// Start merge
	ch := make(chan git.Result)
	_, cancel := context.WithCancel(context.Background())
	m.StartMerge("session-1", ch, cancel, MergeTypePR)

	// Now should be merging
	state := m.GetIfExists("session-1")
	if state == nil || !state.IsMerging() {
		t.Error("expected merging after StartMerge")
	}

	// Check merge type
	if state.MergeType != MergeTypePR {
		t.Errorf("expected MergeTypePR, got %v", state.MergeType)
	}

	// Stop merge
	m.StopMerge("session-1")
	state = m.GetIfExists("session-1")
	if state != nil && state.IsMerging() {
		t.Error("expected not merging after StopMerge")
	}
	if state != nil && state.MergeType != MergeTypeNone {
		t.Errorf("expected MergeTypeNone after StopMerge, got %v", state.MergeType)
	}
}

func TestSessionStateManager_InputText(t *testing.T) {
	m := NewSessionStateManager()

	// Initially empty (no state exists)
	if state := m.GetIfExists("session-1"); state != nil && state.InputText != "" {
		t.Error("expected empty input initially")
	}

	// Save input
	m.GetOrCreate("session-1").InputText = "Hello, world!"

	// Should be retrievable
	state := m.GetIfExists("session-1")
	if state == nil || state.InputText != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got %q", state.InputText)
	}

	// Clear input
	state.InputText = ""
	if m.GetIfExists("session-1").InputText != "" {
		t.Error("expected empty input after clear")
	}
}

func TestSessionStateManager_Streaming(t *testing.T) {
	m := NewSessionStateManager()

	// Initially empty (no state exists)
	if state := m.GetIfExists("session-1"); state != nil && state.StreamingContent != "" {
		t.Error("expected empty streaming initially")
	}

	// Save streaming
	m.GetOrCreate("session-1").StreamingContent = "First chunk"

	// Should be retrievable
	state := m.GetIfExists("session-1")
	if state == nil || state.StreamingContent != "First chunk" {
		t.Errorf("expected 'First chunk', got %q", state.StreamingContent)
	}

	// Append streaming
	state.StreamingContent += " second chunk"
	if m.GetIfExists("session-1").StreamingContent != "First chunk second chunk" {
		t.Errorf("expected 'First chunk second chunk', got %q", m.GetIfExists("session-1").StreamingContent)
	}

	// Clear streaming
	state.StreamingContent = ""
	if m.GetIfExists("session-1").StreamingContent != "" {
		t.Error("expected empty streaming after clear")
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
					m.GetOrCreate(sessionID)
				case 1:
					state := m.GetOrCreate(sessionID)
					state.PendingPermission = &mcp.PermissionRequest{ID: "perm"}
					state.PendingPermission = nil
				case 2:
					state := m.GetOrCreate(sessionID)
					state.InputText = "input"
					_ = state.InputText
				case 3:
					state := m.GetOrCreate(sessionID)
					state.StreamingContent += "chunk"
					_ = state.StreamingContent
				case 4:
					state := m.GetIfExists(sessionID)
					if state != nil {
						_ = state.IsWaiting
					}
				case 5:
					state := m.GetIfExists(sessionID)
					if state != nil {
						_ = state.IsMerging()
					}
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

func TestSessionState_HelperMethods(t *testing.T) {
	// Test HasDetectedOptions
	state := &SessionState{}
	if state.HasDetectedOptions() {
		t.Error("expected HasDetectedOptions to be false with empty slice")
	}
	state.DetectedOptions = []DetectedOption{{Number: 1}}
	if state.HasDetectedOptions() {
		t.Error("expected HasDetectedOptions to be false with single option")
	}
	state.DetectedOptions = []DetectedOption{{Number: 1}, {Number: 2}}
	if !state.HasDetectedOptions() {
		t.Error("expected HasDetectedOptions to be true with 2+ options")
	}

	// Test HasTodoList
	state = &SessionState{}
	if state.HasTodoList() {
		t.Error("expected HasTodoList to be false with nil list")
	}

	// Test IsMerging
	state = &SessionState{}
	if state.IsMerging() {
		t.Error("expected IsMerging to be false with nil channel")
	}
	state.MergeChan = make(chan git.Result)
	if !state.IsMerging() {
		t.Error("expected IsMerging to be true with channel")
	}
}

func TestSessionStateManager_GetPendingMessage(t *testing.T) {
	m := NewSessionStateManager()

	// Initially empty
	if msg := m.GetPendingMessage("session-1"); msg != "" {
		t.Error("expected empty message initially")
	}

	// Set a message
	m.GetOrCreate("session-1").PendingMessage = "test message"

	// GetPendingMessage should return and clear
	msg := m.GetPendingMessage("session-1")
	if msg != "test message" {
		t.Errorf("expected 'test message', got %q", msg)
	}

	// Should be cleared after get
	if msg2 := m.GetPendingMessage("session-1"); msg2 != "" {
		t.Errorf("expected empty after get, got %q", msg2)
	}
}

func TestSessionStateManager_GetInitialMessage(t *testing.T) {
	m := NewSessionStateManager()

	// Initially empty
	if msg := m.GetInitialMessage("session-1"); msg != "" {
		t.Error("expected empty message initially")
	}

	// Set a message
	m.GetOrCreate("session-1").InitialMessage = "initial test"

	// GetInitialMessage should return and clear
	msg := m.GetInitialMessage("session-1")
	if msg != "initial test" {
		t.Errorf("expected 'initial test', got %q", msg)
	}

	// Should be cleared after get
	if msg2 := m.GetInitialMessage("session-1"); msg2 != "" {
		t.Errorf("expected empty after get, got %q", msg2)
	}
}

func TestSessionStateManager_ReplaceToolUseMarker(t *testing.T) {
	m := NewSessionStateManager()

	// Set up streaming content with a marker
	state := m.GetOrCreate("session-1")
	state.StreamingContent = "prefix[MARKER]suffix"

	// Replace the marker
	m.ReplaceToolUseMarker("session-1", "[MARKER]", "[DONE]", 6)

	if state.StreamingContent != "prefix[DONE]suffix" {
		t.Errorf("expected 'prefix[DONE]suffix', got %q", state.StreamingContent)
	}

	// Try to replace at wrong position - should not change
	state.StreamingContent = "prefix[MARKER]suffix"
	m.ReplaceToolUseMarker("session-1", "[MARKER]", "[DONE]", 0)
	if state.StreamingContent != "prefix[MARKER]suffix" {
		t.Errorf("expected unchanged content, got %q", state.StreamingContent)
	}
}
