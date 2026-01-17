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
		// Show the initial interface
		demo.Wait(1 * time.Second),

		// Select the session
		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Type a message
		demo.Type("Add a hello world function to main.go"),
		demo.Wait(300 * time.Millisecond),
		demo.Capture(), // Show the typed message

		// Send the message
		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(), // Show message sent (appears in chat)

		// Simulate Claude's streaming response
		demo.TextResponse(`I'll add a hello world function to main.go for you.

`+"```"+`go
func helloWorld() string {
    return "Hello, World!"
}
`+"```"+`

I've added the `+"`helloWorld`"+` function. Would you like me to also add a test for it?`),

		// Final pause to show completed response
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
		demo.Capture(),

		// Select first session
		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Send task to first session
		demo.Type("Add a /users endpoint"),
		demo.Wait(300 * time.Millisecond),
		demo.Capture(),

		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Claude responds
		demo.TextResponse("I'll create a /users endpoint with CRUD operations. Here's the implementation:\n\n```go\nfunc (h *Handler) GetUsers(w http.ResponseWriter, r *http.Request) {\n    users, err := h.store.ListUsers(r.Context())\n    if err != nil {\n        http.Error(w, err.Error(), 500)\n        return\n    }\n    json.NewEncoder(w).Encode(users)\n}\n```"),

		demo.Wait(800 * time.Millisecond),

		// Switch to second session
		demo.Key("tab"),
		demo.Wait(200 * time.Millisecond),
		demo.Key("down"),
		demo.Wait(200 * time.Millisecond),
		demo.Capture(),

		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Send task to second session
		demo.Type("Create a UserList component"),
		demo.Wait(300 * time.Millisecond),
		demo.Capture(),

		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Second session responds
		demo.TextResponse("I'll create a UserList React component:\n\n```tsx\nexport function UserList() {\n  const { data: users } = useQuery(['users'], fetchUsers);\n  return (\n    <table>\n      <thead><tr><th>Name</th><th>Email</th></tr></thead>\n      <tbody>\n        {users?.map(u => <tr key={u.id}><td>{u.name}</td><td>{u.email}</td></tr>)}\n      </tbody>\n    </table>\n  );\n}\n```"),

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
		demo.Capture(),

		// Select session
		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Request tests
		demo.Type("Run the test suite"),
		demo.Wait(300 * time.Millisecond),
		demo.Capture(),

		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Claude requests permission
		demo.Permission("Bash", "go test ./..."),
		demo.Wait(800 * time.Millisecond),

		// Approve with 'y'
		demo.Key("y"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Claude responds after permission
		demo.TextResponse(`Running tests...

`+"```"+`
ok  	github.com/user/myproject/pkg/api	0.042s
ok  	github.com/user/myproject/pkg/models	0.038s
ok  	github.com/user/myproject/pkg/utils	0.025s
`+"```"+`

All tests passed! The test suite completed successfully.`),

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
