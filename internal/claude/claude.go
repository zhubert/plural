package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
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
const OptionsSystemPrompt = `When presenting the user with numbered choices or options to choose from, wrap the options in <options> tags. For example:
<options>
1. First option
2. Second option
3. Third option
</options>
The opening and closing tags should be on their own lines, with the numbered options between them.

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

// Runner manages a Claude Code CLI session
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
	permReqChan    chan mcp.PermissionRequest
	permRespChan   chan mcp.PermissionResponse
	questReqChan   chan mcp.QuestionRequest
	questRespChan  chan mcp.QuestionResponse
	stopOnce       sync.Once // Ensures Stop() is idempotent
	stopped        bool      // Set to true when Stop() is called, prevents reading from closed channels

	// Persistent process management for stream-json input
	persistentCmd     *exec.Cmd        // The running Claude CLI process
	persistentStdin   io.WriteCloser   // Stdin pipe for sending messages
	persistentStdout  *bufio.Reader    // Stdout reader for responses
	persistentStderr  io.ReadCloser    // Stderr pipe for errors
	processRunning    bool             // Whether persistent process is running
	processMu         sync.Mutex       // Guards process lifecycle operations
	currentResponseCh chan ResponseChunk // Current response channel for routing

	// Per-session streaming state
	isStreaming  bool                   // Whether this runner is currently streaming
	responseChan <-chan ResponseChunk   // Current response channel
	streamCtx    context.Context        // Context for current streaming operation
	streamCancel context.CancelFunc     // Cancel function for current streaming

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
	logger.Log("Claude: New Runner created: sessionID=%s, workingDir=%s, started=%v, messages=%d", sessionID, workingDir, sessionStarted, len(initialMessages))
	msgs := initialMessages
	if msgs == nil {
		msgs = []Message{}
	}
	// Copy default allowed tools
	allowedTools := make([]string, len(DefaultAllowedTools))
	copy(allowedTools, DefaultAllowedTools)

	return &Runner{
		sessionID:      sessionID,
		workingDir:     workingDir,
		messages:       msgs,
		sessionStarted: sessionStarted,
		allowedTools:   allowedTools,
		permReqChan:    make(chan mcp.PermissionRequest, PermissionChannelBuffer),
		permRespChan:   make(chan mcp.PermissionResponse, PermissionChannelBuffer),
		questReqChan:   make(chan mcp.QuestionRequest, PermissionChannelBuffer),
		questRespChan:  make(chan mcp.QuestionResponse, PermissionChannelBuffer),
	}
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
	logger.Log("Claude: Set %d external MCP servers for session %s", len(servers), r.sessionID)
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
		logger.Log("Claude: SendPermissionResponse called on stopped runner, ignoring")
		return
	}

	// Use non-blocking send to avoid deadlock if channel is closed between check and send
	select {
	case ch <- resp:
	default:
		logger.Log("Claude: SendPermissionResponse channel full or closed, ignoring")
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
		logger.Log("Claude: SendQuestionResponse called on stopped runner, ignoring")
		return
	}

	// Use non-blocking send to avoid deadlock if channel is closed between check and send
	select {
	case ch <- resp:
	default:
		logger.Log("Claude: SendQuestionResponse channel full or closed, ignoring")
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
	return r.responseChan
}

// ChunkType represents the type of streaming chunk
type ChunkType string

const (
	ChunkTypeText       ChunkType = "text"        // Regular text content
	ChunkTypeToolUse    ChunkType = "tool_use"    // Claude is calling a tool
	ChunkTypeToolResult ChunkType = "tool_result" // Tool execution result
	ChunkTypeStatus     ChunkType = "status"      // Status message (init, result)
)

// ResponseChunk represents a chunk of streaming response
type ResponseChunk struct {
	Type      ChunkType // Type of this chunk
	Content   string    // Text content (for text chunks and status)
	ToolName  string    // Tool being used (for tool_use chunks)
	ToolInput string    // Brief description of tool input
	Done      bool
	Error     error
}

// streamMessage represents a JSON message from Claude's stream-json output
type streamMessage struct {
	Type    string `json:"type"`    // "system", "assistant", "user", "result"
	Subtype string `json:"subtype"` // "init", "success", etc.
	Message struct {
		Content []struct {
			Type      string          `json:"type"` // "text", "tool_use", "tool_result"
			Text      string          `json:"text,omitempty"`
			Name      string          `json:"name,omitempty"`       // tool name
			Input     json.RawMessage `json:"input,omitempty"`      // tool input
			ToolUseID string          `json:"tool_use_id,omitempty"`
			ToolUseId string          `json:"toolUseId,omitempty"` // camelCase variant from Claude CLI
			Content   json.RawMessage `json:"content,omitempty"`   // tool result content (can be string or array)
		} `json:"content"`
	} `json:"message"`
	Result    string `json:"result,omitempty"` // Final result text
	SessionID string `json:"session_id,omitempty"`
}

// parseStreamMessage parses a JSON line from Claude's stream-json output
// and returns zero or more ResponseChunks representing the message content.
func parseStreamMessage(line string) []ResponseChunk {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	var msg streamMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		// Log the raw JSON for debugging
		logger.Log("Claude: Failed to parse stream message: %v, line=%q", err, line)
		// Show user-friendly error requesting they report the issue
		return []ResponseChunk{{
			Type:    ChunkTypeText,
			Content: "\n[Plural bug: failed to parse Claude response. Please open an issue at https://github.com/zhubert/plural/issues with your /tmp/plural-debug.log]\n",
		}}
	}

	// If this looks like a stream-json message but we don't handle it, request a bug report
	if msg.Type == "" && strings.HasPrefix(line, "{") {
		logger.Log("Claude: Unrecognized JSON message type: %s", truncateForLog(line))
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
			logger.Log("Claude: Session initialized")
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
				// Extract a brief description from the tool input
				inputDesc := extractToolInputDescription(content.Name, content.Input)
				chunks = append(chunks, ResponseChunk{
					Type:      ChunkTypeToolUse,
					ToolName:  content.Name,
					ToolInput: inputDesc,
				})
				logger.Log("Claude: Tool use: %s - %s", content.Name, inputDesc)
			}
		}

	case "user":
		// User messages in stream-json are tool results
		// We silently skip these - they're internal to Claude's operation
		// and don't need to be displayed to users. We check for both
		// "tool_result" type and the presence of toolUseId field (camelCase variant).
		for _, content := range msg.Message.Content {
			// Check for tool_result type or presence of tool use ID (indicates tool result)
			isToolResult := content.Type == "tool_result" ||
				content.ToolUseID != "" ||
				content.ToolUseId != ""
			if isToolResult {
				// Log but don't display tool results - they're internal
				logger.Log("Claude: Tool result received (not displayed)")
			}
		}
		// Return empty - user messages (tool results) don't need to be displayed

	case "result":
		// Final result - the actual result text is in msg.Result
		// but we've already captured text chunks, so just log completion
		logger.Log("Claude: Result received, subtype=%s", msg.Subtype)
	}

	return chunks
}

// toolInputConfig defines how to extract a description from a tool's input.
type toolInputConfig struct {
	Field      string // JSON field to extract
	ShortenPath bool   // Whether to shorten file paths to just filename
	MaxLen     int    // Maximum length before truncation (0 = no limit)
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

	logger.Info("Claude: Starting persistent MCP server for session %s", r.sessionID)
	startTime := time.Now()

	// Create socket server
	socketServer, err := mcp.NewSocketServer(r.sessionID, r.permReqChan, r.permRespChan, r.questReqChan, r.questRespChan)
	if err != nil {
		logger.Error("Claude: Failed to create socket server: %v", err)
		return fmt.Errorf("failed to start permission server: %v", err)
	}
	r.socketServer = socketServer
	logger.Debug("Claude: Socket server created in %v", time.Since(startTime))

	// Start socket server in background
	go r.socketServer.Run()

	// Create MCP config file
	mcpConfigPath, err := r.createMCPConfigLocked(r.socketServer.SocketPath())
	if err != nil {
		r.socketServer.Close()
		r.socketServer = nil
		logger.Error("Claude: Failed to create MCP config: %v", err)
		return fmt.Errorf("failed to create MCP config: %v", err)
	}
	r.mcpConfigPath = mcpConfigPath

	r.serverRunning = true
	logger.Info("Claude: Persistent MCP server started in %v, socket=%s, config=%s",
		time.Since(startTime), r.socketServer.SocketPath(), r.mcpConfigPath)

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

// startPersistentProcess starts the Claude CLI process with stream-json input/output.
// This process stays running and receives messages via stdin.
func (r *Runner) startPersistentProcess() error {
	r.processMu.Lock()
	defer r.processMu.Unlock()

	if r.processRunning {
		return nil
	}

	logger.Info("Claude: Starting persistent process for session %s", r.sessionID)
	startTime := time.Now()

	// Build command arguments
	r.mu.RLock()
	sessionStarted := r.sessionStarted
	allowedTools := make([]string, len(r.allowedTools))
	copy(allowedTools, r.allowedTools)
	mcpConfigPath := r.mcpConfigPath
	r.mu.RUnlock()

	var args []string
	if sessionStarted {
		args = []string{
			"--print",
			"--output-format", "stream-json",
			"--input-format", "stream-json",
			"--verbose",
			"--resume", r.sessionID,
		}
	} else {
		args = []string{
			"--print",
			"--output-format", "stream-json",
			"--input-format", "stream-json",
			"--verbose",
			"--session-id", r.sessionID,
		}
	}

	// Add MCP config and permission prompt tool
	args = append(args,
		"--mcp-config", mcpConfigPath,
		"--permission-prompt-tool", "mcp__plural__permission",
		"--append-system-prompt", OptionsSystemPrompt,
	)

	// Add pre-allowed tools
	for _, tool := range allowedTools {
		args = append(args, "--allowedTools", tool)
	}

	logger.Log("Claude: Starting persistent process: claude %s", strings.Join(args, " "))

	cmd := exec.Command("claude", args...)
	cmd.Dir = r.workingDir

	// Get stdin pipe for writing messages
	stdin, err := cmd.StdinPipe()
	if err != nil {
		logger.Error("Claude: Failed to get stdin pipe: %v", err)
		return fmt.Errorf("failed to get stdin pipe: %v", err)
	}

	// Get stdout pipe for reading responses
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		logger.Error("Claude: Failed to get stdout pipe: %v", err)
		return fmt.Errorf("failed to get stdout pipe: %v", err)
	}

	// Get stderr pipe for error messages
	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		logger.Error("Claude: Failed to get stderr pipe: %v", err)
		return fmt.Errorf("failed to get stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		stderr.Close()
		logger.Error("Claude: Failed to start persistent process: %v", err)
		return fmt.Errorf("failed to start process: %v", err)
	}

	r.persistentCmd = cmd
	r.persistentStdin = stdin
	r.persistentStdout = bufio.NewReader(stdout)
	r.persistentStderr = stderr
	r.processRunning = true

	logger.Info("Claude: Persistent process started in %v, pid=%d", time.Since(startTime), cmd.Process.Pid)

	// Start goroutine to read responses
	go r.readPersistentResponses()

	// Start goroutine to monitor for process exit
	go r.monitorProcessExit()

	return nil
}

// readPersistentResponses continuously reads from stdout and routes to the current response channel
func (r *Runner) readPersistentResponses() {
	logger.Log("Claude: Response reader started for session %s", r.sessionID)

	var fullResponse strings.Builder
	fullResponse.Grow(8192) // Pre-allocate for typical response size
	var lastWasToolUse bool
	endsWithNewline := false     // Track if response ends with \n
	endsWithDoubleNewline := false // Track if response ends with \n\n
	firstChunk := true
	responseStartTime := time.Now()

	for {
		r.processMu.Lock()
		running := r.processRunning
		reader := r.persistentStdout
		r.processMu.Unlock()

		if !running || reader == nil {
			logger.Log("Claude: Response reader exiting - process not running")
			return
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				logger.Log("Claude: EOF on stdout - process exited")
			} else {
				logger.Log("Claude: Error reading stdout: %v", err)
			}
			r.handleProcessExit(err)
			return
		}

		if len(line) == 0 {
			continue
		}

		if firstChunk {
			logger.Log("Claude: First response chunk received after %v", time.Since(responseStartTime))
			firstChunk = false
		}

		// Parse the JSON message
		chunks := parseStreamMessage(line)

		// Get the current response channel
		r.mu.RLock()
		ch := r.currentResponseCh
		r.mu.RUnlock()

		for _, chunk := range chunks {
			switch chunk.Type {
			case ChunkTypeText:
				// Add extra newline after tool use for visual separation
				if lastWasToolUse && endsWithNewline && !endsWithDoubleNewline {
					fullResponse.WriteString("\n")
					endsWithDoubleNewline = true
				}
				fullResponse.WriteString(chunk.Content)
				// Update newline tracking based on content
				if len(chunk.Content) > 0 {
					endsWithNewline = chunk.Content[len(chunk.Content)-1] == '\n'
					endsWithDoubleNewline = len(chunk.Content) >= 2 && chunk.Content[len(chunk.Content)-2:] == "\n\n"
				}
				lastWasToolUse = false
			case ChunkTypeToolUse:
				// Format tool use line - add newline if needed
				if fullResponse.Len() > 0 && !endsWithNewline {
					fullResponse.WriteString("\n")
				}
				fullResponse.WriteString("● ")
				fullResponse.WriteString(formatToolIcon(chunk.ToolName))
				fullResponse.WriteString("(")
				fullResponse.WriteString(chunk.ToolName)
				if chunk.ToolInput != "" {
					fullResponse.WriteString(": ")
					fullResponse.WriteString(chunk.ToolInput)
				}
				fullResponse.WriteString(")\n")
				endsWithNewline = true
				endsWithDoubleNewline = false
				lastWasToolUse = true
			}

			// Send to response channel if available with timeout
			// Uses a timeout to handle slow consumers without blocking forever
			if ch != nil {
				select {
				case ch <- chunk:
					// Sent successfully
				case <-time.After(5 * time.Second):
					logger.Log("Claude: Response channel full after 5s timeout, chunk may be lost")
				}
			}
		}

		// Check for result message which indicates end of response
		var msg streamMessage
		if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &msg); err == nil {
			if msg.Type == "result" {
				logger.Log("Claude: Result message received, response complete")

				// Add assistant message to history
				r.mu.Lock()
				r.sessionStarted = true
				r.messages = append(r.messages, Message{Role: "assistant", Content: fullResponse.String()})
				r.mu.Unlock()

				// Signal completion and close channel so listeners detect end
				// This fixes a race condition where GetResponseChan() could return nil
				// while Bubble Tea is still processing earlier chunks from the buffer
				if ch != nil {
					ch <- ResponseChunk{Done: true}
					close(ch)
				}

				// Reset streaming state for next message
				// Note: Don't set responseChan to nil here - GetResponseChan() should
				// return the closed channel so listeners can detect completion.
				// It will be replaced on the next SendContent call.
				r.mu.Lock()
				r.currentResponseCh = nil
				r.isStreaming = false
				r.mu.Unlock()

				// Reset for next message
				fullResponse.Reset()
				fullResponse.Grow(8192) // Pre-allocate for next response
				lastWasToolUse = false
				endsWithNewline = false
				endsWithDoubleNewline = false
				firstChunk = true
				responseStartTime = time.Now()
			}
		}
	}
}

// monitorProcessExit waits for the process to exit and handles cleanup
func (r *Runner) monitorProcessExit() {
	r.processMu.Lock()
	cmd := r.persistentCmd
	r.processMu.Unlock()

	if cmd == nil {
		return
	}

	err := cmd.Wait()
	logger.Log("Claude: Persistent process exited: %v", err)
	r.handleProcessExit(err)
}

// handleProcessExit handles cleanup when the persistent process exits
func (r *Runner) handleProcessExit(err error) {
	r.processMu.Lock()
	defer r.processMu.Unlock()

	if !r.processRunning {
		return
	}

	logger.Log("Claude: Handling process exit for session %s", r.sessionID)

	// Notify current response channel of error and close it
	// This fixes a race condition where GetResponseChan() could return nil
	// while Bubble Tea is still processing earlier chunks from the buffer
	r.mu.Lock()
	ch := r.currentResponseCh
	if ch != nil {
		ch <- ResponseChunk{Error: fmt.Errorf("process exited: %v", err), Done: true}
		close(ch)
		r.currentResponseCh = nil
	}
	r.isStreaming = false
	// Note: Don't set responseChan to nil - let GetResponseChan() return
	// the closed channel so listeners can detect completion
	r.mu.Unlock()

	// Clean up pipes
	if r.persistentStdin != nil {
		r.persistentStdin.Close()
		r.persistentStdin = nil
	}
	if r.persistentStderr != nil {
		r.persistentStderr.Close()
		r.persistentStderr = nil
	}

	r.persistentCmd = nil
	r.persistentStdout = nil
	r.processRunning = false
}

// stopPersistentProcess stops the persistent Claude CLI process
func (r *Runner) stopPersistentProcess() {
	r.processMu.Lock()
	defer r.processMu.Unlock()

	if !r.processRunning {
		return
	}

	logger.Log("Claude: Stopping persistent process for session %s", r.sessionID)

	// Close stdin to signal EOF to the process
	if r.persistentStdin != nil {
		r.persistentStdin.Close()
		r.persistentStdin = nil
	}

	// Kill the process if it doesn't exit gracefully
	if r.persistentCmd != nil && r.persistentCmd.Process != nil {
		// Give it a moment to exit gracefully, then force kill
		done := make(chan struct{})
		go func() {
			r.persistentCmd.Wait()
			close(done)
		}()

		select {
		case <-done:
			logger.Log("Claude: Process exited gracefully")
		case <-time.After(2 * time.Second):
			logger.Log("Claude: Force killing process")
			r.persistentCmd.Process.Kill()
		}
	}

	if r.persistentStderr != nil {
		r.persistentStderr.Close()
		r.persistentStderr = nil
	}

	r.persistentCmd = nil
	r.persistentStdout = nil
	r.processRunning = false
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
		logger.Log("Claude: SendContent started: sessionID=%s, content=%q", r.sessionID, promptPreview)

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

		// Start persistent process if not running
		if err := r.startPersistentProcess(); err != nil {
			ch <- ResponseChunk{Error: err, Done: true}
			close(ch)
			return
		}

		// Set up the response channel for routing
		r.mu.Lock()
		r.isStreaming = true
		r.responseChan = ch
		r.currentResponseCh = ch
		r.streamCtx = cmdCtx
		r.mu.Unlock()

		// Build the input message
		inputMsg := StreamInputMessage{
			Type: "user",
		}
		inputMsg.Message.Role = "user"
		inputMsg.Message.Content = content

		// Serialize to JSON
		msgJSON, err := json.Marshal(inputMsg)
		if err != nil {
			logger.Log("Claude: Failed to serialize message: %v", err)
			ch <- ResponseChunk{Error: fmt.Errorf("failed to serialize message: %v", err), Done: true}
			close(ch)
			return
		}

		// Write to stdin
		r.processMu.Lock()
		stdin := r.persistentStdin
		r.processMu.Unlock()

		if stdin == nil {
			ch <- ResponseChunk{Error: fmt.Errorf("process stdin not available"), Done: true}
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
			logger.Log("Claude: Writing message to stdin: [message with image, %d bytes]", len(msgJSON))
		} else {
			logger.Log("Claude: Writing message to stdin: %s", string(msgJSON))
		}
		if _, err := stdin.Write(append(msgJSON, '\n')); err != nil {
			logger.Log("Claude: Failed to write to stdin: %v", err)
			ch <- ResponseChunk{Error: fmt.Errorf("failed to write to process: %v", err), Done: true}
			close(ch)
			return
		}

		logger.Log("Claude: Message sent in %v, waiting for response", time.Since(sendStartTime))

		// The response will be read by readPersistentResponses goroutine
		// and routed to this channel.
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
		logger.Info("Claude: Stopping runner for session %s", r.sessionID)

		// Stop the persistent Claude CLI process first (needs its own lock)
		r.stopPersistentProcess()

		r.mu.Lock()
		defer r.mu.Unlock()

		// Mark as stopped BEFORE closing channels to prevent reads from closed channels
		// PermissionRequestChan() and QuestionRequestChan() check this flag
		r.stopped = true

		// Close socket server if running
		if r.socketServer != nil {
			logger.Debug("Claude: Closing persistent socket server for session %s", r.sessionID)
			r.socketServer.Close()
			r.socketServer = nil
		}

		// Remove MCP config file and log any errors
		if r.mcpConfigPath != "" {
			logger.Debug("Claude: Removing MCP config file: %s", r.mcpConfigPath)
			if err := os.Remove(r.mcpConfigPath); err != nil && !os.IsNotExist(err) {
				logger.Warn("Claude: Failed to remove MCP config file %s: %v", r.mcpConfigPath, err)
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

		logger.Info("Claude: Runner stopped for session %s", r.sessionID)
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
	case "TodoWrite":
		return "Planning"
	default:
		return "Using"
	}
}
