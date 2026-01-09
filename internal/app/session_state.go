package app

import (
	"context"
	"sync"
	"time"

	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/mcp"
)

// SessionState holds all per-session state in one place.
// This consolidates what was previously 11 separate maps in the Model,
// making it easier to manage session lifecycle and avoid race conditions.
type SessionState struct {
	// Permission and question handling
	PendingPermission *mcp.PermissionRequest
	PendingQuestion   *mcp.QuestionRequest

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

	// Error state
	SessionInUseError bool // Whether session has "session in use" error

	// Parallel options state
	DetectedOptions []DetectedOption // Options detected in last assistant message

	// Queued message to send when streaming completes
	PendingMessage string
}

// SessionStateManager provides thread-safe access to per-session state.
// All access to session state should go through this manager.
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

// Get returns the state for a session, creating it if it doesn't exist.
func (m *SessionStateManager) Get(sessionID string) *SessionState {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sessionID]; exists {
		return state
	}

	state := &SessionState{}
	m.states[sessionID] = state
	return state
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

		// Clear channel reference
		state.MergeChan = nil

		// Clear other references
		state.PendingPermission = nil
		state.PendingQuestion = nil
		state.DetectedOptions = nil

		delete(m.states, sessionID)
	}
}

// SetPendingPermission sets the pending permission for a session.
func (m *SessionStateManager) SetPendingPermission(sessionID string, req *mcp.PermissionRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.getOrCreate(sessionID)
	state.PendingPermission = req
}

// ClearPendingPermission clears the pending permission for a session.
func (m *SessionStateManager) ClearPendingPermission(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sessionID]; exists {
		state.PendingPermission = nil
	}
}

// GetPendingPermission returns the pending permission for a session.
func (m *SessionStateManager) GetPendingPermission(sessionID string) *mcp.PermissionRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, exists := m.states[sessionID]; exists {
		return state.PendingPermission
	}
	return nil
}

// SetPendingQuestion sets the pending question for a session.
func (m *SessionStateManager) SetPendingQuestion(sessionID string, req *mcp.QuestionRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.getOrCreate(sessionID)
	state.PendingQuestion = req
}

// ClearPendingQuestion clears the pending question for a session.
func (m *SessionStateManager) ClearPendingQuestion(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sessionID]; exists {
		state.PendingQuestion = nil
	}
}

// GetPendingQuestion returns the pending question for a session.
func (m *SessionStateManager) GetPendingQuestion(sessionID string) *mcp.QuestionRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, exists := m.states[sessionID]; exists {
		return state.PendingQuestion
	}
	return nil
}

// StartWaiting marks a session as waiting for Claude response.
func (m *SessionStateManager) StartWaiting(sessionID string, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.getOrCreate(sessionID)
	state.WaitStart = time.Now()
	state.IsWaiting = true
	state.StreamCancel = cancel
}

// StopWaiting marks a session as no longer waiting.
func (m *SessionStateManager) StopWaiting(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sessionID]; exists {
		state.IsWaiting = false
		state.WaitStart = time.Time{}
		state.StreamCancel = nil
	}
}

// ClearWaitStart clears the wait start time (response has started arriving).
func (m *SessionStateManager) ClearWaitStart(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sessionID]; exists {
		state.WaitStart = time.Time{}
	}
}

// IsWaiting returns whether a session is waiting for Claude response.
func (m *SessionStateManager) IsWaiting(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, exists := m.states[sessionID]; exists {
		return state.IsWaiting
	}
	return false
}

// GetWaitStart returns when the session started waiting, or zero time if not waiting.
func (m *SessionStateManager) GetWaitStart(sessionID string) (time.Time, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, exists := m.states[sessionID]; exists && state.IsWaiting {
		return state.WaitStart, true
	}
	return time.Time{}, false
}

// GetStreamCancel returns the stream cancel function for a session.
func (m *SessionStateManager) GetStreamCancel(sessionID string) context.CancelFunc {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, exists := m.states[sessionID]; exists {
		return state.StreamCancel
	}
	return nil
}

// StartMerge starts a merge operation for a session.
func (m *SessionStateManager) StartMerge(sessionID string, ch <-chan git.Result, cancel context.CancelFunc, mergeType MergeType) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.getOrCreate(sessionID)
	state.MergeChan = ch
	state.MergeCancel = cancel
	state.MergeType = mergeType
}

// StopMerge clears the merge state for a session.
func (m *SessionStateManager) StopMerge(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sessionID]; exists {
		state.MergeChan = nil
		state.MergeCancel = nil
		state.MergeType = MergeTypeNone
	}
}

// GetMergeChan returns the merge channel for a session.
func (m *SessionStateManager) GetMergeChan(sessionID string) <-chan git.Result {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, exists := m.states[sessionID]; exists {
		return state.MergeChan
	}
	return nil
}

// GetMergeType returns the merge type for a session.
func (m *SessionStateManager) GetMergeType(sessionID string) MergeType {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, exists := m.states[sessionID]; exists {
		return state.MergeType
	}
	return MergeTypeNone
}

// IsMerging returns whether a session has a merge in progress.
func (m *SessionStateManager) IsMerging(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, exists := m.states[sessionID]; exists {
		return state.MergeChan != nil
	}
	return false
}

// SaveInput saves the input text for a session.
func (m *SessionStateManager) SaveInput(sessionID, input string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.getOrCreate(sessionID)
	state.InputText = input
}

// GetInput returns the saved input text for a session.
func (m *SessionStateManager) GetInput(sessionID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, exists := m.states[sessionID]; exists {
		return state.InputText
	}
	return ""
}

// ClearInput clears the saved input text for a session.
func (m *SessionStateManager) ClearInput(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sessionID]; exists {
		state.InputText = ""
	}
}

// SaveStreaming saves streaming content for a non-active session.
func (m *SessionStateManager) SaveStreaming(sessionID, content string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.getOrCreate(sessionID)
	state.StreamingContent = content
}

// AppendStreaming appends to streaming content for a non-active session.
func (m *SessionStateManager) AppendStreaming(sessionID, content string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.getOrCreate(sessionID)
	state.StreamingContent += content
}

// GetStreaming returns the streaming content for a session.
func (m *SessionStateManager) GetStreaming(sessionID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, exists := m.states[sessionID]; exists {
		return state.StreamingContent
	}
	return ""
}

// ClearStreaming clears the streaming content for a session.
func (m *SessionStateManager) ClearStreaming(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sessionID]; exists {
		state.StreamingContent = ""
	}
}

// SetToolUsePos sets the tool use position for a session.
func (m *SessionStateManager) SetToolUsePos(sessionID string, pos int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.getOrCreate(sessionID)
	state.ToolUsePos = pos
}

// GetToolUsePos returns the tool use position for a session.
func (m *SessionStateManager) GetToolUsePos(sessionID string) (int, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, exists := m.states[sessionID]; exists && state.ToolUsePos >= 0 {
		return state.ToolUsePos, true
	}
	return -1, false
}

// ClearToolUsePos clears the tool use position for a session.
func (m *SessionStateManager) ClearToolUsePos(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sessionID]; exists {
		state.ToolUsePos = -1
	}
}

// SetSessionInUseError sets whether a session has a "session in use" error.
func (m *SessionStateManager) SetSessionInUseError(sessionID string, hasError bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.getOrCreate(sessionID)
	state.SessionInUseError = hasError
}

// HasSessionInUseError returns whether a session has a "session in use" error.
func (m *SessionStateManager) HasSessionInUseError(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, exists := m.states[sessionID]; exists {
		return state.SessionInUseError
	}
	return false
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

// getOrCreate returns existing state or creates new one. Caller must hold lock.
func (m *SessionStateManager) getOrCreate(sessionID string) *SessionState {
	if state, exists := m.states[sessionID]; exists {
		return state
	}
	state := &SessionState{ToolUsePos: -1}
	m.states[sessionID] = state
	return state
}

// SetDetectedOptions sets the detected options for a session.
func (m *SessionStateManager) SetDetectedOptions(sessionID string, options []DetectedOption) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.getOrCreate(sessionID)
	state.DetectedOptions = options
}

// GetDetectedOptions returns the detected options for a session.
func (m *SessionStateManager) GetDetectedOptions(sessionID string) []DetectedOption {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, exists := m.states[sessionID]; exists {
		return state.DetectedOptions
	}
	return nil
}

// ClearDetectedOptions clears the detected options for a session.
func (m *SessionStateManager) ClearDetectedOptions(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sessionID]; exists {
		state.DetectedOptions = nil
	}
}

// HasDetectedOptions returns whether a session has detected options.
func (m *SessionStateManager) HasDetectedOptions(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, exists := m.states[sessionID]; exists {
		return len(state.DetectedOptions) >= 2
	}
	return false
}

// SetPendingMessage queues a message to be sent when streaming completes.
func (m *SessionStateManager) SetPendingMessage(sessionID, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := m.getOrCreate(sessionID)
	state.PendingMessage = message
}

// GetPendingMessage returns and clears the pending message for a session.
// Use PeekPendingMessage if you need to check the message without clearing it.
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

// PeekPendingMessage returns the pending message for a session without clearing it.
// Use this when you need to check or display the message without consuming it.
func (m *SessionStateManager) PeekPendingMessage(sessionID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, exists := m.states[sessionID]; exists {
		return state.PendingMessage
	}
	return ""
}

// HasPendingMessage returns whether a session has a pending message.
func (m *SessionStateManager) HasPendingMessage(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if state, exists := m.states[sessionID]; exists {
		return state.PendingMessage != ""
	}
	return false
}
