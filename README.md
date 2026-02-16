# Plural

**Explore multiple solutions at once.** Parallel Claude Code sessions.

Run multiple Claude sessions on the same codebase—each in its own git branch. When Claude offers different approaches, fork the session and try them all in parallel. Switch freely. Merge the winner.

![Plural demo](docs/demo.gif)

## Why Plural?

Plural isn't a terminal multiplexer with Claude instances. It's a purpose-built TUI that integrates deeply with the Claude Code API—streaming responses, parsing tool calls, handling permissions, and understanding Claude's structured output.

This integration enables features that wouldn't be possible with a tmux wrapper:
- **Live todo sidebar** that updates as Claude works through tasks
- **Option detection** that parses Claude's proposed approaches and lets you fork into parallel sessions
- **Automatic permission handling** with inline prompts and "always allow" memory
- **Git-aware merge workflow** that understands session branches and creates PRs
- **Token cost tracking** parsed from Claude's session data
- **PR review comments** imported directly into sessions for iterating on feedback

## Requirements

- [Claude Code CLI](https://claude.ai/code) installed and authenticated
- Git
- GitHub CLI (`gh`) for PR creation and issue import (optional)

Run `plural help` to see all available commands and options.

## Installation

### Homebrew (Recommended)

```bash
brew tap zhubert/tap
brew install plural
```

### From Source

See [CONTRIBUTING.md](CONTRIBUTING.md) for build instructions.

## Quick Start

```bash
plural
```

Add a repository, create a session, and start chatting with Claude. **Press `?` at any time to see all available keyboard shortcuts for your current context**—the help adapts based on what you're doing.

When Claude requests permission for tool use: `y` (allow), `n` (deny), or `a` (always allow).

---

## What You Can Do

### Isolated Sessions

Each session runs in its own git worktree with a dedicated branch. Claude can edit files freely without touching your main branch—multiple sessions can work on the same repo simultaneously. You decide when and what to merge.

### Parallel Exploration

When Claude offers multiple approaches ("Option 1: Use Redis" vs "Option 2: Use PostgreSQL"), press `Ctrl+O` to detect options and fork into parallel sessions automatically. Each approach gets its own branch. Compare results and merge the winner.

You can also fork manually with `f` to branch off any session at any point.

### Import Issues in Parallel

Press `i` to import issues or tasks. Select multiple and Plural creates a session for each—Claude starts working on all of them simultaneously.

**GitHub Issues** — Always available (uses the `gh` CLI). When you create a PR from an issue session, "Fixes #N" is automatically added to close the issue on merge.

**Asana Tasks** — Available when configured. To set up Asana integration:

1. Create a [Personal Access Token](https://app.asana.com/0/developer-console) in Asana
2. Set the `ASANA_PAT` environment variable:
   ```bash
   export ASANA_PAT="your-token-here"
   ```
3. Map a repository to an Asana project by pressing `,` to open Settings, then select an Asana project from the list for the current repo

When both GitHub and Asana are configured for a repository, Plural will prompt you to choose a source before importing.

### Broadcast Across Repos

Send the same prompt to multiple repositories at once with `Ctrl+B`. Plural creates a session for each repo and sends your prompt in parallel—perfect for applying the same change across a fleet of services.

Later, use the broadcast group modal (`Ctrl+Shift+B`) to send follow-up prompts or create PRs for all sessions at once.

### Open PRs in Parallel

When sessions are part of a broadcast group, you can create PRs for all of them in one action via `Ctrl+Shift+B`. For individual sessions, press `m` and choose "Create PR". Uncommitted changes are auto-committed.

### PR Review Comments

After a PR is created, the sidebar shows an indicator when new review comments arrive. Press `Ctrl+R` to import comments into the session so Claude can address the feedback directly.

### Merge & PR Workflow

When a session's work is ready, merge directly to your main branch or create a GitHub PR. Uncommitted changes are auto-committed. If there are merge conflicts, Claude can help resolve them. Optionally enable squash-on-merge per repository in settings.

### Preview Changes

Preview a session's branch in your main repository (`p`) so dev servers pick up the changes without merging. The header shows a `[PREVIEW]` indicator while active. Press `p` again to restore your original branch.

### Container Mode (Sandboxed Execution)

Run Claude CLI inside a Docker container for defense-in-depth security. The container serves as the sandbox—Claude can freely use tools without permission prompts.

**Requirements:**
- Docker installed
- Authentication via one of:
  - `ANTHROPIC_API_KEY` environment variable
  - API key in macOS keychain (`anthropic_api_key`)
  - Long-lived OAuth token from `claude setup-token`

**How to use:**
- Check the "Run in container" box when creating a new session
- Forked sessions inherit their parent's container setting
- Sessions show a `[CONTAINER]` indicator in the header
- Press `ctrl-e` to open a terminal inside the container for debugging

**Tradeoffs:**
- No permission prompts for tool use
- Filesystem isolation from your host
- External MCP servers not supported
- Containers are defense-in-depth, not a complete security boundary

**Pre-built images:**
```bash
docker pull ghcr.io/zhubert/plural-claude
```

**Building a custom image:**
```bash
# Build with latest plural binary from GitHub releases
./scripts/build-container.sh ghcr.io/zhubert/plural-claude

# Build with a specific plural version
./scripts/build-container.sh ghcr.io/zhubert/plural-claude v0.1.0
```

The Docker image downloads the plural binary from GitHub releases rather than building from source, making it stable and version-independent. This means you don't need to rebuild the image every time plural is updated.

**Automatic updates:**
The container checks for newer plural versions on startup and automatically updates if available. This happens:
- On every container start
- Silently in the background (logs show update status)
- With graceful fallback if GitHub is unreachable

To disable auto-updates:
```bash
export PLURAL_SKIP_UPDATE=1
```

### Rich Chat Features

- **Image pasting**: Share screenshots and diagrams directly with Claude
- **Message search** (`Ctrl+/`): Find anything in your conversation history
- **Text selection**: Select and copy text from the chat (click+drag, double-click for word, triple-click for paragraph)
- **Tool use rollup**: Collapsed view of Claude's tool operations, expandable with `ctrl-t`
- **Log viewer** (`ctrl-l`): View debug, MCP, and stream logs in an overlay

### Customization

Choose from 8 built-in themes (`t`), configure branch naming prefixes, set up desktop notifications, and extend Claude's capabilities with MCP servers and plugins. Open settings with `,`.

### Slash Commands

- `/cost` - Token usage and estimated cost for the current session
- `/help` - Available Plural commands
- `/mcp` - MCP servers configuration
- `/plugins` - Manage marketplaces and plugins

---

## Reference

### CLI Options

```bash
plural                  # Start the application
plural --debug          # Enable debug logging
plural --version        # Show version
plural help             # Show help
plural clean            # Remove all sessions, logs, orphaned worktrees, and containers
plural clean -y         # Clear without confirmation prompt
```

### Data Storage

By default, configuration and session data are stored in `~/.plural/`.

Plural also supports the [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/latest/). If XDG environment variables are set and `~/.plural/` doesn't exist, files are organized as:

| Purpose | XDG Directory | Contents |
|---------|--------------|----------|
| Config | `$XDG_CONFIG_HOME/plural/` | `config.json` |
| Data | `$XDG_DATA_HOME/plural/` | `sessions/*.json` |
| State | `$XDG_STATE_HOME/plural/` | `logs/` |

Existing `~/.plural/` installations continue to work as-is.

---

## Changelog

See the [GitHub Releases](https://github.com/zhubert/plural/releases) page for version history and release notes.

## License

MIT License - see [LICENSE](LICENSE) for details.
