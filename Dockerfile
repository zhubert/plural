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

# Build gopls in the builder stage
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go install golang.org/x/tools/gopls@latest && \
    cp /go/bin/${TARGETOS}_${TARGETARCH}/gopls /out/gopls 2>/dev/null || cp /go/bin/gopls /out/gopls

# Stage 2: Runtime image
FROM alpine

# Install Node.js, npm, git, and su-exec (for user switching in entrypoint)
# Node.js is needed for Claude CLI, git for worktree operations
RUN apk add --no-cache \
    git \
    nodejs \
    npm \
    su-exec

# Install Claude CLI globally
RUN npm install -g @anthropic-ai/claude-code

# Copy Go toolchain from builder stage (exact version matching go.mod)
COPY --from=builder /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:$PATH"

# Copy plural binary and gopls from builder stage
COPY --from=builder /out/plural /usr/local/bin/plural
COPY --from=builder /out/gopls /usr/local/bin/gopls

# Create non-root user with Go environment
RUN adduser -D -s /bin/sh claude && \
    mkdir -p /home/claude/go && \
    chown claude:claude /home/claude/go

ENV GOPATH=/home/claude/go
ENV PATH="$GOPATH/bin:$PATH"

# Entrypoint runs as root to fix socket permissions, then switches to claude user.
# entrypoint.sh: root-level setup (socket chmod, user switch via su-exec)
# entrypoint-claude.sh: claude-level setup (config copy, auth, exec claude)
COPY entrypoint.sh /entrypoint.sh
COPY --chown=claude:claude entrypoint-claude.sh /home/claude/entrypoint-claude.sh

# Default working directory (overridden by -w flag)
WORKDIR /workspace

ENTRYPOINT ["/entrypoint.sh"]
