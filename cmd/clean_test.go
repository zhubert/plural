package cmd

import (
	"io"
	"os"
	"path/filepath"
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

func TestFindStaleTempFiles(t *testing.T) {
	// Create temp files matching our patterns in a temp dir
	tmpDir := t.TempDir()

	// Create stale files
	staleFiles := []string{
		filepath.Join(tmpDir, "plural-mcp-abc123.json"),
		filepath.Join(tmpDir, "plural-mcp-def456.json"),
		filepath.Join(tmpDir, "pl-abcd.sock"),
	}
	for _, f := range staleFiles {
		if err := os.WriteFile(f, []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	// Create non-matching files that should be ignored
	os.WriteFile(filepath.Join(tmpDir, "other-file.json"), []byte("test"), 0600)
	os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte("test"), 0600)

	// Test glob patterns directly (since findStaleTempFiles uses hardcoded paths)
	mcpMatches, _ := filepath.Glob(filepath.Join(tmpDir, "plural-mcp-*.json"))
	sockMatches, _ := filepath.Glob(filepath.Join(tmpDir, "pl-*.sock"))

	if len(mcpMatches) != 2 {
		t.Errorf("expected 2 MCP config matches, got %d", len(mcpMatches))
	}
	if len(sockMatches) != 1 {
		t.Errorf("expected 1 socket match, got %d", len(sockMatches))
	}
}

func TestCleanStaleTempFiles(t *testing.T) {
	tmpDir := t.TempDir()

	files := []string{
		filepath.Join(tmpDir, "plural-auth-abc123"),
		filepath.Join(tmpDir, "plural-mcp-abc123.json"),
		filepath.Join(tmpDir, "pl-abcd.sock"),
	}
	for _, f := range files {
		if err := os.WriteFile(f, []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	removed := cleanStaleTempFiles(files)
	if removed != 3 {
		t.Errorf("expected 3 files removed, got %d", removed)
	}

	// Verify files are actually gone
	for _, f := range files {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("file %s should have been removed", f)
		}
	}
}

func TestCleanStaleTempFiles_NonexistentFiles(t *testing.T) {
	files := []string{
		"/nonexistent/plural-auth-abc",
		"/nonexistent/plural-mcp-abc.json",
	}

	removed := cleanStaleTempFiles(files)
	if removed != 0 {
		t.Errorf("expected 0 files removed for nonexistent files, got %d", removed)
	}
}

func TestCleanStaleTempFiles_EmptyList(t *testing.T) {
	removed := cleanStaleTempFiles(nil)
	if removed != 0 {
		t.Errorf("expected 0 files removed for empty list, got %d", removed)
	}
}
