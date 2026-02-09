#!/bin/bash
# Copy host claude config to writable location so Claude CLI can write
# debug logs, todos, and session data. The host dir is mounted read-only
# at .claude-host to prevent container writes from reaching the host.
cp -r /home/claude/.claude-host /home/claude/.claude 2>/dev/null || true

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
