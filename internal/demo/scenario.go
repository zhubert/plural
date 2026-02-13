// Package demo provides infrastructure for generating demos of Plural's capabilities.
// It uses the same mock infrastructure as tests to create deterministic, reproducible
// demo recordings without requiring real Claude processes.
package demo

import (
	"time"

	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/mcp"
	"github.com/zhubert/plural/internal/ui"
)

// StepType represents the type of action in a demo step.
type StepType int

const (
	// StepWait pauses for a duration (for timing/pacing).
	StepWait StepType = iota
	// StepKey sends a single key press.
	StepKey
	// StepType types a string character by character.
	StepTypeText
	// StepResponse simulates a Claude response (streaming chunks).
	StepResponse
	// StepPermission simulates a permission request from Claude.
	StepPermission
	// StepCapture captures the current frame (for selective capture).
	StepCapture
	// StepAnnotate adds an annotation/caption to the current frame.
	StepAnnotate
	// StepQuestion simulates a question prompt (AskUserQuestion).
	StepQuestion
	// StepPlanApproval simulates a plan approval request (ExitPlanMode).
	StepPlanApproval
	// StepTodoList displays a TodoList in the sidebar.
	StepTodoList
	// StepFlash shows a flash message in the footer.
	StepFlash
	// StepToolUse shows a tool use in the rollup.
	StepToolUse
	// StepCommitMessage simulates commit message generation completing.
	StepCommitMessage
)

// Step represents a single action in a demo scenario.
type Step struct {
	Type        StepType
	Description string // Human-readable description of what this step does

	// For StepKey
	Key string

	// For StepTypeText
	Text string

	// For StepWait
	Duration time.Duration

	// For StepResponse
	Chunks []claude.ResponseChunk

	// For StepPermission
	PermissionTool        string
	PermissionDescription string

	// For StepAnnotate
	Annotation string

	// For StepQuestion
	Questions []mcp.Question

	// For StepPlanApproval
	Plan           string
	AllowedPrompts []mcp.AllowedPrompt

	// For StepTodoList
	TodoItems []claude.TodoItem

	// For StepFlash
	FlashText string
	FlashType ui.FlashType

	// For StepToolUse
	ToolName  string
	ToolInput string

	// For StepCommitMessage
	CommitMessage string
}

// Scenario defines a complete demo scenario.
type Scenario struct {
	Name        string
	Description string
	Width       int // Terminal width (default 120)
	Height      int // Terminal height (default 40)
	Setup       *ScenarioSetup
	Steps       []Step
}

// ScenarioSetup defines the initial state for a demo.
type ScenarioSetup struct {
	// Repos to register
	Repos []string

	// Sessions to create (pre-populated)
	Sessions []config.Session

	// Initial focus (sidebar or chat)
	Focus string
}

// DefaultSetup returns a minimal setup for demos.
func DefaultSetup() *ScenarioSetup {
	return &ScenarioSetup{
		Repos: []string{"/demo/myproject"},
		Sessions: []config.Session{
			{
				ID:        "demo-session-1",
				RepoPath:  "/demo/myproject",
				WorkTree:  "/demo/.plural-worktrees/demo-session-1",
				Branch:    "plural-demo",
				Name:      "myproject/demo",
				CreatedAt: time.Now(),
				Started:   true,
			},
		},
		Focus: "sidebar",
	}
}

// Validate checks that the scenario is valid.
func (s *Scenario) Validate() error {
	if s.Name == "" {
		return &ValidationError{Field: "Name", Message: "scenario name is required"}
	}
	if s.Width <= 0 {
		s.Width = 120
	}
	if s.Height <= 0 {
		s.Height = 40
	}
	if s.Setup == nil {
		s.Setup = DefaultSetup()
	}
	return nil
}

// ValidationError represents a scenario validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return "validation error: " + e.Field + ": " + e.Message
}

// Step builder functions for fluent scenario construction

// Wait creates a wait step.
func Wait(d time.Duration) Step {
	return Step{
		Type:     StepWait,
		Duration: d,
	}
}

// Key creates a key press step.
func Key(key string) Step {
	return Step{
		Type: StepKey,
		Key:  key,
	}
}

// KeyWithDesc creates a key press step with a description.
func KeyWithDesc(key, description string) Step {
	return Step{
		Type:        StepKey,
		Key:         key,
		Description: description,
	}
}

// Type creates a text typing step.
func Type(text string) Step {
	return Step{
		Type: StepTypeText,
		Text: text,
	}
}

// StreamingTextResponse creates a text response that streams character by character.
func StreamingTextResponse(text string, chunkSize int) Step {
	if chunkSize <= 0 {
		chunkSize = 5 // Default to 5 characters per chunk
	}

	var chunks []claude.ResponseChunk
	for i := 0; i < len(text); i += chunkSize {
		end := i + chunkSize
		if end > len(text) {
			end = len(text)
		}
		chunks = append(chunks, claude.ResponseChunk{
			Type:    claude.ChunkTypeText,
			Content: text[i:end],
		})
	}
	chunks = append(chunks, claude.ResponseChunk{Done: true})

	return Step{
		Type:   StepResponse,
		Chunks: chunks,
	}
}

// Capture creates a frame capture step.
func Capture() Step {
	return Step{
		Type: StepCapture,
	}
}

// Annotate creates an annotation step that captions the next captured frame.
func Annotate(text string) Step {
	return Step{
		Type:       StepAnnotate,
		Annotation: text,
	}
}

// Permission creates a permission request step.
func Permission(tool, description string) Step {
	return Step{
		Type:                  StepPermission,
		PermissionTool:        tool,
		PermissionDescription: description,
	}
}

// Question creates a question prompt step.
func Question(questions ...mcp.Question) Step {
	return Step{
		Type:      StepQuestion,
		Questions: questions,
	}
}

// PlanApproval creates a plan approval step.
func PlanApproval(plan string, allowedPrompts ...mcp.AllowedPrompt) Step {
	return Step{
		Type:           StepPlanApproval,
		Plan:           plan,
		AllowedPrompts: allowedPrompts,
	}
}

// TodoList creates a step that displays a todo list in the sidebar.
func TodoList(items ...claude.TodoItem) Step {
	return Step{
		Type:      StepTodoList,
		TodoItems: items,
	}
}

// Flash creates a flash message step.
func Flash(text string, flashType ui.FlashType) Step {
	return Step{
		Type:      StepFlash,
		FlashText: text,
		FlashType: flashType,
	}
}

// FlashSuccess creates a success flash message step.
func FlashSuccess(text string) Step {
	return Flash(text, ui.FlashSuccess)
}

// FlashError creates an error flash message step.
func FlashError(text string) Step {
	return Flash(text, ui.FlashError)
}

// FlashInfo creates an info flash message step.
func FlashInfo(text string) Step {
	return Flash(text, ui.FlashInfo)
}

// FlashWarning creates a warning flash message step.
func FlashWarning(text string) Step {
	return Flash(text, ui.FlashWarning)
}

// ToolUse creates a tool use step for the rollup.
func ToolUse(name, input string) Step {
	return Step{
		Type:      StepToolUse,
		ToolName:  name,
		ToolInput: input,
	}
}

// CommitMessage creates a step that simulates commit message generation completing.
// This transitions the LoadingCommitState modal to EditCommitState with the given message.
func CommitMessage(message string) Step {
	return Step{
		Type:          StepCommitMessage,
		CommitMessage: message,
	}
}
