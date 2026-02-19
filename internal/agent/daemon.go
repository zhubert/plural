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
	engines         map[string]*workflow.Engine // keyed by repo path
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
	d.processWorkItems(ctx)        // Process active items via engine
	d.pollForNewIssues(ctx)        // Find new issues (if slots available)
	d.startQueuedItems(ctx)        // Start coding on queued items
	d.saveState()                  // Persist
}

// collectCompletedWorkers checks for finished Claude sessions and advances work items.
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

		d.logger.Info("worker completed", "workItem", workItemID, "step", item.CurrentStep, "phase", item.Phase)

		switch item.Phase {
		case "async_pending":
			// Main async action completed (e.g., coding)
			d.handleAsyncComplete(ctx, item)

		case "addressing_feedback":
			// Feedback addressing completed — push changes
			d.handleFeedbackComplete(ctx, item)
		}

		delete(d.workers, workItemID)
	}
}

// handleAsyncComplete handles the completion of an async action.
func (d *Daemon) handleAsyncComplete(ctx context.Context, item *WorkItem) {
	log := d.logger.With("workItem", item.ID, "step", item.CurrentStep)

	sess := d.config.GetSession(item.SessionID)
	repoPath := ""
	if sess != nil {
		repoPath = sess.RepoPath
	}

	engine := d.getEngine(repoPath)
	if engine == nil {
		log.Error("no engine for repo", "repo", repoPath)
		return
	}

	// Get state definition for after-hooks
	state := engine.GetState(item.CurrentStep)

	// Check if the worker already created and merged a PR via MCP tools
	if sess != nil && sess.PRMerged {
		log.Info("PR already created and merged by worker, fast-pathing to completed")
		// Run current step's after-hooks
		if state != nil {
			d.runHooks(ctx, state.After, item, sess)
		}
		d.state.AdvanceWorkItem(item.ID, "done", "idle")
		d.state.MarkWorkItemTerminal(item.ID, true)

		// Run merge hooks if exists
		mergeState := engine.GetState("merge")
		if mergeState != nil {
			d.runHooks(ctx, mergeState.After, item, sess)
		}
		return
	}

	// Check if the worker already created a PR via MCP tools (but not merged)
	if sess != nil && sess.PRCreated && item.CurrentStep == "coding" {
		log.Info("PR already created by worker, skipping open_pr step")
		// Run coding after-hooks
		if state != nil {
			d.runHooks(ctx, state.After, item, sess)
		}
		// Skip open_pr, go directly to await_review
		prState := engine.GetState("open_pr")
		if prState != nil {
			d.runHooks(ctx, prState.After, item, sess)
		}
		d.state.AdvanceWorkItem(item.ID, "await_review", "idle")
		return
	}

	// Normal async completion — advance via engine
	view := d.workItemView(item)
	result, err := engine.AdvanceAfterAsync(view, true)
	if err != nil {
		log.Error("failed to advance after async", "error", err)
		d.state.SetErrorMessage(item.ID, err.Error())
		d.state.MarkWorkItemTerminal(item.ID, false)
		return
	}

	// Run after-hooks
	if state != nil && sess != nil {
		d.runHooks(ctx, state.After, item, sess)
	}

	if result.Terminal {
		d.state.AdvanceWorkItem(item.ID, result.NewStep, result.NewPhase)
		d.state.MarkWorkItemTerminal(item.ID, result.TerminalOK)
		return
	}

	// For task states with sync next actions, execute them inline
	d.state.AdvanceWorkItem(item.ID, result.NewStep, result.NewPhase)

	// If the next step is a sync task (like open_pr), execute it now
	d.executeSyncChain(ctx, item, engine)
}

// executeSyncChain executes synchronous task states in sequence until
// hitting an async task, a wait state, or a terminal state.
func (d *Daemon) executeSyncChain(ctx context.Context, item *WorkItem, engine *workflow.Engine) {
	for {
		view := d.workItemView(item)
		result, err := engine.ProcessStep(ctx, view)
		if err != nil {
			d.logger.Error("sync chain error", "workItem", item.ID, "step", item.CurrentStep, "error", err)
			d.state.SetErrorMessage(item.ID, err.Error())
			d.state.MarkWorkItemTerminal(item.ID, false)
			return
		}

		if result.Terminal {
			d.state.AdvanceWorkItem(item.ID, result.NewStep, result.NewPhase)
			d.state.MarkWorkItemTerminal(item.ID, result.TerminalOK)
			return
		}

		// Run after-hooks
		if len(result.Hooks) > 0 {
			sess := d.config.GetSession(item.SessionID)
			if sess != nil {
				d.runHooks(ctx, result.Hooks, item, sess)
			}
		}

		// Merge data and apply known fields to the work item
		if result.Data != nil {
			for k, v := range result.Data {
				item.StepData[k] = v
			}
			if prURL, ok := result.Data["pr_url"].(string); ok && prURL != "" {
				item.PRURL = prURL
				item.UpdatedAt = time.Now()
			}
		}

		d.state.AdvanceWorkItem(item.ID, result.NewStep, result.NewPhase)

		// Stop if we hit an async pending state or a wait state
		if result.NewPhase == "async_pending" {
			return
		}
		nextState := engine.GetState(result.NewStep)
		if nextState != nil && nextState.Type == workflow.StateTypeWait {
			return
		}
	}
}

// handleFeedbackComplete handles the transition after Claude finishes addressing feedback.
func (d *Daemon) handleFeedbackComplete(ctx context.Context, item *WorkItem) {
	log := d.logger.With("workItem", item.ID, "branch", item.Branch)

	// Push changes
	if err := d.pushChanges(ctx, item); err != nil {
		log.Error("failed to push changes", "error", err)
		d.state.SetErrorMessage(item.ID, fmt.Sprintf("push failed: %v", err))
		d.state.MarkWorkItemTerminal(item.ID, false)
		return
	}

	// Run review after-hooks
	sess := d.config.GetSession(item.SessionID)
	if sess != nil {
		engine := d.getEngine(sess.RepoPath)
		if engine != nil {
			state := engine.GetState(item.CurrentStep)
			if state != nil {
				d.runHooks(ctx, state.After, item, sess)
			}
		}
	}

	// Back to idle phase for the wait state to continue polling
	d.state.AdvanceWorkItem(item.ID, item.CurrentStep, "idle")

	item.FeedbackRounds++
	item.UpdatedAt = time.Now()
	log.Info("pushed feedback changes", "round", item.FeedbackRounds)
}

// processWorkItems checks active items via the engine.
func (d *Daemon) processWorkItems(ctx context.Context) {
	// Check wait-state items (review, CI) at the review poll interval
	if time.Since(d.lastReviewPollAt) >= d.reviewPollInterval {
		d.processWaitItems(ctx)
		d.lastReviewPollAt = time.Now()
	}

	// Check CI items on every tick (they don't need the slower interval)
	d.processCIItems(ctx)
}

// processWaitItems processes items in wait states for review events.
func (d *Daemon) processWaitItems(ctx context.Context) {
	for _, item := range d.state.GetActiveWorkItems() {
		if item.IsTerminal() || item.Phase == "async_pending" || item.Phase == "addressing_feedback" {
			continue
		}

		sess := d.config.GetSession(item.SessionID)
		if sess == nil {
			continue
		}

		engine := d.getEngine(sess.RepoPath)
		if engine == nil {
			continue
		}

		state := engine.GetState(item.CurrentStep)
		if state == nil || state.Type != workflow.StateTypeWait {
			continue
		}

		// Only process pr.reviewed events here
		if state.Event != "pr.reviewed" {
			continue
		}

		view := d.workItemView(item)
		result, err := engine.ProcessStep(ctx, view)
		if err != nil {
			d.logger.Error("wait step error", "workItem", item.ID, "error", err)
			continue
		}

		if result.NewStep != item.CurrentStep || result.NewPhase != item.Phase {
			if len(result.Hooks) > 0 {
				d.runHooks(ctx, result.Hooks, item, sess)
			}
			d.state.AdvanceWorkItem(item.ID, result.NewStep, result.NewPhase)
			if result.Terminal {
				d.state.MarkWorkItemTerminal(item.ID, result.TerminalOK)
			} else {
				// Continue sync chain if next is a sync task
				d.executeSyncChain(ctx, item, engine)
			}
		}
	}
}

// processCIItems processes items waiting for CI events.
func (d *Daemon) processCIItems(ctx context.Context) {
	for _, item := range d.state.GetActiveWorkItems() {
		if item.IsTerminal() || item.Phase == "async_pending" || item.Phase == "addressing_feedback" {
			continue
		}

		sess := d.config.GetSession(item.SessionID)
		if sess == nil {
			continue
		}

		engine := d.getEngine(sess.RepoPath)
		if engine == nil {
			continue
		}

		state := engine.GetState(item.CurrentStep)
		if state == nil || state.Type != workflow.StateTypeWait || state.Event != "ci.complete" {
			continue
		}

		view := d.workItemView(item)
		result, err := engine.ProcessStep(ctx, view)
		if err != nil {
			d.logger.Error("ci step error", "workItem", item.ID, "error", err)
			continue
		}

		if result.NewStep != item.CurrentStep || result.NewPhase != item.Phase {
			if len(result.Hooks) > 0 {
				d.runHooks(ctx, result.Hooks, item, sess)
			}
			d.state.AdvanceWorkItem(item.ID, result.NewStep, result.NewPhase)
			if result.Terminal {
				d.state.MarkWorkItemTerminal(item.ID, result.TerminalOK)
			} else {
				d.executeSyncChain(ctx, item, engine)
			}
		}
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

// loadWorkflowConfigs loads workflow configs and creates engines for all registered repos.
func (d *Daemon) loadWorkflowConfigs() {
	d.workflowConfigs = make(map[string]*workflow.Config)
	d.engines = make(map[string]*workflow.Engine)

	for _, repoPath := range d.config.GetRepos() {
		cfg, err := workflow.LoadAndMerge(repoPath)
		if err != nil {
			d.logger.Warn("failed to load workflow config", "repo", repoPath, "error", err)
			continue
		}
		d.workflowConfigs[repoPath] = cfg

		// Create engine with action registry and event checker
		registry := d.buildActionRegistry()
		checker := NewDaemonEventChecker(d)
		engine := workflow.NewEngine(cfg, registry, checker, d.logger)
		d.engines[repoPath] = engine

		d.logger.Debug("loaded workflow config", "repo", repoPath, "provider", cfg.Source.Provider)
	}
}

// buildActionRegistry creates the action registry with all daemon actions.
func (d *Daemon) buildActionRegistry() *workflow.ActionRegistry {
	registry := workflow.NewActionRegistry()
	registry.Register("ai.code", &CodingAction{daemon: d})
	registry.Register("github.create_pr", &CreatePRAction{daemon: d})
	registry.Register("github.push", &PushAction{daemon: d})
	registry.Register("github.merge", &MergeAction{daemon: d})
	return registry
}

// getWorkflowConfig returns the workflow config for a repo, or defaults.
func (d *Daemon) getWorkflowConfig(repoPath string) *workflow.Config {
	if cfg, ok := d.workflowConfigs[repoPath]; ok {
		return cfg
	}
	return workflow.DefaultConfig()
}

// getEngine returns the workflow engine for a repo, or creates one with defaults.
func (d *Daemon) getEngine(repoPath string) *workflow.Engine {
	if engine, ok := d.engines[repoPath]; ok {
		return engine
	}
	// Create a default engine on the fly
	cfg := workflow.DefaultConfig()
	registry := d.buildActionRegistry()
	checker := NewDaemonEventChecker(d)
	return workflow.NewEngine(cfg, registry, checker, d.logger)
}

// getEffectiveMaxTurns returns the effective max turns considering CLI > workflow > config > default.
func (d *Daemon) getEffectiveMaxTurns(repoPath string) int {
	if d.maxTurns > 0 {
		return d.maxTurns
	}
	wfCfg := d.getWorkflowConfig(repoPath)
	codingState := wfCfg.States["coding"]
	if codingState != nil {
		p := workflow.NewParamHelper(codingState.Params)
		if v := p.Int("max_turns", 0); v > 0 {
			return v
		}
	}
	return d.config.GetAutoMaxTurns()
}

// getEffectiveMaxDuration returns the effective max duration in minutes.
func (d *Daemon) getEffectiveMaxDuration(repoPath string) int {
	if d.maxDuration > 0 {
		return d.maxDuration
	}
	wfCfg := d.getWorkflowConfig(repoPath)
	codingState := wfCfg.States["coding"]
	if codingState != nil {
		p := workflow.NewParamHelper(codingState.Params)
		dur := p.Duration("max_duration", 0)
		if dur > 0 {
			return int(dur.Minutes())
		}
	}
	return d.config.GetAutoMaxDurationMin()
}

// getEffectiveMergeMethod returns the effective merge method.
func (d *Daemon) getEffectiveMergeMethod(repoPath string) string {
	if d.mergeMethod != "" {
		return d.mergeMethod
	}
	wfCfg := d.getWorkflowConfig(repoPath)
	mergeState := wfCfg.States["merge"]
	if mergeState != nil {
		p := workflow.NewParamHelper(mergeState.Params)
		if m := p.String("method", ""); m != "" {
			return m
		}
	}
	return d.config.GetAutoMergeMethod()
}

// runHooks runs the after-hooks for a given workflow step.
func (d *Daemon) runHooks(ctx context.Context, hooks []workflow.HookConfig, item *WorkItem, sess *config.Session) {
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

// workItemView creates a read-only view of a work item for the engine.
func (d *Daemon) workItemView(item *WorkItem) *workflow.WorkItemView {
	// Use the session's actual repo path rather than d.repoFilter,
	// which may be empty or a pattern (e.g., "owner/repo") in multi-repo daemons.
	repoPath := d.repoFilter
	if sess := d.config.GetSession(item.SessionID); sess != nil {
		repoPath = sess.RepoPath
	}

	return &workflow.WorkItemView{
		ID:                item.ID,
		SessionID:         item.SessionID,
		RepoPath:          repoPath,
		Branch:            item.Branch,
		PRURL:             item.PRURL,
		CurrentStep:       item.CurrentStep,
		Phase:             item.Phase,
		StepData:          item.StepData,
		FeedbackRounds:    item.FeedbackRounds,
		CommentsAddressed: item.CommentsAddressed,
	}
}

// activeSlotCount returns the number of work items consuming concurrency slots.
func (d *Daemon) activeSlotCount() int {
	return d.state.ActiveSlotCount()
}
