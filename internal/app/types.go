package app

import "github.com/zhubert/plural-core/manager"

// PendingCommit tracks state for commit message editing.
// Non-nil when a commit message is being edited.
type PendingCommit struct {
	SessionID       string           // Session ID waiting for commit message confirmation
	Type            manager.MergeType // What operation follows after commit
	ParentSessionID string           // Parent session ID for merge-to-parent operations
}

// PendingConflict tracks state for conflict resolution.
// Non-nil when conflicts are being resolved.
type PendingConflict struct {
	SessionID string // Session ID with pending conflict resolution
	RepoPath  string // Path to repo with conflicts
}
