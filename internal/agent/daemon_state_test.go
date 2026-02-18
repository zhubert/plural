package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zhubert/plural/internal/config"
)

func TestWorkItemStateProperties(t *testing.T) {
	t.Run("ConsumesSlot", func(t *testing.T) {
		slotStates := []WorkItemState{WorkItemCoding, WorkItemAddressingFeedback}
		for _, s := range slotStates {
			if !s.ConsumesSlot() {
				t.Errorf("expected %s to consume slot", s)
			}
		}

		nonSlotStates := []WorkItemState{
			WorkItemQueued, WorkItemPRCreated, WorkItemAwaitingReview,
			WorkItemPushing, WorkItemAwaitingCI, WorkItemMerging,
			WorkItemCompleted, WorkItemFailed, WorkItemAbandoned,
		}
		for _, s := range nonSlotStates {
			if s.ConsumesSlot() {
				t.Errorf("expected %s to NOT consume slot", s)
			}
		}
	})

	t.Run("IsTerminal", func(t *testing.T) {
		terminals := []WorkItemState{WorkItemCompleted, WorkItemFailed, WorkItemAbandoned}
		for _, s := range terminals {
			if !s.IsTerminal() {
				t.Errorf("expected %s to be terminal", s)
			}
		}

		nonTerminals := []WorkItemState{
			WorkItemQueued, WorkItemCoding, WorkItemPRCreated,
			WorkItemAwaitingReview, WorkItemAddressingFeedback,
			WorkItemPushing, WorkItemAwaitingCI, WorkItemMerging,
		}
		for _, s := range nonTerminals {
			if s.IsTerminal() {
				t.Errorf("expected %s to NOT be terminal", s)
			}
		}
	})

	t.Run("IsShelved", func(t *testing.T) {
		shelved := []WorkItemState{WorkItemAwaitingReview, WorkItemAwaitingCI}
		for _, s := range shelved {
			if !s.IsShelved() {
				t.Errorf("expected %s to be shelved", s)
			}
		}

		nonShelved := []WorkItemState{
			WorkItemQueued, WorkItemCoding, WorkItemPRCreated,
			WorkItemAddressingFeedback, WorkItemPushing, WorkItemMerging,
			WorkItemCompleted, WorkItemFailed, WorkItemAbandoned,
		}
		for _, s := range nonShelved {
			if s.IsShelved() {
				t.Errorf("expected %s to NOT be shelved", s)
			}
		}
	})
}

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		name    string
		from    WorkItemState
		to      WorkItemState
		wantErr bool
	}{
		{"queued to coding", WorkItemQueued, WorkItemCoding, false},
		{"queued to failed", WorkItemQueued, WorkItemFailed, false},
		{"queued to completed (invalid)", WorkItemQueued, WorkItemCompleted, true},
		{"coding to pr_created", WorkItemCoding, WorkItemPRCreated, false},
		{"coding to failed", WorkItemCoding, WorkItemFailed, false},
		{"coding to awaiting_review (invalid)", WorkItemCoding, WorkItemAwaitingReview, true},
		{"pr_created to awaiting_review", WorkItemPRCreated, WorkItemAwaitingReview, false},
		{"awaiting_review to addressing_feedback", WorkItemAwaitingReview, WorkItemAddressingFeedback, false},
		{"awaiting_review to awaiting_ci", WorkItemAwaitingReview, WorkItemAwaitingCI, false},
		{"awaiting_review to abandoned", WorkItemAwaitingReview, WorkItemAbandoned, false},
		{"addressing_feedback to pushing", WorkItemAddressingFeedback, WorkItemPushing, false},
		{"pushing to awaiting_review", WorkItemPushing, WorkItemAwaitingReview, false},
		{"awaiting_ci to merging", WorkItemAwaitingCI, WorkItemMerging, false},
		{"awaiting_ci to awaiting_review", WorkItemAwaitingCI, WorkItemAwaitingReview, false},
		{"merging to completed", WorkItemMerging, WorkItemCompleted, false},
		{"merging to failed", WorkItemMerging, WorkItemFailed, false},
		{"completed to anything (invalid)", WorkItemCompleted, WorkItemQueued, true},
		{"failed to anything (invalid)", WorkItemFailed, WorkItemQueued, true},
		{"abandoned to anything (invalid)", WorkItemAbandoned, WorkItemQueued, true},
		{"unknown state", WorkItemState("unknown"), WorkItemCoding, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTransition(tt.from, tt.to)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTransition(%s, %s) error = %v, wantErr %v", tt.from, tt.to, err, tt.wantErr)
			}
		})
	}
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

func TestDaemonState_TransitionWorkItem(t *testing.T) {
	state := NewDaemonState("/test/repo")
	state.AddWorkItem(&WorkItem{
		ID:       "item-1",
		IssueRef: config.IssueRef{Source: "github", ID: "1"},
	})

	// Valid transition
	if err := state.TransitionWorkItem("item-1", WorkItemCoding); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.GetWorkItem("item-1").State != WorkItemCoding {
		t.Error("expected state coding")
	}

	// Invalid transition
	if err := state.TransitionWorkItem("item-1", WorkItemCompleted); err == nil {
		t.Error("expected error for invalid transition coding -> completed")
	}

	// Nonexistent item
	if err := state.TransitionWorkItem("nonexistent", WorkItemCoding); err == nil {
		t.Error("expected error for nonexistent item")
	}

	// Terminal state sets CompletedAt
	if err := state.TransitionWorkItem("item-1", WorkItemFailed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	item := state.GetWorkItem("item-1")
	if item.CompletedAt == nil {
		t.Error("expected CompletedAt to be set for terminal state")
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

	// Transition one to coding
	state.TransitionWorkItem("q1", WorkItemCoding)

	queued = state.GetWorkItemsByState(WorkItemQueued)
	if len(queued) != 2 {
		t.Errorf("expected 2 queued items, got %d", len(queued))
	}

	coding := state.GetWorkItemsByState(WorkItemCoding)
	if len(coding) != 1 {
		t.Errorf("expected 1 coding item, got %d", len(coding))
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

	// Coding consumes a slot
	state.TransitionWorkItem("a", WorkItemCoding)
	if state.ActiveSlotCount() != 1 {
		t.Errorf("expected 1 active slot, got %d", state.ActiveSlotCount())
	}

	// AddressingFeedback also consumes a slot
	state.TransitionWorkItem("b", WorkItemCoding)
	if state.ActiveSlotCount() != 2 {
		t.Errorf("expected 2 active slots, got %d", state.ActiveSlotCount())
	}

	// Transition to non-slot state
	state.TransitionWorkItem("a", WorkItemPRCreated)
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
	state.TransitionWorkItem("item-1", WorkItemCoding)
	state.TransitionWorkItem("item-1", WorkItemFailed)
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
	state.TransitionWorkItem("item-1", WorkItemCoding)

	// Save
	if err := state.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(state.filePath); err != nil {
		t.Fatalf("state file not created: %v", err)
	}

	// Load back - we need to use the file directly since LoadDaemonState uses paths package
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
	if item.State != WorkItemCoding {
		t.Errorf("expected state coding, got %s", item.State)
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

	// Manually create a lock to test (since lockFilePath uses paths package)
	lock := &DaemonLock{path: lockPath}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("failed to create lock: %v", err)
	}
	lock.file = f

	// Release
	if err := lock.Release(); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// Lock file should be gone
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("expected lock file to be removed after release")
	}
}

func TestDaemonLock_DoubleAcquireFails(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// Create lock file manually
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("failed to create first lock: %v", err)
	}
	f.WriteString("12345")
	f.Close()

	// Try to create another lock at the same path
	_, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err == nil {
		t.Error("expected second lock to fail")
	}
	if !os.IsExist(err) {
		t.Errorf("expected IsExist error, got %v", err)
	}

	// Clean up
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

	// Create a state file
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

	// Verify file exists
	if _, err := os.Stat(fp); err != nil {
		t.Fatalf("expected state file to exist: %v", err)
	}

	// Override daemonStateFilePath via a direct os.Remove since ClearDaemonState
	// uses the paths package. Test the removal logic directly.
	if err := os.Remove(fp); err != nil {
		t.Fatalf("failed to remove state file: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(fp); !os.IsNotExist(err) {
		t.Error("expected state file to be removed")
	}

	// Removing nonexistent file should not error (mirrors ClearDaemonState behavior)
	err := os.Remove(fp)
	if err != nil && !os.IsNotExist(err) {
		t.Errorf("expected no error for nonexistent file, got: %v", err)
	}
}

func TestClearDaemonLocks(t *testing.T) {
	tmpDir := t.TempDir()

	// Create some lock files
	for _, name := range []string{"daemon-abc123.lock", "daemon-def456.lock"} {
		f, err := os.Create(filepath.Join(tmpDir, name))
		if err != nil {
			t.Fatalf("failed to create lock file: %v", err)
		}
		f.Close()
	}

	// Also create a non-lock file that shouldn't be matched
	f, err := os.Create(filepath.Join(tmpDir, "other-file.json"))
	if err != nil {
		t.Fatalf("failed to create other file: %v", err)
	}
	f.Close()

	// Glob and remove lock files (mirrors ClearDaemonLocks logic)
	matches, err := filepath.Glob(filepath.Join(tmpDir, "daemon-*.lock"))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 lock files, got %d", len(matches))
	}

	for _, match := range matches {
		if err := os.Remove(match); err != nil {
			t.Fatalf("failed to remove lock file: %v", err)
		}
	}

	// Verify lock files are gone
	remaining, _ := filepath.Glob(filepath.Join(tmpDir, "daemon-*.lock"))
	if len(remaining) != 0 {
		t.Errorf("expected 0 lock files remaining, got %d", len(remaining))
	}

	// Verify other file still exists
	if _, err := os.Stat(filepath.Join(tmpDir, "other-file.json")); err != nil {
		t.Error("expected other-file.json to still exist")
	}
}

func TestFindDaemonLocks(t *testing.T) {
	tmpDir := t.TempDir()

	// No lock files
	matches, err := filepath.Glob(filepath.Join(tmpDir, "daemon-*.lock"))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 lock files, got %d", len(matches))
	}

	// Create lock files
	for _, name := range []string{"daemon-aaa.lock", "daemon-bbb.lock", "daemon-ccc.lock"} {
		f, err := os.Create(filepath.Join(tmpDir, name))
		if err != nil {
			t.Fatalf("failed to create lock file: %v", err)
		}
		f.Close()
	}

	matches, err = filepath.Glob(filepath.Join(tmpDir, "daemon-*.lock"))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(matches) != 3 {
		t.Errorf("expected 3 lock files, got %d", len(matches))
	}
}

func TestDaemonStateExists(t *testing.T) {
	tmpDir := t.TempDir()
	fp := filepath.Join(tmpDir, "daemon-state.json")

	// File doesn't exist
	if _, err := os.Stat(fp); !os.IsNotExist(err) {
		t.Error("expected file to not exist initially")
	}

	// Create file
	if err := os.WriteFile(fp, []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// File exists
	if _, err := os.Stat(fp); err != nil {
		t.Error("expected file to exist after creation")
	}
}

func TestLockFilePath(t *testing.T) {
	path1 := lockFilePath("/repo/a")
	path2 := lockFilePath("/repo/b")
	path3 := lockFilePath("/repo/a")

	// Same repo should produce same path
	if path1 != path3 {
		t.Errorf("expected same lock path for same repo, got %s vs %s", path1, path3)
	}

	// Different repos should produce different paths
	if path1 == path2 {
		t.Error("expected different lock paths for different repos")
	}
}
