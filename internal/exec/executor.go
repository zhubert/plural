// Package exec provides an abstraction over command execution for testability.
// It allows production code to use real exec.Command while tests
// can inject mock executors that return pre-recorded responses.
package exec

import (
	"bytes"
	"context"
	"os/exec"
	"sync"
)

// CommandExecutor abstracts command execution for testability.
// Production code uses RealExecutor, while tests use MockExecutor.
type CommandExecutor interface {
	// Run executes a command and returns stdout, stderr, and any error.
	Run(ctx context.Context, dir string, name string, args ...string) (stdout, stderr []byte, err error)

	// Output executes a command and returns stdout, or error with stderr context.
	Output(ctx context.Context, dir string, name string, args ...string) ([]byte, error)

	// CombinedOutput executes a command and returns combined stdout+stderr.
	CombinedOutput(ctx context.Context, dir string, name string, args ...string) ([]byte, error)

	// Start starts a command without waiting for it to complete.
	// Returns a CommandHandle that can be used to wait for completion.
	Start(ctx context.Context, dir string, name string, args ...string) (CommandHandle, error)
}

// CommandHandle represents a running command.
type CommandHandle interface {
	// Wait blocks until the command completes and returns stdout, stderr, error.
	Wait() (stdout, stderr []byte, err error)

	// StdoutPipe returns a reader for stdout.
	StdoutPipe() *bytes.Buffer

	// StderrPipe returns a reader for stderr.
	StderrPipe() *bytes.Buffer
}

// RealExecutor executes commands using os/exec.
type RealExecutor struct{}

// NewRealExecutor returns a new RealExecutor.
func NewRealExecutor() *RealExecutor {
	return &RealExecutor{}
}

// Run executes a command and returns stdout, stderr, and any error.
func (e *RealExecutor) Run(ctx context.Context, dir string, name string, args ...string) (stdout, stderr []byte, err error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err = cmd.Run()
	return stdoutBuf.Bytes(), stderrBuf.Bytes(), err
}

// Output executes a command and returns stdout, or error with stderr context.
func (e *RealExecutor) Output(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.Output()
}

// CombinedOutput executes a command and returns combined stdout+stderr.
func (e *RealExecutor) CombinedOutput(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

// Start starts a command without waiting for it to complete.
func (e *RealExecutor) Start(ctx context.Context, dir string, name string, args ...string) (CommandHandle, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &realCommandHandle{
		cmd:        cmd,
		stdoutPipe: stdoutPipe,
		stderrPipe: stderrPipe,
	}, nil
}

// realCommandHandle wraps a real exec.Cmd.
type realCommandHandle struct {
	cmd        *exec.Cmd
	stdoutPipe interface{ Read([]byte) (int, error) }
	stderrPipe interface{ Read([]byte) (int, error) }
	stdoutBuf  bytes.Buffer
	stderrBuf  bytes.Buffer
}

func (h *realCommandHandle) Wait() (stdout, stderr []byte, err error) {
	// Read all output
	h.stdoutBuf.ReadFrom(h.stdoutPipe)
	h.stderrBuf.ReadFrom(h.stderrPipe)

	err = h.cmd.Wait()
	return h.stdoutBuf.Bytes(), h.stderrBuf.Bytes(), err
}

func (h *realCommandHandle) StdoutPipe() *bytes.Buffer {
	return &h.stdoutBuf
}

func (h *realCommandHandle) StderrPipe() *bytes.Buffer {
	return &h.stderrBuf
}

// MockResponse defines the response for a mocked command.
type MockResponse struct {
	Stdout []byte
	Stderr []byte
	Err    error
}

// CommandMatcher is a function that determines if a command matches.
type CommandMatcher func(dir, name string, args []string) bool

// MockRule defines a matching rule and its response.
type MockRule struct {
	Match    CommandMatcher
	Response MockResponse
}

// MockExecutor returns pre-recorded responses for commands.
// Commands are matched in order of rule registration.
type MockExecutor struct {
	mu       sync.RWMutex
	rules    []MockRule
	calls    []MockCall
	fallback CommandExecutor
}

// MockCall records a command invocation for verification.
type MockCall struct {
	Dir  string
	Name string
	Args []string
}

// NewMockExecutor creates a new MockExecutor.
// If fallback is provided, unmatched commands will be delegated to it.
func NewMockExecutor(fallback CommandExecutor) *MockExecutor {
	return &MockExecutor{
		fallback: fallback,
	}
}

// AddRule adds a matching rule with its response.
func (e *MockExecutor) AddRule(match CommandMatcher, response MockResponse) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, MockRule{Match: match, Response: response})
}

// AddExactMatch adds a rule that matches a specific command exactly.
func (e *MockExecutor) AddExactMatch(name string, args []string, response MockResponse) {
	e.AddRule(func(dir, n string, a []string) bool {
		if n != name {
			return false
		}
		if len(a) != len(args) {
			return false
		}
		for i, arg := range args {
			if a[i] != arg {
				return false
			}
		}
		return true
	}, response)
}

// AddPrefixMatch adds a rule that matches commands starting with specific args.
func (e *MockExecutor) AddPrefixMatch(name string, prefixArgs []string, response MockResponse) {
	e.AddRule(func(dir, n string, a []string) bool {
		if n != name {
			return false
		}
		if len(a) < len(prefixArgs) {
			return false
		}
		for i, arg := range prefixArgs {
			if a[i] != arg {
				return false
			}
		}
		return true
	}, response)
}

// GetCalls returns all recorded command invocations.
func (e *MockExecutor) GetCalls() []MockCall {
	e.mu.RLock()
	defer e.mu.RUnlock()
	calls := make([]MockCall, len(e.calls))
	copy(calls, e.calls)
	return calls
}

// ClearCalls clears the recorded command invocations.
func (e *MockExecutor) ClearCalls() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = nil
}

func (e *MockExecutor) findMatch(dir, name string, args []string) *MockResponse {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, rule := range e.rules {
		if rule.Match(dir, name, args) {
			return &rule.Response
		}
	}
	return nil
}

func (e *MockExecutor) recordCall(dir, name string, args []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, MockCall{Dir: dir, Name: name, Args: args})
}

// Run executes a mocked command.
func (e *MockExecutor) Run(ctx context.Context, dir string, name string, args ...string) (stdout, stderr []byte, err error) {
	e.recordCall(dir, name, args)

	if resp := e.findMatch(dir, name, args); resp != nil {
		return resp.Stdout, resp.Stderr, resp.Err
	}

	if e.fallback != nil {
		return e.fallback.Run(ctx, dir, name, args...)
	}

	// Default: return empty success
	return nil, nil, nil
}

// Output executes a mocked command.
func (e *MockExecutor) Output(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	e.recordCall(dir, name, args)

	if resp := e.findMatch(dir, name, args); resp != nil {
		return resp.Stdout, resp.Err
	}

	if e.fallback != nil {
		return e.fallback.Output(ctx, dir, name, args...)
	}

	return nil, nil
}

// CombinedOutput executes a mocked command.
func (e *MockExecutor) CombinedOutput(ctx context.Context, dir string, name string, args ...string) ([]byte, error) {
	e.recordCall(dir, name, args)

	if resp := e.findMatch(dir, name, args); resp != nil {
		combined := append(resp.Stdout, resp.Stderr...)
		return combined, resp.Err
	}

	if e.fallback != nil {
		return e.fallback.CombinedOutput(ctx, dir, name, args...)
	}

	return nil, nil
}

// Start starts a mocked command (returns immediately with buffered response).
func (e *MockExecutor) Start(ctx context.Context, dir string, name string, args ...string) (CommandHandle, error) {
	e.recordCall(dir, name, args)

	if resp := e.findMatch(dir, name, args); resp != nil {
		return newMockCommandHandle(*resp), nil
	}

	if e.fallback != nil {
		return e.fallback.Start(ctx, dir, name, args...)
	}

	return newMockCommandHandle(MockResponse{}), nil
}

// mockCommandHandle wraps a mock response.
type mockCommandHandle struct {
	response  MockResponse
	stdoutBuf bytes.Buffer
	stderrBuf bytes.Buffer
}

// newMockCommandHandle creates a mock handle with buffers pre-populated once.
func newMockCommandHandle(resp MockResponse) *mockCommandHandle {
	h := &mockCommandHandle{response: resp}
	h.stdoutBuf.Write(resp.Stdout)
	h.stderrBuf.Write(resp.Stderr)
	return h
}

func (h *mockCommandHandle) Wait() (stdout, stderr []byte, err error) {
	return h.response.Stdout, h.response.Stderr, h.response.Err
}

func (h *mockCommandHandle) StdoutPipe() *bytes.Buffer {
	return &h.stdoutBuf
}

func (h *mockCommandHandle) StderrPipe() *bytes.Buffer {
	return &h.stderrBuf
}

// Ensure implementations satisfy the interface.
var _ CommandExecutor = (*RealExecutor)(nil)
var _ CommandExecutor = (*MockExecutor)(nil)
var _ CommandHandle = (*realCommandHandle)(nil)
var _ CommandHandle = (*mockCommandHandle)(nil)

// defaultExecutorMu protects defaultExecutor for concurrent access.
var defaultExecutorMu sync.RWMutex

// defaultExecutor is the global default executor (can be swapped for testing).
var defaultExecutor CommandExecutor = NewRealExecutor()

// GetDefaultExecutor returns the global default executor.
func GetDefaultExecutor() CommandExecutor {
	defaultExecutorMu.RLock()
	defer defaultExecutorMu.RUnlock()
	return defaultExecutor
}

// SetDefaultExecutor sets the global default executor.
func SetDefaultExecutor(e CommandExecutor) {
	defaultExecutorMu.Lock()
	defer defaultExecutorMu.Unlock()
	defaultExecutor = e
}
