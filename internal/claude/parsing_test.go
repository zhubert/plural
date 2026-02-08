package claude

import (
	"log/slog"
	"os"
	"testing"
)

func TestParseStreamEvent_TextDelta(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test parsing a content_block_delta with text
	line := `{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}}`

	chunks := parseStreamMessage(line, false, log)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}

	if chunks[0].Type != ChunkTypeText {
		t.Errorf("expected ChunkTypeText, got %v", chunks[0].Type)
	}

	if chunks[0].Content != "Hello" {
		t.Errorf("expected 'Hello', got %q", chunks[0].Content)
	}
}

func TestParseStreamEvent_MessageStart(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test parsing a message_start event (should not produce chunks, just logs)
	line := `{"type":"stream_event","event":{"type":"message_start","message":{"id":"msg_123","usage":{"output_tokens":5}}}}`

	chunks := parseStreamMessage(line, false, log)

	// message_start doesn't produce content chunks
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for message_start, got %d", len(chunks))
	}
}

func TestParseStreamEvent_MessageDelta(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test parsing a message_delta event with usage data
	line := `{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":25}}}`

	chunks := parseStreamMessage(line, false, log)

	// message_delta doesn't produce content chunks (token handling is in claude.go)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for message_delta, got %d", len(chunks))
	}
}

func TestParseStreamEvent_ContentBlockStart(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test parsing a content_block_start for text
	line := `{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}}`

	chunks := parseStreamMessage(line, false, log)

	// content_block_start doesn't produce content chunks
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for content_block_start, got %d", len(chunks))
	}
}

func TestParseStreamEvent_ContentBlockStop(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test parsing a content_block_stop event
	line := `{"type":"stream_event","event":{"type":"content_block_stop","index":0}}`

	chunks := parseStreamMessage(line, false, log)

	// content_block_stop doesn't produce content chunks
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for content_block_stop, got %d", len(chunks))
	}
}

func TestParseStreamEvent_MessageStop(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test parsing a message_stop event
	line := `{"type":"stream_event","event":{"type":"message_stop"}}`

	chunks := parseStreamMessage(line, false, log)

	// message_stop doesn't produce content chunks
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for message_stop, got %d", len(chunks))
	}
}

func TestParseStreamEvent_InputJSONDelta(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test parsing an input_json_delta (for tool use streaming)
	line := `{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"file_path\":"}}}`

	chunks := parseStreamMessage(line, false, log)

	// input_json_delta doesn't produce content chunks (we wait for complete tool info)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for input_json_delta, got %d", len(chunks))
	}
}

func TestParseStreamEvent_ToolUseStart(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test parsing a content_block_start for tool_use
	line := `{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_123","name":"Read"}}}`

	chunks := parseStreamMessage(line, false, log)

	// content_block_start for tool_use doesn't produce chunks (we wait for complete assistant message)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for tool_use content_block_start, got %d", len(chunks))
	}
}

func TestParseStreamEvent_MultipleTextDeltas(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Simulate multiple text deltas coming through
	lines := []string{
		`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" "}}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"World"}}}`,
	}

	var allChunks []ResponseChunk
	for _, line := range lines {
		chunks := parseStreamMessage(line, false, log)
		allChunks = append(allChunks, chunks...)
	}

	if len(allChunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(allChunks))
	}

	expected := []string{"Hello", " ", "World"}
	for i, chunk := range allChunks {
		if chunk.Content != expected[i] {
			t.Errorf("chunk %d: expected %q, got %q", i, expected[i], chunk.Content)
		}
	}
}

func TestParseStreamEvent_NilEvent(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test with stream_event type but null event field
	line := `{"type":"stream_event","event":null}`

	chunks := parseStreamMessage(line, false, log)

	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for null event, got %d", len(chunks))
	}
}

func TestParseStreamEvent_MessageDeltaNilDelta(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test message_delta with nil delta (edge case)
	line := `{"type":"stream_event","event":{"type":"message_delta","usage":{"output_tokens":25}}}`

	// Should not panic
	chunks := parseStreamMessage(line, false, log)

	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks, got %d", len(chunks))
	}
}

func TestParseStreamMessage_NonJSONLineSkipped(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Non-JSON lines (e.g., verbose output from Claude CLI) should be silently skipped
	lines := []string{
		"Loading configuration...",
		"Warning: some deprecation notice",
		"  indented non-JSON line",
	}

	for _, line := range lines {
		chunks := parseStreamMessage(line, false, log)
		if len(chunks) != 0 {
			t.Errorf("expected 0 chunks for non-JSON line %q, got %d", line, len(chunks))
		}
	}
}

func TestParseStreamMessage_InvalidJSONSkipped(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Malformed JSON that starts with '{' should be silently skipped
	chunks := parseStreamMessage("{malformed json}", false, log)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for malformed JSON, got %d", len(chunks))
	}
}

func TestParseStreamMessage_EmptyTypeSkipped(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Valid JSON but with empty type should be silently skipped
	chunks := parseStreamMessage(`{"data":"something"}`, false, log)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty type JSON, got %d", len(chunks))
	}
}

func TestParseStreamMessage_AssistantTextSkippedWithStreamEvents(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// When hasStreamEvents is true, assistant text should be skipped
	// (it was already delivered via stream_event deltas)
	msg := `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello, world!"}]}}`
	chunks := parseStreamMessage(msg, true, log)

	// No text chunks should be produced
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks when hasStreamEvents=true, got %d", len(chunks))
	}
}

func TestParseStreamMessage_AssistantTextEmittedWithoutStreamEvents(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// When hasStreamEvents is false, assistant text should be emitted
	msg := `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello, world!"}]}}`
	chunks := parseStreamMessage(msg, false, log)

	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk when hasStreamEvents=false, got %d", len(chunks))
	}

	if chunks[0].Type != ChunkTypeText {
		t.Errorf("expected ChunkTypeText, got %v", chunks[0].Type)
	}

	if chunks[0].Content != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got %q", chunks[0].Content)
	}
}

func TestParseStreamMessage_AssistantToolUseNotSkippedWithStreamEvents(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Tool uses should still be emitted even when hasStreamEvents=true
	msg := `{"type":"assistant","message":{"content":[{"type":"text","text":"Let me read that file."},{"type":"tool_use","id":"toolu_123","name":"Read","input":{"file_path":"/path/to/file.go"}}]}}`
	chunks := parseStreamMessage(msg, true, log)

	// Only tool use chunk should be produced, text is skipped
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk (tool_use only), got %d", len(chunks))
	}

	if chunks[0].Type != ChunkTypeToolUse {
		t.Errorf("expected ChunkTypeToolUse, got %v", chunks[0].Type)
	}

	if chunks[0].ToolName != "Read" {
		t.Errorf("expected tool name 'Read', got %q", chunks[0].ToolName)
	}
}

func TestParseStreamMessage_StreamEventTextDeltaUnaffected(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Stream events should produce text chunks regardless of hasStreamEvents flag
	line := `{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}}`

	// With hasStreamEvents=true
	chunks := parseStreamMessage(line, true, log)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk with hasStreamEvents=true, got %d", len(chunks))
	}
	if chunks[0].Content != "Hello" {
		t.Errorf("expected 'Hello', got %q", chunks[0].Content)
	}

	// With hasStreamEvents=false
	chunks = parseStreamMessage(line, false, log)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk with hasStreamEvents=false, got %d", len(chunks))
	}
	if chunks[0].Content != "Hello" {
		t.Errorf("expected 'Hello', got %q", chunks[0].Content)
	}
}

func TestTruncateString_IncludesEllipsis(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short string unchanged", "hi", 10, "hi"},
		{"exact length unchanged", "hello", 5, "hello"},
		{"truncated with ellipsis", "hello world", 8, "hello..."},
		{"very short maxLen", "hello", 2, "he"},
		{"maxLen 3", "hello", 3, "hel"},
		{"maxLen 4", "hello", 4, "h..."},
		{"empty string", "", 5, ""},
		{"zero maxLen means no limit", "hello", 0, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
			// Verify output never exceeds maxLen (when maxLen > 0)
			if tt.maxLen > 0 && len(got) > tt.maxLen {
				t.Errorf("output length %d exceeds maxLen %d", len(got), tt.maxLen)
			}
		})
	}
}
