# Plural

**Explore multiple solutions at once.** Parallel Claude Code sessions in a keyboard-driven TUI.

Plural is a terminal interface for Claude Code. Run parallel sessions across branches and repos, fork when Claude offers competing approaches, import issues from GitHub/Asana/Linear, broadcast prompts across services, and create PRs—all from a keyboard-driven terminal interface.

> **Looking for the headless agent?** See [plural-agent](https://github.com/zhubert/plural-agent) — an autonomous daemon that picks up issues, writes code, opens PRs, addresses review comments, and merges.

![Plural demo](docs/demo.gif)

## Install

```bash
brew tap zhubert/tap
brew install plural
```

## Requirements

- [Claude Code CLI](https://claude.ai/code) installed and authenticated
- Git
- GitHub CLI (`gh`) — optional, for PRs and GitHub issue import
- Docker — optional, for container mode

## Quick Start

```bash
plural
```

Add a repository, create a session, and start chatting. Press `?` for context-sensitive help at any time.

When Claude requests tool permissions: `y` (allow), `n` (deny), or `a` (always allow).

---

## One Session

Every session runs in its own git worktree with a dedicated branch. Claude edits files freely without touching your main branch. Press `n` to create one, start chatting, and press `m` when you're ready to merge or open a PR.

## Try Multiple Approaches

_Can't decide between JWT and session-based auth? Try both._

When Claude proposes competing approaches, press `Ctrl+O` to auto-detect the options and fork into parallel sessions — each gets its own branch. Or press `f` at any point to manually fork a session and take it in a different direction.

Compare results with `v` (git diff), preview a branch in your main repo with `p` (so dev servers pick up the changes), and merge the winner with `m`.

## Work Your Backlog

_You have a pile of issues. Let Claude chew through them in parallel._

Press `i` to import issues from your tracker. Select several at once and Plural creates a session per issue — Claude starts working all of them simultaneously. PRs are automatically linked (`Fixes #N` / `Fixes ENG-123`).

| Provider          | Auth                     | Setup                                 |
| ----------------- | ------------------------ | ------------------------------------- |
| **GitHub Issues** | `gh` CLI                 | Always available                      |
| **Asana Tasks**   | `ASANA_PAT` env var      | Map repo to project in settings (`,`) |
| **Linear Issues** | `LINEAR_API_KEY` env var | Map repo to team in settings (`,`)    |

After a PR is created, the sidebar shows when new review comments arrive. Press `Ctrl+R` to import them so Claude can address the feedback directly.

## Change Everything at Once

_Bump a dependency, update a config pattern, or apply a migration across a fleet of repos._

Press `Ctrl+B` to broadcast a prompt to every registered repository. Plural creates a session per repo and sends your message in parallel. Follow up with `Ctrl+Shift+B` to send additional prompts or create PRs for the entire group.

## Sandboxed Execution

Run Claude inside a Docker container — the container IS the sandbox, so Claude can use tools freely without permission prompts. Check "Run in container" when creating a session, and press `Ctrl+E` to open a terminal inside it.

Auth: `ANTHROPIC_API_KEY` env var, macOS keychain (`anthropic_api_key`), OAuth token from `claude setup-token`, or `~/.claude/.credentials.json` from `claude login`.

## More

- **Image pasting** (`Ctrl+V`) — share screenshots directly with Claude
- **Message search** (`Ctrl+/`) — search conversation history
- **Text selection** — click+drag, double-click for word, triple-click for paragraph
- **Tool use rollup** (`Ctrl+T`) — toggle collapsed/expanded tool operations
- **Log viewer** (`Ctrl+L`) — debug, MCP, and stream logs
- **Cost tracking** (`/cost`) — token usage and estimated cost
- **8 themes** — press `t` to switch (tokyo-night, dracula, nord, gruvbox, catppuccin, and more)
- **MCP servers and plugins** (`/mcp`, `/plugins`)
- **Per-repo settings** for allowed tools, squash-on-merge, and issue provider mapping
- **Settings** — global with `Alt+,`, per-session with `,`

Press `?` at any time for the full keyboard shortcut list.

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
```

## Data Storage

All data lives in `~/.plural/` by default. If [XDG environment variables](https://specifications.freedesktop.org/basedir-spec/latest/) are set and `~/.plural/` doesn't exist, Plural follows the XDG Base Directory Specification.

| Purpose  | Default                 | XDG                        |
| -------- | ----------------------- | -------------------------- |
| Config   | `~/.plural/config.json` | `$XDG_CONFIG_HOME/plural/` |
| Sessions | `~/.plural/sessions/`   | `$XDG_DATA_HOME/plural/`   |
| Logs     | `~/.plural/logs/`       | `$XDG_STATE_HOME/plural/`  |

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
