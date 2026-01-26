package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/mcp"
)

// =============================================================================
// Theme-Aware Syntax Highlighting Tests
// =============================================================================

func TestTheme_GetSyntaxStyle(t *testing.T) {
	tests := []struct {
		themeName     ThemeName
		expectedStyle string
	}{
		{ThemeDarkPurple, "monokai"},
		{ThemeNord, "nord"},
		{ThemeDracula, "dracula"},
		{ThemeGruvbox, "gruvbox"},
		{ThemeTokyoNight, "native"},
		{ThemeCatppuccin, "catppuccin-mocha"},
		{ThemeScienceFiction, "native"},
		{ThemeLight, "github"},
	}

	for _, tt := range tests {
		t.Run(string(tt.themeName), func(t *testing.T) {
			theme := GetTheme(tt.themeName)
			style := theme.GetSyntaxStyle()
			if style != tt.expectedStyle {
				t.Errorf("Theme %s: GetSyntaxStyle() = %q, want %q", tt.themeName, style, tt.expectedStyle)
			}
		})
	}
}

func TestTheme_GetSyntaxStyle_DefaultFallback(t *testing.T) {
	// Create a theme with empty SyntaxStyle to test fallback
	theme := Theme{
		Name:        "Test Theme",
		SyntaxStyle: "", // Empty should fallback to "monokai"
	}

	style := theme.GetSyntaxStyle()
	if style != "monokai" {
		t.Errorf("GetSyntaxStyle() with empty SyntaxStyle = %q, want %q", style, "monokai")
	}
}

func TestHighlightCode_UsesCurrentThemeSyntaxStyle(t *testing.T) {
	// Save the current theme to restore it later
	originalThemeName := CurrentThemeName()
	defer SetTheme(originalThemeName)

	code := `func main() {
	fmt.Println("Hello, World!")
}`

	// Test with different themes and verify output differs
	themes := []ThemeName{ThemeDarkPurple, ThemeNord, ThemeDracula, ThemeLight}
	outputs := make(map[ThemeName]string)

	for _, themeName := range themes {
		SetTheme(themeName)
		output := highlightCode(code, "go")
		outputs[themeName] = output

		// Basic sanity check - output should contain the code
		if !strings.Contains(output, "main") {
			t.Errorf("Theme %s: highlightCode output should contain 'main', got %q", themeName, output)
		}
	}

	// Verify that at least some themes produce different output
	// (Light vs Dark themes typically have different ANSI codes)
	if outputs[ThemeDarkPurple] == outputs[ThemeLight] {
		// This is not necessarily an error, but worth noting
		// Different chroma styles may produce similar output for simple code
		t.Log("Note: Dark Purple and Light themes produced identical output for test code")
	}
}

func TestHighlightCode_WithAllThemes(t *testing.T) {
	// Save the current theme to restore it later
	originalThemeName := CurrentThemeName()
	defer SetTheme(originalThemeName)

	testCases := []struct {
		language string
		code     string
	}{
		{"go", "package main\nfunc main() { }"},
		{"python", "def hello():\n    print('world')"},
		{"javascript", "const x = () => console.log('hi')"},
		{"rust", "fn main() { println!(\"Hello\"); }"},
	}

	// Test all themes with all languages
	for _, themeName := range ThemeNames() {
		SetTheme(themeName)

		for _, tc := range testCases {
			t.Run(string(themeName)+"/"+tc.language, func(t *testing.T) {
				output := highlightCode(tc.code, tc.language)

				// Basic sanity check - output should not be empty
				if output == "" {
					t.Errorf("highlightCode returned empty output for theme %s, language %s", themeName, tc.language)
				}

				// Output should contain some part of the original code
				// (may have ANSI codes interspersed)
				if len(output) < len(tc.code)/2 {
					t.Errorf("highlightCode output seems too short for theme %s, language %s", themeName, tc.language)
				}
			})
		}
	}
}

func TestHighlightCode_FallbackLexer(t *testing.T) {
	// Test with unknown language - should use fallback lexer
	output := highlightCode("some random code", "unknownlang123")

	if output == "" {
		t.Error("highlightCode should return non-empty output even with unknown language")
	}

	if !strings.Contains(output, "random") {
		t.Errorf("highlightCode output should contain original code, got %q", output)
	}
}

func TestRenderMarkdown_CodeBlockUsesThemeSyntaxStyle(t *testing.T) {
	// Save the current theme to restore it later
	originalThemeName := CurrentThemeName()
	defer SetTheme(originalThemeName)

	markdown := "```go\nfunc hello() {}\n```"

	// Test with different themes
	SetTheme(ThemeDarkPurple)
	output1 := renderMarkdown(markdown, 80)

	SetTheme(ThemeNord)
	output2 := renderMarkdown(markdown, 80)

	// Both outputs should contain the function name
	if !strings.Contains(output1, "hello") {
		t.Error("Dark Purple theme: renderMarkdown should contain 'hello'")
	}
	if !strings.Contains(output2, "hello") {
		t.Error("Nord theme: renderMarkdown should contain 'hello'")
	}
}

func TestAllBuiltinThemes_HaveSyntaxStyle(t *testing.T) {
	for name, theme := range BuiltinThemes {
		t.Run(string(name), func(t *testing.T) {
			if theme.SyntaxStyle == "" {
				t.Errorf("Theme %s has empty SyntaxStyle field", name)
			}

			// Verify the style is a known chroma style
			// (GetSyntaxStyle will fallback to monokai for unknown styles)
			style := theme.GetSyntaxStyle()
			if style == "" {
				t.Errorf("Theme %s: GetSyntaxStyle returned empty string", name)
			}
		})
	}
}

func TestGetToolIcon(t *testing.T) {
	tests := []struct {
		toolName string
		expected string
	}{
		{"Read", "Reading"},
		{"Edit", "Editing"},
		{"Write", "Writing"},
		{"Glob", "Searching"},
		{"Grep", "Searching"},
		{"Bash", "Running"},
		{"Task", "Delegating"},
		{"WebFetch", "Fetching"},
		{"WebSearch", "Searching"},
		{"TodoWrite", "Planning"},
		{"UnknownTool", "Using"},
		{"", "Using"},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			result := GetToolIcon(tt.toolName)
			if result != tt.expected {
				t.Errorf("GetToolIcon(%q) = %q, want %q", tt.toolName, result, tt.expected)
			}
		})
	}
}

func TestWrapText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		width    int
		expected string
	}{
		{
			name:     "short text within width",
			text:     "hello world",
			width:    20,
			expected: "hello world",
		},
		{
			name:     "long text needs wrap",
			text:     "this is a longer text that needs wrapping",
			width:    20,
			expected: "this is a longer\ntext that needs\nwrapping",
		},
		{
			name:     "zero width returns original",
			text:     "hello world",
			width:    0,
			expected: "hello world",
		},
		{
			name:     "negative width returns original",
			text:     "hello world",
			width:    -1,
			expected: "hello world",
		},
		{
			name:     "empty string",
			text:     "",
			width:    20,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapText(tt.text, tt.width)
			if result != tt.expected {
				t.Errorf("wrapText(%q, %d) = %q, want %q", tt.text, tt.width, result, tt.expected)
			}
		})
	}
}

func TestWrapTextWithANSI(t *testing.T) {
	// Test that ANSI escape codes don't affect visible line length calculation
	// The tool use line "○ Searching(Grep: pattern...)" was wrapping incorrectly
	// because ANSI codes were being counted towards the line width

	// ANSI escape code for cyan text
	cyan := "\x1b[36m"
	reset := "\x1b[0m"

	tests := []struct {
		name          string
		text          string
		width         int
		shouldNotWrap bool // if true, result should have no newlines
	}{
		{
			name:          "styled text within visible width should not wrap",
			text:          cyan + "Searching" + reset + "(Grep: short)",
			width:         40,
			shouldNotWrap: true,
		},
		{
			name:          "tool use line with ANSI codes",
			text:          "○ " + cyan + "Searching" + reset + "(Grep: pattern...)",
			width:         80,
			shouldNotWrap: true,
		},
		{
			name:          "multiple ANSI codes should not affect wrapping",
			text:          cyan + "First" + reset + " " + cyan + "Second" + reset + " " + cyan + "Third" + reset,
			width:         30,
			shouldNotWrap: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapText(tt.text, tt.width)
			hasNewline := strings.Contains(result, "\n")

			if tt.shouldNotWrap && hasNewline {
				t.Errorf("wrapText with ANSI codes should not have wrapped at width %d\nInput: %q\nOutput: %q",
					tt.width, tt.text, result)
			}
		})
	}
}

func TestHighlightDiff(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(string) bool
	}{
		{
			name:  "empty diff",
			input: "",
			check: func(result string) bool { return result == "" },
		},
		{
			name:  "added lines have color",
			input: "+added line",
			check: func(result string) bool { return strings.Contains(result, "+added line") },
		},
		{
			name:  "removed lines have color",
			input: "-removed line",
			check: func(result string) bool { return strings.Contains(result, "-removed line") },
		},
		{
			name:  "hunk markers have color",
			input: "@@ -1,3 +1,4 @@",
			check: func(result string) bool { return strings.Contains(result, "@@ -1,3 +1,4 @@") },
		},
		{
			name:  "file headers have color",
			input: "--- a/file.go\n+++ b/file.go",
			check: func(result string) bool {
				return strings.Contains(result, "--- a/file.go") && strings.Contains(result, "+++ b/file.go")
			},
		},
		{
			name:  "diff command header",
			input: "diff --git a/file.go b/file.go",
			check: func(result string) bool { return strings.Contains(result, "diff --git") },
		},
		{
			name:  "index line",
			input: "index abc123..def456 100644",
			check: func(result string) bool { return strings.Contains(result, "index abc123") },
		},
		{
			name:  "context lines unchanged",
			input: " unchanged line",
			check: func(result string) bool { return strings.Contains(result, " unchanged line") },
		},
		{
			name:  "new file mode",
			input: "new file mode 100644",
			check: func(result string) bool { return strings.Contains(result, "new file mode") },
		},
		{
			name:  "deleted file mode",
			input: "deleted file mode 100644",
			check: func(result string) bool { return strings.Contains(result, "deleted file mode") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HighlightDiff(tt.input)
			if !tt.check(result) {
				t.Errorf("HighlightDiff(%q) = %q, check failed", tt.input, result)
			}
		})
	}
}

func TestHighlightDiff_MultiLine(t *testing.T) {
	diff := `diff --git a/file.go b/file.go
index abc123..def456 100644
--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 context line
-removed line
+added line
 another context`

	result := HighlightDiff(diff)

	// Verify all parts are present
	if !strings.Contains(result, "diff --git") {
		t.Error("Expected diff header in result")
	}
	if !strings.Contains(result, "index abc123") {
		t.Error("Expected index line in result")
	}
	if !strings.Contains(result, "--- a/file.go") {
		t.Error("Expected file header in result")
	}
	if !strings.Contains(result, "@@ -1,3 +1,4 @@") {
		t.Error("Expected hunk marker in result")
	}
	if !strings.Contains(result, "-removed line") {
		t.Error("Expected removed line in result")
	}
	if !strings.Contains(result, "+added line") {
		t.Error("Expected added line in result")
	}
}

func TestRenderMarkdownLine(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		width int
		check func(string) bool
	}{
		{
			name:  "h1 header",
			line:  "# Header One",
			width: 80,
			check: func(s string) bool { return strings.Contains(s, "Header One") },
		},
		{
			name:  "h2 header",
			line:  "## Header Two",
			width: 80,
			check: func(s string) bool { return strings.Contains(s, "Header Two") },
		},
		{
			name:  "h3 header",
			line:  "### Header Three",
			width: 80,
			check: func(s string) bool { return strings.Contains(s, "Header Three") },
		},
		{
			name:  "h4 header",
			line:  "#### Header Four",
			width: 80,
			check: func(s string) bool { return strings.Contains(s, "Header Four") },
		},
		{
			name:  "horizontal rule dash",
			line:  "---",
			width: 80,
			check: func(s string) bool { return strings.Contains(s, "─") },
		},
		{
			name:  "horizontal rule asterisk",
			line:  "***",
			width: 80,
			check: func(s string) bool { return strings.Contains(s, "─") },
		},
		{
			name:  "horizontal rule underscore",
			line:  "___",
			width: 80,
			check: func(s string) bool { return strings.Contains(s, "─") },
		},
		{
			name:  "blockquote",
			line:  "> This is a quote",
			width: 80,
			check: func(s string) bool { return strings.Contains(s, "This is a quote") },
		},
		{
			name:  "unordered list dash",
			line:  "- List item",
			width: 80,
			check: func(s string) bool { return strings.Contains(s, "•") && strings.Contains(s, "List item") },
		},
		{
			name:  "unordered list asterisk",
			line:  "* List item",
			width: 80,
			check: func(s string) bool { return strings.Contains(s, "•") && strings.Contains(s, "List item") },
		},
		{
			name:  "numbered list",
			line:  "1. First item",
			width: 80,
			check: func(s string) bool { return strings.Contains(s, "1.") && strings.Contains(s, "First item") },
		},
		{
			name:  "regular text",
			line:  "This is regular text",
			width: 80,
			check: func(s string) bool { return strings.Contains(s, "This is regular text") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderMarkdownLine(tt.line, tt.width)
			if !tt.check(result) {
				t.Errorf("renderMarkdownLine(%q, %d) = %q, check failed", tt.line, tt.width, result)
			}
		})
	}
}

func TestRenderInlineMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		check func(string) bool
	}{
		{
			name:  "bold text",
			line:  "This is **bold** text",
			check: func(s string) bool { return strings.Contains(s, "bold") },
		},
		{
			name:  "inline code",
			line:  "Use `code` here",
			check: func(s string) bool { return strings.Contains(s, "code") },
		},
		{
			name: "link",
			line: "Click [here](https://example.com)",
			// The link is formatted with styled text and URL, contains ANSI codes
			// Just check that Click and example.com are present (may have ANSI between chars)
			check: func(s string) bool { return strings.Contains(s, "Click") },
		},
		{
			name:  "tool use in progress marker",
			line:  "○ Working",
			check: func(s string) bool { return strings.Contains(s, "○") },
		},
		{
			name:  "tool use complete marker",
			line:  "● Done",
			check: func(s string) bool { return strings.Contains(s, "●") },
		},
		{
			name:  "plain text unchanged",
			line:  "Just plain text",
			check: func(s string) bool { return strings.Contains(s, "Just plain text") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderInlineMarkdown(tt.line)
			if !tt.check(result) {
				t.Errorf("renderInlineMarkdown(%q) = %q, check failed", tt.line, result)
			}
		})
	}
}

func TestRenderMarkdown(t *testing.T) {
	tests := []struct {
		name    string
		content string
		width   int
		check   func(string) bool
	}{
		{
			name:    "simple text",
			content: "Hello world",
			width:   80,
			check:   func(s string) bool { return strings.Contains(s, "Hello world") },
		},
		{
			name:    "code block",
			content: "```go\nfunc main() {}\n```",
			width:   80,
			// Code blocks use syntax highlighting, so check for "main" which should be highlighted
			check: func(s string) bool { return strings.Contains(s, "main") },
		},
		{
			name:    "mixed content",
			content: "# Title\n\nSome text\n\n```python\nprint('hi')\n```\n\nMore text",
			width:   80,
			check: func(s string) bool {
				return strings.Contains(s, "Title") && strings.Contains(s, "print")
			},
		},
		{
			name:    "zero width uses default",
			content: "Test content",
			width:   0,
			check:   func(s string) bool { return strings.Contains(s, "Test content") },
		},
		{
			name:    "unclosed code block",
			content: "```go\nsome code",
			width:   80,
			// Check for "code" which should be present even in highlighted output
			check: func(s string) bool { return strings.Contains(s, "code") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderMarkdown(tt.content, tt.width)
			if !tt.check(result) {
				t.Errorf("renderMarkdown check failed for %s, got: %q", tt.name, result)
			}
		})
	}
}

func TestIsTableRow(t *testing.T) {
	tests := []struct {
		line     string
		expected bool
	}{
		{"|a|b|c|", true},
		{"| a | b | c |", true},
		{"|Theme|Old|New|", true},
		{"| Theme | Old | New |", true},
		{"|---|---|---|", true},
		{"|:---|:---:|---:|", true},
		{"not a table", false},
		{"|only one pipe", false},
		{"no pipes at all", false},
		{"", false},
		{"|", false},
		{"||", false},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			result := isTableRow(tt.line)
			if result != tt.expected {
				t.Errorf("isTableRow(%q) = %v, want %v", tt.line, result, tt.expected)
			}
		})
	}
}

func TestIsTableSeparator(t *testing.T) {
	tests := []struct {
		line     string
		expected bool
	}{
		{"|---|---|---|", true},
		{"| --- | --- | --- |", true},
		{"|:---|:---:|---:|", true},
		{"| :--- | :---: | ---: |", true},
		{"|-----|-----|-----|", true},
		{"|a|b|c|", false},
		{"| Theme | Old | New |", false},
		{"not a separator", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			result := isTableSeparator(tt.line)
			if result != tt.expected {
				t.Errorf("isTableSeparator(%q) = %v, want %v", tt.line, result, tt.expected)
			}
		})
	}
}

func TestParseTableRow(t *testing.T) {
	tests := []struct {
		line     string
		expected []string
	}{
		{"|a|b|c|", []string{"a", "b", "c"}},
		{"| a | b | c |", []string{"a", "b", "c"}},
		{"|Theme|Old|New|", []string{"Theme", "Old", "New"}},
		{"| Theme | Old | New |", []string{"Theme", "Old", "New"}},
		{"|  spaced  |  content  |", []string{"spaced", "content"}},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			result := parseTableRow(tt.line)
			if len(result) != len(tt.expected) {
				t.Errorf("parseTableRow(%q) returned %d cells, want %d", tt.line, len(result), len(tt.expected))
				return
			}
			for i, cell := range result {
				if cell != tt.expected[i] {
					t.Errorf("parseTableRow(%q)[%d] = %q, want %q", tt.line, i, cell, tt.expected[i])
				}
			}
		})
	}
}

func TestRenderTable(t *testing.T) {
	tests := []struct {
		name      string
		rows      [][]string
		hasHeader bool
		checks    []string // strings that should be present in output
	}{
		{
			name:      "simple table without header",
			rows:      [][]string{{"a", "b"}, {"c", "d"}},
			hasHeader: false,
			checks:    []string{"a", "b", "c", "d", "┌", "┐", "└", "┘", "│"},
		},
		{
			name:      "table with header",
			rows:      [][]string{{"Name", "Value"}, {"foo", "bar"}},
			hasHeader: true,
			checks:    []string{"Name", "Value", "foo", "bar", "├", "┤", "┼"},
		},
		{
			name:      "single row table",
			rows:      [][]string{{"only", "row"}},
			hasHeader: false,
			checks:    []string{"only", "row"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderTable(tt.rows, tt.hasHeader, 80)
			for _, check := range tt.checks {
				if !strings.Contains(result, check) {
					t.Errorf("renderTable output should contain %q, got: %q", check, result)
				}
			}
		})
	}
}

func TestRenderTableWithLongContent(t *testing.T) {
	// Test that tables with long cell content are wrapped properly
	rows := [][]string{
		{"Item", "Description"},
		{"Short", "This is a very long description that should be wrapped to fit within the available width of the table cell"},
	}
	result := renderTable(rows, true, 60) // 60 char width should force wrapping

	// The table should contain all the content
	if !strings.Contains(result, "Item") {
		t.Errorf("Table should contain 'Item'")
	}
	if !strings.Contains(result, "Description") {
		t.Errorf("Table should contain 'Description'")
	}
	if !strings.Contains(result, "Short") {
		t.Errorf("Table should contain 'Short'")
	}

	// Each line should not exceed the width (accounting for some buffer)
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		// Use visual width to measure, not byte length
		visualWidth := lipgloss.Width(line)
		if visualWidth > 65 { // Allow some buffer for styling
			t.Errorf("Line %d exceeds width limit: %d chars: %q", i, visualWidth, line)
		}
	}

	// Table borders should be properly closed
	if !strings.Contains(result, "┌") || !strings.Contains(result, "┐") {
		t.Errorf("Table should have top border corners")
	}
	if !strings.Contains(result, "└") || !strings.Contains(result, "┘") {
		t.Errorf("Table should have bottom border corners")
	}
}

func TestRenderMarkdownWithTables(t *testing.T) {
	tests := []struct {
		name    string
		content string
		checks  []string
	}{
		{
			name: "simple table",
			content: `| Theme | Old | New |
|-------|-----|-----|
| Dark Purple | #9CA3AF | #B0B8C4 |`,
			checks: []string{"Theme", "Old", "New", "Dark Purple", "#9CA3AF", "#B0B8C4", "┌", "┐", "└", "┘"},
		},
		{
			name: "table with text before and after",
			content: `Here is a table:

| A | B |
|---|---|
| 1 | 2 |

And some text after.`,
			checks: []string{"Here is a table", "A", "B", "1", "2", "And some text after"},
		},
		{
			name: "multiple tables",
			content: `| First | Table |
|-------|-------|
| a | b |

Some text between.

| Second | Table |
|--------|-------|
| c | d |`,
			checks: []string{"First", "Table", "a", "b", "Second", "c", "d"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderMarkdown(tt.content, 80)
			for _, check := range tt.checks {
				if !strings.Contains(result, check) {
					t.Errorf("renderMarkdown output should contain %q, got: %q", check, result)
				}
			}
		})
	}
}

func TestRenderSpinner(t *testing.T) {
	verbs := []string{"Thinking", "Pondering", "Analyzing"}
	for _, verb := range verbs {
		for i := 0; i < len(spinnerFrames); i++ {
			result := renderSpinner(verb, i)
			if result == "" {
				t.Errorf("renderSpinner(%q, %d) returned empty string", verb, i)
			}
			if !strings.Contains(result, verb) {
				t.Errorf("renderSpinner(%q, %d) = %q, should contain verb", verb, i, result)
			}
			if !strings.Contains(result, "...") {
				t.Errorf("renderSpinner(%q, %d) = %q, should contain ellipsis", verb, i, result)
			}
		}
	}
}

func TestRandomThinkingVerb(t *testing.T) {
	// Call multiple times and verify we get valid verbs
	for i := 0; i < 100; i++ {
		verb := randomThinkingVerb()
		found := false
		for _, v := range thinkingVerbs {
			if v == verb {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("randomThinkingVerb returned invalid verb: %q", verb)
		}
	}
}

func TestChat_NewChat(t *testing.T) {
	chat := NewChat()

	if chat == nil {
		t.Fatal("NewChat() returned nil")
	}

	if chat.hasSession {
		t.Error("New chat should not have session")
	}

	if len(chat.messages) != 0 {
		t.Errorf("New chat should have 0 messages, got %d", len(chat.messages))
	}

	if chat.streaming != "" {
		t.Error("New chat should have empty streaming")
	}

	if chat.IsStreaming() {
		t.Error("New chat should not be streaming")
	}

	if chat.IsWaiting() {
		t.Error("New chat should not be waiting")
	}

	if chat.HasPendingPermission() {
		t.Error("New chat should not have pending permission")
	}

	if chat.HasPendingQuestion() {
		t.Error("New chat should not have pending question")
	}
}

func TestChat_SessionManagement(t *testing.T) {
	chat := NewChat()

	// Set session
	messages := []claude.Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
	}
	chat.SetSession("test-session", messages)

	if !chat.hasSession {
		t.Error("Chat should have session after SetSession")
	}

	if chat.sessionName != "test-session" {
		t.Errorf("Expected session name 'test-session', got %q", chat.sessionName)
	}

	if len(chat.messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(chat.messages))
	}

	// Clear session
	chat.ClearSession()

	if chat.hasSession {
		t.Error("Chat should not have session after ClearSession")
	}

	if len(chat.messages) != 0 {
		t.Errorf("Expected 0 messages after clear, got %d", len(chat.messages))
	}

	// Verify waiting state is also cleared
	if chat.waiting {
		t.Error("Chat waiting state should be cleared after ClearSession")
	}

	// Verify View shows "No session selected" message
	chat.SetSize(80, 24)
	view := chat.View()
	if !strings.Contains(view, "No session selected") {
		t.Errorf("View should contain 'No session selected' after ClearSession, got: %s", view)
	}
}

func TestChat_ClearSessionWhileWaiting(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)

	// Set up session with messages
	messages := []claude.Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
	}
	chat.SetSession("test-session", messages)

	// Simulate waiting state (Claude is thinking)
	chat.SetWaiting(true)
	if !chat.waiting {
		t.Error("Chat should be waiting after SetWaiting(true)")
	}

	// Clear session
	chat.ClearSession()

	// Verify all state is cleared
	if chat.hasSession {
		t.Error("Chat should not have session after ClearSession")
	}
	if chat.waiting {
		t.Error("Waiting state should be cleared after ClearSession")
	}
	if len(chat.messages) != 0 {
		t.Errorf("Expected 0 messages after clear, got %d", len(chat.messages))
	}

	// Verify View shows "No session selected" (not stuck on "Thinking...")
	view := chat.View()
	if !strings.Contains(view, "No session selected") {
		t.Errorf("View should contain 'No session selected' after ClearSession, got: %s", view)
	}
	if strings.Contains(view, "Thinking") || strings.Contains(view, "Claude:") {
		t.Error("View should not contain thinking indicator after ClearSession")
	}
}

func TestChat_Streaming(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)

	// Initially not streaming
	if chat.IsStreaming() {
		t.Error("Should not be streaming initially")
	}

	if chat.GetStreaming() != "" {
		t.Error("Streaming content should be empty initially")
	}

	// Append streaming content
	chat.AppendStreaming("Hello")
	if chat.GetStreaming() != "Hello" {
		t.Errorf("Expected streaming 'Hello', got %q", chat.GetStreaming())
	}

	if !chat.IsStreaming() {
		t.Error("Should be streaming after AppendStreaming")
	}

	chat.AppendStreaming(" world")
	if chat.GetStreaming() != "Hello world" {
		t.Errorf("Expected 'Hello world', got %q", chat.GetStreaming())
	}

	// Set streaming directly
	chat.SetStreaming("New content")
	if chat.GetStreaming() != "New content" {
		t.Errorf("Expected 'New content', got %q", chat.GetStreaming())
	}

	// Finish streaming
	chat.FinishStreaming()
	if chat.IsStreaming() {
		t.Error("Should not be streaming after FinishStreaming")
	}

	if len(chat.messages) != 1 {
		t.Errorf("Expected 1 message after finish, got %d", len(chat.messages))
	}

	if chat.messages[0].Content != "New content" {
		t.Errorf("Expected message content 'New content', got %q", chat.messages[0].Content)
	}
}

func TestChat_AppendPermissionDenials(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)

	// Test with empty denials - should not change streaming content
	chat.AppendPermissionDenials(nil)
	if chat.GetStreaming() != "" {
		t.Errorf("Expected empty streaming after nil denials, got %q", chat.GetStreaming())
	}

	chat.AppendPermissionDenials([]claude.PermissionDenial{})
	if chat.GetStreaming() != "" {
		t.Errorf("Expected empty streaming after empty denials, got %q", chat.GetStreaming())
	}

	// Test with actual denials
	denials := []claude.PermissionDenial{
		{Tool: "Bash", Description: "rm -rf /", Reason: "destructive command"},
		{Tool: "Edit", Description: "/etc/passwd"},
	}
	chat.AppendPermissionDenials(denials)

	streaming := chat.GetStreaming()
	if !strings.Contains(streaming, "[Permission Denials]") {
		t.Error("Streaming should contain '[Permission Denials]' header")
	}
	if !strings.Contains(streaming, "Bash: rm -rf /") {
		t.Error("Streaming should contain first denial")
	}
	if !strings.Contains(streaming, "destructive command") {
		t.Error("Streaming should contain denial reason")
	}
	if !strings.Contains(streaming, "Edit: /etc/passwd") {
		t.Error("Streaming should contain second denial")
	}
}

func TestChat_AppendPermissionDenials_FlushesToolUse(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)

	// Add a tool use first
	chat.AppendToolUse("Read", "file.go", "tool-123")

	// Append permission denials - should flush tool use rollup first
	denials := []claude.PermissionDenial{
		{Tool: "Bash", Description: "dangerous command"},
	}
	chat.AppendPermissionDenials(denials)

	streaming := chat.GetStreaming()
	// Tool use should appear before denials (flushed first)
	toolUsePos := strings.Index(streaming, "Read")
	denialsPos := strings.Index(streaming, "[Permission Denials]")

	if toolUsePos == -1 {
		t.Error("Streaming should contain tool use")
	}
	if denialsPos == -1 {
		t.Error("Streaming should contain permission denials")
	}
	if toolUsePos > denialsPos {
		t.Error("Tool use should appear before permission denials (flushed first)")
	}
}

func TestChat_ToolUseMarkers(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)

	// Append tool use - now goes to rollup, not directly to streaming
	chat.AppendToolUse("Read", "file.go", "tool-123")

	// Tool uses are now stored in rollup until text arrives
	rollup := chat.GetToolUseRollup()
	if rollup == nil || len(rollup.Items) != 1 {
		t.Fatal("Expected one item in rollup")
	}
	if rollup.Items[0].ToolName != "Read" {
		t.Errorf("Expected tool name 'Read', got %q", rollup.Items[0].ToolName)
	}
	if rollup.Items[0].ToolInput != "file.go" {
		t.Errorf("Expected tool input 'file.go', got %q", rollup.Items[0].ToolInput)
	}
	if rollup.Items[0].ToolUseID != "tool-123" {
		t.Errorf("Expected tool use ID 'tool-123', got %q", rollup.Items[0].ToolUseID)
	}
	if rollup.Items[0].Complete {
		t.Error("Expected tool use to be incomplete")
	}

	// Mark complete by ID
	chat.MarkToolUseComplete("tool-123", nil)

	rollup = chat.GetToolUseRollup()
	if !rollup.Items[0].Complete {
		t.Error("Expected tool use to be complete after MarkToolUseComplete")
	}

	// When text arrives, tool uses are flushed to streaming
	chat.AppendStreaming("File contents here")
	streaming := chat.GetStreaming()
	if !strings.Contains(streaming, ToolUseComplete) {
		t.Error("Expected complete marker in streaming after flush")
	}
	if !strings.Contains(streaming, "Reading") {
		t.Error("Expected 'Reading' icon in streaming after flush")
	}
	if !strings.Contains(streaming, "file.go") {
		t.Error("Expected 'file.go' in streaming after flush")
	}
}

func TestChat_ToolUseRollupResetOnFinishStreaming(t *testing.T) {
	// This tests that tool use rollup is flushed and reset when FinishStreaming is called,
	// preventing stale tool use state from affecting subsequent streaming content.
	chat := NewChat()
	chat.SetSession("test", nil)

	// Simulate a tool use during Claude response
	chat.AppendToolUse("Read", "file.go", "tool-123")

	// Tool use should be in rollup
	rollup := chat.GetToolUseRollup()
	if rollup == nil || len(rollup.Items) != 1 {
		t.Fatal("Expected tool use to be in rollup after AppendToolUse")
	}

	// Add some response text (flushes rollup)
	chat.AppendStreaming("File contents here\n")

	// Finish streaming (converts to message)
	chat.FinishStreaming()

	// After FinishStreaming, rollup should be nil
	if chat.GetToolUseRollup() != nil {
		t.Error("Expected rollup to be nil after FinishStreaming")
	}

	// Now simulate merge output (new streaming after previous response finished)
	chat.AppendStreaming("Merging branch...\n")
	chat.AppendStreaming("Checking out main...\n")
	chat.AppendStreaming("Already up to date.\n")

	// The streaming content should NOT have extra newlines inserted
	streaming := chat.GetStreaming()
	expected := "Merging branch...\nChecking out main...\nAlready up to date.\n"
	if streaming != expected {
		t.Errorf("Expected streaming content %q, got %q", expected, streaming)
	}
}

func TestChat_ToolUseRollupMultipleToolUses(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)

	// Add multiple tool uses with IDs
	chat.AppendToolUse("Read", "file1.go", "tool-1")
	chat.AppendToolUse("Read", "file2.go", "tool-2")
	chat.AppendToolUse("Edit", "file3.go", "tool-3")

	rollup := chat.GetToolUseRollup()
	if rollup == nil || len(rollup.Items) != 3 {
		t.Fatalf("Expected 3 items in rollup, got %v", rollup)
	}

	// Mark tool-2 as complete (out of order to test ID matching)
	chat.MarkToolUseComplete("tool-2", nil)
	if !rollup.Items[1].Complete {
		t.Error("Expected tool-2 (file2.go) to be complete")
	}
	if rollup.Items[0].Complete || rollup.Items[2].Complete {
		t.Error("Expected tool-1 and tool-3 to still be incomplete")
	}

	// Mark tool-1 as complete
	chat.MarkToolUseComplete("tool-1", nil)
	if !rollup.Items[0].Complete {
		t.Error("Expected tool-1 (file1.go) to be complete")
	}

	// Verify HasActiveToolUseRollup returns true for multiple items
	if !chat.HasActiveToolUseRollup() {
		t.Error("Expected HasActiveToolUseRollup to return true with 3 items")
	}
}

func TestChat_ToolUseRollupToggle(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)

	// Add multiple tool uses
	chat.AppendToolUse("Read", "file1.go", "tool-1")
	chat.AppendToolUse("Read", "file2.go", "tool-2")

	rollup := chat.GetToolUseRollup()
	if rollup.Expanded {
		t.Error("Expected rollup to start collapsed")
	}

	// Toggle to expanded
	chat.ToggleToolUseRollup()
	if !rollup.Expanded {
		t.Error("Expected rollup to be expanded after toggle")
	}

	// Toggle back to collapsed
	chat.ToggleToolUseRollup()
	if rollup.Expanded {
		t.Error("Expected rollup to be collapsed after second toggle")
	}
}

func TestChat_ToolUseRollupRenderCollapsed(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	// Set viewport width for renderToolUseRollup to work
	chat.SetSize(80, 40)

	// Add multiple tool uses
	chat.AppendToolUse("Read", "file1.go", "tool-1")
	chat.AppendToolUse("Read", "file2.go", "tool-2")
	chat.AppendToolUse("Edit", "main.go", "tool-3")

	// Render the rollup
	rendered := chat.renderToolUseRollup()

	// Should show the most recent tool (main.go) and a "+2 more" indicator
	if !strings.Contains(rendered, "main.go") {
		t.Error("Expected rendered rollup to contain 'main.go'")
	}
	if !strings.Contains(rendered, "+2 more tool uses") {
		t.Error("Expected rendered rollup to contain '+2 more tool uses'")
	}
	if !strings.Contains(rendered, "ctrl-t") {
		t.Error("Expected rendered rollup to contain 'ctrl-t' hint")
	}
	// Should NOT show the other files when collapsed
	if strings.Contains(rendered, "file1.go") || strings.Contains(rendered, "file2.go") {
		t.Error("Expected collapsed rollup to NOT show earlier tool uses")
	}
}

func TestChat_ToolUseRollupRenderExpanded(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	// Set viewport width for renderToolUseRollup to work
	chat.SetSize(80, 40)

	// Add multiple tool uses
	chat.AppendToolUse("Read", "file1.go", "tool-1")
	chat.AppendToolUse("Read", "file2.go", "tool-2")
	chat.AppendToolUse("Edit", "main.go", "tool-3")

	// Expand the rollup
	chat.ToggleToolUseRollup()

	// Render the rollup
	rendered := chat.renderToolUseRollup()

	// Should show all tool uses when expanded
	if !strings.Contains(rendered, "main.go") {
		t.Error("Expected expanded rollup to contain 'main.go'")
	}
	if !strings.Contains(rendered, "file1.go") {
		t.Error("Expected expanded rollup to contain 'file1.go'")
	}
	if !strings.Contains(rendered, "file2.go") {
		t.Error("Expected expanded rollup to contain 'file2.go'")
	}
}

func TestChat_ToolUseRollupSingleItem(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(80, 40)

	// Add single tool use
	chat.AppendToolUse("Read", "file.go", "tool-1")

	// Should NOT have active rollup (need > 1 item)
	if chat.HasActiveToolUseRollup() {
		t.Error("Expected HasActiveToolUseRollup to return false with 1 item")
	}

	// Render should still work but not show "+N more" message
	rendered := chat.renderToolUseRollup()
	if !strings.Contains(rendered, "file.go") {
		t.Error("Expected rendered rollup to contain 'file.go'")
	}
	if strings.Contains(rendered, "+") {
		t.Error("Expected single-item rollup to NOT show '+N more' message")
	}
}

func TestChat_ToolUseRollupFlushOnText(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)

	// Add tool uses
	chat.AppendToolUse("Read", "file1.go", "tool-1")
	chat.AppendToolUse("Read", "file2.go", "tool-2")

	// Verify rollup exists
	if chat.GetToolUseRollup() == nil {
		t.Fatal("Expected rollup to exist before text")
	}

	// Append text - should flush rollup
	chat.AppendStreaming("Here are the file contents")

	// Rollup should be cleared
	if chat.GetToolUseRollup() != nil {
		t.Error("Expected rollup to be nil after text flush")
	}

	// Streaming should contain both tool uses
	streaming := chat.GetStreaming()
	if !strings.Contains(streaming, "file1.go") {
		t.Error("Expected streaming to contain 'file1.go'")
	}
	if !strings.Contains(streaming, "file2.go") {
		t.Error("Expected streaming to contain 'file2.go'")
	}
}

// TestChat_ToolUseRollupWhitespaceSeparation verifies that tool use rollup
// is properly separated from streaming text content with a newline
func TestChat_ToolUseRollupWhitespaceSeparation(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(80, 40)

	// Add streaming text content
	chat.streaming = "Looking at the codebase, I need to search for any syntax highlighting implementation."

	// Add tool use - this should appear on a new line, not concatenated with the text
	chat.AppendToolUse("Grep", "code.*block|```", "tool-1")

	// The key behavior: when we have both streaming content and a tool use rollup,
	// the tool use rollup should be rendered on its own line, not concatenated
	// with the streaming text.

	// Verify the rollup contains the in-progress marker (may have ANSI styling)
	rollup := chat.renderToolUseRollup()
	if !strings.Contains(rollup, ToolUseInProgress) {
		t.Errorf("Expected rollup to contain in-progress marker, got: %q", rollup)
	}

	// The streaming text should NOT have a trailing newline added by AppendToolUse
	// (the newline separation is handled in updateContent, not in streaming content)
	if strings.HasSuffix(chat.streaming, "\n") {
		t.Error("Expected streaming content to not have trailing newline (separation is done during render)")
	}

	// Verify that when content is updated, the rendered output has proper separation
	// by checking that updateContent adds a newline before the rollup
	chat.updateContent()

	// Get the viewport content which should have the proper formatting
	viewportContent := chat.viewport.View()

	// The text "implementation." should appear, followed eventually by the tool marker
	// They should NOT be on the same line (there should be a newline between them)
	if strings.Contains(viewportContent, "implementation."+ToolUseInProgress) {
		t.Error("Expected newline between streaming text and tool use marker, but they appear concatenated")
	}
}

// TestChat_ToolUseFlushedNewlineSeparation verifies that when tool uses are
// flushed to streaming content and followed by text, there's a proper newline
// separator between the tool use output and the following text.
// This is a regression test for the bug where tool uses and following text
// were squished together without a separator.
func TestChat_ToolUseFlushedNewlineSeparation(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(80, 40)

	// Simulate a sequence: tool use runs, then text follows
	// This is what happens when Claude does a search, then comments on results
	chat.AppendToolUse("Grep", "HighlightDiff", "tool-1")
	chat.MarkToolUseComplete("tool-1", nil)

	// Now text arrives - this triggers flushToolUseRollup
	chat.AppendStreaming("Yes, there is syntax highlighting for diffs")

	streaming := chat.GetStreaming()

	// The streaming content should have:
	// 1. The tool use line ending with )\n
	// 2. An extra newline for separation
	// 3. The text content

	// Check that there are two consecutive newlines between tool output and text
	// The tool use line ends with ")\n" and we should have an extra "\n" before text
	if !strings.Contains(streaming, ")\n\n") {
		t.Errorf("Expected double newline after tool use, got streaming: %q", streaming)
	}

	// Check that the text is NOT directly concatenated with the closing paren
	if strings.Contains(streaming, ")Yes") {
		t.Error("Text should not be concatenated directly with tool use closing paren")
	}
}

// TestChat_ToolUseFlushBlankLineBeforeToolUses verifies that when text precedes
// tool uses and the tool uses are flushed, there's a blank line (two newlines)
// between the text and the tool uses for visual separation.
func TestChat_ToolUseFlushBlankLineBeforeToolUses(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(80, 40)

	// Simulate Claude sending text first
	chat.AppendStreaming("Let me search for the implementation.")

	// Then Claude does tool uses (these go into the rollup)
	chat.AppendToolUse("Grep", "somePattern", "tool-1")
	chat.MarkToolUseComplete("tool-1", nil)

	// Now more text arrives - this triggers flushToolUseRollup
	chat.AppendStreaming("Found it!")

	streaming := chat.GetStreaming()

	// The streaming content should have:
	// 1. First text ending with period
	// 2. A blank line (\n\n) before tool uses
	// 3. The tool use line
	// 4. A blank line (\n\n) after tool uses
	// 5. The second text

	// Check for blank line BEFORE tool uses (text ends, then double newline)
	if !strings.Contains(streaming, ".\n\n"+ToolUseComplete) {
		t.Errorf("Expected blank line (double newline) before tool use, got streaming: %q", streaming)
	}

	// Also verify the blank line AFTER tool uses is still there
	if !strings.Contains(streaming, ")\n\nFound") {
		t.Errorf("Expected blank line (double newline) after tool use, got streaming: %q", streaming)
	}
}

// TestChat_ToolUseFlushNormalizesTrailingNewlines verifies that the flush
// normalizes various trailing newline patterns to exactly one blank line.
func TestChat_ToolUseFlushNormalizesTrailingNewlines(t *testing.T) {
	tests := []struct {
		name            string
		initialStreaming string
		wantBlankLine   bool
	}{
		{
			name:            "no trailing newline",
			initialStreaming: "Some text",
			wantBlankLine:   true,
		},
		{
			name:            "single trailing newline",
			initialStreaming: "Some text\n",
			wantBlankLine:   true,
		},
		{
			name:            "already has blank line",
			initialStreaming: "Some text\n\n",
			wantBlankLine:   true,
		},
		{
			name:            "multiple trailing newlines",
			initialStreaming: "Some text\n\n\n",
			wantBlankLine:   true,
		},
		{
			name:            "empty streaming",
			initialStreaming: "",
			wantBlankLine:   false, // no blank line needed when empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chat := NewChat()
			chat.SetSession("test", nil)
			chat.SetSize(80, 40)

			// Set initial streaming content
			chat.streaming = tt.initialStreaming

			// Add tool use and flush it by sending more text
			chat.AppendToolUse("Read", "file.go", "tool-1")
			chat.AppendStreaming("Next text")

			streaming := chat.GetStreaming()

			if tt.wantBlankLine {
				// Should have exactly one blank line before tool use
				// Note: we use ToolUseInProgress because the tool is not marked complete before flush
				expectedPrefix := strings.TrimRight(tt.initialStreaming, "\n") + "\n\n" + ToolUseInProgress
				if !strings.Contains(streaming, expectedPrefix) {
					t.Errorf("Expected normalized blank line before tool use.\nInitial: %q\nGot streaming: %q\nWanted prefix: %q",
						tt.initialStreaming, streaming, expectedPrefix)
				}
			}
		})
	}
}

// TestChat_ToolUseCompleteByID verifies that tool uses are marked complete
// by their ID, not by position. This is critical for parallel tool uses
// where results may arrive out of order.
func TestChat_ToolUseCompleteByID(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)

	// Simulate parallel tool uses (3 reads kicked off simultaneously)
	chat.AppendToolUse("Read", "file1.go", "tool-aaa")
	chat.AppendToolUse("Read", "file2.go", "tool-bbb")
	chat.AppendToolUse("Read", "file3.go", "tool-ccc")

	rollup := chat.GetToolUseRollup()
	if rollup == nil || len(rollup.Items) != 3 {
		t.Fatalf("Expected 3 items in rollup, got %v", rollup)
	}

	// Results arrive out of order: file2 completes first
	chat.MarkToolUseComplete("tool-bbb", nil)
	if !rollup.Items[1].Complete {
		t.Error("Expected tool-bbb (file2.go) to be marked complete")
	}
	if rollup.Items[0].Complete || rollup.Items[2].Complete {
		t.Error("Expected tool-aaa and tool-ccc to still be incomplete")
	}

	// file3 completes second
	chat.MarkToolUseComplete("tool-ccc", nil)
	if !rollup.Items[2].Complete {
		t.Error("Expected tool-ccc (file3.go) to be marked complete")
	}
	if rollup.Items[0].Complete {
		t.Error("Expected tool-aaa to still be incomplete")
	}

	// file1 completes last
	chat.MarkToolUseComplete("tool-aaa", nil)
	if !rollup.Items[0].Complete {
		t.Error("Expected tool-aaa (file1.go) to be marked complete")
	}

	// All should be complete now
	for i, item := range rollup.Items {
		if !item.Complete {
			t.Errorf("Expected item %d to be complete", i)
		}
	}
}

// TestChat_ToolUseCompleteUnknownIDFallback verifies that when a tool_result
// arrives with an unknown ID (or empty ID), we fall back to marking the first
// incomplete tool use as complete.
func TestChat_ToolUseCompleteUnknownIDFallback(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)

	// Add tool uses
	chat.AppendToolUse("Read", "file1.go", "tool-1")
	chat.AppendToolUse("Read", "file2.go", "tool-2")

	rollup := chat.GetToolUseRollup()

	// Complete with empty ID - should mark first incomplete
	chat.MarkToolUseComplete("", nil)
	if !rollup.Items[0].Complete {
		t.Error("Expected first item to be complete (fallback behavior)")
	}
	if rollup.Items[1].Complete {
		t.Error("Expected second item to still be incomplete")
	}

	// Complete with unknown ID - should mark first incomplete (which is now item 1)
	chat.MarkToolUseComplete("unknown-id", nil)
	if !rollup.Items[1].Complete {
		t.Error("Expected second item to be complete (fallback behavior)")
	}
}

func TestChat_ToolUseWithResultInfo(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 40)
	chat.SetSession("test", nil)

	// Add a tool use
	chat.AppendToolUse("Read", "file.go", "tool-123")

	// Mark it complete with result info
	resultInfo := &claude.ToolResultInfo{
		FilePath:   "/path/to/file.go",
		NumLines:   45,
		StartLine:  1,
		TotalLines: 138,
	}
	chat.MarkToolUseComplete("tool-123", resultInfo)

	// Check that the rollup has the result info
	rollup := chat.GetToolUseRollup()
	if rollup == nil {
		t.Fatal("Expected rollup to exist")
	}
	if len(rollup.Items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(rollup.Items))
	}
	if rollup.Items[0].ResultInfo == nil {
		t.Fatal("Expected ResultInfo to be set")
	}
	if rollup.Items[0].ResultInfo.NumLines != 45 {
		t.Errorf("Expected NumLines 45, got %d", rollup.Items[0].ResultInfo.NumLines)
	}

	// Render and check for the result info in output
	rendered := chat.renderToolUseRollup()
	if !strings.Contains(rendered, "→ lines 1-45 of 138") {
		t.Errorf("Expected rendered output to contain '→ lines 1-45 of 138', got: %s", rendered)
	}
}

func TestChat_ToolUseFlushWithResultInfo(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 40)
	chat.SetSession("test", nil)

	// Add a tool use
	chat.AppendToolUse("Bash", "ls -la", "tool-456")

	// Mark it complete with result info (exit code 0)
	exitCode := 0
	resultInfo := &claude.ToolResultInfo{
		ExitCode: &exitCode,
	}
	chat.MarkToolUseComplete("tool-456", resultInfo)

	// Now append text to trigger flush
	chat.AppendStreaming("Here's the output:")

	// The streaming content should include the result info
	if !strings.Contains(chat.streaming, "→ success") {
		t.Errorf("Expected streaming to contain '→ success', got: %s", chat.streaming)
	}
}

func TestChat_UserMessage(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)

	chat.AddUserMessage("Hello, Claude!")

	if len(chat.messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(chat.messages))
	}

	if chat.messages[0].Role != "user" {
		t.Errorf("Expected role 'user', got %q", chat.messages[0].Role)
	}

	if chat.messages[0].Content != "Hello, Claude!" {
		t.Errorf("Expected content 'Hello, Claude!', got %q", chat.messages[0].Content)
	}
}

func TestChat_Input(t *testing.T) {
	chat := NewChat()

	// Set input
	chat.SetInput("test input")
	input := chat.GetInput()
	if input != "test input" {
		t.Errorf("Expected 'test input', got %q", input)
	}

	// Clear input
	chat.ClearInput()
	input = chat.GetInput()
	if input != "" {
		t.Errorf("Expected empty input after clear, got %q", input)
	}
}

func TestChat_Waiting(t *testing.T) {
	chat := NewChat()

	// Initially not waiting
	if chat.IsWaiting() {
		t.Error("Should not be waiting initially")
	}

	// Set waiting
	chat.SetWaiting(true)
	if !chat.IsWaiting() {
		t.Error("Should be waiting after SetWaiting(true)")
	}

	// Should have a random verb
	if chat.spinner.Verb == "" {
		t.Error("Expected waiting verb to be set")
	}

	// Clear waiting
	chat.SetWaiting(false)
	if chat.IsWaiting() {
		t.Error("Should not be waiting after SetWaiting(false)")
	}
}

// TestFormatElapsed verifies the elapsed time formatting
func TestFormatElapsed(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"zero duration", 0, "0s"},
		{"under a minute", 45 * time.Second, "45s"},
		{"exactly one minute", 60 * time.Second, "1m0s"},
		{"over a minute", 90 * time.Second, "1m30s"},
		{"multiple minutes", 5*time.Minute + 23*time.Second, "5m23s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatElapsed(tt.duration)
			if result != tt.expected {
				t.Errorf("formatElapsed(%v) = %q, want %q", tt.duration, result, tt.expected)
			}
		})
	}
}

// TestChat_ZeroStreamStartTimeGivesZeroElapsed verifies that when
// streamStartTime is zero (not set), the elapsed duration is 0
// rather than calculating from the Go epoch (year 1).
func TestChat_ZeroStreamStartTimeGivesZeroElapsed(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(80, 24)

	// Set streaming content WITHOUT setting waiting state first
	// This leaves streamStartTime as zero value
	chat.streaming = "Some streaming content"

	// Verify streamStartTime is zero
	if !chat.streamStartTime.IsZero() {
		t.Fatal("Expected streamStartTime to be zero for this test")
	}

	// The View should not crash and should handle zero streamStartTime
	// by treating elapsed as 0, not as ~292 years since year 1
	view := chat.View()

	// View should contain "0s" (zero elapsed) not something like "153722867m"
	if strings.Contains(view, "153722867m") {
		t.Error("View contains absurdly large elapsed time (~292 years) due to zero streamStartTime")
	}
}

// TestChat_StopwatchContinuesDuringStreaming verifies that the stopwatch tick
// continues while streaming content is being received, not just while waiting.
func TestChat_StopwatchContinuesDuringStreaming(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(80, 24)

	// Start waiting (this starts the stopwatch tick)
	chat.SetWaiting(true)
	cmd := chat.handleStopwatchTick()
	if cmd == nil {
		t.Error("Expected stopwatch to tick while waiting")
	}

	// Simulate receiving streaming content (waiting becomes false, but streaming is active)
	chat.waiting = false
	chat.streaming = "Hello, I'm Claude..."
	chat.streamStartTime = time.Now()

	// The stopwatch should STILL tick while streaming, even though waiting is false
	cmd = chat.handleStopwatchTick()
	if cmd == nil {
		t.Error("Expected stopwatch to continue ticking while streaming (timer was stopping when waiting became false)")
	}

	// After streaming finishes, ticks should stop
	chat.streaming = ""
	cmd = chat.handleStopwatchTick()
	if cmd != nil {
		t.Error("Expected stopwatch to stop after streaming finishes")
	}
}

func TestChat_FocusState(t *testing.T) {
	chat := NewChat()

	// Initially not focused
	if chat.IsFocused() {
		t.Error("Should not be focused initially")
	}

	// Set focused
	chat.SetFocused(true)
	if !chat.IsFocused() {
		t.Error("Should be focused after SetFocused(true)")
	}

	// Unfocus
	chat.SetFocused(false)
	if chat.IsFocused() {
		t.Error("Should not be focused after SetFocused(false)")
	}
}

func TestChat_PendingPermission(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)

	// Initially no pending permission
	if chat.HasPendingPermission() {
		t.Error("Should not have pending permission initially")
	}

	// Set pending permission
	chat.SetPendingPermission("Bash", "Run: git status")

	if !chat.HasPendingPermission() {
		t.Error("Should have pending permission after SetPendingPermission")
	}

	if chat.permission.Tool != "Bash" {
		t.Errorf("Expected tool 'Bash', got %q", chat.permission.Tool)
	}

	if chat.permission.Description != "Run: git status" {
		t.Errorf("Expected description 'Run: git status', got %q", chat.permission.Description)
	}

	// Clear pending permission
	chat.ClearPendingPermission()

	if chat.HasPendingPermission() {
		t.Error("Should not have pending permission after clear")
	}
}

func TestChat_PendingQuestion(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)

	// Initially no pending question
	if chat.HasPendingQuestion() {
		t.Error("Should not have pending question initially")
	}

	questions := []mcp.Question{
		{
			Question: "Which option do you prefer?",
			Header:   "Choice",
			Options: []mcp.QuestionOption{
				{Label: "Option A", Description: "First option"},
				{Label: "Option B", Description: "Second option"},
			},
		},
	}

	// Set pending question
	chat.SetPendingQuestion(questions)

	if !chat.HasPendingQuestion() {
		t.Error("Should have pending question after SetPendingQuestion")
	}

	if len(chat.question.Questions) != 1 {
		t.Errorf("Expected 1 question, got %d", len(chat.question.Questions))
	}

	// Test question selection
	chat.MoveQuestionSelection(1) // Move down
	if chat.question.SelectedOption != 1 {
		t.Errorf("Expected selected index 1, got %d", chat.question.SelectedOption)
	}

	chat.MoveQuestionSelection(-1) // Move up
	if chat.question.SelectedOption != 0 {
		t.Errorf("Expected selected index 0 after moving up, got %d", chat.question.SelectedOption)
	}

	// Test wrap-around
	chat.MoveQuestionSelection(-1) // Should wrap to last
	expectedIdx := len(questions[0].Options) // +1 for "Other" option
	if chat.question.SelectedOption != expectedIdx {
		t.Errorf("Expected wrap to %d, got %d", expectedIdx, chat.question.SelectedOption)
	}

	// Select by number
	chat.question.SelectedOption = 0
	completed := chat.SelectOptionByNumber(1)
	if !completed {
		t.Error("Should complete with single question")
	}

	answers := chat.GetQuestionAnswers()
	if len(answers) != 1 {
		t.Errorf("Expected 1 answer, got %d", len(answers))
	}

	// Clear pending question
	chat.ClearPendingQuestion()

	if chat.HasPendingQuestion() {
		t.Error("Should not have pending question after clear")
	}
}

func TestChat_MultipleQuestions(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)

	questions := []mcp.Question{
		{
			Question: "Question 1",
			Header:   "Q1",
			Options: []mcp.QuestionOption{
				{Label: "A1"},
			},
		},
		{
			Question: "Question 2",
			Header:   "Q2",
			Options: []mcp.QuestionOption{
				{Label: "B1"},
			},
		},
	}

	chat.SetPendingQuestion(questions)

	// Answer first question
	completed := chat.SelectCurrentOption()
	if completed {
		t.Error("Should not be complete after first answer")
	}

	if chat.question.CurrentIdx != 1 {
		t.Errorf("Expected currentQuestionIdx 1, got %d", chat.question.CurrentIdx)
	}

	// Answer second question
	completed = chat.SelectCurrentOption()
	if !completed {
		t.Error("Should be complete after second answer")
	}

	answers := chat.GetQuestionAnswers()
	if len(answers) != 2 {
		t.Errorf("Expected 2 answers, got %d", len(answers))
	}
}

func TestChat_SetSize(t *testing.T) {
	chat := NewChat()

	// Should not panic with various sizes
	chat.SetSize(80, 24)
	chat.SetSize(120, 40)
	chat.SetSize(40, 10)
	chat.SetSize(1, 1) // Minimum size

	if chat.width != 1 {
		t.Errorf("Expected width 1, got %d", chat.width)
	}

	if chat.height != 1 {
		t.Errorf("Expected height 1, got %d", chat.height)
	}
}

func TestToolUseConstants(t *testing.T) {
	if ToolUseInProgress != "○" {
		t.Errorf("Expected ToolUseInProgress to be ○, got %q", ToolUseInProgress)
	}

	if ToolUseComplete != "●" {
		t.Errorf("Expected ToolUseComplete to be ●, got %q", ToolUseComplete)
	}
}

func TestSpinnerFrames(t *testing.T) {
	if len(spinnerFrames) == 0 {
		t.Error("spinnerFrames should not be empty")
	}

	if len(spinnerFrameHoldTimes) != len(spinnerFrames) {
		t.Errorf("spinnerFrameHoldTimes length (%d) should match spinnerFrames (%d)",
			len(spinnerFrameHoldTimes), len(spinnerFrames))
	}

	// Verify all hold times are positive
	for i, holdTime := range spinnerFrameHoldTimes {
		if holdTime < 1 {
			t.Errorf("spinnerFrameHoldTimes[%d] = %d, should be >= 1", i, holdTime)
		}
	}
}

func TestThinkingVerbs(t *testing.T) {
	if len(thinkingVerbs) == 0 {
		t.Error("thinkingVerbs should not be empty")
	}

	// Verify no empty verbs
	for i, verb := range thinkingVerbs {
		if verb == "" {
			t.Errorf("thinkingVerbs[%d] is empty", i)
		}
	}
}

// =============================================================================
// Text Selection Tests
// =============================================================================

func TestChat_StartSelection(t *testing.T) {
	chat := NewChat()

	// Start selection at position (5, 10)
	chat.StartSelection(5, 10)

	if chat.selection.StartCol != 5 {
		t.Errorf("Expected selectionStartCol 5, got %d", chat.selection.StartCol)
	}
	if chat.selection.StartLine != 10 {
		t.Errorf("Expected selectionStartLine 10, got %d", chat.selection.StartLine)
	}
	if chat.selection.EndCol != 5 {
		t.Errorf("Expected selectionEndCol 5, got %d", chat.selection.EndCol)
	}
	if chat.selection.EndLine != 10 {
		t.Errorf("Expected selectionEndLine 10, got %d", chat.selection.EndLine)
	}
	if !chat.selection.Active {
		t.Error("Expected selectionActive to be true")
	}
}

func TestChat_EndSelection(t *testing.T) {
	chat := NewChat()

	// EndSelection without active selection should do nothing
	chat.EndSelection(10, 20)
	if chat.selection.EndCol != 0 || chat.selection.EndLine != 0 {
		t.Error("EndSelection should not modify coordinates when selection is not active")
	}

	// Start selection then end it
	chat.StartSelection(5, 10)
	chat.EndSelection(15, 25)

	if chat.selection.EndCol != 15 {
		t.Errorf("Expected selectionEndCol 15, got %d", chat.selection.EndCol)
	}
	if chat.selection.EndLine != 25 {
		t.Errorf("Expected selectionEndLine 25, got %d", chat.selection.EndLine)
	}
	// Start position should be unchanged
	if chat.selection.StartCol != 5 {
		t.Errorf("Expected selectionStartCol unchanged at 5, got %d", chat.selection.StartCol)
	}
	if chat.selection.StartLine != 10 {
		t.Errorf("Expected selectionStartLine unchanged at 10, got %d", chat.selection.StartLine)
	}
}

func TestChat_SelectionStop(t *testing.T) {
	chat := NewChat()
	chat.StartSelection(5, 10)
	chat.EndSelection(15, 20)

	if !chat.selection.Active {
		t.Error("Expected selectionActive to be true before stop")
	}

	chat.SelectionStop()

	if chat.selection.Active {
		t.Error("Expected selectionActive to be false after stop")
	}
	// Coordinates should be preserved
	if chat.selection.StartCol != 5 || chat.selection.StartLine != 10 {
		t.Error("Selection start coordinates should be preserved after stop")
	}
	if chat.selection.EndCol != 15 || chat.selection.EndLine != 20 {
		t.Error("Selection end coordinates should be preserved after stop")
	}
}

func TestChat_SelectionClear(t *testing.T) {
	chat := NewChat()
	chat.StartSelection(5, 10)
	chat.EndSelection(15, 20)
	chat.SelectionStop()

	chat.SelectionClear()

	if chat.selection.StartCol != -1 {
		t.Errorf("Expected selectionStartCol -1, got %d", chat.selection.StartCol)
	}
	if chat.selection.StartLine != -1 {
		t.Errorf("Expected selectionStartLine -1, got %d", chat.selection.StartLine)
	}
	if chat.selection.EndCol != -1 {
		t.Errorf("Expected selectionEndCol -1, got %d", chat.selection.EndCol)
	}
	if chat.selection.EndLine != -1 {
		t.Errorf("Expected selectionEndLine -1, got %d", chat.selection.EndLine)
	}
	if chat.selection.Active {
		t.Error("Expected selectionActive to be false after clear")
	}
}

func TestChat_HasTextSelection(t *testing.T) {
	chat := NewChat()

	// Initially no selection
	if chat.HasTextSelection() {
		t.Error("Expected no selection initially")
	}

	// Selection cleared (negative coords)
	chat.SelectionClear()
	if chat.HasTextSelection() {
		t.Error("Expected no selection after clear")
	}

	// Start selection but end at same position (no selection)
	chat.StartSelection(5, 10)
	if chat.HasTextSelection() {
		t.Error("Expected no selection when start equals end")
	}

	// Update end position to create actual selection
	chat.EndSelection(10, 10)
	if !chat.HasTextSelection() {
		t.Error("Expected selection when end differs from start")
	}

	// Multi-line selection
	chat.EndSelection(5, 15)
	if !chat.HasTextSelection() {
		t.Error("Expected selection for multi-line selection")
	}
}

func TestAbs(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{0, 0},
		{5, 5},
		{-5, 5},
		{-100, 100},
		{100, 100},
	}

	for _, tt := range tests {
		result := abs(tt.input)
		if result != tt.expected {
			t.Errorf("abs(%d) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestChat_SelectionArea(t *testing.T) {
	chat := NewChat()

	tests := []struct {
		name                                     string
		startCol, startLine, endCol, endLine     int
		wantStartCol, wantStartLine              int
		wantEndCol, wantEndLine                  int
	}{
		{
			name:          "already normalized - same line",
			startCol:      5, startLine: 10, endCol: 15, endLine: 10,
			wantStartCol:  5, wantStartLine: 10, wantEndCol: 15, wantEndLine: 10,
		},
		{
			name:          "already normalized - multi line",
			startCol:      5, startLine: 10, endCol: 15, endLine: 20,
			wantStartCol:  5, wantStartLine: 10, wantEndCol: 15, wantEndLine: 20,
		},
		{
			name:          "needs normalization - same line reversed",
			startCol:      15, startLine: 10, endCol: 5, endLine: 10,
			wantStartCol:  5, wantStartLine: 10, wantEndCol: 15, wantEndLine: 10,
		},
		{
			name:          "needs normalization - multi line reversed",
			startCol:      15, startLine: 20, endCol: 5, endLine: 10,
			wantStartCol:  5, wantStartLine: 10, wantEndCol: 15, wantEndLine: 20,
		},
		{
			name:          "drag selection upward",
			startCol:      10, startLine: 15, endCol: 3, endLine: 5,
			wantStartCol:  3, wantStartLine: 5, wantEndCol: 10, wantEndLine: 15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chat.selection.StartCol = tt.startCol
			chat.selection.StartLine = tt.startLine
			chat.selection.EndCol = tt.endCol
			chat.selection.EndLine = tt.endLine

			gotStartCol, gotStartLine, gotEndCol, gotEndLine := chat.selectionArea()

			if gotStartCol != tt.wantStartCol {
				t.Errorf("startCol = %d, want %d", gotStartCol, tt.wantStartCol)
			}
			if gotStartLine != tt.wantStartLine {
				t.Errorf("startLine = %d, want %d", gotStartLine, tt.wantStartLine)
			}
			if gotEndCol != tt.wantEndCol {
				t.Errorf("endCol = %d, want %d", gotEndCol, tt.wantEndCol)
			}
			if gotEndLine != tt.wantEndLine {
				t.Errorf("endLine = %d, want %d", gotEndLine, tt.wantEndLine)
			}
		})
	}
}

func TestChat_HandleMouseClick_SingleClick(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(80, 24)

	// First click should start selection
	_ = chat.handleMouseClick(10, 5)

	if chat.selection.ClickCount != 1 {
		t.Errorf("Expected clickCount 1, got %d", chat.selection.ClickCount)
	}
	if chat.selection.StartCol != 10 {
		t.Errorf("Expected selectionStartCol 10, got %d", chat.selection.StartCol)
	}
	if chat.selection.StartLine != 5 {
		t.Errorf("Expected selectionStartLine 5, got %d", chat.selection.StartLine)
	}
	if !chat.selection.Active {
		t.Error("Expected selectionActive to be true after single click")
	}
}

func TestChat_HandleMouseClick_ClickCountReset(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(80, 24)

	// First click
	_ = chat.handleMouseClick(10, 5)
	if chat.selection.ClickCount != 1 {
		t.Errorf("Expected clickCount 1 after first click, got %d", chat.selection.ClickCount)
	}

	// Click far away - should reset count to 1 (new click sequence)
	_ = chat.handleMouseClick(50, 20)
	if chat.selection.ClickCount != 1 {
		t.Errorf("Expected clickCount reset to 1 when clicking far away, got %d", chat.selection.ClickCount)
	}
}

func TestChat_HandleMouseClick_DoubleClick(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(80, 24)

	// Simulate rapid double click at same position
	_ = chat.handleMouseClick(10, 5)
	if chat.selection.ClickCount != 1 {
		t.Errorf("Expected clickCount 1 after first click, got %d", chat.selection.ClickCount)
	}

	// Second click at same position (within tolerance and time threshold)
	_ = chat.handleMouseClick(10, 5)
	if chat.selection.ClickCount != 2 {
		t.Errorf("Expected clickCount 2 after second click at same position, got %d", chat.selection.ClickCount)
	}
}

func TestChat_HandleMouseClick_TripleClick(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(80, 24)

	// Simulate rapid triple click at same position
	_ = chat.handleMouseClick(10, 5)
	_ = chat.handleMouseClick(10, 5)
	_ = chat.handleMouseClick(10, 5)

	// After triple click, count is explicitly reset to 0
	if chat.selection.ClickCount != 0 {
		t.Errorf("Expected clickCount 0 after triple click, got %d", chat.selection.ClickCount)
	}
}

func TestChat_HandleMouseClick_TripleClickResets(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(80, 24)

	// Simulate clicks at same position
	chat.selection.LastClickX = 10
	chat.selection.LastClickY = 5

	// First click
	_ = chat.handleMouseClick(10, 5)
	// Second click
	_ = chat.handleMouseClick(10, 5)
	// Third click - should reset to 0
	_ = chat.handleMouseClick(10, 5)

	if chat.selection.ClickCount != 0 {
		t.Errorf("Expected clickCount 0 after triple click, got %d", chat.selection.ClickCount)
	}
}

// TestChat_SelectWord_WithViewportContent tests word selection with actual viewport content
func TestChat_SelectWord_WithViewportContent(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(80, 24)

	// Add some messages to create content
	chat.messages = []claude.Message{
		{Role: "assistant", Content: "Hello world this is a test"},
	}

	// The viewport needs content to work with SelectWord
	// We'll directly test the function's behavior with known inputs

	// Test bounds checking for negative line
	chat.SelectWord(5, -1)
	if chat.HasTextSelection() {
		t.Error("SelectWord should not create selection for negative line")
	}

	// Test bounds checking for negative column
	chat.SelectWord(-1, 0)
	if chat.HasTextSelection() {
		t.Error("SelectWord should not create selection for negative column")
	}
}

func TestChat_SelectWord_EdgeCases(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)

	// Test with out-of-bounds line index (should not panic)
	chat.SelectWord(0, 1000)
	// Just verify it doesn't crash and doesn't set invalid state
	if chat.selection.Active {
		t.Error("SelectWord should not activate selection for out-of-bounds line")
	}
}

// TestChat_SelectParagraph_EdgeCases tests paragraph selection edge cases
func TestChat_SelectParagraph_EdgeCases(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)

	// Test with out-of-bounds line index (should not panic)
	chat.SelectParagraph(0, 1000)
	// Verify it doesn't crash
	if chat.selection.Active {
		t.Error("SelectParagraph should not activate selection for out-of-bounds line")
	}

	// Test with negative line
	chat.SelectParagraph(0, -1)
	if chat.HasTextSelection() {
		t.Error("SelectParagraph should not create selection for negative line")
	}
}

func TestChat_GetSelectedText_NoSelection(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)

	// No selection should return empty string
	text := chat.GetSelectedText()
	if text != "" {
		t.Errorf("Expected empty string for no selection, got %q", text)
	}
}

func TestChat_GetSelectedText_WithSelection(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Create a selection
	chat.selection.StartCol = 0
	chat.selection.StartLine = 0
	chat.selection.EndCol = 5
	chat.selection.EndLine = 0

	// The viewport content depends on the view rendering
	// Test that GetSelectedText doesn't crash with valid coordinates
	_ = chat.GetSelectedText()
	// Just verify it doesn't panic
}

func TestChat_GetSelectedText_BoundsValidation(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Test with reversed selection (end before start on same line)
	chat.selection.StartCol = 10
	chat.selection.StartLine = 0
	chat.selection.EndCol = 5
	chat.selection.EndLine = 0

	// Should handle reversed selection gracefully
	_ = chat.GetSelectedText()

	// Test with negative bounds (will be normalized by selectionArea)
	chat.selection.StartCol = -5
	chat.selection.StartLine = 0
	chat.selection.EndCol = 10
	chat.selection.EndLine = 0

	// The function should handle this gracefully
	_ = chat.GetSelectedText()
}

func TestChat_CopySelectedText_NoSelection(t *testing.T) {
	chat := NewChat()

	// Without selection, should return nil
	cmd := chat.CopySelectedText()
	if cmd != nil {
		t.Error("Expected nil command when no selection")
	}
}

func TestChat_CopySelectedText_EmptyText(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Create a selection but ensure GetSelectedText returns empty
	chat.selection.StartCol = 0
	chat.selection.StartLine = 0
	chat.selection.EndCol = 0
	chat.selection.EndLine = 0

	// Same position means HasTextSelection returns false
	cmd := chat.CopySelectedText()
	if cmd != nil {
		t.Error("Expected nil command for point selection (no area)")
	}
}

func TestChat_CopySelectedText_WithValidSelection(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Create a valid selection
	chat.selection.StartCol = 0
	chat.selection.StartLine = 0
	chat.selection.EndCol = 10
	chat.selection.EndLine = 0

	// Even with valid coordinates, the viewport may be empty
	// Just ensure it doesn't crash
	_ = chat.CopySelectedText()
}

func TestChat_SelectionView_NoSelection(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)

	originalView := "test content"
	result := chat.selectionView(originalView)

	// Without selection, should return original view unchanged
	if result != originalView {
		t.Errorf("Expected unchanged view without selection, got %q", result)
	}
}

func TestChat_SelectionView_ZeroDimensions(t *testing.T) {
	chat := NewChat()
	// Don't set size, so viewport has zero dimensions

	// Create a selection
	chat.selection.StartCol = 0
	chat.selection.StartLine = 0
	chat.selection.EndCol = 5
	chat.selection.EndLine = 0

	originalView := "test content"
	result := chat.selectionView(originalView)

	// With zero dimensions, should return original view
	if result != originalView {
		t.Errorf("Expected unchanged view with zero dimensions, got %q", result)
	}
}

func TestChat_SelectionView_WithValidSelection(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Create a valid selection
	chat.selection.StartCol = 0
	chat.selection.StartLine = 0
	chat.selection.EndCol = 5
	chat.selection.EndLine = 0

	// Test with simple content - should not panic
	testView := "Hello World"
	_ = chat.selectionView(testView)
}

func TestSelectionCopyMsg(t *testing.T) {
	msg := SelectionCopyMsg{
		clickCount:   2,
		endSelection: true,
		x:            10,
		y:            5,
	}

	if msg.clickCount != 2 {
		t.Errorf("Expected clickCount 2, got %d", msg.clickCount)
	}
	if !msg.endSelection {
		t.Error("Expected endSelection true")
	}
	if msg.x != 10 {
		t.Errorf("Expected x 10, got %d", msg.x)
	}
	if msg.y != 5 {
		t.Errorf("Expected y 5, got %d", msg.y)
	}
}

func TestDoubleClickThreshold(t *testing.T) {
	// Verify the constant is a reasonable value
	if doubleClickThreshold <= 0 {
		t.Error("doubleClickThreshold should be positive")
	}
	if doubleClickThreshold > 1000*time.Millisecond {
		t.Error("doubleClickThreshold should not be greater than 1 second")
	}
}

func TestClickTolerance(t *testing.T) {
	// Verify the click tolerance is reasonable
	if clickTolerance < 0 {
		t.Error("clickTolerance should not be negative")
	}
	if clickTolerance > 10 {
		t.Error("clickTolerance seems too high (> 10 pixels)")
	}
}

// TestChat_SelectionIntegration tests the full selection workflow
func TestChat_SelectionIntegration(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Simulate single click drag selection workflow
	t.Run("drag selection workflow", func(t *testing.T) {
		// Start selection
		chat.StartSelection(5, 2)
		if !chat.selection.Active {
			t.Error("Expected selection to be active after start")
		}

		// Drag to extend selection
		chat.EndSelection(20, 5)
		if !chat.selection.Active {
			t.Error("Expected selection to still be active during drag")
		}
		if chat.selection.EndCol != 20 || chat.selection.EndLine != 5 {
			t.Error("Selection end not updated correctly during drag")
		}

		// Stop selection (mouse release)
		chat.SelectionStop()
		if chat.selection.Active {
			t.Error("Expected selection to be inactive after stop")
		}

		// Selection should still be visible
		if !chat.HasTextSelection() {
			t.Error("Expected selection to exist after stop")
		}

		// Clear selection
		chat.SelectionClear()
		if chat.HasTextSelection() {
			t.Error("Expected no selection after clear")
		}
	})

	t.Run("double click word selection", func(t *testing.T) {
		// Double click selects a word
		// This is tested indirectly through handleMouseClick
		chat.SelectionClear()

		// After SelectWord, selectionActive should be false (word selected immediately)
		chat.SelectWord(5, 0)
		if chat.selection.Active {
			t.Error("Expected selectionActive false after SelectWord (immediate selection)")
		}
	})

	t.Run("triple click paragraph selection", func(t *testing.T) {
		chat.SelectionClear()

		// After SelectParagraph, selectionActive should be false
		chat.SelectParagraph(5, 0)
		if chat.selection.Active {
			t.Error("Expected selectionActive false after SelectParagraph")
		}
	})
}

// TestChat_SelectionCoordinates tests various coordinate scenarios
func TestChat_SelectionCoordinates(t *testing.T) {
	chat := NewChat()

	tests := []struct {
		name   string
		startX int
		startY int
		endX   int
		endY   int
	}{
		{"zero coordinates", 0, 0, 10, 10},
		{"large coordinates", 1000, 500, 2000, 1000},
		{"same line", 5, 10, 50, 10},
		{"reversed single line", 50, 10, 5, 10},
		{"multi-line forward", 5, 10, 50, 20},
		{"multi-line backward", 50, 20, 5, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chat.StartSelection(tt.startX, tt.startY)
			chat.EndSelection(tt.endX, tt.endY)

			// Verify coordinates are set correctly
			if chat.selection.StartCol != tt.startX {
				t.Errorf("Expected startCol %d, got %d", tt.startX, chat.selection.StartCol)
			}
			if chat.selection.StartLine != tt.startY {
				t.Errorf("Expected startLine %d, got %d", tt.startY, chat.selection.StartLine)
			}
			if chat.selection.EndCol != tt.endX {
				t.Errorf("Expected endCol %d, got %d", tt.endX, chat.selection.EndCol)
			}
			if chat.selection.EndLine != tt.endY {
				t.Errorf("Expected endLine %d, got %d", tt.endY, chat.selection.EndLine)
			}

			// Clean up for next test
			chat.SelectionClear()
		})
	}
}

// TestChat_GetSelectedText_MultiLineSelection tests multi-line text extraction
func TestChat_GetSelectedText_MultiLineSelection(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Multi-line selection
	chat.selection.StartCol = 5
	chat.selection.StartLine = 0
	chat.selection.EndCol = 10
	chat.selection.EndLine = 2

	// Test that it doesn't crash with multi-line coordinates
	_ = chat.GetSelectedText()
}

// TestChat_CopySelectedText_EmptySelection tests CopySelectedText with empty selection
func TestChat_CopySelectedText_EmptySelection(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Create selection that would result in empty text after trim
	chat.selection.StartCol = 0
	chat.selection.StartLine = 0
	chat.selection.EndCol = 1
	chat.selection.EndLine = 0

	// This may or may not return nil depending on viewport content
	_ = chat.CopySelectedText()
}

// TestChat_SelectionView_MultiLineSelection tests multi-line highlighting
func TestChat_SelectionView_MultiLineSelection(t *testing.T) {
	chat := NewChat()
	chat.SetSize(40, 10)
	chat.SetSession("test", nil)

	// Multi-line selection
	chat.selection.StartCol = 0
	chat.selection.StartLine = 0
	chat.selection.EndCol = 20
	chat.selection.EndLine = 3

	testView := "Line 1 content\nLine 2 content\nLine 3 content\nLine 4 content"
	result := chat.selectionView(testView)

	// The result should be different from input (highlighting applied)
	// Just verify it doesn't crash
	if result == "" {
		t.Error("Expected non-empty result from selectionView")
	}
}

// TestChat_SelectionView_SingleLineSelection tests single-line highlighting
func TestChat_SelectionView_SingleLineSelection(t *testing.T) {
	chat := NewChat()
	chat.SetSize(40, 10)
	chat.SetSession("test", nil)

	// Single line selection
	chat.selection.StartCol = 2
	chat.selection.StartLine = 1
	chat.selection.EndCol = 8
	chat.selection.EndLine = 1

	testView := "Line 1\nLine 2 here\nLine 3"
	result := chat.selectionView(testView)

	// The result should not be empty
	if result == "" {
		t.Error("Expected non-empty result from selectionView")
	}
}

// TestChat_SelectionView_FirstLineOnly tests first line of multi-line selection
func TestChat_SelectionView_FirstLineOnly(t *testing.T) {
	chat := NewChat()
	chat.SetSize(40, 10)
	chat.SetSession("test", nil)

	// Selection starting mid-first line through multiple lines
	chat.selection.StartCol = 5
	chat.selection.StartLine = 0
	chat.selection.EndCol = 10
	chat.selection.EndLine = 2

	testView := "First line text\nSecond line\nThird line text"
	_ = chat.selectionView(testView)
}

// TestChat_SelectionView_LastLineOnly tests last line of multi-line selection
func TestChat_SelectionView_LastLineOnly(t *testing.T) {
	chat := NewChat()
	chat.SetSize(40, 10)
	chat.SetSession("test", nil)

	// Selection ending mid-last line
	chat.selection.StartCol = 0
	chat.selection.StartLine = 0
	chat.selection.EndCol = 5
	chat.selection.EndLine = 2

	testView := "First line\nMiddle line\nLast line here"
	_ = chat.selectionView(testView)
}

// TestChat_SelectionView_MiddleLinesFullWidth tests middle lines of multi-line selection
func TestChat_SelectionView_MiddleLinesFullWidth(t *testing.T) {
	chat := NewChat()
	chat.SetSize(40, 10)
	chat.SetSession("test", nil)

	// Selection spanning 4 lines (tests middle line branches)
	chat.selection.StartCol = 5
	chat.selection.StartLine = 0
	chat.selection.EndCol = 5
	chat.selection.EndLine = 3

	testView := "Line 0\nLine 1\nLine 2\nLine 3"
	_ = chat.selectionView(testView)
}

// TestChat_SelectWord_ValidPosition tests SelectWord with valid viewport content
func TestChat_SelectWord_ValidPosition(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Test SelectWord at column 0 (edge case for backward search)
	chat.SelectWord(0, 0)

	// Test SelectWord at end of line (edge case for forward search)
	chat.SelectWord(100, 0) // Column beyond line length
}

// TestChat_SelectParagraph_ValidPosition tests SelectParagraph with viewport content
func TestChat_SelectParagraph_ValidPosition(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Test at line 0 (edge case - can't search backward past start)
	chat.SelectParagraph(5, 0)

	// Selection should be set (even if empty content)
	if chat.selection.StartLine != 0 {
		t.Errorf("Expected selectionStartLine 0, got %d", chat.selection.StartLine)
	}
}

// =============================================================================
// Selection Flash Animation Tests
// =============================================================================

func TestChat_SelectionFlashFrameInitialization(t *testing.T) {
	chat := NewChat()

	// New chat should have selectionFlashFrame initialized to -1
	if chat.selection.FlashFrame != -1 {
		t.Errorf("Expected selectionFlashFrame -1, got %d", chat.selection.FlashFrame)
	}

	if chat.IsSelectionFlashing() {
		t.Error("New chat should not be selection flashing")
	}
}

func TestChat_IsSelectionFlashing(t *testing.T) {
	chat := NewChat()

	// Initially not flashing
	if chat.IsSelectionFlashing() {
		t.Error("Expected not flashing initially")
	}

	// When selectionFlashFrame is 0, should be flashing
	chat.selection.FlashFrame = 0
	if !chat.IsSelectionFlashing() {
		t.Error("Expected flashing when selectionFlashFrame is 0")
	}

	// When selectionFlashFrame is 1, should still be considered flashing
	chat.selection.FlashFrame = 1
	if !chat.IsSelectionFlashing() {
		t.Error("Expected flashing when selectionFlashFrame is 1")
	}

	// When selectionFlashFrame is -1, should not be flashing
	chat.selection.FlashFrame = -1
	if chat.IsSelectionFlashing() {
		t.Error("Expected not flashing when selectionFlashFrame is -1")
	}
}

func TestChat_CopySelectedText_StartsFlashAnimation(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Create a valid selection
	chat.selection.StartCol = 0
	chat.selection.StartLine = 0
	chat.selection.EndCol = 10
	chat.selection.EndLine = 0

	// Initially not flashing
	if chat.selection.FlashFrame != -1 {
		t.Errorf("Expected selectionFlashFrame -1 initially, got %d", chat.selection.FlashFrame)
	}

	// Copy selected text should start flash animation
	cmd := chat.CopySelectedText()

	// Flash frame should now be 0 (animation started)
	if chat.selection.FlashFrame != 0 {
		t.Errorf("Expected selectionFlashFrame 0 after copy, got %d", chat.selection.FlashFrame)
	}

	// Should return a command (batch of clipboard + flash tick)
	if cmd == nil {
		t.Error("Expected non-nil command from CopySelectedText with valid selection")
	}
}

func TestChat_SelectionFlashTickMsg_ClearsSelection(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Create a selection and start flash
	chat.selection.StartCol = 0
	chat.selection.StartLine = 0
	chat.selection.EndCol = 10
	chat.selection.EndLine = 0
	chat.selection.FlashFrame = 0

	// Verify selection exists
	if !chat.HasTextSelection() {
		t.Error("Expected selection to exist before tick")
	}

	// Process the flash tick message
	_, _ = chat.Update(SelectionFlashTickMsg(time.Now()))

	// After tick, selectionFlashFrame should increment
	// Since frame was 0, it becomes 1, which triggers clear and reset to -1
	if chat.selection.FlashFrame != -1 {
		t.Errorf("Expected selectionFlashFrame -1 after tick clears, got %d", chat.selection.FlashFrame)
	}

	// Selection should be cleared
	if chat.HasTextSelection() {
		t.Error("Expected selection to be cleared after flash tick")
	}
}

func TestChat_SelectionFlashTickMsg_DoesNothingWhenNotFlashing(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Create a selection but don't start flash
	chat.selection.StartCol = 0
	chat.selection.StartLine = 0
	chat.selection.EndCol = 10
	chat.selection.EndLine = 0
	chat.selection.FlashFrame = -1 // Not flashing

	// Process the flash tick message
	_, _ = chat.Update(SelectionFlashTickMsg(time.Now()))

	// Selection should still exist (tick should be ignored when not flashing)
	if !chat.HasTextSelection() {
		t.Error("Expected selection to still exist when not flashing")
	}

	// Flash frame should still be -1
	if chat.selection.FlashFrame != -1 {
		t.Errorf("Expected selectionFlashFrame to remain -1, got %d", chat.selection.FlashFrame)
	}
}

func TestSelectionFlashTick(t *testing.T) {
	// Verify SelectionFlashTick returns a command
	cmd := SelectionFlashTick()
	if cmd == nil {
		t.Error("Expected non-nil command from SelectionFlashTick")
	}
}

func TestChat_SelectionFlash_ClearsAfterCopy(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Create a valid selection
	chat.StartSelection(0, 0)
	chat.EndSelection(10, 0)
	chat.SelectionStop()

	// Verify selection exists
	if !chat.HasTextSelection() {
		t.Error("Expected selection to exist")
	}

	// Start copy (which starts flash)
	_ = chat.CopySelectedText()

	// Flash should be active
	if !chat.IsSelectionFlashing() {
		t.Error("Expected flash to be active after copy")
	}

	// Process flash tick to complete the flash and clear selection
	_, _ = chat.Update(SelectionFlashTickMsg(time.Now()))

	// After flash completes, selection should be cleared
	if chat.HasTextSelection() {
		t.Error("Expected selection to be cleared after flash completes")
	}

	// Flash should no longer be active
	if chat.IsSelectionFlashing() {
		t.Error("Expected flash to be inactive after tick")
	}
}

func TestChat_SelectionView_UsesFlashStyle(t *testing.T) {
	chat := NewChat()
	chat.SetSize(40, 10)
	chat.SetSession("test", nil)

	// Create a valid selection
	chat.selection.StartCol = 0
	chat.selection.StartLine = 0
	chat.selection.EndCol = 5
	chat.selection.EndLine = 0

	testView := "Hello World"

	// Test with normal selection (no flash)
	chat.selection.FlashFrame = -1
	result1 := chat.selectionView(testView)

	// Test with flash active
	chat.selection.FlashFrame = 0
	result2 := chat.selectionView(testView)

	// Both should produce output (we can't easily verify colors in text output)
	if result1 == "" {
		t.Error("Expected non-empty result for normal selection")
	}
	if result2 == "" {
		t.Error("Expected non-empty result for flash selection")
	}

	// Note: We can't easily test that the colors are different without
	// parsing ANSI codes, but we verified the code paths execute
}

// =============================================================================
// Todo List Tests
// =============================================================================

func TestChat_SetTodoList_InProgress(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Create a todo list with items still in progress
	todoList := &claude.TodoList{
		Items: []claude.TodoItem{
			{Content: "Task 1", Status: claude.TodoStatusCompleted, ActiveForm: "Completing task 1"},
			{Content: "Task 2", Status: claude.TodoStatusInProgress, ActiveForm: "Working on task 2"},
			{Content: "Task 3", Status: claude.TodoStatusPending, ActiveForm: "Will do task 3"},
		},
	}

	chat.SetTodoList(todoList)

	// Todo list should be active (not baked into messages)
	if !chat.HasTodoList() {
		t.Error("Expected HasTodoList() to be true for in-progress list")
	}

	// Messages should still be empty
	if len(chat.messages) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(chat.messages))
	}

	// The current todo list should be set
	if chat.GetTodoList() != todoList {
		t.Error("Expected GetTodoList() to return the set list")
	}
}

func TestChat_SetTodoList_AllCompleted(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Create a todo list with all items completed
	todoList := &claude.TodoList{
		Items: []claude.TodoItem{
			{Content: "Task 1", Status: claude.TodoStatusCompleted, ActiveForm: "Completing task 1"},
			{Content: "Task 2", Status: claude.TodoStatusCompleted, ActiveForm: "Completing task 2"},
			{Content: "Task 3", Status: claude.TodoStatusCompleted, ActiveForm: "Completing task 3"},
		},
	}

	chat.SetTodoList(todoList)

	// Todo list should NOT be active (it was baked into messages)
	if chat.HasTodoList() {
		t.Error("Expected HasTodoList() to be false for completed list")
	}

	// There should be one message (the baked todo list)
	if len(chat.messages) != 1 {
		t.Errorf("Expected 1 message after baking completed list, got %d", len(chat.messages))
	}

	// The message should be from assistant role
	if chat.messages[0].Role != "assistant" {
		t.Errorf("Expected message role 'assistant', got %q", chat.messages[0].Role)
	}

	// The message content should contain the task progress header
	if !strings.Contains(chat.messages[0].Content, "Task Progress") {
		t.Error("Expected baked message to contain 'Task Progress'")
	}

	// The message content should contain the completed count
	if !strings.Contains(chat.messages[0].Content, "(3/3)") {
		t.Error("Expected baked message to show (3/3) completion")
	}

	// The current todo list should be nil
	if chat.GetTodoList() != nil {
		t.Error("Expected GetTodoList() to return nil after baking")
	}
}

func TestChat_SetTodoList_Nil(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	chat.SetTodoList(nil)

	if chat.HasTodoList() {
		t.Error("Expected HasTodoList() to be false for nil list")
	}

	if len(chat.messages) != 0 {
		t.Errorf("Expected 0 messages for nil list, got %d", len(chat.messages))
	}
}

func TestChat_SetTodoList_EmptyList(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Empty list - should not be considered complete
	todoList := &claude.TodoList{
		Items: []claude.TodoItem{},
	}

	chat.SetTodoList(todoList)

	// Empty list should not be "complete", so hasTodoList should be false
	// (the condition len(list.Items) > 0 fails)
	if chat.HasTodoList() {
		t.Error("Expected HasTodoList() to be false for empty list")
	}

	// No messages should be added for empty list
	if len(chat.messages) != 0 {
		t.Errorf("Expected 0 messages for empty list, got %d", len(chat.messages))
	}
}

func TestChat_SetTodoList_SingleCompleted(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Single completed task
	todoList := &claude.TodoList{
		Items: []claude.TodoItem{
			{Content: "Only task", Status: claude.TodoStatusCompleted, ActiveForm: "Completing only task"},
		},
	}

	chat.SetTodoList(todoList)

	// Should be baked into messages
	if chat.HasTodoList() {
		t.Error("Expected HasTodoList() to be false for single completed item")
	}

	if len(chat.messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(chat.messages))
	}

	if !strings.Contains(chat.messages[0].Content, "(1/1)") {
		t.Error("Expected message to show (1/1) completion")
	}
}

func TestChat_SetTodoList_TransitionFromInProgressToComplete(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// First, set an in-progress list
	inProgressList := &claude.TodoList{
		Items: []claude.TodoItem{
			{Content: "Task 1", Status: claude.TodoStatusCompleted, ActiveForm: "Done with task 1"},
			{Content: "Task 2", Status: claude.TodoStatusInProgress, ActiveForm: "Working on task 2"},
		},
	}

	chat.SetTodoList(inProgressList)

	if !chat.HasTodoList() {
		t.Error("Expected HasTodoList() to be true for in-progress list")
	}
	if len(chat.messages) != 0 {
		t.Errorf("Expected 0 messages while in progress, got %d", len(chat.messages))
	}

	// Now update to a completed list
	completedList := &claude.TodoList{
		Items: []claude.TodoItem{
			{Content: "Task 1", Status: claude.TodoStatusCompleted, ActiveForm: "Done with task 1"},
			{Content: "Task 2", Status: claude.TodoStatusCompleted, ActiveForm: "Done with task 2"},
		},
	}

	chat.SetTodoList(completedList)

	// Should now be baked
	if chat.HasTodoList() {
		t.Error("Expected HasTodoList() to be false after completion")
	}
	if len(chat.messages) != 1 {
		t.Errorf("Expected 1 message after completion, got %d", len(chat.messages))
	}
}

func TestChat_ClearTodoList(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Set an in-progress list
	todoList := &claude.TodoList{
		Items: []claude.TodoItem{
			{Content: "Task 1", Status: claude.TodoStatusInProgress, ActiveForm: "Working"},
		},
	}

	chat.SetTodoList(todoList)

	if !chat.HasTodoList() {
		t.Error("Expected HasTodoList() to be true before clear")
	}

	chat.ClearTodoList()

	if chat.HasTodoList() {
		t.Error("Expected HasTodoList() to be false after clear")
	}

	if chat.GetTodoList() != nil {
		t.Error("Expected GetTodoList() to return nil after clear")
	}
}

// =============================================================================
// Todo Sidebar Tests
// =============================================================================

func TestChat_TodoSidebar_WidthCalculation(t *testing.T) {
	chat := NewChat()
	chat.SetSize(120, 40)
	chat.SetSession("test", nil)

	// Without todo list, todoWidth should be 0
	if chat.todoWidth != 0 {
		t.Errorf("Expected todoWidth=0 without todo list, got %d", chat.todoWidth)
	}

	// Set a todo list
	todoList := &claude.TodoList{
		Items: []claude.TodoItem{
			{Content: "Task 1", Status: claude.TodoStatusInProgress, ActiveForm: "Working on task 1"},
		},
	}
	chat.SetTodoList(todoList)

	// Now todoWidth should be approximately 1/4 of the total width
	expectedWidth := 120 / TodoSidebarWidthRatio
	if chat.todoWidth != expectedWidth {
		t.Errorf("Expected todoWidth=%d (1/4 of 120), got %d", expectedWidth, chat.todoWidth)
	}

	// Clear the todo list
	chat.ClearTodoList()

	// todoWidth should go back to 0
	if chat.todoWidth != 0 {
		t.Errorf("Expected todoWidth=0 after clearing todo list, got %d", chat.todoWidth)
	}
}

func TestChat_TodoSidebar_ViewportWidthAdjustment(t *testing.T) {
	chat := NewChat()
	totalWidth := 120
	chat.SetSize(totalWidth, 40)
	chat.SetSession("test", nil)

	// Get viewport width without todo list
	fullViewportWidth := chat.viewport.Width()

	// Set a todo list
	todoList := &claude.TodoList{
		Items: []claude.TodoItem{
			{Content: "Task 1", Status: claude.TodoStatusInProgress, ActiveForm: "Working on task 1"},
		},
	}
	chat.SetTodoList(todoList)

	// Viewport width should be reduced to make room for todo sidebar
	splitViewportWidth := chat.viewport.Width()
	if splitViewportWidth >= fullViewportWidth {
		t.Errorf("Expected viewport width to decrease when todo sidebar is shown. Full: %d, Split: %d",
			fullViewportWidth, splitViewportWidth)
	}

	// The difference should approximately equal the todo sidebar width (accounting for borders)
	expectedReduction := chat.todoWidth
	actualReduction := fullViewportWidth - splitViewportWidth
	// Allow for some variance due to border calculations
	if actualReduction < expectedReduction-BorderSize || actualReduction > expectedReduction+BorderSize {
		t.Errorf("Viewport width reduction (%d) doesn't match todo sidebar width (%d)",
			actualReduction, expectedReduction)
	}
}

func TestChat_TodoSidebar_ViewportContent(t *testing.T) {
	chat := NewChat()
	chat.SetSize(120, 40)
	chat.SetSession("test", nil)

	// Without todo list, todo viewport should have empty content
	content := chat.todoViewport.View()
	if strings.TrimSpace(content) != "" {
		t.Errorf("Expected empty content in todo viewport without todo list, got %q", content)
	}

	// Set a todo list
	todoList := &claude.TodoList{
		Items: []claude.TodoItem{
			{Content: "Task 1", Status: claude.TodoStatusCompleted, ActiveForm: "Done with task 1"},
			{Content: "Task 2", Status: claude.TodoStatusInProgress, ActiveForm: "Working on task 2"},
			{Content: "Task 3", Status: claude.TodoStatusPending, ActiveForm: "Pending task 3"},
		},
	}
	chat.SetTodoList(todoList)

	// Now todo viewport should have content
	content = chat.todoViewport.View()
	if strings.TrimSpace(content) == "" {
		t.Error("Expected non-empty content in todo viewport with todo list")
	}

	// Content should contain task markers
	if !strings.Contains(content, "✓") {
		t.Error("Expected todo sidebar to contain completed marker (✓)")
	}
	if !strings.Contains(content, "▸") {
		t.Error("Expected todo sidebar to contain in-progress marker (▸)")
	}
	if !strings.Contains(content, "○") {
		t.Error("Expected todo sidebar to contain pending marker (○)")
	}
}

func TestChat_TodoSidebar_MinimumWidth(t *testing.T) {
	chat := NewChat()
	// Use a very small width to test minimum width enforcement
	chat.SetSize(60, 40)
	chat.SetSession("test", nil)

	todoList := &claude.TodoList{
		Items: []claude.TodoItem{
			{Content: "Task 1", Status: claude.TodoStatusInProgress, ActiveForm: "Working"},
		},
	}
	chat.SetTodoList(todoList)

	// todoWidth should be at least TodoListMinWrapWidth + BorderSize
	minWidth := TodoListMinWrapWidth + BorderSize
	if chat.todoWidth < minWidth {
		t.Errorf("Expected todoWidth >= %d, got %d", minWidth, chat.todoWidth)
	}
}

func TestChat_TodoSidebar_ScrollableViewport(t *testing.T) {
	chat := NewChat()
	chat.SetSize(120, 20) // Smaller height to force scrolling
	chat.SetSession("test", nil)

	// Create a todo list with many items to force scrolling
	items := make([]claude.TodoItem, 15)
	for i := 0; i < 15; i++ {
		items[i] = claude.TodoItem{
			Content:    fmt.Sprintf("Task %d with some longer description to fill space", i+1),
			Status:     claude.TodoStatusPending,
			ActiveForm: fmt.Sprintf("Pending task %d", i+1),
		}
	}
	todoList := &claude.TodoList{Items: items}
	chat.SetTodoList(todoList)

	// Verify todo viewport is initialized with correct dimensions
	if chat.todoViewport.Width() <= 0 {
		t.Error("Expected todo viewport width > 0")
	}
	if chat.todoViewport.Height() <= 0 {
		t.Error("Expected todo viewport height > 0")
	}

	// Verify viewport has content
	content := chat.todoViewport.View()
	if strings.TrimSpace(content) == "" {
		t.Error("Expected todo viewport to have content")
	}

	// Verify the content is scrollable (more content than viewport height)
	// The viewport should truncate content if it's taller than the viewport
	viewportHeight := chat.todoViewport.Height()
	contentLines := strings.Split(content, "\n")
	// Viewport.View() returns exactly viewportHeight lines when content exceeds height
	if len(contentLines) > viewportHeight {
		// Content is being displayed within the viewport bounds
		t.Logf("Content has %d lines, viewport height is %d (scrollable)", len(contentLines), viewportHeight)
	}
}

func TestChat_TodoSidebar_MouseWheelRouting(t *testing.T) {
	chat := NewChat()
	chat.SetSize(120, 40)
	chat.SetSession("test", nil)

	// Create a todo list with many items
	items := make([]claude.TodoItem, 20)
	for i := 0; i < 20; i++ {
		items[i] = claude.TodoItem{
			Content:    fmt.Sprintf("Task %d", i+1),
			Status:     claude.TodoStatusPending,
			ActiveForm: fmt.Sprintf("Pending task %d", i+1),
		}
	}
	todoList := &claude.TodoList{Items: items}
	chat.SetTodoList(todoList)

	// Calculate the boundary between chat and todo sidebar
	mainWidth := chat.width - chat.todoWidth

	// Verify that mouse wheel events over the todo sidebar area should be
	// routed to the todo viewport (this is tested implicitly by the Update function)
	// A mouse wheel event with X >= mainWidth should update the todo viewport

	// Get initial scroll position (should be at top)
	initialYOffset := chat.todoViewport.YOffset()

	// Simulate mouse wheel scroll over the todo sidebar area
	msg := tea.MouseWheelMsg{
		X:      mainWidth + 5, // Over the todo sidebar
		Y:      10,
		Button: tea.MouseWheelDown,
	}

	_, _ = chat.Update(msg)

	// After scrolling down, YOffset should increase (if content exceeds viewport)
	// Note: This may not change if there's not enough content to scroll
	newYOffset := chat.todoViewport.YOffset()
	t.Logf("Initial YOffset: %d, After scroll: %d", initialYOffset, newYOffset)

	// Test that mouse wheel over the main chat area doesn't affect todo viewport
	chat.todoViewport.SetYOffset(0) // Reset todo viewport
	mainViewportOffset := chat.viewport.YOffset()

	msgMainArea := tea.MouseWheelMsg{
		X:      mainWidth - 5, // Over the main chat area
		Y:      10,
		Button: tea.MouseWheelDown,
	}

	_, _ = chat.Update(msgMainArea)

	// Main viewport should change (or stay same if at bottom), todo should stay at 0
	if chat.todoViewport.YOffset() != 0 {
		t.Errorf("Expected todo viewport to stay at 0, got %d", chat.todoViewport.YOffset())
	}
	t.Logf("Main viewport: Initial %d, After %d", mainViewportOffset, chat.viewport.YOffset())
}

func TestChat_TodoSidebar_ContentUpdatesOnListChange(t *testing.T) {
	chat := NewChat()
	chat.SetSize(120, 40)
	chat.SetSession("test", nil)

	// Set initial todo list
	todoList := &claude.TodoList{
		Items: []claude.TodoItem{
			{Content: "Initial Task", Status: claude.TodoStatusPending, ActiveForm: "Working"},
		},
	}
	chat.SetTodoList(todoList)

	content1 := chat.todoViewport.View()
	if !strings.Contains(content1, "Initial Task") {
		t.Error("Expected todo viewport to contain 'Initial Task'")
	}

	// Update todo list (use InProgress so it doesn't get "baked" into messages)
	// Note: When a task is in-progress, the ActiveForm is shown instead of Content
	todoList2 := &claude.TodoList{
		Items: []claude.TodoItem{
			{Content: "Updated Task", Status: claude.TodoStatusInProgress, ActiveForm: "Working on update"},
		},
	}
	chat.SetTodoList(todoList2)

	content2 := chat.todoViewport.View()
	// In-progress tasks show ActiveForm, not Content
	if !strings.Contains(content2, "Working on update") {
		t.Errorf("Expected todo viewport to contain 'Working on update', got: %q", content2)
	}
	if strings.Contains(content2, "Initial Task") {
		t.Error("Expected todo viewport to no longer contain 'Initial Task'")
	}

	// Clear todo list
	chat.ClearTodoList()

	content3 := chat.todoViewport.View()
	if strings.TrimSpace(content3) != "" {
		t.Errorf("Expected empty todo viewport after clear, got %q", content3)
	}
}

// =============================================================================
// Width Calculation Tests
// =============================================================================

// TestWidthConstants verifies that the width constants are consistent
// with the actual prefixes used in rendering.
func TestWidthConstants(t *testing.T) {
	// Verify ListItemPrefixWidth matches visual width of "  • " (2 spaces + bullet + space)
	// Note: We use lipgloss.Width for visual width since bullet is multi-byte UTF-8
	prefix := "  • "
	visualWidth := lipgloss.Width(prefix)
	if visualWidth != ListItemPrefixWidth {
		t.Errorf("ListItemPrefixWidth = %d, but actual prefix %q has visual width %d",
			ListItemPrefixWidth, prefix, visualWidth)
	}

	// Verify ListItemContinuationIndent matches prefix width
	if ListItemContinuationIndent != ListItemPrefixWidth {
		t.Errorf("ListItemContinuationIndent (%d) != ListItemPrefixWidth (%d)",
			ListItemContinuationIndent, ListItemPrefixWidth)
	}

	// Verify NumberedListPrefixWidth for single-digit numbers "  1. "
	numPrefix := "  1. "
	numVisualWidth := lipgloss.Width(numPrefix)
	if numVisualWidth != NumberedListPrefixWidth {
		t.Errorf("NumberedListPrefixWidth = %d, but actual prefix %q has visual width %d",
			NumberedListPrefixWidth, numPrefix, numVisualWidth)
	}

	// Verify ContentPadding equals Padding(0, 1) effect (1 char on each side)
	if ContentPadding != 2 {
		t.Errorf("ContentPadding should be 2 (1 left + 1 right), got %d", ContentPadding)
	}

	// Verify BorderSize equals 2 (top + bottom borders)
	if BorderSize != 2 {
		t.Errorf("BorderSize should be 2 (1 top + 1 bottom), got %d", BorderSize)
	}
}

// TestListItemWrapping verifies that list items wrap correctly and
// continuation lines align with the first line.
func TestListItemWrapping(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		width    int
		checkFn  func(result string) error
	}{
		{
			name:  "unordered list wraps at correct width",
			line:  "- This is a long list item that should wrap to multiple lines when the width is narrow enough",
			width: 40,
			checkFn: func(result string) error {
				lines := strings.Split(result, "\n")
				if len(lines) < 2 {
					return nil // Content might fit, that's ok
				}
				// First line should start with "  • "
				if !strings.HasPrefix(lines[0], "  ") {
					return errorf("first line should start with 2 spaces, got: %q", lines[0])
				}
				// Continuation lines should be indented with spaces
				for i := 1; i < len(lines); i++ {
					if !strings.HasPrefix(lines[i], "    ") {
						return errorf("continuation line %d should start with 4 spaces, got: %q", i, lines[i])
					}
				}
				return nil
			},
		},
		{
			name:  "numbered list wraps at correct width",
			line:  "1. This is a long numbered list item that should wrap to multiple lines when the width is narrow",
			width: 40,
			checkFn: func(result string) error {
				lines := strings.Split(result, "\n")
				if len(lines) < 2 {
					return nil // Content might fit
				}
				// First line should start with "  1. " (or styled version)
				if !strings.HasPrefix(lines[0], "  ") {
					return errorf("first line should start with 2 spaces, got: %q", lines[0])
				}
				// Continuation lines should be indented with 5 spaces for single-digit numbers
				for i := 1; i < len(lines); i++ {
					if !strings.HasPrefix(lines[i], "     ") {
						return errorf("continuation line %d should start with 5 spaces, got: %q", i, lines[i])
					}
				}
				return nil
			},
		},
		{
			name:  "double-digit numbered list has correct indent",
			line:  "10. This is item ten which has a wider prefix and should wrap correctly",
			width: 40,
			checkFn: func(result string) error {
				lines := strings.Split(result, "\n")
				if len(lines) < 2 {
					return nil // Content might fit
				}
				// Continuation lines should be indented with 6 spaces for double-digit numbers
				for i := 1; i < len(lines); i++ {
					if !strings.HasPrefix(lines[i], "      ") {
						return errorf("continuation line %d should start with 6 spaces for double-digit, got: %q", i, lines[i])
					}
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderMarkdownLine(tt.line, tt.width)
			if err := tt.checkFn(result); err != nil {
				t.Error(err)
			}
		})
	}
}

// errorf is a helper that returns an error with formatting
func errorf(format string, args ...interface{}) error {
	return &testError{msg: sprintf(format, args...)}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func sprintf(format string, args ...interface{}) string {
	// Simple sprintf implementation for test errors
	result := format
	for _, arg := range args {
		switch v := arg.(type) {
		case string:
			result = strings.Replace(result, "%q", "\""+v+"\"", 1)
			result = strings.Replace(result, "%s", v, 1)
		case int:
			result = strings.Replace(result, "%d", itoa(v), 1)
		}
	}
	return result
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var s string
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

// TestBlockquoteWrapping verifies blockquote content wraps correctly
func TestBlockquoteWrapping(t *testing.T) {
	line := "> This is a long blockquote that should be wrapped to fit within the available width minus the blockquote prefix"
	result := renderMarkdownLine(line, 50)

	// Result should contain the content
	if !strings.Contains(result, "blockquote") {
		t.Error("Result should contain 'blockquote'")
	}

	// Visual width of each line should not exceed the width
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		visualWidth := lipgloss.Width(line)
		if visualWidth > 55 { // Allow some buffer for styling
			t.Errorf("Line %d visual width %d exceeds limit: %q", i, visualWidth, line)
		}
	}
}

// =============================================================================
// Message Cache Behavior Tests
// =============================================================================

// TestMessageCache_InvalidatesOnWidthChange verifies the cache is re-rendered
// when viewport width changes.
func TestMessageCache_InvalidatesOnWidthChange(t *testing.T) {
	chat := NewChat()

	// Set initial size first, then session
	chat.SetSize(80, 24)
	chat.SetSession("test", []claude.Message{
		{Role: "user", Content: "Hello world"},
		{Role: "assistant", Content: "Hi there"},
	})

	// Force render to populate cache
	chat.View()

	// Cache should be populated after View
	if len(chat.messageCache) != 2 {
		t.Fatalf("Expected 2 cached messages after initial render, got %d", len(chat.messageCache))
	}

	// Remember the wrap width from first render
	firstWrapWidth := chat.messageCache[0].wrapWidth

	// Change width significantly - this triggers re-render with new wrap width
	chat.SetSize(120, 24)

	// After SetSize with width change, cache should be repopulated (not nil)
	// because SetSize now calls updateContent() when width changes
	if len(chat.messageCache) != 2 {
		t.Fatalf("Expected 2 cached messages after resize, got %d", len(chat.messageCache))
	}

	// Wrap width should be different
	secondWrapWidth := chat.messageCache[0].wrapWidth
	if secondWrapWidth == firstWrapWidth {
		t.Errorf("Expected different wrap width after resize, but both are %d", firstWrapWidth)
	}
}

// TestMessageCache_HitsOnSameWidth verifies cache hits when content
// and width are unchanged.
func TestMessageCache_HitsOnSameWidth(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", []claude.Message{
		{Role: "user", Content: "Test message"},
	})

	// Force render to populate cache
	chat.View()

	if len(chat.messageCache) == 0 {
		t.Fatal("Cache should be populated after View")
	}

	// Get first rendered content
	firstRendered := chat.messageCache[0].rendered

	// Call View again (should use cache)
	chat.View()

	// Rendered content should be identical (same object reference or same content)
	secondRendered := chat.messageCache[0].rendered
	if firstRendered != secondRendered {
		t.Error("Expected cache hit - rendered content should be identical")
	}
}

// TestMessageCache_GrowsWithNewMessages verifies cache grows when
// new messages are added.
func TestMessageCache_GrowsWithNewMessages(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", []claude.Message{
		{Role: "user", Content: "First message"},
	})

	// Force render to populate cache
	chat.View()

	// Should have 1 cached message
	initialCacheLen := len(chat.messageCache)
	if initialCacheLen != 1 {
		t.Fatalf("Expected 1 cached message, got %d", initialCacheLen)
	}

	// Add a new message through the API
	chat.AddUserMessage("Second message")

	// Cache should now have 2 entries
	if len(chat.messageCache) != 2 {
		t.Errorf("Expected 2 cached messages after AddUserMessage, got %d", len(chat.messageCache))
	}

	// The new message should be cached
	if len(chat.messageCache) >= 2 && !strings.Contains(chat.messageCache[1].content, "Second") {
		t.Error("Second message should be in cache")
	}
}

// TestMessageCache_ClearedOnSessionChange verifies cache is cleared
// when session changes.
func TestMessageCache_ClearedOnSessionChange(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test1", []claude.Message{
		{Role: "user", Content: "Session 1 message"},
	})

	// Force render to populate cache
	chat.View()

	// Cache should be populated
	if len(chat.messageCache) == 0 {
		t.Fatal("Expected cache to be populated after View")
	}

	// Change session
	chat.SetSession("test2", []claude.Message{
		{Role: "user", Content: "Session 2 message"},
	})

	// Force render with new session
	chat.View()

	// Verify no stale content from old session
	if len(chat.messageCache) > 0 && strings.Contains(chat.messageCache[0].content, "Session 1") {
		t.Error("Cache contains stale content from old session")
	}

	// Verify new session content is cached
	if len(chat.messageCache) > 0 && !strings.Contains(chat.messageCache[0].content, "Session 2") {
		t.Error("Cache should contain Session 2 content")
	}
}

// =============================================================================
// Boundary and Edge Case Tests
// =============================================================================

// TestMinimumTerminalSize verifies the UI handles minimum terminal sizes
func TestMinimumTerminalSize(t *testing.T) {
	chat := NewChat()

	// Test with MinTerminalWidth and MinTerminalHeight
	chat.SetSize(MinTerminalWidth, MinTerminalHeight)
	chat.SetSession("test", []claude.Message{
		{Role: "user", Content: "Test"},
	})

	// Should not panic and should produce valid output
	view := chat.View()
	if view == "" {
		t.Error("View should not be empty at minimum terminal size")
	}
}

// TestVeryNarrowWidth verifies wrapping at very narrow widths
func TestVeryNarrowWidth(t *testing.T) {
	tests := []struct {
		name    string
		content string
		width   int
	}{
		{
			name:    "narrow width with short words",
			content: "a b c d e",
			width:   5,
		},
		{
			name:    "narrow width with long word",
			content: "superlongwordthatcannotbreak",
			width:   10,
		},
		{
			name:    "minimum wrap width",
			content: "test content here",
			width:   MinWrapWidth,
		},
		{
			name:    "below minimum wrap width uses default",
			content: "test content",
			width:   5, // Below MinWrapWidth
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			result := renderMarkdown(tt.content, tt.width)
			if result == "" && tt.content != "" {
				t.Error("Result should not be empty for non-empty content")
			}
		})
	}
}

// TestVerySmallViewport verifies the chat panel handles very small viewports
func TestVerySmallViewport(t *testing.T) {
	chat := NewChat()

	sizes := []struct {
		width  int
		height int
	}{
		{40, 10}, // Minimum
		{50, 12}, // Slightly above minimum
		{30, 8},  // Below minimum (should clamp)
		{1, 1},   // Extreme minimum
	}

	for _, size := range sizes {
		t.Run(itoa(size.width)+"x"+itoa(size.height), func(t *testing.T) {
			// Should not panic
			chat.SetSize(size.width, size.height)
			chat.SetSession("test", []claude.Message{
				{Role: "user", Content: "Test message"},
			})
			view := chat.View()
			if view == "" {
				t.Error("View should not be empty")
			}
		})
	}
}

// TestResizeSequence verifies multiple resize operations
func TestResizeSequence(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", []claude.Message{
		{Role: "user", Content: "This is a message that may need to rewrap on resize"},
	})

	// Sequence of resize operations
	sizes := []struct{ w, h int }{
		{80, 24},
		{120, 40},
		{60, 20},
		{80, 24},  // Back to original
		{40, 10},  // Minimum
		{200, 60}, // Large
		{80, 24},  // Back to original again
	}

	for _, size := range sizes {
		chat.SetSize(size.w, size.h)
		view := chat.View()
		if view == "" {
			t.Errorf("View empty after resize to %dx%d", size.w, size.h)
		}
	}
}

// =============================================================================
// Table Overflow Tests
// =============================================================================

// TestTableColumnOverflow verifies tables handle more columns than available width
func TestTableColumnOverflow(t *testing.T) {
	// Table with many columns
	rows := [][]string{
		{"A", "B", "C", "D", "E", "F", "G", "H"},
		{"1", "2", "3", "4", "5", "6", "7", "8"},
	}

	// Render at narrow width (8 columns * min 3 chars + borders = 33 chars minimum)
	result := renderTable(rows, true, 30) // Less than minimum

	// Should still render without panic
	if result == "" {
		t.Error("Table should render even at narrow width")
	}

	// Should contain all column headers
	for _, header := range []string{"A", "B", "C", "D", "E", "F", "G", "H"} {
		if !strings.Contains(result, header) {
			t.Errorf("Table should contain header %q", header)
		}
	}
}

// TestTableNarrowWidth verifies table rendering at various narrow widths
func TestTableNarrowWidth(t *testing.T) {
	rows := [][]string{
		{"Column 1", "Column 2"},
		{"Data that is longer than column", "Short"},
	}

	widths := []int{20, 30, 40, 50, 60}

	for _, width := range widths {
		t.Run(itoa(width)+"_width", func(t *testing.T) {
			result := renderTable(rows, true, width)
			if result == "" {
				t.Errorf("Table should render at width %d", width)
			}

			// Each line should not drastically exceed the width
			// (some overflow is acceptable for borders and content)
			lines := strings.Split(result, "\n")
			for i, line := range lines {
				visualWidth := lipgloss.Width(line)
				// Allow 20% buffer for styling and edge cases
				maxAllowed := width + width/5 + 10
				if visualWidth > maxAllowed {
					t.Errorf("Line %d width %d exceeds allowed %d at render width %d: %q",
						i, visualWidth, maxAllowed, width, line)
				}
			}
		})
	}
}

// TestCalculateTableColumnWidths verifies the column width distribution algorithm
func TestCalculateTableColumnWidths(t *testing.T) {
	tests := []struct {
		name           string
		naturalWidths  []int
		availableWidth int
		checkFn        func(result []int) error
	}{
		{
			name:           "fits naturally",
			naturalWidths:  []int{10, 20, 15},
			availableWidth: 50,
			checkFn: func(result []int) error {
				// Should use natural widths
				if result[0] != 10 || result[1] != 20 || result[2] != 15 {
					return errorf("expected natural widths [10, 20, 15], got %v", result)
				}
				return nil
			},
		},
		{
			name:           "needs shrinking",
			naturalWidths:  []int{30, 30, 30},
			availableWidth: 60,
			checkFn: func(result []int) error {
				// Total should not exceed available
				total := 0
				for _, w := range result {
					total += w
				}
				if total > 60 {
					return errorf("total width %d exceeds available 60", total)
				}
				return nil
			},
		},
		{
			name:           "minimum width columns preserved",
			naturalWidths:  []int{3, 3, 3}, // Already at minimum (3)
			availableWidth: 12,             // Exactly enough for 3 columns at min width
			checkFn: func(result []int) error {
				// Each should be exactly 3 since that's the natural width
				// and there's just enough space
				for i, w := range result {
					if w != 3 {
						return errorf("column %d width %d, expected 3", i, w)
					}
				}
				return nil
			},
		},
		{
			name:           "very narrow available",
			naturalWidths:  []int{50, 50, 50},
			availableWidth: 10,
			checkFn: func(result []int) error {
				// Each should be at least minimum
				for i, w := range result {
					if w < TableMinColumnWidth {
						return errorf("column %d width %d below minimum", i, w)
					}
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateTableColumnWidths(tt.naturalWidths, tt.availableWidth)
			if err := tt.checkFn(result); err != nil {
				t.Error(err)
			}
		})
	}
}

// TestTableWithLongCellContent verifies tables handle very long cell content
func TestTableWithLongCellContent(t *testing.T) {
	rows := [][]string{
		{"Header", "Description"},
		{"Short", strings.Repeat("This is very long content. ", 10)},
	}

	result := renderTable(rows, true, 60)

	// Should contain content
	if !strings.Contains(result, "Header") {
		t.Error("Table should contain 'Header'")
	}

	// Should have proper borders
	if !strings.Contains(result, "┌") || !strings.Contains(result, "┘") {
		t.Error("Table should have proper corner borders")
	}
}

// =============================================================================
// Overlay Box Width Tests
// =============================================================================

// TestOverlayBoxWidthCapping verifies overlay boxes respect max widths
func TestOverlayBoxWidthCapping(t *testing.T) {
	// Test permission prompt at wide width
	permResult := renderPermissionPrompt("Bash", "rm -rf /", 200)
	permLines := strings.Split(permResult, "\n")
	for i, line := range permLines {
		visualWidth := lipgloss.Width(line)
		if visualWidth > OverlayBoxMaxWidth+10 { // Allow some margin for borders
			t.Errorf("Permission box line %d width %d exceeds max %d",
				i, visualWidth, OverlayBoxMaxWidth)
		}
	}

	// Test todo list at wide width
	todoList := &claude.TodoList{
		Items: []claude.TodoItem{
			{Content: "A todo item", Status: claude.TodoStatusPending, ActiveForm: "Pending"},
		},
	}
	todoResult := renderTodoList(todoList, 200)
	todoLines := strings.Split(todoResult, "\n")
	for i, line := range todoLines {
		visualWidth := lipgloss.Width(line)
		if visualWidth > OverlayBoxMaxWidth+10 {
			t.Errorf("Todo box line %d width %d exceeds max %d",
				i, visualWidth, OverlayBoxMaxWidth)
		}
	}
}

// TestPlanBoxWidthCapping verifies plan boxes use their specific max width
func TestPlanBoxWidthCapping(t *testing.T) {
	chat := NewChat()
	chat.SetSize(200, 40) // Wide terminal
	chat.SetSession("test", nil)
	chat.SetPendingPlanApproval("# Test Plan\n\nThis is a test plan.", nil)

	result := chat.renderPlanApprovalPrompt(150)
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		visualWidth := lipgloss.Width(line)
		if visualWidth > PlanBoxMaxWidth+10 { // Allow some margin
			t.Errorf("Plan box line %d width %d exceeds max %d",
				i, visualWidth, PlanBoxMaxWidth)
		}
	}
}

// =============================================================================
// Constants Consistency Tests
// =============================================================================

// TestLayoutConstantsConsistency verifies layout constants are consistent
func TestLayoutConstantsConsistency(t *testing.T) {
	// InputTotalHeight should equal TextareaHeight + TextareaBorderHeight
	if InputTotalHeight != TextareaHeight+TextareaBorderHeight {
		t.Errorf("InputTotalHeight (%d) != TextareaHeight (%d) + TextareaBorderHeight (%d)",
			InputTotalHeight, TextareaHeight, TextareaBorderHeight)
	}

	// MinTerminalHeight should be greater than basic chrome
	minChrome := HeaderHeight + FooterHeight + InputTotalHeight + BorderSize
	if MinTerminalHeight < minChrome {
		t.Errorf("MinTerminalHeight (%d) is less than minimum chrome (%d)",
			MinTerminalHeight, minChrome)
	}

	// OverlayBoxMaxWidth should be less than or equal to PlanBoxMaxWidth
	if OverlayBoxMaxWidth > PlanBoxMaxWidth {
		t.Errorf("OverlayBoxMaxWidth (%d) > PlanBoxMaxWidth (%d)",
			OverlayBoxMaxWidth, PlanBoxMaxWidth)
	}
}

// =============================================================================
// Final Stats and Model Breakdown Tests
// =============================================================================

func TestChat_FinalStatsPreservedAfterStreaming(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(80, 24)

	// Start waiting/streaming
	chat.SetWaiting(true)

	// Add streaming content
	chat.AppendStreaming("Hello, I'm Claude!")

	// Set stream stats with model breakdown
	stats := &claude.StreamStats{
		OutputTokens: 231,
		TotalCostUSD: 0.085,
		ByModel: []claude.ModelTokenCount{
			{Model: "claude-opus-4-5-20251101", OutputTokens: 207},
			{Model: "claude-haiku-4-5-20251001", OutputTokens: 24},
		},
	}
	chat.SetStreamStats(stats)

	// Verify stats are set
	if chat.streamStats == nil {
		t.Fatal("streamStats should be set")
	}

	// Finish streaming
	chat.FinishStreaming()

	// finalStats should be preserved
	if chat.finalStats == nil {
		t.Fatal("finalStats should be preserved after FinishStreaming")
	}

	if chat.finalStats.OutputTokens != 231 {
		t.Errorf("finalStats.OutputTokens = %d, want 231", chat.finalStats.OutputTokens)
	}

	if len(chat.finalStats.ByModel) != 2 {
		t.Errorf("finalStats.ByModel length = %d, want 2", len(chat.finalStats.ByModel))
	}
}

func TestChat_FinalStatsClearedOnNewRequest(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)

	// Set some final stats
	chat.finalStats = &claude.StreamStats{
		OutputTokens: 100,
		ByModel: []claude.ModelTokenCount{
			{Model: "claude-opus-4-5-20251101", OutputTokens: 100},
		},
	}

	// Start a new request
	chat.SetWaiting(true)

	// finalStats should be cleared
	if chat.finalStats != nil {
		t.Error("finalStats should be cleared when starting new request")
	}
}

func TestShortModelName(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{"claude-opus-4-5-20251101", "opus"},
		{"claude-sonnet-3-5-20241022", "sonnet"},
		{"claude-haiku-4-5-20251001", "haiku"},
		{"unknown-model", "unknown-model"}, // Falls back to full name when no pattern matches
		{"simple", "simple"},               // Single part fallback
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := shortModelName(tt.model)
			if result != tt.expected {
				t.Errorf("shortModelName(%q) = %q, want %q", tt.model, result, tt.expected)
			}
		})
	}
}

func TestRenderFinalStats(t *testing.T) {
	// Test nil stats
	result := renderFinalStats(nil)
	if result != "" {
		t.Errorf("renderFinalStats(nil) = %q, want empty string", result)
	}

	// Test zero tokens
	stats := &claude.StreamStats{OutputTokens: 0}
	result = renderFinalStats(stats)
	if result != "" {
		t.Errorf("renderFinalStats with 0 tokens = %q, want empty string", result)
	}

	// Test single model (no breakdown)
	stats = &claude.StreamStats{
		OutputTokens: 100,
		ByModel: []claude.ModelTokenCount{
			{Model: "claude-opus-4-5", OutputTokens: 100},
		},
	}
	result = renderFinalStats(stats)
	if !strings.Contains(result, "100 tokens") {
		t.Errorf("renderFinalStats should contain '100 tokens', got %q", result)
	}
	// Single model should NOT show breakdown
	if strings.Contains(result, "opus:") {
		t.Errorf("Single model should not show breakdown, got %q", result)
	}

	// Test multiple models (shows breakdown)
	stats = &claude.StreamStats{
		OutputTokens: 231,
		ByModel: []claude.ModelTokenCount{
			{Model: "claude-opus-4-5-20251101", OutputTokens: 207},
			{Model: "claude-haiku-4-5-20251001", OutputTokens: 24},
		},
	}
	result = renderFinalStats(stats)
	if !strings.Contains(result, "231 tokens") {
		t.Errorf("renderFinalStats should contain '231 tokens', got %q", result)
	}
	if !strings.Contains(result, "opus:") {
		t.Errorf("Multi-model should show opus breakdown, got %q", result)
	}
	if !strings.Contains(result, "haiku:") {
		t.Errorf("Multi-model should show haiku breakdown, got %q", result)
	}
}

func TestRenderCompletionFlash_WithStats(t *testing.T) {
	// Test without stats
	result := renderCompletionFlash(0, nil)
	if !strings.Contains(result, "Done") {
		t.Errorf("Completion flash frame 0 should contain 'Done', got %q", result)
	}

	// Test with stats
	stats := &claude.StreamStats{
		OutputTokens: 231,
		ByModel: []claude.ModelTokenCount{
			{Model: "claude-opus-4-5", OutputTokens: 207},
			{Model: "claude-haiku-4-5", OutputTokens: 24},
		},
	}
	result = renderCompletionFlash(0, stats)
	if !strings.Contains(result, "Done") {
		t.Errorf("Completion flash with stats should contain 'Done', got %q", result)
	}
	if !strings.Contains(result, "231") {
		t.Errorf("Completion flash with stats should contain token count, got %q", result)
	}

	// Frame 1 should also show stats
	result = renderCompletionFlash(1, stats)
	if !strings.Contains(result, "231") {
		t.Errorf("Completion flash frame 1 should contain token count, got %q", result)
	}

	// Frame 2+ should be empty
	result = renderCompletionFlash(2, stats)
	if result != "" {
		t.Errorf("Completion flash frame 2+ should be empty, got %q", result)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		ms       int
		expected string
	}{
		{0, "0s"},
		{500, "0s"},
		{1000, "1s"},
		{5000, "5s"},
		{30000, "30s"},
		{59000, "59s"},
		{60000, "1m0s"},
		{90000, "1m30s"},
		{120000, "2m0s"},
		{125000, "2m5s"},
		{3600000, "60m0s"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatDuration(tt.ms)
			if result != tt.expected {
				t.Errorf("formatDuration(%d) = %q, want %q", tt.ms, result, tt.expected)
			}
		})
	}
}

func TestRenderFinalStats_WithTiming(t *testing.T) {
	// Test timing only (no tokens)
	stats := &claude.StreamStats{
		OutputTokens: 0,
		DurationMs:   45000,
	}
	result := renderFinalStats(stats)
	if !strings.Contains(result, "45s") {
		t.Errorf("renderFinalStats with timing should contain '45s', got %q", result)
	}

	// Test tokens and timing together
	stats = &claude.StreamStats{
		OutputTokens: 500,
		DurationMs:   90000,
	}
	result = renderFinalStats(stats)
	if !strings.Contains(result, "500 tokens") {
		t.Errorf("renderFinalStats should contain '500 tokens', got %q", result)
	}
	if !strings.Contains(result, "1m30s") {
		t.Errorf("renderFinalStats should contain '1m30s', got %q", result)
	}
	// Should use bullet separator
	if !strings.Contains(result, "•") {
		t.Errorf("renderFinalStats should use bullet separator between tokens and timing, got %q", result)
	}

	// Test multi-model with timing
	stats = &claude.StreamStats{
		OutputTokens: 231,
		DurationMs:   46000,
		ByModel: []claude.ModelTokenCount{
			{Model: "claude-opus-4-5-20251101", OutputTokens: 207},
			{Model: "claude-haiku-4-5-20251001", OutputTokens: 24},
		},
	}
	result = renderFinalStats(stats)
	if !strings.Contains(result, "231 tokens") {
		t.Errorf("renderFinalStats should contain '231 tokens', got %q", result)
	}
	if !strings.Contains(result, "opus:") {
		t.Errorf("renderFinalStats should contain model breakdown, got %q", result)
	}
	if !strings.Contains(result, "46s") {
		t.Errorf("renderFinalStats should contain '46s', got %q", result)
	}
}

func TestRenderCompletionFlash_WithTiming(t *testing.T) {
	// Test with timing only
	stats := &claude.StreamStats{
		OutputTokens: 0,
		DurationMs:   30000,
	}
	result := renderCompletionFlash(0, stats)
	if !strings.Contains(result, "Done") {
		t.Errorf("Completion flash should contain 'Done', got %q", result)
	}
	if !strings.Contains(result, "30s") {
		t.Errorf("Completion flash should contain timing '30s', got %q", result)
	}

	// Test with both tokens and timing
	stats = &claude.StreamStats{
		OutputTokens: 100,
		DurationMs:   45000,
	}
	result = renderCompletionFlash(0, stats)
	if !strings.Contains(result, "100 tokens") {
		t.Errorf("Completion flash should contain '100 tokens', got %q", result)
	}
	if !strings.Contains(result, "45s") {
		t.Errorf("Completion flash should contain '45s', got %q", result)
	}
}

// =============================================================================
// Image Attachment Layout Tests
// =============================================================================

func TestChat_ImageAttachment_DynamicInputHeight(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(100, 30)

	// Initially, no image attached
	if chat.HasPendingImage() {
		t.Error("New chat should not have pending image")
	}
	if chat.getInputTotalHeight() != InputTotalHeight {
		t.Errorf("Without image, input height should be %d, got %d", InputTotalHeight, chat.getInputTotalHeight())
	}

	// Attach an image
	chat.AttachImage([]byte("test image data"), "image/png")

	if !chat.HasPendingImage() {
		t.Error("Chat should have pending image after AttachImage")
	}
	expectedHeight := InputTotalHeight + ImageIndicatorHeight
	if chat.getInputTotalHeight() != expectedHeight {
		t.Errorf("With image, input height should be %d, got %d", expectedHeight, chat.getInputTotalHeight())
	}

	// Clear the image
	chat.ClearImage()

	if chat.HasPendingImage() {
		t.Error("Chat should not have pending image after ClearImage")
	}
	if chat.getInputTotalHeight() != InputTotalHeight {
		t.Errorf("After clearing image, input height should be %d, got %d", InputTotalHeight, chat.getInputTotalHeight())
	}
}

func TestChat_ImageAttachment_ViewportResize(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(100, 30)

	// Record viewport height without image
	heightWithoutImage := chat.viewport.Height()

	// Attach image and check viewport was resized
	chat.AttachImage([]byte("test image data"), "image/png")
	heightWithImage := chat.viewport.Height()

	// Viewport should be smaller when image is attached (makes room for indicator)
	if heightWithImage >= heightWithoutImage {
		t.Errorf("Viewport height with image (%d) should be less than without (%d)", heightWithImage, heightWithoutImage)
	}

	// Clear image and verify viewport returns to original size
	chat.ClearImage()
	heightAfterClear := chat.viewport.Height()

	if heightAfterClear != heightWithoutImage {
		t.Errorf("Viewport height after clearing image (%d) should equal original (%d)", heightAfterClear, heightWithoutImage)
	}
}

func TestChat_ImageAttachment_ViewContainsIndicator(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(100, 30)

	// Attach a 2KB image
	imageData := make([]byte, 2048)
	chat.AttachImage(imageData, "image/png")

	view := chat.View()

	// View should contain the image indicator
	if !strings.Contains(view, "Image attached") {
		t.Error("View should contain 'Image attached' when image is pending")
	}
	if !strings.Contains(view, "2KB") {
		t.Error("View should show image size '2KB'")
	}
	if !strings.Contains(view, "backspace to remove") {
		t.Error("View should contain removal instructions")
	}

	// Clear and verify indicator is gone
	chat.ClearImage()
	view = chat.View()

	if strings.Contains(view, "Image attached") {
		t.Error("View should not contain 'Image attached' after clearing")
	}
}

func TestChat_GetPendingImageSizeKB(t *testing.T) {
	chat := NewChat()

	// No image
	if chat.GetPendingImageSizeKB() != 0 {
		t.Errorf("Size should be 0 with no image, got %d", chat.GetPendingImageSizeKB())
	}

	// 1.5KB image
	chat.AttachImage(make([]byte, 1536), "image/png")
	if chat.GetPendingImageSizeKB() != 1 {
		t.Errorf("Size should be 1 for 1536 bytes, got %d", chat.GetPendingImageSizeKB())
	}

	// 10KB image
	chat.AttachImage(make([]byte, 10240), "image/png")
	if chat.GetPendingImageSizeKB() != 10 {
		t.Errorf("Size should be 10 for 10240 bytes, got %d", chat.GetPendingImageSizeKB())
	}
}

// =============================================================================
// Subagent Indicator Tests
// =============================================================================

func TestChat_SubagentModel_SetAndGet(t *testing.T) {
	chat := NewChat()
	chat.SetSize(100, 40)
	chat.SetSession("test-session", nil)

	// Initially should be empty
	if chat.GetSubagentModel() != "" {
		t.Errorf("Expected empty subagent model initially, got %q", chat.GetSubagentModel())
	}

	// Set a subagent model
	chat.SetSubagentModel("claude-haiku-4-5-20251001")
	if chat.GetSubagentModel() != "claude-haiku-4-5-20251001" {
		t.Errorf("Expected haiku model, got %q", chat.GetSubagentModel())
	}

	// Clear the subagent model
	chat.ClearSubagentModel()
	if chat.GetSubagentModel() != "" {
		t.Errorf("Expected empty subagent model after clear, got %q", chat.GetSubagentModel())
	}
}

func TestChat_RenderStreamingStatus_WithSubagent(t *testing.T) {
	// Test that the streaming status includes subagent indicator
	elapsed := 5 * time.Second
	stats := &claude.StreamStats{OutputTokens: 100}

	// Without subagent
	statusNoSubagent := renderStreamingStatus("Thinking", 0, elapsed, stats, "")
	if strings.Contains(statusNoSubagent, "haiku") {
		t.Error("Status without subagent should not contain haiku")
	}
	if strings.Contains(statusNoSubagent, "working") {
		t.Error("Status without subagent should not contain 'working'")
	}

	// With subagent (Haiku)
	statusWithSubagent := renderStreamingStatus("Thinking", 0, elapsed, stats, "claude-haiku-4-5-20251001")
	if !strings.Contains(statusWithSubagent, "haiku") {
		t.Error("Status with haiku subagent should contain 'haiku'")
	}
	if !strings.Contains(statusWithSubagent, "working") {
		t.Error("Status with subagent should contain 'working'")
	}
}
