package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/exec"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/issues"
	"github.com/zhubert/plural/internal/session"
)

// testDaemon creates a daemon suitable for testing with mock services.
func testDaemon(cfg *config.Config) *Daemon {
	mockExec := exec.NewMockExecutor(nil)
	gitSvc := git.NewGitServiceWithExecutor(mockExec)
	sessSvc := session.NewSessionServiceWithExecutor(mockExec)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry := issues.NewProviderRegistry()

	d := NewDaemon(cfg, gitSvc, sessSvc, registry, logger)
	d.sessionMgr.SetSkipMessageLoad(true)
	d.state = NewDaemonState("/test/repo")
	return d
}

// testDaemonWithExec creates a daemon with a custom mock executor.
func testDaemonWithExec(cfg *config.Config, mockExec *exec.MockExecutor) *Daemon {
	gitSvc := git.NewGitServiceWithExecutor(mockExec)
	sessSvc := session.NewSessionServiceWithExecutor(mockExec)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry := issues.NewProviderRegistry()

	d := NewDaemon(cfg, gitSvc, sessSvc, registry, logger)
	d.sessionMgr.SetSkipMessageLoad(true)
	d.state = NewDaemonState("/test/repo")
	return d
}

func TestDaemonOptions(t *testing.T) {
	cfg := testConfig()

	t.Run("defaults", func(t *testing.T) {
		d := testDaemon(cfg)
		if d.autoMerge != true {
			t.Error("expected autoMerge=true by default for daemon")
		}
		if d.pollInterval != defaultPollInterval {
			t.Errorf("expected default poll interval, got %v", d.pollInterval)
		}
	})

	t.Run("WithDaemonOnce", func(t *testing.T) {
		d := testDaemon(cfg)
		WithDaemonOnce(true)(d)
		if !d.once {
			t.Error("expected once=true")
		}
	})

	t.Run("WithDaemonRepoFilter", func(t *testing.T) {
		d := testDaemon(cfg)
		WithDaemonRepoFilter("owner/repo")(d)
		if d.repoFilter != "owner/repo" {
			t.Errorf("expected owner/repo, got %s", d.repoFilter)
		}
	})

	t.Run("WithDaemonMaxConcurrent", func(t *testing.T) {
		d := testDaemon(cfg)
		WithDaemonMaxConcurrent(5)(d)
		if d.getMaxConcurrent() != 5 {
			t.Errorf("expected 5, got %d", d.getMaxConcurrent())
		}
	})

	t.Run("WithDaemonAutoMerge false", func(t *testing.T) {
		d := testDaemon(cfg)
		WithDaemonAutoMerge(false)(d)
		if d.autoMerge {
			t.Error("expected autoMerge=false")
		}
	})

	t.Run("WithDaemonPollInterval", func(t *testing.T) {
		d := testDaemon(cfg)
		WithDaemonPollInterval(10 * time.Second)(d)
		if d.pollInterval != 10*time.Second {
			t.Errorf("expected 10s, got %v", d.pollInterval)
		}
	})

	t.Run("WithDaemonMergeMethod", func(t *testing.T) {
		d := testDaemon(cfg)
		WithDaemonMergeMethod("squash")(d)
		if d.mergeMethod != "squash" {
			t.Errorf("expected squash, got %s", d.mergeMethod)
		}
	})

	t.Run("WithDaemonReviewPollInterval", func(t *testing.T) {
		d := testDaemon(cfg)
		WithDaemonReviewPollInterval(5 * time.Second)(d)
		if d.reviewPollInterval != 5*time.Second {
			t.Errorf("expected 5s, got %v", d.reviewPollInterval)
		}
	})

	t.Run("default reviewPollInterval", func(t *testing.T) {
		d := testDaemon(cfg)
		if d.reviewPollInterval != defaultReviewPollInterval {
			t.Errorf("expected default review poll interval, got %v", d.reviewPollInterval)
		}
	})
}

func TestDaemon_GetMaxConcurrent(t *testing.T) {
	t.Run("uses config when no override", func(t *testing.T) {
		cfg := testConfig()
		cfg.IssueMaxConcurrent = 5
		d := testDaemon(cfg)
		if got := d.getMaxConcurrent(); got != 5 {
			t.Errorf("expected 5, got %d", got)
		}
	})

	t.Run("uses override", func(t *testing.T) {
		cfg := testConfig()
		cfg.IssueMaxConcurrent = 5
		d := testDaemon(cfg)
		d.maxConcurrent = 10
		if got := d.getMaxConcurrent(); got != 10 {
			t.Errorf("expected 10, got %d", got)
		}
	})
}

func TestDaemon_GetMaxTurns(t *testing.T) {
	cfg := testConfig()
	cfg.AutoMaxTurns = 50
	d := testDaemon(cfg)
	if got := d.getMaxTurns(); got != 50 {
		t.Errorf("expected 50, got %d", got)
	}

	d.maxTurns = 100
	if got := d.getMaxTurns(); got != 100 {
		t.Errorf("expected 100, got %d", got)
	}
}

func TestDaemon_GetMaxDuration(t *testing.T) {
	cfg := testConfig()
	cfg.AutoMaxDurationMin = 30
	d := testDaemon(cfg)
	if got := d.getMaxDuration(); got != 30 {
		t.Errorf("expected 30, got %d", got)
	}

	d.maxDuration = 60
	if got := d.getMaxDuration(); got != 60 {
		t.Errorf("expected 60, got %d", got)
	}
}

func TestDaemon_ActiveSlotCount(t *testing.T) {
	cfg := testConfig()
	d := testDaemon(cfg)

	if d.activeSlotCount() != 0 {
		t.Error("expected 0 active slots")
	}

	d.state.AddWorkItem(&WorkItem{
		ID:       "item-1",
		IssueRef: config.IssueRef{Source: "github", ID: "1"},
	})
	d.state.TransitionWorkItem("item-1", WorkItemCoding)

	if d.activeSlotCount() != 1 {
		t.Errorf("expected 1 active slot, got %d", d.activeSlotCount())
	}
}

func TestDaemon_CollectCompletedWorkers(t *testing.T) {
	cfg := testConfig()
	d := testDaemon(cfg)

	// Add a work item in Coding state with a done worker
	d.state.AddWorkItem(&WorkItem{
		ID:        "item-1",
		IssueRef:  config.IssueRef{Source: "github", ID: "1"},
		SessionID: "sess-1",
		Branch:    "feature-1",
	})
	d.state.TransitionWorkItem("item-1", WorkItemCoding)

	// Add a session for the work item
	sess := testSession("sess-1")
	cfg.AddSession(*sess)

	// Create a done worker
	mock := newMockDoneWorker()
	d.workers["item-1"] = mock

	// collectCompletedWorkers should detect the done worker
	ctx := context.Background()
	d.collectCompletedWorkers(ctx)

	// Worker should be removed
	if _, ok := d.workers["item-1"]; ok {
		t.Error("expected done worker to be removed")
	}
}

func TestDaemon_ProcessWorkItems_AwaitingReview_PRClosed(t *testing.T) {
	cfg := testConfig()
	mockExec := exec.NewMockExecutor(nil)

	// Mock PR state check returning CLOSED
	prStateJSON, _ := json.Marshal(struct {
		State string `json:"state"`
	}{State: "CLOSED"})
	mockExec.AddPrefixMatch("gh", []string{"pr", "view"}, exec.MockResponse{
		Stdout: prStateJSON,
	})

	d := testDaemonWithExec(cfg, mockExec)
	d.repoFilter = "/test/repo"

	// Add session
	sess := testSession("sess-1")
	cfg.AddSession(*sess)

	// Add work item in AwaitingReview
	d.state.AddWorkItem(&WorkItem{
		ID:        "item-1",
		IssueRef:  config.IssueRef{Source: "github", ID: "1"},
		SessionID: "sess-1",
		Branch:    "feature-sess-1",
	})
	d.state.TransitionWorkItem("item-1", WorkItemCoding)
	d.state.TransitionWorkItem("item-1", WorkItemPRCreated)
	d.state.TransitionWorkItem("item-1", WorkItemAwaitingReview)

	// Process
	d.processWorkItems(context.Background())

	// Should be abandoned
	item := d.state.GetWorkItem("item-1")
	if item.State != WorkItemAbandoned {
		t.Errorf("expected abandoned, got %s", item.State)
	}
}

func TestDaemon_ProcessWorkItems_AwaitingCI_Passing(t *testing.T) {
	cfg := testConfig()
	mockExec := exec.NewMockExecutor(nil)

	// Mock CI checks returning passing
	checksJSON, _ := json.Marshal([]struct {
		State string `json:"state"`
	}{{State: "SUCCESS"}})
	mockExec.AddPrefixMatch("gh", []string{"pr", "checks"}, exec.MockResponse{
		Stdout: checksJSON,
	})

	// Mock merge success
	mockExec.AddPrefixMatch("gh", []string{"pr", "merge"}, exec.MockResponse{
		Stdout: []byte("merged"),
	})

	d := testDaemonWithExec(cfg, mockExec)
	d.repoFilter = "/test/repo"

	// Add session
	sess := testSession("sess-1")
	cfg.AddSession(*sess)

	// Add work item in AwaitingCI
	d.state.AddWorkItem(&WorkItem{
		ID:        "item-1",
		IssueRef:  config.IssueRef{Source: "github", ID: "1"},
		SessionID: "sess-1",
		Branch:    "feature-sess-1",
	})
	d.state.TransitionWorkItem("item-1", WorkItemCoding)
	d.state.TransitionWorkItem("item-1", WorkItemPRCreated)
	d.state.TransitionWorkItem("item-1", WorkItemAwaitingReview)
	d.state.TransitionWorkItem("item-1", WorkItemAwaitingCI)

	// Process
	d.processWorkItems(context.Background())

	// Should be completed (CI passed, auto-merge on by default)
	item := d.state.GetWorkItem("item-1")
	if item.State != WorkItemCompleted {
		t.Errorf("expected completed, got %s", item.State)
	}
}

func TestDaemon_ProcessWorkItems_AwaitingCI_Failing(t *testing.T) {
	cfg := testConfig()
	mockExec := exec.NewMockExecutor(nil)

	// Mock CI checks returning failure
	checksJSON, _ := json.Marshal([]struct {
		State string `json:"state"`
	}{{State: "FAILURE"}})
	mockExec.AddPrefixMatch("gh", []string{"pr", "checks"}, exec.MockResponse{
		Stdout: checksJSON,
		Err:    fmt.Errorf("exit status 1"),
	})

	d := testDaemonWithExec(cfg, mockExec)
	d.repoFilter = "/test/repo"

	// Add session
	sess := testSession("sess-1")
	cfg.AddSession(*sess)

	// Add work item in AwaitingCI
	d.state.AddWorkItem(&WorkItem{
		ID:        "item-1",
		IssueRef:  config.IssueRef{Source: "github", ID: "1"},
		SessionID: "sess-1",
		Branch:    "feature-sess-1",
	})
	d.state.TransitionWorkItem("item-1", WorkItemCoding)
	d.state.TransitionWorkItem("item-1", WorkItemPRCreated)
	d.state.TransitionWorkItem("item-1", WorkItemAwaitingReview)
	d.state.TransitionWorkItem("item-1", WorkItemAwaitingCI)

	// Process
	d.processWorkItems(context.Background())

	// Should be back in AwaitingReview (CI failed)
	item := d.state.GetWorkItem("item-1")
	if item.State != WorkItemAwaitingReview {
		t.Errorf("expected awaiting_review (CI failed), got %s", item.State)
	}
}

func TestDaemon_ProcessWorkItems_AwaitingCI_NoAutoMerge(t *testing.T) {
	cfg := testConfig()
	mockExec := exec.NewMockExecutor(nil)

	checksJSON, _ := json.Marshal([]struct {
		State string `json:"state"`
	}{{State: "SUCCESS"}})
	mockExec.AddPrefixMatch("gh", []string{"pr", "checks"}, exec.MockResponse{
		Stdout: checksJSON,
	})

	d := testDaemonWithExec(cfg, mockExec)
	d.repoFilter = "/test/repo"
	d.autoMerge = false // Disable auto-merge

	sess := testSession("sess-1")
	cfg.AddSession(*sess)

	d.state.AddWorkItem(&WorkItem{
		ID:        "item-1",
		IssueRef:  config.IssueRef{Source: "github", ID: "1"},
		SessionID: "sess-1",
		Branch:    "feature-sess-1",
	})
	d.state.TransitionWorkItem("item-1", WorkItemCoding)
	d.state.TransitionWorkItem("item-1", WorkItemPRCreated)
	d.state.TransitionWorkItem("item-1", WorkItemAwaitingReview)
	d.state.TransitionWorkItem("item-1", WorkItemAwaitingCI)

	d.processWorkItems(context.Background())

	// Should still be in AwaitingCI (auto-merge disabled)
	item := d.state.GetWorkItem("item-1")
	if item.State != WorkItemAwaitingCI {
		t.Errorf("expected awaiting_ci (auto-merge disabled), got %s", item.State)
	}
}

func TestDaemon_MatchesRepoFilter(t *testing.T) {
	t.Run("exact path", func(t *testing.T) {
		cfg := testConfig()
		d := testDaemon(cfg)
		d.repoFilter = "/path/to/repo"
		if !d.matchesRepoFilter(context.Background(), "/path/to/repo") {
			t.Error("expected match")
		}
	})

	t.Run("owner/repo via remote", func(t *testing.T) {
		cfg := testConfig()
		mockExec := exec.NewMockExecutor(nil)
		mockExec.AddPrefixMatch("git", []string{"remote", "get-url"}, exec.MockResponse{
			Stdout: []byte("git@github.com:owner/repo.git\n"),
		})
		d := testDaemonWithExec(cfg, mockExec)
		d.repoFilter = "owner/repo"
		if !d.matchesRepoFilter(context.Background(), "/some/path") {
			t.Error("expected match via remote")
		}
	})

	t.Run("no match", func(t *testing.T) {
		cfg := testConfig()
		d := testDaemon(cfg)
		d.repoFilter = "/other/path"
		if d.matchesRepoFilter(context.Background(), "/path/to/repo") {
			t.Error("expected no match")
		}
	})
}

func TestDaemon_HasExistingSession(t *testing.T) {
	cfg := testConfig()
	d := testDaemon(cfg)

	if d.hasExistingSession("/repo", "42") {
		t.Error("expected false for empty sessions")
	}

	cfg.AddSession(config.Session{
		ID:       "sess-1",
		RepoPath: "/repo",
		IssueRef: &config.IssueRef{ID: "42", Source: "github"},
	})

	if !d.hasExistingSession("/repo", "42") {
		t.Error("expected true for existing session")
	}

	if d.hasExistingSession("/repo", "99") {
		t.Error("expected false for different issue")
	}
}

func TestDaemon_GetMergeMethod(t *testing.T) {
	t.Run("uses config when no override", func(t *testing.T) {
		cfg := testConfig()
		cfg.AutoMergeMethod = "squash"
		d := testDaemon(cfg)
		if got := d.getMergeMethod(); got != "squash" {
			t.Errorf("expected squash, got %s", got)
		}
	})

	t.Run("uses override", func(t *testing.T) {
		cfg := testConfig()
		cfg.AutoMergeMethod = "squash"
		d := testDaemon(cfg)
		d.mergeMethod = "merge"
		if got := d.getMergeMethod(); got != "merge" {
			t.Errorf("expected merge, got %s", got)
		}
	})

	t.Run("defaults to rebase", func(t *testing.T) {
		cfg := testConfig()
		d := testDaemon(cfg)
		if got := d.getMergeMethod(); got != "rebase" {
			t.Errorf("expected rebase, got %s", got)
		}
	})
}

func TestDaemon_ReviewPollIntervalGating(t *testing.T) {
	cfg := testConfig()
	mockExec := exec.NewMockExecutor(nil)

	// Mock PR state check returning OPEN
	prStateJSON, _ := json.Marshal(struct {
		State string `json:"state"`
	}{State: "OPEN"})
	mockExec.AddPrefixMatch("gh", []string{"pr", "view"}, exec.MockResponse{
		Stdout: prStateJSON,
	})

	// Mock batch PR states with comments (no new comments)
	batchJSON, _ := json.Marshal([]struct {
		State       string            `json:"state"`
		HeadRefName string            `json:"headRefName"`
		Comments    []json.RawMessage `json:"comments"`
		Reviews     []json.RawMessage `json:"reviews"`
	}{
		{State: "OPEN", HeadRefName: "feature-sess-1", Comments: nil, Reviews: nil},
	})
	mockExec.AddPrefixMatch("gh", []string{"pr", "list"}, exec.MockResponse{
		Stdout: batchJSON,
	})

	d := testDaemonWithExec(cfg, mockExec)
	d.repoFilter = "/test/repo"
	// Set review poll interval to something large
	d.reviewPollInterval = 1 * time.Hour
	// Set last poll to now, so the interval hasn't elapsed
	d.lastReviewPollAt = time.Now()

	// Add session
	sess := testSession("sess-1")
	cfg.AddSession(*sess)

	// Add work item in AwaitingReview
	d.state.AddWorkItem(&WorkItem{
		ID:        "item-1",
		IssueRef:  config.IssueRef{Source: "github", ID: "1"},
		SessionID: "sess-1",
		Branch:    "feature-sess-1",
	})
	d.state.TransitionWorkItem("item-1", WorkItemCoding)
	d.state.TransitionWorkItem("item-1", WorkItemPRCreated)
	d.state.TransitionWorkItem("item-1", WorkItemAwaitingReview)

	// Process — review polling should be skipped because interval hasn't elapsed
	d.processWorkItems(context.Background())

	// Should still be in AwaitingReview (review poll was gated)
	item := d.state.GetWorkItem("item-1")
	if item.State != WorkItemAwaitingReview {
		t.Errorf("expected awaiting_review (review poll gated), got %s", item.State)
	}

	// Now set lastReviewPollAt far in the past to simulate elapsed interval
	d.lastReviewPollAt = time.Now().Add(-2 * time.Hour)

	// Process again — review polling should now proceed
	d.processWorkItems(context.Background())

	// Item should still be in AwaitingReview (no new comments, no review decision)
	// but the poll should have been executed (lastReviewPollAt updated)
	if time.Since(d.lastReviewPollAt) > 1*time.Second {
		t.Error("expected lastReviewPollAt to be updated after review poll ran")
	}
}

func TestDaemon_ToAgent(t *testing.T) {
	cfg := testConfig()
	d := testDaemon(cfg)
	d.repoFilter = "owner/repo"
	d.maxConcurrent = 5
	d.maxTurns = 100
	d.maxDuration = 60
	d.autoMerge = true
	d.mergeMethod = "squash"

	a := d.toAgent()

	if a.config != d.config {
		t.Error("config mismatch")
	}
	if a.repoFilter != "owner/repo" {
		t.Error("repoFilter mismatch")
	}
	if a.maxConcurrent != 5 {
		t.Error("maxConcurrent mismatch")
	}
	if a.maxTurns != 100 {
		t.Error("maxTurns mismatch")
	}
	if a.maxDuration != 60 {
		t.Error("maxDuration mismatch")
	}
	if !a.autoMerge {
		t.Error("autoMerge mismatch")
	}
	if a.mergeMethod != "squash" {
		t.Errorf("mergeMethod mismatch: expected squash, got %s", a.mergeMethod)
	}
}

// Recovery tests

func TestDaemon_RecoverFromState_Queued(t *testing.T) {
	cfg := testConfig()
	d := testDaemon(cfg)

	d.state.AddWorkItem(&WorkItem{
		ID:       "item-1",
		IssueRef: config.IssueRef{Source: "github", ID: "1"},
	})

	d.recoverFromState(context.Background())

	// Queued items should remain queued
	item := d.state.GetWorkItem("item-1")
	if item.State != WorkItemQueued {
		t.Errorf("expected queued, got %s", item.State)
	}
}

func TestDaemon_RecoverFromState_CodingWithPR(t *testing.T) {
	cfg := testConfig()
	mockExec := exec.NewMockExecutor(nil)

	// PR exists and is open
	prStateJSON, _ := json.Marshal(struct {
		State string `json:"state"`
	}{State: "OPEN"})
	mockExec.AddPrefixMatch("gh", []string{"pr", "view"}, exec.MockResponse{
		Stdout: prStateJSON,
	})

	d := testDaemonWithExec(cfg, mockExec)

	sess := testSession("sess-1")
	cfg.AddSession(*sess)

	d.state.AddWorkItem(&WorkItem{
		ID:        "item-1",
		IssueRef:  config.IssueRef{Source: "github", ID: "1"},
		SessionID: "sess-1",
		Branch:    "feature-sess-1",
	})
	d.state.TransitionWorkItem("item-1", WorkItemCoding)

	d.recoverFromState(context.Background())

	item := d.state.GetWorkItem("item-1")
	if item.State != WorkItemAwaitingReview {
		t.Errorf("expected awaiting_review (PR exists), got %s", item.State)
	}
}

func TestDaemon_RecoverFromState_CodingNoPR(t *testing.T) {
	cfg := testConfig()
	mockExec := exec.NewMockExecutor(nil)

	// PR not found (error)
	mockExec.AddPrefixMatch("gh", []string{"pr", "view"}, exec.MockResponse{
		Err: fmt.Errorf("no PR found"),
	})

	d := testDaemonWithExec(cfg, mockExec)

	sess := testSession("sess-1")
	cfg.AddSession(*sess)

	d.state.AddWorkItem(&WorkItem{
		ID:        "item-1",
		IssueRef:  config.IssueRef{Source: "github", ID: "1"},
		SessionID: "sess-1",
		Branch:    "feature-sess-1",
	})
	d.state.TransitionWorkItem("item-1", WorkItemCoding)

	d.recoverFromState(context.Background())

	item := d.state.GetWorkItem("item-1")
	if item.State != WorkItemQueued {
		t.Errorf("expected queued (no PR), got %s", item.State)
	}
}

func TestDaemon_RecoverFromState_AddressingFeedback(t *testing.T) {
	cfg := testConfig()
	d := testDaemon(cfg)

	d.state.AddWorkItem(&WorkItem{
		ID:        "item-1",
		IssueRef:  config.IssueRef{Source: "github", ID: "1"},
		SessionID: "sess-1",
		Branch:    "feature-1",
	})
	d.state.TransitionWorkItem("item-1", WorkItemCoding)
	d.state.TransitionWorkItem("item-1", WorkItemPRCreated)
	d.state.TransitionWorkItem("item-1", WorkItemAwaitingReview)
	d.state.TransitionWorkItem("item-1", WorkItemAddressingFeedback)

	d.recoverFromState(context.Background())

	item := d.state.GetWorkItem("item-1")
	if item.State != WorkItemAwaitingReview {
		t.Errorf("expected awaiting_review, got %s", item.State)
	}
}

func TestDaemon_RecoverFromState_Pushing(t *testing.T) {
	cfg := testConfig()
	d := testDaemon(cfg)

	d.state.AddWorkItem(&WorkItem{
		ID:        "item-1",
		IssueRef:  config.IssueRef{Source: "github", ID: "1"},
		SessionID: "sess-1",
		Branch:    "feature-1",
	})
	d.state.TransitionWorkItem("item-1", WorkItemCoding)
	d.state.TransitionWorkItem("item-1", WorkItemPRCreated)
	d.state.TransitionWorkItem("item-1", WorkItemAwaitingReview)
	d.state.TransitionWorkItem("item-1", WorkItemAddressingFeedback)
	d.state.TransitionWorkItem("item-1", WorkItemPushing)

	d.recoverFromState(context.Background())

	item := d.state.GetWorkItem("item-1")
	if item.State != WorkItemAwaitingReview {
		t.Errorf("expected awaiting_review, got %s", item.State)
	}
}

func TestDaemon_RecoverFromState_MergingCompleted(t *testing.T) {
	cfg := testConfig()
	mockExec := exec.NewMockExecutor(nil)

	// PR is merged
	prStateJSON, _ := json.Marshal(struct {
		State string `json:"state"`
	}{State: "MERGED"})
	mockExec.AddPrefixMatch("gh", []string{"pr", "view"}, exec.MockResponse{
		Stdout: prStateJSON,
	})

	d := testDaemonWithExec(cfg, mockExec)

	sess := testSession("sess-1")
	cfg.AddSession(*sess)

	d.state.AddWorkItem(&WorkItem{
		ID:        "item-1",
		IssueRef:  config.IssueRef{Source: "github", ID: "1"},
		SessionID: "sess-1",
		Branch:    "feature-sess-1",
	})
	d.state.TransitionWorkItem("item-1", WorkItemCoding)
	d.state.TransitionWorkItem("item-1", WorkItemPRCreated)
	d.state.TransitionWorkItem("item-1", WorkItemAwaitingReview)
	d.state.TransitionWorkItem("item-1", WorkItemAwaitingCI)
	d.state.TransitionWorkItem("item-1", WorkItemMerging)

	d.recoverFromState(context.Background())

	item := d.state.GetWorkItem("item-1")
	if item.State != WorkItemCompleted {
		t.Errorf("expected completed, got %s", item.State)
	}
	if item.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
}

func TestDaemon_RecoverFromState_MergingNotCompleted(t *testing.T) {
	cfg := testConfig()
	mockExec := exec.NewMockExecutor(nil)

	// PR is still open
	prStateJSON, _ := json.Marshal(struct {
		State string `json:"state"`
	}{State: "OPEN"})
	mockExec.AddPrefixMatch("gh", []string{"pr", "view"}, exec.MockResponse{
		Stdout: prStateJSON,
	})

	d := testDaemonWithExec(cfg, mockExec)

	sess := testSession("sess-1")
	cfg.AddSession(*sess)

	d.state.AddWorkItem(&WorkItem{
		ID:        "item-1",
		IssueRef:  config.IssueRef{Source: "github", ID: "1"},
		SessionID: "sess-1",
		Branch:    "feature-sess-1",
	})
	d.state.TransitionWorkItem("item-1", WorkItemCoding)
	d.state.TransitionWorkItem("item-1", WorkItemPRCreated)
	d.state.TransitionWorkItem("item-1", WorkItemAwaitingReview)
	d.state.TransitionWorkItem("item-1", WorkItemAwaitingCI)
	d.state.TransitionWorkItem("item-1", WorkItemMerging)

	d.recoverFromState(context.Background())

	item := d.state.GetWorkItem("item-1")
	if item.State != WorkItemAwaitingCI {
		t.Errorf("expected awaiting_ci, got %s", item.State)
	}
}

func TestDaemon_RecoverFromState_TerminalStatesUntouched(t *testing.T) {
	cfg := testConfig()
	d := testDaemon(cfg)

	// Completed
	d.state.AddWorkItem(&WorkItem{
		ID:       "completed-1",
		IssueRef: config.IssueRef{Source: "github", ID: "1"},
	})
	d.state.TransitionWorkItem("completed-1", WorkItemCoding)
	d.state.TransitionWorkItem("completed-1", WorkItemFailed)

	// Failed
	d.state.AddWorkItem(&WorkItem{
		ID:       "failed-1",
		IssueRef: config.IssueRef{Source: "github", ID: "2"},
	})
	d.state.TransitionWorkItem("failed-1", WorkItemCoding)
	d.state.TransitionWorkItem("failed-1", WorkItemFailed)

	d.recoverFromState(context.Background())

	// Should remain unchanged
	if d.state.GetWorkItem("completed-1").State != WorkItemFailed {
		t.Error("expected completed item to remain unchanged")
	}
	if d.state.GetWorkItem("failed-1").State != WorkItemFailed {
		t.Error("expected failed item to remain unchanged")
	}
}

func TestDaemon_RecoverFromState_CodingNoBranch(t *testing.T) {
	cfg := testConfig()
	d := testDaemon(cfg)

	d.state.AddWorkItem(&WorkItem{
		ID:       "item-1",
		IssueRef: config.IssueRef{Source: "github", ID: "1"},
	})
	d.state.TransitionWorkItem("item-1", WorkItemCoding)

	d.recoverFromState(context.Background())

	item := d.state.GetWorkItem("item-1")
	if item.State != WorkItemQueued {
		t.Errorf("expected queued (no branch), got %s", item.State)
	}
}

func TestDaemon_RecoverFromState_PRCreatedNoSession(t *testing.T) {
	cfg := testConfig()
	d := testDaemon(cfg)

	d.state.AddWorkItem(&WorkItem{
		ID:        "item-1",
		IssueRef:  config.IssueRef{Source: "github", ID: "1"},
		SessionID: "nonexistent",
		Branch:    "feature-1",
	})
	// Manually set state to PRCreated (bypassing validation since session doesn't exist)
	d.state.TransitionWorkItem("item-1", WorkItemCoding)
	d.state.mu.Lock()
	d.state.WorkItems["item-1"].State = WorkItemPRCreated
	d.state.mu.Unlock()

	d.recoverFromState(context.Background())

	item := d.state.GetWorkItem("item-1")
	if item.State != WorkItemFailed {
		t.Errorf("expected failed (no session), got %s", item.State)
	}
}

func TestDaemon_PollForNewIssues(t *testing.T) {
	t.Run("no repo filter skips polling", func(t *testing.T) {
		cfg := testConfig()
		d := testDaemon(cfg)
		// repoFilter is empty by default in testDaemon
		d.repoFilter = ""

		d.pollForNewIssues(context.Background())

		// No items should be added
		if len(d.state.WorkItems) != 0 {
			t.Errorf("expected 0 work items, got %d", len(d.state.WorkItems))
		}
	})

	t.Run("at concurrency limit skips polling", func(t *testing.T) {
		cfg := testConfig()
		d := testDaemon(cfg)
		d.repoFilter = "/test/repo"
		d.maxConcurrent = 1

		// Add an active item
		d.state.AddWorkItem(&WorkItem{
			ID:       "active-1",
			IssueRef: config.IssueRef{Source: "github", ID: "1"},
		})
		d.state.TransitionWorkItem("active-1", WorkItemCoding)

		d.pollForNewIssues(context.Background())

		// Should not have added more items
		if len(d.state.WorkItems) != 1 {
			t.Errorf("expected 1 work item, got %d", len(d.state.WorkItems))
		}
	})
}

func TestDaemon_IssueFromWorkItem(t *testing.T) {
	item := &WorkItem{
		ID: "item-1",
		IssueRef: config.IssueRef{
			Source: "github",
			ID:     "42",
			Title:  "Fix the bug",
			URL:    "https://github.com/owner/repo/issues/42",
		},
	}

	issue := issueFromWorkItem(item)

	if issue.ID != "42" {
		t.Errorf("expected ID 42, got %s", issue.ID)
	}
	if issue.Title != "Fix the bug" {
		t.Errorf("expected title, got %s", issue.Title)
	}
	if issue.Source != issues.SourceGitHub {
		t.Errorf("expected github source, got %s", issue.Source)
	}
}

func TestDaemon_StartQueuedItems(t *testing.T) {
	t.Run("respects concurrency limit", func(t *testing.T) {
		cfg := testConfig()
		d := testDaemon(cfg)
		d.maxConcurrent = 1
		d.repoFilter = "/test/repo"
		cfg.Repos = []string{"/test/repo"}

		// Add two queued items
		d.state.AddWorkItem(&WorkItem{
			ID:       "item-1",
			IssueRef: config.IssueRef{Source: "github", ID: "1", Title: "Bug 1"},
		})
		d.state.AddWorkItem(&WorkItem{
			ID:       "item-2",
			IssueRef: config.IssueRef{Source: "github", ID: "2", Title: "Bug 2"},
		})

		// Add a coding item to fill the slot
		d.state.AddWorkItem(&WorkItem{
			ID:       "active-1",
			IssueRef: config.IssueRef{Source: "github", ID: "3"},
		})
		d.state.TransitionWorkItem("active-1", WorkItemCoding)

		d.startQueuedItems(context.Background())

		// Both should still be queued since slot is full
		if d.state.GetWorkItem("item-1").State != WorkItemQueued {
			t.Error("item-1 should still be queued")
		}
		if d.state.GetWorkItem("item-2").State != WorkItemQueued {
			t.Error("item-2 should still be queued")
		}
	})
}

// newMockDoneWorker creates a SessionWorker that is already done.
func newMockDoneWorker() *SessionWorker {
	w := &SessionWorker{
		done: make(chan struct{}),
	}
	close(w.done)
	return w
}
