package demo

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/app"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/mcp"
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
		CaptureEveryStep:   true,
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

// Run executes a scenario and returns the captured frames.
func (e *Executor) Run(scenario *Scenario) ([]Frame, error) {
	if err := scenario.Validate(); err != nil {
		return nil, fmt.Errorf("invalid scenario: %w", err)
	}

	// Initialize the model
	if err := e.setup(scenario); err != nil {
		return nil, fmt.Errorf("setup failed: %w", err)
	}

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

	// Set size
	e.model.Update(tea.WindowSizeMsg{
		Width:  scenario.Width,
		Height: scenario.Height,
	})

	// Inject mock runner factory
	e.model.SessionMgr().SetRunnerFactory(e.factory.Create)

	return nil
}

// executeStep executes a single demo step.
func (e *Executor) executeStep(index int, step Step) error {
	switch step.Type {
	case StepWait:
		e.captureFrame(index, step.Duration)

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

		for _, chunk := range step.Chunks {
			e.simulateResponse(session.ID, chunk)
			if e.config.CaptureEveryStep && chunk.Type == claude.ChunkTypeText {
				e.captureFrame(index, e.config.ResponseChunkDelay)
			}
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

	case StepQuestion:
		session := e.model.ActiveSession()
		if session == nil {
			return fmt.Errorf("no active session for question")
		}
		e.simulateQuestion(session.ID, step.Questions)
		e.captureFrame(index, 300*time.Millisecond)

	case StepAnnotate:
		e.currentAnnotation = step.Annotation
		// Don't capture, annotation applies to next frame

	case StepCapture:
		e.captureFrame(index, 0)
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
