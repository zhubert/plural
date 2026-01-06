# Plural

A TUI for managing multiple concurrent Claude Code sessions, each running in its own isolated git worktree.

## Features

- Run multiple Claude Code conversations simultaneously
- Each session gets its own git branch and worktree for isolated changes
- Sessions grouped by repository in the sidebar
- Conversation history persists across restarts (last 100 lines)
- Streaming responses from Claude Code CLI
- Interactive permission prompts (Allow, Deny, Always Allow)
- Merge session branches to main or create GitHub PRs

## Requirements

- Go 1.24+
- [Claude Code CLI](https://claude.ai/code) installed and authenticated
- Git
- GitHub CLI (`gh`) for PR creation (optional)

## Installation

```bash
git clone https://github.com/zhubert/plural.git
cd plural
go build -o plural .
```

## Usage

```bash
./plural
```

### Keyboard Shortcuts

Shortcuts are context-aware and shown in the footer when available.

| Key | Context | Action |
|-----|---------|--------|
| `Tab` | Any (with session) | Switch focus between sidebar and chat |
| `n` | Sidebar | Create new session |
| `r` | Sidebar | Add repository |
| `m` | Sidebar (session selected) | Merge to main or create PR |
| `d` | Sidebar (session selected) | Delete session |
| `Enter` | Sidebar | Select/open session |
| `Enter` | Chat | Send message |
| `↑/↓` or `j/k` | Sidebar | Navigate sessions |
| `Esc` | Modal | Close modal |
| `q` | Sidebar | Quit |
| `Ctrl+C` | Any | Force quit |

### Workflow

1. Press `r` to add a git repository (current directory suggested if it's a git repo)
2. Press `n` to create a new session (select from registered repos)
3. Press `Enter` or `Tab` to focus the chat panel
4. Type your message and press `Enter` to send
5. When Claude requests permission, choose: `y` (Allow), `n` (Deny), or `a` (Always Allow)
6. Create additional sessions with `n` to work on multiple tasks in parallel
7. Press `m` to merge your changes to main or create a GitHub PR

### Session Isolation

Each session creates:
- A new git branch: `plural-<session-uuid>`
- A worktree in `.plural-worktrees/<session-uuid>` (sibling to your repo)

This allows Claude to make changes in each session without conflicts.

### Applying Changes

When you're ready to apply changes from a session:

1. Select the session in the sidebar
2. Press `m` to open the merge modal
3. Choose:
   - **Merge to main**: Directly merges the session branch into your default branch
   - **Create PR**: Pushes the branch and creates a GitHub PR (requires `gh` CLI)

## Configuration

Data is stored in `~/.plural/`:
- `config.json` - Registered repositories, sessions, and permission settings
- `sessions/<id>.json` - Conversation history for each session

Clear all sessions:
```bash
./plural --clear
```

## Debug

Logs are written to `/tmp/plural-debug.log`:
```bash
tail -f /tmp/plural-debug.log
```
