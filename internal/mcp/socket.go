package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
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

	// HostToolResponseTimeout is the timeout for host tool operations (create_pr, push_branch).
	// These operations involve git pushes and GitHub API calls which can take longer
	// than interactive prompts. Must be >= the 2-minute context timeout in TUI handlers.
	HostToolResponseTimeout = 5 * time.Minute
)

// MessageType identifies the type of socket message
type MessageType string

const (
	MessageTypePermission        MessageType = "permission"
	MessageTypeQuestion          MessageType = "question"
	MessageTypePlanApproval      MessageType = "planApproval"
	MessageTypeCreateChild       MessageType = "createChild"
	MessageTypeListChildren      MessageType = "listChildren"
	MessageTypeMergeChild        MessageType = "mergeChild"
	MessageTypeCreatePR          MessageType = "createPR"
	MessageTypePushBranch        MessageType = "pushBranch"
	MessageTypeGetReviewComments MessageType = "getReviewComments"
)

// SocketMessage wraps permission, question, plan approval, or supervisor requests/responses
type SocketMessage struct {
	Type                  MessageType                  `json:"type"`
	PermReq               *PermissionRequest           `json:"permReq,omitempty"`
	PermResp              *PermissionResponse          `json:"permResp,omitempty"`
	QuestReq              *QuestionRequest             `json:"questReq,omitempty"`
	QuestResp             *QuestionResponse            `json:"questResp,omitempty"`
	PlanReq               *PlanApprovalRequest         `json:"planReq,omitempty"`
	PlanResp              *PlanApprovalResponse        `json:"planResp,omitempty"`
	CreateChildReq        *CreateChildRequest          `json:"createChildReq,omitempty"`
	CreateChildResp       *CreateChildResponse         `json:"createChildResp,omitempty"`
	ListChildrenReq       *ListChildrenRequest         `json:"listChildrenReq,omitempty"`
	ListChildrenResp      *ListChildrenResponse        `json:"listChildrenResp,omitempty"`
	MergeChildReq         *MergeChildRequest           `json:"mergeChildReq,omitempty"`
	MergeChildResp        *MergeChildResponse          `json:"mergeChildResp,omitempty"`
	CreatePRReq           *CreatePRRequest             `json:"createPRReq,omitempty"`
	CreatePRResp          *CreatePRResponse            `json:"createPRResp,omitempty"`
	PushBranchReq         *PushBranchRequest           `json:"pushBranchReq,omitempty"`
	PushBranchResp        *PushBranchResponse          `json:"pushBranchResp,omitempty"`
	GetReviewCommentsReq  *GetReviewCommentsRequest    `json:"getReviewCommentsReq,omitempty"`
	GetReviewCommentsResp *GetReviewCommentsResponse   `json:"getReviewCommentsResp,omitempty"`
}

// SocketServer listens for permission requests from MCP server subprocesses
type SocketServer struct {
	socketPath       string // Unix socket path (empty for TCP servers)
	listener         net.Listener
	isTCP            bool // True if listening on TCP instead of Unix socket
	requestCh        chan<- PermissionRequest
	responseCh       <-chan PermissionResponse
	questionCh       chan<- QuestionRequest
	answerCh         <-chan QuestionResponse
	planReqCh        chan<- PlanApprovalRequest
	planRespCh       <-chan PlanApprovalResponse
	createChildReq   chan<- CreateChildRequest
	createChildResp  <-chan CreateChildResponse
	listChildrenReq  chan<- ListChildrenRequest
	listChildrenResp <-chan ListChildrenResponse
	mergeChildReq         chan<- MergeChildRequest
	mergeChildResp        <-chan MergeChildResponse
	createPRReq           chan<- CreatePRRequest
	createPRResp          <-chan CreatePRResponse
	pushBranchReq         chan<- PushBranchRequest
	pushBranchResp        <-chan PushBranchResponse
	getReviewCommentsReq  chan<- GetReviewCommentsRequest
	getReviewCommentsResp <-chan GetReviewCommentsResponse
	closed                bool           // Set to true when Close() is called
	closedMu              sync.RWMutex   // Guards closed flag
	wg                    sync.WaitGroup // Tracks the Run() goroutine for clean shutdown
	log                   *slog.Logger   // Logger with session context
}

// NewSocketServer creates a new socket server for the given session
func NewSocketServer(sessionID string, reqCh chan<- PermissionRequest, respCh <-chan PermissionResponse, questCh chan<- QuestionRequest, ansCh <-chan QuestionResponse, planReqCh chan<- PlanApprovalRequest, planRespCh <-chan PlanApprovalResponse, opts ...SocketServerOption) (*SocketServer, error) {
	// Use abbreviated session ID (first 12 chars) in the socket path to keep
	// it short. Unix domain socket paths have a max of ~104 characters.
	// 12 hex chars gives ~2^48 combinations, making collisions negligible.
	shortID := sessionID
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	socketPath := filepath.Join(os.TempDir(), "pl-"+shortID+".sock")
	log := logger.WithSession(sessionID).With("component", "mcp-socket")

	// Remove existing socket if present
	os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}

	log.Info("listening", "socketPath", socketPath)

	s := &SocketServer{
		socketPath: socketPath,
		listener:   listener,
		requestCh:  reqCh,
		responseCh: respCh,
		questionCh: questCh,
		answerCh:   ansCh,
		planReqCh:  planReqCh,
		planRespCh: planRespCh,
		log:        log,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// SocketServerOption is a functional option for configuring SocketServer
type SocketServerOption func(*SocketServer)

// WithSupervisorChannels sets the supervisor tool channels on a SocketServer
func WithSupervisorChannels(
	createChildReq chan<- CreateChildRequest, createChildResp <-chan CreateChildResponse,
	listChildrenReq chan<- ListChildrenRequest, listChildrenResp <-chan ListChildrenResponse,
	mergeChildReq chan<- MergeChildRequest, mergeChildResp <-chan MergeChildResponse,
) SocketServerOption {
	return func(s *SocketServer) {
		s.createChildReq = createChildReq
		s.createChildResp = createChildResp
		s.listChildrenReq = listChildrenReq
		s.listChildrenResp = listChildrenResp
		s.mergeChildReq = mergeChildReq
		s.mergeChildResp = mergeChildResp
	}
}

// WithHostToolChannels sets the host tool channels on a SocketServer
func WithHostToolChannels(
	createPRReq chan<- CreatePRRequest, createPRResp <-chan CreatePRResponse,
	pushBranchReq chan<- PushBranchRequest, pushBranchResp <-chan PushBranchResponse,
	getReviewCommentsReq chan<- GetReviewCommentsRequest, getReviewCommentsResp <-chan GetReviewCommentsResponse,
) SocketServerOption {
	return func(s *SocketServer) {
		s.createPRReq = createPRReq
		s.createPRResp = createPRResp
		s.pushBranchReq = pushBranchReq
		s.pushBranchResp = pushBranchResp
		s.getReviewCommentsReq = getReviewCommentsReq
		s.getReviewCommentsResp = getReviewCommentsResp
	}
}

// NewTCPSocketServer creates a socket server that listens on TCP instead of a
// Unix socket. Used for container sessions where Unix sockets can't cross the
// Docker container boundary.
//
// Bind address selection:
//   - macOS/Windows: 127.0.0.1 — Docker Desktop routes host.docker.internal
//     through the VM to the host's loopback, so localhost binding works.
//   - Linux: 0.0.0.0 — Docker bridge networking requires the host to listen
//     on an interface reachable from the bridge (host-gateway maps to the
//     bridge gateway IP, not 127.0.0.1). The port is ephemeral and short-lived.
func NewTCPSocketServer(sessionID string, reqCh chan<- PermissionRequest, respCh <-chan PermissionResponse, questCh chan<- QuestionRequest, ansCh <-chan QuestionResponse, planReqCh chan<- PlanApprovalRequest, planRespCh <-chan PlanApprovalResponse, opts ...SocketServerOption) (*SocketServer, error) {
	log := logger.WithSession(sessionID).With("component", "mcp-socket")

	// On macOS/Windows (Docker Desktop), bind to loopback for security.
	// On Linux, bind to all interfaces since host.docker.internal resolves
	// to the bridge gateway IP, not 127.0.0.1. Note: the ephemeral port is
	// exposed to the local network on Linux for the lifetime of the session.
	// This is acceptable for local development; production deployments on
	// shared Linux hosts should use firewall rules to restrict access.
	bindAddr := "127.0.0.1:0"
	if runtime.GOOS == "linux" {
		bindAddr = "0.0.0.0:0"
	}
	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return nil, err
	}

	addr := listener.Addr().(*net.TCPAddr)
	log.Info("listening on TCP", "addr", addr.String(), "port", addr.Port)

	s := &SocketServer{
		listener:   listener,
		isTCP:      true,
		requestCh:  reqCh,
		responseCh: respCh,
		questionCh: questCh,
		answerCh:   ansCh,
		planReqCh:  planReqCh,
		planRespCh: planRespCh,
		log:        log,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// SocketPath returns the path to the socket
func (s *SocketServer) SocketPath() string {
	return s.socketPath
}

// TCPAddr returns the TCP address the server is listening on.
// Returns empty string if not a TCP server.
func (s *SocketServer) TCPAddr() string {
	if !s.isTCP {
		return ""
	}
	return s.listener.Addr().String()
}

// TCPPort returns just the port number for TCP servers.
// Returns 0 if not a TCP server.
func (s *SocketServer) TCPPort() int {
	if !s.isTCP {
		return 0
	}
	if addr, ok := s.listener.Addr().(*net.TCPAddr); ok {
		return addr.Port
	}
	return 0
}

// Start launches Run() in a goroutine. It increments the WaitGroup before
// starting the goroutine to avoid a race with Close()/wg.Wait().
func (s *SocketServer) Start() {
	s.wg.Add(1)
	go s.Run()
}

// Run starts accepting connections. Must be paired with a wg.Add(1) call
// before the goroutine is launched — use Start() instead of calling go Run() directly.
func (s *SocketServer) Run() {
	defer s.wg.Done()

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
		case MessageTypeCreateChild:
			s.handleCreateChildMessage(conn, msg.CreateChildReq)
		case MessageTypeListChildren:
			s.handleListChildrenMessage(conn, msg.ListChildrenReq)
		case MessageTypeMergeChild:
			s.handleMergeChildMessage(conn, msg.MergeChildReq)
		case MessageTypeCreatePR:
			s.handleCreatePRMessage(conn, msg.CreatePRReq)
		case MessageTypePushBranch:
			s.handlePushBranchMessage(conn, msg.PushBranchReq)
		case MessageTypeGetReviewComments:
			s.handleGetReviewCommentsMessage(conn, msg.GetReviewCommentsReq)
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

func (s *SocketServer) handleCreateChildMessage(conn net.Conn, req *CreateChildRequest) {
	if req == nil || s.createChildReq == nil {
		s.log.Warn("create child request ignored (nil request or no channel)")
		s.sendCreateChildResponse(conn, CreateChildResponse{Success: false, Error: "Supervisor tools not available"})
		return
	}

	s.log.Info("received create child request", "task", req.Task)

	select {
	case s.createChildReq <- *req:
	case <-time.After(SocketReadTimeout):
		s.log.Warn("timeout sending create child request to TUI")
		s.sendCreateChildResponse(conn, CreateChildResponse{ID: req.ID, Success: false, Error: "Timeout waiting for TUI"})
		return
	}

	select {
	case resp := <-s.createChildResp:
		s.sendCreateChildResponse(conn, resp)
		s.log.Info("sent create child response", "success", resp.Success, "childID", resp.ChildID)
	case <-time.After(PermissionResponseTimeout):
		s.log.Warn("timeout waiting for create child response")
		s.sendCreateChildResponse(conn, CreateChildResponse{ID: req.ID, Success: false, Error: "Timeout"})
	}
}

func (s *SocketServer) sendCreateChildResponse(conn net.Conn, resp CreateChildResponse) {
	msg := SocketMessage{Type: MessageTypeCreateChild, CreateChildResp: &resp}
	respJSON, err := json.Marshal(msg)
	if err != nil {
		s.log.Error("failed to marshal create child response", "error", err)
		return
	}
	conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	if _, err := conn.Write(append(respJSON, '\n')); err != nil {
		s.log.Error("write error", "error", err)
	}
}

func (s *SocketServer) handleListChildrenMessage(conn net.Conn, req *ListChildrenRequest) {
	if req == nil || s.listChildrenReq == nil {
		s.log.Warn("list children request ignored (nil request or no channel)")
		s.sendListChildrenResponse(conn, ListChildrenResponse{Children: []ChildSessionInfo{}})
		return
	}

	s.log.Info("received list children request")

	select {
	case s.listChildrenReq <- *req:
	case <-time.After(SocketReadTimeout):
		s.log.Warn("timeout sending list children request to TUI")
		s.sendListChildrenResponse(conn, ListChildrenResponse{ID: req.ID, Children: []ChildSessionInfo{}})
		return
	}

	select {
	case resp := <-s.listChildrenResp:
		s.sendListChildrenResponse(conn, resp)
		s.log.Info("sent list children response", "count", len(resp.Children))
	case <-time.After(PermissionResponseTimeout):
		s.log.Warn("timeout waiting for list children response")
		s.sendListChildrenResponse(conn, ListChildrenResponse{ID: req.ID, Children: []ChildSessionInfo{}})
	}
}

func (s *SocketServer) sendListChildrenResponse(conn net.Conn, resp ListChildrenResponse) {
	msg := SocketMessage{Type: MessageTypeListChildren, ListChildrenResp: &resp}
	respJSON, err := json.Marshal(msg)
	if err != nil {
		s.log.Error("failed to marshal list children response", "error", err)
		return
	}
	conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	if _, err := conn.Write(append(respJSON, '\n')); err != nil {
		s.log.Error("write error", "error", err)
	}
}

func (s *SocketServer) handleMergeChildMessage(conn net.Conn, req *MergeChildRequest) {
	if req == nil || s.mergeChildReq == nil {
		s.log.Warn("merge child request ignored (nil request or no channel)")
		s.sendMergeChildResponse(conn, MergeChildResponse{Success: false, Error: "Supervisor tools not available"})
		return
	}

	s.log.Info("received merge child request", "childSessionID", req.ChildSessionID)

	select {
	case s.mergeChildReq <- *req:
	case <-time.After(SocketReadTimeout):
		s.log.Warn("timeout sending merge child request to TUI")
		s.sendMergeChildResponse(conn, MergeChildResponse{ID: req.ID, Success: false, Error: "Timeout waiting for TUI"})
		return
	}

	select {
	case resp := <-s.mergeChildResp:
		s.sendMergeChildResponse(conn, resp)
		s.log.Info("sent merge child response", "success", resp.Success)
	case <-time.After(PermissionResponseTimeout):
		s.log.Warn("timeout waiting for merge child response")
		s.sendMergeChildResponse(conn, MergeChildResponse{ID: req.ID, Success: false, Error: "Timeout"})
	}
}

func (s *SocketServer) sendMergeChildResponse(conn net.Conn, resp MergeChildResponse) {
	msg := SocketMessage{Type: MessageTypeMergeChild, MergeChildResp: &resp}
	respJSON, err := json.Marshal(msg)
	if err != nil {
		s.log.Error("failed to marshal merge child response", "error", err)
		return
	}
	conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	if _, err := conn.Write(append(respJSON, '\n')); err != nil {
		s.log.Error("write error", "error", err)
	}
}

func (s *SocketServer) handleCreatePRMessage(conn net.Conn, req *CreatePRRequest) {
	if req == nil || s.createPRReq == nil {
		s.log.Warn("create PR request ignored (nil request or no channel)")
		s.sendCreatePRResponse(conn, CreatePRResponse{Success: false, Error: "Host tools not available"})
		return
	}

	s.log.Info("received create PR request", "title", req.Title)

	select {
	case s.createPRReq <- *req:
	case <-time.After(SocketReadTimeout):
		s.log.Warn("timeout sending create PR request to TUI")
		s.sendCreatePRResponse(conn, CreatePRResponse{ID: req.ID, Success: false, Error: "Timeout waiting for TUI"})
		return
	}

	select {
	case resp := <-s.createPRResp:
		s.sendCreatePRResponse(conn, resp)
		s.log.Info("sent create PR response", "success", resp.Success, "prURL", resp.PRURL)
	case <-time.After(HostToolResponseTimeout):
		s.log.Warn("timeout waiting for create PR response")
		s.sendCreatePRResponse(conn, CreatePRResponse{ID: req.ID, Success: false, Error: "Timeout"})
	}
}

func (s *SocketServer) sendCreatePRResponse(conn net.Conn, resp CreatePRResponse) {
	msg := SocketMessage{Type: MessageTypeCreatePR, CreatePRResp: &resp}
	respJSON, err := json.Marshal(msg)
	if err != nil {
		s.log.Error("failed to marshal create PR response", "error", err)
		return
	}
	conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	if _, err := conn.Write(append(respJSON, '\n')); err != nil {
		s.log.Error("write error", "error", err)
	}
}

func (s *SocketServer) handlePushBranchMessage(conn net.Conn, req *PushBranchRequest) {
	if req == nil || s.pushBranchReq == nil {
		s.log.Warn("push branch request ignored (nil request or no channel)")
		s.sendPushBranchResponse(conn, PushBranchResponse{Success: false, Error: "Host tools not available"})
		return
	}

	s.log.Info("received push branch request", "commitMessage", req.CommitMessage)

	select {
	case s.pushBranchReq <- *req:
	case <-time.After(SocketReadTimeout):
		s.log.Warn("timeout sending push branch request to TUI")
		s.sendPushBranchResponse(conn, PushBranchResponse{ID: req.ID, Success: false, Error: "Timeout waiting for TUI"})
		return
	}

	select {
	case resp := <-s.pushBranchResp:
		s.sendPushBranchResponse(conn, resp)
		s.log.Info("sent push branch response", "success", resp.Success)
	case <-time.After(HostToolResponseTimeout):
		s.log.Warn("timeout waiting for push branch response")
		s.sendPushBranchResponse(conn, PushBranchResponse{ID: req.ID, Success: false, Error: "Timeout"})
	}
}

func (s *SocketServer) sendPushBranchResponse(conn net.Conn, resp PushBranchResponse) {
	msg := SocketMessage{Type: MessageTypePushBranch, PushBranchResp: &resp}
	respJSON, err := json.Marshal(msg)
	if err != nil {
		s.log.Error("failed to marshal push branch response", "error", err)
		return
	}
	conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	if _, err := conn.Write(append(respJSON, '\n')); err != nil {
		s.log.Error("write error", "error", err)
	}
}

func (s *SocketServer) handleGetReviewCommentsMessage(conn net.Conn, req *GetReviewCommentsRequest) {
	if req == nil || s.getReviewCommentsReq == nil {
		s.log.Warn("get review comments request ignored (nil request or no channel)")
		s.sendGetReviewCommentsResponse(conn, GetReviewCommentsResponse{Success: false, Error: "Host tools not available"})
		return
	}

	s.log.Info("received get review comments request")

	select {
	case s.getReviewCommentsReq <- *req:
	case <-time.After(SocketReadTimeout):
		s.log.Warn("timeout sending get review comments request to TUI")
		s.sendGetReviewCommentsResponse(conn, GetReviewCommentsResponse{ID: req.ID, Success: false, Error: "Timeout waiting for TUI"})
		return
	}

	select {
	case resp := <-s.getReviewCommentsResp:
		s.sendGetReviewCommentsResponse(conn, resp)
		s.log.Info("sent get review comments response", "success", resp.Success, "commentCount", len(resp.Comments))
	case <-time.After(HostToolResponseTimeout):
		s.log.Warn("timeout waiting for get review comments response")
		s.sendGetReviewCommentsResponse(conn, GetReviewCommentsResponse{ID: req.ID, Success: false, Error: "Timeout"})
	}
}

func (s *SocketServer) sendGetReviewCommentsResponse(conn net.Conn, resp GetReviewCommentsResponse) {
	msg := SocketMessage{Type: MessageTypeGetReviewComments, GetReviewCommentsResp: &resp}
	respJSON, err := json.Marshal(msg)
	if err != nil {
		s.log.Error("failed to marshal get review comments response", "error", err)
		return
	}
	conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	if _, err := conn.Write(append(respJSON, '\n')); err != nil {
		s.log.Error("write error", "error", err)
	}
}

// Close shuts down the socket server and waits for the Run() goroutine to exit.
func (s *SocketServer) Close() error {
	s.log.Info("closing socket server")

	// Mark as closed BEFORE closing listener to signal Run() goroutine to exit
	s.closedMu.Lock()
	s.closed = true
	s.closedMu.Unlock()

	// Close listener (this will unblock Accept())
	err := s.listener.Close()

	// Wait for the Run() goroutine to finish so we don't remove the socket
	// file while it's still being used
	s.wg.Wait()

	// Remove socket file for Unix socket servers (TCP servers have no file to clean up)
	if !s.isTCP && s.socketPath != "" {
		if removeErr := os.Remove(s.socketPath); removeErr != nil && !os.IsNotExist(removeErr) {
			s.log.Warn("failed to remove socket file", "socketPath", s.socketPath, "error", removeErr)
		}
	}

	return err
}

// SocketClient connects to the TUI's socket server (used by MCP server subprocess)
type SocketClient struct {
	socketPath string
	conn       net.Conn
	reader     *bufio.Reader
}

// NewSocketClient creates a client connected to the TUI socket via Unix socket
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

// NewTCPSocketClient creates a client connected to the TUI via TCP.
// Used inside containers where Unix sockets can't cross the container boundary.
func NewTCPSocketClient(addr string) (*SocketClient, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	return &SocketClient{
		conn:   conn,
		reader: bufio.NewReader(conn),
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

// SendCreateChildRequest sends a create child request and waits for response
func (c *SocketClient) SendCreateChildRequest(req CreateChildRequest) (CreateChildResponse, error) {
	msg := SocketMessage{Type: MessageTypeCreateChild, CreateChildReq: &req}
	reqJSON, err := json.Marshal(msg)
	if err != nil {
		return CreateChildResponse{}, err
	}
	c.conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	if _, err = c.conn.Write(append(reqJSON, '\n')); err != nil {
		return CreateChildResponse{}, fmt.Errorf("write create child request: %w", err)
	}
	c.conn.SetReadDeadline(time.Time{})
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return CreateChildResponse{}, fmt.Errorf("read create child response: %w", err)
	}
	var respMsg SocketMessage
	if err := json.Unmarshal([]byte(line), &respMsg); err != nil {
		return CreateChildResponse{}, err
	}
	if respMsg.CreateChildResp == nil {
		return CreateChildResponse{}, fmt.Errorf("expected create child response, got nil")
	}
	return *respMsg.CreateChildResp, nil
}

// SendListChildrenRequest sends a list children request and waits for response
func (c *SocketClient) SendListChildrenRequest(req ListChildrenRequest) (ListChildrenResponse, error) {
	msg := SocketMessage{Type: MessageTypeListChildren, ListChildrenReq: &req}
	reqJSON, err := json.Marshal(msg)
	if err != nil {
		return ListChildrenResponse{}, err
	}
	c.conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	if _, err = c.conn.Write(append(reqJSON, '\n')); err != nil {
		return ListChildrenResponse{}, fmt.Errorf("write list children request: %w", err)
	}
	c.conn.SetReadDeadline(time.Time{})
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return ListChildrenResponse{}, fmt.Errorf("read list children response: %w", err)
	}
	var respMsg SocketMessage
	if err := json.Unmarshal([]byte(line), &respMsg); err != nil {
		return ListChildrenResponse{}, err
	}
	if respMsg.ListChildrenResp == nil {
		return ListChildrenResponse{}, fmt.Errorf("expected list children response, got nil")
	}
	return *respMsg.ListChildrenResp, nil
}

// SendMergeChildRequest sends a merge child request and waits for response
func (c *SocketClient) SendMergeChildRequest(req MergeChildRequest) (MergeChildResponse, error) {
	msg := SocketMessage{Type: MessageTypeMergeChild, MergeChildReq: &req}
	reqJSON, err := json.Marshal(msg)
	if err != nil {
		return MergeChildResponse{}, err
	}
	c.conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	if _, err = c.conn.Write(append(reqJSON, '\n')); err != nil {
		return MergeChildResponse{}, fmt.Errorf("write merge child request: %w", err)
	}
	c.conn.SetReadDeadline(time.Time{})
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return MergeChildResponse{}, fmt.Errorf("read merge child response: %w", err)
	}
	var respMsg SocketMessage
	if err := json.Unmarshal([]byte(line), &respMsg); err != nil {
		return MergeChildResponse{}, err
	}
	if respMsg.MergeChildResp == nil {
		return MergeChildResponse{}, fmt.Errorf("expected merge child response, got nil")
	}
	return *respMsg.MergeChildResp, nil
}

// SendCreatePRRequest sends a create PR request and waits for response
func (c *SocketClient) SendCreatePRRequest(req CreatePRRequest) (CreatePRResponse, error) {
	msg := SocketMessage{Type: MessageTypeCreatePR, CreatePRReq: &req}
	reqJSON, err := json.Marshal(msg)
	if err != nil {
		return CreatePRResponse{}, err
	}
	c.conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	if _, err = c.conn.Write(append(reqJSON, '\n')); err != nil {
		return CreatePRResponse{}, fmt.Errorf("write create PR request: %w", err)
	}
	c.conn.SetReadDeadline(time.Now().Add(HostToolResponseTimeout))
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return CreatePRResponse{}, fmt.Errorf("read create PR response: %w", err)
	}
	var respMsg SocketMessage
	if err := json.Unmarshal([]byte(line), &respMsg); err != nil {
		return CreatePRResponse{}, err
	}
	if respMsg.CreatePRResp == nil {
		return CreatePRResponse{}, fmt.Errorf("expected create PR response, got nil")
	}
	return *respMsg.CreatePRResp, nil
}

// SendPushBranchRequest sends a push branch request and waits for response
func (c *SocketClient) SendPushBranchRequest(req PushBranchRequest) (PushBranchResponse, error) {
	msg := SocketMessage{Type: MessageTypePushBranch, PushBranchReq: &req}
	reqJSON, err := json.Marshal(msg)
	if err != nil {
		return PushBranchResponse{}, err
	}
	c.conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	if _, err = c.conn.Write(append(reqJSON, '\n')); err != nil {
		return PushBranchResponse{}, fmt.Errorf("write push branch request: %w", err)
	}
	c.conn.SetReadDeadline(time.Now().Add(HostToolResponseTimeout))
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return PushBranchResponse{}, fmt.Errorf("read push branch response: %w", err)
	}
	var respMsg SocketMessage
	if err := json.Unmarshal([]byte(line), &respMsg); err != nil {
		return PushBranchResponse{}, err
	}
	if respMsg.PushBranchResp == nil {
		return PushBranchResponse{}, fmt.Errorf("expected push branch response, got nil")
	}
	return *respMsg.PushBranchResp, nil
}

// SendGetReviewCommentsRequest sends a get review comments request and waits for response
func (c *SocketClient) SendGetReviewCommentsRequest(req GetReviewCommentsRequest) (GetReviewCommentsResponse, error) {
	msg := SocketMessage{Type: MessageTypeGetReviewComments, GetReviewCommentsReq: &req}
	reqJSON, err := json.Marshal(msg)
	if err != nil {
		return GetReviewCommentsResponse{}, err
	}
	c.conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	if _, err = c.conn.Write(append(reqJSON, '\n')); err != nil {
		return GetReviewCommentsResponse{}, fmt.Errorf("write get review comments request: %w", err)
	}
	c.conn.SetReadDeadline(time.Now().Add(HostToolResponseTimeout))
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return GetReviewCommentsResponse{}, fmt.Errorf("read get review comments response: %w", err)
	}
	var respMsg SocketMessage
	if err := json.Unmarshal([]byte(line), &respMsg); err != nil {
		return GetReviewCommentsResponse{}, err
	}
	if respMsg.GetReviewCommentsResp == nil {
		return GetReviewCommentsResponse{}, fmt.Errorf("expected get review comments response, got nil")
	}
	return *respMsg.GetReviewCommentsResp, nil
}

// Close closes the client connection
func (c *SocketClient) Close() error {
	return c.conn.Close()
}
