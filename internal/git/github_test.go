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

func TestGetPRState_UnknownState(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "state"}, pexec.MockResponse{
		Stdout: []byte(`{"state":"DRAFT"}`),
	})

	svc := NewGitServiceWithExecutor(mock)
	state, err := svc.GetPRState(context.Background(), "/repo", "feature-branch")
	if err == nil {
		t.Fatal("expected error for unknown state, got nil")
	}
	if state != PRStateUnknown {
		t.Errorf("expected unknown state, got %s", state)
	}
}
