package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/zhubert/plural/internal/app"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/issues"
	"github.com/zhubert/plural/internal/session"
)

const (
	defaultPollInterval = 30 * time.Second
	autonomousFilterLabel = "queued"
	autonomousWIPLabel    = "wip"
)

// Agent is the headless autonomous agent that polls for issues
// and manages worker goroutines to process them.
type Agent struct {
	config         *config.Config
	gitService     *git.GitService
	sessionService *session.SessionService
	sessionMgr     *app.SessionManager
	issueRegistry  *issues.ProviderRegistry
	workers        map[string]*SessionWorker
	mu             sync.Mutex
	logger         *slog.Logger

	// Options
	once                  bool          // Process available issues and exit
	repoFilter            string        // Limit to specific repo
	maxConcurrent         int           // Override config's IssueMaxConcurrent (0 = use config)
	maxTurns              int           // Override config's AutoMaxTurns (0 = use config)
	maxDuration           int           // Override config's AutoMaxDurationMin (0 = use config)
	autoAddressPRComments bool          // Auto-address PR review comments
	autoBroadcastPR       bool          // Auto-create PRs when broadcast group completes
	pollInterval          time.Duration
}

// Option configures the agent.
type Option func(*Agent)

// WithOnce configures the agent to process available issues and exit.
func WithOnce(once bool) Option {
	return func(a *Agent) { a.once = once }
}

// WithRepoFilter limits polling to a specific repo.
func WithRepoFilter(repo string) Option {
	return func(a *Agent) { a.repoFilter = repo }
}

// WithMaxConcurrent overrides the config's max concurrent setting.
func WithMaxConcurrent(max int) Option {
	return func(a *Agent) { a.maxConcurrent = max }
}

// WithMaxTurns overrides the config's max autonomous turns setting.
func WithMaxTurns(max int) Option {
	return func(a *Agent) { a.maxTurns = max }
}

// WithMaxDuration overrides the config's max autonomous duration (minutes) setting.
func WithMaxDuration(max int) Option {
	return func(a *Agent) { a.maxDuration = max }
}

// WithAutoAddressPRComments enables auto-addressing PR review comments.
func WithAutoAddressPRComments(v bool) Option {
	return func(a *Agent) { a.autoAddressPRComments = v }
}

// WithAutoBroadcastPR enables auto-creating PRs when broadcast group completes.
func WithAutoBroadcastPR(v bool) Option {
	return func(a *Agent) { a.autoBroadcastPR = v }
}

// WithPollInterval sets the polling interval (mainly for testing).
func WithPollInterval(d time.Duration) Option {
	return func(a *Agent) { a.pollInterval = d }
}

// New creates a new headless agent.
func New(cfg *config.Config, gitSvc *git.GitService, sessSvc *session.SessionService, registry *issues.ProviderRegistry, logger *slog.Logger, opts ...Option) *Agent {
	a := &Agent{
		config:         cfg,
		gitService:     gitSvc,
		sessionService: sessSvc,
		sessionMgr:     app.NewSessionManager(cfg, gitSvc),
		issueRegistry:  registry,
		workers:        make(map[string]*SessionWorker),
		logger:         logger,
		pollInterval:   defaultPollInterval,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Run starts the agent's main loop. It blocks until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) error {
	a.logger.Info("agent starting",
		"once", a.once,
		"repoFilter", a.repoFilter,
		"maxConcurrent", a.getMaxConcurrent(),
		"maxTurns", a.getMaxTurns(),
		"maxDuration", a.getMaxDuration(),
	)

	// Do an immediate poll
	if err := a.pollAndProcess(ctx); err != nil {
		a.logger.Error("initial poll failed", "error", err)
	}

	if a.once {
		// Wait for all workers to finish
		a.waitForWorkers(ctx)
		a.logger.Info("all workers completed, exiting (--once mode)")
		return nil
	}

	// Continuous polling loop
	ticker := time.NewTicker(a.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("context cancelled, shutting down")
			a.Shutdown()
			return ctx.Err()
		case <-ticker.C:
			if err := a.pollAndProcess(ctx); err != nil {
				a.logger.Error("poll failed", "error", err)
			}
			// Clean up completed workers
			a.cleanupCompletedWorkers()
		}
	}
}

// Shutdown gracefully stops all workers and cleans up resources.
func (a *Agent) Shutdown() {
	a.mu.Lock()
	workers := make([]*SessionWorker, 0, len(a.workers))
	for _, w := range a.workers {
		workers = append(workers, w)
	}
	a.mu.Unlock()

	a.logger.Info("shutting down workers", "count", len(workers))
	for _, w := range workers {
		w.Cancel()
	}

	// Wait for workers to finish (with timeout)
	done := make(chan struct{})
	go func() {
		for _, w := range workers {
			w.Wait()
		}
		close(done)
	}()

	select {
	case <-done:
		a.logger.Info("all workers shut down")
	case <-time.After(30 * time.Second):
		a.logger.Warn("shutdown timed out, some workers may still be running")
	}

	a.sessionMgr.Shutdown()
}

// pollAndProcess polls for new issues and creates workers for them.
func (a *Agent) pollAndProcess(ctx context.Context) error {
	allNewIssues := a.pollForIssues(ctx)
	if len(allNewIssues) == 0 {
		a.logger.Debug("no new issues found")
		return nil
	}

	for _, ri := range allNewIssues {
		for _, issue := range ri.Issues {
			if err := a.createSessionForIssue(ctx, ri.RepoPath, issue); err != nil {
				a.logger.Error("failed to create session for issue",
					"repo", ri.RepoPath,
					"issue", issue.ID,
					"error", err,
				)
			}
		}
	}
	return nil
}

// createSessionForIssue creates a new autonomous session and starts a worker for an issue.
func (a *Agent) createSessionForIssue(ctx context.Context, repoPath string, issue issues.Issue) error {
	log := a.logger.With("repo", repoPath, "issue", issue.ID)
	branchPrefix := a.config.GetDefaultBranchPrefix()

	// Generate branch name
	var branchName string
	if a.issueRegistry != nil {
		provider := a.issueRegistry.GetProvider(issue.Source)
		if provider != nil {
			branchName = provider.GenerateBranchName(issue)
		}
	}
	if branchName == "" {
		branchName = fmt.Sprintf("issue-%s", issue.ID)
	}

	fullBranchName := branchPrefix + branchName

	// Check if branch already exists
	if a.sessionService.BranchExists(ctx, repoPath, fullBranchName) {
		log.Debug("skipping issue - branch already exists", "branch", fullBranchName)
		return nil
	}

	// Create new session
	sess, err := a.sessionService.Create(ctx, repoPath, branchName, branchPrefix, session.BasePointOrigin)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// Configure as autonomous, containerized supervisor
	sess.Autonomous = true
	sess.Containerized = true
	sess.IsSupervisor = true
	sess.IssueRef = &config.IssueRef{
		Source: string(issue.Source),
		ID:     issue.ID,
		Title:  issue.Title,
		URL:    issue.URL,
	}

	a.config.AddSession(*sess)
	if err := a.config.Save(); err != nil {
		log.Error("failed to save config", "error", err)
	}

	log.Info("created session", "sessionID", sess.ID, "branch", sess.Branch)

	// Swap labels in the background
	go a.swapIssueLabels(repoPath, issue)

	// Build initial message
	initialMsg := fmt.Sprintf("GitHub Issue #%s: %s\n\n%s",
		issue.ID, issue.Title, issue.Body)

	// Start worker
	a.startWorker(ctx, sess, initialMsg)

	return nil
}

// startWorker creates and starts a new session worker.
func (a *Agent) startWorker(ctx context.Context, sess *config.Session, initialMsg string) {
	// Get or create runner via session manager
	runner := a.sessionMgr.GetOrCreateRunner(sess)

	worker := NewSessionWorker(a, sess, runner, initialMsg)

	a.mu.Lock()
	a.workers[sess.ID] = worker
	a.mu.Unlock()

	worker.Start(ctx)
}

// waitForWorkers waits for all active workers to complete.
func (a *Agent) waitForWorkers(ctx context.Context) {
	a.mu.Lock()
	workers := make([]*SessionWorker, 0, len(a.workers))
	for _, w := range a.workers {
		workers = append(workers, w)
	}
	a.mu.Unlock()

	for _, w := range workers {
		w.Wait()
	}
}

// cleanupCompletedWorkers removes completed workers from the map.
func (a *Agent) cleanupCompletedWorkers() {
	a.mu.Lock()
	defer a.mu.Unlock()

	for id, w := range a.workers {
		if w.Done() {
			delete(a.workers, id)
		}
	}
}

// activeWorkerCount returns the number of active (non-done) workers.
func (a *Agent) activeWorkerCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()

	count := 0
	for _, w := range a.workers {
		if !w.Done() {
			count++
		}
	}
	return count
}

// getMaxConcurrent returns the effective max concurrent limit.
func (a *Agent) getMaxConcurrent() int {
	if a.maxConcurrent > 0 {
		return a.maxConcurrent
	}
	return a.config.GetIssueMaxConcurrent()
}

// getMaxTurns returns the effective max autonomous turns limit.
func (a *Agent) getMaxTurns() int {
	if a.maxTurns > 0 {
		return a.maxTurns
	}
	return a.config.GetAutoMaxTurns()
}

// getMaxDuration returns the effective max autonomous duration (minutes).
func (a *Agent) getMaxDuration() int {
	if a.maxDuration > 0 {
		return a.maxDuration
	}
	return a.config.GetAutoMaxDurationMin()
}

// getAutoAddressPRComments returns whether auto-address PR comments is enabled.
func (a *Agent) getAutoAddressPRComments() bool {
	return a.autoAddressPRComments || a.config.GetAutoAddressPRComments()
}

// getAutoBroadcastPR returns whether auto-broadcast PR is enabled.
func (a *Agent) getAutoBroadcastPR() bool {
	return a.autoBroadcastPR || a.config.GetAutoBroadcastPR()
}

// swapIssueLabels removes "queued" and adds "wip" label, and comments on the issue.
func (a *Agent) swapIssueLabels(repoPath string, issue issues.Issue) {
	issueNum, err := strconv.Atoi(issue.ID)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := a.gitService.RemoveIssueLabel(ctx, repoPath, issueNum, autonomousFilterLabel); err != nil {
		a.logger.Error("failed to remove issue label", "issue", issueNum, "label", autonomousFilterLabel, "error", err)
	}
	if err := a.gitService.AddIssueLabel(ctx, repoPath, issueNum, autonomousWIPLabel); err != nil {
		a.logger.Error("failed to add wip label", "issue", issueNum, "error", err)
	}
	comment := "This issue has been picked up by [Plural](https://github.com/zhubert/plural) and is being worked on autonomously."
	if err := a.gitService.CommentOnIssue(ctx, repoPath, issueNum, comment); err != nil {
		a.logger.Error("failed to comment on issue", "issue", issueNum, "error", err)
	}
}

// removeIssueWIPLabel removes the "wip" label from a session's issue.
func (a *Agent) removeIssueWIPLabel(sess *config.Session) {
	if sess.IssueRef == nil {
		return
	}
	issueNum, err := strconv.Atoi(sess.IssueRef.ID)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := a.gitService.RemoveIssueLabel(ctx, sess.RepoPath, issueNum, autonomousWIPLabel); err != nil {
		a.logger.Error("failed to remove wip label from issue", "issue", issueNum, "error", err)
	}
}

// cleanupSession cleans up a session's worktree and removes it from config.
func (a *Agent) cleanupSession(ctx context.Context, sessionID string) error {
	sess := a.config.GetSession(sessionID)
	if sess == nil {
		return nil
	}

	log := a.logger.With("sessionID", sessionID, "branch", sess.Branch)

	// Stop runner
	a.sessionMgr.DeleteSession(sessionID)

	// Delete worktree
	if err := a.sessionService.Delete(ctx, sess); err != nil {
		log.Warn("failed to delete worktree", "error", err)
	}

	// Remove session from config
	a.config.RemoveSession(sessionID)
	a.config.ClearOrphanedParentIDs([]string{sessionID})
	config.DeleteSessionMessages(sessionID)

	if err := a.config.Save(); err != nil {
		log.Error("failed to save config after cleanup", "error", err)
	}

	log.Info("cleaned up session")
	return nil
}

// autoCreatePR creates a PR for a session.
func (a *Agent) autoCreatePR(ctx context.Context, sessionID string) (string, error) {
	sess := a.config.GetSession(sessionID)
	if sess == nil {
		return "", fmt.Errorf("session not found")
	}

	log := a.logger.With("sessionID", sessionID, "branch", sess.Branch)
	log.Info("creating PR")

	resultCh := a.gitService.CreatePR(ctx, sess.RepoPath, sess.WorkTree, sess.Branch, sess.BaseBranch, "", sess.GetIssueRef())

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
		log.Debug("PR creation progress", "output", result.Output, "done", result.Done)
	}

	if lastErr != nil {
		return "", lastErr
	}

	// Mark session as PR created
	a.config.MarkSessionPRCreated(sessionID)
	if err := a.config.Save(); err != nil {
		log.Error("failed to save config after PR creation", "error", err)
	}

	log.Info("PR created", "url", prURL)
	return prURL, nil
}

// createChildSession creates an autonomous child session from a supervisor session.
func (a *Agent) createChildSession(ctx context.Context, supervisorID, taskDescription string) (*config.Session, error) {
	supervisorSess := a.config.GetSession(supervisorID)
	if supervisorSess == nil {
		return nil, fmt.Errorf("supervisor session not found")
	}

	branchPrefix := a.config.GetDefaultBranchPrefix()
	branchName := fmt.Sprintf("child-%s", time.Now().Format("20060102-150405"))

	childSess, err := a.sessionService.CreateFromBranch(ctx, supervisorSess.RepoPath, supervisorSess.Branch, branchName, branchPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to create child session: %w", err)
	}

	childSess.Autonomous = true
	childSess.Containerized = supervisorSess.Containerized
	childSess.SupervisorID = supervisorID
	childSess.ParentID = supervisorID

	a.config.AddSession(*childSess)
	a.config.AddChildSession(supervisorID, childSess.ID)
	if err := a.config.Save(); err != nil {
		a.logger.Error("failed to save config after creating child", "error", err)
	}

	initialMsg := fmt.Sprintf("You are a child session working on a specific task assigned by a supervisor session.\n\nTask: %s\n\nPlease complete this task. When you are done, make sure all changes are committed.", taskDescription)

	a.startWorker(ctx, childSess, initialMsg)

	a.logger.Info("created child session", "childID", childSess.ID, "branch", childSess.Branch, "supervisorID", supervisorID)
	return childSess, nil
}

// saveRunnerMessages saves messages for a session's runner.
func (a *Agent) saveRunnerMessages(sessionID string, runner claude.RunnerInterface) {
	if err := a.sessionMgr.SaveRunnerMessages(sessionID, runner); err != nil {
		a.logger.Error("failed to save session messages", "sessionID", sessionID, "error", err)
	}
}
