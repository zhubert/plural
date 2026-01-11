package ui

import (
	"strings"
	"testing"

	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/mcp"
)

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
			line:  "⏺ Working",
			check: func(s string) bool { return strings.Contains(s, "⏺") },
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

func TestRenderSpinner(t *testing.T) {
	verbs := []string{"Thinking", "Pondering", "Analyzing"}
	for _, verb := range verbs {
		for i := 0; i < len(spinnerFrames); i++ {
			for w := 0; w < len(waveFrames); w++ {
				result := renderSpinner(verb, i, w)
				if result == "" {
					t.Errorf("renderSpinner(%q, %d, %d) returned empty string", verb, i, w)
				}
				if !strings.Contains(result, verb) {
					t.Errorf("renderSpinner(%q, %d, %d) = %q, should contain verb", verb, i, w, result)
				}
				if !strings.Contains(result, "...") {
					t.Errorf("renderSpinner(%q, %d, %d) = %q, should contain ellipsis", verb, i, w, result)
				}
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

func TestChat_ToolUseMarkers(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)

	// Append tool use
	chat.AppendToolUse("Read", "file.go")

	streaming := chat.GetStreaming()
	if !strings.Contains(streaming, ToolUseInProgress) {
		t.Error("Expected in-progress marker in streaming")
	}
	if !strings.Contains(streaming, "Reading") {
		t.Error("Expected 'Reading' icon in streaming")
	}
	if !strings.Contains(streaming, "file.go") {
		t.Error("Expected 'file.go' in streaming")
	}

	// Mark complete
	chat.MarkLastToolUseComplete()

	streaming = chat.GetStreaming()
	if strings.Contains(streaming, ToolUseInProgress) {
		t.Error("Should not have in-progress marker after completion")
	}
	if !strings.Contains(streaming, ToolUseComplete) {
		t.Error("Expected complete marker in streaming")
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
	if chat.waitingVerb == "" {
		t.Error("Expected waiting verb to be set")
	}

	// Clear waiting
	chat.SetWaiting(false)
	if chat.IsWaiting() {
		t.Error("Should not be waiting after SetWaiting(false)")
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

	if chat.pendingPermissionTool != "Bash" {
		t.Errorf("Expected tool 'Bash', got %q", chat.pendingPermissionTool)
	}

	if chat.pendingPermissionDesc != "Run: git status" {
		t.Errorf("Expected description 'Run: git status', got %q", chat.pendingPermissionDesc)
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

	if len(chat.pendingQuestions) != 1 {
		t.Errorf("Expected 1 question, got %d", len(chat.pendingQuestions))
	}

	// Test question selection
	chat.MoveQuestionSelection(1) // Move down
	if chat.selectedOptionIdx != 1 {
		t.Errorf("Expected selected index 1, got %d", chat.selectedOptionIdx)
	}

	chat.MoveQuestionSelection(-1) // Move up
	if chat.selectedOptionIdx != 0 {
		t.Errorf("Expected selected index 0 after moving up, got %d", chat.selectedOptionIdx)
	}

	// Test wrap-around
	chat.MoveQuestionSelection(-1) // Should wrap to last
	expectedIdx := len(questions[0].Options) // +1 for "Other" option
	if chat.selectedOptionIdx != expectedIdx {
		t.Errorf("Expected wrap to %d, got %d", expectedIdx, chat.selectedOptionIdx)
	}

	// Select by number
	chat.selectedOptionIdx = 0
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

	if chat.currentQuestionIdx != 1 {
		t.Errorf("Expected currentQuestionIdx 1, got %d", chat.currentQuestionIdx)
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
	if ToolUseInProgress != "⏺" {
		t.Errorf("Expected ToolUseInProgress to be ⏺, got %q", ToolUseInProgress)
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
