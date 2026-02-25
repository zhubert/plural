// Package cli provides utilities for CLI tool management and validation.
package cli

import (
	"fmt"
	"os/exec"
	"strings"
)

// Prerequisite represents a required CLI tool
type Prerequisite struct {
	Name        string // Command name (e.g., "claude", "git")
	Required    bool   // Whether the tool is required to run the app
	Description string // Human-readable description
	InstallURL  string // URL for installation instructions
}

// DefaultPrerequisites returns the list of CLI tools needed by Plural
func DefaultPrerequisites() []Prerequisite {
	return []Prerequisite{
		{
			Name:        "claude",
			Required:    true,
			Description: "Claude Code CLI",
			InstallURL:  "https://claude.ai/code",
		},
		{
			Name:        "git",
			Required:    true,
			Description: "Git version control",
			InstallURL:  "https://git-scm.com/downloads",
		},
		{
			Name:        "gh",
			Required:    false, // Only needed for PR creation
			Description: "GitHub CLI (optional, for PR creation)",
			InstallURL:  "https://cli.github.com",
		},
	}
}

// CheckResult contains the result of checking a prerequisite
type CheckResult struct {
	Prerequisite Prerequisite
	Found        bool
	Path         string // Path to the executable if found
	Version      string // Version string if available
	Error        error
}

// Check verifies that a CLI tool is available in PATH
func Check(prereq Prerequisite) CheckResult {
	result := CheckResult{Prerequisite: prereq}

	path, err := exec.LookPath(prereq.Name)
	if err != nil {
		result.Error = fmt.Errorf("%s not found in PATH", prereq.Name)
		return result
	}

	result.Found = true
	result.Path = path

	// Try to get version
	version := getVersion(prereq.Name)
	if version != "" {
		result.Version = version
	}

	return result
}

// CheckAll verifies all prerequisites and returns results
func CheckAll(prereqs []Prerequisite) []CheckResult {
	results := make([]CheckResult, len(prereqs))
	for i, prereq := range prereqs {
		results[i] = Check(prereq)
	}
	return results
}

// ValidateRequired checks that all required prerequisites are met
// Returns nil if all required tools are found, otherwise returns an error
// describing what's missing
func ValidateRequired(prereqs []Prerequisite) error {
	var missing []string

	for _, prereq := range prereqs {
		if !prereq.Required {
			continue
		}
		result := Check(prereq)
		if !result.Found {
			missing = append(missing, fmt.Sprintf("  - %s (%s)\n    Install: %s",
				prereq.Name, prereq.Description, prereq.InstallURL))
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required CLI tools:\n%s", strings.Join(missing, "\n"))
	}

	return nil
}

// getVersion attempts to get the version of a CLI tool
func getVersion(name string) string {
	// Different tools use different version flags
	versionFlags := []string{"--version", "-v", "version"}

	for _, flag := range versionFlags {
		cmd := exec.Command(name, flag)
		output, err := cmd.Output()
		if err == nil {
			// Return first line of output, trimmed
			lines := strings.Split(string(output), "\n")
			if len(lines) > 0 {
				version := strings.TrimSpace(lines[0])
				// Limit length to avoid overly long version strings
				if len(version) > 100 {
					version = version[:100] + "..."
				}
				return version
			}
		}
	}

	return ""
}

// FormatCheckResults formats check results for display
func FormatCheckResults(results []CheckResult) string {
	var sb strings.Builder

	sb.WriteString("CLI Prerequisites:\n")
	for _, r := range results {
		status := "✓"
		if !r.Found {
			if r.Prerequisite.Required {
				status = "✗"
			} else {
				status = "○"
			}
		}

		sb.WriteString(fmt.Sprintf("  %s %s", status, r.Prerequisite.Name))
		if r.Found && r.Version != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", r.Version))
		} else if !r.Found {
			if r.Prerequisite.Required {
				sb.WriteString(" [REQUIRED]")
			} else {
				sb.WriteString(" [optional]")
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
