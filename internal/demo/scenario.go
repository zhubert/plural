// Package demo provides infrastructure for generating demos of Plural's capabilities.
// It uses the same mock infrastructure as tests to create deterministic, reproducible
// demo recordings without requiring real Claude processes.
package demo

import (
	"time"

	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/mcp"
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
	// StepQuestion simulates a question request from Claude.
	StepQuestion
	// StepCapture captures the current frame (for selective capture).
	StepCapture
	// StepAnnotate adds an annotation/caption to the current frame.
	StepAnnotate
	// StepStartStreaming starts a streaming response without completing it.
	// This leaves the session in a "waiting" state with spinner showing.
	StepStartStreaming
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

	// For StepQuestion
	Questions []mcp.Question

	// For StepAnnotate
	Annotation string
}

// Scenario defines a complete demo scenario.
type Scenario struct {
	Name        string
	Description string
	Width       int           // Terminal width (default 120)
	Height      int           // Terminal height (default 40)
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

// TypeWithDesc creates a text typing step with a description.
func TypeWithDesc(text, description string) Step {
	return Step{
		Type:        StepTypeText,
		Text:        text,
		Description: description,
	}
}

// Response creates a Claude response step.
func Response(chunks ...claude.ResponseChunk) Step {
	return Step{
		Type:   StepResponse,
		Chunks: chunks,
	}
}

// TextResponse creates a simple text response step.
func TextResponse(text string) Step {
	return Step{
		Type: StepResponse,
		Chunks: []claude.ResponseChunk{
			{Type: claude.ChunkTypeText, Content: text},
			{Done: true},
		},
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

// Permission creates a permission request step.
func Permission(tool, description string) Step {
	return Step{
		Type:                  StepPermission,
		PermissionTool:        tool,
		PermissionDescription: description,
	}
}

// Question creates a question request step.
func Question(questions ...mcp.Question) Step {
	return Step{
		Type:      StepQuestion,
		Questions: questions,
	}
}

// Annotate creates an annotation step.
func Annotate(text string) Step {
	return Step{
		Type:       StepAnnotate,
		Annotation: text,
	}
}

// Capture creates a frame capture step.
func Capture() Step {
	return Step{
		Type: StepCapture,
	}
}

// StartStreaming starts a streaming response without completing it.
// This leaves the session in a "waiting/streaming" state, showing the spinner.
// Use this to demonstrate parallel work - start a task in one session, then
// switch to another session while the first is still "processing".
// The optional initialText parameter adds some initial streaming content.
func StartStreaming(initialText string) Step {
	var chunks []claude.ResponseChunk
	if initialText != "" {
		chunks = []claude.ResponseChunk{
			{Type: claude.ChunkTypeText, Content: initialText},
		}
	}
	// Note: No Done chunk - this keeps the session streaming
	return Step{
		Type:   StepStartStreaming,
		Chunks: chunks,
	}
}
