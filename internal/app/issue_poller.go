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
	"github.com/zhubert/plural/internal/ui"
)

const issuePollInterval = 2 * time.Minute

// IssuePollTickMsg triggers an issue polling cycle
type IssuePollTickMsg time.Time

// NewIssuesDetectedMsg carries newly detected issues to be processed
type NewIssuesDetectedMsg struct {
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
	// Collect repos with issue polling enabled
	repos := cfg.GetRepos()
	type repoPolling struct {
		Path  string
		Label string
	}
	var pollingRepos []repoPolling
	for _, repoPath := range repos {
		if cfg.GetRepoIssuePolling(repoPath) {
			pollingRepos = append(pollingRepos, repoPolling{
				Path:  repoPath,
				Label: cfg.GetRepoIssueLabels(repoPath),
			})
		}
	}

	if len(pollingRepos) == 0 {
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

		for _, repo := range pollingRepos {
			if activeAutoCount >= maxConcurrent {
				log.Debug("max concurrent auto-sessions reached, skipping remaining repos",
					"active", activeAutoCount, "max", maxConcurrent)
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
				if activeAutoCount+len(newIssues) >= maxConcurrent {
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
				return NewIssuesDetectedMsg{
					RepoPath: repo.Path,
					Issues:   newIssues,
				}
			}
		}

		return nil
	}
}

// handleNewIssuesDetectedMsg creates autonomous containerized sessions for newly detected issues.
func (m *Model) handleNewIssuesDetectedMsg(msg NewIssuesDetectedMsg) (tea.Model, tea.Cmd) {
	log := logger.WithComponent("issue-poller")
	log.Info("creating autonomous sessions for new issues", "repo", msg.RepoPath, "count", len(msg.Issues))

	// Convert to IssueItems and use createSessionsFromIssuesAutonomous
	var issueItems []issueAutoInfo
	for _, issue := range msg.Issues {
		issueItems = append(issueItems, issueAutoInfo{
			Issue: issue,
		})
	}

	return m.createAutonomousIssueSessions(msg.RepoPath, issueItems)
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

		// Configure as autonomous and containerized
		sess.Autonomous = true
		sess.IsSupervisor = true
		sess.Containerized = true
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

		// Build initial message
		initialMsg := fmt.Sprintf("GitHub Issue #%s: %s\n\n%s\n\n---\nPlease help me work on this issue.",
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
		if err := m.config.Save(); err != nil {
			log.Error("failed to save config after creating issue sessions", "error", err)
		}
		m.sidebar.SetSessions(m.getFilteredSessions())
		cmds = append(cmds, m.ShowFlashInfo(fmt.Sprintf("Auto-created %d session(s) from issues", created)))
		cmds = append(cmds, ui.SidebarTick(), ui.StopwatchTick())
		m.setState(StateStreamingClaude)
	}

	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, nil
}
