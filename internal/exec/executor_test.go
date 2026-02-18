package exec

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestRealExecutor_Run(t *testing.T) {
	executor := NewRealExecutor()
	ctx := context.Background()

	// Test running a simple command
	stdout, stderr, err := executor.Run(ctx, "", "echo", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "hello\n" {
		t.Errorf("expected 'hello\\n', got %q", string(stdout))
	}
	if len(stderr) != 0 {
		t.Errorf("expected empty stderr, got %q", string(stderr))
	}
}

func TestRealExecutor_Output(t *testing.T) {
	executor := NewRealExecutor()
	ctx := context.Background()

	output, err := executor.Output(ctx, "", "echo", "world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(output) != "world\n" {
		t.Errorf("expected 'world\\n', got %q", string(output))
	}
}

func TestRealExecutor_CombinedOutput(t *testing.T) {
	executor := NewRealExecutor()
	ctx := context.Background()

	output, err := executor.CombinedOutput(ctx, "", "echo", "combined")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(output) != "combined\n" {
		t.Errorf("expected 'combined\\n', got %q", string(output))
	}
}

func TestMockExecutor_Run(t *testing.T) {
	mock := NewMockExecutor(nil)

	// Add a rule
	mock.AddExactMatch("git", []string{"status"}, MockResponse{
		Stdout: []byte("On branch main"),
		Stderr: nil,
		Err:    nil,
	})

	ctx := context.Background()
	stdout, stderr, err := mock.Run(ctx, "/some/dir", "git", "status")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "On branch main" {
		t.Errorf("expected 'On branch main', got %q", string(stdout))
	}
	if len(stderr) != 0 {
		t.Errorf("expected empty stderr, got %q", string(stderr))
	}

	// Verify call was recorded
	calls := mock.GetCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Dir != "/some/dir" {
		t.Errorf("expected dir '/some/dir', got %q", calls[0].Dir)
	}
	if calls[0].Name != "git" {
		t.Errorf("expected name 'git', got %q", calls[0].Name)
	}
}

func TestMockExecutor_PrefixMatch(t *testing.T) {
	mock := NewMockExecutor(nil)

	// Add a prefix match rule
	mock.AddPrefixMatch("git", []string{"rev-parse"}, MockResponse{
		Stdout: []byte("abc123"),
	})

	ctx := context.Background()

	// Should match "git rev-parse --verify main"
	stdout, _, err := mock.Run(ctx, "", "git", "rev-parse", "--verify", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "abc123" {
		t.Errorf("expected 'abc123', got %q", string(stdout))
	}

	// Should match "git rev-parse HEAD"
	stdout, _, err = mock.Run(ctx, "", "git", "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "abc123" {
		t.Errorf("expected 'abc123', got %q", string(stdout))
	}

	// Should NOT match "git status" (different prefix)
	mock.ClearCalls()
	stdout, _, err = mock.Run(ctx, "", "git", "status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Unmatched commands return empty response
	if string(stdout) != "" {
		t.Errorf("expected empty response for unmatched command, got %q", string(stdout))
	}
}

func TestMockExecutor_Error(t *testing.T) {
	mock := NewMockExecutor(nil)

	expectedErr := errors.New("command failed")
	mock.AddExactMatch("git", []string{"push"}, MockResponse{
		Stdout: nil,
		Stderr: []byte("permission denied"),
		Err:    expectedErr,
	})

	ctx := context.Background()
	_, stderr, err := mock.Run(ctx, "", "git", "push")

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	if string(stderr) != "permission denied" {
		t.Errorf("expected 'permission denied', got %q", string(stderr))
	}
}

func TestMockExecutor_Output(t *testing.T) {
	mock := NewMockExecutor(nil)

	mock.AddExactMatch("echo", []string{"hello"}, MockResponse{
		Stdout: []byte("hello"),
	})

	ctx := context.Background()
	output, err := mock.Output(ctx, "", "echo", "hello")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(output) != "hello" {
		t.Errorf("expected 'hello', got %q", string(output))
	}
}

func TestMockExecutor_CombinedOutput(t *testing.T) {
	mock := NewMockExecutor(nil)

	mock.AddExactMatch("cmd", []string{"test"}, MockResponse{
		Stdout: []byte("out"),
		Stderr: []byte("err"),
	})

	ctx := context.Background()
	output, err := mock.CombinedOutput(ctx, "", "cmd", "test")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(output) != "outerr" {
		t.Errorf("expected 'outerr', got %q", string(output))
	}
}

func TestMockExecutor_Start(t *testing.T) {
	mock := NewMockExecutor(nil)

	mock.AddExactMatch("git", []string{"clone"}, MockResponse{
		Stdout: []byte("Cloning into 'repo'..."),
		Stderr: nil,
	})

	ctx := context.Background()
	handle, err := mock.Start(ctx, "", "git", "clone")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stdout, stderr, err := handle.Wait()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "Cloning into 'repo'..." {
		t.Errorf("expected 'Cloning into 'repo'...', got %q", string(stdout))
	}
	if len(stderr) != 0 {
		t.Errorf("expected empty stderr, got %q", string(stderr))
	}
}

func TestMockExecutor_Fallback(t *testing.T) {
	real := NewRealExecutor()
	mock := NewMockExecutor(real)

	// Only mock "git" commands
	mock.AddPrefixMatch("git", []string{}, MockResponse{
		Stdout: []byte("mocked"),
	})

	ctx := context.Background()

	// "git status" should use mock
	stdout, _, err := mock.Run(ctx, "", "git", "status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "mocked" {
		t.Errorf("expected 'mocked', got %q", string(stdout))
	}

	// "echo hello" should fall through to real executor
	stdout, _, err = mock.Run(ctx, "", "echo", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "hello\n" {
		t.Errorf("expected 'hello\\n', got %q", string(stdout))
	}
}

func TestMockExecutor_AddRule(t *testing.T) {
	mock := NewMockExecutor(nil)

	// Add a custom matching rule
	mock.AddRule(func(dir, name string, args []string) bool {
		return dir == "/special/dir"
	}, MockResponse{
		Stdout: []byte("special response"),
	})

	ctx := context.Background()

	// Match based on directory
	stdout, _, err := mock.Run(ctx, "/special/dir", "any", "command")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "special response" {
		t.Errorf("expected 'special response', got %q", string(stdout))
	}

	// Different directory shouldn't match
	stdout, _, err = mock.Run(ctx, "/other/dir", "any", "command")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(stdout) != "" {
		t.Errorf("expected empty response, got %q", string(stdout))
	}
}

func TestMockExecutor_GetCallsClearCalls(t *testing.T) {
	mock := NewMockExecutor(nil)
	ctx := context.Background()

	mock.Run(ctx, "/dir1", "cmd1", "arg1")
	mock.Run(ctx, "/dir2", "cmd2", "arg2")

	calls := mock.GetCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}

	mock.ClearCalls()

	calls = mock.GetCalls()
	if len(calls) != 0 {
		t.Errorf("expected 0 calls after clear, got %d", len(calls))
	}
}

func TestMockExecutor_RuleOrder(t *testing.T) {
	mock := NewMockExecutor(nil)

	// Add a specific rule first
	mock.AddExactMatch("git", []string{"push", "origin", "main"}, MockResponse{
		Stdout: []byte("specific"),
	})

	// Add a more general rule second
	mock.AddPrefixMatch("git", []string{"push"}, MockResponse{
		Stdout: []byte("general"),
	})

	ctx := context.Background()

	// Specific match should win (first added)
	stdout, _, _ := mock.Run(ctx, "", "git", "push", "origin", "main")
	if string(stdout) != "specific" {
		t.Errorf("expected 'specific', got %q", string(stdout))
	}

	// General match for other push commands
	stdout, _, _ = mock.Run(ctx, "", "git", "push", "origin", "feature")
	if string(stdout) != "general" {
		t.Errorf("expected 'general', got %q", string(stdout))
	}
}

func TestDefaultExecutor(t *testing.T) {
	// Verify DefaultExecutor is set
	if GetDefaultExecutor() == nil {
		t.Fatal("DefaultExecutor should not be nil")
	}

	// Verify it's a RealExecutor
	if _, ok := GetDefaultExecutor().(*RealExecutor); !ok {
		t.Errorf("DefaultExecutor should be *RealExecutor, got %T", GetDefaultExecutor())
	}

	// Test SetDefaultExecutor
	mock := NewMockExecutor(nil)
	originalExecutor := GetDefaultExecutor()

	SetDefaultExecutor(mock)
	if GetDefaultExecutor() != mock {
		t.Error("SetDefaultExecutor did not set the executor")
	}

	// Restore original
	SetDefaultExecutor(originalExecutor)
}

func TestDefaultExecutorConcurrentAccess(t *testing.T) {
	original := GetDefaultExecutor()
	defer SetDefaultExecutor(original)

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			SetDefaultExecutor(NewMockExecutor(nil))
		}()
		go func() {
			defer wg.Done()
			_ = GetDefaultExecutor()
		}()
	}
	wg.Wait()
}

func TestMockCommandHandle_PipeIdempotent(t *testing.T) {
	mock := NewMockExecutor(nil)
	mock.AddExactMatch("test", []string{}, MockResponse{
		Stdout: []byte("hello"),
		Stderr: []byte("err"),
	})

	ctx := context.Background()
	handle, err := mock.Start(ctx, "", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Call StdoutPipe multiple times - should return same data, not duplicate
	pipe1 := handle.StdoutPipe()
	data1 := pipe1.String()
	if data1 != "hello" {
		t.Errorf("first StdoutPipe call: expected %q, got %q", "hello", data1)
	}

	pipe2 := handle.StdoutPipe()
	data2 := pipe2.String()
	if data2 != "hello" {
		t.Errorf("second StdoutPipe call: expected %q, got %q (data should not duplicate)", "hello", data2)
	}

	// Same for StderrPipe
	errPipe1 := handle.StderrPipe()
	errData1 := errPipe1.String()
	if errData1 != "err" {
		t.Errorf("first StderrPipe call: expected %q, got %q", "err", errData1)
	}

	errPipe2 := handle.StderrPipe()
	errData2 := errPipe2.String()
	if errData2 != "err" {
		t.Errorf("second StderrPipe call: expected %q, got %q (data should not duplicate)", "err", errData2)
	}
}
