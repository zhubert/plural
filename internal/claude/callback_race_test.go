package claude

import (
	"sync"
	"testing"
	"time"
)

// TestHandleProcessExit_RaceCondition tests that handleProcessExit doesn't panic
// when the channel is closed between the nil/closed check and the send.
func TestHandleProcessExit_RaceCondition(t *testing.T) {
	runner := New("test-session", "/tmp", false, nil)
	runner.log = testLogger()

	// Initialize response channel
	runner.mu.Lock()
	runner.responseChan.Channel = make(chan ResponseChunk, 10)
	runner.responseChan.Closed = false
	runner.streaming.Active = true
	runner.streaming.Complete = false
	runner.mu.Unlock()

	// Use a WaitGroup to synchronize goroutines
	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: Call handleProcessExit
	go func() {
		defer wg.Done()
		// Add a small delay to increase chance of race
		time.Sleep(1 * time.Millisecond)
		runner.handleProcessExit(nil, "test stderr")
	}()

	// Goroutine 2: Close the channel (simulating Stop())
	go func() {
		defer wg.Done()
		time.Sleep(1 * time.Millisecond)
		runner.mu.Lock()
		if runner.responseChan.Channel != nil && !runner.responseChan.Closed {
			close(runner.responseChan.Channel)
			runner.responseChan.Closed = true
		}
		runner.mu.Unlock()
	}()

	// Wait for both goroutines to complete
	// If there's a panic, the test will fail
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - no panic occurred
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out - possible deadlock")
	}
}

// TestHandleRestartAttempt_RaceCondition tests that handleRestartAttempt doesn't panic
// when the channel is closed between the nil/closed check and the send.
func TestHandleRestartAttempt_RaceCondition(t *testing.T) {
	runner := New("test-session", "/tmp", false, nil)
	runner.log = testLogger()

	// Initialize response channel
	runner.mu.Lock()
	runner.responseChan.Channel = make(chan ResponseChunk, 10)
	runner.responseChan.Closed = false
	runner.mu.Unlock()

	// Use a WaitGroup to synchronize goroutines
	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: Call handleRestartAttempt
	go func() {
		defer wg.Done()
		time.Sleep(1 * time.Millisecond)
		runner.handleRestartAttempt(1)
	}()

	// Goroutine 2: Close the channel (simulating Stop())
	go func() {
		defer wg.Done()
		time.Sleep(1 * time.Millisecond)
		runner.mu.Lock()
		if runner.responseChan.Channel != nil && !runner.responseChan.Closed {
			close(runner.responseChan.Channel)
			runner.responseChan.Closed = true
		}
		runner.mu.Unlock()
	}()

	// Wait for both goroutines to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - no panic occurred
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out - possible deadlock")
	}
}

// TestHandleProcessExit_MultipleRaces runs the race test multiple times
// to increase the likelihood of catching the race condition.
func TestHandleProcessExit_MultipleRaces(t *testing.T) {
	for i := 0; i < 100; i++ {
		runner := New("test-session", "/tmp", false, nil)
		runner.log = testLogger()

		runner.mu.Lock()
		runner.responseChan.Channel = make(chan ResponseChunk, 10)
		runner.responseChan.Closed = false
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
			if runner.responseChan.Channel != nil && !runner.responseChan.Closed {
				close(runner.responseChan.Channel)
				runner.responseChan.Closed = true
			}
			runner.mu.Unlock()
		}()

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatalf("Test timed out on iteration %d", i)
		}
	}
}

// TestHandleRestartAttempt_MultipleRaces runs the race test multiple times
// to increase the likelihood of catching the race condition.
func TestHandleRestartAttempt_MultipleRaces(t *testing.T) {
	for i := 0; i < 100; i++ {
		runner := New("test-session", "/tmp", false, nil)
		runner.log = testLogger()

		runner.mu.Lock()
		runner.responseChan.Channel = make(chan ResponseChunk, 10)
		runner.responseChan.Closed = false
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
			if runner.responseChan.Channel != nil && !runner.responseChan.Closed {
				close(runner.responseChan.Channel)
				runner.responseChan.Closed = true
			}
			runner.mu.Unlock()
		}()

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Success
		case <-time.After(1 * time.Second):
			t.Fatalf("Test timed out on iteration %d", i)
		}
	}
}

// TestHandleProcessExit_AfterStop verifies that handleProcessExit handles
// the case where Stop() has already been called.
func TestHandleProcessExit_AfterStop(t *testing.T) {
	runner := New("test-session", "/tmp", false, nil)
	runner.log = testLogger()

	// Initialize and then stop
	runner.mu.Lock()
	runner.responseChan.Channel = make(chan ResponseChunk, 10)
	runner.responseChan.Closed = false
	runner.streaming.Active = true
	runner.stopped = true // Simulate Stop() having been called
	runner.mu.Unlock()

	// This should return false immediately without attempting to send
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

	runner.mu.Lock()
	runner.responseChan.Channel = make(chan ResponseChunk, 10)
	runner.responseChan.Closed = false
	runner.streaming.Active = true
	runner.streaming.Complete = true // Response already complete
	runner.mu.Unlock()

	// This should return false since response is complete
	shouldRestart := runner.handleProcessExit(nil, "test stderr")
	if shouldRestart {
		t.Error("handleProcessExit should return false when response is complete")
	}
}

// TestHandleProcessExit_NormalCase verifies the normal case where
// the process exits unexpectedly and should restart.
func TestHandleProcessExit_NormalCase(t *testing.T) {
	runner := New("test-session", "/tmp", false, nil)
	runner.log = testLogger()

	runner.mu.Lock()
	runner.responseChan.Channel = make(chan ResponseChunk, 10)
	runner.responseChan.Closed = false
	runner.streaming.Active = true
	runner.streaming.Complete = false
	runner.mu.Unlock()

	// Drain the channel in a goroutine
	go func() {
		for range runner.responseChan.Channel {
			// Consume messages
		}
	}()

	// This should return true to indicate restart should happen
	shouldRestart := runner.handleProcessExit(nil, "test stderr")
	if !shouldRestart {
		t.Error("handleProcessExit should return true when process crashes unexpectedly")
	}

	// Verify streaming is no longer active
	runner.mu.Lock()
	if runner.streaming.Active {
		t.Error("streaming.Active should be false after handleProcessExit")
	}
	runner.mu.Unlock()
}

// TestHandleRestartAttempt_NormalCase verifies that handleRestartAttempt
// successfully sends a restart message.
func TestHandleRestartAttempt_NormalCase(t *testing.T) {
	runner := New("test-session", "/tmp", false, nil)
	runner.log = testLogger()

	runner.mu.Lock()
	runner.responseChan.Channel = make(chan ResponseChunk, 10)
	runner.responseChan.Closed = false
	runner.mu.Unlock()

	// Call handleRestartAttempt
	runner.handleRestartAttempt(1)

	// Try to receive the message
	select {
	case chunk := <-runner.responseChan.Channel:
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
