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

			// Swap labels in the background (GitHub only)
			if provider == issues.SourceGitHub {
				go d.swapIssueLabels(repoPath, issue)
			}
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

// processAwaitingReview handles items waiting for review by checking for new comments and review decisions.
func (d *Daemon) processAwaitingReview(ctx context.Context, item *WorkItem) {
	log := d.logger.With("workItem", item.ID, "branch", item.Branch)

	sess := d.config.GetSession(item.SessionID)
	if sess == nil {
		log.Warn("session not found for work item")
		return
	}

	pollCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Check if PR was closed
	prState, err := d.gitService.GetPRState(pollCtx, sess.RepoPath, item.Branch)
	if err != nil {
		log.Debug("failed to check PR state", "error", err)
		return
	}

	if prState == git.PRStateClosed {
		log.Info("PR was closed, marking as abandoned")
		d.state.TransitionWorkItem(item.ID, WorkItemAbandoned)
		return
	}

	if prState == git.PRStateMerged {
		log.Info("PR was merged externally")
		d.state.TransitionWorkItem(item.ID, WorkItemAwaitingCI)
		d.state.TransitionWorkItem(item.ID, WorkItemMerging)
		d.state.TransitionWorkItem(item.ID, WorkItemCompleted)
		d.removeIssueWIPLabel(sess)
		return
	}

	// Check for new review comments
	results, err := d.gitService.GetBatchPRStatesWithComments(pollCtx, sess.RepoPath, []string{item.Branch})
	if err != nil {
		log.Debug("failed to check PR comments", "error", err)
		return
	}

	result, ok := results[item.Branch]
	if !ok {
		return
	}

	if result.CommentCount > item.CommentsAddressed {
		log.Info("new review comments detected",
			"addressed", item.CommentsAddressed,
			"current", result.CommentCount,
		)

		// Check max feedback rounds from workflow config
		wfCfg := d.getWorkflowConfig(sess.RepoPath)
		if wfCfg.Workflow.Review.MaxFeedbackRounds != nil && item.FeedbackRounds >= *wfCfg.Workflow.Review.MaxFeedbackRounds {
			log.Warn("max feedback rounds reached, skipping",
				"rounds", item.FeedbackRounds,
				"max", *wfCfg.Workflow.Review.MaxFeedbackRounds,
			)
			return
		}

		// Check concurrency before starting feedback
		if d.activeSlotCount() >= d.getMaxConcurrent() {
			log.Debug("no concurrency slot available for feedback, deferring")
			return
		}

		d.addressFeedback(ctx, item)
		return
	}

	// Check review decision
	reviewDecision, err := d.gitService.CheckPRReviewDecision(pollCtx, sess.RepoPath, item.Branch)
	if err != nil {
		log.Debug("failed to check review decision", "error", err)
		return
	}

	if reviewDecision == git.ReviewApproved {
		log.Info("PR approved, transitioning to awaiting CI")
		if err := d.state.TransitionWorkItem(item.ID, WorkItemAwaitingCI); err != nil {
			log.Error("failed to transition to awaiting_ci", "error", err)
		}
	}
}

// processAwaitingCI handles items waiting for CI by checking CI status and merging if passing.
func (d *Daemon) processAwaitingCI(ctx context.Context, item *WorkItem) {
	log := d.logger.With("workItem", item.ID, "branch", item.Branch)

	sess := d.config.GetSession(item.SessionID)
	if sess == nil {
		log.Warn("session not found for work item")
		return
	}

	pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ciStatus, err := d.gitService.CheckPRChecks(pollCtx, sess.RepoPath, item.Branch)
	if err != nil {
		log.Debug("failed to check CI status", "error", err)
		return
	}

	switch ciStatus {
	case git.CIStatusPassing, git.CIStatusNone:
		if !d.autoMerge {
			log.Info("CI passed but auto-merge disabled")
			return
		}

		log.Info("CI passed, merging PR")
		if err := d.state.TransitionWorkItem(item.ID, WorkItemMerging); err != nil {
			log.Error("failed to transition to merging", "error", err)
			return
		}

		if err := d.mergePR(ctx, item); err != nil {
			log.Error("merge failed", "error", err)
			d.state.SetErrorMessage(item.ID, fmt.Sprintf("merge failed: %v", err))
			d.state.TransitionWorkItem(item.ID, WorkItemFailed)
			return
		}

		d.state.TransitionWorkItem(item.ID, WorkItemCompleted)
		d.removeIssueWIPLabel(sess)

		// Run merge after-hooks
		wfCfg := d.getWorkflowConfig(sess.RepoPath)
		d.runWorkflowHooks(ctx, wfCfg.Workflow.Merge.After, item, sess)

		log.Info("PR merged successfully")

	case git.CIStatusFailing:
		wfCfg := d.getWorkflowConfig(sess.RepoPath)
		onFailure := wfCfg.Workflow.CI.OnFailure
		if onFailure == "" {
			onFailure = "retry"
		}

		switch onFailure {
		case "abandon":
			log.Warn("CI failed, abandoning (on_failure=abandon)")
			d.state.TransitionWorkItem(item.ID, WorkItemAbandoned)
		case "notify":
			log.Warn("CI failed (on_failure=notify)")
			d.state.SetErrorMessage(item.ID, "CI failed")
			d.state.TransitionWorkItem(item.ID, WorkItemFailed)
		default: // "retry"
			log.Warn("CI failed, transitioning to awaiting review (on_failure=retry)")
			d.state.TransitionWorkItem(item.ID, WorkItemAwaitingReview)
		}

	case git.CIStatusPending:
		log.Debug("CI still pending")
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

// swapIssueLabels removes the filter label and adds "wip" label on a GitHub issue.
// This is a no-op for non-GitHub providers.
func (d *Daemon) swapIssueLabels(repoPath string, issue issues.Issue) {
	if issue.Source != issues.SourceGitHub {
		return
	}

	issueNum, err := strconv.Atoi(issue.ID)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use configured label instead of hardcoded constant
	wfCfg := d.getWorkflowConfig(repoPath)
	filterLabel := wfCfg.Source.Filter.Label
	if filterLabel == "" {
		filterLabel = autonomousFilterLabel
	}

	if err := d.gitService.RemoveIssueLabel(ctx, repoPath, issueNum, filterLabel); err != nil {
		d.logger.Error("failed to remove issue label", "issue", issueNum, "error", err)
	}
	if err := d.gitService.AddIssueLabel(ctx, repoPath, issueNum, autonomousWIPLabel); err != nil {
		d.logger.Error("failed to add wip label", "issue", issueNum, "error", err)
	}
	comment := "This issue has been picked up by [Plural](https://github.com/zhubert/plural) and is being worked on autonomously."
	if err := d.gitService.CommentOnIssue(ctx, repoPath, issueNum, comment); err != nil {
		d.logger.Error("failed to comment on issue", "issue", issueNum, "error", err)
	}
}

// removeIssueWIPLabel removes the "wip" label from a session's issue.
// This is a no-op for non-GitHub providers.
func (d *Daemon) removeIssueWIPLabel(sess *config.Session) {
	if sess.IssueRef == nil {
		return
	}
	if issues.Source(sess.IssueRef.Source) != issues.SourceGitHub {
		return
	}
	issueNum, err := strconv.Atoi(sess.IssueRef.ID)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := d.gitService.RemoveIssueLabel(ctx, sess.RepoPath, issueNum, autonomousWIPLabel); err != nil {
		d.logger.Error("failed to remove wip label from issue", "issue", issueNum, "error", err)
	}
}
