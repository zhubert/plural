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

The project uses [GoReleaser](https://goreleaser.com/) for automated releases and Homebrew distribution.

### Prerequisites

```bash
# Install GoReleaser
brew install goreleaser

# Set up GitHub token for Homebrew tap updates
export HOMEBREW_TAP_GITHUB_TOKEN=your_github_token
```

### Creating a Release

```bash
# Tag the release
git tag v1.0.0
git push origin v1.0.0

# Run GoReleaser (requires GITHUB_TOKEN and HOMEBREW_TAP_GITHUB_TOKEN)
goreleaser release

# Or do a dry run to test without publishing
goreleaser release --snapshot --clean
```

### What GoReleaser Does

1. Builds binaries for Linux and macOS (amd64 and arm64)
2. Creates GitHub release with changelog
3. Generates checksums
4. Updates the Homebrew tap formula at `zhubert/homebrew-tap`
