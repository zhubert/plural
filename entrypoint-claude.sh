#!/bin/sh
set -e

# Copy only needed config from host into writable location so Claude CLI
# can write debug logs, todos, and session data. The host dir is mounted
# read-only at .claude-host to prevent container writes from reaching the host.
# Selective copy avoids slow startup for users with large ~/.claude directories.
HOST_DIR=/home/claude/.claude-host
DEST_DIR=/home/claude/.claude
mkdir -p "$DEST_DIR"

# Copy essential config files
for f in settings.json CLAUDE.md .credentials.json; do
    [ -f "$HOST_DIR/$f" ] && cp "$HOST_DIR/$f" "$DEST_DIR/$f" 2>/dev/null
done

# Copy projects dir (session JSONL files needed for fork/resume)
[ -d "$HOST_DIR/projects" ] && cp -r "$HOST_DIR/projects" "$DEST_DIR/projects" 2>/dev/null

# Copy plugins config
[ -d "$HOST_DIR/plugins" ] && cp -r "$HOST_DIR/plugins" "$DEST_DIR/plugins" 2>/dev/null

# Setup gopls plugin (after host plugins copy to avoid overwriting)
# Only create if gopls binary exists and plugin config doesn't already exist
if command -v gopls >/dev/null 2>&1 && [ ! -f "$DEST_DIR/plugins/gopls/plugin.json" ]; then
    mkdir -p "$DEST_DIR/plugins/gopls"
    cat > "$DEST_DIR/plugins/gopls/plugin.json" <<'EOF'
{"command": "gopls", "args": ["serve"], "extensionToLanguage": {".go": "go"}}
EOF
fi

# Log the exact command for debugging container startup issues.
# This appears in the host stream log as a non-JSON line.
echo "[entrypoint] exec claude $*" >&2

exec claude "$@"
