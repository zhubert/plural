package app

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/changelog"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/clipboard"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/issues"
	"github.com/zhubert/plural/internal/keys"
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
	claudeRunner  claude.RunnerInterface // Currently active runner (convenience reference)

	// Session lifecycle management
	sessionMgr *SessionManager

	// Service instances for dependency injection
	gitService     *git.GitService
	sessionService *session.SessionService
	issueRegistry  *issues.ProviderRegistry

	// State machine
	state AppState // Current application state

	// Window focus state for notifications
	windowFocused bool // Whether the terminal window is focused

	// Pending commit message editing state (nil when inactive)
	pendingCommit *PendingCommit

	// Pending conflict resolution state (nil when inactive)
	pendingConflict *PendingConflict

	// Pending container action to execute after async prerequisite checks pass (nil when inactive)
	pendingContainerAction func() (tea.Model, tea.Cmd)

	// Terminal capability flags
	kittyKeyboard bool // Terminal supports Kitty keyboard protocol (Shift+Enter distinguishable)
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
// Deprecated: use IssuesFetchedMsg instead
type GitHubIssuesFetchedMsg struct {
	RepoPath string
	Issues   []git.GitHubIssue
	Error    error
}

// IssuesFetchedMsg is sent when issues/tasks have been fetched from any source
type IssuesFetchedMsg struct {
	RepoPath  string
	Source    string // "github" or "asana"
	ProjectID string // Asana project GID (only for Asana)
	Issues    []issues.Issue
	Error     error
}

// ReviewCommentsFetchedMsg is sent when PR review comments have been fetched
type ReviewCommentsFetchedMsg struct {
	SessionID string
	Branch    string
	Comments  []git.PRReviewComment
	Error     error
}

// ChangelogFetchedMsg is sent when changelog has been fetched from GitHub
type ChangelogFetchedMsg struct {
	Entries  []changelog.Entry
	Error    error
	ShowAll  bool // If true, show all entries; if false, filter by lastSeen version
	IsManual bool // If true, this was triggered by user shortcut (don't update lastSeen)
}

// AsanaProjectsFetchedMsg is sent when Asana projects have been fetched
type AsanaProjectsFetchedMsg struct {
	Projects []issues.AsanaProject
	Error    error
}

// ContainerPrereqCheckMsg is sent when async container prerequisite checks complete
type ContainerPrereqCheckMsg struct {
	Result process.ContainerPrerequisites
}

// SessionCompletedMsg is emitted when an autonomous session finishes a response
// with no pending interactions. This is the primitive all automation hooks into.
type SessionCompletedMsg struct {
	SessionID string
}

// SessionPipelineCompleteMsg is emitted when a session's full pipeline completes
// (after tests pass or max retries exhausted).
type SessionPipelineCompleteMsg struct {
	SessionID   string
	TestsPassed bool
}

// AutonomousLimitReachedMsg is sent when an autonomous session hits its turn or duration limit.
type AutonomousLimitReachedMsg struct {
	SessionID string
	Reason    string // "turn_limit" or "duration_limit"
}

// CreateChildRequestMsg is sent when the supervisor's MCP tool create_child_session is called
type CreateChildRequestMsg struct {
	SessionID string
	Request   mcp.CreateChildRequest
}

// ListChildrenRequestMsg is sent when the supervisor's MCP tool list_child_sessions is called
type ListChildrenRequestMsg struct {
	SessionID string
	Request   mcp.ListChildrenRequest
}

// MergeChildRequestMsg is sent when the supervisor's MCP tool merge_child_to_parent is called
type MergeChildRequestMsg struct {
	SessionID string
	Request   mcp.MergeChildRequest
}

// MergeChildCompleteMsg is sent when a child-to-parent merge operation completes
type MergeChildCompleteMsg struct {
	SessionID string // Supervisor session ID
	ChildID   string
	Success   bool
	Message   string
	Error     error
}

// CreatePRRequestMsg is sent when the automated supervisor's MCP tool create_pr is called
type CreatePRRequestMsg struct {
	SessionID string
	Request   mcp.CreatePRRequest
}

// PushBranchRequestMsg is sent when the automated supervisor's MCP tool push_branch is called
type PushBranchRequestMsg struct {
	SessionID string
	Request   mcp.PushBranchRequest
}

// ContainerImageUpdateMsg is sent when the background container image update check completes
type ContainerImageUpdateMsg struct {
	NeedsUpdate bool
	Image       string
}

// New creates a new app model
func New(cfg *config.Config, version string) *Model {
	// Load saved theme from config, or use default
	if savedTheme := cfg.GetTheme(); savedTheme != "" {
		ui.SetThemeByName(savedTheme)
	}

	gitSvc := git.NewGitService()
	sessionSvc := session.NewSessionService()

	// Initialize issue providers
	githubProvider := issues.NewGitHubProvider(gitSvc)
	asanaProvider := issues.NewAsanaProvider(cfg)
	issueRegistry := issues.NewProviderRegistry(githubProvider, asanaProvider)

	m := &Model{
		config:         cfg,
		version:        version,
		header:         ui.NewHeader(),
		footer:         ui.NewFooter(),
		sidebar:        ui.NewSidebar(),
		chat:           ui.NewChat(),
		modal:          ui.NewModal(),
		focus:          FocusSidebar,
		sessionMgr:     NewSessionManager(cfg, gitSvc),
		gitService:     gitSvc,
		sessionService: sessionSvc,
		issueRegistry:  issueRegistry,
		state:          StateIdle,
		windowFocused:  true, // Assume window is focused on startup
	}

	// Load repos and sessions into sidebar (filtered by active workspace)
	m.sidebar.SetRepos(cfg.GetRepos())
	m.sidebar.SetSessions(m.getFilteredSessions())
	m.sidebar.SetFocused(true)

	// Set workspace name in header
	m.header.SetWorkspaceName(m.getActiveWorkspaceName())

	// Restore preview state from config (in case app was closed during a preview)
	if cfg.IsPreviewActive() {
		m.header.SetPreviewActive(true)
	}

	return m
}

// Close gracefully shuts down all Claude sessions and releases resources.
// This should be called when the application is exiting.
func (m *Model) Close() {
	logger.Get().Info("closing and shutting down all sessions")
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
	// Check if the active session is currently waiting for a response, has a merge in progress,
	// or has a container initializing. Each session can operate independently.
	state := m.sessionMgr.StateManager().GetIfExists(m.activeSession.ID)
	if state == nil {
		return true // No state means not waiting, merging, or initializing
	}
	return !state.GetIsWaiting() && !state.IsMerging() && !state.GetContainerInitializing()
}

// setState transitions to a new state with logging
func (m *Model) setState(newState AppState) {
	if m.state != newState {
		logger.Get().Debug("state transition", "from", m.state.String(), "to", newState.String())
		m.state = newState
	}
}

// sessionState returns the session state manager (convenience accessor)
func (m *Model) sessionState() *SessionStateManager {
	return m.sessionMgr.StateManager()
}

// getFilteredSessions returns sessions filtered by the active workspace.
// If no workspace is active, returns all sessions.
func (m *Model) getFilteredSessions() []config.Session {
	sessions := m.config.GetSessions()
	activeWS := m.config.GetActiveWorkspaceID()
	if activeWS == "" {
		return sessions
	}
	var filtered []config.Session
	for _, s := range sessions {
		if s.WorkspaceID == activeWS {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// getActiveWorkspaceName returns the name of the active workspace, or empty if none.
func (m *Model) getActiveWorkspaceName() string {
	activeWS := m.config.GetActiveWorkspaceID()
	if activeWS == "" {
		return ""
	}
	for _, ws := range m.config.GetWorkspaces() {
		if ws.ID == activeWS {
			return ws.Name
		}
	}
	return ""
}

// refreshDiffStats updates the header with current git diff statistics for the active session
func (m *Model) refreshDiffStats() {
	if m.activeSession == nil || m.activeSession.WorkTree == "" {
		m.header.SetDiffStats(nil)
		return
	}

	ctx := context.Background()
	gitStats, err := m.gitService.GetDiffStats(ctx, m.activeSession.WorkTree)
	if err != nil {
		logger.Get().Debug("failed to refresh diff stats", "error", err)
		m.header.SetDiffStats(nil)
		return
	}

	m.header.SetDiffStats(&ui.DiffStats{
		FilesChanged: gitStats.FilesChanged,
		Additions:    gitStats.Additions,
		Deletions:    gitStats.Deletions,
	})

	// Update sidebar attention state for uncommitted changes
	if m.activeSession != nil {
		m.sidebar.SetUncommittedChanges(m.activeSession.ID, gitStats.FilesChanged > 0)
	}
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		// Trigger startup modal check (welcome or changelog)
		func() tea.Msg {
			return StartupModalMsg{}
		},
		// Start background PR merge detection polling
		PRPollTick(),
		// Start background issue polling
		IssuePollTick(),
		// Check for container image updates in the background
		m.checkContainerImageUpdate(),
	)
}

// checkContainerImageUpdate returns a command that checks if the container image
// has an update available in the remote registry. Silently returns an empty message
// on any failure (no CLI, no image, network error, etc.).
func (m *Model) checkContainerImageUpdate() tea.Cmd {
	image := m.config.GetContainerImage()
	return func() tea.Msg {
		needsUpdate, err := process.CheckContainerImageUpdate(image)
		if err != nil {
			logger.Get().Debug("container image update check failed", "error", err)
			return ContainerImageUpdateMsg{}
		}
		return ContainerImageUpdateMsg{NeedsUpdate: needsUpdate, Image: image}
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
		logger.Get().Debug("window focused")

	case tea.BlurMsg:
		m.windowFocused = false
		logger.Get().Debug("window blurred")

	case tea.KeyboardEnhancementsMsg:
		m.kittyKeyboard = msg.SupportsKeyDisambiguation()
		logger.Get().Debug("keyboard enhancements detected", "kitty", m.kittyKeyboard, "flags", msg.Flags)

	case tea.PasteStartMsg:
		// Handle paste events - check for images in clipboard when paste starts
		// Terminals intercept Ctrl+V and send paste events instead of key presses
		logger.Get().Debug("paste start received", "focus", m.focus, "hasActiveSession", m.activeSession != nil)
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
		if len(preview) > ui.PasteContentPreviewLen {
			preview = preview[:ui.PasteContentPreviewLen] + "..."
		}
		logger.Get().Debug("paste received", "length", len(content), "preview", preview)

	case tea.KeyPressMsg:
		logger.Get().Debug("key press received", "key", msg.String(), "focus", m.focus, "modalVisible", m.modal.IsVisible())

		// Handle modal first if visible
		if m.modal.IsVisible() {
			return m.handleModalKey(msg)
		}

		// Handle Escape to exit multi-select mode, search mode, view changes mode, log viewer, or interrupt streaming
		if msg.String() == keys.Escape {
			// First check if sidebar is in multi-select mode
			if m.sidebar.IsMultiSelectMode() {
				m.sidebar.ExitMultiSelect()
				return m, nil
			}
			// Then check if sidebar is in search mode
			if m.sidebar.IsSearchMode() {
				m.sidebar.ExitSearchMode()
				return m, nil
			}
			// Check if view changes mode is active (regardless of focus)
			if m.chat.IsInViewChangesMode() {
				m.chat.ExitViewChangesMode()
				return m, nil
			}
			// Check if log viewer is active (regardless of focus)
			if m.chat.IsInLogViewerMode() {
				m.chat.ExitLogViewerMode()
				return m, nil
			}
			// Then check for streaming interruption
			if m.activeSession != nil {
				if state := m.sessionState().GetIfExists(m.activeSession.ID); state != nil {
					if cancel := state.GetStreamCancel(); cancel != nil {
						logger.WithSession(m.activeSession.ID).Debug("interrupting streaming")
						cancel()
						// Send SIGINT to interrupt the Claude process (handles sub-agent work)
						if m.claudeRunner != nil {
							if err := m.claudeRunner.Interrupt(); err != nil {
								logger.WithSession(m.activeSession.ID).Error("failed to interrupt Claude", "error", err)
							}
						}
						m.sessionState().StopWaiting(m.activeSession.ID)
						m.sidebar.SetStreaming(m.activeSession.ID, false)
						m.chat.SetWaiting(false)
						// Save partial response to runner before finishing
						var saveErr error
						if content := m.chat.GetStreaming(); content != "" {
							m.claudeRunner.AddAssistantMessage(content + "\n[Interrupted]")
							saveErr = m.sessionMgr.SaveRunnerMessages(m.activeSession.ID, m.claudeRunner)
						}
						m.chat.AppendStreaming("\n[Interrupted]\n")
						m.chat.FinishStreaming()
						// Check if any sessions are still streaming
						if !m.hasAnyStreamingSessions() {
							m.setState(StateIdle)
						}
						if saveErr != nil {
							return m, m.ShowFlashError("Failed to save session messages")
						}
						return m, nil
					}
				}
			}
		}

		// Handle chat-focused keys when chat is focused with an active session
		if m.focus == FocusChat && m.activeSession != nil {
			key := msg.String()

			// Permission response
			state := m.sessionState().GetIfExists(m.activeSession.ID)
			if state != nil {
				if req := state.GetPendingPermission(); req != nil {
					switch key {
					case "y", "Y", "n", "N", "a", "A":
						return m.handlePermissionResponse(key, m.activeSession.ID, req)
					}
				}
			}

			// Question response (reuse state from permission check)
			if state != nil && state.GetPendingQuestion() != nil {
				switch key {
				case "1", "2", "3", "4", "5":
					num := int(key[0] - '0')
					if m.chat.SelectOptionByNumber(num) {
						return m.submitQuestionResponse(m.activeSession.ID)
					}
					return m, nil
				case keys.Up, "k":
					m.chat.MoveQuestionSelection(-1)
					return m, nil
				case keys.Down, "j":
					m.chat.MoveQuestionSelection(1)
					return m, nil
				case keys.Enter:
					if m.chat.SelectCurrentOption() {
						return m.submitQuestionResponse(m.activeSession.ID)
					}
					return m, nil
				}
			}

			// Plan approval response (reuse state from permission check)
			if state != nil && state.GetPendingPlanApproval() != nil {
				switch key {
				case "y", "Y":
					return m.submitPlanApprovalResponse(m.activeSession.ID, true)
				case "n", "N":
					return m.submitPlanApprovalResponse(m.activeSession.ID, false)
				case keys.Up, "k":
					m.chat.ScrollPlan(-3)
					return m, nil
				case keys.Down, "j":
					m.chat.ScrollPlan(3)
					return m, nil
				}
			}

			// Ctrl+V for image pasting (fallback for terminals that send raw key presses)
			if key == keys.CtrlV {
				return m.handleImagePaste()
			}

			// Ctrl+O for parallel option exploration (reuse state from permission check)
			if key == keys.CtrlO && state != nil && state.HasDetectedOptions() {
				return m.showExploreOptionsModal()
			}

			// Backspace to remove pending image when input is empty
			if key == keys.Backspace && m.chat.HasPendingImage() && m.chat.GetInput() == "" {
				m.chat.ClearImage()
				return m, nil
			}
		}

		// Global keys
		key := msg.String()

		// Handle ctrl+c specially - always quits
		if key == keys.CtrlC {
			return m, tea.Quit
		}

		// Handle multi-select mode keys when sidebar is focused
		if m.sidebar.IsMultiSelectMode() && m.focus == FocusSidebar {
			switch key {
			case keys.Escape:
				m.sidebar.ExitMultiSelect()
				return m, nil
			case keys.Space:
				m.sidebar.ToggleSelected()
				return m, nil
			case "a":
				m.sidebar.SelectAll()
				return m, nil
			case "n":
				m.sidebar.DeselectAll()
				return m, nil
			case keys.Enter:
				ids := m.sidebar.GetSelectedSessionIDs()
				if len(ids) > 0 {
					workspaces := m.config.GetWorkspaces()
					m.modal.Show(ui.NewBulkActionState(ids, workspaces))
				}
				return m, nil
			case keys.Up, "k", keys.Down, "j":
				// Let sidebar handle navigation
				m.sidebar, _ = m.sidebar.Update(msg)
				return m, nil
			case "?":
				// Allow help modal in multi-select mode
				result, cmd := shortcutHelp(m)
				return result, cmd
			}
			// Block other keys in multi-select mode
			return m, nil
		}

		// Try executing from shortcut registry
		if result, cmd, handled := m.ExecuteShortcut(key); handled {
			return result, cmd
		}

		// Handle special cases not in the registry
		switch key {
		case keys.Enter:
			switch m.focus {
			case FocusSidebar:
				// "+ New Session" action
				if repoPath := m.sidebar.SelectedNewSessionRepo(); repoPath != "" {
					state := ui.NewNewSessionState(m.config.GetRepos(), process.ContainersSupported(), claude.ContainerAuthAvailable())
					state.LockedRepo = repoPath
					state.Focus = 1 // Skip repo selector, start on base branch
					m.modal.Show(state)
					return m, nil
				}
				// Select session
				if sess := m.sidebar.SelectedSession(); sess != nil {
					m.selectSession(sess)
					// Check if this session has an unsent initial message (from issue import)
					if initialMsg := m.sessionState().GetInitialMessage(sess.ID); initialMsg != "" {
						m.sessionState().GetOrCreate(sess.ID).SetPendingMsg(initialMsg)
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
					if sessState != nil && sessState.GetIsWaiting() {
						input := m.chat.GetInput()
						if input != "" {
							sessState.SetPendingMsg(input)
							m.chat.ClearInput()
							m.chat.SetQueuedMessage(input)
							logger.WithSession(m.activeSession.ID).Debug("queued message while streaming")
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
			logger.Get().Error("commit message generation failed", "error", msg.Error)
			m.modal.Hide()
			m.chat.AppendStreaming(fmt.Sprintf("Failed to generate commit message: %v\n", msg.Error))
			m.pendingCommit = nil
			return m, nil
		}

		// Show the edit commit modal with the generated message
		commitType := MergeTypeNone
		if m.pendingCommit != nil {
			commitType = m.pendingCommit.Type
		}
		m.modal.Show(ui.NewEditCommitState(msg.Message, commitType.String()))
		return m, nil

	case SendPendingMessageMsg:
		return m.handleSendPendingMessageMsg(msg)

	case MergeResultMsg:
		return m.handleMergeResultMsg(msg)

	case GitHubIssuesFetchedMsg:
		return m.handleGitHubIssuesFetchedMsg(msg)

	case IssuesFetchedMsg:
		return m.handleIssuesFetchedMsg(msg)

	case ReviewCommentsFetchedMsg:
		return m.handleReviewCommentsFetchedMsg(msg)

	case ChangelogFetchedMsg:
		return m.handleChangelogFetchedMsg(msg)

	case AsanaProjectsFetchedMsg:
		return m.handleAsanaProjectsFetchedMsg(msg)

	case ContainerPrereqCheckMsg:
		return m.handleContainerPrereqCheckMsg(msg)

	case SessionCompletedMsg:
		return m.handleSessionCompletedMsg(msg)

	case SessionPipelineCompleteMsg:
		return m.handleSessionPipelineCompleteMsg(msg)

	case AutonomousLimitReachedMsg:
		return m.handleAutonomousLimitReachedMsg(msg)

	case AutoPRCommentsFetchedMsg:
		return m.handleAutoPRCommentsFetchedMsg(msg)

	case CIPollResultMsg:
		return m.handleCIPollResultMsg(msg)

	case AutoMergeResultMsg:
		return m.handleAutoMergeResultMsg(msg)

	case CreateChildRequestMsg:
		return m.handleCreateChildRequestMsg(msg)

	case ListChildrenRequestMsg:
		return m.handleListChildrenRequestMsg(msg)

	case MergeChildRequestMsg:
		return m.handleMergeChildRequestMsg(msg)

	case MergeChildCompleteMsg:
		return m.handleMergeChildCompleteMsg(msg)

	case CreatePRRequestMsg:
		return m.handleCreatePRRequestMsg(msg)

	case PushBranchRequestMsg:
		return m.handlePushBranchRequestMsg(msg)

	case ContainerImageUpdateMsg:
		if msg.NeedsUpdate {
			return m, m.ShowFlashWarning(fmt.Sprintf("Container image update available — run: docker pull %s", msg.Image))
		}
		return m, nil

	case PRCreatedFromToolMsg:
		return m.handlePRCreatedFromToolMsg(msg)

	case PRPollTickMsg:
		// Re-schedule next tick and check PR statuses for eligible sessions
		checkCmd := checkPRStatuses(m.config.GetSessions(), m.gitService)
		if checkCmd != nil {
			return m, tea.Batch(PRPollTick(), checkCmd)
		}
		return m, PRPollTick()

	case PRBatchStatusCheckMsg:
		return m.handlePRBatchStatusCheckMsg(msg)

	case IssuePollTickMsg:
		checkCmd := checkForNewIssues(m.config, m.gitService, m.config.GetSessions())
		if checkCmd != nil {
			return m, tea.Batch(IssuePollTick(), checkCmd)
		}
		return m, IssuePollTick()

	case NewIssuesDetectedMsg:
		return m.handleNewIssuesDetectedMsg(msg)

	case StartupModalMsg:
		return m.handleStartupModals()

	case ui.HelpShortcutTriggeredMsg:
		// Handle shortcut triggered from help modal
		return m.handleHelpShortcutTrigger(msg.Key)

	case TerminalErrorMsg:
		// Show terminal error as flash message
		m.footer.SetFlash(msg.Error, ui.FlashError)
		cmds = append(cmds, ui.FlashTick())
		return m, tea.Batch(cmds...)
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
		// Update chat panel spinner if waiting
		chat, cmd := m.chat.Update(msg)
		m.chat = chat
		cmds = append(cmds, cmd)
		// Also update loading commit modal spinner if visible
		if loadingState, ok := m.modal.State.(*ui.LoadingCommitState); ok {
			loadingState.AdvanceSpinner()
			cmds = append(cmds, ui.StopwatchTick())
		}
		return m, tea.Batch(cmds...)
	case ui.SelectionCopyMsg:
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
			case keys.PgUp, keys.PgDown, keys.CtrlU, keys.CtrlD, keys.Home, keys.End:
				chat, cmd := m.chat.Update(msg)
				m.chat = chat
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
		}
		// Route mouse wheel events to chat panel for scrolling (no coordinate adjustment needed)
		if mouseMsg, isMouse := msg.(tea.MouseWheelMsg); isMouse {
			if mouseMsg.X > m.sidebar.Width() {
				chat, cmd := m.chat.Update(msg)
				m.chat = chat
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
		}
		// Route mouse click/motion/release events to chat panel for text selection
		if _, cmd, handled := m.routeMouseToChat(msg); handled {
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}
	}

	// Handle mouse events when chat is focused - adjust coordinates for sidebar and header
	if m.focus == FocusChat && m.activeSession != nil {
		if _, cmd, handled := m.routeMouseToChat(msg); handled {
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
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
	ctx := context.Background()
	repoPath := ""
	if m.pendingConflict != nil {
		repoPath = m.pendingConflict.RepoPath
	}
	conflictedFiles, err := m.gitService.GetConflictedFiles(ctx, repoPath)
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
	// Show preview indicator if this session is being previewed
	m.header.SetPreviewActive(m.config.GetPreviewSessionID() == sess.ID)
	// Show container indicator if this session is containerized
	m.header.SetContainerActive(sess.Containerized)
	if result.DiffStats != nil {
		m.header.SetDiffStats(&ui.DiffStats{
			FilesChanged: result.DiffStats.FilesChanged,
			Additions:    result.DiffStats.Additions,
			Deletions:    result.DiffStats.Deletions,
		})
		m.sidebar.SetUncommittedChanges(sess.ID, result.DiffStats.FilesChanged > 0)
	} else {
		m.header.SetDiffStats(nil)
		m.sidebar.SetUncommittedChanges(sess.ID, false)
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

	// Restore container initialization state
	if result.ContainerInitializing {
		m.chat.SetContainerInitializing(true, result.ContainerInitStart)
	} else {
		m.chat.SetContainerInitializing(false, time.Time{})
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

	// Restore subagent indicator
	m.chat.SetSubagentModel(result.SubagentModel)

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

	// Detect options in the session's messages (for Ctrl+P fork feature)
	// This ensures options are detected when returning to a session, not just when streaming completes
	m.detectOptionsInSession(sess.ID, result.Runner)

	// Restore queued message display if this session has a pending message
	if state := m.sessionState().GetIfExists(sess.ID); state != nil {
		if pendingMsg := state.GetPendingMsg(); pendingMsg != "" {
			m.chat.SetQueuedMessage(pendingMsg)
		} else {
			m.chat.ClearQueuedMessage()
		}
	} else {
		m.chat.ClearQueuedMessage()
	}

	logger.WithSession(sess.ID).Debug("session selected and focused")
}

// handleImagePaste attempts to read an image from the clipboard and attach it
func (m *Model) handleImagePaste() (tea.Model, tea.Cmd) {
	logger.Get().Debug("handling image paste")

	// Try to read image from clipboard
	img, err := clipboard.ReadImage()
	if err != nil {
		logger.Get().Debug("failed to read image from clipboard", "error", err)
		// Don't show error to user - might just be text paste
		return m, nil
	}

	if img == nil {
		logger.Get().Debug("no image in clipboard")
		// No image, let text paste happen normally
		return m, nil
	}

	// Validate the image
	if err := img.Validate(); err != nil {
		logger.Get().Warn("image validation failed", "error", err)
		// Show error message in chat
		m.chat.AppendStreaming(fmt.Sprintf("\n[Error: %s]\n", err.Error()))
		return m, nil
	}

	// Attach the image
	logger.Get().Info("attaching image", "sizeKB", img.SizeKB(), "mediaType", img.MediaType)
	m.chat.AttachImage(img.Data, img.MediaType)

	return m, nil
}

func (m *Model) sendMessage() (tea.Model, tea.Cmd) {
	input := m.chat.GetInput()
	hasImage := m.chat.HasPendingImage()
	logger.Get().Debug("sendMessage called", "inputLen", len(input), "hasImage", hasImage, "canSend", m.CanSendMessage())

	// Check for "exit" command (case-insensitive, no image)
	if !hasImage && strings.TrimSpace(strings.ToLower(input)) == "exit" {
		m.chat.ClearInput()
		return m.handleExitCommand()
	}

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
	if len(inputPreview) > ui.InputMessagePreviewLen {
		inputPreview = inputPreview[:ui.InputMessagePreviewLen] + "..."
	}
	logger.WithSession(m.activeSession.ID).Debug("sending message", "preview", inputPreview, "hasImage", hasImage)

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
	m.sidebar.SetIdleWithResponse(sessionID, false)
	m.setState(StateStreamingClaude)

	// For containerized sessions that haven't started yet, mark as initializing
	// The callback set in SessionManager.GetOrCreateRunner will clear this when the init message arrives
	if m.activeSession.Containerized && !m.activeSession.Started {
		m.sessionState().StartContainerInit(sessionID)
		m.chat.SetContainerInitializing(true, time.Now())
	}

	// Start Claude request with content blocks
	responseChan := runner.SendContent(ctx, content)

	// Return commands to listen for session events plus UI ticks
	cmds := append(m.sessionListeners(sessionID, runner, responseChan),
		ui.SidebarTick(),
		ui.StopwatchTick(),
	)
	return m, tea.Batch(cmds...)
}

// handleExitCommand handles the "exit" text command.
// If no sessions are currently streaming, it exits immediately.
// If sessions are streaming, it shows a confirmation modal.
func (m *Model) handleExitCommand() (tea.Model, tea.Cmd) {
	log := logger.Get()

	// Check if any sessions are actively streaming (waiting for Claude response)
	if !m.sessionMgr.HasActiveStreaming() {
		log.Info("no active streaming sessions, exiting immediately")
		return m, tea.Quit
	}

	// Count streaming sessions for the modal message
	streamingCount := 0
	for _, runner := range m.sessionMgr.GetRunners() {
		if runner.IsStreaming() {
			streamingCount++
		}
	}

	// Show confirmation modal
	log.Debug("showing exit confirmation modal", "streamingCount", streamingCount)
	m.modal.Show(ui.NewConfirmExitState(streamingCount))
	return m, nil
}

// Note: Permission, question, plan approval, and merge result handling has been
// moved to listeners.go for better organization.

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
	gitSvc := m.gitService
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		msg, err := gitSvc.GenerateCommitMessageWithClaude(ctx, worktreePath)
		if err != nil {
			// Fall back to simple message
			msg, err = gitSvc.GenerateCommitMessage(ctx, worktreePath)
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
		state.SetDetectedOptions(nil)
		return
	}

	// Find the last assistant message
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			options := DetectOptions(msgs[i].Content)
			if len(options) >= 2 {
				logger.WithSession(sessionID).Debug("detected options", "count", len(options))
				state.SetDetectedOptions(options)
				return
			}
			break // Only check the most recent assistant message
		}
	}

	// No options found
	state.SetDetectedOptions(nil)
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
	options := state.GetDetectedOptions()

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
	// Priority 0: Preview mode warning (highest priority - user needs to know immediately)
	if m.config.IsPreviewActive() {
		sessionID := m.config.GetPreviewSessionID()
		if sess := m.config.GetSession(sessionID); sess != nil {
			logger.Get().Debug("showing preview active modal on startup", "session", sess.Name)
			m.modal.Show(ui.NewPreviewActiveState(sess.Name, sess.Branch))
			return m, nil
		}
	}

	// Priority 1: Welcome modal for first-time users
	if !m.config.HasSeenWelcome() {
		logger.Get().Debug("showing welcome modal for first-time user")
		m.modal.Show(ui.NewWelcomeState())
		return m, nil
	}

	// Priority 2: Changelog modal for new versions
	// Skip for dev builds; fetch changelog from GitHub asynchronously
	if m.version != "" && m.version != "dev" {
		lastSeen := m.config.GetLastSeenVersion()
		if lastSeen != m.version {
			logger.Get().Debug("fetching changelog from GitHub", "lastSeen", lastSeen, "current", m.version)
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
		logger.Get().Warn("failed to fetch changelog", "error", msg.Error)
		if !msg.IsManual {
			// Only update lastSeen on startup flow failures
			m.config.SetLastSeenVersion(m.version)
			if err := m.config.Save(); err != nil {
				logger.Get().Warn("failed to save last-seen version after changelog fetch error", "error", err)
			}
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
		logger.Get().Debug("showing changelog modal", "entries", len(changes), "showAll", msg.ShowAll)
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
			if err := m.config.Save(); err != nil {
				logger.Get().Warn("failed to save last-seen version after showing changelog", "error", err)
			}
		}
		return m, nil
	}

	if !msg.IsManual {
		// No new changes on startup, just update last seen version
		m.config.SetLastSeenVersion(m.version)
		if err := m.config.Save(); err != nil {
			logger.Get().Warn("failed to save last-seen version", "error", err)
		}
	}
	return m, nil
}

// handleAsanaProjectsFetchedMsg handles the fetched Asana projects for the settings modal.
func (m *Model) handleAsanaProjectsFetchedMsg(msg AsanaProjectsFetchedMsg) (tea.Model, tea.Cmd) {
	// Convert issues.AsanaProject to ui.AsanaProjectOption and prepend "(none)" entry
	options := make([]ui.AsanaProjectOption, 0, len(msg.Projects)+1)
	options = append(options, ui.AsanaProjectOption{GID: "", Name: "(none)"})
	for _, p := range msg.Projects {
		options = append(options, ui.AsanaProjectOption{GID: p.GID, Name: p.Name})
	}

	// Deliver to whichever settings modal is currently open
	switch state := m.modal.State.(type) {
	case *ui.RepoSettingsState:
		if msg.Error != nil {
			state.SetAsanaProjectsError("Failed to fetch projects: " + msg.Error.Error())
		} else {
			state.SetAsanaProjects(options)
		}
	default:
		// Modal closed or wrong type - ignore
	}
	return m, nil
}

// fetchAsanaProjects creates a command to fetch Asana projects asynchronously.
func (m *Model) fetchAsanaProjects() tea.Cmd {
	registry := m.issueRegistry
	return func() tea.Msg {
		provider := registry.GetProvider(issues.SourceAsana)
		if provider == nil {
			return AsanaProjectsFetchedMsg{
				Error: fmt.Errorf("Asana provider not available"),
			}
		}
		asanaProvider, ok := provider.(*issues.AsanaProvider)
		if !ok {
			return AsanaProjectsFetchedMsg{
				Error: fmt.Errorf("Asana provider type assertion failed"),
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		projects, err := asanaProvider.FetchProjects(ctx)
		return AsanaProjectsFetchedMsg{
			Projects: projects,
			Error:    err,
		}
	}
}

// Note: updateSizes() and View() have been moved to view.go for better organization.

// fetchIssues creates a command to fetch issues/tasks from any source asynchronously
func (m *Model) fetchIssues(repoPath, source, projectID string) tea.Cmd {
	registry := m.issueRegistry
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		provider := registry.GetProvider(issues.Source(source))
		if provider == nil {
			return IssuesFetchedMsg{
				RepoPath: repoPath,
				Source:   source,
				Error:    fmt.Errorf("unknown issue source: %s", source),
			}
		}

		fetchedIssues, err := provider.FetchIssues(ctx, repoPath, projectID)
		return IssuesFetchedMsg{
			RepoPath:  repoPath,
			Source:    source,
			ProjectID: projectID,
			Issues:    fetchedIssues,
			Error:     err,
		}
	}
}

// fetchReviewComments creates a command to fetch PR review comments asynchronously
func (m *Model) fetchReviewComments(sessionID, repoPath, branch string) tea.Cmd {
	gitSvc := m.gitService
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		comments, err := gitSvc.FetchPRReviewComments(ctx, repoPath, branch)
		return ReviewCommentsFetchedMsg{
			SessionID: sessionID,
			Branch:    branch,
			Comments:  comments,
			Error:     err,
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

// SetGitService sets the git service (for testing/demos).
func (m *Model) SetGitService(svc *git.GitService) {
	m.gitService = svc
	m.sessionMgr.SetGitService(svc)
}

// SetSessionService sets the session service (for testing/demos).
func (m *Model) SetSessionService(svc *session.SessionService) {
	m.sessionService = svc
}

// Note: RenderToString(), flash message helpers, and mouse routing have been
// moved to view.go and flash.go for better organization.
