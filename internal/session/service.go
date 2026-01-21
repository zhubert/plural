package session

import (
	pexec "github.com/zhubert/plural/internal/exec"
)

// SessionService provides session operations with explicit dependency injection.
// Instead of using a package-level executor variable, each SessionService instance
// holds its own executor, enabling proper testing and avoiding global state.
type SessionService struct {
	executor pexec.CommandExecutor
}

// NewSessionService creates a new SessionService with the default real executor.
func NewSessionService() *SessionService {
	return &SessionService{executor: pexec.NewRealExecutor()}
}

// NewSessionServiceWithExecutor creates a new SessionService with a custom executor.
// This is primarily used for testing and demo generation where a mock
// executor is needed.
func NewSessionServiceWithExecutor(exec pexec.CommandExecutor) *SessionService {
	return &SessionService{executor: exec}
}
