package claude

import (
	"encoding/json"
	"log/slog"
	"strings"
)

// PermissionDenial represents a permission that was denied during the session.
// This is reported in the result message's permission_denials array.
type PermissionDenial struct {
	Tool        string `json:"tool"`        // Tool name that was denied (e.g., "Bash", "Edit")
	Description string `json:"description"` // Human-readable description of what was requested
	Reason      string `json:"reason"`      // Why it was denied (optional)
}

// streamMessage represents a JSON message from Claude's stream-json output
type streamMessage struct {
	Type            string `json:"type"`               // "system", "assistant", "user", "result", "stream_event"
	Subtype         string `json:"subtype"`            // "init", "success", etc.
	ParentToolUseID string `json:"parent_tool_use_id"` // Non-empty when message is from a subagent (e.g., Haiku via Task)
	Message         struct {
		ID      string `json:"id,omitempty"`    // Message ID for tracking API calls
		Model   string `json:"model,omitempty"` // Model that generated this message (e.g., "claude-haiku-4-5-20251001")
		Content []struct {
			Type      string          `json:"type"`         // "text", "tool_use", "tool_result"
			ID        string          `json:"id,omitempty"` // tool use ID (for tool_use)
			Text      string          `json:"text,omitempty"`
			Name      string          `json:"name,omitempty"`        // tool name
			Input     json.RawMessage `json:"input,omitempty"`       // tool input
			ToolUseID string          `json:"tool_use_id,omitempty"` // tool use ID reference (for tool_result)
			ToolUseId string          `json:"toolUseId,omitempty"`   // camelCase variant from Claude CLI
			Content   json.RawMessage `json:"content,omitempty"`     // tool result content (can be string or array)
		} `json:"content"`
		Usage *StreamUsage `json:"usage,omitempty"` // Token usage (for assistant messages)
	} `json:"message"`
	// Stream event fields (for type="stream_event" with --include-partial-messages)
	Event *streamEvent `json:"event,omitempty"`
	// ToolUseResult contains rich details about the tool execution result.
	// This is a top-level field in user messages, separate from message.content.
	// Can be either a string (for errors/simple results) or a structured object.
	ToolUseResult     *toolUseResultField         `json:"tool_use_result,omitempty"`
	Result            string                      `json:"result,omitempty"`             // Final result text
	Error             string                      `json:"error,omitempty"`              // Error message (alternative to result)
	Errors            []string                    `json:"errors,omitempty"`             // Error messages array (used by error_during_execution)
	PermissionDenials []PermissionDenial          `json:"permission_denials,omitempty"` // Permissions denied during session
	SessionID         string                      `json:"session_id,omitempty"`
	DurationMs        int                         `json:"duration_ms,omitempty"`     // Total duration in milliseconds
	DurationAPIMs     int                         `json:"duration_api_ms,omitempty"` // API duration in milliseconds
	NumTurns          int                         `json:"num_turns,omitempty"`       // Number of conversation turns
	TotalCostUSD      float64                     `json:"total_cost_usd,omitempty"`  // Total cost in USD
	Usage             *StreamUsage                `json:"usage,omitempty"`           // Token usage breakdown
	ModelUsage        map[string]*ModelUsageEntry `json:"modelUsage,omitempty"`      // Per-model usage breakdown (includes sub-agents)
}

// toolUseResultData represents the tool_use_result field in user messages.
// Different tool types populate different fields.
type toolUseResultData struct {
	// Common fields
	Type string `json:"type,omitempty"` // e.g., "text"

	// Read tool results
	File *toolUseResultFile `json:"file,omitempty"`

	// Edit tool results
	FilePath        string `json:"filePath,omitempty"`
	NewString       string `json:"newString,omitempty"`
	OldString       string `json:"oldString,omitempty"`
	StructuredPatch any    `json:"structuredPatch,omitempty"` // Indicates edit was applied

	// Glob tool results
	NumFiles  int      `json:"numFiles,omitempty"`
	Filenames []string `json:"filenames,omitempty"`

	// Bash tool results
	ExitCode *int   `json:"exitCode,omitempty"`
	Stdout   string `json:"stdout,omitempty"`
	Stderr   string `json:"stderr,omitempty"`
}

// toolUseResultField wraps toolUseResultData to handle the case where
// tool_use_result can be either a string (for errors/simple results)
// or a structured object (for rich result data).
type toolUseResultField struct {
	// StringValue is populated when tool_use_result is a plain string
	StringValue string
	// Data is populated when tool_use_result is a structured object
	Data *toolUseResultData
}

// UnmarshalJSON implements json.Unmarshaler for toolUseResultField.
// It handles both string values and structured objects.
func (f *toolUseResultField) UnmarshalJSON(data []byte) error {
	// First, try to unmarshal as a string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		f.StringValue = s
		return nil
	}

	// Not a string, try as structured object
	var obj toolUseResultData
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	f.Data = &obj
	return nil
}

// toolUseResultFile represents file info in Read tool results
type toolUseResultFile struct {
	FilePath   string `json:"filePath,omitempty"`
	NumLines   int    `json:"numLines,omitempty"`
	StartLine  int    `json:"startLine,omitempty"`
	TotalLines int    `json:"totalLines,omitempty"`
}

// streamEvent represents the event payload in stream_event messages
// These are sent when --include-partial-messages is enabled
type streamEvent struct {
	Type    string `json:"type"` // "message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"
	Index   int    `json:"index,omitempty"`
	Message *struct {
		ID    string       `json:"id,omitempty"`
		Usage *StreamUsage `json:"usage,omitempty"`
	} `json:"message,omitempty"`
	ContentBlock *struct {
		Type string `json:"type,omitempty"` // "text", "tool_use"
		Text string `json:"text,omitempty"`
		ID   string `json:"id,omitempty"`   // tool use ID
		Name string `json:"name,omitempty"` // tool name
	} `json:"content_block,omitempty"`
	Delta *struct {
		Type        string          `json:"type,omitempty"` // "text_delta", "input_json_delta"
		Text        string          `json:"text,omitempty"`
		PartialJSON string          `json:"partial_json,omitempty"`
		StopReason  string          `json:"stop_reason,omitempty"`
		Input       json.RawMessage `json:"input,omitempty"` // Complete tool input (for tool_use blocks)
	} `json:"delta,omitempty"`
	Usage *StreamUsage `json:"usage,omitempty"` // Token usage in message_delta
}

// parseStreamMessage parses a JSON line from Claude's stream-json output
// and returns zero or more ResponseChunks representing the message content.
// When hasStreamEvents is true, text content from "assistant" messages is skipped
// because it duplicates content already delivered via stream_event deltas.
func parseStreamMessage(line string, hasStreamEvents bool, log *slog.Logger) []ResponseChunk {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	// Skip lines that aren't JSON objects. Claude CLI with --verbose may output
	// non-JSON informational lines to stdout that we should silently ignore.
	if !strings.HasPrefix(line, "{") {
		log.Debug("skipping non-JSON line from Claude CLI", "line", truncateForLog(line))
		return nil
	}

	var msg streamMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		log.Warn("failed to parse stream message", "error", err, "line", truncateForLog(line))
		// Log the error but don't show it to the user - this can happen with
		// unexpected output from Claude CLI (e.g., verbose messages, warnings)
		return nil
	}

	// If this looks like a stream-json message but we don't handle it, log and skip
	if msg.Type == "" {
		log.Warn("unrecognized JSON message type", "line", truncateForLog(line))
		return nil
	}

	var chunks []ResponseChunk

	switch msg.Type {
	case "system":
		// Init message - we could show "Session started" but skip for now
		if msg.Subtype == "init" {
			log.Debug("session initialized")
		}

	case "stream_event":
		// Stream events are sent when --include-partial-messages is enabled
		// They contain incremental updates (text deltas, token counts, etc.)
		if msg.Event != nil {
			chunks = append(chunks, parseStreamEvent(msg.Event, log)...)
		}

	case "assistant":
		// Assistant messages can contain text or tool_use
		for _, content := range msg.Message.Content {
			switch content.Type {
			case "text":
				// When stream events are active (--include-partial-messages), text content
				// was already delivered incrementally via content_block_delta events.
				// Skip it here to avoid duplication.
				if hasStreamEvents {
					log.Debug("skipping assistant text (already streamed via deltas)", "len", len(content.Text))
					continue
				}
				if content.Text != "" {
					chunks = append(chunks, ResponseChunk{
						Type:    ChunkTypeText,
						Content: content.Text,
					})
				}
			case "tool_use":
				// Handle TodoWrite specially - parse and return the full todo list
				if content.Name == "TodoWrite" {
					todoList, err := ParseTodoWriteInput(content.Input)
					if err != nil {
						log.Warn("failed to parse TodoWrite input", "error", err)
						// Fall through to regular tool use display on parse error
					} else {
						chunks = append(chunks, ResponseChunk{
							Type:     ChunkTypeTodoUpdate,
							TodoList: todoList,
						})
						log.Debug("TodoWrite parsed", "itemCount", len(todoList.Items))
						continue
					}
				}

				// Extract a brief description from the tool input
				inputDesc := extractToolInputDescription(content.Name, content.Input)
				chunks = append(chunks, ResponseChunk{
					Type:      ChunkTypeToolUse,
					ToolName:  content.Name,
					ToolInput: inputDesc,
					ToolUseID: content.ID,
				})
				log.Debug("tool use", "tool", content.Name, "id", content.ID, "input", inputDesc)
			}
		}
		// Note: Stream stats are emitted by handleProcessLine with accumulated token counts,
		// not here, because parseStreamMessage is a pure function without runner state access.

	case "user":
		// User messages in stream-json are tool results
		// We need to emit a ChunkTypeToolResult so the UI can mark the tool use as complete.
		// We also extract rich result info from the top-level tool_use_result field.
		for _, content := range msg.Message.Content {
			// Check for tool_result type or presence of tool use ID (indicates tool result)
			// Get the tool use ID from either snake_case or camelCase field
			toolUseID := content.ToolUseID
			if toolUseID == "" {
				toolUseID = content.ToolUseId
			}
			isToolResult := content.Type == "tool_result" || toolUseID != ""
			if isToolResult {
				// Extract rich result info from the top-level tool_use_result field
				resultInfo := extractToolResultInfo(msg.ToolUseResult)

				// Emit a tool result chunk so UI can mark tool as complete
				log.Debug("tool result received", "toolUseID", toolUseID, "resultInfo", resultInfo != nil)
				chunks = append(chunks, ResponseChunk{
					Type:       ChunkTypeToolResult,
					ToolUseID:  toolUseID,
					ResultInfo: resultInfo,
				})
			}
		}

	case "result":
		// Final result - the actual result text is in msg.Result
		// For error results, the error message is in msg.Result
		log.Debug("result received", "subtype", msg.Subtype, "result", msg.Result)
	}

	return chunks
}

// parseStreamEvent extracts content from stream_event messages
// These provide real-time streaming updates when --include-partial-messages is enabled
func parseStreamEvent(event *streamEvent, log *slog.Logger) []ResponseChunk {
	var chunks []ResponseChunk

	switch event.Type {
	case "message_start":
		// Initial message with usage data - we handle token updates in claude.go
		log.Debug("stream: message_start")

	case "content_block_start":
		// Start of a content block (text or tool_use)
		if event.ContentBlock != nil {
			switch event.ContentBlock.Type {
			case "text":
				log.Debug("stream: content_block_start (text)")
			case "tool_use":
				// Tool use is starting - we'll get the full tool info when it completes
				log.Debug("stream: content_block_start (tool_use)", "id", event.ContentBlock.ID, "name", event.ContentBlock.Name)
			}
		}

	case "content_block_delta":
		// Incremental content update
		if event.Delta != nil {
			switch event.Delta.Type {
			case "text_delta":
				// Text chunk - emit as regular text
				if event.Delta.Text != "" {
					chunks = append(chunks, ResponseChunk{
						Type:    ChunkTypeText,
						Content: event.Delta.Text,
					})
				}
			case "input_json_delta":
				// Tool input being streamed - we wait for the complete input
				// in the assistant message, so we don't emit anything here
				log.Debug("stream: input_json_delta")
			}
		}

	case "content_block_stop":
		// End of a content block
		log.Debug("stream: content_block_stop", "index", event.Index)

	case "message_delta":
		// Message-level update with final usage stats
		// Token updates are handled in claude.go via the event's Usage field
		if event.Delta != nil {
			log.Debug("stream: message_delta", "stop_reason", event.Delta.StopReason)
		} else {
			log.Debug("stream: message_delta")
		}

	case "message_stop":
		// End of message
		log.Debug("stream: message_stop")
	}

	return chunks
}

// toolInputConfig defines how to extract a description from a tool's input.
type toolInputConfig struct {
	Field       string // JSON field to extract
	ShortenPath bool   // Whether to shorten file paths to just filename
	MaxLen      int    // Maximum length before truncation (0 = no limit)
}

// toolInputConfigs maps tool names to their input extraction configuration.
// This replaces the hardcoded switch statement, making it easier to add new tools.
var toolInputConfigs = map[string]toolInputConfig{
	// File operations - extract file_path and shorten to filename
	"Read":  {Field: "file_path", ShortenPath: true},
	"Edit":  {Field: "file_path", ShortenPath: true},
	"Write": {Field: "file_path", ShortenPath: true},

	// Search operations - extract the pattern/query
	"Glob":      {Field: "pattern"},
	"Grep":      {Field: "pattern", MaxLen: 30},
	"WebSearch": {Field: "query"},

	// Command execution - show the command with truncation
	"Bash": {Field: "command", MaxLen: 40},

	// Task delegation - show the description
	"Task": {Field: "description"},

	// Web operations - show URL with truncation
	"WebFetch": {Field: "url", MaxLen: 40},
}

// DefaultToolInputMaxLen is the default max length for tool descriptions.
const DefaultToolInputMaxLen = 40

// extractToolInputDescription extracts a brief, human-readable description from tool input.
// Uses the toolInputConfigs map for configuration-driven extraction.
func extractToolInputDescription(toolName string, input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}

	var inputMap map[string]any
	if err := json.Unmarshal(input, &inputMap); err != nil {
		return ""
	}

	// Check if we have a config for this tool
	if cfg, ok := toolInputConfigs[toolName]; ok {
		if value, exists := inputMap[cfg.Field].(string); exists {
			return formatToolInput(value, cfg.ShortenPath, cfg.MaxLen)
		}
	}

	// Default: return first string value found
	for _, v := range inputMap {
		if s, ok := v.(string); ok && s != "" {
			return truncateString(s, DefaultToolInputMaxLen)
		}
	}
	return ""
}

// formatToolInput formats a tool input value according to the config.
func formatToolInput(value string, shorten bool, maxLen int) string {
	if shorten {
		value = shortenPath(value)
	}
	if maxLen > 0 {
		value = truncateString(value, maxLen)
	}
	return value
}

// truncateString truncates a string to maxLen characters, including "..." suffix.
// A maxLen of 0 means no limit.
func truncateString(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// shortenPath returns just the filename or last path component
func shortenPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}

// truncateForLog truncates long strings for log messages
func truncateForLog(s string) string {
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

// formatToolIcon returns a human-readable verb for the tool type
func formatToolIcon(toolName string) string {
	switch toolName {
	case "Read":
		return "Reading"
	case "Edit":
		return "Editing"
	case "Write":
		return "Writing"
	case "Glob":
		return "Searching"
	case "Grep":
		return "Searching"
	case "Bash":
		return "Running"
	case "Task":
		return "Delegating"
	case "WebFetch":
		return "Fetching"
	case "WebSearch":
		return "Searching"
	// Note: TodoWrite is handled specially via ChunkTypeTodoUpdate,
	// so it won't reach this function in normal operation
	default:
		return "Using"
	}
}

// extractToolResultInfo extracts rich result information from the tool_use_result field.
// Returns nil if no meaningful info can be extracted.
func extractToolResultInfo(field *toolUseResultField) *ToolResultInfo {
	if field == nil {
		return nil
	}

	// If tool_use_result was a string (error message or simple result), no rich info
	if field.Data == nil {
		return nil
	}

	data := field.Data
	info := &ToolResultInfo{}
	hasData := false

	// Read tool results - file info
	if data.File != nil {
		info.FilePath = data.File.FilePath
		info.NumLines = data.File.NumLines
		info.StartLine = data.File.StartLine
		info.TotalLines = data.File.TotalLines
		hasData = true
	}

	// Edit tool results - check if structuredPatch exists (indicates edit was applied)
	if data.StructuredPatch != nil {
		info.Edited = true
		info.FilePath = data.FilePath
		hasData = true
	}

	// Glob tool results - file count
	if data.NumFiles > 0 {
		info.NumFiles = data.NumFiles
		hasData = true
	} else if len(data.Filenames) > 0 {
		info.NumFiles = len(data.Filenames)
		hasData = true
	}

	// Bash tool results - exit code
	if data.ExitCode != nil {
		info.ExitCode = data.ExitCode
		hasData = true
	}

	if !hasData {
		return nil
	}

	return info
}
