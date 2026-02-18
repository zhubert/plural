# Plural

**Explore multiple solutions at once.** Parallel Claude Code sessions, or a fully autonomous agent.

Plural is a TUI and headless agent for Claude Code. Run parallel sessions across branches and repos, fork when Claude offers competing approaches, import issues from GitHub/Asana/Linear, broadcast prompts across services, and create PRs—all from a keyboard-driven terminal interface. Or skip the TUI entirely and run `plural agent` as an autonomous daemon that picks up GitHub issues, writes code, opens PRs, addresses review comments, and merges.

![Plural demo](docs/demo.gif)

## Install

```bash
brew tap zhubert/tap
brew install plural
```

Or [build from source](CONTRIBUTING.md).

## Requirements

- [Claude Code CLI](https://claude.ai/code) installed and authenticated
- Git
- GitHub CLI (`gh`) — optional, for PRs and GitHub issue import
- Docker — optional, for container mode and headless agent

## Quick Start

```bash
plural
```

Add a repository, create a session, and start chatting. Press `?` for context-sensitive help at any time.

When Claude requests tool permissions: `y` (allow), `n` (deny), or `a` (always allow).

---

## TUI Features

### Parallel Sessions

Each session runs in its own git worktree with a dedicated branch. Claude can edit files freely without touching your main branch. Multiple sessions can work on the same repo simultaneously.

### Option Forking

When Claude proposes competing approaches, press `Ctrl+O` to detect options and fork into parallel sessions automatically. Each approach gets its own branch. Compare results and merge the winner.

Fork manually with `f` to branch off any session at any point.

### Issue Import

Press `i` to import issues or tasks. Select multiple and Plural creates a session for each—Claude starts working on all of them in parallel.

| Provider | Auth | Setup |
|----------|------|-------|
| **GitHub Issues** | `gh` CLI | Always available |
| **Asana Tasks** | `ASANA_PAT` env var | Map repo to project in settings (`,`) |
| **Linear Issues** | `LINEAR_API_KEY` env var | Map repo to team in settings (`,`) |

PRs created from issue sessions automatically include closing references (`Fixes #N` or `Fixes ENG-123`).

### Broadcasting

Send the same prompt to multiple repositories at once with `Ctrl+B`. Plural creates a session per repo and sends your prompt in parallel—useful for applying the same change across a fleet of services.

Follow up with `Ctrl+Shift+B` to send additional prompts or create PRs for all sessions in the group.

### PR Workflow

Press `m` to merge or create a PR. Options include merge to main, merge to parent session, create PR, or push updates to an existing PR. Uncommitted changes are auto-committed.

After a PR is created, the sidebar shows when new review comments arrive. Press `Ctrl+R` to import them so Claude can address the feedback directly.

Merge conflicts can be resolved by Claude, manually in a terminal, or aborted.

### Branch Preview

Press `p` to preview a session's branch in your main repo so dev servers pick up the changes without merging. The header shows `[PREVIEW]` while active. Press `p` again to restore.

### Container Mode

Run Claude inside a Docker container for sandboxed execution. The container IS the sandbox—Claude can use tools freely without permission prompts.

- Check "Run in container" when creating a session
- Press `Ctrl+E` to open a terminal inside the container
- Sessions show a `[CONTAINER]` indicator

Auth: `ANTHROPIC_API_KEY` env var, macOS keychain (`anthropic_api_key`), OAuth token from `claude setup-token`, or `~/.claude/.credentials.json` from `claude login`.

### Chat

- **Image pasting** (`Ctrl+V`) — share screenshots directly with Claude
- **Message search** (`Ctrl+/`) — search conversation history
- **Text selection** — click+drag, double-click for word, triple-click for paragraph
- **Tool use rollup** (`Ctrl+T`) — toggle collapsed/expanded view of tool operations
- **Log viewer** (`Ctrl+L`) — debug, MCP, and stream logs in an overlay
- **Cost tracking** (`/cost`) — token usage and estimated cost for the session

### Customization

- 8 built-in themes — press `t` to switch (tokyo-night, dracula, nord, gruvbox, catppuccin, and more)
- Branch naming prefixes
- Desktop notifications
- MCP servers and plugins (`/mcp`, `/plugins`)
- Per-repo settings for allowed tools, squash-on-merge, and issue provider mapping
- Global settings with `Alt+,`, session settings with `,`

---

## Headless Agent Mode

Run Plural as an autonomous daemon—no TUI required. The agent polls a repo for GitHub issues labeled `queued`, creates containerized Claude sessions, and works each issue end-to-end.

```bash
plural agent --repo owner/repo
```

### How It Works

1. Agent finds issues labeled `queued` on the target repo
2. Creates a containerized Claude session on a new branch
3. Swaps the label from `queued` to `wip` and posts a comment
4. Claude works the issue autonomously
5. A PR is created when coding is complete
6. Agent polls for review approval and CI, then merges
7. The `wip` label is removed

For complex issues, Claude can delegate subtasks to child sessions via MCP tools (`create_child_session`, `list_child_sessions`, `merge_child_to_parent`). The supervisor waits for all children before creating a PR.

### Agent Flags

```bash
plural agent --repo owner/repo              # Required: repo to poll
plural agent --repo owner/repo --once       # Single tick, then exit
plural agent --repo owner/repo --auto-merge # Auto-merge after review + CI (default)
plural agent --repo owner/repo --no-auto-merge
plural agent --repo owner/repo --max-concurrent 5
plural agent --repo owner/repo --max-turns 80
plural agent --repo owner/repo --max-duration 45
plural agent --repo owner/repo --merge-method squash
plural agent --repo owner/repo --auto-address-pr-comments
```

### Agent Configuration

These can also be set via TUI settings or `~/.plural/config.json`:

| Setting | JSON Key | Default |
|---------|----------|---------|
| Max concurrent sessions | `issue_max_concurrent` | `3` |
| Max turns per session | `auto_max_turns` | `50` |
| Max duration (minutes) | `auto_max_duration_min` | `30` |
| Merge method | `auto_merge_method` | `rebase` |
| Auto-cleanup after merge | `auto_cleanup_merged` | `false` |
| Auto-address PR comments | `auto_address_pr_comments` | `false` |

Graceful shutdown: `SIGINT`/`SIGTERM` once to finish current work, twice to force exit.

---

## Keyboard Shortcuts

Press `?` for the full list. Key shortcuts:

| Key | Action |
|-----|--------|
| `Tab` | Switch focus between sidebar and chat |
| `n` | New session |
| `f` | Fork session |
| `i` | Import issues |
| `d` | Delete session |
| `r` | Rename session |
| `m` | Merge / Create PR |
| `p` | Preview branch |
| `v` | View git diff |
| `s` | Multi-select sessions |
| `/` | Filter sessions in sidebar |
| `a` | Add repository |
| `,` | Session settings |
| `Alt+,` | Global settings |
| `t` | Switch theme |
| `Ctrl+O` | Fork detected options |
| `Ctrl+B` | Broadcast prompt |
| `Ctrl+Shift+B` | Broadcast group actions |
| `Ctrl+R` | Import PR review comments |
| `Ctrl+E` | Open terminal in worktree |
| `Ctrl+/` | Search messages |
| `Ctrl+T` | Toggle tool use expansion |
| `Ctrl+L` | Log viewer |
| `Ctrl+V` | Paste image |
| `W` | What's New |
| `?` | Help |
| `q` | Quit |

---

## CLI Reference

```bash
plural                    # Start the TUI
plural --debug            # Debug logging (default: on)
plural -q / --quiet       # Info-level logging only
plural --version          # Show version
plural help               # Show help
plural clean              # Remove sessions, logs, worktrees, and containers
plural clean -y           # Clean without confirmation
plural agent --repo ...   # Headless agent mode (see above)
```

## Data Storage

All data lives in `~/.plural/` by default. If [XDG environment variables](https://specifications.freedesktop.org/basedir-spec/latest/) are set and `~/.plural/` doesn't exist, Plural follows the XDG Base Directory Specification.

| Purpose | Default | XDG |
|---------|---------|-----|
| Config | `~/.plural/config.json` | `$XDG_CONFIG_HOME/plural/` |
| Sessions | `~/.plural/sessions/` | `$XDG_DATA_HOME/plural/` |
| Logs | `~/.plural/logs/` | `$XDG_STATE_HOME/plural/` |

## Container Image

Pre-built:
```bash
docker pull ghcr.io/zhubert/plural-claude
```

Build your own:
```bash
./scripts/build-container.sh ghcr.io/zhubert/plural-claude         # latest
./scripts/build-container.sh ghcr.io/zhubert/plural-claude v0.1.0  # pinned
```

The container auto-updates both the plural binary and Claude CLI on startup. Disable with `PLURAL_SKIP_UPDATE=1`.

---

## Changelog

See [GitHub Releases](https://github.com/zhubert/plural/releases).

## License

MIT — see [LICENSE](LICENSE).
