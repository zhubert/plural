package app

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/zhubert/plural/internal/changelog"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/clipboard"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/mcp"
	"github.com/zhubert/plural/internal/process"
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
	claudeRunner  *claude.Runner // Currently active runner (convenience reference)

	// Session lifecycle management
	sessionMgr *SessionManager

	// State machine
	state AppState // Current application state

	// Pending commit message editing state (one at a time)
	pendingCommitSession string    // Session ID waiting for commit message confirmation
	pendingCommitType    MergeType // What operation follows after commit
}

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

// New creates a new app model
func New(cfg *config.Config, version string) *Model {
	// Load saved theme from config, or use default
	if savedTheme := cfg.GetTheme(); savedTheme != "" {
		ui.SetThemeByName(savedTheme)
	}

	m := &Model{
		config:     cfg,
		version:    version,
		header:     ui.NewHeader(),
		footer:     ui.NewFooter(),
		sidebar:    ui.NewSidebar(),
		chat:       ui.NewChat(),
		modal:      ui.NewModal(),
		focus:      FocusSidebar,
		sessionMgr: NewSessionManager(cfg),
		state:      StateIdle,
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

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	// Trigger startup modal check (welcome or changelog)
	return func() tea.Msg {
		return StartupModalMsg{}
	}
}

// Update handles messages
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateSizes()

	case tea.PasteStartMsg:
		// Handle paste events - check for images in clipboard when paste starts
		// Terminals intercept Ctrl+V and send paste events instead of key presses
		logger.Log("App: PasteStartMsg received, focus=%v, hasActiveSession=%v", m.focus, m.activeSession != nil)
		if m.focus == FocusChat && m.activeSession != nil {
			model, cmd := m.handleImagePaste()
			if m.chat.HasPendingImage() {
				// Image was attached, don't process text paste
				return model, cmd
			}
			// No image found, let text paste proceed normally
		}

	case tea.PasteMsg:
		// Debug: log paste content to understand what's being pasted
		content := msg.Content
		preview := content
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		logger.Log("App: PasteMsg received: len=%d, preview=%q", len(content), preview)

	case tea.KeyPressMsg:
		// Handle modal first if visible
		if m.modal.IsVisible() {
			return m.handleModalKey(msg)
		}

		// Handle permission response when chat is focused and has pending permission
		if m.focus == FocusChat && m.activeSession != nil {
			if req := m.sessionState().GetPendingPermission(m.activeSession.ID); req != nil {
				switch msg.String() {
				case "y", "Y", "n", "N", "a", "A":
					return m.handlePermissionResponse(msg.String(), m.activeSession.ID, req)
				}
			}
		}

		// Handle question response when chat is focused and has pending question
		if m.focus == FocusChat && m.activeSession != nil {
			if m.sessionState().GetPendingQuestion(m.activeSession.ID) != nil {
				key := msg.String()
				switch key {
				case "1", "2", "3", "4", "5":
					num := int(key[0] - '0')
					if m.chat.SelectOptionByNumber(num) {
						return m.submitQuestionResponse(m.activeSession.ID)
					}
					return m, nil
				case "up", "k":
					m.chat.MoveQuestionSelection(-1)
					return m, nil
				case "down", "j":
					m.chat.MoveQuestionSelection(1)
					return m, nil
				case "enter":
					if m.chat.SelectCurrentOption() {
						return m.submitQuestionResponse(m.activeSession.ID)
					}
					return m, nil
				}
			}
		}

		// Handle Escape to interrupt streaming
		if msg.String() == "esc" && m.activeSession != nil {
			if cancel := m.sessionState().GetStreamCancel(m.activeSession.ID); cancel != nil {
				logger.Log("App: Interrupting streaming for session %s", m.activeSession.ID)
				cancel()
				m.sessionState().StopWaiting(m.activeSession.ID)
				m.sidebar.SetStreaming(m.activeSession.ID, false)
				m.chat.SetWaiting(false)
				// Save partial response to runner before finishing
				if content := m.chat.GetStreaming(); content != "" {
					m.claudeRunner.AddAssistantMessage(content + "\n[Interrupted]")
					m.sessionMgr.SaveRunnerMessages(m.activeSession.ID, m.claudeRunner)
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

		// Handle Ctrl+V for image pasting when chat is focused
		// Note: Most terminals intercept Ctrl+V and send PasteStartMsg instead,
		// but we keep this as a fallback for terminals that send raw key presses.
		if msg.String() == "ctrl+v" && m.focus == FocusChat && m.activeSession != nil {
			return m.handleImagePaste()
		}

		// Handle backspace to remove pending image when input is empty
		if msg.String() == "backspace" && m.focus == FocusChat && m.activeSession != nil {
			if m.chat.HasPendingImage() && m.chat.GetInput() == "" {
				m.chat.ClearImage()
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
				m.modal.Show(ui.NewNewSessionState(m.config.GetRepos()))
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
				m.modal.Show(ui.NewAddRepoState(currentRepo))
			}
		case "d":
			if !m.chat.IsFocused() && m.sidebar.SelectedSession() != nil {
				m.modal.Show(ui.NewConfirmDeleteState())
			}
		case "v":
			if !m.chat.IsFocused() && m.sidebar.SelectedSession() != nil {
				sess := m.sidebar.SelectedSession()
				// Select the session first so we can display in its chat panel
				if m.activeSession == nil || m.activeSession.ID != sess.ID {
					m.selectSession(sess)
				}
				// Get worktree status and display it in view changes overlay
				status, err := git.GetWorktreeStatus(sess.WorkTree)
				var content string
				if err != nil {
					content = fmt.Sprintf("[Error getting status: %v]\n", err)
				} else if !status.HasChanges {
					content = "No uncommitted changes in this session."
				} else {
					var sb strings.Builder
					sb.WriteString(fmt.Sprintf("📝 Uncommitted changes (%s):\n\n", status.Summary))
					for _, file := range status.Files {
						sb.WriteString(fmt.Sprintf("  • %s\n", file))
					}
					if status.Diff != "" {
						sb.WriteString("\n--- Diff ---\n")
						sb.WriteString(ui.HighlightDiff(status.Diff))
					}
					content = sb.String()
				}
				m.chat.EnterViewChangesMode(content)
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
				m.modal.Show(ui.NewMergeState(hasRemote, changesSummary))
			}
		case "f":
			// Force-resume: kill orphaned processes and clear the error state
			if !m.chat.IsFocused() && m.sidebar.SelectedSession() != nil {
				sess := m.sidebar.SelectedSession()
				if m.sessionState().HasSessionInUseError(sess.ID) {
					return m.forceResumeSession(sess)
				}
			}
		case "s":
			if !m.chat.IsFocused() {
				m.showMCPServersModal()
			}
		case "t":
			if !m.chat.IsFocused() {
				m.modal.Show(ui.NewThemeState(ui.CurrentThemeName()))
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
		runner := m.sessionMgr.GetRunner(msg.SessionID)
		exists := runner != nil
		if !exists {
			logger.Log("App: Received response for unknown session %s", msg.SessionID)
			return m, nil
		}

		isActiveSession := m.activeSession != nil && m.activeSession.ID == msg.SessionID

		if msg.Chunk.Error != nil {
			errMsg := msg.Chunk.Error.Error()
			logger.Log("App: Error in session %s: %v", msg.SessionID, errMsg)
			m.sidebar.SetStreaming(msg.SessionID, false)
			m.sessionState().StopWaiting(msg.SessionID)

			// Check if this is a "session in use" error
			if process.IsSessionInUseError(errMsg) {
				logger.Log("App: Session %s appears to be in use by another process", msg.SessionID)
				m.sessionState().SetSessionInUseError(msg.SessionID, true)
				m.sidebar.SetSessionInUse(msg.SessionID, true)
				errMsg = "Session is in use by another process. Press 'f' to force resume by killing orphaned processes."
			}

			if isActiveSession {
				m.chat.SetWaiting(false)
				m.chat.AppendStreaming("\n[Error: " + errMsg + "]")
			} else {
				// Store error for non-active session
				m.sessionState().AppendStreaming(msg.SessionID, "\n[Error: "+errMsg+"]")
			}
			// Check if any sessions are still streaming
			if !m.hasAnyStreamingSessions() {
				m.setState(StateIdle)
			}
		} else if msg.Chunk.Done {
			logger.Log("App: Session %s completed streaming", msg.SessionID)
			m.sidebar.SetStreaming(msg.SessionID, false)
			m.sessionState().StopWaiting(msg.SessionID)
			if isActiveSession {
				m.chat.SetWaiting(false)
				m.chat.FinishStreaming()
			} else {
				// For non-active session, just clear our saved streaming content
				// The runner already adds the assistant message when streaming completes (claude.go)
				m.sessionState().ClearStreaming(msg.SessionID)
			}
			// Mark session as started and save messages
			sess := m.sessionMgr.GetSession(msg.SessionID)
			if sess != nil && runner.SessionStarted() {
				if !sess.Started {
					m.config.MarkSessionStarted(sess.ID)
					sess.Started = true
					m.config.Save()
				}
				// Save messages for this session
				m.sessionMgr.SaveRunnerMessages(msg.SessionID, runner)
			}
			// Check if any sessions are still streaming
			if !m.hasAnyStreamingSessions() {
				m.setState(StateIdle)
			}
		} else {
			// Streaming content - clear wait time since response has started
			m.sessionState().ClearWaitStart(msg.SessionID)
			if isActiveSession {
				m.chat.SetWaiting(false)
				// Handle different chunk types
				switch msg.Chunk.Type {
				case claude.ChunkTypeToolUse:
					// Append tool use to streaming content so it persists in history
					m.chat.AppendToolUse(msg.Chunk.ToolName, msg.Chunk.ToolInput)
				case claude.ChunkTypeToolResult:
					// Tool completed, mark the tool use line as complete
					m.chat.MarkLastToolUseComplete()
				case claude.ChunkTypeText:
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
					existing := m.sessionState().GetStreaming(msg.SessionID)
					if existing != "" && !strings.HasSuffix(existing, "\n") {
						m.sessionState().AppendStreaming(msg.SessionID, "\n")
					}
					// Track position where the marker starts
					m.sessionState().SetToolUsePos(msg.SessionID, len(m.sessionState().GetStreaming(msg.SessionID)))
					m.sessionState().AppendStreaming(msg.SessionID, line)
				case claude.ChunkTypeToolResult:
					// Mark the tool use as complete for non-active session
					if pos, exists := m.sessionState().GetToolUsePos(msg.SessionID); exists && pos >= 0 {
						m.sessionState().ReplaceToolUseMarker(msg.SessionID, ui.ToolUseInProgress, ui.ToolUseComplete, pos)
						m.sessionState().ClearToolUsePos(msg.SessionID)
					}
				case claude.ChunkTypeText:
					// Add extra newline after tool use for visual separation
					if pos, exists := m.sessionState().GetToolUsePos(msg.SessionID); exists && pos >= 0 {
						streaming := m.sessionState().GetStreaming(msg.SessionID)
						if strings.HasSuffix(streaming, "\n") && !strings.HasSuffix(streaming, "\n\n") {
							m.sessionState().AppendStreaming(msg.SessionID, "\n")
						}
					}
					m.sessionState().AppendStreaming(msg.SessionID, msg.Chunk.Content)
				default:
					if msg.Chunk.Content != "" {
						m.sessionState().AppendStreaming(msg.SessionID, msg.Chunk.Content)
					}
				}
			}
			// Continue listening for more chunks from this session
			return m, tea.Batch(
				m.listenForSessionResponse(msg.SessionID, runner.GetResponseChan()),
				m.listenForSessionPermission(msg.SessionID, runner),
				m.listenForSessionQuestion(msg.SessionID, runner),
			)
		}

	case PermissionRequestMsg:
		// Get the runner for this session
		runner := m.sessionMgr.GetRunner(msg.SessionID)
		exists := runner != nil
		if !exists {
			logger.Log("App: Received permission request for unknown session %s", msg.SessionID)
			return m, nil
		}

		// Store permission request for this session (inline, not modal)
		logger.Log("App: Permission request for session %s: tool=%s", msg.SessionID, msg.Request.Tool)
		m.sessionState().SetPendingPermission(msg.SessionID, &msg.Request)
		m.sidebar.SetPendingPermission(msg.SessionID, true)

		// If this is the active session, show permission in chat
		if m.activeSession != nil && m.activeSession.ID == msg.SessionID {
			m.chat.SetPendingPermission(msg.Request.Tool, msg.Request.Description)
		}

		// Continue listening for more permission requests and responses
		return m, tea.Batch(
			m.listenForSessionResponse(msg.SessionID, runner.GetResponseChan()),
			m.listenForSessionPermission(msg.SessionID, runner),
			m.listenForSessionQuestion(msg.SessionID, runner),
		)

	case QuestionRequestMsg:
		// Get the runner for this session
		runner := m.sessionMgr.GetRunner(msg.SessionID)
		exists := runner != nil
		if !exists {
			logger.Log("App: Received question request for unknown session %s", msg.SessionID)
			return m, nil
		}

		// Store question request for this session
		logger.Log("App: Question request for session %s: %d questions", msg.SessionID, len(msg.Request.Questions))
		m.sessionState().SetPendingQuestion(msg.SessionID, &msg.Request)
		m.sidebar.SetPendingPermission(msg.SessionID, true) // Reuse permission indicator for questions

		// If this is the active session, show question in chat
		if m.activeSession != nil && m.activeSession.ID == msg.SessionID {
			m.chat.SetPendingQuestion(msg.Request.Questions)
		}

		// Continue listening for more requests and responses
		return m, tea.Batch(
			m.listenForSessionResponse(msg.SessionID, runner.GetResponseChan()),
			m.listenForSessionPermission(msg.SessionID, runner),
			m.listenForSessionQuestion(msg.SessionID, runner),
		)

	case CommitMessageGeneratedMsg:
		// Commit message generation completed
		if msg.Error != nil {
			logger.Log("App: Commit message generation failed: %v", msg.Error)
			m.chat.AppendStreaming(fmt.Sprintf("Failed to generate commit message: %v\n", msg.Error))
			m.pendingCommitSession = ""
			m.pendingCommitType = MergeTypeNone
			return m, nil
		}

		// Show the edit commit modal with the generated message
		m.modal.Show(ui.NewEditCommitState(msg.Message, m.pendingCommitType.String()))
		return m, nil

	case MergeResultMsg:
		isActiveSession := m.activeSession != nil && m.activeSession.ID == msg.SessionID
		if msg.Result.Error != nil {
			if isActiveSession {
				m.chat.AppendStreaming("\n[Error: " + msg.Result.Error.Error() + "]\n")
			} else {
				m.sessionState().AppendStreaming(msg.SessionID, "\n[Error: "+msg.Result.Error.Error()+"]\n")
			}
			// Clean up merge state for this session
			m.sessionState().StopMerge(msg.SessionID)
		} else if msg.Result.Done {
			if isActiveSession {
				m.chat.FinishStreaming()
			} else {
				// Store completed merge output as a message for when user switches back
				if content := m.sessionState().GetStreaming(msg.SessionID); content != "" {
					if runner := m.sessionMgr.GetRunner(msg.SessionID); runner != nil {
						runner.AddAssistantMessage(content)
						m.sessionMgr.SaveRunnerMessages(msg.SessionID, runner)
					}
					m.sessionState().ClearStreaming(msg.SessionID)
				}
			}
			// Mark session as merged or PR created based on operation type
			mergeType := m.sessionState().GetMergeType(msg.SessionID)
			if mergeType == MergeTypePR {
				m.config.MarkSessionPRCreated(msg.SessionID)
				logger.Log("App: Marked session %s as PR created", msg.SessionID)
			} else if mergeType == MergeTypeMerge {
				m.config.MarkSessionMerged(msg.SessionID)
				logger.Log("App: Marked session %s as merged", msg.SessionID)
			}
			m.config.Save()
			// Update sidebar with new session status
			m.sidebar.SetSessions(m.config.GetSessions())
			// Clean up merge state for this session
			m.sessionState().StopMerge(msg.SessionID)
		} else {
			if isActiveSession {
				m.chat.AppendStreaming(msg.Result.Output)
			} else {
				m.sessionState().AppendStreaming(msg.SessionID, msg.Result.Output)
			}
			return m, m.listenForMergeResult(msg.SessionID)
		}

	case StartupModalMsg:
		return m.handleStartupModals()
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

	m.modal.Show(ui.NewMCPServersState(globalServers, perRepoServers, repos))
}

func (m *Model) selectSession(sess *config.Session) {
	if sess == nil {
		return
	}

	// Get previous session state to save
	var previousSessionID, previousInput, previousStreaming string
	if m.activeSession != nil {
		previousSessionID = m.activeSession.ID
		previousInput = m.chat.GetInput()
		previousStreaming = m.chat.GetStreaming()
	}

	// Use SessionManager to handle selection (creates/reuses runner, gathers state)
	result := m.sessionMgr.Select(sess, previousSessionID, previousInput, previousStreaming)
	if result == nil {
		return
	}

	// Update app state
	m.activeSession = sess
	m.claudeRunner = result.Runner

	// Exit view changes mode when switching sessions
	if m.chat.IsInViewChangesMode() {
		m.chat.ExitViewChangesMode()
	}

	// Update UI components with session state
	m.chat.SetSession(sess.Name, result.Messages)
	m.header.SetSessionName(result.HeaderName)
	m.focus = FocusChat
	m.sidebar.SetFocused(false)
	m.chat.SetFocused(true)

	// Restore waiting state
	if result.IsWaiting {
		m.chat.SetWaitingWithStart(true, result.WaitStart)
	} else {
		m.chat.SetWaiting(false)
	}

	// Restore pending permission
	if result.Permission != nil {
		m.chat.SetPendingPermission(result.Permission.Tool, result.Permission.Description)
	} else {
		m.chat.ClearPendingPermission()
	}

	// Restore pending question
	if result.Question != nil {
		m.chat.SetPendingQuestion(result.Question.Questions)
	} else {
		m.chat.ClearPendingQuestion()
	}

	// Restore streaming content
	if result.Streaming != "" {
		m.chat.SetStreaming(result.Streaming)
	}

	// Restore saved input
	if result.SavedInput != "" {
		m.chat.SetInput(result.SavedInput)
	} else {
		m.chat.ClearInput()
	}

	logger.Log("App: Session selected and focused: %s", sess.ID)
}

// forceResumeSession kills any orphaned Claude processes for the session and clears the error state
func (m *Model) forceResumeSession(sess *config.Session) (tea.Model, tea.Cmd) {
	// Use SessionManager to handle force resume
	result := m.sessionMgr.ForceResume(sess)
	if result.Error != nil {
		m.chat.AppendStreaming(fmt.Sprintf("\n[Error killing orphaned processes: %v]", result.Error))
		return m, nil
	}

	// Update sidebar UI
	m.sidebar.SetSessionInUse(sess.ID, false)

	// Show result in chat
	if result.Killed > 0 {
		m.chat.AppendStreaming(fmt.Sprintf("\n[Killed %d orphaned process(es). Session ready to resume.]", result.Killed))
	} else {
		m.chat.AppendStreaming("\n[No orphaned processes found. Session state cleared.]")
	}

	// Re-select the session to create a fresh runner
	m.selectSession(sess)

	return m, nil
}

// handleImagePaste attempts to read an image from the clipboard and attach it
func (m *Model) handleImagePaste() (tea.Model, tea.Cmd) {
	logger.Log("App: Handling image paste")

	// Try to read image from clipboard
	img, err := clipboard.ReadImage()
	if err != nil {
		logger.Log("App: Failed to read image from clipboard: %v", err)
		// Don't show error to user - might just be text paste
		return m, nil
	}

	if img == nil {
		logger.Log("App: No image in clipboard")
		// No image, let text paste happen normally
		return m, nil
	}

	// Validate the image
	if err := img.Validate(); err != nil {
		logger.Log("App: Image validation failed: %v", err)
		// Show error message in chat
		m.chat.AppendStreaming(fmt.Sprintf("\n[Error: %s]\n", err.Error()))
		return m, nil
	}

	// Attach the image
	logger.Log("App: Attaching image: %dKB, %s", img.SizeKB(), img.MediaType)
	m.chat.AttachImage(img.Data, img.MediaType)

	return m, nil
}

func (m *Model) sendMessage() (tea.Model, tea.Cmd) {
	input := m.chat.GetInput()
	hasImage := m.chat.HasPendingImage()
	logger.Log("App: sendMessage called, input=%q, len=%d, hasImage=%v, canSend=%v", input, len(input), hasImage, m.CanSendMessage())

	// Need either text or image
	if input == "" && !hasImage {
		return m, nil
	}
	if !m.CanSendMessage() {
		return m, nil
	}

	inputPreview := input
	if len(inputPreview) > 50 {
		inputPreview = inputPreview[:50] + "..."
	}
	logger.Log("App: Sending message to session %s: %q, hasImage=%v", m.activeSession.ID, inputPreview, hasImage)

	// Capture session info before any async operations
	sessionID := m.activeSession.ID
	runner := m.claudeRunner

	// Build content blocks
	var content []claude.ContentBlock

	// Add text if present
	if input != "" {
		content = append(content, claude.ContentBlock{
			Type: claude.ContentTypeText,
			Text: input,
		})
	}

	// Add image if present
	if hasImage {
		imageData, mediaType := m.chat.GetPendingImage()
		content = append(content, claude.ContentBlock{
			Type: claude.ContentTypeImage,
			Source: &claude.ImageSource{
				Type:      "base64",
				MediaType: mediaType,
				Data:      base64.StdEncoding.EncodeToString(imageData),
			},
		})
	}

	// Display message to user (text only, images shown as [Image])
	displayMsg := input
	if hasImage {
		if displayMsg != "" {
			displayMsg += "\n[Image attached]"
		} else {
			displayMsg = "[Image attached]"
		}
	}
	m.chat.AddUserMessage(displayMsg)
	m.chat.ClearInput()

	// Create context for this request
	ctx, cancel := context.WithCancel(context.Background())
	m.sessionState().StartWaiting(sessionID, cancel)
	startTime, _ := m.sessionState().GetWaitStart(sessionID)
	m.chat.SetWaitingWithStart(true, startTime)
	m.sidebar.SetStreaming(sessionID, true)
	m.setState(StateStreamingClaude)

	// Start Claude request with content blocks
	responseChan := runner.SendContent(ctx, content)

	// Return commands to listen for response, permission requests, and question requests
	// Also start the spinner and stopwatch ticks
	return m, tea.Batch(
		m.listenForSessionResponse(sessionID, responseChan),
		m.listenForSessionPermission(sessionID, runner),
		m.listenForSessionQuestion(sessionID, runner),
		ui.SidebarTick(),
		ui.StopwatchTick(),
	)
}

// handlePermissionResponse handles y/n/a key presses for permission prompts
func (m *Model) handlePermissionResponse(key string, sessionID string, req *mcp.PermissionRequest) (tea.Model, tea.Cmd) {
	runner := m.sessionMgr.GetRunner(sessionID)
	if runner == nil {
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
		m.sessionMgr.AddAllowedTool(sessionID, req.Tool)
	}

	// Send response
	runner.SendPermissionResponse(resp)

	// Clear pending permission
	m.sessionState().ClearPendingPermission(sessionID)
	m.sidebar.SetPendingPermission(sessionID, false)
	m.chat.ClearPendingPermission()

	// Continue listening for responses, permissions, and questions
	return m, tea.Batch(
		m.listenForSessionResponse(sessionID, runner.GetResponseChan()),
		m.listenForSessionPermission(sessionID, runner),
		m.listenForSessionQuestion(sessionID, runner),
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

// listenForSessionQuestion creates a command to listen for question requests from a specific session
func (m *Model) listenForSessionQuestion(sessionID string, runner *claude.Runner) tea.Cmd {
	if runner == nil {
		return nil
	}

	ch := runner.QuestionRequestChan()
	return func() tea.Msg {
		req, ok := <-ch
		if !ok {
			return nil
		}
		return QuestionRequestMsg{SessionID: sessionID, Request: req}
	}
}

// submitQuestionResponse sends the collected question answers back to Claude
func (m *Model) submitQuestionResponse(sessionID string) (tea.Model, tea.Cmd) {
	runner := m.sessionMgr.GetRunner(sessionID)
	if runner == nil {
		logger.Log("App: Question response for unknown session %s", sessionID)
		return m, nil
	}

	req := m.sessionState().GetPendingQuestion(sessionID)
	if req == nil {
		logger.Log("App: No pending question for session %s", sessionID)
		return m, nil
	}

	// Get answers from chat
	answers := m.chat.GetQuestionAnswers()
	logger.Log("App: Question response for session %s: %d answers", sessionID, len(answers))

	// Build response
	resp := mcp.QuestionResponse{
		ID:      req.ID,
		Answers: answers,
	}

	// Send response
	runner.SendQuestionResponse(resp)

	// Clear pending question
	m.sessionState().ClearPendingQuestion(sessionID)
	m.sidebar.SetPendingPermission(sessionID, false)
	m.chat.ClearPendingQuestion()

	// Continue listening for responses and more requests
	return m, tea.Batch(
		m.listenForSessionResponse(sessionID, runner.GetResponseChan()),
		m.listenForSessionPermission(sessionID, runner),
		m.listenForSessionQuestion(sessionID, runner),
	)
}

func (m *Model) listenForMergeResult(sessionID string) tea.Cmd {
	ch := m.sessionState().GetMergeChan(sessionID)
	if ch == nil {
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

// generateCommitMessage creates a command to generate a commit message asynchronously
func (m *Model) generateCommitMessage(sessionID, worktreePath string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		msg, err := git.GenerateCommitMessageWithClaude(ctx, worktreePath)
		if err != nil {
			// Fall back to simple message
			msg, err = git.GenerateCommitMessage(worktreePath)
		}

		return CommitMessageGeneratedMsg{
			SessionID: sessionID,
			Message:   msg,
			Error:     err,
		}
	}
}

// hasAnyStreamingSessions returns true if any session is currently streaming
func (m *Model) hasAnyStreamingSessions() bool {
	return m.sessionMgr.HasActiveStreaming()
}

// handleStartupModals checks and shows welcome or changelog modals on startup
func (m *Model) handleStartupModals() (tea.Model, tea.Cmd) {
	// Priority 1: Welcome modal for first-time users
	if !m.config.HasSeenWelcome() {
		logger.Log("App: Showing welcome modal (first-time user)")
		m.modal.Show(ui.NewWelcomeState())
		return m, nil
	}

	// Priority 2: Changelog modal for new versions
	// Skip for dev builds
	if m.version != "" && m.version != "dev" {
		lastSeen := m.config.GetLastSeenVersion()
		if lastSeen != m.version {
			entries := changelog.Parse(changelog.Content)
			changes := changelog.GetChangesSince(lastSeen, entries)
			if len(changes) > 0 {
				logger.Log("App: Showing changelog modal (version %s -> %s, %d entries)", lastSeen, m.version, len(changes))
				// Convert changelog entries to UI entries
				uiEntries := make([]ui.ChangelogEntry, len(changes))
				for i, e := range changes {
					uiEntries[i] = ui.ChangelogEntry{
						Version: e.Version,
						Date:    e.Date,
						Changes: e.Changes,
					}
				}
				m.modal.Show(ui.NewChangelogState(uiEntries))
				return m, nil
			}
			// No new changes, just update last seen version
			m.config.SetLastSeenVersion(m.version)
			m.config.Save()
		}
	}

	return m, nil
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
	hasPendingPermission := m.activeSession != nil && m.sessionState().GetPendingPermission(m.activeSession.ID) != nil
	hasPendingQuestion := m.activeSession != nil && m.sessionState().GetPendingQuestion(m.activeSession.ID) != nil
	isStreaming := m.activeSession != nil && m.sessionState().GetStreamCancel(m.activeSession.ID) != nil
	selectedSess := m.sidebar.SelectedSession()
	sessionInUse := selectedSess != nil && m.sessionState().HasSessionInUseError(selectedSess.ID)
	viewChangesMode := m.chat.IsInViewChangesMode()
	m.footer.SetContext(hasSession, sidebarFocused, hasPendingPermission, hasPendingQuestion, isStreaming, sessionInUse, viewChangesMode)

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
