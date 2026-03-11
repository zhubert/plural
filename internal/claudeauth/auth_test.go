package claudeauth

import (
	"encoding/json"
	"testing"
)

func TestAuthStatus_DisplayString(t *testing.T) {
	tests := []struct {
		name     string
		status   *AuthStatus
		expected string
	}{
		{
			name:     "nil status",
			status:   nil,
			expected: "",
		},
		{
			name:     "not logged in",
			status:   &AuthStatus{LoggedIn: false},
			expected: "",
		},
		{
			name:     "logged in with email only",
			status:   &AuthStatus{LoggedIn: true, Email: "user@example.com"},
			expected: "user@example.com",
		},
		{
			name:     "logged in with email and org",
			status:   &AuthStatus{LoggedIn: true, Email: "zack@planningcenter.com", OrgName: "Planning Center"},
			expected: "zack@planningcenter.com @ Planning Center",
		},
		{
			name:     "logged in with auth method only",
			status:   &AuthStatus{LoggedIn: true, AuthMethod: "claude.ai"},
			expected: "claude.ai",
		},
		{
			name:     "logged in but no useful info",
			status:   &AuthStatus{LoggedIn: true},
			expected: "",
		},
		{
			name:     "email takes priority over auth method",
			status:   &AuthStatus{LoggedIn: true, Email: "user@example.com", AuthMethod: "api-key"},
			expected: "user@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.DisplayString()
			if got != tt.expected {
				t.Errorf("DisplayString() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestAuthStatus_JSONParsing(t *testing.T) {
	tests := []struct {
		name       string
		jsonInput  string
		wantErr    bool
		wantLogin  bool
		wantEmail  string
		wantOrg    string
		wantMethod string
	}{
		{
			name:       "full valid JSON",
			jsonInput:  `{"loggedIn":true,"authMethod":"claude.ai","email":"zack@planningcenter.com","orgName":"Planning Center"}`,
			wantLogin:  true,
			wantEmail:  "zack@planningcenter.com",
			wantOrg:    "Planning Center",
			wantMethod: "claude.ai",
		},
		{
			name:      "not logged in",
			jsonInput: `{"loggedIn":false}`,
			wantLogin: false,
		},
		{
			name:      "missing fields defaults to zero values",
			jsonInput: `{"loggedIn":true}`,
			wantLogin: true,
		},
		{
			name:    "malformed JSON",
			jsonInput: `not valid json`,
			wantErr: true,
		},
		{
			name:    "empty input",
			jsonInput: ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var status AuthStatus
			err := json.Unmarshal([]byte(tt.jsonInput), &status)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if status.LoggedIn != tt.wantLogin {
				t.Errorf("LoggedIn = %v, want %v", status.LoggedIn, tt.wantLogin)
			}
			if status.Email != tt.wantEmail {
				t.Errorf("Email = %q, want %q", status.Email, tt.wantEmail)
			}
			if status.OrgName != tt.wantOrg {
				t.Errorf("OrgName = %q, want %q", status.OrgName, tt.wantOrg)
			}
			if status.AuthMethod != tt.wantMethod {
				t.Errorf("AuthMethod = %q, want %q", status.AuthMethod, tt.wantMethod)
			}
		})
	}
}
