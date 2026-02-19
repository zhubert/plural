package workflow

import (
	"context"
	"fmt"
	"log/slog"
)

// EventChecker checks whether an external event has fired.
type EventChecker interface {
	// CheckEvent returns whether the event has fired, along with any data.
	CheckEvent(ctx context.Context, event string, params *ParamHelper, item *WorkItemView) (fired bool, data map[string]any, err error)
}

// WorkItemView is a read-only view of a work item for the engine.
type WorkItemView struct {
	ID            string
	SessionID     string
	RepoPath      string
	Branch        string
	PRURL         string
	CurrentStep   string
	Phase         string
	StepData      map[string]any
	FeedbackRounds int
	CommentsAddressed int

	// Extra is an opaque map for passing implementation-specific data.
	Extra map[string]any
}

// StepResult is the outcome of processing a step.
type StepResult struct {
	NewStep    string         // The step to move to (empty if no change)
	NewPhase   string         // The phase within the new step
	Terminal   bool           // True if the workflow has reached a terminal state
	TerminalOK bool           // True if terminal state is succeed (false = fail)
	Data       map[string]any // Data to merge into step data
	Hooks      []HookConfig   // After-hooks to run
}

// Engine is the core workflow orchestrator.
type Engine struct {
	config       *Config
	actions      *ActionRegistry
	eventChecker EventChecker
	logger       *slog.Logger
}

// NewEngine creates a new workflow engine.
func NewEngine(cfg *Config, actions *ActionRegistry, checker EventChecker, logger *slog.Logger) *Engine {
	return &Engine{
		config:       cfg,
		actions:      actions,
		eventChecker: checker,
		logger:       logger,
	}
}

// GetStartState returns the start state name from the config.
func (e *Engine) GetStartState() string {
	return e.config.Start
}

// GetConfig returns the engine's workflow config.
func (e *Engine) GetConfig() *Config {
	return e.config
}

// ProcessStep processes the current step for a work item.
// It dispatches based on state type: succeed/fail → terminal,
// task → execute action, wait → check event.
func (e *Engine) ProcessStep(ctx context.Context, item *WorkItemView) (*StepResult, error) {
	state, ok := e.config.States[item.CurrentStep]
	if !ok {
		return nil, fmt.Errorf("unknown state %q", item.CurrentStep)
	}

	switch state.Type {
	case StateTypeSucceed:
		return &StepResult{
			Terminal:   true,
			TerminalOK: true,
			Hooks:      state.After,
		}, nil

	case StateTypeFail:
		return &StepResult{
			Terminal:   true,
			TerminalOK: false,
			Hooks:      state.After,
		}, nil

	case StateTypeTask:
		return e.processTaskState(ctx, item, state)

	case StateTypeWait:
		return e.processWaitState(ctx, item, state)

	default:
		return nil, fmt.Errorf("unsupported state type %q", state.Type)
	}
}

// processTaskState handles task state execution.
func (e *Engine) processTaskState(ctx context.Context, item *WorkItemView, state *State) (*StepResult, error) {
	action := e.actions.Get(state.Action)
	if action == nil {
		return nil, fmt.Errorf("no action registered for %q", state.Action)
	}

	params := NewParamHelper(state.Params)
	ac := &ActionContext{
		WorkItemID: item.ID,
		SessionID:  item.SessionID,
		RepoPath:   item.RepoPath,
		Branch:     item.Branch,
		Params:     params,
		Logger:     e.logger,
		Extra:      item.Extra,
	}

	result := action.Execute(ctx, ac)

	if result.Async {
		// Action spawned async work — stay on current step with async_pending phase
		return &StepResult{
			NewStep:  item.CurrentStep,
			NewPhase: "async_pending",
			Data:     result.Data,
			Hooks:    nil, // hooks run after async completes
		}, nil
	}

	if !result.Success {
		// Follow error edge if available
		if state.Error != "" {
			return &StepResult{
				NewStep:  state.Error,
				NewPhase: "idle",
				Data:     result.Data,
				Hooks:    state.After,
			}, nil
		}
		return nil, fmt.Errorf("action %q failed: %v", state.Action, result.Error)
	}

	// Success — follow next edge
	return &StepResult{
		NewStep:  state.Next,
		NewPhase: "idle",
		Data:     result.Data,
		Hooks:    state.After,
	}, nil
}

// processWaitState handles wait state event checking.
func (e *Engine) processWaitState(ctx context.Context, item *WorkItemView, state *State) (*StepResult, error) {
	if e.eventChecker == nil {
		return nil, fmt.Errorf("no event checker configured")
	}

	params := NewParamHelper(state.Params)
	fired, data, err := e.eventChecker.CheckEvent(ctx, state.Event, params, item)
	if err != nil {
		e.logger.Debug("event check error", "event", state.Event, "error", err)
		// Don't advance on error — stay in current state
		return &StepResult{
			NewStep:  item.CurrentStep,
			NewPhase: item.Phase,
		}, nil
	}

	if !fired {
		// Event hasn't fired — no change
		return &StepResult{
			NewStep:  item.CurrentStep,
			NewPhase: item.Phase,
		}, nil
	}

	// Event fired — follow next edge
	return &StepResult{
		NewStep:  state.Next,
		NewPhase: "idle",
		Data:     data,
		Hooks:    state.After,
	}, nil
}

// AdvanceAfterAsync is called when an async action (e.g., Claude worker) completes.
// It determines the next step based on success/failure.
func (e *Engine) AdvanceAfterAsync(item *WorkItemView, success bool) (*StepResult, error) {
	state, ok := e.config.States[item.CurrentStep]
	if !ok {
		return nil, fmt.Errorf("unknown state %q", item.CurrentStep)
	}

	if !success {
		if state.Error != "" {
			return &StepResult{
				NewStep:  state.Error,
				NewPhase: "idle",
				Hooks:    state.After,
			}, nil
		}
		return nil, fmt.Errorf("async action failed in state %q with no error edge", item.CurrentStep)
	}

	return &StepResult{
		NewStep:  state.Next,
		NewPhase: "idle",
		Hooks:    state.After,
	}, nil
}

// GetState returns the state definition for a given state name.
func (e *Engine) GetState(name string) *State {
	if e.config.States == nil {
		return nil
	}
	return e.config.States[name]
}

// IsTerminalState returns true if the named state is a terminal state.
func (e *Engine) IsTerminalState(name string) bool {
	state, ok := e.config.States[name]
	if !ok {
		return false
	}
	return state.Type == StateTypeSucceed || state.Type == StateTypeFail
}
