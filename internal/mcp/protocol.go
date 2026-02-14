package mcp

import "encoding/json"

// JSON-RPC 2.0 message types for MCP protocol

// JSONRPCRequest represents an incoming JSON-RPC request
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents an outgoing JSON-RPC response
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// MCP Protocol specific types

// InitializeParams for the initialize method
type InitializeParams struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    Capability `json:"capabilities"`
	ClientInfo      ClientInfo `json:"clientInfo"`
}

// Capability represents MCP capabilities
type Capability struct {
	Tools *ToolCapability `json:"tools,omitempty"`
}

// ToolCapability represents tool-related capabilities
type ToolCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ClientInfo represents client information
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult for the initialize response
type InitializeResult struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    Capability `json:"capabilities"`
	ServerInfo      ServerInfo `json:"serverInfo"`
	Instructions    string     `json:"instructions,omitempty"`
}

// ServerInfo represents server information
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ToolsListResult for tools/list response
type ToolsListResult struct {
	Tools []ToolDefinition `json:"tools"`
}

// ToolDefinition represents a tool available in the MCP server
type ToolDefinition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema represents the JSON schema for tool input
type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

// Property represents a property in the input schema
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ToolCallParams represents parameters for tools/call
type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolCallResult represents the result of a tool call
type ToolCallResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ContentItem represents content in a tool result
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// PermissionRequest represents a permission request sent to the TUI
type PermissionRequest struct {
	ID          interface{}            `json:"id"`          // JSON-RPC request ID for response correlation
	Tool        string                 `json:"tool"`        // Tool name (e.g., "Edit", "Bash")
	Description string                 `json:"description"` // Human-readable description
	Arguments   map[string]interface{} `json:"arguments"`   // Tool arguments for context
}

// PermissionResponse represents the user's response to a permission request
type PermissionResponse struct {
	ID      interface{} `json:"id"`      // Correlates with request ID
	Allowed bool        `json:"allowed"` // Whether permission was granted
	Always  bool        `json:"always"`  // Whether to remember this decision
	Message string      `json:"message"` // Optional denial message
}

// PermissionResult is the format expected by Claude Code's permission-prompt-tool
type PermissionResult struct {
	Behavior     string                 `json:"behavior"`               // "allow" or "deny"
	UpdatedInput map[string]interface{} `json:"updatedInput,omitempty"` // Original or modified input
	Message      string                 `json:"message,omitempty"`      // Reason for denial
}

// QuestionOption represents a single option in a question
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// Question represents a single question with options
type Question struct {
	Question    string           `json:"question"`
	Header      string           `json:"header"`
	Options     []QuestionOption `json:"options"`
	MultiSelect bool             `json:"multiSelect"`
}

// QuestionRequest represents an AskUserQuestion request sent to the TUI
type QuestionRequest struct {
	ID        interface{} `json:"id"`        // JSON-RPC request ID for response correlation
	Questions []Question  `json:"questions"` // Questions to ask the user
}

// QuestionResponse represents the user's answers to questions
type QuestionResponse struct {
	ID      interface{}       `json:"id"`      // Correlates with request ID
	Answers map[string]string `json:"answers"` // Map of question text to selected option label
}

// AllowedPrompt represents a Bash permission that Claude is requesting as part of the plan
type AllowedPrompt struct {
	Tool   string `json:"tool"`   // Tool name (typically "Bash")
	Prompt string `json:"prompt"` // Description of the action (e.g., "run tests")
}

// PlanApprovalRequest represents an ExitPlanMode request sent to the TUI
type PlanApprovalRequest struct {
	ID             interface{}            `json:"id"`             // JSON-RPC request ID for response correlation
	Plan           string                 `json:"plan"`           // The plan content (markdown)
	AllowedPrompts []AllowedPrompt        `json:"allowedPrompts"` // Bash permissions being requested
	Arguments      map[string]interface{} `json:"arguments"`      // Original arguments for the response
}

// PlanApprovalResponse represents the user's response to a plan approval request
type PlanApprovalResponse struct {
	ID       interface{} `json:"id"`       // Correlates with request ID
	Approved bool        `json:"approved"` // Whether the plan was approved
}

// CreateChildRequest represents a request from the supervisor to create a child session
type CreateChildRequest struct {
	ID   interface{} `json:"id"`   // JSON-RPC request ID for response correlation
	Task string      `json:"task"` // Task description for the child session
}

// CreateChildResponse represents the result of creating a child session
type CreateChildResponse struct {
	ID      interface{} `json:"id"`                // Correlates with request ID
	Success bool        `json:"success"`           // Whether child was created successfully
	ChildID string      `json:"child_id,omitempty"` // ID of the created child session
	Branch  string      `json:"branch,omitempty"`  // Branch name of the child session
	Error   string      `json:"error,omitempty"`   // Error message if creation failed
}

// ListChildrenRequest represents a request from the supervisor to list child sessions
type ListChildrenRequest struct {
	ID interface{} `json:"id"` // JSON-RPC request ID for response correlation
}

// ChildSessionInfo represents the status of a child session
type ChildSessionInfo struct {
	ID     string `json:"id"`     // Session ID
	Branch string `json:"branch"` // Branch name
	Status string `json:"status"` // "running", "idle", "completed", "merged"
}

// ListChildrenResponse represents the result of listing child sessions
type ListChildrenResponse struct {
	ID       interface{}        `json:"id"`       // Correlates with request ID
	Children []ChildSessionInfo `json:"children"` // List of child sessions
}

// MergeChildRequest represents a request from the supervisor to merge a child session
type MergeChildRequest struct {
	ID             interface{} `json:"id"`               // JSON-RPC request ID for response correlation
	ChildSessionID string      `json:"child_session_id"` // ID of the child session to merge
}

// MergeChildResponse represents the result of merging a child session
type MergeChildResponse struct {
	ID      interface{} `json:"id"`                // Correlates with request ID
	Success bool        `json:"success"`           // Whether merge was successful
	Message string      `json:"message,omitempty"` // Success or error message
	Error   string      `json:"error,omitempty"`   // Error message if merge failed
}
