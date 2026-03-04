// Package container provides auto-provisioning of Docker containers for Claude CLI sessions.
// It detects programming languages in a repository, generates appropriate Dockerfiles,
// and builds images with content-hash caching.
package container

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zhubert/plural/internal/logger"
)

// Language represents a detected programming language.
type Language string

const (
	LangGo     Language = "go"
	LangNode   Language = "node"
	LangPython Language = "python"
	LangRuby   Language = "ruby"
	LangRust   Language = "rust"
	LangJava   Language = "java"
)

// DetectedLang holds a detected language and its version (if determinable).
type DetectedLang struct {
	Language Language
	Version  string // e.g., "1.22", "20", "3.12"; empty if unknown
}

// markerFile maps a filename to the language it indicates.
var markerFiles = map[string]Language{
	"go.mod":         LangGo,
	"package.json":   LangNode,
	"requirements.txt": LangPython,
	"pyproject.toml":   LangPython,
	"Pipfile":          LangPython,
	"setup.py":        LangPython,
	"Gemfile":         LangRuby,
	"Cargo.toml":     LangRust,
	"pom.xml":        LangJava,
	"build.gradle":   LangJava,
}

// langOrder defines the deterministic sort order for detected languages.
var langOrder = map[Language]int{
	LangGo:     0,
	LangNode:   1,
	LangPython: 2,
	LangRuby:   3,
	LangRust:   4,
	LangJava:   5,
}

// Detect scans the given repository path for language marker files and returns
// the detected languages sorted in a deterministic order. Only local marker file
// detection is performed.
func Detect(ctx context.Context, repoPath string) []DetectedLang {
	log := logger.WithComponent("container")
	log.Debug("detecting languages", "path", repoPath)

	seen := make(map[Language]bool)
	var langs []DetectedLang

	for file, lang := range markerFiles {
		select {
		case <-ctx.Done():
			return langs
		default:
		}

		fullPath := filepath.Join(repoPath, file)
		if _, err := os.Stat(fullPath); err != nil {
			continue
		}

		if seen[lang] {
			continue
		}
		seen[lang] = true

		version := detectVersion(repoPath, lang)
		langs = append(langs, DetectedLang{Language: lang, Version: version})
		log.Debug("detected language", "lang", lang, "version", version)
	}

	// Deterministic sort
	sort.Slice(langs, func(i, j int) bool {
		return langOrder[langs[i].Language] < langOrder[langs[j].Language]
	})

	return langs
}

// detectVersion attempts to extract the version for a given language from its config files.
func detectVersion(repoPath string, lang Language) string {
	switch lang {
	case LangGo:
		return detectGoVersion(repoPath)
	case LangNode:
		return detectNodeVersion(repoPath)
	case LangPython:
		return detectPythonVersion(repoPath)
	case LangRuby:
		return detectRubyVersion(repoPath)
	default:
		return ""
	}
}

// detectGoVersion reads the Go version from go.mod.
// Returns the major.minor version (e.g., "1.22") or empty string.
func detectGoVersion(repoPath string) string {
	data, err := os.ReadFile(filepath.Join(repoPath, "go.mod"))
	if err != nil {
		return ""
	}

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if version, ok := strings.CutPrefix(line, "go "); ok {
			version = strings.TrimSpace(version)
			// Return major.minor only (strip patch if present)
			parts := strings.Split(version, ".")
			if len(parts) >= 2 {
				return parts[0] + "." + parts[1]
			}
			return version
		}
	}
	return ""
}

// detectNodeVersion reads the Node.js version from .nvmrc or package.json engines.
// Returns the major version (e.g., "20") or empty string.
func detectNodeVersion(repoPath string) string {
	// Try .nvmrc first
	data, err := os.ReadFile(filepath.Join(repoPath, ".nvmrc"))
	if err == nil {
		version := strings.TrimSpace(string(data))
		version = strings.TrimPrefix(version, "v")
		if parts := strings.Split(version, "."); len(parts) >= 1 && parts[0] != "" {
			return parts[0]
		}
	}

	// Try .node-version
	data, err = os.ReadFile(filepath.Join(repoPath, ".node-version"))
	if err == nil {
		version := strings.TrimSpace(string(data))
		version = strings.TrimPrefix(version, "v")
		if parts := strings.Split(version, "."); len(parts) >= 1 && parts[0] != "" {
			return parts[0]
		}
	}

	return ""
}

// detectPythonVersion reads the Python version from .python-version.
// Returns the major.minor version (e.g., "3.12") or empty string.
func detectPythonVersion(repoPath string) string {
	data, err := os.ReadFile(filepath.Join(repoPath, ".python-version"))
	if err != nil {
		return ""
	}

	version := strings.TrimSpace(string(data))
	parts := strings.Split(version, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return version
}

// detectRubyVersion reads the Ruby version from .ruby-version.
// Returns the version string (e.g., "3.2") or empty string.
func detectRubyVersion(repoPath string) string {
	data, err := os.ReadFile(filepath.Join(repoPath, ".ruby-version"))
	if err != nil {
		return ""
	}

	version := strings.TrimSpace(string(data))
	// Strip "ruby-" prefix if present (rbenv style)
	version = strings.TrimPrefix(version, "ruby-")
	parts := strings.Split(version, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return version
}
