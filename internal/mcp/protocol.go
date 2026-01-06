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
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capability   `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
	Instructions    string       `json:"instructions,omitempty"`
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
	ID          interface{}            `json:"id"`           // JSON-RPC request ID for response correlation
	Tool        string                 `json:"tool"`         // Tool name (e.g., "Edit", "Bash")
	Description string                 `json:"description"`  // Human-readable description
	Arguments   map[string]interface{} `json:"arguments"`    // Tool arguments for context
}

// PermissionResponse represents the user's response to a permission request
type PermissionResponse struct {
	ID       interface{} `json:"id"`       // Correlates with request ID
	Allowed  bool        `json:"allowed"`  // Whether permission was granted
	Always   bool        `json:"always"`   // Whether to remember this decision
	Message  string      `json:"message"`  // Optional denial message
}

// PermissionResult is the format expected by Claude Code's permission-prompt-tool
type PermissionResult struct {
	Behavior     string                 `json:"behavior"`               // "allow" or "deny"
	UpdatedInput map[string]interface{} `json:"updatedInput,omitempty"` // Original or modified input
	Message      string                 `json:"message,omitempty"`      // Reason for denial
}
