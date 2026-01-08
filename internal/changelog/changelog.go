// Package changelog provides parsing and filtering of changelog entries.
// The changelog is embedded from the CHANGELOG.md file at the repo root.
package changelog

import (
	_ "embed"
	"regexp"
	"strconv"
	"strings"
)

//go:embed CHANGELOG.md
var Content string

// Entry represents a single version's changelog entry
type Entry struct {
	Version string
	Date    string
	Changes []string
}

// versionRegex matches version headers like "## v0.0.12 (2026-01-08)" or "## v0.0.12"
var versionRegex = regexp.MustCompile(`^##\s+v?(\d+\.\d+\.\d+)(?:\s+\(([^)]+)\))?`)

// Parse extracts changelog entries from markdown content
func Parse(content string) []Entry {
	var entries []Entry
	var current *Entry

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check for version header
		if matches := versionRegex.FindStringSubmatch(line); matches != nil {
			// Save previous entry
			if current != nil {
				entries = append(entries, *current)
			}
			current = &Entry{
				Version: matches[1],
				Date:    matches[2],
				Changes: []string{},
			}
			continue
		}

		// Check for bullet point (change item)
		if current != nil && strings.HasPrefix(line, "- ") {
			change := strings.TrimPrefix(line, "- ")
			current.Changes = append(current.Changes, change)
		}
	}

	// Don't forget the last entry
	if current != nil {
		entries = append(entries, *current)
	}

	return entries
}

// GetChangesSince returns all entries newer than lastSeen version.
// Entries are returned in reverse chronological order (newest first).
func GetChangesSince(lastSeen string, entries []Entry) []Entry {
	if lastSeen == "" {
		return entries
	}

	var result []Entry
	for _, entry := range entries {
		if CompareVersions(entry.Version, lastSeen) > 0 {
			result = append(result, entry)
		}
	}
	return result
}

// CompareVersions compares two semantic versions.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
func CompareVersions(a, b string) int {
	aParts := parseVersion(a)
	bParts := parseVersion(b)

	for i := 0; i < 3; i++ {
		if aParts[i] < bParts[i] {
			return -1
		}
		if aParts[i] > bParts[i] {
			return 1
		}
	}
	return 0
}

// parseVersion extracts [major, minor, patch] from a version string
func parseVersion(v string) [3]int {
	// Strip leading 'v' if present
	v = strings.TrimPrefix(v, "v")

	parts := strings.Split(v, ".")
	var result [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		result[i], _ = strconv.Atoi(parts[i])
	}
	return result
}
