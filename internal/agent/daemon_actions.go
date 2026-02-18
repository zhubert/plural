package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/issues"
	"github.com/zhubert/plural/internal/session"
	"github.com/zhubert/plural/internal/workflow"
)

// startCoding creates a session and starts a Claude worker for a queued work item.
func (d *Daemon) startCoding(ctx context.Context, item *WorkItem) {
	log := d.logger.With("workItem", item.ID, "issue", item.IssueRef.ID)

	// Find the matching repo path
	repoPath := d.findRepoPath(ctx)
	if repoPath == "" {
		log.Error("no matching repo found")
		d.state.SetErrorMessage(item.ID, "no matching repo found")
		d.state.TransitionWorkItem(item.ID, WorkItemFailed)
		return
	}

	branchPrefix := d.config.GetDefaultBranchPrefix()

	// Generate branch name
	var branchName string
	if d.issueRegistry != nil {
		issue := issueFromWorkItem(item)
		provider := d.issueRegistry.GetProvider(issue.Source)
		if provider != nil {
			branchName = provider.GenerateBranchName(issue)
		}
	}
	if branchName == "" {
		branchName = fmt.Sprintf("issue-%s", item.IssueRef.ID)
	}

	fullBranchName := branchPrefix + branchName

	// Check if branch already exists
	if d.sessionService.BranchExists(ctx, repoPath, fullBranchName) {
		log.Debug("branch already exists, skipping", "branch", fullBranchName)
		d.state.SetErrorMessage(item.ID, "branch already exists")
		d.state.TransitionWorkItem(item.ID, WorkItemFailed)
		return
	}

	// Create new session
	sess, err := d.sessionService.Create(ctx, repoPath, branchName, branchPrefix, session.BasePointOrigin)
	if err != nil {
		log.Error("failed to create session", "error", err)
		d.state.SetErrorMessage(item.ID, fmt.Sprintf("session creation failed: %v", err))
		d.state.TransitionWorkItem(item.ID, WorkItemFailed)
		return
	}

	// Configure session from workflow config
	wfCfg := d.getWorkflowConfig(repoPath)
	sess.Autonomous = true
	sess.Containerized = wfCfg.Workflow.Coding.Containerized == nil || *wfCfg.Workflow.Coding.Containerized
	sess.IsSupervisor = wfCfg.Workflow.Coding.Supervisor == nil || *wfCfg.Workflow.Coding.Supervisor
	sess.IssueRef = &config.IssueRef{
		Source: item.IssueRef.Source,
		ID:     item.IssueRef.ID,
		Title:  item.IssueRef.Title,
		URL:    item.IssueRef.URL,
	}

	d.config.AddSession(*sess)
	if err := d.config.Save(); err != nil {
		log.Error("failed to save config", "error", err)
	}

	// Update work item with session info
	item.SessionID = sess.ID
	item.Branch = sess.Branch
	item.UpdatedAt = time.Now()

	// Transition to Coding
	if err := d.state.TransitionWorkItem(item.ID, WorkItemCoding); err != nil {
		log.Error("failed to transition to coding", "error", err)
		return
	}

	// Build initial message using provider-aware formatting
	initialMsg := formatInitialMessage(item.IssueRef)

	// Resolve coding system prompt from workflow config
	codingPrompt, err := workflow.ResolveSystemPrompt(wfCfg.Workflow.Coding.SystemPrompt, repoPath)
	if err != nil {
		log.Warn("failed to resolve coding system prompt", "error", err)
	}

	// Start worker with custom system prompt
	d.startWorkerWithPrompt(ctx, item, sess, initialMsg, codingPrompt)

	log.Info("started coding", "sessionID", sess.ID, "branch", sess.Branch)
}

// addressFeedback resumes the Claude session to address review comments.
func (d *Daemon) addressFeedback(ctx context.Context, item *WorkItem) {
	log := d.logger.With("workItem", item.ID, "branch", item.Branch)

	sess := d.config.GetSession(item.SessionID)
	if sess == nil {
		log.Error("session not found")
		return
	}

	// Fetch review comments
	pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	comments, err := d.gitService.FetchPRReviewComments(pollCtx, sess.RepoPath, item.Branch)
	if err != nil {
		log.Warn("failed to fetch review comments", "error", err)
		return
	}

	if len(comments) == 0 {
		log.Debug("no comments to address")
		return
	}

	// Mark comments as addressed
	item.CommentsAddressed += len(comments)
	item.UpdatedAt = time.Now()

	// Transition to AddressingFeedback
	if err := d.state.TransitionWorkItem(item.ID, WorkItemAddressingFeedback); err != nil {
		log.Error("failed to transition to addressing_feedback", "error", err)
		return
	}

	// Format comments as a prompt
	prompt := formatPRCommentsPrompt(comments)

	// Resolve review system prompt from workflow config
	wfCfg := d.getWorkflowConfig(sess.RepoPath)
	reviewPrompt, err := workflow.ResolveSystemPrompt(wfCfg.Workflow.Review.SystemPrompt, sess.RepoPath)
	if err != nil {
		log.Warn("failed to resolve review system prompt", "error", err)
	}

	// Resume the existing session with the review system prompt
	d.startWorkerWithPrompt(ctx, item, sess, prompt, reviewPrompt)

	log.Info("addressing review feedback", "commentCount", len(comments), "round", item.FeedbackRounds+1)
}

// createPR creates a pull request for a work item's session.
func (d *Daemon) createPR(ctx context.Context, item *WorkItem) (string, error) {
	sess := d.config.GetSession(item.SessionID)
	if sess == nil {
		return "", fmt.Errorf("session not found")
	}

	log := d.logger.With("workItem", item.ID, "branch", item.Branch)
	log.Info("creating PR")

	prCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	resultCh := d.gitService.CreatePR(prCtx, sess.RepoPath, sess.WorkTree, sess.Branch, sess.BaseBranch, "", sess.GetIssueRef(), item.SessionID)

	var lastErr error
	var prURL string
	for result := range resultCh {
		if result.Error != nil {
			lastErr = result.Error
		}
		if result.Output != "" {
			trimmed := trimURL(result.Output)
			if trimmed != "" {
				prURL = trimmed
			}
		}
	}

	if lastErr != nil {
		return "", lastErr
	}

	// Mark session as PR created
	d.config.MarkSessionPRCreated(item.SessionID)
	if err := d.config.Save(); err != nil {
		log.Error("failed to save config after PR creation", "error", err)
	}

	return prURL, nil
}

// pushChanges pushes changes for a work item's session.
func (d *Daemon) pushChanges(ctx context.Context, item *WorkItem) error {
	sess := d.config.GetSession(item.SessionID)
	if sess == nil {
		return fmt.Errorf("session not found")
	}

	pushCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	resultCh := d.gitService.PushUpdates(pushCtx, sess.RepoPath, sess.WorkTree, sess.Branch, "Address review feedback")

	var lastErr error
	for result := range resultCh {
		if result.Error != nil {
			lastErr = result.Error
		}
	}

	return lastErr
}

// mergePR merges the PR for a work item.
func (d *Daemon) mergePR(ctx context.Context, item *WorkItem) error {
	sess := d.config.GetSession(item.SessionID)
	if sess == nil {
		return fmt.Errorf("session not found")
	}

	mergeCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	err := d.gitService.MergePR(mergeCtx, sess.RepoPath, item.Branch, false, d.getEffectiveMergeMethod(sess.RepoPath))
	if err != nil {
		return err
	}

	// Mark session as merged
	d.config.MarkSessionPRMerged(item.SessionID)
	if err := d.config.Save(); err != nil {
		d.logger.Error("failed to save config after merge", "error", err)
	}

	// Auto-cleanup if enabled
	if d.config.GetAutoCleanupMerged() {
		d.cleanupSession(ctx, item.SessionID)
	}

	return nil
}

// startWorker creates and starts a session worker for a work item.
func (d *Daemon) startWorker(ctx context.Context, item *WorkItem, sess *config.Session, initialMsg string) {
	d.startWorkerWithPrompt(ctx, item, sess, initialMsg, "")
}

// startWorkerWithPrompt creates and starts a session worker with an optional custom system prompt.
func (d *Daemon) startWorkerWithPrompt(ctx context.Context, item *WorkItem, sess *config.Session, initialMsg, customPrompt string) {
	runner := d.sessionMgr.GetOrCreateRunner(sess)
	if customPrompt != "" {
		runner.SetCustomSystemPrompt(customPrompt)
	}
	worker := NewSessionWorker(d.toAgent(), sess, runner, initialMsg)

	d.mu.Lock()
	d.workers[item.ID] = worker
	d.mu.Unlock()

	worker.Start(ctx)
}

// toAgent returns an Agent-compatible wrapper for the daemon.
// This allows reusing SessionWorker which expects an *Agent.
func (d *Daemon) toAgent() *Agent {
	return &Agent{
		config:                d.config,
		gitService:            d.gitService,
		sessionService:        d.sessionService,
		sessionMgr:            d.sessionMgr,
		issueRegistry:         d.issueRegistry,
		workers:               d.workers,
		logger:                d.logger,
		once:                  d.once,
		repoFilter:            d.repoFilter,
		maxConcurrent:         d.maxConcurrent,
		maxTurns:              d.maxTurns,
		maxDuration:           d.maxDuration,
		autoAddressPRComments: d.autoAddressPRComments,
		autoBroadcastPR:       d.autoBroadcastPR,
		autoMerge:             d.autoMerge,
		mergeMethod:           d.mergeMethod,
		pollInterval:          d.pollInterval,
	}
}

// cleanupSession cleans up a session's worktree and removes it from config.
func (d *Daemon) cleanupSession(ctx context.Context, sessionID string) {
	sess := d.config.GetSession(sessionID)
	if sess == nil {
		return
	}

	log := d.logger.With("sessionID", sessionID, "branch", sess.Branch)

	d.sessionMgr.DeleteSession(sessionID)

	if err := d.sessionService.Delete(ctx, sess); err != nil {
		log.Warn("failed to delete worktree", "error", err)
	}

	d.config.RemoveSession(sessionID)
	d.config.ClearOrphanedParentIDs([]string{sessionID})
	config.DeleteSessionMessages(sessionID)

	if err := d.config.Save(); err != nil {
		log.Error("failed to save config after cleanup", "error", err)
	}

	log.Info("cleaned up session")
}

// findRepoPath returns the first repo path that matches the daemon's filter.
func (d *Daemon) findRepoPath(ctx context.Context) string {
	for _, repoPath := range d.config.GetRepos() {
		if d.matchesRepoFilter(ctx, repoPath) {
			return repoPath
		}
	}
	return ""
}

// issueFromWorkItem converts a WorkItem's issue ref to an issues.Issue.
func issueFromWorkItem(item *WorkItem) issues.Issue {
	return issues.Issue{
		ID:     item.IssueRef.ID,
		Title:  item.IssueRef.Title,
		URL:    item.IssueRef.URL,
		Source: issues.Source(item.IssueRef.Source),
	}
}

// saveRunnerMessages saves messages for a session's runner.
func (d *Daemon) saveRunnerMessages(sessionID string, runner claude.RunnerInterface) {
	if err := d.sessionMgr.SaveRunnerMessages(sessionID, runner); err != nil {
		d.logger.Error("failed to save session messages", "sessionID", sessionID, "error", err)
	}
}
