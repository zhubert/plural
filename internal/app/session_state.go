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
// Thread Safety:
// SessionState has an internal mutex to protect concurrent field access.
// For simple reads/writes, use the thread-safe methods (e.g., AppendStreamingContent,
// GetStreamingContent, SetStreamingContent). For operations that need to access
// multiple fields atomically, use WithLock() to hold the mutex during the operation.
//
// The SessionStateManager's mutex protects the map of sessions, while each
// SessionState's internal mutex protects its own fields.
type SessionState struct {
	mu sync.Mutex // Protects all fields below

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
	InputText          string    // Saved input text
	StreamingContent   string    // In-progress streaming content
	StreamingStartTime time.Time // When streaming started (for elapsed time display)
	ToolUsePos         int       // Position of tool use marker for replacement

	// Tool use rollup for non-active sessions
	ToolUseRollup *ToolUseRollupState // Current rollup group (nil when no tool uses yet)

	// Parallel options state
	DetectedOptions []DetectedOption // Options detected in last assistant message

	// Queued message to send when streaming completes
	PendingMessage string

	// Initial message to send when session is first selected (for issue imports)
	InitialMessage string

	// Current todo list from TodoWrite tool
	CurrentTodoList *claude.TodoList
}

// ToolUseRollupState tracks consecutive tool uses for non-active sessions
type ToolUseRollupState struct {
	Items    []ToolUseItemState // All tool uses in this group
	Expanded bool               // Whether the rollup is expanded
}

// ToolUseItemState represents a single tool use
type ToolUseItemState struct {
	ToolName   string
	ToolInput  string
	ToolUseID  string
	Complete   bool
	ResultInfo *claude.ToolResultInfo // Rich details about the result (populated on completion)
}

// HasDetectedOptions returns true if there are at least 2 detected options.
// Thread-safe.
func (s *SessionState) HasDetectedOptions() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.DetectedOptions) >= 2
}

// HasTodoList returns true if there is a non-empty todo list.
// Thread-safe.
func (s *SessionState) HasTodoList() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.CurrentTodoList != nil && len(s.CurrentTodoList.Items) > 0
}

// IsMerging returns true if a merge operation is in progress.
// Thread-safe.
func (s *SessionState) IsMerging() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.MergeChan != nil
}

// WithLock executes fn while holding the session state lock.
// Use this for operations that need to access multiple fields atomically.
// The function receives the SessionState pointer for direct field access.
//
// Example:
//
//	state.WithLock(func(s *SessionState) {
//	    s.ToolUsePos = len(s.StreamingContent)
//	    s.StreamingContent += line
//	})
func (s *SessionState) WithLock(fn func(*SessionState)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(s)
}

// --- Thread-safe accessors for StreamingContent ---

// GetStreamingContent returns the current streaming content.
// Thread-safe.
func (s *SessionState) GetStreamingContent() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.StreamingContent
}

// SetStreamingContent sets the streaming content.
// Thread-safe.
func (s *SessionState) SetStreamingContent(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.StreamingContent = content
}

// AppendStreamingContent appends to the streaming content.
// Thread-safe.
func (s *SessionState) AppendStreamingContent(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.StreamingContent += content
}

// --- Thread-safe accessors for ToolUsePos ---

// GetToolUsePos returns the current tool use position.
// Thread-safe.
func (s *SessionState) GetToolUsePos() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ToolUsePos
}

// SetToolUsePos sets the tool use position.
// Thread-safe.
func (s *SessionState) SetToolUsePos(pos int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ToolUsePos = pos
}

// --- Thread-safe accessors for ToolUseRollup ---

// GetToolUseRollup returns the current tool use rollup.
// Thread-safe.
func (s *SessionState) GetToolUseRollup() *ToolUseRollupState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ToolUseRollup
}

// SetToolUseRollup sets the tool use rollup.
// Thread-safe.
func (s *SessionState) SetToolUseRollup(rollup *ToolUseRollupState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ToolUseRollup = rollup
}

// AddToolUse adds a tool use to the rollup.
// Thread-safe.
func (s *SessionState) AddToolUse(toolName, toolInput, toolUseID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ToolUseRollup == nil {
		s.ToolUseRollup = &ToolUseRollupState{
			Items:    []ToolUseItemState{},
			Expanded: false,
		}
	}
	s.ToolUseRollup.Items = append(s.ToolUseRollup.Items, ToolUseItemState{
		ToolName:  toolName,
		ToolInput: toolInput,
		ToolUseID: toolUseID,
		Complete:  false,
	})
}

// MarkToolUseComplete marks the tool use with the given ID as complete.
// If the ID is empty or not found, falls back to marking the first incomplete tool use.
// The optional resultInfo provides rich details about the tool execution result.
// Thread-safe.
func (s *SessionState) MarkToolUseComplete(toolUseID string, resultInfo *claude.ToolResultInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ToolUseRollup == nil || len(s.ToolUseRollup.Items) == 0 {
		return
	}

	// If we have a tool use ID, find and mark the matching item
	if toolUseID != "" {
		for i := range s.ToolUseRollup.Items {
			if s.ToolUseRollup.Items[i].ToolUseID == toolUseID {
				s.ToolUseRollup.Items[i].Complete = true
				s.ToolUseRollup.Items[i].ResultInfo = resultInfo
				return
			}
		}
	}

	// Fallback: mark the first incomplete tool use as complete
	for i := range s.ToolUseRollup.Items {
		if !s.ToolUseRollup.Items[i].Complete {
			s.ToolUseRollup.Items[i].Complete = true
			s.ToolUseRollup.Items[i].ResultInfo = resultInfo
			return
		}
	}
}

// FlushToolUseRollup flushes the tool use rollup to streaming content and clears it.
// Thread-safe.
func (s *SessionState) FlushToolUseRollup(getToolIcon func(string) string, inProgressMarker, completeMarker string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ToolUseRollup == nil || len(s.ToolUseRollup.Items) == 0 {
		return
	}

	// Add newline before if there's existing content that doesn't end with newline
	if s.StreamingContent != "" && len(s.StreamingContent) > 0 && s.StreamingContent[len(s.StreamingContent)-1] != '\n' {
		s.StreamingContent += "\n"
	}

	// Render all tool uses in the rollup to streaming content
	for _, item := range s.ToolUseRollup.Items {
		marker := inProgressMarker
		if item.Complete {
			marker = completeMarker
		}
		icon := getToolIcon(item.ToolName)
		line := marker + " " + icon + "(" + item.ToolName
		if item.ToolInput != "" {
			line += ": " + item.ToolInput
		}
		line += ")"

		// Add result info for completed tool uses
		if item.Complete && item.ResultInfo != nil {
			summary := item.ResultInfo.Summary()
			if summary != "" {
				line += " → " + summary
			}
		}

		line += "\n"
		s.StreamingContent += line
	}

	// Clear the rollup
	s.ToolUseRollup = nil
	s.ToolUsePos = -1
}

// --- Thread-safe accessors for PendingPermission ---

// GetPendingPermission returns the pending permission request.
// Thread-safe.
func (s *SessionState) GetPendingPermission() *mcp.PermissionRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.PendingPermission
}

// SetPendingPermission sets the pending permission request.
// Thread-safe.
func (s *SessionState) SetPendingPermission(req *mcp.PermissionRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PendingPermission = req
}

// --- Thread-safe accessors for PendingQuestion ---

// GetPendingQuestion returns the pending question request.
// Thread-safe.
func (s *SessionState) GetPendingQuestion() *mcp.QuestionRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.PendingQuestion
}

// SetPendingQuestion sets the pending question request.
// Thread-safe.
func (s *SessionState) SetPendingQuestion(req *mcp.QuestionRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PendingQuestion = req
}

// --- Thread-safe accessors for PendingPlanApproval ---

// GetPendingPlanApproval returns the pending plan approval request.
// Thread-safe.
func (s *SessionState) GetPendingPlanApproval() *mcp.PlanApprovalRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.PendingPlanApproval
}

// SetPendingPlanApproval sets the pending plan approval request.
// Thread-safe.
func (s *SessionState) SetPendingPlanApproval(req *mcp.PlanApprovalRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PendingPlanApproval = req
}

// --- Thread-safe accessors for CurrentTodoList ---

// GetCurrentTodoList returns the current todo list.
// Thread-safe.
func (s *SessionState) GetCurrentTodoList() *claude.TodoList {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.CurrentTodoList
}

// SetCurrentTodoList sets the current todo list.
// Thread-safe.
func (s *SessionState) SetCurrentTodoList(list *claude.TodoList) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CurrentTodoList = list
}

// --- Thread-safe accessors for WaitStart ---

// GetWaitStartTime returns the wait start time.
// Thread-safe.
func (s *SessionState) GetWaitStartTime() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.WaitStart
}

// SetWaitStartTime sets the wait start time.
// Thread-safe.
func (s *SessionState) SetWaitStartTime(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.WaitStart = t
}

// --- Thread-safe accessors for IsWaiting ---

// GetIsWaiting returns whether the session is waiting.
// Thread-safe.
func (s *SessionState) GetIsWaiting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.IsWaiting
}

// --- Thread-safe accessors for PendingMessage ---

// GetPendingMsg returns the pending message (non-consuming).
// Thread-safe.
func (s *SessionState) GetPendingMsg() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.PendingMessage
}

// SetPendingMsg sets the pending message.
// Thread-safe.
func (s *SessionState) SetPendingMsg(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PendingMessage = msg
}

// --- Thread-safe accessors for InputText ---

// GetInputText returns the saved input text.
// Thread-safe.
func (s *SessionState) GetInputText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.InputText
}

// SetInputText sets the saved input text.
// Thread-safe.
func (s *SessionState) SetInputText(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.InputText = text
}

// --- Thread-safe accessors for StreamCancel ---

// GetStreamCancel returns the stream cancel function.
// Thread-safe.
func (s *SessionState) GetStreamCancel() context.CancelFunc {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.StreamCancel
}

// --- Thread-safe accessors for MergeType ---

// GetMergeType returns the current merge type.
// Thread-safe.
func (s *SessionState) GetMergeType() MergeType {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.MergeType
}

// SessionStateManager provides thread-safe access to per-session state.
//
// Basic usage pattern with thread-safe accessors:
//
//	// For simple field access, use the thread-safe methods on SessionState:
//	state := manager.GetOrCreate(sessionID)
//	state.SetPendingPermission(req)          // Thread-safe write
//	req := state.GetPendingPermission()      // Thread-safe read
//	state.AppendStreamingContent("chunk")    // Thread-safe append
//
//	// For operations involving multiple fields atomically, use WithLock:
//	state.WithLock(func(s *SessionState) {
//	    s.ToolUsePos = len(s.StreamingContent)
//	    s.StreamingContent += line
//	})
//
//	// For reads when session may not exist:
//	if state := manager.GetIfExists(sessionID); state != nil {
//	    req := state.GetPendingPermission()
//	}
//
// For operations that span multiple fields with special semantics (StartWaiting,
// StartMerge, consuming gets like GetPendingMessage), use the dedicated methods
// on SessionStateManager which handle locking internally.
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
		// Lock the state to safely access and clear fields
		state.mu.Lock()

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
		state.ToolUseRollup = nil

		state.mu.Unlock()
		delete(m.states, sessionID)
	}
}

// StartWaiting marks a session as waiting for Claude response.
// This sets WaitStart, IsWaiting, StreamCancel, and StreamingStartTime atomically.
func (m *SessionStateManager) StartWaiting(sessionID string, cancel context.CancelFunc) {
	m.mu.Lock()
	state := m.getOrCreate(sessionID)
	m.mu.Unlock()

	state.mu.Lock()
	defer state.mu.Unlock()
	now := time.Now()
	state.WaitStart = now
	state.StreamingStartTime = now // Also set streaming start time for UI display
	state.IsWaiting = true
	state.StreamCancel = cancel
}

// GetWaitStart returns when the session started streaming, and whether it's waiting.
// Returns StreamingStartTime (not WaitStart) because WaitStart gets cleared when
// the first chunk arrives, but we still need the start time for elapsed display.
func (m *SessionStateManager) GetWaitStart(sessionID string) (time.Time, bool) {
	m.mu.RLock()
	state, exists := m.states[sessionID]
	m.mu.RUnlock()

	if !exists {
		return time.Time{}, false
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if state.IsWaiting {
		return state.StreamingStartTime, true
	}
	return time.Time{}, false
}

// StopWaiting marks a session as no longer waiting.
// This clears IsWaiting, WaitStart, StreamCancel, and StreamingStartTime atomically.
func (m *SessionStateManager) StopWaiting(sessionID string) {
	m.mu.RLock()
	state, exists := m.states[sessionID]
	m.mu.RUnlock()

	if exists {
		state.mu.Lock()
		defer state.mu.Unlock()
		state.IsWaiting = false
		state.WaitStart = time.Time{}
		state.StreamingStartTime = time.Time{}
		state.StreamCancel = nil
	}
}

// StartMerge starts a merge operation for a session.
// This sets MergeChan, MergeCancel, and MergeType atomically.
func (m *SessionStateManager) StartMerge(sessionID string, ch <-chan git.Result, cancel context.CancelFunc, mergeType MergeType) {
	m.mu.Lock()
	state := m.getOrCreate(sessionID)
	m.mu.Unlock()

	state.mu.Lock()
	defer state.mu.Unlock()
	state.MergeChan = ch
	state.MergeCancel = cancel
	state.MergeType = mergeType
}

// StopMerge clears the merge state for a session.
// This clears MergeChan, MergeCancel, and MergeType atomically.
func (m *SessionStateManager) StopMerge(sessionID string) {
	m.mu.RLock()
	state, exists := m.states[sessionID]
	m.mu.RUnlock()

	if exists {
		state.mu.Lock()
		defer state.mu.Unlock()
		state.MergeChan = nil
		state.MergeCancel = nil
		state.MergeType = MergeTypeNone
	}
}

// ReplaceToolUseMarker replaces the tool use marker in streaming content.
// The function validates that the old marker actually exists at the given position
// to prevent corruption if the streaming content has changed since the position was recorded.
func (m *SessionStateManager) ReplaceToolUseMarker(sessionID, oldMarker, newMarker string, pos int) {
	m.mu.RLock()
	state, exists := m.states[sessionID]
	m.mu.RUnlock()

	if exists {
		state.mu.Lock()
		defer state.mu.Unlock()

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
// Use state.GetPendingMsg() if you need to read without clearing.
func (m *SessionStateManager) GetPendingMessage(sessionID string) string {
	m.mu.RLock()
	state, exists := m.states[sessionID]
	m.mu.RUnlock()

	if exists {
		state.mu.Lock()
		defer state.mu.Unlock()
		msg := state.PendingMessage
		state.PendingMessage = ""
		return msg
	}
	return ""
}

// GetInitialMessage returns and clears the initial message for a session.
// This is a consuming get - the message is cleared after retrieval.
// Use state.InitialMessage directly (with WithLock) if you need to read without clearing.
func (m *SessionStateManager) GetInitialMessage(sessionID string) string {
	m.mu.RLock()
	state, exists := m.states[sessionID]
	m.mu.RUnlock()

	if exists {
		state.mu.Lock()
		defer state.mu.Unlock()
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
