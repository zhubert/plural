package mcp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// ChannelPair groups a request and response channel for a single MCP operation.
type ChannelPair[Req, Resp any] struct {
	Req  chan Req
	Resp chan Resp
}

// NewChannelPair creates a new ChannelPair with the given buffer size.
func NewChannelPair[Req, Resp any](bufferSize int) *ChannelPair[Req, Resp] {
	return &ChannelPair[Req, Resp]{
		Req:  make(chan Req, bufferSize),
		Resp: make(chan Resp, bufferSize),
	}
}

// Close closes both channels. Safe to call on nil ChannelPair.
func (cp *ChannelPair[Req, Resp]) Close() {
	if cp == nil {
		return
	}
	if cp.Req != nil {
		close(cp.Req)
		cp.Req = nil
	}
	if cp.Resp != nil {
		close(cp.Resp)
		cp.Resp = nil
	}
}

// IsInitialized returns true if both channels are non-nil.
func (cp *ChannelPair[Req, Resp]) IsInitialized() bool {
	return cp != nil && cp.Req != nil && cp.Resp != nil
}

// handleChannelMessage is the generic handler for SocketServer channel-based messages.
// It replaces the 6 identical handler methods (handleCreateChildMessage, etc.).
func handleChannelMessage[Req, Resp any](
	log *slog.Logger,
	conn net.Conn,
	req *Req,
	reqCh chan<- Req,
	respCh <-chan Resp,
	responseTimeout time.Duration,
	nilResp Resp,
	timeoutResp func(any) Resp,
	getID func(*Req) any,
	msgType MessageType,
	setResp func(*SocketMessage, *Resp),
	label string,
) {
	if req == nil || reqCh == nil {
		log.Warn(label + " request ignored (nil request or no channel)")
		sendResponse(log, conn, msgType, nilResp, setResp)
		return
	}

	log.Info("received " + label + " request")

	select {
	case reqCh <- *req:
	case <-time.After(SocketReadTimeout):
		log.Warn("timeout sending " + label + " request to TUI")
		resp := timeoutResp(getID(req))
		sendResponse(log, conn, msgType, resp, setResp)
		return
	}

	select {
	case resp := <-respCh:
		sendResponse(log, conn, msgType, resp, setResp)
		log.Info("sent " + label + " response")
	case <-time.After(responseTimeout):
		log.Warn("timeout waiting for " + label + " response")
		resp := timeoutResp(getID(req))
		sendResponse(log, conn, msgType, resp, setResp)
	}
}

// sendResponse is the generic response sender for SocketServer.
// It replaces the 6 identical sendXxxResponse methods.
func sendResponse[Resp any](
	log *slog.Logger,
	conn net.Conn,
	msgType MessageType,
	resp Resp,
	setResp func(*SocketMessage, *Resp),
) {
	var msg SocketMessage
	msg.Type = msgType
	setResp(&msg, &resp)

	respJSON, err := json.Marshal(msg)
	if err != nil {
		log.Error("failed to marshal response", "error", err)
		return
	}

	conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	if _, err := conn.Write(append(respJSON, '\n')); err != nil {
		log.Error("write error", "error", err)
	}
}

// sendSocketRequest is the generic request sender for SocketClient.
// It replaces the 9 identical SendXxxRequest methods.
func sendSocketRequest[Req, Resp any](
	c *SocketClient,
	req Req,
	msgType MessageType,
	setReq func(*SocketMessage, *Req),
	getResp func(*SocketMessage) *Resp,
	readTimeout time.Duration, // 0 means no deadline
	label string,
) (Resp, error) {
	var msg SocketMessage
	msg.Type = msgType
	setReq(&msg, &req)

	reqJSON, err := json.Marshal(msg)
	if err != nil {
		var zero Resp
		return zero, err
	}

	c.conn.SetWriteDeadline(time.Now().Add(SocketWriteTimeout))
	if _, err = c.conn.Write(append(reqJSON, '\n')); err != nil {
		var zero Resp
		return zero, fmt.Errorf("write %s request: %w", label, err)
	}

	if readTimeout == 0 {
		c.conn.SetReadDeadline(time.Time{})
	} else {
		c.conn.SetReadDeadline(time.Now().Add(readTimeout))
	}

	line, err := c.reader.ReadString('\n')
	if err != nil {
		var zero Resp
		return zero, fmt.Errorf("read %s response: %w", label, err)
	}

	var respMsg SocketMessage
	if err := json.Unmarshal([]byte(line), &respMsg); err != nil {
		var zero Resp
		return zero, err
	}

	resp := getResp(&respMsg)
	if resp == nil {
		var zero Resp
		return zero, fmt.Errorf("expected %s response, got nil", label)
	}

	return *resp, nil
}

// handleToolChannelRequest is the generic handler for Server tool channel requests.
// It replaces the 6 identical tool handler channel send/receive/respond patterns.
func handleToolChannelRequest[Req, Resp any](
	s *Server,
	reqID any,
	req Req,
	reqCh chan<- Req,
	respCh <-chan Resp,
	receiveTimeout time.Duration,
	isError func(Resp) bool,
	label string,
) {
	select {
	case reqCh <- req:
	case <-time.After(ChannelSendTimeout):
		s.sendToolResult(reqID, true, `{"error":"TUI not responding"}`)
		return
	}

	select {
	case resp := <-respCh:
		resultJSON, err := json.Marshal(resp)
		if err != nil {
			s.sendToolResult(reqID, true, `{"error":"failed to marshal response"}`)
			return
		}
		s.sendToolResult(reqID, isError(resp), string(resultJSON))
	case <-time.After(receiveTimeout):
		s.sendToolResult(reqID, true, fmt.Sprintf(`{"error":"timeout waiting for %s"}`, label))
	}
}

// ForwardRequests starts a goroutine that forwards requests from reqCh to sendFn
// and sends responses back to respCh. On send errors, errRespFn is used to
// generate a fallback response. Exported because it's used from cmd/mcp_server.go.
func ForwardRequests[Req, Resp any](
	wg *sync.WaitGroup,
	reqCh <-chan Req,
	respCh chan<- Resp,
	sendFn func(Req) (Resp, error),
	errRespFn func(Req) Resp,
) {
	wg.Go(func() {
		for req := range reqCh {
			resp, err := sendFn(req)
			if err != nil {
				respCh <- errRespFn(req)
			} else {
				respCh <- resp
			}
		}
	})
}
