#!/bin/sh
# Build the plural-claude Docker image.
#
# This script builds the Docker image that downloads the plural binary from GitHub releases.
# The image can be built with the latest version or a specific version of plural.
#
# Usage:
#   ./scripts/build-container.sh                                    # Build with default image name and latest plural
#   ./scripts/build-container.sh my-image                           # Build with custom image name and latest plural
#   ./scripts/build-container.sh ghcr.io/zhubert/plural-claude      # Build with default image name and latest plural
#   ./scripts/build-container.sh ghcr.io/zhubert/plural-claude v0.1.0  # Build with specific plural version
#
# Environment variables:
#   PLURAL_VERSION: Version of plural to download (default: latest)

set -e

IMAGE_NAME="${1:-ghcr.io/zhubert/plural-claude}"
PLURAL_VERSION="${2:-${PLURAL_VERSION:-latest}}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "Building Docker image: $IMAGE_NAME"
echo "Plural version: $PLURAL_VERSION"

docker build \
    --build-arg PLURAL_VERSION="$PLURAL_VERSION" \
    -t "$IMAGE_NAME" \
    "$REPO_ROOT"

echo "Done. Image: $IMAGE_NAME (plural $PLURAL_VERSION)"
