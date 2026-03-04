package container

import (
	"runtime"
	"strings"
	"testing"
)

func TestGenerateDockerfile_GoOnly(t *testing.T) {
	langs := []DetectedLang{{Language: LangGo, Version: "1.22"}}
	df := GenerateDockerfile(langs, "v0.5.0")

	if !strings.Contains(df, "golang:1.22-alpine AS builder") {
		t.Error("expected Go builder stage with version 1.22")
	}
	if !strings.Contains(df, "gopls") {
		t.Error("expected gopls installation")
	}
	// Should use node:XX-alpine as base (not plain alpine)
	if !strings.Contains(df, "FROM node:") {
		t.Error("expected node:XX-alpine base image")
	}
	if !strings.Contains(df, "GOPATH") {
		t.Error("expected GOPATH setup for Go repos")
	}
	if !strings.Contains(df, "npm install -g @anthropic-ai/claude-code") {
		t.Error("expected Claude CLI installation")
	}
	// Should NOT contain TARGETARCH — arch is baked in at Go level
	if strings.Contains(df, "TARGETARCH") {
		t.Error("should not contain TARGETARCH — arch should be baked in")
	}
}

func TestGenerateDockerfile_NodeOnly(t *testing.T) {
	langs := []DetectedLang{{Language: LangNode, Version: "20"}}
	df := GenerateDockerfile(langs, "v0.1.0")

	if strings.Contains(df, "golang") {
		t.Error("should not include Go builder stage for Node-only repo")
	}
	// Should use node:20-alpine as base
	if !strings.Contains(df, "FROM node:20-alpine") {
		t.Error("expected node:20-alpine base image for Node.js 20 repo")
	}
	// Should contain the versioned download URL
	if !strings.Contains(df, "v0.1.0") {
		t.Error("expected version v0.1.0 in download URL")
	}
}

func TestGenerateDockerfile_PythonOnly(t *testing.T) {
	langs := []DetectedLang{{Language: LangPython, Version: "3.12"}}
	df := GenerateDockerfile(langs, "v1.0.0")

	if !strings.Contains(df, "python3") {
		t.Error("expected python3 package")
	}
	if !strings.Contains(df, "py3-pip") {
		t.Error("expected py3-pip package")
	}
}

func TestGenerateDockerfile_RubyOnly(t *testing.T) {
	langs := []DetectedLang{{Language: LangRuby, Version: "3.2"}}
	df := GenerateDockerfile(langs, "v1.0.0")

	if !strings.Contains(df, "ruby") {
		t.Error("expected ruby package")
	}
	if !strings.Contains(df, "ruby-dev") {
		t.Error("expected ruby-dev package")
	}
	if !strings.Contains(df, "build-base") {
		t.Error("expected build-base for native extensions")
	}
}

func TestGenerateDockerfile_RustOnly(t *testing.T) {
	langs := []DetectedLang{{Language: LangRust}}
	df := GenerateDockerfile(langs, "v1.0.0")

	if !strings.Contains(df, "cargo") {
		t.Error("expected cargo package")
	}
	if !strings.Contains(df, "rust") {
		t.Error("expected rust package")
	}
}

func TestGenerateDockerfile_JavaOnly(t *testing.T) {
	langs := []DetectedLang{{Language: LangJava}}
	df := GenerateDockerfile(langs, "v1.0.0")

	if !strings.Contains(df, "openjdk17") {
		t.Error("expected openjdk17 package")
	}
	if !strings.Contains(df, "maven") {
		t.Error("expected maven package")
	}
}

func TestGenerateDockerfile_MultiLanguage(t *testing.T) {
	langs := []DetectedLang{
		{Language: LangGo, Version: "1.22"},
		{Language: LangPython, Version: "3.12"},
	}
	df := GenerateDockerfile(langs, "v1.0.0")

	if !strings.Contains(df, "golang:1.22-alpine") {
		t.Error("expected Go builder stage")
	}
	if !strings.Contains(df, "python3") {
		t.Error("expected Python packages")
	}
	if !strings.Contains(df, "gopls") {
		t.Error("expected gopls")
	}
}

func TestGenerateDockerfile_NoLanguages(t *testing.T) {
	df := GenerateDockerfile(nil, "v1.0.0")

	// Should still use node:XX-alpine base (for Claude CLI)
	if !strings.Contains(df, "FROM node:") {
		t.Error("expected node base image even with no detected languages")
	}
	if !strings.Contains(df, "npm install -g @anthropic-ai/claude-code") {
		t.Error("expected Claude CLI installation")
	}
	// No Go builder stage
	if strings.Contains(df, "golang") {
		t.Error("should not include Go builder stage when Go not detected")
	}
}

func TestGenerateDockerfile_DefaultGoVersion(t *testing.T) {
	// Go detected but no version
	langs := []DetectedLang{{Language: LangGo}}
	df := GenerateDockerfile(langs, "v1.0.0")

	if !strings.Contains(df, "golang:1.25-alpine") {
		t.Error("expected default Go version 1.25")
	}
}

func TestGenerateDockerfile_DevVersion(t *testing.T) {
	df := GenerateDockerfile(nil, "dev")

	// Should use latest/download URL pattern, not "dev" as a tag
	if !strings.Contains(df, "/latest/download/") {
		t.Error("expected /latest/download/ URL pattern for dev version")
	}
	if strings.Contains(df, "/download/dev/") {
		t.Error("should not use 'dev' as a release tag")
	}
}

func TestGenerateDockerfile_EmptyVersion(t *testing.T) {
	df := GenerateDockerfile(nil, "")

	// Should use latest/download URL pattern
	if !strings.Contains(df, "/latest/download/") {
		t.Error("expected /latest/download/ URL pattern when version is empty")
	}
}

func TestGenerateDockerfile_ReleaseVersion(t *testing.T) {
	df := GenerateDockerfile(nil, "v0.5.0")

	if !strings.Contains(df, "/download/v0.5.0/") {
		t.Error("expected exact version in download URL")
	}
	if strings.Contains(df, "/latest/download/") {
		t.Error("should not use latest for a specific version")
	}
}

func TestGenerateDockerfile_ArchBakedIn(t *testing.T) {
	df := GenerateDockerfile(nil, "v1.0.0")

	// Architecture should be baked in at Go level
	arch := releaseArch()
	if !strings.Contains(df, arch) {
		t.Errorf("expected baked-in architecture %q in Dockerfile", arch)
	}
	// Should NOT contain any Docker ARG for architecture
	if strings.Contains(df, "TARGETARCH") {
		t.Error("should not contain TARGETARCH — arch resolved at Go level")
	}
	if strings.Contains(df, "BUILDPLATFORM") {
		t.Error("should not contain BUILDPLATFORM")
	}
}

func TestGenerateDockerfile_HasEntrypoint(t *testing.T) {
	df := GenerateDockerfile(nil, "v1.0.0")

	if !strings.Contains(df, "ENTRYPOINT") {
		t.Error("expected ENTRYPOINT in Dockerfile")
	}
	if !strings.Contains(df, "/entrypoint.sh") {
		t.Error("expected entrypoint.sh script")
	}
}

func TestGenerateDockerfile_HasClaudeUser(t *testing.T) {
	df := GenerateDockerfile(nil, "v1.0.0")

	if !strings.Contains(df, "adduser -D -s /bin/bash claude") {
		t.Error("expected claude user creation")
	}
}

func TestGenerateDockerfile_SkipUpdateEnvVar(t *testing.T) {
	df := GenerateDockerfile(nil, "v1.0.0")

	if !strings.Contains(df, "PLURAL_SKIP_UPDATE") {
		t.Error("expected PLURAL_SKIP_UPDATE env var check in entrypoint")
	}
}

func TestGenerateDockerfile_EntrypointExecsClaude(t *testing.T) {
	df := GenerateDockerfile(nil, "v1.0.0")

	// The entrypoint must exec `claude` as the command (not just "$@")
	// su-exec claude claude "$@" — first "claude" is the user, second is the command
	if !strings.Contains(df, "su-exec claude claude") {
		t.Error("entrypoint must exec 'claude' command via su-exec (su-exec claude claude \"$@\")")
	}
}

func TestGenerateDockerfile_SelectiveConfigCopy(t *testing.T) {
	df := GenerateDockerfile(nil, "v1.0.0")

	// Should copy specific files, not everything
	if !strings.Contains(df, "settings.json") {
		t.Error("entrypoint should copy settings.json from host config")
	}
	if !strings.Contains(df, ".credentials.json") {
		t.Error("entrypoint should copy .credentials.json from host config")
	}
}

func TestGenerateDockerfile_NodeBaseImage(t *testing.T) {
	// With no Node detected, should use default node version
	df := GenerateDockerfile(nil, "v1.0.0")
	if !strings.Contains(df, "FROM node:22-alpine") {
		t.Error("expected default node:22-alpine base")
	}

	// With Node 18 detected, should use that
	langs := []DetectedLang{{Language: LangNode, Version: "18"}}
	df = GenerateDockerfile(langs, "v1.0.0")
	if !strings.Contains(df, "FROM node:18-alpine") {
		t.Error("expected node:18-alpine base for Node.js 18")
	}
}

func TestReleaseArch(t *testing.T) {
	arch := releaseArch()
	switch runtime.GOARCH {
	case "amd64":
		if arch != "x86_64" {
			t.Errorf("expected x86_64 for amd64, got %s", arch)
		}
	case "arm64":
		if arch != "arm64" {
			t.Errorf("expected arm64 for arm64, got %s", arch)
		}
	default:
		if arch != runtime.GOARCH {
			t.Errorf("expected %s, got %s", runtime.GOARCH, arch)
		}
	}
}

func TestImageTag_Deterministic(t *testing.T) {
	df := GenerateDockerfile(nil, "v1.0.0")
	tag1 := ImageTag(df)
	tag2 := ImageTag(df)

	if tag1 != tag2 {
		t.Errorf("expected deterministic tags, got %s and %s", tag1, tag2)
	}
}

func TestImageTag_DifferentContent(t *testing.T) {
	df1 := GenerateDockerfile(nil, "v1.0.0")
	df2 := GenerateDockerfile([]DetectedLang{{Language: LangGo, Version: "1.22"}}, "v1.0.0")

	tag1 := ImageTag(df1)
	tag2 := ImageTag(df2)

	if tag1 == tag2 {
		t.Error("different Dockerfiles should produce different tags")
	}
}

func TestImageTag_Format(t *testing.T) {
	tag := ImageTag("test content")

	if !strings.HasPrefix(tag, "plural:") {
		t.Errorf("expected tag to start with 'plural:', got %s", tag)
	}

	// "plural:" + 12 hex chars
	parts := strings.SplitN(tag, ":", 2)
	if len(parts) != 2 {
		t.Fatalf("expected tag in format 'plural:<hash>', got %s", tag)
	}
	if len(parts[1]) != 12 {
		t.Errorf("expected 12-char hash suffix, got %d chars: %s", len(parts[1]), parts[1])
	}
}

func TestLangSummary(t *testing.T) {
	tests := []struct {
		name  string
		langs []DetectedLang
		want  string
	}{
		{
			name:  "empty",
			langs: nil,
			want:  "",
		},
		{
			name:  "single without version",
			langs: []DetectedLang{{Language: LangGo}},
			want:  "go",
		},
		{
			name:  "single with version",
			langs: []DetectedLang{{Language: LangGo, Version: "1.22"}},
			want:  "go@1.22",
		},
		{
			name: "multiple",
			langs: []DetectedLang{
				{Language: LangGo, Version: "1.22"},
				{Language: LangNode, Version: "20"},
			},
			want: "go@1.22,node@20",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := langSummary(tt.langs)
			if got != tt.want {
				t.Errorf("langSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLanguageInstallBlock(t *testing.T) {
	tests := []struct {
		lang    Language
		want    string
		notWant string
	}{
		{LangPython, "python3", ""},
		{LangRuby, "ruby-dev", ""},
		{LangRust, "cargo", ""},
		{LangJava, "openjdk17", ""},
		{LangNode, "", "nodejs"}, // Node is base image, no extra packages
		{LangGo, "", "golang"},   // Go uses builder stage, not apk
	}

	for _, tt := range tests {
		t.Run(string(tt.lang), func(t *testing.T) {
			block := languageInstallBlock(tt.lang)
			if tt.want != "" && !strings.Contains(block, tt.want) {
				t.Errorf("expected %q in block for %s, got %q", tt.want, tt.lang, block)
			}
			if tt.notWant != "" && strings.Contains(block, tt.notWant) {
				t.Errorf("unexpected %q in block for %s", tt.notWant, tt.lang)
			}
		})
	}
}

func TestPluralDownloadBlock(t *testing.T) {
	t.Run("dev version uses latest", func(t *testing.T) {
		block := pluralDownloadBlock("dev", "x86_64")
		if !strings.Contains(block, "/latest/download/") {
			t.Error("dev version should use /latest/download/")
		}
		if !strings.Contains(block, "x86_64") {
			t.Error("expected architecture in URL")
		}
	})

	t.Run("empty version uses latest", func(t *testing.T) {
		block := pluralDownloadBlock("", "arm64")
		if !strings.Contains(block, "/latest/download/") {
			t.Error("empty version should use /latest/download/")
		}
	})

	t.Run("release version uses exact URL", func(t *testing.T) {
		block := pluralDownloadBlock("v0.5.0", "x86_64")
		if !strings.Contains(block, "/download/v0.5.0/") {
			t.Error("release version should use exact version URL")
		}
		if strings.Contains(block, "/latest/download/") {
			t.Error("release version should not use /latest/download/")
		}
	})
}
