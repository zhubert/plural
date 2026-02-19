package app

import "github.com/zhubert/plural-core/manager"

// MergeType is an alias for manager.MergeType to avoid breaking existing code.
type MergeType = manager.MergeType

// Re-export MergeType constants from manager package.
const (
	MergeTypeNone   = manager.MergeTypeNone
	MergeTypeMerge  = manager.MergeTypeMerge
	MergeTypePR     = manager.MergeTypePR
	MergeTypeParent = manager.MergeTypeParent
	MergeTypePush   = manager.MergeTypePush
)

// PendingCommit tracks state for commit message editing.
// Non-nil when a commit message is being edited.
type PendingCommit struct {
	SessionID       string    // Session ID waiting for commit message confirmation
	Type            MergeType // What operation follows after commit
	ParentSessionID string    // Parent session ID for merge-to-parent operations
}

// PendingConflict tracks state for conflict resolution.
// Non-nil when conflicts are being resolved.
type PendingConflict struct {
	SessionID string // Session ID with pending conflict resolution
	RepoPath  string // Path to repo with conflicts
}
