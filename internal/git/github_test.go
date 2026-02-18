package git

import (
	"context"
	"fmt"
	"strings"
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

func TestFetchPRReviewComments_ApprovedReviewBodySkipped(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "reviews,comments"}, pexec.MockResponse{
		Stdout: []byte(`{
			"reviews": [{
				"author": {"login": "reviewer"},
				"body": "The implementation is solid and well-tested!",
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
	// APPROVED review body should be skipped (not actionable feedback)
	if len(comments) != 0 {
		t.Errorf("expected 0 comments (APPROVED review body skipped), got %d", len(comments))
	}
}

func TestFetchPRReviewComments_ApprovedReviewInlineCommentsKept(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "reviews,comments"}, pexec.MockResponse{
		Stdout: []byte(`{
			"reviews": [{
				"author": {"login": "reviewer"},
				"body": "LGTM with a small nit",
				"state": "APPROVED",
				"comments": [{
					"author": {"login": "reviewer"},
					"body": "Consider renaming this variable",
					"path": "main.go",
					"line": 10,
					"url": "https://github.com/repo/pull/1#discussion_r1"
				}]
			}],
			"comments": []
		}`),
	})

	svc := NewGitServiceWithExecutor(mock)
	comments, err := svc.FetchPRReviewComments(context.Background(), "/repo", "feature-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Body should be skipped (APPROVED), but inline comment should be kept
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment (inline from APPROVED review), got %d", len(comments))
	}
	if comments[0].Path != "main.go" {
		t.Errorf("expected path 'main.go', got '%s'", comments[0].Path)
	}
	if comments[0].Body != "Consider renaming this variable" {
		t.Errorf("unexpected body: %s", comments[0].Body)
	}
}

func TestFetchPRReviewComments_DismissedReviewBodySkipped(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "reviews,comments"}, pexec.MockResponse{
		Stdout: []byte(`{
			"reviews": [{
				"author": {"login": "reviewer"},
				"body": "This was dismissed",
				"state": "DISMISSED",
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
	// DISMISSED review body should be skipped
	if len(comments) != 0 {
		t.Errorf("expected 0 comments (DISMISSED review body skipped), got %d", len(comments))
	}
}

func TestFetchPRReviewComments_MixedReviewStates(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "reviews,comments"}, pexec.MockResponse{
		Stdout: []byte(`{
			"reviews": [
				{
					"author": {"login": "reviewer1"},
					"body": "Looks great!",
					"state": "APPROVED",
					"comments": []
				},
				{
					"author": {"login": "reviewer2"},
					"body": "Please fix the error handling",
					"state": "CHANGES_REQUESTED",
					"comments": [{
						"author": {"login": "reviewer2"},
						"body": "Missing error check here",
						"path": "handler.go",
						"line": 55,
						"url": "https://github.com/repo/pull/1#discussion_r2"
					}]
				}
			],
			"comments": []
		}`),
	})

	svc := NewGitServiceWithExecutor(mock)
	comments, err := svc.FetchPRReviewComments(context.Background(), "/repo", "feature-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// APPROVED body skipped, CHANGES_REQUESTED body + inline = 2
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments (CHANGES_REQUESTED body + inline), got %d", len(comments))
	}
	if comments[0].Body != "Please fix the error handling" {
		t.Errorf("expected CHANGES_REQUESTED body, got: %s", comments[0].Body)
	}
	if comments[1].Path != "handler.go" {
		t.Errorf("expected inline comment path 'handler.go', got '%s'", comments[1].Path)
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
				"reviews": [{"body": "review1", "state": "CHANGES_REQUESTED"}]
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

	// branch-a: OPEN, 2 comments + 1 actionable review (CHANGES_REQUESTED) = 3
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

func TestGetBatchPRStatesWithComments_ApprovedReviewsExcludedFromCount(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "list", "--state", "all", "--json", "state,headRefName,comments,reviews", "--limit", "200"}, pexec.MockResponse{
		Stdout: []byte(`[
			{
				"state": "OPEN",
				"headRefName": "branch-a",
				"comments": [{"body": "comment1"}],
				"reviews": [
					{"body": "LGTM, ship it!", "state": "APPROVED"},
					{"body": "Please fix the formatting", "state": "CHANGES_REQUESTED"},
					{"body": "Old review", "state": "DISMISSED"}
				]
			}
		]`),
	})

	svc := NewGitServiceWithExecutor(mock)
	results, err := svc.GetBatchPRStatesWithComments(context.Background(), "/repo", []string{"branch-a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1 comment + 1 actionable review (CHANGES_REQUESTED) = 2
	// APPROVED and DISMISSED reviews should be excluded
	if results["branch-a"].CommentCount != 2 {
		t.Errorf("expected CommentCount 2 (1 comment + 1 CHANGES_REQUESTED review), got %d", results["branch-a"].CommentCount)
	}
}

func TestGetBatchPRStatesWithComments_AllApprovedReviewsExcluded(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "list", "--state", "all", "--json", "state,headRefName,comments,reviews", "--limit", "200"}, pexec.MockResponse{
		Stdout: []byte(`[
			{
				"state": "OPEN",
				"headRefName": "branch-a",
				"comments": [],
				"reviews": [
					{"body": "Looks great!", "state": "APPROVED"},
					{"body": "Ship it!", "state": "APPROVED"}
				]
			}
		]`),
	})

	svc := NewGitServiceWithExecutor(mock)
	results, err := svc.GetBatchPRStatesWithComments(context.Background(), "/repo", []string{"branch-a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All reviews are APPROVED, so count should be 0
	if results["branch-a"].CommentCount != 0 {
		t.Errorf("expected CommentCount 0 (all reviews APPROVED), got %d", results["branch-a"].CommentCount)
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
	mock.AddExactMatch("gh", []string{"pr", "merge", "feature-branch", "--rebase", "--delete-branch"}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	svc := NewGitServiceWithExecutor(mock)
	err := svc.MergePR(context.Background(), "/repo", "feature-branch", true, "rebase")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMergePR_Error(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "merge", "feature-branch", "--rebase", "--delete-branch"}, pexec.MockResponse{
		Err: fmt.Errorf("exit status 1"),
	})

	svc := NewGitServiceWithExecutor(mock)
	err := svc.MergePR(context.Background(), "/repo", "feature-branch", true, "rebase")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestMergePR_ErrorIncludesStderr(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "merge", "feature-branch", "--rebase", "--delete-branch"}, pexec.MockResponse{
		Stderr: []byte("Pull request #42 is not mergeable: the base branch policy prohibits the merge"),
		Err:    fmt.Errorf("exit status 1"),
	})

	svc := NewGitServiceWithExecutor(mock)
	err := svc.MergePR(context.Background(), "/repo", "feature-branch", true, "rebase")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "prohibits the merge") {
		t.Errorf("expected error to contain stderr message, got: %s", err.Error())
	}
}

func TestMergePR_WithoutDeletingBranch(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "merge", "feature-branch", "--rebase"}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	svc := NewGitServiceWithExecutor(mock)
	err := svc.MergePR(context.Background(), "/repo", "feature-branch", false, "rebase")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMergePR_SquashMethod(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "merge", "feature-branch", "--squash"}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	svc := NewGitServiceWithExecutor(mock)
	err := svc.MergePR(context.Background(), "/repo", "feature-branch", false, "squash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMergePR_MergeMethod(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "merge", "feature-branch", "--merge"}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	svc := NewGitServiceWithExecutor(mock)
	err := svc.MergePR(context.Background(), "/repo", "feature-branch", false, "merge")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMergePR_EmptyMethodDefaultsToRebase(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "merge", "feature-branch", "--rebase"}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	svc := NewGitServiceWithExecutor(mock)
	err := svc.MergePR(context.Background(), "/repo", "feature-branch", false, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckPRReviewDecision(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     ReviewDecision
	}{
		{
			name:     "single approval",
			response: `{"reviews":[{"author":{"login":"alice"},"state":"APPROVED"}]}`,
			want:     ReviewApproved,
		},
		{
			name:     "single changes requested",
			response: `{"reviews":[{"author":{"login":"alice"},"state":"CHANGES_REQUESTED"}]}`,
			want:     ReviewChangesRequested,
		},
		{
			name:     "changes requested then approved (same author)",
			response: `{"reviews":[{"author":{"login":"alice"},"state":"CHANGES_REQUESTED"},{"author":{"login":"alice"},"state":"APPROVED"}]}`,
			want:     ReviewApproved,
		},
		{
			name:     "approved then changes requested (same author)",
			response: `{"reviews":[{"author":{"login":"alice"},"state":"APPROVED"},{"author":{"login":"alice"},"state":"CHANGES_REQUESTED"}]}`,
			want:     ReviewChangesRequested,
		},
		{
			name:     "multiple reviewers: one approves, one requests changes",
			response: `{"reviews":[{"author":{"login":"alice"},"state":"APPROVED"},{"author":{"login":"bob"},"state":"CHANGES_REQUESTED"}]}`,
			want:     ReviewChangesRequested,
		},
		{
			name:     "only COMMENTED and DISMISSED reviews",
			response: `{"reviews":[{"author":{"login":"alice"},"state":"COMMENTED"},{"author":{"login":"bob"},"state":"DISMISSED"}]}`,
			want:     ReviewNone,
		},
		{
			name:     "empty reviews",
			response: `{"reviews":[]}`,
			want:     ReviewNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := pexec.NewMockExecutor(nil)
			mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "reviews"}, pexec.MockResponse{
				Stdout: []byte(tt.response),
			})

			svc := NewGitServiceWithExecutor(mock)
			decision, err := svc.CheckPRReviewDecision(context.Background(), "/repo", "feature-branch")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if decision != tt.want {
				t.Errorf("expected %q, got %q", tt.want, decision)
			}
		})
	}
}

func TestCheckPRReviewDecision_CLIError(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"pr", "view", "feature-branch", "--json", "reviews"}, pexec.MockResponse{
		Err: fmt.Errorf("gh failed"),
	})

	svc := NewGitServiceWithExecutor(mock)
	_, err := svc.CheckPRReviewDecision(context.Background(), "/repo", "feature-branch")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- AddIssueLabel tests ---

func TestAddIssueLabel_Success(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"issue", "edit", "42", "--add-label", "wip"}, pexec.MockResponse{})

	svc := NewGitServiceWithExecutor(mock)
	err := svc.AddIssueLabel(context.Background(), "/repo", 42, "wip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAddIssueLabel_Error(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"issue", "edit", "42", "--add-label", "foo"}, pexec.MockResponse{
		Err: fmt.Errorf("gh failed"),
	})

	svc := NewGitServiceWithExecutor(mock)
	err := svc.AddIssueLabel(context.Background(), "/repo", 42, "foo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "gh issue edit --add-label failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// --- RemoveIssueLabel tests ---

func TestRemoveIssueLabel_Success(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"issue", "edit", "42", "--remove-label", "queued"}, pexec.MockResponse{})

	svc := NewGitServiceWithExecutor(mock)
	err := svc.RemoveIssueLabel(context.Background(), "/repo", 42, "queued")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRemoveIssueLabel_Error(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"issue", "edit", "42", "--remove-label", "foo"}, pexec.MockResponse{
		Err: fmt.Errorf("gh failed"),
	})

	svc := NewGitServiceWithExecutor(mock)
	err := svc.RemoveIssueLabel(context.Background(), "/repo", 42, "foo")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "gh issue edit --remove-label failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// --- CommentOnIssue tests ---

func TestCommentOnIssue_Success(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"issue", "comment", "42", "--body", "Hello world"}, pexec.MockResponse{})

	svc := NewGitServiceWithExecutor(mock)
	err := svc.CommentOnIssue(context.Background(), "/repo", 42, "Hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCommentOnIssue_Error(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddExactMatch("gh", []string{"issue", "comment", "42", "--body", "test"}, pexec.MockResponse{
		Err: fmt.Errorf("gh failed"),
	})

	svc := NewGitServiceWithExecutor(mock)
	err := svc.CommentOnIssue(context.Background(), "/repo", 42, "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "gh issue comment failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestUploadTranscriptToPR_Success(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddPrefixMatch("gh", []string{"pr", "comment", "feature-branch", "--body"}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	svc := NewGitServiceWithExecutor(mock)
	err := svc.UploadTranscriptToPR(context.Background(), "/repo", "feature-branch", "User:\nHello\n\nAssistant:\nHi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the command was called
	calls := mock.GetCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	call := calls[0]
	if call.Name != "gh" {
		t.Errorf("expected 'gh' command, got %q", call.Name)
	}
	// The args should contain "pr", "comment", the branch, and "--body" with wrapped content
	argStr := strings.Join(call.Args, " ")
	if !strings.Contains(argStr, "pr comment feature-branch") {
		t.Errorf("expected PR comment args, got: %s", argStr)
	}
	if !strings.Contains(argStr, "<details>") {
		t.Error("expected <details> block in PR comment body")
	}
	if !strings.Contains(argStr, "Session Transcript") {
		t.Error("expected 'Session Transcript' in PR comment body")
	}
	if !strings.Contains(argStr, "User:") {
		t.Error("expected transcript content in PR comment body")
	}
}

func TestUploadTranscriptToPR_Empty(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	svc := NewGitServiceWithExecutor(mock)

	// Empty transcript should be a no-op (no gh call)
	err := svc.UploadTranscriptToPR(context.Background(), "/repo", "feature-branch", "")
	if err != nil {
		t.Fatalf("expected no error for empty transcript, got: %v", err)
	}
}

func TestUploadTranscriptToPR_Error(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)
	mock.AddPrefixMatch("gh", []string{"pr", "comment"}, pexec.MockResponse{
		Err: fmt.Errorf("gh failed"),
	})

	svc := NewGitServiceWithExecutor(mock)
	err := svc.UploadTranscriptToPR(context.Background(), "/repo", "feature-branch", "some transcript")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "gh pr comment failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}
