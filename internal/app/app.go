package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/mcp"
	"github.com/zhubert/plural/internal/session"
	"github.com/zhubert/plural/internal/ui"
)

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

// Model is the main Bubble Tea model
type Model struct {
	config  *config.Config
	header  *ui.Header
	footer  *ui.Footer
	sidebar *ui.Sidebar
	chat    *ui.Chat
	modal   *ui.Modal

	width  int
	height int
	focus  Focus

	activeSession *config.Session
	claudeRunner  *claude.Runner
	claudeRunners map[string]*claude.Runner // Cache runners by session ID

	// State machine
	state AppState // Current application state

	// Per-session pending permissions (sessionID -> request)
	pendingPermissions map[string]*mcp.PermissionRequest

	// Per-session merge/PR operation state
	sessionMergeChans   map[string]<-chan git.Result
	sessionMergeCancels map[string]context.CancelFunc

	// Per-session Claude streaming cancel functions
	sessionStreamCancels map[string]context.CancelFunc

	// Per-session wait tracking for timer
	sessionWaitStart map[string]time.Time // When each session started waiting

	// Per-session input text (saved when switching sessions)
	sessionInputs map[string]string

	// Per-session streaming content (for non-active sessions)
	sessionStreaming map[string]string

	// Per-session last tool use position (for non-active sessions)
	sessionToolUsePos map[string]int
}

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

// MergeResultMsg is sent when a merge/PR operation produces output
type MergeResultMsg struct {
	SessionID string
	Result    git.Result
}

// New creates a new app model
func New(cfg *config.Config) *Model {
	m := &Model{
		config:              cfg,
		header:              ui.NewHeader(),
		footer:              ui.NewFooter(),
		sidebar:             ui.NewSidebar(),
		chat:                ui.NewChat(),
		modal:               ui.NewModal(),
		focus:               FocusSidebar,
		claudeRunners:       make(map[string]*claude.Runner),
		sessionWaitStart:    make(map[string]time.Time),
		pendingPermissions:  make(map[string]*mcp.PermissionRequest),
		sessionInputs:       make(map[string]string),
		sessionMergeChans:    make(map[string]<-chan git.Result),
		sessionMergeCancels:  make(map[string]context.CancelFunc),
		sessionStreamCancels: make(map[string]context.CancelFunc),
		sessionStreaming:     make(map[string]string),
		sessionToolUsePos:    make(map[string]int),
		state:               StateIdle,
	}

	// Load sessions into sidebar
	m.sidebar.SetSessions(cfg.GetSessions())
	m.sidebar.SetFocused(true)

	return m
}

// State helper methods

// IsIdle returns true if the app is ready for user input
func (m *Model) IsIdle() bool {
	return m.state == StateIdle
}

// CanSendMessage returns true if the user can send a new message
func (m *Model) CanSendMessage() bool {
	if m.claudeRunner == nil || m.activeSession == nil {
		return false
	}
	// Check if the active session is currently waiting for a response
	_, isWaiting := m.sessionWaitStart[m.activeSession.ID]
	// Check if the active session has a merge in progress
	_, isMerging := m.sessionMergeChans[m.activeSession.ID]
	// Each session can operate independently
	return !isWaiting && !isMerging
}

// setState transitions to a new state with logging
func (m *Model) setState(newState AppState) {
	if m.state != newState {
		logger.Log("App: State transition %s -> %s", m.state, newState)
		m.state = newState
	}
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateSizes()

	case tea.KeyPressMsg:
		// Handle modal first if visible
		if m.modal.IsVisible() {
			return m.handleModalKey(msg)
		}

		// Handle permission response when chat is focused and has pending permission
		if m.focus == FocusChat && m.activeSession != nil {
			if req, exists := m.pendingPermissions[m.activeSession.ID]; exists {
				switch msg.String() {
				case "y", "Y", "n", "N", "a", "A":
					return m.handlePermissionResponse(msg.String(), m.activeSession.ID, req)
				}
			}
		}

		// Handle Escape to interrupt streaming
		if msg.String() == "esc" && m.activeSession != nil {
			if cancel, exists := m.sessionStreamCancels[m.activeSession.ID]; exists {
				logger.Log("App: Interrupting streaming for session %s", m.activeSession.ID)
				cancel()
				delete(m.sessionStreamCancels, m.activeSession.ID)
				delete(m.sessionWaitStart, m.activeSession.ID)
				m.sidebar.SetStreaming(m.activeSession.ID, false)
				m.chat.SetWaiting(false)
				m.chat.ClearToolStatus()
				// Save partial response to runner before finishing
				if content := m.chat.GetStreaming(); content != "" {
					m.claudeRunner.AddAssistantMessage(content + "\n[Interrupted]")
					m.saveRunnerMessages(m.activeSession.ID, m.claudeRunner)
				}
				m.chat.AppendStreaming("\n[Interrupted]\n")
				m.chat.FinishStreaming()
				// Check if any sessions are still streaming
				if !m.hasAnyStreamingSessions() {
					m.setState(StateIdle)
				}
				return m, nil
			}
		}

		// Global keys
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			// Only quit on 'q' when sidebar is focused (so user can type 'q' in chat)
			if !m.chat.IsFocused() {
				return m, tea.Quit
			}
		case "tab":
			m.toggleFocus()
		case "n":
			if !m.chat.IsFocused() {
				m.modal.SetRepoOptions(m.config.GetRepos())
				m.modal.Show(ui.ModalNewSession)
			}
		case "r":
			if !m.chat.IsFocused() {
				// Check if current directory is a git repo and not already added
				currentRepo := session.GetCurrentDirGitRoot()
				if currentRepo != "" {
					// Check if already added
					for _, repo := range m.config.GetRepos() {
						if repo == currentRepo {
							currentRepo = "" // Already added, don't suggest
							break
						}
					}
				}
				m.modal.SetSuggestedRepo(currentRepo)
				m.modal.Show(ui.ModalAddRepo)
			}
		case "d":
			if !m.chat.IsFocused() && m.sidebar.SelectedSession() != nil {
				m.modal.Show(ui.ModalConfirmDelete)
			}
		case "v":
			if !m.chat.IsFocused() && m.sidebar.SelectedSession() != nil {
				sess := m.sidebar.SelectedSession()
				// Select the session first so we can display in its chat
				if m.activeSession == nil || m.activeSession.ID != sess.ID {
					m.selectSession(sess)
				}
				// Get worktree status and display it
				status, err := git.GetWorktreeStatus(sess.WorkTree)
				if err != nil {
					m.chat.AppendStreaming(fmt.Sprintf("[Error getting status: %v]\n", err))
					m.chat.FinishStreaming()
				} else if !status.HasChanges {
					m.chat.AppendStreaming("No uncommitted changes in this session.\n")
					m.chat.FinishStreaming()
				} else {
					var sb strings.Builder
					sb.WriteString(fmt.Sprintf("📝 Uncommitted changes (%s):\n\n", status.Summary))
					for _, file := range status.Files {
						sb.WriteString(fmt.Sprintf("  • %s\n", file))
					}
					if status.Diff != "" {
						sb.WriteString("\n--- Diff ---\n")
						// Truncate diff if too long
						diff := status.Diff
						if len(diff) > 3000 {
							diff = diff[:3000] + "\n... (truncated)"
						}
						sb.WriteString(diff)
					}
					sb.WriteString("\n")
					m.chat.AppendStreaming(sb.String())
					m.chat.FinishStreaming()
				}
				return m, nil
			}
		case "m":
			if !m.chat.IsFocused() && m.sidebar.SelectedSession() != nil {
				sess := m.sidebar.SelectedSession()
				hasRemote := git.HasRemoteOrigin(sess.RepoPath)
				// Get changes summary to display in modal
				var changesSummary string
				if status, err := git.GetWorktreeStatus(sess.WorkTree); err == nil && status.HasChanges {
					changesSummary = status.Summary
					// Add file list if not too many files
					if len(status.Files) <= 5 {
						changesSummary += ": " + strings.Join(status.Files, ", ")
					}
				}
				m.modal.SetMergeOptions(hasRemote, changesSummary)
				m.modal.Show(ui.ModalMerge)
			}
		case "s":
			if !m.chat.IsFocused() {
				m.showMCPServersModal()
			}
		case "enter":
			if m.focus == FocusSidebar {
				// Select session
				if sess := m.sidebar.SelectedSession(); sess != nil {
					m.selectSession(sess)
				}
			} else if m.focus == FocusChat && m.CanSendMessage() {
				// Send message
				return m.sendMessage()
			}
		}

	case ClaudeResponseMsg:
		// Get the runner for this session
		runner, exists := m.claudeRunners[msg.SessionID]
		if !exists {
			logger.Log("App: Received response for unknown session %s", msg.SessionID)
			return m, nil
		}

		isActiveSession := m.activeSession != nil && m.activeSession.ID == msg.SessionID

		if msg.Chunk.Error != nil {
			logger.Log("App: Error in session %s: %v", msg.SessionID, msg.Chunk.Error)
			m.sidebar.SetStreaming(msg.SessionID, false)
			delete(m.sessionWaitStart, msg.SessionID)
			delete(m.sessionStreamCancels, msg.SessionID)
			if isActiveSession {
				m.chat.SetWaiting(false)
				m.chat.AppendStreaming("\n[Error: " + msg.Chunk.Error.Error() + "]")
			} else {
				// Store error for non-active session
				m.sessionStreaming[msg.SessionID] += "\n[Error: " + msg.Chunk.Error.Error() + "]"
			}
			// Check if any sessions are still streaming
			if !m.hasAnyStreamingSessions() {
				m.setState(StateIdle)
			}
		} else if msg.Chunk.Done {
			logger.Log("App: Session %s completed streaming", msg.SessionID)
			m.sidebar.SetStreaming(msg.SessionID, false)
			delete(m.sessionWaitStart, msg.SessionID)
			delete(m.sessionStreamCancels, msg.SessionID)
			if isActiveSession {
				m.chat.SetWaiting(false)
				m.chat.FinishStreaming()
			} else {
				// For non-active session, just clear our saved streaming content
				// The runner already adds the assistant message when streaming completes (claude.go)
				delete(m.sessionStreaming, msg.SessionID)
			}
			// Mark session as started and save messages
			sess := m.getSessionByID(msg.SessionID)
			if sess != nil && runner.SessionStarted() {
				if !sess.Started {
					m.config.MarkSessionStarted(sess.ID)
					sess.Started = true
					m.config.Save()
				}
				// Save messages for this session
				m.saveRunnerMessages(msg.SessionID, runner)
			}
			// Check if any sessions are still streaming
			if !m.hasAnyStreamingSessions() {
				m.setState(StateIdle)
			}
		} else {
			// Streaming content - clear wait time since response has started
			delete(m.sessionWaitStart, msg.SessionID)
			if isActiveSession {
				m.chat.SetWaiting(false)
				// Handle different chunk types
				switch msg.Chunk.Type {
				case claude.ChunkTypeToolUse:
					// Append tool use to streaming content so it persists in history
					m.chat.AppendToolUse(msg.Chunk.ToolName, msg.Chunk.ToolInput)
					m.chat.SetToolStatus(msg.Chunk.ToolName, msg.Chunk.ToolInput)
				case claude.ChunkTypeToolResult:
					// Tool completed, mark the tool use line as complete and clear status
					m.chat.MarkLastToolUseComplete()
					m.chat.ClearToolStatus()
				case claude.ChunkTypeText:
					// Don't clear tool status on text - let it persist until tool_result
					m.chat.AppendStreaming(msg.Chunk.Content)
				default:
					// For backwards compatibility, treat unknown types as text
					if msg.Chunk.Content != "" {
						m.chat.AppendStreaming(msg.Chunk.Content)
					}
				}
			} else {
				// Store streaming content for non-active session
				switch msg.Chunk.Type {
				case claude.ChunkTypeToolUse:
					// Format tool use for non-active session
					icon := ui.GetToolIcon(msg.Chunk.ToolName)
					line := ui.ToolUseInProgress + " " + icon + "(" + msg.Chunk.ToolName
					if msg.Chunk.ToolInput != "" {
						line += ": " + msg.Chunk.ToolInput
					}
					line += ")\n"
					if existing := m.sessionStreaming[msg.SessionID]; existing != "" && !strings.HasSuffix(existing, "\n") {
						m.sessionStreaming[msg.SessionID] += "\n"
					}
					// Track position where the marker starts
					m.sessionToolUsePos[msg.SessionID] = len(m.sessionStreaming[msg.SessionID])
					m.sessionStreaming[msg.SessionID] += line
				case claude.ChunkTypeToolResult:
					// Mark the tool use as complete for non-active session
					if pos, exists := m.sessionToolUsePos[msg.SessionID]; exists && pos >= 0 {
						streaming := m.sessionStreaming[msg.SessionID]
						markerLen := len(ui.ToolUseInProgress)
						if pos+markerLen <= len(streaming) {
							prefix := streaming[:pos]
							suffix := streaming[pos+markerLen:]
							m.sessionStreaming[msg.SessionID] = prefix + ui.ToolUseComplete + suffix
						}
						delete(m.sessionToolUsePos, msg.SessionID)
					}
				case claude.ChunkTypeText:
					m.sessionStreaming[msg.SessionID] += msg.Chunk.Content
				default:
					if msg.Chunk.Content != "" {
						m.sessionStreaming[msg.SessionID] += msg.Chunk.Content
					}
				}
			}
			// Continue listening for more chunks from this session
			return m, tea.Batch(
				m.listenForSessionResponse(msg.SessionID, runner.GetResponseChan()),
				m.listenForSessionPermission(msg.SessionID, runner),
			)
		}

	case PermissionRequestMsg:
		// Get the runner for this session
		runner, exists := m.claudeRunners[msg.SessionID]
		if !exists {
			logger.Log("App: Received permission request for unknown session %s", msg.SessionID)
			return m, nil
		}

		// Store permission request for this session (inline, not modal)
		logger.Log("App: Permission request for session %s: tool=%s", msg.SessionID, msg.Request.Tool)
		m.pendingPermissions[msg.SessionID] = &msg.Request
		m.sidebar.SetPendingPermission(msg.SessionID, true)

		// If this is the active session, show permission in chat
		if m.activeSession != nil && m.activeSession.ID == msg.SessionID {
			m.chat.SetPendingPermission(msg.Request.Tool, msg.Request.Description)
		}

		// Continue listening for more permission requests and responses
		return m, tea.Batch(
			m.listenForSessionResponse(msg.SessionID, runner.GetResponseChan()),
			m.listenForSessionPermission(msg.SessionID, runner),
		)

	case MergeResultMsg:
		isActiveSession := m.activeSession != nil && m.activeSession.ID == msg.SessionID
		if msg.Result.Error != nil {
			if isActiveSession {
				m.chat.AppendStreaming("\n[Error: " + msg.Result.Error.Error() + "]\n")
			} else {
				m.sessionStreaming[msg.SessionID] += "\n[Error: " + msg.Result.Error.Error() + "]\n"
			}
			// Clean up merge state for this session
			delete(m.sessionMergeChans, msg.SessionID)
			delete(m.sessionMergeCancels, msg.SessionID)
		} else if msg.Result.Done {
			if isActiveSession {
				m.chat.FinishStreaming()
			} else {
				// Store completed merge output as a message for when user switches back
				if content := m.sessionStreaming[msg.SessionID]; content != "" {
					if runner, exists := m.claudeRunners[msg.SessionID]; exists {
						runner.AddAssistantMessage(content)
						m.saveRunnerMessages(msg.SessionID, runner)
					}
					delete(m.sessionStreaming, msg.SessionID)
				}
			}
			// Clean up merge state for this session
			delete(m.sessionMergeChans, msg.SessionID)
			delete(m.sessionMergeCancels, msg.SessionID)
		} else {
			if isActiveSession {
				m.chat.AppendStreaming(msg.Result.Output)
			} else {
				m.sessionStreaming[msg.SessionID] += msg.Result.Output
			}
			return m, m.listenForMergeResult(msg.SessionID)
		}
	}

	// Update modal
	if m.modal.IsVisible() {
		modal, cmd := m.modal.Update(msg)
		m.modal = modal
		cmds = append(cmds, cmd)
	}

	// Handle tick messages - both panels need these regardless of focus
	switch msg.(type) {
	case ui.SidebarTickMsg:
		sidebar, cmd := m.sidebar.Update(msg)
		m.sidebar = sidebar
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	case ui.StopwatchTickMsg:
		chat, cmd := m.chat.Update(msg)
		m.chat = chat
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	// Route scroll keys and mouse wheel to chat panel even when sidebar is focused
	// This allows scrolling content (e.g., after 'v' to view changes)
	// Note: up/down/j/k are reserved for sidebar navigation
	if m.focus == FocusSidebar && m.activeSession != nil {
		if keyMsg, isKey := msg.(tea.KeyPressMsg); isKey {
			switch keyMsg.String() {
			case "pgup", "pgdown", "page up", "page down", "ctrl+u", "ctrl+d", "home", "end":
				chat, cmd := m.chat.Update(msg)
				m.chat = chat
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
		}
		// Route mouse wheel events to chat panel for scrolling
		if mouseMsg, isMouse := msg.(tea.MouseWheelMsg); isMouse {
			// Check if mouse is in chat panel area (right side of screen)
			if mouseMsg.X > m.sidebar.Width() {
				chat, cmd := m.chat.Update(msg)
				m.chat = chat
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
		}
	}

	// Update focused panel for other messages
	if m.focus == FocusSidebar {
		sidebar, cmd := m.sidebar.Update(msg)
		m.sidebar = sidebar
		cmds = append(cmds, cmd)
	} else {
		chat, cmd := m.chat.Update(msg)
		m.chat = chat
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) handleModalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch m.modal.Type {
	case ui.ModalAddRepo:
		switch key {
		case "esc":
			m.modal.Hide()
			return m, nil
		case "enter":
			path := m.modal.GetAddRepoPath()
			if path == "" {
				m.modal.SetError("Please enter a path")
				return m, nil
			}
			if err := session.ValidateRepo(path); err != nil {
				m.modal.SetError(err.Error())
				return m, nil
			}
			if !m.config.AddRepo(path) {
				m.modal.SetError("Repository already added")
				return m, nil
			}
			if err := m.config.Save(); err != nil {
				m.modal.SetError("Failed to save: " + err.Error())
				return m, nil
			}
			m.modal.Hide()
			return m, nil
		}

	case ui.ModalNewSession:
		switch key {
		case "esc":
			m.modal.Hide()
			return m, nil
		case "enter":
			repoPath := m.modal.GetSelectedRepo()
			if repoPath == "" {
				return m, nil
			}
			branchName := m.modal.GetBranchName()
			// Validate branch name
			if err := session.ValidateBranchName(branchName); err != nil {
				m.modal.SetError(err.Error())
				return m, nil
			}
			// Check if branch already exists
			if branchName != "" && session.BranchExists(repoPath, branchName) {
				m.modal.SetError("Branch already exists: " + branchName)
				return m, nil
			}
			logger.Log("App: Creating new session for repo=%s, branch=%q", repoPath, branchName)
			sess, err := session.Create(repoPath, branchName)
			if err != nil {
				logger.Log("App: Failed to create session: %v", err)
				m.modal.SetError(err.Error())
				return m, nil
			}
			logger.Log("App: Session created: id=%s, name=%s", sess.ID, sess.Name)
			m.config.AddSession(*sess)
			if err := m.config.Save(); err != nil {
				logger.Log("App: Failed to save config: %v", err)
				m.modal.SetError("Failed to save: " + err.Error())
				return m, nil
			}
			m.sidebar.SetSessions(m.config.GetSessions())
			m.sidebar.SelectSession(sess.ID)
			m.selectSession(sess)
			m.modal.Hide()
			return m, nil
		}

	case ui.ModalConfirmDelete:
		switch key {
		case "esc":
			m.modal.Hide()
			return m, nil
		case "enter":
			if sess := m.sidebar.SelectedSession(); sess != nil {
				deleteWorktree := m.modal.ShouldDeleteWorktree()
				logger.Log("App: Deleting session: id=%s, name=%s, deleteWorktree=%v", sess.ID, sess.Name, deleteWorktree)

				// Delete worktree if requested
				if deleteWorktree {
					if err := session.Delete(sess); err != nil {
						logger.Log("App: Failed to delete worktree: %v", err)
						// Continue with session removal even if worktree deletion fails
					}
				}

				m.config.RemoveSession(sess.ID)
				m.config.Save()
				config.DeleteSessionMessages(sess.ID)
				m.sidebar.SetSessions(m.config.GetSessions())
				if runner, exists := m.claudeRunners[sess.ID]; exists {
					logger.Log("App: Stopping runner for deleted session %s", sess.ID)
					runner.Stop()
					delete(m.claudeRunners, sess.ID)
				}
				// Cancel any in-progress merge operation
				if cancel, exists := m.sessionMergeCancels[sess.ID]; exists {
					cancel()
					delete(m.sessionMergeCancels, sess.ID)
					delete(m.sessionMergeChans, sess.ID)
				}
				// Cancel any in-progress streaming
				if cancel, exists := m.sessionStreamCancels[sess.ID]; exists {
					cancel()
					delete(m.sessionStreamCancels, sess.ID)
				}
				// Clean up per-session state maps
				delete(m.pendingPermissions, sess.ID)
				delete(m.sessionWaitStart, sess.ID)
				delete(m.sessionInputs, sess.ID)
				delete(m.sessionStreaming, sess.ID)
				m.sidebar.SetPendingPermission(sess.ID, false)
				if m.activeSession != nil && m.activeSession.ID == sess.ID {
					m.activeSession = nil
					m.claudeRunner = nil
					m.chat.ClearSession()
					m.header.SetSessionName("")
				}
				logger.Log("App: Session deleted successfully: %s", sess.ID)
			}
			m.modal.Hide()
			return m, nil
		case "up", "down", "j", "k":
			// Forward navigation keys to modal for option selection
			modal, cmd := m.modal.Update(msg)
			m.modal = modal
			return m, cmd
		}
		return m, nil

	case ui.ModalMerge:
		switch key {
		case "esc":
			m.modal.Hide()
			return m, nil
		case "enter":
			option := m.modal.GetSelectedMergeOption()
			sess := m.sidebar.SelectedSession()
			if option == "" || sess == nil {
				return m, nil
			}
			// Check if this session already has a merge in progress
			if _, exists := m.sessionMergeChans[sess.ID]; exists {
				logger.Log("App: Merge already in progress for session %s", sess.ID)
				return m, nil
			}
			logger.Log("App: Starting merge operation: option=%q, session=%s, branch=%s, worktree=%s", option, sess.ID, sess.Branch, sess.WorkTree)
			m.modal.Hide()
			if m.activeSession == nil || m.activeSession.ID != sess.ID {
				m.selectSession(sess)
			}
			// Create per-session merge context
			ctx, cancel := context.WithCancel(context.Background())
			m.sessionMergeCancels[sess.ID] = cancel
			if option == "Create PR" {
				logger.Log("App: Creating PR for branch %s", sess.Branch)
				m.chat.AppendStreaming("Creating PR for " + sess.Branch + "...\n\n")
				m.sessionMergeChans[sess.ID] = git.CreatePR(ctx, sess.RepoPath, sess.WorkTree, sess.Branch)
			} else {
				logger.Log("App: Merging branch %s to main", sess.Branch)
				m.chat.AppendStreaming("Merging " + sess.Branch + " to main...\n\n")
				m.sessionMergeChans[sess.ID] = git.MergeToMain(ctx, sess.RepoPath, sess.WorkTree, sess.Branch)
			}
			return m, m.listenForMergeResult(sess.ID)
		}

	case ui.ModalMCPServers:
		switch key {
		case "esc":
			m.modal.Hide()
			return m, nil
		case "a":
			m.modal.ShowAddMCPServer(m.config.GetRepos())
			return m, nil
		case "d":
			if server := m.modal.GetSelectedMCPServer(); server != nil {
				if server.IsGlobal {
					m.config.RemoveGlobalMCPServer(server.Name)
				} else {
					m.config.RemoveRepoMCPServer(server.RepoPath, server.Name)
				}
				m.config.Save()
				m.showMCPServersModal() // Refresh the modal
			}
			return m, nil
		}

	case ui.ModalAddMCPServer:
		switch key {
		case "esc":
			m.showMCPServersModal() // Go back to list
			return m, nil
		case "enter":
			name, command, args, repoPath, isGlobal := m.modal.GetNewMCPServer()
			if name == "" || command == "" {
				return m, nil
			}
			// Parse args (space-separated)
			var argsList []string
			if args != "" {
				argsList = strings.Fields(args)
			}
			server := config.MCPServer{
				Name:    name,
				Command: command,
				Args:    argsList,
			}
			if isGlobal {
				m.config.AddGlobalMCPServer(server)
			} else {
				m.config.AddRepoMCPServer(repoPath, server)
			}
			m.config.Save()
			m.modal.Hide()
			return m, nil
		case " ":
			// Space toggles scope when on scope selector
			m.modal.ToggleMCPScope()
			return m, nil
		}
	}

	// Update modal input (for text-based modals like AddRepo)
	modal, cmd := m.modal.Update(msg)
	m.modal = modal
	return m, cmd
}

func (m *Model) toggleFocus() {
	if m.focus == FocusSidebar {
		// Only allow switching to chat if there's an active session
		if m.activeSession == nil {
			return
		}
		m.focus = FocusChat
		m.sidebar.SetFocused(false)
		m.chat.SetFocused(true)
	} else {
		m.focus = FocusSidebar
		m.sidebar.SetFocused(true)
		m.chat.SetFocused(false)
	}
}

func (m *Model) showMCPServersModal() {
	// Build global servers display list
	var globalServers []ui.MCPServerDisplay
	for _, s := range m.config.GetGlobalMCPServers() {
		globalServers = append(globalServers, ui.MCPServerDisplay{
			Name:     s.Name,
			Command:  s.Command,
			Args:     strings.Join(s.Args, " "),
			IsGlobal: true,
		})
	}

	// Build per-repo servers display map
	repos := m.config.GetRepos()
	perRepoServers := make(map[string][]ui.MCPServerDisplay)
	for _, repo := range repos {
		repoServers := m.config.GetRepoMCPServers(repo)
		if len(repoServers) > 0 {
			var displays []ui.MCPServerDisplay
			for _, s := range repoServers {
				displays = append(displays, ui.MCPServerDisplay{
					Name:     s.Name,
					Command:  s.Command,
					Args:     strings.Join(s.Args, " "),
					IsGlobal: false,
					RepoPath: repo,
				})
			}
			perRepoServers[repo] = displays
		}
	}

	m.modal.ShowMCPServers(globalServers, perRepoServers, repos)
}

func (m *Model) selectSession(sess *config.Session) {
	if sess == nil {
		return
	}

	// Save current session's state before switching
	if m.activeSession != nil {
		currentInput := m.chat.GetInput()
		m.sessionInputs[m.activeSession.ID] = currentInput
		logger.Log("App: Saved input for session %s: %q", m.activeSession.ID, currentInput)

		// Save current streaming content if any
		if streamingContent := m.chat.GetStreaming(); streamingContent != "" {
			m.sessionStreaming[m.activeSession.ID] = streamingContent
			logger.Log("App: Saved streaming content for session %s", m.activeSession.ID)
		}
	}

	logger.Log("App: Selecting session: id=%s, name=%s", sess.ID, sess.Name)
	m.activeSession = sess

	// Reuse existing runner or create new one
	if runner, exists := m.claudeRunners[sess.ID]; exists {
		logger.Log("App: Reusing existing runner for session %s", sess.ID)
		m.claudeRunner = runner
	} else {
		logger.Log("App: Creating new runner for session %s", sess.ID)
		// Load saved messages from disk
		savedMsgs, err := config.LoadSessionMessages(sess.ID)
		if err != nil {
			// Log the error but continue with empty messages
			// This allows the session to work even if message history is corrupted
			logger.Log("App: Warning - failed to load session messages for %s: %v", sess.ID, err)
			savedMsgs = []config.Message{}
		} else {
			logger.Log("App: Loaded %d saved messages for session %s", len(savedMsgs), sess.ID)
		}
		var initialMsgs []claude.Message
		for _, msg := range savedMsgs {
			initialMsgs = append(initialMsgs, claude.Message{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
		m.claudeRunner = claude.New(sess.ID, sess.WorkTree, sess.Started, initialMsgs)
		m.claudeRunners[sess.ID] = m.claudeRunner
	}

	// Load allowed tools from config (global + per-repo)
	allowedTools := m.config.GetAllowedToolsForRepo(sess.RepoPath)
	if len(allowedTools) > 0 {
		logger.Log("App: Loaded %d allowed tools for repo %s", len(allowedTools), sess.RepoPath)
		m.claudeRunner.SetAllowedTools(allowedTools)
	}

	// Load MCP servers for this session's repo
	mcpServers := m.config.GetMCPServersForRepo(sess.RepoPath)
	if len(mcpServers) > 0 {
		logger.Log("App: Loaded %d MCP servers for session %s (repo: %s)", len(mcpServers), sess.ID, sess.RepoPath)
		var servers []claude.MCPServer
		for _, s := range mcpServers {
			servers = append(servers, claude.MCPServer{
				Name:    s.Name,
				Command: s.Command,
				Args:    s.Args,
			})
		}
		m.claudeRunner.SetMCPServers(servers)
	}

	m.chat.SetSession(sess.Name, m.claudeRunner.GetMessages())
	m.header.SetSessionName(sess.Name)
	m.focus = FocusChat
	m.sidebar.SetFocused(false)
	m.chat.SetFocused(true)

	// Restore waiting state if this session is streaming
	if startTime, isWaiting := m.sessionWaitStart[sess.ID]; isWaiting {
		m.chat.SetWaitingWithStart(true, startTime)
	} else {
		m.chat.SetWaiting(false)
	}

	// Restore pending permission if this session has one
	if req, exists := m.pendingPermissions[sess.ID]; exists {
		m.chat.SetPendingPermission(req.Tool, req.Description)
	} else {
		m.chat.ClearPendingPermission()
	}

	// Restore streaming content if this session has ongoing streaming
	if streamingContent, exists := m.sessionStreaming[sess.ID]; exists && streamingContent != "" {
		m.chat.SetStreaming(streamingContent)
		delete(m.sessionStreaming, sess.ID) // Clear so it doesn't persist if we switch away again
		logger.Log("App: Restored streaming content for session %s", sess.ID)
	}

	// Restore saved input text for this session
	if savedInput, exists := m.sessionInputs[sess.ID]; exists {
		m.chat.SetInput(savedInput)
		logger.Log("App: Restored input for session %s: %q", sess.ID, savedInput)
	} else {
		m.chat.ClearInput()
	}

	logger.Log("App: Session selected and focused: %s", sess.ID)
}

func (m *Model) sendMessage() (tea.Model, tea.Cmd) {
	input := m.chat.GetInput()
	logger.Log("App: sendMessage called, input=%q, len=%d, canSend=%v", input, len(input), m.CanSendMessage())
	if input == "" || !m.CanSendMessage() {
		return m, nil
	}

	inputPreview := input
	if len(inputPreview) > 50 {
		inputPreview = inputPreview[:50] + "..."
	}
	logger.Log("App: Sending message to session %s: %q", m.activeSession.ID, inputPreview)

	// Capture session info before any async operations
	sessionID := m.activeSession.ID
	runner := m.claudeRunner

	m.chat.AddUserMessage(input)
	m.chat.ClearInput()
	m.sessionWaitStart[sessionID] = time.Now()
	m.chat.SetWaitingWithStart(true, m.sessionWaitStart[sessionID])
	m.sidebar.SetStreaming(sessionID, true)
	m.setState(StateStreamingClaude)

	// Create context for this request
	ctx, cancel := context.WithCancel(context.Background())
	m.sessionStreamCancels[sessionID] = cancel

	// Start Claude request - runner tracks its own response channel
	responseChan := runner.Send(ctx, input)

	// Return commands to listen for response and permission requests
	// Also start the spinner and stopwatch ticks
	return m, tea.Batch(
		m.listenForSessionResponse(sessionID, responseChan),
		m.listenForSessionPermission(sessionID, runner),
		ui.SidebarTick(),
		ui.StopwatchTick(),
	)
}

// handlePermissionResponse handles y/n/a key presses for permission prompts
func (m *Model) handlePermissionResponse(key string, sessionID string, req *mcp.PermissionRequest) (tea.Model, tea.Cmd) {
	runner, exists := m.claudeRunners[sessionID]
	if !exists {
		logger.Log("App: Permission response for unknown session %s", sessionID)
		return m, nil
	}

	var allowed, always bool
	switch key {
	case "y", "Y":
		allowed = true
	case "a", "A":
		allowed = true
		always = true
	case "n", "N":
		allowed = false
	}

	logger.Log("App: Permission response for session %s: key=%s, allowed=%v, always=%v", sessionID, key, allowed, always)

	// Build response
	resp := mcp.PermissionResponse{
		ID:      req.ID,
		Allowed: allowed,
		Always:  always,
	}
	if !allowed {
		resp.Message = "User denied permission"
	}

	// If always, save the tool to per-repo allowed list
	if always {
		if sess := m.config.GetSession(sessionID); sess != nil {
			m.config.AddRepoAllowedTool(sess.RepoPath, req.Tool)
			m.config.Save()
			runner.AddAllowedTool(req.Tool)
			logger.Log("App: Added tool %s to allowed list for repo %s", req.Tool, sess.RepoPath)
		}
	}

	// Send response
	runner.SendPermissionResponse(resp)

	// Clear pending permission
	delete(m.pendingPermissions, sessionID)
	m.sidebar.SetPendingPermission(sessionID, false)
	m.chat.ClearPendingPermission()

	// Continue listening for responses and permissions
	return m, tea.Batch(
		m.listenForSessionResponse(sessionID, runner.GetResponseChan()),
		m.listenForSessionPermission(sessionID, runner),
	)
}

// listenForSessionResponse creates a command to listen for responses from a specific session
func (m *Model) listenForSessionResponse(sessionID string, ch <-chan claude.ResponseChunk) tea.Cmd {
	if ch == nil {
		return nil
	}

	return func() tea.Msg {
		chunk, ok := <-ch
		if !ok {
			return ClaudeResponseMsg{SessionID: sessionID, Chunk: claude.ResponseChunk{Done: true}}
		}
		return ClaudeResponseMsg{SessionID: sessionID, Chunk: chunk}
	}
}

// listenForSessionPermission creates a command to listen for permission requests from a specific session
func (m *Model) listenForSessionPermission(sessionID string, runner *claude.Runner) tea.Cmd {
	if runner == nil {
		return nil
	}

	ch := runner.PermissionRequestChan()
	return func() tea.Msg {
		req, ok := <-ch
		if !ok {
			return nil
		}
		return PermissionRequestMsg{SessionID: sessionID, Request: req}
	}
}

func (m *Model) listenForMergeResult(sessionID string) tea.Cmd {
	ch, exists := m.sessionMergeChans[sessionID]
	if !exists || ch == nil {
		return nil
	}

	return func() tea.Msg {
		result, ok := <-ch
		if !ok {
			return MergeResultMsg{SessionID: sessionID, Result: git.Result{Done: true}}
		}
		return MergeResultMsg{SessionID: sessionID, Result: result}
	}
}

// saveRunnerMessages saves messages for a specific runner/session
func (m *Model) saveRunnerMessages(sessionID string, runner *claude.Runner) {
	if runner == nil {
		return
	}

	msgs := runner.GetMessages()
	var configMsgs []config.Message
	for _, msg := range msgs {
		configMsgs = append(configMsgs, config.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	config.SaveSessionMessages(sessionID, configMsgs, config.MaxSessionMessageLines)
}

// hasAnyStreamingSessions returns true if any session is currently streaming
func (m *Model) hasAnyStreamingSessions() bool {
	for _, runner := range m.claudeRunners {
		if runner.IsStreaming() {
			return true
		}
	}
	return false
}

// getSessionByID returns the session config for a given session ID
func (m *Model) getSessionByID(sessionID string) *config.Session {
	sessions := m.config.GetSessions()
	for i := range sessions {
		if sessions[i].ID == sessionID {
			return &sessions[i]
		}
	}
	return nil
}

func (m *Model) updateSizes() {
	ctx := ui.GetViewContext()
	ctx.UpdateTerminalSize(m.width, m.height)

	m.header.SetWidth(ctx.TerminalWidth)
	m.footer.SetWidth(ctx.TerminalWidth)
	m.sidebar.SetSize(ctx.SidebarWidth, ctx.ContentHeight)
	m.chat.SetSize(ctx.ChatWidth, ctx.ContentHeight)
}

// View renders the app
func (m *Model) View() tea.View {
	var v tea.View
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	if m.width == 0 || m.height == 0 {
		v.SetContent("Loading...")
		return v
	}

	// Update footer context for conditional bindings
	hasSession := m.sidebar.SelectedSession() != nil
	sidebarFocused := m.focus == FocusSidebar
	hasPendingPermission := m.activeSession != nil && m.pendingPermissions[m.activeSession.ID] != nil
	isStreaming := m.activeSession != nil && m.sessionStreamCancels[m.activeSession.ID] != nil
	m.footer.SetContext(hasSession, sidebarFocused, hasPendingPermission, isStreaming)

	header := m.header.View()
	footer := m.footer.View()

	// Render panels side by side
	sidebarView := m.sidebar.View()
	chatView := m.chat.View()

	panels := lipgloss.JoinHorizontal(
		lipgloss.Top,
		sidebarView,
		chatView,
	)

	view := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		panels,
		footer,
	)

	// Overlay modal if visible
	if m.modal.IsVisible() {
		modalView := m.modal.View(m.width, m.height)
		// Center modal over the view
		bgStyle := lipgloss.NewStyle().Background(lipgloss.Color("#000000"))
		v.SetContent(lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			modalView,
			lipgloss.WithWhitespaceStyle(bgStyle),
		))
		return v
	}

	v.SetContent(view)
	return v
}
