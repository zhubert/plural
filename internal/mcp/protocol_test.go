package mcp

import (
	"encoding/json"
	"testing"
)

func TestJSONRPCRequest_Marshal(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "test/method",
		Params:  json.RawMessage(`{"key": "value"}`),
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded JSONRPCRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want %q", decoded.JSONRPC, "2.0")
	}
	if decoded.Method != "test/method" {
		t.Errorf("Method = %q, want %q", decoded.Method, "test/method")
	}
}

func TestJSONRPCResponse_Marshal(t *testing.T) {
	tests := []struct {
		name     string
		response JSONRPCResponse
	}{
		{
			name: "success response",
			response: JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      1,
				Result:  map[string]string{"status": "ok"},
			},
		},
		{
			name: "error response",
			response: JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      1,
				Error: &RPCError{
					Code:    -32600,
					Message: "Invalid Request",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.response)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			var decoded JSONRPCResponse
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if decoded.JSONRPC != "2.0" {
				t.Errorf("JSONRPC = %q, want %q", decoded.JSONRPC, "2.0")
			}
		})
	}
}

func TestRPCError_Marshal(t *testing.T) {
	err := RPCError{
		Code:    -32601,
		Message: "Method not found",
		Data:    "additional info",
	}

	data, err2 := json.Marshal(err)
	if err2 != nil {
		t.Fatalf("Failed to marshal: %v", err2)
	}

	var decoded RPCError
	if err2 := json.Unmarshal(data, &decoded); err2 != nil {
		t.Fatalf("Failed to unmarshal: %v", err2)
	}

	if decoded.Code != -32601 {
		t.Errorf("Code = %d, want %d", decoded.Code, -32601)
	}
	if decoded.Message != "Method not found" {
		t.Errorf("Message = %q, want %q", decoded.Message, "Method not found")
	}
}

func TestInitializeParams_Marshal(t *testing.T) {
	params := InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities: Capability{
			Tools: &ToolCapability{ListChanged: true},
		},
		ClientInfo: ClientInfo{
			Name:    "test-client",
			Version: "1.0.0",
		},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded InitializeParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.ProtocolVersion != "2024-11-05" {
		t.Errorf("ProtocolVersion = %q, want %q", decoded.ProtocolVersion, "2024-11-05")
	}
	if decoded.ClientInfo.Name != "test-client" {
		t.Errorf("ClientInfo.Name = %q, want %q", decoded.ClientInfo.Name, "test-client")
	}
}

func TestInitializeResult_Marshal(t *testing.T) {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities:    Capability{},
		ServerInfo: ServerInfo{
			Name:    "plural-permission",
			Version: "1.0.0",
		},
		Instructions: "Handle permission prompts",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded InitializeResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.ServerInfo.Name != "plural-permission" {
		t.Errorf("ServerInfo.Name = %q, want %q", decoded.ServerInfo.Name, "plural-permission")
	}
}

func TestToolsListResult_Marshal(t *testing.T) {
	result := ToolsListResult{
		Tools: []ToolDefinition{
			{
				Name:        "permission",
				Description: "Handle permission prompts",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"tool_name": {Type: "string", Description: "Name of the tool"},
					},
					Required: []string{"tool_name"},
				},
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ToolsListResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if len(decoded.Tools) != 1 {
		t.Fatalf("len(Tools) = %d, want %d", len(decoded.Tools), 1)
	}
	if decoded.Tools[0].Name != "permission" {
		t.Errorf("Tools[0].Name = %q, want %q", decoded.Tools[0].Name, "permission")
	}
}

func TestToolCallParams_Marshal(t *testing.T) {
	params := ToolCallParams{
		Name: "permission",
		Arguments: map[string]any{
			"tool_name":   "Edit",
			"description": "Edit file.go",
		},
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ToolCallParams
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Name != "permission" {
		t.Errorf("Name = %q, want %q", decoded.Name, "permission")
	}
	if decoded.Arguments["tool_name"] != "Edit" {
		t.Errorf("Arguments[tool_name] = %v, want %q", decoded.Arguments["tool_name"], "Edit")
	}
}

func TestToolCallResult_Marshal(t *testing.T) {
	tests := []struct {
		name   string
		result ToolCallResult
	}{
		{
			name: "success result",
			result: ToolCallResult{
				Content: []ContentItem{
					{Type: "text", Text: "Permission granted"},
				},
				IsError: false,
			},
		},
		{
			name: "error result",
			result: ToolCallResult{
				Content: []ContentItem{
					{Type: "text", Text: "Permission denied"},
				},
				IsError: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.result)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			var decoded ToolCallResult
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if len(decoded.Content) != 1 {
				t.Fatalf("len(Content) = %d, want %d", len(decoded.Content), 1)
			}
			if decoded.IsError != tt.result.IsError {
				t.Errorf("IsError = %v, want %v", decoded.IsError, tt.result.IsError)
			}
		})
	}
}

func TestPermissionRequest_Marshal(t *testing.T) {
	req := PermissionRequest{
		ID:          123,
		Tool:        "Bash",
		Description: "Run: git status",
		Arguments: map[string]any{
			"command": "git status",
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded PermissionRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Tool != "Bash" {
		t.Errorf("Tool = %q, want %q", decoded.Tool, "Bash")
	}
	if decoded.Description != "Run: git status" {
		t.Errorf("Description = %q, want %q", decoded.Description, "Run: git status")
	}
}

func TestPermissionResponse_Marshal(t *testing.T) {
	tests := []struct {
		name     string
		response PermissionResponse
	}{
		{
			name: "allowed",
			response: PermissionResponse{
				ID:      123,
				Allowed: true,
				Always:  false,
			},
		},
		{
			name: "always allowed",
			response: PermissionResponse{
				ID:      123,
				Allowed: true,
				Always:  true,
			},
		},
		{
			name: "denied",
			response: PermissionResponse{
				ID:      123,
				Allowed: false,
				Message: "User denied permission",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.response)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			var decoded PermissionResponse
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if decoded.Allowed != tt.response.Allowed {
				t.Errorf("Allowed = %v, want %v", decoded.Allowed, tt.response.Allowed)
			}
			if decoded.Always != tt.response.Always {
				t.Errorf("Always = %v, want %v", decoded.Always, tt.response.Always)
			}
		})
	}
}

func TestPermissionResult_Marshal(t *testing.T) {
	tests := []struct {
		name   string
		result PermissionResult
	}{
		{
			name: "allow",
			result: PermissionResult{
				Behavior:     "allow",
				UpdatedInput: map[string]any{"command": "git status"},
			},
		},
		{
			name: "deny",
			result: PermissionResult{
				Behavior: "deny",
				Message:  "User denied this action",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.result)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			var decoded PermissionResult
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if decoded.Behavior != tt.result.Behavior {
				t.Errorf("Behavior = %q, want %q", decoded.Behavior, tt.result.Behavior)
			}
		})
	}
}

func TestContentItem_Marshal(t *testing.T) {
	item := ContentItem{
		Type: "text",
		Text: "Hello, world!",
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded ContentItem
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Type != "text" {
		t.Errorf("Type = %q, want %q", decoded.Type, "text")
	}
	if decoded.Text != "Hello, world!" {
		t.Errorf("Text = %q, want %q", decoded.Text, "Hello, world!")
	}
}

func TestInputSchema_Marshal(t *testing.T) {
	schema := InputSchema{
		Type: "object",
		Properties: map[string]Property{
			"name":    {Type: "string", Description: "User name"},
			"age":     {Type: "integer", Description: "User age"},
			"enabled": {Type: "boolean", Description: "Is enabled"},
		},
		Required: []string{"name"},
	}

	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded InputSchema
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Type != "object" {
		t.Errorf("Type = %q, want %q", decoded.Type, "object")
	}
	if len(decoded.Properties) != 3 {
		t.Errorf("len(Properties) = %d, want %d", len(decoded.Properties), 3)
	}
	if len(decoded.Required) != 1 || decoded.Required[0] != "name" {
		t.Errorf("Required = %v, want [\"name\"]", decoded.Required)
	}
}
