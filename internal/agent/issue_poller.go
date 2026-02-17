package agent

import (
	"context"
	"strconv"
	"time"

	"github.com/zhubert/plural/internal/issues"
)

// repoIssues groups new issues by repo path.
type repoIssues struct {
	RepoPath string
	Issues   []issues.Issue
}

// pollForIssues checks all repos with issue polling enabled for new issues.
// It filters by label, deduplicates against existing sessions, and respects concurrency limits.
func (a *Agent) pollForIssues(ctx context.Context) []repoIssues {
	log := a.logger.With("component", "issue-poller")
	log.Debug("checking for new issues")

	// Collect repos with issue polling enabled
	repos := a.config.GetRepos()
	var pollingRepos []string
	for _, repoPath := range repos {
		// If repo filter is set, only poll that repo
		if a.repoFilter != "" && repoPath != a.repoFilter {
			continue
		}
		if a.config.GetRepoIssuePolling(repoPath) {
			pollingRepos = append(pollingRepos, repoPath)
		}
	}

	if len(pollingRepos) == 0 {
		log.Debug("no repos with polling enabled")
		return nil
	}

	// Build set of existing issue IDs per repo to deduplicate
	existingSessions := a.config.GetSessions()
	existingIssueIDs := make(map[string]map[string]bool)
	for _, sess := range existingSessions {
		if sess.IssueRef != nil {
			if existingIssueIDs[sess.RepoPath] == nil {
				existingIssueIDs[sess.RepoPath] = make(map[string]bool)
			}
			existingIssueIDs[sess.RepoPath][sess.IssueRef.ID] = true
		}
	}

	// Count active autonomous sessions for concurrency limit
	maxConcurrent := a.getMaxConcurrent()
	activeCount := a.activeWorkerCount()

	pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var allNewIssues []repoIssues
	totalNew := 0

	for _, repoPath := range pollingRepos {
		if activeCount+totalNew >= maxConcurrent {
			log.Debug("max concurrent sessions reached, skipping remaining repos",
				"active", activeCount+totalNew, "max", maxConcurrent)
			break
		}

		ghIssues, err := a.gitService.FetchGitHubIssuesWithLabel(pollCtx, repoPath, autonomousFilterLabel)
		if err != nil {
			log.Debug("failed to fetch issues", "repo", repoPath, "error", err)
			continue
		}

		repoExisting := existingIssueIDs[repoPath]
		var newIssues []issues.Issue
		for _, ghIssue := range ghIssues {
			issueID := strconv.Itoa(ghIssue.Number)
			if repoExisting != nil && repoExisting[issueID] {
				continue
			}
			if activeCount+totalNew+len(newIssues) >= maxConcurrent {
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
			log.Info("detected new issues", "repo", repoPath, "count", len(newIssues))
			allNewIssues = append(allNewIssues, repoIssues{RepoPath: repoPath, Issues: newIssues})
			totalNew += len(newIssues)
		}
	}

	return allNewIssues
}
