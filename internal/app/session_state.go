package app

import (
	"context"
	"sync"
	"time"

	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/mcp"
)

// SessionState holds all per-session state in one place.
// This consolidates what was previously 11 separate maps in the Model,
// making it easier to manage session lifecycle and avoid race conditions.
//
// Fields are accessed directly after obtaining a *SessionState via
// GetOrCreate() or GetIfExists(). For operations involving multiple
// fields or special semantics, use the dedicated methods on SessionStateManager.
type SessionState struct {
	// Permission, question, and plan approval handling
	PendingPermission   *mcp.PermissionRequest
	PendingQuestion     *mcp.QuestionRequest
	PendingPlanApproval *mcp.PlanApprovalRequest

	// Merge/PR operation state
	MergeChan   <-chan git.Result
	MergeCancel context.CancelFunc
	MergeType   MergeType // What operation is in progress

	// Claude streaming state
	StreamCancel context.CancelFunc
	WaitStart    time.Time // When the session started waiting for Claude
	IsWaiting    bool      // Whether we're waiting for Claude response

	// UI state preserved when switching sessions
	InputText        string // Saved input text
	StreamingContent string // In-progress streaming content
	ToolUsePos       int    // Position of tool use marker for replacement

	// Parallel options state
	DetectedOptions []DetectedOption // Options detected in last assistant message

	// Queued message to send when streaming completes
	PendingMessage string

	// Initial message to send when session is first selected (for issue imports)
	InitialMessage string

	// Current todo list from TodoWrite tool
	CurrentTodoList *claude.TodoList
}

// HasDetectedOptions returns true if there are at least 2 detected options.
func (s *SessionState) HasDetectedOptions() bool {
	return len(s.DetectedOptions) >= 2
}

// HasTodoList returns true if there is a non-empty todo list.
func (s *SessionState) HasTodoList() bool {
	return s.CurrentTodoList != nil && len(s.CurrentTodoList.Items) > 0
}

// IsMerging returns true if a merge operation is in progress.
func (s *SessionState) IsMerging() bool {
	return s.MergeChan != nil
}

// SessionStateManager provides thread-safe access to per-session state.
//
// Basic usage pattern:
//
//	// For writes or when state should be created:
//	state := manager.GetOrCreate(sessionID)
//	state.PendingPermission = req
//
//	// For reads when session may not exist:
//	if state := manager.GetIfExists(sessionID); state != nil {
//	    // use state.PendingPermission
//	}
//
// For operations involving multiple fields atomically (StartWaiting, StartMerge, etc.)
// or special semantics (consuming gets), use the dedicated methods.
type SessionStateManager struct {
	mu     sync.RWMutex
	states map[string]*SessionState
}

// NewSessionStateManager creates a new session state manager.
func NewSessionStateManager() *SessionStateManager {
	return &SessionStateManager{
		states: make(map[string]*SessionState),
	}
}

// GetOrCreate returns the state for a session, creating it if it doesn't exist.
// Use GetIfExists when you don't want to create state for sessions that haven't been accessed.
func (m *SessionStateManager) GetOrCreate(sessionID string) *SessionState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getOrCreate(sessionID)
}

// GetIfExists returns the state for a session if it exists, nil otherwise.
func (m *SessionStateManager) GetIfExists(sessionID string) *SessionState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.states[sessionID]
}

// Delete removes all state for a session and releases all associated resources.
func (m *SessionStateManager) Delete(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sessionID]; exists {
		// Cancel any in-progress operations
		if state.MergeCancel != nil {
			state.MergeCancel()
			state.MergeCancel = nil
		}
		if state.StreamCancel != nil {
			state.StreamCancel()
			state.StreamCancel = nil
		}

		// Clear string fields to help GC (especially for large streaming content)
		state.InputText = ""
		state.StreamingContent = ""
		state.PendingMessage = ""
		state.InitialMessage = ""

		// Clear channel reference
		state.MergeChan = nil

		// Clear other references
		state.PendingPermission = nil
		state.PendingQuestion = nil
		state.PendingPlanApproval = nil
		state.DetectedOptions = nil
		state.CurrentTodoList = nil

		delete(m.states, sessionID)
	}
}

// StartWaiting marks a session as waiting for Claude response.
// This sets WaitStart, IsWaiting, and StreamCancel atomically.
func (m *SessionStateManager) StartWaiting(sessionID string, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.getOrCreate(sessionID)
	state.WaitStart = time.Now()
	state.IsWaiting = true
	state.StreamCancel = cancel
}

// GetWaitStart returns when the session started waiting, and whether it's waiting.
func (m *SessionStateManager) GetWaitStart(sessionID string) (time.Time, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, exists := m.states[sessionID]; exists && state.IsWaiting {
		return state.WaitStart, true
	}
	return time.Time{}, false
}

// StopWaiting marks a session as no longer waiting.
// This clears IsWaiting, WaitStart, and StreamCancel atomically.
func (m *SessionStateManager) StopWaiting(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sessionID]; exists {
		state.IsWaiting = false
		state.WaitStart = time.Time{}
		state.StreamCancel = nil
	}
}

// StartMerge starts a merge operation for a session.
// This sets MergeChan, MergeCancel, and MergeType atomically.
func (m *SessionStateManager) StartMerge(sessionID string, ch <-chan git.Result, cancel context.CancelFunc, mergeType MergeType) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.getOrCreate(sessionID)
	state.MergeChan = ch
	state.MergeCancel = cancel
	state.MergeType = mergeType
}

// StopMerge clears the merge state for a session.
// This clears MergeChan, MergeCancel, and MergeType atomically.
func (m *SessionStateManager) StopMerge(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sessionID]; exists {
		state.MergeChan = nil
		state.MergeCancel = nil
		state.MergeType = MergeTypeNone
	}
}

// ReplaceToolUseMarker replaces the tool use marker in streaming content.
// The function validates that the old marker actually exists at the given position
// to prevent corruption if the streaming content has changed since the position was recorded.
func (m *SessionStateManager) ReplaceToolUseMarker(sessionID, oldMarker, newMarker string, pos int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sessionID]; exists {
		streaming := state.StreamingContent
		markerLen := len(oldMarker)

		// Validate bounds
		if pos < 0 || pos+markerLen > len(streaming) {
			return
		}

		// Validate that the old marker actually exists at this position
		// This prevents corruption if the streaming content has been modified
		if streaming[pos:pos+markerLen] != oldMarker {
			return
		}

		prefix := streaming[:pos]
		suffix := streaming[pos+markerLen:]
		state.StreamingContent = prefix + newMarker + suffix
	}
}

// GetPendingMessage returns and clears the pending message for a session.
// This is a consuming get - the message is cleared after retrieval.
// Use state.PendingMessage directly if you need to read without clearing.
func (m *SessionStateManager) GetPendingMessage(sessionID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sessionID]; exists {
		msg := state.PendingMessage
		state.PendingMessage = ""
		return msg
	}
	return ""
}

// GetInitialMessage returns and clears the initial message for a session.
// This is a consuming get - the message is cleared after retrieval.
// Use state.InitialMessage directly if you need to read without clearing.
func (m *SessionStateManager) GetInitialMessage(sessionID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sessionID]; exists {
		msg := state.InitialMessage
		state.InitialMessage = ""
		return msg
	}
	return ""
}

// getOrCreate returns existing state or creates new one. Caller must hold lock.
func (m *SessionStateManager) getOrCreate(sessionID string) *SessionState {
	if state, exists := m.states[sessionID]; exists {
		return state
	}
	state := &SessionState{ToolUsePos: -1}
	m.states[sessionID] = state
	return state
}
