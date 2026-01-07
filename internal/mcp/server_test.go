package mcp

import (
	"strings"
	"testing"
)

func TestBuildToolDescription(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		input    map[string]interface{}
		expected string
	}{
		{
			name: "Edit with file path",
			tool: "Edit",
			input: map[string]interface{}{
				"file_path": "/path/to/file.go",
			},
			expected: "Edit file: /path/to/file.go",
		},
		{
			name: "Write with file path",
			tool: "Write",
			input: map[string]interface{}{
				"file_path": "/path/to/new.txt",
			},
			expected: "Write file: /path/to/new.txt",
		},
		{
			name: "Read with file path",
			tool: "Read",
			input: map[string]interface{}{
				"file_path": "/path/to/read.go",
			},
			expected: "Read file: /path/to/read.go",
		},
		{
			name: "Bash with short command",
			tool: "Bash",
			input: map[string]interface{}{
				"command": "ls -la",
			},
			expected: "Run: ls -la",
		},
		{
			name: "Bash with long command truncated",
			tool: "Bash",
			input: map[string]interface{}{
				"command": strings.Repeat("a", 150),
			},
			expected: "Run: " + strings.Repeat("a", 97) + "...",
		},
		{
			name: "Glob with pattern only",
			tool: "Glob",
			input: map[string]interface{}{
				"pattern": "**/*.go",
			},
			expected: "Search for files: **/*.go",
		},
		{
			name: "Glob with pattern and path",
			tool: "Glob",
			input: map[string]interface{}{
				"pattern": "*.ts",
				"path":    "/src",
			},
			expected: "Search for files: *.ts in /src",
		},
		{
			name: "Grep with pattern only",
			tool: "Grep",
			input: map[string]interface{}{
				"pattern": "func main",
			},
			expected: "Search for: func main",
		},
		{
			name: "Grep with pattern and path",
			tool: "Grep",
			input: map[string]interface{}{
				"pattern": "TODO",
				"path":    "/internal",
			},
			expected: "Search for: TODO in /internal",
		},
		{
			name: "Task with description",
			tool: "Task",
			input: map[string]interface{}{
				"description": "Explore codebase",
			},
			expected: "Delegate task: Explore codebase",
		},
		{
			name: "Task with prompt",
			tool: "Task",
			input: map[string]interface{}{
				"prompt": "Find all API endpoints",
			},
			expected: "Delegate task: Find all API endpoints",
		},
		{
			name: "WebFetch with URL",
			tool: "WebFetch",
			input: map[string]interface{}{
				"url": "https://example.com",
			},
			expected: "Fetch URL: https://example.com",
		},
		{
			name: "WebSearch with query",
			tool: "WebSearch",
			input: map[string]interface{}{
				"query": "golang testing",
			},
			expected: "Web search: golang testing",
		},
		{
			name: "NotebookEdit with path",
			tool: "NotebookEdit",
			input: map[string]interface{}{
				"notebook_path": "/notebooks/analysis.ipynb",
			},
			expected: "Edit notebook: /notebooks/analysis.ipynb",
		},
		{
			name: "Unknown tool with file_path",
			tool: "CustomTool",
			input: map[string]interface{}{
				"file_path": "/some/file.txt",
			},
			expected: "CustomTool: /some/file.txt",
		},
		{
			name: "Unknown tool with command",
			tool: "CustomTool",
			input: map[string]interface{}{
				"command": "some command",
			},
			expected: "CustomTool: some command",
		},
		{
			name: "Empty input returns empty string",
			tool: "Edit",
			input: map[string]interface{}{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildToolDescription(tt.tool, tt.input)
			if got != tt.expected {
				t.Errorf("buildToolDescription() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFormatInputForDisplay(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]interface{}
		contains []string
	}{
		{
			name:     "Empty args",
			args:     map[string]interface{}{},
			contains: []string{"(no details available)"},
		},
		{
			name: "Simple string value",
			args: map[string]interface{}{
				"file_path": "/path/to/file.go",
			},
			contains: []string{"File: /path/to/file.go"},
		},
		{
			name: "Boolean values",
			args: map[string]interface{}{
				"replace_all": true,
			},
			contains: []string{"Replace all: yes"},
		},
		{
			name: "Skips tool_use_id",
			args: map[string]interface{}{
				"file_path":   "/path/to/file.go",
				"tool_use_id": "abc123",
			},
			contains: []string{"File: /path/to/file.go"},
		},
		{
			name: "Multiple values joined with bullet separator",
			args: map[string]interface{}{
				"path":    "/dir",
				"command": "ls",
			},
			contains: []string{"Command: ls", "•", "Path: /dir"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatInputForDisplay(tt.args)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("formatInputForDisplay() = %q, want to contain %q", got, want)
				}
			}
		})
	}
}

func TestHumanizeKey(t *testing.T) {
	tests := []struct {
		key      string
		expected string
	}{
		{"file_path", "File"},
		{"command", "Command"},
		{"pattern", "Pattern"},
		{"old_string", "Find"},
		{"new_string", "Replace with"},
		{"replace_all", "Replace all"},
		{"unknown_key", "Unknown Key"},
		{"simple", "Simple"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := humanizeKey(tt.key)
			if got != tt.expected {
				t.Errorf("humanizeKey(%q) = %q, want %q", tt.key, got, tt.expected)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		maxLen   int
		expected string
	}{
		{
			name:     "Short string unchanged",
			s:        "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "Exact length unchanged",
			s:        "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "Long string truncated with ellipsis",
			s:        "hello world",
			maxLen:   8,
			expected: "hello...",
		},
		{
			name:     "Very short maxLen",
			s:        "hello",
			maxLen:   2,
			expected: "he",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.s, tt.maxLen)
			if got != tt.expected {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.expected)
			}
		})
	}
}

func TestFormatNestedObject(t *testing.T) {
	tests := []struct {
		name     string
		obj      map[string]interface{}
		expected string
	}{
		{
			name:     "Empty object",
			obj:      map[string]interface{}{},
			expected: "(empty)",
		},
		{
			name: "Small object inline",
			obj: map[string]interface{}{
				"file_path": "/test.go",
			},
			expected: "File: /test.go",
		},
		{
			name: "Large object shows count",
			obj: map[string]interface{}{
				"a": "1",
				"b": "2",
				"c": "3",
				"d": "4",
			},
			expected: "(4 properties)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatNestedObject(tt.obj)
			if got != tt.expected {
				t.Errorf("formatNestedObject() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFormatArray(t *testing.T) {
	tests := []struct {
		name     string
		arr      []interface{}
		expected string
	}{
		{
			name:     "Empty array",
			arr:      []interface{}{},
			expected: "(empty)",
		},
		{
			name:     "Single string item",
			arr:      []interface{}{"hello"},
			expected: "hello",
		},
		{
			name:     "Multiple items shows count",
			arr:      []interface{}{"a", "b", "c"},
			expected: "(3 items)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatArray(tt.arr)
			if got != tt.expected {
				t.Errorf("formatArray() = %q, want %q", got, tt.expected)
			}
		})
	}
}
