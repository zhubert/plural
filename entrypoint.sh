#!/bin/sh
set -e

# Switch to claude user for the rest of the entrypoint.
# Everything below runs as claude via su-exec (like gosu but Alpine-native).
exec su-exec claude /home/claude/entrypoint-claude.sh "$@"
