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

// =============================================================================
// FetchPRReviewComments Tests
// =============================================================================

func TestFetchPRReviewComments_Success(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "reviews,comments"}, pexec.MockResponse{
		Stdout: []byte(`{
			"reviews": [{
				"author": {"login": "reviewer1"},
				"body": "Overall looks good but needs changes",
				"state": "CHANGES_REQUESTED",
				"comments": [{
					"author": {"login": "reviewer1"},
					"body": "Use a mutex here",
					"path": "internal/app.go",
					"line": 42,
					"url": "https://github.com/repo/pull/1#discussion_r1"
				}]
			}],
			"comments": [{
				"author": {"login": "someone"},
				"body": "What about edge case?",
				"url": "https://github.com/repo/pull/1#issuecomment-1"
			}]
		}`),
	})

	svc := NewGitServiceWithExecutor(mock)
	comments, err := svc.FetchPRReviewComments(context.Background(), "/repo", "feature-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 1 top-level comment + 1 review body + 1 inline = 3
	if len(comments) != 3 {
		t.Fatalf("expected 3 comments, got %d", len(comments))
	}

	// Top-level comment
	if comments[0].Author != "someone" {
		t.Errorf("expected author 'someone', got '%s'", comments[0].Author)
	}
	if comments[0].Body != "What about edge case?" {
		t.Errorf("unexpected body: %s", comments[0].Body)
	}
	if comments[0].Path != "" {
		t.Errorf("expected empty path for top-level comment, got '%s'", comments[0].Path)
	}

	// Review body
	if comments[1].Author != "reviewer1" {
		t.Errorf("expected author 'reviewer1', got '%s'", comments[1].Author)
	}
	if comments[1].Body != "Overall looks good but needs changes" {
		t.Errorf("unexpected review body: %s", comments[1].Body)
	}

	// Inline comment
	if comments[2].Path != "internal/app.go" {
		t.Errorf("expected path 'internal/app.go', got '%s'", comments[2].Path)
	}
	if comments[2].Line != 42 {
		t.Errorf("expected line 42, got %d", comments[2].Line)
	}
	if comments[2].Body != "Use a mutex here" {
		t.Errorf("unexpected inline body: %s", comments[2].Body)
	}
}

func TestFetchPRReviewComments_CLIError(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "reviews,comments"}, pexec.MockResponse{
		Err: fmt.Errorf("no pull requests found for branch feature-branch"),
	})

	svc := NewGitServiceWithExecutor(mock)
	comments, err := svc.FetchPRReviewComments(context.Background(), "/repo", "feature-branch")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if comments != nil {
		t.Errorf("expected nil comments on error, got %v", comments)
	}
}

func TestFetchPRReviewComments_InvalidJSON(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "reviews,comments"}, pexec.MockResponse{
		Stdout: []byte(`not valid json`),
	})

	svc := NewGitServiceWithExecutor(mock)
	comments, err := svc.FetchPRReviewComments(context.Background(), "/repo", "feature-branch")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if comments != nil {
		t.Errorf("expected nil comments on error, got %v", comments)
	}
}

func TestFetchPRReviewComments_EmptyReviews(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "reviews,comments"}, pexec.MockResponse{
		Stdout: []byte(`{"reviews": [], "comments": []}`),
	})

	svc := NewGitServiceWithExecutor(mock)
	comments, err := svc.FetchPRReviewComments(context.Background(), "/repo", "feature-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comments) != 0 {
		t.Errorf("expected 0 comments for empty reviews, got %d", len(comments))
	}
}

func TestFetchPRReviewComments_EmptyReviewBody(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "reviews,comments"}, pexec.MockResponse{
		Stdout: []byte(`{
			"reviews": [{
				"author": {"login": "reviewer"},
				"body": "",
				"state": "APPROVED",
				"comments": []
			}],
			"comments": []
		}`),
	})

	svc := NewGitServiceWithExecutor(mock)
	comments, err := svc.FetchPRReviewComments(context.Background(), "/repo", "feature-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty review body should be skipped
	if len(comments) != 0 {
		t.Errorf("expected 0 comments (empty review body skipped), got %d", len(comments))
	}
}

func TestFetchPRReviewComments_ReviewBodyOnly(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "reviews,comments"}, pexec.MockResponse{
		Stdout: []byte(`{
			"reviews": [{
				"author": {"login": "reviewer"},
				"body": "Please fix the formatting",
				"state": "CHANGES_REQUESTED",
				"comments": []
			}],
			"comments": []
		}`),
	})

	svc := NewGitServiceWithExecutor(mock)
	comments, err := svc.FetchPRReviewComments(context.Background(), "/repo", "feature-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment (review body), got %d", len(comments))
	}
	if comments[0].Author != "reviewer" {
		t.Errorf("expected author 'reviewer', got '%s'", comments[0].Author)
	}
	if comments[0].Body != "Please fix the formatting" {
		t.Errorf("unexpected body: %s", comments[0].Body)
	}
}

// =============================================================================
// GetBatchPRStatesWithComments Tests
// =============================================================================

func TestGetBatchPRStatesWithComments_Success(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "list", "--state", "all", "--json", "state,headRefName,comments,reviews", "--limit", "200"}, pexec.MockResponse{
		Stdout: []byte(`[
			{
				"state": "OPEN",
				"headRefName": "branch-a",
				"comments": [{"body": "comment1"}, {"body": "comment2"}],
				"reviews": [{"body": "review1"}]
			},
			{
				"state": "MERGED",
				"headRefName": "branch-b",
				"comments": [],
				"reviews": []
			}
		]`),
	})

	svc := NewGitServiceWithExecutor(mock)
	results, err := svc.GetBatchPRStatesWithComments(context.Background(), "/repo", []string{"branch-a", "branch-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// branch-a: OPEN, 2 comments + 1 review = 3
	if results["branch-a"].State != PRStateOpen {
		t.Errorf("expected branch-a OPEN, got %s", results["branch-a"].State)
	}
	if results["branch-a"].CommentCount != 3 {
		t.Errorf("expected branch-a CommentCount 3, got %d", results["branch-a"].CommentCount)
	}

	// branch-b: MERGED, 0 comments + 0 reviews = 0
	if results["branch-b"].State != PRStateMerged {
		t.Errorf("expected branch-b MERGED, got %s", results["branch-b"].State)
	}
	if results["branch-b"].CommentCount != 0 {
		t.Errorf("expected branch-b CommentCount 0, got %d", results["branch-b"].CommentCount)
	}
}

func TestGetBatchPRStatesWithComments_NoComments(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "list", "--state", "all", "--json", "state,headRefName,comments,reviews", "--limit", "200"}, pexec.MockResponse{
		Stdout: []byte(`[{"state": "OPEN", "headRefName": "branch-a", "comments": [], "reviews": []}]`),
	})

	svc := NewGitServiceWithExecutor(mock)
	results, err := svc.GetBatchPRStatesWithComments(context.Background(), "/repo", []string{"branch-a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results["branch-a"].CommentCount != 0 {
		t.Errorf("expected CommentCount 0, got %d", results["branch-a"].CommentCount)
	}
}

func TestGetBatchPRStatesWithComments_CLIError(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "list", "--state", "all", "--json", "state,headRefName,comments,reviews", "--limit", "200"}, pexec.MockResponse{
		Err: fmt.Errorf("not a git repository"),
	})

	svc := NewGitServiceWithExecutor(mock)
	results, err := svc.GetBatchPRStatesWithComments(context.Background(), "/repo", []string{"branch-a"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if results != nil {
		t.Errorf("expected nil results on error, got %v", results)
	}
}

func TestGetBatchPRStatesWithComments_InvalidJSON(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "list", "--state", "all", "--json", "state,headRefName,comments,reviews", "--limit", "200"}, pexec.MockResponse{
		Stdout: []byte(`not valid json`),
	})

	svc := NewGitServiceWithExecutor(mock)
	results, err := svc.GetBatchPRStatesWithComments(context.Background(), "/repo", []string{"branch-a"})
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if results != nil {
		t.Errorf("expected nil results on error, got %v", results)
	}
}

func TestGetBatchPRStatesWithComments_MissingBranch(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "list", "--state", "all", "--json", "state,headRefName,comments,reviews", "--limit", "200"}, pexec.MockResponse{
		Stdout: []byte(`[{"state": "OPEN", "headRefName": "other-branch", "comments": [{"body": "c"}], "reviews": []}]`),
	})

	svc := NewGitServiceWithExecutor(mock)
	results, err := svc.GetBatchPRStatesWithComments(context.Background(), "/repo", []string{"my-branch"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for missing branch, got %d", len(results))
	}
}

func TestGetBatchPRStatesWithComments_DraftTreatedAsOpen(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "list", "--state", "all", "--json", "state,headRefName,comments,reviews", "--limit", "200"}, pexec.MockResponse{
		Stdout: []byte(`[{"state": "DRAFT", "headRefName": "draft-branch", "comments": [{"body": "c"}], "reviews": []}]`),
	})

	svc := NewGitServiceWithExecutor(mock)
	results, err := svc.GetBatchPRStatesWithComments(context.Background(), "/repo", []string{"draft-branch"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results["draft-branch"].State != PRStateOpen {
		t.Errorf("expected DRAFT to be treated as OPEN, got %s", results["draft-branch"].State)
	}
	if results["draft-branch"].CommentCount != 1 {
		t.Errorf("expected CommentCount 1, got %d", results["draft-branch"].CommentCount)
	}
}

// =============================================================================
// FetchGitHubIssuesWithLabel Tests
// =============================================================================

func TestFetchGitHubIssuesWithLabel_WithLabel(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"issue", "list", "--json", "number,title,body,url", "--state", "open", "--label", "bug"}, pexec.MockResponse{
		Stdout: []byte(`[{"number":1,"title":"Fix crash","body":"App crashes on startup","url":"https://github.com/repo/issues/1"}]`),
	})

	svc := NewGitServiceWithExecutor(mock)
	issues, err := svc.FetchGitHubIssuesWithLabel(context.Background(), "/repo", "bug")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Number != 1 {
		t.Errorf("expected issue number 1, got %d", issues[0].Number)
	}
	if issues[0].Title != "Fix crash" {
		t.Errorf("expected title 'Fix crash', got '%s'", issues[0].Title)
	}
	if issues[0].Body != "App crashes on startup" {
		t.Errorf("expected body 'App crashes on startup', got '%s'", issues[0].Body)
	}
	if issues[0].URL != "https://github.com/repo/issues/1" {
		t.Errorf("expected URL 'https://github.com/repo/issues/1', got '%s'", issues[0].URL)
	}
}

func TestFetchGitHubIssuesWithLabel_WithoutLabel(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	// When label is empty, no --label flag should be added
	mock.AddExactMatch("gh", []string{"issue", "list", "--json", "number,title,body,url", "--state", "open"}, pexec.MockResponse{
		Stdout: []byte(`[{"number":1,"title":"Issue 1","body":"","url":"https://github.com/repo/issues/1"},{"number":2,"title":"Issue 2","body":"","url":"https://github.com/repo/issues/2"}]`),
	})

	svc := NewGitServiceWithExecutor(mock)
	issues, err := svc.FetchGitHubIssuesWithLabel(context.Background(), "/repo", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
}

func TestFetchGitHubIssuesWithLabel_CLIError(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"issue", "list", "--json", "number,title,body,url", "--state", "open", "--label", "bug"}, pexec.MockResponse{
		Err: fmt.Errorf("not a git repository"),
	})

	svc := NewGitServiceWithExecutor(mock)
	issues, err := svc.FetchGitHubIssuesWithLabel(context.Background(), "/repo", "bug")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if issues != nil {
		t.Errorf("expected nil issues on error, got %v", issues)
	}
}

// =============================================================================
// CheckPRChecks Tests
// =============================================================================

func TestCheckPRChecks_AllPassing(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "checks", "feature-branch", "--json", "state"}, pexec.MockResponse{
		Stdout: []byte(`[{"state":"SUCCESS"},{"state":"SUCCESS"}]`),
	})

	svc := NewGitServiceWithExecutor(mock)
	status, err := svc.CheckPRChecks(context.Background(), "/repo", "feature-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != CIStatusPassing {
		t.Errorf("expected CIStatusPassing, got %s", status)
	}
}

func TestCheckPRChecks_SomeFailing(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	// gh pr checks returns non-zero exit code when checks fail, so we set Err
	mock.AddExactMatch("gh", []string{"pr", "checks", "feature-branch", "--json", "state"}, pexec.MockResponse{
		Stdout: []byte(`[{"state":"SUCCESS"},{"state":"FAILURE"}]`),
		Err:    fmt.Errorf("exit status 1"),
	})

	svc := NewGitServiceWithExecutor(mock)
	status, err := svc.CheckPRChecks(context.Background(), "/repo", "feature-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != CIStatusFailing {
		t.Errorf("expected CIStatusFailing, got %s", status)
	}
}

func TestCheckPRChecks_Pending(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	// gh pr checks returns non-zero when checks are pending
	mock.AddExactMatch("gh", []string{"pr", "checks", "feature-branch", "--json", "state"}, pexec.MockResponse{
		Stdout: []byte(`[{"state":"SUCCESS"},{"state":"PENDING"}]`),
		Err:    fmt.Errorf("exit status 1"),
	})

	svc := NewGitServiceWithExecutor(mock)
	status, err := svc.CheckPRChecks(context.Background(), "/repo", "feature-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != CIStatusPending {
		t.Errorf("expected CIStatusPending, got %s", status)
	}
}

func TestCheckPRChecks_NoChecks(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	// Empty checks array with successful exit code
	mock.AddExactMatch("gh", []string{"pr", "checks", "feature-branch", "--json", "state"}, pexec.MockResponse{
		Stdout: []byte(`[]`),
	})

	svc := NewGitServiceWithExecutor(mock)
	status, err := svc.CheckPRChecks(context.Background(), "/repo", "feature-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != CIStatusNone {
		t.Errorf("expected CIStatusNone, got %s", status)
	}
}

func TestCheckPRChecks_NoChecksWithError(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	// Empty checks array with error exit code
	mock.AddExactMatch("gh", []string{"pr", "checks", "feature-branch", "--json", "state"}, pexec.MockResponse{
		Stdout: []byte(`[]`),
		Err:    fmt.Errorf("exit status 1"),
	})

	svc := NewGitServiceWithExecutor(mock)
	status, err := svc.CheckPRChecks(context.Background(), "/repo", "feature-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != CIStatusNone {
		t.Errorf("expected CIStatusNone, got %s", status)
	}
}

func TestCheckPRChecks_ErrorNoOutput(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	// Error with no stdout (e.g., no PR found)
	mock.AddExactMatch("gh", []string{"pr", "checks", "feature-branch", "--json", "state"}, pexec.MockResponse{
		Err: fmt.Errorf("no pull requests found"),
	})

	svc := NewGitServiceWithExecutor(mock)
	status, err := svc.CheckPRChecks(context.Background(), "/repo", "feature-branch")
	// When there's an error with no output, return the error to prevent
	// infinite polling (instead of silently treating it as pending)
	if err == nil {
		t.Fatal("expected error for empty output with command failure")
	}
	if status != CIStatusPending {
		t.Errorf("expected CIStatusPending as fallback, got %s", status)
	}
}

// =============================================================================
// MergePR Tests
// =============================================================================

func TestMergePR_Success(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "merge", "feature-branch", "--squash", "--delete-branch"}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	svc := NewGitServiceWithExecutor(mock)
	err := svc.MergePR(context.Background(), "/repo", "feature-branch", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMergePR_Error(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "merge", "feature-branch", "--squash", "--delete-branch"}, pexec.MockResponse{
		Err: fmt.Errorf("pull request is not mergeable"),
	})

	svc := NewGitServiceWithExecutor(mock)
	err := svc.MergePR(context.Background(), "/repo", "feature-branch", true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestMergePR_WithoutDeletingBranch(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "merge", "feature-branch", "--squash"}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	svc := NewGitServiceWithExecutor(mock)
	err := svc.MergePR(context.Background(), "/repo", "feature-branch", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
