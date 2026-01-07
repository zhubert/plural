# Contributing to Plural

## Building from Source

### Requirements

- Go 1.24+
- [Claude Code CLI](https://claude.ai/code) installed and authenticated
- Git
- GitHub CLI (`gh`) for PR creation (optional)

### Build

```bash
git clone https://github.com/zhubert/plural.git
cd plural
go build -o plural .
```

### Run

```bash
./plural
```

### Run Tests

```bash
go test ./...
```

## Debug

Logs are written to `/tmp/plural-debug.log`:
```bash
tail -f /tmp/plural-debug.log
```

MCP server logs (per session):
```bash
tail -f /tmp/plural-mcp-*.log
```

## Releasing

The project uses [GoReleaser](https://goreleaser.com/) for automated releases and supports distribution via Homebrew and Devbox/Nix.

### Prerequisites

```bash
# Install GoReleaser
brew install goreleaser

# Set up GitHub tokens
export GITHUB_TOKEN=your_github_token
export HOMEBREW_TAP_GITHUB_TOKEN=your_github_token
```

### Creating a Release

1. **Update version in `flake.nix`** (for Nix/Devbox users):
   ```bash
   # Edit flake.nix and update the version string
   # version = "X.Y.Z";
   ```

2. **Commit version changes**:
   ```bash
   git add flake.nix
   git commit -m "Bump version to vX.Y.Z"
   ```

3. **Tag the release**:
   ```bash
   git tag vX.Y.Z
   ```

4. **Push the commit and tag**:
   ```bash
   git push origin main
   git push origin vX.Y.Z
   ```

5. **Run GoReleaser**:
   ```bash
   goreleaser release --clean
   ```

6. **Update Nix flake hash** (if dependencies changed):

   If `go.mod` dependencies changed since the last release, the `vendorHash` in `flake.nix` needs updating:
   ```bash
   # Temporarily set vendorHash to get the new hash
   # In flake.nix, change vendorHash to: pkgs.lib.fakeHash

   nix build  # This will fail and print the correct hash

   # Update vendorHash with the printed hash, commit, and push
   ```

### Dry Run

To test the release process without publishing:
```bash
goreleaser release --snapshot --clean
```

### What GoReleaser Does

1. Builds binaries for Linux and macOS (amd64 and arm64)
2. Creates GitHub release with changelog
3. Generates checksums
4. Updates the Homebrew tap formula at `zhubert/homebrew-tap`

### Distribution Channels

After releasing, users can install via:

**Homebrew:**
```bash
brew tap zhubert/tap
brew install plural
```

**Devbox:**
```json
{
  "packages": ["github:zhubert/plural"]
}
```

**Nix:**
```bash
nix run github:zhubert/plural
# or
nix profile install github:zhubert/plural
```
