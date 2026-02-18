package app

import (
	"testing"

	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/mcp"
)

// TestGetToolUseRollupReturnsCopy ensures that GetToolUseRollup returns a copy,
// not a pointer to shared mutable state.
func TestGetToolUseRollupReturnsCopy(t *testing.T) {
	state := &SessionState{ToolUsePos: -1}

	// Add a tool use
	state.AddToolUse("Read", "file.go", "tool-1")

	// Get the rollup
	rollup1 := state.GetToolUseRollup()
	if rollup1 == nil {
		t.Fatal("expected rollup to be non-nil")
	}

	// Modify the returned rollup
	rollup1.Expanded = true
	rollup1.Items = append(rollup1.Items, ToolUseItemState{
		ToolName:  "Edit",
		ToolInput: "another.go",
		ToolUseID: "tool-2",
		Complete:  false,
	})

	// Get the rollup again - should be unaffected by previous modifications
	rollup2 := state.GetToolUseRollup()
	if rollup2 == nil {
		t.Fatal("expected rollup to be non-nil")
	}

	// Verify the original state is unchanged
	if rollup2.Expanded {
		t.Error("expected Expanded to be false (original state)")
	}
	if len(rollup2.Items) != 1 {
		t.Errorf("expected 1 item (original state), got %d", len(rollup2.Items))
	}
	if len(rollup2.Items) > 0 && rollup2.Items[0].ToolName != "Read" {
		t.Errorf("expected first tool to be Read, got %s", rollup2.Items[0].ToolName)
	}
}

// TestGetPendingPermissionReturnsCopy ensures that GetPendingPermission returns a copy.
func TestGetPendingPermissionReturnsCopy(t *testing.T) {
	state := &SessionState{ToolUsePos: -1}

	// Set a permission request
	originalReq := &mcp.PermissionRequest{
		ID:          "req-1",
		Tool:        "Bash",
		Description: "Run tests",
		Arguments: map[string]any{
			"command": "go test",
		},
	}
	state.SetPendingPermission(originalReq)

	// Get the permission
	perm1 := state.GetPendingPermission()
	if perm1 == nil {
		t.Fatal("expected permission to be non-nil")
	}

	// Modify the returned permission
	perm1.Tool = "Modified"
	perm1.Description = "Modified description"
	perm1.Arguments["command"] = "modified command"
	perm1.Arguments["new_field"] = "new value"

	// Get the permission again - should be unaffected
	perm2 := state.GetPendingPermission()
	if perm2 == nil {
		t.Fatal("expected permission to be non-nil")
	}

	// Verify the original state is unchanged
	if perm2.Tool != "Bash" {
		t.Errorf("expected Tool to be 'Bash', got %q", perm2.Tool)
	}
	if perm2.Description != "Run tests" {
		t.Errorf("expected Description to be 'Run tests', got %q", perm2.Description)
	}
	if cmd, ok := perm2.Arguments["command"].(string); !ok || cmd != "go test" {
		t.Errorf("expected command to be 'go test', got %v", perm2.Arguments["command"])
	}
	if _, exists := perm2.Arguments["new_field"]; exists {
		t.Error("expected new_field to not exist in original state")
	}
}

// TestGetPendingQuestionReturnsCopy ensures that GetPendingQuestion returns a copy.
func TestGetPendingQuestionReturnsCopy(t *testing.T) {
	state := &SessionState{ToolUsePos: -1}

	// Set a question request
	originalReq := &mcp.QuestionRequest{
		ID: "req-1",
		Questions: []mcp.Question{
			{
				Question:    "Which approach?",
				Header:      "Approach",
				MultiSelect: false,
				Options: []mcp.QuestionOption{
					{Label: "Option 1", Description: "First option"},
					{Label: "Option 2", Description: "Second option"},
				},
			},
		},
	}
	state.SetPendingQuestion(originalReq)

	// Get the question
	q1 := state.GetPendingQuestion()
	if q1 == nil {
		t.Fatal("expected question to be non-nil")
	}

	// Modify the returned question
	q1.Questions[0].Question = "Modified question"
	q1.Questions[0].Options = append(q1.Questions[0].Options, mcp.QuestionOption{
		Label:       "Option 3",
		Description: "Third option",
	})
	q1.Questions = append(q1.Questions, mcp.Question{
		Question: "New question",
		Header:   "New",
	})

	// Get the question again - should be unaffected
	q2 := state.GetPendingQuestion()
	if q2 == nil {
		t.Fatal("expected question to be non-nil")
	}

	// Verify the original state is unchanged
	if len(q2.Questions) != 1 {
		t.Errorf("expected 1 question, got %d", len(q2.Questions))
	}
	if q2.Questions[0].Question != "Which approach?" {
		t.Errorf("expected question to be 'Which approach?', got %q", q2.Questions[0].Question)
	}
	if len(q2.Questions[0].Options) != 2 {
		t.Errorf("expected 2 options, got %d", len(q2.Questions[0].Options))
	}
}

// TestGetCurrentTodoListReturnsCopy ensures that GetCurrentTodoList returns a copy.
func TestGetCurrentTodoListReturnsCopy(t *testing.T) {
	state := &SessionState{ToolUsePos: -1}

	// Set a todo list
	originalList := &claude.TodoList{
		Items: []claude.TodoItem{
			{
				Content:    "Fix bug",
				Status:     claude.TodoStatusPending,
				ActiveForm: "Fixing bug",
			},
			{
				Content:    "Write tests",
				Status:     claude.TodoStatusInProgress,
				ActiveForm: "Writing tests",
			},
		},
	}
	state.SetCurrentTodoList(originalList)

	// Get the todo list
	list1 := state.GetCurrentTodoList()
	if list1 == nil {
		t.Fatal("expected todo list to be non-nil")
	}

	// Modify the returned list
	list1.Items[0].Content = "Modified content"
	list1.Items[0].Status = claude.TodoStatusCompleted
	list1.Items = append(list1.Items, claude.TodoItem{
		Content:    "New task",
		Status:     claude.TodoStatusPending,
		ActiveForm: "Working on new task",
	})

	// Get the todo list again - should be unaffected
	list2 := state.GetCurrentTodoList()
	if list2 == nil {
		t.Fatal("expected todo list to be non-nil")
	}

	// Verify the original state is unchanged
	if len(list2.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(list2.Items))
	}
	if list2.Items[0].Content != "Fix bug" {
		t.Errorf("expected content to be 'Fix bug', got %q", list2.Items[0].Content)
	}
	if list2.Items[0].Status != claude.TodoStatusPending {
		t.Errorf("expected status to be pending, got %s", list2.Items[0].Status)
	}
}

// TestGetPendingPlanApprovalReturnsCopy ensures that GetPendingPlanApproval returns a copy.
func TestGetPendingPlanApprovalReturnsCopy(t *testing.T) {
	state := &SessionState{ToolUsePos: -1}

	// Set a plan approval request
	originalReq := &mcp.PlanApprovalRequest{
		ID:   "req-1",
		Plan: "# Implementation Plan\n\n1. Do something\n2. Do something else",
		AllowedPrompts: []mcp.AllowedPrompt{
			{Tool: "Bash", Prompt: "run tests"},
			{Tool: "Bash", Prompt: "build project"},
		},
		Arguments: map[string]any{
			"key": "value",
		},
	}
	state.SetPendingPlanApproval(originalReq)

	// Get the plan approval
	plan1 := state.GetPendingPlanApproval()
	if plan1 == nil {
		t.Fatal("expected plan approval to be non-nil")
	}

	// Modify the returned plan
	plan1.Plan = "Modified plan"
	plan1.AllowedPrompts = append(plan1.AllowedPrompts, mcp.AllowedPrompt{
		Tool:   "Bash",
		Prompt: "deploy",
	})
	plan1.Arguments["key"] = "modified"
	plan1.Arguments["new"] = "added"

	// Get the plan approval again - should be unaffected
	plan2 := state.GetPendingPlanApproval()
	if plan2 == nil {
		t.Fatal("expected plan approval to be non-nil")
	}

	// Verify the original state is unchanged
	if plan2.Plan != "# Implementation Plan\n\n1. Do something\n2. Do something else" {
		t.Errorf("expected original plan, got %q", plan2.Plan)
	}
	if len(plan2.AllowedPrompts) != 2 {
		t.Errorf("expected 2 prompts, got %d", len(plan2.AllowedPrompts))
	}
	if val, ok := plan2.Arguments["key"].(string); !ok || val != "value" {
		t.Errorf("expected key=value, got %v", plan2.Arguments["key"])
	}
	if _, exists := plan2.Arguments["new"]; exists {
		t.Error("expected 'new' key to not exist in original state")
	}
}

// TestNilGetters ensures that getters return nil when state is nil (not panic).
func TestNilGetters(t *testing.T) {
	state := &SessionState{ToolUsePos: -1}

	// All getters should return nil for unset state
	if state.GetToolUseRollup() != nil {
		t.Error("expected nil for unset rollup")
	}
	if state.GetPendingPermission() != nil {
		t.Error("expected nil for unset permission")
	}
	if state.GetPendingQuestion() != nil {
		t.Error("expected nil for unset question")
	}
	if state.GetCurrentTodoList() != nil {
		t.Error("expected nil for unset todo list")
	}
	if state.GetPendingPlanApproval() != nil {
		t.Error("expected nil for unset plan approval")
	}
}

// TestConcurrentAccessSafety demonstrates that the fix prevents concurrent
// modification issues. This test simulates a scenario where one goroutine
// retrieves state and another modifies it - the first goroutine's copy
// should remain unchanged.
func TestConcurrentAccessSafety(t *testing.T) {
	state := &SessionState{ToolUsePos: -1}

	// Set up initial state
	state.AddToolUse("Read", "file.go", "tool-1")

	// Simulate goroutine 1: gets the rollup and expects it to remain stable
	rollup := state.GetToolUseRollup()
	if rollup == nil || len(rollup.Items) != 1 {
		t.Fatal("expected rollup with 1 item")
	}

	// Simulate goroutine 2: modifies the state
	state.AddToolUse("Edit", "another.go", "tool-2")
	state.MarkToolUseComplete("tool-1", nil)

	// Goroutine 1's copy should be unchanged
	if len(rollup.Items) != 1 {
		t.Errorf("goroutine 1's rollup was modified: expected 1 item, got %d", len(rollup.Items))
	}
	if rollup.Items[0].Complete {
		t.Error("goroutine 1's rollup item was modified to complete")
	}

	// Fresh get should show the new state
	freshRollup := state.GetToolUseRollup()
	if len(freshRollup.Items) != 2 {
		t.Errorf("expected 2 items in fresh rollup, got %d", len(freshRollup.Items))
	}
	if !freshRollup.Items[0].Complete {
		t.Error("expected first item to be complete in fresh rollup")
	}
}
