# Plural

**Do more, faster.** Parallel Claude Code sessions.

Run multiple Claude sessions on the same codebase—each in its own git branch. When Claude offers different approaches, fork the session and try them all in parallel. Switch freely. Merge the winner.

![Plural demo](demo.gif)

## Why Plural?

Plural isn't a terminal multiplexer with Claude instances. It's a purpose-built TUI that integrates deeply with the Claude Code API—streaming responses, parsing tool calls, handling permissions, and understanding Claude's structured output.

This integration enables features that wouldn't be possible with a tmux wrapper:
- **Live todo sidebar** that updates as Claude works through tasks
- **Option detection** that parses Claude's proposed approaches and lets you fork into parallel sessions
- **Automatic permission handling** with inline prompts and "always allow" memory
- **Git-aware merge workflow** that understands session branches and creates PRs
- **Token cost tracking** parsed from Claude's session data

## Requirements

- [Claude Code CLI](https://claude.ai/code) installed and authenticated
- Git
- GitHub CLI (`gh`) for PR creation (optional)

Run `plural --check-prereqs` to verify all required tools are installed and configured correctly.

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

Add a repository, create a session, and start chatting with Claude. **Press `?` at any time to see all available keyboard shortcuts for your current context**—the help adapts based on what you're doing.

When Claude requests permission for tool use: `y` (allow), `n` (deny), or `a` (always allow).

---

## What You Can Do

### Isolated Sessions

Each session runs in its own git worktree with a dedicated branch. Claude can edit files freely without touching your main branch—multiple sessions can work on the same repo simultaneously. You decide when and what to merge.

### Parallel Exploration

When Claude offers multiple approaches ("Option 1: Use Redis" vs "Option 2: Use PostgreSQL"), fork the session and explore them all at once. Child sessions appear indented in the sidebar. Try different solutions in parallel and merge the winner.

### GitHub Integration

Import GitHub issues directly—Plural creates a session for each with full context, and Claude starts working immediately. When you create a PR from an issue session, "Fixes #N" is automatically added to close the issue on merge.

### Merge & PR Workflow

When a session's work is ready, merge directly to your main branch or create a GitHub PR. Uncommitted changes are auto-committed. If there are merge conflicts, Claude can help resolve them.

### Preview Changes

Preview a session's branch in your main repository so dev servers pick up the changes without merging. The header shows a `[PREVIEW]` indicator while active.

### Rich Chat Features

- **Image pasting**: Share screenshots and diagrams directly with Claude
- **Message search**: Find anything in your conversation history
- **Text selection**: Select and copy text from the chat
- **Tool use rollup**: Collapsed view of Claude's tool operations, expandable on demand

### Customization

Choose from 8 built-in themes, configure branch naming prefixes, set up desktop notifications, and extend Claude's capabilities with MCP servers and plugins.

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
plural --check-prereqs  # Verify required tools
plural --clear          # Remove all sessions
plural --prune          # Clean up orphaned worktrees and processes
plural --debug          # Enable debug logging
```

### Data Storage

Configuration and session history are stored in `~/.plural/`.

### Troubleshooting

**Devbox/Nix upgrade fails**: Use `devbox global rm` then `devbox global add` as a workaround, or use Homebrew which handles upgrades correctly.

---

## Changelog

See the [GitHub Releases](https://github.com/zhubert/plural/releases) page for version history and release notes.

## License

MIT License - see [LICENSE](LICENSE) for details.
