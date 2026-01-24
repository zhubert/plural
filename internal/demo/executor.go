package demo

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/app"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	pexec "github.com/zhubert/plural/internal/exec"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/mcp"
	"github.com/zhubert/plural/internal/session"
	"github.com/zhubert/plural/internal/ui"
)

// Frame represents a captured frame from the demo.
type Frame struct {
	Content    string        // ANSI-encoded terminal content
	Delay      time.Duration // Delay before this frame
	Annotation string        // Optional annotation/caption
	StepIndex  int           // Index of the step that produced this frame
}

// ExecutorConfig configures the demo executor.
type ExecutorConfig struct {
	// CaptureEveryStep captures a frame after every step (default: true)
	CaptureEveryStep bool

	// TypeDelay is the delay between characters when typing (default: 50ms)
	TypeDelay time.Duration

	// KeyDelay is the delay after key presses (default: 100ms)
	KeyDelay time.Duration

	// ResponseChunkDelay is the delay between response chunks (default: 30ms)
	ResponseChunkDelay time.Duration
}

// DefaultExecutorConfig returns the default executor configuration.
func DefaultExecutorConfig() ExecutorConfig {
	return ExecutorConfig{
		CaptureEveryStep:   false, // Don't capture every step by default for cleaner demos
		TypeDelay:          50 * time.Millisecond,
		KeyDelay:           100 * time.Millisecond,
		ResponseChunkDelay: 30 * time.Millisecond,
	}
}

// Executor runs demo scenarios and captures frames.
type Executor struct {
	config  ExecutorConfig
	model   *app.Model
	factory *runnerFactory
	frames  []Frame

	currentAnnotation string

	// mockExecutor is the command executor used for git/session operations
	mockExecutor *pexec.MockExecutor
}

// runnerFactory creates mock runners for demo sessions.
type runnerFactory struct {
	runners map[string]*claude.MockRunner
}

func newRunnerFactory() *runnerFactory {
	return &runnerFactory{
		runners: make(map[string]*claude.MockRunner),
	}
}

func (f *runnerFactory) Create(sessionID, workingDir string, started bool, msgs []claude.Message) claude.RunnerInterface {
	mock := claude.NewMockRunner(sessionID, started, msgs)
	f.runners[sessionID] = mock
	return mock
}

func (f *runnerFactory) GetMock(sessionID string) *claude.MockRunner {
	return f.runners[sessionID]
}

// NewExecutor creates a new demo executor.
func NewExecutor(cfg ExecutorConfig) *Executor {
	return &Executor{
		config:  cfg,
		factory: newRunnerFactory(),
		frames:  []Frame{},
	}
}

// Cleanup performs any cleanup after Run completes.
// Since we now use service injection instead of global executors,
// there's no state to restore.
func (e *Executor) Cleanup() {
	// No cleanup needed - services are injected per-instance
}

// Run executes a scenario and returns the captured frames.
func (e *Executor) Run(scenario *Scenario) ([]Frame, error) {
	if err := scenario.Validate(); err != nil {
		return nil, fmt.Errorf("invalid scenario: %w", err)
	}

	// Initialize the model
	if err := e.setup(scenario); err != nil {
		return nil, fmt.Errorf("setup failed: %w", err)
	}

	// Ensure cleanup is called when we're done
	defer e.Cleanup()

	// Capture initial frame
	e.captureFrame(0, 500*time.Millisecond)

	// Execute each step
	for i, step := range scenario.Steps {
		if err := e.executeStep(i, step); err != nil {
			return nil, fmt.Errorf("step %d failed: %w", i, err)
		}
	}

	return e.frames, nil
}

// setup initializes the model for the scenario.
func (e *Executor) setup(scenario *Scenario) error {
	// Create mock executor with common git command responses
	e.mockExecutor = pexec.NewMockExecutor(nil)
	e.setupMockResponses(scenario)

	// Create config from scenario setup
	cfg := &config.Config{
		Repos:            scenario.Setup.Repos,
		Sessions:         scenario.Setup.Sessions,
		AllowedTools:     []string{},
		RepoAllowedTools: make(map[string][]string),
		MCPServers:       []config.MCPServer{},
		RepoMCP:          make(map[string][]config.MCPServer),
		WelcomeShown:     true, // Skip welcome modal in demos
	}

	// Create model
	e.model = app.New(cfg, "demo")

	// Create services with mock executor and inject them
	mockGitService := git.NewGitServiceWithExecutor(e.mockExecutor)
	mockSessionService := session.NewSessionServiceWithExecutor(e.mockExecutor)
	e.model.SetGitService(mockGitService)
	e.model.SetSessionService(mockSessionService)

	// Set size
	e.model.Update(tea.WindowSizeMsg{
		Width:  scenario.Width,
		Height: scenario.Height,
	})

	// Configure for demo mode: skip loading saved messages and use mock runners
	e.model.SessionMgr().SetSkipMessageLoad(true)
	e.model.SessionMgr().SetRunnerFactory(e.factory.Create)

	return nil
}

// setupMockResponses configures mock responses for common git commands.
func (e *Executor) setupMockResponses(scenario *Scenario) {
	// git rev-parse --git-dir (validate repo)
	e.mockExecutor.AddPrefixMatch("git", []string{"rev-parse", "--git-dir"}, pexec.MockResponse{
		Stdout: []byte(".git\n"),
	})

	// git rev-parse --show-toplevel (get git root)
	e.mockExecutor.AddPrefixMatch("git", []string{"rev-parse", "--show-toplevel"}, pexec.MockResponse{
		Stdout: []byte("/home/user/webapp\n"),
	})

	// git rev-parse --verify (branch exists check)
	e.mockExecutor.AddPrefixMatch("git", []string{"rev-parse", "--verify"}, pexec.MockResponse{
		Stdout: []byte(""),
		Err:    fmt.Errorf("branch not found"), // Default: branch doesn't exist
	})

	// git worktree add (create worktree)
	e.mockExecutor.AddPrefixMatch("git", []string{"worktree", "add"}, pexec.MockResponse{
		Stdout: []byte("Preparing worktree (new branch 'plural-fork')\n"),
	})

	// git worktree remove (delete worktree)
	e.mockExecutor.AddPrefixMatch("git", []string{"worktree", "remove"}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	// git worktree prune
	e.mockExecutor.AddPrefixMatch("git", []string{"worktree", "prune"}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	// git branch -D (delete branch)
	e.mockExecutor.AddPrefixMatch("git", []string{"branch", "-D"}, pexec.MockResponse{
		Stdout: []byte("Deleted branch plural-fork\n"),
	})

	// git branch -m (rename branch)
	e.mockExecutor.AddPrefixMatch("git", []string{"branch", "-m"}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	// git status --porcelain (check for changes) - return realistic file changes
	e.mockExecutor.AddPrefixMatch("git", []string{"status", "--porcelain"}, pexec.MockResponse{
		Stdout: []byte(`M  src/services/search.py
M  src/models/task.py
A  src/api/tasks.py
A  db/migrations/add_search_index.sql
A  tests/test_search.py
`),
	})

	// git diff commands - return realistic diff content
	diffContent := `diff --git a/src/services/search.py b/src/services/search.py
new file mode 100644
index 0000000..a1b2c3d
--- /dev/null
+++ b/src/services/search.py
@@ -0,0 +1,45 @@
+from sqlalchemy import text
+from models import Task
+
+class SearchService:
+    """Full-text search service using PostgreSQL tsvector."""
+
+    def __init__(self, db):
+        self.db = db
+
+    def search_tasks(self, query: str, user_id: int) -> list[Task]:
+        """Search tasks using PostgreSQL full-text search."""
+        sql = text("""
+            SELECT * FROM tasks
+            WHERE user_id = :user_id
+            AND search_vector @@ plainto_tsquery('english', :query)
+            ORDER BY ts_rank(search_vector, plainto_tsquery('english', :query)) DESC
+        """)
+        return self.db.execute(sql, {"user_id": user_id, "query": query}).fetchall()

diff --git a/src/models/task.py b/src/models/task.py
index 1234567..89abcde 100644
--- a/src/models/task.py
+++ b/src/models/task.py
@@ -1,5 +1,6 @@
 from sqlalchemy import Column, Integer, String, DateTime
+from sqlalchemy.dialects.postgresql import TSVECTOR
 from database import Base

 class Task(Base):
@@ -8,6 +9,7 @@ class Task(Base):
     title = Column(String(255), nullable=False)
     description = Column(String(1000))
     due_date = Column(DateTime)
+    search_vector = Column(TSVECTOR)
     created_at = Column(DateTime, default=datetime.utcnow)

diff --git a/db/migrations/add_search_index.sql b/db/migrations/add_search_index.sql
new file mode 100644
index 0000000..def4567
--- /dev/null
+++ b/db/migrations/add_search_index.sql
@@ -0,0 +1,12 @@
+-- Add full-text search support to tasks table
+ALTER TABLE tasks ADD COLUMN search_vector tsvector;
+
+-- Create GIN index for fast full-text search
+CREATE INDEX idx_tasks_search ON tasks USING GIN(search_vector);
+
+-- Create trigger to automatically update search vector
+CREATE TRIGGER tasks_search_update
+    BEFORE INSERT OR UPDATE ON tasks
+    FOR EACH ROW EXECUTE FUNCTION
+    tsvector_update_trigger(search_vector, 'pg_catalog.english', title, description);
`
	e.mockExecutor.AddPrefixMatch("git", []string{"diff"}, pexec.MockResponse{
		Stdout: []byte(diffContent),
	})

	// git add -A (stage all)
	e.mockExecutor.AddPrefixMatch("git", []string{"add", "-A"}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	// git commit
	e.mockExecutor.AddPrefixMatch("git", []string{"commit"}, pexec.MockResponse{
		Stdout: []byte("[main abc1234] Commit message\n"),
	})

	// git checkout (for merges)
	e.mockExecutor.AddPrefixMatch("git", []string{"checkout"}, pexec.MockResponse{
		Stdout: []byte("Switched to branch 'main'\n"),
	})

	// git merge
	e.mockExecutor.AddPrefixMatch("git", []string{"merge"}, pexec.MockResponse{
		Stdout: []byte("Merge successful\n"),
	})

	// git push
	e.mockExecutor.AddPrefixMatch("git", []string{"push"}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	// git remote get-url origin (check for remote)
	e.mockExecutor.AddPrefixMatch("git", []string{"remote", "get-url", "origin"}, pexec.MockResponse{
		Stdout: []byte("git@github.com:user/repo.git\n"),
	})

	// git symbolic-ref refs/remotes/origin/HEAD (get default branch)
	e.mockExecutor.AddPrefixMatch("git", []string{"symbolic-ref", "refs/remotes/origin/HEAD"}, pexec.MockResponse{
		Stdout: []byte("refs/remotes/origin/main\n"),
	})

	// gh commands (GitHub CLI)
	e.mockExecutor.AddPrefixMatch("gh", []string{"pr", "create"}, pexec.MockResponse{
		Stdout: []byte("https://github.com/user/repo/pull/123\n"),
	})

	e.mockExecutor.AddPrefixMatch("gh", []string{"issue", "list"}, pexec.MockResponse{
		Stdout: []byte("[]"), // Empty issues list
	})
}

// executeStep executes a single demo step.
func (e *Executor) executeStep(index int, step Step) error {
	switch step.Type {
	case StepWait:
		// If there's active streaming, capture animated frames to show spinner
		if e.model.HasActiveStreaming() && step.Duration >= 300*time.Millisecond {
			e.captureAnimatedFrames(index, step.Duration, 300*time.Millisecond)
		} else {
			e.captureFrame(index, step.Duration)
		}

	case StepKey:
		e.sendKey(step.Key)
		if e.config.CaptureEveryStep {
			e.captureFrame(index, e.config.KeyDelay)
		}

	case StepTypeText:
		for _, ch := range step.Text {
			e.sendKey(string(ch))
			if e.config.CaptureEveryStep {
				e.captureFrame(index, e.config.TypeDelay)
			}
		}

	case StepResponse:
		session := e.model.ActiveSession()
		if session == nil {
			return fmt.Errorf("no active session for response")
		}

		// First pass: collect the full response text and send non-Done chunks
		var fullResponse string
		var doneChunk *claude.ResponseChunk
		for _, chunk := range step.Chunks {
			if chunk.Type == claude.ChunkTypeText {
				fullResponse += chunk.Content
			}
			if chunk.Done {
				// Save Done chunk for later - we need to add assistant message first
				doneChunk = &chunk
				continue
			}
			e.simulateResponse(session.ID, chunk)
			if e.config.CaptureEveryStep && chunk.Type == claude.ChunkTypeText {
				e.captureFrame(index, e.config.ResponseChunkDelay)
			}
		}

		// Add the assistant message to the mock runner BEFORE sending Done chunk
		// This is needed for detectOptionsInSession to find the options
		if mock := e.factory.GetMock(session.ID); mock != nil && fullResponse != "" {
			mock.AddAssistantMessage(fullResponse)
		}

		// Now send the Done chunk to trigger handleClaudeDone -> detectOptionsInSession
		if doneChunk != nil {
			e.simulateResponse(session.ID, *doneChunk)
		}

		// Always capture after response completes
		e.captureFrame(index, 200*time.Millisecond)

	case StepPermission:
		session := e.model.ActiveSession()
		if session == nil {
			return fmt.Errorf("no active session for permission")
		}
		e.simulatePermission(session.ID, step.PermissionTool, step.PermissionDescription)
		e.captureFrame(index, 300*time.Millisecond)

	case StepAnnotate:
		e.currentAnnotation = step.Annotation
		// Don't capture, annotation applies to next frame

	case StepCapture:
		// Send tick messages before capture to ensure spinner is up-to-date
		if e.model.HasActiveStreaming() {
			e.sendTickMessages()
		}
		e.captureFrame(index, 0)

	case StepQuestion:
		session := e.model.ActiveSession()
		if session == nil {
			return fmt.Errorf("no active session for question")
		}
		e.simulateQuestion(session.ID, step.Questions)
		e.captureFrame(index, 300*time.Millisecond)

	case StepPlanApproval:
		session := e.model.ActiveSession()
		if session == nil {
			return fmt.Errorf("no active session for plan approval")
		}
		e.simulatePlanApproval(session.ID, step.Plan, step.AllowedPrompts)
		e.captureFrame(index, 300*time.Millisecond)

	case StepTodoList:
		session := e.model.ActiveSession()
		if session == nil {
			return fmt.Errorf("no active session for todo list")
		}
		e.simulateTodoList(session.ID, step.TodoItems)
		e.captureFrame(index, 200*time.Millisecond)

	case StepFlash:
		e.simulateFlash(step.FlashText, step.FlashType)
		e.captureFrame(index, 100*time.Millisecond)

	case StepToolUse:
		session := e.model.ActiveSession()
		if session == nil {
			return fmt.Errorf("no active session for tool use")
		}
		e.simulateToolUse(session.ID, step.ToolName, step.ToolInput)
		e.captureFrame(index, 200*time.Millisecond)

	case StepCommitMessage:
		session := e.model.ActiveSession()
		if session == nil {
			return fmt.Errorf("no active session for commit message")
		}
		e.simulateCommitMessage(session.ID, step.CommitMessage)
		e.captureFrame(index, 300*time.Millisecond)
	}

	return nil
}

// captureFrame captures the current view as a frame.
func (e *Executor) captureFrame(stepIndex int, delay time.Duration) {
	content := e.model.RenderToString()

	frame := Frame{
		Content:    content,
		Delay:      delay,
		Annotation: e.currentAnnotation,
		StepIndex:  stepIndex,
	}
	e.frames = append(e.frames, frame)

	// Clear annotation after use
	e.currentAnnotation = ""
}

// captureAnimatedFrames captures multiple frames with spinner animation.
// This is used for Wait steps when streaming is active to show animated spinners.
func (e *Executor) captureAnimatedFrames(stepIndex int, totalDuration time.Duration, frameInterval time.Duration) {
	if frameInterval <= 0 {
		frameInterval = 300 * time.Millisecond // Match SidebarTick interval
	}

	numFrames := int(totalDuration / frameInterval)
	if numFrames < 1 {
		numFrames = 1
	}

	delayPerFrame := totalDuration / time.Duration(numFrames)

	for i := 0; i < numFrames; i++ {
		// Send tick messages to advance spinner animation
		e.sendTickMessages()

		// Capture the frame
		e.captureFrame(stepIndex, delayPerFrame)
	}
}

// sendTickMessages sends tick messages to animate spinners.
func (e *Executor) sendTickMessages() {
	// Send sidebar tick to advance spinner
	result, _ := e.model.Update(ui.SidebarTickMsg(time.Now()))
	e.model = result.(*app.Model)

	// Send stopwatch tick for chat spinner
	result, _ = e.model.Update(ui.StopwatchTickMsg(time.Now()))
	e.model = result.(*app.Model)
}

// sendKey sends a key press to the model.
func (e *Executor) sendKey(key string) {
	msg := keyPress(key)
	result, _ := e.model.Update(msg)
	e.model = result.(*app.Model)
}

// simulateResponse injects a Claude response chunk.
func (e *Executor) simulateResponse(sessionID string, chunk claude.ResponseChunk) {
	msg := app.ClaudeResponseMsg{
		SessionID: sessionID,
		Chunk:     chunk,
	}
	result, _ := e.model.Update(msg)
	e.model = result.(*app.Model)
}

// simulatePermission injects a permission request.
func (e *Executor) simulatePermission(sessionID, tool, description string) {
	msg := app.PermissionRequestMsg{
		SessionID: sessionID,
		Request: mcp.PermissionRequest{
			Tool:        tool,
			Description: description,
		},
	}
	result, _ := e.model.Update(msg)
	e.model = result.(*app.Model)
}

// simulateQuestion injects a question request.
func (e *Executor) simulateQuestion(sessionID string, questions []mcp.Question) {
	msg := app.QuestionRequestMsg{
		SessionID: sessionID,
		Request: mcp.QuestionRequest{
			Questions: questions,
		},
	}
	result, _ := e.model.Update(msg)
	e.model = result.(*app.Model)
}

// simulatePlanApproval injects a plan approval request.
func (e *Executor) simulatePlanApproval(sessionID, plan string, allowedPrompts []mcp.AllowedPrompt) {
	msg := app.PlanApprovalRequestMsg{
		SessionID: sessionID,
		Request: mcp.PlanApprovalRequest{
			Plan:           plan,
			AllowedPrompts: allowedPrompts,
		},
	}
	result, _ := e.model.Update(msg)
	e.model = result.(*app.Model)
}

// simulateTodoList injects a todo list update.
func (e *Executor) simulateTodoList(sessionID string, items []claude.TodoItem) {
	msg := app.ClaudeResponseMsg{
		SessionID: sessionID,
		Chunk: claude.ResponseChunk{
			Type:     claude.ChunkTypeTodoUpdate,
			TodoList: &claude.TodoList{Items: items},
		},
	}
	result, _ := e.model.Update(msg)
	e.model = result.(*app.Model)
}

// simulateFlash shows a flash message.
func (e *Executor) simulateFlash(text string, flashType ui.FlashType) {
	cmd := e.model.ShowFlash(text, flashType)
	// Execute the returned command to start the flash tick
	if cmd != nil {
		msg := cmd()
		if msg != nil {
			result, _ := e.model.Update(msg)
			e.model = result.(*app.Model)
		}
	}
}

// simulateToolUse injects a tool use chunk.
func (e *Executor) simulateToolUse(sessionID, name, input string) {
	msg := app.ClaudeResponseMsg{
		SessionID: sessionID,
		Chunk: claude.ResponseChunk{
			Type:      claude.ChunkTypeToolUse,
			ToolName:  name,
			ToolInput: input,
		},
	}
	result, _ := e.model.Update(msg)
	e.model = result.(*app.Model)
}

// simulateCommitMessage injects a commit message generated result.
// This transitions the LoadingCommitState modal to EditCommitState.
func (e *Executor) simulateCommitMessage(sessionID, message string) {
	msg := app.CommitMessageGeneratedMsg{
		SessionID: sessionID,
		Message:   message,
		Error:     nil,
	}
	result, _ := e.model.Update(msg)
	e.model = result.(*app.Model)
}

// keyPress converts a key string to a tea.KeyPressMsg.
// Duplicated from testutil to avoid import cycle.
func keyPress(key string) tea.KeyPressMsg {
	switch key {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "escape", "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "ctrl+v":
		return tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl}
	case "ctrl+s":
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	case "ctrl+o":
		return tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl}
	case "ctrl+p":
		return tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	default:
		if len(key) == 1 {
			return tea.KeyPressMsg{Code: rune(key[0]), Text: key}
		}
		return tea.KeyPressMsg{Text: key}
	}
}
