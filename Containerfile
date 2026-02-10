FROM golang:1.25-bookworm

# Install Node.js, npm, and git
# Node.js is needed for Claude CLI, git for worktree operations
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    ca-certificates \
    && mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_22.x nodistro main" | tee /etc/apt/sources.list.d/nodesource.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

# Install Claude CLI globally
RUN npm install -g @anthropic-ai/claude-code

# Create non-root user (Claude CLI refuses --dangerously-skip-permissions as root)
RUN useradd -m -s /bin/bash claude
USER claude

# Install gopls (Go language server) for code intelligence
RUN go install golang.org/x/tools/gopls@latest

# Configure gopls LSP for Claude Code
# Create plugins directory and gopls plugin config
RUN mkdir -p /home/claude/.claude/plugins/gopls && \
    echo '{"command": "gopls", "args": ["serve"], "extensionToLanguage": {".go": "go"}}' > /home/claude/.claude/plugins/gopls/plugin.json

# Entrypoint copies read-only host .claude config to a writable location
# Host mounts ~/.claude at /home/claude/.claude-host:ro for security
COPY --chown=claude:claude entrypoint.sh /home/claude/entrypoint.sh

# Default working directory (overridden by -w flag)
WORKDIR /workspace

ENTRYPOINT ["/home/claude/entrypoint.sh"]
