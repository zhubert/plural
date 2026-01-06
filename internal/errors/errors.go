// Package errors provides structured error types for the Plural application.
// These errors provide context about what operation failed and where.
package errors

import (
	"errors"
	"fmt"
)

// Op describes an operation, usually as "package.function".
type Op string

// Kind categorizes the type of error.
type Kind int

const (
	KindUnknown Kind = iota
	KindNotFound
	KindInvalid
	KindPermission
	KindIO
	KindNetwork
	KindConfig
	KindGit
	KindClaude
	KindTimeout
)

func (k Kind) String() string {
	switch k {
	case KindNotFound:
		return "not found"
	case KindInvalid:
		return "invalid"
	case KindPermission:
		return "permission denied"
	case KindIO:
		return "I/O error"
	case KindNetwork:
		return "network error"
	case KindConfig:
		return "configuration error"
	case KindGit:
		return "git error"
	case KindClaude:
		return "claude error"
	case KindTimeout:
		return "timeout"
	default:
		return "unknown error"
	}
}

// Error is the structured error type for Plural.
type Error struct {
	Op      Op     // Operation that failed
	Kind    Kind   // Category of error
	Err     error  // Underlying error
	Context string // Additional context
}

// Error returns the error message.
func (e *Error) Error() string {
	if e.Context != "" {
		return fmt.Sprintf("%s: %s: %s", e.Op, e.Context, e.Err)
	}
	if e.Op != "" {
		return fmt.Sprintf("%s: %s", e.Op, e.Err)
	}
	return e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *Error) Unwrap() error {
	return e.Err
}

// E creates a new Error. Arguments can be:
// - Op: the operation name
// - Kind: the error kind
// - string: context message
// - error: the underlying error
func E(args ...interface{}) error {
	e := &Error{}
	for _, arg := range args {
		switch a := arg.(type) {
		case Op:
			e.Op = a
		case Kind:
			e.Kind = a
		case string:
			e.Context = a
		case error:
			e.Err = a
		}
	}
	if e.Err == nil {
		e.Err = errors.New(e.Context)
		e.Context = ""
	}
	return e
}

// Is reports whether err is of the given Kind.
func Is(err error, kind Kind) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind == kind
	}
	return false
}

// GetKind returns the Kind of an error.
func GetKind(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindUnknown
}

// Session errors
func SessionNotFound(id string) error {
	return E(Op("session.Get"), KindNotFound, fmt.Sprintf("session %s not found", id))
}

func SessionCreateFailed(repoPath string, err error) error {
	return E(Op("session.Create"), KindGit, fmt.Sprintf("failed to create session for %s", repoPath), err)
}

// Config errors
func ConfigLoadFailed(path string, err error) error {
	return E(Op("config.Load"), KindConfig, fmt.Sprintf("failed to load config from %s", path), err)
}

func ConfigSaveFailed(path string, err error) error {
	return E(Op("config.Save"), KindConfig, fmt.Sprintf("failed to save config to %s", path), err)
}

func ConfigInvalid(reason string) error {
	return E(Op("config.Validate"), KindInvalid, reason)
}

// Git errors
func GitNotRepo(path string) error {
	return E(Op("git.ValidateRepo"), KindInvalid, fmt.Sprintf("%s is not a git repository", path))
}

func GitWorktreeFailed(branch string, err error) error {
	return E(Op("git.CreateWorktree"), KindGit, fmt.Sprintf("failed to create worktree for branch %s", branch), err)
}

func GitMergeFailed(branch string, err error) error {
	return E(Op("git.Merge"), KindGit, fmt.Sprintf("failed to merge branch %s", branch), err)
}

// Claude errors
func ClaudeStartFailed(sessionID string, err error) error {
	return E(Op("claude.Start"), KindClaude, fmt.Sprintf("failed to start claude for session %s", sessionID), err)
}

func ClaudeResponseFailed(sessionID string, err error) error {
	return E(Op("claude.Response"), KindClaude, fmt.Sprintf("failed to get response for session %s", sessionID), err)
}

// Permission errors
func PermissionTimeout(tool string) error {
	return E(Op("permission.Wait"), KindTimeout, fmt.Sprintf("timeout waiting for permission response for tool %s", tool))
}

// CLI prerequisite errors
func CLINotFound(name string) error {
	return E(Op("cli.Check"), KindNotFound, fmt.Sprintf("required CLI tool '%s' not found in PATH", name))
}
