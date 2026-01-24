// Package scenarios contains built-in demo scenarios for Plural.
package scenarios

import (
	"time"

	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/demo"
	"github.com/zhubert/plural/internal/mcp"
)

// Overview demonstrates all major Plural features in a cohesive workflow:
// - Multi-repo, multi-session workspace (parallel work)
// - Plan approval with allowed prompts
// - Todo list sidebar showing task progress
// - Tool use rollups for multiple operations
// - Question prompts for user input
// - Permission requests
// - Explore options workflow (fork into parallel sessions)
// - Reviewing child sessions
// - Merging and creating PRs
var Overview = &demo.Scenario{
	Name:        "overview",
	Description: "Complete Plural overview - parallel sessions, forking, merging, Claude integration",
	Width:       120,
	Height:      40,
	Setup: &demo.ScenarioSetup{
		Repos: []string{"/home/user/webapp", "/home/user/api-service", "/home/user/mobile-app"},
		Sessions: []config.Session{
			// Main webapp sessions
			{
				ID:        "session-auth",
				RepoPath:  "/home/user/webapp",
				WorkTree:  "/home/user/.plural-worktrees/session-auth",
				Branch:    "plural-user-auth",
				Name:      "user-authentication",
				CreatedAt: time.Now().Add(-1 * time.Hour),
				Started:   true,
			},
			{
				ID:        "session-dashboard",
				RepoPath:  "/home/user/webapp",
				WorkTree:  "/home/user/.plural-worktrees/session-dashboard",
				Branch:    "plural-dashboard",
				Name:      "dashboard-redesign",
				CreatedAt: time.Now().Add(-3 * time.Hour),
				Started:   true,
			},
			{
				ID:        "session-perf",
				RepoPath:  "/home/user/webapp",
				WorkTree:  "/home/user/.plural-worktrees/session-perf",
				Branch:    "plural-performance",
				Name:      "performance-audit",
				CreatedAt: time.Now().Add(-24 * time.Hour),
				Started:   true,
			},
			// API service sessions
			{
				ID:        "session-graphql",
				RepoPath:  "/home/user/api-service",
				WorkTree:  "/home/user/.plural-worktrees/session-graphql",
				Branch:    "plural-graphql",
				Name:      "graphql-migration",
				CreatedAt: time.Now().Add(-2 * time.Hour),
				Started:   true,
			},
			{
				ID:        "session-caching",
				RepoPath:  "/home/user/api-service",
				WorkTree:  "/home/user/.plural-worktrees/session-caching",
				Branch:    "plural-redis",
				Name:      "redis-caching",
				CreatedAt: time.Now().Add(-48 * time.Hour),
				Started:   true,
			},
			// Mobile app sessions
			{
				ID:        "session-offline",
				RepoPath:  "/home/user/mobile-app",
				WorkTree:  "/home/user/.plural-worktrees/session-offline",
				Branch:    "plural-offline",
				Name:      "offline-mode",
				CreatedAt: time.Now().Add(-6 * time.Hour),
				Started:   true,
			},
		},
		Focus: "sidebar",
	},
	Steps: []demo.Step{
		// ============================================================
		// OPENING: The Multi-Session Workspace
		// ============================================================

		// Initial view - show the workspace with multiple repos and sessions
		demo.Wait(1 * time.Second),
		demo.Capture(),

		// Navigate through sessions to show the multi-session concept
		demo.Key("down"),
		demo.Wait(400 * time.Millisecond),
		demo.Key("down"),
		demo.Wait(400 * time.Millisecond),
		demo.Key("down"),
		demo.Wait(400 * time.Millisecond),
		demo.Capture(),

		// Go back to first session
		demo.Key("up"),
		demo.Wait(300 * time.Millisecond),
		demo.Key("up"),
		demo.Wait(300 * time.Millisecond),
		demo.Key("up"),
		demo.Wait(300 * time.Millisecond),

		// ============================================================
		// ACT 1: Starting a Feature (Plan Approval + Todo List)
		// ============================================================

		// Select the auth session
		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Type a message to Claude
		demo.Type("Add JWT-based user authentication with login and signup flows"),
		demo.Wait(300 * time.Millisecond),
		demo.Capture(),

		// Send the message
		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),

		// Claude presents a plan for approval
		demo.PlanApproval(`## Authentication Implementation Plan

I'll implement JWT-based authentication with the following approach:

1. **Create AuthService** - JWT token generation, validation, refresh logic
2. **Add User model** - Email, password hash, created_at fields
3. **Build API endpoints** - POST /auth/login, POST /auth/signup, POST /auth/refresh
4. **Add middleware** - Token validation for protected routes
5. **Write tests** - Unit tests for auth logic, integration tests for endpoints

**Files to create/modify:**
- src/services/auth.ts
- src/models/user.ts
- src/routes/auth.ts
- src/middleware/authenticate.ts
- tests/auth.test.ts`,
			mcp.AllowedPrompt{Tool: "Bash", Prompt: "run tests"},
			mcp.AllowedPrompt{Tool: "Bash", Prompt: "install dependencies"},
		),
		demo.Wait(1500 * time.Millisecond),
		demo.Capture(),

		// User approves plan
		demo.Key("y"),
		demo.Wait(500 * time.Millisecond),

		// Claude creates a todo list
		demo.TodoList(
			claude.TodoItem{Content: "Create AuthService with JWT logic", Status: claude.TodoStatusInProgress, ActiveForm: "Creating AuthService"},
			claude.TodoItem{Content: "Add User model with validation", Status: claude.TodoStatusPending},
			claude.TodoItem{Content: "Build login/signup endpoints", Status: claude.TodoStatusPending},
			claude.TodoItem{Content: "Add authentication middleware", Status: claude.TodoStatusPending},
			claude.TodoItem{Content: "Write comprehensive tests", Status: claude.TodoStatusPending},
		),
		demo.Wait(800 * time.Millisecond),
		demo.Capture(),

		// ============================================================
		// ACT 2: Claude Working (Tool Rollups + Questions + Permissions)
		// ============================================================

		// Claude performs tool operations (shown in rollup)
		demo.ToolUse("Read", "src/services/index.ts"),
		demo.Wait(200 * time.Millisecond),
		demo.ToolUse("Read", "src/models/index.ts"),
		demo.Wait(200 * time.Millisecond),
		demo.ToolUse("Write", "src/services/auth.ts"),
		demo.Wait(200 * time.Millisecond),
		demo.ToolUse("Write", "src/models/user.ts"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Update todo - first task complete, second in progress
		demo.TodoList(
			claude.TodoItem{Content: "Create AuthService with JWT logic", Status: claude.TodoStatusCompleted},
			claude.TodoItem{Content: "Add User model with validation", Status: claude.TodoStatusCompleted},
			claude.TodoItem{Content: "Build login/signup endpoints", Status: claude.TodoStatusInProgress, ActiveForm: "Building endpoints"},
			claude.TodoItem{Content: "Add authentication middleware", Status: claude.TodoStatusPending},
			claude.TodoItem{Content: "Write comprehensive tests", Status: claude.TodoStatusPending},
		),
		demo.Wait(500 * time.Millisecond),

		// Claude asks a question about password requirements
		demo.Question(mcp.Question{
			Question: "What password requirements should we enforce?",
			Header:   "Security",
			Options: []mcp.QuestionOption{
				{Label: "Standard (8+ chars)", Description: "Minimum length only"},
				{Label: "Strong (12+ mixed)", Description: "Letters, numbers, symbols required"},
				{Label: "Custom policy", Description: "Configure specific rules"},
			},
		}),
		demo.Wait(1 * time.Second),
		demo.Capture(),

		// User selects strong password option
		demo.Key("down"),
		demo.Wait(300 * time.Millisecond),
		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),

		// More tool operations
		demo.ToolUse("Write", "src/routes/auth.ts"),
		demo.Wait(200 * time.Millisecond),
		demo.ToolUse("Write", "src/middleware/authenticate.ts"),
		demo.Wait(200 * time.Millisecond),
		demo.ToolUse("Edit", "src/routes/index.ts"),
		demo.Wait(500 * time.Millisecond),

		// Update todo - more progress
		demo.TodoList(
			claude.TodoItem{Content: "Create AuthService with JWT logic", Status: claude.TodoStatusCompleted},
			claude.TodoItem{Content: "Add User model with validation", Status: claude.TodoStatusCompleted},
			claude.TodoItem{Content: "Build login/signup endpoints", Status: claude.TodoStatusCompleted},
			claude.TodoItem{Content: "Add authentication middleware", Status: claude.TodoStatusCompleted},
			claude.TodoItem{Content: "Write comprehensive tests", Status: claude.TodoStatusInProgress, ActiveForm: "Writing tests"},
		),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Claude requests permission to run tests
		demo.Permission("Bash", "npm test -- --coverage"),
		demo.Wait(800 * time.Millisecond),
		demo.Capture(),

		// User approves
		demo.Key("y"),
		demo.Wait(500 * time.Millisecond),

		// Final todo state - all complete
		demo.TodoList(
			claude.TodoItem{Content: "Create AuthService with JWT logic", Status: claude.TodoStatusCompleted},
			claude.TodoItem{Content: "Add User model with validation", Status: claude.TodoStatusCompleted},
			claude.TodoItem{Content: "Build login/signup endpoints", Status: claude.TodoStatusCompleted},
			claude.TodoItem{Content: "Add authentication middleware", Status: claude.TodoStatusCompleted},
			claude.TodoItem{Content: "Write comprehensive tests", Status: claude.TodoStatusCompleted},
		),
		demo.Wait(500 * time.Millisecond),

		// Claude presents options for next steps
		demo.StreamingTextResponse(`All tests passing with 94% coverage. The authentication system is complete.

For the session management, there are several approaches we could take:

<options>
1. In-memory sessions - Simple Map-based storage, fast but lost on restart
2. Redis sessions - Distributed session store, scales horizontally
3. Database sessions - PostgreSQL storage, consistent with existing data
</options>

Which approach would you prefer?`, 40),
		demo.Wait(1500 * time.Millisecond),
		demo.Capture(),

		// ============================================================
		// ACT 3: Exploring Options (Parallel Exploration)
		// ============================================================

		// Press Ctrl+O to explore options
		demo.KeyWithDesc("ctrl+o", "Explore options"),
		demo.Wait(800 * time.Millisecond),
		demo.Capture(),

		// Select all three options
		demo.Key("space"),
		demo.Wait(300 * time.Millisecond),
		demo.Key("down"),
		demo.Wait(300 * time.Millisecond),
		demo.Key("space"),
		demo.Wait(300 * time.Millisecond),
		demo.Key("down"),
		demo.Wait(300 * time.Millisecond),
		demo.Key("space"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Fork all selected options
		demo.Key("enter"),
		demo.Wait(1500 * time.Millisecond),
		demo.Capture(),

		// ============================================================
		// ACT 4: Reviewing & Merging
		// ============================================================

		// Navigate to first child session
		demo.Key("down"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Select child session to review
		demo.Key("enter"),
		demo.Wait(800 * time.Millisecond),

		// Claude's implementation in child session
		demo.StreamingTextResponse(`I've implemented Redis-based session management:

**Changes made:**
- Added redis client configuration in src/config/redis.ts
- Created SessionStore class with get/set/delete operations
- Updated AuthService to use Redis for token storage
- Added session expiration and refresh logic
- Included connection pooling for performance

The implementation supports:
- Automatic session expiration (24h default)
- Secure token rotation on refresh
- Horizontal scaling across multiple instances`, 35),
		demo.Wait(1500 * time.Millisecond),
		demo.Capture(),

		// Switch to sidebar to use shortcuts
		demo.KeyWithDesc("tab", "Focus sidebar"),
		demo.Wait(300 * time.Millisecond),

		// View changed files
		demo.KeyWithDesc("v", "View changed files"),
		demo.Wait(800 * time.Millisecond),
		demo.Capture(),

		// Navigate through files
		demo.Key("down"),
		demo.Wait(400 * time.Millisecond),
		demo.Key("down"),
		demo.Wait(400 * time.Millisecond),
		demo.Capture(),

		// Exit view changes
		demo.Key("escape"),
		demo.Wait(400 * time.Millisecond),

		// Ensure sidebar focus for merge shortcut
		demo.Key("tab"),
		demo.Wait(300 * time.Millisecond),

		// Open merge modal
		demo.KeyWithDesc("m", "Open merge options"),
		demo.Wait(800 * time.Millisecond),
		demo.Capture(),

		// Navigate to Create PR option
		demo.Key("down"),
		demo.Wait(300 * time.Millisecond),
		demo.Key("down"),
		demo.Wait(300 * time.Millisecond),
		demo.Capture(),

		// Select Create PR - shows loading modal while generating commit message
		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(), // Capture "waiting for claude" state

		// Wait a bit to show the loading state
		demo.Wait(2 * time.Second),

		// Commit message generated - transitions to edit commit modal
		demo.CommitMessage(`Add Redis-based session management

Implement distributed session storage using Redis for horizontal scaling.

Changes:
- Add redis client configuration
- Create SessionStore class with get/set/delete operations
- Update AuthService to use Redis for token storage
- Add session expiration and refresh logic
- Include connection pooling for performance`),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(), // Capture edit commit modal with proposed message

		// User confirms with Ctrl+S
		demo.Key("ctrl+s"),
		demo.Wait(1 * time.Second),
		demo.Capture(),

		// ============================================================
		// ACT 5: Quick Glimpse at Other Sessions
		// ============================================================

		// Navigate to another repo's session
		demo.Key("down"),
		demo.Wait(400 * time.Millisecond),
		demo.Key("down"),
		demo.Wait(400 * time.Millisecond),
		demo.Key("down"),
		demo.Wait(400 * time.Millisecond),
		demo.Key("down"),
		demo.Wait(400 * time.Millisecond),
		demo.Capture(),

		// Select the API session briefly
		demo.Key("enter"),
		demo.Wait(800 * time.Millisecond),
		demo.Capture(),

		// Final pause
		demo.Wait(2 * time.Second),
	},
}

// All returns all built-in scenarios.
func All() []*demo.Scenario {
	return []*demo.Scenario{
		Overview,
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
