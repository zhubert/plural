# CLAUDE.md

Guidance for Claude Code when working with this repository.

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
main.go                    Entry point, Bubble Tea setup, mcp-server subcommand

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
├── changelog/             Fetches release notes from GitHub API
├── cli/                   Prerequisites checking (claude, git, gh)
├── clipboard/             Cross-platform clipboard image reading
├── config/                Persists to ~/.plural/
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

- `~/.plural/config.json` - Repos, sessions, allowed tools, MCP servers, theme, branch prefix
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

### Slash Commands

Plural implements its own slash commands because Claude CLI built-in commands (like `/cost`, `/help`, `/mcp`) are designed for interactive REPL mode and don't work in stream-json mode.

Available commands:
- `/cost` - Show token usage and estimated cost for the current session (reads from Claude's JSONL session files)
- `/help` - Show available Plural slash commands
- `/mcp` - Open MCP servers configuration modal (same as `s` shortcut)

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
- **Key strings**: Use `"space"` not `" "`, `"tab"` not `"\t"`, etc.

---

## Releasing

```bash
./scripts/release.sh v0.0.5           # Full release
./scripts/release.sh v0.0.5 --dry-run # Dry run
```

GoReleaser builds binaries for Linux/macOS (amd64/arm64) and updates Homebrew tap at `zhubert/homebrew-tap`.

## License

MIT License - see [LICENSE](LICENSE) for details.
