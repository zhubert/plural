package agent

import (
	"context"
	"time"

	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
)

const (
	maxAutoMergePollAttempts = 120 // ~2 hours at 60s intervals
	autoMergePollInterval   = 60 * time.Second
)

// runAutoMerge runs the auto-merge state machine for a session.
// It polls for review approval and CI status, then merges the PR.
// This is a blocking function intended to run in a goroutine.
func runAutoMerge(a *Agent, sessionID string) {
	log := a.logger.With("sessionID", sessionID, "component", "auto-merge")
	sess := a.config.GetSession(sessionID)
	if sess == nil {
		return
	}

	log.Info("starting auto-merge polling", "branch", sess.Branch)

	for attempt := 1; attempt <= maxAutoMergePollAttempts; attempt++ {
		// Wait before check to give CI/reviews time
		time.Sleep(autoMergePollInterval)

		// Refresh session in case it was updated
		sess = a.config.GetSession(sessionID)
		if sess == nil {
			log.Warn("session disappeared during auto-merge polling")
			return
		}

		// Step 1: Check for unaddressed review comments
		action := checkAndAddressComments(a, sessionID, sess, attempt)
		switch action {
		case mergeActionContinue:
			continue // Poll again
		case mergeActionStop:
			return
		case mergeActionProceed:
			// Fall through to review/CI checks
		}

		// Step 2: Check review approval
		action = checkReviewApproval(a, sessionID, sess, attempt)
		switch action {
		case mergeActionContinue:
			continue
		case mergeActionStop:
			return
		case mergeActionProceed:
			// Fall through to CI check
		}

		// Step 3: Check CI status
		action = checkCIAndMerge(a, sessionID, sess, attempt)
		switch action {
		case mergeActionContinue:
			continue
		case mergeActionStop:
			return
		case mergeActionProceed:
			// Merge succeeded, done
			return
		}
	}

	log.Warn("auto-merge polling exhausted all attempts", "branch", sess.Branch)
}

type mergeAction int

const (
	mergeActionContinue mergeAction = iota // Keep polling
	mergeActionStop                        // Stop polling (failure or done)
	mergeActionProceed                     // Move to next step
)

// checkAndAddressComments checks for unaddressed PR review comments.
func checkAndAddressComments(a *Agent, sessionID string, sess *config.Session, attempt int) mergeAction {
	log := a.logger.With("sessionID", sessionID)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	results, err := a.gitService.GetBatchPRStatesWithComments(ctx, sess.RepoPath, []string{sess.Branch})
	if err != nil {
		log.Warn("failed to check PR comment count", "error", err)
		return mergeActionProceed // Don't block on comment check failure
	}

	result, ok := results[sess.Branch]
	if !ok {
		return mergeActionProceed
	}

	if result.CommentCount > sess.PRCommentsAddressedCount {
		log.Info("unaddressed review comments detected",
			"addressed", sess.PRCommentsAddressedCount,
			"current", result.CommentCount,
		)

		// Mark these comments as addressed
		a.config.UpdateSessionPRCommentsAddressedCount(sessionID, result.CommentCount)

		// Fetch and send comments to Claude
		if addressComments(a, sessionID, sess) {
			// Comments were sent to Claude — the worker's main loop will handle
			// the response and re-trigger auto-merge via handleCompletion
			return mergeActionStop
		}
	}

	return mergeActionProceed
}

// addressComments fetches PR comments and queues them for Claude.
// Returns true if comments were queued successfully.
func addressComments(a *Agent, sessionID string, sess *config.Session) bool {
	log := a.logger.With("sessionID", sessionID)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	comments, err := a.gitService.FetchPRReviewComments(ctx, sess.RepoPath, sess.Branch)
	if err != nil {
		log.Warn("failed to fetch PR review comments", "error", err)
		return false
	}

	if len(comments) == 0 {
		return false
	}

	prompt := formatPRCommentsPrompt(comments)
	state := a.sessionMgr.StateManager().GetOrCreate(sessionID)
	state.SetPendingMsg(prompt)

	log.Info("queued review comments for Claude", "commentCount", len(comments))
	return true
}

// checkReviewApproval checks the PR review status.
func checkReviewApproval(a *Agent, sessionID string, sess *config.Session, attempt int) mergeAction {
	log := a.logger.With("sessionID", sessionID)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	reviewDecision, err := a.gitService.CheckPRReviewDecision(ctx, sess.RepoPath, sess.Branch)
	if err != nil {
		log.Warn("failed to check PR review decision", "error", err)
	}

	switch reviewDecision {
	case git.ReviewChangesRequested:
		log.Info("changes requested, waiting for re-review", "branch", sess.Branch)
		return mergeActionContinue

	case git.ReviewNone:
		if attempt >= maxAutoMergePollAttempts {
			log.Warn("timed out waiting for review", "branch", sess.Branch)
			return mergeActionStop
		}
		if attempt == 1 {
			log.Info("waiting for review", "branch", sess.Branch)
		}
		return mergeActionContinue

	case git.ReviewApproved:
		return mergeActionProceed
	}

	return mergeActionProceed
}

// checkCIAndMerge checks CI status and merges if passing.
func checkCIAndMerge(a *Agent, sessionID string, sess *config.Session, attempt int) mergeAction {
	log := a.logger.With("sessionID", sessionID)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ciStatus, err := a.gitService.CheckPRChecks(ctx, sess.RepoPath, sess.Branch)
	if err != nil {
		log.Warn("failed to check CI status", "error", err)
		ciStatus = git.CIStatusPending
	}

	switch ciStatus {
	case git.CIStatusPassing, git.CIStatusNone:
		log.Info("review approved, CI passed, merging PR", "branch", sess.Branch, "ciStatus", ciStatus)
		return doMerge(a, sessionID, sess)

	case git.CIStatusFailing:
		log.Warn("CI checks failed, skipping auto-merge", "branch", sess.Branch)
		return mergeActionStop

	case git.CIStatusPending:
		if attempt >= maxAutoMergePollAttempts {
			log.Warn("timed out waiting for CI", "branch", sess.Branch)
			return mergeActionStop
		}
		log.Debug("CI checks still pending", "branch", sess.Branch, "attempt", attempt)
		return mergeActionContinue
	}

	return mergeActionContinue
}

// doMerge performs the actual PR merge.
func doMerge(a *Agent, sessionID string, sess *config.Session) mergeAction {
	log := a.logger.With("sessionID", sessionID)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Don't delete branch - it will be deleted during session cleanup
	err := a.gitService.MergePR(ctx, sess.RepoPath, sess.Branch, false, a.getMergeMethod())
	if err != nil {
		log.Error("auto-merge failed", "error", err)
		return mergeActionStop
	}

	log.Info("auto-merge successful", "branch", sess.Branch)

	// Mark as merged
	a.config.MarkSessionPRMerged(sessionID)
	if err := a.config.Save(); err != nil {
		log.Error("failed to save config after merge", "error", err)
	}

	// Auto-cleanup if enabled
	if a.config.GetAutoCleanupMerged() {
		if err := a.cleanupSession(context.Background(), sessionID); err != nil {
			log.Error("auto-cleanup failed", "error", err)
		}
	}

	return mergeActionProceed
}
