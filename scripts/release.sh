#!/bin/bash
#
# Release script for Plural
# Usage: ./scripts/release.sh <patch|minor|major> [--dry-run]
#
# This script automates the release process for Homebrew distribution.
# It automatically determines the next version based on the latest git tag.
#
# Examples:
#   ./scripts/release.sh patch      # v0.0.3 -> v0.0.4
#   ./scripts/release.sh minor      # v0.0.3 -> v0.1.0
#   ./scripts/release.sh major      # v0.0.3 -> v1.0.0

set -e

# Get the directory where this script lives
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Change to repo root for all operations
cd "$REPO_ROOT"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Parse arguments
BUMP_TYPE=""
DRY_RUN=false

for arg in "$@"; do
    case $arg in
        --dry-run)
            DRY_RUN=true
            ;;
        patch|minor|major)
            BUMP_TYPE="$arg"
            ;;
        *)
            echo -e "${RED}Unknown argument: $arg${NC}"
            echo "Usage: ./release.sh <patch|minor|major> [--dry-run]"
            exit 1
            ;;
    esac
done

# Validate bump type argument
if [ -z "$BUMP_TYPE" ]; then
    echo -e "${RED}Error: Bump type argument required (patch, minor, or major)${NC}"
    echo "Usage: ./release.sh <patch|minor|major> [--dry-run]"
    exit 1
fi

# Get the latest version tag
LATEST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")

# Validate the latest tag format
if ! [[ "$LATEST_TAG" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
    echo -e "${RED}Error: Latest tag '$LATEST_TAG' is not in format vX.Y.Z${NC}"
    exit 1
fi

# Extract version components
MAJOR="${BASH_REMATCH[1]}"
MINOR="${BASH_REMATCH[2]}"
PATCH="${BASH_REMATCH[3]}"

# Calculate new version based on bump type
case $BUMP_TYPE in
    patch)
        PATCH=$((PATCH + 1))
        ;;
    minor)
        MINOR=$((MINOR + 1))
        PATCH=0
        ;;
    major)
        MAJOR=$((MAJOR + 1))
        MINOR=0
        PATCH=0
        ;;
esac

VERSION="v${MAJOR}.${MINOR}.${PATCH}"

echo -e "Current version: ${YELLOW}${LATEST_TAG}${NC}"
echo -e "New version:     ${GREEN}${VERSION}${NC} (${BUMP_TYPE} bump)"
echo ""

echo -e "${GREEN}Preparing release ${VERSION}${NC}"
if [ "$DRY_RUN" = true ]; then
    echo -e "${YELLOW}(Dry run mode - no changes will be pushed)${NC}"
fi

# Check prerequisites
echo ""
echo "Checking prerequisites..."

# Check for goreleaser
if ! command -v goreleaser &> /dev/null; then
    echo -e "${RED}Error: goreleaser is not installed${NC}"
    echo "Install with: brew install goreleaser"
    exit 1
fi
echo "  goreleaser: found"

# Check for required environment variables
if [ -z "$GITHUB_TOKEN" ]; then
    echo -e "${RED}Error: GITHUB_TOKEN environment variable is not set${NC}"
    exit 1
fi
echo "  GITHUB_TOKEN: set"

if [ -z "$HOMEBREW_TAP_GITHUB_TOKEN" ]; then
    echo -e "${RED}Error: HOMEBREW_TAP_GITHUB_TOKEN environment variable is not set${NC}"
    exit 1
fi
echo "  HOMEBREW_TAP_GITHUB_TOKEN: set"

# Check for clean working directory
if [ -n "$(git status --porcelain)" ]; then
    echo -e "${RED}Error: Working directory is not clean${NC}"
    echo "Please commit or stash your changes before releasing."
    git status --short
    exit 1
fi
echo "  Working directory: clean"

# Check we're on main branch
CURRENT_BRANCH=$(git branch --show-current)
if [ "$CURRENT_BRANCH" != "main" ]; then
    echo -e "${RED}Error: Not on main branch (currently on: $CURRENT_BRANCH)${NC}"
    echo "Please switch to main branch before releasing."
    exit 1
fi
echo "  Branch: main"

# Check if tag already exists
if git rev-parse "$VERSION" >/dev/null 2>&1; then
    echo -e "${RED}Error: Tag $VERSION already exists${NC}"
    exit 1
fi
echo "  Tag $VERSION: available"

echo ""
echo -e "${GREEN}Prerequisites check passed${NC}"

# Step 1: Tag the release
echo ""
echo "Step 1: Creating tag ${VERSION}..."

git tag "$VERSION"
echo "  Tagged"

if [ "$DRY_RUN" = true ]; then
    # Dry run: run goreleaser snapshot and then undo changes
    echo ""
    echo "Step 2: Running goreleaser (snapshot mode)..."
    goreleaser release --snapshot --clean

    echo ""
    echo -e "${YELLOW}Dry run complete. Reverting changes...${NC}"
    git tag -d "$VERSION"
    echo "  Reverted tag"

    echo ""
    echo -e "${GREEN}Dry run completed successfully!${NC}"
    echo "To perform the actual release, run: ./scripts/release.sh ${BUMP_TYPE}"
else
    # Actual release
    echo ""
    echo "Step 2: Pushing tag to origin..."

    git push origin "$VERSION"
    echo "  Pushed"

    echo ""
    echo "Step 3: Running goreleaser..."

    goreleaser release --clean

    echo ""
    echo -e "${GREEN}Release ${VERSION} completed successfully!${NC}"
    echo ""
    echo "Next steps:"
    echo "  - Verify the GitHub release: https://github.com/zhubert/plural/releases/tag/${VERSION}"
    echo "  - Test Homebrew installation: brew upgrade plural"
fi
