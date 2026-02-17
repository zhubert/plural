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

	// HostToolResponseTimeout is the timeout for host tool operations (create_pr, push_branch).
	// These operations involve git pushes and GitHub API calls which can take longer
	// than interactive prompts. Must be >= the 2-minute context timeout in TUI handlers.
	HostToolResponseTimeout = 5 * time.Minute

	// ContainerMCPPort is the fixed port the MCP subprocess listens on inside the
	// container. Docker publishes this port to an ephemeral host port via -p 0:21120.
	// The host then dials into the container, reversing the TCP direction so that
	// macOS firewall rules (which block inbound connections to the host) are avoided.
	ContainerMCPPort = 21120
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
	readyCh               chan struct{}   // Closed when the server is ready to accept connections
	log                   *slog.Logger   // Logger with session context
	activeConn            net.Conn       // Active connection (for dialing servers that receive a conn via HandleConn)
	activeConnMu          sync.Mutex     // Guards activeConn
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
		readyCh:    make(chan struct{}),
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
// Binds to 0.0.0.0 (all interfaces) because host.docker.internal may resolve
// to a non-loopback IP depending on the Docker runtime. For example, Colima
// routes through the Lima VM bridge rather than loopback. The port is ephemeral
// and short-lived (session lifetime only).
func NewTCPSocketServer(sessionID string, reqCh chan<- PermissionRequest, respCh <-chan PermissionResponse, questCh chan<- QuestionRequest, ansCh <-chan QuestionResponse, planReqCh chan<- PlanApprovalRequest, planRespCh <-chan PlanApprovalResponse, opts ...SocketServerOption) (*SocketServer, error) {
	log := logger.WithSession(sessionID).With("component", "mcp-socket")

	bindAddr := "0.0.0.0:0"
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
		readyCh:    make(chan struct{}),
		log:        log,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// NewDialingSocketServer creates a SocketServer without a listener. Instead of
// accepting connections, the host dials into the container and passes the
// connection via HandleConn(). Used for container sessions where the MCP
// subprocess inside the container listens on a port and the host connects to it.
// readyCh is closed immediately since there's no accept loop to wait for.
func NewDialingSocketServer(sessionID string, reqCh chan<- PermissionRequest, respCh <-chan PermissionResponse, questCh chan<- QuestionRequest, ansCh <-chan QuestionResponse, planReqCh chan<- PlanApprovalRequest, planRespCh <-chan PlanApprovalResponse, opts ...SocketServerOption) *SocketServer {
	log := logger.WithSession(sessionID).With("component", "mcp-socket")
	log.Info("created dialing socket server (no listener)")

	readyCh := make(chan struct{})
	close(readyCh) // Ready immediately — no accept loop

	s := &SocketServer{
		requestCh:  reqCh,
		responseCh: respCh,
		questionCh: questCh,
		answerCh:   ansCh,
		planReqCh:  planReqCh,
		planRespCh: planRespCh,
		readyCh:    readyCh,
		log:        log,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// HandleConn accepts an externally-established connection and processes messages
// on it. The connection is stored so Close() can clean it up. This method blocks
// until the connection is closed or the server is shut down.
func (s *SocketServer) HandleConn(conn net.Conn) {
	s.activeConnMu.Lock()
	s.activeConn = conn
	s.activeConnMu.Unlock()

	s.handleConnection(conn)

	s.activeConnMu.Lock()
	s.activeConn = nil
	s.activeConnMu.Unlock()
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

// WaitReady blocks until the server is ready to accept connections.
func (s *SocketServer) WaitReady() {
	<-s.readyCh
}

// Run starts accepting connections. Must be paired with a wg.Add(1) call
// before the goroutine is launched — use Start() instead of calling go Run() directly.
func (s *SocketServer) Run() {
	defer s.wg.Done()

	close(s.readyCh)

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
			handleChannelMessage(s.log, conn, msg.CreateChildReq,
				s.createChildReq, s.createChildResp,
				PermissionResponseTimeout,
				CreateChildResponse{Success: false, Error: "Supervisor tools not available"},
				func(id interface{}) CreateChildResponse {
					return CreateChildResponse{ID: id, Success: false, Error: "Timeout"}
				},
				func(r *CreateChildRequest) interface{} { return r.ID },
				MessageTypeCreateChild,
				func(m *SocketMessage, r *CreateChildResponse) { m.CreateChildResp = r },
				"create child")
		case MessageTypeListChildren:
			handleChannelMessage(s.log, conn, msg.ListChildrenReq,
				s.listChildrenReq, s.listChildrenResp,
				PermissionResponseTimeout,
				ListChildrenResponse{Children: []ChildSessionInfo{}},
				func(id interface{}) ListChildrenResponse {
					return ListChildrenResponse{ID: id, Children: []ChildSessionInfo{}}
				},
				func(r *ListChildrenRequest) interface{} { return r.ID },
				MessageTypeListChildren,
				func(m *SocketMessage, r *ListChildrenResponse) { m.ListChildrenResp = r },
				"list children")
		case MessageTypeMergeChild:
			handleChannelMessage(s.log, conn, msg.MergeChildReq,
				s.mergeChildReq, s.mergeChildResp,
				PermissionResponseTimeout,
				MergeChildResponse{Success: false, Error: "Supervisor tools not available"},
				func(id interface{}) MergeChildResponse {
					return MergeChildResponse{ID: id, Success: false, Error: "Timeout"}
				},
				func(r *MergeChildRequest) interface{} { return r.ID },
				MessageTypeMergeChild,
				func(m *SocketMessage, r *MergeChildResponse) { m.MergeChildResp = r },
				"merge child")
		case MessageTypeCreatePR:
			handleChannelMessage(s.log, conn, msg.CreatePRReq,
				s.createPRReq, s.createPRResp,
				HostToolResponseTimeout,
				CreatePRResponse{Success: false, Error: "Host tools not available"},
				func(id interface{}) CreatePRResponse {
					return CreatePRResponse{ID: id, Success: false, Error: "Timeout"}
				},
				func(r *CreatePRRequest) interface{} { return r.ID },
				MessageTypeCreatePR,
				func(m *SocketMessage, r *CreatePRResponse) { m.CreatePRResp = r },
				"create PR")
		case MessageTypePushBranch:
			handleChannelMessage(s.log, conn, msg.PushBranchReq,
				s.pushBranchReq, s.pushBranchResp,
				HostToolResponseTimeout,
				PushBranchResponse{Success: false, Error: "Host tools not available"},
				func(id interface{}) PushBranchResponse {
					return PushBranchResponse{ID: id, Success: false, Error: "Timeout"}
				},
				func(r *PushBranchRequest) interface{} { return r.ID },
				MessageTypePushBranch,
				func(m *SocketMessage, r *PushBranchResponse) { m.PushBranchResp = r },
				"push branch")
		case MessageTypeGetReviewComments:
			handleChannelMessage(s.log, conn, msg.GetReviewCommentsReq,
				s.getReviewCommentsReq, s.getReviewCommentsResp,
				HostToolResponseTimeout,
				GetReviewCommentsResponse{Success: false, Error: "Host tools not available"},
				func(id interface{}) GetReviewCommentsResponse {
					return GetReviewCommentsResponse{ID: id, Success: false, Error: "Timeout"}
				},
				func(r *GetReviewCommentsRequest) interface{} { return r.ID },
				MessageTypeGetReviewComments,
				func(m *SocketMessage, r *GetReviewCommentsResponse) { m.GetReviewCommentsResp = r },
				"get review comments")
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

// Close shuts down the socket server and waits for the Run() goroutine to exit.
func (s *SocketServer) Close() error {
	s.log.Info("closing socket server")

	// Mark as closed BEFORE closing listener to signal Run() goroutine to exit
	s.closedMu.Lock()
	s.closed = true
	s.closedMu.Unlock()

	// Close listener (this will unblock Accept()). Dialing servers have no listener.
	var err error
	if s.listener != nil {
		err = s.listener.Close()
	}

	// Close active connection if present (dialing servers receive a conn via HandleConn)
	s.activeConnMu.Lock()
	if s.activeConn != nil {
		s.activeConn.Close()
	}
	s.activeConnMu.Unlock()

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

// NewListeningSocketClient creates a SocketClient by listening on a TCP address
// and accepting a single connection. Used inside containers where the MCP
// subprocess listens on a port and waits for the host to dial in.
// The listener is closed after accepting the first connection.
func NewListeningSocketClient(listenAddr string) (*SocketClient, error) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", listenAddr, err)
	}

	conn, err := ln.Accept()
	ln.Close() // Only need one connection
	if err != nil {
		return nil, fmt.Errorf("accept on %s: %w", listenAddr, err)
	}

	return &SocketClient{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}, nil
}

// NewSocketClientFromConn wraps an existing net.Conn into a SocketClient.
// Used in the --listen path where Accept() happens asynchronously after the
// MCP JSONRPC server is already running.
func NewSocketClientFromConn(conn net.Conn) *SocketClient {
	return &SocketClient{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}
}

// SendPermissionRequest sends a permission request and waits for response
func (c *SocketClient) SendPermissionRequest(req PermissionRequest) (PermissionResponse, error) {
	return sendSocketRequest(c, req, MessageTypePermission,
		func(m *SocketMessage, r *PermissionRequest) { m.PermReq = r },
		func(m *SocketMessage) *PermissionResponse { return m.PermResp },
		0, "permission")
}

// SendQuestionRequest sends a question request and waits for response
func (c *SocketClient) SendQuestionRequest(req QuestionRequest) (QuestionResponse, error) {
	return sendSocketRequest(c, req, MessageTypeQuestion,
		func(m *SocketMessage, r *QuestionRequest) { m.QuestReq = r },
		func(m *SocketMessage) *QuestionResponse { return m.QuestResp },
		0, "question")
}

// SendPlanApprovalRequest sends a plan approval request and waits for response
func (c *SocketClient) SendPlanApprovalRequest(req PlanApprovalRequest) (PlanApprovalResponse, error) {
	return sendSocketRequest(c, req, MessageTypePlanApproval,
		func(m *SocketMessage, r *PlanApprovalRequest) { m.PlanReq = r },
		func(m *SocketMessage) *PlanApprovalResponse { return m.PlanResp },
		0, "plan approval")
}

// SendCreateChildRequest sends a create child request and waits for response
func (c *SocketClient) SendCreateChildRequest(req CreateChildRequest) (CreateChildResponse, error) {
	return sendSocketRequest(c, req, MessageTypeCreateChild,
		func(m *SocketMessage, r *CreateChildRequest) { m.CreateChildReq = r },
		func(m *SocketMessage) *CreateChildResponse { return m.CreateChildResp },
		0, "create child")
}

// SendListChildrenRequest sends a list children request and waits for response
func (c *SocketClient) SendListChildrenRequest(req ListChildrenRequest) (ListChildrenResponse, error) {
	return sendSocketRequest(c, req, MessageTypeListChildren,
		func(m *SocketMessage, r *ListChildrenRequest) { m.ListChildrenReq = r },
		func(m *SocketMessage) *ListChildrenResponse { return m.ListChildrenResp },
		0, "list children")
}

// SendMergeChildRequest sends a merge child request and waits for response
func (c *SocketClient) SendMergeChildRequest(req MergeChildRequest) (MergeChildResponse, error) {
	return sendSocketRequest(c, req, MessageTypeMergeChild,
		func(m *SocketMessage, r *MergeChildRequest) { m.MergeChildReq = r },
		func(m *SocketMessage) *MergeChildResponse { return m.MergeChildResp },
		0, "merge child")
}

// SendCreatePRRequest sends a create PR request and waits for response
func (c *SocketClient) SendCreatePRRequest(req CreatePRRequest) (CreatePRResponse, error) {
	return sendSocketRequest(c, req, MessageTypeCreatePR,
		func(m *SocketMessage, r *CreatePRRequest) { m.CreatePRReq = r },
		func(m *SocketMessage) *CreatePRResponse { return m.CreatePRResp },
		HostToolResponseTimeout, "create PR")
}

// SendPushBranchRequest sends a push branch request and waits for response
func (c *SocketClient) SendPushBranchRequest(req PushBranchRequest) (PushBranchResponse, error) {
	return sendSocketRequest(c, req, MessageTypePushBranch,
		func(m *SocketMessage, r *PushBranchRequest) { m.PushBranchReq = r },
		func(m *SocketMessage) *PushBranchResponse { return m.PushBranchResp },
		HostToolResponseTimeout, "push branch")
}

// SendGetReviewCommentsRequest sends a get review comments request and waits for response
func (c *SocketClient) SendGetReviewCommentsRequest(req GetReviewCommentsRequest) (GetReviewCommentsResponse, error) {
	return sendSocketRequest(c, req, MessageTypeGetReviewComments,
		func(m *SocketMessage, r *GetReviewCommentsRequest) { m.GetReviewCommentsReq = r },
		func(m *SocketMessage) *GetReviewCommentsResponse { return m.GetReviewCommentsResp },
		HostToolResponseTimeout, "get review comments")
}

// Close closes the client connection
func (c *SocketClient) Close() error {
	return c.conn.Close()
}
