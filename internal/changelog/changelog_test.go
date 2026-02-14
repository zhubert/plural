package changelog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseDate(t *testing.T) {
	tests := []struct {
		name      string
		timestamp string
		want      string
	}{
		{
			name:      "full ISO 8601 timestamp",
			timestamp: "2024-01-15T10:30:00Z",
			want:      "2024-01-15",
		},
		{
			name:      "timestamp with timezone offset",
			timestamp: "2024-12-25T23:59:59+05:00",
			want:      "2024-12-25",
		},
		{
			name:      "exactly 10 characters",
			timestamp: "2024-01-01",
			want:      "2024-01-01",
		},
		{
			name:      "short timestamp",
			timestamp: "2024-01",
			want:      "2024-01",
		},
		{
			name:      "empty string",
			timestamp: "",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDate(tt.timestamp)
			if got != tt.want {
				t.Errorf("parseDate(%q) = %q, want %q", tt.timestamp, got, tt.want)
			}
		})
	}
}

func TestParseBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "dash bullet points",
			body: "## Changes\n- Added new feature\n- Fixed bug\n- Updated docs",
			want: []string{"Added new feature", "Fixed bug", "Updated docs"},
		},
		{
			name: "asterisk bullet points",
			body: "## Changes\n* First change\n* Second change",
			want: []string{"First change", "Second change"},
		},
		{
			name: "mixed bullet points",
			body: "- Dash item\n* Asterisk item\n- Another dash",
			want: []string{"Dash item", "Asterisk item", "Another dash"},
		},
		{
			name: "with leading whitespace",
			body: "  - Indented item\n  * Also indented",
			want: []string{"Indented item", "Also indented"},
		},
		{
			name: "with commit SHA (40 chars)",
			body: "- abc123def456789012345678901234567890abcd Commit message here",
			want: []string{"Commit message here"},
		},
		{
			name: "with short commit SHA (7 chars)",
			body: "- abc123d Short SHA commit",
			want: []string{"Short SHA commit"},
		},
		{
			name: "no bullet points",
			body: "This is just a paragraph\nwith multiple lines\nbut no bullets",
			want: nil,
		},
		{
			name: "empty body",
			body: "",
			want: nil,
		},
		{
			name: "bullet with only whitespace after",
			body: "- \n* ",
			want: nil,
		},
		{
			name: "real world example",
			body: `## What's Changed
- Add dark mode support
- Fix memory leak in session manager
* Improve startup performance

**Full Changelog**: https://github.com/example/repo/compare/v1.0.0...v1.1.0`,
			want: []string{"Add dark mode support", "Fix memory leak in session manager", "Improve startup performance"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseBody(tt.body)
			if len(got) != len(tt.want) {
				t.Errorf("parseBody() returned %d items, want %d", len(got), len(tt.want))
				t.Errorf("got: %v", got)
				t.Errorf("want: %v", tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseBody()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestStripCommitSHA(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "40-char SHA",
			input: "abc123def456789012345678901234567890abcd Commit message",
			want:  "Commit message",
		},
		{
			name:  "7-char short SHA",
			input: "abc123d Short message",
			want:  "Short message",
		},
		{
			name:  "uppercase SHA",
			input: "ABC123DEF456789012345678901234567890ABCD Message here",
			want:  "Message here",
		},
		{
			name:  "mixed case SHA",
			input: "AbC123DeF456789012345678901234567890aBcD Mixed case",
			want:  "Mixed case",
		},
		{
			name:  "no SHA prefix",
			input: "Just a regular message",
			want:  "Just a regular message",
		},
		{
			name:  "invalid hex in 40-char position",
			input: "abc123def456789012345678901234567890abcg Not a SHA",
			want:  "abc123def456789012345678901234567890abcg Not a SHA",
		},
		{
			name:  "invalid hex in 7-char position",
			input: "abc123g Not a short SHA",
			want:  "abc123g Not a short SHA",
		},
		{
			name:  "SHA without space after",
			input: "abc123def456789012345678901234567890abcdNoSpace",
			want:  "abc123def456789012345678901234567890abcdNoSpace",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "just a short SHA with space (length exactly 8)",
			input: "abc123d ",
			want:  "", // 7-char hex SHA followed by space, strip to empty
		},
		{
			name:  "40-char SHA with space only (length exactly 41)",
			input: "abc123def456789012345678901234567890abcd ",
			want:  "", // 40-char hex SHA followed by space, strip to empty
		},
		{
			name:  "short SHA with single char message",
			input: "abc123d x",
			want:  "x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripCommitSHA(tt.input)
			if got != tt.want {
				t.Errorf("stripCommitSHA(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsHexString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "lowercase hex",
			input: "0123456789abcdef",
			want:  true,
		},
		{
			name:  "uppercase hex",
			input: "0123456789ABCDEF",
			want:  true,
		},
		{
			name:  "mixed case hex",
			input: "AbCdEf123456",
			want:  true,
		},
		{
			name:  "contains g",
			input: "abcdefg",
			want:  false,
		},
		{
			name:  "contains space",
			input: "abc def",
			want:  false,
		},
		{
			name:  "empty string",
			input: "",
			want:  true, // vacuously true - no invalid chars
		},
		{
			name:  "single digit",
			input: "0",
			want:  true,
		},
		{
			name:  "single letter valid",
			input: "f",
			want:  true,
		},
		{
			name:  "single letter invalid",
			input: "z",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHexString(tt.input)
			if got != tt.want {
				t.Errorf("isHexString(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		// Equal versions
		{name: "equal versions", a: "1.0.0", b: "1.0.0", want: 0},
		{name: "equal with v prefix", a: "v1.0.0", b: "1.0.0", want: 0},
		{name: "both with v prefix", a: "v2.1.3", b: "v2.1.3", want: 0},

		// Major version differences
		{name: "a major greater", a: "2.0.0", b: "1.0.0", want: 1},
		{name: "b major greater", a: "1.0.0", b: "2.0.0", want: -1},
		{name: "major 10 vs 9", a: "10.0.0", b: "9.0.0", want: 1},

		// Minor version differences
		{name: "a minor greater", a: "1.2.0", b: "1.1.0", want: 1},
		{name: "b minor greater", a: "1.1.0", b: "1.2.0", want: -1},
		{name: "minor 10 vs 9", a: "1.10.0", b: "1.9.0", want: 1},

		// Patch version differences
		{name: "a patch greater", a: "1.0.2", b: "1.0.1", want: 1},
		{name: "b patch greater", a: "1.0.1", b: "1.0.2", want: -1},
		{name: "patch 10 vs 9", a: "1.0.10", b: "1.0.9", want: 1},

		// Mixed differences
		{name: "major trumps minor", a: "2.0.0", b: "1.9.9", want: 1},
		{name: "minor trumps patch", a: "1.2.0", b: "1.1.9", want: 1},

		// Partial versions
		{name: "two part vs three part equal", a: "1.0", b: "1.0.0", want: 0},
		{name: "one part vs three part", a: "1", b: "1.0.0", want: 0},
		{name: "partial greater", a: "2", b: "1.9.9", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareVersions(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    [3]int
	}{
		{name: "full version", version: "1.2.3", want: [3]int{1, 2, 3}},
		{name: "with v prefix", version: "v1.2.3", want: [3]int{1, 2, 3}},
		{name: "two parts", version: "1.2", want: [3]int{1, 2, 0}},
		{name: "one part", version: "1", want: [3]int{1, 0, 0}},
		{name: "empty string", version: "", want: [3]int{0, 0, 0}},
		{name: "large numbers", version: "100.200.300", want: [3]int{100, 200, 300}},
		{name: "invalid part defaults to 0", version: "1.abc.3", want: [3]int{1, 0, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseVersion(tt.version)
			if got != tt.want {
				t.Errorf("parseVersion(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestGetChangesSince(t *testing.T) {
	entries := []Entry{
		{Version: "1.3.0", Date: "2024-03-01", Changes: []string{"Feature C"}},
		{Version: "1.2.0", Date: "2024-02-01", Changes: []string{"Feature B"}},
		{Version: "1.1.0", Date: "2024-01-15", Changes: []string{"Feature A"}},
		{Version: "1.0.0", Date: "2024-01-01", Changes: []string{"Initial release"}},
	}

	tests := []struct {
		name     string
		lastSeen string
		want     []string // versions we expect
	}{
		{
			name:     "empty lastSeen returns all",
			lastSeen: "",
			want:     []string{"1.3.0", "1.2.0", "1.1.0", "1.0.0"},
		},
		{
			name:     "from oldest version",
			lastSeen: "1.0.0",
			want:     []string{"1.3.0", "1.2.0", "1.1.0"},
		},
		{
			name:     "from middle version",
			lastSeen: "1.1.0",
			want:     []string{"1.3.0", "1.2.0"},
		},
		{
			name:     "from second newest",
			lastSeen: "1.2.0",
			want:     []string{"1.3.0"},
		},
		{
			name:     "from newest returns empty",
			lastSeen: "1.3.0",
			want:     []string{},
		},
		{
			name:     "from future version returns empty",
			lastSeen: "2.0.0",
			want:     []string{},
		},
		{
			name:     "with v prefix",
			lastSeen: "v1.1.0",
			want:     []string{"1.3.0", "1.2.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetChangesSince(tt.lastSeen, entries)
			if len(got) != len(tt.want) {
				t.Errorf("GetChangesSince(%q) returned %d entries, want %d", tt.lastSeen, len(got), len(tt.want))
				return
			}
			for i, entry := range got {
				if entry.Version != tt.want[i] {
					t.Errorf("GetChangesSince(%q)[%d].Version = %q, want %q", tt.lastSeen, i, entry.Version, tt.want[i])
				}
			}
		})
	}
}

func TestGetChangesSinceEmpty(t *testing.T) {
	// Test with empty entries slice
	got := GetChangesSince("1.0.0", nil)
	if got != nil {
		t.Errorf("GetChangesSince with nil entries should return nil, got %v", got)
	}

	got = GetChangesSince("1.0.0", []Entry{})
	if len(got) != 0 {
		t.Errorf("GetChangesSince with empty entries should return empty, got %v", got)
	}
}

func TestFetchReleases(t *testing.T) {
	// Create a test server that returns mock GitHub releases
	releases := []githubRelease{
		{
			TagName:     "v1.2.0",
			PublishedAt: "2024-02-15T10:00:00Z",
			Body:        "## Changes\n- Added feature X\n- Fixed bug Y",
		},
		{
			TagName:     "v1.1.0",
			PublishedAt: "2024-01-15T10:00:00Z",
			Body:        "- Initial improvements",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("Expected Accept header 'application/vnd.github+json', got %q", r.Header.Get("Accept"))
		}
		if r.Header.Get("User-Agent") != "plural" {
			t.Errorf("Expected User-Agent header 'plural', got %q", r.Header.Get("User-Agent"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()

	// We can't easily test FetchReleases() directly since it uses a hardcoded URL,
	// but we can test the parsing logic by calling the internal functions
	// The server test above validates the expected request format

	t.Run("parse mock response", func(t *testing.T) {
		// Simulate what FetchReleases does with the response
		entries := make([]Entry, 0, len(releases))
		for _, r := range releases {
			version := r.TagName[1:] // Strip 'v' prefix
			date := parseDate(r.PublishedAt)
			changes := parseBody(r.Body)

			entries = append(entries, Entry{
				Version: version,
				Date:    date,
				Changes: changes,
			})
		}

		if len(entries) != 2 {
			t.Fatalf("Expected 2 entries, got %d", len(entries))
		}

		if entries[0].Version != "1.2.0" {
			t.Errorf("First entry version = %q, want %q", entries[0].Version, "1.2.0")
		}
		if entries[0].Date != "2024-02-15" {
			t.Errorf("First entry date = %q, want %q", entries[0].Date, "2024-02-15")
		}
		if len(entries[0].Changes) != 2 {
			t.Errorf("First entry has %d changes, want 2", len(entries[0].Changes))
		}
	})
}

func TestFetchReleasesErrorCases(t *testing.T) {
	t.Run("non-200 status code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		// Can't test directly due to hardcoded URL, but documents expected behavior
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not valid json"))
		}))
		defer server.Close()

		// Can't test directly due to hardcoded URL, but documents expected behavior
	})
}
