package app

import (
	"context"
	"fmt"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/session"
	"github.com/zhubert/plural/internal/ui"
)

// handleExploreOptionsModal handles key events for the Explore Options modal.
func (m *Model) handleExploreOptionsModal(key string, msg tea.KeyPressMsg, state *ui.ExploreOptionsState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.modal.Hide()
		return m, nil
	case "enter":
		selected := state.GetSelectedOptions()
		if len(selected) == 0 {
			return m, nil
		}
		m.modal.Hide()
		return m.createParallelSessions(selected)
	case "up", "k", "down", "j", "space":
		// Forward navigation and space (toggle) keys to modal
		modal, cmd := m.modal.Update(msg)
		m.modal = modal
		return m, cmd
	}
	return m, nil
}

// handleSelectRepoForIssuesModal handles key events for the Select Repo for Issues modal.
func (m *Model) handleSelectRepoForIssuesModal(key string, msg tea.KeyPressMsg, state *ui.SelectRepoForIssuesState) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.modal.Hide()
		return m, nil
	case "enter":
		repoPath := state.GetSelectedRepo()
		if repoPath == "" {
			return m, nil
		}
		repoName := filepath.Base(repoPath)
		m.modal.Show(ui.NewImportIssuesState(repoPath, repoName))
		return m, m.fetchGitHubIssues(repoPath)
	case "up", "k", "down", "j":
		// Forward navigation keys to modal
		modal, cmd := m.modal.Update(msg)
		m.modal = modal
		return m, cmd
	}
	return m, nil
}

// handleImportIssuesModal handles key events for the Import Issues modal.
func (m *Model) handleImportIssuesModal(key string, msg tea.KeyPressMsg, state *ui.ImportIssuesState) (tea.Model, tea.Cmd) {
	// Don't handle keys while loading
	if state.Loading {
		return m, nil
	}

	switch key {
	case "esc":
		m.modal.Hide()
		return m, nil
	case "enter":
		selected := state.GetSelectedIssues()
		if len(selected) == 0 {
			return m, nil
		}
		m.modal.Hide()
		return m.createSessionsFromIssues(state.RepoPath, selected)
	case "up", "k", "down", "j", "space":
		// Forward navigation and space (toggle) keys to modal
		modal, cmd := m.modal.Update(msg)
		m.modal = modal
		return m, cmd
	}
	return m, nil
}

// issueSessionInfo holds info needed to start an issue session after creation.
type issueSessionInfo struct {
	Session    *config.Session
	InitialMsg string
}

// createSessionsFromIssues creates new sessions for each selected GitHub issue.
func (m *Model) createSessionsFromIssues(repoPath string, issues []ui.IssueItem) (tea.Model, tea.Cmd) {
	branchPrefix := m.config.GetDefaultBranchPrefix()

	var createdSessions []issueSessionInfo
	var firstSession *config.Session
	var failedIssues []int

	for _, issue := range issues {
		// Create branch name from issue number
		branchName := fmt.Sprintf("issue-%d", issue.Number)
		fullBranchName := branchPrefix + branchName

		// Check if branch already exists and skip if so
		ctx := context.Background()
		if m.sessionService.BranchExists(ctx, repoPath, fullBranchName) {
			logger.Get().Debug("skipping issue - branch already exists", "issue", issue.Number, "branch", fullBranchName)
			continue
		}

		// Create new session (always from origin for issue-based sessions)
		sess, err := m.sessionService.Create(ctx, repoPath, branchName, branchPrefix, session.BasePointOrigin)
		if err != nil {
			logger.Get().Error("failed to create session for issue", "issue", issue.Number, "error", err)
			failedIssues = append(failedIssues, issue.Number)
			continue
		}

		// Store the issue number so we can reference it in the PR
		sess.IssueNumber = issue.Number

		// Create initial message with issue context
		initialMsg := fmt.Sprintf("GitHub Issue #%d: %s\n\n%s\n\n---\nPlease help me work on this issue.",
			issue.Number, issue.Title, issue.Body)

		// No parent ID - these are top-level sessions
		logger.WithSession(sess.ID).Info("created session for issue", "issue", issue.Number, "name", sess.Name)

		m.config.AddSession(*sess)
		createdSessions = append(createdSessions, issueSessionInfo{
			Session:    sess,
			InitialMsg: initialMsg,
		})

		if firstSession == nil {
			firstSession = sess
		}
	}

	// Save config and update sidebar
	var cmds []tea.Cmd
	if err := m.config.Save(); err != nil {
		logger.Get().Error("failed to save config", "error", err)
		cmds = append(cmds, m.ShowFlashError("Failed to save configuration"))
	}
	m.sidebar.SetSessions(m.config.GetSessions())

	// Show flash message for any failed session creations
	if len(failedIssues) > 0 {
		if len(failedIssues) == 1 {
			cmds = append(cmds, m.ShowFlashError(fmt.Sprintf("Failed to create session for issue #%d", failedIssues[0])))
		} else {
			cmds = append(cmds, m.ShowFlashError(fmt.Sprintf("Failed to create sessions for %d issues", len(failedIssues))))
		}
	}

	// Start all sessions in parallel (similar to createParallelSessions)
	if len(createdSessions) > 0 {
		for _, info := range createdSessions {
			sess := info.Session
			initialMsg := info.InitialMsg

			// Get or create runner for this session
			result := m.sessionMgr.Select(sess, "", "", "")
			if result == nil || result.Runner == nil {
				logger.WithSession(sess.ID).Error("failed to get runner for issue session")
				continue
			}

			runner := result.Runner

			// Start streaming for this session
			ctx, cancel := context.WithCancel(context.Background())
			m.sessionState().StartWaiting(sess.ID, cancel)
			m.sidebar.SetStreaming(sess.ID, true)

			logger.WithSession(sess.ID).Debug("auto-starting issue session", "issue", sess.IssueNumber)

			// Send the initial message to Claude
			content := []claude.ContentBlock{{Type: claude.ContentTypeText, Text: initialMsg}}
			responseChan := runner.SendContent(ctx, content)

			// Add listeners for this session
			cmds = append(cmds, m.sessionListeners(sess.ID, runner, responseChan)...)
		}

		// Switch to the first session's UI
		if firstSession != nil {
			m.sidebar.SelectSession(firstSession.ID)
			m.selectSession(firstSession)

			// Update UI for the active session
			if m.claudeRunner != nil {
				startTime, _ := m.sessionState().GetWaitStart(firstSession.ID)
				m.chat.SetWaitingWithStart(true, startTime)
			}
		}

		m.setState(StateStreamingClaude)
		cmds = append(cmds, ui.SidebarTick(), ui.StopwatchTick())
	}

	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// parallelSessionInfo holds info needed to start a session after creation.
type parallelSessionInfo struct {
	Session      *config.Session
	OptionPrompt string
}

// createParallelSessions creates new sessions for each selected option, pre-populated with history.
func (m *Model) createParallelSessions(selectedOptions []ui.OptionItem) (tea.Model, tea.Cmd) {
	if m.activeSession == nil || m.claudeRunner == nil {
		return m, nil
	}

	parentSession := m.activeSession
	parentMessages := m.claudeRunner.GetMessages()

	logger.WithSession(parentSession.ID).Info("creating parallel sessions", "count", len(selectedOptions))

	// Generate branch names for all options in a single Claude call
	ctx := context.Background()
	optionsForClaude := make([]struct {
		Number int
		Text   string
	}, len(selectedOptions))
	for i, opt := range selectedOptions {
		optionsForClaude[i] = struct {
			Number int
			Text   string
		}{Number: opt.Number, Text: opt.Text}
	}
	branchNames, err := m.gitService.GenerateBranchNamesFromOptions(ctx, optionsForClaude)
	if err != nil {
		logger.Get().Warn("failed to generate branch names with Claude, using fallback names", "error", err)
		branchNames = make(map[int]string) // Will use fallback names
	}

	branchPrefix := m.config.GetDefaultBranchPrefix()

	var cmds []tea.Cmd
	var createdSessions []parallelSessionInfo
	var firstSession *config.Session

	for _, opt := range selectedOptions {
		// Use generated branch name or fallback
		branchName, ok := branchNames[opt.Number]
		if !ok || branchName == "" {
			branchName = fmt.Sprintf("option-%d", opt.Number)
		}

		// Create new session forked from parent's branch
		sess, err := m.sessionService.CreateFromBranch(ctx, parentSession.RepoPath, parentSession.Branch, branchName, branchPrefix)
		if err != nil {
			logger.Get().Error("failed to create parallel session for option", "option", opt.Number, "error", err)
			m.chat.AppendStreaming(fmt.Sprintf("[Error creating session for option %d: %v]\n", opt.Number, err))
			continue
		}

		logger.WithSession(sess.ID).Debug("created parallel session for option", "option", opt.Number)

		// Build message history: parent messages only (option prompt will be added by SendContent)
		var messages []config.Message
		for _, msg := range parentMessages {
			messages = append(messages, config.Message{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}

		// Option prompt to send (will be added to history by SendContent)
		optionPrompt := fmt.Sprintf("Let's go with option %d: %s", opt.Number, opt.Text)

		// Save parent messages to disk for this new session
		if err := config.SaveSessionMessages(sess.ID, messages, config.MaxSessionMessageLines); err != nil {
			logger.WithSession(sess.ID).Warn("failed to save messages for parallel session", "error", err)
		}

		// Set parent ID to track fork relationship
		sess.ParentID = parentSession.ID

		// Add session to config
		m.config.AddSession(*sess)
		createdSessions = append(createdSessions, parallelSessionInfo{
			Session:      sess,
			OptionPrompt: optionPrompt,
		})

		if firstSession == nil {
			firstSession = sess
		}
	}

	// Save config
	if err := m.config.Save(); err != nil {
		logger.Get().Error("failed to save config after creating parallel sessions", "error", err)
	}

	// Update sidebar
	m.sidebar.SetSessions(m.config.GetSessions())

	// Clear detected options since we've acted on them
	if state := m.sessionState().GetIfExists(parentSession.ID); state != nil {
		state.DetectedOptions = nil
	}

	// Start all sessions in parallel
	if len(createdSessions) > 0 {
		m.chat.AppendStreaming(fmt.Sprintf("\nCreated %d parallel session(s) to explore options.\n", len(createdSessions)))

		// Start each session
		for _, info := range createdSessions {
			sess := info.Session
			optionPrompt := info.OptionPrompt

			// Get or create runner for this session (this loads pre-populated messages)
			result := m.sessionMgr.Select(sess, "", "", "")
			if result == nil || result.Runner == nil {
				logger.WithSession(sess.ID).Error("failed to get runner for parallel session")
				continue
			}

			runner := result.Runner

			// Start streaming for this session
			ctx, cancel := context.WithCancel(context.Background())
			m.sessionState().StartWaiting(sess.ID, cancel)
			m.sidebar.SetStreaming(sess.ID, true)

			logger.WithSession(sess.ID).Debug("auto-starting parallel session", "prompt", optionPrompt)

			// Send the option choice to Claude
			content := []claude.ContentBlock{{Type: claude.ContentTypeText, Text: optionPrompt}}
			responseChan := runner.SendContent(ctx, content)

			// Add listeners for this session
			cmds = append(cmds, m.sessionListeners(sess.ID, runner, responseChan)...)
		}

		// Switch to the first session's UI
		if firstSession != nil {
			m.sidebar.SelectSession(firstSession.ID)
			m.selectSession(firstSession)

			// Update UI for the active session - the user message is already in the runner's
			// message history (added by SendContent) and selectSession sets the chat messages
			// from the runner, so we don't need to add it again here
			if m.claudeRunner != nil {
				startTime, _ := m.sessionState().GetWaitStart(firstSession.ID)
				m.chat.SetWaitingWithStart(true, startTime)
			}
		}

		m.setState(StateStreamingClaude)
		cmds = append(cmds, ui.SidebarTick(), ui.StopwatchTick())
	}

	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}
