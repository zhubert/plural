FROM node:22-slim

# Install git (needed for worktree operations inside container)
RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*

# Install Claude CLI globally
RUN npm install -g @anthropic-ai/claude-code

# Create non-root user (Claude CLI refuses --dangerously-skip-permissions as root)
RUN useradd -m -s /bin/bash claude
USER claude

# Entrypoint copies read-only host .claude config to a writable location
# Host mounts ~/.claude at /home/claude/.claude-host:ro for security
COPY --chown=claude:claude entrypoint.sh /home/claude/entrypoint.sh

# Default working directory (overridden by -w flag)
WORKDIR /workspace

ENTRYPOINT ["/home/claude/entrypoint.sh"]
