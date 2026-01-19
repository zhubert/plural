// Package app provides the main Bubble Tea model for the Plural TUI application.
//
// The app package is organized into several files for maintainability:
//   - app.go: Core types, constructor, and state management
//   - app_update.go: Main Update function and message routing
//   - app_view.go: View rendering functions
//   - app_helpers.go: Helper functions (session selection, messaging, git operations)
//   - app_mouse.go: Mouse event handling and coordinate adjustment
//   - shortcuts.go: Keyboard shortcut registry
//   - modal_handlers.go: Modal key handling and transitions
//   - msg_handlers.go: Message type handlers (Claude responses, permissions, etc.)
//   - session_manager.go: Session lifecycle management
//   - session_state.go: Thread-safe per-session state
package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/changelog"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/mcp"
	"github.com/zhubert/plural/internal/ui"
)

// =============================================================================
// Core Types
// =============================================================================

// Focus represents which panel is focused
type Focus int

const (
	FocusSidebar Focus = iota
	FocusChat
)

// AppState represents the current state of the application.
// Using an explicit state machine prevents invalid state combinations
// and makes state transitions clear and traceable.
type AppState int

const (
	StateIdle            AppState = iota // Ready for user input
	StateStreamingClaude                 // Receiving Claude response
)

// String returns a human-readable name for the state
func (s AppState) String() string {
	switch s {
	case StateIdle:
		return "Idle"
	case StateStreamingClaude:
		return "StreamingClaude"
	default:
		return "Unknown"
	}
}

// =============================================================================
// Model
// =============================================================================

// Model is the main Bubble Tea model
type Model struct {
	config  *config.Config
	version string // App version (injected at build time)
	header  *ui.Header
	footer  *ui.Footer
	sidebar *ui.Sidebar
	chat    *ui.Chat
	modal   *ui.Modal

	width  int
	height int
	focus  Focus

	activeSession *config.Session
	claudeRunner  claude.RunnerInterface // Currently active runner (convenience reference)

	// Session lifecycle management
	sessionMgr *SessionManager

	// State machine
	state AppState // Current application state

	// Window focus state for notifications
	windowFocused bool // Whether the terminal window is focused

	// Pending commit message editing state (one at a time)
	pendingCommitSession string    // Session ID waiting for commit message confirmation
	pendingCommitType    MergeType // What operation follows after commit
	pendingParentSession string    // Parent session ID for merge-to-parent operations

	// Pending conflict resolution state
	pendingConflictSessionID string // Session ID with pending conflict resolution
	pendingConflictRepoPath  string // Path to repo with conflicts
}

// =============================================================================
// Message Types
// =============================================================================

// StartupModalMsg is sent on app start to trigger welcome/changelog modals
type StartupModalMsg struct{}

// ClaudeResponseMsg is sent when Claude sends a response chunk
type ClaudeResponseMsg struct {
	SessionID string
	Chunk     claude.ResponseChunk
}

// PermissionRequestMsg is sent when Claude needs permission for an operation
type PermissionRequestMsg struct {
	SessionID string
	Request   mcp.PermissionRequest
}

// QuestionRequestMsg is sent when Claude asks a question via AskUserQuestion
type QuestionRequestMsg struct {
	SessionID string
	Request   mcp.QuestionRequest
}

// PlanApprovalRequestMsg is sent when Claude calls ExitPlanMode
type PlanApprovalRequestMsg struct {
	SessionID string
	Request   mcp.PlanApprovalRequest
}

// MergeResultMsg is sent when a merge/PR operation produces output
type MergeResultMsg struct {
	SessionID string
	Result    git.Result
}

// CommitMessageGeneratedMsg is sent when commit message generation completes
type CommitMessageGeneratedMsg struct {
	SessionID string
	Message   string
	Error     error
}

// SendPendingMessageMsg triggers sending a queued message for a session
type SendPendingMessageMsg struct {
	SessionID string
}

// GitHubIssuesFetchedMsg is sent when GitHub issues have been fetched
type GitHubIssuesFetchedMsg struct {
	RepoPath string
	Issues   []git.GitHubIssue
	Error    error
}

// ChangelogFetchedMsg is sent when changelog has been fetched from GitHub
type ChangelogFetchedMsg struct {
	Entries  []changelog.Entry
	Error    error
	ShowAll  bool // If true, show all entries; if false, filter by lastSeen version
	IsManual bool // If true, this was triggered by user shortcut (don't update lastSeen)
}

// =============================================================================
// Constructor & Lifecycle
// =============================================================================

// New creates a new app model
func New(cfg *config.Config, version string) *Model {
	// Load saved theme from config, or use default
	if savedTheme := cfg.GetTheme(); savedTheme != "" {
		ui.SetThemeByName(savedTheme)
	}

	m := &Model{
		config:        cfg,
		version:       version,
		header:        ui.NewHeader(),
		footer:        ui.NewFooter(),
		sidebar:       ui.NewSidebar(),
		chat:          ui.NewChat(),
		modal:         ui.NewModal(),
		focus:         FocusSidebar,
		sessionMgr:    NewSessionManager(cfg),
		state:         StateIdle,
		windowFocused: true, // Assume window is focused on startup
	}

	// Load sessions into sidebar
	m.sidebar.SetSessions(cfg.GetSessions())
	m.sidebar.SetFocused(true)

	return m
}

// Close gracefully shuts down all Claude sessions and releases resources.
// This should be called when the application is exiting.
func (m *Model) Close() {
	logger.Info("App: Closing and shutting down all sessions")
	m.sessionMgr.Shutdown()
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	// Trigger startup modal check (welcome or changelog)
	return func() tea.Msg {
		return StartupModalMsg{}
	}
}

// =============================================================================
// State Helpers
// =============================================================================

// IsIdle returns true if the app is ready for user input
func (m *Model) IsIdle() bool {
	return m.state == StateIdle
}

// CanSendMessage returns true if the user can send a new message
func (m *Model) CanSendMessage() bool {
	if m.claudeRunner == nil || m.activeSession == nil {
		return false
	}
	// Sessions merged to parent are locked and cannot accept new messages
	if m.activeSession.MergedToParent {
		return false
	}
	// Check if the active session is currently waiting for a response or has a merge in progress
	// Each session can operate independently
	sm := m.sessionMgr.StateManager()
	return !sm.IsWaiting(m.activeSession.ID) && !sm.IsMerging(m.activeSession.ID)
}

// setState transitions to a new state with logging
func (m *Model) setState(newState AppState) {
	if m.state != newState {
		logger.Log("App: State transition %s -> %s", m.state, newState)
		m.state = newState
	}
}

// sessionState returns the session state manager (convenience accessor)
func (m *Model) sessionState() *SessionStateManager {
	return m.sessionMgr.StateManager()
}
