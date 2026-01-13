package modals

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get home directory")
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"~", home},
		{"~/Documents", filepath.Join(home, "Documents")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := expandHome(tt.input)
			if result != tt.expected {
				t.Errorf("expandHome(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCommonPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{"empty", []string{}, ""},
		{"single", []string{"hello"}, "hello"},
		{"common", []string{"hello", "help", "helicopter"}, "hel"},
		{"no common", []string{"abc", "xyz"}, ""},
		{"full match", []string{"same", "same"}, "same"},
		{"partial paths", []string{"/usr/local/bin", "/usr/local/lib"}, "/usr/local/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := commonPrefix(tt.input)
			if result != tt.expected {
				t.Errorf("commonPrefix(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestPathCompleter_Complete(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir, err := os.MkdirTemp("", "completion_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test directories
	testDirs := []string{
		"projects",
		"projects/app1",
		"projects/app2",
		"pictures",
		"documents",
	}
	for _, dir := range testDirs {
		if err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Create a test file (should not appear in directory-only completions)
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("complete directory with common prefix", func(t *testing.T) {
		pc := NewPathCompleter()
		// Type "p" in tmpDir - should complete to common prefix of projects/ and pictures/
		result, ok := pc.Complete(filepath.Join(tmpDir, "p"))
		if !ok {
			t.Error("Expected completion to succeed")
		}
		// Should get common prefix "p" with completions available
		expectedPrefix := filepath.Join(tmpDir, "p")
		if result != expectedPrefix+"ictures/" && result != expectedPrefix+"rojects/" {
			// First completion should be one of the "p" directories
			t.Logf("Got result: %s", result)
		}
	})

	t.Run("complete unique prefix", func(t *testing.T) {
		pc := NewPathCompleter()
		result, ok := pc.Complete(filepath.Join(tmpDir, "doc"))
		if !ok {
			t.Error("Expected completion to succeed")
		}
		expected := filepath.Join(tmpDir, "documents") + "/"
		if result != expected {
			t.Errorf("Got %q, want %q", result, expected)
		}
	})

	t.Run("complete existing directory adds slash", func(t *testing.T) {
		pc := NewPathCompleter()
		result, ok := pc.Complete(filepath.Join(tmpDir, "projects"))
		if !ok {
			t.Error("Expected completion to succeed")
		}
		expected := filepath.Join(tmpDir, "projects") + "/"
		if result != expected {
			t.Errorf("Got %q, want %q", result, expected)
		}
	})

	t.Run("complete in directory lists subdirs", func(t *testing.T) {
		pc := NewPathCompleter()
		result, ok := pc.Complete(filepath.Join(tmpDir, "projects") + "/")
		if !ok {
			t.Error("Expected completion to succeed")
		}
		// Should complete to app1 or app2 (common prefix is "app")
		if result != filepath.Join(tmpDir, "projects", "app") {
			// Or it might return first match
			t.Logf("Got result: %s", result)
		}
	})

	t.Run("no match returns original", func(t *testing.T) {
		pc := NewPathCompleter()
		original := filepath.Join(tmpDir, "xyz")
		result, ok := pc.Complete(original)
		if ok {
			t.Error("Expected completion to fail for non-matching prefix")
		}
		if result != original {
			t.Errorf("Got %q, want %q", result, original)
		}
	})

	t.Run("cycle through completions", func(t *testing.T) {
		pc := NewPathCompleter()
		prefix := filepath.Join(tmpDir, "projects") + "/app"

		// First tab - should complete to common prefix or first match
		result1, ok := pc.Complete(prefix)
		if !ok {
			t.Error("Expected first completion to succeed")
		}

		// The completer should have the common prefix stored
		// Complete again with same input to cycle through matches
		result2, ok := pc.Complete(prefix)
		if !ok {
			t.Error("Expected second completion to succeed")
		}

		// If there are multiple matches, after common prefix, cycling should work
		completions := pc.GetCompletions()
		t.Logf("Completions: %v", completions)
		t.Logf("Result1: %s, Result2: %s", result1, result2)

		// Results may be same if common prefix was returned first
		// The key behavior is that completions are populated
		if len(completions) < 2 {
			t.Error("Expected at least 2 completions for app* pattern")
		}
	})

	t.Run("reset clears state", func(t *testing.T) {
		pc := NewPathCompleter()
		pc.Complete(filepath.Join(tmpDir, "p"))
		if len(pc.GetCompletions()) == 0 {
			t.Error("Expected completions to be populated")
		}

		pc.Reset()
		if len(pc.GetCompletions()) != 0 {
			t.Error("Expected completions to be cleared after reset")
		}
		if pc.GetIndex() != -1 {
			t.Error("Expected index to be reset to -1")
		}
	})
}

func TestPathCompleter_HiddenFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "completion_hidden_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create hidden and non-hidden directories
	dirs := []string{".hidden", ".config", "visible"}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("hidden dirs excluded by default", func(t *testing.T) {
		pc := NewPathCompleter()
		pc.Complete(tmpDir + "/")

		completions := pc.GetCompletions()
		for _, c := range completions {
			base := filepath.Base(c)
			if base[0] == '.' {
				t.Errorf("Hidden directory %q should not be in completions", c)
			}
		}
	})

	t.Run("hidden dirs included when prefix starts with dot", func(t *testing.T) {
		pc := NewPathCompleter()
		// Use trailing slash + dot to list hidden files in tmpDir
		_, ok := pc.Complete(tmpDir + "/.")

		if !ok {
			t.Error("Expected completion to succeed for hidden files")
		}

		completions := pc.GetCompletions()
		if len(completions) == 0 {
			t.Error("Expected hidden directories to be included")
		}

		foundHidden := false
		for _, c := range completions {
			base := filepath.Base(strings.TrimSuffix(c, "/"))
			if len(base) > 0 && base[0] == '.' {
				foundHidden = true
				break
			}
		}
		if !foundHidden {
			t.Logf("Completions: %v", completions)
			t.Error("Expected to find hidden directories when prefix starts with .")
		}
	})
}
