package git

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/logger"
)

// PRState represents the state of a GitHub pull request
type PRState string

const (
	PRStateOpen    PRState = "OPEN"
	PRStateMerged  PRState = "MERGED"
	PRStateClosed  PRState = "CLOSED"
	PRStateUnknown PRState = ""
)

// GetPRState returns the state of a PR for the given branch using the gh CLI.
// Returns PRStateUnknown and an error if the PR cannot be found or gh fails.
func (s *GitService) GetPRState(ctx context.Context, repoPath, branch string) (PRState, error) {
	output, err := s.executor.Output(ctx, repoPath, "gh", "pr", "view", branch, "--json", "state")
	if err != nil {
		return PRStateUnknown, fmt.Errorf("gh pr view failed: %w", err)
	}

	var result struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return PRStateUnknown, fmt.Errorf("failed to parse PR state: %w", err)
	}

	switch PRState(result.State) {
	case PRStateOpen, PRStateMerged, PRStateClosed:
		return PRState(result.State), nil
	default:
		// Treat unrecognized states (e.g., DRAFT) as OPEN
		return PRStateOpen, nil
	}
}

// GetBatchPRStates returns the PR states for multiple branches in a single gh CLI call.
// It uses `gh pr list --state all` to fetch all PRs for the repo, then matches by branch name.
// Branches without a matching PR are omitted from the result map.
func (s *GitService) GetBatchPRStates(ctx context.Context, repoPath string, branches []string) (map[string]PRState, error) {
	output, err := s.executor.Output(ctx, repoPath, "gh", "pr", "list",
		"--state", "all",
		"--json", "state,headRefName",
		"--limit", "200",
	)
	if err != nil {
		return nil, fmt.Errorf("gh pr list failed: %w", err)
	}

	var prs []struct {
		State       string `json:"state"`
		HeadRefName string `json:"headRefName"`
	}
	if err := json.Unmarshal(output, &prs); err != nil {
		return nil, fmt.Errorf("failed to parse PR list: %w", err)
	}

	// Build a lookup set for the branches we care about
	branchSet := make(map[string]struct{}, len(branches))
	for _, b := range branches {
		branchSet[b] = struct{}{}
	}

	// Match PRs to requested branches
	result := make(map[string]PRState, len(branches))
	for _, pr := range prs {
		if _, ok := branchSet[pr.HeadRefName]; !ok {
			continue
		}
		switch PRState(pr.State) {
		case PRStateOpen, PRStateMerged, PRStateClosed:
			result[pr.HeadRefName] = PRState(pr.State)
		default:
			// Treat unrecognized states (e.g., DRAFT) as OPEN
			result[pr.HeadRefName] = PRStateOpen
		}
	}

	return result, nil
}

// PRBatchResult holds the state and comment count for a PR from a batch query.
type PRBatchResult struct {
	State        PRState
	CommentCount int // len(comments) + len(reviews) from gh pr list
}

// GetBatchPRStatesWithComments returns PR states and comment counts for multiple branches.
// Uses a single `gh pr list` call per repo. The comment count is len(comments) + len(reviews),
// which captures top-level PR comments and review submissions.
func (s *GitService) GetBatchPRStatesWithComments(ctx context.Context, repoPath string, branches []string) (map[string]PRBatchResult, error) {
	output, err := s.executor.Output(ctx, repoPath, "gh", "pr", "list",
		"--state", "all",
		"--json", "state,headRefName,comments,reviews",
		"--limit", "200",
	)
	if err != nil {
		return nil, fmt.Errorf("gh pr list failed: %w", err)
	}

	var prs []struct {
		State       string            `json:"state"`
		HeadRefName string            `json:"headRefName"`
		Comments    []json.RawMessage `json:"comments"`
		Reviews     []json.RawMessage `json:"reviews"`
	}
	if err := json.Unmarshal(output, &prs); err != nil {
		return nil, fmt.Errorf("failed to parse PR list: %w", err)
	}

	// Build a lookup set for the branches we care about
	branchSet := make(map[string]struct{}, len(branches))
	for _, b := range branches {
		branchSet[b] = struct{}{}
	}

	// Match PRs to requested branches
	result := make(map[string]PRBatchResult, len(branches))
	for _, pr := range prs {
		if _, ok := branchSet[pr.HeadRefName]; !ok {
			continue
		}
		var state PRState
		switch PRState(pr.State) {
		case PRStateOpen, PRStateMerged, PRStateClosed:
			state = PRState(pr.State)
		default:
			state = PRStateOpen
		}
		result[pr.HeadRefName] = PRBatchResult{
			State:        state,
			CommentCount: len(pr.Comments) + len(pr.Reviews),
		}
	}

	return result, nil
}

// PRReviewComment represents a single review comment from a GitHub pull request.
// This can be a top-level PR comment, a review body, or an inline code review comment.
type PRReviewComment struct {
	Author string // GitHub username
	Body   string // Comment text
	Path   string // File path (empty for top-level/review body comments)
	Line   int    // Line number (0 for top-level/review body comments)
	URL    string // Permalink
}

// JSON types for gh pr view --json reviews,comments response
type ghPRReviewsResponse struct {
	Reviews  []ghReview  `json:"reviews"`
	Comments []ghComment `json:"comments"`
}

type ghReview struct {
	Author   ghAuthor    `json:"author"`
	Body     string      `json:"body"`
	State    string      `json:"state"`
	Comments []ghComment `json:"comments"`
}

type ghComment struct {
	Author ghAuthor `json:"author"`
	Body   string   `json:"body"`
	Path   string   `json:"path"`
	Line   int      `json:"line"`
	URL    string   `json:"url"`
}

type ghAuthor struct {
	Login string `json:"login"`
}

// FetchPRReviewComments fetches review comments from a pull request using the gh CLI.
// Returns top-level PR comments, review body comments, and inline code review comments
// as a flattened slice. The repoPath is used as the working directory.
func (s *GitService) FetchPRReviewComments(ctx context.Context, repoPath, branch string) ([]PRReviewComment, error) {
	output, err := s.executor.Output(ctx, repoPath, "gh", "pr", "view", branch, "--json", "reviews,comments")
	if err != nil {
		return nil, fmt.Errorf("gh pr view failed: %w", err)
	}

	var response ghPRReviewsResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("failed to parse PR review data: %w", err)
	}

	var comments []PRReviewComment

	// Top-level PR comments
	for _, c := range response.Comments {
		if c.Body == "" {
			continue
		}
		comments = append(comments, PRReviewComment{
			Author: c.Author.Login,
			Body:   c.Body,
			URL:    c.URL,
		})
	}

	// Review-level body comments and inline code review comments
	for _, review := range response.Reviews {
		// Include review body if non-empty (skip empty state-only reviews)
		if review.Body != "" {
			comments = append(comments, PRReviewComment{
				Author: review.Author.Login,
				Body:   review.Body,
			})
		}
		for _, c := range review.Comments {
			if c.Body == "" {
				continue
			}
			comments = append(comments, PRReviewComment{
				Author: c.Author.Login,
				Body:   c.Body,
				Path:   c.Path,
				Line:   c.Line,
				URL:    c.URL,
			})
		}
	}

	return comments, nil
}

// GitHubIssue represents a GitHub issue fetched via the gh CLI
type GitHubIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	URL    string `json:"url"`
}

// FetchGitHubIssues fetches open issues from a GitHub repository using the gh CLI.
// The repoPath is used as the working directory to determine which repo to query.
func (s *GitService) FetchGitHubIssues(ctx context.Context, repoPath string) ([]GitHubIssue, error) {
	output, err := s.executor.Output(ctx, repoPath, "gh", "issue", "list",
		"--json", "number,title,body,url",
		"--state", "open",
	)
	if err != nil {
		return nil, fmt.Errorf("gh issue list failed: %w", err)
	}

	var issues []GitHubIssue
	if err := json.Unmarshal(output, &issues); err != nil {
		return nil, fmt.Errorf("failed to parse issues: %w", err)
	}

	return issues, nil
}

// FetchGitHubIssuesWithLabel fetches open issues with a specific label from a GitHub repository.
func (s *GitService) FetchGitHubIssuesWithLabel(ctx context.Context, repoPath, label string) ([]GitHubIssue, error) {
	args := []string{"issue", "list",
		"--json", "number,title,body,url",
		"--state", "open",
	}
	if label != "" {
		args = append(args, "--label", label)
	}
	output, err := s.executor.Output(ctx, repoPath, "gh", args...)
	if err != nil {
		return nil, fmt.Errorf("gh issue list failed: %w", err)
	}

	var issues []GitHubIssue
	if err := json.Unmarshal(output, &issues); err != nil {
		return nil, fmt.Errorf("failed to parse issues: %w", err)
	}

	return issues, nil
}

// CIStatus represents the overall CI check status for a PR.
type CIStatus string

const (
	CIStatusPassing CIStatus = "passing"
	CIStatusFailing CIStatus = "failing"
	CIStatusPending CIStatus = "pending"
	CIStatusNone    CIStatus = "none" // No checks configured
)

// CheckPRChecks checks the CI status of a PR for the given branch.
// Uses `gh pr checks` which returns exit code 0 if all checks pass.
func (s *GitService) CheckPRChecks(ctx context.Context, repoPath, branch string) (CIStatus, error) {
	output, err := s.executor.Output(ctx, repoPath, "gh", "pr", "checks", branch, "--json", "state")
	if err != nil {
		// gh pr checks returns non-zero if checks fail or are pending
		outputStr := string(output)
		if outputStr != "" {
			// Parse the JSON output to determine status
			var checks []struct {
				State string `json:"state"`
			}
			if jsonErr := json.Unmarshal(output, &checks); jsonErr == nil {
				if len(checks) == 0 {
					return CIStatusNone, nil
				}
				hasFailing := false
				hasPending := false
				for _, c := range checks {
					switch c.State {
					case "FAILURE", "ERROR", "CANCELLED":
						hasFailing = true
					case "PENDING", "QUEUED", "IN_PROGRESS", "WAITING", "REQUESTED":
						hasPending = true
					}
				}
				if hasFailing {
					return CIStatusFailing, nil
				}
				if hasPending {
					return CIStatusPending, nil
				}
			}
		}
		// If output is empty (e.g., network error, no PR found), return the error
		// rather than silently treating it as pending (which could cause infinite polling).
		if outputStr == "" {
			return CIStatusPending, fmt.Errorf("gh pr checks failed with no output: %w", err)
		}
		return CIStatusPending, nil
	}

	// Exit code 0 means all checks pass
	var checks []struct {
		State string `json:"state"`
	}
	if jsonErr := json.Unmarshal(output, &checks); jsonErr == nil && len(checks) == 0 {
		return CIStatusNone, nil
	}
	return CIStatusPassing, nil
}

// MergePR merges a PR for the given branch using squash merge.
// Note: hardcoded to --squash --delete-branch for simplicity. If per-repo merge
// strategy configuration is needed, this should accept a merge strategy parameter.
func (s *GitService) MergePR(ctx context.Context, repoPath, branch string) error {
	_, err := s.executor.Output(ctx, repoPath, "gh", "pr", "merge", branch, "--squash", "--delete-branch")
	if err != nil {
		return fmt.Errorf("gh pr merge failed: %w", err)
	}
	return nil
}

// GeneratePRTitleAndBody uses Claude to generate a PR title and body from the branch changes.
// If issueNumber is provided (non-zero), it will be included as "Fixes #N" in the PR body.
// Deprecated: use GeneratePRTitleAndBodyWithIssueRef for new code.
func (s *GitService) GeneratePRTitleAndBody(ctx context.Context, repoPath, branch string, issueNumber int) (title, body string, err error) {
	// Convert legacy issueNumber to IssueRef for backwards compatibility
	var issueRef *config.IssueRef
	if issueNumber > 0 {
		issueRef = &config.IssueRef{
			Source: "github",
			ID:     fmt.Sprintf("%d", issueNumber),
		}
	}
	return s.GeneratePRTitleAndBodyWithIssueRef(ctx, repoPath, branch, "", issueRef)
}

// GeneratePRTitleAndBodyWithIssueRef uses Claude to generate a PR title and body from the branch changes.
// If issueRef is provided, it will add appropriate link text based on the source:
//   - GitHub: adds "Fixes #{number}" to auto-close the issue
//   - Asana: no auto-close support (Asana doesn't use commit message keywords)
//
// baseBranch is the branch this PR will be compared against (typically the session's BaseBranch or main).
func (s *GitService) GeneratePRTitleAndBodyWithIssueRef(ctx context.Context, repoPath, branch, baseBranch string, issueRef *config.IssueRef) (title, body string, err error) {
	log := logger.WithComponent("git")
	log.Info("generating PR title and body with Claude", "branch", branch, "baseBranch", baseBranch, "issueRef", issueRef)

	// If baseBranch is empty, fall back to default branch
	if baseBranch == "" {
		baseBranch = s.GetDefaultBranch(ctx, repoPath)
		log.Debug("baseBranch empty, using default", "defaultBranch", baseBranch)
	}

	// Get the commit log for this branch
	commitLog, err := s.executor.Output(ctx, repoPath, "git", "log", fmt.Sprintf("%s..%s", baseBranch, branch), "--oneline")
	if err != nil {
		log.Error("failed to get commit log", "error", err, "branch", branch)
		return "", "", fmt.Errorf("failed to get commit log: %w", err)
	}

	// Get the diff from base branch (use --no-ext-diff to ensure output goes to stdout)
	diffOutput, err := s.executor.Output(ctx, repoPath, "git", "diff", "--no-ext-diff", fmt.Sprintf("%s...%s", baseBranch, branch))
	if err != nil {
		log.Error("failed to get diff", "error", err, "branch", branch)
		return "", "", fmt.Errorf("failed to get diff: %w", err)
	}

	fullDiff := string(diffOutput)

	// Truncate diff if too large
	maxDiffSize := MaxDiffSize
	if len(fullDiff) > maxDiffSize {
		fullDiff = fullDiff[:maxDiffSize] + "\n... (diff truncated)"
	}

	// Build the prompt for Claude
	prompt := fmt.Sprintf(`Generate a GitHub pull request title and body for the following changes.

Output format (use exactly this format with the markers):
---TITLE---
Your PR title here in conventional commit format
---BODY---
## Summary
Brief description of what this PR does

## Changes
- Bullet points of key changes

## Test plan
- How to test these changes

Rules:
1. Title MUST follow conventional commit format: <type>[optional scope]: <description>
   - type: feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert
   - scope: optional component/module name in parentheses
   - description: concise summary in imperative mood, lowercase, no period at end
   - Example: "feat(auth): add OAuth2 login support"
   - Example: "fix: prevent race condition in request handling"
   - Keep total title length under 72 characters
2. Body should explain the purpose and changes clearly
3. Include a test plan section
4. Do NOT include any preamble - start directly with ---TITLE---

Commits in this branch:
%s

Diff:
%s`, string(commitLog), fullDiff)

	// Call Claude CLI
	output, err := s.executor.Output(ctx, repoPath, "claude", "--print", "-p", prompt)
	if err != nil {
		log.Error("Claude PR generation failed", "error", err)
		return "", "", fmt.Errorf("failed to generate PR with Claude: %w", err)
	}

	result := strings.TrimSpace(string(output))

	// Parse the output
	titleMarker := "---TITLE---"
	bodyMarker := "---BODY---"

	titleStart := strings.Index(result, titleMarker)
	bodyStart := strings.Index(result, bodyMarker)

	if titleStart == -1 || bodyStart == -1 {
		// Fallback: use first line as title, rest as body
		lines := strings.SplitN(result, "\n", 2)
		title = strings.TrimSpace(lines[0])
		if len(lines) > 1 {
			body = strings.TrimSpace(lines[1])
		}
	} else {
		title = strings.TrimSpace(result[titleStart+len(titleMarker) : bodyStart])
		body = strings.TrimSpace(result[bodyStart+len(bodyMarker):])
	}

	if title == "" {
		return "", "", fmt.Errorf("Claude returned empty PR title")
	}

	// Add issue reference to the body based on source
	if issueRef != nil {
		linkText := GetPRLinkText(issueRef)
		if linkText != "" {
			body = body + linkText
			log.Info("added issue reference", "source", issueRef.Source, "id", issueRef.ID)
		}
	}

	log.Info("generated PR title", "title", title)
	return title, body, nil
}

// GetPRLinkText returns the appropriate text to add to a PR body based on the issue source.
// For GitHub issues: returns "\n\nFixes #123"
// For Asana tasks: returns "" (no auto-close support)
// For unknown sources: returns ""
func GetPRLinkText(issueRef *config.IssueRef) string {
	if issueRef == nil {
		return ""
	}

	switch issueRef.Source {
	case "github":
		return fmt.Sprintf("\n\nFixes #%s", issueRef.ID)
	case "asana":
		// Asana doesn't support auto-closing tasks via commit message keywords.
		// Users can manually link PRs in Asana or use the Asana GitHub integration.
		return ""
	default:
		return ""
	}
}
