package modals

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PathCompleter handles filesystem path auto-completion.
type PathCompleter struct {
	completions []string // Available completions for current prefix
	index       int      // Current selection index (-1 means no selection)
	prefix      string   // The prefix used to generate completions
}

// NewPathCompleter creates a new path completer.
func NewPathCompleter() *PathCompleter {
	return &PathCompleter{
		completions: nil,
		index:       -1,
		prefix:      "",
	}
}

// Complete attempts to complete the given path.
// Returns the completed path and whether completion was successful.
// On first tab press, it completes to common prefix or first match.
// On subsequent tab presses with same prefix, it cycles through matches.
func (pc *PathCompleter) Complete(path string) (string, bool) {
	// Expand ~ to home directory
	expandedPath := expandHome(path)

	// If path changed from last completion, regenerate completions
	if expandedPath != pc.prefix || pc.completions == nil {
		pc.generateCompletions(expandedPath)
		pc.prefix = expandedPath
	}

	if len(pc.completions) == 0 {
		return path, false
	}

	// Single completion - just return it
	if len(pc.completions) == 1 {
		return pc.completions[0], true
	}

	// Multiple completions - find common prefix first time, then cycle
	if pc.index == -1 {
		// First tab - try to complete to common prefix
		common := commonPrefix(pc.completions)
		if common != expandedPath && common != "" {
			// Reset so next tab cycles through options
			pc.prefix = common
			pc.index = -1
			return common, true
		}
		// Common prefix is same as input, start cycling
		pc.index = 0
		return pc.completions[0], true
	}

	// Cycle through completions
	pc.index = (pc.index + 1) % len(pc.completions)
	return pc.completions[pc.index], true
}

// Reset clears the current completion state.
// Call this when the input changes (not via tab completion).
func (pc *PathCompleter) Reset() {
	pc.completions = nil
	pc.index = -1
	pc.prefix = ""
}

// GetCompletions returns the current list of completions.
func (pc *PathCompleter) GetCompletions() []string {
	return pc.completions
}

// GetIndex returns the current selection index.
func (pc *PathCompleter) GetIndex() int {
	return pc.index
}

// GenerateCompletions populates the completions for the given path.
// This is the public version that expands home directory.
func (pc *PathCompleter) GenerateCompletions(path string) {
	expandedPath := expandHome(path)
	pc.generateCompletions(expandedPath)
	pc.prefix = expandedPath
}

// GetCommonPrefix returns the longest common prefix of all completions.
func (pc *PathCompleter) GetCommonPrefix() string {
	return commonPrefix(pc.completions)
}

// generateCompletions populates the completions slice for the given path.
func (pc *PathCompleter) generateCompletions(path string) {
	pc.completions = nil
	pc.index = -1

	if path == "" {
		path = "/"
	}

	var dir, prefix string

	// Check if path exists and is a directory
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		// Path is an existing directory
		if strings.HasSuffix(path, "/") || path == "/" {
			// User typed trailing slash, list contents
			dir = path
			prefix = ""
		} else {
			// No trailing slash - could mean they want to complete the name or go into dir
			// Add trailing slash to complete the directory
			pc.completions = []string{path + "/"}
			return
		}
	} else {
		// Path doesn't exist or is a file - get parent directory and filename prefix
		dir = filepath.Dir(path)
		prefix = filepath.Base(path)
		if path == prefix {
			// No directory component, use current dir
			dir = "."
		}
	}

	// Read directory contents
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// Filter entries that match prefix and are directories
	for _, entry := range entries {
		name := entry.Name()
		// Skip hidden files unless prefix starts with .
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".") {
			continue
		}

		if strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix)) {
			fullPath := filepath.Join(dir, name)
			if entry.IsDir() {
				fullPath += "/"
			}
			pc.completions = append(pc.completions, fullPath)
		}
	}

	// Sort completions
	sort.Strings(pc.completions)
}

// expandHome expands ~ to the user's home directory.
func expandHome(path string) string {
	if path == "" {
		return ""
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// commonPrefix finds the longest common prefix among all strings.
func commonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}

	// Find shortest string
	shortest := strs[0]
	for _, s := range strs[1:] {
		if len(s) < len(shortest) {
			shortest = s
		}
	}

	// Find common prefix
	for i := 0; i < len(shortest); i++ {
		char := shortest[i]
		for _, s := range strs {
			if s[i] != char {
				return shortest[:i]
			}
		}
	}

	return shortest
}
