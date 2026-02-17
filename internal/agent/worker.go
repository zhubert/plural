package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/mcp"
)

// SessionWorker manages a single autonomous session's lifecycle.
// It runs a goroutine with a select loop over all runner channels,
// replacing the TUI's Bubble Tea listener pattern.
type SessionWorker struct {
	agent      *Agent
	sessionID  string
	session    *config.Session
	runner     claude.RunnerInterface
	initialMsg string
	turns      int
	startTime  time.Time

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

// NewSessionWorker creates a new session worker.
func NewSessionWorker(agent *Agent, sess *config.Session, runner claude.RunnerInterface, initialMsg string) *SessionWorker {
	return &SessionWorker{
		agent:      agent,
		sessionID:  sess.ID,
		session:    sess,
		runner:     runner,
		initialMsg: initialMsg,
		startTime:  time.Now(),
		done:       make(chan struct{}),
	}
}

// Start begins the worker's goroutine.
func (w *SessionWorker) Start(ctx context.Context) {
	w.ctx, w.cancel = context.WithCancel(ctx)
	go w.run()
}

// Cancel requests the worker to stop.
func (w *SessionWorker) Cancel() {
	if w.cancel != nil {
		w.cancel()
	}
}

// Wait blocks until the worker has finished.
func (w *SessionWorker) Wait() {
	<-w.done
}

// Done returns true if the worker has finished.
func (w *SessionWorker) Done() bool {
	select {
	case <-w.done:
		return true
	default:
		return false
	}
}

// run is the main worker loop.
func (w *SessionWorker) run() {
	defer w.once.Do(func() { close(w.done) })

	log := w.agent.logger.With("sessionID", w.sessionID, "branch", w.session.Branch)
	log.Info("worker started")

	// Send initial message
	content := []claude.ContentBlock{{Type: claude.ContentTypeText, Text: w.initialMsg}}
	responseChan := w.runner.SendContent(w.ctx, content)

	for {
		if err := w.processOneResponse(responseChan); err != nil {
			log.Info("worker stopping", "reason", err.Error())
			return
		}

		// Check limits
		if w.checkLimits() {
			log.Warn("autonomous limit reached", "turns", w.turns)
			return
		}

		// Check for pending messages (e.g., child completion notifications)
		pendingMsg := w.agent.sessionMgr.StateManager().GetPendingMessage(w.sessionID)
		if pendingMsg != "" {
			log.Debug("sending pending message")
			content := []claude.ContentBlock{{Type: claude.ContentTypeText, Text: pendingMsg}}
			responseChan = w.runner.SendContent(w.ctx, content)
			continue
		}

		// For supervisor sessions, check if children are still active
		if w.session.IsSupervisor && w.hasActiveChildren() {
			log.Debug("supervisor has active children, waiting...")
			// Wait a bit then check again for pending messages
			select {
			case <-w.ctx.Done():
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}

		// Session completed with no pending work
		log.Info("session completed")
		w.handleCompletion()
		return
	}
}

// processOneResponse processes a single streaming response from Claude.
// It blocks until the response is done or an error occurs.
// Returns nil when the response completes normally, or an error to stop the worker.
func (w *SessionWorker) processOneResponse(responseChan <-chan claude.ResponseChunk) error {
	log := w.agent.logger.With("sessionID", w.sessionID)

	for {
		select {
		case <-w.ctx.Done():
			return fmt.Errorf("context cancelled")

		case chunk, ok := <-responseChan:
			if !ok {
				// Channel closed = done
				w.turns++
				w.handleDone()
				return nil
			}

			if chunk.Error != nil {
				log.Error("Claude error", "error", chunk.Error)
				return fmt.Errorf("claude error: %w", chunk.Error)
			}

			if chunk.Done {
				w.turns++
				w.handleDone()
				return nil
			}

			// Log streaming progress periodically
			w.handleStreaming(chunk)

		case req, ok := <-w.runner.PermissionRequestChan():
			if !ok {
				continue
			}
			// Should not happen in containerized mode, but auto-deny
			log.Warn("unexpected permission request in containerized mode", "tool", req.Tool)
			w.runner.SendPermissionResponse(mcp.PermissionResponse{
				ID:      req.ID,
				Allowed: false,
				Message: "Permission auto-denied in headless agent mode",
			})

		case req, ok := <-w.runner.QuestionRequestChan():
			if !ok {
				continue
			}
			w.autoRespondQuestion(req)

		case req, ok := <-w.runner.PlanApprovalRequestChan():
			if !ok {
				continue
			}
			w.autoApprovePlan(req)

		case req, ok := <-w.safeChanCreateChild():
			if !ok {
				continue
			}
			w.handleCreateChild(req)

		case req, ok := <-w.safeChanListChildren():
			if !ok {
				continue
			}
			w.handleListChildren(req)

		case req, ok := <-w.safeChanMergeChild():
			if !ok {
				continue
			}
			w.handleMergeChild(req)

		case req, ok := <-w.safeChanCreatePR():
			if !ok {
				continue
			}
			w.handleCreatePR(req)

		case req, ok := <-w.safeChanPushBranch():
			if !ok {
				continue
			}
			w.handlePushBranch(req)

		case req, ok := <-w.safeChanGetReviewComments():
			if !ok {
				continue
			}
			w.handleGetReviewComments(req)
		}
	}
}

// Safe channel accessors that return nil channels (which block forever in select) when not available.
func (w *SessionWorker) safeChanCreateChild() <-chan mcp.CreateChildRequest {
	ch := w.runner.CreateChildRequestChan()
	if ch == nil {
		return nil
	}
	return ch
}

func (w *SessionWorker) safeChanListChildren() <-chan mcp.ListChildrenRequest {
	ch := w.runner.ListChildrenRequestChan()
	if ch == nil {
		return nil
	}
	return ch
}

func (w *SessionWorker) safeChanMergeChild() <-chan mcp.MergeChildRequest {
	ch := w.runner.MergeChildRequestChan()
	if ch == nil {
		return nil
	}
	return ch
}

func (w *SessionWorker) safeChanCreatePR() <-chan mcp.CreatePRRequest {
	ch := w.runner.CreatePRRequestChan()
	if ch == nil {
		return nil
	}
	return ch
}

func (w *SessionWorker) safeChanPushBranch() <-chan mcp.PushBranchRequest {
	ch := w.runner.PushBranchRequestChan()
	if ch == nil {
		return nil
	}
	return ch
}

func (w *SessionWorker) safeChanGetReviewComments() <-chan mcp.GetReviewCommentsRequest {
	ch := w.runner.GetReviewCommentsRequestChan()
	if ch == nil {
		return nil
	}
	return ch
}

// handleStreaming logs streaming progress.
func (w *SessionWorker) handleStreaming(chunk claude.ResponseChunk) {
	// Just log text chunks at debug level for now
	if chunk.Type == claude.ChunkTypeText && chunk.Content != "" {
		// Log first 100 chars of content for debugging
		preview := chunk.Content
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		w.agent.logger.Debug("streaming", "sessionID", w.sessionID, "content", preview)
	}
}

// handleDone handles completion of a streaming response.
func (w *SessionWorker) handleDone() {
	log := w.agent.logger.With("sessionID", w.sessionID, "turn", w.turns)
	log.Debug("response completed")

	// Mark session as started if needed
	sess := w.agent.config.GetSession(w.sessionID)
	if sess != nil && w.runner.SessionStarted() && !sess.Started {
		w.agent.config.MarkSessionStarted(w.sessionID)
		if err := w.agent.config.Save(); err != nil {
			log.Error("failed to save config after marking started", "error", err)
		}
	}

	// Save messages
	w.agent.saveRunnerMessages(w.sessionID, w.runner)
}

// handleCompletion handles full session completion (all turns done, no pending work).
func (w *SessionWorker) handleCompletion() {
	log := w.agent.logger.With("sessionID", w.sessionID, "branch", w.session.Branch)
	sess := w.agent.config.GetSession(w.sessionID)
	if sess == nil {
		return
	}

	// For non-supervisor standalone sessions, auto-create PR
	if !sess.IsSupervisor && sess.SupervisorID == "" && !sess.PRCreated {
		log.Info("auto-creating PR")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		prURL, err := w.agent.autoCreatePR(ctx, w.sessionID)
		if err != nil {
			log.Error("failed to create PR", "error", err)
			return
		}
		log.Info("PR created", "url", prURL)

		// Start auto-merge if enabled
		if w.agent.config.GetRepoAutoMerge(sess.RepoPath) {
			w.runAutoMerge()
		}
	}

	// For supervisor sessions, the PR was created via MCP tools.
	// Check if auto-merge is needed.
	if sess.PRCreated && !sess.PRMerged && !sess.PRClosed &&
		w.agent.config.GetRepoAutoMerge(sess.RepoPath) {
		w.runAutoMerge()
	}

	// Notify supervisor if this is a child session
	if sess.SupervisorID != "" {
		w.notifySupervisor(sess.SupervisorID, true)
	}
}

// checkLimits returns true if the session has hit its turn or duration limit.
func (w *SessionWorker) checkLimits() bool {
	maxTurns := w.agent.getMaxTurns()
	maxDuration := time.Duration(w.agent.getMaxDuration()) * time.Minute

	if w.turns >= maxTurns {
		w.agent.logger.Warn("turn limit reached",
			"sessionID", w.sessionID,
			"turns", w.turns,
			"max", maxTurns,
		)
		return true
	}

	if time.Since(w.startTime) >= maxDuration {
		w.agent.logger.Warn("duration limit reached",
			"sessionID", w.sessionID,
			"elapsed", time.Since(w.startTime),
			"max", maxDuration,
		)
		return true
	}

	return false
}

// autoRespondQuestion automatically responds to questions by selecting the first option.
func (w *SessionWorker) autoRespondQuestion(req mcp.QuestionRequest) {
	log := w.agent.logger.With("sessionID", w.sessionID)
	log.Info("auto-responding to question")

	answers := make(map[string]string)
	for _, q := range req.Questions {
		if len(q.Options) > 0 {
			answers[q.Question] = q.Options[0].Label
		} else {
			answers[q.Question] = "Continue as you see fit"
		}
	}

	w.runner.SendQuestionResponse(mcp.QuestionResponse{
		ID:      req.ID,
		Answers: answers,
	})
}

// autoApprovePlan automatically approves plans.
func (w *SessionWorker) autoApprovePlan(req mcp.PlanApprovalRequest) {
	log := w.agent.logger.With("sessionID", w.sessionID)
	log.Info("auto-approving plan")

	w.runner.SendPlanApprovalResponse(mcp.PlanApprovalResponse{
		ID:       req.ID,
		Approved: true,
	})
}

// handleCreateChild handles a create_child_session MCP tool call.
func (w *SessionWorker) handleCreateChild(req mcp.CreateChildRequest) {
	log := w.agent.logger.With("sessionID", w.sessionID)

	childSess, err := w.agent.createChildSession(w.ctx, w.sessionID, req.Task)
	if err != nil {
		log.Error("failed to create child session", "error", err)
		w.runner.SendCreateChildResponse(mcp.CreateChildResponse{
			ID:    req.ID,
			Error: fmt.Sprintf("Failed to create child session: %v", err),
		})
		return
	}

	w.runner.SendCreateChildResponse(mcp.CreateChildResponse{
		ID:      req.ID,
		Success: true,
		ChildID: childSess.ID,
		Branch:  childSess.Branch,
	})
}

// handleListChildren handles a list_child_sessions MCP tool call.
func (w *SessionWorker) handleListChildren(req mcp.ListChildrenRequest) {
	children := w.agent.config.GetChildSessions(w.sessionID)
	var childInfos []mcp.ChildSessionInfo
	for _, child := range children {
		status := "idle"
		w.agent.mu.Lock()
		childWorker, exists := w.agent.workers[child.ID]
		w.agent.mu.Unlock()

		if exists && !childWorker.Done() {
			status = "running"
		} else if child.MergedToParent {
			status = "merged"
		} else if child.PRCreated {
			status = "pr_created"
		}

		childInfos = append(childInfos, mcp.ChildSessionInfo{
			ID:     child.ID,
			Branch: child.Branch,
			Status: status,
		})
	}

	w.runner.SendListChildrenResponse(mcp.ListChildrenResponse{
		ID:       req.ID,
		Children: childInfos,
	})
}

// handleMergeChild handles a merge_child_to_parent MCP tool call.
func (w *SessionWorker) handleMergeChild(req mcp.MergeChildRequest) {
	log := w.agent.logger.With("sessionID", w.sessionID)

	sess := w.agent.config.GetSession(w.sessionID)
	if sess == nil {
		w.runner.SendMergeChildResponse(mcp.MergeChildResponse{
			ID:    req.ID,
			Error: "Supervisor session not found",
		})
		return
	}

	childSess := w.agent.config.GetSession(req.ChildSessionID)
	if childSess == nil {
		w.runner.SendMergeChildResponse(mcp.MergeChildResponse{
			ID:    req.ID,
			Error: "Child session not found",
		})
		return
	}

	if childSess.SupervisorID != w.sessionID {
		w.runner.SendMergeChildResponse(mcp.MergeChildResponse{
			ID:    req.ID,
			Error: "Child session does not belong to this supervisor",
		})
		return
	}

	if childSess.MergedToParent {
		w.runner.SendMergeChildResponse(mcp.MergeChildResponse{
			ID:    req.ID,
			Error: "Child session already merged",
		})
		return
	}

	log.Info("merging child to parent", "childID", childSess.ID, "childBranch", childSess.Branch)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	resultCh := w.agent.gitService.MergeToParent(ctx, childSess.WorkTree, childSess.Branch, sess.WorkTree, sess.Branch, "")

	var lastErr error
	for result := range resultCh {
		if result.Error != nil {
			lastErr = result.Error
		}
	}

	if lastErr != nil {
		log.Error("merge child failed", "childID", childSess.ID, "error", lastErr)
		w.runner.SendMergeChildResponse(mcp.MergeChildResponse{
			ID:    req.ID,
			Error: lastErr.Error(),
		})
		return
	}

	// Mark child as merged
	w.agent.config.MarkSessionMergedToParent(childSess.ID)
	if err := w.agent.config.Save(); err != nil {
		log.Error("failed to save config after merge", "error", err)
	}

	w.runner.SendMergeChildResponse(mcp.MergeChildResponse{
		ID:      req.ID,
		Success: true,
		Message: fmt.Sprintf("Successfully merged %s into %s", childSess.Branch, sess.Branch),
	})
}

// handleCreatePR handles a create_pr MCP tool call.
func (w *SessionWorker) handleCreatePR(req mcp.CreatePRRequest) {
	log := w.agent.logger.With("sessionID", w.sessionID)
	sess := w.agent.config.GetSession(w.sessionID)
	if sess == nil {
		w.runner.SendCreatePRResponse(mcp.CreatePRResponse{
			ID:    req.ID,
			Error: "Session not found",
		})
		return
	}

	log.Info("creating PR via MCP tool", "branch", sess.Branch, "title", req.Title)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	resultCh := w.agent.gitService.CreatePR(ctx, sess.RepoPath, sess.WorkTree, sess.Branch, sess.BaseBranch, req.Title, sess.GetIssueRef())

	var lastErr error
	var prURL string
	for result := range resultCh {
		if result.Error != nil {
			lastErr = result.Error
		}
		if trimmed := trimURL(result.Output); trimmed != "" {
			prURL = trimmed
		}
	}

	if lastErr != nil {
		w.runner.SendCreatePRResponse(mcp.CreatePRResponse{
			ID:    req.ID,
			Error: fmt.Sprintf("Failed to create PR: %v", lastErr),
		})
		return
	}

	// Mark session as PR created
	w.agent.config.MarkSessionPRCreated(w.sessionID)
	if err := w.agent.config.Save(); err != nil {
		log.Error("failed to save config after PR creation", "error", err)
	}

	w.runner.SendCreatePRResponse(mcp.CreatePRResponse{
		ID:      req.ID,
		Success: true,
		PRURL:   prURL,
	})

	// Start auto-merge if enabled
	if w.agent.config.GetRepoAutoMerge(sess.RepoPath) {
		go w.runAutoMerge()
	}
}

// handlePushBranch handles a push_branch MCP tool call.
func (w *SessionWorker) handlePushBranch(req mcp.PushBranchRequest) {
	log := w.agent.logger.With("sessionID", w.sessionID)
	sess := w.agent.config.GetSession(w.sessionID)
	if sess == nil {
		w.runner.SendPushBranchResponse(mcp.PushBranchResponse{
			ID:    req.ID,
			Error: "Session not found",
		})
		return
	}

	log.Info("pushing branch via MCP tool", "branch", sess.Branch)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	resultCh := w.agent.gitService.PushUpdates(ctx, sess.RepoPath, sess.WorkTree, sess.Branch, req.CommitMessage)

	var lastErr error
	for result := range resultCh {
		if result.Error != nil {
			lastErr = result.Error
		}
	}

	if lastErr != nil {
		w.runner.SendPushBranchResponse(mcp.PushBranchResponse{
			ID:    req.ID,
			Error: fmt.Sprintf("Failed to push branch: %v", lastErr),
		})
		return
	}

	w.runner.SendPushBranchResponse(mcp.PushBranchResponse{
		ID:      req.ID,
		Success: true,
	})
}

// handleGetReviewComments handles a get_review_comments MCP tool call.
func (w *SessionWorker) handleGetReviewComments(req mcp.GetReviewCommentsRequest) {
	log := w.agent.logger.With("sessionID", w.sessionID)
	sess := w.agent.config.GetSession(w.sessionID)
	if sess == nil {
		w.runner.SendGetReviewCommentsResponse(mcp.GetReviewCommentsResponse{
			ID:    req.ID,
			Error: "Session not found",
		})
		return
	}

	log.Info("fetching review comments via MCP tool", "branch", sess.Branch)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	comments, err := w.agent.gitService.FetchPRReviewComments(ctx, sess.RepoPath, sess.Branch)
	if err != nil {
		w.runner.SendGetReviewCommentsResponse(mcp.GetReviewCommentsResponse{
			ID:    req.ID,
			Error: fmt.Sprintf("Failed to fetch review comments: %v", err),
		})
		return
	}

	mcpComments := make([]mcp.ReviewComment, len(comments))
	for i, c := range comments {
		mcpComments[i] = mcp.ReviewComment{
			Author: c.Author,
			Body:   c.Body,
			Path:   c.Path,
			Line:   c.Line,
			URL:    c.URL,
		}
	}

	w.runner.SendGetReviewCommentsResponse(mcp.GetReviewCommentsResponse{
		ID:       req.ID,
		Success:  true,
		Comments: mcpComments,
	})
}

// hasActiveChildren checks if any child sessions are still running.
func (w *SessionWorker) hasActiveChildren() bool {
	children := w.agent.config.GetChildSessions(w.sessionID)
	for _, child := range children {
		w.agent.mu.Lock()
		childWorker, exists := w.agent.workers[child.ID]
		w.agent.mu.Unlock()

		if exists && !childWorker.Done() {
			return true
		}
	}
	return false
}

// notifySupervisor sends a status update to the supervisor session.
func (w *SessionWorker) notifySupervisor(supervisorID string, testsPassed bool) {
	childSess := w.agent.config.GetSession(w.sessionID)
	if childSess == nil {
		return
	}

	status := "completed successfully"
	if !testsPassed {
		status = "completed (tests failed)"
	}

	allChildren := w.agent.config.GetChildSessions(supervisorID)
	allDone := true
	completedCount := 0
	for _, child := range allChildren {
		w.agent.mu.Lock()
		childWorker, exists := w.agent.workers[child.ID]
		w.agent.mu.Unlock()

		if exists && !childWorker.Done() {
			allDone = false
		} else {
			completedCount++
		}
	}

	var prompt string
	sessionName := childSess.Branch
	if allDone {
		prompt = fmt.Sprintf("Child session '%s' %s.\n\nAll %d child sessions have completed. You should now review the results, merge children to parent with `merge_child_to_parent`, and create a PR with `push_branch` and `create_pr`.",
			sessionName, status, len(allChildren))
	} else {
		prompt = fmt.Sprintf("Child session '%s' %s. (%d/%d children completed)\n\nWait for all children to complete before merging or creating PRs.",
			sessionName, status, completedCount, len(allChildren))
	}

	// Queue the message for the supervisor
	state := w.agent.sessionMgr.StateManager().GetOrCreate(supervisorID)
	state.SetPendingMsg(prompt)
}

// runAutoMerge runs the auto-merge state machine. See auto_merge.go.
func (w *SessionWorker) runAutoMerge() {
	runAutoMerge(w.agent, w.sessionID)
}

// formatOutput trims strings for logging.
func formatOutput(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
