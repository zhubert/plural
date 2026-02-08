package claude

import (
	"testing"
)

func TestSafeSendChannel_Success(t *testing.T) {
	ch := make(chan int, 1)
	sent := safeSendChannel(ch, 42)
	if !sent {
		t.Error("expected send to succeed on buffered channel")
	}
	val := <-ch
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}
}

func TestSafeSendChannel_FullChannel(t *testing.T) {
	ch := make(chan int) // unbuffered
	sent := safeSendChannel(ch, 42)
	if sent {
		t.Error("expected send to fail on full/unbuffered channel with no receiver")
	}
}

func TestSafeSendChannel_ClosedChannel(t *testing.T) {
	ch := make(chan int, 1)
	close(ch)

	// Should not panic, should return false
	sent := safeSendChannel(ch, 42)
	if sent {
		t.Error("expected send to fail on closed channel")
	}
}

func TestSafeSendChannel_NilChannel(t *testing.T) {
	// Sending to nil channel in a select should fall through to default case
	var ch chan int
	sent := safeSendChannel(ch, 42)
	if sent {
		t.Error("expected send to fail on nil channel")
	}
}
