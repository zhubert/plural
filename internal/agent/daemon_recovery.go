package agent

import (
	"context"
	"time"

	"github.com/zhubert/plural/internal/git"
)

// recoverFromState reconciles daemon state with reality after a restart.
// It handles each interrupted state case to resume or fix up work items.
func (d *Daemon) recoverFromState(ctx context.Context) {
	if d.state == nil || len(d.state.WorkItems) == 0 {
		return
	}

	log := d.logger.With("component", "recovery")
	log.Info("recovering from previous state", "workItems", len(d.state.WorkItems))

	for _, item := range d.state.WorkItems {
		if item.State.IsTerminal() {
			continue
		}

		log := log.With("workItem", item.ID, "state", item.State, "branch", item.Branch)

		switch item.State {
		case WorkItemQueued:
			// Nothing to do — will be picked up by startQueuedItems
			log.Info("work item queued, will start on next tick")

		case WorkItemCoding:
			d.recoverCoding(ctx, item, log)

		case WorkItemAddressingFeedback:
			d.recoverAddressingFeedback(item, log)

		case WorkItemPRCreated:
			d.recoverPRCreated(ctx, item, log)

		case WorkItemPushing:
			d.recoverPushing(item, log)

		case WorkItemAwaitingReview:
			// Nothing to do — will be polled by processWorkItems
			log.Info("work item awaiting review, resuming polling")

		case WorkItemAwaitingCI:
			// Nothing to do — will be polled by processWorkItems
			log.Info("work item awaiting CI, resuming polling")

		case WorkItemMerging:
			d.recoverMerging(ctx, item, log)
		}
	}
}

// recoverCoding handles recovery from the Coding state.
// If a PR exists, transition to AwaitingReview; otherwise, re-queue.
func (d *Daemon) recoverCoding(ctx context.Context, item *WorkItem, log interface{ Info(string, ...any) }) {
	if item.Branch == "" {
		log.Info("no branch, re-queuing")
		d.state.mu.Lock()
		item.State = WorkItemQueued
		item.UpdatedAt = time.Now()
		d.state.mu.Unlock()
		return
	}

	sess := d.config.GetSession(item.SessionID)
	if sess == nil {
		log.Info("session not found, re-queuing")
		d.state.mu.Lock()
		item.State = WorkItemQueued
		item.UpdatedAt = time.Now()
		d.state.mu.Unlock()
		return
	}

	// Check if PR was already created
	pollCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	prState, err := d.gitService.GetPRState(pollCtx, sess.RepoPath, item.Branch)
	if err == nil && (prState == git.PRStateOpen || prState == git.PRStateMerged) {
		log.Info("PR exists, transitioning to awaiting_review")
		d.state.mu.Lock()
		if prState == git.PRStateMerged {
			item.State = WorkItemCompleted
			now := time.Now()
			item.CompletedAt = &now
		} else {
			item.State = WorkItemAwaitingReview
		}
		item.UpdatedAt = time.Now()
		d.state.mu.Unlock()
		return
	}

	// No PR — re-queue to restart coding
	log.Info("no PR found, re-queuing")
	d.state.mu.Lock()
	item.State = WorkItemQueued
	item.UpdatedAt = time.Now()
	d.state.mu.Unlock()
}

// recoverAddressingFeedback transitions back to AwaitingReview.
// The next processWorkItems tick will detect any new comments and resume.
func (d *Daemon) recoverAddressingFeedback(item *WorkItem, log interface{ Info(string, ...any) }) {
	log.Info("was addressing feedback, transitioning to awaiting_review")
	d.state.mu.Lock()
	item.State = WorkItemAwaitingReview
	item.UpdatedAt = time.Now()
	d.state.mu.Unlock()
}

// recoverPRCreated checks if the PR exists and transitions accordingly.
func (d *Daemon) recoverPRCreated(ctx context.Context, item *WorkItem, log interface{ Info(string, ...any) }) {
	sess := d.config.GetSession(item.SessionID)
	if sess == nil {
		log.Info("session not found, marking failed")
		d.state.mu.Lock()
		item.State = WorkItemFailed
		item.ErrorMessage = "session lost during recovery"
		now := time.Now()
		item.CompletedAt = &now
		item.UpdatedAt = now
		d.state.mu.Unlock()
		return
	}

	pollCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	prState, err := d.gitService.GetPRState(pollCtx, sess.RepoPath, item.Branch)
	if err == nil && (prState == git.PRStateOpen || prState == git.PRStateMerged) {
		log.Info("PR exists, transitioning to awaiting_review")
		d.state.mu.Lock()
		if prState == git.PRStateMerged {
			item.State = WorkItemCompleted
			now := time.Now()
			item.CompletedAt = &now
		} else {
			item.State = WorkItemAwaitingReview
		}
		item.UpdatedAt = time.Now()
		d.state.mu.Unlock()
		return
	}

	// PR creation was interrupted — retry
	log.Info("PR not found, will retry creation")
	d.state.mu.Lock()
	item.State = WorkItemCoding // Will trigger PR creation in collectCompletedWorkers
	item.UpdatedAt = time.Now()
	d.state.mu.Unlock()
}

// recoverPushing transitions back to AwaitingReview.
// The push may have succeeded; the next review poll will catch up.
func (d *Daemon) recoverPushing(item *WorkItem, log interface{ Info(string, ...any) }) {
	log.Info("was pushing, transitioning to awaiting_review")
	d.state.mu.Lock()
	item.State = WorkItemAwaitingReview
	item.UpdatedAt = time.Now()
	d.state.mu.Unlock()
}

// recoverMerging checks if the PR was actually merged and transitions accordingly.
func (d *Daemon) recoverMerging(ctx context.Context, item *WorkItem, log interface{ Info(string, ...any) }) {
	sess := d.config.GetSession(item.SessionID)
	if sess == nil {
		log.Info("session not found, marking completed (optimistic)")
		d.state.mu.Lock()
		item.State = WorkItemCompleted
		now := time.Now()
		item.CompletedAt = &now
		item.UpdatedAt = now
		d.state.mu.Unlock()
		return
	}

	pollCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	prState, err := d.gitService.GetPRState(pollCtx, sess.RepoPath, item.Branch)
	if err != nil {
		log.Info("failed to check PR state, transitioning to awaiting_ci")
		d.state.mu.Lock()
		item.State = WorkItemAwaitingCI
		item.UpdatedAt = time.Now()
		d.state.mu.Unlock()
		return
	}

	if prState == git.PRStateMerged {
		log.Info("PR merged, marking completed")
		d.state.mu.Lock()
		item.State = WorkItemCompleted
		now := time.Now()
		item.CompletedAt = &now
		item.UpdatedAt = now
		d.state.mu.Unlock()
		return
	}

	// Merge didn't complete — transition back to AwaitingCI
	log.Info("PR not merged, transitioning to awaiting_ci")
	d.state.mu.Lock()
	item.State = WorkItemAwaitingCI
	item.UpdatedAt = time.Now()
	d.state.mu.Unlock()
}
