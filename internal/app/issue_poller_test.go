package app

import (
	"testing"

	"github.com/zhubert/plural/internal/config"
	pexec "github.com/zhubert/plural/internal/exec"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/issues"
)

func TestIssuePollTick(t *testing.T) {
	cmd := IssuePollTick()
	if cmd == nil {
		t.Fatal("expected non-nil cmd from IssuePollTick")
	}
}

func TestCheckForNewIssues_NoPollingRepos(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
	}{
		{
			name: "no repos at all",
			cfg: func() *config.Config {
				cfg := testConfig()
				cfg.Repos = []string{}
				return cfg
			}(),
		},
		{
			name: "repos exist but none have polling enabled",
			cfg: func() *config.Config {
				cfg := testConfig()
				// testConfig has repos but no polling enabled by default
				return cfg
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := pexec.NewMockExecutor(nil)
			gitSvc := git.NewGitServiceWithExecutor(mock)

			cmd := checkForNewIssues(tt.cfg, gitSvc, []config.Session{})
			if cmd != nil {
				t.Error("expected nil cmd when no repos have polling enabled")
			}
		})
	}
}

func TestCheckForNewIssues_DeduplicatesExistingSessions(t *testing.T) {
	cfg := testConfig()
	cfg.SetRepoIssuePolling("/test/repo1", true)
	cfg.SetRepoIssueLabels("/test/repo1", "auto")

	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"issue", "list", "--json", "number,title,body,url", "--state", "open", "--label", "auto"}, pexec.MockResponse{
		Stdout: []byte(`[
			{"number": 1, "title": "Issue One", "body": "Body one", "url": "https://github.com/repo/issues/1"},
			{"number": 2, "title": "Issue Two", "body": "Body two", "url": "https://github.com/repo/issues/2"},
			{"number": 3, "title": "Issue Three", "body": "Body three", "url": "https://github.com/repo/issues/3"}
		]`),
	})
	gitSvc := git.NewGitServiceWithExecutor(mock)

	// Existing sessions already cover issues 1 and 3
	existingSessions := []config.Session{
		{
			ID:       "sess-1",
			RepoPath: "/test/repo1",
			IssueRef: &config.IssueRef{
				Source: string(issues.SourceGitHub),
				ID:     "1",
				Title:  "Issue One",
			},
		},
		{
			ID:       "sess-3",
			RepoPath: "/test/repo1",
			IssueRef: &config.IssueRef{
				Source: string(issues.SourceGitHub),
				ID:     "3",
				Title:  "Issue Three",
			},
		},
	}

	cmd := checkForNewIssues(cfg, gitSvc, existingSessions)
	if cmd == nil {
		t.Fatal("expected non-nil cmd when polling repos exist")
	}

	msg := cmd()
	detected, ok := msg.(NewIssuesDetectedMsg)
	if !ok {
		t.Fatalf("expected NewIssuesDetectedMsg, got %T", msg)
	}

	if len(detected.Issues) != 1 {
		t.Fatalf("expected 1 new issue (issue #2), got %d", len(detected.Issues))
	}
	if detected.Issues[0].ID != "2" {
		t.Errorf("expected issue ID '2', got '%s'", detected.Issues[0].ID)
	}
	if detected.Issues[0].Title != "Issue Two" {
		t.Errorf("expected title 'Issue Two', got '%s'", detected.Issues[0].Title)
	}
	if detected.RepoPath != "/test/repo1" {
		t.Errorf("expected repo path '/test/repo1', got '%s'", detected.RepoPath)
	}
}

func TestCheckForNewIssues_RespectsMaxConcurrent(t *testing.T) {
	cfg := testConfig()
	cfg.SetRepoIssuePolling("/test/repo1", true)
	cfg.SetRepoIssueLabels("/test/repo1", "auto")
	cfg.SetIssueMaxConcurrent(2)

	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"issue", "list", "--json", "number,title,body,url", "--state", "open", "--label", "auto"}, pexec.MockResponse{
		Stdout: []byte(`[
			{"number": 10, "title": "Issue Ten", "body": "Body ten", "url": "https://github.com/repo/issues/10"},
			{"number": 11, "title": "Issue Eleven", "body": "Body eleven", "url": "https://github.com/repo/issues/11"},
			{"number": 12, "title": "Issue Twelve", "body": "Body twelve", "url": "https://github.com/repo/issues/12"}
		]`),
	})
	gitSvc := git.NewGitServiceWithExecutor(mock)

	// One existing autonomous session already counts toward the limit
	existingSessions := []config.Session{
		{
			ID:         "existing-auto",
			RepoPath:   "/test/repo1",
			Autonomous: true,
		},
	}

	cmd := checkForNewIssues(cfg, gitSvc, existingSessions)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	msg := cmd()
	detected, ok := msg.(NewIssuesDetectedMsg)
	if !ok {
		t.Fatalf("expected NewIssuesDetectedMsg, got %T", msg)
	}

	// Max concurrent is 2, 1 already active, so only 1 new issue should be returned
	if len(detected.Issues) != 1 {
		t.Fatalf("expected 1 new issue (max concurrent 2, 1 active), got %d", len(detected.Issues))
	}
	if detected.Issues[0].ID != "10" {
		t.Errorf("expected first available issue ID '10', got '%s'", detected.Issues[0].ID)
	}
}
