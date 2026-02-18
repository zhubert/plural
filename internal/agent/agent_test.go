package agent

import (
	"context"
	"testing"
	"time"

	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/exec"
	"github.com/zhubert/plural/internal/git"
)

func TestAgentOptions(t *testing.T) {
	cfg := testConfig()

	t.Run("WithOnce", func(t *testing.T) {
		a := testAgent(cfg)
		if a.once {
			t.Error("expected once=false by default")
		}

		a2 := testAgent(cfg)
		WithOnce(true)(a2)
		if !a2.once {
			t.Error("expected once=true after WithOnce")
		}
	})

	t.Run("WithRepoFilter", func(t *testing.T) {
		a := testAgent(cfg)
		if a.repoFilter != "" {
			t.Error("expected empty repoFilter by default")
		}

		WithRepoFilter("/test/repo")(a)
		if a.repoFilter != "/test/repo" {
			t.Errorf("expected /test/repo, got %q", a.repoFilter)
		}
	})

	t.Run("WithMaxConcurrent", func(t *testing.T) {
		a := testAgent(cfg)
		if a.maxConcurrent != 0 {
			t.Error("expected 0 maxConcurrent by default")
		}

		WithMaxConcurrent(5)(a)
		if a.maxConcurrent != 5 {
			t.Errorf("expected 5, got %d", a.maxConcurrent)
		}
	})

	t.Run("WithPollInterval", func(t *testing.T) {
		a := testAgent(cfg)
		if a.pollInterval != defaultPollInterval {
			t.Errorf("expected default interval, got %v", a.pollInterval)
		}

		WithPollInterval(10 * time.Second)(a)
		if a.pollInterval != 10*time.Second {
			t.Errorf("expected 10s, got %v", a.pollInterval)
		}
	})

	t.Run("WithMaxTurns", func(t *testing.T) {
		a := testAgent(cfg)
		if a.maxTurns != 0 {
			t.Error("expected 0 maxTurns by default")
		}

		WithMaxTurns(100)(a)
		if a.maxTurns != 100 {
			t.Errorf("expected 100, got %d", a.maxTurns)
		}
	})

	t.Run("WithMaxDuration", func(t *testing.T) {
		a := testAgent(cfg)
		if a.maxDuration != 0 {
			t.Error("expected 0 maxDuration by default")
		}

		WithMaxDuration(60)(a)
		if a.maxDuration != 60 {
			t.Errorf("expected 60, got %d", a.maxDuration)
		}
	})

	t.Run("WithAutoAddressPRComments", func(t *testing.T) {
		a := testAgent(cfg)
		if a.autoAddressPRComments {
			t.Error("expected autoAddressPRComments=false by default")
		}

		WithAutoAddressPRComments(true)(a)
		if !a.autoAddressPRComments {
			t.Error("expected autoAddressPRComments=true after WithAutoAddressPRComments")
		}
	})

	t.Run("WithAutoBroadcastPR", func(t *testing.T) {
		a := testAgent(cfg)
		if a.autoBroadcastPR {
			t.Error("expected autoBroadcastPR=false by default")
		}

		WithAutoBroadcastPR(true)(a)
		if !a.autoBroadcastPR {
			t.Error("expected autoBroadcastPR=true after WithAutoBroadcastPR")
		}
	})

	t.Run("WithAutoMerge", func(t *testing.T) {
		a := testAgent(cfg)
		if a.autoMerge {
			t.Error("expected autoMerge=false by default")
		}

		WithAutoMerge(true)(a)
		if !a.autoMerge {
			t.Error("expected autoMerge=true after WithAutoMerge")
		}
	})
}

func TestGetMaxConcurrent(t *testing.T) {
	t.Run("uses config when no override", func(t *testing.T) {
		cfg := testConfig()
		cfg.IssueMaxConcurrent = 5
		a := testAgent(cfg)

		if got := a.getMaxConcurrent(); got != 5 {
			t.Errorf("expected 5, got %d", got)
		}
	})

	t.Run("uses override when set", func(t *testing.T) {
		cfg := testConfig()
		cfg.IssueMaxConcurrent = 5
		a := testAgent(cfg)
		a.maxConcurrent = 10

		if got := a.getMaxConcurrent(); got != 10 {
			t.Errorf("expected 10, got %d", got)
		}
	})

	t.Run("defaults to 3 when config is zero", func(t *testing.T) {
		cfg := testConfig()
		cfg.IssueMaxConcurrent = 0
		a := testAgent(cfg)

		if got := a.getMaxConcurrent(); got != 3 {
			t.Errorf("expected default 3, got %d", got)
		}
	})
}

func TestActiveWorkerCount(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	if got := a.activeWorkerCount(); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}

	// Add an active worker
	sess1 := testSession("worker-1")
	mock1 := claude.NewMockRunner("worker-1", true, nil)
	w1 := NewSessionWorker(a, sess1, mock1, "")
	a.workers["worker-1"] = w1

	if got := a.activeWorkerCount(); got != 1 {
		t.Errorf("expected 1, got %d", got)
	}

	// Add another active worker
	sess2 := testSession("worker-2")
	mock2 := claude.NewMockRunner("worker-2", true, nil)
	w2 := NewSessionWorker(a, sess2, mock2, "")
	a.workers["worker-2"] = w2

	if got := a.activeWorkerCount(); got != 2 {
		t.Errorf("expected 2, got %d", got)
	}

	// Mark one as done
	close(w1.done)

	if got := a.activeWorkerCount(); got != 1 {
		t.Errorf("expected 1 after completing one, got %d", got)
	}
}

func TestCleanupCompletedWorkers(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	// Add workers — one active, one done
	sess1 := testSession("active-1")
	mock1 := claude.NewMockRunner("active-1", true, nil)
	w1 := NewSessionWorker(a, sess1, mock1, "")
	a.workers["active-1"] = w1

	sess2 := testSession("done-1")
	mock2 := claude.NewMockRunner("done-1", true, nil)
	w2 := NewSessionWorker(a, sess2, mock2, "")
	close(w2.done) // Mark as done
	a.workers["done-1"] = w2

	if len(a.workers) != 2 {
		t.Fatalf("expected 2 workers, got %d", len(a.workers))
	}

	a.cleanupCompletedWorkers()

	if len(a.workers) != 1 {
		t.Errorf("expected 1 worker after cleanup, got %d", len(a.workers))
	}
	if _, ok := a.workers["active-1"]; !ok {
		t.Error("expected active-1 to remain")
	}
	if _, ok := a.workers["done-1"]; ok {
		t.Error("expected done-1 to be removed")
	}
}

func TestWaitForWorkers(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	sess := testSession("wait-1")
	cfg.AddSession(*sess)

	mock := claude.NewMockRunner("wait-1", true, nil)
	mock.QueueResponse(
		claude.ResponseChunk{Done: true},
	)

	w := NewSessionWorker(a, sess, mock, "Do this")
	a.workers["wait-1"] = w

	ctx := context.Background()
	w.Start(ctx)

	// waitForWorkers should return after the worker finishes
	done := make(chan struct{})
	go func() {
		a.waitForWorkers(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("waitForWorkers timed out")
	}
}

func TestShutdown(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	// Create a worker that is in-flight (no done chunk in response)
	sess := testSession("shutdown-1")
	cfg.AddSession(*sess)

	mock := claude.NewMockRunner("shutdown-1", true, nil)
	mock.QueueResponse(
		claude.ResponseChunk{Type: claude.ChunkTypeText, Content: "Working..."},
	)

	w := NewSessionWorker(a, sess, mock, "Do work")
	a.workers["shutdown-1"] = w

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	// Give worker time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel first, then shutdown (Shutdown also cancels workers)
	cancel()
	a.Shutdown()

	// Worker should be done after shutdown
	done := make(chan struct{})
	go func() {
		w.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop after shutdown")
	}
}

func TestPollForIssues_NoPollingRepos(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	// No repos configured at all
	result := a.pollForIssues(context.Background())
	if len(result) != 0 {
		t.Errorf("expected no issues, got %d groups", len(result))
	}
}

func TestPollForIssues_RespectsRepoFilter(t *testing.T) {
	cfg := testConfig()
	cfg.Repos = []string{"/repo1", "/repo2"}
	a := testAgent(cfg)
	a.repoFilter = "/repo1"

	// Even with two repos registered, only repo1 should be polled.
	// The actual fetch will fail (mock executor returns no output) but
	// the filtering logic is what we're testing.
	result := a.pollForIssues(context.Background())
	// No issues found because mock executor returns nothing, but the code path is exercised.
	if len(result) != 0 {
		t.Errorf("expected 0 groups (mock returns nothing), got %d", len(result))
	}
}

func TestPollForIssues_DeduplicatesExistingSessions(t *testing.T) {
	cfg := testConfig()
	cfg.Repos = []string{"/repo1"}
	// Add an existing session for issue #1
	cfg.Sessions = []config.Session{
		{
			ID:       "existing-1",
			RepoPath: "/repo1",
			IssueRef: &config.IssueRef{
				ID:     "1",
				Source: "github",
			},
		},
	}
	a := testAgent(cfg)

	// The deduplication map should include issue #1
	result := a.pollForIssues(context.Background())
	// No new issues found (mock executor returns nothing)
	if len(result) != 0 {
		t.Errorf("expected 0 groups, got %d", len(result))
	}
}

func TestRemoveIssueWIPLabel_NilIssueRef(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	sess := &config.Session{ID: "test", RepoPath: "/repo"}
	// Should not panic with nil IssueRef
	a.removeIssueWIPLabel(sess)
}

func TestRemoveIssueWIPLabel_InvalidIssueID(t *testing.T) {
	cfg := testConfig()
	a := testAgent(cfg)

	sess := &config.Session{
		ID:       "test",
		RepoPath: "/repo",
		IssueRef: &config.IssueRef{ID: "not-a-number"},
	}
	// Should not panic with non-numeric issue ID
	a.removeIssueWIPLabel(sess)
}

func TestGetMaxTurns(t *testing.T) {
	t.Run("uses config when no override", func(t *testing.T) {
		cfg := testConfig()
		cfg.AutoMaxTurns = 50
		a := testAgent(cfg)

		if got := a.getMaxTurns(); got != 50 {
			t.Errorf("expected 50, got %d", got)
		}
	})

	t.Run("uses override when set", func(t *testing.T) {
		cfg := testConfig()
		cfg.AutoMaxTurns = 50
		a := testAgent(cfg)
		a.maxTurns = 100

		if got := a.getMaxTurns(); got != 100 {
			t.Errorf("expected 100, got %d", got)
		}
	})

	t.Run("defaults to 50 when config is zero", func(t *testing.T) {
		cfg := testConfig()
		cfg.AutoMaxTurns = 0
		a := testAgent(cfg)

		if got := a.getMaxTurns(); got != 50 {
			t.Errorf("expected default 50, got %d", got)
		}
	})
}

func TestGetMaxDuration(t *testing.T) {
	t.Run("uses config when no override", func(t *testing.T) {
		cfg := testConfig()
		cfg.AutoMaxDurationMin = 30
		a := testAgent(cfg)

		if got := a.getMaxDuration(); got != 30 {
			t.Errorf("expected 30, got %d", got)
		}
	})

	t.Run("uses override when set", func(t *testing.T) {
		cfg := testConfig()
		cfg.AutoMaxDurationMin = 30
		a := testAgent(cfg)
		a.maxDuration = 60

		if got := a.getMaxDuration(); got != 60 {
			t.Errorf("expected 60, got %d", got)
		}
	})

	t.Run("defaults to 30 when config is zero", func(t *testing.T) {
		cfg := testConfig()
		cfg.AutoMaxDurationMin = 0
		a := testAgent(cfg)

		if got := a.getMaxDuration(); got != 30 {
			t.Errorf("expected default 30, got %d", got)
		}
	})
}

func TestGetAutoMerge(t *testing.T) {
	t.Run("false by default", func(t *testing.T) {
		cfg := testConfig()
		a := testAgent(cfg)

		if a.getAutoMerge() {
			t.Error("expected false by default")
		}
	})

	t.Run("true when set", func(t *testing.T) {
		cfg := testConfig()
		a := testAgent(cfg)
		a.autoMerge = true

		if !a.getAutoMerge() {
			t.Error("expected true when set")
		}
	})
}

func TestGetAutoAddressPRComments(t *testing.T) {
	t.Run("false when both CLI and config are false", func(t *testing.T) {
		cfg := testConfig()
		a := testAgent(cfg)

		if a.getAutoAddressPRComments() {
			t.Error("expected false")
		}
	})

	t.Run("true when CLI flag is true", func(t *testing.T) {
		cfg := testConfig()
		a := testAgent(cfg)
		a.autoAddressPRComments = true

		if !a.getAutoAddressPRComments() {
			t.Error("expected true when CLI flag set")
		}
	})

	t.Run("true when config is true", func(t *testing.T) {
		cfg := testConfig()
		cfg.AutoAddressPRComments = true
		a := testAgent(cfg)

		if !a.getAutoAddressPRComments() {
			t.Error("expected true when config set")
		}
	})
}

func TestGetAutoBroadcastPR(t *testing.T) {
	t.Run("false when both CLI and config are false", func(t *testing.T) {
		cfg := testConfig()
		a := testAgent(cfg)

		if a.getAutoBroadcastPR() {
			t.Error("expected false")
		}
	})

	t.Run("true when CLI flag is true", func(t *testing.T) {
		cfg := testConfig()
		a := testAgent(cfg)
		a.autoBroadcastPR = true

		if !a.getAutoBroadcastPR() {
			t.Error("expected true when CLI flag set")
		}
	})

	t.Run("true when config is true", func(t *testing.T) {
		cfg := testConfig()
		cfg.AutoBroadcastPR = true
		a := testAgent(cfg)

		if !a.getAutoBroadcastPR() {
			t.Error("expected true when config set")
		}
	})
}

func TestMatchesRepoFilter(t *testing.T) {
	t.Run("exact path match", func(t *testing.T) {
		cfg := testConfig()
		a := testAgent(cfg)
		a.repoFilter = "/path/to/repo"

		if !a.matchesRepoFilter(context.Background(), "/path/to/repo") {
			t.Error("expected exact path to match")
		}
	})

	t.Run("owner/repo matches SSH remote", func(t *testing.T) {
		cfg := testConfig()
		mockExec := exec.NewMockExecutor(nil)
		mockExec.AddPrefixMatch("git", []string{"remote", "get-url"}, exec.MockResponse{
			Stdout: []byte("git@github.com:zhubert/plural.git\n"),
		})
		gitSvc := git.NewGitServiceWithExecutor(mockExec)
		a := testAgent(cfg)
		a.gitService = gitSvc
		a.repoFilter = "zhubert/plural"

		if !a.matchesRepoFilter(context.Background(), "/some/path") {
			t.Error("expected owner/repo to match SSH remote")
		}
	})

	t.Run("owner/repo matches HTTPS remote", func(t *testing.T) {
		cfg := testConfig()
		mockExec := exec.NewMockExecutor(nil)
		mockExec.AddPrefixMatch("git", []string{"remote", "get-url"}, exec.MockResponse{
			Stdout: []byte("https://github.com/zhubert/plural.git\n"),
		})
		gitSvc := git.NewGitServiceWithExecutor(mockExec)
		a := testAgent(cfg)
		a.gitService = gitSvc
		a.repoFilter = "zhubert/plural"

		if !a.matchesRepoFilter(context.Background(), "/some/path") {
			t.Error("expected owner/repo to match HTTPS remote")
		}
	})

	t.Run("no match when remote differs", func(t *testing.T) {
		cfg := testConfig()
		mockExec := exec.NewMockExecutor(nil)
		mockExec.AddPrefixMatch("git", []string{"remote", "get-url"}, exec.MockResponse{
			Stdout: []byte("git@github.com:other/repo.git\n"),
		})
		gitSvc := git.NewGitServiceWithExecutor(mockExec)
		a := testAgent(cfg)
		a.gitService = gitSvc
		a.repoFilter = "zhubert/plural"

		if a.matchesRepoFilter(context.Background(), "/some/path") {
			t.Error("expected no match when remote differs")
		}
	})

	t.Run("filesystem path with slash does not trigger owner/repo", func(t *testing.T) {
		cfg := testConfig()
		a := testAgent(cfg)
		a.repoFilter = "/path/to/repo"

		// Should only match exact path, not try owner/repo matching
		if a.matchesRepoFilter(context.Background(), "/different/path") {
			t.Error("expected no match for different path")
		}
	})
}
