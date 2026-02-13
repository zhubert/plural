# CLAUDE.md

Guidance for Claude Code when working with this repository.

## Testing Requirements

**ALWAYS write tests for new code.** This is a non-negotiable requirement.

- Every new function or method must have corresponding unit tests
- Every bug fix must include a regression test
- Run `go test ./...` before considering any task complete
- Aim for high coverage (80%+) on new code
- Use table-driven tests where appropriate
- Test edge cases and error conditions, not just happy paths

## Project Overview

Plural is a TUI application for managing multiple concurrent Claude Code sessions. Each session runs in its own git worktree, allowing isolated Claude conversations on the same codebase.

## Build and Run

```bash
go build -o plural .     # Build
./plural                 # Run TUI (default)
go test ./...            # Test

# CLI commands and flags
./plural help            # Show help
./plural clean           # Clear sessions, logs, orphaned worktrees, and containers (prompts for confirmation)
./plural clean -y        # Clear without confirmation prompt
./plural demo list       # List available demo scenarios
./plural --debug         # Enable debug logging
./plural --version       # Show version
```

## Debug Logs

```bash
tail -f ~/.plural/logs/plural.log      # Main app logs
tail -f ~/.plural/logs/mcp-*.log       # MCP permission logs (per-session)
tail -f ~/.plural/logs/stream-*.log    # Raw Claude stream messages (per-session, pretty-printed JSON)
```

Log levels: Debug, Info, Warn, Error. Default shows Info+. Use `--debug` for verbose output.

---

## Architecture

### Core Flow

1. User registers git repositories via the TUI
2. Creating a session generates a UUID, creates a git worktree in `.plural-worktrees/<UUID>` (sibling to repo), starts Claude CLI with `--session-id`
3. Each session maintains independent message history and Claude CLI process

### Package Structure

```
main.go                    Entry point, calls cmd.Execute()
cmd/                       CLI commands (Cobra)
├── root.go                Root command, TUI startup, global flags
├── clean.go               "plural clean" - session/worktree cleanup
├── mcp_server.go          "plural mcp-server" - internal MCP server
└── demo.go                "plural demo" - demo generation subcommands

internal/
├── app/                   Main Bubble Tea model
│   ├── app.go             Model, Update/View, key handling
│   ├── shortcuts.go       Keyboard shortcut registry (single source of truth)
│   ├── slash_commands.go  Local slash command handling (/cost, /help, /mcp, /plugins)
│   ├── session_manager.go Session lifecycle, runner caching, message persistence
│   ├── session_state.go   Thread-safe per-session state
│   ├── modal_handlers.go  Modal key handlers (split into _config, _git, _issues, _navigation, _session)
│   ├── msg_handlers.go    Bubble Tea message handlers
│   ├── options.go         Options detection and parsing
│   └── types.go           Shared types
├── claude/                Claude CLI wrapper (stream-json I/O)
│   ├── claude.go          Runner: message handling, MCP server
│   ├── process_manager.go ProcessManager: process lifecycle, auto-recovery
│   ├── runner_interface.go Interfaces for testing
│   ├── mock_runner.go     Mock runner for testing and demos
│   ├── plugins.go         Plugin/marketplace management via Claude CLI
│   └── todo.go            TodoWrite tool parsing and completion detection
├── changelog/             Fetches release notes from GitHub API
├── cli/                   Prerequisites checking (claude, git, gh)
├── clipboard/             Cross-platform clipboard image reading
├── config/                Persists to ~/.plural/
├── demo/                  Demo generation infrastructure
│   ├── scenario.go        Scenario definition types and step builders
│   ├── executor.go        Executes scenarios step-by-step, captures frames
│   ├── vhs.go             VHS tape and asciinema cast generation
│   └── scenarios/         Built-in demo scenarios
├── exec/                  Command executor abstraction for testability
├── git/                   Git operations for merge/PR workflow
├── issues/                Issue provider abstraction (GitHub, Asana)
│   ├── provider.go        Provider interface, Issue struct, ProviderRegistry
│   ├── github.go          GitHub provider (wraps git.FetchGitHubIssues)
│   └── asana.go           Asana provider (HTTP client, ASANA_PAT env var)
├── keys/                  Key string constants for Bubble Tea v2 key events
├── logger/                Thread-safe file logger
├── mcp/                   MCP server for permissions via Unix socket IPC
├── notification/          Desktop notifications (beeep library)
├── process/               Find/kill orphaned Claude processes
├── session/               Git worktree creation/management
└── ui/                    Bubble Tea UI components
    ├── constants.go       Layout constants
    ├── context.go         Singleton ViewContext for layout
    ├── theme.go           Theme system
    ├── styles.go          Lipgloss styles
    ├── header.go          Header bar with gradient, session name, diff stats
    ├── footer.go          Footer with keybindings, flash messages
    ├── sidebar.go         Session list with hierarchy
    ├── chat.go            Chat panel main component
    ├── chat_render.go     Chat message rendering and markdown
    ├── chat_animation.go  Streaming animation effects
    ├── text_selection.go  Mouse-based text selection with clipboard
    ├── view_changes.go    Git diff viewing overlay
    ├── modal.go           Modal container
    └── modals/            Modal implementations (plugins, settings, etc.)
```

### Data Storage

- `~/.plural/config.json` - Repos, sessions, allowed tools, MCP servers, plugins, theme, branch prefix, container settings
- `~/.plural/sessions/<session-id>.json` - Conversation history (last 10,000 lines)
- `~/.plural/logs/` - Debug logs, MCP logs, and stream logs

### Key Patterns

- **Bubble Tea Model-Update-View** with `tea.Msg` for events
- **Focus system**: Tab switches between sidebar and chat
- **Streaming**: Claude responses stream via channel as `ClaudeResponseMsg`
- **Runner caching**: Claude runners cached by session ID in `claudeRunners` map
- **Thread-safe config**: Uses `sync.RWMutex`
- **Per-session state**: Independent state maps for concurrent operation
- **State machine**: `AppState` enum (StateIdle, StateStreamingClaude)
- **Text selection**: Mouse-based selection with ultraviolet screen buffer rendering

### Documented Unusual Patterns

Some patterns in this codebase may appear unusual at first glance. These are intentional design choices documented in detail:

**Text Selection Coordinate System** - See `internal/ui/text_selection.go`
- Mouse events arrive in terminal coordinates, are adjusted to panel coordinates, then viewport coordinates
- Border offset (1px) subtracted: `x := msg.X - 1; y := msg.Y - 1`
- ANSI codes stripped before text extraction since coordinates correspond to visible positions

**Layout Constants** - See `internal/ui/constants.go`
- All magic numbers for layout have documented rationale
- ViewContext singleton (`context.go`) centralizes layout calculations
- Key formula: `ChatViewportHeight = ContentHeight - InputTotalHeight - BorderSize`

**Text Wrapping Width Constants** - See `internal/ui/constants.go`
- All width subtractions for text wrapping are defined as named constants
- Visual width is used (via `lipgloss.Width`), not byte length, since Unicode chars like `•` are multi-byte
- Key constants and their purpose:
  - `ContentPadding = 2`: Horizontal padding applied via `Padding(0, 1)` (1 char each side)
  - `ListItemPrefixWidth = 4`: Visual width of `"  • "` (2 spaces + bullet + space)
  - `NumberedListPrefixWidth = 5`: Visual width of `"  1. "` (for single-digit numbers)
  - `BlockquotePrefixWidth = 4`: Effective width of blockquote left border and padding
  - `OverlayBoxPadding = 4`: Padding inside permission/question/todo boxes
  - `OverlayBoxMaxWidth = 80`: Max width for overlay boxes (readability)
  - `PlanBoxMaxWidth = 100`: Wider max for plan boxes (often contain code)
  - `TableMinColumnWidth = 3`: Minimum readable column width in tables

**Message Cache** - See `internal/ui/chat.go`
- Messages are cached after rendering to avoid expensive re-rendering
- Cache is keyed on `{content, wrapWidth}` - both must match for cache hit
- Cache is cleared and content re-rendered when viewport width changes
- `SetSize()` now triggers `updateContent()` on width change to ensure proper re-wrapping

---

## Implementation Guide

### Adding Keyboard Shortcuts

Shortcuts are defined in a central registry (`shortcuts.go`):

```go
ShortcutRegistry      // Executable shortcuts with handlers
DisplayOnlyShortcuts  // Informational entries for help modal
```

Guards available: `RequiresSidebar`, `RequiresSession`, `Condition` functions.

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

Sessions support parent-child relationships:

- **Fork** (`f`): Creates child session with own worktree, optionally copies history
- **Explore options** (`Ctrl+O`): Parses Claude's numbered options, forks into parallel sessions
- **Merge to parent** (`m`): Merges git changes and appends history to parent, locks child

Merge types enum: `MergeTypeMerge` (to main), `MergeTypePR` (create PR), `MergeTypeParent` (to parent), `MergeTypePush` (push to existing PR)

**Squash on Merge**: Per-repo setting (`config.RepoSquashOnMerge`) that squashes all commits into one when using "Merge to main". Uses `git merge --squash` followed by explicit commit with user-provided message.

### Broadcasting

Broadcast allows sending the same prompt to multiple repositories or sessions at once:

- **Broadcast to repos** (`Ctrl+B`): Opens modal to select repositories, enter an optional session name, and a prompt. Creates new sessions for each selected repo and sends the prompt to all of them. Sessions share a `BroadcastGroupID` for grouping.
- **Broadcast group actions** (`Ctrl+Shift+B`): When on a session that's part of a broadcast group, opens modal with two action options:
  - **Send Prompt**: Send the same message to all (or selected) sessions in the group
  - **Create PRs**: Trigger PR creation for all (or selected) sessions in the group. Skips sessions that already have PRs, are merged, or have uncommitted changes.

**Data structures** (`internal/ui/modals/broadcast.go`):
- `BroadcastState`: Modal for creating new sessions across repos
- `BroadcastGroupState`: Modal for actions on existing broadcast group sessions
- `BroadcastGroupAction`: Enum for `BroadcastActionSendPrompt` and `BroadcastActionCreatePRs`
- `SessionItem`: Represents a session in the broadcast group modal

**Implementation** (`internal/app/modal_handlers_session.go`):
- `handleBroadcastModal()`: Handles new session creation across repos
- `handleBroadcastGroupModal()`: Handles actions on existing broadcast groups
- `createBroadcastSessions()`: Creates sessions and sends initial prompt
- `broadcastToSessions()`: Sends prompt to existing sessions
- `createPRsForSessions()`: Triggers PR creation for multiple sessions

### Session Struct

Session struct (`internal/config/session.go`) tracks:
- `ID`, `RepoPath`, `WorkTree`, `Branch`, `Name`, `CreatedAt`
- `BaseBranch`: Branch the session was created from (e.g., "main", parent branch) - shown in header
- `Started`: Whether Claude CLI has been started
- `Merged`, `PRCreated`: Merge/PR status
- `ParentID`, `MergedToParent`: Parent-child relationships for forked sessions
- `IssueNumber`: Deprecated - use `IssueRef` instead
- `IssueRef`: Generic issue/task reference (supports GitHub and Asana)
- `BroadcastGroupID`: Links sessions created from the same broadcast operation

### Issue/Task Import

Sessions can be created from issues/tasks. Press `i` to import:

**GitHub Issues**: Always available (uses `gh` CLI). Branch naming: `issue-{number}`. When PR created, "Fixes #N" auto-added to body.

**Asana Tasks**: Available when:
1. `ASANA_PAT` environment variable is set (Personal Access Token)
2. Repository has an Asana project mapped in config (`repo_asana_project`)

Branch naming: `task-{slug}` where slug is derived from task name. Asana doesn't support auto-close via PR.

**Data structures** (`internal/issues/`):
- `Provider` interface: Generic issue provider abstraction
- `GitHubProvider`: Wraps `gh issue list`
- `AsanaProvider`: Uses Asana REST API
- `ProviderRegistry`: Manages available providers

**Config** (`internal/config/config.go`):
- `RepoAsanaProject map[string]string`: Maps repo paths to Asana project GIDs

**Session** (`internal/config/session.go`):
- `IssueRef` struct: `Source` (github/asana), `ID`, `Title`, `URL`
- `GetIssueRef()`: Returns IssueRef, converting legacy IssueNumber if needed

### User Preferences

**Themes** (`t` key): 8 built-in themes defined in `internal/ui/theme.go`:
- Dark Purple (default), Nord, Dracula, Gruvbox Dark, Tokyo Night, Catppuccin Mocha, Science Fiction, Light

**Settings** (`,` key): Opens modal for:
- Branch prefix (e.g., `yourname/` creates branches like `yourname/plural-abc123`)
- Desktop notifications toggle
- Squash commits on merge (per-repo): When enabled, all commits on a session branch are squashed into a single commit when merging to main

**Session rename** (`r` key): Renames session and its git branch.

**Message search** (`Ctrl+/`): Search within conversation history when chat is focused.

### Slash Commands

Plural implements its own slash commands because Claude CLI built-in commands (like `/cost`, `/help`, `/mcp`) are designed for interactive REPL mode and don't work in stream-json mode.

Available commands:
- `/cost` - Show token usage and estimated cost for the current session (reads from Claude's JSONL session files)
- `/help` - Show available Plural slash commands
- `/mcp` - Open MCP servers configuration modal
- `/plugins` - Open plugins modal for managing marketplaces and plugins

### Plugin Management

Plural provides a UI for managing Claude Code plugins (`/plugins` command or through modal):

**Implementation** (`internal/claude/plugins.go`):
- `ListMarketplaces()` / `ListPlugins()`: Read from `~/.claude/plugins/` config files
- `AddMarketplace()` / `RemoveMarketplace()`: Manage plugin sources
- `InstallPlugin()` / `UninstallPlugin()`: Install/remove plugins
- `EnablePlugin()` / `DisablePlugin()`: Toggle plugin activation

**Modal** (`internal/ui/modals/plugins.go`):
- Three tabs: Marketplaces, Installed, Discover
- Search filtering in Discover tab
- Keybindings vary by tab:
  - Marketplaces: `a` (add), `d` (delete), `u` (update)
  - Installed: `e` (enable/disable), `u` (uninstall)
  - Discover: `/` (search), `Enter` (install)

Implementation in `internal/app/slash_commands.go`:
- Commands are intercepted in `sendMessage()` before being sent to Claude
- Unknown slash commands are passed through to Claude (for custom commands)
- Cost data is read from `~/.claude/projects/<escaped-path>/<session-id>.jsonl`

### Path Auto-Completion

The Add Repository modal (`a` key) supports Tab completion for paths:

- **PathCompleter** (`internal/ui/modals/completion.go`): Handles filesystem path auto-completion
- Expands `~` to home directory
- Shows completion options when multiple matches exist
- Hidden files only shown when typing `.` prefix
- Use up/down to navigate options, Tab/Enter to select

### Tool Use Rollup

When Claude performs multiple tool operations in sequence, they are collapsed into a compact rollup view to reduce visual noise:

**Display behavior**:
- Shows the most recent tool use with status marker (⏺ in progress, ● complete)
- When multiple tool uses exist, shows "+N more tool uses (ctrl-t to expand)"
- Expanded view shows all tool uses in the current group
- Tool uses are flushed to streaming content when text arrives or streaming finishes

**Data structures** (`internal/ui/chat.go`):
- `ToolUseRollup`: Tracks items and expanded/collapsed state
- `ToolUseItem`: Contains `ToolName`, `ToolInput`, `Complete` flag

**Non-active sessions** (`internal/app/session_state.go`):
- `ToolUseRollupState` mirrors the UI rollup for background sessions
- `AddToolUse()`, `MarkLastToolUseComplete()`, `FlushToolUseRollup()` methods

**Key binding** (`ctrl-t`): Toggle between collapsed and expanded view when chat is focused and multiple tool uses exist.

### Text Selection

Chat panel supports mouse-based text selection with visual highlighting:

- **Single click + drag**: Select arbitrary text region
- **Double click**: Select word (Unicode grapheme-aware via `uniseg`)
- **Triple click**: Select paragraph (finds empty line boundaries)
- **Auto-copy**: Selected text is automatically copied to clipboard on mouse release
- **Esc**: Clear selection
- **Visual highlighting**: Uses `ultraviolet` screen buffer to apply selection background color

Implementation in `internal/ui/text_selection.go`:
- Selection state: `selectionStartCol/Line`, `selectionEndCol/Line`, `selectionActive`
- Multi-click detection: Tracks `lastClickTime`, `clickCount` with 500ms threshold
- Rendering: `selectionView()` applies highlight style to cells in selection range
- Clipboard: Dual approach using OSC 52 escape sequence + native `internal/clipboard` package

### Flash Messages

The footer supports temporary flash messages for user notifications (errors, warnings, info, success):

**Types** (`internal/ui/footer.go`):
- `FlashError` - Red background, ✕ icon
- `FlashWarning` - Amber background, ⚠ icon
- `FlashInfo` - Blue background, ℹ icon
- `FlashSuccess` - Green background, ✓ icon

**Usage from app** (`internal/app/app.go`):
```go
// Show an error message (auto-dismisses after 5 seconds)
cmds = append(cmds, m.ShowFlashError("Failed to save file"))

// Show other types
cmds = append(cmds, m.ShowFlashWarning("Connection unstable"))
cmds = append(cmds, m.ShowFlashInfo("Processing..."))
cmds = append(cmds, m.ShowFlashSuccess("File saved"))

// Custom duration via footer directly
m.footer.SetFlashWithDuration("Custom message", ui.FlashInfo, 10*time.Second)
```

**Behavior**:
- Flash messages replace the keybindings in the footer while active
- Auto-dismiss after `DefaultFlashDuration` (5 seconds)
- `FlashTickMsg` handles expiration checking via periodic ticks

### Todo List Integration

Plural integrates with Claude's TodoWrite tool to display task progress:

**Types** (`internal/claude/todo.go`):
- `TodoStatus`: `pending`, `in_progress`, `completed`
- `TodoItem`: Contains `Content`, `Status`, `ActiveForm` (present tense description)
- `TodoList`: Collection of todo items with helper methods

**Behavior**:
- Todo lists are parsed from Claude's `TodoWrite` tool calls
- When all items in a todo list are completed, it's rendered and appended to chat history
- `ParseTodoWriteInput()` extracts todo items from JSON input
- `IsComplete()` checks if all items are done
- `CountByStatus()` returns counts of pending, in-progress, and completed items

**Todo Sidebar Display** (`internal/ui/chat.go`):
- When a todo list is active, it displays as a scrollable sidebar on the right side of the chat panel
- The sidebar takes 1/4 of the chat panel width (`TodoSidebarWidthRatio = 4`)
- Chat history scrolls on the left while the todo list is independently scrollable on the right
- Sidebar appears automatically when Claude creates a todo list, disappears when cleared
- `SetTodoList()` and `ClearTodoList()` trigger layout recalculation via `SetSize()`
- `TodoSidebarStyle` uses a connected border style (shares left edge with chat panel)
- `renderTodoListForSidebar()` renders without box border since panel has its own borders
- `todoViewport` is a separate viewport for scrollable todo list content
- Mouse wheel events are routed to the appropriate viewport based on X coordinate:
  - Events over the main chat area go to the main viewport
  - Events over the todo sidebar (X >= mainWidth) go to the todo viewport

### Log Viewer

The log viewer (`ctrl-l`) provides an in-chat overlay for viewing Plural's log files:

**Implementation** (`internal/ui/view_logs.go`):
- `LogFile` struct: `Name`, `Path`, `Content`
- `LogViewerState`: Tracks files, current index, viewport, follow mode
- `GetLogFiles(sessionID)`: Discovers available log files

**Log files displayed**:
- Main debug log (`~/.plural/logs/plural.log`)
- MCP logs (`~/.plural/logs/mcp-*.log`) - per-session permission server logs
- Stream logs (`~/.plural/logs/stream-*.log`) - raw Claude stream messages

**Keybindings in log viewer**:
- `←/→` or `h/l`: Navigate between log files
- `↑/↓` or `j/k`: Scroll within log
- `f`: Toggle follow tail mode (auto-scroll to bottom)
- `r`: Refresh current log file
- `q` or `Esc` or `ctrl-l`: Exit log viewer

**Features**:
- Syntax highlighting for log levels (ERROR=red, WARN=amber, INFO=blue, DEBUG=muted)
- Follow tail mode (enabled by default) auto-scrolls to latest logs
- Current session's logs are prioritized in the file list

### Git Diff Stats Display

The header displays uncommitted changes for the current session:

**Implementation** (`internal/ui/header.go`):
- `DiffStats` struct: `FilesChanged`, `Additions`, `Deletions`
- Stats shown in header when session has uncommitted changes
- Format: `3 files, +157, -5` with color coding (green for additions, red for deletions)
- `SetDiffStats()` updates the header with current stats

**Data flow**:
- `git/git.go`: `GetDiffStats()` runs `git diff --shortstat` to get stats
- `session_manager.go`: Fetches stats for current session
- `app.go`: Updates header when session changes or on refresh

### Preview Session in Main

Allows previewing a session's branch in the main repository so dev servers (puma, etc.) pick up the changes:

**How it works**:
1. User presses `p` on a session in the sidebar
2. System checks if session worktree has uncommitted changes - if so, auto-commits them first
3. System checks if main repo has uncommitted changes (error if dirty)
4. Records current branch in main repo (to restore later)
5. Checks out the session's branch in the main repo
6. Header shows `[PREVIEW]` indicator in amber/warning color
7. Press `p` again to end preview and restore the original branch

**State tracking** (`internal/config/config.go`):
- `PreviewSessionID`: Session ID being previewed (empty if none)
- `PreviewPreviousBranch`: Branch to restore when preview ends
- `PreviewRepoPath`: Path to main repo where preview is active
- Helper methods: `StartPreview()`, `EndPreview()`, `GetPreviewState()`, `IsPreviewActive()`, `GetPreviewSessionID()`

**Git operations** (`internal/git/git.go`):
- `GetCurrentBranch(ctx, repoPath)`: Returns current branch name
- `CheckoutBranch(ctx, repoPath, branch)`: Checks out specified branch

**UI** (`internal/ui/header.go`):
- `previewActive` field controls display of `[PREVIEW]` indicator
- `SetPreviewActive(bool)` updates the indicator state
- Preview indicator uses warning color (amber) and bold styling

**Safety**:
- Cannot start preview if main repo has uncommitted changes
- Cannot end preview if changes were made in main during preview (must commit/stash first)
- Only one preview can be active at a time across all sessions
- Preview state persists across app restarts (stored in config)

### Container Mode (Docker)

Sessions can optionally run Claude CLI inside Docker containers with `--dangerously-skip-permissions`. The container IS the sandbox, so all regular tool permissions are auto-approved. However, interactive prompts (`AskUserQuestion`, `ExitPlanMode`) still route through the TUI via a lightweight MCP server running inside the container with `--auto-approve` (wildcard `"*"` allowed tools). The MCP socket server runs on the host and communicates with the in-container MCP subprocess via TCP (since Unix sockets can't cross the Docker container boundary).

**Authentication Requirement**: Container mode requires one of: `ANTHROPIC_API_KEY` (env var or macOS keychain `anthropic_api_key`), or `CLAUDE_CODE_OAUTH_TOKEN` (long-lived token from `claude setup-token`, ~1 year lifetime). The short-lived OAuth access token from the macOS keychain is NOT supported because it rotates every ~8-12 hours and would become invalid inside the container. The UI shows a warning when the container checkbox is checked without available credentials, and session creation is blocked.

**Config** (`internal/config/config.go`):
- `ContainerImage string`: Container image name (default: `"ghcr.io/zhubert/plural-claude"`)
- `GetContainerImage`/`SetContainerImage`: Image name with default

**Session** (`internal/config/session.go`):
- `Containerized bool`: Set at creation time via New Session modal checkbox, immutable. Determines whether the session runs in a container.
- Forked sessions inherit the parent's `Containerized` flag.

**ProcessManager** (`internal/claude/process_manager.go`):
- `ProcessConfig.Containerized`/`ContainerImage`/`SocketPath`: Passed from Runner
- `BuildCommandArgs()`: When containerized, uses `--dangerously-skip-permissions` plus `--mcp-config`/`--permission-prompt-tool` (when MCPConfigPath is set) for AskUserQuestion/ExitPlanMode routing
- `Start()`: When containerized, checks `ContainerAuthAvailable()` first, then builds command as `docker run -i --rm ...` wrapping `claude`
- `Stop()`: Runs `docker rm -f` as defense-in-depth cleanup
- `ContainerAuthAvailable()`: Checks if credentials exist (ANTHROPIC_API_KEY, CLAUDE_CODE_OAUTH_TOKEN, or keychain)
- `buildContainerRunArgs()`: Constructs `docker run` arguments with `--add-host host.docker.internal:host-gateway`, worktree mount, `--env-file` for auth, MCP config mount, working directory
- `writeContainerAuthFile()`: Writes credentials to temp file in Docker env-file format for `--env-file` flag (API key, OAuth token, or keychain)

**Runner** (`internal/claude/claude.go`):
- `SetContainerized(bool, string)`: Stores container mode and image
- `SendContent()`: Calls `ensureServerRunning()` for all sessions (container and non-container)
- `ensureProcessRunning()`: Passes container fields and socket path to `ProcessConfig`
- `Stop()`: Cleans up socket server and MCP config for all sessions

**UI**:
- New Session modal (`n`): Container checkbox (focus index 3, when Docker is available), auth warning when no API key
- Fork Session modal (`f`): Container checkbox (focus index 2, defaults to parent's state), auth warning when no API key
- Broadcast modal (`Ctrl+B`): Container checkbox (focus index 3, when Docker is available), auth warning when no API key
- Header: `[CONTAINER]` indicator in green when viewing a containerized session
- Modal constructors accept `containerAuthAvailable bool` parameter for auth state

**MCP Config** (`internal/claude/mcp_config.go`):
- `ContainerGatewayIP`: `"host.docker.internal"` — hostname for reaching the host from inside Docker containers
- `ensureServerRunning()`: Runs socket server on host for both container and non-container sessions
- `createMCPConfigLocked()`: Host config pointing to current plural binary
- `createContainerMCPConfigLocked()`: Container config pointing to `/usr/local/bin/plural` with `--auto-approve`

**MCP Server** (`cmd/mcp_server.go`):
- `--auto-approve` flag: When set, passes `[]string{"*"}` as allowedTools to auto-approve all regular permissions
- Wildcard `"*"` in `isToolAllowed()` matches any tool, but AskUserQuestion/ExitPlanMode are handled before the allowed check

**Dockerfile** (repo root):
- Multi-stage build: builder stage compiles plural and gopls from source, runtime stage uses `alpine`
- Installs git, Claude CLI via npm, and builds plural binary at `/usr/local/bin/plural`
- Supports multi-arch builds via `$TARGETOS`/`$TARGETARCH` build args
- Build locally: `./scripts/build-container.sh` or `docker build -t ghcr.io/zhubert/plural-claude .`
- Pre-built images: `docker pull ghcr.io/zhubert/plural-claude`

**GitHub Actions** (`.github/workflows/docker.yml`):
- Triggers on version tags (`v*`)
- Builds `linux/amd64,linux/arm64` and pushes to `ghcr.io/zhubert/plural-claude:{tag}` + `:latest`

**Container orphan cleanup** (`internal/process/process.go`):
- `OrphanedContainer` struct: `Name string` (e.g., `plural-abc123`)
- `FindOrphanedContainers()`: Runs `docker ps -a --format '{{.Names}}'`, filters `plural-*`, compares against known sessions
- `CleanupOrphanedContainers()`: Calls find, then `docker rm -f` for each orphan
- Returns empty list (no error) if `docker` CLI is not installed
- Integrated into `cmd/clean.go` as a 4th parallel cleanup goroutine

**Platform support** (`internal/process/process.go`):
- `ContainersSupported()`: Returns true when Docker is installed (all platforms)
- New Session modal hides container checkbox when Docker is not installed
- Warning displayed near checkbox: containers are defense in depth, not a complete security boundary

**Known limitations (prototype)**:
- External MCP servers not supported in container mode

### Claude Process Management

Claude CLI process management is split across two components for better separation of concerns:

**ProcessManager** (`internal/claude/process_manager.go`):
- Manages the Claude CLI process lifecycle (start, stop, monitor)
- Handles stdin/stdout/stderr pipes
- Implements auto-recovery on process crash (max 3 attempts)
- Tracks restart attempts and provides reset on successful response
- Uses callbacks to notify the Runner of process events

**Runner** (`internal/claude/claude.go`):
- High-level API for Claude interaction
- Manages message history and streaming state
- Handles response parsing and routing
- Manages MCP server for permission prompts
- Uses ProcessManager internally for process operations

Key interfaces:
- `ProcessManagerInterface` - enables mocking process management in tests
- `RunnerInterface` - enables mocking Claude runners in tests

### Process Error Handling and Recovery

The ProcessManager (`internal/claude/process_manager.go`) implements robust error handling:

**Response Channel Full Handling**:
- When the response channel is full for >10 seconds, reports error instead of silently dropping chunks
- User sees `[Error: Response buffer full - some output may be lost]` message

**Auto-Recovery on Crash** (max 3 attempts):
- If Claude process crashes unexpectedly, automatically restarts it
- User sees `[Process crashed, attempting restart 1/3...]` message
- Restart attempts tracked per session, reset on successful response
- After max attempts exceeded, reports fatal error with stderr content

**Zombie Process Detection** (`internal/process/process.go`):
- `plural --prune` now also finds and kills orphaned Claude processes
- Uses `pgrep` to find processes with `--session-id` flag
- Compares against known sessions to identify orphans

Constants in `internal/claude/claude.go`:
```go
MaxProcessRestartAttempts = 3               // Max auto-restart attempts
ProcessRestartDelay = 500 * time.Millisecond // Delay between restarts
ResponseChannelFullTimeout = 10 * time.Second // Before reporting full channel
```

### Demo Generation

Plural includes infrastructure for generating demo recordings programmatically. This uses mock runners (same as tests) to simulate Claude responses without real processes.

**CLI Commands:**
```bash
plural demo list              # List available scenarios
plural demo run basic         # Run scenario, print frames to stdout
plural demo generate basic    # Generate VHS tape file
plural demo cast basic        # Generate asciinema cast file

# Options
-o, --output <file>           # Output file path
-w, --width <int>             # Terminal width (default: 120)
-h, --height <int>            # Terminal height (default: 40)
--no-capture-all              # Don't capture frame after every step
```

**Rendering demos:**
```bash
# VHS (Charmbracelet tool) - renders to GIF/MP4
vhs basic.tape

# asciinema - plays in terminal or web player
asciinema play basic.cast
```

**Creating new scenarios** in `internal/demo/scenarios/`:
```go
var MyScenario = &demo.Scenario{
    Name:        "my-demo",
    Description: "Demonstrates feature X",
    Width:       120,
    Height:      40,
    Setup: &demo.ScenarioSetup{
        Repos:    []string{"/demo/repo"},
        Sessions: []config.Session{...},
    },
    Steps: []demo.Step{
        demo.Wait(500 * time.Millisecond),
        demo.Annotate("Press Enter to select"),
        demo.Key("enter"),
        demo.Type("Hello Claude"),
        demo.Key("enter"),
        demo.StreamingTextResponse("I'll help you...", 10),
        demo.Permission("Bash", "ls -la"),
        demo.Key("y"),
    },
}
```

**Step types:**
- `Wait(duration)` - Pause for timing
- `Key(key)` / `KeyWithDesc(key, desc)` - Single key press
- `Type(text)` / `TypeWithDesc(text, desc)` - Type characters
- `TextResponse(text)` - Simple Claude response
- `StreamingTextResponse(text, chunkSize)` - Streaming response
- `StartStreaming(initialText)` - Start streaming without completing (for parallel work demos)
- `Permission(tool, desc)` - Simulate permission request
- `Question(questions...)` - Simulate question request
- `Annotate(text)` - Add caption to next frame
- `Capture()` - Force frame capture

**Git Command Mocking:**
The demo executor uses a MockExecutor to intercept git commands, enabling demos of fork/merge operations without real filesystem changes. Common git commands are pre-mocked with sensible defaults. Custom responses can be added via `executor.AddMockResponse()`.

### Service Pattern with Context Propagation

The `internal/git` and `internal/session` packages use a service struct pattern with explicit dependency injection and context propagation:

**Service Structs:**
- `GitService` (`internal/git/service.go`) - All git operations (status, commit, merge, PR)
- `SessionService` (`internal/session/service.go`) - Session lifecycle (create, delete, validate)

**Key Principles:**
1. **Context as first parameter**: All I/O operations accept `context.Context` for cancellation/timeout
2. **Explicit dependencies**: Services hold their executor rather than using global state
3. **Testability**: `NewGitServiceWithExecutor()` / `NewSessionServiceWithExecutor()` for mocking

**Usage in app:**
```go
// In app.Model (internal/app/app.go)
type Model struct {
    gitService     *git.GitService
    sessionService *session.SessionService
    // ...
}

func New(cfg *config.Config, version string) *Model {
    gitSvc := git.NewGitService()
    sessionSvc := session.NewSessionService()
    return &Model{
        gitService:     gitSvc,
        sessionService: sessionSvc,
        sessionMgr:     NewSessionManager(cfg, gitSvc),
        // ...
    }
}

// Calling service methods with context
ctx := context.Background()
status, err := m.gitService.GetWorktreeStatus(ctx, sess.WorkTree)
sess, err := m.sessionService.Create(ctx, repoPath, branch, prefix, basePoint)
```

**For testing/demos:**
```go
mockExec := pexec.NewMockExecutor(nil)
mockExec.AddPrefixMatch("git", []string{"status"}, pexec.MockResponse{
    Stdout: []byte("On branch main\n"),
})

mockGitService := git.NewGitServiceWithExecutor(mockExec)
mockSessionService := session.NewSessionServiceWithExecutor(mockExec)

model.SetGitService(mockGitService)
model.SetSessionService(mockSessionService)
```

### Command Executor Interface

The `internal/exec` package provides a `CommandExecutor` interface for abstracting command execution:

```go
type CommandExecutor interface {
    Run(ctx, dir, name string, args ...string) (stdout, stderr []byte, err error)
    Output(ctx, dir, name string, args ...string) ([]byte, error)
    CombinedOutput(ctx, dir, name string, args ...string) ([]byte, error)
    Start(ctx, dir, name string, args ...string) (CommandHandle, error)
}
```

**Implementations:**
- `RealExecutor` - Wraps `os/exec.Command` for production use
- `MockExecutor` - Returns pre-recorded responses for testing/demos

---

## Dependencies

Charm's Bubble Tea v2 stack:
- `charm.land/bubbletea/v2` (v2.0.0-rc.2)
- `charm.land/bubbles/v2` (v2.0.0-rc.1)
- `charm.land/lipgloss/v2` (v2.0.0-beta.3+)
- `github.com/charmbracelet/ultraviolet` - Screen buffer for text selection rendering
- `github.com/charmbracelet/x/ansi` - ANSI escape code handling
- `github.com/alecthomas/chroma/v2` - Syntax highlighting for code blocks
- `github.com/google/uuid`
- `github.com/gen2brain/beeep` - Desktop notifications
- `github.com/rivo/uniseg` - Unicode grapheme segmentation for word selection
- `golang.design/x/clipboard` - Cross-platform clipboard (Linux/Windows)

**Bubble Tea v2 API notes:**
- Imports use `charm.land/*` not `github.com/charmbracelet/*`
- `tea.KeyMsg` → `tea.KeyPressMsg`
- `tea.View` returns declarative view with properties
- Viewport uses `SetWidth()`/`SetHeight()` methods

**Bubble Tea v2 key strings** — Use constants from `internal/keys/` instead of hardcoded strings:

```go
import "github.com/zhubert/plural/internal/keys"

// In key handlers:
case keys.Escape:  // "esc"
case keys.Enter:   // "enter"
case keys.Up:      // "up"
case keys.CtrlC:   // "ctrl+c"
```

The `keys` package derives all values from `tea.KeyPressMsg{Code: tea.KeyXxx}.String()` at init time, guaranteeing correctness. Single-character keys like `"a"`, `"y"`, `"?"` are not included (unambiguous, cannot be misspelled).

| Key | Constant | String Value |
|-----|----------|-------------|
| Escape | `keys.Escape` | `"esc"` |
| Enter | `keys.Enter` | `"enter"` |
| Space | `keys.Space` | `"space"` |
| Tab | `keys.Tab` | `"tab"` |
| Shift+Tab | `keys.ShiftTab` | `"shift+tab"` |
| Backspace | `keys.Backspace` | `"backspace"` |
| Delete | `keys.Delete` | `"delete"` |
| Up arrow | `keys.Up` | `"up"` |
| Down arrow | `keys.Down` | `"down"` |
| Left arrow | `keys.Left` | `"left"` |
| Right arrow | `keys.Right` | `"right"` |
| Page Up | `keys.PgUp` | `"pgup"` |
| Page Down | `keys.PgDown` | `"pgdown"` |
| Home | `keys.Home` | `"home"` |
| End | `keys.End` | `"end"` |
| Ctrl+C | `keys.CtrlC` | `"ctrl+c"` |
| Ctrl+S | `keys.CtrlS` | `"ctrl+s"` |

Regular letter/number keys return their lowercase value (e.g., `"a"`, `"1"`, `"/"`).
Always use `keys.` constants for special keys — never hardcode strings like `"esc"` or `"escape"`.

---

## Releasing

```bash
./scripts/release.sh patch            # v0.0.3 -> v0.0.4
./scripts/release.sh minor            # v0.0.3 -> v0.1.0
./scripts/release.sh major            # v0.0.3 -> v1.0.0
./scripts/release.sh patch --dry-run  # Dry run
```

GoReleaser builds binaries for Linux/macOS (amd64/arm64) and updates Homebrew tap at `zhubert/homebrew-tap`.

## License

MIT License - see [LICENSE](LICENSE) for details.
