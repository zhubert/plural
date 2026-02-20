package app

import (
	"context"
	"os"
	"testing"

	"github.com/zhubert/plural-core/claude"
	"github.com/zhubert/plural-core/config"
	"github.com/zhubert/plural-core/manager"
	pexec "github.com/zhubert/plural-core/exec"
	"github.com/zhubert/plural-core/git"
	"github.com/zhubert/plural-core/paths"
	"github.com/zhubert/plural-core/session"
)

// TestForkSessionInheritsContext verifies that when a session is forked with copyMessages=true,
// the child session properly inherits the parent's conversation context in Claude, not just
// the UI message history.
func TestForkSessionInheritsContext(t *testing.T) {
	// Set up a temporary home directory to avoid polluting ~/.plural/ during tests
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	paths.Reset()
	t.Cleanup(paths.Reset)

	// Create test config and repo
	cfg := &config.Config{
		Repos:    []string{},
		Sessions: []config.Session{},
	}
	testRepo := t.TempDir() + "/test-repo"
	cfg.AddRepo(testRepo)

	// Set up mock executor for git operations
	mockExec := pexec.NewMockExecutor(nil)
	mockExec.AddPrefixMatch("git", []string{"status"}, pexec.MockResponse{
		Stdout: []byte("On branch main\nnothing to commit\n"),
	})
	mockExec.AddPrefixMatch("git", []string{"worktree"}, pexec.MockResponse{
		Stdout: []byte(""),
	})
	mockExec.AddPrefixMatch("git", []string{"branch"}, pexec.MockResponse{
		Stdout: []byte(""),
	})
	mockExec.AddPrefixMatch("git", []string{"rev-parse"}, pexec.MockResponse{
		Stdout: []byte("main\n"),
	})
	mockExec.AddPrefixMatch("git", []string{"diff"}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	gitSvc := git.NewGitServiceWithExecutor(mockExec)
	sessionSvc := session.NewSessionServiceWithExecutor(mockExec)

	// Create session manager with mock runner factory
	sessionMgr := manager.NewSessionManager(cfg, gitSvc)
	sessionMgr.SetSkipMessageLoad(true) // We'll manually set up messages

	// Track runners created by the factory
	var createdRunners []*claude.MockRunner
	mockFactory := func(sessionID, workingDir, repoPath string, sessionStarted bool, initialMessages []claude.Message) claude.RunnerInterface {
		mockRunner := claude.NewMockRunner(sessionID, sessionStarted, initialMessages)
		createdRunners = append(createdRunners, mockRunner)
		return mockRunner
	}
	sessionMgr.SetRunnerFactory(mockFactory)

	// Create parent session
	ctx := context.Background()
	parentSess, err := sessionSvc.Create(ctx, testRepo, "parent", "", session.BasePointOrigin)
	if err != nil {
		t.Fatalf("Failed to create parent session: %v", err)
	}
	cfg.AddSession(*parentSess)

	// Get the parent runner and simulate a conversation
	parentResult := sessionMgr.Select(parentSess, "", "", "")
	if parentResult == nil || parentResult.Runner == nil {
		t.Fatal("Failed to get parent runner")
	}

	// Mark parent as started (simulating that it has sent at least one message)
	cfg.MarkSessionStarted(parentSess.ID)
	parentSess = cfg.GetSession(parentSess.ID) // Get updated session
	if parentSess == nil {
		t.Fatal("Parent session not found after marking started")
	}

	// Create worktree directories and Claude session file for fork to work
	// The fork logic copies Claude's session JSONL file from parent to child worktree
	if err := os.MkdirAll(parentSess.WorkTree, 0755); err != nil {
		t.Fatalf("Failed to create parent worktree: %v", err)
	}

	// Save parent messages to disk so child can load them
	parentConfigMsgs := []config.Message{
		{Role: "user", Content: "What is the capital of France?"},
		{Role: "assistant", Content: "The capital of France is Paris."},
		{Role: "user", Content: "What about Germany?"},
		{Role: "assistant", Content: "The capital of Germany is Berlin."},
	}
	if err := config.SaveSessionMessages(parentSess.ID, parentConfigMsgs, config.MaxSessionMessageLines); err != nil {
		t.Fatalf("Failed to save parent messages: %v", err)
	}

	// Now create a forked session (simulating the fork modal with copyMessages=true)
	childSess, err := sessionSvc.CreateFromBranch(ctx, testRepo, parentSess.Branch, "child", "")
	if err != nil {
		t.Fatalf("Failed to create child session: %v", err)
	}
	childSess.ParentID = parentSess.ID

	// Copy messages from parent to child (this is what the fork modal does with copyMessages=true)
	if err := config.SaveSessionMessages(childSess.ID, parentConfigMsgs, config.MaxSessionMessageLines); err != nil {
		t.Fatalf("Failed to copy messages to child: %v", err)
	}

	cfg.AddSession(*childSess)

	// Now select the child session - this should set up the fork
	// Re-enable message loading so the child loads the copied messages
	sessionMgr.SetSkipMessageLoad(false)

	t.Logf("Before Select - Child session ID: %s, ParentID: %s, Started: %t",
		childSess.ID, childSess.ParentID, childSess.Started)
	t.Logf("Before Select - Parent session ID: %s, Started: %t",
		parentSess.ID, parentSess.Started)

	childResult := sessionMgr.Select(childSess, "", "", "")
	if childResult == nil || childResult.Runner == nil {
		t.Fatal("Failed to get child runner")
	}

	t.Logf("After Select - Created %d runners total", len(createdRunners))

	// Verify the child runner was created with the copied messages
	if len(childResult.Messages) != 4 {
		t.Errorf("Child runner has %d messages, expected 4", len(childResult.Messages))
	}

	// The critical test: verify that the child runner is set up to fork from parent
	childMockRunner := createdRunners[len(createdRunners)-1]

	// Verify that the child runner was configured to fork from parent
	forkFromID := childMockRunner.GetForkFromSessionID()
	if forkFromID != parentSess.ID {
		t.Errorf("Child runner not configured to fork from parent!\n"+
			"  Expected ForkFromSessionID: %s\n"+
			"  Got: %s\n"+
			"This means Claude won't have the parent conversation context even though the UI shows the messages.",
			parentSess.ID, forkFromID)
	}

	t.Logf("SUCCESS: Child runner correctly configured to fork from parent %s", forkFromID)
}
