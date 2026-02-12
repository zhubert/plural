FROM golang:1.25-alpine

# Install Node.js, npm, git, and su-exec (for user switching in entrypoint)
# Node.js is needed for Claude CLI, git for worktree operations
RUN apk add --no-cache \
    git \
    nodejs \
    npm \
    su-exec

# Install Claude CLI globally
RUN npm install -g @anthropic-ai/claude-code

# Copy pre-built plural binary for in-container MCP server.
# Built on host by scripts/build-container.sh with:
#   CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o <tmpdir>/plural .
COPY plural /usr/local/bin/plural

# Install gopls (Go language server) for code intelligence.
RUN go install golang.org/x/tools/gopls@latest

# Create non-root user
RUN adduser -D -s /bin/sh claude

# Copy gopls binary to claude user's Go bin directory
RUN mkdir -p /home/claude/go/bin && \
    cp /go/bin/gopls /home/claude/go/bin/gopls && \
    chown -R claude:claude /home/claude/go

# Add Go bin directory to PATH for gopls
ENV PATH="/home/claude/go/bin:${PATH}"

# Entrypoint runs as root to fix socket permissions, then switches to claude user.
# entrypoint.sh: root-level setup (socket chmod, user switch via su-exec)
# entrypoint-claude.sh: claude-level setup (config copy, auth, exec claude)
COPY entrypoint.sh /entrypoint.sh
COPY --chown=claude:claude entrypoint-claude.sh /home/claude/entrypoint-claude.sh

# Default working directory (overridden by -w flag)
WORKDIR /workspace

ENTRYPOINT ["/entrypoint.sh"]
