package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/mcp"
)

// Claude runner constants
const (
	// PermissionChannelBuffer is the buffer size for permission request/response channels.
	// We use a buffer of 1 to allow the MCP server to send a request without blocking,
	// giving the user time to respond before the channel blocks on a second request.
	// A larger buffer would allow multiple permissions to queue up, which could confuse users.
	PermissionChannelBuffer = 1

	// PermissionTimeout is the timeout for waiting for permission responses.
	// 5 minutes allows users to read the prompt, check documentation, or switch tasks
	// without the request timing out. If this expires, the permission is denied.
	PermissionTimeout = 5 * time.Minute

	// MaxProcessRestartAttempts is the maximum number of times to try restarting
	// a crashed Claude process before giving up.
	MaxProcessRestartAttempts = 3

	// ProcessRestartDelay is the delay between restart attempts.
	ProcessRestartDelay = 500 * time.Millisecond

	// ResponseChannelFullTimeout is how long to wait when the response channel is full
	// before reporting an error (instead of silently dropping chunks).
	ResponseChannelFullTimeout = 10 * time.Second
)

// DefaultAllowedTools is the minimal set of safe tools allowed by default.
// Users can add more tools via global or per-repo config, or by pressing 'a' during sessions.
var DefaultAllowedTools = []string{
	// Read-only operations
	"Read",
	"Glob",
	"Grep",
	// File modifications (core editing workflow)
	"Edit",
	"Write",
	// Planning mode (safe - just signals plan completion for user review)
	"ExitPlanMode",
	// Safe read-only shell commands
	"Bash(ls:*)",
	"Bash(cat:*)",
	"Bash(head:*)",
	"Bash(tail:*)",
	"Bash(wc:*)",
	"Bash(pwd:*)",
}

// Message represents a chat message
type Message struct {
	Role    string // "user" or "assistant"
	Content string
}

// ContentType represents the type of content in a message block
type ContentType string

const (
	ContentTypeText  ContentType = "text"
	ContentTypeImage ContentType = "image"
)

// ContentBlock represents a single piece of content in a message
type ContentBlock struct {
	Type   ContentType  `json:"type"`
	Text   string       `json:"text,omitempty"`
	Source *ImageSource `json:"source,omitempty"`
}

// ImageSource represents an embedded image
type ImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/png", "image/jpeg", etc.
	Data      string `json:"data"`       // base64 encoded image data
}

// StreamInputMessage is the format sent to Claude CLI via stdin in stream-json mode
type StreamInputMessage struct {
	Type    string `json:"type"` // "user"
	Message struct {
		Role    string         `json:"role"`    // "user"
		Content []ContentBlock `json:"content"` // content blocks
	} `json:"message"`
}

// TextContent creates a text-only content block slice for convenience
func TextContent(text string) []ContentBlock {
	return []ContentBlock{{Type: ContentTypeText, Text: text}}
}

// GetDisplayContent returns the text representation of content blocks for display
func GetDisplayContent(blocks []ContentBlock) string {
	var parts []string
	for _, block := range blocks {
		switch block.Type {
		case ContentTypeText:
			parts = append(parts, block.Text)
		case ContentTypeImage:
			parts = append(parts, "[Image]")
		}
	}
	return strings.Join(parts, "\n")
}

// OptionsSystemPrompt is appended to Claude's system prompt to request structured option formatting.
// This allows Plural to reliably detect when Claude presents numbered choices to the user.
const OptionsSystemPrompt = `When presenting the user with numbered or lettered choices or options to choose from, wrap the options in <options> tags. For example:
<options>
1. First option
2. Second option
3. Third option
</options>
The opening and closing tags should be on their own lines, with the numbered options between them.

This also applies to letter-based options (A, B, C, etc.):
<options>
A. First approach
B. Second approach
C. Third approach
</options>

If you have multiple groups of options (e.g., high priority and low priority items), use <optgroup> tags within the <options> block:
<options>
<optgroup>
1. High priority option A
2. High priority option B
</optgroup>
<optgroup>
1. Lower priority option X
2. Lower priority option Y
</optgroup>
</options>`

// Runner manages a Claude Code CLI session.
//
// MCP Channel Architecture:
// The Runner uses pairs of channels to communicate with the MCP server for interactive
// prompts (permissions, questions, plan approvals). Each pair has a request channel
// (populated by the MCP server) and a response channel (populated by the TUI).
//
// Channel Flow:
//  1. MCP server receives permission/question/plan request from Claude
//  2. MCP server sends request to the appropriate reqChan
//  3. Runner reads from reqChan and displays prompt to user (via TUI)
//  4. User responds, TUI sends response to respChan
//  5. MCP server reads from respChan and returns result to Claude
//
// All channels have a buffer of PermissionChannelBuffer (1) to allow the MCP server
// to send a request without blocking, while still limiting how many can queue up.
// Only one request of each type can be pending at a time.
type Runner struct {
	sessionID      string
	workingDir     string
	messages       []Message
	sessionStarted bool // tracks if session has been created
	mu             sync.RWMutex
	allowedTools   []string          // Pre-allowed tools for this session
	socketServer   *mcp.SocketServer // Socket server for MCP communication (persistent)
	mcpConfigPath  string            // Path to MCP config file (persistent)
	serverRunning  bool              // Whether the socket server is running

	// Session-scoped logger with sessionID pre-attached
	log *slog.Logger

	// Stream log file for raw Claude messages (separate from main debug log)
	streamLogFile *os.File

	// MCP interactive prompt channels. Each pair handles one type of interaction:
	// - Permission: Tool use authorization (y/n/always)
	// - Question: Multiple-choice questions from Claude
	// - PlanApproval: Plan mode approval requests
	// See PermissionChannelBuffer constant for buffer size rationale.
	permReqChan  chan mcp.PermissionRequest
	permRespChan chan mcp.PermissionResponse
	questReqChan chan mcp.QuestionRequest
	questRespChan  chan mcp.QuestionResponse
	planReqChan    chan mcp.PlanApprovalRequest
	planRespChan   chan mcp.PlanApprovalResponse

	stopOnce sync.Once // Ensures Stop() is idempotent
	stopped  bool      // Set to true when Stop() is called, prevents reading from closed channels

	// Fork support: when set, first CLI invocation uses --resume <parentID> --fork-session
	// to inherit the parent's conversation history while creating a new session
	forkFromSessionID string

	// Process management via ProcessManager
	processManager          *ProcessManager    // Manages Claude CLI process lifecycle
	currentResponseCh       chan ResponseChunk // Current response channel for routing (protected by mu)
	currentResponseChClosed bool               // Whether currentResponseCh has been closed (protected by mu)
	closeResponseChOnce     *sync.Once         // Ensures response channel is closed exactly once per Send

	// Per-session streaming state (all protected by mu)
	isStreaming  bool               // Whether this runner is currently streaming
	streamCtx    context.Context    // Context for current streaming operation
	streamCancel context.CancelFunc // Cancel function for current streaming

	// Response building state (protected by mu)
	fullResponse          strings.Builder // Accumulates response content
	lastWasToolUse        bool            // Track if last chunk was tool use
	endsWithNewline       bool            // Track if response ends with \n
	endsWithDoubleNewline bool            // Track if response ends with \n\n
	firstChunk            bool            // Track if this is first chunk
	responseStartTime     time.Time       // When response started
	responseComplete      bool            // Track if result message was received (response is done)

	// Token tracking state (protected by mu)
	accumulatedOutputTokens int    // Accumulated output tokens from completed API calls
	lastMessageID           string // Track the message ID to detect new API calls
	lastMessageOutputTokens int    // Last seen output tokens for the current message ID

	// External MCP servers to include in config
	mcpServers []MCPServer
}

// MCPServer represents an external MCP server configuration
type MCPServer struct {
	Name    string
	Command string
	Args    []string
}


// New creates a new Claude runner for a session
func New(sessionID, workingDir string, sessionStarted bool, initialMessages []Message) *Runner {
	log := logger.WithSession(sessionID)
	log.Debug("runner created", "workDir", workingDir, "started", sessionStarted, "messageCount", len(initialMessages))

	msgs := initialMessages
	if msgs == nil {
		msgs = []Message{}
	}
	// Copy default allowed tools
	allowedTools := make([]string, len(DefaultAllowedTools))
	copy(allowedTools, DefaultAllowedTools)

	// Open stream log file for raw Claude messages
	streamLogPath := logger.StreamLogPath(sessionID)
	streamLogFile, err := os.OpenFile(streamLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Warn("failed to open stream log file", "path", streamLogPath, "error", err)
	}

	r := &Runner{
		sessionID:      sessionID,
		workingDir:     workingDir,
		messages:       msgs,
		sessionStarted: sessionStarted,
		allowedTools:   allowedTools,
		log:            log,
		streamLogFile:  streamLogFile,
		permReqChan:    make(chan mcp.PermissionRequest, PermissionChannelBuffer),
		permRespChan:   make(chan mcp.PermissionResponse, PermissionChannelBuffer),
		questReqChan:   make(chan mcp.QuestionRequest, PermissionChannelBuffer),
		questRespChan:  make(chan mcp.QuestionResponse, PermissionChannelBuffer),
		planReqChan:    make(chan mcp.PlanApprovalRequest, PermissionChannelBuffer),
		planRespChan:   make(chan mcp.PlanApprovalResponse, PermissionChannelBuffer),
		firstChunk:     true,
	}

	// Initialize response builder
	r.fullResponse.Grow(8192)

	// ProcessManager will be created lazily when first needed (after MCP config is ready)
	return r
}

// SessionStarted returns whether the session has been started
func (r *Runner) SessionStarted() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sessionStarted
}

// SetAllowedTools merges additional tools with the existing allowed tools list.
// This preserves defaults while adding any user-approved tools from config.
func (r *Runner) SetAllowedTools(tools []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, tool := range tools {
		found := false
		for _, existing := range r.allowedTools {
			if existing == tool {
				found = true
				break
			}
		}
		if !found {
			r.allowedTools = append(r.allowedTools, tool)
		}
	}
}

// AddAllowedTool adds a tool to the allowed list
func (r *Runner) AddAllowedTool(tool string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.allowedTools {
		if t == tool {
			return
		}
	}
	r.allowedTools = append(r.allowedTools, tool)
}

// SetMCPServers sets the external MCP servers to include in the config
func (r *Runner) SetMCPServers(servers []MCPServer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mcpServers = servers
	r.log.Debug("set external MCP servers", "count", len(servers))
}

// SetForkFromSession sets the parent session ID to fork from.
// When set and the session hasn't started yet, the CLI will use
// --resume <parentID> --fork-session to inherit the parent's conversation history.
func (r *Runner) SetForkFromSession(parentSessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.forkFromSessionID = parentSessionID
	r.log.Debug("set fork from session", "parentSessionID", parentSessionID)
}

// PermissionRequestChan returns the channel for receiving permission requests.
// Returns nil if the runner has been stopped to prevent reading from closed channel.
func (r *Runner) PermissionRequestChan() <-chan mcp.PermissionRequest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.stopped {
		return nil
	}
	return r.permReqChan
}

// SendPermissionResponse sends a response to a permission request.
// Safe to call even if the runner has been stopped - will silently drop the response.
func (r *Runner) SendPermissionResponse(resp mcp.PermissionResponse) {
	r.mu.RLock()
	stopped := r.stopped
	ch := r.permRespChan
	r.mu.RUnlock()

	if stopped || ch == nil {
		r.log.Debug("SendPermissionResponse called on stopped runner, ignoring")
		return
	}

	// Use non-blocking send to avoid deadlock if channel is closed between check and send
	select {
	case ch <- resp:
	default:
		r.log.Debug("SendPermissionResponse channel full or closed, ignoring")
	}
}

// QuestionRequestChan returns the channel for receiving question requests.
// Returns nil if the runner has been stopped to prevent reading from closed channel.
func (r *Runner) QuestionRequestChan() <-chan mcp.QuestionRequest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.stopped {
		return nil
	}
	return r.questReqChan
}

// SendQuestionResponse sends a response to a question request.
// Safe to call even if the runner has been stopped - will silently drop the response.
func (r *Runner) SendQuestionResponse(resp mcp.QuestionResponse) {
	r.mu.RLock()
	stopped := r.stopped
	ch := r.questRespChan
	r.mu.RUnlock()

	if stopped || ch == nil {
		r.log.Debug("SendQuestionResponse called on stopped runner, ignoring")
		return
	}

	// Use non-blocking send to avoid deadlock if channel is closed between check and send
	select {
	case ch <- resp:
	default:
		r.log.Debug("SendQuestionResponse channel full or closed, ignoring")
	}
}

// PlanApprovalRequestChan returns the channel for receiving plan approval requests.
// Returns nil if the runner has been stopped to prevent reading from closed channel.
func (r *Runner) PlanApprovalRequestChan() <-chan mcp.PlanApprovalRequest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.stopped {
		return nil
	}
	return r.planReqChan
}

// SendPlanApprovalResponse sends a response to a plan approval request.
// Safe to call even if the runner has been stopped - will silently drop the response.
func (r *Runner) SendPlanApprovalResponse(resp mcp.PlanApprovalResponse) {
	r.mu.RLock()
	stopped := r.stopped
	ch := r.planRespChan
	r.mu.RUnlock()

	if stopped || ch == nil {
		r.log.Debug("SendPlanApprovalResponse called on stopped runner, ignoring")
		return
	}

	// Use non-blocking send to avoid deadlock if channel is closed between check and send
	select {
	case ch <- resp:
	default:
		r.log.Debug("SendPlanApprovalResponse channel full or closed, ignoring")
	}
}

// IsStreaming returns whether this runner is currently streaming a response
func (r *Runner) IsStreaming() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.isStreaming
}

// GetResponseChan returns the current response channel (nil if not streaming)
func (r *Runner) GetResponseChan() <-chan ResponseChunk {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentResponseCh
}

// ChunkType represents the type of streaming chunk
type ChunkType string

const (
	ChunkTypeText        ChunkType = "text"         // Regular text content
	ChunkTypeToolUse     ChunkType = "tool_use"     // Claude is calling a tool
	ChunkTypeToolResult  ChunkType = "tool_result"  // Tool execution result
	ChunkTypeTodoUpdate  ChunkType = "todo_update"  // TodoWrite tool call with todo list
	ChunkTypeStreamStats ChunkType = "stream_stats" // Streaming statistics from result message
)

// StreamUsage represents token usage data from Claude's result message
type StreamUsage struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	OutputTokens             int `json:"output_tokens"`
}

// StreamStats represents streaming statistics for display in the UI
type StreamStats struct {
	OutputTokens int     // Output tokens generated
	TotalCostUSD float64 // Total cost in USD
}

// ResponseChunk represents a chunk of streaming response
type ResponseChunk struct {
	Type      ChunkType    // Type of this chunk
	Content   string       // Text content (for text chunks and status)
	ToolName  string       // Tool being used (for tool_use chunks)
	ToolInput string       // Brief description of tool input
	TodoList  *TodoList    // Todo list (for ChunkTypeTodoUpdate)
	Stats     *StreamStats // Streaming statistics (for ChunkTypeStreamStats)
	Done      bool
	Error     error
}

// streamMessage represents a JSON message from Claude's stream-json output
type streamMessage struct {
	Type    string `json:"type"`    // "system", "assistant", "user", "result"
	Subtype string `json:"subtype"` // "init", "success", etc.
	Message struct {
		ID      string `json:"id,omitempty"` // Message ID for tracking API calls
		Content []struct {
			Type      string          `json:"type"` // "text", "tool_use", "tool_result"
			Text      string          `json:"text,omitempty"`
			Name      string          `json:"name,omitempty"`       // tool name
			Input     json.RawMessage `json:"input,omitempty"`      // tool input
			ToolUseID string          `json:"tool_use_id,omitempty"`
			ToolUseId string          `json:"toolUseId,omitempty"` // camelCase variant from Claude CLI
			Content   json.RawMessage `json:"content,omitempty"`   // tool result content (can be string or array)
		} `json:"content"`
		Usage *StreamUsage `json:"usage,omitempty"` // Token usage (for assistant messages)
	} `json:"message"`
	Result       string       `json:"result,omitempty"`        // Final result text
	Error        string       `json:"error,omitempty"`         // Error message (alternative to result)
	Errors       []string     `json:"errors,omitempty"`        // Error messages array (used by error_during_execution)
	SessionID    string       `json:"session_id,omitempty"`
	DurationMs   int          `json:"duration_ms,omitempty"`   // Total duration in milliseconds
	DurationAPIMs int         `json:"duration_api_ms,omitempty"` // API duration in milliseconds
	NumTurns     int          `json:"num_turns,omitempty"`     // Number of conversation turns
	TotalCostUSD float64      `json:"total_cost_usd,omitempty"` // Total cost in USD
	Usage        *StreamUsage `json:"usage,omitempty"`         // Token usage breakdown
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
				})
				log.Debug("tool use", "tool", content.Name, "input", inputDesc)
			}
		}
		// Note: Stream stats are emitted by handleProcessLine with accumulated token counts,
		// not here, because parseStreamMessage is a pure function without runner state access.

	case "user":
		// User messages in stream-json are tool results
		// We don't display the content but we need to emit a ChunkTypeToolResult
		// so the UI can mark the tool use as complete. We check for both
		// "tool_result" type and the presence of toolUseId field (camelCase variant).
		for _, content := range msg.Message.Content {
			// Check for tool_result type or presence of tool use ID (indicates tool result)
			isToolResult := content.Type == "tool_result" ||
				content.ToolUseID != "" ||
				content.ToolUseId != ""
			if isToolResult {
				// Emit a tool result chunk so UI can mark tool as complete
				log.Debug("tool result received")
				chunks = append(chunks, ResponseChunk{
					Type: ChunkTypeToolResult,
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

// ensureServerRunning starts the socket server and creates MCP config if not already running.
// This makes the MCP server persistent across multiple Send() calls within a session.
func (r *Runner) ensureServerRunning() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.serverRunning {
		return nil
	}

	r.log.Info("starting persistent MCP server")
	startTime := time.Now()

	// Create socket server
	socketServer, err := mcp.NewSocketServer(r.sessionID, r.permReqChan, r.permRespChan, r.questReqChan, r.questRespChan, r.planReqChan, r.planRespChan)
	if err != nil {
		r.log.Error("failed to create socket server", "error", err)
		return fmt.Errorf("failed to start permission server: %v", err)
	}
	r.socketServer = socketServer
	r.log.Debug("socket server created", "elapsed", time.Since(startTime))

	// Start socket server in background
	go r.socketServer.Run()

	// Create MCP config file
	mcpConfigPath, err := r.createMCPConfigLocked(r.socketServer.SocketPath())
	if err != nil {
		r.socketServer.Close()
		r.socketServer = nil
		r.log.Error("failed to create MCP config", "error", err)
		return fmt.Errorf("failed to create MCP config: %v", err)
	}
	r.mcpConfigPath = mcpConfigPath

	r.serverRunning = true
	r.log.Info("persistent MCP server started",
		"elapsed", time.Since(startTime),
		"socket", r.socketServer.SocketPath(),
		"config", r.mcpConfigPath)

	return nil
}

// createMCPConfigLocked creates the MCP config file. Must be called with mu held.
func (r *Runner) createMCPConfigLocked(socketPath string) (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}

	// Start with the plural permission handler
	mcpServers := map[string]interface{}{
		"plural": map[string]interface{}{
			"command": execPath,
			"args":    []string{"mcp-server", "--socket", socketPath},
		},
	}

	// Add external MCP servers
	for _, server := range r.mcpServers {
		mcpServers[server.Name] = map[string]interface{}{
			"command": server.Command,
			"args":    server.Args,
		}
	}

	config := map[string]interface{}{
		"mcpServers": mcpServers,
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return "", err
	}

	configPath := filepath.Join(os.TempDir(), fmt.Sprintf("plural-mcp-%s.json", r.sessionID))
	if err := os.WriteFile(configPath, configJSON, 0644); err != nil {
		return "", err
	}

	return configPath, nil
}

// ensureProcessRunning starts the ProcessManager if not already running.
func (r *Runner) ensureProcessRunning() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Create ProcessManager if it doesn't exist
	if r.processManager == nil {
		config := ProcessConfig{
			SessionID:         r.sessionID,
			WorkingDir:        r.workingDir,
			SessionStarted:    r.sessionStarted,
			AllowedTools:      make([]string, len(r.allowedTools)),
			MCPConfigPath:     r.mcpConfigPath,
			ForkFromSessionID: r.forkFromSessionID,
		}
		copy(config.AllowedTools, r.allowedTools)

		r.processManager = NewProcessManager(config, r.createProcessCallbacks(), r.log)
	}

	// Start the process if not running
	if !r.processManager.IsRunning() {
		// Update config before starting (in case allowed tools changed)
		config := ProcessConfig{
			SessionID:         r.sessionID,
			WorkingDir:        r.workingDir,
			SessionStarted:    r.sessionStarted,
			AllowedTools:      make([]string, len(r.allowedTools)),
			MCPConfigPath:     r.mcpConfigPath,
			ForkFromSessionID: r.forkFromSessionID,
		}
		copy(config.AllowedTools, r.allowedTools)
		r.processManager.UpdateConfig(config)

		return r.processManager.Start()
	}

	return nil
}

// createProcessCallbacks creates the callbacks for ProcessManager events.
func (r *Runner) createProcessCallbacks() ProcessCallbacks {
	return ProcessCallbacks{
		OnLine:           r.handleProcessLine,
		OnProcessExit:    r.handleProcessExit,
		OnRestartAttempt: r.handleRestartAttempt,
		OnRestartFailed:  r.handleRestartFailed,
		OnFatalError:     r.handleFatalError,
	}
}

// handleProcessLine processes a line of output from the Claude process.
func (r *Runner) handleProcessLine(line string) {
	// Write raw message to dedicated stream log file (pretty-printed JSON)
	if r.streamLogFile != nil {
		var prettyJSON map[string]any
		if err := json.Unmarshal([]byte(line), &prettyJSON); err == nil {
			if formatted, err := json.MarshalIndent(prettyJSON, "", "  "); err == nil {
				fmt.Fprintf(r.streamLogFile, "%s\n", formatted)
			} else {
				fmt.Fprintf(r.streamLogFile, "%s\n", line)
			}
		} else {
			fmt.Fprintf(r.streamLogFile, "%s\n", line)
		}
	}

	// Parse the JSON message
	chunks := parseStreamMessage(line, r.log)

	// Get the current response channel (nil if already closed)
	r.mu.RLock()
	ch := r.currentResponseCh
	if r.currentResponseChClosed {
		ch = nil
	}
	r.mu.RUnlock()

	for _, chunk := range chunks {
		r.mu.Lock()
		switch chunk.Type {
		case ChunkTypeText:
			// Add extra newline after tool use for visual separation
			if r.lastWasToolUse && r.endsWithNewline && !r.endsWithDoubleNewline {
				r.fullResponse.WriteString("\n")
				r.endsWithDoubleNewline = true
			}
			r.fullResponse.WriteString(chunk.Content)
			// Update newline tracking based on content
			if len(chunk.Content) > 0 {
				r.endsWithNewline = chunk.Content[len(chunk.Content)-1] == '\n'
				r.endsWithDoubleNewline = len(chunk.Content) >= 2 && chunk.Content[len(chunk.Content)-2:] == "\n\n"
			}
			r.lastWasToolUse = false
		case ChunkTypeToolUse:
			// Format tool use line - add newline if needed
			if r.fullResponse.Len() > 0 && !r.endsWithNewline {
				r.fullResponse.WriteString("\n")
			}
			r.fullResponse.WriteString("● ")
			r.fullResponse.WriteString(formatToolIcon(chunk.ToolName))
			r.fullResponse.WriteString("(")
			r.fullResponse.WriteString(chunk.ToolName)
			if chunk.ToolInput != "" {
				r.fullResponse.WriteString(": ")
				r.fullResponse.WriteString(chunk.ToolInput)
			}
			r.fullResponse.WriteString(")\n")
			r.endsWithNewline = true
			r.endsWithDoubleNewline = false
			r.lastWasToolUse = true
		}

		if r.firstChunk {
			r.log.Debug("first response chunk received", "elapsed", time.Since(r.responseStartTime))
			r.firstChunk = false
		}
		r.mu.Unlock()

		// Send to response channel if available with timeout
		if ch != nil {
			if err := r.sendChunkWithTimeout(ch, chunk); err != nil {
				if err == errChannelFull {
					// Report error to user instead of silently dropping
					r.log.Error("response channel full, reporting error")
					r.sendChunkWithTimeout(ch, ResponseChunk{
						Type:    ChunkTypeText,
						Content: "\n[Error: Response buffer full - some output may be lost]\n",
					})
				}
				return
			}
		}
	}

	// Parse the message to handle token accumulation and result messages
	var msg streamMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &msg); err == nil {
		// Handle token accumulation for assistant messages
		// Claude CLI sends cumulative output_tokens within each API call, but resets on new API calls.
		// We track message IDs to detect new API calls and accumulate across them.
		if msg.Type == "assistant" && msg.Message.Usage != nil && msg.Message.Usage.OutputTokens > 0 {
			r.mu.Lock()
			messageID := msg.Message.ID

			// If this is a new message ID, we're starting a new API call
			// Add the final token count from the previous API call to the accumulator
			if messageID != "" && messageID != r.lastMessageID {
				if r.lastMessageID != "" {
					// Add the previous message's final token count to the accumulator
					r.accumulatedOutputTokens += r.lastMessageOutputTokens
				}
				r.lastMessageID = messageID
				r.lastMessageOutputTokens = 0
			}

			// Update the current message's token count (this is cumulative within the API call)
			r.lastMessageOutputTokens = msg.Message.Usage.OutputTokens

			// The displayed total is accumulated tokens from completed API calls
			// plus the current API call's running token count
			currentTotal := r.accumulatedOutputTokens + r.lastMessageOutputTokens

			r.mu.Unlock()

			// Emit stream stats with the accumulated token count
			if ch != nil {
				r.sendChunkWithTimeout(ch, ResponseChunk{
					Type: ChunkTypeStreamStats,
					Stats: &StreamStats{
						OutputTokens: currentTotal,
						TotalCostUSD: 0, // Not available during streaming, only on result
					},
				})
			}
		}

		if msg.Type == "result" {
			r.log.Debug("result message received",
				"subtype", msg.Subtype,
				"result", msg.Result,
				"error", msg.Error,
				"raw", strings.TrimSpace(line))

			r.mu.Lock()
			r.sessionStarted = true
			r.responseComplete = true // Mark that response finished - process exit after this is expected
			if r.processManager != nil {
				r.processManager.MarkSessionStarted()
				r.processManager.ResetRestartAttempts()
			}

			// Determine error message from Result, Error, or Errors fields
			errorText := msg.Result
			if errorText == "" {
				errorText = msg.Error
			}
			if errorText == "" && len(msg.Errors) > 0 {
				errorText = strings.Join(msg.Errors, "; ")
			}

			// If this is an error result, send the error message to the user
			// Check for various error subtypes that Claude CLI might use
			isError := msg.Subtype == "error_during_execution" ||
				msg.Subtype == "error" ||
				strings.Contains(msg.Subtype, "error")
			if isError && errorText != "" {
				if ch != nil && !r.currentResponseChClosed {
					errorMsg := fmt.Sprintf("\n[Error: %s]\n", errorText)
					r.fullResponse.WriteString(errorMsg)
					select {
					case ch <- ResponseChunk{Type: ChunkTypeText, Content: errorMsg}:
					default:
					}
				}
			}

			r.messages = append(r.messages, Message{Role: "assistant", Content: r.fullResponse.String()})

			// Emit stream stats chunk before Done if we have usage data
			// Use accumulated total for accurate token count across all API calls
			if ch != nil && !r.currentResponseChClosed && msg.Usage != nil {
				// Add the final token count from the last API call to get total
				totalOutputTokens := r.accumulatedOutputTokens + r.lastMessageOutputTokens
				// If the result message has its own usage, it represents the final state
				if msg.Usage.OutputTokens > r.lastMessageOutputTokens {
					totalOutputTokens = r.accumulatedOutputTokens + msg.Usage.OutputTokens
				}
				stats := &StreamStats{
					OutputTokens: totalOutputTokens,
					TotalCostUSD: msg.TotalCostUSD,
				}
				r.log.Debug("emitting final stream stats",
					"outputTokens", stats.OutputTokens,
					"accumulated", r.accumulatedOutputTokens,
					"lastMessage", r.lastMessageOutputTokens,
					"totalCostUSD", stats.TotalCostUSD)
				select {
				case ch <- ResponseChunk{Type: ChunkTypeStreamStats, Stats: stats}:
				default:
				}
			}

			// Signal completion and close channel
			if ch != nil && !r.currentResponseChClosed {
				select {
				case ch <- ResponseChunk{Done: true}:
				default:
				}
				r.closeResponseChannel()
			}
			r.isStreaming = false

			// Reset for next message
			r.fullResponse.Reset()
			r.fullResponse.Grow(8192)
			r.lastWasToolUse = false
			r.endsWithNewline = false
			r.endsWithDoubleNewline = false
			r.firstChunk = true
			r.responseStartTime = time.Now()
			r.mu.Unlock()
		}
	}
}

// handleProcessExit is called when the process exits.
// Returns true if the process should be restarted.
func (r *Runner) handleProcessExit(err error, stderrContent string) bool {
	r.mu.Lock()
	ch := r.currentResponseCh
	chClosed := r.currentResponseChClosed
	stopped := r.stopped
	responseComplete := r.responseComplete

	// If stopped, don't do anything
	if stopped {
		r.mu.Unlock()
		return false
	}

	// If response was already complete (we got a result message), the process
	// exiting is expected behavior - don't restart
	if responseComplete {
		r.log.Debug("process exited after response complete, not restarting")
		r.mu.Unlock()
		return false
	}

	// Mark streaming as done
	if ch != nil && !chClosed {
		select {
		case ch <- ResponseChunk{Done: true}:
		default:
		}
		r.closeResponseChannel()
	}
	r.isStreaming = false
	r.mu.Unlock()

	// Return true to allow ProcessManager to handle restart logic
	return true
}

// handleRestartAttempt is called when a restart is being attempted.
func (r *Runner) handleRestartAttempt(attemptNum int) {
	r.mu.Lock()
	ch := r.currentResponseCh
	chClosed := r.currentResponseChClosed
	r.mu.Unlock()

	if ch != nil && !chClosed {
		select {
		case ch <- ResponseChunk{
			Type:    ChunkTypeText,
			Content: fmt.Sprintf("\n[Process crashed, attempting restart %d/%d...]\n", attemptNum, MaxProcessRestartAttempts),
		}:
		default:
		}
	}
}

// handleRestartFailed is called when restart fails.
func (r *Runner) handleRestartFailed(err error) {
	r.log.Error("restart failed", "error", err)
}

// handleFatalError is called when max restarts exceeded or unrecoverable error.
func (r *Runner) handleFatalError(err error) {
	r.mu.Lock()
	ch := r.currentResponseCh
	chClosed := r.currentResponseChClosed

	if ch != nil && !chClosed {
		select {
		case ch <- ResponseChunk{Error: err, Done: true}:
		default:
		}
		r.closeResponseChannel()
	}
	r.isStreaming = false
	r.mu.Unlock()
}

// sendChunkWithTimeout sends a chunk to the response channel with timeout handling.
func (r *Runner) sendChunkWithTimeout(ch chan ResponseChunk, chunk ResponseChunk) error {
	select {
	case ch <- chunk:
		return nil
	case <-time.After(ResponseChannelFullTimeout):
		r.log.Error("response channel full after timeout", "timeout", ResponseChannelFullTimeout)
		return errChannelFull
	}
}

// closeResponseChannel safely closes the current response channel exactly once.
// Uses sync.Once to prevent double-close panics when multiple code paths
// (processResponse, handleProcessExit, handleFatalError) race to close the channel.
// The caller must hold r.mu when calling this method.
func (r *Runner) closeResponseChannel() {
	if r.closeResponseChOnce == nil || r.currentResponseCh == nil {
		return
	}
	r.closeResponseChOnce.Do(func() {
		close(r.currentResponseCh)
		r.currentResponseChClosed = true
	})
}

// Interrupt sends SIGINT to the Claude process to interrupt its current operation.
// This is used when the user presses Escape to stop a streaming response.
// Unlike Stop(), this doesn't terminate the process - it just interrupts the current task.
func (r *Runner) Interrupt() error {
	r.mu.Lock()
	pm := r.processManager
	r.mu.Unlock()

	if pm == nil {
		r.log.Debug("interrupt called but no process manager")
		return nil
	}

	// Set interrupted flag so handleProcessExit doesn't report an error
	pm.SetInterrupted(true)

	return pm.Interrupt()
}

// Send sends a message to Claude and streams the response
func (r *Runner) Send(cmdCtx context.Context, prompt string) <-chan ResponseChunk {
	return r.SendContent(cmdCtx, TextContent(prompt))
}

// SendContent sends structured content to Claude and streams the response
func (r *Runner) SendContent(cmdCtx context.Context, content []ContentBlock) <-chan ResponseChunk {
	ch := make(chan ResponseChunk, 100) // Buffered to avoid blocking response reader

	go func() {
		sendStartTime := time.Now()

		// Build display content for logging and history
		displayContent := GetDisplayContent(content)
		promptPreview := displayContent
		if len(promptPreview) > 50 {
			promptPreview = promptPreview[:50] + "..."
		}
		r.log.Debug("SendContent started", "content", promptPreview)

		// Add user message to history
		r.mu.Lock()
		r.messages = append(r.messages, Message{Role: "user", Content: displayContent})
		r.mu.Unlock()

		// Ensure MCP server is running (persistent across Send calls)
		if err := r.ensureServerRunning(); err != nil {
			ch <- ResponseChunk{Error: err, Done: true}
			close(ch)
			return
		}

		// Set up the response channel for routing BEFORE starting the process.
		// This is critical because the process might crash immediately after starting,
		// and handleFatalError needs the channel to report the error to the user.
		r.mu.Lock()
		r.isStreaming = true
		r.currentResponseCh = ch
		r.currentResponseChClosed = false    // Reset closed flag for new channel
		r.closeResponseChOnce = &sync.Once{} // Fresh sync.Once for this channel
		r.streamCtx = cmdCtx
		r.responseStartTime = time.Now()
		r.responseComplete = false // Reset for new message - we haven't received result yet
		r.accumulatedOutputTokens = 0   // Reset token accumulator for new request
		r.lastMessageID = ""            // Reset message ID tracker
		r.lastMessageOutputTokens = 0   // Reset last message token count
		if r.processManager != nil {
			r.processManager.SetInterrupted(false) // Reset interrupt flag for new message
		}
		r.mu.Unlock()

		// Start process manager if not running
		if err := r.ensureProcessRunning(); err != nil {
			// Clean up state since we're aborting
			r.mu.Lock()
			r.isStreaming = false
			r.currentResponseCh = nil
			r.currentResponseChClosed = true
			r.mu.Unlock()

			ch <- ResponseChunk{Error: err, Done: true}
			close(ch)
			return
		}

		// Build the input message
		inputMsg := StreamInputMessage{
			Type: "user",
		}
		inputMsg.Message.Role = "user"
		inputMsg.Message.Content = content

		// Serialize to JSON
		msgJSON, err := json.Marshal(inputMsg)
		if err != nil {
			r.log.Error("failed to serialize message", "error", err)
			ch <- ResponseChunk{Error: fmt.Errorf("failed to serialize message: %v", err), Done: true}
			close(ch)
			return
		}

		// Log message without base64 image data (which can be huge)
		hasImage := false
		for _, block := range content {
			if block.Type == ContentTypeImage {
				hasImage = true
				break
			}
		}
		if hasImage {
			r.log.Debug("writing message to stdin", "size", len(msgJSON), "hasImage", true)
		} else {
			r.log.Debug("writing message to stdin", "message", string(msgJSON))
		}

		// Write to process via ProcessManager
		r.mu.Lock()
		pm := r.processManager
		r.mu.Unlock()

		if pm == nil {
			ch <- ResponseChunk{Error: fmt.Errorf("process manager not available"), Done: true}
			close(ch)
			return
		}

		if err := pm.WriteMessage(append(msgJSON, '\n')); err != nil {
			r.log.Error("failed to write to stdin", "error", err)
			ch <- ResponseChunk{Error: err, Done: true}
			close(ch)
			return
		}

		r.log.Debug("message sent, waiting for response", "elapsed", time.Since(sendStartTime))

		// The response will be read by ProcessManager and routed via callbacks
	}()

	return ch
}

// GetMessages returns a copy of the message history.
// Thread-safe: takes a snapshot of messages under lock to prevent
// race conditions with concurrent appends from readPersistentResponses
// and SendContent goroutines.
func (r *Runner) GetMessages() []Message {
	r.mu.RLock()
	// Create a new slice with exact capacity to prevent any aliasing
	// issues during concurrent appends to the original slice
	msgLen := len(r.messages)
	messages := make([]Message, msgLen)
	copy(messages, r.messages)
	r.mu.RUnlock()
	return messages
}

// AddAssistantMessage adds an assistant message to the history
func (r *Runner) AddAssistantMessage(content string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, Message{Role: "assistant", Content: content})
}

// Stop cleanly stops the runner and releases resources.
// This method is idempotent - multiple calls are safe.
func (r *Runner) Stop() {
	r.stopOnce.Do(func() {
		r.log.Info("stopping runner")

		// Stop the ProcessManager first
		r.mu.Lock()
		pm := r.processManager
		r.mu.Unlock()

		if pm != nil {
			pm.Stop()
		}

		r.mu.Lock()
		defer r.mu.Unlock()

		// Mark as stopped BEFORE closing channels to prevent reads from closed channels
		// PermissionRequestChan() and QuestionRequestChan() check this flag
		r.stopped = true

		// Close socket server if running
		if r.socketServer != nil {
			r.log.Debug("closing persistent socket server")
			r.socketServer.Close()
			r.socketServer = nil
		}

		// Remove MCP config file and log any errors
		if r.mcpConfigPath != "" {
			r.log.Debug("removing MCP config file", "path", r.mcpConfigPath)
			if err := os.Remove(r.mcpConfigPath); err != nil && !os.IsNotExist(err) {
				r.log.Warn("failed to remove MCP config file", "path", r.mcpConfigPath, "error", err)
			}
			r.mcpConfigPath = ""
		}

		r.serverRunning = false

		// Close permission channels to unblock any waiting goroutines
		if r.permReqChan != nil {
			close(r.permReqChan)
			r.permReqChan = nil
		}
		if r.permRespChan != nil {
			close(r.permRespChan)
			r.permRespChan = nil
		}

		// Close question channels to unblock any waiting goroutines
		if r.questReqChan != nil {
			close(r.questReqChan)
			r.questReqChan = nil
		}
		if r.questRespChan != nil {
			close(r.questRespChan)
			r.questRespChan = nil
		}

		// Close plan approval channels to unblock any waiting goroutines
		if r.planReqChan != nil {
			close(r.planReqChan)
			r.planReqChan = nil
		}
		if r.planRespChan != nil {
			close(r.planRespChan)
			r.planRespChan = nil
		}

		// Close stream log file
		if r.streamLogFile != nil {
			r.streamLogFile.Close()
			r.streamLogFile = nil
		}

		r.log.Info("runner stopped")
	})
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
