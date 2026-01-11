# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## Project Overview

Plural is a TUI application for managing multiple concurrent Claude Code sessions. Each session runs in its own git worktree, allowing isolated Claude conversations on the same codebase.

## Build and Run Commands

```bash
# Generate embedded files (copies CHANGELOG.md for embedding)
go generate ./...

# Build the application
go build -o plural .

# Run the application
./plural

# Run with go directly (requires go generate first)
go generate ./... && go run .

# Run tests
go test ./...

# CLI flags
./plural --check-prereqs  # Validate required tools
./plural --clear          # Clear all sessions
./plural --prune          # Remove orphaned worktrees
./plural --debug          # Enable debug logging (verbose output)
```

## Debug Logs

```bash
# Main app logs (UI events, session management, state transitions)
tail -f /tmp/plural-debug.log

# MCP permission logs (per-session)
tail -f /tmp/plural-mcp-*.log
```

### Log Levels

The logger supports four levels: Debug, Info, Warn, Error.

- **Default**: Only Info, Warn, and Error messages are logged
- **Debug mode** (`--debug`): All messages including verbose Debug output

Use `--debug` when troubleshooting to see detailed operation logs (streaming chunks, IPC messages, per-message events).

## Architecture

### Core Flow

1. User registers git repositories via the TUI
2. Creating a session generates a UUID, creates a git worktree in `.plural-worktrees/<UUID>` (sibling to repo), starts Claude CLI with `--session-id`
3. Each session maintains independent message history and Claude CLI process

### Package Structure

- **main.go** - Entry point, Bubble Tea program setup, `mcp-server` subcommand
- **internal/app** - Main Bubble Tea model coordinating UI and Claude runners
  - `app.go` - Main model, Update/View, key handling
  - `shortcuts.go` - Central keyboard shortcut registry (single source of truth)
  - `session_manager.go` - Session lifecycle, runner caching, message persistence
  - `session_state.go` - Thread-safe per-session state (permissions, streaming, UI)
  - `modal_handlers.go` - Modal key handlers
  - `types.go` - Shared types
- **internal/claude** - Claude CLI wrapper (`--output-format stream-json --input-format stream-json`)
  - `claude.go` - Runner with persistent process, streaming, tool status, permissions, multi-modal support
- **internal/changelog** - Changelog parsing for version comparison
- **internal/cli** - CLI prerequisites checking (claude, git, gh)
- **internal/clipboard** - Cross-platform clipboard image reading
- **internal/config** - Persists repos, sessions, tools, history to `~/.plural/`
- **internal/git** - Git operations for merge/PR workflow
- **internal/logger** - Thread-safe file logger
- **internal/mcp** - MCP server for permission prompts via Unix socket IPC
- **internal/notification** - Cross-platform desktop notifications (uses beeep library)
- **internal/process** - Find/kill orphaned Claude processes
- **internal/session** - Git worktree creation/management
- **internal/ui** - Bubble Tea UI components
  - `constants.go` - Layout constants
  - `context.go` - Singleton ViewContext for layout calculations
  - `theme.go` - Theme system
  - `styles.go` - Lipgloss styles
  - `sidebar.go`, `chat.go`, `modal.go`, `header.go`, `footer.go` - UI components

### Data Storage

- `~/.plural/config.json` - Repos, sessions, allowed tools, MCP servers, theme, branch prefix
- `~/.plural/sessions/<session-id>.json` - Conversation history (last 10,000 lines)

### Branch Prefix Configuration

A default branch prefix can be configured to prepend to all auto-generated branch names:

- **Settings modal** (`,`): Open the Settings modal to configure the branch prefix
- **Config field**: `default_branch_prefix` in `~/.plural/config.json`
- **Example**: Setting `"default_branch_prefix": "zhubert/"` will create branches like:
  - `zhubert/plural-<uuid>` for new sessions
  - `zhubert/issue-42` for GitHub issue imports
  - `zhubert/option-1` for parallel option exploration
  - `zhubert/my-feature` for manually named branches

### Key Patterns

- **Bubble Tea Model-Update-View** with tea.Msg for events
- **Focus system**: Tab switches between sidebar and chat
- **Streaming**: Claude responses stream via channel as `ClaudeResponseMsg`
- **Runner caching**: Claude runners cached by session ID in `claudeRunners` map
- **Thread-safe config**: Uses sync.RWMutex
- **Per-session state**: Independent state maps for concurrent operation
- **Explicit state machine**: `AppState` enum (StateIdle, StateStreamingClaude)

### Keyboard Shortcuts

Shortcuts are defined in a central registry (`shortcuts.go`) with:
- `ShortcutRegistry` - All executable shortcuts with handlers
- `DisplayOnlyShortcuts` - Informational entries shown in help modal
- Guards: `RequiresSidebar`, `RequiresSession`, `Condition` functions
- `ExecuteShortcut()` - Unified execution with automatic guard checking

To add a new shortcut:
1. Add entry to `ShortcutRegistry` in `shortcuts.go`
2. Create handler function (e.g., `shortcutNewFeature`)
3. The shortcut automatically appears in help modal and works from both direct key press and help modal selection

### Permission System

1. Claude CLI started with `--permission-prompt-tool mcp__plural__permission`
2. MCP server subprocess communicates with TUI via Unix socket (`/tmp/plural-<session-id>.sock`)
3. Permission prompts appear inline in chat (y/n/a responses)
4. Allowed tools: defaults + global (`allowed_tools`) + per-repo (`repo_allowed_tools`)

### Session Forking and Merging

Sessions support a git-like branching workflow:

1. **Fork session** (`f`): Creates a child session with its own worktree, optionally copying conversation history
2. **Explore options** (`Ctrl+P`): When Claude offers multiple options, fork into parallel sessions to try each
3. **Merge to parent** (`m`): Child sessions can merge back to their parent:
   - Git changes merged into parent's worktree
   - Conversation history appended to parent's history
   - Conflict resolution uses same flow as merge-to-main (Claude resolve / manual)
   - Child session locked after merge (marked "merged to parent")
4. **Merge types**: `MergeTypeMerge` (to main), `MergeTypePR` (create PR), `MergeTypeParent` (to parent), `MergeTypePush` (push updates to existing PR)

### PR Updates Workflow

After creating a PR, the session remains active for continued development:

1. **Continue working**: Send messages to Claude, make additional changes in the session
2. **Push updates** (`m`): When ready, press `m` to open merge modal - shows "Push updates to PR" instead of "Create PR"
3. **Commit and push**: Uncommitted changes are committed with Claude-generated message, then pushed to the PR branch
4. **Iterate**: Continue making changes and pushing updates as needed based on PR feedback

### GitHub Issue Import

Import GitHub issues directly into new sessions:

1. **Press `i`** from anywhere (sidebar or with/without a session selected)
2. **Repo selection**: If a session is selected, uses that session's repo. Otherwise, shows a repo picker modal
3. **Issue list**: Shows open issues from the selected repo (fetched via `gh issue list`)
4. **Select issues**: Use space to toggle selection, up/down to navigate
5. **Create sessions**: Press Enter to create a new session for each selected issue
6. **Auto-start**: Claude automatically receives the issue context and begins working

Each issue becomes a session with branch name `issue-{number}`. The issue number is stored in the session and when a PR is created, "Fixes #N" is automatically added to the PR body, which will close the issue when the PR is merged.

### Message Search

Search within conversation history:

1. **Press `Ctrl+/`** when the chat panel is focused
2. **Type to search**: Real-time filtering as you type
3. **Navigate results**: Use ↑/↓ to move through matches
4. **View match context**: Each result shows a snippet with the match highlighted
5. **Press Enter** to close the modal

The search is case-insensitive and matches any substring within messages. Results show the message number, role (You/Claude), and a snippet around the match.

### Desktop Notifications

Plural can send desktop notifications when Claude finishes responding while the application is in the background:

1. **Enable via Settings** (`,`): Open Settings modal and toggle the "Desktop notifications" checkbox
2. **When triggered**: Notifications appear when Claude completes streaming and the terminal window is not focused
3. **Notification content**: Shows "Plural" as title and "[session name] is ready" as the message
4. **Platform support**: Uses the beeep library for cross-platform notifications:
   - macOS: terminal-notifier or AppleScript
   - Linux: D-Bus or notify-send
   - Windows: Windows Runtime COM API

The setting is persisted in `~/.plural/config.json` as `notifications_enabled`.

### Dependencies

Charm's Bubble Tea v2 stack:
- `charm.land/bubbletea/v2` (v2.0.0-rc.2)
- `charm.land/bubbles/v2` (v2.0.0-rc.1)
- `charm.land/lipgloss/v2` (v2.0.0-beta.3)
- `github.com/google/uuid` (v1.6.0)
- `github.com/gen2brain/beeep` (v0.11.2) - Cross-platform desktop notifications

Key v2 API notes:
- Imports use `charm.land/*` instead of `github.com/charmbracelet/*`
- `tea.KeyMsg` is now `tea.KeyPressMsg`
- `tea.View` returns declarative view with properties
- Viewport uses `SetWidth()`/`SetHeight()` methods
- **Key strings**: Special keys use string names, not literals. Use `"space"` not `" "`, `"tab"` not `"\t"`, etc.

## Releasing

```bash
# Run release script (updates flake.nix, vendorHash, tags, pushes)
./scripts/release.sh v0.0.5

# Dry run
./scripts/release.sh v0.0.5 --dry-run
```

GoReleaser builds binaries for Linux/macOS (amd64/arm64) and updates Homebrew tap at `zhubert/homebrew-tap`.

## License

MIT License - see [LICENSE](LICENSE) for details.
