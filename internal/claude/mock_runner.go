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

	// Permission/Question/Plan channels
	permission   *mcp.ChannelPair[mcp.PermissionRequest, mcp.PermissionResponse]
	question     *mcp.ChannelPair[mcp.QuestionRequest, mcp.QuestionResponse]
	planApproval *mcp.ChannelPair[mcp.PlanApprovalRequest, mcp.PlanApprovalResponse]

	// Supervisor tool channels
	createChild  *mcp.ChannelPair[mcp.CreateChildRequest, mcp.CreateChildResponse]
	listChildren *mcp.ChannelPair[mcp.ListChildrenRequest, mcp.ListChildrenResponse]
	mergeChild   *mcp.ChannelPair[mcp.MergeChildRequest, mcp.MergeChildResponse]

	// Host tool channels
	createPR          *mcp.ChannelPair[mcp.CreatePRRequest, mcp.CreatePRResponse]
	pushBranch        *mcp.ChannelPair[mcp.PushBranchRequest, mcp.PushBranchResponse]
	getReviewComments *mcp.ChannelPair[mcp.GetReviewCommentsRequest, mcp.GetReviewCommentsResponse]

	// Callbacks for test assertions
	OnSend             func(content []ContentBlock)
	OnPermissionResp   func(resp mcp.PermissionResponse)
	OnQuestionResp     func(resp mcp.QuestionResponse)
	OnPlanApprovalResp func(resp mcp.PlanApprovalResponse)

	// Fork tracking
	forkFromSessionID string

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
		permission:     mcp.NewChannelPair[mcp.PermissionRequest, mcp.PermissionResponse](1),
		question:       mcp.NewChannelPair[mcp.QuestionRequest, mcp.QuestionResponse](1),
		planApproval:   mcp.NewChannelPair[mcp.PlanApprovalRequest, mcp.PlanApprovalResponse](1),
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
	m.permission.Req <- req
}

// SimulateQuestionRequest triggers a question request that the UI will receive.
func (m *MockRunner) SimulateQuestionRequest(req mcp.QuestionRequest) {
	m.mu.RLock()
	stopped := m.stopped
	m.mu.RUnlock()
	if stopped {
		return
	}
	m.question.Req <- req
}

// SimulatePlanApprovalRequest triggers a plan approval request that the UI will receive.
func (m *MockRunner) SimulatePlanApprovalRequest(req mcp.PlanApprovalRequest) {
	m.mu.RLock()
	stopped := m.stopped
	m.mu.RUnlock()
	if stopped {
		return
	}
	m.planApproval.Req <- req
}

// SimulateCreateChildRequest triggers a create child request that the UI will receive.
func (m *MockRunner) SimulateCreateChildRequest(req mcp.CreateChildRequest) {
	m.mu.RLock()
	stopped := m.stopped
	ch := m.createChild
	m.mu.RUnlock()
	if stopped || ch == nil {
		return
	}
	ch.Req <- req
}

// SimulateListChildrenRequest triggers a list children request that the UI will receive.
func (m *MockRunner) SimulateListChildrenRequest(req mcp.ListChildrenRequest) {
	m.mu.RLock()
	stopped := m.stopped
	ch := m.listChildren
	m.mu.RUnlock()
	if stopped || ch == nil {
		return
	}
	ch.Req <- req
}

// SimulateMergeChildRequest triggers a merge child request that the UI will receive.
func (m *MockRunner) SimulateMergeChildRequest(req mcp.MergeChildRequest) {
	m.mu.RLock()
	stopped := m.stopped
	ch := m.mergeChild
	m.mu.RUnlock()
	if stopped || ch == nil {
		return
	}
	ch.Req <- req
}

// SimulateCreatePRRequest triggers a create PR request that the UI will receive.
func (m *MockRunner) SimulateCreatePRRequest(req mcp.CreatePRRequest) {
	m.mu.RLock()
	stopped := m.stopped
	ch := m.createPR
	m.mu.RUnlock()
	if stopped || ch == nil {
		return
	}
	ch.Req <- req
}

// SimulatePushBranchRequest triggers a push branch request that the UI will receive.
func (m *MockRunner) SimulatePushBranchRequest(req mcp.PushBranchRequest) {
	m.mu.RLock()
	stopped := m.stopped
	ch := m.pushBranch
	m.mu.RUnlock()
	if stopped || ch == nil {
		return
	}
	ch.Req <- req
}

// SimulateGetReviewCommentsRequest triggers a get review comments request that the UI will receive.
func (m *MockRunner) SimulateGetReviewCommentsRequest(req mcp.GetReviewCommentsRequest) {
	m.mu.RLock()
	stopped := m.stopped
	ch := m.getReviewComments
	m.mu.RUnlock()
	if stopped || ch == nil {
		return
	}
	ch.Req <- req
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

// SetForkFromSession implements RunnerInterface.
// In mock, this stores the parent session ID for test verification.
func (m *MockRunner) SetForkFromSession(parentSessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forkFromSessionID = parentSessionID
}

// GetForkFromSessionID returns the parent session ID if set (for testing).
func (m *MockRunner) GetForkFromSessionID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.forkFromSessionID
}

// SetContainerized implements RunnerInterface.
// In mock, this is a no-op since we don't spawn real processes.
func (m *MockRunner) SetContainerized(containerized bool, image string) {
	// No-op for mock
}

// SetOnContainerReady implements RunnerInterface.
// In mock, this is a no-op since we don't spawn real containers.
func (m *MockRunner) SetOnContainerReady(callback func()) {
	// No-op for mock
}

// SetDisableStreamingChunks implements RunnerInterface.
// In mock, this is a no-op since we don't use real streaming.
func (m *MockRunner) SetDisableStreamingChunks(disable bool) {
	// No-op for mock
}

// PermissionRequestChan implements RunnerInterface.
func (m *MockRunner) PermissionRequestChan() <-chan mcp.PermissionRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stopped {
		return nil
	}
	return m.permission.Req
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
	case m.permission.Resp <- resp:
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
	return m.question.Req
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
	case m.question.Resp <- resp:
	default:
	}
}

// PlanApprovalRequestChan implements RunnerInterface.
func (m *MockRunner) PlanApprovalRequestChan() <-chan mcp.PlanApprovalRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stopped {
		return nil
	}
	return m.planApproval.Req
}

// SendPlanApprovalResponse implements RunnerInterface.
func (m *MockRunner) SendPlanApprovalResponse(resp mcp.PlanApprovalResponse) {
	m.mu.RLock()
	stopped := m.stopped
	m.mu.RUnlock()

	if stopped {
		return
	}

	if m.OnPlanApprovalResp != nil {
		m.OnPlanApprovalResp(resp)
	}

	select {
	case m.planApproval.Resp <- resp:
	default:
	}
}

// SetSupervisor implements RunnerInterface.
func (m *MockRunner) SetSupervisor(supervisor bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if supervisor && m.createChild == nil {
		m.createChild = mcp.NewChannelPair[mcp.CreateChildRequest, mcp.CreateChildResponse](1)
		m.listChildren = mcp.NewChannelPair[mcp.ListChildrenRequest, mcp.ListChildrenResponse](1)
		m.mergeChild = mcp.NewChannelPair[mcp.MergeChildRequest, mcp.MergeChildResponse](1)
	}
}

// CreateChildRequestChan implements RunnerInterface.
func (m *MockRunner) CreateChildRequestChan() <-chan mcp.CreateChildRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stopped || m.createChild == nil {
		return nil
	}
	return m.createChild.Req
}

// SendCreateChildResponse implements RunnerInterface.
func (m *MockRunner) SendCreateChildResponse(resp mcp.CreateChildResponse) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stopped || m.createChild == nil {
		return
	}
	select {
	case m.createChild.Resp <- resp:
	default:
	}
}

// ListChildrenRequestChan implements RunnerInterface.
func (m *MockRunner) ListChildrenRequestChan() <-chan mcp.ListChildrenRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stopped || m.listChildren == nil {
		return nil
	}
	return m.listChildren.Req
}

// SendListChildrenResponse implements RunnerInterface.
func (m *MockRunner) SendListChildrenResponse(resp mcp.ListChildrenResponse) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stopped || m.listChildren == nil {
		return
	}
	select {
	case m.listChildren.Resp <- resp:
	default:
	}
}

// MergeChildRequestChan implements RunnerInterface.
func (m *MockRunner) MergeChildRequestChan() <-chan mcp.MergeChildRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stopped || m.mergeChild == nil {
		return nil
	}
	return m.mergeChild.Req
}

// SendMergeChildResponse implements RunnerInterface.
func (m *MockRunner) SendMergeChildResponse(resp mcp.MergeChildResponse) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stopped || m.mergeChild == nil {
		return
	}
	select {
	case m.mergeChild.Resp <- resp:
	default:
	}
}

// SetHostTools implements RunnerInterface.
func (m *MockRunner) SetHostTools(hostTools bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if hostTools && m.createPR == nil {
		m.createPR = mcp.NewChannelPair[mcp.CreatePRRequest, mcp.CreatePRResponse](1)
		m.pushBranch = mcp.NewChannelPair[mcp.PushBranchRequest, mcp.PushBranchResponse](1)
		m.getReviewComments = mcp.NewChannelPair[mcp.GetReviewCommentsRequest, mcp.GetReviewCommentsResponse](1)
	}
}

// CreatePRRequestChan implements RunnerInterface.
func (m *MockRunner) CreatePRRequestChan() <-chan mcp.CreatePRRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stopped || m.createPR == nil {
		return nil
	}
	return m.createPR.Req
}

// SendCreatePRResponse implements RunnerInterface.
func (m *MockRunner) SendCreatePRResponse(resp mcp.CreatePRResponse) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stopped || m.createPR == nil {
		return
	}
	select {
	case m.createPR.Resp <- resp:
	default:
	}
}

// PushBranchRequestChan implements RunnerInterface.
func (m *MockRunner) PushBranchRequestChan() <-chan mcp.PushBranchRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stopped || m.pushBranch == nil {
		return nil
	}
	return m.pushBranch.Req
}

// SendPushBranchResponse implements RunnerInterface.
func (m *MockRunner) SendPushBranchResponse(resp mcp.PushBranchResponse) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stopped || m.pushBranch == nil {
		return
	}
	select {
	case m.pushBranch.Resp <- resp:
	default:
	}
}

// GetReviewCommentsRequestChan implements RunnerInterface.
func (m *MockRunner) GetReviewCommentsRequestChan() <-chan mcp.GetReviewCommentsRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stopped || m.getReviewComments == nil {
		return nil
	}
	return m.getReviewComments.Req
}

// SendGetReviewCommentsResponse implements RunnerInterface.
func (m *MockRunner) SendGetReviewCommentsResponse(resp mcp.GetReviewCommentsResponse) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.stopped || m.getReviewComments == nil {
		return
	}
	select {
	case m.getReviewComments.Resp <- resp:
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
	m.permission.Close()
	m.question.Close()
	m.planApproval.Close()
	m.createChild.Close()
	m.listChildren.Close()
	m.mergeChild.Close()
	m.createPR.Close()
	m.pushBranch.Close()
	m.getReviewComments.Close()
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

// Interrupt implements RunnerInterface.Interrupt for mock.
// In tests, this is a no-op since there's no real Claude process.
func (m *MockRunner) Interrupt() error {
	return nil
}

// Ensure MockRunner implements RunnerInterface at compile time.
var _ RunnerInterface = (*MockRunner)(nil)
