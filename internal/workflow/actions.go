package workflow

import (
	"context"
	"log/slog"
)

// Action defines the interface for executable workflow actions.
type Action interface {
	// Execute runs the action. Returns a result indicating success/failure
	// and whether the action is async (e.g., spawned a Claude worker).
	Execute(ctx context.Context, ac *ActionContext) ActionResult
}

// ActionContext provides the information an action needs to execute.
type ActionContext struct {
	WorkItemID string
	SessionID  string
	RepoPath   string
	Branch     string
	Params     *ParamHelper
	Logger     *slog.Logger

	// Extra is an opaque map for passing implementation-specific data
	// between the daemon and actions (e.g., work item reference).
	Extra map[string]any
}

// ActionResult is the outcome of an action execution.
type ActionResult struct {
	Success bool           // Whether the action succeeded
	Async   bool           // True if action spawned async work (e.g., Claude worker)
	Error   error          // Error if not successful
	Data    map[string]any // Output data to merge into step data
}

// ActionRegistry maps action names to Action implementations.
type ActionRegistry struct {
	actions map[string]Action
}

// NewActionRegistry creates a new empty action registry.
func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{
		actions: make(map[string]Action),
	}
}

// Register adds an action to the registry.
func (r *ActionRegistry) Register(name string, action Action) {
	r.actions[name] = action
}

// Get returns the action for a given name, or nil if not found.
func (r *ActionRegistry) Get(name string) Action {
	return r.actions[name]
}

// Has returns true if an action with the given name is registered.
func (r *ActionRegistry) Has(name string) bool {
	_, ok := r.actions[name]
	return ok
}
