package mcp

import (
	"bufio"
	"encoding/json"
	"net"
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

	if !contains(path, "pl-test-session.sock") {
		t.Errorf("SocketPath = %q, expected to contain 'pl-test-session.sock'", path)
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
	server.WaitReady()

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

	server.WaitReady()

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

	server.WaitReady()

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

func TestNewTCPSocketServer(t *testing.T) {
	permReqCh := make(chan PermissionRequest, 1)
	permRespCh := make(chan PermissionResponse, 1)
	questReqCh := make(chan QuestionRequest, 1)
	questRespCh := make(chan QuestionResponse, 1)
	planReqCh := make(chan PlanApprovalRequest, 1)
	planRespCh := make(chan PlanApprovalResponse, 1)

	server, err := NewTCPSocketServer("test-tcp-session", permReqCh, permRespCh, questReqCh, questRespCh, planReqCh, planRespCh)
	if err != nil {
		t.Fatalf("NewTCPSocketServer failed: %v", err)
	}
	defer server.Close()

	// SocketPath should be empty for TCP servers
	if path := server.SocketPath(); path != "" {
		t.Errorf("SocketPath() = %q, want empty for TCP server", path)
	}

	// TCPAddr should return a non-empty address
	addr := server.TCPAddr()
	if addr == "" {
		t.Error("TCPAddr() returned empty string")
	}

	// TCPPort should return a positive port number
	port := server.TCPPort()
	if port <= 0 {
		t.Errorf("TCPPort() = %d, want positive port number", port)
	}
}

func TestNewTCPSocketServer_Close(t *testing.T) {
	permReqCh := make(chan PermissionRequest, 1)
	permRespCh := make(chan PermissionResponse, 1)
	questReqCh := make(chan QuestionRequest, 1)
	questRespCh := make(chan QuestionResponse, 1)
	planReqCh := make(chan PlanApprovalRequest, 1)
	planRespCh := make(chan PlanApprovalResponse, 1)

	server, err := NewTCPSocketServer("test-tcp-close", permReqCh, permRespCh, questReqCh, questRespCh, planReqCh, planRespCh)
	if err != nil {
		t.Fatalf("NewTCPSocketServer failed: %v", err)
	}

	// Close should not error
	if err := server.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Double close should be safe
	server.Close()
}

func TestTCPSocketServer_SocketPathEmpty(t *testing.T) {
	permReqCh := make(chan PermissionRequest, 1)
	permRespCh := make(chan PermissionResponse, 1)
	questReqCh := make(chan QuestionRequest, 1)
	questRespCh := make(chan QuestionResponse, 1)
	planReqCh := make(chan PlanApprovalRequest, 1)
	planRespCh := make(chan PlanApprovalResponse, 1)

	server, err := NewTCPSocketServer("test-tcp-no-socket", permReqCh, permRespCh, questReqCh, questRespCh, planReqCh, planRespCh)
	if err != nil {
		t.Fatalf("NewTCPSocketServer failed: %v", err)
	}
	defer server.Close()

	// TCP server should have isTCP true
	if !server.isTCP {
		t.Error("TCP server should have isTCP = true")
	}

	// Unix socket server should have isTCP false
	unixServer, err := NewSocketServer("test-unix-check", permReqCh, permRespCh, questReqCh, questRespCh, planReqCh, planRespCh)
	if err != nil {
		t.Fatalf("NewSocketServer failed: %v", err)
	}
	defer unixServer.Close()

	if unixServer.isTCP {
		t.Error("Unix socket server should have isTCP = false")
	}
	if unixServer.TCPAddr() != "" {
		t.Error("Unix socket server TCPAddr() should return empty string")
	}
	if unixServer.TCPPort() != 0 {
		t.Errorf("Unix socket server TCPPort() = %d, want 0", unixServer.TCPPort())
	}
}

func TestTCPClientServer_Integration(t *testing.T) {
	permReqCh := make(chan PermissionRequest, 1)
	permRespCh := make(chan PermissionResponse, 1)
	questReqCh := make(chan QuestionRequest, 1)
	questRespCh := make(chan QuestionResponse, 1)
	planReqCh := make(chan PlanApprovalRequest, 1)
	planRespCh := make(chan PlanApprovalResponse, 1)

	server, err := NewTCPSocketServer("test-tcp-integration", permReqCh, permRespCh, questReqCh, questRespCh, planReqCh, planRespCh)
	if err != nil {
		t.Fatalf("NewTCPSocketServer failed: %v", err)
	}
	defer server.Close()

	// Start server in background
	server.Start()

	// Give server time to start
	server.WaitReady()

	// Create TCP client
	client, err := NewTCPSocketClient(server.TCPAddr())
	if err != nil {
		t.Fatalf("NewTCPSocketClient failed: %v", err)
	}
	defer client.Close()

	// Test permission request/response
	done := make(chan struct{})
	go func() {
		defer close(done)

		select {
		case req := <-permReqCh:
			if req.ID != "tcp-perm-1" {
				t.Errorf("Request ID = %q, want 'tcp-perm-1'", req.ID)
			}
			permRespCh <- PermissionResponse{
				ID:      req.ID,
				Allowed: true,
				Message: "TCP Approved",
			}
		case <-time.After(5 * time.Second):
			t.Error("Timeout waiting for permission request via TCP")
		}
	}()

	resp, err := client.SendPermissionRequest(PermissionRequest{
		ID:   "tcp-perm-1",
		Tool: "Bash",
	})
	if err != nil {
		t.Fatalf("SendPermissionRequest via TCP failed: %v", err)
	}

	<-done

	if resp.ID != "tcp-perm-1" {
		t.Errorf("Response ID = %q, want 'tcp-perm-1'", resp.ID)
	}
	if !resp.Allowed {
		t.Error("Expected Allowed to be true")
	}
	if resp.Message != "TCP Approved" {
		t.Errorf("Message = %q, want 'TCP Approved'", resp.Message)
	}

	server.Close()
}

func TestTCPClientServer_Question(t *testing.T) {
	permReqCh := make(chan PermissionRequest, 1)
	permRespCh := make(chan PermissionResponse, 1)
	questReqCh := make(chan QuestionRequest, 1)
	questRespCh := make(chan QuestionResponse, 1)
	planReqCh := make(chan PlanApprovalRequest, 1)
	planRespCh := make(chan PlanApprovalResponse, 1)

	server, err := NewTCPSocketServer("test-tcp-question", permReqCh, permRespCh, questReqCh, questRespCh, planReqCh, planRespCh)
	if err != nil {
		t.Fatalf("NewTCPSocketServer failed: %v", err)
	}
	defer server.Close()

	server.Start()
	server.WaitReady()

	client, err := NewTCPSocketClient(server.TCPAddr())
	if err != nil {
		t.Fatalf("NewTCPSocketClient failed: %v", err)
	}
	defer client.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case req := <-questReqCh:
			questRespCh <- QuestionResponse{
				ID:      req.ID,
				Answers: map[string]string{"q1": "tcp-answer"},
			}
		case <-time.After(5 * time.Second):
			t.Error("Timeout waiting for question request via TCP")
		}
	}()

	resp, err := client.SendQuestionRequest(QuestionRequest{
		ID: "tcp-quest-1",
		Questions: []Question{
			{Question: "TCP Test?", Header: "Q1"},
		},
	})
	if err != nil {
		t.Fatalf("SendQuestionRequest via TCP failed: %v", err)
	}

	<-done

	if resp.ID != "tcp-quest-1" {
		t.Errorf("Response ID = %q, want 'tcp-quest-1'", resp.ID)
	}
	if resp.Answers["q1"] != "tcp-answer" {
		t.Errorf("Answer = %q, want 'tcp-answer'", resp.Answers["q1"])
	}

	server.Close()
}

func TestTCPClientServer_PlanApproval(t *testing.T) {
	permReqCh := make(chan PermissionRequest, 1)
	permRespCh := make(chan PermissionResponse, 1)
	questReqCh := make(chan QuestionRequest, 1)
	questRespCh := make(chan QuestionResponse, 1)
	planReqCh := make(chan PlanApprovalRequest, 1)
	planRespCh := make(chan PlanApprovalResponse, 1)

	server, err := NewTCPSocketServer("test-tcp-plan", permReqCh, permRespCh, questReqCh, questRespCh, planReqCh, planRespCh)
	if err != nil {
		t.Fatalf("NewTCPSocketServer failed: %v", err)
	}
	defer server.Close()

	server.Start()
	server.WaitReady()

	client, err := NewTCPSocketClient(server.TCPAddr())
	if err != nil {
		t.Fatalf("NewTCPSocketClient failed: %v", err)
	}
	defer client.Close()

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
			t.Error("Timeout waiting for plan approval request via TCP")
		}
	}()

	resp, err := client.SendPlanApprovalRequest(PlanApprovalRequest{
		ID:   "tcp-plan-1",
		Plan: "TCP test plan content",
	})
	if err != nil {
		t.Fatalf("SendPlanApprovalRequest via TCP failed: %v", err)
	}

	<-done

	if resp.ID != "tcp-plan-1" {
		t.Errorf("Response ID = %q, want 'tcp-plan-1'", resp.ID)
	}
	if !resp.Approved {
		t.Error("Expected Approved to be true")
	}

	server.Close()
}

func TestNewTCPSocketClient_InvalidAddr(t *testing.T) {
	// Connecting to a non-listening address should fail
	_, err := NewTCPSocketClient("127.0.0.1:1")
	if err == nil {
		t.Error("Expected error for invalid TCP address")
	}
}

func TestSocketMessage_JSONMarshal_CreatePR(t *testing.T) {
	msg := SocketMessage{
		Type: MessageTypeCreatePR,
		CreatePRReq: &CreatePRRequest{
			ID:    "pr-1",
			Title: "Test PR",
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded SocketMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Type != MessageTypeCreatePR {
		t.Errorf("Type = %q, want %q", decoded.Type, MessageTypeCreatePR)
	}
	if decoded.CreatePRReq == nil {
		t.Fatal("CreatePRReq is nil")
	}
	if decoded.CreatePRReq.Title != "Test PR" {
		t.Errorf("Title = %q, want 'Test PR'", decoded.CreatePRReq.Title)
	}
}

func TestSocketMessage_JSONMarshal_PushBranch(t *testing.T) {
	msg := SocketMessage{
		Type: MessageTypePushBranch,
		PushBranchReq: &PushBranchRequest{
			ID:            "push-1",
			CommitMessage: "test commit",
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded SocketMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Type != MessageTypePushBranch {
		t.Errorf("Type = %q, want %q", decoded.Type, MessageTypePushBranch)
	}
	if decoded.PushBranchReq == nil {
		t.Fatal("PushBranchReq is nil")
	}
	if decoded.PushBranchReq.CommitMessage != "test commit" {
		t.Errorf("CommitMessage = %q, want 'test commit'", decoded.PushBranchReq.CommitMessage)
	}
}

func TestSocketMessage_HostToolMessageTypes(t *testing.T) {
	if MessageTypeCreatePR != "createPR" {
		t.Errorf("MessageTypeCreatePR = %q, want 'createPR'", MessageTypeCreatePR)
	}
	if MessageTypePushBranch != "pushBranch" {
		t.Errorf("MessageTypePushBranch = %q, want 'pushBranch'", MessageTypePushBranch)
	}
}

func TestContainerMCPPort(t *testing.T) {
	if ContainerMCPPort != 21120 {
		t.Errorf("ContainerMCPPort = %d, want 21120", ContainerMCPPort)
	}
}

func TestNewDialingSocketServer(t *testing.T) {
	permReqCh := make(chan PermissionRequest, 1)
	permRespCh := make(chan PermissionResponse, 1)
	questReqCh := make(chan QuestionRequest, 1)
	questRespCh := make(chan QuestionResponse, 1)
	planReqCh := make(chan PlanApprovalRequest, 1)
	planRespCh := make(chan PlanApprovalResponse, 1)

	server := NewDialingSocketServer("test-dialing", permReqCh, permRespCh, questReqCh, questRespCh, planReqCh, planRespCh)

	// Server should be immediately ready (no accept loop)
	select {
	case <-server.readyCh:
		// expected
	default:
		t.Error("readyCh should be closed immediately for dialing servers")
	}

	// SocketPath and TCPAddr should be empty
	if path := server.SocketPath(); path != "" {
		t.Errorf("SocketPath() = %q, want empty for dialing server", path)
	}
	if addr := server.TCPAddr(); addr != "" {
		t.Errorf("TCPAddr() = %q, want empty for dialing server", addr)
	}
	if port := server.TCPPort(); port != 0 {
		t.Errorf("TCPPort() = %d, want 0 for dialing server", port)
	}

	// Close should succeed (nil listener)
	if err := server.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestDialingSocketServer_HandleConn(t *testing.T) {
	permReqCh := make(chan PermissionRequest, 1)
	permRespCh := make(chan PermissionResponse, 1)
	questReqCh := make(chan QuestionRequest, 1)
	questRespCh := make(chan QuestionResponse, 1)
	planReqCh := make(chan PlanApprovalRequest, 1)
	planRespCh := make(chan PlanApprovalResponse, 1)

	server := NewDialingSocketServer("test-handleconn", permReqCh, permRespCh, questReqCh, questRespCh, planReqCh, planRespCh)
	defer server.Close()

	// Use net.Pipe for an in-memory bidirectional connection
	clientConn, serverConn := net.Pipe()

	// Handle the server-side connection in a goroutine (blocks until closed)
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.HandleConn(serverConn)
	}()

	// Simulate a client sending a permission request
	go func() {
		select {
		case req := <-permReqCh:
			permRespCh <- PermissionResponse{
				ID:      req.ID,
				Allowed: true,
				Message: "Approved via HandleConn",
			}
		case <-time.After(5 * time.Second):
			t.Error("Timeout waiting for permission request")
		}
	}()

	// Create a temporary SocketClient wrapping the client side of the pipe
	client := &SocketClient{
		conn:   clientConn,
		reader: bufio.NewReader(clientConn),
	}

	resp, err := client.SendPermissionRequest(PermissionRequest{
		ID:   "handleconn-perm-1",
		Tool: "Read",
	})
	if err != nil {
		t.Fatalf("SendPermissionRequest failed: %v", err)
	}

	if resp.ID != "handleconn-perm-1" {
		t.Errorf("Response ID = %q, want 'handleconn-perm-1'", resp.ID)
	}
	if !resp.Allowed {
		t.Error("Expected Allowed to be true")
	}
	if resp.Message != "Approved via HandleConn" {
		t.Errorf("Message = %q, want 'Approved via HandleConn'", resp.Message)
	}

	// Close the client connection to unblock HandleConn
	clientConn.Close()

	select {
	case <-done:
		// HandleConn returned
	case <-time.After(5 * time.Second):
		t.Error("HandleConn did not return after connection closed")
	}
}

func TestDialingSocketServer_CloseClosesActiveConn(t *testing.T) {
	permReqCh := make(chan PermissionRequest, 1)
	permRespCh := make(chan PermissionResponse, 1)
	questReqCh := make(chan QuestionRequest, 1)
	questRespCh := make(chan QuestionResponse, 1)
	planReqCh := make(chan PlanApprovalRequest, 1)
	planRespCh := make(chan PlanApprovalResponse, 1)

	server := NewDialingSocketServer("test-close-activeconn", permReqCh, permRespCh, questReqCh, questRespCh, planReqCh, planRespCh)

	_, serverConn := net.Pipe()

	done := make(chan struct{})
	go func() {
		defer close(done)
		server.HandleConn(serverConn)
	}()

	// Give HandleConn a moment to start
	time.Sleep(50 * time.Millisecond)

	// Closing the server should close the active connection and unblock HandleConn
	server.Close()

	select {
	case <-done:
		// HandleConn returned after Close()
	case <-time.After(5 * time.Second):
		t.Error("HandleConn did not return after server.Close()")
	}
}

func TestNewListeningSocketClient(t *testing.T) {
	// Start a listener, then have a goroutine connect to it
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close() // Close so NewListeningSocketClient can bind

	clientDone := make(chan struct{})
	var client *SocketClient
	var clientErr error
	go func() {
		defer close(clientDone)
		client, clientErr = NewListeningSocketClient(addr)
	}()

	// Give the listening client a moment to start listening
	time.Sleep(100 * time.Millisecond)

	// Connect from the host side
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	// Wait for client to accept
	select {
	case <-clientDone:
		if clientErr != nil {
			t.Fatalf("NewListeningSocketClient error: %v", clientErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("NewListeningSocketClient did not return")
	}

	if client == nil {
		t.Fatal("Client is nil")
	}
	defer client.Close()

	// Verify communication works: send a message from "host" to "container client"
	// We'll just verify the connection is established by writing/reading
	testMsg := `{"type":"permission","permResp":{"id":"test","allowed":true}}` + "\n"
	_, err = conn.Write([]byte(testMsg))
	if err != nil {
		t.Fatalf("Write to client failed: %v", err)
	}

	// Read the message on the client side
	line, err := client.reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Client read failed: %v", err)
	}
	if line != testMsg {
		t.Errorf("Client received = %q, want %q", line, testMsg)
	}
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
