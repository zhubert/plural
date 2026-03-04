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

func TestFindStaleTempFilesInDirs(t *testing.T) {
	configDir := t.TempDir()
	tmpDir := t.TempDir()

	// Create stale files in config dir
	os.WriteFile(filepath.Join(configDir, "plural-auth-abc123"), []byte("test"), 0600)
	os.WriteFile(filepath.Join(configDir, "plural-auth-def456"), []byte("test"), 0600)
	os.WriteFile(filepath.Join(configDir, "plural-mcp-abc123.json"), []byte("test"), 0600)

	// Create stale files in tmp dir
	os.WriteFile(filepath.Join(tmpDir, "plural-mcp-ghi789.json"), []byte("test"), 0600)
	os.WriteFile(filepath.Join(tmpDir, "pl-abcd.sock"), []byte("test"), 0600)

	// Create non-matching files that should be ignored
	os.WriteFile(filepath.Join(configDir, "config.json"), []byte("test"), 0600)
	os.WriteFile(filepath.Join(tmpDir, "other-file.json"), []byte("test"), 0600)

	files := findStaleTempFilesInDirs(configDir, tmpDir)

	if len(files) != 5 {
		t.Errorf("expected 5 stale files, got %d: %v", len(files), files)
	}
}

func TestFindStaleTempFilesInDirs_EmptyDirs(t *testing.T) {
	files := findStaleTempFilesInDirs("", "")
	if len(files) != 0 {
		t.Errorf("expected 0 stale files for empty dirs, got %d", len(files))
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
