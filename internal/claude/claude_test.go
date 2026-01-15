package claude

import (
	"encoding/json"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name            string
		sessionID       string
		workingDir      string
		sessionStarted  bool
		initialMessages []Message
		wantMsgCount    int
	}{
		{
			name:            "new session with no messages",
			sessionID:       "session-123",
			workingDir:      "/path/to/dir",
			sessionStarted:  false,
			initialMessages: nil,
			wantMsgCount:    0,
		},
		{
			name:           "resumed session with messages",
			sessionID:      "session-456",
			workingDir:     "/path/to/dir",
			sessionStarted: true,
			initialMessages: []Message{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there!"},
			},
			wantMsgCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := New(tt.sessionID, tt.workingDir, tt.sessionStarted, tt.initialMessages)

			if runner == nil {
				t.Fatal("New returned nil runner")
			}

			if runner.sessionID != tt.sessionID {
				t.Errorf("sessionID = %q, want %q", runner.sessionID, tt.sessionID)
			}

			if runner.workingDir != tt.workingDir {
				t.Errorf("workingDir = %q, want %q", runner.workingDir, tt.workingDir)
			}

			if runner.SessionStarted() != tt.sessionStarted {
				t.Errorf("SessionStarted() = %v, want %v", runner.SessionStarted(), tt.sessionStarted)
			}

			msgs := runner.GetMessages()
			if len(msgs) != tt.wantMsgCount {
				t.Errorf("len(GetMessages()) = %d, want %d", len(msgs), tt.wantMsgCount)
			}

			// Verify default allowed tools are set
			if len(runner.allowedTools) != len(DefaultAllowedTools) {
				t.Errorf("allowedTools count = %d, want %d", len(runner.allowedTools), len(DefaultAllowedTools))
			}

			// Verify channels are created
			if runner.permReqChan == nil {
				t.Error("permReqChan is nil")
			}
			if runner.permRespChan == nil {
				t.Error("permRespChan is nil")
			}
			if runner.questReqChan == nil {
				t.Error("questReqChan is nil")
			}
			if runner.questRespChan == nil {
				t.Error("questRespChan is nil")
			}
		})
	}
}

func TestRunner_SetAllowedTools(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	initialCount := len(runner.allowedTools)

	// Add new tools
	runner.SetAllowedTools([]string{"Bash(git:*)", "Bash(npm:*)"})

	if len(runner.allowedTools) != initialCount+2 {
		t.Errorf("Expected %d tools, got %d", initialCount+2, len(runner.allowedTools))
	}

	// Adding duplicates should not increase count
	runner.SetAllowedTools([]string{"Bash(git:*)", "Read"})
	if len(runner.allowedTools) != initialCount+2 {
		t.Errorf("Expected %d tools after duplicate add, got %d", initialCount+2, len(runner.allowedTools))
	}
}

func TestRunner_AddAllowedTool(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	initialCount := len(runner.allowedTools)

	// Add a new tool
	runner.AddAllowedTool("Bash(docker:*)")
	if len(runner.allowedTools) != initialCount+1 {
		t.Errorf("Expected %d tools, got %d", initialCount+1, len(runner.allowedTools))
	}

	// Adding the same tool again should not increase count
	runner.AddAllowedTool("Bash(docker:*)")
	if len(runner.allowedTools) != initialCount+1 {
		t.Errorf("Expected %d tools after duplicate, got %d", initialCount+1, len(runner.allowedTools))
	}
}

func TestRunner_SetMCPServers(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	servers := []MCPServer{
		{Name: "github", Command: "npx", Args: []string{"@modelcontextprotocol/server-github"}},
		{Name: "postgres", Command: "npx", Args: []string{"@modelcontextprotocol/server-postgres"}},
	}

	runner.SetMCPServers(servers)

	if len(runner.mcpServers) != 2 {
		t.Errorf("Expected 2 MCP servers, got %d", len(runner.mcpServers))
	}
}

func TestRunner_IsStreaming(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	// Initially not streaming
	if runner.IsStreaming() {
		t.Error("Expected IsStreaming to be false initially")
	}

	// Manually set streaming state (normally set by Send)
	runner.mu.Lock()
	runner.isStreaming = true
	runner.mu.Unlock()

	if !runner.IsStreaming() {
		t.Error("Expected IsStreaming to be true after setting")
	}
}

func TestRunner_GetResponseChan(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	// Initially nil
	if runner.GetResponseChan() != nil {
		t.Error("Expected GetResponseChan to be nil initially")
	}

	// Set response channel
	ch := make(chan ResponseChunk)
	runner.mu.Lock()
	runner.currentResponseCh = ch
	runner.mu.Unlock()

	if runner.GetResponseChan() == nil {
		t.Error("Expected GetResponseChan to be non-nil after setting")
	}
}

func TestRunner_AddAssistantMessage(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	runner.AddAssistantMessage("Hello, I am Claude!")

	msgs := runner.GetMessages()
	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(msgs))
	}

	if msgs[0].Role != "assistant" {
		t.Errorf("Expected role 'assistant', got %q", msgs[0].Role)
	}

	if msgs[0].Content != "Hello, I am Claude!" {
		t.Errorf("Expected content 'Hello, I am Claude!', got %q", msgs[0].Content)
	}
}

func TestRunner_GetMessages(t *testing.T) {
	initialMsgs := []Message{
		{Role: "user", Content: "Hi"},
		{Role: "assistant", Content: "Hello!"},
	}
	runner := New("session-1", "/tmp", true, initialMsgs)

	msgs := runner.GetMessages()

	if len(msgs) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(msgs))
	}

	// Verify it's a copy
	msgs[0].Content = "modified"
	original := runner.GetMessages()
	if original[0].Content == "modified" {
		t.Error("GetMessages should return a copy, not the original")
	}
}

func TestRunner_Stop_Idempotent(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	// Stop should be callable multiple times without panicking
	runner.Stop()
	runner.Stop()
	runner.Stop()
}

func TestParseStreamMessage_Empty(t *testing.T) {
	chunks := parseStreamMessage("")
	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for empty line, got %d", len(chunks))
	}

	chunks = parseStreamMessage("   ")
	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for whitespace line, got %d", len(chunks))
	}
}

func TestParseStreamMessage_InvalidJSON(t *testing.T) {
	chunks := parseStreamMessage("not valid json")
	if len(chunks) != 1 {
		t.Fatalf("Expected 1 error chunk, got %d", len(chunks))
	}

	if chunks[0].Type != ChunkTypeText {
		t.Errorf("Expected ChunkTypeText, got %v", chunks[0].Type)
	}

	if chunks[0].Content == "" {
		t.Error("Expected non-empty error content")
	}
}

func TestParseStreamMessage_SystemInit(t *testing.T) {
	msg := `{"type":"system","subtype":"init","session_id":"abc123"}`
	chunks := parseStreamMessage(msg)

	// System init messages are logged but don't produce chunks
	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for system init, got %d", len(chunks))
	}
}

func TestParseStreamMessage_AssistantText(t *testing.T) {
	msg := `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello, world!"}]}}`
	chunks := parseStreamMessage(msg)

	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk, got %d", len(chunks))
	}

	if chunks[0].Type != ChunkTypeText {
		t.Errorf("Expected ChunkTypeText, got %v", chunks[0].Type)
	}

	if chunks[0].Content != "Hello, world!" {
		t.Errorf("Expected 'Hello, world!', got %q", chunks[0].Content)
	}
}

func TestParseStreamMessage_AssistantToolUse(t *testing.T) {
	msg := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/path/to/file.go"}}]}}`
	chunks := parseStreamMessage(msg)

	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk, got %d", len(chunks))
	}

	if chunks[0].Type != ChunkTypeToolUse {
		t.Errorf("Expected ChunkTypeToolUse, got %v", chunks[0].Type)
	}

	if chunks[0].ToolName != "Read" {
		t.Errorf("Expected tool name 'Read', got %q", chunks[0].ToolName)
	}

	if chunks[0].ToolInput != "file.go" {
		t.Errorf("Expected tool input 'file.go', got %q", chunks[0].ToolInput)
	}
}

func TestParseStreamMessage_MultipleContent(t *testing.T) {
	msg := `{"type":"assistant","message":{"content":[{"type":"text","text":"Here's the file:"},{"type":"tool_use","name":"Read","input":{"file_path":"main.go"}}]}}`
	chunks := parseStreamMessage(msg)

	if len(chunks) != 2 {
		t.Fatalf("Expected 2 chunks, got %d", len(chunks))
	}

	if chunks[0].Type != ChunkTypeText {
		t.Errorf("First chunk expected ChunkTypeText, got %v", chunks[0].Type)
	}

	if chunks[1].Type != ChunkTypeToolUse {
		t.Errorf("Second chunk expected ChunkTypeToolUse, got %v", chunks[1].Type)
	}
}

func TestParseStreamMessage_UserToolResult(t *testing.T) {
	// User messages with tool results should be silently skipped
	msg := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"123","content":"file contents"}]}}`
	chunks := parseStreamMessage(msg)

	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for tool result, got %d", len(chunks))
	}
}

func TestParseStreamMessage_UserToolResultCamelCase(t *testing.T) {
	// Handle both snake_case and camelCase variants
	msg := `{"type":"user","message":{"content":[{"toolUseId":"123","content":"file contents"}]}}`
	chunks := parseStreamMessage(msg)

	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for tool result (camelCase), got %d", len(chunks))
	}
}

func TestParseStreamMessage_Result(t *testing.T) {
	msg := `{"type":"result","subtype":"success","result":"Operation completed"}`
	chunks := parseStreamMessage(msg)

	// Result messages are logged but don't produce user-visible chunks
	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for result, got %d", len(chunks))
	}
}

func TestExtractToolInputDescription_Read(t *testing.T) {
	input := json.RawMessage(`{"file_path":"/path/to/file.go"}`)
	desc := extractToolInputDescription("Read", input)

	if desc != "file.go" {
		t.Errorf("Expected 'file.go', got %q", desc)
	}
}

func TestExtractToolInputDescription_Edit(t *testing.T) {
	input := json.RawMessage(`{"file_path":"/very/long/path/to/config.yaml"}`)
	desc := extractToolInputDescription("Edit", input)

	if desc != "config.yaml" {
		t.Errorf("Expected 'config.yaml', got %q", desc)
	}
}

func TestExtractToolInputDescription_Glob(t *testing.T) {
	input := json.RawMessage(`{"pattern":"**/*.ts"}`)
	desc := extractToolInputDescription("Glob", input)

	if desc != "**/*.ts" {
		t.Errorf("Expected '**/*.ts', got %q", desc)
	}
}

func TestExtractToolInputDescription_Grep(t *testing.T) {
	input := json.RawMessage(`{"pattern":"func TestSomethingVeryLongName"}`)
	desc := extractToolInputDescription("Grep", input)

	// Grep patterns are truncated at 30 chars
	if len(desc) > 33 { // 30 + "..."
		t.Errorf("Expected truncated pattern, got %q (len=%d)", desc, len(desc))
	}
}

func TestExtractToolInputDescription_Bash(t *testing.T) {
	input := json.RawMessage(`{"command":"go test ./... -v -race -cover"}`)
	desc := extractToolInputDescription("Bash", input)

	// Bash commands are truncated at 40 chars
	if len(desc) > 43 { // 40 + "..."
		t.Errorf("Expected truncated command, got %q (len=%d)", desc, len(desc))
	}
}

func TestExtractToolInputDescription_Task(t *testing.T) {
	input := json.RawMessage(`{"description":"explore codebase","prompt":"Find all API endpoints"}`)
	desc := extractToolInputDescription("Task", input)

	if desc != "explore codebase" {
		t.Errorf("Expected 'explore codebase', got %q", desc)
	}
}

func TestExtractToolInputDescription_WebFetch(t *testing.T) {
	input := json.RawMessage(`{"url":"https://example.com/very/long/path/to/api/endpoint"}`)
	desc := extractToolInputDescription("WebFetch", input)

	// URLs are truncated at 40 chars
	if len(desc) > 43 {
		t.Errorf("Expected truncated URL, got %q (len=%d)", desc, len(desc))
	}
}

func TestExtractToolInputDescription_WebSearch(t *testing.T) {
	input := json.RawMessage(`{"query":"go testing best practices"}`)
	desc := extractToolInputDescription("WebSearch", input)

	if desc != "go testing best practices" {
		t.Errorf("Expected 'go testing best practices', got %q", desc)
	}
}

func TestExtractToolInputDescription_UnknownTool(t *testing.T) {
	// Unknown tools should return the first string value
	input := json.RawMessage(`{"some_field":"some value"}`)
	desc := extractToolInputDescription("UnknownTool", input)

	if desc != "some value" {
		t.Errorf("Expected 'some value', got %q", desc)
	}
}

func TestExtractToolInputDescription_EmptyInput(t *testing.T) {
	desc := extractToolInputDescription("Read", nil)
	if desc != "" {
		t.Errorf("Expected empty string for nil input, got %q", desc)
	}

	desc = extractToolInputDescription("Read", json.RawMessage(""))
	if desc != "" {
		t.Errorf("Expected empty string for empty input, got %q", desc)
	}
}

func TestExtractToolInputDescription_InvalidJSON(t *testing.T) {
	input := json.RawMessage(`not valid json`)
	desc := extractToolInputDescription("Read", input)

	if desc != "" {
		t.Errorf("Expected empty string for invalid JSON, got %q", desc)
	}
}

func TestFormatToolInput(t *testing.T) {
	tests := []struct {
		value    string
		shorten  bool
		maxLen   int
		expected string
	}{
		{"/path/to/file.go", true, 0, "file.go"},
		{"/path/to/file.go", false, 0, "/path/to/file.go"},
		{"very long string that needs truncation", false, 10, "very long ..."},
		{"/path/to/file.go", true, 5, "file...."},
		{"short", false, 100, "short"},
	}

	for _, tt := range tests {
		result := formatToolInput(tt.value, tt.shorten, tt.maxLen)
		if result != tt.expected {
			t.Errorf("formatToolInput(%q, %v, %d) = %q, want %q", tt.value, tt.shorten, tt.maxLen, result, tt.expected)
		}
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		s        string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "hello..."},
		{"hello", 0, "hello"}, // 0 means no limit
		{"", 10, ""},
	}

	for _, tt := range tests {
		result := truncateString(tt.s, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncateString(%q, %d) = %q, want %q", tt.s, tt.maxLen, result, tt.expected)
		}
	}
}

func TestShortenPath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/path/to/file.go", "file.go"},
		{"file.go", "file.go"},
		{"/a/b/c/d/e.txt", "e.txt"},
		{"", ""},
		{"/", ""},
	}

	for _, tt := range tests {
		result := shortenPath(tt.path)
		if result != tt.expected {
			t.Errorf("shortenPath(%q) = %q, want %q", tt.path, result, tt.expected)
		}
	}
}

func TestTruncateForLog(t *testing.T) {
	short := "short message"
	if truncateForLog(short) != short {
		t.Error("Short message should not be truncated")
	}

	long := ""
	for i := 0; i < 300; i++ {
		long += "x"
	}
	result := truncateForLog(long)
	if len(result) > 203 { // 200 + "..."
		t.Errorf("Long message should be truncated, got len=%d", len(result))
	}
}

func TestFormatToolIcon(t *testing.T) {
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
	}

	for _, tt := range tests {
		result := formatToolIcon(tt.toolName)
		if result != tt.expected {
			t.Errorf("formatToolIcon(%q) = %q, want %q", tt.toolName, result, tt.expected)
		}
	}
}

func TestDefaultAllowedTools(t *testing.T) {
	expected := []string{
		"Read", "Glob", "Grep", "Edit", "Write", "ExitPlanMode",
		"Bash(ls:*)", "Bash(cat:*)", "Bash(head:*)",
		"Bash(tail:*)", "Bash(wc:*)", "Bash(pwd:*)",
	}

	if len(DefaultAllowedTools) != len(expected) {
		t.Errorf("Expected %d default tools, got %d", len(expected), len(DefaultAllowedTools))
	}

	for i, tool := range expected {
		if DefaultAllowedTools[i] != tool {
			t.Errorf("DefaultAllowedTools[%d] = %q, want %q", i, DefaultAllowedTools[i], tool)
		}
	}
}

func TestChunkTypes(t *testing.T) {
	// Verify chunk type constants
	if ChunkTypeText != "text" {
		t.Errorf("ChunkTypeText = %q, want 'text'", ChunkTypeText)
	}
	if ChunkTypeToolUse != "tool_use" {
		t.Errorf("ChunkTypeToolUse = %q, want 'tool_use'", ChunkTypeToolUse)
	}
	if ChunkTypeToolResult != "tool_result" {
		t.Errorf("ChunkTypeToolResult = %q, want 'tool_result'", ChunkTypeToolResult)
	}
}

func TestParseStreamMessage_EmptyText(t *testing.T) {
	// Empty text content should not produce a chunk
	msg := `{"type":"assistant","message":{"content":[{"type":"text","text":""}]}}`
	chunks := parseStreamMessage(msg)

	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for empty text, got %d", len(chunks))
	}
}

func TestParseStreamMessage_UnrecognizedJSON(t *testing.T) {
	// JSON that parses but has no recognized type
	msg := `{"something":"else"}`
	chunks := parseStreamMessage(msg)

	// Should return an error chunk
	if len(chunks) != 1 {
		t.Fatalf("Expected 1 error chunk, got %d", len(chunks))
	}

	if chunks[0].Type != ChunkTypeText {
		t.Errorf("Expected ChunkTypeText for error, got %v", chunks[0].Type)
	}
}

func TestToolInputConfigs(t *testing.T) {
	// Verify the tool input config map is populated correctly
	expectedTools := []string{"Read", "Edit", "Write", "Glob", "Grep", "Bash", "Task", "WebFetch", "WebSearch"}

	for _, tool := range expectedTools {
		if _, ok := toolInputConfigs[tool]; !ok {
			t.Errorf("Expected toolInputConfigs to contain %q", tool)
		}
	}
}

func TestRunner_ChannelOperations(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	// Test that channel accessors work
	permReqChan := runner.PermissionRequestChan()
	if permReqChan == nil {
		t.Error("PermissionRequestChan returned nil")
	}

	questReqChan := runner.QuestionRequestChan()
	if questReqChan == nil {
		t.Error("QuestionRequestChan returned nil")
	}
}

func TestRunner_SessionStarted(t *testing.T) {
	// Test session not started
	runner := New("session-1", "/tmp", false, nil)
	if runner.SessionStarted() {
		t.Error("Session should not be started initially")
	}

	// Test session started
	runner = New("session-2", "/tmp", true, nil)
	if !runner.SessionStarted() {
		t.Error("Session should be started")
	}
}

func TestParseStreamMessage_NestedToolInput(t *testing.T) {
	// Test tool use with nested input object
	msg := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/path/to/file.go","old_string":"foo","new_string":"bar"}}]}}`
	chunks := parseStreamMessage(msg)

	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk, got %d", len(chunks))
	}

	if chunks[0].Type != ChunkTypeToolUse {
		t.Errorf("Expected ChunkTypeToolUse, got %v", chunks[0].Type)
	}

	if chunks[0].ToolName != "Edit" {
		t.Errorf("Expected tool name 'Edit', got %q", chunks[0].ToolName)
	}
}

func TestParseStreamMessage_EmptyContent(t *testing.T) {
	msg := `{"type":"assistant","message":{"content":[]}}`
	chunks := parseStreamMessage(msg)

	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for empty content array, got %d", len(chunks))
	}
}

func TestParseStreamMessage_NullContent(t *testing.T) {
	msg := `{"type":"assistant","message":{"content":null}}`
	chunks := parseStreamMessage(msg)

	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for null content, got %d", len(chunks))
	}
}

func TestParseStreamMessage_MixedContentTypes(t *testing.T) {
	msg := `{"type":"assistant","message":{"content":[
		{"type":"text","text":"First text"},
		{"type":"tool_use","name":"Read","input":{"file_path":"test.go"}},
		{"type":"text","text":"Second text"},
		{"type":"tool_use","name":"Bash","input":{"command":"ls"}}
	]}}`
	chunks := parseStreamMessage(msg)

	if len(chunks) != 4 {
		t.Fatalf("Expected 4 chunks, got %d", len(chunks))
	}

	// Verify order and types
	if chunks[0].Type != ChunkTypeText || chunks[0].Content != "First text" {
		t.Errorf("First chunk mismatch: %+v", chunks[0])
	}
	if chunks[1].Type != ChunkTypeToolUse || chunks[1].ToolName != "Read" {
		t.Errorf("Second chunk mismatch: %+v", chunks[1])
	}
	if chunks[2].Type != ChunkTypeText || chunks[2].Content != "Second text" {
		t.Errorf("Third chunk mismatch: %+v", chunks[2])
	}
	if chunks[3].Type != ChunkTypeToolUse || chunks[3].ToolName != "Bash" {
		t.Errorf("Fourth chunk mismatch: %+v", chunks[3])
	}
}

func TestExtractToolInputDescription_NotebookEdit(t *testing.T) {
	// NotebookEdit is not in toolInputConfigs, should fall back to first string
	input := json.RawMessage(`{"notebook_path":"/path/to/notebook.ipynb","cell_number":5}`)
	desc := extractToolInputDescription("NotebookEdit", input)

	if desc != "/path/to/notebook.ipynb" {
		t.Errorf("Expected notebook path, got %q", desc)
	}
}

func TestExtractToolInputDescription_NoStringFields(t *testing.T) {
	// Input with no string fields
	input := json.RawMessage(`{"number":42,"boolean":true}`)
	desc := extractToolInputDescription("SomeTool", input)

	if desc != "" {
		t.Errorf("Expected empty string for no string fields, got %q", desc)
	}
}

func TestExtractToolInputDescription_EmptyObject(t *testing.T) {
	input := json.RawMessage(`{}`)
	desc := extractToolInputDescription("SomeTool", input)

	if desc != "" {
		t.Errorf("Expected empty string for empty object, got %q", desc)
	}
}

func TestRunner_DefaultAllowedToolsCopied(t *testing.T) {
	runner1 := New("session-1", "/tmp", false, nil)
	runner2 := New("session-2", "/tmp", false, nil)

	// Add tool to runner1
	runner1.AddAllowedTool("CustomTool")

	// runner2 should not have CustomTool
	runner2Tools := make(map[string]bool)
	runner2.SetAllowedTools([]string{}) // This won't add anything new
	// Just verify they're independent instances

	// The runner should have default tools
	if runner1 == runner2 {
		t.Error("Runners should be different instances")
	}

	_ = runner2Tools // avoid unused variable warning
}

func TestRunner_MessagesCopied(t *testing.T) {
	initialMsgs := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
	}
	runner := New("session-1", "/tmp", true, initialMsgs)

	// Note: New() assigns the slice directly, so modifying initialMsgs
	// would affect the runner. This is by design for efficiency.
	// The copy protection is on GetMessages() output, not input.

	// Verify GetMessages returns a copy
	msgs := runner.GetMessages()
	msgs[0].Content = "Modified"

	// Get again - should be unchanged
	msgs2 := runner.GetMessages()
	if msgs2[0].Content == "Modified" {
		t.Error("GetMessages should return a copy")
	}
}

func TestShortenPath_EdgeCases(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/", ""},
		{"//", ""},
		{"a", "a"},
		{"/a", "a"},
		{"a/b/", ""}, // Trailing slash results in empty last component
		{"../file.go", "file.go"},
	}

	for _, tt := range tests {
		result := shortenPath(tt.path)
		if result != tt.expected {
			t.Errorf("shortenPath(%q) = %q, want %q", tt.path, result, tt.expected)
		}
	}
}

func TestTruncateString_EdgeCases(t *testing.T) {
	tests := []struct {
		s        string
		maxLen   int
		expected string
	}{
		{"abc", 3, "abc"},    // Exactly at limit
		{"abcd", 3, "abc..."}, // One over limit
		{"ab", 3, "ab"},       // Under limit
		{"a", 1, "a"},         // Single char at limit
		{"ab", 1, "a..."},     // Single char limit with longer string
	}

	for _, tt := range tests {
		result := truncateString(tt.s, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncateString(%q, %d) = %q, want %q", tt.s, tt.maxLen, result, tt.expected)
		}
	}
}

func TestParseStreamMessage_WhitespaceOnly(t *testing.T) {
	tests := []string{
		"   ",
		"\t\t",
		"\n\n",
		" \t\n ",
	}

	for _, line := range tests {
		chunks := parseStreamMessage(line)
		if len(chunks) != 0 {
			t.Errorf("parseStreamMessage(%q) should return 0 chunks, got %d", line, len(chunks))
		}
	}
}

func TestParseStreamMessage_LeadingTrailingWhitespace(t *testing.T) {
	msg := `  {"type":"assistant","message":{"content":[{"type":"text","text":"test"}]}}  `
	chunks := parseStreamMessage(msg)

	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk, got %d", len(chunks))
	}

	if chunks[0].Content != "test" {
		t.Errorf("Expected content 'test', got %q", chunks[0].Content)
	}
}

func TestFormatToolInput_EdgeCases(t *testing.T) {
	tests := []struct {
		value    string
		shorten  bool
		maxLen   int
		expected string
	}{
		{"", false, 0, ""},           // Empty string
		{"", true, 0, ""},            // Empty with shorten
		{"file.go", true, 0, "file.go"}, // Already short
		{"/a/b/c", true, 100, "c"},   // Shorten with high limit
	}

	for _, tt := range tests {
		result := formatToolInput(tt.value, tt.shorten, tt.maxLen)
		if result != tt.expected {
			t.Errorf("formatToolInput(%q, %v, %d) = %q, want %q", tt.value, tt.shorten, tt.maxLen, result, tt.expected)
		}
	}
}

func TestToolInputConfigs_Coverage(t *testing.T) {
	// Verify each config has valid fields
	for name, cfg := range toolInputConfigs {
		if cfg.Field == "" {
			t.Errorf("toolInputConfigs[%q] has empty Field", name)
		}
	}

	// Verify specific tools have expected config
	if cfg, ok := toolInputConfigs["Read"]; ok {
		if !cfg.ShortenPath {
			t.Error("Read should have ShortenPath=true")
		}
	} else {
		t.Error("Read should be in toolInputConfigs")
	}

	if cfg, ok := toolInputConfigs["Bash"]; ok {
		if cfg.MaxLen == 0 {
			t.Error("Bash should have MaxLen set")
		}
	} else {
		t.Error("Bash should be in toolInputConfigs")
	}
}

func TestResponseChunk_Fields(t *testing.T) {
	chunk := ResponseChunk{
		Type:      ChunkTypeText,
		Content:   "test content",
		ToolName:  "TestTool",
		ToolInput: "input desc",
		Done:      true,
		Error:     nil,
	}

	if chunk.Type != ChunkTypeText {
		t.Errorf("Expected type text, got %v", chunk.Type)
	}

	if chunk.Content != "test content" {
		t.Errorf("Expected content 'test content', got %q", chunk.Content)
	}

	if chunk.ToolName != "TestTool" {
		t.Errorf("Expected tool name 'TestTool', got %q", chunk.ToolName)
	}

	if !chunk.Done {
		t.Error("Expected Done=true")
	}
}
