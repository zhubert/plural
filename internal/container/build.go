package container

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/zhubert/plural/internal/logger"
)

// buildTimeout is the maximum time to wait for a docker build.
const buildTimeout = 10 * time.Minute

// inspectTimeout is the maximum time to wait for docker image inspect.
const inspectTimeout = 5 * time.Second

// defaultVersions provides fallback versions for language runtimes.
var defaultVersions = map[Language]string{
	LangGo:     "1.25",
	LangNode:   "22",
	LangPython: "3",
	LangRuby:   "3.2",
}

// releaseArch maps runtime.GOARCH to the architecture suffix used in GitHub release asset names.
func releaseArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "arm64"
	default:
		return runtime.GOARCH
	}
}

// nodeVersion returns the Node.js major version to use as the base image tag.
// If Node.js is among the detected languages, use that version; otherwise use default.
func nodeVersion(langs []DetectedLang) string {
	for _, l := range langs {
		if l.Language == LangNode && l.Version != "" {
			return l.Version
		}
	}
	return defaultVersions[LangNode]
}

// GenerateDockerfile creates a Dockerfile tailored to the detected languages.
// Node.js is always included as the base image since Claude Code requires it.
// Architecture is resolved at Go level (runtime.GOARCH) and baked into the Dockerfile
// to avoid issues with legacy docker build not setting TARGETARCH.
func GenerateDockerfile(langs []DetectedLang, version string) string {
	var b strings.Builder
	arch := releaseArch()

	// Determine Go version for builder stage
	goVersion := ""
	needsGo := false
	for _, l := range langs {
		if l.Language == LangGo {
			needsGo = true
			goVersion = l.Version
			break
		}
	}
	if goVersion == "" {
		goVersion = defaultVersions[LangGo]
	}

	// Builder stage for Go toolchain (gopls)
	if needsGo {
		fmt.Fprintf(&b, "FROM golang:%s-alpine AS builder\n\n", goVersion)
		b.WriteString("RUN CGO_ENABLED=0 go install golang.org/x/tools/gopls@latest\n\n")
	}

	// Runtime stage: use node:XX-alpine as base (includes nodejs + npm)
	nodeVer := nodeVersion(langs)
	fmt.Fprintf(&b, "FROM node:%s-alpine\n\n", nodeVer)

	// Base packages
	b.WriteString("RUN apk add --no-cache \\\n")
	b.WriteString("    bash \\\n")
	b.WriteString("    git \\\n")
	b.WriteString("    su-exec \\\n")
	b.WriteString("    curl \\\n")
	b.WriteString("    jq")

	// Language-specific packages
	for _, l := range langs {
		block := languageInstallBlock(l.Language)
		if block != "" {
			b.WriteString(block)
		}
	}
	b.WriteString("\n\n")

	// Install Claude CLI
	b.WriteString("RUN npm install -g @anthropic-ai/claude-code\n\n")

	// Go toolchain from builder
	if needsGo {
		b.WriteString("COPY --from=builder /usr/local/go /usr/local/go\n")
		b.WriteString("COPY --from=builder /go/bin/gopls /usr/local/bin/gopls\n")
		b.WriteString("ENV PATH=\"/usr/local/go/bin:$PATH\"\n\n")
	}

	// Download plural binary (arch baked in at generation time)
	b.WriteString(pluralDownloadBlock(version, arch))

	// Environment
	b.WriteString("ENV SHELL=/bin/bash\n\n")

	// Create non-root user
	b.WriteString("RUN adduser -D -s /bin/bash claude")
	if needsGo {
		b.WriteString(" && \\\n    mkdir -p /home/claude/go && \\\n    chown claude:claude /home/claude/go\n\n")
		b.WriteString("ENV GOPATH=/home/claude/go\n")
		b.WriteString("ENV PATH=\"$GOPATH/bin:$PATH\"\n\n")
	} else {
		b.WriteString("\n\n")
	}

	// Install mise and languages if needed
	miseBlock := miseInstallBlock(langs)
	if miseBlock != "" {
		b.WriteString(miseBlock)
	}

	// Entrypoint script (embedded inline)
	// Runs as root: updates Claude CLI, copies host config selectively, fixes permissions,
	// activates mise if installed, then switches to claude user and execs `claude`.
	b.WriteString("RUN printf '#!/bin/bash\\nset -e\\n")
	b.WriteString("# Update Claude CLI if possible\\n")
	b.WriteString("if [ -z \"$PLURAL_SKIP_UPDATE\" ]; then\\n")
	b.WriteString("  npm update -g @anthropic-ai/claude-code 2>/dev/null || true\\n")
	b.WriteString("fi\\n")
	b.WriteString("# Copy selective host claude config if mounted\\n")
	b.WriteString("if [ -d /home/claude/.claude-host ]; then\\n")
	b.WriteString("  mkdir -p /home/claude/.claude\\n")
	b.WriteString("  for f in settings.json CLAUDE.md .credentials.json; do\\n")
	b.WriteString("    [ -f /home/claude/.claude-host/$f ] && cp /home/claude/.claude-host/$f /home/claude/.claude/$f 2>/dev/null || true\\n")
	b.WriteString("  done\\n")
	b.WriteString("  for d in projects plugins; do\\n")
	b.WriteString("    [ -d /home/claude/.claude-host/$d ] && cp -r /home/claude/.claude-host/$d /home/claude/.claude/$d 2>/dev/null || true\\n")
	b.WriteString("  done\\n")
	b.WriteString("  chown -R claude:claude /home/claude/.claude 2>/dev/null || true\\n")
	b.WriteString("fi\\n")
	b.WriteString("# Fix workspace permissions\\n")
	b.WriteString("chown -R claude:claude /workspace 2>/dev/null || true\\n")
	b.WriteString("exec su-exec claude claude \"$@\"\\n")
	b.WriteString("' > /entrypoint.sh && chmod +x /entrypoint.sh\n\n")

	b.WriteString("WORKDIR /workspace\n\n")
	b.WriteString("ENTRYPOINT [\"/entrypoint.sh\"]\n")

	return b.String()
}

// languageInstallBlock returns the apk add fragment for a given language.
// Returns empty string for languages that don't need extra packages (e.g., Node is the base image).
// Ruby and Python are installed via mise (see miseInstallBlock), not apk.
func languageInstallBlock(lang Language) string {
	switch lang {
	case LangRuby:
		// build-base needed for native gem extensions; Ruby itself installed via mise
		return " \\\n    build-base \\\n    linux-headers \\\n    yaml-dev \\\n    zlib-dev \\\n    openssl-dev \\\n    readline-dev"
	case LangPython:
		// Build deps for Python; Python itself installed via mise
		return " \\\n    build-base \\\n    zlib-dev \\\n    bzip2-dev \\\n    xz-dev \\\n    readline-dev \\\n    sqlite-dev \\\n    openssl-dev \\\n    libffi-dev"
	case LangRust:
		return " \\\n    cargo \\\n    rust"
	case LangJava:
		return " \\\n    openjdk17-jdk \\\n    maven"
	default:
		return ""
	}
}

// miseLanguages are languages that should be installed via mise instead of apk.
var miseLanguages = map[Language]bool{
	LangRuby:   true,
	LangPython: true,
}

// miseInstallBlock generates Dockerfile commands to install mise and use it to
// install the correct versions of Ruby, Python, etc. as the claude user.
// Returns empty string if no mise-managed languages are detected.
func miseInstallBlock(langs []DetectedLang) string {
	var installs []string
	for _, l := range langs {
		if !miseLanguages[l.Language] {
			continue
		}
		version := l.Version
		if version == "" {
			version = defaultVersions[l.Language]
		}
		installs = append(installs, fmt.Sprintf("%s@%s", l.Language, version))
	}
	if len(installs) == 0 {
		return ""
	}

	var b strings.Builder

	// Install mise and languages as the claude user via su-exec.
	// su-exec avoids USER directive for legacy docker build compatibility.
	// HOME must be set explicitly since su-exec doesn't set it.
	b.WriteString("RUN HOME=/home/claude su-exec claude sh -c 'curl -fsSL https://mise.run | sh'\n\n")

	// Install languages via mise as claude user
	for _, install := range installs {
		fmt.Fprintf(&b, "RUN HOME=/home/claude su-exec claude /home/claude/.local/bin/mise use --global %s\n", install)
	}
	// Add mise shims to PATH for all subsequent commands and the entrypoint
	b.WriteString("ENV PATH=\"/home/claude/.local/share/mise/shims:$PATH\"\n\n")

	return b.String()
}

// pluralDownloadBlock generates the RUN command to download the plural binary.
// For "dev" or empty version, uses the /latest/download/ URL pattern.
// For release tags (e.g., "v0.1.0"), uses the exact release URL.
func pluralDownloadBlock(version, arch string) string {
	var b strings.Builder

	if version == "" || version == "dev" {
		// Use latest release URL pattern (no API call needed)
		fmt.Fprintf(&b, "RUN curl -sfL \"https://github.com/zhubert/plural/releases/latest/download/plural_Linux_%s.tar.gz\" | tar -xz -C /tmp plural && \\\n", arch)
		b.WriteString("    mv /tmp/plural /usr/local/bin/plural && \\\n")
		b.WriteString("    chmod +x /usr/local/bin/plural\n\n")
	} else {
		// Exact version download
		fmt.Fprintf(&b, "RUN curl -sfL \"https://github.com/zhubert/plural/releases/download/%s/plural_Linux_%s.tar.gz\" | tar -xz -C /tmp plural && \\\n", version, arch)
		b.WriteString("    mv /tmp/plural /usr/local/bin/plural && \\\n")
		b.WriteString("    chmod +x /usr/local/bin/plural\n\n")
	}

	return b.String()
}

// ImageTag returns a content-addressed image tag based on the Dockerfile content.
// Format: "plural:<first 12 chars of sha256 hex>".
func ImageTag(dockerfile string) string {
	h := sha256.Sum256([]byte(dockerfile))
	return fmt.Sprintf("plural:%x", h[:6])
}

// EnsureImage checks if the image for the given languages already exists locally,
// and builds it if not. Returns the image tag used.
func EnsureImage(ctx context.Context, langs []DetectedLang, version string) (string, error) {
	log := logger.WithComponent("container")

	dockerfile := GenerateDockerfile(langs, version)
	tag := ImageTag(dockerfile)

	// Check if image already exists
	if imageExists(ctx, tag) {
		log.Info("container image already cached", "tag", tag)
		return tag, nil
	}

	log.Info("building container image", "tag", tag, "langs", langSummary(langs))

	// Build the image using docker build with stdin Dockerfile.
	// Use an empty temp dir as build context since we don't COPY any local files.
	buildCtx, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()

	tmpDir, err := os.MkdirTemp("", "plural-build-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp build context: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.CommandContext(buildCtx, "docker", "build", "-t", tag, "-f-", tmpDir)
	cmd.Stdin = strings.NewReader(dockerfile)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error("container image build failed", "tag", tag, "error", err, "output", string(output))
		return "", fmt.Errorf("docker build failed: %w\n%s", err, string(output))
	}

	log.Info("container image built successfully", "tag", tag)
	return tag, nil
}

// imageExists checks if a Docker image exists locally.
func imageExists(ctx context.Context, tag string) bool {
	checkCtx, cancel := context.WithTimeout(ctx, inspectTimeout)
	defer cancel()
	cmd := exec.CommandContext(checkCtx, "docker", "image", "inspect", tag)
	return cmd.Run() == nil
}

// langSummary returns a comma-separated summary of detected languages for logging.
func langSummary(langs []DetectedLang) string {
	names := make([]string, len(langs))
	for i, l := range langs {
		if l.Version != "" {
			names[i] = fmt.Sprintf("%s@%s", l.Language, l.Version)
		} else {
			names[i] = string(l.Language)
		}
	}
	return strings.Join(names, ",")
}
