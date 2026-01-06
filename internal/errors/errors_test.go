package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestKind_String(t *testing.T) {
	tests := []struct {
		kind     Kind
		expected string
	}{
		{KindUnknown, "unknown error"},
		{KindNotFound, "not found"},
		{KindInvalid, "invalid"},
		{KindPermission, "permission denied"},
		{KindIO, "I/O error"},
		{KindNetwork, "network error"},
		{KindConfig, "configuration error"},
		{KindGit, "git error"},
		{KindClaude, "claude error"},
		{KindTimeout, "timeout"},
		{Kind(999), "unknown error"}, // Unknown kind
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.kind.String(); got != tt.expected {
				t.Errorf("Kind.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		expected string
	}{
		{
			name:     "with op and context",
			err:      &Error{Op: "test.Op", Context: "some context", Err: errors.New("underlying error")},
			expected: "test.Op: some context: underlying error",
		},
		{
			name:     "with op only",
			err:      &Error{Op: "test.Op", Err: errors.New("underlying error")},
			expected: "test.Op: underlying error",
		},
		{
			name:     "without op",
			err:      &Error{Err: errors.New("underlying error")},
			expected: "underlying error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("Error.Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestError_Unwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	err := &Error{Op: "test.Op", Err: underlying}

	if got := err.Unwrap(); got != underlying {
		t.Errorf("Error.Unwrap() = %v, want %v", got, underlying)
	}
}

func TestE(t *testing.T) {
	tests := []struct {
		name       string
		args       []interface{}
		wantOp     Op
		wantKind   Kind
		wantHasErr bool
	}{
		{
			name:       "with all args",
			args:       []interface{}{Op("test.Op"), KindNotFound, "context", errors.New("error")},
			wantOp:     "test.Op",
			wantKind:   KindNotFound,
			wantHasErr: true,
		},
		{
			name:       "with op and kind",
			args:       []interface{}{Op("test.Op"), KindInvalid, "just a message"},
			wantOp:     "test.Op",
			wantKind:   KindInvalid,
			wantHasErr: true, // Context becomes the error when no error is provided
		},
		{
			name:       "with just error",
			args:       []interface{}{errors.New("simple error")},
			wantOp:     "",
			wantKind:   KindUnknown,
			wantHasErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := E(tt.args...)
			e, ok := err.(*Error)
			if !ok {
				t.Fatalf("E() returned %T, want *Error", err)
			}

			if e.Op != tt.wantOp {
				t.Errorf("E().Op = %q, want %q", e.Op, tt.wantOp)
			}
			if e.Kind != tt.wantKind {
				t.Errorf("E().Kind = %v, want %v", e.Kind, tt.wantKind)
			}
			if (e.Err != nil) != tt.wantHasErr {
				t.Errorf("E().Err nil = %v, want nil = %v", e.Err == nil, !tt.wantHasErr)
			}
		})
	}
}

func TestIs(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		kind     Kind
		expected bool
	}{
		{
			name:     "matching kind",
			err:      E(Op("test"), KindNotFound, "not found"),
			kind:     KindNotFound,
			expected: true,
		},
		{
			name:     "non-matching kind",
			err:      E(Op("test"), KindNotFound, "not found"),
			kind:     KindInvalid,
			expected: false,
		},
		{
			name:     "non-plural error",
			err:      errors.New("regular error"),
			kind:     KindNotFound,
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			kind:     KindNotFound,
			expected: false,
		},
		{
			name:     "wrapped error",
			err:      fmt.Errorf("wrapped: %w", E(Op("test"), KindTimeout, "timeout")),
			kind:     KindTimeout,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Is(tt.err, tt.kind); got != tt.expected {
				t.Errorf("Is() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetKind(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected Kind
	}{
		{
			name:     "plural error",
			err:      E(Op("test"), KindNotFound, "not found"),
			expected: KindNotFound,
		},
		{
			name:     "regular error",
			err:      errors.New("regular error"),
			expected: KindUnknown,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: KindUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetKind(tt.err); got != tt.expected {
				t.Errorf("GetKind() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSessionNotFound(t *testing.T) {
	err := SessionNotFound("test-session-123")

	if !Is(err, KindNotFound) {
		t.Error("SessionNotFound should return KindNotFound error")
	}

	if e, ok := err.(*Error); ok {
		if e.Op != "session.Get" {
			t.Errorf("Op = %q, want %q", e.Op, "session.Get")
		}
	} else {
		t.Error("SessionNotFound should return *Error")
	}
}

func TestSessionCreateFailed(t *testing.T) {
	underlying := errors.New("git worktree failed")
	err := SessionCreateFailed("/path/to/repo", underlying)

	if !Is(err, KindGit) {
		t.Error("SessionCreateFailed should return KindGit error")
	}

	if !errors.Is(err, underlying) {
		t.Error("SessionCreateFailed should wrap the underlying error")
	}
}

func TestConfigLoadFailed(t *testing.T) {
	underlying := errors.New("file not found")
	err := ConfigLoadFailed("/path/to/config", underlying)

	if !Is(err, KindConfig) {
		t.Error("ConfigLoadFailed should return KindConfig error")
	}
}

func TestConfigSaveFailed(t *testing.T) {
	underlying := errors.New("permission denied")
	err := ConfigSaveFailed("/path/to/config", underlying)

	if !Is(err, KindConfig) {
		t.Error("ConfigSaveFailed should return KindConfig error")
	}
}

func TestConfigInvalid(t *testing.T) {
	err := ConfigInvalid("duplicate session ID")

	if !Is(err, KindInvalid) {
		t.Error("ConfigInvalid should return KindInvalid error")
	}
}

func TestGitNotRepo(t *testing.T) {
	err := GitNotRepo("/path/to/dir")

	if !Is(err, KindInvalid) {
		t.Error("GitNotRepo should return KindInvalid error")
	}
}

func TestGitWorktreeFailed(t *testing.T) {
	underlying := errors.New("branch already exists")
	err := GitWorktreeFailed("feature-branch", underlying)

	if !Is(err, KindGit) {
		t.Error("GitWorktreeFailed should return KindGit error")
	}
}

func TestGitMergeFailed(t *testing.T) {
	underlying := errors.New("merge conflict")
	err := GitMergeFailed("feature-branch", underlying)

	if !Is(err, KindGit) {
		t.Error("GitMergeFailed should return KindGit error")
	}
}

func TestClaudeStartFailed(t *testing.T) {
	underlying := errors.New("claude not found in PATH")
	err := ClaudeStartFailed("session-123", underlying)

	if !Is(err, KindClaude) {
		t.Error("ClaudeStartFailed should return KindClaude error")
	}
}

func TestClaudeResponseFailed(t *testing.T) {
	underlying := errors.New("stream closed")
	err := ClaudeResponseFailed("session-123", underlying)

	if !Is(err, KindClaude) {
		t.Error("ClaudeResponseFailed should return KindClaude error")
	}
}

func TestPermissionTimeout(t *testing.T) {
	err := PermissionTimeout("Bash")

	if !Is(err, KindTimeout) {
		t.Error("PermissionTimeout should return KindTimeout error")
	}
}

func TestCLINotFound(t *testing.T) {
	err := CLINotFound("claude")

	if !Is(err, KindNotFound) {
		t.Error("CLINotFound should return KindNotFound error")
	}

	if e, ok := err.(*Error); ok {
		if e.Op != "cli.Check" {
			t.Errorf("Op = %q, want %q", e.Op, "cli.Check")
		}
	}
}

func TestErrorChaining(t *testing.T) {
	// Test that errors can be properly chained and unwrapped
	innerErr := errors.New("original error")
	middleErr := E(Op("middle.Op"), KindIO, innerErr)
	outerErr := E(Op("outer.Op"), KindConfig, middleErr)

	// Should be able to unwrap to find inner error
	if !errors.Is(outerErr, innerErr) {
		t.Error("Should be able to find inner error through chain")
	}

	// Kind should be from the outer error
	if GetKind(outerErr) != KindConfig {
		t.Error("GetKind should return outer error's kind")
	}
}
