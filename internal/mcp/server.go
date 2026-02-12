package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zhubert/plural/internal/logger"
)

// MCP server timeout constants
const (
	// ChannelSendTimeout is the timeout for sending to TUI channels
	ChannelSendTimeout = 10 * time.Second
	// ChannelReceiveTimeout is the timeout for receiving from TUI channels
	ChannelReceiveTimeout = 5 * time.Minute
)

const (
	ProtocolVersion = "2024-11-05"
	ServerName      = "plural-permission"
	ServerVersion   = "1.0.0"
	ToolName        = "permission"
)

// Server implements an MCP server for handling permission prompts
type Server struct {
	reader           *bufio.Reader
	writer           io.Writer
	requestChan      chan<- PermissionRequest      // Send permission requests to TUI
	responseChan     <-chan PermissionResponse     // Receive responses from TUI
	questionChan     chan<- QuestionRequest        // Send question requests to TUI
	answerChan       <-chan QuestionResponse       // Receive answers from TUI
	planApprovalChan chan<- PlanApprovalRequest    // Send plan approval requests to TUI
	planResponseChan <-chan PlanApprovalResponse   // Receive plan approval responses from TUI
	allowedTools     []string                      // Pre-allowed tools for this session
	mu               sync.Mutex
	log              *slog.Logger                  // Logger with session context
}

// NewServer creates a new MCP server
func NewServer(r io.Reader, w io.Writer, reqChan chan<- PermissionRequest, respChan <-chan PermissionResponse, questionChan chan<- QuestionRequest, answerChan <-chan QuestionResponse, planApprovalChan chan<- PlanApprovalRequest, planResponseChan <-chan PlanApprovalResponse, allowedTools []string, sessionID string) *Server {
	return &Server{
		reader:           bufio.NewReader(r),
		writer:           w,
		requestChan:      reqChan,
		responseChan:     respChan,
		questionChan:     questionChan,
		answerChan:       answerChan,
		planApprovalChan: planApprovalChan,
		planResponseChan: planResponseChan,
		allowedTools:     allowedTools,
		log:              logger.WithSession(sessionID).With("component", "mcp"),
	}
}

// Run starts the MCP server loop
func (s *Server) Run() error {
	s.log.Info("server starting")

	for {
		line, err := s.reader.ReadString('\n')
		if err == io.EOF {
			s.log.Info("EOF received, shutting down")
			return nil
		}
		if err != nil {
			s.log.Error("read error", "error", err)
			return err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		s.log.Debug("received message", "line", line)

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.log.Error("JSON parse error", "error", err)
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
		s.log.Debug("initialized notification received")
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	default:
		s.log.Warn("unknown method", "method", req.Method)
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
		s.log.Error("failed to parse tool call params", "error", err)
		s.sendError(req.ID, -32602, "Invalid params", nil)
		return
	}

	if params.Name != ToolName {
		s.log.Warn("unknown tool", "tool", params.Name)
		s.sendError(req.ID, -32602, "Unknown tool", nil)
		return
	}

	// Log the full arguments for debugging
	argsJSON, err := json.Marshal(params.Arguments)
	if err != nil {
		s.log.Debug("permission tool called", "marshalError", err)
	} else {
		s.log.Debug("permission tool called", "arguments", string(argsJSON))
	}

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

	s.log.Info("permission request", "tool", tool, "description", description)

	// Special handling for AskUserQuestion
	if tool == "AskUserQuestion" {
		s.handleAskUserQuestion(req.ID, arguments)
		return
	}

	// Special handling for ExitPlanMode - show plan approval UI instead of permission prompt
	if tool == "ExitPlanMode" {
		s.handleExitPlanMode(req.ID, arguments)
		return
	}

	// Check if tool is pre-allowed
	if s.isToolAllowed(tool) {
		s.log.Debug("tool is pre-allowed", "tool", tool)
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

	// Send to TUI with timeout to prevent deadlock if TUI is unresponsive
	select {
	case s.requestChan <- permReq:
		s.log.Debug("waiting for TUI response")
	case <-time.After(ChannelSendTimeout):
		s.log.Warn("timeout sending permission request to TUI")
		s.sendPermissionResult(req.ID, false, arguments, "TUI not responding")
		return
	}

	// Wait for response with timeout
	select {
	case resp := <-s.responseChan:
		s.log.Info("received TUI response", "allowed", resp.Allowed, "always", resp.Always)

		// If user selected "always allow", remember this tool for future requests
		if resp.Always {
			s.addAllowedTool(tool)
		}

		s.sendPermissionResult(req.ID, resp.Allowed, arguments, resp.Message)
	case <-time.After(ChannelReceiveTimeout):
		s.log.Warn("timeout waiting for TUI response")
		s.sendPermissionResult(req.ID, false, arguments, "Permission request timed out")
	}
}

// handleAskUserQuestion handles the AskUserQuestion tool specially
func (s *Server) handleAskUserQuestion(reqID interface{}, arguments map[string]interface{}) {
	s.log.Debug("handling AskUserQuestion")

	// Parse questions from arguments
	questionsRaw, ok := arguments["questions"]
	if !ok {
		s.log.Warn("AskUserQuestion missing 'questions' field")
		s.sendPermissionResult(reqID, false, arguments, "Missing questions field")
		return
	}

	questionsSlice, ok := questionsRaw.([]interface{})
	if !ok {
		s.log.Warn("AskUserQuestion 'questions' is not an array")
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
		s.log.Warn("AskUserQuestion has no valid questions")
		s.sendPermissionResult(reqID, false, arguments, "No valid questions")
		return
	}

	s.log.Debug("parsed questions, sending to TUI", "count", len(questions))

	// Send question request to TUI
	questionReq := QuestionRequest{
		ID:        reqID,
		Questions: questions,
	}

	// Send to TUI with timeout to prevent deadlock if TUI is unresponsive
	select {
	case s.questionChan <- questionReq:
		s.log.Debug("waiting for TUI answer")
	case <-time.After(ChannelSendTimeout):
		s.log.Warn("timeout sending question request to TUI")
		s.sendPermissionResult(reqID, false, arguments, "TUI not responding")
		return
	}

	// Wait for answer with timeout
	var answer QuestionResponse
	select {
	case answer = <-s.answerChan:
		s.log.Info("received TUI answer", "answerCount", len(answer.Answers))
	case <-time.After(ChannelReceiveTimeout):
		s.log.Warn("timeout waiting for TUI answer")
		s.sendPermissionResult(reqID, false, arguments, "Question request timed out")
		return
	}

	// Build the response with answers in updatedInput
	updatedInput := map[string]interface{}{
		"questions": arguments["questions"],
		"answers":   answer.Answers,
	}

	s.sendPermissionResult(reqID, true, updatedInput, "")
}

// handleExitPlanMode handles the ExitPlanMode tool specially to show a plan approval UI
func (s *Server) handleExitPlanMode(reqID interface{}, arguments map[string]interface{}) {
	s.log.Debug("handling ExitPlanMode", "arguments", arguments)

	// Extract plan content - try multiple sources
	plan, _ := arguments["plan"].(string)
	if plan == "" {
		// Try to get file path from arguments (Claude Code stores plans in ~/.claude/plans/)
		if filePath, ok := arguments["filePath"].(string); ok && filePath != "" {
			s.log.Debug("ExitPlanMode has filePath, reading from file", "filePath", filePath)
			plan = s.readPlanFromPath(filePath)
		} else {
			s.log.Warn("ExitPlanMode missing both 'plan' and 'filePath' fields")
			plan = "*No plan content provided. Please check the plan file manually.*"
		}
	}

	// Parse allowedPrompts if present
	var allowedPrompts []AllowedPrompt
	if promptsRaw, ok := arguments["allowedPrompts"].([]interface{}); ok {
		for _, p := range promptsRaw {
			pMap, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			prompt := AllowedPrompt{}
			if tool, ok := pMap["tool"].(string); ok {
				prompt.Tool = tool
			}
			if desc, ok := pMap["prompt"].(string); ok {
				prompt.Prompt = desc
			}
			if prompt.Tool != "" && prompt.Prompt != "" {
				allowedPrompts = append(allowedPrompts, prompt)
			}
		}
	}

	s.log.Debug("parsed plan, sending to TUI", "planLength", len(plan), "allowedPromptCount", len(allowedPrompts))

	// Send plan approval request to TUI
	planReq := PlanApprovalRequest{
		ID:             reqID,
		Plan:           plan,
		AllowedPrompts: allowedPrompts,
		Arguments:      arguments,
	}

	// Send to TUI with timeout
	select {
	case s.planApprovalChan <- planReq:
		s.log.Debug("waiting for TUI plan approval")
	case <-time.After(ChannelSendTimeout):
		s.log.Warn("timeout sending plan approval request to TUI")
		s.sendPermissionResult(reqID, false, arguments, "TUI not responding")
		return
	}

	// Wait for approval with timeout
	var response PlanApprovalResponse
	select {
	case response = <-s.planResponseChan:
		s.log.Info("received TUI plan approval response", "approved", response.Approved)
	case <-time.After(ChannelReceiveTimeout):
		s.log.Warn("timeout waiting for TUI plan approval")
		s.sendPermissionResult(reqID, false, arguments, "Plan approval request timed out")
		return
	}

	if response.Approved {
		s.sendPermissionResult(reqID, true, arguments, "")
	} else {
		s.sendPermissionResult(reqID, false, arguments, "Plan rejected by user")
	}
}

func (s *Server) isToolAllowed(tool string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, allowed := range s.allowedTools {
		// Wildcard matches any tool — used in container mode where the container
		// IS the sandbox, so all regular permissions are auto-approved while
		// AskUserQuestion and ExitPlanMode still route through the TUI.
		if allowed == "*" {
			return true
		}
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
	resultJSON, err := json.Marshal(result)
	if err != nil {
		s.log.Error("failed to marshal permission result", "error", err)
		// Send a fallback error response to prevent blocking
		toolResult := ToolCallResult{
			Content: []ContentItem{
				{
					Type: "text",
					Text: `{"behavior":"deny","message":"internal error: failed to marshal result"}`,
				},
			},
		}
		s.sendResult(id, toolResult)
		return
	}
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
		s.log.Error("failed to marshal response", "error", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err = fmt.Fprintf(s.writer, "%s\n", data)
	if err != nil {
		s.log.Error("failed to write response", "error", err)
	} else {
		s.log.Debug("sent response", "data", string(data))
	}
}

// readPlanFromPath attempts to read the plan from the specified file path.
// The path must resolve to within ~/.claude/plans/ to prevent path traversal attacks.
// Returns the file contents if found, or an error message if not.
func (s *Server) readPlanFromPath(planPath string) string {
	// Validate the path is within the allowed plans directory
	if err := validatePlanPath(planPath); err != nil {
		s.log.Warn("plan path validation failed", "path", planPath, "error", err)
		return fmt.Sprintf("*Invalid plan path: %v*", err)
	}

	content, err := os.ReadFile(planPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.log.Warn("plan file not found", "path", planPath)
			return fmt.Sprintf("*Plan file not found at %s*", planPath)
		}
		s.log.Error("failed to read plan file", "error", err)
		return fmt.Sprintf("*Error reading plan file: %v*", err)
	}

	s.log.Debug("successfully read plan file", "bytes", len(content), "path", planPath)
	return string(content)
}

// validatePlanPath ensures the given path resolves to within ~/.claude/plans/.
// This prevents path traversal attacks where a malicious filePath argument
// could read arbitrary files from the filesystem.
func validatePlanPath(planPath string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	allowedDir := filepath.Join(homeDir, ".claude", "plans")

	// Clean and resolve the path to eliminate ../ traversal
	absPath, err := filepath.Abs(planPath)
	if err != nil {
		return fmt.Errorf("cannot resolve path: %w", err)
	}
	cleanPath := filepath.Clean(absPath)

	// Ensure the resolved path is within the allowed directory.
	// We append os.PathSeparator to prevent prefix matches like
	// ~/.claude/plans-evil/ matching ~/.claude/plans
	if !strings.HasPrefix(cleanPath, allowedDir+string(os.PathSeparator)) && cleanPath != allowedDir {
		return fmt.Errorf("path must be within %s", allowedDir)
	}

	return nil
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

// truncateString truncates a string to maxLen, adding ellipsis if needed.
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
