#!/bin/bash
set -e

# Copy only needed config from host into writable location so Claude CLI
# can write debug logs, todos, and session data. The host dir is mounted
# read-only at .claude-host to prevent container writes from reaching the host.
# Selective copy avoids slow startup for users with large ~/.claude directories.
HOST_DIR=/home/claude/.claude-host
DEST_DIR=/home/claude/.claude
mkdir -p "$DEST_DIR"

# Copy essential config files
for f in settings.json CLAUDE.md; do
    [ -f "$HOST_DIR/$f" ] && cp "$HOST_DIR/$f" "$DEST_DIR/$f" 2>/dev/null
done

# Copy projects dir (session JSONL files needed for fork/resume)
[ -d "$HOST_DIR/projects" ] && cp -r "$HOST_DIR/projects" "$DEST_DIR/projects" 2>/dev/null

# Copy plugins config
[ -d "$HOST_DIR/plugins" ] && cp -r "$HOST_DIR/plugins" "$DEST_DIR/plugins" 2>/dev/null

# Read auth credentials from mounted secrets file (not passed via -e to
# avoid exposing the key in `ps` output on the host).
# File format: ENV_VAR_NAME=value (e.g. CLAUDE_CODE_OAUTH_TOKEN=sk-ant-...)
# Uses set -a to auto-export all variables, then sources the file.
# This is safer than export "$(cat ...)" which breaks on newlines.
if [ -f /home/claude/.auth ]; then
    set -a
    . /home/claude/.auth
    set +a
fi

exec claude "$@"
