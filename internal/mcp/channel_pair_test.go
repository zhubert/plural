package mcp

import (
	"errors"
	"sync"
	"testing"
)

func TestNewChannelPair(t *testing.T) {
	cp := NewChannelPair[int, string](1)
	if cp == nil {
		t.Fatal("NewChannelPair returned nil")
	}
	if cp.Req == nil {
		t.Error("Req channel is nil")
	}
	if cp.Resp == nil {
		t.Error("Resp channel is nil")
	}

	// Verify buffer size by sending without blocking
	cp.Req <- 42
	cp.Resp <- "hello"

	got := <-cp.Req
	if got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
	gotStr := <-cp.Resp
	if gotStr != "hello" {
		t.Errorf("expected hello, got %s", gotStr)
	}
}

func TestNewChannelPairZeroBuffer(t *testing.T) {
	cp := NewChannelPair[int, int](0)
	if cp == nil {
		t.Fatal("NewChannelPair returned nil")
	}
	if cp.Req == nil || cp.Resp == nil {
		t.Error("channels should be non-nil even with zero buffer")
	}
}

func TestChannelPairClose(t *testing.T) {
	cp := NewChannelPair[int, string](1)

	// Close should work
	cp.Close()

	if cp.Req != nil {
		t.Error("Req should be nil after Close")
	}
	if cp.Resp != nil {
		t.Error("Resp should be nil after Close")
	}
}

func TestChannelPairCloseNil(t *testing.T) {
	// Close on nil ChannelPair should not panic
	var cp *ChannelPair[int, string]
	cp.Close() // should be a no-op
}

func TestChannelPairCloseIdempotent(t *testing.T) {
	cp := NewChannelPair[int, string](1)

	// Double close should not panic
	cp.Close()
	cp.Close()
}

func TestChannelPairIsInitialized(t *testing.T) {
	tests := []struct {
		name     string
		cp       *ChannelPair[int, string]
		expected bool
	}{
		{
			name:     "nil pair",
			cp:       nil,
			expected: false,
		},
		{
			name:     "initialized pair",
			cp:       NewChannelPair[int, string](1),
			expected: true,
		},
		{
			name: "nil Req channel",
			cp: &ChannelPair[int, string]{
				Req:  nil,
				Resp: make(chan string, 1),
			},
			expected: false,
		},
		{
			name: "nil Resp channel",
			cp: &ChannelPair[int, string]{
				Req:  make(chan int, 1),
				Resp: nil,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cp.IsInitialized(); got != tt.expected {
				t.Errorf("IsInitialized() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestChannelPairIsInitializedAfterClose(t *testing.T) {
	cp := NewChannelPair[int, string](1)
	if !cp.IsInitialized() {
		t.Error("should be initialized before Close")
	}

	cp.Close()
	if cp.IsInitialized() {
		t.Error("should not be initialized after Close")
	}
}

func TestForwardRequests(t *testing.T) {
	reqCh := make(chan int, 3)
	respCh := make(chan string, 3)

	var wg sync.WaitGroup

	ForwardRequests(&wg, reqCh, respCh,
		func(req int) (string, error) {
			return "ok", nil
		},
		func(req int) string {
			return "error"
		})

	// Send requests
	reqCh <- 1
	reqCh <- 2
	reqCh <- 3
	close(reqCh)

	wg.Wait()

	// All responses should be "ok"
	for range 3 {
		resp := <-respCh
		if resp != "ok" {
			t.Errorf("expected ok, got %s", resp)
		}
	}
}

func TestForwardRequestsWithErrors(t *testing.T) {
	reqCh := make(chan int, 2)
	respCh := make(chan string, 2)

	var wg sync.WaitGroup

	ForwardRequests(&wg, reqCh, respCh,
		func(req int) (string, error) {
			if req == 1 {
				return "", errors.New("fail")
			}
			return "ok", nil
		},
		func(req int) string {
			return "fallback"
		})

	// Send requests: first fails, second succeeds
	reqCh <- 1
	reqCh <- 2
	close(reqCh)

	wg.Wait()

	resp1 := <-respCh
	resp2 := <-respCh

	if resp1 != "fallback" {
		t.Errorf("expected fallback for failed request, got %s", resp1)
	}
	if resp2 != "ok" {
		t.Errorf("expected ok for successful request, got %s", resp2)
	}
}

func TestForwardRequestsEmptyChannel(t *testing.T) {
	reqCh := make(chan int)
	respCh := make(chan string, 1)

	var wg sync.WaitGroup

	ForwardRequests(&wg, reqCh, respCh,
		func(req int) (string, error) {
			return "ok", nil
		},
		func(req int) string {
			return "error"
		})

	// Close immediately - goroutine should exit cleanly
	close(reqCh)
	wg.Wait()

	// No responses should have been sent
	select {
	case resp := <-respCh:
		t.Errorf("unexpected response: %s", resp)
	default:
		// Expected - no responses
	}
}
