# Stage 1: Build gopls (only tool we need to build)
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

# Build gopls in the builder stage
RUN mkdir -p /out && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go install golang.org/x/tools/gopls@latest && \
    cp /go/bin/${TARGETOS}_${TARGETARCH}/gopls /out/gopls 2>/dev/null || cp /go/bin/gopls /out/gopls

# Stage 2: Runtime image
FROM alpine

# Install runtime dependencies: Node.js, npm, git, su-exec, curl (for downloading plural binary)
# Node.js is needed for Claude CLI, git for worktree operations, curl for downloading plural
RUN apk add --no-cache \
    bash \
    git \
    nodejs \
    npm \
    su-exec \
    curl \
    jq

# Install Claude CLI globally
RUN npm install -g @anthropic-ai/claude-code

# Copy Go toolchain from builder stage (exact version matching go.mod)
COPY --from=builder /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:$PATH"

# Download the latest plural binary from GitHub releases
# This allows the Docker image to be stable while pulling the latest plural version on build
# To build with a specific version: docker build --build-arg PLURAL_VERSION=v0.1.0
ARG PLURAL_VERSION=latest
ARG TARGETARCH
RUN set -ex; \
    # Map Docker arch names to GoReleaser arch names (amd64->x86_64, arm64->arm64)
    GOARCH="${TARGETARCH}"; \
    if [ "$TARGETARCH" = "amd64" ]; then GOARCH="x86_64"; fi; \
    if [ "$PLURAL_VERSION" = "latest" ]; then \
        DOWNLOAD_URL=$(curl -sL https://api.github.com/repos/zhubert/plural/releases/latest | \
            jq -r ".assets[] | select(.name | contains(\"Linux_${GOARCH}\")) | .browser_download_url"); \
    else \
        DOWNLOAD_URL="https://github.com/zhubert/plural/releases/download/${PLURAL_VERSION}/plural_Linux_${GOARCH}.tar.gz"; \
    fi; \
    echo "Downloading plural from: $DOWNLOAD_URL"; \
    curl -sL "$DOWNLOAD_URL" | tar -xz -C /tmp plural; \
    mv /tmp/plural /usr/local/bin/plural; \
    chmod +x /usr/local/bin/plural

# Copy gopls from builder stage
COPY --from=builder /out/gopls /usr/local/bin/gopls

# Claude CLI's Bash tool requires SHELL to be set and point to a valid shell
ENV SHELL=/bin/bash

# Create non-root user with Go environment
RUN adduser -D -s /bin/bash claude && \
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
