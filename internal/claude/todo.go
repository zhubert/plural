package claude

import (
	"encoding/json"
	"fmt"
)

// TodoStatus represents the status of a todo item
type TodoStatus string

const (
	// TodoStatusPending indicates the task has not been started
	TodoStatusPending TodoStatus = "pending"
	// TodoStatusInProgress indicates the task is currently being worked on
	TodoStatusInProgress TodoStatus = "in_progress"
	// TodoStatusCompleted indicates the task has been finished
	TodoStatusCompleted TodoStatus = "completed"
)

// TodoItem represents a single todo item from the TodoWrite tool
type TodoItem struct {
	// Content is the description of the task to be completed
	Content string `json:"content"`
	// Status is the current state of the task
	Status TodoStatus `json:"status"`
	// ActiveForm is the present participle form shown during execution
	// e.g., "Running tests" for a task with content "Run tests"
	ActiveForm string `json:"activeForm"`
}

// TodoList represents a complete todo list from TodoWrite
type TodoList struct {
	Items []TodoItem
}

// todoWriteInput represents the JSON structure of TodoWrite tool input
type todoWriteInput struct {
	Todos []TodoItem `json:"todos"`
}

// ParseTodoWriteInput parses the raw JSON input from a TodoWrite tool call
// and returns the todo list. Returns nil if parsing fails or input is empty.
func ParseTodoWriteInput(input json.RawMessage) (*TodoList, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("empty input")
	}

	var parsed todoWriteInput
	if err := json.Unmarshal(input, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse TodoWrite input: %w", err)
	}

	if len(parsed.Todos) == 0 {
		return nil, fmt.Errorf("no todos in input")
	}

	return &TodoList{Items: parsed.Todos}, nil
}

// CountByStatus returns the count of items with each status
func (t *TodoList) CountByStatus() (pending, inProgress, completed int) {
	if t == nil {
		return 0, 0, 0
	}
	for _, item := range t.Items {
		switch item.Status {
		case TodoStatusPending:
			pending++
		case TodoStatusInProgress:
			inProgress++
		case TodoStatusCompleted:
			completed++
		}
	}
	return
}

// HasItems returns true if the todo list has any items
func (t *TodoList) HasItems() bool {
	return t != nil && len(t.Items) > 0
}

// IsComplete returns true if all items in the todo list are completed
func (t *TodoList) IsComplete() bool {
	if t == nil || len(t.Items) == 0 {
		return false
	}
	for _, item := range t.Items {
		if item.Status != TodoStatusCompleted {
			return false
		}
	}
	return true
}
