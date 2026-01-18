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
tail -f /tmp/plural-debug.log    # Main app logs
tail -f /tmp/plural-mcp-*.log    # MCP permission logs (per-session)
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
│   ├── slash_commands.go  Local slash command handling (/cost, /help)
│   ├── session_manager.go Session lifecycle, runner caching, message persistence
│   ├── session_state.go   Thread-safe per-session state
│   ├── modal_handlers.go  Modal key handlers
│   └── types.go           Shared types
├── claude/                Claude CLI wrapper (stream-json I/O)
│   ├── claude.go          Runner: message handling, MCP server
│   ├── process_manager.go ProcessManager: process lifecycle, auto-recovery
│   └── runner_interface.go Interfaces for testing
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
    └── *.go               sidebar, chat, modal, header, footer
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
- **Explore options** (`Ctrl+P`): Parses Claude's numbered options, forks into parallel sessions
- **Merge to parent** (`m`): Merges git changes and appends history to parent, locks child

Merge types enum: `MergeTypeMerge` (to main), `MergeTypePR` (create PR), `MergeTypeParent` (to parent), `MergeTypePush` (push to existing PR)

### GitHub Issue Sessions

Issue number stored in session. When PR created from issue session, "Fixes #N" auto-added to PR body. Branch naming: `issue-{number}`.

### User Preferences

**Themes** (`t` key): 8 built-in themes defined in `internal/ui/theme.go`:
- Dark Purple (default), Nord, Dracula, Gruvbox Dark, Tokyo Night, Catppuccin Mocha, Science Fiction, Light

**Settings** (`,` key): Opens modal for:
- Branch prefix (e.g., `yourname/` creates branches like `yourname/plural-abc123`)
- Desktop notifications toggle

**Session rename** (`r` key): Renames session and its git branch.

**Message search** (`Ctrl+/`): Search within conversation history when chat is focused.

### Slash Commands

Plural implements its own slash commands because Claude CLI built-in commands (like `/cost`, `/help`, `/mcp`) are designed for interactive REPL mode and don't work in stream-json mode.

Available commands:
- `/cost` - Show token usage and estimated cost for the current session (reads from Claude's JSONL session files)
- `/help` - Show available Plural slash commands
- `/mcp` - Open MCP servers configuration modal (same as `s` shortcut)
- `/plugins` - Open plugin directories configuration modal

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

### Text Selection

Chat panel supports mouse-based text selection with visual highlighting:

- **Single click + drag**: Select arbitrary text region
- **Double click**: Select word (Unicode grapheme-aware via `uniseg`)
- **Triple click**: Select paragraph (finds empty line boundaries)
- **Auto-copy**: Selected text is automatically copied to clipboard on mouse release
- **Esc**: Clear selection
- **Visual highlighting**: Uses `ultraviolet` screen buffer to apply selection background color

Implementation in `internal/ui/chat.go`:
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

**Response Read Timeout** (2 minutes):
- Prevents UI freeze when Claude process hangs mid-response
- Uses goroutine-based timeout on `ReadString()` calls
- On timeout, kills the hung process and reports error to user

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
ResponseReadTimeout = 2 * time.Minute       // Timeout for hung process detection
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

### Command Executor Pattern

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

**Usage in packages:**
```go
// In session or git package
var executor pexec.CommandExecutor = pexec.NewRealExecutor()

func SetExecutor(e pexec.CommandExecutor) { executor = e }
func GetExecutor() pexec.CommandExecutor { return executor }

// Use in functions
output, err := executor.CombinedOutput(ctx, dir, "git", "status")
```

**Adding mock responses:**
```go
mock := pexec.NewMockExecutor(nil)
mock.AddPrefixMatch("git", []string{"status"}, pexec.MockResponse{
    Stdout: []byte("On branch main\n"),
})
session.SetExecutor(mock)
git.SetExecutor(mock)
```

---

## Dependencies

Charm's Bubble Tea v2 stack:
- `charm.land/bubbletea/v2` (v2.0.0-rc.2)
- `charm.land/bubbles/v2` (v2.0.0-rc.1)
- `charm.land/lipgloss/v2` (v2.0.0-beta.3)
- `github.com/charmbracelet/ultraviolet` - Screen buffer for text selection rendering
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
