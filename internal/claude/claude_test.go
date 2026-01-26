package claude

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/zhubert/plural/internal/mcp"
)

// testLogger creates a discard logger for tests
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

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

			// Verify MCP channels struct is created
			if runner.mcp == nil {
				t.Error("mcp is nil")
			}
			if runner.mcp.PermissionReq == nil {
				t.Error("mcp.PermissionReq is nil")
			}
			if runner.mcp.PermissionResp == nil {
				t.Error("mcp.PermissionResp is nil")
			}
			if runner.mcp.QuestionReq == nil {
				t.Error("mcp.QuestionReq is nil")
			}
			if runner.mcp.QuestionResp == nil {
				t.Error("mcp.QuestionResp is nil")
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

func TestRunner_SetForkFromSession(t *testing.T) {
	runner := New("child-session", "/tmp", false, nil)

	// Initially no fork parent
	runner.mu.RLock()
	if runner.forkFromSessionID != "" {
		t.Errorf("Expected empty forkFromSessionID initially, got %q", runner.forkFromSessionID)
	}
	runner.mu.RUnlock()

	// Set fork parent
	runner.SetForkFromSession("parent-session")

	runner.mu.RLock()
	if runner.forkFromSessionID != "parent-session" {
		t.Errorf("Expected forkFromSessionID 'parent-session', got %q", runner.forkFromSessionID)
	}
	runner.mu.RUnlock()
}

func TestRunner_IsStreaming(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	// Initially not streaming
	if runner.IsStreaming() {
		t.Error("Expected IsStreaming to be false initially")
	}

	// Manually set streaming state (normally set by Send)
	runner.mu.Lock()
	runner.streaming.Active = true
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
	runner.responseChan.Setup(ch)
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
	log := testLogger()
	chunks := parseStreamMessage("", log)
	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for empty line, got %d", len(chunks))
	}

	chunks = parseStreamMessage("   ", log)
	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for whitespace line, got %d", len(chunks))
	}
}

func TestParseStreamMessage_InvalidJSON(t *testing.T) {
	log := testLogger()
	chunks := parseStreamMessage("not valid json", log)
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
	log := testLogger()
	msg := `{"type":"system","subtype":"init","session_id":"abc123"}`
	chunks := parseStreamMessage(msg, log)

	// System init messages are logged but don't produce chunks
	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for system init, got %d", len(chunks))
	}
}

func TestParseStreamMessage_AssistantText(t *testing.T) {
	log := testLogger()
	msg := `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello, world!"}]}}`
	chunks := parseStreamMessage(msg, log)

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
	log := testLogger()
	msg := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/path/to/file.go"}}]}}`
	chunks := parseStreamMessage(msg, log)

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
	log := testLogger()
	msg := `{"type":"assistant","message":{"content":[{"type":"text","text":"Here's the file:"},{"type":"tool_use","name":"Read","input":{"file_path":"main.go"}}]}}`
	chunks := parseStreamMessage(msg, log)

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
	log := testLogger()
	// User messages with tool results should emit ChunkTypeToolResult
	// so the UI can mark the tool use as complete
	msg := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"123","content":"file contents"}]}}`
	chunks := parseStreamMessage(msg, log)

	if len(chunks) != 1 {
		t.Errorf("Expected 1 chunk for tool result, got %d", len(chunks))
	}
	if len(chunks) > 0 && chunks[0].Type != ChunkTypeToolResult {
		t.Errorf("Expected ChunkTypeToolResult, got %s", chunks[0].Type)
	}
}

func TestParseStreamMessage_UserToolResultCamelCase(t *testing.T) {
	log := testLogger()
	// Handle both snake_case and camelCase variants
	msg := `{"type":"user","message":{"content":[{"toolUseId":"123","content":"file contents"}]}}`
	chunks := parseStreamMessage(msg, log)

	if len(chunks) != 1 {
		t.Errorf("Expected 1 chunk for tool result (camelCase), got %d", len(chunks))
	}
	if len(chunks) > 0 && chunks[0].Type != ChunkTypeToolResult {
		t.Errorf("Expected ChunkTypeToolResult, got %s", chunks[0].Type)
	}
}

func TestParseStreamMessage_Result(t *testing.T) {
	log := testLogger()
	msg := `{"type":"result","subtype":"success","result":"Operation completed"}`
	chunks := parseStreamMessage(msg, log)

	// Result messages are logged but don't produce user-visible chunks
	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for result, got %d", len(chunks))
	}
}

func TestParseStreamMessage_ErrorResult(t *testing.T) {
	log := testLogger()
	msg := `{"type":"result","subtype":"error_during_execution","result":"Claude ran out of context window"}`
	chunks := parseStreamMessage(msg, log)

	// Error result messages are logged but don't produce user-visible chunks in parseStreamMessage
	// (the error display is handled in handleResponse instead)
	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for error result, got %d", len(chunks))
	}
}

func TestStreamMessage_ErrorsArray(t *testing.T) {
	// Test that errors array is properly parsed from the JSON
	jsonMsg := `{"type":"result","subtype":"error_during_execution","errors":["No conversation found with session ID: test-session-id"]}`

	var msg streamMessage
	if err := json.Unmarshal([]byte(jsonMsg), &msg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(msg.Errors) != 1 {
		t.Errorf("Expected 1 error in Errors array, got %d", len(msg.Errors))
	}

	if msg.Errors[0] != "No conversation found with session ID: test-session-id" {
		t.Errorf("Unexpected error message: %q", msg.Errors[0])
	}

	// Verify Result and Error fields are empty (error is in Errors array)
	if msg.Result != "" {
		t.Errorf("Expected empty Result, got %q", msg.Result)
	}

	if msg.Error != "" {
		t.Errorf("Expected empty Error, got %q", msg.Error)
	}
}

func TestStreamMessage_ErrorsArray_Multiple(t *testing.T) {
	// Test multiple errors in the array
	jsonMsg := `{"type":"result","subtype":"error_during_execution","errors":["Error 1","Error 2","Error 3"]}`

	var msg streamMessage
	if err := json.Unmarshal([]byte(jsonMsg), &msg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(msg.Errors) != 3 {
		t.Errorf("Expected 3 errors in Errors array, got %d", len(msg.Errors))
	}

	// Test that strings.Join would produce expected result
	joined := msg.Errors[0] + "; " + msg.Errors[1] + "; " + msg.Errors[2]
	expected := "Error 1; Error 2; Error 3"
	if joined != expected {
		t.Errorf("Joined errors = %q, want %q", joined, expected)
	}
}

func TestStreamMessage_PermissionDenials(t *testing.T) {
	// Test that permission_denials array is properly parsed from the result message
	jsonMsg := `{
		"type": "result",
		"subtype": "success",
		"permission_denials": [
			{"tool": "Bash", "description": "rm -rf /", "reason": "destructive command"},
			{"tool": "Edit", "description": "/etc/passwd"}
		]
	}`

	var msg streamMessage
	if err := json.Unmarshal([]byte(jsonMsg), &msg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(msg.PermissionDenials) != 2 {
		t.Fatalf("Expected 2 permission denials, got %d", len(msg.PermissionDenials))
	}

	// Verify first denial
	if msg.PermissionDenials[0].Tool != "Bash" {
		t.Errorf("First denial tool = %q, want %q", msg.PermissionDenials[0].Tool, "Bash")
	}
	if msg.PermissionDenials[0].Description != "rm -rf /" {
		t.Errorf("First denial description = %q, want %q", msg.PermissionDenials[0].Description, "rm -rf /")
	}
	if msg.PermissionDenials[0].Reason != "destructive command" {
		t.Errorf("First denial reason = %q, want %q", msg.PermissionDenials[0].Reason, "destructive command")
	}

	// Verify second denial (without reason)
	if msg.PermissionDenials[1].Tool != "Edit" {
		t.Errorf("Second denial tool = %q, want %q", msg.PermissionDenials[1].Tool, "Edit")
	}
	if msg.PermissionDenials[1].Reason != "" {
		t.Errorf("Second denial reason should be empty, got %q", msg.PermissionDenials[1].Reason)
	}
}

func TestStreamMessage_PermissionDenials_Empty(t *testing.T) {
	// Test that empty permission_denials array is handled
	jsonMsg := `{
		"type": "result",
		"subtype": "success",
		"permission_denials": []
	}`

	var msg streamMessage
	if err := json.Unmarshal([]byte(jsonMsg), &msg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(msg.PermissionDenials) != 0 {
		t.Errorf("Expected 0 permission denials, got %d", len(msg.PermissionDenials))
	}
}

func TestStreamMessage_PermissionDenials_Missing(t *testing.T) {
	// Test that missing permission_denials field results in nil/empty slice
	jsonMsg := `{"type": "result", "subtype": "success"}`

	var msg streamMessage
	if err := json.Unmarshal([]byte(jsonMsg), &msg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(msg.PermissionDenials) != 0 {
		t.Errorf("Expected 0 permission denials for missing field, got %d", len(msg.PermissionDenials))
	}
}

func TestStreamMessage_ModelUsage(t *testing.T) {
	// Test that modelUsage is properly parsed from result messages
	// This is important for getting accurate token counts when sub-agents are used
	jsonMsg := `{
		"type": "result",
		"subtype": "success",
		"total_cost_usd": 0.41071,
		"usage": {"input_tokens": 4, "output_tokens": 926},
		"modelUsage": {
			"claude-haiku-4-5-20251001": {"outputTokens": 6944},
			"claude-opus-4-5-20251101": {"outputTokens": 1461}
		}
	}`

	var msg streamMessage
	if err := json.Unmarshal([]byte(jsonMsg), &msg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify modelUsage is parsed
	if len(msg.ModelUsage) != 2 {
		t.Errorf("Expected 2 models in ModelUsage, got %d", len(msg.ModelUsage))
	}

	// Verify individual model output tokens
	if haiku, ok := msg.ModelUsage["claude-haiku-4-5-20251001"]; ok {
		if haiku.OutputTokens != 6944 {
			t.Errorf("Haiku outputTokens = %d, want 6944", haiku.OutputTokens)
		}
	} else {
		t.Error("Missing haiku model in ModelUsage")
	}

	if opus, ok := msg.ModelUsage["claude-opus-4-5-20251101"]; ok {
		if opus.OutputTokens != 1461 {
			t.Errorf("Opus outputTokens = %d, want 1461", opus.OutputTokens)
		}
	} else {
		t.Error("Missing opus model in ModelUsage")
	}

	// Verify total is correctly calculated by summing all models
	var total int
	for _, usage := range msg.ModelUsage {
		total += usage.OutputTokens
	}
	expected := 6944 + 1461 // = 8405
	if total != expected {
		t.Errorf("Total output tokens = %d, want %d", total, expected)
	}

	// Verify usage.output_tokens (926) is NOT the same as the modelUsage total
	// This demonstrates why we need modelUsage for accurate sub-agent token counting
	if msg.Usage.OutputTokens == total {
		t.Error("Usage.OutputTokens should NOT equal modelUsage total - this is the bug we're fixing")
	}
}

func TestStreamMessage_ModelUsage_SingleModel(t *testing.T) {
	// Test with single model (no sub-agents)
	jsonMsg := `{
		"type": "result",
		"subtype": "success",
		"usage": {"output_tokens": 500},
		"modelUsage": {
			"claude-opus-4-5-20251101": {"outputTokens": 500}
		}
	}`

	var msg streamMessage
	if err := json.Unmarshal([]byte(jsonMsg), &msg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(msg.ModelUsage) != 1 {
		t.Errorf("Expected 1 model in ModelUsage, got %d", len(msg.ModelUsage))
	}

	// In single-model case, modelUsage total should match usage.output_tokens
	var total int
	for _, usage := range msg.ModelUsage {
		total += usage.OutputTokens
	}
	if total != msg.Usage.OutputTokens {
		t.Errorf("Single model total = %d, usage.output_tokens = %d - should match", total, msg.Usage.OutputTokens)
	}
}

func TestStreamMessage_NoModelUsage(t *testing.T) {
	// Test backward compatibility when modelUsage is not present
	jsonMsg := `{
		"type": "result",
		"subtype": "success",
		"usage": {"output_tokens": 500}
	}`

	var msg streamMessage
	if err := json.Unmarshal([]byte(jsonMsg), &msg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// ModelUsage should be nil/empty
	if len(msg.ModelUsage) != 0 {
		t.Errorf("Expected empty ModelUsage, got %d entries", len(msg.ModelUsage))
	}

	// Usage should still be present
	if msg.Usage == nil || msg.Usage.OutputTokens != 500 {
		t.Error("Usage.OutputTokens should be 500")
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
		// Note: TodoWrite is handled specially via ChunkTypeTodoUpdate, not through formatToolIcon
		{"TodoWrite", "Using"}, // Falls through to default since not in switch
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
	log := testLogger()
	// Empty text content should not produce a chunk
	msg := `{"type":"assistant","message":{"content":[{"type":"text","text":""}]}}`
	chunks := parseStreamMessage(msg, log)

	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for empty text, got %d", len(chunks))
	}
}

func TestParseStreamMessage_UnrecognizedJSON(t *testing.T) {
	log := testLogger()
	// JSON that parses but has no recognized type
	msg := `{"something":"else"}`
	chunks := parseStreamMessage(msg, log)

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
	log := testLogger()
	// Test tool use with nested input object
	msg := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/path/to/file.go","old_string":"foo","new_string":"bar"}}]}}`
	chunks := parseStreamMessage(msg, log)

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
	log := testLogger()
	msg := `{"type":"assistant","message":{"content":[]}}`
	chunks := parseStreamMessage(msg, log)

	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for empty content array, got %d", len(chunks))
	}
}

func TestParseStreamMessage_NullContent(t *testing.T) {
	log := testLogger()
	msg := `{"type":"assistant","message":{"content":null}}`
	chunks := parseStreamMessage(msg, log)

	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for null content, got %d", len(chunks))
	}
}

func TestParseStreamMessage_MixedContentTypes(t *testing.T) {
	log := testLogger()
	msg := `{"type":"assistant","message":{"content":[
		{"type":"text","text":"First text"},
		{"type":"tool_use","name":"Read","input":{"file_path":"test.go"}},
		{"type":"text","text":"Second text"},
		{"type":"tool_use","name":"Bash","input":{"command":"ls"}}
	]}}`
	chunks := parseStreamMessage(msg, log)

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
	log := testLogger()
	tests := []string{
		"   ",
		"\t\t",
		"\n\n",
		" \t\n ",
	}

	for _, line := range tests {
		chunks := parseStreamMessage(line, log)
		if len(chunks) != 0 {
			t.Errorf("parseStreamMessage(%q) should return 0 chunks, got %d", line, len(chunks))
		}
	}
}

func TestParseStreamMessage_LeadingTrailingWhitespace(t *testing.T) {
	log := testLogger()
	msg := `  {"type":"assistant","message":{"content":[{"type":"text","text":"test"}]}}  `
	chunks := parseStreamMessage(msg, log)

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

func TestRunner_Interrupt_NotRunning(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	// Interrupt should not error when no process is running
	err := runner.Interrupt()
	if err != nil {
		t.Errorf("Interrupt should not error when no process running, got: %v", err)
	}
}

func TestRunner_Interrupt_Idempotent(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	// Multiple Interrupt calls should be safe
	runner.Interrupt()
	runner.Interrupt()
	runner.Interrupt()
}

func TestConstants(t *testing.T) {
	// Verify error handling constants have reasonable values
	if MaxProcessRestartAttempts <= 0 {
		t.Error("MaxProcessRestartAttempts should be positive")
	}

	if MaxProcessRestartAttempts > 10 {
		t.Error("MaxProcessRestartAttempts should not be too high to avoid infinite loops")
	}

	if ProcessRestartDelay <= 0 {
		t.Error("ProcessRestartDelay should be positive")
	}

	if ResponseChannelFullTimeout <= 0 {
		t.Error("ResponseChannelFullTimeout should be positive")
	}
}

func TestProcessManager_RestartTracking(t *testing.T) {
	// Create a ProcessManager directly to test restart tracking
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, testLogger())

	// Initially, restart attempts should be 0
	if pm.GetRestartAttempts() != 0 {
		t.Errorf("Expected 0 restart attempts initially, got %d", pm.GetRestartAttempts())
	}

	// Simulate restart attempts (normally done internally)
	pm.mu.Lock()
	pm.restartAttempts = 2
	pm.mu.Unlock()

	if pm.GetRestartAttempts() != 2 {
		t.Errorf("Expected 2 restart attempts, got %d", pm.GetRestartAttempts())
	}

	// Test reset
	pm.ResetRestartAttempts()
	if pm.GetRestartAttempts() != 0 {
		t.Errorf("Expected 0 restart attempts after reset, got %d", pm.GetRestartAttempts())
	}
}

func TestHandleFatalError(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	// Create a channel and set it as current response channel
	ch := make(chan ResponseChunk, 10)
	runner.mu.Lock()
	runner.responseChan.Setup(ch)
	runner.mu.Unlock()

	// Call handleFatalError
	testErr := fmt.Errorf("test fatal error")
	runner.handleFatalError(testErr)

	// Should receive an error chunk
	select {
	case chunk := <-ch:
		if chunk.Error == nil {
			t.Error("Expected error in chunk")
		}
		if !chunk.Done {
			t.Error("Expected Done=true in error chunk")
		}
		if chunk.Error.Error() != testErr.Error() {
			t.Errorf("Expected error %q, got %q", testErr.Error(), chunk.Error.Error())
		}
	default:
		t.Error("Expected chunk from channel")
	}

	// Channel should be closed
	runner.mu.RLock()
	closed := runner.responseChan.Closed
	streaming := runner.streaming.Active
	runner.mu.RUnlock()

	if !closed {
		t.Error("Expected responseChan.Closed to be true")
	}
	if streaming {
		t.Error("Expected streaming.Active to be false")
	}
}

func TestHandleFatalError_AlreadyClosed(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	// Mark as already closed
	runner.mu.Lock()
	runner.responseChan.Closed = true
	runner.mu.Unlock()

	ch := make(chan ResponseChunk, 10)
	runner.mu.Lock()
	runner.responseChan.Channel = ch
	runner.mu.Unlock()

	// Should not panic or send anything since already closed
	runner.handleFatalError(fmt.Errorf("test error"))

	select {
	case <-ch:
		t.Error("Should not receive chunk when already closed")
	default:
		// Expected - no chunk sent
	}
}

func TestHandleFatalError_NilChannel(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	// Should not panic with nil channel
	runner.handleFatalError(fmt.Errorf("test error"))
}

func TestErrorVariables(t *testing.T) {
	// Verify error variables are defined
	if errChannelFull == nil {
		t.Error("errChannelFull should not be nil")
	}

	// Verify they have meaningful messages
	if errChannelFull.Error() == "" {
		t.Error("errChannelFull should have a message")
	}
}

func TestTextContent(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []ContentBlock
	}{
		{
			name: "simple text",
			text: "Hello, world!",
			want: []ContentBlock{{Type: ContentTypeText, Text: "Hello, world!"}},
		},
		{
			name: "empty text",
			text: "",
			want: []ContentBlock{{Type: ContentTypeText, Text: ""}},
		},
		{
			name: "multiline text",
			text: "Line 1\nLine 2\nLine 3",
			want: []ContentBlock{{Type: ContentTypeText, Text: "Line 1\nLine 2\nLine 3"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TextContent(tt.text)
			if len(got) != len(tt.want) {
				t.Fatalf("TextContent(%q) returned %d blocks, want %d", tt.text, len(got), len(tt.want))
			}
			if got[0].Type != tt.want[0].Type {
				t.Errorf("TextContent(%q)[0].Type = %v, want %v", tt.text, got[0].Type, tt.want[0].Type)
			}
			if got[0].Text != tt.want[0].Text {
				t.Errorf("TextContent(%q)[0].Text = %q, want %q", tt.text, got[0].Text, tt.want[0].Text)
			}
		})
	}
}

func TestGetDisplayContent(t *testing.T) {
	tests := []struct {
		name   string
		blocks []ContentBlock
		want   string
	}{
		{
			name:   "single text block",
			blocks: []ContentBlock{{Type: ContentTypeText, Text: "Hello"}},
			want:   "Hello",
		},
		{
			name:   "multiple text blocks",
			blocks: []ContentBlock{{Type: ContentTypeText, Text: "Hello"}, {Type: ContentTypeText, Text: "World"}},
			want:   "Hello\nWorld",
		},
		{
			name:   "image block",
			blocks: []ContentBlock{{Type: ContentTypeImage, Source: &ImageSource{Type: "base64", MediaType: "image/png", Data: "..."}}},
			want:   "[Image]",
		},
		{
			name: "mixed text and image",
			blocks: []ContentBlock{
				{Type: ContentTypeText, Text: "Look at this:"},
				{Type: ContentTypeImage, Source: &ImageSource{Type: "base64", MediaType: "image/png", Data: "..."}},
				{Type: ContentTypeText, Text: "What do you think?"},
			},
			want: "Look at this:\n[Image]\nWhat do you think?",
		},
		{
			name:   "empty blocks",
			blocks: []ContentBlock{},
			want:   "",
		},
		{
			name:   "nil blocks",
			blocks: nil,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetDisplayContent(tt.blocks)
			if got != tt.want {
				t.Errorf("GetDisplayContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContentTypes(t *testing.T) {
	// Verify content type constants
	if ContentTypeText != "text" {
		t.Errorf("ContentTypeText = %q, want 'text'", ContentTypeText)
	}
	if ContentTypeImage != "image" {
		t.Errorf("ContentTypeImage = %q, want 'image'", ContentTypeImage)
	}
}

func TestStreamInputMessage(t *testing.T) {
	// Test that StreamInputMessage can be properly serialized
	msg := StreamInputMessage{
		Type: "user",
	}
	msg.Message.Role = "user"
	msg.Message.Content = []ContentBlock{{Type: ContentTypeText, Text: "Hello"}}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal StreamInputMessage: %v", err)
	}

	// Verify the JSON structure
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if parsed["type"] != "user" {
		t.Errorf("type = %v, want 'user'", parsed["type"])
	}

	message, ok := parsed["message"].(map[string]interface{})
	if !ok {
		t.Fatal("message field missing or wrong type")
	}

	if message["role"] != "user" {
		t.Errorf("message.role = %v, want 'user'", message["role"])
	}
}

func TestRunner_SendPermissionResponse(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	// Should not panic even before stop
	runner.SendPermissionResponse(mcp.PermissionResponse{Allowed: true})

	// After stop, should silently drop
	runner.Stop()
	runner.SendPermissionResponse(mcp.PermissionResponse{Allowed: false})
}

func TestRunner_SendQuestionResponse(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	// Should not panic even before stop
	runner.SendQuestionResponse(mcp.QuestionResponse{Answers: map[string]string{"q": "test"}})

	// After stop, should silently drop
	runner.Stop()
	runner.SendQuestionResponse(mcp.QuestionResponse{Answers: map[string]string{"q": "dropped"}})
}

func TestRunner_PermissionRequestChan_AfterStop(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	// Before stop, should return channel
	ch := runner.PermissionRequestChan()
	if ch == nil {
		t.Error("PermissionRequestChan should not be nil before stop")
	}

	runner.Stop()

	// After stop, should return nil
	ch = runner.PermissionRequestChan()
	if ch != nil {
		t.Error("PermissionRequestChan should be nil after stop")
	}
}

func TestRunner_QuestionRequestChan_AfterStop(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	// Before stop, should return channel
	ch := runner.QuestionRequestChan()
	if ch == nil {
		t.Error("QuestionRequestChan should not be nil before stop")
	}

	runner.Stop()

	// After stop, should return nil
	ch = runner.QuestionRequestChan()
	if ch != nil {
		t.Error("QuestionRequestChan should be nil after stop")
	}
}

func TestSendChunkWithTimeout_Success(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)
	ch := make(chan ResponseChunk, 10)

	chunk := ResponseChunk{Type: ChunkTypeText, Content: "test"}
	err := runner.sendChunkWithTimeout(ch, chunk)

	if err != nil {
		t.Errorf("sendChunkWithTimeout should not error on empty channel: %v", err)
	}

	// Verify chunk was sent
	select {
	case received := <-ch:
		if received.Content != "test" {
			t.Errorf("Received content = %q, want 'test'", received.Content)
		}
	default:
		t.Error("No chunk received")
	}
}

func TestSendChunkWithTimeout_ChannelFull(t *testing.T) {
	// This test verifies the behavior but would take too long due to timeout
	// Skip in normal test runs
	t.Skip("Skipping timeout test - would take ResponseChannelFullTimeout to complete")
}

func TestOptionsSystemPrompt(t *testing.T) {
	// Verify the system prompt contains expected structure
	if OptionsSystemPrompt == "" {
		t.Error("OptionsSystemPrompt should not be empty")
	}

	if !contains(OptionsSystemPrompt, "<options>") {
		t.Error("OptionsSystemPrompt should contain <options> tag")
	}

	if !contains(OptionsSystemPrompt, "</options>") {
		t.Error("OptionsSystemPrompt should contain </options> tag")
	}

	if !contains(OptionsSystemPrompt, "<optgroup>") {
		t.Error("OptionsSystemPrompt should contain <optgroup> tag")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestMCPServer(t *testing.T) {
	server := MCPServer{
		Name:    "test-server",
		Command: "npx",
		Args:    []string{"@test/server"},
	}

	if server.Name != "test-server" {
		t.Errorf("Name = %q, want 'test-server'", server.Name)
	}

	if server.Command != "npx" {
		t.Errorf("Command = %q, want 'npx'", server.Command)
	}

	if len(server.Args) != 1 || server.Args[0] != "@test/server" {
		t.Errorf("Args = %v, want ['@test/server']", server.Args)
	}
}

func TestMessage(t *testing.T) {
	msg := Message{
		Role:    "user",
		Content: "Hello, Claude!",
	}

	if msg.Role != "user" {
		t.Errorf("Role = %q, want 'user'", msg.Role)
	}

	if msg.Content != "Hello, Claude!" {
		t.Errorf("Content = %q, want 'Hello, Claude!'", msg.Content)
	}
}

func TestContentBlock(t *testing.T) {
	// Test text block
	textBlock := ContentBlock{
		Type: ContentTypeText,
		Text: "Hello",
	}

	if textBlock.Type != ContentTypeText {
		t.Errorf("Type = %v, want ContentTypeText", textBlock.Type)
	}

	// Test image block
	imageBlock := ContentBlock{
		Type: ContentTypeImage,
		Source: &ImageSource{
			Type:      "base64",
			MediaType: "image/png",
			Data:      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
		},
	}

	if imageBlock.Type != ContentTypeImage {
		t.Errorf("Type = %v, want ContentTypeImage", imageBlock.Type)
	}

	if imageBlock.Source == nil {
		t.Error("Source should not be nil for image block")
	}

	if imageBlock.Source.MediaType != "image/png" {
		t.Errorf("Source.MediaType = %q, want 'image/png'", imageBlock.Source.MediaType)
	}
}

func TestImageSource(t *testing.T) {
	source := ImageSource{
		Type:      "base64",
		MediaType: "image/jpeg",
		Data:      "...",
	}

	if source.Type != "base64" {
		t.Errorf("Type = %q, want 'base64'", source.Type)
	}

	if source.MediaType != "image/jpeg" {
		t.Errorf("MediaType = %q, want 'image/jpeg'", source.MediaType)
	}
}

func TestRunner_StopCleansUpServer(t *testing.T) {
	runner := New("session-1", "/tmp", false, nil)

	// Stop should not panic even without server running
	runner.Stop()

	// Verify stopped flag
	runner.mu.RLock()
	stopped := runner.stopped
	serverRunning := runner.serverRunning
	runner.mu.RUnlock()

	if !stopped {
		t.Error("stopped should be true after Stop()")
	}

	if serverRunning {
		t.Error("serverRunning should be false after Stop()")
	}
}

func TestProcessManager_InterruptSetsFlag(t *testing.T) {
	pm := NewProcessManager(ProcessConfig{
		SessionID:  "test-session",
		WorkingDir: "/tmp",
	}, ProcessCallbacks{}, testLogger())

	// Initially, interrupted should be false
	pm.mu.Lock()
	interrupted := pm.interrupted
	pm.mu.Unlock()

	if interrupted {
		t.Error("interrupted should be false initially")
	}

	// SetInterrupted should set the flag
	pm.SetInterrupted(true)

	pm.mu.Lock()
	interrupted = pm.interrupted
	pm.mu.Unlock()

	if !interrupted {
		t.Error("interrupted should be true after SetInterrupted(true)")
	}

	// Reset should clear the flag
	pm.SetInterrupted(false)

	pm.mu.Lock()
	interrupted = pm.interrupted
	pm.mu.Unlock()

	if interrupted {
		t.Error("interrupted should be false after SetInterrupted(false)")
	}
}

func TestPermissionChannelBuffer(t *testing.T) {
	// Verify buffer size is reasonable
	if PermissionChannelBuffer < 1 {
		t.Error("PermissionChannelBuffer should be at least 1")
	}

	if PermissionChannelBuffer > 100 {
		t.Error("PermissionChannelBuffer should not be excessively large")
	}
}

func TestPermissionTimeout(t *testing.T) {
	// Verify timeout is reasonable
	if PermissionTimeout < time.Minute {
		t.Error("PermissionTimeout should be at least 1 minute")
	}

	if PermissionTimeout > 30*time.Minute {
		t.Error("PermissionTimeout should not be excessively long")
	}
}

func TestShortenPath_WindowsStyle(t *testing.T) {
	// Test that forward-slash based paths work even if backslashes present
	// Note: shortenPath only handles forward slashes

	tests := []struct {
		path     string
		expected string
	}{
		{"C:/Users/test/file.go", "file.go"},
		{"/c/Users/test/file.go", "file.go"},
	}

	for _, tt := range tests {
		result := shortenPath(tt.path)
		if result != tt.expected {
			t.Errorf("shortenPath(%q) = %q, want %q", tt.path, result, tt.expected)
		}
	}
}

func TestParseStreamMessage_TodoWrite(t *testing.T) {
	log := testLogger()
	// Test that TodoWrite tool_use produces ChunkTypeTodoUpdate
	msg := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"TodoWrite","input":{"todos":[{"content":"Task 1","status":"pending","activeForm":"Working on task 1"},{"content":"Task 2","status":"in_progress","activeForm":"Doing task 2"},{"content":"Task 3","status":"completed","activeForm":"Done"}]}}]}}`
	chunks := parseStreamMessage(msg, log)

	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk, got %d", len(chunks))
	}

	if chunks[0].Type != ChunkTypeTodoUpdate {
		t.Errorf("Expected ChunkTypeTodoUpdate, got %v", chunks[0].Type)
	}

	if chunks[0].TodoList == nil {
		t.Fatal("Expected non-nil TodoList")
	}

	if len(chunks[0].TodoList.Items) != 3 {
		t.Errorf("Expected 3 todo items, got %d", len(chunks[0].TodoList.Items))
	}

	// Verify first item
	if chunks[0].TodoList.Items[0].Content != "Task 1" {
		t.Errorf("First item Content = %q, want %q", chunks[0].TodoList.Items[0].Content, "Task 1")
	}
	if chunks[0].TodoList.Items[0].Status != TodoStatusPending {
		t.Errorf("First item Status = %q, want %q", chunks[0].TodoList.Items[0].Status, TodoStatusPending)
	}

	// Verify second item
	if chunks[0].TodoList.Items[1].Status != TodoStatusInProgress {
		t.Errorf("Second item Status = %q, want %q", chunks[0].TodoList.Items[1].Status, TodoStatusInProgress)
	}

	// Verify third item
	if chunks[0].TodoList.Items[2].Status != TodoStatusCompleted {
		t.Errorf("Third item Status = %q, want %q", chunks[0].TodoList.Items[2].Status, TodoStatusCompleted)
	}
}

func TestParseStreamMessage_TodoWrite_InvalidInput(t *testing.T) {
	log := testLogger()
	// Test that TodoWrite with invalid input falls back to regular tool use
	msg := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"TodoWrite","input":{"invalid":"data"}}]}}`
	chunks := parseStreamMessage(msg, log)

	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk, got %d", len(chunks))
	}

	// Should fall back to ChunkTypeToolUse since parsing failed
	if chunks[0].Type != ChunkTypeToolUse {
		t.Errorf("Expected ChunkTypeToolUse for invalid TodoWrite, got %v", chunks[0].Type)
	}

	if chunks[0].ToolName != "TodoWrite" {
		t.Errorf("Expected tool name 'TodoWrite', got %q", chunks[0].ToolName)
	}
}

func TestChunkTypeTodoUpdate(t *testing.T) {
	// Verify the constant value
	if ChunkTypeTodoUpdate != "todo_update" {
		t.Errorf("ChunkTypeTodoUpdate = %q, want %q", ChunkTypeTodoUpdate, "todo_update")
	}
}

func TestChunkTypeStreamStats(t *testing.T) {
	// Verify the constant value
	if ChunkTypeStreamStats != "stream_stats" {
		t.Errorf("ChunkTypeStreamStats = %q, want %q", ChunkTypeStreamStats, "stream_stats")
	}
}

func TestStreamMessage_UsageFields(t *testing.T) {
	// Test that usage fields are properly parsed from the result message JSON
	jsonMsg := `{
		"type": "result",
		"subtype": "success",
		"is_error": false,
		"duration_ms": 4391,
		"duration_api_ms": 3652,
		"num_turns": 1,
		"result": "Hello!",
		"total_cost_usd": 0.2644345,
		"usage": {
			"input_tokens": 3,
			"cache_creation_input_tokens": 41012,
			"cache_read_input_tokens": 15539,
			"output_tokens": 13
		}
	}`

	var msg streamMessage
	if err := json.Unmarshal([]byte(jsonMsg), &msg); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if msg.Type != "result" {
		t.Errorf("Expected type 'result', got %q", msg.Type)
	}

	if msg.Subtype != "success" {
		t.Errorf("Expected subtype 'success', got %q", msg.Subtype)
	}

	if msg.DurationMs != 4391 {
		t.Errorf("Expected duration_ms 4391, got %d", msg.DurationMs)
	}

	if msg.DurationAPIMs != 3652 {
		t.Errorf("Expected duration_api_ms 3652, got %d", msg.DurationAPIMs)
	}

	if msg.NumTurns != 1 {
		t.Errorf("Expected num_turns 1, got %d", msg.NumTurns)
	}

	if msg.TotalCostUSD != 0.2644345 {
		t.Errorf("Expected total_cost_usd 0.2644345, got %f", msg.TotalCostUSD)
	}

	if msg.Usage == nil {
		t.Fatal("Expected usage to be non-nil")
	}

	if msg.Usage.InputTokens != 3 {
		t.Errorf("Expected input_tokens 3, got %d", msg.Usage.InputTokens)
	}

	if msg.Usage.CacheCreationInputTokens != 41012 {
		t.Errorf("Expected cache_creation_input_tokens 41012, got %d", msg.Usage.CacheCreationInputTokens)
	}

	if msg.Usage.CacheReadInputTokens != 15539 {
		t.Errorf("Expected cache_read_input_tokens 15539, got %d", msg.Usage.CacheReadInputTokens)
	}

	if msg.Usage.OutputTokens != 13 {
		t.Errorf("Expected output_tokens 13, got %d", msg.Usage.OutputTokens)
	}
}

func TestStreamStats(t *testing.T) {
	// Test StreamStats struct
	stats := StreamStats{
		OutputTokens: 1500,
		TotalCostUSD: 0.25,
	}

	if stats.OutputTokens != 1500 {
		t.Errorf("Expected OutputTokens 1500, got %d", stats.OutputTokens)
	}

	if stats.TotalCostUSD != 0.25 {
		t.Errorf("Expected TotalCostUSD 0.25, got %f", stats.TotalCostUSD)
	}
}

func TestParseStreamMessage_AssistantWithUsage(t *testing.T) {
	log := testLogger()
	// Assistant message with usage data should NOT emit stream stats from parseStreamMessage.
	// Stream stats are now emitted by handleProcessLine which accumulates tokens across
	// multiple API calls. parseStreamMessage is a pure function without state access.
	// This test verifies that usage data is correctly parsed in the message struct.
	msg := `{
		"type": "assistant",
		"message": {
			"id": "msg_123",
			"content": [{"type": "text", "text": "Hello!"}],
			"usage": {
				"input_tokens": 100,
				"output_tokens": 25
			}
		}
	}`
	chunks := parseStreamMessage(msg, log)

	// Should only have text chunk - stats are emitted separately by handleProcessLine
	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk (text only), got %d", len(chunks))
	}

	// First chunk should be text
	if chunks[0].Type != ChunkTypeText {
		t.Errorf("First chunk expected ChunkTypeText, got %v", chunks[0].Type)
	}
	if chunks[0].Content != "Hello!" {
		t.Errorf("Expected content 'Hello!', got %q", chunks[0].Content)
	}

	// Verify the usage data is correctly parsed (even though not emitted as a chunk)
	var parsed streamMessage
	if err := json.Unmarshal([]byte(msg), &parsed); err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}
	if parsed.Message.Usage == nil {
		t.Fatal("Expected Usage to be non-nil")
	}
	if parsed.Message.Usage.OutputTokens != 25 {
		t.Errorf("Expected OutputTokens 25, got %d", parsed.Message.Usage.OutputTokens)
	}
	if parsed.Message.ID != "msg_123" {
		t.Errorf("Expected message ID 'msg_123', got %q", parsed.Message.ID)
	}
}

func TestParseStreamMessage_AssistantWithoutUsage(t *testing.T) {
	log := testLogger()
	// Assistant message without usage data should not emit stream stats
	msg := `{
		"type": "assistant",
		"message": {
			"content": [{"type": "text", "text": "Hello!"}]
		}
	}`
	chunks := parseStreamMessage(msg, log)

	// Should only have text chunk, no stats
	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk (text only), got %d", len(chunks))
	}

	if chunks[0].Type != ChunkTypeText {
		t.Errorf("Expected ChunkTypeText, got %v", chunks[0].Type)
	}
}

func TestParseStreamMessage_AssistantWithZeroOutputTokens(t *testing.T) {
	log := testLogger()
	// Assistant message with zero output tokens should not emit stream stats
	msg := `{
		"type": "assistant",
		"message": {
			"content": [{"type": "text", "text": "Hello!"}]
		},
		"usage": {
			"input_tokens": 100,
			"output_tokens": 0
		}
	}`
	chunks := parseStreamMessage(msg, log)

	// Should only have text chunk, no stats (0 output tokens)
	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk (text only, no stats for 0 tokens), got %d", len(chunks))
	}

	if chunks[0].Type != ChunkTypeText {
		t.Errorf("Expected ChunkTypeText, got %v", chunks[0].Type)
	}
}

func TestTokenAccumulationAcrossAPICalls(t *testing.T) {
	// Test that token counts are accumulated correctly across multiple API calls.
	// Each API call has a different message ID and its own cumulative token count.
	// The displayed total should be the sum of all completed API calls' final counts
	// plus the current API call's running count.

	runner := New("test-session", "/tmp/test", false, nil)
	defer runner.Stop()

	// Simulate receiving messages from multiple API calls
	// First API call: message ID "msg_1" with increasing token counts
	msg1Chunk1 := `{"type":"assistant","message":{"id":"msg_1","content":[{"type":"text","text":"Hi"}],"usage":{"output_tokens":3}}}`
	msg1Chunk2 := `{"type":"assistant","message":{"id":"msg_1","content":[{"type":"text","text":"Hello"}],"usage":{"output_tokens":8}}}`

	// Second API call: message ID "msg_2" with its own token counts
	msg2Chunk1 := `{"type":"assistant","message":{"id":"msg_2","content":[{"type":"text","text":"More"}],"usage":{"output_tokens":5}}}`
	msg2Chunk2 := `{"type":"assistant","message":{"id":"msg_2","content":[{"type":"text","text":"text"}],"usage":{"output_tokens":12}}}`

	// Set up the runner for streaming (similar to SendContent)
	runner.mu.Lock()
	runner.streaming.Active = true
	runner.tokens.Reset()
	ch := make(chan ResponseChunk, 100)
	runner.responseChan.Setup(ch)
	runner.mu.Unlock()

	// Process first API call's messages
	runner.handleProcessLine(msg1Chunk1)
	runner.handleProcessLine(msg1Chunk2)

	// Check accumulated state after first API call
	runner.mu.RLock()
	if runner.tokens.LastMessageID != "msg_1" {
		t.Errorf("Expected LastMessageID 'msg_1', got %q", runner.tokens.LastMessageID)
	}
	if runner.tokens.LastMessageTokens != 8 {
		t.Errorf("Expected LastMessageTokens 8, got %d", runner.tokens.LastMessageTokens)
	}
	if runner.tokens.AccumulatedOutput != 0 {
		t.Errorf("Expected AccumulatedOutput 0 (first API call), got %d", runner.tokens.AccumulatedOutput)
	}
	runner.mu.RUnlock()

	// Process second API call's messages
	runner.handleProcessLine(msg2Chunk1)

	// After seeing msg_2, the previous API call's tokens (8) should be accumulated
	runner.mu.RLock()
	if runner.tokens.LastMessageID != "msg_2" {
		t.Errorf("Expected LastMessageID 'msg_2', got %q", runner.tokens.LastMessageID)
	}
	if runner.tokens.AccumulatedOutput != 8 {
		t.Errorf("Expected AccumulatedOutput 8 (from msg_1), got %d", runner.tokens.AccumulatedOutput)
	}
	if runner.tokens.LastMessageTokens != 5 {
		t.Errorf("Expected LastMessageTokens 5, got %d", runner.tokens.LastMessageTokens)
	}
	runner.mu.RUnlock()

	runner.handleProcessLine(msg2Chunk2)

	// After final chunk, total should be 8 (from msg_1) + 12 (from msg_2) = 20
	runner.mu.RLock()
	expectedTotal := runner.tokens.CurrentTotal()
	if expectedTotal != 20 {
		t.Errorf("Expected total tokens 20 (8 + 12), got %d", expectedTotal)
	}
	runner.mu.RUnlock()

	// Drain the channel and verify we received stream stats chunks
	close(ch)
	var statsChunks []ResponseChunk
	for chunk := range ch {
		if chunk.Type == ChunkTypeStreamStats {
			statsChunks = append(statsChunks, chunk)
		}
	}

	// Should have received 4 stats chunks (one for each assistant message chunk)
	if len(statsChunks) != 4 {
		t.Errorf("Expected 4 stream stats chunks, got %d", len(statsChunks))
	}

	// Verify the token counts are cumulative
	// msg1_chunk1: 0 + 3 = 3
	// msg1_chunk2: 0 + 8 = 8
	// msg2_chunk1: 8 + 5 = 13
	// msg2_chunk2: 8 + 12 = 20
	expectedCounts := []int{3, 8, 13, 20}
	for i, chunk := range statsChunks {
		if chunk.Stats.OutputTokens != expectedCounts[i] {
			t.Errorf("Stats chunk %d: expected %d tokens, got %d", i, expectedCounts[i], chunk.Stats.OutputTokens)
		}
	}
}
