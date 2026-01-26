# Contributing to Plural

Thank you for your interest in contributing to Plural! This document provides guidelines for contributing to the project.

## How to Contribute

### Reporting Bugs

Before submitting a bug report:
- Check existing [GitHub Issues](https://github.com/zhubert/plural/issues) to avoid duplicates
- Run with `--debug` flag and include relevant logs from `/tmp/plural-debug.log`

When filing a bug report, include:
- Steps to reproduce the issue
- Expected vs actual behavior
- Your environment (OS, Go version, Claude Code version)
- Relevant log output

### Suggesting Features

Open a GitHub Issue with:
- A clear description of the feature
- The problem it solves or use case it enables
- Any implementation ideas (optional)

### Submitting Pull Requests

1. **Start with an issue**: For non-trivial changes, open an issue first to discuss the approach
2. **Fork and branch**: Create a feature branch from `main`
3. **Keep changes focused**: One logical change per PR
4. **Write tests**: Add tests for new functionality
5. **Update documentation**: Update CLAUDE.md and README.md if behavior changes
6. **Test locally**: Run `go test ./...` and test the feature manually

#### PR Guidelines

- Write clear commit messages explaining *why*, not just *what*
- Keep PRs reasonably sized—large changes are harder to review
- Respond to review feedback promptly
- Squash fixup commits before merge

#### Code Style

- Follow standard Go conventions (`go fmt`, `go vet`)
- Match existing code patterns in the codebase
- Add comments for non-obvious logic

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

## Debugging

Main application logs:
```bash
tail -f /tmp/plural-debug.log
```

MCP server logs (per session):
```bash
tail -f /tmp/plural-mcp-*.log
```

Use `--debug` flag for verbose output.

## Releasing (Maintainers)

The project uses [GoReleaser](https://goreleaser.com/) for automated releases. The release script automatically determines the next version based on the latest git tag:

```bash
./scripts/release.sh patch            # v0.0.3 -> v0.0.4
./scripts/release.sh minor            # v0.0.3 -> v0.1.0
./scripts/release.sh major            # v0.0.3 -> v1.0.0
./scripts/release.sh patch --dry-run  # Dry run
```

### Distribution Channels

After releasing, users can install via:

**Homebrew:**
```bash
brew tap zhubert/tap
brew install plural
```

**Nix/Devbox:**
```bash
nix run github:zhubert/plural
```

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
