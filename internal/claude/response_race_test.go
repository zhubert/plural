package claude

import (
	"sync"
	"testing"
	"time"

	"github.com/zhubert/plural/internal/mcp"
)

// TestSendResponseNoRaceWithStop verifies that Send*Response methods don't panic
// when racing with Stop(). This test exercises the fix for issue #255.
func TestSendResponseNoRaceWithStop(t *testing.T) {
	tests := []struct {
		name     string
		sendFunc func(*Runner)
	}{
		{
			name: "SendPermissionResponse",
			sendFunc: func(r *Runner) {
				r.SendPermissionResponse(mcp.PermissionResponse{Allowed: true})
			},
		},
		{
			name: "SendQuestionResponse",
			sendFunc: func(r *Runner) {
				r.SendQuestionResponse(mcp.QuestionResponse{Answers: map[string]string{"q1": "a1"}})
			},
		},
		{
			name: "SendPlanApprovalResponse",
			sendFunc: func(r *Runner) {
				r.SendPlanApprovalResponse(mcp.PlanApprovalResponse{Approved: true})
			},
		},
		{
			name: "SendCreateChildResponse",
			sendFunc: func(r *Runner) {
				r.SetSupervisor(true) // Initialize supervisor channels
				r.SendCreateChildResponse(mcp.CreateChildResponse{ChildID: "test"})
			},
		},
		{
			name: "SendListChildrenResponse",
			sendFunc: func(r *Runner) {
				r.SetSupervisor(true) // Initialize supervisor channels
				r.SendListChildrenResponse(mcp.ListChildrenResponse{})
			},
		},
		{
			name: "SendMergeChildResponse",
			sendFunc: func(r *Runner) {
				r.SetSupervisor(true) // Initialize supervisor channels
				r.SendMergeChildResponse(mcp.MergeChildResponse{})
			},
		},
		{
			name: "SendCreatePRResponse",
			sendFunc: func(r *Runner) {
				r.SetHostTools(true) // Initialize host tool channels
				r.SendCreatePRResponse(mcp.CreatePRResponse{})
			},
		},
		{
			name: "SendPushBranchResponse",
			sendFunc: func(r *Runner) {
				r.SetHostTools(true) // Initialize host tool channels
				r.SendPushBranchResponse(mcp.PushBranchResponse{})
			},
		},
		{
			name: "SendGetReviewCommentsResponse",
			sendFunc: func(r *Runner) {
				r.SetHostTools(true) // Initialize host tool channels
				r.SendGetReviewCommentsResponse(mcp.GetReviewCommentsResponse{})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a runner with MCP channels initialized
			r := New("test-session", "/tmp", "/tmp", false, nil)

			// Run many iterations to increase chance of hitting the race
			iterations := 1000
			var wg sync.WaitGroup

			for i := 0; i < iterations; i++ {
				// Reset runner for each iteration
				r = New("test-session", "/tmp", "/tmp", false, nil)

				wg.Add(2)

				// Goroutine 1: Send response
				go func() {
					defer wg.Done()
					tt.sendFunc(r)
				}()

				// Goroutine 2: Stop runner (closes channels)
				go func() {
					defer wg.Done()
					// Add tiny delay to increase chance of hitting the race window
					time.Sleep(time.Nanosecond)
					r.Stop()
				}()

				wg.Wait()
			}

			// If we get here without panicking, the fix works
		})
	}
}

// TestSendResponseAfterStop verifies that Send*Response methods gracefully
// handle calls after Stop() has been called.
func TestSendResponseAfterStop(t *testing.T) {
	r := New("test-session", "/tmp", "/tmp", false, nil)

	// Stop the runner first
	r.Stop()

	// These should all be no-ops and not panic
	r.SendPermissionResponse(mcp.PermissionResponse{Allowed: true})
	r.SendQuestionResponse(mcp.QuestionResponse{Answers: map[string]string{"q1": "a1"}})
	r.SendPlanApprovalResponse(mcp.PlanApprovalResponse{Approved: true})

	r.SetSupervisor(true)
	r.SendCreateChildResponse(mcp.CreateChildResponse{ChildID: "test"})
	r.SendListChildrenResponse(mcp.ListChildrenResponse{})
	r.SendMergeChildResponse(mcp.MergeChildResponse{})

	r.SetHostTools(true)
	r.SendCreatePRResponse(mcp.CreatePRResponse{})
	r.SendPushBranchResponse(mcp.PushBranchResponse{})
	r.SendGetReviewCommentsResponse(mcp.GetReviewCommentsResponse{})

	// If we get here without panicking, the fix works
}

// TestSendResponseBeforeStop verifies that Send*Response methods work correctly
// when called before Stop() in a controlled manner.
func TestSendResponseBeforeStop(t *testing.T) {
	r := New("test-session", "/tmp", "/tmp", false, nil)

	// Start goroutines to consume from the channels
	var wg sync.WaitGroup

	// Permission response
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case resp := <-r.mcp.Permission.Resp:
			if !resp.Allowed {
				t.Error("expected Allowed=true")
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("timeout waiting for permission response")
		}
	}()

	r.SendPermissionResponse(mcp.PermissionResponse{Allowed: true})
	wg.Wait()

	// Question response
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case resp := <-r.mcp.Question.Resp:
			if resp.Answers["q1"] != "a1" {
				t.Error("expected answer a1 for question q1")
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("timeout waiting for question response")
		}
	}()

	r.SendQuestionResponse(mcp.QuestionResponse{Answers: map[string]string{"q1": "a1"}})
	wg.Wait()

	// Plan approval response
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case resp := <-r.mcp.PlanApproval.Resp:
			if !resp.Approved {
				t.Error("expected Approved=true")
			}
		case <-time.After(100 * time.Millisecond):
			t.Error("timeout waiting for plan approval response")
		}
	}()

	r.SendPlanApprovalResponse(mcp.PlanApprovalResponse{Approved: true})
	wg.Wait()

	r.Stop()
}

// TestRequestChannelsReturnNilAfterStop verifies that request channel getters
// return nil after Stop() to prevent reading from closed channels.
func TestRequestChannelsReturnNilAfterStop(t *testing.T) {
	r := New("test-session", "/tmp", "/tmp", false, nil)
	r.SetSupervisor(true)
	r.SetHostTools(true)

	// Before stop, channels should be non-nil
	if r.PermissionRequestChan() == nil {
		t.Error("expected PermissionRequestChan to be non-nil before stop")
	}
	if r.QuestionRequestChan() == nil {
		t.Error("expected QuestionRequestChan to be non-nil before stop")
	}
	if r.PlanApprovalRequestChan() == nil {
		t.Error("expected PlanApprovalRequestChan to be non-nil before stop")
	}

	r.Stop()

	// After stop, channels should be nil
	if r.PermissionRequestChan() != nil {
		t.Error("expected PermissionRequestChan to be nil after stop")
	}
	if r.QuestionRequestChan() != nil {
		t.Error("expected QuestionRequestChan to be nil after stop")
	}
	if r.PlanApprovalRequestChan() != nil {
		t.Error("expected PlanApprovalRequestChan to be nil after stop")
	}
	if r.CreateChildRequestChan() != nil {
		t.Error("expected CreateChildRequestChan to be nil after stop")
	}
	if r.ListChildrenRequestChan() != nil {
		t.Error("expected ListChildrenRequestChan to be nil after stop")
	}
	if r.MergeChildRequestChan() != nil {
		t.Error("expected MergeChildRequestChan to be nil after stop")
	}
	if r.CreatePRRequestChan() != nil {
		t.Error("expected CreatePRRequestChan to be nil after stop")
	}
	if r.PushBranchRequestChan() != nil {
		t.Error("expected PushBranchRequestChan to be nil after stop")
	}
	if r.GetReviewCommentsRequestChan() != nil {
		t.Error("expected GetReviewCommentsRequestChan to be nil after stop")
	}
}
