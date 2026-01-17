package ui

import (
	"strings"
	"testing"
	"time"

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

// =============================================================================
// Text Selection Tests
// =============================================================================

func TestChat_StartSelection(t *testing.T) {
	chat := NewChat()

	// Start selection at position (5, 10)
	chat.StartSelection(5, 10)

	if chat.selectionStartCol != 5 {
		t.Errorf("Expected selectionStartCol 5, got %d", chat.selectionStartCol)
	}
	if chat.selectionStartLine != 10 {
		t.Errorf("Expected selectionStartLine 10, got %d", chat.selectionStartLine)
	}
	if chat.selectionEndCol != 5 {
		t.Errorf("Expected selectionEndCol 5, got %d", chat.selectionEndCol)
	}
	if chat.selectionEndLine != 10 {
		t.Errorf("Expected selectionEndLine 10, got %d", chat.selectionEndLine)
	}
	if !chat.selectionActive {
		t.Error("Expected selectionActive to be true")
	}
}

func TestChat_EndSelection(t *testing.T) {
	chat := NewChat()

	// EndSelection without active selection should do nothing
	chat.EndSelection(10, 20)
	if chat.selectionEndCol != 0 || chat.selectionEndLine != 0 {
		t.Error("EndSelection should not modify coordinates when selection is not active")
	}

	// Start selection then end it
	chat.StartSelection(5, 10)
	chat.EndSelection(15, 25)

	if chat.selectionEndCol != 15 {
		t.Errorf("Expected selectionEndCol 15, got %d", chat.selectionEndCol)
	}
	if chat.selectionEndLine != 25 {
		t.Errorf("Expected selectionEndLine 25, got %d", chat.selectionEndLine)
	}
	// Start position should be unchanged
	if chat.selectionStartCol != 5 {
		t.Errorf("Expected selectionStartCol unchanged at 5, got %d", chat.selectionStartCol)
	}
	if chat.selectionStartLine != 10 {
		t.Errorf("Expected selectionStartLine unchanged at 10, got %d", chat.selectionStartLine)
	}
}

func TestChat_SelectionStop(t *testing.T) {
	chat := NewChat()
	chat.StartSelection(5, 10)
	chat.EndSelection(15, 20)

	if !chat.selectionActive {
		t.Error("Expected selectionActive to be true before stop")
	}

	chat.SelectionStop()

	if chat.selectionActive {
		t.Error("Expected selectionActive to be false after stop")
	}
	// Coordinates should be preserved
	if chat.selectionStartCol != 5 || chat.selectionStartLine != 10 {
		t.Error("Selection start coordinates should be preserved after stop")
	}
	if chat.selectionEndCol != 15 || chat.selectionEndLine != 20 {
		t.Error("Selection end coordinates should be preserved after stop")
	}
}

func TestChat_SelectionClear(t *testing.T) {
	chat := NewChat()
	chat.StartSelection(5, 10)
	chat.EndSelection(15, 20)
	chat.SelectionStop()

	chat.SelectionClear()

	if chat.selectionStartCol != -1 {
		t.Errorf("Expected selectionStartCol -1, got %d", chat.selectionStartCol)
	}
	if chat.selectionStartLine != -1 {
		t.Errorf("Expected selectionStartLine -1, got %d", chat.selectionStartLine)
	}
	if chat.selectionEndCol != -1 {
		t.Errorf("Expected selectionEndCol -1, got %d", chat.selectionEndCol)
	}
	if chat.selectionEndLine != -1 {
		t.Errorf("Expected selectionEndLine -1, got %d", chat.selectionEndLine)
	}
	if chat.selectionActive {
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
			chat.selectionStartCol = tt.startCol
			chat.selectionStartLine = tt.startLine
			chat.selectionEndCol = tt.endCol
			chat.selectionEndLine = tt.endLine

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

	if chat.clickCount != 1 {
		t.Errorf("Expected clickCount 1, got %d", chat.clickCount)
	}
	if chat.selectionStartCol != 10 {
		t.Errorf("Expected selectionStartCol 10, got %d", chat.selectionStartCol)
	}
	if chat.selectionStartLine != 5 {
		t.Errorf("Expected selectionStartLine 5, got %d", chat.selectionStartLine)
	}
	if !chat.selectionActive {
		t.Error("Expected selectionActive to be true after single click")
	}
}

func TestChat_HandleMouseClick_ClickCountReset(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(80, 24)

	// First click
	_ = chat.handleMouseClick(10, 5)
	if chat.clickCount != 1 {
		t.Errorf("Expected clickCount 1 after first click, got %d", chat.clickCount)
	}

	// Click far away - should reset count to 1 (new click sequence)
	_ = chat.handleMouseClick(50, 20)
	if chat.clickCount != 1 {
		t.Errorf("Expected clickCount reset to 1 when clicking far away, got %d", chat.clickCount)
	}
}

func TestChat_HandleMouseClick_DoubleClick(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(80, 24)

	// Simulate rapid double click at same position
	_ = chat.handleMouseClick(10, 5)
	if chat.clickCount != 1 {
		t.Errorf("Expected clickCount 1 after first click, got %d", chat.clickCount)
	}

	// Second click at same position (within tolerance and time threshold)
	_ = chat.handleMouseClick(10, 5)
	if chat.clickCount != 2 {
		t.Errorf("Expected clickCount 2 after second click at same position, got %d", chat.clickCount)
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
	if chat.clickCount != 0 {
		t.Errorf("Expected clickCount 0 after triple click, got %d", chat.clickCount)
	}
}

func TestChat_HandleMouseClick_TripleClickResets(t *testing.T) {
	chat := NewChat()
	chat.SetSession("test", nil)
	chat.SetSize(80, 24)

	// Simulate clicks at same position
	chat.lastClickX = 10
	chat.lastClickY = 5

	// First click
	_ = chat.handleMouseClick(10, 5)
	// Second click
	_ = chat.handleMouseClick(10, 5)
	// Third click - should reset to 0
	_ = chat.handleMouseClick(10, 5)

	if chat.clickCount != 0 {
		t.Errorf("Expected clickCount 0 after triple click, got %d", chat.clickCount)
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
	if chat.selectionActive {
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
	if chat.selectionActive {
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
	chat.selectionStartCol = 0
	chat.selectionStartLine = 0
	chat.selectionEndCol = 5
	chat.selectionEndLine = 0

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
	chat.selectionStartCol = 10
	chat.selectionStartLine = 0
	chat.selectionEndCol = 5
	chat.selectionEndLine = 0

	// Should handle reversed selection gracefully
	_ = chat.GetSelectedText()

	// Test with negative bounds (will be normalized by selectionArea)
	chat.selectionStartCol = -5
	chat.selectionStartLine = 0
	chat.selectionEndCol = 10
	chat.selectionEndLine = 0

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
	chat.selectionStartCol = 0
	chat.selectionStartLine = 0
	chat.selectionEndCol = 0
	chat.selectionEndLine = 0

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
	chat.selectionStartCol = 0
	chat.selectionStartLine = 0
	chat.selectionEndCol = 10
	chat.selectionEndLine = 0

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
	chat.selectionStartCol = 0
	chat.selectionStartLine = 0
	chat.selectionEndCol = 5
	chat.selectionEndLine = 0

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
	chat.selectionStartCol = 0
	chat.selectionStartLine = 0
	chat.selectionEndCol = 5
	chat.selectionEndLine = 0

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
		if !chat.selectionActive {
			t.Error("Expected selection to be active after start")
		}

		// Drag to extend selection
		chat.EndSelection(20, 5)
		if !chat.selectionActive {
			t.Error("Expected selection to still be active during drag")
		}
		if chat.selectionEndCol != 20 || chat.selectionEndLine != 5 {
			t.Error("Selection end not updated correctly during drag")
		}

		// Stop selection (mouse release)
		chat.SelectionStop()
		if chat.selectionActive {
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
		if chat.selectionActive {
			t.Error("Expected selectionActive false after SelectWord (immediate selection)")
		}
	})

	t.Run("triple click paragraph selection", func(t *testing.T) {
		chat.SelectionClear()

		// After SelectParagraph, selectionActive should be false
		chat.SelectParagraph(5, 0)
		if chat.selectionActive {
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
			if chat.selectionStartCol != tt.startX {
				t.Errorf("Expected startCol %d, got %d", tt.startX, chat.selectionStartCol)
			}
			if chat.selectionStartLine != tt.startY {
				t.Errorf("Expected startLine %d, got %d", tt.startY, chat.selectionStartLine)
			}
			if chat.selectionEndCol != tt.endX {
				t.Errorf("Expected endCol %d, got %d", tt.endX, chat.selectionEndCol)
			}
			if chat.selectionEndLine != tt.endY {
				t.Errorf("Expected endLine %d, got %d", tt.endY, chat.selectionEndLine)
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
	chat.selectionStartCol = 5
	chat.selectionStartLine = 0
	chat.selectionEndCol = 10
	chat.selectionEndLine = 2

	// Test that it doesn't crash with multi-line coordinates
	_ = chat.GetSelectedText()
}

// TestChat_CopySelectedText_EmptySelection tests CopySelectedText with empty selection
func TestChat_CopySelectedText_EmptySelection(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Create selection that would result in empty text after trim
	chat.selectionStartCol = 0
	chat.selectionStartLine = 0
	chat.selectionEndCol = 1
	chat.selectionEndLine = 0

	// This may or may not return nil depending on viewport content
	_ = chat.CopySelectedText()
}

// TestChat_SelectionView_MultiLineSelection tests multi-line highlighting
func TestChat_SelectionView_MultiLineSelection(t *testing.T) {
	chat := NewChat()
	chat.SetSize(40, 10)
	chat.SetSession("test", nil)

	// Multi-line selection
	chat.selectionStartCol = 0
	chat.selectionStartLine = 0
	chat.selectionEndCol = 20
	chat.selectionEndLine = 3

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
	chat.selectionStartCol = 2
	chat.selectionStartLine = 1
	chat.selectionEndCol = 8
	chat.selectionEndLine = 1

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
	chat.selectionStartCol = 5
	chat.selectionStartLine = 0
	chat.selectionEndCol = 10
	chat.selectionEndLine = 2

	testView := "First line text\nSecond line\nThird line text"
	_ = chat.selectionView(testView)
}

// TestChat_SelectionView_LastLineOnly tests last line of multi-line selection
func TestChat_SelectionView_LastLineOnly(t *testing.T) {
	chat := NewChat()
	chat.SetSize(40, 10)
	chat.SetSession("test", nil)

	// Selection ending mid-last line
	chat.selectionStartCol = 0
	chat.selectionStartLine = 0
	chat.selectionEndCol = 5
	chat.selectionEndLine = 2

	testView := "First line\nMiddle line\nLast line here"
	_ = chat.selectionView(testView)
}

// TestChat_SelectionView_MiddleLinesFullWidth tests middle lines of multi-line selection
func TestChat_SelectionView_MiddleLinesFullWidth(t *testing.T) {
	chat := NewChat()
	chat.SetSize(40, 10)
	chat.SetSession("test", nil)

	// Selection spanning 4 lines (tests middle line branches)
	chat.selectionStartCol = 5
	chat.selectionStartLine = 0
	chat.selectionEndCol = 5
	chat.selectionEndLine = 3

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
	if chat.selectionStartLine != 0 {
		t.Errorf("Expected selectionStartLine 0, got %d", chat.selectionStartLine)
	}
}

// =============================================================================
// Selection Flash Animation Tests
// =============================================================================

func TestChat_SelectionFlashFrameInitialization(t *testing.T) {
	chat := NewChat()

	// New chat should have selectionFlashFrame initialized to -1
	if chat.selectionFlashFrame != -1 {
		t.Errorf("Expected selectionFlashFrame -1, got %d", chat.selectionFlashFrame)
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
	chat.selectionFlashFrame = 0
	if !chat.IsSelectionFlashing() {
		t.Error("Expected flashing when selectionFlashFrame is 0")
	}

	// When selectionFlashFrame is 1, should still be considered flashing
	chat.selectionFlashFrame = 1
	if !chat.IsSelectionFlashing() {
		t.Error("Expected flashing when selectionFlashFrame is 1")
	}

	// When selectionFlashFrame is -1, should not be flashing
	chat.selectionFlashFrame = -1
	if chat.IsSelectionFlashing() {
		t.Error("Expected not flashing when selectionFlashFrame is -1")
	}
}

func TestChat_CopySelectedText_StartsFlashAnimation(t *testing.T) {
	chat := NewChat()
	chat.SetSize(80, 24)
	chat.SetSession("test", nil)

	// Create a valid selection
	chat.selectionStartCol = 0
	chat.selectionStartLine = 0
	chat.selectionEndCol = 10
	chat.selectionEndLine = 0

	// Initially not flashing
	if chat.selectionFlashFrame != -1 {
		t.Errorf("Expected selectionFlashFrame -1 initially, got %d", chat.selectionFlashFrame)
	}

	// Copy selected text should start flash animation
	cmd := chat.CopySelectedText()

	// Flash frame should now be 0 (animation started)
	if chat.selectionFlashFrame != 0 {
		t.Errorf("Expected selectionFlashFrame 0 after copy, got %d", chat.selectionFlashFrame)
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
	chat.selectionStartCol = 0
	chat.selectionStartLine = 0
	chat.selectionEndCol = 10
	chat.selectionEndLine = 0
	chat.selectionFlashFrame = 0

	// Verify selection exists
	if !chat.HasTextSelection() {
		t.Error("Expected selection to exist before tick")
	}

	// Process the flash tick message
	_, _ = chat.Update(SelectionFlashTickMsg(time.Now()))

	// After tick, selectionFlashFrame should increment
	// Since frame was 0, it becomes 1, which triggers clear and reset to -1
	if chat.selectionFlashFrame != -1 {
		t.Errorf("Expected selectionFlashFrame -1 after tick clears, got %d", chat.selectionFlashFrame)
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
	chat.selectionStartCol = 0
	chat.selectionStartLine = 0
	chat.selectionEndCol = 10
	chat.selectionEndLine = 0
	chat.selectionFlashFrame = -1 // Not flashing

	// Process the flash tick message
	_, _ = chat.Update(SelectionFlashTickMsg(time.Now()))

	// Selection should still exist (tick should be ignored when not flashing)
	if !chat.HasTextSelection() {
		t.Error("Expected selection to still exist when not flashing")
	}

	// Flash frame should still be -1
	if chat.selectionFlashFrame != -1 {
		t.Errorf("Expected selectionFlashFrame to remain -1, got %d", chat.selectionFlashFrame)
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
	chat.selectionStartCol = 0
	chat.selectionStartLine = 0
	chat.selectionEndCol = 5
	chat.selectionEndLine = 0

	testView := "Hello World"

	// Test with normal selection (no flash)
	chat.selectionFlashFrame = -1
	result1 := chat.selectionView(testView)

	// Test with flash active
	chat.selectionFlashFrame = 0
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
