package agent

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/issues"
	"github.com/zhubert/plural/internal/workflow"
)

// pollForNewIssues checks for new issues and creates work items for them.
func (d *Daemon) pollForNewIssues(ctx context.Context) {
	log := d.logger.With("component", "issue-poller")

	if d.repoFilter == "" {
		log.Debug("no repo filter set, skipping issue polling")
		return
	}

	// Check concurrency
	maxConcurrent := d.getMaxConcurrent()
	activeSlots := d.activeSlotCount()
	queuedCount := len(d.state.GetWorkItemsByState(WorkItemQueued))

	if activeSlots+queuedCount >= maxConcurrent {
		log.Debug("at concurrency limit, skipping poll",
			"active", activeSlots, "queued", queuedCount, "max", maxConcurrent)
		return
	}

	// Find matching repos
	repos := d.config.GetRepos()
	var pollingRepos []string
	for _, repoPath := range repos {
		if d.matchesRepoFilter(ctx, repoPath) {
			pollingRepos = append(pollingRepos, repoPath)
		}
	}

	if len(pollingRepos) == 0 {
		log.Debug("no repos to poll")
		return
	}

	pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	for _, repoPath := range pollingRepos {
		remaining := maxConcurrent - activeSlots - queuedCount
		if remaining <= 0 {
			break
		}

		wfCfg := d.getWorkflowConfig(repoPath)
		provider := issues.Source(wfCfg.Source.Provider)

		fetchedIssues, err := d.fetchIssuesForProvider(pollCtx, repoPath, wfCfg)
		if err != nil {
			log.Debug("failed to fetch issues", "repo", repoPath, "provider", provider, "error", err)
			continue
		}

		for _, issue := range fetchedIssues {
			if remaining <= 0 {
				break
			}

			// Check if we already have a work item for this issue
			if d.state.HasWorkItemForIssue(string(provider), issue.ID) {
				continue
			}

			// Also check config sessions for deduplication
			if d.hasExistingSession(repoPath, issue.ID) {
				continue
			}

			item := &WorkItem{
				ID: fmt.Sprintf("%s-%s", repoPath, issue.ID),
				IssueRef: config.IssueRef{
					Source: string(provider),
					ID:     issue.ID,
					Title:  issue.Title,
					URL:    issue.URL,
				},
			}

			d.state.AddWorkItem(item)
			queuedCount++
			remaining--

			log.Info("queued new issue", "issue", issue.ID, "title", issue.Title, "provider", provider)
		}
	}
}

// fetchIssuesForProvider fetches issues using the appropriate provider.
func (d *Daemon) fetchIssuesForProvider(ctx context.Context, repoPath string, wfCfg *workflow.Config) ([]issues.Issue, error) {
	provider := issues.Source(wfCfg.Source.Provider)

	switch provider {
	case issues.SourceGitHub:
		label := wfCfg.Source.Filter.Label
		if label == "" {
			label = autonomousFilterLabel
		}
		ghIssues, err := d.gitService.FetchGitHubIssuesWithLabel(ctx, repoPath, label)
		if err != nil {
			return nil, err
		}
		result := make([]issues.Issue, 0, len(ghIssues))
		for _, ghIssue := range ghIssues {
			result = append(result, issues.Issue{
				ID:     strconv.Itoa(ghIssue.Number),
				Title:  ghIssue.Title,
				Body:   ghIssue.Body,
				URL:    ghIssue.URL,
				Source: issues.SourceGitHub,
			})
		}
		return result, nil

	case issues.SourceAsana, issues.SourceLinear:
		p := d.issueRegistry.GetProvider(provider)
		if p == nil {
			return nil, fmt.Errorf("provider %q not registered", provider)
		}
		filterID := wfCfg.Source.Filter.Project
		if provider == issues.SourceLinear {
			filterID = wfCfg.Source.Filter.Team
		}
		return p.FetchIssues(ctx, repoPath, filterID)

	default:
		return nil, fmt.Errorf("unknown provider %q", provider)
	}
}

// startQueuedItems starts coding on queued work items that have available slots.
func (d *Daemon) startQueuedItems(ctx context.Context) {
	maxConcurrent := d.getMaxConcurrent()
	queued := d.state.GetWorkItemsByState(WorkItemQueued)

	for _, item := range queued {
		if d.activeSlotCount() >= maxConcurrent {
			break
		}
		d.startCoding(ctx, item)
	}
}

// Helper methods adapted from Agent

// matchesRepoFilter checks if a repo path matches the daemon's repo filter.
func (d *Daemon) matchesRepoFilter(ctx context.Context, repoPath string) bool {
	if repoPath == d.repoFilter {
		return true
	}
	if strings.Contains(d.repoFilter, "/") && !strings.HasPrefix(d.repoFilter, "/") {
		remoteURL, err := d.gitService.GetRemoteOriginURL(ctx, repoPath)
		if err != nil {
			return false
		}
		ownerRepo := git.ExtractOwnerRepo(remoteURL)
		return ownerRepo == d.repoFilter
	}
	return false
}

// hasExistingSession checks if a session already exists for the given issue.
func (d *Daemon) hasExistingSession(repoPath, issueID string) bool {
	for _, sess := range d.config.GetSessions() {
		if sess.RepoPath == repoPath && sess.IssueRef != nil && sess.IssueRef.ID == issueID {
			return true
		}
	}
	return false
}
