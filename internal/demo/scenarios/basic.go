// Package scenarios contains built-in demo scenarios for Plural.
package scenarios

import (
	"time"

	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/demo"
)

// Basic demonstrates a user workflow for working on a feature and merging:
// - Starting with multiple repos and sessions in various states (realistic to-do app context)
// - Selecting a session and sending a message to Claude
// - Viewing the changed files
// - Merging to main
var Basic = &demo.Scenario{
	Name:        "basic",
	Description: "Work on feature, view files, merge to main",
	Width:       120,
	Height:      40,
	Setup: &demo.ScenarioSetup{
		Repos: []string{"/home/user/todo-app", "/home/user/todo-api"},
		Sessions: []config.Session{
			// Active session for user to work on
			{
				ID:        "session-notifications",
				RepoPath:  "/home/user/todo-app",
				WorkTree:  "/home/user/.plural-worktrees/session-notifications",
				Branch:    "plural-push-notifications",
				Name:      "push-notifications",
				CreatedAt: time.Now().Add(-1 * time.Hour),
				Started:   true,
			},
			// Completed frontend session
			{
				ID:        "session-darkmode",
				RepoPath:  "/home/user/todo-app",
				WorkTree:  "/home/user/.plural-worktrees/session-darkmode",
				Branch:    "plural-dark-mode",
				Name:      "dark-mode-theme",
				CreatedAt: time.Now().Add(-48 * time.Hour),
				Started:   true,
			},
			// Another frontend session
			{
				ID:        "session-duedate",
				RepoPath:  "/home/user/todo-app",
				WorkTree:  "/home/user/.plural-worktrees/session-duedate",
				Branch:    "plural-fix-due-date-sort",
				Name:      "fix-due-date-sort",
				CreatedAt: time.Now().Add(-24 * time.Hour),
				Started:   true,
			},
			// API session
			{
				ID:        "session-api-auth",
				RepoPath:  "/home/user/todo-api",
				WorkTree:  "/home/user/.plural-worktrees/session-api-auth",
				Branch:    "plural-oauth-login",
				Name:      "oauth-login",
				CreatedAt: time.Now().Add(-4 * time.Hour),
				Started:   true,
			},
		},
		Focus: "sidebar",
	},
	Steps: []demo.Step{
		// Initial view - show the interface with existing repos and sessions
		demo.Wait(1 * time.Second),
		demo.Capture(),

		// Select the push-notifications session
		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Type a message to Claude
		demo.Type("Add push notification support for task reminders"),
		demo.Wait(300 * time.Millisecond),
		demo.Capture(),

		// Send the message
		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),

		// Claude responds with streaming
		demo.StreamingTextResponse(`I'll add push notification support for task reminders. Let me implement this feature.

First, I'll create the notification service and integrate it with the task scheduler.

I've made the following changes:
- Created NotificationService class with FCM integration
- Added TaskReminderWorker for background processing
- Updated TaskModel to include reminder settings
- Added notification preferences to user settings

The implementation is complete. You can now set reminders on tasks and users will receive push notifications.`, 50),
		demo.Wait(1 * time.Second),
		demo.Capture(),

		// Switch focus back to sidebar
		demo.KeyWithDesc("tab", "Focus sidebar"),
		demo.Wait(300 * time.Millisecond),

		// View the changed files (v key)
		demo.KeyWithDesc("v", "View changed files"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Navigate through files
		demo.Key("down"),
		demo.Wait(300 * time.Millisecond),
		demo.Key("down"),
		demo.Wait(300 * time.Millisecond),
		demo.Capture(),

		// Exit view changes mode
		demo.Key("escape"),
		demo.Wait(300 * time.Millisecond),

		// Open merge modal (m key) - focus returns to sidebar after escape
		demo.KeyWithDesc("m", "Open merge options"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Select "Merge to main" (first option)
		demo.Key("enter"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Final pause
		demo.Wait(2 * time.Second),
	},
}

// Comprehensive demonstrates the explore options workflow:
// - Starting with multiple repos and sessions in various states (realistic to-do app context)
// - Asking Claude a question that produces multiple options
// - Using Ctrl+P to explore ALL options at once (forking into parallel sessions)
// - Reviewing child sessions with Claude's implementation
// - Viewing files and submitting a PR for the preferred option
var Comprehensive = &demo.Scenario{
	Name:        "comprehensive",
	Description: "Explore options workflow - fork all options, review children, create PR",
	Width:       120,
	Height:      40,
	Setup: &demo.ScenarioSetup{
		Repos: []string{"/home/user/todo-app", "/home/user/todo-api"},
		Sessions: []config.Session{
			// Session where user will ask about approaches
			{
				ID:        "session-search",
				RepoPath:  "/home/user/todo-app",
				WorkTree:  "/home/user/.plural-worktrees/session-search",
				Branch:    "plural-task-search",
				Name:      "task-search",
				CreatedAt: time.Now().Add(-1 * time.Hour),
				Started:   true,
			},
			// Older completed sessions
			{
				ID:        "session-recurring",
				RepoPath:  "/home/user/todo-app",
				WorkTree:  "/home/user/.plural-worktrees/session-recurring",
				Branch:    "plural-recurring-tasks",
				Name:      "recurring-tasks",
				CreatedAt: time.Now().Add(-72 * time.Hour),
				Started:   true,
			},
			{
				ID:        "session-export",
				RepoPath:  "/home/user/todo-app",
				WorkTree:  "/home/user/.plural-worktrees/session-export",
				Branch:    "plural-export-csv",
				Name:      "export-csv",
				CreatedAt: time.Now().Add(-48 * time.Hour),
				Started:   true,
			},
			{
				ID:        "session-api-cache",
				RepoPath:  "/home/user/todo-api",
				WorkTree:  "/home/user/.plural-worktrees/session-api-cache",
				Branch:    "plural-redis-cache",
				Name:      "redis-cache",
				CreatedAt: time.Now().Add(-24 * time.Hour),
				Started:   true,
			},
		},
		Focus: "sidebar",
	},
	Steps: []demo.Step{
		// Initial view - show existing repos and sessions
		demo.Wait(2 * time.Second),
		demo.Capture(),

		// Select the task-search session
		demo.Key("enter"),
		demo.Wait(1 * time.Second),
		demo.Capture(),

		// Ask Claude about search implementation approaches
		demo.Type("What are the best approaches for implementing task search?"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Send the message
		demo.Key("enter"),
		demo.Wait(1 * time.Second),

		// Claude responds with numbered options
		demo.StreamingTextResponse(`There are several approaches for implementing task search, each with different trade-offs:

<options>
1. Full-Text Search with PostgreSQL - Use PostgreSQL's built-in tsvector/tsquery for full-text search, simple to implement with existing database
2. Elasticsearch Integration - Deploy Elasticsearch for advanced search features like fuzzy matching, relevance scoring, and faceted search
3. Client-Side Filtering - Implement search entirely in the browser using existing task data, zero backend changes needed
</options>

Each approach varies in complexity, scalability, and feature richness. Would you like me to implement any of these?`, 30),

		demo.Wait(2 * time.Second),
		demo.Capture(),

		// Press Ctrl+P to explore options (works from chat focus)
		demo.KeyWithDesc("ctrl+p", "Explore options"),
		demo.Wait(1 * time.Second),
		demo.Capture(),

		// Select ALL options - start with option 1
		demo.Key("space"),
		demo.Wait(400 * time.Millisecond),

		// Move down and select option 2
		demo.Key("down"),
		demo.Wait(400 * time.Millisecond),
		demo.Key("space"),
		demo.Wait(400 * time.Millisecond),

		// Move down and select option 3
		demo.Key("down"),
		demo.Wait(400 * time.Millisecond),
		demo.Key("space"),
		demo.Wait(500 * time.Millisecond),
		demo.Capture(),

		// Press Enter to fork all selected options
		demo.Key("enter"),
		demo.Wait(1500 * time.Millisecond),
		demo.Capture(),

		// Navigate to first child session (down arrow)
		demo.Key("down"),
		demo.Wait(800 * time.Millisecond),
		demo.Capture(),

		// Select the child session to review it
		demo.Key("enter"),
		demo.Wait(1 * time.Second),
		demo.Capture(),

		// Claude responds with the implementation in the child session
		demo.StreamingTextResponse(`I'll implement PostgreSQL full-text search for task search. Here's my implementation:

I've made the following changes:

**db/migrations/add_search_index.sql** - Added GIN index for full-text search
**src/models/task.py** - Added search vector column and trigger
**src/services/search.py** - New search service with tsvector/tsquery
**src/api/tasks.py** - Added /tasks/search endpoint
**tests/test_search.py** - Unit tests for search functionality

The implementation uses PostgreSQL's built-in full-text search with:
- Automatic indexing via triggers
- Relevance-based ranking
- Support for prefix matching and phrase queries

You can now search tasks with: GET /tasks/search?q=meeting+notes`, 25),

		demo.Wait(2 * time.Second),
		demo.Capture(),

		// Switch focus back to sidebar for shortcuts
		demo.KeyWithDesc("tab", "Focus sidebar"),
		demo.Wait(500 * time.Millisecond),

		// View changed files for this option (v key)
		demo.KeyWithDesc("v", "View changed files"),
		demo.Wait(1 * time.Second),
		demo.Capture(),

		// Navigate through files
		demo.Key("down"),
		demo.Wait(600 * time.Millisecond),
		demo.Key("down"),
		demo.Wait(600 * time.Millisecond),
		demo.Capture(),

		// Exit view changes
		demo.Key("escape"),
		demo.Wait(500 * time.Millisecond),

		// Ensure focus is on sidebar for merge shortcut
		demo.Key("tab"),
		demo.Wait(300 * time.Millisecond),

		// Open merge modal for PR (m key)
		demo.KeyWithDesc("m", "Open merge options"),
		demo.Wait(1 * time.Second),
		demo.Capture(),

		// Navigate to "Create PR" option
		demo.Key("down"),
		demo.Wait(400 * time.Millisecond),
		demo.Key("down"),
		demo.Wait(400 * time.Millisecond),
		demo.Capture(),

		// Select Create PR
		demo.Key("enter"),
		demo.Wait(1 * time.Second),
		demo.Capture(),

		// Final pause
		demo.Wait(3 * time.Second),
	},
}

// All returns all built-in scenarios.
func All() []*demo.Scenario {
	return []*demo.Scenario{
		Basic,
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
