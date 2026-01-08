# Plural

A TUI for managing multiple concurrent Claude Code sessions, each running in its own isolated git worktree.

## Why Plural?

Claude Code works best with focused, single-purpose sessions. But real work often involves multiple parallel tasks: fixing a bug while prototyping a feature, reviewing code in one repo while implementing in another, or running several experimental approaches simultaneously.

Plural lets you:
- **Run multiple Claude sessions in parallel** without conflicts
- **Keep changes isolated** in separate git branches until you're ready to merge
- **Switch context instantly** between different tasks or repos
- **Work on the same codebase** from multiple angles simultaneously

## Features

- **Isolated Sessions**: Each session gets its own git branch and worktree, so Claude's changes never conflict
- **Multiple Repositories**: Register any git repo and create sessions across your projects
- **Streaming Responses**: See Claude's work in real-time with tool status indicators
- **Inline Permissions**: Approve file edits and commands per-session without blocking modals
- **Image Support**: Paste screenshots directly into conversations (Ctrl+V)
- **Session Search**: Quickly find sessions with `/` search
- **Themes**: Seven built-in color themes (press `t` to browse)
- **MCP Servers**: Configure external tools per-repo or globally
- **Merge Workflow**: Merge to main or create GitHub PRs when ready

## Requirements

- [Claude Code CLI](https://claude.ai/code) installed and authenticated
- Git
- GitHub CLI (`gh`) for PR creation (optional)

## Installation

### Homebrew (Recommended)

```bash
brew tap zhubert/tap
brew install plural
```

### Nix / Devbox

```bash
# Run directly without installing
nix run github:zhubert/plural

# Install to your profile
nix profile install github:zhubert/plural

# Or add to devbox
devbox add github:zhubert/plural
devbox global add github:zhubert/plural
```

### From Source

See [CONTRIBUTING.md](CONTRIBUTING.md) for build instructions.

## Quick Start

```bash
plural
```

1. Press `r` to add a git repository
2. Press `n` to create a new session
3. Press `Tab` or `Enter` to focus the chat
4. Type your message and press `Enter`
5. When Claude requests permission: `y` (allow), `n` (deny), or `a` (always allow)

## Keyboard Shortcuts

Shortcuts are context-aware and shown in the footer.

### Sidebar (Session List)

| Key | Action |
|-----|--------|
| `n` | Create new session |
| `r` | Add repository |
| `Enter` | Select session |
| `↑/↓` or `j/k` | Navigate sessions |
| `/` | Search sessions |
| `m` | Merge or create PR |
| `v` | View uncommitted changes |
| `d` | Delete session |
| `f` | Force resume (hung session) |
| `s` | Manage MCP servers |
| `t` | Change theme |
| `q` | Quit |

### Chat Panel

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Esc` | Stop Claude response |
| `Ctrl+V` | Paste image from clipboard |
| `Tab` | Switch to sidebar |

### Permission Prompts

| Key | Action |
|-----|--------|
| `y` | Allow this operation |
| `n` | Deny this operation |
| `a` | Always allow this tool |

## How Sessions Work

When you create a session, Plural:
1. Creates a new git branch (`plural-<uuid>` or your custom name)
2. Sets up a worktree in `.plural-worktrees/<uuid>` (sibling to your repo)
3. Starts a persistent Claude Code process in that worktree

This isolation means:
- Claude can edit files freely without affecting your main branch
- Multiple sessions can work on the same repo simultaneously
- You control when changes get merged

## Image Pasting

Share screenshots and diagrams with Claude:

1. Copy an image to your clipboard (e.g., `Cmd+Shift+4` on macOS)
2. Focus the chat input
3. Press `Ctrl+V` to attach
4. You'll see `[Image attached: XXkb]`
5. Add a message and press `Enter`

Supports PNG, JPEG, GIF, and WebP (max 3.75MB).

## Session Search

With many sessions, use `/` to search:

1. Press `/` in the sidebar
2. Type to filter by branch name, session name, or repo
3. Use `↑/↓` to navigate results
4. Press `Enter` to select, `Esc` to cancel

## Applying Changes

When you're ready to use your session's changes:

1. Select the session
2. Press `m` to open the merge modal
3. Choose:
   - **Merge to main**: Directly merges into your default branch
   - **Create PR**: Pushes and creates a GitHub PR (requires `gh`)

Uncommitted changes are auto-committed before merge/PR.

## MCP Servers

Extend Claude's capabilities with MCP servers:

1. Press `s` from the sidebar
2. Add servers globally or per-repository
3. Configure name, command, and arguments

Example: Add a GitHub MCP server globally, or a database server for a specific project.

## Themes

Press `t` to choose from:
- Dark Purple (default)
- Nord
- Dracula
- Gruvbox Dark
- Tokyo Night
- Catppuccin Mocha
- Light

## Configuration

Data stored in `~/.plural/`:
- `config.json` - Repos, sessions, tools, MCP servers, theme
- `sessions/<id>.json` - Conversation history

### Commands

```bash
plural --clear   # Remove all sessions
plural --prune   # Clean up orphaned worktrees
plural --check-prereqs  # Verify required tools
```

## Recovering Hung Sessions

If a session shows ⛔ (stuck from a crash):

1. Select the session
2. Press `f` to force resume
3. Plural kills orphaned processes and resets the session

## Troubleshooting

### Devbox/Nix upgrade fails

```bash
# Workaround for nix profile upgrade limitation
devbox global rm github:zhubert/plural
devbox global add github:zhubert/plural
```

Or use Homebrew which handles upgrades correctly.

## License

MIT License - see [LICENSE](LICENSE) for details.
