package app

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/session"
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
		Key:         "tab",
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
		Key:             "r",
		Description:     "Rename selected session",
		Category:        CategorySessions,
		RequiresSidebar: true,
		RequiresSession: true,
		Handler:         shortcutRenameSession,
	},
	// Git Operations
	{
		Key:             "ctrl+e",
		DisplayKey:      "ctrl-e",
		Description:     "Open terminal in worktree",
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
		Condition:       func(m *Model) bool { return m.pendingConflictRepoPath != "" },
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
		Key:             "t",
		Description:     "Change theme",
		Category:        CategoryConfiguration,
		RequiresSidebar: true,
		Handler:         shortcutTheme,
	},
	{
		Key:             ",",
		Description:     "Settings",
		Category:        CategoryConfiguration,
		RequiresSidebar: true,
		Handler:         shortcutSettings,
	},

	// Chat
	{
		Key:             "ctrl+/",
		DisplayKey:      "ctrl-/",
		Description:     "Search messages",
		Category:        CategoryChat,
		RequiresSession: true,
		Handler:         shortcutSearchMessages,
		Condition:       func(m *Model) bool { return m.chat.IsFocused() },
	},

	// General
	// Note: "?" (help) is handled specially in ExecuteShortcut to avoid init cycle
	{
		Key:             "w",
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
	{DisplayKey: "Enter", Description: "Select session / Send message", Category: CategoryNavigation},
	{DisplayKey: "Esc", Description: "Cancel search / Stop streaming", Category: CategoryNavigation},

	// Chat (display-only, context-sensitive)
	{DisplayKey: "ctrl-v", Description: "Paste image", Category: CategoryChat},
	{DisplayKey: "ctrl-p", Description: "Fork detected options", Category: CategoryChat},
	{DisplayKey: "Mouse drag", Description: "Select text (auto-copies)", Category: CategoryChat},
	{DisplayKey: "Esc", Description: "Clear text selection", Category: CategoryChat},

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
	// If sidebar is in search mode, don't process shortcuts - let keys go to search input
	// Exception: "/" is handled by its own Condition guard to allow entering search mode
	if m.sidebar.IsSearchMode() && key != "/" {
		logger.Log("Shortcut: Sidebar in search mode, letting key %q go to search input", key)
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

	for _, s := range ShortcutRegistry {
		if s.Key == key {
			selectedSess := m.sidebar.SelectedSession()
			var selectedID string
			if selectedSess != nil {
				selectedID = selectedSess.ID
			}
			logger.Log("Shortcut: Found shortcut for key=%q, checking guards: chatFocused=%v, selectedSession=%q", key, m.chat.IsFocused(), selectedID)
			// Check guards
			if s.RequiresSidebar && m.chat.IsFocused() {
				logger.Log("Shortcut: Guard failed - RequiresSidebar but chat is focused")
				return m, nil, false // Guard failed, let key propagate to textarea
			}
			if s.RequiresSession && selectedSess == nil {
				logger.Log("Shortcut: Guard failed - RequiresSession but no session selected")
				return m, nil, false // Guard failed, let key propagate to textarea
			}
			if s.Condition != nil && !s.Condition(m) {
				logger.Log("Shortcut: Guard failed - Condition returned false")
				return m, nil, false // Guard failed, let key propagate to textarea
			}
			logger.Log("Shortcut: All guards passed, executing handler for %q", key)
			result, cmd := s.Handler(m)
			return result, cmd, true
		}
	}
	return m, nil, false
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
	m.modal.Show(ui.NewNewSessionState(m.config.GetRepos()))
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
	m.modal.Show(ui.NewForkSessionState(displayName, sess.ID, sess.RepoPath))
	return m, nil
}

func shortcutImportIssues(m *Model) (tea.Model, tea.Cmd) {
	if sess := m.sidebar.SelectedSession(); sess != nil {
		// Session selected - use its repo
		repoName := filepath.Base(sess.RepoPath)
		m.modal.Show(ui.NewImportIssuesState(sess.RepoPath, repoName))
		return m, m.fetchGitHubIssues(sess.RepoPath)
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
	logger.Log("Shortcut: Opening terminal at worktree: %s", sess.WorkTree)
	return m, openTerminalAtPath(sess.WorkTree)
}

func shortcutViewChanges(m *Model) (tea.Model, tea.Cmd) {
	sess := m.sidebar.SelectedSession()
	// Select the session first so we can display in its chat panel
	if m.activeSession == nil || m.activeSession.ID != sess.ID {
		m.selectSession(sess)
	}
	// Get worktree status and display it in view changes overlay
	status, err := git.GetWorktreeStatus(sess.WorkTree)
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
	return m, nil
}

func shortcutMCPServers(m *Model) (tea.Model, tea.Cmd) {
	m.showMCPServersModal()
	return m, nil
}

func shortcutTheme(m *Model) (tea.Model, tea.Cmd) {
	m.modal.Show(ui.NewThemeState(ui.CurrentThemeName()))
	return m, nil
}

func shortcutHelp(m *Model) (tea.Model, tea.Cmd) {
	// Include help shortcut in the registry for display purposes
	allShortcuts := append(ShortcutRegistry, helpShortcut)
	sections := m.getApplicableHelpSections(allShortcuts, DisplayOnlyShortcuts)
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

func shortcutSettings(m *Model) (tea.Model, tea.Cmd) {
	m.modal.Show(ui.NewSettingsState(m.config.GetDefaultBranchPrefix(), m.config.GetNotificationsEnabled()))
	return m, nil
}

func shortcutWhatsNew(m *Model) (tea.Model, tea.Cmd) {
	return m, m.fetchChangelogAll()
}

// TerminalErrorMsg is sent when opening a terminal fails
type TerminalErrorMsg struct {
	Error string
}

// openTerminalAtPath returns a command that opens a new terminal window at the given path.
// Supports macOS (Terminal.app) and Linux (common terminal emulators).
func openTerminalAtPath(path string) tea.Cmd {
	return func() tea.Msg {
		logger.Log("Shortcut: openTerminalAtPath called for OS=%s, path=%s", runtime.GOOS, path)

		switch runtime.GOOS {
		case "darwin":
			// macOS: use 'open' command with Terminal.app
			// Escape backslashes and quotes for AppleScript string
			escapedPath := strings.ReplaceAll(path, `\`, `\\`)
			escapedPath = strings.ReplaceAll(escapedPath, `"`, `\"`)
			script := fmt.Sprintf(`tell application "Terminal"
	do script "cd \"%s\""
	activate
end tell`, escapedPath)
			cmd := exec.Command("osascript", "-e", script)
			logger.Log("Shortcut: Running AppleScript to open Terminal")

			// Run and capture any error output
			output, err := cmd.CombinedOutput()
			if err != nil {
				errMsg := fmt.Sprintf("Failed to open terminal: %v", err)
				if len(output) > 0 {
					errMsg = fmt.Sprintf("Failed to open terminal: %s", strings.TrimSpace(string(output)))
				}
				logger.Log("Shortcut: %s", errMsg)
				return TerminalErrorMsg{Error: errMsg}
			}
			logger.Log("Shortcut: Terminal opened successfully")
			return nil

		case "linux":
			// Linux: try common terminal emulators in order of preference
			terminals := []struct {
				name string
				args []string
			}{
				{"gnome-terminal", []string{"--working-directory=" + path}},
				{"konsole", []string{"--workdir", path}},
				{"xfce4-terminal", []string{"--working-directory=" + path}},
				{"xterm", []string{"-e", fmt.Sprintf("cd %q && $SHELL", path)}},
			}

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
				logger.Log("Shortcut: %s", errMsg)
				return TerminalErrorMsg{Error: errMsg}
			}
			logger.Log("Shortcut: Terminal opened successfully")
			return nil

		default:
			errMsg := fmt.Sprintf("Unsupported OS for terminal: %s", runtime.GOOS)
			logger.Log("Shortcut: %s", errMsg)
			return TerminalErrorMsg{Error: errMsg}
		}
	}
}
