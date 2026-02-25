// Package git provides Git operations for managing worktrees, branches, merges, and PRs.
//
// The package is organized into focused modules:
//   - service.go: GitService struct and constructor
//   - status.go: Worktree status, diff stats, file diffs
//   - commit.go: Commit operations, message generation
//   - merge.go: Merge and PR operations
//   - branch.go: Branch management, divergence checking
//   - github.go: GitHub issue/PR integration
package git
