package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/zhubert/plural/internal/logger"
)

const (
	ProtocolVersion = "2024-11-05"
	ServerName      = "plural-permission"
	ServerVersion   = "1.0.0"
	ToolName        = "permission"
)

// Server implements an MCP server for handling permission prompts
type Server struct {
	reader      *bufio.Reader
	writer      io.Writer
	requestChan chan<- PermissionRequest  // Send permission requests to TUI
	responseChan <-chan PermissionResponse // Receive responses from TUI
	allowedTools []string                  // Pre-allowed tools for this session
	mu           sync.Mutex
}

// NewServer creates a new MCP server
func NewServer(r io.Reader, w io.Writer, reqChan chan<- PermissionRequest, respChan <-chan PermissionResponse, allowedTools []string) *Server {
	return &Server{
		reader:       bufio.NewReader(r),
		writer:       w,
		requestChan:  reqChan,
		responseChan: respChan,
		allowedTools: allowedTools,
	}
}

// Run starts the MCP server loop
func (s *Server) Run() error {
	logger.Log("MCP: Server starting")

	for {
		line, err := s.reader.ReadString('\n')
		if err == io.EOF {
			logger.Log("MCP: EOF received, shutting down")
			return nil
		}
		if err != nil {
			logger.Log("MCP: Read error: %v", err)
			return err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		logger.Log("MCP: Received: %s", line)

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			logger.Log("MCP: JSON parse error: %v", err)
			s.sendError(nil, -32700, "Parse error", nil)
			continue
		}

		s.handleRequest(&req)
	}
}

func (s *Server) handleRequest(req *JSONRPCRequest) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "initialized":
		// Notification, no response needed
		logger.Log("MCP: Initialized notification received")
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	default:
		logger.Log("MCP: Unknown method: %s", req.Method)
		s.sendError(req.ID, -32601, "Method not found", nil)
	}
}

func (s *Server) handleInitialize(req *JSONRPCRequest) {
	result := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: Capability{
			Tools: &ToolCapability{},
		},
		ServerInfo: ServerInfo{
			Name:    ServerName,
			Version: ServerVersion,
		},
		Instructions: "This server handles permission prompts for Claude Code sessions.",
	}

	s.sendResult(req.ID, result)
}

func (s *Server) handleToolsList(req *JSONRPCRequest) {
	result := ToolsListResult{
		Tools: []ToolDefinition{
			{
				Name:        ToolName,
				Description: "Handle permission prompts for Claude Code operations",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"tool": {
							Type:        "string",
							Description: "The tool requesting permission (e.g., Edit, Bash, Read)",
						},
						"description": {
							Type:        "string",
							Description: "Human-readable description of the operation",
						},
						"arguments": {
							Type:        "object",
							Description: "The arguments to the tool",
						},
					},
					Required: []string{"tool", "description"},
				},
			},
		},
	}

	s.sendResult(req.ID, result)
}

func (s *Server) handleToolsCall(req *JSONRPCRequest) {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		logger.Log("MCP: Failed to parse tool call params: %v", err)
		s.sendError(req.ID, -32602, "Invalid params", nil)
		return
	}

	if params.Name != ToolName {
		logger.Log("MCP: Unknown tool: %s", params.Name)
		s.sendError(req.ID, -32602, "Unknown tool", nil)
		return
	}

	// Log the full arguments for debugging
	argsJSON, _ := json.Marshal(params.Arguments)
	logger.Log("MCP: Permission tool called with arguments: %s", string(argsJSON))

	// Extract permission request details from Claude Code's format
	var tool, description string
	var arguments map[string]interface{}

	// Claude Code sends: tool_name, input, tool_use_id
	if toolName, ok := params.Arguments["tool_name"].(string); ok {
		tool = toolName
	}

	// Get the input object for building description
	if input, ok := params.Arguments["input"].(map[string]interface{}); ok {
		arguments = input

		// Build description based on tool type
		switch tool {
		case "Edit":
			if filePath, ok := input["file_path"].(string); ok {
				description = "Edit file: " + filePath
			}
		case "Write":
			if filePath, ok := input["file_path"].(string); ok {
				description = "Write file: " + filePath
			}
		case "Read":
			if filePath, ok := input["file_path"].(string); ok {
				description = "Read file: " + filePath
			}
		case "Bash":
			if cmd, ok := input["command"].(string); ok {
				// Truncate long commands
				if len(cmd) > 80 {
					cmd = cmd[:77] + "..."
				}
				description = "Run: " + cmd
			}
		case "Glob", "Grep":
			if pattern, ok := input["pattern"].(string); ok {
				description = tool + ": " + pattern
			}
		default:
			// For unknown tools, try to create something useful
			if filePath, ok := input["file_path"].(string); ok {
				description = tool + ": " + filePath
			} else if cmd, ok := input["command"].(string); ok {
				description = tool + ": " + cmd
			}
		}
	}

	// Fallback if we couldn't build a description
	if description == "" {
		description = string(argsJSON)
		if len(description) > 100 {
			description = description[:97] + "..."
		}
	}

	// Fallback for tool name
	if tool == "" {
		tool = "Operation"
	}

	logger.Log("MCP: Permission request for tool=%s, desc=%s", tool, description)

	// Check if tool is pre-allowed
	if s.isToolAllowed(tool) {
		logger.Log("MCP: Tool %s is pre-allowed", tool)
		s.sendPermissionResult(req.ID, true, arguments, "")
		return
	}

	// Send request to TUI and wait for response
	permReq := PermissionRequest{
		ID:          req.ID,
		Tool:        tool,
		Description: description,
		Arguments:   arguments,
	}

	s.requestChan <- permReq
	logger.Log("MCP: Waiting for TUI response...")

	resp := <-s.responseChan
	logger.Log("MCP: Received TUI response: allowed=%v, always=%v", resp.Allowed, resp.Always)

	s.sendPermissionResult(req.ID, resp.Allowed, arguments, resp.Message)
}

func (s *Server) isToolAllowed(tool string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, allowed := range s.allowedTools {
		if allowed == tool {
			return true
		}
		// Handle pattern matching (e.g., "Bash(git:*)")
		if strings.HasPrefix(allowed, tool+"(") {
			return true
		}
	}
	return false
}

// AddAllowedTool adds a tool to the allowed list (called when user selects "Always")
func (s *Server) AddAllowedTool(tool string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, t := range s.allowedTools {
		if t == tool {
			return
		}
	}
	s.allowedTools = append(s.allowedTools, tool)
}

func (s *Server) sendPermissionResult(id interface{}, allowed bool, args map[string]interface{}, message string) {
	var result PermissionResult
	if allowed {
		result = PermissionResult{
			Behavior:     "allow",
			UpdatedInput: args,
		}
	} else {
		result = PermissionResult{
			Behavior: "deny",
			Message:  message,
		}
	}

	// Wrap result in tool call result format
	resultJSON, _ := json.Marshal(result)
	toolResult := ToolCallResult{
		Content: []ContentItem{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}

	s.sendResult(id, toolResult)
}

func (s *Server) sendResult(id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}

	s.send(resp)
}

func (s *Server) sendError(id interface{}, code int, message string, data interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}

	s.send(resp)
}

func (s *Server) send(resp JSONRPCResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		logger.Log("MCP: Failed to marshal response: %v", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err = fmt.Fprintf(s.writer, "%s\n", data)
	if err != nil {
		logger.Log("MCP: Failed to write response: %v", err)
	} else {
		logger.Log("MCP: Sent: %s", string(data))
	}
}
