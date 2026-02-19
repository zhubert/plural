package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/exec"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/workflow"
)

var errGHFailed = fmt.Errorf("gh: command failed")

func TestCommentIssueAction_Execute_WorkItemNotFound(t *testing.T) {
	cfg := testConfig()
	d := testDaemon(cfg)

	action := &CommentIssueAction{daemon: d}
	params := workflow.NewParamHelper(map[string]any{"body": "Hello!"})
	ac := &workflow.ActionContext{
		WorkItemID: "nonexistent",
		Params:     params,
	}

	result := action.Execute(context.Background(), ac)

	if result.Success {
		t.Error("expected failure for missing work item")
	}
	if result.Error == nil {
		t.Error("expected error for missing work item")
	}
}

func TestCommentIssueAction_Execute_NonGitHubIssue(t *testing.T) {
	cfg := testConfig()
	d := testDaemon(cfg)

	// Add a work item with Asana source — should succeed (no-op)
	d.state.AddWorkItem(&WorkItem{
		ID:       "item-1",
		IssueRef: config.IssueRef{Source: "asana", ID: "task-abc"},
	})

	action := &CommentIssueAction{daemon: d}
	params := workflow.NewParamHelper(map[string]any{"body": "Hello!"})
	ac := &workflow.ActionContext{
		WorkItemID: "item-1",
		Params:     params,
	}

	result := action.Execute(context.Background(), ac)

	if !result.Success {
		t.Errorf("expected success (no-op) for non-github issue, got error: %v", result.Error)
	}
}

func TestCommentIssueAction_Execute_InvalidIssueNumber(t *testing.T) {
	cfg := testConfig()
	d := testDaemon(cfg)

	// GitHub issue with non-numeric ID (invalid)
	d.state.AddWorkItem(&WorkItem{
		ID:       "item-1",
		IssueRef: config.IssueRef{Source: "github", ID: "not-a-number"},
	})

	action := &CommentIssueAction{daemon: d}
	params := workflow.NewParamHelper(map[string]any{"body": "Hello!"})
	ac := &workflow.ActionContext{
		WorkItemID: "item-1",
		Params:     params,
	}

	result := action.Execute(context.Background(), ac)

	if result.Success {
		t.Error("expected failure for invalid issue number")
	}
	if result.Error == nil {
		t.Error("expected error for invalid issue number")
	}
}

func TestCommentIssueAction_Execute_Success(t *testing.T) {
	cfg := testConfig()
	mockExec := exec.NewMockExecutor(nil)

	// Mock `gh issue comment` to succeed
	mockExec.AddPrefixMatch("gh", []string{"issue", "comment"}, exec.MockResponse{
		Stdout: []byte("https://github.com/owner/repo/issues/42#issuecomment-1\n"),
	})

	gitSvc := git.NewGitServiceWithExecutor(mockExec)
	d := testDaemonWithExec(cfg, mockExec)
	d.gitService = gitSvc
	d.repoFilter = "/test/repo"
	cfg.Repos = []string{"/test/repo"}

	// Add session so repo path can be resolved
	sess := testSession("sess-1")
	cfg.AddSession(*sess)

	d.state.AddWorkItem(&WorkItem{
		ID:        "item-1",
		IssueRef:  config.IssueRef{Source: "github", ID: "42"},
		SessionID: "sess-1",
	})

	action := &CommentIssueAction{daemon: d}
	params := workflow.NewParamHelper(map[string]any{"body": "Work has started on this issue."})
	ac := &workflow.ActionContext{
		WorkItemID: "item-1",
		Params:     params,
	}

	result := action.Execute(context.Background(), ac)

	if !result.Success {
		t.Errorf("expected success, got error: %v", result.Error)
	}

	// Verify gh issue comment was called
	calls := mockExec.GetCalls()
	found := false
	for _, c := range calls {
		if c.Name == "gh" && len(c.Args) >= 2 && c.Args[0] == "issue" && c.Args[1] == "comment" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected gh issue comment to be called")
	}
}

func TestCommentIssueAction_Execute_EmptyBody(t *testing.T) {
	cfg := testConfig()
	d := testDaemon(cfg)
	d.repoFilter = "/test/repo"

	sess := testSession("sess-1")
	cfg.AddSession(*sess)

	d.state.AddWorkItem(&WorkItem{
		ID:        "item-1",
		IssueRef:  config.IssueRef{Source: "github", ID: "42"},
		SessionID: "sess-1",
	})

	action := &CommentIssueAction{daemon: d}
	// Empty body — should fail
	params := workflow.NewParamHelper(map[string]any{"body": ""})
	ac := &workflow.ActionContext{
		WorkItemID: "item-1",
		Params:     params,
	}

	result := action.Execute(context.Background(), ac)

	if result.Success {
		t.Error("expected failure for empty comment body")
	}
	if result.Error == nil {
		t.Error("expected error for empty comment body")
	}
}

func TestCommentIssueAction_Execute_GhError(t *testing.T) {
	cfg := testConfig()
	mockExec := exec.NewMockExecutor(nil)

	// Mock `gh issue comment` to fail
	mockExec.AddPrefixMatch("gh", []string{"issue", "comment"}, exec.MockResponse{
		Err: errGHFailed,
	})

	gitSvc := git.NewGitServiceWithExecutor(mockExec)
	d := testDaemonWithExec(cfg, mockExec)
	d.gitService = gitSvc
	d.repoFilter = "/test/repo"

	sess := testSession("sess-1")
	cfg.AddSession(*sess)

	d.state.AddWorkItem(&WorkItem{
		ID:        "item-1",
		IssueRef:  config.IssueRef{Source: "github", ID: "42"},
		SessionID: "sess-1",
	})

	action := &CommentIssueAction{daemon: d}
	params := workflow.NewParamHelper(map[string]any{"body": "Starting work!"})
	ac := &workflow.ActionContext{
		WorkItemID: "item-1",
		Params:     params,
	}

	result := action.Execute(context.Background(), ac)

	if result.Success {
		t.Error("expected failure when gh CLI fails")
	}
	if result.Error == nil {
		t.Error("expected error when gh CLI fails")
	}
}
