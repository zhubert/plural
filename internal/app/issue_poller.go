package app

import (
	"context"
	"fmt"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/issues"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/session"
)

const issuePollInterval = 30 * time.Second

// IssuePollTickMsg triggers an issue polling cycle
type IssuePollTickMsg time.Time

// NewIssuesDetectedMsg carries newly detected issues to be processed.
// May contain issues from multiple repos.
type NewIssuesDetectedMsg struct {
	RepoPath string
	Issues   []issues.Issue
	// Additional repos with new issues (processed after the primary repo)
	AdditionalRepos []repoIssues
}

// repoIssues groups new issues by repo path for collection across all polled repos.
type repoIssues struct {
	RepoPath string
	Issues   []issues.Issue
}

// IssuePollTick returns a command that sends an IssuePollTickMsg after the poll interval
func IssuePollTick() tea.Cmd {
	return tea.Tick(issuePollInterval, func(t time.Time) tea.Msg {
		return IssuePollTickMsg(t)
	})
}

// checkForNewIssues checks all repos with issue polling enabled for new issues.
// It filters by label, deduplicates against existing sessions, and respects concurrency limits.
func checkForNewIssues(cfg *config.Config, gitSvc *git.GitService, existingSessions []config.Session) tea.Cmd {
	log := logger.WithComponent("issue-poller")
	log.Debug("checking for new issues")

	// Collect repos with issue polling enabled
	repos := cfg.GetRepos()
	log.Debug("repos found", "count", len(repos))

	type repoPolling struct {
		Path  string
		Label string
	}
	var pollingRepos []repoPolling
	for _, repoPath := range repos {
		enabled := cfg.GetRepoIssuePolling(repoPath)
		log.Debug("checking repo", "path", repoPath, "polling_enabled", enabled)
		if enabled {
			label := cfg.GetRepoIssueLabels(repoPath)
			log.Debug("adding repo to polling list", "path", repoPath, "label", label)
			pollingRepos = append(pollingRepos, repoPolling{
				Path:  repoPath,
				Label: label,
			})
		}
	}

	log.Debug("polling repos collected", "count", len(pollingRepos))
	if len(pollingRepos) == 0 {
		log.Debug("no repos with polling enabled, skipping")
		return nil
	}

	// Build set of existing issue IDs per repo to deduplicate
	existingIssueIDs := make(map[string]map[string]bool) // repoPath -> issueID -> true
	for _, sess := range existingSessions {
		if sess.IssueRef != nil {
			if existingIssueIDs[sess.RepoPath] == nil {
				existingIssueIDs[sess.RepoPath] = make(map[string]bool)
			}
			existingIssueIDs[sess.RepoPath][sess.IssueRef.ID] = true
		}
	}

	// Count active autonomous sessions for concurrency limit
	maxConcurrent := cfg.GetIssueMaxConcurrent()
	activeAutoCount := 0
	for _, sess := range existingSessions {
		if sess.Autonomous {
			activeAutoCount++
		}
	}

	return func() tea.Msg {
		log := logger.WithComponent("issue-poller")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Collect new issues from all repos (not just the first one with issues)
		var allNewIssues []repoIssues
		totalNew := 0

		for _, repo := range pollingRepos {
			if activeAutoCount+totalNew >= maxConcurrent {
				log.Debug("max concurrent auto-sessions reached, skipping remaining repos",
					"active", activeAutoCount+totalNew, "max", maxConcurrent)
				break
			}

			ghIssues, err := gitSvc.FetchGitHubIssuesWithLabel(ctx, repo.Path, repo.Label)
			if err != nil {
				log.Debug("failed to fetch issues", "repo", repo.Path, "error", err)
				continue
			}

			// Filter out issues that already have sessions
			repoExisting := existingIssueIDs[repo.Path]
			var newIssues []issues.Issue
			for _, ghIssue := range ghIssues {
				issueID := strconv.Itoa(ghIssue.Number)
				if repoExisting != nil && repoExisting[issueID] {
					continue
				}
				// Respect concurrency limit
				if activeAutoCount+totalNew+len(newIssues) >= maxConcurrent {
					break
				}
				newIssues = append(newIssues, issues.Issue{
					ID:     issueID,
					Title:  ghIssue.Title,
					Body:   ghIssue.Body,
					URL:    ghIssue.URL,
					Source: issues.SourceGitHub,
				})
			}

			if len(newIssues) > 0 {
				log.Info("detected new issues", "repo", repo.Path, "count", len(newIssues))
				allNewIssues = append(allNewIssues, repoIssues{RepoPath: repo.Path, Issues: newIssues})
				totalNew += len(newIssues)
			}
		}

		if len(allNewIssues) == 0 {
			return nil
		}

		// Return all repos' issues so none are dropped between poll cycles
		msg := NewIssuesDetectedMsg{
			RepoPath: allNewIssues[0].RepoPath,
			Issues:   allNewIssues[0].Issues,
		}
		if len(allNewIssues) > 1 {
			msg.AdditionalRepos = allNewIssues[1:]
		}
		return msg
	}
}

// handleNewIssuesDetectedMsg creates autonomous containerized sessions for newly detected issues.
func (m *Model) handleNewIssuesDetectedMsg(msg NewIssuesDetectedMsg) (tea.Model, tea.Cmd) {
	log := logger.WithComponent("issue-poller")

	// Process primary repo
	log.Info("creating autonomous sessions for new issues", "repo", msg.RepoPath, "count", len(msg.Issues))
	var issueItems []issueAutoInfo
	for _, issue := range msg.Issues {
		issueItems = append(issueItems, issueAutoInfo{Issue: issue})
	}
	_, cmd := m.createAutonomousIssueSessions(msg.RepoPath, issueItems)

	var cmds []tea.Cmd
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	// Process additional repos
	for _, repo := range msg.AdditionalRepos {
		log.Info("creating autonomous sessions for new issues", "repo", repo.RepoPath, "count", len(repo.Issues))
		var items []issueAutoInfo
		for _, issue := range repo.Issues {
			items = append(items, issueAutoInfo{Issue: issue})
		}
		_, extraCmd := m.createAutonomousIssueSessions(repo.RepoPath, items)
		if extraCmd != nil {
			cmds = append(cmds, extraCmd)
		}
	}

	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// issueAutoInfo holds issue info for autonomous session creation.
type issueAutoInfo struct {
	Issue issues.Issue
}

// createAutonomousIssueSessions creates autonomous containerized sessions for issues.
func (m *Model) createAutonomousIssueSessions(repoPath string, issueInfos []issueAutoInfo) (tea.Model, tea.Cmd) {
	branchPrefix := m.config.GetDefaultBranchPrefix()
	ctx := context.Background()
	log := logger.WithComponent("issue-poller")

	var cmds []tea.Cmd
	created := 0
	var firstCreatedSession *config.Session

	for _, info := range issueInfos {
		issue := info.Issue

		// Generate branch name
		var branchName string
		if m.issueRegistry != nil {
			provider := m.issueRegistry.GetProvider(issue.Source)
			if provider != nil {
				branchName = provider.GenerateBranchName(issue)
			}
		}
		if branchName == "" {
			branchName = fmt.Sprintf("issue-%s", issue.ID)
		}

		fullBranchName := branchPrefix + branchName

		// Check if branch already exists
		if m.sessionService.BranchExists(ctx, repoPath, fullBranchName) {
			log.Debug("skipping issue - branch already exists", "issue", issue.ID, "branch", fullBranchName)
			continue
		}

		// Create new session
		sess, err := m.sessionService.Create(ctx, repoPath, branchName, branchPrefix, session.BasePointOrigin)
		if err != nil {
			log.Error("failed to create session for issue", "issue", issue.ID, "error", err)
			continue
		}

		// Configure as autonomous, containerized supervisor.
		// Containerized is required for autonomous mode (sandbox = the container).
		// IsSupervisor enables delegation to child sessions for parallel work.
		sess.Autonomous = true
		sess.Containerized = true
		sess.IsSupervisor = true
		sess.IssueRef = &config.IssueRef{
			Source: string(issue.Source),
			ID:     issue.ID,
			Title:  issue.Title,
			URL:    issue.URL,
		}

		// Auto-assign to active workspace
		if activeWS := m.config.GetActiveWorkspaceID(); activeWS != "" {
			sess.WorkspaceID = activeWS
		}

		m.config.AddSession(*sess)
		created++
		if firstCreatedSession == nil {
			firstCreatedSession = sess
		}

		// Build initial message — just the issue content.
		// Orchestrator instructions are in the system prompt (SupervisorSystemPrompt).
		initialMsg := fmt.Sprintf("GitHub Issue #%s: %s\n\n%s",
			issue.ID, issue.Title, issue.Body)

		// Start the session
		result := m.sessionMgr.Select(sess, "", "", "")
		if result == nil || result.Runner == nil {
			logger.WithSession(sess.ID).Error("failed to get runner for auto issue session")
			continue
		}

		runner := result.Runner
		sendCtx, cancel := context.WithCancel(context.Background())
		m.sessionState().StartWaiting(sess.ID, cancel)
		m.sidebar.SetStreaming(sess.ID, true)

		content := []claude.ContentBlock{{Type: claude.ContentTypeText, Text: initialMsg}}
		responseChan := runner.SendContent(sendCtx, content)
		cmds = append(cmds, m.sessionListeners(sess.ID, runner, responseChan)...)
	}

	if created > 0 {
		if cmd := m.saveConfigOrFlash(); cmd != nil {
			cmds = append(cmds, cmd)
		}
		m.sidebar.SetSessions(m.getFilteredSessions())

		// Auto-select the new session if user is browsing the sidebar
		if firstCreatedSession != nil && m.focus == FocusSidebar {
			m.sidebar.SelectSession(firstCreatedSession.ID)
			m.selectSession(firstCreatedSession)
			// Keep focus on sidebar (selectSession moves it to chat)
			m.focus = FocusSidebar
			m.sidebar.SetFocused(true)
			m.chat.SetFocused(false)
		}

		cmds = append(cmds, m.ShowFlashInfo(fmt.Sprintf("Auto-created %d session(s) from issues", created)))
		cmds = append(cmds, m.sidebar.SidebarTick(), m.chat.SpinnerTick())
		// Only transition to streaming state if not already there, to avoid
		// disrupting user interaction in the currently active session.
		if m.state != StateStreamingClaude {
			m.setState(StateStreamingClaude)
		}
	}

	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}
