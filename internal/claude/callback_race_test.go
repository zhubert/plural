package claude

import (
	"sync"
	"testing"
	"time"
)

// TestHandleProcessExit_ConcurrentStop tests that handleProcessExit doesn't panic
// or deadlock when Stop() closes the channel concurrently.
// Run with -race to verify no data races: go test -race -run TestHandleProcessExit_ConcurrentStop
func TestHandleProcessExit_ConcurrentStop(t *testing.T) {
	runner := New("test-session", "/tmp", false, nil)
	runner.log = testLogger()

	ch := make(chan ResponseChunk, 10)
	runner.mu.Lock()
	runner.responseChan.Setup(ch)
	runner.streaming.Active = true
	runner.streaming.Complete = false
	runner.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		runner.handleProcessExit(nil, "test stderr")
	}()

	go func() {
		defer wg.Done()
		runner.mu.Lock()
		runner.responseChan.Close()
		runner.mu.Unlock()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - no panic or deadlock
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out - possible deadlock")
	}
}

// TestHandleRestartAttempt_ConcurrentStop tests that handleRestartAttempt doesn't panic
// or deadlock when Stop() closes the channel concurrently.
// Run with -race to verify no data races: go test -race -run TestHandleRestartAttempt_ConcurrentStop
func TestHandleRestartAttempt_ConcurrentStop(t *testing.T) {
	runner := New("test-session", "/tmp", false, nil)
	runner.log = testLogger()

	ch := make(chan ResponseChunk, 10)
	runner.mu.Lock()
	runner.responseChan.Setup(ch)
	runner.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		runner.handleRestartAttempt(1)
	}()

	go func() {
		defer wg.Done()
		runner.mu.Lock()
		runner.responseChan.Close()
		runner.mu.Unlock()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - no panic or deadlock
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out - possible deadlock")
	}
}

// TestHandleProcessExit_ConcurrentStop_Repeated runs the concurrent test many times
// to increase the likelihood of exposing ordering issues.
func TestHandleProcessExit_ConcurrentStop_Repeated(t *testing.T) {
	for i := 0; i < 100; i++ {
		runner := New("test-session", "/tmp", false, nil)
		runner.log = testLogger()

		ch := make(chan ResponseChunk, 10)
		runner.mu.Lock()
		runner.responseChan.Setup(ch)
		runner.streaming.Active = true
		runner.streaming.Complete = false
		runner.mu.Unlock()

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			runner.handleProcessExit(nil, "test stderr")
		}()

		go func() {
			defer wg.Done()
			runner.mu.Lock()
			runner.responseChan.Close()
			runner.mu.Unlock()
		}()

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Fatalf("Test timed out on iteration %d", i)
		}
	}
}

// TestHandleRestartAttempt_ConcurrentStop_Repeated runs the concurrent test many times
// to increase the likelihood of exposing ordering issues.
func TestHandleRestartAttempt_ConcurrentStop_Repeated(t *testing.T) {
	for i := 0; i < 100; i++ {
		runner := New("test-session", "/tmp", false, nil)
		runner.log = testLogger()

		ch := make(chan ResponseChunk, 10)
		runner.mu.Lock()
		runner.responseChan.Setup(ch)
		runner.mu.Unlock()

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			runner.handleRestartAttempt(1)
		}()

		go func() {
			defer wg.Done()
			runner.mu.Lock()
			runner.responseChan.Close()
			runner.mu.Unlock()
		}()

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Fatalf("Test timed out on iteration %d", i)
		}
	}
}

// TestHandleProcessExit_AfterStop verifies that handleProcessExit returns false
// when Stop() has already been called.
func TestHandleProcessExit_AfterStop(t *testing.T) {
	runner := New("test-session", "/tmp", false, nil)
	runner.log = testLogger()

	ch := make(chan ResponseChunk, 10)
	runner.mu.Lock()
	runner.responseChan.Setup(ch)
	runner.streaming.Active = true
	runner.stopped = true
	runner.mu.Unlock()

	shouldRestart := runner.handleProcessExit(nil, "test stderr")
	if shouldRestart {
		t.Error("handleProcessExit should return false when stopped=true")
	}
}

// TestHandleProcessExit_ResponseComplete verifies that handleProcessExit
// doesn't restart when the response is already complete.
func TestHandleProcessExit_ResponseComplete(t *testing.T) {
	runner := New("test-session", "/tmp", false, nil)
	runner.log = testLogger()

	ch := make(chan ResponseChunk, 10)
	runner.mu.Lock()
	runner.responseChan.Setup(ch)
	runner.streaming.Active = true
	runner.streaming.Complete = true
	runner.mu.Unlock()

	shouldRestart := runner.handleProcessExit(nil, "test stderr")
	if shouldRestart {
		t.Error("handleProcessExit should return false when response is complete")
	}
}

// TestHandleProcessExit_NormalCase verifies the normal case where
// the process exits unexpectedly: sends Done chunk, closes channel,
// clears streaming.Active, and returns true.
func TestHandleProcessExit_NormalCase(t *testing.T) {
	runner := New("test-session", "/tmp", false, nil)
	runner.log = testLogger()

	ch := make(chan ResponseChunk, 10)
	runner.mu.Lock()
	runner.responseChan.Setup(ch)
	runner.streaming.Active = true
	runner.streaming.Complete = false
	runner.mu.Unlock()

	shouldRestart := runner.handleProcessExit(nil, "test stderr")
	if !shouldRestart {
		t.Error("handleProcessExit should return true when process crashes unexpectedly")
	}

	// Verify Done chunk was sent
	select {
	case chunk := <-ch:
		if !chunk.Done {
			t.Error("expected Done chunk, got non-Done chunk")
		}
	default:
		t.Error("expected Done chunk on channel, but channel was empty")
	}

	// Verify streaming is no longer active and channel is closed
	runner.mu.Lock()
	if runner.streaming.Active {
		t.Error("streaming.Active should be false after handleProcessExit")
	}
	if !runner.responseChan.Closed {
		t.Error("response channel should be closed after handleProcessExit")
	}
	runner.mu.Unlock()
}

// TestHandleRestartAttempt_NormalCase verifies that handleRestartAttempt
// sends a restart notification message.
func TestHandleRestartAttempt_NormalCase(t *testing.T) {
	runner := New("test-session", "/tmp", false, nil)
	runner.log = testLogger()

	ch := make(chan ResponseChunk, 10)
	runner.mu.Lock()
	runner.responseChan.Setup(ch)
	runner.mu.Unlock()

	runner.handleRestartAttempt(1)

	select {
	case chunk := <-ch:
		if chunk.Type != ChunkTypeText {
			t.Errorf("chunk.Type = %q, want %q", chunk.Type, ChunkTypeText)
		}
		expectedContent := "\n[Process crashed, attempting restart 1/3...]\n"
		if chunk.Content != expectedContent {
			t.Errorf("chunk.Content = %q, want %q", chunk.Content, expectedContent)
		}
	case <-time.After(1 * time.Second):
		t.Error("handleRestartAttempt did not send message to channel")
	}
}

// TestHandleProcessExit_ChannelAlreadyClosed verifies that handleProcessExit
// handles the case where the channel was already closed gracefully.
func TestHandleProcessExit_ChannelAlreadyClosed(t *testing.T) {
	runner := New("test-session", "/tmp", false, nil)
	runner.log = testLogger()

	ch := make(chan ResponseChunk, 10)
	runner.mu.Lock()
	runner.responseChan.Setup(ch)
	runner.responseChan.Close() // Pre-close the channel
	runner.streaming.Active = true
	runner.streaming.Complete = false
	runner.mu.Unlock()

	// Should not panic
	shouldRestart := runner.handleProcessExit(nil, "test stderr")
	if !shouldRestart {
		t.Error("handleProcessExit should return true even when channel is already closed")
	}

	runner.mu.Lock()
	if runner.streaming.Active {
		t.Error("streaming.Active should be false after handleProcessExit")
	}
	runner.mu.Unlock()
}
