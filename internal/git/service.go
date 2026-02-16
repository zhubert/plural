package git

import (
	pexec "github.com/zhubert/plural/internal/exec"
)

// GitService provides git operations with explicit dependency injection.
// Instead of using a package-level executor variable, each GitService instance
// holds its own executor, enabling proper testing and avoiding global state.
type GitService struct {
	executor pexec.CommandExecutor
}

// NewGitService creates a new GitService with the default real executor.
func NewGitService() *GitService {
	return &GitService{executor: pexec.NewRealExecutor()}
}

// NewGitServiceWithExecutor creates a new GitService with a custom executor.
// This is primarily used for testing where a mock executor is needed.
func NewGitServiceWithExecutor(exec pexec.CommandExecutor) *GitService {
	return &GitService{executor: exec}
}
