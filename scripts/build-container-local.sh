#!/bin/sh
# Build the plural-claude Docker image using a locally-compiled plural binary.
#
# This is useful during development when the code hasn't been released yet,
# so the normal build (which downloads from GitHub releases) can't pick up
# the latest changes.
#
# Usage:
#   ./scripts/build-container-local.sh                                # Build with default image name
#   ./scripts/build-container-local.sh my-image                       # Build with custom image name
#
# What it does:
#   1. Cross-compiles the plural binary for Linux (matching host arch)
#   2. Builds a Docker image using the local binary instead of downloading from releases
#   3. Cleans up the temporary binary

set -e

IMAGE_NAME="${1:-ghcr.io/zhubert/plural-claude}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  GOARCH="amd64" ;;
    arm64|aarch64) GOARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

BINARY_NAME="plural-linux-${GOARCH}"

echo "Building plural binary for linux/${GOARCH}..."
CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" go build -o "$BINARY_NAME" .

echo "Building Docker image: $IMAGE_NAME"
docker build \
    -f - \
    -t "$IMAGE_NAME" \
    . <<DOCKERFILE
# Stage 1: Build gopls (only tool we need to build)
FROM --platform=\$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

# Build gopls in the builder stage
RUN mkdir -p /out && \\
    CGO_ENABLED=0 GOOS=\${TARGETOS} GOARCH=\${TARGETARCH} go install golang.org/x/tools/gopls@latest && \\
    cp /go/bin/\${TARGETOS}_\${TARGETARCH}/gopls /out/gopls 2>/dev/null || cp /go/bin/gopls /out/gopls

# Stage 2: Runtime image
FROM alpine

RUN apk add --no-cache \\
    bash \\
    git \\
    nodejs \\
    npm \\
    su-exec \\
    curl \\
    jq

RUN npm install -g @anthropic-ai/claude-code

COPY --from=builder /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:\$PATH"

# Copy locally-built plural binary instead of downloading from releases
COPY ${BINARY_NAME} /usr/local/bin/plural
RUN chmod +x /usr/local/bin/plural

COPY --from=builder /out/gopls /usr/local/bin/gopls

ENV SHELL=/bin/bash

RUN adduser -D -s /bin/bash claude && \\
    mkdir -p /home/claude/go && \\
    chown claude:claude /home/claude/go

ENV GOPATH=/home/claude/go
ENV PATH="\$GOPATH/bin:\$PATH"

COPY entrypoint.sh /entrypoint.sh
COPY --chown=claude:claude entrypoint-claude.sh /home/claude/entrypoint-claude.sh

WORKDIR /workspace

ENTRYPOINT ["/entrypoint.sh"]
DOCKERFILE

# Clean up the temporary binary
rm -f "$BINARY_NAME"

echo "Done. Image: $IMAGE_NAME (local build)"
