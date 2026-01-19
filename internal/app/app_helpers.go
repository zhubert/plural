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
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/ui"
)

// =============================================================================
// Focus Management
// =============================================================================

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

// =============================================================================
// Session Selection
// =============================================================================

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
	if pendingMsg := m.sessionState().PeekPendingMessage(sess.ID); pendingMsg != "" {
		m.chat.SetQueuedMessage(pendingMsg)
	} else {
		m.chat.ClearQueuedMessage()
	}

	logger.Log("App: Session selected and focused: %s", sess.ID)
}

// =============================================================================
// Image Handling
// =============================================================================

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

// =============================================================================
// Message Sending
// =============================================================================

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

// =============================================================================
// Git Operations
// =============================================================================

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

// =============================================================================
// Option Detection
// =============================================================================

// detectOptionsInSession scans the runner's messages for numbered options
func (m *Model) detectOptionsInSession(sessionID string, runner claude.RunnerInterface) {
	msgs := runner.GetMessages()
	if len(msgs) == 0 {
		m.sessionState().ClearDetectedOptions(sessionID)
		return
	}

	// Find the last assistant message
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			options := DetectOptions(msgs[i].Content)
			if len(options) >= 2 {
				logger.Log("App: Detected %d options in session %s", len(options), sessionID)
				m.sessionState().SetDetectedOptions(sessionID, options)
				return
			}
			break // Only check the most recent assistant message
		}
	}

	// No options found
	m.sessionState().ClearDetectedOptions(sessionID)
}

// showExploreOptionsModal displays the modal for selecting options to explore in parallel
func (m *Model) showExploreOptionsModal() (tea.Model, tea.Cmd) {
	if m.activeSession == nil {
		return m, nil
	}

	options := m.sessionState().GetDetectedOptions(m.activeSession.ID)
	if len(options) < 2 {
		return m, nil
	}

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

// =============================================================================
// Streaming Helpers
// =============================================================================

// hasAnyStreamingSessions returns true if any session is currently streaming
func (m *Model) hasAnyStreamingSessions() bool {
	return m.sessionMgr.HasActiveStreaming()
}

// HasActiveStreaming returns true if any session is currently streaming (public for demos).
func (m *Model) HasActiveStreaming() bool {
	return m.sessionMgr.HasActiveStreaming()
}

// =============================================================================
// Startup Modals
// =============================================================================

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

// =============================================================================
// GitHub Issues
// =============================================================================

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
// MCP Servers & Plugins Modals
// =============================================================================

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
