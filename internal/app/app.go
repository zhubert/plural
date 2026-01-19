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
	// Sessions merged to parent are locked and cannot accept new messages
	if m.activeSession.MergedToParent {
		return false
	}
	// Check if the active session is currently waiting for a response or has a merge in progress
	// Each session can operate independently
	state := m.sessionMgr.StateManager().GetIfExists(m.activeSession.ID)
	if state == nil {
		return true // No state means not waiting or merging
	}
	return !state.IsWaiting && !state.IsMerging()
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

// refreshDiffStats updates the header with current git diff statistics for the active session
func (m *Model) refreshDiffStats() {
	if m.activeSession == nil || m.activeSession.WorkTree == "" {
		m.header.SetDiffStats(nil)
		return
	}

	gitStats, err := git.GetDiffStats(m.activeSession.WorkTree)
	if err != nil {
		logger.Log("App: Failed to refresh diff stats: %v", err)
		m.header.SetDiffStats(nil)
		return
	}

	m.header.SetDiffStats(&ui.DiffStats{
		FilesChanged: gitStats.FilesChanged,
		Additions:    gitStats.Additions,
		Deletions:    gitStats.Deletions,
	})
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

	case tea.FocusMsg:
		m.windowFocused = true
		logger.Log("App: Window focused")

	case tea.BlurMsg:
		m.windowFocused = false
		logger.Log("App: Window blurred")

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
		logger.Log("App: KeyPressMsg received: key=%q, focus=%v, modalVisible=%v", msg.String(), m.focus, m.modal.IsVisible())

		// Handle modal first if visible
		if m.modal.IsVisible() {
			return m.handleModalKey(msg)
		}

		// Handle Escape to exit search mode, view changes mode, or interrupt streaming
		if msg.String() == "esc" {
			// First check if sidebar is in search mode
			if m.sidebar.IsSearchMode() {
				m.sidebar.ExitSearchMode()
				return m, nil
			}
			// Check if view changes mode is active (regardless of focus)
			if m.chat.IsInViewChangesMode() {
				m.chat.ExitViewChangesMode()
				return m, nil
			}
			// Then check for streaming interruption
			if m.activeSession != nil {
				if state := m.sessionState().GetIfExists(m.activeSession.ID); state != nil && state.StreamCancel != nil {
					cancel := state.StreamCancel
					logger.Log("App: Interrupting streaming for session %s", m.activeSession.ID)
					cancel()
					// Send SIGINT to interrupt the Claude process (handles sub-agent work)
					if m.claudeRunner != nil {
						if err := m.claudeRunner.Interrupt(); err != nil {
							logger.Error("App: Failed to interrupt Claude: %v", err)
						}
					}
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
		}

		// Handle chat-focused keys when chat is focused with an active session
		if m.focus == FocusChat && m.activeSession != nil {
			key := msg.String()

			// Permission response
			state := m.sessionState().GetIfExists(m.activeSession.ID)
			if state != nil && state.PendingPermission != nil {
				req := state.PendingPermission
				switch key {
				case "y", "Y", "n", "N", "a", "A":
					return m.handlePermissionResponse(key, m.activeSession.ID, req)
				}
			}

			// Question response (reuse state from permission check)
			if state != nil && state.PendingQuestion != nil {
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

			// Plan approval response (reuse state from permission check)
			if state != nil && state.PendingPlanApproval != nil {
				switch key {
				case "y", "Y":
					return m.submitPlanApprovalResponse(m.activeSession.ID, true)
				case "n", "N":
					return m.submitPlanApprovalResponse(m.activeSession.ID, false)
				case "up", "k":
					m.chat.ScrollPlan(-3)
					return m, nil
				case "down", "j":
					m.chat.ScrollPlan(3)
					return m, nil
				}
			}

			// Ctrl+V for image pasting (fallback for terminals that send raw key presses)
			if key == "ctrl+v" {
				return m.handleImagePaste()
			}

			// Ctrl+P for parallel option exploration (reuse state from permission check)
			if key == "ctrl+p" && state != nil && state.HasDetectedOptions() {
				return m.showExploreOptionsModal()
			}

			// Backspace to remove pending image when input is empty
			if key == "backspace" && m.chat.HasPendingImage() && m.chat.GetInput() == "" {
				m.chat.ClearImage()
				return m, nil
			}
		}

		// Global keys
		key := msg.String()

		// Handle ctrl+c specially - always quits
		if key == "ctrl+c" {
			return m, tea.Quit
		}

		// Try executing from shortcut registry
		if result, cmd, handled := m.ExecuteShortcut(key); handled {
			return result, cmd
		}

		// Handle special cases not in the registry
		switch key {
		case "enter":
			switch m.focus {
			case FocusSidebar:
				// Select session
				if sess := m.sidebar.SelectedSession(); sess != nil {
					m.selectSession(sess)
					// Check if this session has an unsent initial message (from issue import)
					if initialMsg := m.sessionState().GetInitialMessage(sess.ID); initialMsg != "" {
						m.sessionState().GetOrCreate(sess.ID).PendingMessage = initialMsg
						return m, func() tea.Msg {
							return SendPendingMessageMsg{SessionID: sess.ID}
						}
					}
					return m, nil
				}
			case FocusChat:
				if m.CanSendMessage() {
					// Send message immediately
					return m.sendMessage()
				} else if m.activeSession != nil {
					// Check if waiting and queue message to be sent when streaming completes
					sessState := m.sessionState().GetIfExists(m.activeSession.ID)
					if sessState != nil && sessState.IsWaiting {
						input := m.chat.GetInput()
						if input != "" {
							sessState.PendingMessage = input
							m.chat.ClearInput()
							m.chat.SetQueuedMessage(input)
							logger.Log("App: Queued message for session %s while streaming", m.activeSession.ID)
						}
					}
				}
			}
		}

	case ClaudeResponseMsg:
		return m.handleClaudeResponseMsg(msg)

	case PermissionRequestMsg:
		return m.handlePermissionRequestMsg(msg)

	case QuestionRequestMsg:
		return m.handleQuestionRequestMsg(msg)

	case PlanApprovalRequestMsg:
		return m.handlePlanApprovalRequestMsg(msg)

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

	case SendPendingMessageMsg:
		return m.handleSendPendingMessageMsg(msg)

	case MergeResultMsg:
		return m.handleMergeResultMsg(msg)

	case GitHubIssuesFetchedMsg:
		return m.handleGitHubIssuesFetchedMsg(msg)

	case ChangelogFetchedMsg:
		return m.handleChangelogFetchedMsg(msg)

	case StartupModalMsg:
		return m.handleStartupModals()

	case ui.HelpShortcutTriggeredMsg:
		// Handle shortcut triggered from help modal
		return m.handleHelpShortcutTrigger(msg.Key)

	case TerminalErrorMsg:
		// Show terminal error to user in chat
		if m.activeSession != nil {
			m.chat.AppendStreaming(fmt.Sprintf("\n[%s]\n", msg.Error))
			m.chat.FinishStreaming()
		}
		return m, nil
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
	case ui.StopwatchTickMsg, ui.SelectionCopyMsg:
		chat, cmd := m.chat.Update(msg)
		m.chat = chat
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	case ui.FlashTickMsg:
		// Check if flash message has expired
		if m.footer.ClearIfExpired() {
			// Flash cleared, no need to continue ticking
			return m, tea.Batch(cmds...)
		}
		// Flash still active, continue ticking
		if m.footer.HasFlash() {
			cmds = append(cmds, ui.FlashTick())
		}
		return m, tea.Batch(cmds...)
	case ui.ClipboardErrorMsg:
		// Show error message when clipboard write fails
		m.footer.SetFlash("Failed to copy to clipboard", ui.FlashError)
		cmds = append(cmds, ui.FlashTick())
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
		// Route mouse click/motion/release events to chat panel for text selection
		if clickMsg, isClick := msg.(tea.MouseClickMsg); isClick {
			if clickMsg.X > m.sidebar.Width() {
				// Adjust coordinates to be relative to chat panel
				// X: subtract sidebar width
				// Y: subtract header height
				adjustedMsg := tea.MouseClickMsg{
					X:      clickMsg.X - m.sidebar.Width(),
					Y:      clickMsg.Y - ui.HeaderHeight,
					Button: clickMsg.Button,
					Mod:    clickMsg.Mod,
				}
				chat, cmd := m.chat.Update(adjustedMsg)
				m.chat = chat
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
		}
		if motionMsg, isMotion := msg.(tea.MouseMotionMsg); isMotion {
			if motionMsg.X > m.sidebar.Width() {
				adjustedMsg := tea.MouseMotionMsg{
					X:      motionMsg.X - m.sidebar.Width(),
					Y:      motionMsg.Y - ui.HeaderHeight,
					Button: motionMsg.Button,
					Mod:    motionMsg.Mod,
				}
				chat, cmd := m.chat.Update(adjustedMsg)
				m.chat = chat
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
		}
		if releaseMsg, isRelease := msg.(tea.MouseReleaseMsg); isRelease {
			if releaseMsg.X > m.sidebar.Width() {
				adjustedMsg := tea.MouseReleaseMsg{
					X:      releaseMsg.X - m.sidebar.Width(),
					Y:      releaseMsg.Y - ui.HeaderHeight,
					Button: releaseMsg.Button,
					Mod:    releaseMsg.Mod,
				}
				chat, cmd := m.chat.Update(adjustedMsg)
				m.chat = chat
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
		}
	}

	// Handle mouse events when chat is focused - adjust coordinates for sidebar and header
	if m.focus == FocusChat && m.activeSession != nil {
		if clickMsg, isClick := msg.(tea.MouseClickMsg); isClick {
			if clickMsg.X > m.sidebar.Width() {
				adjustedMsg := tea.MouseClickMsg{
					X:      clickMsg.X - m.sidebar.Width(),
					Y:      clickMsg.Y - ui.HeaderHeight,
					Button: clickMsg.Button,
					Mod:    clickMsg.Mod,
				}
				chat, cmd := m.chat.Update(adjustedMsg)
				m.chat = chat
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
		}
		if motionMsg, isMotion := msg.(tea.MouseMotionMsg); isMotion {
			if motionMsg.X > m.sidebar.Width() {
				adjustedMsg := tea.MouseMotionMsg{
					X:      motionMsg.X - m.sidebar.Width(),
					Y:      motionMsg.Y - ui.HeaderHeight,
					Button: motionMsg.Button,
					Mod:    motionMsg.Mod,
				}
				chat, cmd := m.chat.Update(adjustedMsg)
				m.chat = chat
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
		}
		if releaseMsg, isRelease := msg.(tea.MouseReleaseMsg); isRelease {
			if releaseMsg.X > m.sidebar.Width() {
				adjustedMsg := tea.MouseReleaseMsg{
					X:      releaseMsg.X - m.sidebar.Width(),
					Y:      releaseMsg.Y - ui.HeaderHeight,
					Button: releaseMsg.Button,
					Mod:    releaseMsg.Mod,
				}
				chat, cmd := m.chat.Update(adjustedMsg)
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

func (m *Model) toggleFocus() tea.Cmd {
	var cmds []tea.Cmd
	if m.focus == FocusSidebar {
		// Only allow switching to chat if there's an active session
		if m.activeSession == nil {
			return nil
		}
		m.focus = FocusChat
		m.sidebar.SetFocused(false)
		m.chat.SetFocused(true)
	} else {
		m.focus = FocusSidebar
		m.sidebar.SetFocused(true)
		m.chat.SetFocused(false)
	}
	if len(cmds) > 0 {
		return tea.Batch(cmds...)
	}
	return nil
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

func (m *Model) showPluginsModal() {
	// Fetch marketplaces and plugins from Claude CLI
	marketplaces, err := claude.ListMarketplaces()
	if err != nil {
		state := ui.NewPluginsState()
		state.SetError(err.Error())
		m.modal.Show(state)
		return
	}

	plugins, err := claude.ListPlugins()
	if err != nil {
		state := ui.NewPluginsState()
		state.SetError(err.Error())
		m.modal.Show(state)
		return
	}

	// Convert to display types
	var mpDisplays []ui.MarketplaceDisplay
	for _, mp := range marketplaces {
		mpDisplays = append(mpDisplays, ui.MarketplaceDisplay{
			Name:        mp.Name,
			Source:      mp.Source,
			Repo:        mp.Repo,
			LastUpdated: mp.LastUpdated.Format("2006-01-02"),
		})
	}

	var pluginDisplays []ui.PluginDisplay
	for _, p := range plugins {
		status := "available"
		if p.Enabled {
			status = "enabled"
		} else if p.Installed {
			status = "installed"
		}
		pluginDisplays = append(pluginDisplays, ui.PluginDisplay{
			Name:        p.Name,
			Marketplace: p.Marketplace,
			FullName:    p.FullName,
			Description: p.Description,
			Status:      status,
			Version:     p.Version,
		})
	}

	m.modal.Show(ui.NewPluginsStateWithData(mpDisplays, pluginDisplays))
}

// showPluginsModalOnTab shows the plugins modal and sets it to a specific tab.
func (m *Model) showPluginsModalOnTab(tab int) {
	// Fetch marketplaces and plugins from Claude CLI
	marketplaces, err := claude.ListMarketplaces()
	if err != nil {
		state := ui.NewPluginsState()
		state.ActiveTab = tab
		state.SetError(err.Error())
		m.modal.Show(state)
		return
	}

	plugins, err := claude.ListPlugins()
	if err != nil {
		state := ui.NewPluginsState()
		state.ActiveTab = tab
		state.SetError(err.Error())
		m.modal.Show(state)
		return
	}

	// Convert to display types
	var mpDisplays []ui.MarketplaceDisplay
	for _, mp := range marketplaces {
		mpDisplays = append(mpDisplays, ui.MarketplaceDisplay{
			Name:        mp.Name,
			Source:      mp.Source,
			Repo:        mp.Repo,
			LastUpdated: mp.LastUpdated.Format("2006-01-02"),
		})
	}

	var pluginDisplays []ui.PluginDisplay
	for _, p := range plugins {
		status := "available"
		if p.Enabled {
			status = "enabled"
		} else if p.Installed {
			status = "installed"
		}
		pluginDisplays = append(pluginDisplays, ui.PluginDisplay{
			Name:        p.Name,
			Marketplace: p.Marketplace,
			FullName:    p.FullName,
			Description: p.Description,
			Status:      status,
			Version:     p.Version,
		})
	}

	state := ui.NewPluginsStateWithData(mpDisplays, pluginDisplays)
	state.ActiveTab = tab
	m.modal.Show(state)
}

// showCommitConflictModal shows the commit message modal for resolved merge conflicts.
func (m *Model) showCommitConflictModal() (tea.Model, tea.Cmd) {
	// Check if there are still conflicts
	conflictedFiles, err := git.GetConflictedFiles(m.pendingConflictRepoPath)
	if err != nil {
		m.chat.AppendStreaming(fmt.Sprintf("[Error checking conflicts: %v]\n", err))
		return m, nil
	}
	if len(conflictedFiles) > 0 {
		m.chat.AppendStreaming(fmt.Sprintf("[Still have %d conflicted files. Please resolve them first.]\n", len(conflictedFiles)))
		return m, nil
	}

	// Show commit message modal with "conflict" type to indicate this is conflict resolution
	m.modal.Show(ui.NewEditCommitState("Resolve merge conflicts", "conflict"))
	return m, nil
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
	m.header.SetBaseBranch(result.BaseBranch)
	if result.DiffStats != nil {
		m.header.SetDiffStats(&ui.DiffStats{
			FilesChanged: result.DiffStats.FilesChanged,
			Additions:    result.DiffStats.Additions,
			Deletions:    result.DiffStats.Deletions,
		})
	} else {
		m.header.SetDiffStats(nil)
	}
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

	// Restore pending plan approval
	if result.PlanApproval != nil {
		m.chat.SetPendingPlanApproval(result.PlanApproval.Plan, result.PlanApproval.AllowedPrompts)
	} else {
		m.chat.ClearPendingPlanApproval()
	}

	// Restore todo list
	if result.TodoList != nil {
		m.chat.SetTodoList(result.TodoList)
	} else {
		m.chat.ClearTodoList()
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

	// Restore queued message display if this session has a pending message
	if state := m.sessionState().GetIfExists(sess.ID); state != nil && state.PendingMessage != "" {
		pendingMsg := state.PendingMessage
		m.chat.SetQueuedMessage(pendingMsg)
	} else {
		m.chat.ClearQueuedMessage()
	}

	logger.Log("App: Session selected and focused: %s", sess.ID)
}

// handleImagePaste attempts to read an image from the clipboard and attach it
func (m *Model) handleImagePaste() (tea.Model, tea.Cmd) {
	logger.Debug("App: Handling image paste")

	// Try to read image from clipboard
	img, err := clipboard.ReadImage()
	if err != nil {
		logger.Debug("App: Failed to read image from clipboard: %v", err)
		// Don't show error to user - might just be text paste
		return m, nil
	}

	if img == nil {
		logger.Debug("App: No image in clipboard")
		// No image, let text paste happen normally
		return m, nil
	}

	// Validate the image
	if err := img.Validate(); err != nil {
		logger.Warn("App: Image validation failed: %v", err)
		// Show error message in chat
		m.chat.AppendStreaming(fmt.Sprintf("\n[Error: %s]\n", err.Error()))
		return m, nil
	}

	// Attach the image
	logger.Info("App: Attaching image: %dKB, %s", img.SizeKB(), img.MediaType)
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

	// Check for slash commands (only if no image attached)
	if !hasImage && strings.HasPrefix(input, "/") {
		result := m.handleSlashCommand(input)
		if result.Handled {
			m.chat.ClearInput()

			// Handle UI actions
			if result.Action != ActionNone {
				switch result.Action {
				case ActionOpenMCP:
					return shortcutMCPServers(m)
				case ActionOpenPlugins:
					return shortcutPlugins(m)
				}
			}

			// Display the command as user message and response
			m.chat.AddUserMessage(input)
			if result.Response != "" {
				m.chat.AddSystemMessage(result.Response)
			}
			return m, nil
		}
		// If not handled, fall through to send to Claude
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

	// Return commands to listen for session events plus UI ticks
	cmds := append(m.sessionListeners(sessionID, runner, responseChan),
		ui.SidebarTick(),
		ui.StopwatchTick(),
	)
	return m, tea.Batch(cmds...)
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
	if state := m.sessionState().GetIfExists(sessionID); state != nil {
		state.PendingPermission = nil
	}
	m.sidebar.SetPendingPermission(sessionID, false)
	m.chat.ClearPendingPermission()

	// Continue listening for session events
	return m, tea.Batch(m.sessionListeners(sessionID, runner, nil)...)
}

// sessionListeners returns all the listener commands for a session.
// This bundles response, permission, question, and plan approval listeners together
// so adding a new listener type only requires changing this one function.
// If responseChan is provided, it will be used instead of runner.GetResponseChan().
func (m *Model) sessionListeners(sessionID string, runner claude.RunnerInterface, responseChan <-chan claude.ResponseChunk) []tea.Cmd {
	if runner == nil {
		return nil
	}

	ch := responseChan
	if ch == nil {
		ch = runner.GetResponseChan()
	}

	return []tea.Cmd{
		m.listenForSessionResponse(sessionID, ch),
		m.listenForSessionPermission(sessionID, runner),
		m.listenForSessionQuestion(sessionID, runner),
		m.listenForSessionPlanApproval(sessionID, runner),
	}
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
func (m *Model) listenForSessionPermission(sessionID string, runner claude.RunnerInterface) tea.Cmd {
	if runner == nil {
		return nil
	}

	ch := runner.PermissionRequestChan()
	if ch == nil {
		// Runner has been stopped, don't create a goroutine that would block forever
		return nil
	}
	return func() tea.Msg {
		req, ok := <-ch
		if !ok {
			return nil
		}
		return PermissionRequestMsg{SessionID: sessionID, Request: req}
	}
}

// listenForSessionQuestion creates a command to listen for question requests from a specific session
func (m *Model) listenForSessionQuestion(sessionID string, runner claude.RunnerInterface) tea.Cmd {
	if runner == nil {
		return nil
	}

	ch := runner.QuestionRequestChan()
	if ch == nil {
		// Runner has been stopped, don't create a goroutine that would block forever
		return nil
	}
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

	state := m.sessionState().GetIfExists(sessionID)
	if state == nil || state.PendingQuestion == nil {
		logger.Log("App: No pending question for session %s", sessionID)
		return m, nil
	}
	req := state.PendingQuestion

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
	state.PendingQuestion = nil
	m.sidebar.SetPendingPermission(sessionID, false)
	m.chat.ClearPendingQuestion()

	// Continue listening for session events
	return m, tea.Batch(m.sessionListeners(sessionID, runner, nil)...)
}

// listenForSessionPlanApproval creates a command that waits for plan approval requests
func (m *Model) listenForSessionPlanApproval(sessionID string, runner claude.RunnerInterface) tea.Cmd {
	if runner == nil {
		return nil
	}

	ch := runner.PlanApprovalRequestChan()
	if ch == nil {
		// Runner has been stopped, don't create a goroutine that would block forever
		return nil
	}
	return func() tea.Msg {
		req, ok := <-ch
		if !ok {
			return nil
		}
		return PlanApprovalRequestMsg{SessionID: sessionID, Request: req}
	}
}

// submitPlanApprovalResponse sends the plan approval response back to Claude
func (m *Model) submitPlanApprovalResponse(sessionID string, approved bool) (tea.Model, tea.Cmd) {
	runner := m.sessionMgr.GetRunner(sessionID)
	if runner == nil {
		logger.Log("App: Plan approval response for unknown session %s", sessionID)
		return m, nil
	}

	state := m.sessionState().GetIfExists(sessionID)
	if state == nil || state.PendingPlanApproval == nil {
		logger.Log("App: No pending plan approval for session %s", sessionID)
		return m, nil
	}
	req := state.PendingPlanApproval

	logger.Log("App: Plan approval response for session %s: approved=%v", sessionID, approved)

	// Build response
	resp := mcp.PlanApprovalResponse{
		ID:       req.ID,
		Approved: approved,
	}

	// Send response
	runner.SendPlanApprovalResponse(resp)

	// Clear pending plan approval
	state.PendingPlanApproval = nil
	m.sidebar.SetPendingPermission(sessionID, false)
	m.chat.ClearPendingPlanApproval()

	// Continue listening for session events
	return m, tea.Batch(m.sessionListeners(sessionID, runner, nil)...)
}

func (m *Model) listenForMergeResult(sessionID string) tea.Cmd {
	state := m.sessionState().GetIfExists(sessionID)
	if state == nil || state.MergeChan == nil {
		return nil
	}
	ch := state.MergeChan

	return func() tea.Msg {
		result, ok := <-ch
		if !ok {
			return MergeResultMsg{SessionID: sessionID, Result: git.Result{Done: true}}
		}
		return MergeResultMsg{SessionID: sessionID, Result: result}
	}
}

// mergeConversationHistory appends child session's conversation history to parent's
func (m *Model) mergeConversationHistory(childSessionID, parentSessionID string) error {
	// Load child messages
	childMessages, err := config.LoadSessionMessages(childSessionID)
	if err != nil {
		return fmt.Errorf("failed to load child messages: %w", err)
	}

	// Load parent messages
	parentMessages, err := config.LoadSessionMessages(parentSessionID)
	if err != nil {
		return fmt.Errorf("failed to load parent messages: %w", err)
	}

	// Add a separator message to indicate the merge
	separatorMsg := config.Message{
		Role:    "assistant",
		Content: "\n---\n[Merged from child session]\n---\n",
	}

	// Combine: parent messages + separator + child messages
	combined := append(parentMessages, separatorMsg)
	combined = append(combined, childMessages...)

	// Save back to parent
	if err := config.SaveSessionMessages(parentSessionID, combined, config.MaxSessionMessageLines); err != nil {
		return fmt.Errorf("failed to save merged messages: %w", err)
	}

	return nil
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

// HasActiveStreaming returns true if any session is currently streaming (public for demos).
func (m *Model) HasActiveStreaming() bool {
	return m.sessionMgr.HasActiveStreaming()
}

// detectOptionsInSession scans the runner's messages for numbered options
func (m *Model) detectOptionsInSession(sessionID string, runner claude.RunnerInterface) {
	state := m.sessionState().GetOrCreate(sessionID)
	msgs := runner.GetMessages()
	if len(msgs) == 0 {
		state.DetectedOptions = nil
		return
	}

	// Find the last assistant message
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			options := DetectOptions(msgs[i].Content)
			if len(options) >= 2 {
				logger.Log("App: Detected %d options in session %s", len(options), sessionID)
				state.DetectedOptions = options
				return
			}
			break // Only check the most recent assistant message
		}
	}

	// No options found
	state.DetectedOptions = nil
}

// showExploreOptionsModal displays the modal for selecting options to explore in parallel
func (m *Model) showExploreOptionsModal() (tea.Model, tea.Cmd) {
	if m.activeSession == nil {
		return m, nil
	}

	state := m.sessionState().GetIfExists(m.activeSession.ID)
	if state == nil || !state.HasDetectedOptions() {
		return m, nil
	}
	options := state.DetectedOptions

	// Convert to UI option items
	items := make([]ui.OptionItem, len(options))
	for i, opt := range options {
		items[i] = ui.OptionItem{
			Number:     opt.Number,
			Letter:     opt.Letter,
			Text:       opt.Text,
			Selected:   false,
			GroupIndex: opt.GroupIndex,
		}
	}

	// Get parent session display name for consistent visual treatment
	parentDisplayName := ui.SessionDisplayName(m.activeSession.Branch, m.activeSession.Name)
	m.modal.Show(ui.NewExploreOptionsState(parentDisplayName, items))
	return m, nil
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
	// Skip for dev builds; fetch changelog from GitHub asynchronously
	if m.version != "" && m.version != "dev" {
		lastSeen := m.config.GetLastSeenVersion()
		if lastSeen != m.version {
			logger.Log("App: Fetching changelog from GitHub (version %s -> %s)", lastSeen, m.version)
			return m, m.fetchChangelog()
		}
	}

	return m, nil
}

// fetchChangelog creates a command to fetch changelog from GitHub asynchronously.
// Used at startup to show new changes since last seen version.
func (m *Model) fetchChangelog() tea.Cmd {
	return func() tea.Msg {
		entries, err := changelog.FetchReleases()
		return ChangelogFetchedMsg{
			Entries:  entries,
			Error:    err,
			ShowAll:  false,
			IsManual: false,
		}
	}
}

// fetchChangelogAll creates a command to fetch and show all changelog entries.
// Used when user manually requests the changelog via shortcut.
func (m *Model) fetchChangelogAll() tea.Cmd {
	return func() tea.Msg {
		entries, err := changelog.FetchReleases()
		return ChangelogFetchedMsg{
			Entries:  entries,
			Error:    err,
			ShowAll:  true,
			IsManual: true,
		}
	}
}

// handleChangelogFetchedMsg handles the fetched changelog entries
func (m *Model) handleChangelogFetchedMsg(msg ChangelogFetchedMsg) (tea.Model, tea.Cmd) {
	if msg.Error != nil {
		logger.Log("App: Failed to fetch changelog: %v", msg.Error)
		if !msg.IsManual {
			// Only update lastSeen on startup flow failures
			m.config.SetLastSeenVersion(m.version)
			m.config.Save()
		}
		return m, nil
	}

	var changes []changelog.Entry
	if msg.ShowAll {
		// Limit to 10 most recent releases for manual view
		changes = msg.Entries
		if len(changes) > 10 {
			changes = changes[:10]
		}
	} else {
		lastSeen := m.config.GetLastSeenVersion()
		changes = changelog.GetChangesSince(lastSeen, msg.Entries)
	}

	if len(changes) > 0 {
		logger.Log("App: Showing changelog modal (%d entries, showAll=%v)", len(changes), msg.ShowAll)
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
		if !msg.IsManual {
			// Update lastSeen only on startup flow
			m.config.SetLastSeenVersion(m.version)
			m.config.Save()
		}
		return m, nil
	}

	if !msg.IsManual {
		// No new changes on startup, just update last seen version
		m.config.SetLastSeenVersion(m.version)
		m.config.Save()
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
	var hasPendingPermission, hasPendingQuestion, isStreaming, hasDetectedOptions bool
	if m.activeSession != nil {
		if state := m.sessionState().GetIfExists(m.activeSession.ID); state != nil {
			hasPendingPermission = state.PendingPermission != nil
			hasPendingQuestion = state.PendingQuestion != nil
			isStreaming = state.StreamCancel != nil
			hasDetectedOptions = state.HasDetectedOptions()
		}
	}
	viewChangesMode := m.chat.IsInViewChangesMode()
	searchMode := m.sidebar.IsSearchMode()
	m.footer.SetContext(hasSession, sidebarFocused, hasPendingPermission, hasPendingQuestion, isStreaming, viewChangesMode, searchMode, hasDetectedOptions)

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
		v.SetContent(lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			modalView,
		))
		return v
	}

	v.SetContent(view)
	return v
}

// fetchGitHubIssues creates a command to fetch GitHub issues asynchronously
func (m *Model) fetchGitHubIssues(repoPath string) tea.Cmd {
	return func() tea.Msg {
		issues, err := git.FetchGitHubIssues(repoPath)
		return GitHubIssuesFetchedMsg{
			RepoPath: repoPath,
			Issues:   issues,
			Error:    err,
		}
	}
}

// =============================================================================
// Public Accessors (for demo/testing)
// =============================================================================

// ActiveSession returns the currently active session, or nil if none.
func (m *Model) ActiveSession() *config.Session {
	return m.activeSession
}

// SessionMgr returns the session manager.
func (m *Model) SessionMgr() *SessionManager {
	return m.sessionMgr
}

// RenderToString renders the current view as a string.
// This is useful for demos and testing.
func (m *Model) RenderToString() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// Update footer context for conditional bindings
	hasSession := m.sidebar.SelectedSession() != nil
	sidebarFocused := m.focus == FocusSidebar
	var hasPendingPermission, hasPendingQuestion, isStreaming, hasDetectedOptions bool
	if m.activeSession != nil {
		if state := m.sessionState().GetIfExists(m.activeSession.ID); state != nil {
			hasPendingPermission = state.PendingPermission != nil
			hasPendingQuestion = state.PendingQuestion != nil
			isStreaming = state.StreamCancel != nil
			hasDetectedOptions = state.HasDetectedOptions()
		}
	}
	viewChangesMode := m.chat.IsInViewChangesMode()
	searchMode := m.sidebar.IsSearchMode()
	m.footer.SetContext(hasSession, sidebarFocused, hasPendingPermission, hasPendingQuestion, isStreaming, viewChangesMode, searchMode, hasDetectedOptions)

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
		return lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			modalView,
		)
	}

	return view
}

// =============================================================================
// Flash Message Helpers
// =============================================================================

// ShowFlash displays a flash message in the footer and returns a command to start the auto-dismiss timer
func (m *Model) ShowFlash(text string, flashType ui.FlashType) tea.Cmd {
	m.footer.SetFlash(text, flashType)
	return ui.FlashTick()
}

// ShowFlashError displays an error flash message
func (m *Model) ShowFlashError(text string) tea.Cmd {
	return m.ShowFlash(text, ui.FlashError)
}

// ShowFlashWarning displays a warning flash message
func (m *Model) ShowFlashWarning(text string) tea.Cmd {
	return m.ShowFlash(text, ui.FlashWarning)
}

// ShowFlashInfo displays an info flash message
func (m *Model) ShowFlashInfo(text string) tea.Cmd {
	return m.ShowFlash(text, ui.FlashInfo)
}

// ShowFlashSuccess displays a success flash message
func (m *Model) ShowFlashSuccess(text string) tea.Cmd {
	return m.ShowFlash(text, ui.FlashSuccess)
}
