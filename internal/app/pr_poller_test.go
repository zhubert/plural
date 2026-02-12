package app

import (
	"testing"

	"github.com/zhubert/plural/internal/config"
	pexec "github.com/zhubert/plural/internal/exec"
	"github.com/zhubert/plural/internal/git"
)

func TestGetEligibleSessions(t *testing.T) {
	sessions := []config.Session{
		{ID: "s1", RepoPath: "/repo1", Branch: "b1", PRCreated: true},                                // eligible
		{ID: "s2", RepoPath: "/repo2", Branch: "b2", PRCreated: true, PRMerged: true},                 // already merged, skip
		{ID: "s3", RepoPath: "/repo3", Branch: "b3", PRCreated: true, PRClosed: true},                 // already closed, skip
		{ID: "s4", RepoPath: "/repo4", Branch: "b4", PRCreated: true, Merged: true},                   // locally merged, skip
		{ID: "s5", RepoPath: "/repo5", Branch: "b5"},                                                  // no PR, skip
		{ID: "s6", RepoPath: "/repo6", Branch: "b6", PRCreated: true},                                 // eligible
		{ID: "s7", RepoPath: "/repo1", Branch: "b7", PRCreated: true, PRMerged: true, PRClosed: true}, // both flags, skip
	}

	eligible := getEligibleSessions(sessions)
	if len(eligible) != 2 {
		t.Fatalf("expected 2 eligible sessions, got %d", len(eligible))
	}
	if eligible[0].ID != "s1" {
		t.Errorf("expected first eligible session to be s1, got %s", eligible[0].ID)
	}
	if eligible[1].ID != "s6" {
		t.Errorf("expected second eligible session to be s6, got %s", eligible[1].ID)
	}
}

func TestGetEligibleSessions_None(t *testing.T) {
	sessions := []config.Session{
		{ID: "s1", RepoPath: "/repo", Branch: "b1"},                                    // no PR
		{ID: "s2", RepoPath: "/repo", Branch: "b2", PRCreated: true, Merged: true},     // locally merged
		{ID: "s3", RepoPath: "/repo", Branch: "b3", PRCreated: true, PRMerged: true},   // already PR merged
		{ID: "s4", RepoPath: "/repo", Branch: "b4", PRCreated: true, PRClosed: true},   // already PR closed
	}

	eligible := getEligibleSessions(sessions)
	if len(eligible) != 0 {
		t.Errorf("expected 0 eligible sessions, got %d", len(eligible))
	}
}

func TestGetEligibleSessions_Empty(t *testing.T) {
	eligible := getEligibleSessions([]config.Session{})
	if len(eligible) != 0 {
		t.Errorf("expected 0 eligible sessions, got %d", len(eligible))
	}
}

func TestCheckPRStatuses_ReturnsNilWhenNoEligible(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	gitSvc := git.NewGitServiceWithExecutor(mock)

	sessions := []config.Session{
		{ID: "s1", RepoPath: "/repo", Branch: "b1"}, // no PR
	}

	cmd := checkPRStatuses(sessions, gitSvc)
	if cmd != nil {
		t.Error("expected nil cmd when no eligible sessions exist")
	}
}

func TestCheckPRStatuses_ReturnsNilForEmpty(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	gitSvc := git.NewGitServiceWithExecutor(mock)

	cmd := checkPRStatuses([]config.Session{}, gitSvc)
	if cmd != nil {
		t.Error("expected nil cmd for empty sessions")
	}
}

func TestCheckPRStatuses_ReturnsCmdWhenEligible(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	gitSvc := git.NewGitServiceWithExecutor(mock)

	sessions := []config.Session{
		{ID: "s1", RepoPath: "/repo1", Branch: "b1", PRCreated: true},
		{ID: "s2", RepoPath: "/repo1", Branch: "b2", PRCreated: true},
	}

	cmd := checkPRStatuses(sessions, gitSvc)
	if cmd == nil {
		t.Fatal("expected non-nil cmd for sessions with eligible PRs")
	}
}

func TestCheckPRStatuses_GroupsByRepo(t *testing.T) {
	// Verify that sessions from the same repo are grouped together
	// by checking that getEligibleSessions preserves repo info
	sessions := []config.Session{
		{ID: "s1", RepoPath: "/repo1", Branch: "b1", PRCreated: true},
		{ID: "s2", RepoPath: "/repo1", Branch: "b2", PRCreated: true},
		{ID: "s3", RepoPath: "/repo2", Branch: "b3", PRCreated: true},
	}

	eligible := getEligibleSessions(sessions)
	if len(eligible) != 3 {
		t.Fatalf("expected 3 eligible sessions, got %d", len(eligible))
	}

	// Group by repo to verify the structure used by checkPRStatuses
	repoSessions := make(map[string][]eligibleSession)
	for _, s := range eligible {
		repoSessions[s.RepoPath] = append(repoSessions[s.RepoPath], s)
	}

	if len(repoSessions) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repoSessions))
	}
	if len(repoSessions["/repo1"]) != 2 {
		t.Errorf("expected 2 sessions for /repo1, got %d", len(repoSessions["/repo1"]))
	}
	if len(repoSessions["/repo2"]) != 1 {
		t.Errorf("expected 1 session for /repo2, got %d", len(repoSessions["/repo2"]))
	}
}
