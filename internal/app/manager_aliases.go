package app

import "github.com/zhubert/plural-core/manager"

// Type aliases for manager package types, allowing the app package
// to reference them without qualifying with "manager." everywhere.
type (
	SessionManager       = manager.SessionManager
	SessionManagerConfig = manager.SessionManagerConfig
	SessionStateManager  = manager.SessionStateManager
	SessionState         = manager.SessionState
	SelectResult         = manager.SelectResult
	DiffStats            = manager.DiffStats
	RunnerFactory        = manager.RunnerFactory
	ToolUseRollupState   = manager.ToolUseRollupState
	ToolUseItemState     = manager.ToolUseItemState
)

// Re-export constructor functions.
var (
	NewSessionManager      = manager.NewSessionManager
	NewSessionStateManager = manager.NewSessionStateManager
)
