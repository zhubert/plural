package claude

import (
	"encoding/json"
	"log/slog"
	"strings"
)

// streamMessage represents a JSON message from Claude's stream-json output
type streamMessage struct {
	Type              string `json:"type"`                 // "system", "assistant", "user", "result"
	Subtype           string `json:"subtype"`              // "init", "success", etc.
	ParentToolUseID   string `json:"parent_tool_use_id"`   // Non-empty when message is from a subagent (e.g., Haiku via Task)
	Message struct {
		ID      string `json:"id,omitempty"` // Message ID for tracking API calls
		Model   string `json:"model,omitempty"` // Model that generated this message (e.g., "claude-haiku-4-5-20251001")
		Content []struct {
			Type      string          `json:"type"` // "text", "tool_use", "tool_result"
			ID        string          `json:"id,omitempty"`         // tool use ID (for tool_use)
			Text      string          `json:"text,omitempty"`
			Name      string          `json:"name,omitempty"`       // tool name
			Input     json.RawMessage `json:"input,omitempty"`      // tool input
			ToolUseID string          `json:"tool_use_id,omitempty"` // tool use ID reference (for tool_result)
			ToolUseId string          `json:"toolUseId,omitempty"`  // camelCase variant from Claude CLI
			Content   json.RawMessage `json:"content,omitempty"`    // tool result content (can be string or array)
		} `json:"content"`
		Usage *StreamUsage `json:"usage,omitempty"` // Token usage (for assistant messages)
	} `json:"message"`
	// ToolUseResult contains rich details about the tool execution result.
	// This is a top-level field in user messages, separate from message.content.
	ToolUseResult *toolUseResultData `json:"tool_use_result,omitempty"`
	Result        string             `json:"result,omitempty"`          // Final result text
	Error         string             `json:"error,omitempty"`           // Error message (alternative to result)
	Errors        []string           `json:"errors,omitempty"`          // Error messages array (used by error_during_execution)
	SessionID     string             `json:"session_id,omitempty"`
	DurationMs    int                `json:"duration_ms,omitempty"`     // Total duration in milliseconds
	DurationAPIMs int                `json:"duration_api_ms,omitempty"` // API duration in milliseconds
	NumTurns      int                `json:"num_turns,omitempty"`       // Number of conversation turns
	TotalCostUSD  float64            `json:"total_cost_usd,omitempty"`  // Total cost in USD
	Usage         *StreamUsage       `json:"usage,omitempty"`           // Token usage breakdown
	ModelUsage    map[string]*ModelUsageEntry `json:"modelUsage,omitempty"` // Per-model usage breakdown (includes sub-agents)
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

// toolUseResultFile represents file info in Read tool results
type toolUseResultFile struct {
	FilePath   string `json:"filePath,omitempty"`
	NumLines   int    `json:"numLines,omitempty"`
	StartLine  int    `json:"startLine,omitempty"`
	TotalLines int    `json:"totalLines,omitempty"`
}

// parseStreamMessage parses a JSON line from Claude's stream-json output
// and returns zero or more ResponseChunks representing the message content.
func parseStreamMessage(line string, log *slog.Logger) []ResponseChunk {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	var msg streamMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		log.Warn("failed to parse stream message", "error", err, "line", truncateForLog(line))
		// Show user-friendly error requesting they report the issue
		return []ResponseChunk{{
			Type:    ChunkTypeText,
			Content: "\n[Plural bug: failed to parse Claude response. Please open an issue at https://github.com/zhubert/plural/issues with your /tmp/plural-debug.log]\n",
		}}
	}

	// If this looks like a stream-json message but we don't handle it, request a bug report
	if msg.Type == "" && strings.HasPrefix(line, "{") {
		log.Warn("unrecognized JSON message type", "line", truncateForLog(line))
		return []ResponseChunk{{
			Type:    ChunkTypeText,
			Content: "\n[Plural bug: unrecognized message format. Please open an issue at https://github.com/zhubert/plural/issues with your /tmp/plural-debug.log]\n",
		}}
	}

	var chunks []ResponseChunk

	switch msg.Type {
	case "system":
		// Init message - we could show "Session started" but skip for now
		if msg.Subtype == "init" {
			log.Debug("session initialized")
		}

	case "assistant":
		// Assistant messages can contain text or tool_use
		for _, content := range msg.Message.Content {
			switch content.Type {
			case "text":
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

// truncateString truncates a string to maxLen characters with "..." suffix.
func truncateString(s string, maxLen int) string {
	if maxLen > 0 && len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
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
func extractToolResultInfo(data *toolUseResultData) *ToolResultInfo {
	if data == nil {
		return nil
	}

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
