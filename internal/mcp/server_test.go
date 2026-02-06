package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhubert/plural/internal/logger"
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

func TestBuildToolDescription_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		input    map[string]interface{}
		expected string
	}{
		{
			name:     "Nil input",
			tool:     "Edit",
			input:    nil,
			expected: "",
		},
		{
			name: "Task with both description and prompt prefers description",
			tool: "Task",
			input: map[string]interface{}{
				"description": "Short desc",
				"prompt":      "Long prompt text",
			},
			expected: "Delegate task: Short desc",
		},
		{
			name: "Unknown tool with url",
			tool: "CustomTool",
			input: map[string]interface{}{
				"url": "https://example.com",
			},
			expected: "CustomTool: https://example.com",
		},
		{
			name: "Unknown tool with path",
			tool: "CustomTool",
			input: map[string]interface{}{
				"path": "/some/path",
			},
			expected: "CustomTool: /some/path",
		},
		{
			name: "Unknown tool with no recognized fields",
			tool: "CustomTool",
			input: map[string]interface{}{
				"foo": "bar",
			},
			expected: "",
		},
		{
			name: "Wrong type for file_path",
			tool: "Edit",
			input: map[string]interface{}{
				"file_path": 123,
			},
			expected: "",
		},
		{
			name: "Task with long prompt truncated",
			tool: "Task",
			input: map[string]interface{}{
				"prompt": strings.Repeat("x", 100),
			},
			expected: "Delegate task: " + strings.Repeat("x", 57) + "...",
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

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    interface{}
		contains string
	}{
		{
			name:     "String value",
			key:      "file_path",
			value:    "/test/file.go",
			contains: "File: /test/file.go",
		},
		{
			name:     "Empty string value",
			key:      "file_path",
			value:    "",
			contains: "",
		},
		{
			name:     "Boolean true",
			key:      "replace_all",
			value:    true,
			contains: "yes",
		},
		{
			name:     "Boolean false",
			key:      "replace_all",
			value:    false,
			contains: "no",
		},
		{
			name:     "Float64 value",
			key:      "line_number",
			value:    42.0,
			contains: "42",
		},
		{
			name:     "Nil value",
			key:      "something",
			value:    nil,
			contains: "",
		},
		{
			name:     "Nested object",
			key:      "options",
			value:    map[string]interface{}{"key": "val"},
			contains: "Options:",
		},
		{
			name:     "Empty nested object",
			key:      "options",
			value:    map[string]interface{}{},
			contains: "",
		},
		{
			name:     "Array",
			key:      "items",
			value:    []interface{}{"a", "b"},
			contains: "(2 items)",
		},
		{
			name:     "Empty array",
			key:      "items",
			value:    []interface{}{},
			contains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatValue(tt.key, tt.value)
			if tt.contains == "" {
				if got != "" {
					t.Errorf("formatValue(%q, %v) = %q, want empty", tt.key, tt.value, got)
				}
			} else if !strings.Contains(got, tt.contains) {
				t.Errorf("formatValue(%q, %v) = %q, want to contain %q", tt.key, tt.value, got, tt.contains)
			}
		})
	}
}

func TestFormatNestedObject_MultipleFields(t *testing.T) {
	// Test with exactly 3 fields (boundary for inline display)
	obj := map[string]interface{}{
		"file_path": "/test.go",
		"command":   "test",
		"pattern":   "*.go",
	}
	got := formatNestedObject(obj)

	// Should be inline (3 fields is the limit)
	if strings.Contains(got, "properties") {
		t.Errorf("Expected inline format for 3 fields, got %q", got)
	}
}

func TestFormatNestedObject_BooleanField(t *testing.T) {
	obj := map[string]interface{}{
		"enabled": true,
	}
	got := formatNestedObject(obj)
	if !strings.Contains(got, "yes") {
		t.Errorf("Expected 'yes' for true boolean, got %q", got)
	}

	obj["enabled"] = false
	got = formatNestedObject(obj)
	if !strings.Contains(got, "no") {
		t.Errorf("Expected 'no' for false boolean, got %q", got)
	}
}

func TestFormatArray_NonStringItem(t *testing.T) {
	arr := []interface{}{42}
	got := formatArray(arr)
	if got != "42" {
		t.Errorf("Expected '42' for single non-string item, got %q", got)
	}
}

func TestHumanizeKey_AllMapped(t *testing.T) {
	// Test all mapped keys
	mappedKeys := []string{
		"file_path", "command", "pattern", "path", "tool_name",
		"input", "description", "url", "query", "notebook_path",
		"content", "old_string", "new_string", "replace_all",
	}

	for _, key := range mappedKeys {
		got := humanizeKey(key)
		if got == "" {
			t.Errorf("humanizeKey(%q) returned empty string", key)
		}
		// Mapped keys should not contain underscores
		if strings.Contains(got, "_") {
			t.Errorf("humanizeKey(%q) = %q, should not contain underscore", key, got)
		}
	}
}

func TestHumanizeKey_UnmappedMultiWord(t *testing.T) {
	tests := []struct {
		key      string
		expected string
	}{
		{"some_long_key", "Some Long Key"},
		{"a_b_c", "A B C"},
		{"single", "Single"},
		{"", ""},
	}

	for _, tt := range tests {
		got := humanizeKey(tt.key)
		if got != tt.expected {
			t.Errorf("humanizeKey(%q) = %q, want %q", tt.key, got, tt.expected)
		}
	}
}

func TestTruncateString_EdgeCases(t *testing.T) {
	tests := []struct {
		s        string
		maxLen   int
		expected string
	}{
		{"", 10, ""},
		{"a", 0, ""},     // Zero maxLen returns empty (per implementation)
		{"ab", 1, "a"},   // Very short truncation
		{"abc", 3, "abc"}, // Exact length
	}

	for _, tt := range tests {
		got := truncateString(tt.s, tt.maxLen)
		if got != tt.expected {
			t.Errorf("truncateString(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.expected)
		}
	}
}

func TestServerConstants(t *testing.T) {
	if ProtocolVersion == "" {
		t.Error("ProtocolVersion should not be empty")
	}

	if ServerName == "" {
		t.Error("ServerName should not be empty")
	}

	if ServerVersion == "" {
		t.Error("ServerVersion should not be empty")
	}

	if ToolName == "" {
		t.Error("ToolName should not be empty")
	}
}

func TestServer_isToolAllowed(t *testing.T) {
	tests := []struct {
		name         string
		allowedTools []string
		tool         string
		expected     bool
	}{
		{
			name:         "exact match",
			allowedTools: []string{"Edit", "Read"},
			tool:         "Edit",
			expected:     true,
		},
		{
			name:         "no match",
			allowedTools: []string{"Edit", "Read"},
			tool:         "Write",
			expected:     false,
		},
		{
			name:         "pattern match with prefix",
			allowedTools: []string{"Bash(git:*)"},
			tool:         "Bash",
			expected:     true,
		},
		{
			name:         "empty allowed list",
			allowedTools: []string{},
			tool:         "Edit",
			expected:     false,
		},
		{
			name:         "nil allowed list",
			allowedTools: nil,
			tool:         "Edit",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{allowedTools: tt.allowedTools}
			got := s.isToolAllowed(tt.tool)
			if got != tt.expected {
				t.Errorf("isToolAllowed(%q) = %v, want %v", tt.tool, got, tt.expected)
			}
		})
	}
}

func TestServer_addAllowedTool(t *testing.T) {
	t.Run("adds new tool", func(t *testing.T) {
		s := &Server{allowedTools: []string{"Edit"}}
		s.addAllowedTool("Read")

		if len(s.allowedTools) != 2 {
			t.Errorf("expected 2 tools, got %d", len(s.allowedTools))
		}
		if !s.isToolAllowed("Read") {
			t.Error("Read should be allowed after adding")
		}
	})

	t.Run("does not duplicate existing tool", func(t *testing.T) {
		s := &Server{allowedTools: []string{"Edit", "Read"}}
		s.addAllowedTool("Edit")

		if len(s.allowedTools) != 2 {
			t.Errorf("expected 2 tools (no duplicate), got %d", len(s.allowedTools))
		}
	})

	t.Run("adds to nil list", func(t *testing.T) {
		s := &Server{allowedTools: nil}
		s.addAllowedTool("Edit")

		if len(s.allowedTools) != 1 {
			t.Errorf("expected 1 tool, got %d", len(s.allowedTools))
		}
		if !s.isToolAllowed("Edit") {
			t.Error("Edit should be allowed after adding")
		}
	})
}

func TestHandleExitPlanMode_EmptyPlanShowsUI(t *testing.T) {
	// This test verifies that when ExitPlanMode is called without a plan field,
	// it still sends a request to the TUI for approval rather than auto-approving.
	// This is a regression test for the auto-approval bug.

	t.Run("valid plan passes through", func(t *testing.T) {
		planApprovalChan := make(chan PlanApprovalRequest, 1)
		planResponseChan := make(chan PlanApprovalResponse, 1)
		var buf strings.Builder

		s := &Server{
			planApprovalChan: planApprovalChan,
			planResponseChan: planResponseChan,
			writer:           &buf,
			log:              logger.WithSession("test"),
		}

		go func() {
			req := <-planApprovalChan
			wantPlan := "# My Plan\n\n1. Do something"
			if req.Plan != wantPlan {
				t.Errorf("PlanApprovalRequest.Plan = %q, want %q", req.Plan, wantPlan)
			}
			planResponseChan <- PlanApprovalResponse{ID: req.ID, Approved: false}
		}()

		s.handleExitPlanMode("test-id", map[string]interface{}{"plan": "# My Plan\n\n1. Do something"})
	})

	t.Run("missing plan and filePath shows placeholder", func(t *testing.T) {
		planApprovalChan := make(chan PlanApprovalRequest, 1)
		planResponseChan := make(chan PlanApprovalResponse, 1)
		var buf strings.Builder

		s := &Server{
			planApprovalChan: planApprovalChan,
			planResponseChan: planResponseChan,
			writer:           &buf,
			log:              logger.WithSession("test"),
		}

		go func() {
			req := <-planApprovalChan
			// Should show placeholder message when neither plan nor filePath provided
			if !strings.Contains(req.Plan, "No plan content provided") {
				t.Errorf("Expected 'No plan content provided' message, got: %q", req.Plan)
			}
			planResponseChan <- PlanApprovalResponse{ID: req.ID, Approved: false}
		}()

		// Missing both plan and filePath should show placeholder
		s.handleExitPlanMode("test-id", map[string]interface{}{})
	})

	t.Run("filePath argument reads from specified file", func(t *testing.T) {
		// Create a file in the actual plans directory (like Claude Code does)
		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("Failed to get home dir: %v", err)
		}
		plansDir := filepath.Join(homeDir, ".claude", "plans")
		if err := os.MkdirAll(plansDir, 0755); err != nil {
			t.Fatalf("Failed to create plans dir: %v", err)
		}

		planContent := "# Plan from ~/.claude/plans/\n\nThis is a plan with a whimsical name."
		planPath := filepath.Join(plansDir, "test-dancing-purple-elephant.md")
		if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
			t.Fatalf("Failed to write plan file: %v", err)
		}
		defer os.Remove(planPath)

		planApprovalChan := make(chan PlanApprovalRequest, 1)
		planResponseChan := make(chan PlanApprovalResponse, 1)
		var buf strings.Builder

		s := &Server{
			planApprovalChan: planApprovalChan,
			planResponseChan: planResponseChan,
			writer:           &buf,
			log:              logger.WithSession("test"),
		}

		go func() {
			req := <-planApprovalChan
			if req.Plan != planContent {
				t.Errorf("PlanApprovalRequest.Plan = %q, want %q", req.Plan, planContent)
			}
			planResponseChan <- PlanApprovalResponse{ID: req.ID, Approved: false}
		}()

		// filePath argument should be used to read the plan
		s.handleExitPlanMode("test-id", map[string]interface{}{
			"filePath": planPath,
		})
	})
}

func TestReadPlanFromPath(t *testing.T) {
	s := &Server{log: logger.WithSession("test")}

	t.Run("reads existing file in plans directory", func(t *testing.T) {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("Failed to get home dir: %v", err)
		}
		plansDir := filepath.Join(homeDir, ".claude", "plans")

		// Create plans dir if it doesn't exist
		if err := os.MkdirAll(plansDir, 0755); err != nil {
			t.Fatalf("Failed to create plans dir: %v", err)
		}

		planContent := "# Test Plan\n\n1. Step one\n2. Step two"
		planPath := filepath.Join(plansDir, "test-plan-for-unit-test.md")
		if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
			t.Fatalf("Failed to write plan file: %v", err)
		}
		defer os.Remove(planPath)

		result := s.readPlanFromPath(planPath)
		if result != planContent {
			t.Errorf("readPlanFromPath() = %q, want %q", result, planContent)
		}
	})

	t.Run("rejects path outside plans directory", func(t *testing.T) {
		result := s.readPlanFromPath("/etc/passwd")
		if !strings.Contains(result, "Invalid plan path") {
			t.Errorf("Expected 'Invalid plan path' message for /etc/passwd, got: %q", result)
		}
	})

	t.Run("rejects path traversal with dot-dot", func(t *testing.T) {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("Failed to get home dir: %v", err)
		}
		traversalPath := filepath.Join(homeDir, ".claude", "plans", "..", "..", "..", "etc", "passwd")
		result := s.readPlanFromPath(traversalPath)
		if !strings.Contains(result, "Invalid plan path") {
			t.Errorf("Expected 'Invalid plan path' message for traversal path, got: %q", result)
		}
	})

	t.Run("returns error message when file missing in valid dir", func(t *testing.T) {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			t.Fatalf("Failed to get home dir: %v", err)
		}
		missingPath := filepath.Join(homeDir, ".claude", "plans", "nonexistent-plan.md")
		result := s.readPlanFromPath(missingPath)
		if !strings.Contains(result, "Plan file not found") {
			t.Errorf("Expected 'Plan file not found' message, got: %q", result)
		}
	})
}

func TestValidatePlanPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home dir: %v", err)
	}
	plansDir := filepath.Join(homeDir, ".claude", "plans")

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid path in plans directory",
			path:    filepath.Join(plansDir, "whimsical-dancing-unicorn.md"),
			wantErr: false,
		},
		{
			name:    "path traversal with dot-dot",
			path:    filepath.Join(plansDir, "..", "..", "etc", "passwd"),
			wantErr: true,
		},
		{
			name:    "absolute path outside plans dir",
			path:    "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "path to similar directory name (prefix attack)",
			path:    plansDir + "-evil/malicious.md",
			wantErr: true,
		},
		{
			name:    "relative path from current dir",
			path:    "../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "home directory itself",
			path:    homeDir,
			wantErr: true,
		},
		{
			name:    "claude directory but not plans",
			path:    filepath.Join(homeDir, ".claude", "config.json"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePlanPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePlanPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestHandleExitPlanMode_ParsesAllowedPrompts(t *testing.T) {
	planApprovalChan := make(chan PlanApprovalRequest, 1)
	planResponseChan := make(chan PlanApprovalResponse, 1)
	var buf strings.Builder

	s := &Server{
		planApprovalChan: planApprovalChan,
		planResponseChan: planResponseChan,
		writer:           &buf,
		log:              logger.WithSession("test"),
	}

	arguments := map[string]interface{}{
		"plan": "Test plan",
		"allowedPrompts": []interface{}{
			map[string]interface{}{
				"tool":   "Bash",
				"prompt": "run tests",
			},
			map[string]interface{}{
				"tool":   "Bash",
				"prompt": "build project",
			},
		},
	}

	go func() {
		req := <-planApprovalChan
		if len(req.AllowedPrompts) != 2 {
			t.Errorf("Expected 2 allowed prompts, got %d", len(req.AllowedPrompts))
		}
		if req.AllowedPrompts[0].Tool != "Bash" {
			t.Errorf("Expected first prompt tool to be 'Bash', got %q", req.AllowedPrompts[0].Tool)
		}
		if req.AllowedPrompts[0].Prompt != "run tests" {
			t.Errorf("Expected first prompt to be 'run tests', got %q", req.AllowedPrompts[0].Prompt)
		}
		planResponseChan <- PlanApprovalResponse{ID: req.ID, Approved: true}
	}()

	s.handleExitPlanMode("test-id", arguments)
}
