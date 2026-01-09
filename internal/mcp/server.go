package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
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
	reader       *bufio.Reader
	writer       io.Writer
	requestChan  chan<- PermissionRequest  // Send permission requests to TUI
	responseChan <-chan PermissionResponse // Receive responses from TUI
	questionChan chan<- QuestionRequest    // Send question requests to TUI
	answerChan   <-chan QuestionResponse   // Receive answers from TUI
	allowedTools []string                  // Pre-allowed tools for this session
	mu           sync.Mutex
}

// NewServer creates a new MCP server
func NewServer(r io.Reader, w io.Writer, reqChan chan<- PermissionRequest, respChan <-chan PermissionResponse, questionChan chan<- QuestionRequest, answerChan <-chan QuestionResponse, allowedTools []string) *Server {
	return &Server{
		reader:       bufio.NewReader(r),
		writer:       w,
		requestChan:  reqChan,
		responseChan: respChan,
		questionChan: questionChan,
		answerChan:   answerChan,
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
		description = buildToolDescription(tool, input)
	}

	// Fallback if we couldn't build a description
	if description == "" {
		description = formatInputForDisplay(params.Arguments)
	}

	// Fallback for tool name
	if tool == "" {
		tool = "Operation"
	}

	logger.Log("MCP: Permission request for tool=%s, desc=%s", tool, description)

	// Special handling for AskUserQuestion
	if tool == "AskUserQuestion" {
		s.handleAskUserQuestion(req.ID, arguments)
		return
	}

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

	// If user selected "always allow", remember this tool for future requests
	if resp.Always {
		s.addAllowedTool(tool)
	}

	s.sendPermissionResult(req.ID, resp.Allowed, arguments, resp.Message)
}

// handleAskUserQuestion handles the AskUserQuestion tool specially
func (s *Server) handleAskUserQuestion(reqID interface{}, arguments map[string]interface{}) {
	logger.Log("MCP: Handling AskUserQuestion")

	// Parse questions from arguments
	questionsRaw, ok := arguments["questions"]
	if !ok {
		logger.Log("MCP: AskUserQuestion missing 'questions' field")
		s.sendPermissionResult(reqID, false, arguments, "Missing questions field")
		return
	}

	questionsSlice, ok := questionsRaw.([]interface{})
	if !ok {
		logger.Log("MCP: AskUserQuestion 'questions' is not an array")
		s.sendPermissionResult(reqID, false, arguments, "Invalid questions format")
		return
	}

	var questions []Question
	for _, q := range questionsSlice {
		qMap, ok := q.(map[string]interface{})
		if !ok {
			continue
		}

		question := Question{}
		if qText, ok := qMap["question"].(string); ok {
			question.Question = qText
		}
		if header, ok := qMap["header"].(string); ok {
			question.Header = header
		}
		if multiSelect, ok := qMap["multiSelect"].(bool); ok {
			question.MultiSelect = multiSelect
		}

		// Parse options
		if optionsRaw, ok := qMap["options"].([]interface{}); ok {
			for _, opt := range optionsRaw {
				optMap, ok := opt.(map[string]interface{})
				if !ok {
					continue
				}
				option := QuestionOption{}
				if label, ok := optMap["label"].(string); ok {
					option.Label = label
				}
				if desc, ok := optMap["description"].(string); ok {
					option.Description = desc
				}
				question.Options = append(question.Options, option)
			}
		}

		questions = append(questions, question)
	}

	if len(questions) == 0 {
		logger.Log("MCP: AskUserQuestion has no valid questions")
		s.sendPermissionResult(reqID, false, arguments, "No valid questions")
		return
	}

	logger.Log("MCP: Parsed %d questions, sending to TUI", len(questions))

	// Send question request to TUI
	questionReq := QuestionRequest{
		ID:        reqID,
		Questions: questions,
	}

	s.questionChan <- questionReq
	logger.Log("MCP: Waiting for TUI answer...")

	// Wait for answer
	answer := <-s.answerChan
	logger.Log("MCP: Received TUI answer with %d responses", len(answer.Answers))

	// Build the response with answers in updatedInput
	updatedInput := map[string]interface{}{
		"questions": arguments["questions"],
		"answers":   answer.Answers,
	}

	s.sendPermissionResult(reqID, true, updatedInput, "")
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

// addAllowedTool adds a tool to the allowed list (called when user selects "always allow").
// This is used internally by the MCP server to remember tools that were allowed during the session.
func (s *Server) addAllowedTool(tool string) {
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

// buildToolDescription creates a human-readable description for known tools
func buildToolDescription(tool string, input map[string]interface{}) string {
	switch tool {
	case "Edit":
		if filePath, ok := input["file_path"].(string); ok {
			return "Edit file: " + filePath
		}
	case "Write":
		if filePath, ok := input["file_path"].(string); ok {
			return "Write file: " + filePath
		}
	case "Read":
		if filePath, ok := input["file_path"].(string); ok {
			return "Read file: " + filePath
		}
	case "Bash":
		if cmd, ok := input["command"].(string); ok {
			return "Run: " + truncateString(cmd, 100)
		}
	case "Glob":
		if pattern, ok := input["pattern"].(string); ok {
			desc := "Search for files: " + pattern
			if path, ok := input["path"].(string); ok {
				desc += " in " + path
			}
			return desc
		}
	case "Grep":
		if pattern, ok := input["pattern"].(string); ok {
			desc := "Search for: " + pattern
			if path, ok := input["path"].(string); ok {
				desc += " in " + path
			}
			return desc
		}
	case "Task":
		if desc, ok := input["description"].(string); ok {
			return "Delegate task: " + desc
		}
		if prompt, ok := input["prompt"].(string); ok {
			return "Delegate task: " + truncateString(prompt, 60)
		}
	case "WebFetch":
		if url, ok := input["url"].(string); ok {
			return "Fetch URL: " + url
		}
	case "WebSearch":
		if query, ok := input["query"].(string); ok {
			return "Web search: " + query
		}
	case "NotebookEdit":
		if path, ok := input["notebook_path"].(string); ok {
			return "Edit notebook: " + path
		}
	default:
		// For unknown tools, try common field names
		if filePath, ok := input["file_path"].(string); ok {
			return tool + ": " + filePath
		}
		if cmd, ok := input["command"].(string); ok {
			return tool + ": " + truncateString(cmd, 80)
		}
		if url, ok := input["url"].(string); ok {
			return tool + ": " + url
		}
		if path, ok := input["path"].(string); ok {
			return tool + ": " + path
		}
	}
	return ""
}

// formatInputForDisplay converts tool arguments to a human-readable format (horizontal layout)
func formatInputForDisplay(args map[string]interface{}) string {
	if len(args) == 0 {
		return "(no details available)"
	}

	var parts []string

	// Get sorted keys for consistent output
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := args[key]
		// Skip internal fields that aren't useful to display
		if key == "tool_use_id" {
			continue
		}

		formatted := formatValue(key, value)
		if formatted != "" {
			parts = append(parts, formatted)
		}
	}

	if len(parts) == 0 {
		return "(no details available)"
	}

	// Join with separator for horizontal layout
	return strings.Join(parts, "  •  ")
}

// formatValue formats a single key-value pair for display
func formatValue(key string, value interface{}) string {
	// Make key more readable
	displayKey := humanizeKey(key)

	switch v := value.(type) {
	case string:
		if v == "" {
			return ""
		}
		return displayKey + ": " + truncateString(v, 100)
	case bool:
		if v {
			return displayKey + ": yes"
		}
		return displayKey + ": no"
	case float64:
		return fmt.Sprintf("%s: %v", displayKey, v)
	case map[string]interface{}:
		// For nested objects, show a summary
		if len(v) == 0 {
			return ""
		}
		return displayKey + ": " + formatNestedObject(v)
	case []interface{}:
		if len(v) == 0 {
			return ""
		}
		return displayKey + ": " + formatArray(v)
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprintf("%s: %v", displayKey, value)
	}
}

// humanizeKey converts snake_case keys to readable labels
func humanizeKey(key string) string {
	// Common key mappings
	keyMap := map[string]string{
		"file_path":     "File",
		"command":       "Command",
		"pattern":       "Pattern",
		"path":          "Path",
		"tool_name":     "Tool",
		"input":         "Input",
		"description":   "Description",
		"url":           "URL",
		"query":         "Query",
		"notebook_path": "Notebook",
		"content":       "Content",
		"old_string":    "Find",
		"new_string":    "Replace with",
		"replace_all":   "Replace all",
	}

	if mapped, ok := keyMap[key]; ok {
		return mapped
	}

	// Convert snake_case to Title Case
	words := strings.Split(key, "_")
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

// formatNestedObject formats a nested map for display
func formatNestedObject(obj map[string]interface{}) string {
	if len(obj) == 0 {
		return "(empty)"
	}

	// For small objects, show inline
	if len(obj) <= 3 {
		var parts []string
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			v := obj[k]
			switch val := v.(type) {
			case string:
				parts = append(parts, humanizeKey(k)+": "+truncateString(val, 40))
			case bool:
				if val {
					parts = append(parts, humanizeKey(k)+": yes")
				} else {
					parts = append(parts, humanizeKey(k)+": no")
				}
			default:
				parts = append(parts, fmt.Sprintf("%s: %v", humanizeKey(k), v))
			}
		}
		return strings.Join(parts, ", ")
	}

	return fmt.Sprintf("(%d properties)", len(obj))
}

// formatArray formats an array for display
func formatArray(arr []interface{}) string {
	if len(arr) == 0 {
		return "(empty)"
	}
	if len(arr) == 1 {
		if s, ok := arr[0].(string); ok {
			return truncateString(s, 60)
		}
		return fmt.Sprintf("%v", arr[0])
	}
	return fmt.Sprintf("(%d items)", len(arr))
}

// truncateString truncates a string to maxLen, adding ellipsis if needed
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
