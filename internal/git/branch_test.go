package git

import (
	"context"
	"testing"

	pexec "github.com/zhubert/plural/internal/exec"
)

func TestSanitizeBranchName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single word",
			input:    "feature",
			expected: "feature",
		},
		{
			name:     "multi-word with spaces",
			input:    "add new feature",
			expected: "add-new-feature",
		},
		{
			name:     "special characters",
			input:    "fix: bug #123",
			expected: "fix-bug-123",
		},
		{
			name:     "underscores replaced",
			input:    "my_branch_name",
			expected: "my-branch-name",
		},
		{
			name:     "uppercase converted",
			input:    "Fix-Login-Bug",
			expected: "fix-login-bug",
		},
		{
			name:     "consecutive hyphens collapsed",
			input:    "fix--this---bug",
			expected: "fix-this-bug",
		},
		{
			name:     "leading hyphens removed",
			input:    "--leading-hyphen",
			expected: "leading-hyphen",
		},
		{
			name:     "trailing hyphens removed",
			input:    "trailing-hyphen--",
			expected: "trailing-hyphen",
		},
		{
			name:     "leading and trailing hyphens removed",
			input:    "-both-sides-",
			expected: "both-sides",
		},
		{
			name:     "very long name truncated to 50 chars",
			input:    "this-is-a-very-long-branch-name-that-exceeds-the-maximum-allowed-length-for-branches",
			expected: "this-is-a-very-long-branch-name-that-exceeds-the-m",
		},
		{
			name:     "truncation removes trailing hyphen",
			input:    "abcdefghij-abcdefghij-abcdefghij-abcdefghij-abcde-ghij",
			expected: "abcdefghij-abcdefghij-abcdefghij-abcdefghij-abcde",
		},
		{
			name:     "already valid",
			input:    "add-dark-mode",
			expected: "add-dark-mode",
		},
		{
			name:     "mixed special characters stripped",
			input:    "feat/add @new! feature",
			expected: "featadd-new-feature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeBranchName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeBranchName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGenerateBranchNamesFromOptions_MultiWordNames(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)

	// Simulate Claude returning multi-word branch names
	claudeOutput := "1. add new feature\n2. fix login bug\n3. simple-name\n"
	mock.AddPrefixMatch("claude", []string{"--print"}, pexec.MockResponse{
		Stdout: []byte(claudeOutput),
	})

	svc := NewGitServiceWithExecutor(mock)
	options := []struct {
		Number int
		Text   string
	}{
		{1, "Add a new feature to the app"},
		{2, "Fix the login bug"},
		{3, "Simple name change"},
	}

	result, err := svc.GenerateBranchNamesFromOptions(context.Background(), options)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[int]string{
		1: "add-new-feature",
		2: "fix-login-bug",
		3: "simple-name",
	}

	if len(result) != len(expected) {
		t.Fatalf("expected %d results, got %d: %v", len(expected), len(result), result)
	}

	for num, want := range expected {
		got, ok := result[num]
		if !ok {
			t.Errorf("missing result for option %d", num)
			continue
		}
		if got != want {
			t.Errorf("option %d: got %q, want %q", num, got, want)
		}
	}
}

func TestGenerateBranchNamesFromOptions_ParenDelimiter(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)

	// Simulate Claude returning output with ) delimiter
	claudeOutput := "1) add new feature\n2) fix login bug\n3) simple-name\n"
	mock.AddPrefixMatch("claude", []string{"--print"}, pexec.MockResponse{
		Stdout: []byte(claudeOutput),
	})

	svc := NewGitServiceWithExecutor(mock)
	options := []struct {
		Number int
		Text   string
	}{
		{1, "Add a new feature"},
		{2, "Fix the login bug"},
		{3, "Simple change"},
	}

	result, err := svc.GenerateBranchNamesFromOptions(context.Background(), options)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[int]string{
		1: "add-new-feature",
		2: "fix-login-bug",
		3: "simple-name",
	}

	if len(result) != len(expected) {
		t.Fatalf("expected %d results, got %d: %v", len(expected), len(result), result)
	}

	for num, want := range expected {
		got, ok := result[num]
		if !ok {
			t.Errorf("missing result for option %d", num)
			continue
		}
		if got != want {
			t.Errorf("option %d: got %q, want %q", num, got, want)
		}
	}
}

func TestGenerateBranchNamesFromOptions_SingleWordNames(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)

	claudeOutput := "1. refactor\n2. cleanup\n"
	mock.AddPrefixMatch("claude", []string{"--print"}, pexec.MockResponse{
		Stdout: []byte(claudeOutput),
	})

	svc := NewGitServiceWithExecutor(mock)
	options := []struct {
		Number int
		Text   string
	}{
		{1, "Refactor the code"},
		{2, "Clean up unused files"},
	}

	result, err := svc.GenerateBranchNamesFromOptions(context.Background(), options)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result[1] != "refactor" {
		t.Errorf("option 1: got %q, want %q", result[1], "refactor")
	}
	if result[2] != "cleanup" {
		t.Errorf("option 2: got %q, want %q", result[2], "cleanup")
	}
}

func TestGenerateBranchNamesFromOptions_EmptyOptions(t *testing.T) {
	svc := NewGitService()
	result, err := svc.GenerateBranchNamesFromOptions(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty options, got %v", result)
	}
}

func TestGenerateBranchNamesFromOptions_SkipsInvalidLines(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)

	// Include some preamble text and blank lines that should be skipped
	claudeOutput := "Here are the branch names:\n\n1. add-feature\nnot-a-valid-line\n2. fix-bug\n"
	mock.AddPrefixMatch("claude", []string{"--print"}, pexec.MockResponse{
		Stdout: []byte(claudeOutput),
	})

	svc := NewGitServiceWithExecutor(mock)
	options := []struct {
		Number int
		Text   string
	}{
		{1, "Add a feature"},
		{2, "Fix a bug"},
	}

	result, err := svc.GenerateBranchNamesFromOptions(context.Background(), options)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(result), result)
	}
	if result[1] != "add-feature" {
		t.Errorf("option 1: got %q, want %q", result[1], "add-feature")
	}
	if result[2] != "fix-bug" {
		t.Errorf("option 2: got %q, want %q", result[2], "fix-bug")
	}
}

func TestGenerateBranchNamesFromOptions_NamesWithSpecialChars(t *testing.T) {
	mock := pexec.NewMockExecutor(nil)

	// Claude might return names with special characters that need sanitizing
	claudeOutput := "1. Fix: Bug #123\n2. Add_New_Feature\n"
	mock.AddPrefixMatch("claude", []string{"--print"}, pexec.MockResponse{
		Stdout: []byte(claudeOutput),
	})

	svc := NewGitServiceWithExecutor(mock)
	options := []struct {
		Number int
		Text   string
	}{
		{1, "Fix bug 123"},
		{2, "Add new feature"},
	}

	result, err := svc.GenerateBranchNamesFromOptions(context.Background(), options)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result[1] != "fix-bug-123" {
		t.Errorf("option 1: got %q, want %q", result[1], "fix-bug-123")
	}
	if result[2] != "add-new-feature" {
		t.Errorf("option 2: got %q, want %q", result[2], "add-new-feature")
	}
}
