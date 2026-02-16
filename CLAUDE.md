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

## Build and Run

```bash
go build -o plural .     # Build
./plural                 # Run TUI (default)
go test ./...            # Test

./plural help            # Show help
./plural clean           # Clear sessions, logs, orphaned worktrees, and containers
./plural clean -y        # Clear without confirmation prompt
./plural --debug         # Enable debug logging
./plural --version       # Show version
```

## Debug Logs

```bash
tail -f ~/.plural/logs/plural.log      # Main app logs
tail -f ~/.plural/logs/mcp-*.log       # MCP permission logs (per-session)
tail -f ~/.plural/logs/stream-*.log    # Raw Claude stream messages (per-session)
```

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
internal/
├── app/                   Main Bubble Tea model (app.go, shortcuts.go, session_manager.go, modal_handlers*.go)
├── claude/                Claude CLI wrapper (runner in claude.go, process in process_manager.go)
├── config/                Config and session structs, persists to ~/.plural/ or XDG dirs
├── demo/                  Demo recording infrastructure (scenarios in demo/scenarios/)
├── exec/                  CommandExecutor interface (RealExecutor, MockExecutor)
├── git/                   GitService - all git operations with context propagation
├── issues/                Issue providers (GitHub via gh CLI, Asana via REST API)
├── keys/                  Key string constants for Bubble Tea v2 key events
├── mcp/                   MCP server for permission prompts via Unix socket IPC
├── paths/                 XDG Base Directory path resolution
├── process/               Find/kill orphaned Claude processes and Docker containers
├── session/               SessionService - worktree creation/management
└── ui/                    Bubble Tea UI components (chat, sidebar, header, footer, modals/)
```

### Data Storage

Default: `~/.plural/`. Supports XDG Base Directory Specification (see `internal/paths/paths.go`):
- Config (`XDG_CONFIG_HOME`): `config.json`
- Data (`XDG_DATA_HOME`): `sessions/*.json` (conversation history, last 10,000 lines)
- State (`XDG_STATE_HOME`): `logs/`

### Key Patterns

- **Bubble Tea Model-Update-View** with `tea.Msg` for events
- **Focus system**: Tab switches between sidebar and chat
- **Streaming**: Claude responses stream via channel as `ClaudeResponseMsg`
- **Runner caching**: Claude runners cached by session ID in `claudeRunners` map
- **Thread-safe config**: Uses `sync.RWMutex`
- **State machine**: `AppState` enum (StateIdle, StateStreamingClaude)
- **Service pattern**: `GitService` and `SessionService` use explicit dependency injection with `CommandExecutor` interface for testability. All I/O methods take `context.Context` as first param. Use `NewXxxServiceWithExecutor()` for mocking in tests.

### Unusual Patterns (Intentional)

**Text Selection Coordinates** (`internal/ui/text_selection.go`): Mouse events go through terminal -> panel -> viewport coordinate transforms. Border offset (1px) subtracted. ANSI stripped before text extraction.

**Layout Constants** (`internal/ui/constants.go`): All magic numbers have documented rationale. Visual width (via `lipgloss.Width`) is used, not byte length, since Unicode chars like `•` are multi-byte. All width subtractions are named constants.

**Message Cache** (`internal/ui/chat.go`): Keyed on `{content, wrapWidth}`. `SetSize()` triggers `updateContent()` on width change.

---

## Implementation Guide

### Adding Keyboard Shortcuts

Shortcuts are defined in a central registry (`internal/app/shortcuts.go`):

```go
ShortcutRegistry      // Executable shortcuts with handlers
DisplayOnlyShortcuts  // Informational entries for help modal
```

Guards: `RequiresSidebar`, `RequiresSession`, `Condition` functions.

To add a new shortcut:
1. Add entry to `ShortcutRegistry` in `shortcuts.go`
2. Create handler function (e.g., `shortcutNewFeature`)
3. Shortcut automatically appears in help modal and works from both key press and help modal selection

### Permission System

1. Claude CLI started with `--permission-prompt-tool mcp__plural__permission`
2. MCP server communicates with TUI via Unix socket (`/tmp/plural-<session-id>.sock`)
3. Permission prompts appear inline in chat (y/n/a responses)
4. Allowed tools: defaults + global (`allowed_tools`) + per-repo (`repo_allowed_tools`)

### Container Mode

Sessions can run Claude CLI inside Docker containers with `--dangerously-skip-permissions`. The container IS the sandbox. Interactive prompts (AskUserQuestion, ExitPlanMode) still route through TUI via MCP server over TCP (Unix sockets can't cross container boundary).

Auth: `ANTHROPIC_API_KEY`, macOS keychain `anthropic_api_key`, or `CLAUDE_CODE_OAUTH_TOKEN`. Short-lived OAuth tokens NOT supported (rotate too fast).

Key files: `process_manager.go` (builds `docker run` args), `mcp_config.go` (container MCP config), `cmd/mcp_server.go` (`--auto-approve` flag).

**Docker Image Architecture**: The `ghcr.io/zhubert/plural-claude` image downloads the plural binary from GitHub releases at build time (rather than building from source). This makes the image stable and reusable across versions. The Dockerfile accepts a `PLURAL_VERSION` build arg (default: `latest`) to pin to a specific version. See `Dockerfile` and `scripts/build-container.sh`.

**Auto-update on Startup**: The container's entrypoint.sh checks for newer plural versions at startup and automatically downloads/installs updates if available. This runs before switching to the claude user, ensuring the binary at `/usr/local/bin/plural` stays current. The update check:
- Times out quickly (5s connect, 10s total for API check)
- Falls back gracefully on network failures
- Logs all update activity with `[plural-update]` prefix
- Can be disabled by setting `PLURAL_SKIP_UPDATE=1` env var
- Uses the same architecture mapping as the Dockerfile (amd64→x86_64, arm64→arm64)

### Flash Messages

```go
cmds = append(cmds, m.ShowFlashError("Failed to save"))
cmds = append(cmds, m.ShowFlashWarning("Connection unstable"))
cmds = append(cmds, m.ShowFlashInfo("Processing..."))
cmds = append(cmds, m.ShowFlashSuccess("File saved"))
```

Auto-dismiss after 5 seconds. Types: `FlashError`, `FlashWarning`, `FlashInfo`, `FlashSuccess`.

### Slash Commands

Plural implements its own slash commands because Claude CLI built-ins don't work in stream-json mode. Commands are intercepted in `sendMessage()` before being sent to Claude. Unknown slash commands pass through to Claude.

### Session Struct

Key fields on `config.Session`: `ID`, `RepoPath`, `WorkTree`, `Branch`, `Name`, `BaseBranch`, `Started`, `Merged`, `PRCreated`, `ParentID`, `MergedToParent`, `Containerized`, `IssueRef`, `BroadcastGroupID`. See `internal/config/session.go`.

### Demo Generation

Uses mock runners to simulate Claude responses. Scenarios defined in `internal/demo/scenarios/`. See existing scenarios for examples of the step builder API (`demo.Wait`, `demo.Key`, `demo.Type`, `demo.StreamingTextResponse`, `demo.Permission`, etc.).

---

## Dependencies

Charm's Bubble Tea v2 stack:
- `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2`
- `github.com/charmbracelet/ultraviolet` - Screen buffer for text selection
- `github.com/charmbracelet/x/ansi` - ANSI escape code handling

**Bubble Tea v2 API notes:**
- Imports use `charm.land/*` not `github.com/charmbracelet/*`
- `tea.KeyMsg` -> `tea.KeyPressMsg`
- `tea.View` returns declarative view with properties
- Viewport uses `SetWidth()`/`SetHeight()` methods

**Key strings** — Use constants from `internal/keys/` instead of hardcoded strings:

```go
import "github.com/zhubert/plural/internal/keys"

case keys.Escape:  // "esc"
case keys.Enter:   // "enter"
case keys.CtrlC:   // "ctrl+c"
```

The `keys` package derives values from `tea.KeyPressMsg{Code: tea.KeyXxx}.String()` at init time. Single-character keys (`"a"`, `"y"`, `"?"`) are not included. Always use `keys.` constants for special keys — never hardcode strings like `"esc"`.

---

## Releasing

```bash
./scripts/release.sh patch            # v0.0.3 -> v0.0.4
./scripts/release.sh minor            # v0.0.3 -> v0.1.0
./scripts/release.sh major            # v0.0.3 -> v1.0.0
./scripts/release.sh patch --dry-run  # Dry run
```

## License

MIT License - see [LICENSE](LICENSE) for details.
