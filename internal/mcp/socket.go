package mcp

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/zhubert/plural/internal/logger"
)

// Socket communication constants
const (
	// PermissionResponseTimeout is the maximum time to wait for a permission response
	PermissionResponseTimeout = 5 * time.Minute

	// SocketReadTimeout is the timeout for reading from the socket
	SocketReadTimeout = 10 * time.Second
)

// SocketServer listens for permission requests from MCP server subprocesses
type SocketServer struct {
	socketPath string
	listener   net.Listener
	requestCh  chan<- PermissionRequest
	responseCh <-chan PermissionResponse
}

// NewSocketServer creates a new socket server for the given session
func NewSocketServer(sessionID string, reqCh chan<- PermissionRequest, respCh <-chan PermissionResponse) (*SocketServer, error) {
	socketPath := filepath.Join(os.TempDir(), "plural-"+sessionID+".sock")

	// Remove existing socket if present
	os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}

	logger.Log("MCP Socket: Listening on %s", socketPath)

	return &SocketServer{
		socketPath: socketPath,
		listener:   listener,
		requestCh:  reqCh,
		responseCh: respCh,
	}, nil
}

// SocketPath returns the path to the socket
func (s *SocketServer) SocketPath() string {
	return s.socketPath
}

// Run starts accepting connections
func (s *SocketServer) Run() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// Check if the listener was closed (expected during shutdown)
			if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
				logger.Log("MCP Socket: Listener closed, stopping")
				return
			}
			// Log error but continue accepting connections
			logger.Log("MCP Socket: Accept error (continuing): %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *SocketServer) handleConnection(conn net.Conn) {
	defer conn.Close()
	logger.Log("MCP Socket: Connection accepted")

	reader := bufio.NewReader(conn)

	for {
		// Set read deadline
		conn.SetReadDeadline(time.Now().Add(SocketReadTimeout))

		// Read permission request
		line, err := reader.ReadString('\n')
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout is expected, continue waiting
				continue
			}
			logger.Log("MCP Socket: Read error: %v", err)
			return
		}

		var req PermissionRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			logger.Log("MCP Socket: JSON parse error: %v", err)
			continue
		}

		logger.Log("MCP Socket: Received request: tool=%s", req.Tool)

		// Send to TUI (non-blocking with timeout)
		select {
		case s.requestCh <- req:
			// Request sent successfully
		case <-time.After(SocketReadTimeout):
			logger.Log("MCP Socket: Timeout sending request to TUI")
			// Send denial response
			resp := PermissionResponse{
				ID:      req.ID,
				Allowed: false,
				Message: "Timeout waiting for TUI",
			}
			respJSON, _ := json.Marshal(resp)
			if _, err := conn.Write(append(respJSON, '\n')); err != nil {
				logger.Log("MCP Socket: Failed to write timeout response: %v", err)
			}
			continue
		}

		// Wait for response with timeout
		select {
		case resp := <-s.responseCh:
			// Send response back
			respJSON, err := json.Marshal(resp)
			if err != nil {
				logger.Log("MCP Socket: Failed to marshal response: %v", err)
				continue
			}

			conn.SetWriteDeadline(time.Now().Add(SocketReadTimeout))
			_, err = conn.Write(append(respJSON, '\n'))
			if err != nil {
				logger.Log("MCP Socket: Write error: %v", err)
				return
			}

			logger.Log("MCP Socket: Sent response: allowed=%v", resp.Allowed)

		case <-time.After(PermissionResponseTimeout):
			logger.Log("MCP Socket: Timeout waiting for permission response")
			// Send denial response due to timeout
			resp := PermissionResponse{
				ID:      req.ID,
				Allowed: false,
				Message: "Permission request timed out",
			}
			respJSON, _ := json.Marshal(resp)
			if _, err := conn.Write(append(respJSON, '\n')); err != nil {
				logger.Log("MCP Socket: Failed to write timeout denial response: %v", err)
			}
		}
	}
}

// Close shuts down the socket server
func (s *SocketServer) Close() error {
	logger.Log("MCP Socket: Closing")
	err := s.listener.Close()
	os.Remove(s.socketPath)
	return err
}

// SocketClient connects to the TUI's socket server (used by MCP server subprocess)
type SocketClient struct {
	socketPath string
	conn       net.Conn
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
	}, nil
}

// SendRequest sends a permission request and waits for response
func (c *SocketClient) SendRequest(req PermissionRequest) (PermissionResponse, error) {
	// Send request
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return PermissionResponse{}, err
	}

	_, err = c.conn.Write(append(reqJSON, '\n'))
	if err != nil {
		return PermissionResponse{}, err
	}

	// Read response
	reader := bufio.NewReader(c.conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		return PermissionResponse{}, err
	}

	var resp PermissionResponse
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return PermissionResponse{}, err
	}

	return resp, nil
}

// Close closes the client connection
func (c *SocketClient) Close() error {
	return c.conn.Close()
}
