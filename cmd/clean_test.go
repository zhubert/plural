package cmd

import (
	"io"
	"strings"
	"testing"
)

func TestConfirm(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"lowercase y", "y\n", true},
		{"uppercase Y", "Y\n", true},
		{"lowercase yes", "yes\n", true},
		{"uppercase YES", "YES\n", true},
		{"mixed case Yes", "Yes\n", true},
		{"lowercase n", "n\n", false},
		{"lowercase no", "no\n", false},
		{"empty input", "\n", false},
		{"random text", "maybe\n", false},
		{"y with spaces", "  y  \n", true},
		{"yes with spaces", "  yes  \n", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.input)
			result := confirm(reader, "Test?")
			if result != tt.expected {
				t.Errorf("confirm(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestConfirm_EOF(t *testing.T) {
	// Test with empty reader (simulates EOF)
	reader := strings.NewReader("")
	result := confirm(reader, "Test?")
	if result != false {
		t.Errorf("confirm(EOF) = %v, want false", result)
	}
}

func TestConfirm_ErrorReader(t *testing.T) {
	// Test with a reader that returns an error
	reader := &errorReader{}
	result := confirm(reader, "Test?")
	if result != false {
		t.Errorf("confirm(error) = %v, want false", result)
	}
}

// errorReader is a reader that always returns an error
type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}
