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

	chunks := parseStreamMessage(line, log)

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

	chunks := parseStreamMessage(line, log)

	// message_start doesn't produce content chunks
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for message_start, got %d", len(chunks))
	}
}

func TestParseStreamEvent_MessageDelta(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test parsing a message_delta event with usage data
	line := `{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":25}}}`

	chunks := parseStreamMessage(line, log)

	// message_delta doesn't produce content chunks (token handling is in claude.go)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for message_delta, got %d", len(chunks))
	}
}

func TestParseStreamEvent_ContentBlockStart(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test parsing a content_block_start for text
	line := `{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}}`

	chunks := parseStreamMessage(line, log)

	// content_block_start doesn't produce content chunks
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for content_block_start, got %d", len(chunks))
	}
}

func TestParseStreamEvent_ContentBlockStop(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test parsing a content_block_stop event
	line := `{"type":"stream_event","event":{"type":"content_block_stop","index":0}}`

	chunks := parseStreamMessage(line, log)

	// content_block_stop doesn't produce content chunks
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for content_block_stop, got %d", len(chunks))
	}
}

func TestParseStreamEvent_MessageStop(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test parsing a message_stop event
	line := `{"type":"stream_event","event":{"type":"message_stop"}}`

	chunks := parseStreamMessage(line, log)

	// message_stop doesn't produce content chunks
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for message_stop, got %d", len(chunks))
	}
}

func TestParseStreamEvent_InputJSONDelta(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test parsing an input_json_delta (for tool use streaming)
	line := `{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"file_path\":"}}}`

	chunks := parseStreamMessage(line, log)

	// input_json_delta doesn't produce content chunks (we wait for complete tool info)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for input_json_delta, got %d", len(chunks))
	}
}

func TestParseStreamEvent_ToolUseStart(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test parsing a content_block_start for tool_use
	line := `{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_123","name":"Read"}}}`

	chunks := parseStreamMessage(line, log)

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
		chunks := parseStreamMessage(line, log)
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

	chunks := parseStreamMessage(line, log)

	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for null event, got %d", len(chunks))
	}
}

func TestParseStreamEvent_MessageDeltaNilDelta(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Test message_delta with nil delta (edge case)
	line := `{"type":"stream_event","event":{"type":"message_delta","usage":{"output_tokens":25}}}`

	// Should not panic
	chunks := parseStreamMessage(line, log)

	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks, got %d", len(chunks))
	}
}
