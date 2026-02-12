package git

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/logger"
)

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
