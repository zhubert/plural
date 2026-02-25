package claude

import (
	"encoding/json"
	"testing"
)

func TestParseTodoWriteInput(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantErr    bool
		wantItems  int
		checkFirst func(*testing.T, *TodoList)
	}{
		{
			name: "valid single todo",
			input: `{
				"todos": [
					{"content": "Task 1", "status": "pending", "activeForm": "Working on task 1"}
				]
			}`,
			wantErr:   false,
			wantItems: 1,
			checkFirst: func(t *testing.T, list *TodoList) {
				if list.Items[0].Content != "Task 1" {
					t.Errorf("Content = %q, want %q", list.Items[0].Content, "Task 1")
				}
				if list.Items[0].Status != TodoStatusPending {
					t.Errorf("Status = %q, want %q", list.Items[0].Status, TodoStatusPending)
				}
				if list.Items[0].ActiveForm != "Working on task 1" {
					t.Errorf("ActiveForm = %q, want %q", list.Items[0].ActiveForm, "Working on task 1")
				}
			},
		},
		{
			name: "valid multiple todos with different statuses",
			input: `{
				"todos": [
					{"content": "Completed task", "status": "completed", "activeForm": "Done"},
					{"content": "In progress task", "status": "in_progress", "activeForm": "Doing it"},
					{"content": "Pending task", "status": "pending", "activeForm": "Will do"}
				]
			}`,
			wantErr:   false,
			wantItems: 3,
			checkFirst: func(t *testing.T, list *TodoList) {
				if list.Items[0].Status != TodoStatusCompleted {
					t.Errorf("Items[0].Status = %q, want %q", list.Items[0].Status, TodoStatusCompleted)
				}
				if list.Items[1].Status != TodoStatusInProgress {
					t.Errorf("Items[1].Status = %q, want %q", list.Items[1].Status, TodoStatusInProgress)
				}
				if list.Items[2].Status != TodoStatusPending {
					t.Errorf("Items[2].Status = %q, want %q", list.Items[2].Status, TodoStatusPending)
				}
			},
		},
		{
			name:      "empty input",
			input:     "",
			wantErr:   true,
			wantItems: 0,
		},
		{
			name:      "empty todos array",
			input:     `{"todos": []}`,
			wantErr:   true,
			wantItems: 0,
		},
		{
			name:      "invalid JSON",
			input:     `{not valid json}`,
			wantErr:   true,
			wantItems: 0,
		},
		{
			name:      "missing todos field",
			input:     `{"other": "data"}`,
			wantErr:   true,
			wantItems: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input json.RawMessage
			if tt.input != "" {
				input = json.RawMessage(tt.input)
			}

			got, err := ParseTodoWriteInput(input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTodoWriteInput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if got == nil {
				t.Fatal("ParseTodoWriteInput() returned nil")
			}
			if len(got.Items) != tt.wantItems {
				t.Errorf("len(Items) = %d, want %d", len(got.Items), tt.wantItems)
			}
			if tt.checkFirst != nil {
				tt.checkFirst(t, got)
			}
		})
	}
}

func TestTodoList_CountByStatus(t *testing.T) {
	tests := []struct {
		name           string
		list           *TodoList
		wantPending    int
		wantInProgress int
		wantCompleted  int
	}{
		{
			name:           "nil list",
			list:           nil,
			wantPending:    0,
			wantInProgress: 0,
			wantCompleted:  0,
		},
		{
			name:           "empty items",
			list:           &TodoList{Items: []TodoItem{}},
			wantPending:    0,
			wantInProgress: 0,
			wantCompleted:  0,
		},
		{
			name: "mixed statuses",
			list: &TodoList{
				Items: []TodoItem{
					{Content: "Task 1", Status: TodoStatusCompleted},
					{Content: "Task 2", Status: TodoStatusInProgress},
					{Content: "Task 3", Status: TodoStatusPending},
					{Content: "Task 4", Status: TodoStatusPending},
					{Content: "Task 5", Status: TodoStatusCompleted},
				},
			},
			wantPending:    2,
			wantInProgress: 1,
			wantCompleted:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pending, inProgress, completed := tt.list.CountByStatus()

			if pending != tt.wantPending {
				t.Errorf("pending = %d, want %d", pending, tt.wantPending)
			}
			if inProgress != tt.wantInProgress {
				t.Errorf("inProgress = %d, want %d", inProgress, tt.wantInProgress)
			}
			if completed != tt.wantCompleted {
				t.Errorf("completed = %d, want %d", completed, tt.wantCompleted)
			}
		})
	}
}

func TestTodoList_HasItems(t *testing.T) {
	tests := []struct {
		name string
		list *TodoList
		want bool
	}{
		{
			name: "nil list",
			list: nil,
			want: false,
		},
		{
			name: "empty items",
			list: &TodoList{Items: []TodoItem{}},
			want: false,
		},
		{
			name: "with items",
			list: &TodoList{Items: []TodoItem{{Content: "Test", Status: TodoStatusPending}}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.list.HasItems(); got != tt.want {
				t.Errorf("HasItems() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTodoStatusConstants(t *testing.T) {
	// Verify the status constants match what Claude Code sends
	if TodoStatusPending != "pending" {
		t.Errorf("TodoStatusPending = %q, want %q", TodoStatusPending, "pending")
	}
	if TodoStatusInProgress != "in_progress" {
		t.Errorf("TodoStatusInProgress = %q, want %q", TodoStatusInProgress, "in_progress")
	}
	if TodoStatusCompleted != "completed" {
		t.Errorf("TodoStatusCompleted = %q, want %q", TodoStatusCompleted, "completed")
	}
}

func TestTodoList_IsComplete(t *testing.T) {
	tests := []struct {
		name string
		list *TodoList
		want bool
	}{
		{
			name: "nil list",
			list: nil,
			want: false,
		},
		{
			name: "empty items",
			list: &TodoList{Items: []TodoItem{}},
			want: false,
		},
		{
			name: "all completed",
			list: &TodoList{Items: []TodoItem{
				{Content: "Task 1", Status: TodoStatusCompleted},
				{Content: "Task 2", Status: TodoStatusCompleted},
				{Content: "Task 3", Status: TodoStatusCompleted},
			}},
			want: true,
		},
		{
			name: "single completed",
			list: &TodoList{Items: []TodoItem{
				{Content: "Task 1", Status: TodoStatusCompleted},
			}},
			want: true,
		},
		{
			name: "one pending",
			list: &TodoList{Items: []TodoItem{
				{Content: "Task 1", Status: TodoStatusCompleted},
				{Content: "Task 2", Status: TodoStatusPending},
			}},
			want: false,
		},
		{
			name: "one in progress",
			list: &TodoList{Items: []TodoItem{
				{Content: "Task 1", Status: TodoStatusCompleted},
				{Content: "Task 2", Status: TodoStatusInProgress},
			}},
			want: false,
		},
		{
			name: "mixed statuses",
			list: &TodoList{Items: []TodoItem{
				{Content: "Task 1", Status: TodoStatusCompleted},
				{Content: "Task 2", Status: TodoStatusInProgress},
				{Content: "Task 3", Status: TodoStatusPending},
			}},
			want: false,
		},
		{
			name: "all pending",
			list: &TodoList{Items: []TodoItem{
				{Content: "Task 1", Status: TodoStatusPending},
				{Content: "Task 2", Status: TodoStatusPending},
			}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.list.IsComplete(); got != tt.want {
				t.Errorf("IsComplete() = %v, want %v", got, tt.want)
			}
		})
	}
}
