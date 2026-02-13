# Stage 1: Build the plural binary from source
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /out/plural .

# Stage 2: Runtime image
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

# Copy plural binary from builder stage
COPY --from=builder /out/plural /usr/local/bin/plural

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
