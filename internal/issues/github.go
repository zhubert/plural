package issues

import (
	"context"
	"fmt"
	"strconv"

	"github.com/zhubert/plural/internal/git"
)

// GitHubProvider implements Provider for GitHub Issues using the gh CLI.
type GitHubProvider struct {
	gitService *git.GitService
}

// NewGitHubProvider creates a new GitHub issue provider.
func NewGitHubProvider(gitService *git.GitService) *GitHubProvider {
	return &GitHubProvider{gitService: gitService}
}

// Name returns the human-readable name of this provider.
func (p *GitHubProvider) Name() string {
	return "GitHub Issues"
}

// Source returns the source type for this provider.
func (p *GitHubProvider) Source() Source {
	return SourceGitHub
}

// FetchIssues retrieves open GitHub issues for the given repository.
// The projectID parameter is ignored for GitHub (uses gh CLI with repoPath).
func (p *GitHubProvider) FetchIssues(ctx context.Context, repoPath, projectID string) ([]Issue, error) {
	ghIssues, err := p.gitService.FetchGitHubIssues(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	issues := make([]Issue, len(ghIssues))
	for i, gh := range ghIssues {
		issues[i] = Issue{
			ID:     strconv.Itoa(gh.Number),
			Title:  gh.Title,
			Body:   gh.Body,
			URL:    gh.URL,
			Source: SourceGitHub,
		}
	}
	return issues, nil
}

// IsConfigured returns true - GitHub is always available via gh CLI.
// The gh CLI is checked as a prerequisite when the app starts.
func (p *GitHubProvider) IsConfigured(repoPath string) bool {
	return true
}

// GenerateBranchName returns a branch name for the given GitHub issue.
// Format: "issue-{number}"
func (p *GitHubProvider) GenerateBranchName(issue Issue) string {
	return fmt.Sprintf("issue-%s", issue.ID)
}

// GetPRLinkText returns the text to add to PR body to link/close the issue.
// Format: "Fixes #{number}"
func (p *GitHubProvider) GetPRLinkText(issue Issue) string {
	return fmt.Sprintf("Fixes #%s", issue.ID)
}

// GetIssueNumber returns the issue number as an int (for backwards compatibility).
// Returns 0 if the ID is not a valid number.
func GetIssueNumber(issue Issue) int {
	if issue.Source != SourceGitHub {
		return 0
	}
	num, err := strconv.Atoi(issue.ID)
	if err != nil {
		return 0
	}
	return num
}
