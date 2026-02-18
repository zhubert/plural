package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/zhubert/plural/internal/app"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/issues"
	"github.com/zhubert/plural/internal/session"
	"github.com/zhubert/plural/internal/workflow"
)

// Daemon is the persistent orchestrator that manages the full lifecycle of work items.
type Daemon struct {
	config         *config.Config
	gitService     *git.GitService
	sessionService *session.SessionService
	sessionMgr     *app.SessionManager
	issueRegistry  *issues.ProviderRegistry
	state          *DaemonState
	lock           *DaemonLock
	workers         map[string]*SessionWorker
	workflowConfigs map[string]*workflow.Config // keyed by repo path
	mu              sync.Mutex
	logger          *slog.Logger

	// Options (carried over from Agent)
	once                  bool
	repoFilter            string
	maxConcurrent         int
	maxTurns              int
	maxDuration           int
	autoAddressPRComments bool
	autoBroadcastPR       bool
	autoMerge             bool
	mergeMethod           string
	pollInterval          time.Duration
	reviewPollInterval    time.Duration
	lastReviewPollAt      time.Time
}

// DaemonOption configures the daemon.
type DaemonOption func(*Daemon)

// WithDaemonOnce configures the daemon to run one tick and exit.
func WithDaemonOnce(once bool) DaemonOption {
	return func(d *Daemon) { d.once = once }
}

// WithDaemonRepoFilter limits polling to a specific repo.
func WithDaemonRepoFilter(repo string) DaemonOption {
	return func(d *Daemon) { d.repoFilter = repo }
}

// WithDaemonMaxConcurrent overrides the config's max concurrent setting.
func WithDaemonMaxConcurrent(max int) DaemonOption {
	return func(d *Daemon) { d.maxConcurrent = max }
}

// WithDaemonMaxTurns overrides the config's max autonomous turns setting.
func WithDaemonMaxTurns(max int) DaemonOption {
	return func(d *Daemon) { d.maxTurns = max }
}

// WithDaemonMaxDuration overrides the config's max autonomous duration (minutes) setting.
func WithDaemonMaxDuration(max int) DaemonOption {
	return func(d *Daemon) { d.maxDuration = max }
}

// WithDaemonAutoAddressPRComments enables auto-addressing PR review comments.
func WithDaemonAutoAddressPRComments(v bool) DaemonOption {
	return func(d *Daemon) { d.autoAddressPRComments = v }
}

// WithDaemonAutoBroadcastPR enables auto-creating PRs when broadcast group completes.
func WithDaemonAutoBroadcastPR(v bool) DaemonOption {
	return func(d *Daemon) { d.autoBroadcastPR = v }
}

// WithDaemonAutoMerge enables auto-merging PRs after review approval and CI pass.
func WithDaemonAutoMerge(v bool) DaemonOption {
	return func(d *Daemon) { d.autoMerge = v }
}

// WithDaemonMergeMethod sets the merge method (rebase, squash, or merge).
func WithDaemonMergeMethod(method string) DaemonOption {
	return func(d *Daemon) { d.mergeMethod = method }
}

// WithDaemonPollInterval sets the polling interval (mainly for testing).
func WithDaemonPollInterval(d time.Duration) DaemonOption {
	return func(dm *Daemon) { dm.pollInterval = d }
}

// WithDaemonReviewPollInterval sets the review polling interval (mainly for testing).
func WithDaemonReviewPollInterval(d time.Duration) DaemonOption {
	return func(dm *Daemon) { dm.reviewPollInterval = d }
}

// NewDaemon creates a new daemon.
func NewDaemon(cfg *config.Config, gitSvc *git.GitService, sessSvc *session.SessionService, registry *issues.ProviderRegistry, logger *slog.Logger, opts ...DaemonOption) *Daemon {
	d := &Daemon{
		config:             cfg,
		gitService:         gitSvc,
		sessionService:     sessSvc,
		sessionMgr:         app.NewSessionManager(cfg, gitSvc),
		issueRegistry:      registry,
		workers:            make(map[string]*SessionWorker),
		logger:             logger,
		autoMerge:          true, // Auto-merge is default for daemon
		pollInterval:       defaultPollInterval,
		reviewPollInterval: defaultReviewPollInterval,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Run starts the daemon's main loop. It blocks until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	d.logger.Info("daemon starting",
		"once", d.once,
		"repoFilter", d.repoFilter,
		"maxConcurrent", d.getMaxConcurrent(),
		"maxTurns", d.getMaxTurns(),
		"maxDuration", d.getMaxDuration(),
		"autoMerge", d.autoMerge,
	)

	// Acquire lock
	lock, err := AcquireLock(d.repoFilter)
	if err != nil {
		return fmt.Errorf("failed to acquire daemon lock: %w", err)
	}
	d.lock = lock
	defer d.releaseLock()

	// Load or create state
	state, err := LoadDaemonState(d.repoFilter)
	if err != nil {
		// If state is for a different repo, create fresh
		d.logger.Warn("failed to load daemon state, creating new", "error", err)
		state = NewDaemonState(d.repoFilter)
	}
	d.state = state

	// Load workflow configs for all repos
	d.loadWorkflowConfigs()

	// Recover from any interrupted state
	d.recoverFromState(ctx)

	// Immediate first tick
	d.tick(ctx)

	if d.once {
		d.waitForActiveWorkers(ctx)
		d.collectCompletedWorkers(ctx)
		d.saveState()
		d.logger.Info("daemon exiting (--once mode)")
		return nil
	}

	// Continuous polling loop
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.logger.Info("context cancelled, shutting down daemon")
			d.shutdown()
			return ctx.Err()
		case <-ticker.C:
			d.tick(ctx)
		}
	}
}

// tick performs one iteration of the daemon event loop.
func (d *Daemon) tick(ctx context.Context) {
	d.collectCompletedWorkers(ctx) // Detect finished Claude sessions
	d.processWorkItems(ctx)        // Check shelved items for events
	d.pollForNewIssues(ctx)        // Find new issues (if slots available)
	d.startQueuedItems(ctx)        // Start coding on queued items
	d.saveState()                  // Persist
}

// collectCompletedWorkers checks for finished Claude sessions and transitions work items.
func (d *Daemon) collectCompletedWorkers(ctx context.Context) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for workItemID, w := range d.workers {
		if !w.Done() {
			continue
		}

		item := d.state.GetWorkItem(workItemID)
		if item == nil {
			delete(d.workers, workItemID)
			continue
		}

		d.logger.Info("worker completed", "workItem", workItemID, "state", item.State)

		switch item.State {
		case WorkItemCoding:
			// Claude finished coding — create PR
			d.handleCodingComplete(ctx, item)

		case WorkItemAddressingFeedback:
			// Claude finished addressing feedback — push changes
			d.handleFeedbackComplete(ctx, item)
		}

		delete(d.workers, workItemID)
	}
}

// handleCodingComplete handles the transition after Claude finishes coding.
func (d *Daemon) handleCodingComplete(ctx context.Context, item *WorkItem) {
	log := d.logger.With("workItem", item.ID, "branch", item.Branch)

	sess := d.config.GetSession(item.SessionID)
	repoPath := ""
	if sess != nil {
		repoPath = sess.RepoPath
	}
	wfCfg := d.getWorkflowConfig(repoPath)

	// Run coding after-hooks
	if sess != nil {
		d.runWorkflowHooks(ctx, wfCfg.Workflow.Coding.After, item, sess)
	}

	// Check if the worker already created and merged a PR via MCP tools
	if sess != nil && sess.PRMerged {
		log.Info("PR already created and merged by worker, fast-pathing to completed")
		d.state.TransitionWorkItem(item.ID, WorkItemPRCreated)
		d.state.TransitionWorkItem(item.ID, WorkItemAwaitingReview)
		d.state.TransitionWorkItem(item.ID, WorkItemAwaitingCI)
		d.state.TransitionWorkItem(item.ID, WorkItemMerging)
		d.state.TransitionWorkItem(item.ID, WorkItemCompleted)

		// Run merge after-hooks
		d.runWorkflowHooks(ctx, wfCfg.Workflow.Merge.After, item, sess)
		return
	}

	// Check if the worker already created a PR via MCP tools (but not yet merged)
	if sess != nil && sess.PRCreated {
		log.Info("PR already created by worker, skipping createPR")
		if err := d.state.TransitionWorkItem(item.ID, WorkItemPRCreated); err != nil {
			log.Error("failed to transition to pr_created", "error", err)
			return
		}

		// Run PR after-hooks
		d.runWorkflowHooks(ctx, wfCfg.Workflow.PR.After, item, sess)

		// Transition to awaiting review so daemon's review/CI polling takes over
		if err := d.state.TransitionWorkItem(item.ID, WorkItemAwaitingReview); err != nil {
			log.Error("failed to transition to awaiting_review", "error", err)
		}
		return
	}

	prURL, err := d.createPR(ctx, item)
	if err != nil {
		log.Error("failed to create PR", "error", err)
		d.state.SetErrorMessage(item.ID, fmt.Sprintf("PR creation failed: %v", err))
		d.state.TransitionWorkItem(item.ID, WorkItemFailed)
		return
	}

	item.PRURL = prURL
	item.UpdatedAt = time.Now()

	if err := d.state.TransitionWorkItem(item.ID, WorkItemPRCreated); err != nil {
		log.Error("failed to transition to pr_created", "error", err)
		return
	}

	log.Info("PR created", "url", prURL)

	// Run PR after-hooks
	if sess != nil {
		d.runWorkflowHooks(ctx, wfCfg.Workflow.PR.After, item, sess)
	}

	// Immediately transition to awaiting review
	if err := d.state.TransitionWorkItem(item.ID, WorkItemAwaitingReview); err != nil {
		log.Error("failed to transition to awaiting_review", "error", err)
	}
}

// handleFeedbackComplete handles the transition after Claude finishes addressing feedback.
func (d *Daemon) handleFeedbackComplete(ctx context.Context, item *WorkItem) {
	log := d.logger.With("workItem", item.ID, "branch", item.Branch)

	// Push changes
	if err := d.pushChanges(ctx, item); err != nil {
		log.Error("failed to push changes", "error", err)
		d.state.SetErrorMessage(item.ID, fmt.Sprintf("push failed: %v", err))
		d.state.TransitionWorkItem(item.ID, WorkItemFailed)
		return
	}

	if err := d.state.TransitionWorkItem(item.ID, WorkItemPushing); err != nil {
		log.Error("failed to transition to pushing", "error", err)
		return
	}

	// Run review after-hooks
	sess := d.config.GetSession(item.SessionID)
	if sess != nil {
		wfCfg := d.getWorkflowConfig(sess.RepoPath)
		d.runWorkflowHooks(ctx, wfCfg.Workflow.Review.After, item, sess)
	}

	// Immediately transition back to awaiting review
	if err := d.state.TransitionWorkItem(item.ID, WorkItemAwaitingReview); err != nil {
		log.Error("failed to transition to awaiting_review", "error", err)
	}

	item.FeedbackRounds++
	item.UpdatedAt = time.Now()
	log.Info("pushed feedback changes", "round", item.FeedbackRounds)
}

// processWorkItems checks shelved items for external events.
func (d *Daemon) processWorkItems(ctx context.Context) {
	// Check AwaitingReview items for new comments or review decisions.
	// Only poll for reviews at the slower reviewPollInterval since human
	// reviewers may take hours or days to respond.
	if time.Since(d.lastReviewPollAt) >= d.reviewPollInterval {
		for _, item := range d.state.GetWorkItemsByState(WorkItemAwaitingReview) {
			d.processAwaitingReview(ctx, item)
		}
		d.lastReviewPollAt = time.Now()
	}

	// Check AwaitingCI items for CI status
	for _, item := range d.state.GetWorkItemsByState(WorkItemAwaitingCI) {
		d.processAwaitingCI(ctx, item)
	}
}

// waitForActiveWorkers waits for all active workers to complete (used in --once mode).
func (d *Daemon) waitForActiveWorkers(ctx context.Context) {
	d.mu.Lock()
	workers := make([]*SessionWorker, 0, len(d.workers))
	for _, w := range d.workers {
		workers = append(workers, w)
	}
	d.mu.Unlock()

	for _, w := range workers {
		w.Wait()
	}
}

// shutdown gracefully stops all workers and releases the lock.
func (d *Daemon) shutdown() {
	d.mu.Lock()
	workers := make([]*SessionWorker, 0, len(d.workers))
	for _, w := range d.workers {
		workers = append(workers, w)
	}
	d.mu.Unlock()

	d.logger.Info("shutting down workers", "count", len(workers))
	for _, w := range workers {
		w.Cancel()
	}

	done := make(chan struct{})
	go func() {
		for _, w := range workers {
			w.Wait()
		}
		close(done)
	}()

	select {
	case <-done:
		d.logger.Info("all workers shut down")
	case <-time.After(30 * time.Second):
		d.logger.Warn("shutdown timed out")
	}

	d.saveState()
	d.sessionMgr.Shutdown()
}

// releaseLock releases the daemon lock.
func (d *Daemon) releaseLock() {
	if d.lock != nil {
		if err := d.lock.Release(); err != nil {
			d.logger.Warn("failed to release lock", "error", err)
		}
	}
}

// saveState persists the daemon state to disk.
func (d *Daemon) saveState() {
	if d.state == nil {
		return
	}
	d.state.LastPollAt = time.Now()
	if err := d.state.Save(); err != nil {
		d.logger.Error("failed to save daemon state", "error", err)
	}
}

// getMaxConcurrent returns the effective max concurrent limit.
func (d *Daemon) getMaxConcurrent() int {
	if d.maxConcurrent > 0 {
		return d.maxConcurrent
	}
	return d.config.GetIssueMaxConcurrent()
}

// getMaxTurns returns the effective max autonomous turns limit.
func (d *Daemon) getMaxTurns() int {
	if d.maxTurns > 0 {
		return d.maxTurns
	}
	return d.config.GetAutoMaxTurns()
}

// getMaxDuration returns the effective max autonomous duration (minutes).
func (d *Daemon) getMaxDuration() int {
	if d.maxDuration > 0 {
		return d.maxDuration
	}
	return d.config.GetAutoMaxDurationMin()
}

// getAutoMerge returns whether auto-merge is enabled.
func (d *Daemon) getAutoMerge() bool {
	return d.autoMerge
}

// getMergeMethod returns the effective merge method.
func (d *Daemon) getMergeMethod() string {
	if d.mergeMethod != "" {
		return d.mergeMethod
	}
	return d.config.GetAutoMergeMethod()
}

// getAutoAddressPRComments returns whether auto-address PR comments is enabled.
func (d *Daemon) getAutoAddressPRComments() bool {
	return d.autoAddressPRComments || d.config.GetAutoAddressPRComments()
}

// getAutoBroadcastPR returns whether auto-broadcast PR is enabled.
func (d *Daemon) getAutoBroadcastPR() bool {
	return d.autoBroadcastPR || d.config.GetAutoBroadcastPR()
}

// loadWorkflowConfigs loads workflow configs for all registered repos.
func (d *Daemon) loadWorkflowConfigs() {
	d.workflowConfigs = make(map[string]*workflow.Config)

	for _, repoPath := range d.config.GetRepos() {
		cfg, err := workflow.LoadAndMerge(repoPath)
		if err != nil {
			d.logger.Warn("failed to load workflow config", "repo", repoPath, "error", err)
			continue
		}
		d.workflowConfigs[repoPath] = cfg
		d.logger.Debug("loaded workflow config", "repo", repoPath, "provider", cfg.Source.Provider)
	}
}

// getWorkflowConfig returns the workflow config for a repo, or defaults.
func (d *Daemon) getWorkflowConfig(repoPath string) *workflow.Config {
	if cfg, ok := d.workflowConfigs[repoPath]; ok {
		return cfg
	}
	return workflow.DefaultConfig()
}

// getEffectiveMaxTurns returns the effective max turns considering CLI > workflow > config > default.
func (d *Daemon) getEffectiveMaxTurns(repoPath string) int {
	if d.maxTurns > 0 {
		return d.maxTurns
	}
	wfCfg := d.getWorkflowConfig(repoPath)
	if wfCfg.Workflow.Coding.MaxTurns != nil {
		return *wfCfg.Workflow.Coding.MaxTurns
	}
	return d.config.GetAutoMaxTurns()
}

// getEffectiveMaxDuration returns the effective max duration in minutes considering CLI > workflow > config > default.
func (d *Daemon) getEffectiveMaxDuration(repoPath string) int {
	if d.maxDuration > 0 {
		return d.maxDuration
	}
	wfCfg := d.getWorkflowConfig(repoPath)
	if wfCfg.Workflow.Coding.MaxDuration != nil {
		return int(wfCfg.Workflow.Coding.MaxDuration.Duration.Minutes())
	}
	return d.config.GetAutoMaxDurationMin()
}

// getEffectiveMergeMethod returns the effective merge method considering CLI > workflow > config > default.
func (d *Daemon) getEffectiveMergeMethod(repoPath string) string {
	if d.mergeMethod != "" {
		return d.mergeMethod
	}
	wfCfg := d.getWorkflowConfig(repoPath)
	if wfCfg.Workflow.Merge.Method != "" {
		return wfCfg.Workflow.Merge.Method
	}
	return d.config.GetAutoMergeMethod()
}

// runWorkflowHooks runs the after-hooks for a given workflow step.
func (d *Daemon) runWorkflowHooks(ctx context.Context, hooks []workflow.HookConfig, item *WorkItem, sess *config.Session) {
	if len(hooks) == 0 {
		return
	}

	hookCtx := workflow.HookContext{
		RepoPath:  sess.RepoPath,
		Branch:    item.Branch,
		SessionID: item.SessionID,
		IssueID:   item.IssueRef.ID,
		IssueTitle: item.IssueRef.Title,
		IssueURL:  item.IssueRef.URL,
		PRURL:     item.PRURL,
		WorkTree:  sess.WorkTree,
		Provider:  item.IssueRef.Source,
	}

	workflow.RunHooks(ctx, hooks, hookCtx, d.logger)
}

// activeSlotCount returns the number of work items consuming concurrency slots.
func (d *Daemon) activeSlotCount() int {
	return d.state.ActiveSlotCount()
}
