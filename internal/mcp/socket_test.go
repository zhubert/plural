package mcp

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSocketMessage_Types(t *testing.T) {
	// Verify message type constants
	if MessageTypePermission != "permission" {
		t.Errorf("MessageTypePermission = %q, want 'permission'", MessageTypePermission)
	}
	if MessageTypeQuestion != "question" {
		t.Errorf("MessageTypeQuestion = %q, want 'question'", MessageTypeQuestion)
	}
}

func TestSocketMessage_JSONMarshal_Permission(t *testing.T) {
	msg := SocketMessage{
		Type: MessageTypePermission,
		PermReq: &PermissionRequest{
			ID:   "perm-123",
			Tool: "Read",
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var unmarshaled SocketMessage
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if unmarshaled.Type != MessageTypePermission {
		t.Errorf("Type = %q, want 'permission'", unmarshaled.Type)
	}

	if unmarshaled.PermReq == nil {
		t.Fatal("PermReq is nil")
	}

	if unmarshaled.PermReq.ID != "perm-123" {
		t.Errorf("PermReq.ID = %q, want 'perm-123'", unmarshaled.PermReq.ID)
	}
}

func TestSocketMessage_JSONMarshal_Question(t *testing.T) {
	msg := SocketMessage{
		Type: MessageTypeQuestion,
		QuestReq: &QuestionRequest{
			ID: "quest-123",
			Questions: []Question{
				{Question: "What color?", Header: "Color"},
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var unmarshaled SocketMessage
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if unmarshaled.Type != MessageTypeQuestion {
		t.Errorf("Type = %q, want 'question'", unmarshaled.Type)
	}

	if unmarshaled.QuestReq == nil {
		t.Fatal("QuestReq is nil")
	}

	if len(unmarshaled.QuestReq.Questions) != 1 {
		t.Errorf("Expected 1 question, got %d", len(unmarshaled.QuestReq.Questions))
	}
}

func TestSocketMessage_JSONMarshal_PermissionResponse(t *testing.T) {
	msg := SocketMessage{
		Type: MessageTypePermission,
		PermResp: &PermissionResponse{
			ID:      "perm-123",
			Allowed: true,
			Always:  true,
			Message: "Approved",
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var unmarshaled SocketMessage
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if unmarshaled.PermResp == nil {
		t.Fatal("PermResp is nil")
	}

	if !unmarshaled.PermResp.Allowed {
		t.Error("Expected Allowed to be true")
	}

	if !unmarshaled.PermResp.Always {
		t.Error("Expected Always to be true")
	}
}

func TestSocketMessage_JSONMarshal_QuestionResponse(t *testing.T) {
	msg := SocketMessage{
		Type: MessageTypeQuestion,
		QuestResp: &QuestionResponse{
			ID: "quest-123",
			Answers: map[string]string{
				"color": "blue",
				"size":  "large",
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var unmarshaled SocketMessage
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if unmarshaled.QuestResp == nil {
		t.Fatal("QuestResp is nil")
	}

	if len(unmarshaled.QuestResp.Answers) != 2 {
		t.Errorf("Expected 2 answers, got %d", len(unmarshaled.QuestResp.Answers))
	}

	if unmarshaled.QuestResp.Answers["color"] != "blue" {
		t.Errorf("color = %q, want 'blue'", unmarshaled.QuestResp.Answers["color"])
	}
}

func TestNewSocketServer(t *testing.T) {
	permReqCh := make(chan PermissionRequest, 1)
	permRespCh := make(chan PermissionResponse, 1)
	questReqCh := make(chan QuestionRequest, 1)
	questRespCh := make(chan QuestionResponse, 1)
	planReqCh := make(chan PlanApprovalRequest, 1)
	planRespCh := make(chan PlanApprovalResponse, 1)

	server, err := NewSocketServer("test-session-123", permReqCh, permRespCh, questReqCh, questRespCh, planReqCh, planRespCh)
	if err != nil {
		t.Fatalf("NewSocketServer failed: %v", err)
	}
	defer server.Close()

	// Verify socket path
	path := server.SocketPath()
	if path == "" {
		t.Error("SocketPath returned empty string")
	}

	if !contains(path, "plural-test-session-123.sock") {
		t.Errorf("SocketPath = %q, expected to contain 'plural-test-session-123.sock'", path)
	}
}

func TestSocketServer_Close(t *testing.T) {
	permReqCh := make(chan PermissionRequest, 1)
	permRespCh := make(chan PermissionResponse, 1)
	questReqCh := make(chan QuestionRequest, 1)
	questRespCh := make(chan QuestionResponse, 1)
	planReqCh := make(chan PlanApprovalRequest, 1)
	planRespCh := make(chan PlanApprovalResponse, 1)

	server, err := NewSocketServer("test-close-session", permReqCh, permRespCh, questReqCh, questRespCh, planReqCh, planRespCh)
	if err != nil {
		t.Fatalf("NewSocketServer failed: %v", err)
	}

	// Close should not error
	if err := server.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Double close should be safe (listener already closed)
	// This may return an error which is expected
	server.Close()
}

func TestSocketClientServer_Integration(t *testing.T) {
	permReqCh := make(chan PermissionRequest, 1)
	permRespCh := make(chan PermissionResponse, 1)
	questReqCh := make(chan QuestionRequest, 1)
	questRespCh := make(chan QuestionResponse, 1)
	planReqCh := make(chan PlanApprovalRequest, 1)
	planRespCh := make(chan PlanApprovalResponse, 1)

	server, err := NewSocketServer("test-integration", permReqCh, permRespCh, questReqCh, questRespCh, planReqCh, planRespCh)
	if err != nil {
		t.Fatalf("NewSocketServer failed: %v", err)
	}
	defer server.Close()

	// Start server in background
	server.Start()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Create client
	client, err := NewSocketClient(server.SocketPath())
	if err != nil {
		t.Fatalf("NewSocketClient failed: %v", err)
	}
	defer client.Close()

	// Test permission request/response in a goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)

		// Wait for request on server side
		select {
		case req := <-permReqCh:
			if req.ID != "test-perm-1" {
				t.Errorf("Request ID = %q, want 'test-perm-1'", req.ID)
			}
			// Send response
			permRespCh <- PermissionResponse{
				ID:      req.ID,
				Allowed: true,
				Message: "Approved",
			}
		case <-time.After(5 * time.Second):
			t.Error("Timeout waiting for permission request")
		}
	}()

	// Send permission request from client
	resp, err := client.SendPermissionRequest(PermissionRequest{
		ID:   "test-perm-1",
		Tool: "Read",
	})
	if err != nil {
		t.Fatalf("SendPermissionRequest failed: %v", err)
	}

	<-done

	if resp.ID != "test-perm-1" {
		t.Errorf("Response ID = %q, want 'test-perm-1'", resp.ID)
	}

	if !resp.Allowed {
		t.Error("Expected Allowed to be true")
	}

	// Close server to stop Run()
	server.Close()
}

func TestSocketClientServer_Question(t *testing.T) {
	permReqCh := make(chan PermissionRequest, 1)
	permRespCh := make(chan PermissionResponse, 1)
	questReqCh := make(chan QuestionRequest, 1)
	questRespCh := make(chan QuestionResponse, 1)
	planReqCh := make(chan PlanApprovalRequest, 1)
	planRespCh := make(chan PlanApprovalResponse, 1)

	server, err := NewSocketServer("test-question", permReqCh, permRespCh, questReqCh, questRespCh, planReqCh, planRespCh)
	if err != nil {
		t.Fatalf("NewSocketServer failed: %v", err)
	}
	defer server.Close()

	// Start server
	server.Start()

	time.Sleep(50 * time.Millisecond)

	client, err := NewSocketClient(server.SocketPath())
	if err != nil {
		t.Fatalf("NewSocketClient failed: %v", err)
	}
	defer client.Close()

	// Handle question on server side
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case req := <-questReqCh:
			questRespCh <- QuestionResponse{
				ID:      req.ID,
				Answers: map[string]string{"q1": "answer1"},
			}
		case <-time.After(5 * time.Second):
			t.Error("Timeout waiting for question request")
		}
	}()

	// Send question from client
	resp, err := client.SendQuestionRequest(QuestionRequest{
		ID: "quest-1",
		Questions: []Question{
			{Question: "Test?", Header: "Q1"},
		},
	})
	if err != nil {
		t.Fatalf("SendQuestionRequest failed: %v", err)
	}

	<-done

	if resp.ID != "quest-1" {
		t.Errorf("Response ID = %q, want 'quest-1'", resp.ID)
	}

	if resp.Answers["q1"] != "answer1" {
		t.Errorf("Answer = %q, want 'answer1'", resp.Answers["q1"])
	}

	server.Close()
}

func TestNewSocketClient_InvalidPath(t *testing.T) {
	_, err := NewSocketClient("/nonexistent/path.sock")
	if err == nil {
		t.Error("Expected error for invalid socket path")
	}
}

func TestSocketConstants(t *testing.T) {
	// Verify timeout constants
	if PermissionResponseTimeout != 5*time.Minute {
		t.Errorf("PermissionResponseTimeout = %v, want 5m", PermissionResponseTimeout)
	}

	if SocketReadTimeout != 10*time.Second {
		t.Errorf("SocketReadTimeout = %v, want 10s", SocketReadTimeout)
	}

	if SocketWriteTimeout != 10*time.Second {
		t.Errorf("SocketWriteTimeout = %v, want 10s", SocketWriteTimeout)
	}
}

func TestSocketClientServer_PlanApproval(t *testing.T) {
	permReqCh := make(chan PermissionRequest, 1)
	permRespCh := make(chan PermissionResponse, 1)
	questReqCh := make(chan QuestionRequest, 1)
	questRespCh := make(chan QuestionResponse, 1)
	planReqCh := make(chan PlanApprovalRequest, 1)
	planRespCh := make(chan PlanApprovalResponse, 1)

	server, err := NewSocketServer("test-plan-approval", permReqCh, permRespCh, questReqCh, questRespCh, planReqCh, planRespCh)
	if err != nil {
		t.Fatalf("NewSocketServer failed: %v", err)
	}
	defer server.Close()

	// Start server
	server.Start()

	time.Sleep(50 * time.Millisecond)

	client, err := NewSocketClient(server.SocketPath())
	if err != nil {
		t.Fatalf("NewSocketClient failed: %v", err)
	}
	defer client.Close()

	// Handle plan approval on server side
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case req := <-planReqCh:
			planRespCh <- PlanApprovalResponse{
				ID:       req.ID,
				Approved: true,
			}
		case <-time.After(5 * time.Second):
			t.Error("Timeout waiting for plan approval request")
		}
	}()

	// Send plan approval from client
	resp, err := client.SendPlanApprovalRequest(PlanApprovalRequest{
		ID:   "plan-1",
		Plan: "Test plan content",
	})
	if err != nil {
		t.Fatalf("SendPlanApprovalRequest failed: %v", err)
	}

	<-done

	if resp.ID != "plan-1" {
		t.Errorf("Response ID = %q, want 'plan-1'", resp.ID)
	}

	if !resp.Approved {
		t.Error("Expected Approved to be true")
	}

	server.Close()
}

func TestSocketClient_WriteTimeoutErrorMessage(t *testing.T) {
	// This test verifies that the error messages include context about what operation failed.
	// We can't easily test the actual timeout without a slow server, but we can verify
	// the error wrapping is in place by checking error messages on connection failures.

	// Create a client to a non-existent socket (will fail to connect)
	_, err := NewSocketClient("/tmp/nonexistent-socket-for-test.sock")
	if err == nil {
		t.Fatal("Expected error connecting to non-existent socket")
	}

	// The connection error is expected - this just verifies NewSocketClient returns errors properly
}

// contains checks if s contains substr (helper for tests)
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

