// Package session manages Claude Code sessions and their git worktrees.
//
// # Overview
//
// Each session in Plural runs in its own git worktree, allowing multiple
// parallel Claude Code conversations to work on the same repository without
// conflicts. This package handles the creation and validation of these sessions.
//
// # Session Lifecycle
//
// 1. Create: When a new session is created:
//   - A UUID is generated for the session ID
//   - A new git branch is created: plural-<UUID>
//   - A worktree is created under the centralized data directory
//     (~/.plural/worktrees/<UUID> or XDG_DATA_HOME/plural/worktrees/<UUID>)
//   - The session is registered in the config
//
// 2. Resume: When resuming an existing session:
//   - The session's worktree path and branch are retrieved from config
//   - Claude CLI is started with --resume flag to continue the conversation
//
// 3. Delete: When a session is deleted:
//   - The session is removed from config
//   - Message history file is deleted
//   - The git worktree remains (allowing manual recovery if needed)
//
// # Worktree Structure
//
// Session worktrees are stored in a centralized location:
//
//	~/.plural/worktrees/<session-uuid>/
//
// Or when using XDG Base Directory Specification:
//
//	$XDG_DATA_HOME/plural/worktrees/<session-uuid>/
//
// This centralized location avoids scattering .plural-worktrees directories
// across the filesystem. Legacy worktrees from the old sibling-directory layout
// are automatically migrated on startup.
//
// # Git Operations
//
// The package uses git commands for:
//   - Creating worktrees: git worktree add -b <branch> <path>
//   - Validating repos: git rev-parse --git-dir
//   - Finding repo root: git rev-parse --show-toplevel
//
// # Functions
//
// Create: Creates a new session with a git worktree for the given repo path.
//
// ValidateRepo: Checks if a path is a valid git repository.
//
// GetGitRoot: Returns the git root directory for a path.
//
// GetCurrentDirGitRoot: Returns the git root of the current working directory.
//
// MigrateWorktrees: Migrates legacy worktrees to the centralized directory.
package session
