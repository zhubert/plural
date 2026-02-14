// Package claude provides the Claude CLI wrapper for managing conversations.
//
// The package is organized into focused modules:
//   - claude.go: Runner struct and core message handling
//   - runner_state.go: State structs (MCPChannels, StreamingState, TokenTracking, ResponseChannelState)
//   - parsing.go: Stream message parsing and tool input extraction
//   - mcp_config.go: MCP server configuration and socket management
//   - process_manager.go: Process lifecycle and auto-recovery
//   - runner_interface.go: Interfaces for testing
//   - mock_runner.go: Mock runner for testing and demos
//   - todo.go: TodoWrite tool parsing
//   - plugins.go: Plugin/marketplace management
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
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

// ContainerAllowedTools is a broad set of pre-authorized tools for containerized sessions.
// The container IS the sandbox, so all tools are safe to use without permission prompts.
var ContainerAllowedTools = []string{
	"Read",
	"Glob",
	"Grep",
	"Edit",
	"Write",
	"Bash",
	"ExitPlanMode",
	"WebFetch",
	"WebSearch",
	"TodoRead",
	"TodoWrite",
	"NotebookEdit",
	"Task",
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

	// MCP interactive prompt channels (grouped in sub-struct)
	mcp *MCPChannels

	stopOnce sync.Once // Ensures Stop() is idempotent
	stopped  bool      // Set to true when Stop() is called, prevents reading from closed channels

	// Fork support: when set, first CLI invocation uses --resume <parentID> --fork-session
	// to inherit the parent's conversation history while creating a new session
	forkFromSessionID string

	// Process management via ProcessManager
	processManager *ProcessManager // Manages Claude CLI process lifecycle

	// Response channel management (grouped in sub-struct)
	responseChan *ResponseChannelState

	// Per-session streaming state (grouped in sub-struct)
	streaming *StreamingState

	// Token tracking state (grouped in sub-struct)
	tokens *TokenTracking

	// External MCP servers to include in config
	mcpServers []MCPServer

	// Container mode: when true, skip MCP and run inside a container
	containerized  bool
	containerImage string

	// Supervisor mode: when true, MCP config includes --supervisor flag
	supervisor bool

	// Host tools mode: when true, expose create_pr and push_branch MCP tools
	// Only used for autonomous supervisor sessions running inside containers
	hostTools bool

	// Container ready callback: invoked when containerized session receives init message
	onContainerReady func()
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
	var streamLogFile *os.File
	if streamLogPath, err := logger.StreamLogPath(sessionID); err != nil {
		log.Warn("failed to get stream log path", "error", err)
	} else {
		streamLogFile, err = os.OpenFile(streamLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Warn("failed to open stream log file", "path", streamLogPath, "error", err)
		}
	}

	r := &Runner{
		sessionID:      sessionID,
		workingDir:     workingDir,
		messages:       msgs,
		sessionStarted: sessionStarted,
		allowedTools:   allowedTools,
		log:            log,
		streamLogFile:  streamLogFile,
		mcp:            NewMCPChannels(),
		streaming:      NewStreamingState(),
		tokens:         &TokenTracking{},
		responseChan:   NewResponseChannelState(),
	}

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

// SetForkFromSession sets the parent session ID to fork from.
// When set and the session hasn't started yet, the CLI will use
// --resume <parentID> --fork-session to inherit the parent's conversation history.
func (r *Runner) SetForkFromSession(parentSessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.forkFromSessionID = parentSessionID
	r.log.Debug("set fork from session", "parentSessionID", parentSessionID)
}

// SetContainerized configures the runner to run inside a container.
// When containerized, the MCP permission system is skipped entirely.
func (r *Runner) SetContainerized(containerized bool, image string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.containerized = containerized
	r.containerImage = image
	r.log.Debug("set containerized mode", "containerized", containerized, "image", image)
}

// SetOnContainerReady sets the callback to invoke when a containerized session is ready.
// This callback is called when the container initialization completes (init message received).
func (r *Runner) SetOnContainerReady(callback func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onContainerReady = callback
}

// PermissionRequestChan returns the channel for receiving permission requests.
// Returns nil if the runner has been stopped to prevent reading from closed channel.
func (r *Runner) PermissionRequestChan() <-chan mcp.PermissionRequest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.stopped || r.mcp == nil {
		return nil
	}
	return r.mcp.PermissionReq
}

// SendPermissionResponse sends a response to a permission request.
// Safe to call even if the runner has been stopped - will silently drop the response.
func (r *Runner) SendPermissionResponse(resp mcp.PermissionResponse) {
	r.mu.RLock()
	stopped := r.stopped
	var ch chan mcp.PermissionResponse
	if r.mcp != nil {
		ch = r.mcp.PermissionResp
	}
	r.mu.RUnlock()

	if stopped || ch == nil {
		r.log.Debug("SendPermissionResponse called on stopped runner, ignoring")
		return
	}

	// Use safeSendChannel to protect against send-on-closed-channel panic.
	// Between the RUnlock above and the send below, Stop() could close the channel.
	if !safeSendChannel(ch, resp) {
		r.log.Debug("SendPermissionResponse channel full or closed, ignoring")
	}
}

// QuestionRequestChan returns the channel for receiving question requests.
// Returns nil if the runner has been stopped to prevent reading from closed channel.
func (r *Runner) QuestionRequestChan() <-chan mcp.QuestionRequest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.stopped || r.mcp == nil {
		return nil
	}
	return r.mcp.QuestionReq
}

// SendQuestionResponse sends a response to a question request.
// Safe to call even if the runner has been stopped - will silently drop the response.
func (r *Runner) SendQuestionResponse(resp mcp.QuestionResponse) {
	r.mu.RLock()
	stopped := r.stopped
	var ch chan mcp.QuestionResponse
	if r.mcp != nil {
		ch = r.mcp.QuestionResp
	}
	r.mu.RUnlock()

	if stopped || ch == nil {
		r.log.Debug("SendQuestionResponse called on stopped runner, ignoring")
		return
	}

	// Use safeSendChannel to protect against send-on-closed-channel panic.
	if !safeSendChannel(ch, resp) {
		r.log.Debug("SendQuestionResponse channel full or closed, ignoring")
	}
}

// PlanApprovalRequestChan returns the channel for receiving plan approval requests.
// Returns nil if the runner has been stopped to prevent reading from closed channel.
func (r *Runner) PlanApprovalRequestChan() <-chan mcp.PlanApprovalRequest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.stopped || r.mcp == nil {
		return nil
	}
	return r.mcp.PlanReq
}

// SendPlanApprovalResponse sends a response to a plan approval request.
// Safe to call even if the runner has been stopped - will silently drop the response.
func (r *Runner) SendPlanApprovalResponse(resp mcp.PlanApprovalResponse) {
	r.mu.RLock()
	stopped := r.stopped
	var ch chan mcp.PlanApprovalResponse
	if r.mcp != nil {
		ch = r.mcp.PlanResp
	}
	r.mu.RUnlock()

	if stopped || ch == nil {
		r.log.Debug("SendPlanApprovalResponse called on stopped runner, ignoring")
		return
	}

	// Use safeSendChannel to protect against send-on-closed-channel panic.
	if !safeSendChannel(ch, resp) {
		r.log.Debug("SendPlanApprovalResponse channel full or closed, ignoring")
	}
}

// SetSupervisor enables or disables supervisor mode for this runner.
// When enabled, supervisor tool channels are initialized and the MCP config
// will include the --supervisor flag.
func (r *Runner) SetSupervisor(supervisor bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.supervisor = supervisor
	if supervisor && r.mcp != nil && r.mcp.CreateChildReq == nil {
		r.mcp.InitSupervisorChannels()
	}
}

// CreateChildRequestChan returns the channel for receiving create child requests.
func (r *Runner) CreateChildRequestChan() <-chan mcp.CreateChildRequest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.stopped || r.mcp == nil {
		return nil
	}
	return r.mcp.CreateChildReq
}

// SendCreateChildResponse sends a response to a create child request.
func (r *Runner) SendCreateChildResponse(resp mcp.CreateChildResponse) {
	r.mu.RLock()
	stopped := r.stopped
	var ch chan mcp.CreateChildResponse
	if r.mcp != nil {
		ch = r.mcp.CreateChildResp
	}
	r.mu.RUnlock()
	if stopped || ch == nil {
		return
	}
	if !safeSendChannel(ch, resp) {
		r.log.Debug("SendCreateChildResponse channel full or closed, ignoring")
	}
}

// ListChildrenRequestChan returns the channel for receiving list children requests.
func (r *Runner) ListChildrenRequestChan() <-chan mcp.ListChildrenRequest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.stopped || r.mcp == nil {
		return nil
	}
	return r.mcp.ListChildrenReq
}

// SendListChildrenResponse sends a response to a list children request.
func (r *Runner) SendListChildrenResponse(resp mcp.ListChildrenResponse) {
	r.mu.RLock()
	stopped := r.stopped
	var ch chan mcp.ListChildrenResponse
	if r.mcp != nil {
		ch = r.mcp.ListChildrenResp
	}
	r.mu.RUnlock()
	if stopped || ch == nil {
		return
	}
	if !safeSendChannel(ch, resp) {
		r.log.Debug("SendListChildrenResponse channel full or closed, ignoring")
	}
}

// MergeChildRequestChan returns the channel for receiving merge child requests.
func (r *Runner) MergeChildRequestChan() <-chan mcp.MergeChildRequest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.stopped || r.mcp == nil {
		return nil
	}
	return r.mcp.MergeChildReq
}

// SendMergeChildResponse sends a response to a merge child request.
func (r *Runner) SendMergeChildResponse(resp mcp.MergeChildResponse) {
	r.mu.RLock()
	stopped := r.stopped
	var ch chan mcp.MergeChildResponse
	if r.mcp != nil {
		ch = r.mcp.MergeChildResp
	}
	r.mu.RUnlock()
	if stopped || ch == nil {
		return
	}
	if !safeSendChannel(ch, resp) {
		r.log.Debug("SendMergeChildResponse channel full or closed, ignoring")
	}
}

// SetHostTools enables or disables host tools mode for this runner.
// When enabled, host tool channels are initialized and the MCP config
// will include the --host-tools flag.
func (r *Runner) SetHostTools(hostTools bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hostTools = hostTools
	if hostTools && r.mcp != nil && r.mcp.CreatePRReq == nil {
		r.mcp.InitHostToolChannels()
	}
}

// CreatePRRequestChan returns the channel for receiving create PR requests.
func (r *Runner) CreatePRRequestChan() <-chan mcp.CreatePRRequest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.stopped || r.mcp == nil {
		return nil
	}
	return r.mcp.CreatePRReq
}

// SendCreatePRResponse sends a response to a create PR request.
func (r *Runner) SendCreatePRResponse(resp mcp.CreatePRResponse) {
	r.mu.RLock()
	stopped := r.stopped
	var ch chan mcp.CreatePRResponse
	if r.mcp != nil {
		ch = r.mcp.CreatePRResp
	}
	r.mu.RUnlock()
	if stopped || ch == nil {
		return
	}
	if !safeSendChannel(ch, resp) {
		r.log.Debug("SendCreatePRResponse channel full or closed, ignoring")
	}
}

// PushBranchRequestChan returns the channel for receiving push branch requests.
func (r *Runner) PushBranchRequestChan() <-chan mcp.PushBranchRequest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.stopped || r.mcp == nil {
		return nil
	}
	return r.mcp.PushBranchReq
}

// SendPushBranchResponse sends a response to a push branch request.
func (r *Runner) SendPushBranchResponse(resp mcp.PushBranchResponse) {
	r.mu.RLock()
	stopped := r.stopped
	var ch chan mcp.PushBranchResponse
	if r.mcp != nil {
		ch = r.mcp.PushBranchResp
	}
	r.mu.RUnlock()
	if stopped || ch == nil {
		return
	}
	if !safeSendChannel(ch, resp) {
		r.log.Debug("SendPushBranchResponse channel full or closed, ignoring")
	}
}

// IsStreaming returns whether this runner is currently streaming a response
func (r *Runner) IsStreaming() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.streaming.Active
}

// GetResponseChan returns the current response channel (nil if not streaming)
func (r *Runner) GetResponseChan() <-chan ResponseChunk {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.responseChan.Channel
}

// ChunkType represents the type of streaming chunk
type ChunkType string

const (
	ChunkTypeText              ChunkType = "text"               // Regular text content
	ChunkTypeToolUse           ChunkType = "tool_use"           // Claude is calling a tool
	ChunkTypeToolResult        ChunkType = "tool_result"        // Tool execution result
	ChunkTypeTodoUpdate        ChunkType = "todo_update"        // TodoWrite tool call with todo list
	ChunkTypeStreamStats       ChunkType = "stream_stats"       // Streaming statistics from result message
	ChunkTypeSubagentStatus    ChunkType = "subagent_status"    // Subagent activity started or ended
	ChunkTypePermissionDenials ChunkType = "permission_denials" // Permission denials from result message
)

// StreamUsage represents token usage data from Claude's result message
type StreamUsage struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	OutputTokens             int `json:"output_tokens"`
}

// ModelTokenCount represents token usage for a specific model
type ModelTokenCount struct {
	Model        string // Model name (e.g., "claude-opus-4-5-20251101")
	OutputTokens int    // Output tokens for this model
}

// StreamStats represents streaming statistics for display in the UI
type StreamStats struct {
	OutputTokens        int               // Total output tokens generated (sum of all models)
	TotalCostUSD        float64           // Total cost in USD
	ByModel             []ModelTokenCount // Per-model breakdown (only populated from result message)
	DurationMs          int               // Total request duration in milliseconds (from result message)
	DurationAPIMs       int               // API-only duration in milliseconds (from result message)
	CacheCreationTokens int               // Tokens written to cache
	CacheReadTokens     int               // Tokens read from cache (cache hits)
	InputTokens         int               // Non-cached input tokens
}

// ToolResultInfo contains details about the result of a tool execution.
// This is extracted from the tool_use_result field in user messages.
type ToolResultInfo struct {
	// For Read tool results
	FilePath   string // Path to the file that was read
	NumLines   int    // Number of lines returned
	StartLine  int    // Starting line number (1-indexed)
	TotalLines int    // Total lines in the file

	// For Edit tool results
	Edited bool // Whether an edit was applied

	// For Glob tool results
	NumFiles int // Number of files matched

	// For Bash tool results
	ExitCode *int // Exit code (nil if not available)
}

// Summary returns a brief human-readable summary of the tool result.
func (t *ToolResultInfo) Summary() string {
	if t == nil {
		return ""
	}

	// Read tool: show line info
	if t.FilePath != "" && t.TotalLines > 0 {
		if t.NumLines < t.TotalLines {
			return fmt.Sprintf("lines %d-%d of %d", t.StartLine, t.StartLine+t.NumLines-1, t.TotalLines)
		}
		return fmt.Sprintf("%d lines", t.TotalLines)
	}

	// Edit tool: show edited status
	if t.Edited {
		return "applied"
	}

	// Glob tool: show file count
	if t.NumFiles > 0 {
		if t.NumFiles == 1 {
			return "1 file"
		}
		return fmt.Sprintf("%d files", t.NumFiles)
	}

	// Bash tool: show exit code
	if t.ExitCode != nil {
		if *t.ExitCode == 0 {
			return "success"
		}
		return fmt.Sprintf("exit %d", *t.ExitCode)
	}

	return ""
}

// ResponseChunk represents a chunk of streaming response
type ResponseChunk struct {
	Type              ChunkType          // Type of this chunk
	Content           string             // Text content (for text chunks and status)
	ToolName          string             // Tool being used (for tool_use chunks)
	ToolInput         string             // Brief description of tool input
	ToolUseID         string             // Unique ID for tool use (for matching tool_use to tool_result)
	ResultInfo        *ToolResultInfo    // Details about tool result (for tool_result chunks)
	TodoList          *TodoList          // Todo list (for ChunkTypeTodoUpdate)
	Stats             *StreamStats       // Streaming statistics (for ChunkTypeStreamStats)
	SubagentModel     string             // Model name when this is from a subagent (e.g., "claude-haiku-4-5-20251001")
	PermissionDenials []PermissionDenial // Permission denials (for ChunkTypePermissionDenials)
	Done              bool
	Error             error
}

// ModelUsageEntry represents usage statistics for a specific model in the result message.
// This includes both the parent model and any sub-agents (e.g., Haiku for Task agents).
type ModelUsageEntry struct {
	OutputTokens int `json:"outputTokens"`
}

// ensureProcessRunning starts the ProcessManager if not already running.
func (r *Runner) ensureProcessRunning() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// If there's already a running ProcessManager, nothing to do.
	if r.processManager != nil && r.processManager.IsRunning() {
		return nil
	}

	// Always create a fresh ProcessManager when one doesn't exist or isn't running.
	// After an interrupt or crash, the old ProcessManager's goroutines (readOutput,
	// drainStderr, monitorExit) may still be winding down. Reusing it would cause
	// race conditions between old and new goroutines competing for pipes and locks.
	config := ProcessConfig{
		SessionID:         r.sessionID,
		WorkingDir:        r.workingDir,
		SessionStarted:    r.sessionStarted,
		AllowedTools:      make([]string, len(r.allowedTools)),
		MCPConfigPath:     r.mcpConfigPath,
		ForkFromSessionID: r.forkFromSessionID,
		Containerized:     r.containerized,
		ContainerImage:    r.containerImage,
	}
	copy(config.AllowedTools, r.allowedTools)

	r.processManager = NewProcessManager(config, r.createProcessCallbacks(), r.log)

	err := r.processManager.Start()
	if err != nil && config.SessionStarted {
		// Resume failed (e.g., session was interrupted and can't be resumed).
		// Fall back to starting as a new session.
		r.log.Warn("resume failed, falling back to new session", "error", err)
		config.SessionStarted = false
		config.ForkFromSessionID = ""
		r.processManager = NewProcessManager(config, r.createProcessCallbacks(), r.log)
		return r.processManager.Start()
	}
	return err
}

// createProcessCallbacks creates the callbacks for ProcessManager events.
func (r *Runner) createProcessCallbacks() ProcessCallbacks {
	return ProcessCallbacks{
		OnLine:           r.handleProcessLine,
		OnProcessExit:    r.handleProcessExit,
		OnRestartAttempt: r.handleRestartAttempt,
		OnRestartFailed:  r.handleRestartFailed,
		OnFatalError:     r.handleFatalError,
		OnContainerReady: r.handleContainerReady,
	}
}

// handleProcessLine processes a line of output from the Claude process.
func (r *Runner) handleProcessLine(line string) {
	// Snapshot streamLogFile under the lock to avoid racing with Stop(),
	// which sets r.streamLogFile to nil after closing the file.
	r.mu.RLock()
	logFile := r.streamLogFile
	r.mu.RUnlock()

	// Write raw message to dedicated stream log file (pretty-printed JSON)
	if logFile != nil {
		var prettyJSON map[string]any
		if err := json.Unmarshal([]byte(line), &prettyJSON); err == nil {
			if formatted, err := json.MarshalIndent(prettyJSON, "", "  "); err == nil {
				fmt.Fprintf(logFile, "%s\n", formatted)
			} else {
				fmt.Fprintf(logFile, "%s\n", line)
			}
		} else {
			fmt.Fprintf(logFile, "%s\n", line)
		}
	}

	// Mark session as started as soon as we receive the init message.
	// This is the earliest signal that Claude CLI has accepted the session ID.
	// Without this, interrupting before a result message leaves sessionStarted=false,
	// causing subsequent starts to use --session-id (which fails with "already in use")
	// instead of --resume.
	if !r.sessionStarted && strings.Contains(line, `"type":"system"`) && strings.Contains(line, `"subtype":"init"`) {
		r.mu.Lock()
		r.sessionStarted = true
		pm := r.processManager
		r.mu.Unlock()
		// Call MarkSessionStarted outside r.mu to avoid deadlock:
		// MarkSessionStarted -> OnContainerReady -> handleContainerReady acquires r.mu.RLock
		if pm != nil {
			pm.MarkSessionStarted()
		}
		r.log.Info("session marked as started on init message")
	}

	// Parse the JSON message
	// hasStreamEvents is true because we always use --include-partial-messages,
	// which means text content arrives via stream_event deltas. The full assistant
	// message text should be skipped to avoid duplication.
	chunks := parseStreamMessage(line, true, r.log)

	// Get the current response channel (nil if already closed)
	r.mu.RLock()
	ch := r.responseChan.Channel
	if r.responseChan.Closed {
		ch = nil
	}
	r.mu.RUnlock()

	for _, chunk := range chunks {
		r.mu.Lock()
		switch chunk.Type {
		case ChunkTypeText:
			// Add extra newline after tool use for visual separation
			if r.streaming.LastWasToolUse && r.streaming.EndsWithNewline && !r.streaming.EndsWithDoubleNL {
				r.streaming.Response.WriteString("\n")
				r.streaming.EndsWithDoubleNL = true
			}
			r.streaming.Response.WriteString(chunk.Content)
			// Update newline tracking based on content
			if len(chunk.Content) > 0 {
				r.streaming.EndsWithNewline = chunk.Content[len(chunk.Content)-1] == '\n'
				r.streaming.EndsWithDoubleNL = len(chunk.Content) >= 2 && chunk.Content[len(chunk.Content)-2:] == "\n\n"
			}
			r.streaming.LastWasToolUse = false
		case ChunkTypeToolUse:
			// Format tool use line - add newline if needed
			if r.streaming.Response.Len() > 0 && !r.streaming.EndsWithNewline {
				r.streaming.Response.WriteString("\n")
			}
			r.streaming.Response.WriteString("● ")
			r.streaming.Response.WriteString(formatToolIcon(chunk.ToolName))
			r.streaming.Response.WriteString("(")
			r.streaming.Response.WriteString(chunk.ToolName)
			if chunk.ToolInput != "" {
				r.streaming.Response.WriteString(": ")
				r.streaming.Response.WriteString(chunk.ToolInput)
			}
			r.streaming.Response.WriteString(")\n")
			r.streaming.EndsWithNewline = true
			r.streaming.EndsWithDoubleNL = false
			r.streaming.LastWasToolUse = true
		}

		if r.streaming.FirstChunk {
			r.log.Debug("first response chunk received", "elapsed", time.Since(r.streaming.StartTime))
			r.streaming.FirstChunk = false
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

	// Parse the message to handle token accumulation, subagent tracking, and result messages
	var msg streamMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &msg); err == nil {
		// Handle token accumulation from stream_event messages (with --include-partial-messages)
		// These provide real-time token count updates during streaming
		if msg.Type == "stream_event" && msg.Event != nil {
			r.handleStreamEventTokens(msg.Event, ch)
		}

		// Handle subagent status tracking
		// When parent_tool_use_id is non-empty and we have a model, we're in a subagent (e.g., Haiku via Task)
		if msg.Type == "assistant" || msg.Type == "user" {
			r.mu.Lock()
			isSubagent := msg.ParentToolUseID != ""
			subagentModel := ""
			if isSubagent && msg.Message.Model != "" {
				subagentModel = msg.Message.Model
			}

			// Check for state change
			previousModel := r.streaming.CurrentSubagentModel
			stateChanged := (previousModel == "" && subagentModel != "") || // Entering subagent
				(previousModel != "" && subagentModel == "") // Exiting subagent

			if stateChanged {
				r.streaming.CurrentSubagentModel = subagentModel
				r.mu.Unlock()

				// Emit subagent status chunk
				if ch != nil {
					r.sendChunkWithTimeout(ch, ResponseChunk{
						Type:          ChunkTypeSubagentStatus,
						SubagentModel: subagentModel, // Empty string means subagent ended
					})
				}
			} else {
				r.mu.Unlock()
			}
		}

		// Handle token accumulation for assistant messages
		// Claude CLI sends cumulative output_tokens within each API call, but resets on new API calls.
		// We track message IDs to detect new API calls and accumulate across them.
		if msg.Type == "assistant" && msg.Message.Usage != nil && msg.Message.Usage.OutputTokens > 0 {
			r.mu.Lock()
			messageID := msg.Message.ID

			// If this is a new message ID, we're starting a new API call
			// Add the final token count from the previous API call to the accumulator
			if messageID != "" && messageID != r.tokens.LastMessageID {
				if r.tokens.LastMessageID != "" {
					// Add the previous message's final token count to the accumulator
					r.tokens.AccumulatedOutput += r.tokens.LastMessageTokens
				}
				r.tokens.LastMessageID = messageID
				r.tokens.LastMessageTokens = 0
			}

			// Update the current message's token count (this is cumulative within the API call)
			r.tokens.LastMessageTokens = msg.Message.Usage.OutputTokens

			// Update cache efficiency stats (these are cumulative values)
			r.tokens.CacheCreation = msg.Message.Usage.CacheCreationInputTokens
			r.tokens.CacheRead = msg.Message.Usage.CacheReadInputTokens
			r.tokens.Input = msg.Message.Usage.InputTokens

			// The displayed total is accumulated tokens from completed API calls
			// plus the current API call's running token count
			currentTotal := r.tokens.CurrentTotal()

			// Capture token values while still holding the lock to avoid race condition
			cacheCreation := r.tokens.CacheCreation
			cacheRead := r.tokens.CacheRead
			inputTokens := r.tokens.Input

			r.mu.Unlock()

			// Emit stream stats with the accumulated token count and cache stats
			if ch != nil {
				r.sendChunkWithTimeout(ch, ResponseChunk{
					Type: ChunkTypeStreamStats,
					Stats: &StreamStats{
						OutputTokens:        currentTotal,
						TotalCostUSD:        0, // Not available during streaming, only on result
						CacheCreationTokens: cacheCreation,
						CacheReadTokens:     cacheRead,
						InputTokens:         inputTokens,
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
			r.streaming.Complete = true // Mark that response finished - process exit after this is expected
			pm := r.processManager
			r.mu.Unlock()
			// Call MarkSessionStarted outside r.mu to avoid deadlock:
			// MarkSessionStarted -> OnContainerReady -> handleContainerReady acquires r.mu.RLock
			if pm != nil {
				pm.MarkSessionStarted()
				pm.ResetRestartAttempts()
			}
			r.mu.Lock()

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
				if ch != nil && !r.responseChan.Closed {
					errorMsg := fmt.Sprintf("\n[Error: %s]\n", errorText)
					r.streaming.Response.WriteString(errorMsg)
					select {
					case ch <- ResponseChunk{Type: ChunkTypeText, Content: errorMsg}:
					default:
					}
				}
			}

			// Emit permission denials if any were recorded during the session
			if len(msg.PermissionDenials) > 0 {
				r.log.Debug("permission denials in result",
					"count", len(msg.PermissionDenials))
				if ch != nil && !r.responseChan.Closed {
					select {
					case ch <- ResponseChunk{
						Type:              ChunkTypePermissionDenials,
						PermissionDenials: msg.PermissionDenials,
					}:
					default:
					}
				}
			}

			r.messages = append(r.messages, Message{Role: "assistant", Content: r.streaming.Response.String()})

			// Emit stream stats chunk before Done if we have usage data
			// Prefer modelUsage (which includes sub-agent tokens) over the streaming accumulator
			if ch != nil && !r.responseChan.Closed {
				var totalOutputTokens int
				var byModel []ModelTokenCount

				// If modelUsage is present, sum up output tokens from all models
				// This includes both the parent model and any sub-agents (e.g., Haiku for Task)
				if len(msg.ModelUsage) > 0 {
					for model, usage := range msg.ModelUsage {
						totalOutputTokens += usage.OutputTokens
						byModel = append(byModel, ModelTokenCount{
							Model:        model,
							OutputTokens: usage.OutputTokens,
						})
					}
					r.log.Debug("using modelUsage for token count",
						"modelCount", len(msg.ModelUsage),
						"totalOutputTokens", totalOutputTokens)
				} else if msg.Usage != nil {
					// Fall back to streaming accumulator if no modelUsage
					totalOutputTokens = r.tokens.AccumulatedOutput + r.tokens.LastMessageTokens
					if msg.Usage.OutputTokens > r.tokens.LastMessageTokens {
						totalOutputTokens = r.tokens.AccumulatedOutput + msg.Usage.OutputTokens
					}
					r.log.Debug("using streaming accumulator for token count",
						"accumulated", r.tokens.AccumulatedOutput,
						"lastMessage", r.tokens.LastMessageTokens,
						"totalOutputTokens", totalOutputTokens)
				}

				if totalOutputTokens > 0 || msg.TotalCostUSD > 0 || msg.DurationMs > 0 {
					// Get cache stats from result message (prefer result over streaming accumulator)
					var cacheCreation, cacheRead, inputTokens int
					if msg.Usage != nil {
						cacheCreation = msg.Usage.CacheCreationInputTokens
						cacheRead = msg.Usage.CacheReadInputTokens
						inputTokens = msg.Usage.InputTokens
					}

					stats := &StreamStats{
						OutputTokens:        totalOutputTokens,
						TotalCostUSD:        msg.TotalCostUSD,
						ByModel:             byModel,
						DurationMs:          msg.DurationMs,
						DurationAPIMs:       msg.DurationAPIMs,
						CacheCreationTokens: cacheCreation,
						CacheReadTokens:     cacheRead,
						InputTokens:         inputTokens,
					}
					r.log.Debug("emitting final stream stats",
						"outputTokens", stats.OutputTokens,
						"totalCostUSD", stats.TotalCostUSD,
						"modelCount", len(byModel),
						"durationMs", stats.DurationMs,
						"durationAPIMs", stats.DurationAPIMs,
						"cacheRead", cacheRead,
						"cacheCreation", cacheCreation)
					select {
					case ch <- ResponseChunk{Type: ChunkTypeStreamStats, Stats: stats}:
					default:
					}
				}
			}

			// Signal completion and close channel
			if ch != nil && !r.responseChan.Closed {
				select {
				case ch <- ResponseChunk{Done: true}:
				default:
				}
				r.closeResponseChannel()
			}
			r.streaming.Active = false

			// Reset for next message
			r.streaming.Reset()
			r.streaming.StartTime = time.Now()
			r.mu.Unlock()
		}
	}
}

// handleStreamEventTokens extracts and emits token counts from stream_event messages.
// These are sent when --include-partial-messages is enabled and provide real-time token updates.
func (r *Runner) handleStreamEventTokens(event *streamEvent, ch chan ResponseChunk) {
	if event == nil {
		return
	}

	var outputTokens int
	var messageID string

	switch event.Type {
	case "message_start":
		// Initial message with starting token count
		if event.Message != nil {
			messageID = event.Message.ID
			if event.Message.Usage != nil {
				outputTokens = event.Message.Usage.OutputTokens
			}
		}
	case "message_delta":
		// Updated token count during/after streaming
		if event.Usage != nil {
			outputTokens = event.Usage.OutputTokens
		}
	default:
		// Other event types don't have token updates
		return
	}

	if outputTokens == 0 {
		return
	}

	r.mu.Lock()

	// If this is a message_start with a new message ID, handle API call transitions
	if messageID != "" && messageID != r.tokens.LastMessageID {
		if r.tokens.LastMessageID != "" {
			// Add the previous message's final token count to the accumulator
			r.tokens.AccumulatedOutput += r.tokens.LastMessageTokens
		}
		r.tokens.LastMessageID = messageID
		r.tokens.LastMessageTokens = 0
	}

	// Update the current message's token count
	r.tokens.LastMessageTokens = outputTokens

	// Calculate total and check channel state under lock
	currentTotal := r.tokens.CurrentTotal()
	canSend := ch != nil && !r.responseChan.Closed

	// Release lock BEFORE sending to avoid holding it during the 10s timeout
	// in sendChunkWithTimeout, which would block all runner operations.
	r.mu.Unlock()

	if canSend {
		r.sendChunkWithTimeout(ch, ResponseChunk{
			Type: ChunkTypeStreamStats,
			Stats: &StreamStats{
				OutputTokens: currentTotal,
				TotalCostUSD: 0, // Not available during streaming
			},
		})
	}
}

// handleProcessExit is called when the process exits.
// Returns true if the process should be restarted.
func (r *Runner) handleProcessExit(err error, stderrContent string) bool {
	r.mu.Lock()
	stopped := r.stopped
	responseComplete := r.streaming.Complete

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

	// Don't close the response channel here — return true to allow the
	// ProcessManager to attempt a restart.  The channel must stay open so
	// that handleRestartAttempt can send status messages and, if all
	// retries fail, handleFatalError can send the final error+done chunk.
	// Closing the channel prematurely causes the Bubble Tea listener to
	// interpret the close as a successful completion, which triggers the
	// autonomous pipeline (auto-PR creation) on what was actually a crash.
	//
	// Mark streaming as inactive so no code path assumes we're still streaming.
	// handleFatalError also sets this, but we set it here for robustness in case
	// a restart succeeds (which resets streaming state via a new SendContent call).
	r.streaming.Active = false
	r.mu.Unlock()

	// Return true to allow ProcessManager to handle restart logic
	return true
}

// handleRestartAttempt is called when a restart is being attempted.
func (r *Runner) handleRestartAttempt(attemptNum int) {
	r.mu.Lock()
	ch := r.responseChan.Channel
	chClosed := r.responseChan.Closed
	r.mu.Unlock()

	if ch != nil && !chClosed {
		safeSendChannel(ch, ResponseChunk{
			Type:    ChunkTypeText,
			Content: fmt.Sprintf("\n[Process crashed, attempting restart %d/%d...]\n", attemptNum, MaxProcessRestartAttempts),
		})
	}
}

// handleRestartFailed is called when restart fails.
func (r *Runner) handleRestartFailed(err error) {
	r.log.Error("restart failed", "error", err)
}

// handleFatalError is called when max restarts exceeded or unrecoverable error.
func (r *Runner) handleFatalError(err error) {
	r.mu.Lock()
	ch := r.responseChan.Channel
	chClosed := r.responseChan.Closed

	if ch != nil && !chClosed {
		safeSendChannel(ch, ResponseChunk{Error: err, Done: true})
		r.closeResponseChannel()
	}
	r.streaming.Active = false
	r.mu.Unlock()
}

// handleContainerReady is called when a containerized session is ready (init message received).
func (r *Runner) handleContainerReady() {
	r.mu.RLock()
	callback := r.onContainerReady
	r.mu.RUnlock()

	if callback != nil {
		callback()
	}
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
	r.responseChan.Close()
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

		// Ensure MCP server is running (persistent across Send calls).
		// For containerized sessions, the socket server runs on the host and the
		// MCP config uses --auto-approve so regular permissions auto-approve while
		// AskUserQuestion and ExitPlanMode still route through the TUI.
		if err := r.ensureServerRunning(); err != nil {
			ch <- ResponseChunk{Error: err, Done: true}
			close(ch)
			return
		}

		// Set up the response channel for routing BEFORE starting the process.
		// This is critical because the process might crash immediately after starting,
		// and handleFatalError needs the channel to report the error to the user.
		r.mu.Lock()
		r.streaming.Active = true
		r.streaming.Ctx = cmdCtx
		r.streaming.StartTime = time.Now()
		r.streaming.Complete = false // Reset for new message - we haven't received result yet
		r.responseChan.Setup(ch)
		r.tokens.Reset() // Reset token accumulator for new request
		if r.processManager != nil {
			r.processManager.SetInterrupted(false) // Reset interrupt flag for new message
		}
		r.mu.Unlock()

		// Start process manager if not running
		if err := r.ensureProcessRunning(); err != nil {
			// Send error before closing channel
			ch <- ResponseChunk{Error: err, Done: true}

			// Clean up state using Close() to keep sync.Once consistent
			r.mu.Lock()
			r.streaming.Active = false
			r.closeResponseChannel()
			r.mu.Unlock()
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

		// Close socket server if running (runs on host for both container and non-container sessions)
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

		// Close MCP channels to unblock any waiting goroutines
		if r.mcp != nil {
			r.mcp.Close()
		}

		// Close stream log file
		if r.streamLogFile != nil {
			r.streamLogFile.Close()
			r.streamLogFile = nil
		}

		r.log.Info("runner stopped")
	})
}

// safeSendChannel attempts a non-blocking send on a channel, recovering from
// panics caused by sending on a closed channel. This is needed because between
// checking the stopped flag (under RLock) and actually sending (after RUnlock),
// Stop() could close the channel. Returns true if the send succeeded.
func safeSendChannel[T any](ch chan T, value T) (sent bool) {
	defer func() {
		if r := recover(); r != nil {
			sent = false
		}
	}()
	select {
	case ch <- value:
		return true
	default:
		return false
	}
}
