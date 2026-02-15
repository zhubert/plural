package claude

import (
	"context"

	"github.com/zhubert/plural/internal/mcp"
)

// RunnerInterface defines the contract for Claude runners.
// This allows for mock implementations in tests while keeping
// the production Runner implementation unchanged.
type RunnerInterface interface {
	// Session state
	SessionStarted() bool
	IsStreaming() bool

	// Message handling
	Send(ctx context.Context, prompt string) <-chan ResponseChunk
	SendContent(ctx context.Context, content []ContentBlock) <-chan ResponseChunk
	GetMessages() []Message
	AddAssistantMessage(content string)
	GetResponseChan() <-chan ResponseChunk

	// Configuration
	SetAllowedTools(tools []string)
	AddAllowedTool(tool string)
	SetMCPServers(servers []MCPServer)
	SetForkFromSession(parentSessionID string)
	SetContainerized(containerized bool, image string)
	SetOnContainerReady(callback func())

	// Permission/Question/Plan channels
	PermissionRequestChan() <-chan mcp.PermissionRequest
	SendPermissionResponse(resp mcp.PermissionResponse)
	QuestionRequestChan() <-chan mcp.QuestionRequest
	SendQuestionResponse(resp mcp.QuestionResponse)
	PlanApprovalRequestChan() <-chan mcp.PlanApprovalRequest
	SendPlanApprovalResponse(resp mcp.PlanApprovalResponse)

	// Supervisor tool channels
	SetSupervisor(supervisor bool)
	CreateChildRequestChan() <-chan mcp.CreateChildRequest
	SendCreateChildResponse(resp mcp.CreateChildResponse)
	ListChildrenRequestChan() <-chan mcp.ListChildrenRequest
	SendListChildrenResponse(resp mcp.ListChildrenResponse)
	MergeChildRequestChan() <-chan mcp.MergeChildRequest
	SendMergeChildResponse(resp mcp.MergeChildResponse)

	// Host tool channels (for autonomous supervisor sessions)
	SetHostTools(hostTools bool)
	CreatePRRequestChan() <-chan mcp.CreatePRRequest
	SendCreatePRResponse(resp mcp.CreatePRResponse)
	PushBranchRequestChan() <-chan mcp.PushBranchRequest
	SendPushBranchResponse(resp mcp.PushBranchResponse)
	GetReviewCommentsRequestChan() <-chan mcp.GetReviewCommentsRequest
	SendGetReviewCommentsResponse(resp mcp.GetReviewCommentsResponse)

	// Lifecycle
	Stop()
	Interrupt() error
}

// Ensure Runner implements RunnerInterface at compile time.
var _ RunnerInterface = (*Runner)(nil)
