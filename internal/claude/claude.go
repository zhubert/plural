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
	// PermissionChannelBuffer is the buffer size for permission request/response channels
	PermissionChannelBuffer = 1

	// PermissionTimeout is the timeout for waiting for permission responses
	PermissionTimeout = 5 * time.Minute
)

// DefaultAllowedTools is the default set of permitted tools for new sessions.
// These are commonly used, relatively safe operations for development workflows.
var DefaultAllowedTools = []string{
	// Read-only operations
	"Read",
	"Glob",
	"Grep",
	"WebFetch",
	"WebSearch",
	// File modifications
	"Edit",
	"Write",
	"NotebookEdit",
	// Git (read and write operations)
	"Bash(git:*)",
	// Build tools and package managers
	"Bash(go:*)",
	"Bash(npm:*)",
	"Bash(npx:*)",
	"Bash(yarn:*)",
	"Bash(pnpm:*)",
	"Bash(make:*)",
	"Bash(cargo:*)",
	"Bash(rustc:*)",
	"Bash(python:*)",
	"Bash(pip:*)",
	"Bash(poetry:*)",
	"Bash(bundle:*)",
	"Bash(rake:*)",
	"Bash(mix:*)",
	// Read-only shell commands
	"Bash(ls:*)",
	"Bash(cat:*)",
	"Bash(head:*)",
	"Bash(tail:*)",
	"Bash(wc:*)",
	"Bash(find:*)",
	"Bash(tree:*)",
	"Bash(pwd:*)",
	"Bash(which:*)",
	"Bash(env:*)",
	"Bash(echo:*)",
	// Safe file operations
	"Bash(mkdir:*)",
	"Bash(cp:*)",
	"Bash(touch:*)",
}

// Message represents a chat message
type Message struct {
	Role    string // "user" or "assistant"
	Content string
}

// PermissionHandler is called when Claude needs permission for an operation
type PermissionHandler func(req mcp.PermissionRequest) mcp.PermissionResponse

// Runner manages a Claude Code CLI session
type Runner struct {
	sessionID         string
	workingDir        string
	messages          []Message
	sessionStarted    bool // tracks if session has been created
	mu                sync.RWMutex
	allowedTools      []string          // Pre-allowed tools for this session
	permissionHandler PermissionHandler // Callback for permission prompts
	socketServer      *mcp.SocketServer // Socket server for MCP communication (persistent)
	mcpConfigPath     string            // Path to MCP config file (persistent)
	serverRunning     bool              // Whether the socket server is running
	permReqChan       chan mcp.PermissionRequest
	permRespChan      chan mcp.PermissionResponse
	stopOnce          sync.Once // Ensures Stop() is idempotent

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

// SetPermissionHandler sets the callback for permission prompts
func (r *Runner) SetPermissionHandler(handler PermissionHandler) {
	r.permissionHandler = handler
}

// PermissionRequestChan returns the channel for receiving permission requests
func (r *Runner) PermissionRequestChan() <-chan mcp.PermissionRequest {
	return r.permReqChan
}

// SendPermissionResponse sends a response to a permission request
func (r *Runner) SendPermissionResponse(resp mcp.PermissionResponse) {
	r.permRespChan <- resp
}

// GetSessionID returns the session ID for this runner
func (r *Runner) GetSessionID() string {
	return r.sessionID
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

// SetStreamingDone marks the streaming as complete
func (r *Runner) SetStreamingDone() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.isStreaming = false
	r.responseChan = nil
}

// ResponseChunk represents a chunk of streaming response
type ResponseChunk struct {
	Content string
	Done    bool
	Error   error
}

// ensureServerRunning starts the socket server and creates MCP config if not already running.
// This makes the MCP server persistent across multiple Send() calls within a session.
func (r *Runner) ensureServerRunning() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.serverRunning {
		return nil
	}

	logger.Log("Claude: Starting persistent MCP server for session %s", r.sessionID)
	startTime := time.Now()

	// Create socket server
	socketServer, err := mcp.NewSocketServer(r.sessionID, r.permReqChan, r.permRespChan)
	if err != nil {
		logger.Log("Claude: Failed to create socket server: %v", err)
		return fmt.Errorf("failed to start permission server: %v", err)
	}
	r.socketServer = socketServer
	logger.Log("Claude: Socket server created in %v", time.Since(startTime))

	// Start socket server in background
	go r.socketServer.Run()

	// Create MCP config file
	mcpConfigPath, err := r.createMCPConfigLocked(r.socketServer.SocketPath())
	if err != nil {
		r.socketServer.Close()
		r.socketServer = nil
		logger.Log("Claude: Failed to create MCP config: %v", err)
		return fmt.Errorf("failed to create MCP config: %v", err)
	}
	r.mcpConfigPath = mcpConfigPath

	r.serverRunning = true
	logger.Log("Claude: Persistent MCP server started in %v, socket=%s, config=%s",
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

// Send sends a message to Claude and streams the response
func (r *Runner) Send(cmdCtx context.Context, prompt string) <-chan ResponseChunk {
	ch := make(chan ResponseChunk)

	// Track streaming state
	r.mu.Lock()
	r.isStreaming = true
	r.responseChan = ch
	r.streamCtx = cmdCtx
	r.mu.Unlock()

	go func() {
		defer close(ch)
		defer func() {
			r.mu.Lock()
			r.isStreaming = false
			r.responseChan = nil
			r.mu.Unlock()
		}()

		sendStartTime := time.Now()
		promptPreview := prompt
		if len(promptPreview) > 50 {
			promptPreview = promptPreview[:50] + "..."
		}
		logger.Log("Claude: Send started: sessionID=%s, prompt=%q", r.sessionID, promptPreview)

		// Add user message to history
		r.mu.Lock()
		r.messages = append(r.messages, Message{Role: "user", Content: prompt})
		r.mu.Unlock()

		// Ensure MCP server is running (persistent across Send calls)
		if err := r.ensureServerRunning(); err != nil {
			ch <- ResponseChunk{Error: err, Done: true}
			return
		}

		// Build the command
		// Use --session-id for first message, --resume for subsequent
		r.mu.Lock()
		sessionStarted := r.sessionStarted
		allowedTools := r.allowedTools
		mcpConfigPath := r.mcpConfigPath
		r.mu.Unlock()

		logger.Log("Claude: sessionStarted=%v for sessionID=%s", sessionStarted, r.sessionID)

		var args []string
		if sessionStarted {
			args = []string{
				"--print",
				"--resume", r.sessionID,
			}
		} else {
			args = []string{
				"--print",
				"--session-id", r.sessionID,
			}
		}

		// Add the prompt BEFORE multi-value flags like --mcp-config and --allowedTools
		// which consume all following arguments
		args = append(args, prompt)

		// Add MCP config and permission prompt tool
		args = append(args,
			"--mcp-config", mcpConfigPath,
			"--permission-prompt-tool", "mcp__plural__permission",
		)

		// Add pre-allowed tools if any
		for _, tool := range allowedTools {
			args = append(args, "--allowedTools", tool)
		}

		logger.Log("Claude: Running command: claude %s", strings.Join(args, " "))

		cmdStartTime := time.Now()
		cmd := exec.CommandContext(cmdCtx, "claude", args...)
		cmd.Dir = r.workingDir

		// Get stdout pipe for streaming
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			logger.Log("Claude: Failed to get stdout pipe: %v", err)
			ch <- ResponseChunk{Error: err, Done: true}
			return
		}

		stderr, err := cmd.StderrPipe()
		if err != nil {
			logger.Log("Claude: Failed to get stderr pipe: %v", err)
			ch <- ResponseChunk{Error: err, Done: true}
			return
		}

		if err := cmd.Start(); err != nil {
			logger.Log("Claude: Failed to start command: %v", err)
			ch <- ResponseChunk{Error: err, Done: true}
			return
		}
		logger.Log("Claude: Command started in %v, pid=%d", time.Since(cmdStartTime), cmd.Process.Pid)

		// Read and stream output
		var fullResponse string
		reader := bufio.NewReader(stdout)
		firstChunk := true
		for {
			line, err := reader.ReadString('\n')
			if len(line) > 0 {
				if firstChunk {
					logger.Log("Claude: First response chunk received after %v (time to first token)", time.Since(cmdStartTime))
					firstChunk = false
				}
				fullResponse += line
				ch <- ResponseChunk{Content: line, Done: false}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				logger.Log("Claude: Error reading stdout: %v", err)
				ch <- ResponseChunk{Error: err, Done: true}
				return
			}
		}

		// Read any stderr
		stderrBytes, stderrErr := io.ReadAll(stderr)
		if stderrErr != nil {
			logger.Log("Claude: Failed to read stderr: %v", stderrErr)
		}

		if err := cmd.Wait(); err != nil {
			errMsg := string(stderrBytes)
			logger.Log("Claude: Command failed: err=%v, stderr=%q", err, errMsg)
			if errMsg != "" {
				ch <- ResponseChunk{Error: fmt.Errorf("%s", errMsg), Done: true}
			} else {
				ch <- ResponseChunk{Error: err, Done: true}
			}
			return
		}

		// Mark session as started and add assistant message to history
		r.mu.Lock()
		r.sessionStarted = true
		r.messages = append(r.messages, Message{Role: "assistant", Content: fullResponse})
		r.mu.Unlock()

		totalDuration := time.Since(sendStartTime)
		cmdDuration := time.Since(cmdStartTime)
		logger.Log("Claude: Command completed successfully for sessionID=%s, cmd_duration=%v, total_duration=%v, response_len=%d",
			r.sessionID, cmdDuration, totalDuration, len(fullResponse))
		ch <- ResponseChunk{Done: true}
	}()

	return ch
}

// GetMessages returns a copy of the message history
func (r *Runner) GetMessages() []Message {
	r.mu.RLock()
	defer r.mu.RUnlock()

	messages := make([]Message, len(r.messages))
	copy(messages, r.messages)
	return messages
}

// ClearMessages clears the message history
func (r *Runner) ClearMessages() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = []Message{}
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
		r.mu.Lock()
		defer r.mu.Unlock()

		logger.Log("Claude: Stopping runner for session %s", r.sessionID)

		// Close socket server if running
		if r.socketServer != nil {
			logger.Log("Claude: Closing persistent socket server for session %s", r.sessionID)
			r.socketServer.Close()
			r.socketServer = nil
		}

		// Remove MCP config file
		if r.mcpConfigPath != "" {
			logger.Log("Claude: Removing MCP config file: %s", r.mcpConfigPath)
			os.Remove(r.mcpConfigPath)
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

		logger.Log("Claude: Runner stopped for session %s", r.sessionID)
	})
}
