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
		if item.IsTerminal() {
			continue
		}

		log := log.With("workItem", item.ID, "step", item.CurrentStep, "phase", item.Phase, "branch", item.Branch)

		switch item.Phase {
		case "async_pending":
			// Worker was running but daemon restarted — no worker exists
			d.recoverAsyncPending(ctx, item, log)

		case "addressing_feedback":
			// Was addressing feedback — reset to idle for re-polling
			log.Info("was addressing feedback, resetting to idle")
			d.state.AdvanceWorkItem(item.ID, item.CurrentStep, "idle")

		case "pushing":
			// Was pushing — reset to idle for re-polling
			log.Info("was pushing, resetting to idle")
			d.state.AdvanceWorkItem(item.ID, item.CurrentStep, "idle")

		default:
			// "idle" or empty — normal wait/queue state
			if item.State == WorkItemQueued {
				log.Info("work item queued, will start on next tick")
			} else {
				log.Info("work item in wait state, resuming polling")
			}
		}
	}
}

// recoverAsyncPending handles recovery when a worker was active but daemon restarted.
func (d *Daemon) recoverAsyncPending(ctx context.Context, item *WorkItem, log interface{ Info(string, ...any) }) {
	if item.Branch == "" {
		log.Info("no branch, re-queuing")
		d.state.mu.Lock()
		item.State = WorkItemQueued
		item.CurrentStep = ""
		item.Phase = "idle"
		item.UpdatedAt = time.Now()
		d.state.mu.Unlock()
		return
	}

	sess := d.config.GetSession(item.SessionID)
	if sess == nil {
		log.Info("session not found, re-queuing")
		d.state.mu.Lock()
		item.State = WorkItemQueued
		item.CurrentStep = ""
		item.Phase = "idle"
		item.UpdatedAt = time.Now()
		d.state.mu.Unlock()
		return
	}

	// Check if PR was already created
	pollCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	prState, err := d.gitService.GetPRState(pollCtx, sess.RepoPath, item.Branch)
	if err == nil && (prState == git.PRStateOpen || prState == git.PRStateMerged) {
		if prState == git.PRStateMerged {
			log.Info("PR merged, marking completed")
			d.state.mu.Lock()
			item.CurrentStep = "done"
			item.Phase = "idle"
			item.State = WorkItemCompleted
			now := time.Now()
			item.CompletedAt = &now
			item.UpdatedAt = now
			d.state.mu.Unlock()
		} else {
			log.Info("PR exists, advancing to await_review")
			d.state.mu.Lock()
			item.CurrentStep = "await_review"
			item.Phase = "idle"
			item.UpdatedAt = time.Now()
			d.state.mu.Unlock()
		}
		return
	}

	// No PR — re-queue to restart coding
	log.Info("no PR found, re-queuing")
	d.state.mu.Lock()
	item.State = WorkItemQueued
	item.CurrentStep = ""
	item.Phase = "idle"
	item.UpdatedAt = time.Now()
	d.state.mu.Unlock()
}
