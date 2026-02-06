package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/zhubert/plural/internal/logger"
)

// Socket communication constants
const (
	// PermissionResponseTimeout is the maximum time to wait for a permission response
	PermissionResponseTimeout = 5 * time.Minute

	// SocketReadTimeout is the timeout for reading from the socket
	SocketReadTimeout = 10 * time.Second

	// SocketWriteTimeout is the timeout for writing to the socket.
	// This prevents the MCP server subprocess from blocking indefinitely
	// if the TUI becomes unresponsive.
	SocketWriteTimeout = 10 * time.Second
)

// MessageType identifies the type of socket message
type MessageType string

const (
	MessageTypePermission   MessageType = "permission"
	MessageTypeQuestion     MessageType = "question"
	MessageTypePlanApproval MessageType = "planApproval"
)

// SocketMessage wraps permission, question, or plan approval requests/responses
type SocketMessage struct {
	Type      MessageType          `json:"type"`
	PermReq   *PermissionRequest   `json:"permReq,omitempty"`
	PermResp  *PermissionResponse  `json:"permResp,omitempty"`
	QuestReq  *QuestionRequest     `json:"questReq,omitempty"`
	QuestResp *QuestionResponse    `json:"questResp,omitempty"`
	PlanReq   *PlanApprovalRequest  `json:"planReq,omitempty"`
	PlanResp  *PlanApprovalResponse `json:"planResp,omitempty"`
}

// SocketServer listens for permission requests from MCP server subprocesses
type SocketServer struct {
	socketPath    string
	listener      net.Listener
	requestCh     chan<- PermissionRequest
	responseCh    <-chan PermissionResponse
	questionCh    chan<- QuestionRequest
	answerCh      <-chan QuestionResponse
	planReqCh     chan<- PlanApprovalRequest
	planRespCh    <-chan PlanApprovalResponse
	closed        bool         // Set to true when Close() is called
	closedMu      sync.RWMutex // Guards closed flag
	log           *slog.Logger // Logger with session context
}

// NewSocketServer creates a new socket server for the given session
func NewSocketServer(sessionID string, reqCh chan<- PermissionRequest, respCh <-chan PermissionResponse, questCh chan<- QuestionRequest, ansCh <-chan QuestionResponse, planReqCh chan<- PlanApprovalRequest, planRespCh <-chan PlanApprovalResponse) (*SocketServer, error) {
	socketPath := filepath.Join(os.TempDir(), "plural-"+sessionID+".sock")
	log := logger.WithSession(sessionID).With("component", "mcp-socket")

	// Remove existing socket if present
	os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}

	log.Info("listening", "socketPath", socketPath)

	return &SocketServer{
		socketPath: socketPath,
		listener:   listener,
		requestCh:  reqCh,
		responseCh: respCh,
		questionCh: questCh,
		answerCh:   ansCh,
		planReqCh:  planReqCh,
		planRespCh: planRespCh,
		log:        log,
	}, nil
}

// SocketPath returns the path to the socket
func (s *SocketServer) SocketPath() string {
	return s.socketPath
}

// Run starts accepting connections
func (s *SocketServer) Run() {
	for {
		// Check if we're closed before accepting
		s.closedMu.RLock()
		closed := s.closed
		s.closedMu.RUnlock()
		if closed {
			s.log.Info("server closed, stopping accept loop")
			return
		}

		conn, err := s.listener.Accept()
		if err != nil {
			// Check if the listener was closed (expected during shutdown)
			s.closedMu.RLock()
			closed := s.closed
			s.closedMu.RUnlock()
			if closed {
				s.log.Info("listener closed during shutdown, stopping")
				return
			}
			if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
				s.log.Info("listener closed, stopping")
				return
			}
			// Log error but continue accepting connections
			s.log.Warn("accept error (continuing)", "error", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *SocketServer) handleConnection(conn net.Conn) {
	defer conn.Close()
	s.log.Debug("connection accepted")

	reader := bufio.NewReader(conn)

	for {
		// Check if server is closed before waiting for data
		s.closedMu.RLock()
		closed := s.closed
		s.closedMu.RUnlock()
		if closed {
			s.log.Debug("server closed, closing connection handler")
			return
		}

		// Set read deadline
		conn.SetReadDeadline(time.Now().Add(SocketReadTimeout))

		// Read message
		line, err := reader.ReadString('\n')
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout is expected - check if server was closed during timeout
				s.closedMu.RLock()
				closed := s.closed
				s.closedMu.RUnlock()
				if closed {
					s.log.Debug("server closed during timeout, exiting handler")
					return
				}
				// Server still running, continue waiting for messages
				continue
			}
			s.log.Error("read error", "error", err)
			return
		}

		var msg SocketMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			s.log.Error("JSON parse error", "error", err)
			continue
		}

		switch msg.Type {
		case MessageTypePermission:
			s.handlePermissionMessage(conn, msg.PermReq)
		case MessageTypeQuestion:
			s.handleQuestionMessage(conn, msg.QuestReq)
		case MessageTypePlanApproval:
			s.handlePlanApprovalMessage(conn, msg.PlanReq)
		default:
			s.log.Warn("unknown message type", "type", msg.Type)
		}
	}
}

func (s *SocketServer) handlePermissionMessage(conn net.Conn, req *PermissionRequest) {
	if req == nil {
		s.log.Warn("nil permission request, sending deny response")
		// Send a deny response to prevent client from hanging
		s.sendPermissionResponse(conn, PermissionResponse{
			Allowed: false,
			Message: "Invalid permission request",
		})
		return
	}

	s.log.Info("received permission request", "tool", req.Tool)

	// Send to TUI (non-blocking with timeout)
	select {
	case s.requestCh <- *req:
		// Request sent successfully
	case <-time.After(SocketReadTimeout):
		s.log.Warn("timeout sending permission request to TUI")
		s.sendPermissionResponse(conn, PermissionResponse{
			ID:      req.ID,
			Allowed: false,
			Message: "Timeout waiting for TUI",
		})
		return
	}

	// Wait for response with timeout
	select {
	case resp := <-s.responseCh:
		s.sendPermissionResponse(conn, resp)
		s.log.Info("sent permission response", "allowed", resp.Allowed)

	case <-time.After(PermissionResponseTimeout):
		s.log.Warn("timeout waiting for permission response")
		s.sendPermissionResponse(conn, PermissionResponse{
			ID:      req.ID,
			Allowed: false,
			Message: "Permission request timed out",
		})
	}
}

func (s *SocketServer) handleQuestionMessage(conn net.Conn, req *QuestionRequest) {
	if req == nil {
		s.log.Warn("nil question request, sending empty response")
		// Send an empty response to prevent client from hanging
		s.sendQuestionResponse(conn, QuestionResponse{
			Answers: map[string]string{},
		})
		return
	}

	s.log.Info("received question request", "questionCount", len(req.Questions))

	// Send to TUI (non-blocking with timeout)
	select {
	case s.questionCh <- *req:
		// Request sent successfully
	case <-time.After(SocketReadTimeout):
		s.log.Warn("timeout sending question request to TUI")
		s.sendQuestionResponse(conn, QuestionResponse{
			ID:      req.ID,
			Answers: map[string]string{},
		})
		return
	}

	// Wait for response with timeout
	select {
	case resp := <-s.answerCh:
		s.sendQuestionResponse(conn, resp)
		s.log.Info("sent question response", "answerCount", len(resp.Answers))

	case <-time.After(PermissionResponseTimeout):
		s.log.Warn("timeout waiting for question response")
		s.sendQuestionResponse(conn, QuestionResponse{
			ID:      req.ID,
			Answers: map[string]string{},
		})
	}
}

func (s *SocketServer) sendPermissionResponse(conn net.Conn, resp PermissionResponse) {
	msg := SocketMessage{
		Type:     MessageTypePermission,
		PermResp: &resp,
	}
	respJSON, err := json.Marshal(msg)
	if err != nil {
		s.log.Error("failed to marshal permission response", "error", err)
		return
	}

	conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	if _, err := conn.Write(append(respJSON, '\n')); err != nil {
		s.log.Error("write error", "error", err)
	}
}

func (s *SocketServer) sendQuestionResponse(conn net.Conn, resp QuestionResponse) {
	msg := SocketMessage{
		Type:      MessageTypeQuestion,
		QuestResp: &resp,
	}
	respJSON, err := json.Marshal(msg)
	if err != nil {
		s.log.Error("failed to marshal question response", "error", err)
		return
	}

	conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	if _, err := conn.Write(append(respJSON, '\n')); err != nil {
		s.log.Error("write error", "error", err)
	}
}

func (s *SocketServer) handlePlanApprovalMessage(conn net.Conn, req *PlanApprovalRequest) {
	if req == nil {
		s.log.Warn("nil plan approval request, sending reject response")
		s.sendPlanApprovalResponse(conn, PlanApprovalResponse{
			Approved: false,
		})
		return
	}

	s.log.Info("received plan approval request", "planLength", len(req.Plan))

	// Send to TUI (non-blocking with timeout)
	select {
	case s.planReqCh <- *req:
		// Request sent successfully
	case <-time.After(SocketReadTimeout):
		s.log.Warn("timeout sending plan approval request to TUI")
		s.sendPlanApprovalResponse(conn, PlanApprovalResponse{
			ID:       req.ID,
			Approved: false,
		})
		return
	}

	// Wait for response with timeout
	select {
	case resp := <-s.planRespCh:
		s.sendPlanApprovalResponse(conn, resp)
		s.log.Info("sent plan approval response", "approved", resp.Approved)

	case <-time.After(PermissionResponseTimeout):
		s.log.Warn("timeout waiting for plan approval response")
		s.sendPlanApprovalResponse(conn, PlanApprovalResponse{
			ID:       req.ID,
			Approved: false,
		})
	}
}

func (s *SocketServer) sendPlanApprovalResponse(conn net.Conn, resp PlanApprovalResponse) {
	msg := SocketMessage{
		Type:     MessageTypePlanApproval,
		PlanResp: &resp,
	}
	respJSON, err := json.Marshal(msg)
	if err != nil {
		s.log.Error("failed to marshal plan approval response", "error", err)
		return
	}

	conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	if _, err := conn.Write(append(respJSON, '\n')); err != nil {
		s.log.Error("write error", "error", err)
	}
}

// Close shuts down the socket server
func (s *SocketServer) Close() error {
	s.log.Info("closing socket server")

	// Mark as closed BEFORE closing listener to signal Run() goroutine to exit
	s.closedMu.Lock()
	s.closed = true
	s.closedMu.Unlock()

	// Close listener (this will unblock Accept())
	err := s.listener.Close()

	// Remove socket file, logging any errors
	if removeErr := os.Remove(s.socketPath); removeErr != nil && !os.IsNotExist(removeErr) {
		s.log.Warn("failed to remove socket file", "socketPath", s.socketPath, "error", removeErr)
	}

	return err
}

// SocketClient connects to the TUI's socket server (used by MCP server subprocess)
type SocketClient struct {
	socketPath string
	conn       net.Conn
	reader     *bufio.Reader
}

// NewSocketClient creates a client connected to the TUI socket
func NewSocketClient(socketPath string) (*SocketClient, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, err
	}

	return &SocketClient{
		socketPath: socketPath,
		conn:       conn,
		reader:     bufio.NewReader(conn),
	}, nil
}

// SendPermissionRequest sends a permission request and waits for response
func (c *SocketClient) SendPermissionRequest(req PermissionRequest) (PermissionResponse, error) {
	msg := SocketMessage{
		Type:    MessageTypePermission,
		PermReq: &req,
	}

	// Send request with write timeout
	reqJSON, err := json.Marshal(msg)
	if err != nil {
		return PermissionResponse{}, err
	}

	c.conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	_, err = c.conn.Write(append(reqJSON, '\n'))
	if err != nil {
		return PermissionResponse{}, fmt.Errorf("write permission request: %w", err)
	}

	// Read response (no timeout - user may take a while to respond)
	c.conn.SetReadDeadline(time.Time{}) // Clear any deadline
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return PermissionResponse{}, fmt.Errorf("read permission response: %w", err)
	}

	var respMsg SocketMessage
	if err := json.Unmarshal([]byte(line), &respMsg); err != nil {
		return PermissionResponse{}, err
	}

	if respMsg.PermResp == nil {
		return PermissionResponse{}, fmt.Errorf("expected permission response, got nil")
	}

	return *respMsg.PermResp, nil
}

// SendQuestionRequest sends a question request and waits for response
func (c *SocketClient) SendQuestionRequest(req QuestionRequest) (QuestionResponse, error) {
	msg := SocketMessage{
		Type:     MessageTypeQuestion,
		QuestReq: &req,
	}

	// Send request with write timeout
	reqJSON, err := json.Marshal(msg)
	if err != nil {
		return QuestionResponse{}, err
	}

	c.conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	_, err = c.conn.Write(append(reqJSON, '\n'))
	if err != nil {
		return QuestionResponse{}, fmt.Errorf("write question request: %w", err)
	}

	// Read response (no timeout - user may take a while to respond)
	c.conn.SetReadDeadline(time.Time{}) // Clear any deadline
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return QuestionResponse{}, fmt.Errorf("read question response: %w", err)
	}

	var respMsg SocketMessage
	if err := json.Unmarshal([]byte(line), &respMsg); err != nil {
		return QuestionResponse{}, err
	}

	if respMsg.QuestResp == nil {
		return QuestionResponse{}, fmt.Errorf("expected question response, got nil")
	}

	return *respMsg.QuestResp, nil
}

// SendPlanApprovalRequest sends a plan approval request and waits for response
func (c *SocketClient) SendPlanApprovalRequest(req PlanApprovalRequest) (PlanApprovalResponse, error) {
	msg := SocketMessage{
		Type:    MessageTypePlanApproval,
		PlanReq: &req,
	}

	// Send request with write timeout
	reqJSON, err := json.Marshal(msg)
	if err != nil {
		return PlanApprovalResponse{}, err
	}

	c.conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	_, err = c.conn.Write(append(reqJSON, '\n'))
	if err != nil {
		return PlanApprovalResponse{}, fmt.Errorf("write plan approval request: %w", err)
	}

	// Read response (no timeout - user may take a while to respond)
	c.conn.SetReadDeadline(time.Time{}) // Clear any deadline
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return PlanApprovalResponse{}, fmt.Errorf("read plan approval response: %w", err)
	}

	var respMsg SocketMessage
	if err := json.Unmarshal([]byte(line), &respMsg); err != nil {
		return PlanApprovalResponse{}, err
	}

	if respMsg.PlanResp == nil {
		return PlanApprovalResponse{}, fmt.Errorf("expected plan approval response, got nil")
	}

	return *respMsg.PlanResp, nil
}

// Close closes the client connection
func (c *SocketClient) Close() error {
	return c.conn.Close()
}
