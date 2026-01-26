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

// TestStreamingStartTimeSetWithWaiting verifies that StreamingStartTime
// is set when StartWaiting is called, ensuring the elapsed time display
// will work correctly even if session is switched during streaming.
func TestStreamingStartTimeSetWithWaiting(t *testing.T) {
	m := NewSessionStateManager()

	// Start waiting
	cancel := func() {}
	m.StartWaiting("session-1", cancel)
	state := m.GetIfExists("session-1")

	// Both WaitStart and StreamingStartTime should be set
	state.mu.Lock()
	waitStart := state.WaitStart
	streamingStart := state.StreamingStartTime
	state.mu.Unlock()

	if waitStart.IsZero() {
		t.Error("expected WaitStart to be set")
	}
	if streamingStart.IsZero() {
		t.Error("expected StreamingStartTime to be set")
	}
	if !waitStart.Equal(streamingStart) {
		t.Error("expected WaitStart and StreamingStartTime to be equal")
	}

	// Stop waiting should clear both
	m.StopWaiting("session-1")

	state.mu.Lock()
	waitStart = state.WaitStart
	streamingStart = state.StreamingStartTime
	state.mu.Unlock()

	if !waitStart.IsZero() {
		t.Error("expected WaitStart to be cleared after StopWaiting")
	}
	if !streamingStart.IsZero() {
		t.Error("expected StreamingStartTime to be cleared after StopWaiting")
	}
}

// TestGetWaitStartReturnsStreamingStartTime verifies that GetWaitStart returns
// StreamingStartTime, which persists even when WaitStart is cleared.
// This is important because WaitStart gets cleared when the first chunk arrives,
// but we still need the start time for elapsed time display in the UI.
func TestGetWaitStartReturnsStreamingStartTime(t *testing.T) {
	m := NewSessionStateManager()

	// Start waiting
	cancel := func() {}
	m.StartWaiting("session-1", cancel)
	state := m.GetIfExists("session-1")

	// Get the initial time from GetWaitStart
	initialTime, ok := m.GetWaitStart("session-1")
	if !ok {
		t.Fatal("expected GetWaitStart to return true")
	}
	if initialTime.IsZero() {
		t.Fatal("expected GetWaitStart to return non-zero time")
	}

	// Simulate what happens when first streaming chunk arrives:
	// WaitStart gets cleared, but StreamingStartTime should be preserved
	state.mu.Lock()
	state.WaitStart = time.Time{} // Clear WaitStart like msg_handlers.go does
	state.mu.Unlock()

	// GetWaitStart should still return the original time (from StreamingStartTime)
	afterClearTime, ok := m.GetWaitStart("session-1")
	if !ok {
		t.Error("expected GetWaitStart to still return true after WaitStart cleared")
	}
	if afterClearTime.IsZero() {
		t.Error("expected GetWaitStart to return non-zero time from StreamingStartTime")
	}
	if !afterClearTime.Equal(initialTime) {
		t.Error("expected GetWaitStart to return the same time after WaitStart cleared")
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
					state.SetPendingPermission(&mcp.PermissionRequest{ID: "perm"})
					state.SetPendingPermission(nil)
				case 2:
					state := m.GetOrCreate(sessionID)
					state.SetInputText("input")
					_ = state.GetInputText()
				case 3:
					state := m.GetOrCreate(sessionID)
					state.AppendStreamingContent("chunk")
					_ = state.GetStreamingContent()
				case 4:
					state := m.GetIfExists(sessionID)
					if state != nil {
						_ = state.GetIsWaiting()
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

func TestSessionState_ToolUseRollup(t *testing.T) {
	state := &SessionState{ToolUsePos: -1}

	// Initially no rollup
	if state.GetToolUseRollup() != nil {
		t.Error("expected nil rollup initially")
	}

	// Add tool uses with IDs
	state.AddToolUse("Read", "file1.go", "tool-1")
	state.AddToolUse("Edit", "file2.go", "tool-2")

	rollup := state.GetToolUseRollup()
	if rollup == nil {
		t.Fatal("expected rollup after adding tool uses")
	}
	if len(rollup.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(rollup.Items))
	}
	if rollup.Items[0].ToolName != "Read" || rollup.Items[0].ToolInput != "file1.go" || rollup.Items[0].ToolUseID != "tool-1" {
		t.Errorf("first item mismatch: %+v", rollup.Items[0])
	}
	if rollup.Items[1].ToolName != "Edit" || rollup.Items[1].ToolInput != "file2.go" || rollup.Items[1].ToolUseID != "tool-2" {
		t.Errorf("second item mismatch: %+v", rollup.Items[1])
	}

	// Mark tool-2 complete by ID
	state.MarkToolUseComplete("tool-2", nil)
	if !rollup.Items[1].Complete {
		t.Error("expected second item to be complete")
	}
	if rollup.Items[0].Complete {
		t.Error("expected first item to still be incomplete")
	}

	// Mark tool-1 complete by ID
	state.MarkToolUseComplete("tool-1", nil)
	if !rollup.Items[0].Complete {
		t.Error("expected first item to be complete")
	}
}

func TestSessionState_FlushToolUseRollup(t *testing.T) {
	state := &SessionState{ToolUsePos: -1}

	// Add tool uses with IDs
	state.AddToolUse("Read", "file1.go", "tool-1")
	state.AddToolUse("Edit", "file2.go", "tool-2")
	state.MarkToolUseComplete("tool-2", nil)

	// Mock GetToolIcon function
	mockGetIcon := func(tool string) string {
		switch tool {
		case "Read":
			return "Reading"
		case "Edit":
			return "Editing"
		default:
			return "Using"
		}
	}

	// Flush to streaming content
	state.FlushToolUseRollup(mockGetIcon, "⏺", "●")

	// Rollup should be cleared
	if state.GetToolUseRollup() != nil {
		t.Error("expected rollup to be nil after flush")
	}

	// Streaming content should contain both tool uses
	content := state.GetStreamingContent()
	if content == "" {
		t.Fatal("expected streaming content after flush")
	}
	if !contains(content, "file1.go") {
		t.Error("expected streaming to contain 'file1.go'")
	}
	if !contains(content, "file2.go") {
		t.Error("expected streaming to contain 'file2.go'")
	}
	// First should be in-progress marker, second should be complete
	if !contains(content, "⏺") {
		t.Error("expected in-progress marker in streaming")
	}
	if !contains(content, "●") {
		t.Error("expected complete marker in streaming")
	}
}

func TestSessionState_FlushToolUseRollupEmpty(t *testing.T) {
	state := &SessionState{ToolUsePos: -1}

	// Flush with no rollup should be a no-op
	state.FlushToolUseRollup(func(s string) string { return s }, "⏺", "●")

	if state.GetStreamingContent() != "" {
		t.Error("expected empty streaming content when flushing empty rollup")
	}
}

func TestSessionState_SubagentModel(t *testing.T) {
	state := &SessionState{ToolUsePos: -1}

	// Initially empty
	if state.GetSubagentModel() != "" {
		t.Errorf("Expected empty subagent model initially, got %q", state.GetSubagentModel())
	}

	// Set subagent model
	state.SetSubagentModel("claude-haiku-4-5-20251001")
	if state.GetSubagentModel() != "claude-haiku-4-5-20251001" {
		t.Errorf("Expected haiku model, got %q", state.GetSubagentModel())
	}

	// Clear subagent model
	state.SetSubagentModel("")
	if state.GetSubagentModel() != "" {
		t.Errorf("Expected empty subagent model after clear, got %q", state.GetSubagentModel())
	}
}

// Helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
