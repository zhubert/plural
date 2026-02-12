#!/bin/sh
# Build the plural-claude container image.
#
# This script cross-compiles the plural binary for linux/arm64,
# then builds the container from a minimal context (no .git directory).
#
# Apple containers' build context scanner has a bug where it tries to
# stat every file in the directory tree before applying .dockerignore,
# causing failures on repos with dangling APFS directory entries.
# Building from a clean temp directory avoids this entirely.
#
# Usage:
#   ./scripts/build-container.sh              # Build plural-claude image
#   ./scripts/build-container.sh my-image     # Build with custom image name

set -e

IMAGE_NAME="${1:-plural-claude}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="$(mktemp -d)"

trap 'rm -rf "$BUILD_DIR"' EXIT

echo "Building plural binary for linux/arm64..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o "$BUILD_DIR/plural" "$REPO_ROOT"

echo "Preparing build context..."
cp "$REPO_ROOT/Containerfile" "$BUILD_DIR/Containerfile"
cp "$REPO_ROOT/entrypoint.sh" "$BUILD_DIR/entrypoint.sh"
cp "$REPO_ROOT/entrypoint-claude.sh" "$BUILD_DIR/entrypoint-claude.sh"

echo "Building container image: $IMAGE_NAME"
container build -t "$IMAGE_NAME" "$BUILD_DIR"

echo "Done. Image: $IMAGE_NAME"
