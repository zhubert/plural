package app

import (
	"fmt"
	"testing"

	"github.com/zhubert/plural/internal/config"
	pexec "github.com/zhubert/plural/internal/exec"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/issues"
	"github.com/zhubert/plural/internal/session"
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
	if detected.Label != "auto" {
		t.Errorf("expected label 'auto', got '%s'", detected.Label)
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

func TestCheckForNewIssues_LabelThreadedThroughMsg(t *testing.T) {
	cfg := testConfig()
	cfg.SetRepoIssuePolling("/test/repo1", true)
	cfg.SetRepoIssueLabels("/test/repo1", "queued")

	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"issue", "list", "--json", "number,title,body,url", "--state", "open", "--label", "queued"}, pexec.MockResponse{
		Stdout: []byte(`[{"number": 5, "title": "Issue Five", "body": "Body five", "url": "https://github.com/repo/issues/5"}]`),
	})
	gitSvc := git.NewGitServiceWithExecutor(mock)

	cmd := checkForNewIssues(cfg, gitSvc, []config.Session{})
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	msg := cmd()
	detected, ok := msg.(NewIssuesDetectedMsg)
	if !ok {
		t.Fatalf("expected NewIssuesDetectedMsg, got %T", msg)
	}

	if detected.Label != "queued" {
		t.Errorf("expected label 'ready', got '%s'", detected.Label)
	}
	if len(detected.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(detected.Issues))
	}
	if detected.Issues[0].ID != "5" {
		t.Errorf("expected issue ID '5', got '%s'", detected.Issues[0].ID)
	}
}

func TestCheckForNewIssues_EmptyLabelThreaded(t *testing.T) {
	cfg := testConfig()
	cfg.SetRepoIssuePolling("/test/repo1", true)
	// No label set — empty string

	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"issue", "list", "--json", "number,title,body,url", "--state", "open"}, pexec.MockResponse{
		Stdout: []byte(`[{"number": 1, "title": "Issue One", "body": "Body", "url": "https://github.com/repo/issues/1"}]`),
	})
	gitSvc := git.NewGitServiceWithExecutor(mock)

	cmd := checkForNewIssues(cfg, gitSvc, []config.Session{})
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}

	msg := cmd()
	detected, ok := msg.(NewIssuesDetectedMsg)
	if !ok {
		t.Fatalf("expected NewIssuesDetectedMsg, got %T", msg)
	}

	if detected.Label != "" {
		t.Errorf("expected empty label, got '%s'", detected.Label)
	}
}

// mockSessionServiceExecutor creates a mock executor with git commands mocked
// for session creation. MockExecutor.Run returns success by default for unmatched
// commands, so we must explicitly fail rev-parse --verify (used by BranchExists
// and Create's base point resolution) while succeeding for worktree add.
func mockSessionServiceExecutor() *pexec.MockExecutor {
	mockExec := pexec.NewMockExecutor(nil)
	// git rev-parse --verify must fail so BranchExists returns false
	// and Create falls back to HEAD as the start point.
	mockExec.AddPrefixMatch("git", []string{"rev-parse", "--verify"}, pexec.MockResponse{
		Err: fmt.Errorf("not a valid ref"),
	})
	// git worktree add must succeed for session.Create
	mockExec.AddPrefixMatch("git", []string{"worktree", "add"}, pexec.MockResponse{
		Stdout: []byte("Preparing worktree\n"),
	})
	return mockExec
}

func TestCreateAutonomousIssueSessions_SelectsWhenSidebarFocused(t *testing.T) {
	cfg := testConfig()
	m, _ := testModelWithMocks(cfg, 120, 40)

	// Inject mock session service so Create doesn't need real git
	mockExec := mockSessionServiceExecutor()
	mockSessionSvc := session.NewSessionServiceWithExecutor(mockExec)
	m.SetSessionService(mockSessionSvc)

	// Ensure focus is on sidebar (default for new model, but be explicit)
	m.focus = FocusSidebar
	m.sidebar.SetFocused(true)
	m.chat.SetFocused(false)

	// Trigger autonomous session creation via handleNewIssuesDetectedMsg
	msg := NewIssuesDetectedMsg{
		RepoPath: "/test/repo1",
		Issues: []issues.Issue{
			{ID: "42", Title: "Test Issue", Body: "Fix the bug", Source: issues.SourceGitHub},
		},
	}
	result, _ := m.handleNewIssuesDetectedMsg(msg)
	m = result.(*Model)

	// Verify a session was created
	sessions := m.config.Sessions
	if len(sessions) == 0 {
		t.Fatal("expected at least one session to be created")
	}

	// Verify the sidebar selected the new session
	selected := m.sidebar.SelectedSession()
	if selected == nil {
		t.Fatal("expected sidebar to have a selected session")
	}
	if selected.ID != sessions[0].ID {
		t.Errorf("expected sidebar to select the created session %q, got %q", sessions[0].ID, selected.ID)
	}

	// Verify the active session was set
	if m.activeSession == nil {
		t.Fatal("expected active session to be set")
	}
	if m.activeSession.ID != sessions[0].ID {
		t.Errorf("expected active session %q, got %q", sessions[0].ID, m.activeSession.ID)
	}

	// Verify focus stayed on sidebar
	if m.focus != FocusSidebar {
		t.Errorf("expected focus to remain on FocusSidebar, got %d", m.focus)
	}
}

func TestCreateAutonomousIssueSessions_NoSelectWhenChatFocused(t *testing.T) {
	cfg := testConfig()
	m, _ := testModelWithMocks(cfg, 120, 40)

	// Inject mock session service so Create doesn't need real git
	mockExec := mockSessionServiceExecutor()
	mockSessionSvc := session.NewSessionServiceWithExecutor(mockExec)
	m.SetSessionService(mockSessionSvc)

	// Set focus to chat panel
	m.focus = FocusChat
	m.sidebar.SetFocused(false)
	m.chat.SetFocused(true)

	// Trigger autonomous session creation via handleNewIssuesDetectedMsg
	msg := NewIssuesDetectedMsg{
		RepoPath: "/test/repo1",
		Issues: []issues.Issue{
			{ID: "42", Title: "Test Issue", Body: "Fix the bug", Source: issues.SourceGitHub},
		},
	}
	result, _ := m.handleNewIssuesDetectedMsg(msg)
	m = result.(*Model)

	// Verify a session was created
	sessions := m.config.Sessions
	if len(sessions) == 0 {
		t.Fatal("expected at least one session to be created")
	}

	// Verify the sidebar did NOT auto-select the session (no active session change)
	if m.activeSession != nil {
		t.Errorf("expected no active session when chat is focused, got %q", m.activeSession.ID)
	}

	// Verify focus stayed on chat
	if m.focus != FocusChat {
		t.Errorf("expected focus to remain on FocusChat, got %d", m.focus)
	}
}
