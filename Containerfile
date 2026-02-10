FROM golang:1.25-alpine

# Install Node.js, npm, and git
# Node.js is needed for Claude CLI, git for worktree operations
RUN apk add --no-cache \
    git \
    nodejs \
    npm

# Install Claude CLI globally
RUN npm install -g @anthropic-ai/claude-code

# Create non-root user (Claude CLI refuses --dangerously-skip-permissions as root)
RUN adduser -D -s /bin/sh claude
USER claude

# Add Go bin directory to PATH for gopls
ENV PATH="/home/claude/go/bin:${PATH}"

# Install gopls (Go language server) for code intelligence
RUN go install golang.org/x/tools/gopls@latest

# Entrypoint copies read-only host .claude config to a writable location
# Host mounts ~/.claude at /home/claude/.claude-host:ro for security
COPY --chown=claude:claude entrypoint.sh /home/claude/entrypoint.sh

# Default working directory (overridden by -w flag)
WORKDIR /workspace

ENTRYPOINT ["/home/claude/entrypoint.sh"]
