package issues

import (
	"testing"
)

func TestGitHubProvider_Name(t *testing.T) {
	p := NewGitHubProvider(nil)
	if p.Name() != "GitHub Issues" {
		t.Errorf("expected 'GitHub Issues', got '%s'", p.Name())
	}
}

func TestGitHubProvider_Source(t *testing.T) {
	p := NewGitHubProvider(nil)
	if p.Source() != SourceGitHub {
		t.Errorf("expected SourceGitHub, got '%s'", p.Source())
	}
}

func TestGitHubProvider_IsConfigured(t *testing.T) {
	p := NewGitHubProvider(nil)
	// GitHub is always configured (gh CLI is a prerequisite)
	if !p.IsConfigured("/any/repo") {
		t.Error("expected GitHub to always be configured")
	}
}

func TestGitHubProvider_GenerateBranchName(t *testing.T) {
	p := NewGitHubProvider(nil)

	tests := []struct {
		issue    Issue
		expected string
	}{
		{Issue{ID: "123", Source: SourceGitHub}, "issue-123"},
		{Issue{ID: "1", Source: SourceGitHub}, "issue-1"},
		{Issue{ID: "99999", Source: SourceGitHub}, "issue-99999"},
	}

	for _, tc := range tests {
		result := p.GenerateBranchName(tc.issue)
		if result != tc.expected {
			t.Errorf("GenerateBranchName(%v) = %s, expected %s", tc.issue.ID, result, tc.expected)
		}
	}
}

func TestGitHubProvider_GetPRLinkText(t *testing.T) {
	p := NewGitHubProvider(nil)

	tests := []struct {
		issue    Issue
		expected string
	}{
		{Issue{ID: "123", Source: SourceGitHub}, "Fixes #123"},
		{Issue{ID: "1", Source: SourceGitHub}, "Fixes #1"},
		{Issue{ID: "99999", Source: SourceGitHub}, "Fixes #99999"},
	}

	for _, tc := range tests {
		result := p.GetPRLinkText(tc.issue)
		if result != tc.expected {
			t.Errorf("GetPRLinkText(%v) = %s, expected %s", tc.issue.ID, result, tc.expected)
		}
	}
}

func TestGetIssueNumber(t *testing.T) {
	tests := []struct {
		name     string
		issue    Issue
		expected int
	}{
		{"GitHub issue with valid number", Issue{ID: "123", Source: SourceGitHub}, 123},
		{"GitHub issue with 1", Issue{ID: "1", Source: SourceGitHub}, 1},
		{"Asana task returns 0", Issue{ID: "1234567890123", Source: SourceAsana}, 0},
		{"Invalid number returns 0", Issue{ID: "abc", Source: SourceGitHub}, 0},
		{"Empty ID returns 0", Issue{ID: "", Source: SourceGitHub}, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := GetIssueNumber(tc.issue)
			if result != tc.expected {
				t.Errorf("GetIssueNumber(%v) = %d, expected %d", tc.issue, result, tc.expected)
			}
		})
	}
}
