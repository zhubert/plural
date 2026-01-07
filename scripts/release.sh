#!/bin/bash
#
# Release script for Plural
# Usage: ./scripts/release.sh v0.0.5 [--dry-run]
#
# This script automates the release process for Homebrew and Nix distribution.

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
VERSION=""
DRY_RUN=false

for arg in "$@"; do
    case $arg in
        --dry-run)
            DRY_RUN=true
            ;;
        v*)
            VERSION="$arg"
            ;;
        *)
            echo -e "${RED}Unknown argument: $arg${NC}"
            echo "Usage: ./release.sh v0.0.5 [--dry-run]"
            exit 1
            ;;
    esac
done

# Validate version argument
if [ -z "$VERSION" ]; then
    echo -e "${RED}Error: Version argument required${NC}"
    echo "Usage: ./release.sh v0.0.5 [--dry-run]"
    exit 1
fi

# Validate version format (vX.Y.Z)
if ! [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo -e "${RED}Error: Version must be in format vX.Y.Z (e.g., v0.0.5)${NC}"
    exit 1
fi

# Strip the 'v' prefix for flake.nix (uses "0.0.5" not "v0.0.5")
VERSION_NUMBER="${VERSION#v}"

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

# Step 1: Update version in flake.nix
echo ""
echo "Step 1: Updating version in flake.nix to ${VERSION_NUMBER}..."

# Use sed to update the version line in flake.nix
sed -i '' "s/version = \"[0-9]*\.[0-9]*\.[0-9]*\";/version = \"${VERSION_NUMBER}\";/" flake.nix

# Verify the change was made
if ! grep -q "version = \"${VERSION_NUMBER}\";" flake.nix; then
    echo -e "${RED}Error: Failed to update version in flake.nix${NC}"
    git checkout flake.nix
    exit 1
fi
echo "  Updated version"

# Step 2: Update vendorHash in flake.nix
echo ""
echo "Step 2: Updating vendorHash in flake.nix..."

if ! "$SCRIPT_DIR/update-vendor-hash.sh"; then
    echo -e "${RED}Error: Failed to update vendorHash${NC}"
    git checkout flake.nix
    exit 1
fi

# Step 3: Commit the version change
echo ""
echo "Step 3: Committing version change..."

git add flake.nix
git commit -m "Bump version to ${VERSION}"
echo "  Committed"

# Step 4: Tag the release
echo ""
echo "Step 4: Creating tag ${VERSION}..."

git tag "$VERSION"
echo "  Tagged"

if [ "$DRY_RUN" = true ]; then
    # Dry run: run goreleaser snapshot and then undo changes
    echo ""
    echo "Step 5: Running goreleaser (snapshot mode)..."
    goreleaser release --snapshot --clean

    echo ""
    echo -e "${YELLOW}Dry run complete. Reverting changes...${NC}"
    git tag -d "$VERSION"
    git reset --hard HEAD~1
    echo "  Reverted commit and tag"

    echo ""
    echo -e "${GREEN}Dry run completed successfully!${NC}"
    echo "To perform the actual release, run: ./scripts/release.sh ${VERSION}"
else
    # Actual release
    echo ""
    echo "Step 5: Pushing commit and tag to origin..."

    git push origin main
    git push origin "$VERSION"
    echo "  Pushed"

    echo ""
    echo "Step 6: Running goreleaser..."

    goreleaser release --clean

    echo ""
    echo -e "${GREEN}Release ${VERSION} completed successfully!${NC}"
    echo ""
    echo "Next steps:"
    echo "  - Verify the GitHub release: https://github.com/zhubert/plural/releases/tag/${VERSION}"
    echo "  - Test Homebrew installation: brew upgrade plural"
    echo "  - Test Nix installation: nix run github:zhubert/plural"
fi
