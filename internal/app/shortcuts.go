package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/google/uuid"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/keys"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/process"
	"github.com/zhubert/plural/internal/ui"
)

// Shortcut represents a keyboard shortcut with its metadata and handler.
// This is the single source of truth for all shortcuts in the application.
type Shortcut struct {
	Key             string                              // The key binding (e.g., "n", "ctrl+f")
	DisplayKey      string                              // Display name in help (e.g., "Ctrl+F"); defaults to Key
	Description     string                              // Human-readable description
	Category        string                              // Section for help modal grouping
	RequiresSession bool                                // Must have session selected
	RequiresSidebar bool                                // Must not be in chat focus
	Handler         func(m *Model) (tea.Model, tea.Cmd) // Action to perform
	Condition       func(m *Model) bool                 // Optional extra condition
}

// Categories for organizing shortcuts in the help modal
const (
	CategoryNavigation    = "Navigation"
	CategorySessions      = "Sessions"
	CategoryGit           = "Git Operations"
	CategoryConfiguration = "Configuration"
	CategoryChat          = "Chat (when focused)"
	CategoryPermissions   = "Permissions (when prompted)"
	CategoryGeneral       = "General"
)

// categoryOrder defines the display order of categories in the help modal
var categoryOrder = []string{
	CategoryNavigation,
	CategorySessions,
	CategoryGit,
	CategoryConfiguration,
	CategoryChat,
	CategoryPermissions,
	CategoryGeneral,
}

// ShortcutRegistry is the central registry of all keyboard shortcuts.
// Add new shortcuts here and they will automatically appear in the help modal
// and be executable from both direct key presses and the help modal.
var ShortcutRegistry = []Shortcut{
	// Navigation
	{
		Key:         keys.Tab,
		DisplayKey:  "Tab",
		Description: "Switch between sidebar and chat",
		Category:    CategoryNavigation,
		Handler:     shortcutToggleFocus,
	},
	{
		Key:             "/",
		Description:     "Search sessions",
		Category:        CategoryNavigation,
		RequiresSidebar: true,
		Handler:         shortcutSearch,
		Condition:       func(m *Model) bool { return !m.sidebar.IsSearchMode() },
	},

	// Sessions
	{
		Key:             "n",
		Description:     "Create new session",
		Category:        CategorySessions,
		RequiresSidebar: true,
		Handler:         shortcutNewSession,
	},
	{
		Key:             "d",
		Description:     "Delete selected session",
		Category:        CategorySessions,
		RequiresSidebar: true,
		RequiresSession: true,
		Handler:         shortcutDeleteSession,
	},
	{
		Key:             "f",
		Description:     "Fork selected session",
		Category:        CategorySessions,
		RequiresSidebar: true,
		RequiresSession: true,
		Handler:         shortcutForkSession,
	},
	{
		Key:             "i",
		Description:     "Import GitHub issues",
		Category:        CategorySessions,
		RequiresSidebar: true,
		Handler:         shortcutImportIssues,
	},
	{
		Key:         keys.CtrlB,
		DisplayKey:  "ctrl-b",
		Description: "Broadcast prompt to multiple repos",
		Category:    CategorySessions,
		Handler:     shortcutBroadcast,
		Condition:   func(m *Model) bool { return len(m.config.GetRepos()) > 0 },
	},
	{
		Key:             keys.CtrlShiftB,
		DisplayKey:      "ctrl-shift-b",
		Description:     "Broadcast group actions (send prompt/create PRs)",
		Category:        CategorySessions,
		RequiresSession: true,
		Handler:         shortcutBroadcastToGroup,
		Condition: func(m *Model) bool {
			sess := m.sidebar.SelectedSession()
			return sess != nil && sess.BroadcastGroupID != ""
		},
	},
	{
		Key:             "r",
		Description:     "Rename selected session",
		Category:        CategorySessions,
		RequiresSidebar: true,
		RequiresSession: true,
		Handler:         shortcutRenameSession,
	},
	{
		Key:             "s",
		Description:     "Multi-select sessions",
		Category:        CategorySessions,
		RequiresSidebar: true,
		Handler:         shortcutMultiSelect,
		Condition:       func(m *Model) bool { return len(m.config.GetSessions()) > 0 },
	},
	// Git Operations
	{
		Key:             keys.CtrlE,
		DisplayKey:      "ctrl-e",
		Description:     "Open terminal (in container if containerized)",
		Category:        CategoryGit,
		RequiresSession: true,
		Handler:         shortcutOpenTerminal,
	},
	{
		Key:             "v",
		Description:     "View changes in worktree",
		Category:        CategoryGit,
		RequiresSidebar: true,
		RequiresSession: true,
		Handler:         shortcutViewChanges,
	},
	{
		Key:             "m",
		Description:     "Merge to main / Create PR",
		Category:        CategoryGit,
		RequiresSidebar: true,
		RequiresSession: true,
		Handler:         shortcutMerge,
	},
	{
		Key:             "c",
		Description:     "Commit resolved conflicts",
		Category:        CategoryGit,
		RequiresSidebar: true,
		Handler:         shortcutCommitConflicts,
		Condition:       func(m *Model) bool { return m.pendingConflict != nil },
	},
	{
		Key:             "p",
		Description:     "Preview session in main repo",
		Category:        CategoryGit,
		RequiresSidebar: true,
		RequiresSession: true,
		Handler:         shortcutPreviewInMain,
	},
	{
		Key:             keys.CtrlR,
		DisplayKey:      "ctrl-r",
		Description:     "Import PR review comments",
		Category:        CategoryGit,
		RequiresSession: true,
		Handler:         shortcutReviewComments,
	},

	// Configuration
	{
		Key:             "a",
		Description:     "Add repository",
		Category:        CategoryConfiguration,
		RequiresSidebar: true,
		Handler:         shortcutAddRepo,
	},
	{
		Key:             ",",
		Description:     "Session settings",
		Category:        CategoryConfiguration,
		RequiresSidebar: true,
		RequiresSession: true,
		Handler:         shortcutRepoSettings,
	},
	{
		Key:        keys.AltComma,
		DisplayKey: "opt-,",

		Description: "Global settings",
		Category:    CategoryConfiguration,
		Handler:     shortcutGlobalSettings,
	},

	// Chat
	{
		Key:             keys.CtrlSlash,
		DisplayKey:      "ctrl-/",
		Description:     "Search messages",
		Category:        CategoryChat,
		RequiresSession: true,
		Handler:         shortcutSearchMessages,
		Condition:       func(m *Model) bool { return m.chat.IsFocused() },
	},
	{
		Key:             keys.CtrlT,
		DisplayKey:      "ctrl-t",
		Description:     "Toggle tool use expansion",
		Category:        CategoryChat,
		RequiresSession: true,
		Handler:         shortcutToggleToolUseRollup,
		Condition:       func(m *Model) bool { return m.chat.IsFocused() && m.chat.HasActiveToolUseRollup() },
	},

	// General
	// Note: "?" (help) is handled specially in ExecuteShortcut to avoid init cycle
	{
		Key:         keys.CtrlL,
		DisplayKey:  "ctrl-l",
		Description: "Toggle log viewer",
		Category:    CategoryGeneral,
		Handler:     shortcutToggleLogViewer,
	},
	{
		Key:             "W",
		Description:     "What's new (changelog)",
		Category:        CategoryGeneral,
		RequiresSidebar: true,
		Handler:         shortcutWhatsNew,
	},
	{
		Key:             "q",
		Description:     "Quit application",
		Category:        CategoryGeneral,
		RequiresSidebar: true,
		Handler:         shortcutQuit,
	},
}

// helpShortcut is defined separately to avoid initialization cycle.
// It references ShortcutRegistry, so it can't be in the registry itself.
var helpShortcut = Shortcut{
	Key:             "?",
	Description:     "Show this help",
	Category:        CategoryGeneral,
	RequiresSidebar: true,
}

// DisplayOnlyShortcuts are shown in help but not executable from the help modal.
// These are context-sensitive or informational entries.
var DisplayOnlyShortcuts = []Shortcut{
	// Navigation (display-only)
	{DisplayKey: "↑/↓ or j/k", Description: "Navigate session list", Category: CategoryNavigation},
	{DisplayKey: "PgUp/PgDn", Description: "Scroll chat or session list", Category: CategoryNavigation},
	{DisplayKey: "ctrl-u/ctrl-d", Description: "Scroll half page up/down", Category: CategoryNavigation},
	{DisplayKey: "Enter", Description: "Send message / New session action", Category: CategoryNavigation},
	{DisplayKey: "Esc", Description: "Cancel search / Stop streaming", Category: CategoryNavigation},

	// Chat (display-only, context-sensitive)
	{DisplayKey: "Opt+Enter", Description: "Insert newline", Category: CategoryChat},
	{DisplayKey: "ctrl-v", Description: "Paste image", Category: CategoryChat},
	{DisplayKey: "ctrl-o", Description: "Fork detected options", Category: CategoryChat},
	{DisplayKey: "Mouse drag", Description: "Select text (auto-copies)", Category: CategoryChat},
	{DisplayKey: "Esc", Description: "Clear input / selection", Category: CategoryChat},

	// Permissions (display-only, context-sensitive)
	{DisplayKey: "y", Description: "Allow action", Category: CategoryPermissions},
	{DisplayKey: "n", Description: "Deny action", Category: CategoryPermissions},
	{DisplayKey: "a", Description: "Always allow this tool", Category: CategoryPermissions},
}

// isShortcutApplicable checks if a shortcut is applicable given the current model state.
// This is used to filter which shortcuts appear in the help modal.
func (m *Model) isShortcutApplicable(s Shortcut) bool {
	selectedSess := m.sidebar.SelectedSession()

	// Check guards
	if s.RequiresSidebar && m.chat.IsFocused() {
		return false
	}
	if s.RequiresSession && selectedSess == nil {
		return false
	}
	if s.Condition != nil && !s.Condition(m) {
		return false
	}
	return true
}

// ExecuteShortcut finds and executes a shortcut by key.
// It checks all guards (RequiresSidebar, RequiresSession, Condition) before executing.
// Returns (model, cmd, true) if the shortcut was found and executed.
// Returns (model, nil, false) if the shortcut was not found or guards failed.
func (m *Model) ExecuteShortcut(key string) (tea.Model, tea.Cmd, bool) {
	log := logger.WithComponent("Shortcut")

	// If sidebar is in search mode, don't process shortcuts - let keys go to search input
	// Exception: "/" is handled by its own Condition guard to allow entering search mode
	if m.sidebar.IsSearchMode() && key != "/" {
		log.Debug("sidebar in search mode, letting key go to search input", "key", key)
		return m, nil, false
	}

	// Handle help shortcut specially (defined outside registry to avoid init cycle)
	if key == "?" {
		if m.chat.IsFocused() {
			return m, nil, false // Guard failed, let key propagate to textarea
		}
		result, cmd := shortcutHelp(m)
		return result, cmd, true
	}

	selectedSess := m.sidebar.SelectedSession()
	var selectedID string
	if selectedSess != nil {
		selectedID = selectedSess.ID
	}

	for _, s := range ShortcutRegistry {
		if s.Key == key {
			log.Debug("found shortcut, checking guards", "key", key, "chatFocused", m.chat.IsFocused(), "selectedSession", selectedID)
			// Check guards — on failure, continue to next entry so multiple
			// shortcuts with the same key can coexist (first match wins).
			if s.RequiresSidebar && m.chat.IsFocused() {
				log.Debug("guard failed - RequiresSidebar but chat is focused")
				return m, nil, false // Guard failed, let key propagate to textarea
			}
			if s.RequiresSession && selectedSess == nil {
				log.Debug("guard failed - RequiresSession but no session selected, trying next")
				continue
			}
			if s.Condition != nil && !s.Condition(m) {
				log.Debug("guard failed - Condition returned false, trying next")
				continue
			}
			log.Debug("all guards passed, executing handler", "key", key)
			result, cmd := s.Handler(m)
			return result, cmd, true
		}
	}
	return m, nil, false
}

// getApplicableFooterBindings generates footer key bindings from shortcuts that are
// applicable in the current application state. Returns a minimal set of the most
// relevant shortcuts for display in the limited footer space.
func (m *Model) getApplicableFooterBindings() []ui.KeyBinding {
	var bindings []ui.KeyBinding

	// Priority order for footer display (most important first)
	priorityKeys := []string{
		keys.Tab,      // Navigation
		"n",           // New session
		"a",           // Add repo
		"v",           // View changes
		"m",           // Merge/PR
		"f",           // Fork
		"d",           // Delete
		"i",           // Import issues
		keys.CtrlE,    // Open terminal
		keys.CtrlB,    // Broadcast
		",",           // Repo settings
		keys.AltComma, // Global settings
		"q",           // Quit
		"?",           // Help
	}

	// Build a map of applicable shortcuts
	applicableMap := make(map[string]Shortcut)
	for _, s := range ShortcutRegistry {
		if m.isShortcutApplicable(s) {
			applicableMap[s.Key] = s
		}
	}

	// Add help shortcut if applicable (defined separately to avoid init cycle)
	if m.isShortcutApplicable(helpShortcut) {
		applicableMap[helpShortcut.Key] = helpShortcut
	}

	// Build footer bindings in priority order
	for _, key := range priorityKeys {
		if shortcut, ok := applicableMap[key]; ok {
			displayKey := shortcut.DisplayKey
			if displayKey == "" {
				displayKey = shortcut.Key
			}
			bindings = append(bindings, ui.KeyBinding{
				Key:  displayKey,
				Desc: shortcut.Description,
			})
		}
	}

	return bindings
}

// getApplicableHelpSections generates help modal sections from shortcuts that are
// applicable in the current application state. This filters out shortcuts whose
// guards (RequiresSidebar, RequiresSession, Condition) would fail.
func (m *Model) getApplicableHelpSections(registry []Shortcut, displayOnly []Shortcut) []ui.HelpSection {
	// Collect shortcuts by category
	categories := make(map[string][]ui.HelpShortcut)

	// Add executable shortcuts that are applicable
	for _, s := range registry {
		if !m.isShortcutApplicable(s) {
			continue
		}
		displayKey := s.DisplayKey
		if displayKey == "" {
			displayKey = s.Key
		}
		categories[s.Category] = append(categories[s.Category], ui.HelpShortcut{
			Key:  displayKey,
			Desc: s.Description,
		})
	}

	// Add display-only shortcuts for applicable categories
	// Display-only shortcuts are shown when their category has at least one applicable shortcut,
	// or when they don't have guards (navigation, permissions are always shown for context)
	for _, s := range displayOnly {
		// Always show Navigation and Permissions display-only shortcuts for context
		// Chat display-only shortcuts only show when chat is focused
		if s.Category == CategoryChat && !m.chat.IsFocused() {
			continue
		}
		displayKey := s.DisplayKey
		if displayKey == "" {
			displayKey = s.Key
		}
		categories[s.Category] = append(categories[s.Category], ui.HelpShortcut{
			Key:  displayKey,
			Desc: s.Description,
		})
	}

	// Build sections in the correct order
	var sections []ui.HelpSection
	for _, cat := range categoryOrder {
		if shortcuts, ok := categories[cat]; ok && len(shortcuts) > 0 {
			sections = append(sections, ui.HelpSection{
				Title:     cat,
				Shortcuts: shortcuts,
			})
		}
	}

	return sections
}

// =============================================================================
// Shortcut Handlers
// =============================================================================

func shortcutToggleFocus(m *Model) (tea.Model, tea.Cmd) {
	cmd := m.toggleFocus()
	return m, cmd
}

func shortcutSearch(m *Model) (tea.Model, tea.Cmd) {
	m.sidebar.EnterSearchMode()
	return m, nil
}

func shortcutNewSession(m *Model) (tea.Model, tea.Cmd) {
	m.modal.Show(ui.NewNewSessionState(m.config.GetRepos(), process.ContainersSupported(), claude.ContainerAuthAvailable()))
	return m, nil
}

func shortcutDeleteSession(m *Model) (tea.Model, tea.Cmd) {
	sess := m.sidebar.SelectedSession()
	displayName := ui.SessionDisplayName(sess.Branch, sess.Name)
	m.modal.Show(ui.NewConfirmDeleteState(displayName))
	return m, nil
}

func shortcutForkSession(m *Model) (tea.Model, tea.Cmd) {
	sess := m.sidebar.SelectedSession()
	displayName := ui.SessionDisplayName(sess.Branch, sess.Name)
	m.modal.Show(ui.NewForkSessionState(displayName, sess.ID, sess.RepoPath, sess.Containerized, process.ContainersSupported(), claude.ContainerAuthAvailable()))
	return m, nil
}

func shortcutImportIssues(m *Model) (tea.Model, tea.Cmd) {
	if sess := m.sidebar.SelectedSession(); sess != nil {
		// Session selected - use its repo, check for multiple sources
		return m.showIssueSourceOrFetch(sess.RepoPath)
	}
	// No session - show repo picker
	repos := m.config.GetRepos()
	m.modal.Show(ui.NewSelectRepoForIssuesState(repos))
	return m, nil
}

func shortcutRenameSession(m *Model) (tea.Model, tea.Cmd) {
	sess := m.sidebar.SelectedSession()
	// Get current name without the branch prefix (so user edits just the base name)
	branchPrefix := m.config.GetDefaultBranchPrefix()
	currentName := strings.TrimPrefix(sess.Branch, branchPrefix)
	m.modal.Show(ui.NewRenameSessionState(sess.ID, currentName))
	return m, nil
}

func shortcutOpenTerminal(m *Model) (tea.Model, tea.Cmd) {
	// Use activeSession when chat is focused, otherwise use sidebar selection
	var sess *config.Session
	if m.chat.IsFocused() && m.activeSession != nil {
		sess = m.activeSession
	} else {
		sess = m.sidebar.SelectedSession()
	}
	if sess == nil {
		return m, nil
	}
	logger.WithSession(sess.ID).Debug("opening terminal for session", "path", sess.WorkTree, "containerized", sess.Containerized)
	return m, openTerminalForSession(sess)
}

func shortcutViewChanges(m *Model) (tea.Model, tea.Cmd) {
	sess := m.sidebar.SelectedSession()
	// Select the session first so we can display in its chat panel
	if m.activeSession == nil || m.activeSession.ID != sess.ID {
		m.selectSession(sess)
	}
	// Get worktree status and display it in view changes overlay
	ctx := context.Background()
	status, err := m.gitService.GetWorktreeStatus(ctx, sess.WorkTree)
	var files []git.FileDiff
	if err != nil {
		files = []git.FileDiff{{
			Filename: "Error",
			Status:   "!",
			Diff:     fmt.Sprintf("Error getting status: %v", err),
		}}
	} else if !status.HasChanges {
		files = []git.FileDiff{{
			Filename: "No changes",
			Status:   " ",
			Diff:     "No uncommitted changes in this session.",
		}}
	} else {
		files = status.FileDiffs
	}
	m.chat.EnterViewChangesMode(files)
	// Switch focus to chat so arrow keys and Escape work immediately
	m.focus = FocusChat
	m.sidebar.SetFocused(false)
	m.chat.SetFocused(true)
	return m, nil
}

func shortcutMerge(m *Model) (tea.Model, tea.Cmd) {
	sess := m.sidebar.SelectedSession()
	// Don't show merge modal if already merging or generating commit message
	state := m.sessionState().GetIfExists(sess.ID)
	if (state != nil && state.IsMerging()) || (m.pendingCommit != nil && m.pendingCommit.SessionID == sess.ID) {
		return m, nil
	}
	ctx := context.Background()
	hasRemote := m.gitService.HasRemoteOrigin(ctx, sess.RepoPath)
	// Get changes summary to display in modal
	var changesSummary string
	if status, err := m.gitService.GetWorktreeStatus(ctx, sess.WorkTree); err == nil && status.HasChanges {
		changesSummary = status.Summary
		// Add file list if not too many files
		if len(status.Files) <= 5 {
			changesSummary += ": " + strings.Join(status.Files, ", ")
		}
	}
	displayName := ui.SessionDisplayName(sess.Branch, sess.Name)
	// Get parent name if this is a child session
	var parentName string
	if sess.ParentID != "" {
		if parent := m.config.GetSession(sess.ParentID); parent != nil {
			parentName = ui.SessionDisplayName(parent.Branch, parent.Name)
		}
	}
	m.modal.Show(ui.NewMergeState(displayName, hasRemote, changesSummary, parentName, sess.PRCreated))
	return m, nil
}

func shortcutCommitConflicts(m *Model) (tea.Model, tea.Cmd) {
	return m.showCommitConflictModal()
}

func shortcutAddRepo(m *Model) (tea.Model, tea.Cmd) {
	// Check if current directory is a git repo and not already added
	ctx := context.Background()
	currentRepo := m.sessionService.GetCurrentDirGitRoot(ctx)
	if currentRepo != "" {
		// Check if already added
		if slices.Contains(m.config.GetRepos(), currentRepo) {
			currentRepo = ""
		}
	}
	m.modal.Show(ui.NewAddRepoState(currentRepo))
	return m, nil
}

func shortcutMCPServers(m *Model) (tea.Model, tea.Cmd) {
	m.showMCPServersModal()
	return m, nil
}

func shortcutPlugins(m *Model) (tea.Model, tea.Cmd) {
	m.showPluginsModal()
	return m, nil
}

func shortcutHelp(m *Model) (tea.Model, tea.Cmd) {
	// Include help shortcut in the registry for display purposes
	allShortcuts := append(ShortcutRegistry, helpShortcut)

	// Override newline shortcut display based on terminal capabilities
	displayOnly := DisplayOnlyShortcuts
	if m.kittyKeyboard {
		displayOnly = make([]Shortcut, len(DisplayOnlyShortcuts))
		copy(displayOnly, DisplayOnlyShortcuts)
		for i := range displayOnly {
			if displayOnly[i].DisplayKey == "Opt+Enter" && displayOnly[i].Description == "Insert newline" {
				displayOnly[i].DisplayKey = "Shift+Enter"
				break
			}
		}
	}

	sections := m.getApplicableHelpSections(allShortcuts, displayOnly)
	m.modal.Show(ui.NewHelpStateFromSections(sections))
	return m, nil
}

func shortcutQuit(m *Model) (tea.Model, tea.Cmd) {
	return m, tea.Quit
}

func shortcutSearchMessages(m *Model) (tea.Model, tea.Cmd) {
	// Get messages from the current session
	messages := m.chat.GetMessages()
	if len(messages) == 0 {
		return m, nil
	}

	// Convert to the format expected by NewSearchMessagesState
	var searchMessages []struct{ Role, Content string }
	for _, msg := range messages {
		searchMessages = append(searchMessages, struct{ Role, Content string }{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	m.modal.Show(ui.NewSearchMessagesState(searchMessages))
	return m, nil
}

func shortcutToggleToolUseRollup(m *Model) (tea.Model, tea.Cmd) {
	m.chat.ToggleToolUseRollup()
	return m, nil
}

func shortcutRepoSettings(m *Model) (tea.Model, tea.Cmd) {
	sess := m.sidebar.SelectedSession()
	if sess == nil {
		return m, nil
	}
	return m.showSessionSettings(sess)
}

func shortcutGlobalSettings(m *Model) (tea.Model, tea.Cmd) {
	settingsState := ui.NewSettingsState(
		m.config.GetDefaultBranchPrefix(),
		m.config.GetNotificationsEnabled(),
		process.ContainersSupported(),
		m.config.GetContainerImage(),
		m.config.GetAutoCleanupMerged(),
	)
	m.modal.Show(settingsState)
	return m, nil
}

// showSessionSettings opens the session-specific settings modal.
func (m *Model) showSessionSettings(sess *config.Session) (tea.Model, tea.Cmd) {
	// Strip branch prefix for display in the name input
	name := sess.Name
	branchPrefix := m.config.GetDefaultBranchPrefix()
	if branchPrefix != "" {
		name = strings.TrimPrefix(name, branchPrefix)
	}

	asanaPATSet := os.Getenv("ASANA_PAT") != ""
	linearAPIKeySet := os.Getenv("LINEAR_API_KEY") != ""

	state := ui.NewSessionSettingsState(
		sess.ID,
		name,
		sess.Branch,
		sess.BaseBranch,
		sess.Containerized,
		sess.RepoPath,
		asanaPATSet,
		m.config.GetAsanaProject(sess.RepoPath),
		linearAPIKeySet,
		m.config.GetLinearTeam(sess.RepoPath),
	)
	m.modal.Show(state)

	// Kick off async fetches for configured providers
	var cmds []tea.Cmd
	if asanaPATSet {
		cmds = append(cmds, m.fetchAsanaProjects())
	}
	if linearAPIKeySet {
		cmds = append(cmds, m.fetchLinearTeams())
	}
	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func shortcutWhatsNew(m *Model) (tea.Model, tea.Cmd) {
	return m, m.fetchChangelogAll()
}

func shortcutToggleLogViewer(m *Model) (tea.Model, tea.Cmd) {
	// If already in log viewer mode, exit it
	if m.chat.IsInLogViewerMode() {
		m.chat.ExitLogViewerMode()
		return m, nil
	}

	// Get current session ID if available
	var sessionID string
	if m.activeSession != nil {
		sessionID = m.activeSession.ID
	}

	// Get available log files
	logFiles := ui.GetLogFiles(sessionID)
	if len(logFiles) == 0 {
		return m, m.ShowFlashInfo("No log files found. Enable debug mode with --debug.")
	}

	// Enter log viewer mode
	m.chat.EnterLogViewerMode(logFiles)

	// Switch focus to chat so keys work immediately
	m.focus = FocusChat
	m.sidebar.SetFocused(false)
	m.chat.SetFocused(true)

	return m, nil
}

func shortcutReviewComments(m *Model) (tea.Model, tea.Cmd) {
	sess := m.sidebar.SelectedSession()
	// Select the session if not already active
	if m.activeSession == nil || m.activeSession.ID != sess.ID {
		m.selectSession(sess)
	}
	// Show the review comments modal in loading state
	m.modal.Show(ui.NewReviewCommentsState(sess.ID, sess.Branch))
	return m, m.fetchReviewComments(sess.ID, sess.RepoPath, sess.Branch)
}

func shortcutPreviewInMain(m *Model) (tea.Model, tea.Cmd) {
	sess := m.sidebar.SelectedSession()
	ctx := context.Background()
	log := logger.WithSession(sess.ID)

	// Check if this session is already being previewed - if so, end the preview
	previewSessionID := m.config.GetPreviewSessionID()
	if previewSessionID == sess.ID {
		return m.endPreview()
	}

	// Check if a different session is being previewed
	if previewSessionID != "" {
		return m, m.ShowFlashWarning("Another session is being previewed. End that preview first (p).")
	}

	// Check if session worktree has uncommitted changes - commit them first
	sessionStatus, err := m.gitService.GetWorktreeStatus(ctx, sess.WorkTree)
	if err != nil {
		log.Error("failed to check session worktree status", "error", err)
		return m, m.ShowFlashError(fmt.Sprintf("Failed to check session status: %v", err))
	}
	if sessionStatus.HasChanges {
		// Generate a commit message and commit the changes
		commitMsg, err := m.gitService.GenerateCommitMessage(ctx, sess.WorkTree)
		if err != nil {
			log.Error("failed to generate commit message", "error", err)
			return m, m.ShowFlashError(fmt.Sprintf("Failed to generate commit message: %v", err))
		}
		if err := m.gitService.CommitAll(ctx, sess.WorkTree, commitMsg); err != nil {
			log.Error("failed to commit session changes", "error", err)
			return m, m.ShowFlashError(fmt.Sprintf("Failed to commit session changes: %v", err))
		}
		log.Info("committed session changes for preview", "commitMsg", commitMsg)
	}

	// Check if main repo has uncommitted changes
	status, err := m.gitService.GetWorktreeStatus(ctx, sess.RepoPath)
	if err != nil {
		log.Error("failed to check main repo status", "error", err)
		return m, m.ShowFlashError(fmt.Sprintf("Failed to check main repo: %v", err))
	}
	if status.HasChanges {
		return m, m.ShowFlashError("Main repo has uncommitted changes. Commit or stash them first.")
	}

	// Get the current branch in main repo (to restore later)
	currentBranch, err := m.gitService.GetCurrentBranch(ctx, sess.RepoPath)
	if err != nil {
		log.Error("failed to get current branch", "error", err)
		return m, m.ShowFlashError(fmt.Sprintf("Failed to get current branch: %v", err))
	}

	// Checkout the session's branch in the main repo
	// Use CheckoutBranchIgnoreWorktrees because the branch is already checked out in the session's worktree
	if err := m.gitService.CheckoutBranchIgnoreWorktrees(ctx, sess.RepoPath, sess.Branch); err != nil {
		log.Error("failed to checkout session branch", "error", err, "branch", sess.Branch)
		return m, m.ShowFlashError(fmt.Sprintf("Failed to checkout branch: %v", err))
	}

	// Record the preview state
	m.config.StartPreview(sess.ID, currentBranch, sess.RepoPath)
	var saveCmd tea.Cmd
	if cmd := m.saveConfigOrFlash(); cmd != nil {
		saveCmd = cmd
	}

	// Update header to show preview indicator
	m.header.SetPreviewActive(true)

	// Show the preview warning modal
	m.modal.Show(ui.NewPreviewActiveState(sess.Name, sess.Branch))

	log.Info("started preview", "branch", sess.Branch, "previousBranch", currentBranch)
	return m, saveCmd
}

// endPreview ends the current preview and restores the previous branch
func (m *Model) endPreview() (tea.Model, tea.Cmd) {
	sessionID, previousBranch, repoPath := m.config.GetPreviewState()
	if sessionID == "" {
		return m, nil
	}

	ctx := context.Background()
	log := logger.WithSession(sessionID)

	// Check if main repo has uncommitted changes (user may have made changes while previewing)
	status, err := m.gitService.GetWorktreeStatus(ctx, repoPath)
	if err != nil {
		log.Error("failed to check main repo status", "error", err)
		return m, m.ShowFlashError(fmt.Sprintf("Failed to check main repo: %v", err))
	}
	if status.HasChanges {
		return m, m.ShowFlashError("Main repo has uncommitted changes. Commit or stash them before ending preview.")
	}

	// Checkout the previous branch
	if err := m.gitService.CheckoutBranch(ctx, repoPath, previousBranch); err != nil {
		log.Error("failed to restore previous branch", "error", err, "branch", previousBranch)
		return m, m.ShowFlashError(fmt.Sprintf("Failed to restore branch: %v", err))
	}

	// Clear the preview state
	m.config.EndPreview()
	var cmds []tea.Cmd
	if cmd := m.saveConfigOrFlash(); cmd != nil {
		cmds = append(cmds, cmd)
	}

	// Update header to hide preview indicator
	m.header.SetPreviewActive(false)

	log.Info("ended preview", "restoredBranch", previousBranch)
	cmds = append(cmds, m.ShowFlashSuccess(fmt.Sprintf("Preview ended. Restored to %s.", previousBranch)))
	return m, tea.Batch(cmds...)
}

// TerminalErrorMsg is sent when opening a terminal fails
type TerminalErrorMsg struct {
	Error string
}

// detectTerminalApp returns the macOS application name for the user's current terminal.
// It reads the TERM_PROGRAM environment variable (set by most modern terminal emulators)
// and maps it to the corresponding app name. Falls back to "Terminal" if unrecognized.
func detectTerminalApp() string {
	termProgram := os.Getenv("TERM_PROGRAM")
	switch strings.ToLower(termProgram) {
	case "ghostty":
		return "Ghostty"
	case "iterm.app":
		return "iTerm2"
	case "apple_terminal":
		return "Terminal"
	case "wezterm":
		return "WezTerm"
	case "kitty":
		return "kitty"
	case "alacritty":
		return "Alacritty"
	default:
		return "Terminal"
	}
}

// linuxTerminal represents a terminal emulator with its launch arguments.
type linuxTerminal struct {
	name string
	args []string
}

// prependDetectedTerminal checks TERM_PROGRAM and prepends a matching terminal
// entry to the list if found. detectedArgs maps lowercase TERM_PROGRAM values
// to linuxTerminal entries with their specific launch arguments.
func prependDetectedTerminal(terminals []linuxTerminal, detectedArgs map[string]linuxTerminal) []linuxTerminal {
	termProgram := os.Getenv("TERM_PROGRAM")
	if termProgram == "" {
		return terminals
	}
	if detected, ok := detectedArgs[strings.ToLower(termProgram)]; ok {
		log := logger.WithComponent("Shortcut")
		log.Debug("prepending detected terminal from TERM_PROGRAM", "terminal", detected.name)
		return append([]linuxTerminal{detected}, terminals...)
	}
	return terminals
}

// openTerminalForSession returns a command that opens a terminal for the given session.
// If the session is containerized, it opens an interactive shell inside the container.
// Otherwise, it opens a terminal window at the worktree path.
func openTerminalForSession(sess *config.Session) tea.Cmd {
	if sess.Containerized {
		return openTerminalInContainer(sess)
	}
	return openTerminalAtPath(sess.WorkTree)
}

// openTerminalInContainer returns a command that opens a terminal with an interactive
// shell inside the session's container. The container name follows the pattern "plural-<session-id>".
func openTerminalInContainer(sess *config.Session) tea.Cmd {
	return func() tea.Msg {
		log := logger.WithSession(sess.ID)

		// Validate session ID is a valid UUID (defense against command injection)
		if _, err := uuid.Parse(sess.ID); err != nil {
			errMsg := fmt.Sprintf("Invalid session ID: %v", err)
			log.Error("invalid session ID format", "error", err, "sessionID", sess.ID)
			return TerminalErrorMsg{Error: errMsg}
		}

		containerName := "plural-" + sess.ID
		log.Debug("opening terminal in container", "container", containerName)

		// First, verify the container is actually running using the robust helper
		names, err := process.ListContainerNames()
		if err != nil {
			errMsg := fmt.Sprintf("Failed to check running containers: %v", err)
			log.Error("failed to list containers", "error", err)
			return TerminalErrorMsg{Error: errMsg}
		}

		// Check if our container is in the list (exact match)
		found := slices.Contains(names, containerName)
		if !found {
			errMsg := fmt.Sprintf("Container not running. Session must be active (send a message first).")
			log.Debug("container not found in running containers", "container", containerName)
			return TerminalErrorMsg{Error: errMsg}
		}

		switch runtime.GOOS {
		case "darwin":
			termApp := detectTerminalApp()
			log.Debug("detected terminal app for container", "terminal", termApp, "TERM_PROGRAM", os.Getenv("TERM_PROGRAM"))

			escapedContainer := strings.ReplaceAll(containerName, `\`, `\\`)
			escapedContainer = strings.ReplaceAll(escapedContainer, `"`, `\"`)
			dockerCmd := fmt.Sprintf("docker exec -it %s /bin/sh", containerName)

			var cmd *exec.Cmd
			switch termApp {
			case "kitty":
				cmd = exec.Command("kitty", "--single-instance", "sh", "-c", dockerCmd)
			case "WezTerm":
				cmd = exec.Command("wezterm", "cli", "spawn", "--new-window", "--", "sh", "-c", dockerCmd)
			default:
				script := fmt.Sprintf(`tell application "%s"
	do script "docker exec -it %s /bin/sh"
	activate
end tell`, termApp, escapedContainer)
				cmd = exec.Command("osascript", "-e", script)
			}

			output, err := cmd.CombinedOutput()
			if err != nil {
				errMsg := fmt.Sprintf("Failed to open terminal in container: %v", err)
				if len(output) > 0 {
					errMsg = fmt.Sprintf("Failed to open terminal in container: %s", strings.TrimSpace(string(output)))
				}
				log.Error("failed to open terminal in container", "error", errMsg)
				return TerminalErrorMsg{Error: errMsg}
			}
			log.Debug("terminal opened successfully in container", "terminal", termApp)
			return nil

		case "linux":
			// Linux: try common terminal emulators with docker exec command
			containerCmd := fmt.Sprintf("docker exec -it %s /bin/sh", containerName)
			terminals := []linuxTerminal{
				{"gnome-terminal", []string{"--", "sh", "-c", containerCmd}},
				{"konsole", []string{"-e", containerCmd}},
				{"xfce4-terminal", []string{"-e", containerCmd}},
				{"xterm", []string{"-e", containerCmd}},
			}

			terminals = prependDetectedTerminal(terminals, map[string]linuxTerminal{
				"ghostty":   {"ghostty", []string{"-e", "sh", "-c", containerCmd}},
				"kitty":     {"kitty", []string{"--single-instance", "sh", "-c", containerCmd}},
				"wezterm":   {"wezterm", []string{"cli", "spawn", "--new-window", "--", "sh", "-c", containerCmd}},
				"alacritty": {"alacritty", []string{"-e", "sh", "-c", containerCmd}},
			})

			var cmd *exec.Cmd
			for _, term := range terminals {
				if _, err := exec.LookPath(term.name); err == nil {
					cmd = exec.Command(term.name, term.args...)
					break
				}
			}

			if cmd == nil {
				// Fallback: try x-terminal-emulator
				cmd = exec.Command("x-terminal-emulator", "-e", containerCmd)
			}

			if err := cmd.Start(); err != nil {
				errMsg := fmt.Sprintf("Failed to open terminal in container: %v", err)
				log.Error("failed to open terminal in container", "error", errMsg)
				return TerminalErrorMsg{Error: errMsg}
			}
			log.Debug("terminal opened successfully in container")
			return nil

		default:
			errMsg := fmt.Sprintf("Unsupported OS for terminal: %s", runtime.GOOS)
			log.Error("unsupported OS for terminal", "os", runtime.GOOS)
			return TerminalErrorMsg{Error: errMsg}
		}
	}
}

// openTerminalAtPath returns a command that opens a new terminal window at the given path.
// Supports macOS (Terminal.app) and Linux (common terminal emulators).
func openTerminalAtPath(path string) tea.Cmd {
	return func() tea.Msg {
		log := logger.WithComponent("Shortcut")
		log.Debug("opening terminal at path", "os", runtime.GOOS, "path", path)

		switch runtime.GOOS {
		case "darwin":
			termApp := detectTerminalApp()
			log.Debug("detected terminal app", "terminal", termApp, "TERM_PROGRAM", os.Getenv("TERM_PROGRAM"))

			escapedPath := strings.ReplaceAll(path, `\`, `\\`)
			escapedPath = strings.ReplaceAll(escapedPath, `"`, `\"`)

			var cmd *exec.Cmd
			switch termApp {
			case "kitty":
				// kitty uses its own CLI to open new OS windows
				cmd = exec.Command("kitty", "--single-instance", "--directory", path)
			case "WezTerm":
				// WezTerm has a CLI for spawning windows
				cmd = exec.Command("wezterm", "cli", "spawn", "--new-window", "--cwd", path)
			default:
				// Terminal.app, iTerm2, Ghostty, Alacritty, and others use AppleScript
				script := fmt.Sprintf(`tell application "%s"
	do script "cd \"%s\""
	activate
end tell`, termApp, escapedPath)
				cmd = exec.Command("osascript", "-e", script)
			}

			output, err := cmd.CombinedOutput()
			if err != nil {
				errMsg := fmt.Sprintf("Failed to open terminal: %v", err)
				if len(output) > 0 {
					errMsg = fmt.Sprintf("Failed to open terminal: %s", strings.TrimSpace(string(output)))
				}
				log.Error("failed to open terminal", "error", errMsg)
				return TerminalErrorMsg{Error: errMsg}
			}
			log.Debug("terminal opened successfully", "terminal", termApp)
			return nil

		case "linux":
			// Linux: try common terminal emulators in order of preference
			terminals := []linuxTerminal{
				{"gnome-terminal", []string{"--working-directory=" + path}},
				{"konsole", []string{"--workdir", path}},
				{"xfce4-terminal", []string{"--working-directory=" + path}},
				{"xterm", []string{"-e", fmt.Sprintf("cd %q && $SHELL", path)}},
			}

			terminals = prependDetectedTerminal(terminals, map[string]linuxTerminal{
				"ghostty":   {"ghostty", []string{"-e", fmt.Sprintf("cd %q && exec $SHELL", path)}},
				"kitty":     {"kitty", []string{"--single-instance", "--directory", path}},
				"wezterm":   {"wezterm", []string{"cli", "spawn", "--new-window", "--cwd", path}},
				"alacritty": {"alacritty", []string{"--working-directory", path}},
			})

			var cmd *exec.Cmd
			for _, term := range terminals {
				if _, err := exec.LookPath(term.name); err == nil {
					cmd = exec.Command(term.name, term.args...)
					break
				}
			}

			if cmd == nil {
				// Fallback: try x-terminal-emulator (Debian/Ubuntu alternative)
				cmd = exec.Command("x-terminal-emulator", "--working-directory="+path)
			}

			if err := cmd.Start(); err != nil {
				errMsg := fmt.Sprintf("Failed to open terminal: %v", err)
				log.Error("failed to open terminal", "error", errMsg)
				return TerminalErrorMsg{Error: errMsg}
			}
			log.Debug("terminal opened successfully")
			return nil

		default:
			errMsg := fmt.Sprintf("Unsupported OS for terminal: %s", runtime.GOOS)
			log.Error("unsupported OS for terminal", "os", runtime.GOOS)
			return TerminalErrorMsg{Error: errMsg}
		}
	}
}

func shortcutMultiSelect(m *Model) (tea.Model, tea.Cmd) {
	m.sidebar.EnterMultiSelect()
	return m, nil
}

func shortcutBroadcast(m *Model) (tea.Model, tea.Cmd) {
	repos := m.config.GetRepos()
	m.modal.Show(ui.NewBroadcastState(repos, process.ContainersSupported(), claude.ContainerAuthAvailable()))
	return m, nil
}

func shortcutBroadcastToGroup(m *Model) (tea.Model, tea.Cmd) {
	sess := m.sidebar.SelectedSession()
	if sess == nil || sess.BroadcastGroupID == "" {
		return m, m.ShowFlashWarning("Session is not part of a broadcast group")
	}

	groupSessions := m.config.GetSessionsByBroadcastGroup(sess.BroadcastGroupID)
	if len(groupSessions) == 0 {
		return m, m.ShowFlashWarning("No sessions found in broadcast group")
	}

	// Convert sessions to SessionItems for the modal
	sessionItems := make([]ui.SessionItem, len(groupSessions))
	for i, s := range groupSessions {
		sessionItems[i] = ui.SessionItem{
			ID:       s.ID,
			Name:     s.Name,
			RepoName: filepath.Base(s.RepoPath),
			Selected: true,
		}
	}

	m.modal.Show(ui.NewBroadcastGroupState(sess.BroadcastGroupID, sessionItems))
	return m, nil
}
