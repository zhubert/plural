// Package claudeauth provides helpers for querying the Claude CLI authentication status.
package claudeauth

import (
	"encoding/json"
	"os/exec"
)

// AuthStatus holds the parsed result of `claude auth status --output-format json`.
type AuthStatus struct {
	LoggedIn   bool   `json:"loggedIn"`
	AuthMethod string `json:"authMethod"`
	Email      string `json:"email"`
	OrgName    string `json:"orgName"`
}

// GetAuthStatus runs `claude auth status --output-format json` and returns the parsed result.
// It returns nil without an error when the output cannot be parsed, so callers can fail
// silently when the command is unavailable or outputs an unexpected format.
func GetAuthStatus() (*AuthStatus, error) {
	cmd := exec.Command("claude", "auth", "status", "--output-format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var status AuthStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, err
	}

	return &status, nil
}

// DisplayString returns a concise string suitable for display in the UI footer.
// Returns an empty string when there is nothing useful to show.
func (s *AuthStatus) DisplayString() string {
	if s == nil || !s.LoggedIn {
		return ""
	}
	if s.Email != "" {
		if s.OrgName != "" {
			return s.Email + " @ " + s.OrgName
		}
		return s.Email
	}
	if s.AuthMethod != "" {
		return s.AuthMethod
	}
	return ""
}
