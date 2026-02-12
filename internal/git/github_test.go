package git

import (
	"context"
	"fmt"
	"testing"

	pexec "github.com/zhubert/plural/internal/exec"
)

func TestGetPRState_Open(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "state"}, pexec.MockResponse{
		Stdout: []byte(`{"state":"OPEN"}`),
	})

	svc := NewGitServiceWithExecutor(mock)
	state, err := svc.GetPRState(context.Background(), "/repo", "feature-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != PRStateOpen {
		t.Errorf("expected OPEN, got %s", state)
	}
}

func TestGetPRState_Merged(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "state"}, pexec.MockResponse{
		Stdout: []byte(`{"state":"MERGED"}`),
	})

	svc := NewGitServiceWithExecutor(mock)
	state, err := svc.GetPRState(context.Background(), "/repo", "feature-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != PRStateMerged {
		t.Errorf("expected MERGED, got %s", state)
	}
}

func TestGetPRState_Closed(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "state"}, pexec.MockResponse{
		Stdout: []byte(`{"state":"CLOSED"}`),
	})

	svc := NewGitServiceWithExecutor(mock)
	state, err := svc.GetPRState(context.Background(), "/repo", "feature-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != PRStateClosed {
		t.Errorf("expected CLOSED, got %s", state)
	}
}

func TestGetPRState_CLIError(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "state"}, pexec.MockResponse{
		Err: fmt.Errorf("no pull requests found"),
	})

	svc := NewGitServiceWithExecutor(mock)
	state, err := svc.GetPRState(context.Background(), "/repo", "feature-branch")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if state != PRStateUnknown {
		t.Errorf("expected unknown state on error, got %s", state)
	}
}

func TestGetPRState_InvalidJSON(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "state"}, pexec.MockResponse{
		Stdout: []byte(`not valid json`),
	})

	svc := NewGitServiceWithExecutor(mock)
	state, err := svc.GetPRState(context.Background(), "/repo", "feature-branch")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if state != PRStateUnknown {
		t.Errorf("expected unknown state on parse error, got %s", state)
	}
}

func TestGetPRState_DraftTreatedAsOpen(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "state"}, pexec.MockResponse{
		Stdout: []byte(`{"state":"DRAFT"}`),
	})

	svc := NewGitServiceWithExecutor(mock)
	state, err := svc.GetPRState(context.Background(), "/repo", "feature-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != PRStateOpen {
		t.Errorf("expected DRAFT to be treated as OPEN, got %s", state)
	}
}

func TestGetBatchPRStates_MultipleStates(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "list", "--state", "all", "--json", "state,headRefName", "--limit", "200"}, pexec.MockResponse{
		Stdout: []byte(`[
			{"state":"OPEN","headRefName":"branch-a"},
			{"state":"MERGED","headRefName":"branch-b"},
			{"state":"CLOSED","headRefName":"branch-c"},
			{"state":"OPEN","headRefName":"unrelated-branch"}
		]`),
	})

	svc := NewGitServiceWithExecutor(mock)
	states, err := svc.GetBatchPRStates(context.Background(), "/repo", []string{"branch-a", "branch-b", "branch-c"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(states) != 3 {
		t.Fatalf("expected 3 results, got %d", len(states))
	}
	if states["branch-a"] != PRStateOpen {
		t.Errorf("expected branch-a OPEN, got %s", states["branch-a"])
	}
	if states["branch-b"] != PRStateMerged {
		t.Errorf("expected branch-b MERGED, got %s", states["branch-b"])
	}
	if states["branch-c"] != PRStateClosed {
		t.Errorf("expected branch-c CLOSED, got %s", states["branch-c"])
	}
}

func TestGetBatchPRStates_DraftTreatedAsOpen(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "list", "--state", "all", "--json", "state,headRefName", "--limit", "200"}, pexec.MockResponse{
		Stdout: []byte(`[{"state":"DRAFT","headRefName":"draft-branch"}]`),
	})

	svc := NewGitServiceWithExecutor(mock)
	states, err := svc.GetBatchPRStates(context.Background(), "/repo", []string{"draft-branch"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if states["draft-branch"] != PRStateOpen {
		t.Errorf("expected DRAFT to be treated as OPEN, got %s", states["draft-branch"])
	}
}

func TestGetBatchPRStates_MissingBranch(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "list", "--state", "all", "--json", "state,headRefName", "--limit", "200"}, pexec.MockResponse{
		Stdout: []byte(`[{"state":"OPEN","headRefName":"other-branch"}]`),
	})

	svc := NewGitServiceWithExecutor(mock)
	states, err := svc.GetBatchPRStates(context.Background(), "/repo", []string{"my-branch"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("expected 0 results for missing branch, got %d", len(states))
	}
}

func TestGetBatchPRStates_CLIError(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "list", "--state", "all", "--json", "state,headRefName", "--limit", "200"}, pexec.MockResponse{
		Err: fmt.Errorf("not a git repository"),
	})

	svc := NewGitServiceWithExecutor(mock)
	states, err := svc.GetBatchPRStates(context.Background(), "/repo", []string{"branch-a"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if states != nil {
		t.Errorf("expected nil states on error, got %v", states)
	}
}

func TestGetBatchPRStates_InvalidJSON(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "list", "--state", "all", "--json", "state,headRefName", "--limit", "200"}, pexec.MockResponse{
		Stdout: []byte(`not valid json`),
	})

	svc := NewGitServiceWithExecutor(mock)
	states, err := svc.GetBatchPRStates(context.Background(), "/repo", []string{"branch-a"})
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if states != nil {
		t.Errorf("expected nil states on error, got %v", states)
	}
}

func TestGetBatchPRStates_EmptyList(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "list", "--state", "all", "--json", "state,headRefName", "--limit", "200"}, pexec.MockResponse{
		Stdout: []byte(`[]`),
	})

	svc := NewGitServiceWithExecutor(mock)
	states, err := svc.GetBatchPRStates(context.Background(), "/repo", []string{"branch-a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("expected 0 results for empty PR list, got %d", len(states))
	}
}
