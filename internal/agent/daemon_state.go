package agent

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/paths"
)

// WorkItemState represents the current state of a work item in the daemon lifecycle.
// Kept for backward compatibility with serialized state; new items use CurrentStep/Phase.
type WorkItemState string

const (
	WorkItemQueued             WorkItemState = "queued"
	WorkItemCoding             WorkItemState = "coding"
	WorkItemPRCreated          WorkItemState = "pr_created"
	WorkItemAwaitingReview     WorkItemState = "awaiting_review"
	WorkItemAddressingFeedback WorkItemState = "addressing_feedback"
	WorkItemPushing            WorkItemState = "pushing"
	WorkItemAwaitingCI         WorkItemState = "awaiting_ci"
	WorkItemMerging            WorkItemState = "merging"
	WorkItemCompleted          WorkItemState = "completed"
	WorkItemFailed             WorkItemState = "failed"
	WorkItemAbandoned          WorkItemState = "abandoned"
)

// WorkItem tracks a single issue through its full lifecycle.
type WorkItem struct {
	ID                string          `json:"id"`
	IssueRef          config.IssueRef `json:"issue_ref"`
	State             WorkItemState   `json:"state"`
	CurrentStep       string          `json:"current_step"`
	Phase             string          `json:"phase"`
	StepData          map[string]any  `json:"step_data,omitempty"`
	SessionID         string          `json:"session_id"`
	Branch            string          `json:"branch"`
	PRURL             string          `json:"pr_url,omitempty"`
	CommentsAddressed int             `json:"comments_addressed"`
	FeedbackRounds    int             `json:"feedback_rounds"`
	ErrorMessage      string          `json:"error_message,omitempty"`
	ErrorCount        int             `json:"error_count"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	CompletedAt       *time.Time      `json:"completed_at,omitempty"`
}

// ConsumesSlot returns true if the work item currently consumes a concurrency slot.
// This is true when the item has an active async worker (Phase == "async_pending"
// or Phase == "addressing_feedback").
func (item *WorkItem) ConsumesSlot() bool {
	return item.Phase == "async_pending" || item.Phase == "addressing_feedback"
}

// IsTerminal returns true if the work item is in a terminal state.
func (item *WorkItem) IsTerminal() bool {
	return item.State == WorkItemCompleted || item.State == WorkItemFailed || item.State == WorkItemAbandoned
}

// DaemonState holds the persistent state of the daemon.
type DaemonState struct {
	Version    int                  `json:"version"`
	RepoPath   string               `json:"repo_path"`
	WorkItems  map[string]*WorkItem `json:"work_items"`
	LastPollAt time.Time            `json:"last_poll_at"`
	StartedAt  time.Time            `json:"started_at"`

	mu       sync.RWMutex
	filePath string
}

const daemonStateVersion = 2

// NewDaemonState creates a new empty daemon state for the given repo.
func NewDaemonState(repoPath string) *DaemonState {
	return &DaemonState{
		Version:   daemonStateVersion,
		RepoPath:  repoPath,
		WorkItems: make(map[string]*WorkItem),
		StartedAt: time.Now(),
		filePath:  daemonStateFilePath(),
	}
}

// daemonStateFilePath returns the path to the daemon state file.
func daemonStateFilePath() string {
	dir, err := paths.DataDir()
	if err != nil {
		// Fall back to home dir
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".plural")
	}
	return filepath.Join(dir, "daemon-state.json")
}

// lockFilePath returns the path to the lock file for the given repo path.
func lockFilePath(repoPath string) string {
	dir, err := paths.StateDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".plural")
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(repoPath)))
	return filepath.Join(dir, fmt.Sprintf("daemon-%s.lock", hash[:12]))
}

// LoadDaemonState loads daemon state from disk.
// Returns a new empty state if the file doesn't exist.
func LoadDaemonState(repoPath string) (*DaemonState, error) {
	fp := daemonStateFilePath()

	data, err := os.ReadFile(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return NewDaemonState(repoPath), nil
		}
		return nil, fmt.Errorf("failed to read daemon state: %w", err)
	}

	var state DaemonState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse daemon state: %w", err)
	}

	state.filePath = fp
	if state.WorkItems == nil {
		state.WorkItems = make(map[string]*WorkItem)
	}

	// Validate repo path matches
	if state.RepoPath != repoPath {
		return nil, fmt.Errorf("daemon state repo mismatch: expected %s, got %s", repoPath, state.RepoPath)
	}

	return &state, nil
}

// Save persists the daemon state to disk atomically (write temp file, then rename).
func (s *DaemonState) Save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("failed to marshal daemon state: %w", err)
	}

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Atomic write: temp file + rename
	tmpFile := s.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0o644); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}
	if err := os.Rename(tmpFile, s.filePath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}

// AdvanceWorkItem moves a work item to a new step and phase.
func (s *DaemonState) AdvanceWorkItem(id, newStep, newPhase string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := s.WorkItems[id]
	if !ok {
		return fmt.Errorf("work item not found: %s", id)
	}

	item.CurrentStep = newStep
	item.Phase = newPhase
	item.UpdatedAt = time.Now()

	return nil
}

// MarkWorkItemTerminal marks a work item as completed or failed.
func (s *DaemonState) MarkWorkItemTerminal(id string, success bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := s.WorkItems[id]
	if !ok {
		return fmt.Errorf("work item not found: %s", id)
	}

	if success {
		item.State = WorkItemCompleted
	} else {
		item.State = WorkItemFailed
	}

	now := time.Now()
	item.CompletedAt = &now
	item.UpdatedAt = now

	return nil
}

// TransitionWorkItem transitions a work item to a new state.
// Kept for backward compatibility during migration.
func (s *DaemonState) TransitionWorkItem(id string, newState WorkItemState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item, ok := s.WorkItems[id]
	if !ok {
		return fmt.Errorf("work item not found: %s", id)
	}

	item.State = newState
	item.UpdatedAt = time.Now()

	if newState == WorkItemCompleted || newState == WorkItemFailed || newState == WorkItemAbandoned {
		now := time.Now()
		item.CompletedAt = &now
	}

	return nil
}

// AddWorkItem adds a new work item in the Queued state.
func (s *DaemonState) AddWorkItem(item *WorkItem) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item.State = WorkItemQueued
	item.Phase = "idle"
	item.CreatedAt = time.Now()
	item.UpdatedAt = time.Now()
	if item.StepData == nil {
		item.StepData = make(map[string]any)
	}
	s.WorkItems[item.ID] = item
}

// GetWorkItem returns a work item by ID (nil if not found).
func (s *DaemonState) GetWorkItem(id string) *WorkItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.WorkItems[id]
}

// GetWorkItemsByState returns all work items in a given state.
func (s *DaemonState) GetWorkItemsByState(state WorkItemState) []*WorkItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var items []*WorkItem
	for _, item := range s.WorkItems {
		if item.State == state {
			items = append(items, item)
		}
	}
	return items
}

// GetWorkItemsByStep returns all work items at a given step.
func (s *DaemonState) GetWorkItemsByStep(step string) []*WorkItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var items []*WorkItem
	for _, item := range s.WorkItems {
		if item.CurrentStep == step && !item.IsTerminal() {
			items = append(items, item)
		}
	}
	return items
}

// GetActiveWorkItems returns all non-terminal, non-queued work items.
func (s *DaemonState) GetActiveWorkItems() []*WorkItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var items []*WorkItem
	for _, item := range s.WorkItems {
		if !item.IsTerminal() && item.State != WorkItemQueued {
			items = append(items, item)
		}
	}
	return items
}

// ActiveSlotCount returns the number of work items consuming concurrency slots.
func (s *DaemonState) ActiveSlotCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, item := range s.WorkItems {
		if item.ConsumesSlot() {
			count++
		}
	}
	return count
}

// HasWorkItemForIssue checks if a work item already exists for the given issue.
func (s *DaemonState) HasWorkItemForIssue(issueSource, issueID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, item := range s.WorkItems {
		if item.IssueRef.Source == issueSource && item.IssueRef.ID == issueID && !item.IsTerminal() {
			return true
		}
	}
	return false
}

// SetErrorMessage sets the error message on a work item and increments the error count.
func (s *DaemonState) SetErrorMessage(id, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if item, ok := s.WorkItems[id]; ok {
		item.ErrorMessage = msg
		item.ErrorCount++
		item.UpdatedAt = time.Now()
	}
}

// ClearDaemonState removes the daemon state file from disk.
// Returns nil if the file doesn't exist.
func ClearDaemonState() error {
	fp := daemonStateFilePath()
	if err := os.Remove(fp); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove daemon state: %w", err)
	}
	return nil
}

// ClearDaemonLocks finds and removes all daemon lock files.
// Returns the number of lock files removed.
func ClearDaemonLocks() (int, error) {
	dir, err := paths.StateDir()
	if err != nil {
		return 0, fmt.Errorf("failed to resolve state dir: %w", err)
	}

	matches, err := filepath.Glob(filepath.Join(dir, "daemon-*.lock"))
	if err != nil {
		return 0, fmt.Errorf("failed to glob lock files: %w", err)
	}

	removed := 0
	for _, match := range matches {
		if err := os.Remove(match); err != nil && !os.IsNotExist(err) {
			return removed, fmt.Errorf("failed to remove lock file %s: %w", match, err)
		}
		removed++
	}
	return removed, nil
}

// DaemonStateExists returns true if the daemon state file exists on disk.
func DaemonStateExists() bool {
	fp := daemonStateFilePath()
	_, err := os.Stat(fp)
	return err == nil
}

// FindDaemonLocks returns the paths of all daemon lock files.
func FindDaemonLocks() ([]string, error) {
	dir, err := paths.StateDir()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve state dir: %w", err)
	}

	matches, err := filepath.Glob(filepath.Join(dir, "daemon-*.lock"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob lock files: %w", err)
	}
	return matches, nil
}

// DaemonLock manages the lock file to prevent multiple daemons for the same repo.
type DaemonLock struct {
	path string
	file *os.File
}

// AcquireLock attempts to acquire the daemon lock for the given repo path.
// Returns an error if the lock is already held.
func AcquireLock(repoPath string) (*DaemonLock, error) {
	fp := lockFilePath(repoPath)

	dir := filepath.Dir(fp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create lock directory: %w", err)
	}

	// Try to create the lock file exclusively
	f, err := os.OpenFile(fp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			// Check if the lock file is stale (process that created it is gone)
			data, readErr := os.ReadFile(fp)
			if readErr == nil {
				return nil, fmt.Errorf("daemon lock already held (PID: %s). Remove %s if the process is not running", string(data), fp)
			}
			return nil, fmt.Errorf("daemon lock already held at %s", fp)
		}
		return nil, fmt.Errorf("failed to create lock file: %w", err)
	}

	// Write our PID
	fmt.Fprintf(f, "%d", os.Getpid())

	return &DaemonLock{path: fp, file: f}, nil
}

// Release releases the daemon lock.
func (l *DaemonLock) Release() error {
	if l.file != nil {
		l.file.Close()
	}
	return os.Remove(l.path)
}
