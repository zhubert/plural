package scenarios

import (
	"time"

	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/demo"
)

// Comprehensive demonstrates the core value of Plural: working on multiple
// sessions concurrently. The flow is:
// 1. Start a long-running task in the first session
// 2. While that's processing (spinner showing), switch to second session
// 3. Work interactively with Claude in the second session
// 4. Switch back to see the first session's progress
var Comprehensive = &demo.Scenario{
	Name:        "comprehensive",
	Description: "Parallel sessions: start long task, work in another session while waiting",
	Width:       120,
	Height:      40,
	Setup: &demo.ScenarioSetup{
		Repos: []string{"/home/user/webapp"},
		Sessions: []config.Session{
			{
				ID:        "session-refactor",
				RepoPath:  "/home/user/webapp",
				WorkTree:  "/home/user/.plural-worktrees/session-refactor",
				Branch:    "plural-refactor",
				Name:      "refactor",
				CreatedAt: time.Now().Add(-1 * time.Hour),
				Started:   true,
			},
			{
				ID:        "session-bugfix",
				RepoPath:  "/home/user/webapp",
				WorkTree:  "/home/user/.plural-worktrees/session-bugfix",
				Branch:    "plural-bugfix",
				Name:      "bugfix",
				CreatedAt: time.Now().Add(-30 * time.Minute),
				Started:   true,
			},
		},
		Focus: "sidebar",
	},
	Steps: []demo.Step{
		// === Initial view ===
		demo.Wait(1 * time.Second),
		demo.Capture(),

		// === Start long-running task in first session ===
		demo.Key("enter"), // Select refactor session
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Type a complex task that would take a while
		demo.Type("Refactor the authentication module to use dependency injection"),
		demo.Wait(300 * time.Millisecond),
		demo.Capture(),

		demo.Key("enter"), // Send the message
		demo.Wait(300 * time.Millisecond),

		// Start streaming but DON'T complete - this leaves the session "processing"
		demo.StartStreaming("I'll refactor the authentication module to use dependency injection. This will involve:\n\n1. Creating interfaces for dependencies\n2. "),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(), // Shows spinner and partial response

		// === Switch to second session while first is still processing ===
		demo.Key("tab"), // Go to sidebar
		demo.Wait(200 * time.Millisecond),
		demo.Key("down"), // Navigate to bugfix session
		demo.Wait(200 * time.Millisecond),
		demo.Capture(), // Shows first session still has spinner in sidebar

		demo.Key("enter"), // Select bugfix session
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// === Work interactively in second session ===
		demo.Type("Fix the null pointer in user validation"),
		demo.Wait(300 * time.Millisecond),
		demo.Capture(),

		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),

		// This session gets a quick response
		demo.TextResponse(`Found the issue! The validation function doesn't check for nil user before accessing fields.

Here's the fix:

` + "```" + `go
func ValidateUser(user *User) error {
    if user == nil {
        return errors.New("user cannot be nil")
    }
    if user.Email == "" {
        return errors.New("email is required")
    }
    return nil
}
` + "```" + `

The fix adds a nil check at the start. Want me to add tests for this?`),

		demo.Wait(1 * time.Second),

		// Quick follow-up
		demo.Type("Yes, add tests"),
		demo.Wait(300 * time.Millisecond),
		demo.Capture(),

		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),

		demo.TextResponse(`Added test cases:

` + "```" + `go
func TestValidateUser(t *testing.T) {
    tests := []struct {
        name    string
        user    *User
        wantErr bool
    }{
        {"nil user", nil, true},
        {"empty email", &User{}, true},
        {"valid user", &User{Email: "test@example.com"}, false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateUser(tt.user)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateUser() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
` + "```" + `

All tests pass!`),

		demo.Wait(1 * time.Second),

		// === Switch back to first session to see progress ===
		demo.Key("tab"), // Go to sidebar
		demo.Wait(200 * time.Millisecond),
		demo.Key("up"), // Navigate back to refactor session
		demo.Wait(200 * time.Millisecond),
		demo.Capture(), // First session should still show spinner

		demo.Key("enter"), // Select refactor session
		demo.Wait(500 * time.Millisecond),
		demo.Capture(), // Shows the partial streaming content

		// Final pause
		demo.Wait(2 * time.Second),
	},
}
