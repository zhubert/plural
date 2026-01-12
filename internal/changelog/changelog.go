// Package changelog fetches release information from GitHub.
package changelog

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// GitHub API endpoint for releases
	releasesURL = "https://api.github.com/repos/zhubert/plural/releases"

	// Request timeout
	timeout = 10 * time.Second
)

// Entry represents a single version's changelog entry
type Entry struct {
	Version string
	Date    string
	Changes []string
}

// githubRelease represents the GitHub API response for a release
type githubRelease struct {
	TagName     string `json:"tag_name"`
	PublishedAt string `json:"published_at"`
	Body        string `json:"body"`
}

// FetchReleases fetches release information from GitHub.
// Returns entries in reverse chronological order (newest first).
func FetchReleases() ([]Entry, error) {
	client := &http.Client{Timeout: timeout}

	req, err := http.NewRequest("GET", releasesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "plural")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	entries := make([]Entry, 0, len(releases))
	for _, r := range releases {
		version := strings.TrimPrefix(r.TagName, "v")
		date := parseDate(r.PublishedAt)
		changes := parseBody(r.Body)

		entries = append(entries, Entry{
			Version: version,
			Date:    date,
			Changes: changes,
		})
	}

	return entries, nil
}

// parseDate extracts YYYY-MM-DD from an ISO 8601 timestamp
func parseDate(timestamp string) string {
	if len(timestamp) >= 10 {
		return timestamp[:10]
	}
	return timestamp
}

// parseBody extracts bullet points from release body markdown
func parseBody(body string) []string {
	var changes []string
	lines := strings.Split(body, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Match lines starting with "- " or "* "
		if strings.HasPrefix(line, "- ") {
			changes = append(changes, strings.TrimPrefix(line, "- "))
		} else if strings.HasPrefix(line, "* ") {
			changes = append(changes, strings.TrimPrefix(line, "* "))
		}
	}

	return changes
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
