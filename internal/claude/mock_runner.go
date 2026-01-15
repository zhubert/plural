package claude

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/zhubert/plural/internal/mcp"
)

// MockRunner is a test double for Runner that doesn't spawn real processes.
// It allows tests to control response chunks, simulate permissions/questions,
// and verify messages sent to Claude.
//
// NOTE: This file is used by integration tests in internal/app/*_test.go.
// The deadcode tool reports it as unused because it only analyzes the main
// binary, not test code. Do not remove this file.
type MockRunner struct {
	mu sync.RWMutex

	// State
	sessionID      string
	sessionStarted bool
	isStreaming    bool
	messages       []Message
	allowedTools   []string
	mcpServers     []MCPServer

	// Response queue - chunks queued by tests to be returned by Send/SendContent
	responseQueue []ResponseChunk
	responseChan  chan ResponseChunk

	// Permission/Question channels
	permReqChan   chan mcp.PermissionRequest
	permRespChan  chan mcp.PermissionResponse
	questReqChan  chan mcp.QuestionRequest
	questRespChan chan mcp.QuestionResponse

	// Callbacks for test assertions
	OnSend           func(content []ContentBlock)
	OnPermissionResp func(resp mcp.PermissionResponse)
	OnQuestionResp   func(resp mcp.QuestionResponse)

	stopped bool
}

// NewMockRunner creates a mock runner for testing.
func NewMockRunner(sessionID string, sessionStarted bool, initialMessages []Message) *MockRunner {
	msgs := initialMessages
	if msgs == nil {
		msgs = []Message{}
	}
	allowedTools := make([]string, len(DefaultAllowedTools))
	copy(allowedTools, DefaultAllowedTools)

	return &MockRunner{
		sessionID:      sessionID,
		sessionStarted: sessionStarted,
		messages:       msgs,
		allowedTools:   allowedTools,
		permReqChan:    make(chan mcp.PermissionRequest, 1),
		permRespChan:   make(chan mcp.PermissionResponse, 1),
		questReqChan:   make(chan mcp.QuestionRequest, 1),
		questRespChan:  make(chan mcp.QuestionResponse, 1),
	}
}

// QueueResponse queues response chunks to be returned by Send/SendContent.
// Chunks are delivered in order when SendContent is called.
func (m *MockRunner) QueueResponse(chunks ...ResponseChunk) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responseQueue = append(m.responseQueue, chunks...)
}

// ClearResponseQueue clears any queued responses.
func (m *MockRunner) ClearResponseQueue() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responseQueue = nil
}

// SimulatePermissionRequest triggers a permission request that the UI will receive.
func (m *MockRunner) SimulatePermissionRequest(req mcp.PermissionRequest) {
	m.mu.RLock()
	stopped := m.stopped
	m.mu.RUnlock()
	if stopped {
		return
	}
	m.permReqChan <- req
}

// SimulateQuestionRequest triggers a question request that the UI will receive.
func (m *MockRunner) SimulateQuestionRequest(req mcp.QuestionRequest) {
	m.mu.RLock()
	stopped := m.stopped
	m.mu.RUnlock()
	if stopped {
		return
	}
	m.questReqChan <- req
}

// SessionStarted implements RunnerInterface.
func (m *MockRunner) SessionStarted() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessionStarted
}

// IsStreaming implements RunnerInterface.
func (m *MockRunner) IsStreaming() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isStreaming
}

// Send implements RunnerInterface.
func (m *MockRunner) Send(ctx context.Context, prompt string) <-chan ResponseChunk {
	return m.SendContent(ctx, TextContent(prompt))
}

// SendContent implements RunnerInterface.
func (m *MockRunner) SendContent(ctx context.Context, content []ContentBlock) <-chan ResponseChunk {
	m.mu.Lock()

	// Add user message
	displayContent := GetDisplayContent(content)
	m.messages = append(m.messages, Message{Role: "user", Content: displayContent})

	// Call callback if set
	if m.OnSend != nil {
		m.OnSend(content)
	}

	// Create response channel
	ch := make(chan ResponseChunk, 100)
	m.responseChan = ch
	m.isStreaming = true

	// Copy queued responses
	queue := make([]ResponseChunk, len(m.responseQueue))
	copy(queue, m.responseQueue)
	m.responseQueue = nil

	m.mu.Unlock()

	// Stream queued responses in goroutine
	go func() {
		var fullResponse strings.Builder
		for _, chunk := range queue {
			select {
			case <-ctx.Done():
				ch <- ResponseChunk{Done: true}
				close(ch)
				m.mu.Lock()
				m.isStreaming = false
				m.mu.Unlock()
				return
			default:
				if chunk.Type == ChunkTypeText {
					fullResponse.WriteString(chunk.Content)
				}
				ch <- chunk

				// If this is the done chunk, finalize
				if chunk.Done {
					m.mu.Lock()
					m.sessionStarted = true
					m.messages = append(m.messages, Message{Role: "assistant", Content: fullResponse.String()})
					m.isStreaming = false
					m.mu.Unlock()
					close(ch)
					return
				}
			}
		}

		// If no done chunk was in the queue, don't close the channel
		// This allows tests to simulate in-progress streaming
	}()

	return ch
}

// GetMessages implements RunnerInterface.
func (m *MockRunner) GetMessages() []Message {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// Return a copy to prevent race conditions
	msgs := make([]Message, len(m.messages))
	copy(msgs, m.messages)
	return msgs
}

// AddAssistantMessage implements RunnerInterface.
func (m *MockRunner) AddAssistantMessage(content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, Message{Role: "assistant", Content: content})
}

// GetResponseChan implements RunnerInterface.
func (m *MockRunner) GetResponseChan() <-chan ResponseChunk {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.responseChan
}

// SetAllowedTools implements RunnerInterface.
func (m *MockRunner) SetAllowedTools(tools []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, tool := range tools {
		if !slices.Contains(m.allowedTools, tool) {
			m.allowedTools = append(m.allowedTools, tool)
		}
	}
}

// AddAllowedTool implements RunnerInterface.
func (m *MockRunner) AddAllowedTool(tool string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !slices.Contains(m.allowedTools, tool) {
		m.allowedTools = append(m.allowedTools, tool)
	}
}

// SetMCPServers implements RunnerInterface.
func (m *MockRunner) SetMCPServers(servers []MCPServer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mcpServers = servers
}


// PermissionRequestChan implements RunnerInterface.
func (m *MockRunner) PermissionRequestChan() <-chan mcp.PermissionRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stopped {
		return nil
	}
	return m.permReqChan
}

// SendPermissionResponse implements RunnerInterface.
func (m *MockRunner) SendPermissionResponse(resp mcp.PermissionResponse) {
	m.mu.RLock()
	stopped := m.stopped
	m.mu.RUnlock()

	if stopped {
		return
	}

	if m.OnPermissionResp != nil {
		m.OnPermissionResp(resp)
	}

	select {
	case m.permRespChan <- resp:
	default:
	}
}

// QuestionRequestChan implements RunnerInterface.
func (m *MockRunner) QuestionRequestChan() <-chan mcp.QuestionRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stopped {
		return nil
	}
	return m.questReqChan
}

// SendQuestionResponse implements RunnerInterface.
func (m *MockRunner) SendQuestionResponse(resp mcp.QuestionResponse) {
	m.mu.RLock()
	stopped := m.stopped
	m.mu.RUnlock()

	if stopped {
		return
	}

	if m.OnQuestionResp != nil {
		m.OnQuestionResp(resp)
	}

	select {
	case m.questRespChan <- resp:
	default:
	}
}

// Stop implements RunnerInterface.
func (m *MockRunner) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopped {
		return
	}
	m.stopped = true

	// Close channels
	if m.permReqChan != nil {
		close(m.permReqChan)
	}
	if m.permRespChan != nil {
		close(m.permRespChan)
	}
	if m.questReqChan != nil {
		close(m.questReqChan)
	}
	if m.questRespChan != nil {
		close(m.questRespChan)
	}
	if m.responseChan != nil {
		// Only close if we control it
		select {
		case <-m.responseChan:
			// Already closed or has data
		default:
			close(m.responseChan)
		}
	}
}

// GetAllowedTools returns the current allowed tools list (for test assertions).
func (m *MockRunner) GetAllowedTools() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	tools := make([]string, len(m.allowedTools))
	copy(tools, m.allowedTools)
	return tools
}

// SetStreaming allows tests to manually set the streaming state.
func (m *MockRunner) SetStreaming(streaming bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isStreaming = streaming
}

// CompleteStreaming signals that streaming is done and adds the assistant message.
func (m *MockRunner) CompleteStreaming(content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isStreaming = false
	m.sessionStarted = true
	if content != "" {
		m.messages = append(m.messages, Message{Role: "assistant", Content: content})
	}
}

// Ensure MockRunner implements RunnerInterface at compile time.
var _ RunnerInterface = (*MockRunner)(nil)
