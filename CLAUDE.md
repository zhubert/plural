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
./plural                 # Run
go test ./...            # Test

# CLI flags
./plural --check-prereqs # Validate required tools
./plural --clear         # Clear all sessions and log files
./plural --prune         # Remove orphaned worktrees
./plural --debug         # Enable debug logging
```

## Debug Logs

```bash
tail -f /tmp/plural-debug.log      # Main app logs
tail -f /tmp/plural-mcp-*.log      # MCP permission logs (per-session)
tail -f /tmp/plural-stream-*.log   # Raw Claude stream messages (per-session, pretty-printed JSON)
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
main.go                    Entry point, Bubble Tea setup, mcp-server/demo subcommands

internal/
├── app/                   Main Bubble Tea model
│   ├── app.go             Model, Update/View, key handling
│   ├── shortcuts.go       Keyboard shortcut registry (single source of truth)
│   ├── slash_commands.go  Local slash command handling (/cost, /help, /mcp, /plugins)
│   ├── session_manager.go Session lifecycle, runner caching, message persistence
│   ├── session_state.go   Thread-safe per-session state
│   ├── modal_handlers.go  Modal key handlers (split into _config, _git, _github, _navigation, _session)
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

- `~/.plural/config.json` - Repos, sessions, allowed tools, MCP servers, plugins, theme, branch prefix
- `~/.plural/sessions/<session-id>.json` - Conversation history (last 10,000 lines)

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

### Session Struct

Session struct (`internal/config/config.go`) tracks:
- `ID`, `RepoPath`, `WorkTree`, `Branch`, `Name`, `CreatedAt`
- `BaseBranch`: Branch the session was created from (e.g., "main", parent branch) - shown in header
- `Started`: Whether Claude CLI has been started
- `Merged`, `PRCreated`: Merge/PR status
- `ParentID`, `MergedToParent`: Parent-child relationships for forked sessions
- `IssueNumber`: GitHub issue number if created from issue import

### GitHub Issue Sessions

Issue number stored in session. When PR created from issue session, "Fixes #N" auto-added to PR body. Branch naming: `issue-{number}`.

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
- When a todo list is active, it displays as a sidebar on the right side of the chat panel
- The sidebar takes 1/4 of the chat panel width (`TodoSidebarWidthRatio = 4`)
- Chat history scrolls on the left while the todo list remains fixed on the right
- Sidebar appears automatically when Claude creates a todo list, disappears when cleared
- `SetTodoList()` and `ClearTodoList()` trigger layout recalculation via `SetSize()`
- `TodoSidebarStyle` uses a connected border style (shares left edge with chat panel)
- `renderTodoListForSidebar()` renders without box border since panel has its own borders

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

**Bubble Tea v2 key strings** (use `msg.String()` on `tea.KeyPressMsg`):

| Key | String | NOT |
|-----|--------|-----|
| Escape | `"escape"` | `"esc"` |
| Enter | `"enter"` | `"return"` |
| Space | `"space"` | `" "` |
| Tab | `"tab"` | `"\t"` |
| Backspace | `"backspace"` | |
| Delete | `"delete"` | |
| Up arrow | `"up"` | |
| Down arrow | `"down"` | |
| Left arrow | `"left"` | |
| Right arrow | `"right"` | |
| Page Up | `"pgup"` | |
| Page Down | `"pgdown"` | |
| Home | `"home"` | |
| End | `"end"` | |
| Ctrl+C | `"ctrl+c"` | |
| Ctrl+S | `"ctrl+s"` | |
| Shift+Tab | `"shift+tab"` | |

Regular letter/number keys return their lowercase value (e.g., `"a"`, `"1"`, `"/"`).
Always test key handling with actual keypresses when in doubt.

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
