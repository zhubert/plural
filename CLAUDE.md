# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Plural is a TUI (Terminal User Interface) application for managing multiple concurrent Claude Code sessions. It allows users to run multiple isolated Claude Code conversations, each in its own git worktree, from a single interface.

## Build and Run Commands

```bash
# Build the application
go build -o plural .

# Run the application
./plural

# Check CLI prerequisites
./plural --check-prereqs

# Clear all sessions
./plural --clear

# Run with go directly
go run .

# Run tests
go test ./...
```

## Debug Logs

Debug logs are separated by process:
- **Main app**: `/tmp/plural-debug.log` - UI events, session management, state transitions
- **MCP sessions**: `/tmp/plural-mcp-<session-id>.log` - Permission handling for each session

```bash
# Tail main app logs
tail -f /tmp/plural-debug.log

# Tail MCP logs for a specific session
tail -f /tmp/plural-mcp-*.log
```

## Architecture

### Core Flow
1. User registers git repositories via the TUI
2. Creating a new session:
   - Generates a UUID for the session
   - Creates a git worktree with branch `plural-<UUID>` in `.plural-worktrees/<UUID>` (sibling to the repo)
   - Starts a Claude Code CLI process in that worktree using `--session-id` for first message, `--resume` for subsequent
3. Each session maintains its own message history and Claude CLI session

### Package Structure

- **main.go** - Entry point, sets up Bubble Tea program with alt screen; also handles `mcp-server` subcommand
- **internal/app** - Main Bubble Tea model coordinating all UI components and Claude runners
- **internal/claude** - Wrapper around Claude Code CLI (`claude --print`), manages streaming responses via stdout pipe
  - `doc.go` - Package documentation
  - `claude.go` - Runner implementation with message streaming and permission handling
- **internal/cli** - CLI prerequisites checking
  - `prerequisites.go` - Validates required CLI tools (claude, git, gh) are available
  - `prerequisites_test.go` - Test suite
- **internal/config** - Persists repos, sessions, allowed tools, and conversation history
  - `config.go` - Configuration management with validation
  - `config_test.go` - Comprehensive test suite
- **internal/git** - Git operations for merge/PR workflow and change tracking
  - `git.go` - Git operations: HasRemoteOrigin, GetWorktreeStatus, CommitAll, GenerateCommitMessage, MergeToMain, CreatePR
  - `git_test.go` - Test suite
- **internal/logger** - Simple file logger for debugging
  - `logger.go` - Thread-safe logger with process-specific log paths
  - `logger_test.go` - Test suite
- **internal/mcp** - MCP (Model Context Protocol) server for handling permission prompts:
  - `doc.go` - Package documentation with permission flow diagrams
  - `protocol.go` - JSON-RPC message types for MCP
  - `protocol_test.go` - Protocol test suite
  - `server.go` - MCP server implementation (stdio transport)
  - `socket.go` - Unix socket communication with timeouts to prevent deadlocks
- **internal/session** - Creates git worktrees for isolated sessions; validates git repos
  - `doc.go` - Package documentation
  - `session.go` - Session creation and git worktree management
  - `session_test.go` - Test suite
- **internal/ui** - UI components using Bubble Tea + Lipgloss:
  - `doc.go` - Package documentation with layout diagrams
  - `constants.go` - Layout constants (heights, widths, buffer sizes)
  - `context.go` - Singleton ViewContext for centralized layout calculations
  - `styles.go` - All lipgloss styles and color palette
  - `sidebar.go` - Session list grouped by repository with custom rendering and permission indicators
  - `chat.go` - Conversation view with soft-wrapping, waiting indicator, and inline permission prompts
  - `modal.go` - Various modals (add repo, new session, delete, merge)
  - `header.go` - Header with gradient background
  - `footer.go` - Context-aware keyboard shortcuts

### Data Storage

- **~/.plural/config.json** - Repos, sessions, allowed tools, session started state
- **~/.plural/sessions/<session-id>.json** - Conversation history (last 100 lines per session)

### Key Patterns

- **Bubble Tea architecture**: Model-Update-View pattern with tea.Msg for events
- **Focus system**: Tab switches focus between sidebar (session list) and chat panel
- **Streaming responses**: Claude responses stream via channel, converted to ClaudeResponseMsg
- **Runner caching**: Claude runners cached by session ID in `claudeRunners` map for session resumption
- **Thread-safe config**: Config uses sync.RWMutex for concurrent access
- **Inline permission handling**: Permission prompts appear inline in chat (non-blocking, per-session)
- **Context-aware footer**: Shortcuts shown/hidden based on focus, selection state, and pending permissions
- **Session grouping**: Sidebar groups sessions by repository with custom rendering
- **Explicit state machine**: App uses `AppState` enum instead of boolean flags
- **Per-session state**: Each session has independent state (input text, waiting status, permissions) allowing concurrent operation

### Application State Machine

The app uses an explicit state machine (`AppState`) to manage async operations:

```
StateIdle            - Ready for user input
StateStreamingClaude - Receiving Claude response
```

State transitions are logged to `/tmp/plural-debug.log` for debugging. Helper methods:
- `IsIdle()` - Check if ready for input
- `CanSendMessage()` - Check if user can send a new message (per-session: checks `sessionWaitStart`)
- `setState(newState)` - Transition to new state with logging

**Per-session state maps:**
- `sessionWaitStart` - Tracks which sessions are waiting for Claude responses
- `sessionInputs` - Preserves input text when switching between sessions
- `sessionStreaming` - Preserves in-progress streaming content when switching sessions
- `sessionMergeChans` - Per-session merge/PR operation channels
- `sessionMergeCancels` - Per-session merge/PR cancel functions
- `pendingPermissions` - Per-session permission prompts

This allows truly independent session operation - you can send messages to session B while session A is waiting for Claude, and merge operations don't block other sessions.

### Permission System

When Claude needs permission for operations (file edits, bash commands, etc.), Plural handles this via:

1. **MCP Server**: Claude CLI is started with `--permission-prompt-tool mcp__plural__permission` which delegates permission decisions to our MCP server
2. **Unix Socket IPC**: The MCP server subprocess communicates with the TUI via Unix socket (`/tmp/plural-<session-id>.sock`)
3. **Inline Permission Prompts**: Permission requests appear inline in each session's chat panel (not as blocking modals)
   - Sessions with pending permissions show a ⚠ indicator in the sidebar
   - When viewing a session with a pending permission, the prompt appears at the bottom of the chat
   - Press `y` to allow, `n` to deny, or `a` to always allow
   - Users can freely switch between sessions while permissions are pending
4. **Per-Session Tracking**: Each session tracks its own pending permission in `pendingPermissions` map
5. **Persistence**: "Always Allow" decisions are saved per-session in `~/.plural/config.json` and honored on session resume

### Viewing Session Changes

Press `v` with a session selected to view uncommitted changes in that session's worktree:
- Shows a summary of changed files
- Displays the git diff (truncated if too large)
- Useful for reviewing what Claude has modified before merging

### Merge/PR Workflow

Sessions work in isolated git worktrees with their own branches. To apply changes back:

1. **Trigger**: Press `m` with a session selected (sidebar focused)
2. **Options Modal**: Shows:
   - Summary of uncommitted changes (or "No uncommitted changes")
   - **Merge to main**: Merges the session branch directly into the default branch
   - **Create PR**: Pushes branch to origin and creates a GitHub PR via `gh` CLI (only available if remote origin exists)
3. **Auto-Commit**: Before merging or creating a PR, any uncommitted changes in the worktree are automatically staged and committed with a descriptive message
4. **Streaming Output**: Command output streams to the chat panel in real-time
5. **Result**: Success or error message displayed when operation completes

### UI Layout
- Header (1 line) + Content (sidebar 1/3 width | chat 2/3 width) + Footer (1 line)
- Panels use lipgloss rounded borders with purple highlight when focused

### Constants and Configuration

Layout constants are centralized in `internal/ui/constants.go`:
- `HeaderHeight`, `FooterHeight`: Fixed at 1 line each
- `BorderSize`: 2 (1 on each side)
- `SidebarWidthRatio`: 3 (sidebar gets 1/3 of width)
- `TextareaHeight`: 3 lines for input
- `MaxSessionMessageLines`: 100 lines kept in history (in config package)
- `PermissionTimeout`: 5 minutes for permission responses

### CLI Prerequisites

On startup, Plural checks for required CLI tools:
- **claude** (required): Claude Code CLI
- **git** (required): Git version control
- **gh** (optional): GitHub CLI for PR creation

Run `./plural --check-prereqs` to see the status of all prerequisites.

### Dependencies

The project uses Charm's Bubble Tea v2 stack (currently in release candidate):
- **charm.land/bubbletea/v2** (v2.0.0-rc.2): TUI framework
- **charm.land/bubbles/v2** (v2.0.0-rc.1): TUI components (textarea, viewport, textinput)
- **charm.land/lipgloss/v2** (v2.0.0-beta.3): Terminal styling
- **github.com/google/uuid** (v1.6.0): UUID generation for session IDs

Key v2 API changes from v1:
- Imports use `charm.land/*` domain instead of `github.com/charmbracelet/*`
- `tea.KeyMsg` is now `tea.KeyPressMsg` (key releases handled separately)
- `tea.View` return type with declarative properties (`v.AltScreen = true`)
- Viewport uses `SetWidth()`/`SetHeight()` methods instead of direct field assignment
- `lipgloss.WithWhitespaceBackground()` replaced with `lipgloss.WithWhitespaceStyle()`
- Textinput/textarea use `SetWidth()` instead of `Width` field
