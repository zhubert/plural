package scenarios

import (
	"time"

	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/demo"
	"github.com/zhubert/plural/internal/mcp"
)

// Comprehensive demonstrates a complete Plural workflow including:
// - Working with multiple sessions
// - Forking a session to explore alternatives
// - Claude asking questions
// - Permission requests
// - Merging work back
var Comprehensive = &demo.Scenario{
	Name:        "comprehensive",
	Description: "Full workflow: parallel sessions, forking, questions, permissions, and merging",
	Width:       120,
	Height:      40,
	Setup: &demo.ScenarioSetup{
		Repos: []string{"/home/user/webapp"},
		Sessions: []config.Session{
			{
				ID:        "session-main",
				RepoPath:  "/home/user/webapp",
				WorkTree:  "/home/user/.plural-worktrees/session-main",
				Branch:    "plural-auth-feature",
				Name:      "webapp/auth-feature",
				CreatedAt: time.Now().Add(-1 * time.Hour),
				Started:   true,
			},
			{
				ID:        "session-tests",
				RepoPath:  "/home/user/webapp",
				WorkTree:  "/home/user/.plural-worktrees/session-tests",
				Branch:    "plural-add-tests",
				Name:      "webapp/add-tests",
				CreatedAt: time.Now().Add(-30 * time.Minute),
				Started:   true,
			},
		},
		Focus: "sidebar",
	},
	Steps: []demo.Step{
		// === PART 1: Introduction and Setup ===
		demo.Wait(1500 * time.Millisecond),
		demo.Annotate("Plural: Manage multiple Claude sessions in parallel"),

		// Show we have two sessions
		demo.Wait(1 * time.Second),

		// === PART 2: Work on First Session ===
		demo.Annotate("Select the auth-feature session"),
		demo.Key("enter"),
		demo.Wait(600 * time.Millisecond),

		// Send a complex task
		demo.Annotate("Ask Claude to implement authentication"),
		demo.Type("Implement JWT authentication with refresh tokens"),
		demo.Wait(400 * time.Millisecond),
		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),

		// Claude responds with options - asking a question
		demo.Annotate("Claude asks for clarification"),
		demo.Question(
			mcp.Question{
				Question: "Which JWT library would you like me to use?",
				Header:   "Library",
				Options: []mcp.QuestionOption{
					{Label: "go-jose/v3", Description: "Feature-rich, supports JWE"},
					{Label: "golang-jwt/jwt", Description: "Simple, widely used"},
					{Label: "PASETO", Description: "Modern alternative to JWT"},
				},
			},
		),
		demo.Wait(1200 * time.Millisecond),

		// User selects an option
		demo.Annotate("Navigate with arrow keys, select with Enter"),
		demo.Key("down"),
		demo.Wait(300 * time.Millisecond),
		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),

		// Claude continues with implementation
		demo.StreamingTextResponse(`Great choice! I'll use golang-jwt/jwt for the implementation.

Let me create the authentication middleware:

`+"```"+`go
// internal/auth/jwt.go
package auth

import (
    "time"
    "github.com/golang-jwt/jwt/v5"
)

type Claims struct {
    UserID string `+"`"+`json:"user_id"`+"`"+`
    jwt.RegisteredClaims
}

func GenerateTokenPair(userID string) (string, string, error) {
    // Access token - short lived (15 min)
    accessClaims := Claims{
        UserID: userID,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
        },
    }
    // ... implementation continues
}
`+"```"+`

I have two approaches for storing refresh tokens:

1. **Database storage** - More secure, allows revocation
2. **Redis cache** - Faster, good for high-traffic apps

Which approach would you prefer?`, 15),

		demo.Wait(1 * time.Second),

		// === PART 3: Fork to Explore Options ===
		demo.Annotate("Fork session to explore both approaches (press 'f')"),
		demo.Key("tab"), // Go to sidebar
		demo.Wait(300 * time.Millisecond),
		demo.Key("f"), // Fork
		demo.Wait(800 * time.Millisecond),

		// Fork modal appears - toggle copy messages and confirm
		demo.Annotate("Copy conversation history to the fork"),
		demo.Key("space"), // Toggle copy messages
		demo.Wait(300 * time.Millisecond),
		demo.Key("enter"), // Confirm fork
		demo.Wait(800 * time.Millisecond),

		// Now we have a forked session - select it
		demo.Annotate("New forked session created"),
		demo.Wait(500 * time.Millisecond),
		demo.Key("enter"), // Select the new session
		demo.Wait(500 * time.Millisecond),

		// Ask the fork to implement Redis approach
		demo.Type("Let's go with the Redis approach for refresh tokens"),
		demo.Key("enter"),
		demo.Wait(400 * time.Millisecond),

		demo.StreamingTextResponse(`I'll implement the Redis-based refresh token storage:

`+"```"+`go
// internal/auth/refresh.go
package auth

import (
    "context"
    "time"
    "github.com/redis/go-redis/v9"
)

type RefreshTokenStore struct {
    client *redis.Client
}

func (s *RefreshTokenStore) Store(ctx context.Context, token, userID string) error {
    return s.client.Set(ctx, "refresh:"+token, userID, 7*24*time.Hour).Err()
}

func (s *RefreshTokenStore) Validate(ctx context.Context, token string) (string, error) {
    return s.client.Get(ctx, "refresh:"+token).Result()
}
`+"```"+`

Let me also add the Redis connection setup.`, 12),

		demo.Wait(800 * time.Millisecond),

		// === PART 4: Switch to Test Session ===
		demo.Annotate("Switch to test session while auth work continues"),
		demo.Key("tab"),
		demo.Wait(200 * time.Millisecond),
		demo.Key("down"),
		demo.Wait(200 * time.Millisecond),
		demo.Key("down"), // Navigate to tests session
		demo.Wait(200 * time.Millisecond),
		demo.Key("enter"),
		demo.Wait(600 * time.Millisecond),

		// Work on tests
		demo.Type("Add unit tests for the user service"),
		demo.Key("enter"),
		demo.Wait(400 * time.Millisecond),

		demo.StreamingTextResponse(`I'll create comprehensive tests for the user service:

`+"```"+`go
// internal/service/user_test.go
func TestUserService_Create(t *testing.T) {
    tests := []struct {
        name    string
        input   CreateUserInput
        wantErr bool
    }{
        {
            name:    "valid user",
            input:   CreateUserInput{Email: "test@example.com", Name: "Test"},
            wantErr: false,
        },
        {
            name:    "invalid email",
            input:   CreateUserInput{Email: "invalid", Name: "Test"},
            wantErr: true,
        },
    }
    // ... test implementation
}
`+"```"+`

Should I run these tests now?`, 10),

		demo.Wait(800 * time.Millisecond),

		// Claude requests permission to run tests
		demo.Type("Yes, run them"),
		demo.Key("enter"),
		demo.Wait(400 * time.Millisecond),

		demo.Annotate("Claude requests permission to execute tests"),
		demo.Permission("Bash", "go test ./internal/service/... -v"),
		demo.Wait(1 * time.Second),

		// Allow the permission
		demo.Annotate("Press 'a' to always allow test commands"),
		demo.Key("a"),
		demo.Wait(400 * time.Millisecond),

		demo.StreamingTextResponse(`Running tests...

`+"```"+`
=== RUN   TestUserService_Create
=== RUN   TestUserService_Create/valid_user
=== RUN   TestUserService_Create/invalid_email
--- PASS: TestUserService_Create (0.02s)
    --- PASS: TestUserService_Create/valid_user (0.01s)
    --- PASS: TestUserService_Create/invalid_email (0.01s)
PASS
ok  	github.com/user/webapp/internal/service	0.156s
`+"```"+`

All tests passed! The user service is working correctly.`, 8),

		demo.Wait(1 * time.Second),

		// === PART 5: View Changes and Merge ===
		demo.Annotate("View changes made in this session (press 'v')"),
		demo.Key("tab"),
		demo.Wait(200 * time.Millisecond),
		demo.Key("v"),
		demo.Wait(1 * time.Second),

		// Exit view changes
		demo.Key("escape"),
		demo.Wait(500 * time.Millisecond),

		// Initiate merge
		demo.Annotate("Merge changes - create a PR (press 'm')"),
		demo.Key("m"),
		demo.Wait(800 * time.Millisecond),

		// Merge modal - select PR option
		demo.Annotate("Choose to create a Pull Request"),
		demo.Key("down"), // Select PR option
		demo.Wait(300 * time.Millisecond),
		demo.Key("enter"),
		demo.Wait(1 * time.Second),

		// === PART 6: Summary ===
		demo.Annotate("Plural: Parallel Claude sessions with full git workflow"),
		demo.Wait(2 * time.Second),
	},
}
