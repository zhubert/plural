package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zhubert/plural/internal/config"
)

func TestWorkItemProperties(t *testing.T) {
	t.Run("ConsumesSlot", func(t *testing.T) {
		slotItems := []*WorkItem{
			{Phase: "async_pending"},
			{Phase: "addressing_feedback"},
		}
		for _, item := range slotItems {
			if !item.ConsumesSlot() {
				t.Errorf("expected phase %q to consume slot", item.Phase)
			}
		}

		nonSlotItems := []*WorkItem{
			{Phase: "idle"},
			{Phase: "pushing"},
			{Phase: ""},
		}
		for _, item := range nonSlotItems {
			if item.ConsumesSlot() {
				t.Errorf("expected phase %q to NOT consume slot", item.Phase)
			}
		}
	})

	t.Run("IsTerminal", func(t *testing.T) {
		terminals := []*WorkItem{
			{State: WorkItemCompleted},
			{State: WorkItemFailed},
			{State: WorkItemAbandoned},
		}
		for _, item := range terminals {
			if !item.IsTerminal() {
				t.Errorf("expected state %q to be terminal", item.State)
			}
		}

		nonTerminals := []*WorkItem{
			{State: WorkItemQueued},
			{State: WorkItemCoding},
			{State: ""},
		}
		for _, item := range nonTerminals {
			if item.IsTerminal() {
				t.Errorf("expected state %q to NOT be terminal", item.State)
			}
		}
	})
}

func TestDaemonState_AddAndGetWorkItem(t *testing.T) {
	state := NewDaemonState("/test/repo")

	item := &WorkItem{
		ID: "item-1",
		IssueRef: config.IssueRef{
			Source: "github",
			ID:     "42",
			Title:  "Fix the bug",
		},
	}

	state.AddWorkItem(item)

	got := state.GetWorkItem("item-1")
	if got == nil {
		t.Fatal("expected to find work item")
	}
	if got.State != WorkItemQueued {
		t.Errorf("expected state queued, got %s", got.State)
	}
	if got.Phase != "idle" {
		t.Errorf("expected phase idle, got %s", got.Phase)
	}
	if got.IssueRef.ID != "42" {
		t.Errorf("expected issue ID 42, got %s", got.IssueRef.ID)
	}
	if got.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Not found
	if state.GetWorkItem("nonexistent") != nil {
		t.Error("expected nil for nonexistent item")
	}
}

func TestDaemonState_AdvanceWorkItem(t *testing.T) {
	state := NewDaemonState("/test/repo")
	state.AddWorkItem(&WorkItem{
		ID:       "item-1",
		IssueRef: config.IssueRef{Source: "github", ID: "1"},
	})

	if err := state.AdvanceWorkItem("item-1", "coding", "async_pending"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	item := state.GetWorkItem("item-1")
	if item.CurrentStep != "coding" {
		t.Errorf("expected step coding, got %s", item.CurrentStep)
	}
	if item.Phase != "async_pending" {
		t.Errorf("expected phase async_pending, got %s", item.Phase)
	}

	// Nonexistent item
	if err := state.AdvanceWorkItem("nonexistent", "coding", "idle"); err == nil {
		t.Error("expected error for nonexistent item")
	}
}

func TestDaemonState_MarkWorkItemTerminal(t *testing.T) {
	state := NewDaemonState("/test/repo")
	state.AddWorkItem(&WorkItem{
		ID:       "item-1",
		IssueRef: config.IssueRef{Source: "github", ID: "1"},
	})

	if err := state.MarkWorkItemTerminal("item-1", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	item := state.GetWorkItem("item-1")
	if item.State != WorkItemCompleted {
		t.Errorf("expected completed, got %s", item.State)
	}
	if item.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}

	// Failed
	state.AddWorkItem(&WorkItem{
		ID:       "item-2",
		IssueRef: config.IssueRef{Source: "github", ID: "2"},
	})
	state.MarkWorkItemTerminal("item-2", false)
	item2 := state.GetWorkItem("item-2")
	if item2.State != WorkItemFailed {
		t.Errorf("expected failed, got %s", item2.State)
	}
}

func TestDaemonState_GetWorkItemsByState(t *testing.T) {
	state := NewDaemonState("/test/repo")

	state.AddWorkItem(&WorkItem{ID: "q1", IssueRef: config.IssueRef{Source: "github", ID: "1"}})
	state.AddWorkItem(&WorkItem{ID: "q2", IssueRef: config.IssueRef{Source: "github", ID: "2"}})
	state.AddWorkItem(&WorkItem{ID: "q3", IssueRef: config.IssueRef{Source: "github", ID: "3"}})

	// All should be queued
	queued := state.GetWorkItemsByState(WorkItemQueued)
	if len(queued) != 3 {
		t.Errorf("expected 3 queued items, got %d", len(queued))
	}

	// Mark one as completed
	state.MarkWorkItemTerminal("q1", true)

	queued = state.GetWorkItemsByState(WorkItemQueued)
	if len(queued) != 2 {
		t.Errorf("expected 2 queued items, got %d", len(queued))
	}
}

func TestDaemonState_GetWorkItemsByStep(t *testing.T) {
	state := NewDaemonState("/test/repo")

	state.AddWorkItem(&WorkItem{ID: "i1", IssueRef: config.IssueRef{Source: "github", ID: "1"}})
	state.AdvanceWorkItem("i1", "coding", "async_pending")

	state.AddWorkItem(&WorkItem{ID: "i2", IssueRef: config.IssueRef{Source: "github", ID: "2"}})
	state.AdvanceWorkItem("i2", "coding", "async_pending")

	state.AddWorkItem(&WorkItem{ID: "i3", IssueRef: config.IssueRef{Source: "github", ID: "3"}})
	state.AdvanceWorkItem("i3", "await_review", "idle")

	coding := state.GetWorkItemsByStep("coding")
	if len(coding) != 2 {
		t.Errorf("expected 2 coding items, got %d", len(coding))
	}

	review := state.GetWorkItemsByStep("await_review")
	if len(review) != 1 {
		t.Errorf("expected 1 await_review item, got %d", len(review))
	}
}

func TestDaemonState_ActiveSlotCount(t *testing.T) {
	state := NewDaemonState("/test/repo")

	if state.ActiveSlotCount() != 0 {
		t.Error("expected 0 active slots initially")
	}

	state.AddWorkItem(&WorkItem{ID: "a", IssueRef: config.IssueRef{Source: "github", ID: "1"}})
	state.AddWorkItem(&WorkItem{ID: "b", IssueRef: config.IssueRef{Source: "github", ID: "2"}})

	// Queued items don't consume slots
	if state.ActiveSlotCount() != 0 {
		t.Error("expected 0 active slots for queued items")
	}

	// async_pending consumes a slot
	state.GetWorkItem("a").Phase = "async_pending"
	if state.ActiveSlotCount() != 1 {
		t.Errorf("expected 1 active slot, got %d", state.ActiveSlotCount())
	}

	// addressing_feedback also consumes a slot
	state.GetWorkItem("b").Phase = "addressing_feedback"
	if state.ActiveSlotCount() != 2 {
		t.Errorf("expected 2 active slots, got %d", state.ActiveSlotCount())
	}

	// idle does not consume a slot
	state.GetWorkItem("a").Phase = "idle"
	if state.ActiveSlotCount() != 1 {
		t.Errorf("expected 1 active slot, got %d", state.ActiveSlotCount())
	}
}

func TestDaemonState_HasWorkItemForIssue(t *testing.T) {
	state := NewDaemonState("/test/repo")

	if state.HasWorkItemForIssue("github", "42") {
		t.Error("expected false for empty state")
	}

	state.AddWorkItem(&WorkItem{
		ID:       "item-1",
		IssueRef: config.IssueRef{Source: "github", ID: "42"},
	})

	if !state.HasWorkItemForIssue("github", "42") {
		t.Error("expected true for existing non-terminal item")
	}

	// Different issue
	if state.HasWorkItemForIssue("github", "99") {
		t.Error("expected false for different issue")
	}

	// Different source
	if state.HasWorkItemForIssue("asana", "42") {
		t.Error("expected false for different source")
	}

	// Terminal items should not count
	state.MarkWorkItemTerminal("item-1", false)
	if state.HasWorkItemForIssue("github", "42") {
		t.Error("expected false for terminal item")
	}
}

func TestDaemonState_SetErrorMessage(t *testing.T) {
	state := NewDaemonState("/test/repo")
	state.AddWorkItem(&WorkItem{
		ID:       "item-1",
		IssueRef: config.IssueRef{Source: "github", ID: "1"},
	})

	state.SetErrorMessage("item-1", "something went wrong")
	item := state.GetWorkItem("item-1")
	if item.ErrorMessage != "something went wrong" {
		t.Errorf("expected error message, got %q", item.ErrorMessage)
	}
	if item.ErrorCount != 1 {
		t.Errorf("expected error count 1, got %d", item.ErrorCount)
	}

	state.SetErrorMessage("item-1", "second error")
	if item.ErrorCount != 2 {
		t.Errorf("expected error count 2, got %d", item.ErrorCount)
	}

	// No-op for nonexistent item
	state.SetErrorMessage("nonexistent", "error")
}

func TestDaemonState_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()

	state := &DaemonState{
		Version:   daemonStateVersion,
		RepoPath:  "/test/repo",
		WorkItems: make(map[string]*WorkItem),
		StartedAt: time.Now().Truncate(time.Millisecond),
		filePath:  filepath.Join(tmpDir, "daemon-state.json"),
	}

	state.AddWorkItem(&WorkItem{
		ID:       "item-1",
		IssueRef: config.IssueRef{Source: "github", ID: "42", Title: "Fix bug"},
	})
	state.AdvanceWorkItem("item-1", "coding", "async_pending")

	// Save
	if err := state.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(state.filePath); err != nil {
		t.Fatalf("state file not created: %v", err)
	}

	// Load back
	data, err := os.ReadFile(state.filePath)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}

	var loaded DaemonState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if loaded.Version != daemonStateVersion {
		t.Errorf("expected version %d, got %d", daemonStateVersion, loaded.Version)
	}
	if loaded.RepoPath != "/test/repo" {
		t.Errorf("expected repo path /test/repo, got %s", loaded.RepoPath)
	}
	if len(loaded.WorkItems) != 1 {
		t.Fatalf("expected 1 work item, got %d", len(loaded.WorkItems))
	}

	item := loaded.WorkItems["item-1"]
	if item.CurrentStep != "coding" {
		t.Errorf("expected step coding, got %s", item.CurrentStep)
	}
	if item.Phase != "async_pending" {
		t.Errorf("expected phase async_pending, got %s", item.Phase)
	}
	if item.IssueRef.Title != "Fix bug" {
		t.Errorf("expected title 'Fix bug', got %q", item.IssueRef.Title)
	}
}

func TestDaemonState_SaveAtomicity(t *testing.T) {
	tmpDir := t.TempDir()
	fp := filepath.Join(tmpDir, "daemon-state.json")

	state := &DaemonState{
		Version:   daemonStateVersion,
		RepoPath:  "/test/repo",
		WorkItems: make(map[string]*WorkItem),
		StartedAt: time.Now(),
		filePath:  fp,
	}

	// Save twice to verify atomic rename works
	state.AddWorkItem(&WorkItem{
		ID:       "item-1",
		IssueRef: config.IssueRef{Source: "github", ID: "1"},
	})
	if err := state.Save(); err != nil {
		t.Fatalf("first Save failed: %v", err)
	}

	state.AddWorkItem(&WorkItem{
		ID:       "item-2",
		IssueRef: config.IssueRef{Source: "github", ID: "2"},
	})
	if err := state.Save(); err != nil {
		t.Fatalf("second Save failed: %v", err)
	}

	// Verify temp file was cleaned up
	tmpFile := fp + ".tmp"
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error("expected temp file to be cleaned up after rename")
	}

	// Verify content has both items
	data, _ := os.ReadFile(fp)
	var loaded DaemonState
	json.Unmarshal(data, &loaded)
	if len(loaded.WorkItems) != 2 {
		t.Errorf("expected 2 work items, got %d", len(loaded.WorkItems))
	}
}

func TestDaemonLock_AcquireAndRelease(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	lock := &DaemonLock{path: lockPath}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("failed to create lock: %v", err)
	}
	lock.file = f

	if err := lock.Release(); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("expected lock file to be removed after release")
	}
}

func TestDaemonLock_DoubleAcquireFails(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("failed to create first lock: %v", err)
	}
	f.WriteString("12345")
	f.Close()

	_, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err == nil {
		t.Error("expected second lock to fail")
	}
	if !os.IsExist(err) {
		t.Errorf("expected IsExist error, got %v", err)
	}

	os.Remove(lockPath)
}

func TestNewDaemonState(t *testing.T) {
	state := NewDaemonState("/my/repo")

	if state.Version != daemonStateVersion {
		t.Errorf("expected version %d, got %d", daemonStateVersion, state.Version)
	}
	if state.RepoPath != "/my/repo" {
		t.Errorf("expected repo path /my/repo, got %s", state.RepoPath)
	}
	if state.WorkItems == nil {
		t.Error("expected WorkItems map to be initialized")
	}
	if len(state.WorkItems) != 0 {
		t.Errorf("expected 0 work items, got %d", len(state.WorkItems))
	}
	if state.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}
}


func TestClearDaemonState(t *testing.T) {
	tmpDir := t.TempDir()
	fp := filepath.Join(tmpDir, "daemon-state.json")

	state := &DaemonState{
		Version:   daemonStateVersion,
		RepoPath:  "/test/repo",
		WorkItems: make(map[string]*WorkItem),
		StartedAt: time.Now(),
		filePath:  fp,
	}
	if err := state.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if _, err := os.Stat(fp); err != nil {
		t.Fatalf("expected state file to exist: %v", err)
	}

	if err := os.Remove(fp); err != nil {
		t.Fatalf("failed to remove state file: %v", err)
	}

	if _, err := os.Stat(fp); !os.IsNotExist(err) {
		t.Error("expected state file to be removed")
	}
}

func TestLockFilePath(t *testing.T) {
	path1 := lockFilePath("/repo/a")
	path2 := lockFilePath("/repo/b")
	path3 := lockFilePath("/repo/a")

	if path1 != path3 {
		t.Errorf("expected same lock path for same repo, got %s vs %s", path1, path3)
	}

	if path1 == path2 {
		t.Error("expected different lock paths for different repos")
	}
}
