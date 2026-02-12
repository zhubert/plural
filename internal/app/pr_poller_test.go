package app

import (
	"testing"

	"github.com/zhubert/plural/internal/config"
	pexec "github.com/zhubert/plural/internal/exec"
	"github.com/zhubert/plural/internal/git"
)

func TestCheckPRStatuses_EligibleSessions(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	gitSvc := git.NewGitServiceWithExecutor(mock)

	sessions := []config.Session{
		{ID: "s1", RepoPath: "/repo1", Branch: "b1", PRCreated: true},                             // eligible
		{ID: "s2", RepoPath: "/repo2", Branch: "b2", PRCreated: true, PRMerged: true},              // already merged, skip
		{ID: "s3", RepoPath: "/repo3", Branch: "b3", PRCreated: true, PRClosed: true},              // already closed, skip
		{ID: "s4", RepoPath: "/repo4", Branch: "b4", PRCreated: true, Merged: true},                // locally merged, skip
		{ID: "s5", RepoPath: "/repo5", Branch: "b5"},                                               // no PR, skip
		{ID: "s6", RepoPath: "/repo6", Branch: "b6", PRCreated: true},                              // eligible
	}

	cmd := checkPRStatuses(sessions, gitSvc)
	if cmd == nil {
		t.Fatal("expected non-nil cmd for sessions with eligible PRs")
	}
}

func TestCheckPRStatuses_NoEligibleSessions(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	gitSvc := git.NewGitServiceWithExecutor(mock)

	sessions := []config.Session{
		{ID: "s1", RepoPath: "/repo", Branch: "b1"},                                // no PR
		{ID: "s2", RepoPath: "/repo", Branch: "b2", PRCreated: true, Merged: true}, // locally merged
		{ID: "s3", RepoPath: "/repo", Branch: "b3", PRCreated: true, PRMerged: true}, // already PR merged
	}

	cmd := checkPRStatuses(sessions, gitSvc)
	if cmd != nil {
		t.Error("expected nil cmd when no eligible sessions exist")
	}
}

func TestCheckPRStatuses_EmptySessions(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	gitSvc := git.NewGitServiceWithExecutor(mock)

	cmd := checkPRStatuses([]config.Session{}, gitSvc)
	if cmd != nil {
		t.Error("expected nil cmd for empty sessions")
	}
}

func TestCheckPRStatuses_AllSkipped(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	gitSvc := git.NewGitServiceWithExecutor(mock)

	sessions := []config.Session{
		{ID: "s1", RepoPath: "/repo", Branch: "b1", PRCreated: true, PRClosed: true},
		{ID: "s2", RepoPath: "/repo", Branch: "b2", PRCreated: true, PRMerged: true},
	}

	cmd := checkPRStatuses(sessions, gitSvc)
	if cmd != nil {
		t.Error("expected nil cmd when all sessions are already resolved")
	}
}
