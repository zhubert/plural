#!/usr/bin/env bash
# Updates the vendorHash in flake.nix by attempting a build and capturing the correct hash
# Called by release.sh before committing version changes

set -e

FLAKE_FILE="flake.nix"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Set a fake hash to force Nix to compute the correct one
FAKE_HASH="sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

# Get current hash
CURRENT_HASH=$(grep 'vendorHash = "' "$FLAKE_FILE" | sed 's/.*vendorHash = "\([^"]*\)".*/\1/')

echo "  Current vendorHash: ${CURRENT_HASH:0:20}..."

# Temporarily set fake hash
sed -i '' "s|vendorHash = \"$CURRENT_HASH\"|vendorHash = \"$FAKE_HASH\"|" "$FLAKE_FILE"

# Try to build and capture the expected hash from the error
echo "  Computing new hash (this may take a moment)..."
BUILD_OUTPUT=$(nix build .#plural 2>&1 || true)

# Extract the correct hash from the error message
# Nix outputs something like: "got: sha256-abc123..."
NEW_HASH=$(echo "$BUILD_OUTPUT" | grep -oE 'got:[[:space:]]*sha256-[A-Za-z0-9+/=]+' | sed 's/got:[[:space:]]*//' | head -1)

if [ -z "$NEW_HASH" ]; then
    # Maybe build succeeded (hash was already correct), try to extract from a successful build
    # Or restore and report
    echo -e "${YELLOW}  Could not extract hash from build output, checking if build succeeded...${NC}"

    # Restore original hash and try building
    sed -i '' "s|vendorHash = \"$FAKE_HASH\"|vendorHash = \"$CURRENT_HASH\"|" "$FLAKE_FILE"

    if nix build .#plural 2>/dev/null; then
        echo -e "${GREEN}  Build succeeded with current hash - no update needed${NC}"
        exit 0
    else
        echo -e "${RED}  Error: Could not determine correct vendorHash${NC}"
        echo "Build output:"
        echo "$BUILD_OUTPUT"
        exit 1
    fi
fi

# Update with the correct hash
sed -i '' "s|vendorHash = \"$FAKE_HASH\"|vendorHash = \"$NEW_HASH\"|" "$FLAKE_FILE"

if [ "$CURRENT_HASH" = "$NEW_HASH" ]; then
    echo -e "${GREEN}  Hash unchanged${NC}"
else
    echo -e "${GREEN}  Updated vendorHash: ${NEW_HASH:0:20}...${NC}"
fi
