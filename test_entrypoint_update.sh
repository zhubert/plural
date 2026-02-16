#!/bin/sh
# Test script for entrypoint.sh update logic
# This tests the update_plural_binary function in isolation

set -e

# Colors for test output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

TESTS_PASSED=0
TESTS_FAILED=0

# Test helper functions
pass() {
    TESTS_PASSED=$((TESTS_PASSED + 1))
    echo "${GREEN}✓${NC} $1"
}

fail() {
    TESTS_FAILED=$((TESTS_FAILED + 1))
    echo "${RED}✗${NC} $1"
}

info() {
    echo "${YELLOW}ℹ${NC} $1"
}

# Mock plural binary for testing
create_mock_plural() {
    VERSION="$1"
    cat > /tmp/test-plural <<EOF
#!/bin/sh
echo "plural version $VERSION"
EOF
    chmod +x /tmp/test-plural
}

# Mock GitHub API response
create_mock_api_response() {
    VERSION="$1"
    ARCH="$2"
    cat > /tmp/mock-api.json <<EOF
{
    "tag_name": "$VERSION",
    "assets": [
        {
            "name": "plural_Linux_${ARCH}.tar.gz",
            "browser_download_url": "https://example.com/plural_Linux_${ARCH}.tar.gz"
        }
    ]
}
EOF
}

# Extract the update function from entrypoint.sh for testing
# We'll create a testable version
create_test_function() {
    cat > /tmp/update_test.sh <<'EOF'
#!/bin/sh
set -e

# Test version of update function with injectable dependencies
update_plural_binary() {
    if [ -n "$PLURAL_SKIP_UPDATE" ]; then
        echo "[plural-update] Auto-update disabled via PLURAL_SKIP_UPDATE"
        return 0
    fi

    echo "[plural-update] Checking for updates..."

    # Get current version (use mock binary path for testing)
    PLURAL_BIN="${PLURAL_BIN:-/usr/local/bin/plural}"
    CURRENT_VERSION=$($PLURAL_BIN --version 2>&1 | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' || echo "unknown")
    echo "[plural-update] Current version: $CURRENT_VERSION"

    # Fetch latest release info with timeout
    # For testing, allow override of the API endpoint
    API_URL="${GITHUB_API_URL:-https://api.github.com/repos/zhubert/plural/releases/latest}"
    LATEST_INFO=$(curl -sL --connect-timeout 5 --max-time 10 "$API_URL" 2>/dev/null || echo "")

    if [ -z "$LATEST_INFO" ]; then
        echo "[plural-update] Failed to fetch latest release info (network issue or timeout), skipping update"
        return 0
    fi

    LATEST_VERSION=$(echo "$LATEST_INFO" | jq -r '.tag_name // "unknown"' 2>/dev/null || echo "unknown")

    if [ "$LATEST_VERSION" = "unknown" ] || [ -z "$LATEST_VERSION" ]; then
        echo "[plural-update] Could not determine latest version, skipping update"
        return 0
    fi

    echo "[plural-update] Latest version: $LATEST_VERSION"

    # Compare versions (skip if same)
    if [ "$CURRENT_VERSION" = "$LATEST_VERSION" ]; then
        echo "[plural-update] Already running latest version"
        return 0
    fi

    echo "[plural-update] New version available, updating from $CURRENT_VERSION to $LATEST_VERSION..."

    # Determine architecture (map Docker arch to GoReleaser arch)
    ARCH=$(uname -m)
    if [ "$ARCH" = "x86_64" ]; then
        GOARCH="x86_64"
    elif [ "$ARCH" = "aarch64" ]; then
        GOARCH="arm64"
    else
        GOARCH="$ARCH"
    fi

    # Find download URL for this architecture
    DOWNLOAD_URL=$(echo "$LATEST_INFO" | \
        jq -r ".assets[] | select(.name | contains(\"Linux_${GOARCH}\")) | .browser_download_url" 2>/dev/null | head -n1 || echo "")

    if [ -z "$DOWNLOAD_URL" ]; then
        echo "[plural-update] Could not find download URL for architecture $GOARCH, skipping update"
        return 0
    fi

    echo "[plural-update] Downloading from: $DOWNLOAD_URL"

    # For testing, we skip the actual download
    if [ -n "$SKIP_DOWNLOAD" ]; then
        echo "[plural-update] Skipping download (test mode)"
        return 0
    fi

    # Download and install new binary
    if curl -sL --connect-timeout 5 --max-time 30 "$DOWNLOAD_URL" | tar -xz -C /tmp plural 2>/dev/null; then
        if [ -f /tmp/plural ]; then
            mv /tmp/plural "$PLURAL_BIN"
            chmod +x "$PLURAL_BIN"
            echo "[plural-update] Successfully updated to $LATEST_VERSION"
        else
            echo "[plural-update] Failed to extract binary, skipping update"
            return 0
        fi
    else
        echo "[plural-update] Failed to download or extract update, skipping"
        return 0
    fi
}

# Run the update function
update_plural_binary
EOF
    chmod +x /tmp/update_test.sh
}

# Test 1: Skip update when PLURAL_SKIP_UPDATE is set
test_skip_update() {
    info "Test: Skip update when PLURAL_SKIP_UPDATE is set"

    export PLURAL_SKIP_UPDATE=1
    OUTPUT=$(sh /tmp/update_test.sh 2>&1)
    unset PLURAL_SKIP_UPDATE

    if echo "$OUTPUT" | grep -q "Auto-update disabled"; then
        pass "Update skipped when PLURAL_SKIP_UPDATE is set"
    else
        fail "Update not skipped when PLURAL_SKIP_UPDATE is set"
    fi
}

# Test 2: Handle missing version in current binary
test_missing_current_version() {
    info "Test: Handle missing version in current binary"

    cat > /tmp/test-plural-noversion <<'EOF'
#!/bin/sh
echo "plural: unknown version"
EOF
    chmod +x /tmp/test-plural-noversion

    create_mock_api_response "v0.2.0" "x86_64"

    export PLURAL_BIN=/tmp/test-plural-noversion
    export GITHUB_API_URL="file:///tmp/mock-api.json"
    export SKIP_DOWNLOAD=1
    OUTPUT=$(sh /tmp/update_test.sh 2>&1)
    unset PLURAL_BIN GITHUB_API_URL SKIP_DOWNLOAD

    if echo "$OUTPUT" | grep -q "Current version: unknown"; then
        pass "Handles missing current version gracefully"
    else
        fail "Did not handle missing current version"
    fi
}

# Test 3: Skip update when versions match
test_same_version() {
    info "Test: Skip update when versions match"

    create_mock_plural "v0.1.5"
    create_mock_api_response "v0.1.5" "x86_64"

    export PLURAL_BIN=/tmp/test-plural
    export GITHUB_API_URL="file:///tmp/mock-api.json"
    export SKIP_DOWNLOAD=1
    OUTPUT=$(sh /tmp/update_test.sh 2>&1)
    unset PLURAL_BIN GITHUB_API_URL SKIP_DOWNLOAD

    if echo "$OUTPUT" | grep -q "Already running latest version"; then
        pass "Skips update when versions match"
    else
        fail "Did not skip update when versions match"
    fi
}

# Test 4: Detect new version available
test_new_version_available() {
    info "Test: Detect new version available"

    create_mock_plural "v0.1.5"
    create_mock_api_response "v0.2.0" "x86_64"

    export PLURAL_BIN=/tmp/test-plural
    export GITHUB_API_URL="file:///tmp/mock-api.json"
    export SKIP_DOWNLOAD=1
    OUTPUT=$(sh /tmp/update_test.sh 2>&1)
    unset PLURAL_BIN GITHUB_API_URL SKIP_DOWNLOAD

    if echo "$OUTPUT" | grep -q "New version available.*v0.1.5.*v0.2.0"; then
        pass "Detects new version available"
    else
        fail "Did not detect new version available"
    fi
}

# Test 5: Handle network failure gracefully
test_network_failure() {
    info "Test: Handle network failure gracefully"

    create_mock_plural "v0.1.5"

    export PLURAL_BIN=/tmp/test-plural
    export GITHUB_API_URL="http://invalid.example.com/nonexistent"
    export SKIP_DOWNLOAD=1
    OUTPUT=$(sh /tmp/update_test.sh 2>&1)
    EXITCODE=$?
    unset PLURAL_BIN GITHUB_API_URL SKIP_DOWNLOAD

    if [ $EXITCODE -eq 0 ] && echo "$OUTPUT" | grep -q "Failed to fetch latest release info"; then
        pass "Handles network failure gracefully"
    else
        fail "Did not handle network failure gracefully (exit code: $EXITCODE)"
    fi
}

# Test 6: Handle malformed API response
test_malformed_api_response() {
    info "Test: Handle malformed API response"

    create_mock_plural "v0.1.5"
    echo "invalid json" > /tmp/mock-api.json

    export PLURAL_BIN=/tmp/test-plural
    export GITHUB_API_URL="file:///tmp/mock-api.json"
    export SKIP_DOWNLOAD=1
    OUTPUT=$(sh /tmp/update_test.sh 2>&1)
    EXITCODE=$?
    unset PLURAL_BIN GITHUB_API_URL SKIP_DOWNLOAD

    if [ $EXITCODE -eq 0 ] && echo "$OUTPUT" | grep -q "Could not determine latest version"; then
        pass "Handles malformed API response gracefully"
    else
        fail "Did not handle malformed API response gracefully"
    fi
}

# Test 7: Verify architecture mapping
test_architecture_detection() {
    info "Test: Architecture detection and mapping"

    create_mock_plural "v0.1.5"
    ARCH=$(uname -m)

    if [ "$ARCH" = "x86_64" ]; then
        EXPECTED_ARCH="x86_64"
    elif [ "$ARCH" = "aarch64" ]; then
        EXPECTED_ARCH="arm64"
    else
        EXPECTED_ARCH="$ARCH"
    fi

    create_mock_api_response "v0.2.0" "$EXPECTED_ARCH"

    export PLURAL_BIN=/tmp/test-plural
    export GITHUB_API_URL="file:///tmp/mock-api.json"
    export SKIP_DOWNLOAD=1
    OUTPUT=$(sh /tmp/update_test.sh 2>&1)
    unset PLURAL_BIN GITHUB_API_URL SKIP_DOWNLOAD

    if echo "$OUTPUT" | grep -q "Downloading from.*Linux_${EXPECTED_ARCH}"; then
        pass "Correctly maps architecture $ARCH to $EXPECTED_ARCH"
    else
        fail "Did not correctly map architecture"
    fi
}

# Main test execution
main() {
    echo "================================"
    echo "Entrypoint Update Logic Tests"
    echo "================================"
    echo ""

    # Setup
    create_test_function

    # Run tests
    test_skip_update
    test_missing_current_version
    test_same_version
    test_new_version_available
    test_network_failure
    test_malformed_api_response
    test_architecture_detection

    # Cleanup
    rm -f /tmp/test-plural* /tmp/mock-api.json /tmp/update_test.sh

    # Summary
    echo ""
    echo "================================"
    echo "Test Results"
    echo "================================"
    echo "${GREEN}Passed: $TESTS_PASSED${NC}"
    echo "${RED}Failed: $TESTS_FAILED${NC}"
    echo ""

    if [ $TESTS_FAILED -gt 0 ]; then
        echo "${RED}Some tests failed${NC}"
        exit 1
    else
        echo "${GREEN}All tests passed${NC}"
        exit 0
    fi
}

# Run tests
main
