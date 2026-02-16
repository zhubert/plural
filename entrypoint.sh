#!/bin/sh
set -e

# Auto-update plural binary if a newer version is available
# Skip if PLURAL_SKIP_UPDATE is set to any non-empty value
update_plural_binary() {
    if [ -n "$PLURAL_SKIP_UPDATE" ]; then
        echo "[plural-update] Auto-update disabled via PLURAL_SKIP_UPDATE"
        return 0
    fi

    echo "[plural-update] Checking for updates..."

    # Get current version
    CURRENT_VERSION=$(/usr/local/bin/plural --version 2>&1 | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' || echo "unknown")
    echo "[plural-update] Current version: $CURRENT_VERSION"

    # Fetch latest release info with timeout
    LATEST_INFO=$(curl -sL --connect-timeout 5 --max-time 10 \
        https://api.github.com/repos/zhubert/plural/releases/latest 2>/dev/null || echo "")

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

    # Download and install new binary
    if curl -sL --connect-timeout 5 --max-time 30 "$DOWNLOAD_URL" | tar -xz -C /tmp plural 2>/dev/null; then
        if [ -f /tmp/plural ]; then
            mv /tmp/plural /usr/local/bin/plural
            chmod +x /usr/local/bin/plural
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

# Run update check (don't fail container startup if it fails)
update_plural_binary || echo "[plural-update] Update check failed but continuing startup"

# Switch to claude user for the rest of the entrypoint.
# Everything below runs as claude via su-exec (like gosu but Alpine-native).
exec su-exec claude /home/claude/entrypoint-claude.sh "$@"
