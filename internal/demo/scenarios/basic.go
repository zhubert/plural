// Package scenarios contains built-in demo scenarios for Plural.
package scenarios

import (
	"time"

	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/demo"
)

// Basic demonstrates the core Plural workflow:
// - Selecting a session
// - Sending a message to Claude
// - Receiving a response
var Basic = &demo.Scenario{
	Name:        "basic",
	Description: "Basic session workflow - select, send message, receive response",
	Width:       120,
	Height:      40,
	Setup: &demo.ScenarioSetup{
		Repos: []string{"/home/user/myproject"},
		Sessions: []config.Session{
			{
				ID:        "demo-session-1",
				RepoPath:  "/home/user/myproject",
				WorkTree:  "/home/user/.plural-worktrees/demo-session-1",
				Branch:    "plural-feature",
				Name:      "myproject/feature",
				CreatedAt: time.Now(),
				Started:   true,
			},
		},
		Focus: "sidebar",
	},
	Steps: []demo.Step{
		// Initial pause to show the interface
		demo.Wait(1 * time.Second),

		// Annotate what's happening
		demo.Annotate("Press Enter to select the session"),
		demo.KeyWithDesc("enter", "Select session"),
		demo.Wait(500 * time.Millisecond),

		// Type a message
		demo.Annotate("Type a message to Claude"),
		demo.Type("Add a hello world function to main.go"),
		demo.Wait(500 * time.Millisecond),

		// Send the message
		demo.Annotate("Press Enter to send"),
		demo.Key("enter"),
		demo.Wait(300 * time.Millisecond),

		// Simulate Claude's streaming response
		demo.Annotate("Claude responds..."),
		demo.StreamingTextResponse(`I'll add a hello world function to main.go for you.

`+"```"+`go
func helloWorld() string {
    return "Hello, World!"
}
`+"```"+`

I've added the `+"`helloWorld`"+` function. Would you like me to also add a test for it?`, 10),

		// Final pause
		demo.Wait(2 * time.Second),
	},
}

// Parallel demonstrates running multiple sessions in parallel:
// - Starting with two sessions
// - Working on different tasks simultaneously
var Parallel = &demo.Scenario{
	Name:        "parallel",
	Description: "Parallel sessions - work on multiple tasks simultaneously",
	Width:       120,
	Height:      40,
	Setup: &demo.ScenarioSetup{
		Repos: []string{"/home/user/myproject"},
		Sessions: []config.Session{
			{
				ID:        "session-api",
				RepoPath:  "/home/user/myproject",
				WorkTree:  "/home/user/.plural-worktrees/session-api",
				Branch:    "plural-api",
				Name:      "myproject/api",
				CreatedAt: time.Now(),
				Started:   true,
			},
			{
				ID:        "session-ui",
				RepoPath:  "/home/user/myproject",
				WorkTree:  "/home/user/.plural-worktrees/session-ui",
				Branch:    "plural-ui",
				Name:      "myproject/ui",
				CreatedAt: time.Now(),
				Started:   true,
			},
		},
		Focus: "sidebar",
	},
	Steps: []demo.Step{
		demo.Wait(1 * time.Second),

		// Select first session
		demo.Annotate("Select the API session"),
		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),

		// Send task to first session
		demo.Type("Add a /users endpoint"),
		demo.Key("enter"),
		demo.Wait(300 * time.Millisecond),

		// Claude starts responding
		demo.StreamingTextResponse("I'll create a /users endpoint with CRUD operations...", 8),

		// Switch to second session while first is "working"
		demo.Annotate("Switch to UI session with Tab + arrow keys"),
		demo.Key("tab"),
		demo.Wait(200 * time.Millisecond),
		demo.Key("down"),
		demo.Wait(200 * time.Millisecond),
		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),

		// Send task to second session
		demo.Type("Create a UserList component"),
		demo.Key("enter"),
		demo.Wait(300 * time.Millisecond),

		// Second session responds
		demo.StreamingTextResponse("I'll create a UserList React component that displays users in a table...", 8),

		demo.Wait(2 * time.Second),
	},
}

// Permission demonstrates the permission system:
// - Claude requesting permission to run a command
// - User approving/denying
var Permission = &demo.Scenario{
	Name:        "permission",
	Description: "Permission system - approve or deny Claude's tool requests",
	Width:       120,
	Height:      40,
	Setup: &demo.ScenarioSetup{
		Repos: []string{"/home/user/myproject"},
		Sessions: []config.Session{
			{
				ID:        "demo-session",
				RepoPath:  "/home/user/myproject",
				WorkTree:  "/home/user/.plural-worktrees/demo-session",
				Branch:    "plural-tests",
				Name:      "myproject/tests",
				CreatedAt: time.Now(),
				Started:   true,
			},
		},
		Focus: "sidebar",
	},
	Steps: []demo.Step{
		demo.Wait(1 * time.Second),

		// Select session
		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),

		// Request tests
		demo.Type("Run the test suite"),
		demo.Key("enter"),
		demo.Wait(300 * time.Millisecond),

		// Claude requests permission
		demo.Annotate("Claude requests permission to run a command"),
		demo.Permission("Bash", "go test ./..."),
		demo.Wait(1 * time.Second),

		// Approve with 'y'
		demo.Annotate("Press 'y' to allow, 'n' to deny, 'a' to always allow"),
		demo.Key("y"),
		demo.Wait(300 * time.Millisecond),

		// Claude responds after permission
		demo.StreamingTextResponse(`Running tests...

`+"```"+`
ok  	github.com/user/myproject/pkg/api	0.042s
ok  	github.com/user/myproject/pkg/models	0.038s
ok  	github.com/user/myproject/pkg/utils	0.025s
`+"```"+`

All tests passed! The test suite completed successfully.`, 10),

		demo.Wait(2 * time.Second),
	},
}

// All returns all built-in scenarios.
func All() []*demo.Scenario {
	return []*demo.Scenario{
		Basic,
		Parallel,
		Permission,
		Comprehensive,
	}
}

// Get returns a scenario by name, or nil if not found.
func Get(name string) *demo.Scenario {
	for _, s := range All() {
		if s.Name == name {
			return s
		}
	}
	return nil
}
