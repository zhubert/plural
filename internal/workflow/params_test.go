package workflow

import (
	"testing"
	"time"
)

func TestParamHelper_String(t *testing.T) {
	p := NewParamHelper(map[string]any{
		"name":  "test",
		"count": 42,
	})

	if got := p.String("name", "default"); got != "test" {
		t.Errorf("expected test, got %s", got)
	}
	if got := p.String("missing", "default"); got != "default" {
		t.Errorf("expected default, got %s", got)
	}
	// Wrong type returns default
	if got := p.String("count", "default"); got != "default" {
		t.Errorf("expected default for wrong type, got %s", got)
	}
}

func TestParamHelper_Int(t *testing.T) {
	p := NewParamHelper(map[string]any{
		"count":  42,
		"floaty": 3.14,
		"text":   "hello",
	})

	if got := p.Int("count", 0); got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
	if got := p.Int("floaty", 0); got != 3 {
		t.Errorf("expected 3 (from float64), got %d", got)
	}
	if got := p.Int("missing", 99); got != 99 {
		t.Errorf("expected 99, got %d", got)
	}
	if got := p.Int("text", 99); got != 99 {
		t.Errorf("expected 99 for wrong type, got %d", got)
	}
}

func TestParamHelper_Bool(t *testing.T) {
	p := NewParamHelper(map[string]any{
		"enabled":  true,
		"disabled": false,
		"text":     "yes",
	})

	if got := p.Bool("enabled", false); !got {
		t.Error("expected true")
	}
	if got := p.Bool("disabled", true); got {
		t.Error("expected false")
	}
	if got := p.Bool("missing", true); !got {
		t.Error("expected true (default)")
	}
	// Wrong type returns default
	if got := p.Bool("text", false); got {
		t.Error("expected false for wrong type")
	}
}

func TestParamHelper_Duration(t *testing.T) {
	p := NewParamHelper(map[string]any{
		"timeout":  "30m",
		"invalid":  "bogus",
		"notastr":  42,
	})

	if got := p.Duration("timeout", 0); got != 30*time.Minute {
		t.Errorf("expected 30m, got %v", got)
	}
	if got := p.Duration("missing", 5*time.Second); got != 5*time.Second {
		t.Errorf("expected 5s, got %v", got)
	}
	if got := p.Duration("invalid", 10*time.Second); got != 10*time.Second {
		t.Errorf("expected 10s for invalid, got %v", got)
	}
	if got := p.Duration("notastr", 10*time.Second); got != 10*time.Second {
		t.Errorf("expected 10s for wrong type, got %v", got)
	}
}

func TestParamHelper_Has(t *testing.T) {
	p := NewParamHelper(map[string]any{"key": "val"})

	if !p.Has("key") {
		t.Error("expected Has to return true")
	}
	if p.Has("missing") {
		t.Error("expected Has to return false")
	}
}

func TestParamHelper_Raw(t *testing.T) {
	p := NewParamHelper(map[string]any{"key": []string{"a", "b"}})

	if p.Raw("key") == nil {
		t.Error("expected non-nil")
	}
	if p.Raw("missing") != nil {
		t.Error("expected nil")
	}
}

func TestParamHelper_NilParams(t *testing.T) {
	p := NewParamHelper(nil)

	if got := p.String("key", "default"); got != "default" {
		t.Errorf("expected default, got %s", got)
	}
	if got := p.Int("key", 42); got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
	if p.Has("key") {
		t.Error("expected Has to return false on nil params")
	}
}

func TestParamHelper_Validate(t *testing.T) {
	p := NewParamHelper(map[string]any{
		"max_turns": 50,
		"unknown":   true,
	})

	allowed := map[string]bool{"max_turns": true, "max_duration": true}
	err := p.Validate(allowed)
	if err == nil {
		t.Error("expected error for unknown param")
	}

	p2 := NewParamHelper(map[string]any{"max_turns": 50})
	if err := p2.Validate(allowed); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
