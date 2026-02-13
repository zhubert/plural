#!/bin/sh
# Build the plural-claude Docker image.
#
# This script builds the Docker image using a multi-stage Dockerfile.
# The first stage compiles the plural binary from source, and the second
# stage creates the runtime image with Claude CLI and the plural binary.
#
# Usage:
#   ./scripts/build-container.sh                              # Build with default image name
#   ./scripts/build-container.sh my-image                     # Build with custom image name
#   ./scripts/build-container.sh ghcr.io/zhubert/plural-claude:v1.0.0  # Build with tag

set -e

IMAGE_NAME="${1:-ghcr.io/zhubert/plural-claude}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "Building Docker image: $IMAGE_NAME"
docker build -t "$IMAGE_NAME" "$REPO_ROOT"

echo "Done. Image: $IMAGE_NAME"
