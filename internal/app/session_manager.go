package app

import (
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/mcp"
	"github.com/zhubert/plural/internal/process"
	"time"
)

// SelectResult contains all the state needed by the UI after selecting a session.
// This allows SessionManager to handle data operations while app.go handles UI updates.
type SelectResult struct {
	Runner     claude.RunnerInterface
	Messages   []claude.Message
	HeaderName string // Branch name if custom, otherwise session name

	// State to restore
	WaitStart  time.Time
	IsWaiting  bool
	Permission *mcp.PermissionRequest
	Question   *mcp.QuestionRequest
	Streaming  string
	SavedInput string
}

// ForceResumeResult contains the result of a force resume operation.
type ForceResumeResult struct {
	Killed int
	Error  error
}

// RunnerFactory creates a runner for a session.
// This allows tests to inject mock runners.
type RunnerFactory func(sessionID, workingDir string, sessionStarted bool, initialMessages []claude.Message) claude.RunnerInterface

// defaultRunnerFactory creates real Claude runners.
func defaultRunnerFactory(sessionID, workingDir string, sessionStarted bool, initialMessages []claude.Message) claude.RunnerInterface {
	return claude.New(sessionID, workingDir, sessionStarted, initialMessages)
}

// SessionManager handles session lifecycle operations including runner management,
// state coordination, and message persistence. It encapsulates the relationship
// between sessions, runners, and per-session state.
type SessionManager struct {
	config        *config.Config
	stateManager  *SessionStateManager
	runners       map[string]claude.RunnerInterface
	runnerFactory RunnerFactory
}

// NewSessionManager creates a new session manager.
func NewSessionManager(cfg *config.Config) *SessionManager {
	return &SessionManager{
		config:        cfg,
		stateManager:  NewSessionStateManager(),
		runners:       make(map[string]claude.RunnerInterface),
		runnerFactory: defaultRunnerFactory,
	}
}

// SetRunnerFactory sets a custom runner factory (for testing).
func (sm *SessionManager) SetRunnerFactory(factory RunnerFactory) {
	sm.runnerFactory = factory
}

// StateManager returns the underlying session state manager for direct state access.
// This is needed for operations that don't warrant a full SessionManager method.
func (sm *SessionManager) StateManager() *SessionStateManager {
	return sm.stateManager
}

// GetRunner returns the runner for a session, or nil if none exists.
func (sm *SessionManager) GetRunner(sessionID string) claude.RunnerInterface {
	return sm.runners[sessionID]
}

// GetRunners returns all runners (for iteration, e.g., checking streaming status).
func (sm *SessionManager) GetRunners() map[string]claude.RunnerInterface {
	return sm.runners
}

// HasActiveStreaming returns true if any session is currently streaming.
func (sm *SessionManager) HasActiveStreaming() bool {
	for _, runner := range sm.runners {
		if runner.IsStreaming() {
			return true
		}
	}
	return false
}

// GetSession returns the session config for a given session ID.
func (sm *SessionManager) GetSession(sessionID string) *config.Session {
	sessions := sm.config.GetSessions()
	for i := range sessions {
		if sessions[i].ID == sessionID {
			return &sessions[i]
		}
	}
	return nil
}

// Select prepares a session for activation, creating or reusing a runner,
// and gathering all state needed for UI restoration. The caller (app.go)
// is responsible for saving the previous session's state before calling this.
func (sm *SessionManager) Select(sess *config.Session, previousSessionID string, previousInput string, previousStreaming string) *SelectResult {
	if sess == nil {
		return nil
	}

	// Save previous session's state if provided
	if previousSessionID != "" {
		if previousInput != "" {
			sm.stateManager.SaveInput(previousSessionID, previousInput)
			logger.Log("SessionManager: Saved input for session %s", previousSessionID)
		}
		if previousStreaming != "" {
			sm.stateManager.SaveStreaming(previousSessionID, previousStreaming)
			logger.Log("SessionManager: Saved streaming content for session %s", previousSessionID)
		}
	}

	logger.Log("SessionManager: Selecting session: id=%s, name=%s", sess.ID, sess.Name)

	// Get or create runner
	runner := sm.getOrCreateRunner(sess)

	// Determine header name (branch if custom, otherwise session name)
	headerName := sess.Name
	if sess.Branch != "" && len(sess.Branch) > 7 && sess.Branch[:7] != "plural-" {
		headerName = sess.Branch
	}

	// Build result with all state needed for UI
	result := &SelectResult{
		Runner:     runner,
		Messages:   runner.GetMessages(),
		HeaderName: headerName,
	}

	// Get waiting state
	if startTime, isWaiting := sm.stateManager.GetWaitStart(sess.ID); isWaiting {
		result.WaitStart = startTime
		result.IsWaiting = true
	}

	// Get pending permission
	result.Permission = sm.stateManager.GetPendingPermission(sess.ID)

	// Get pending question
	result.Question = sm.stateManager.GetPendingQuestion(sess.ID)

	// Get streaming content (and clear it from state manager)
	if streaming := sm.stateManager.GetStreaming(sess.ID); streaming != "" {
		result.Streaming = streaming
		sm.stateManager.ClearStreaming(sess.ID)
		logger.Log("SessionManager: Retrieved streaming content for session %s", sess.ID)
	}

	// Get saved input
	result.SavedInput = sm.stateManager.GetInput(sess.ID)

	logger.Log("SessionManager: Session selected: %s", sess.ID)
	return result
}

// getOrCreateRunner returns an existing runner or creates a new one for the session.
func (sm *SessionManager) getOrCreateRunner(sess *config.Session) claude.RunnerInterface {
	if runner, exists := sm.runners[sess.ID]; exists {
		logger.Log("SessionManager: Reusing existing runner for session %s", sess.ID)
		return runner
	}

	logger.Log("SessionManager: Creating new runner for session %s", sess.ID)

	// Load saved messages from disk
	savedMsgs, err := config.LoadSessionMessages(sess.ID)
	if err != nil {
		logger.Log("SessionManager: Warning - failed to load session messages for %s: %v", sess.ID, err)
		savedMsgs = []config.Message{}
	} else {
		logger.Log("SessionManager: Loaded %d saved messages for session %s", len(savedMsgs), sess.ID)
	}

	var initialMsgs []claude.Message
	for _, msg := range savedMsgs {
		initialMsgs = append(initialMsgs, claude.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	runner := sm.runnerFactory(sess.ID, sess.WorkTree, sess.Started, initialMsgs)
	sm.runners[sess.ID] = runner

	// Load allowed tools from config (global + per-repo)
	allowedTools := sm.config.GetAllowedToolsForRepo(sess.RepoPath)
	if len(allowedTools) > 0 {
		logger.Log("SessionManager: Loaded %d allowed tools for repo %s", len(allowedTools), sess.RepoPath)
		runner.SetAllowedTools(allowedTools)
	}

	// Load MCP servers for this session's repo
	mcpServers := sm.config.GetMCPServersForRepo(sess.RepoPath)
	if len(mcpServers) > 0 {
		logger.Log("SessionManager: Loaded %d MCP servers for session %s (repo: %s)", len(mcpServers), sess.ID, sess.RepoPath)
		var servers []claude.MCPServer
		for _, s := range mcpServers {
			servers = append(servers, claude.MCPServer{
				Name:    s.Name,
				Command: s.Command,
				Args:    s.Args,
			})
		}
		runner.SetMCPServers(servers)
	}

	return runner
}

// ForceResume kills any orphaned Claude processes for the session and clears the error state.
// Returns the number of processes killed and any error encountered.
func (sm *SessionManager) ForceResume(sess *config.Session) ForceResumeResult {
	logger.Log("SessionManager: Force-resuming session %s", sess.ID)

	// Try to kill orphaned processes
	killed, err := process.KillClaudeProcesses(sess.ID)
	if err != nil {
		logger.Log("SessionManager: Error killing orphaned processes for session %s: %v", sess.ID, err)
		return ForceResumeResult{Killed: killed, Error: err}
	}

	// Clear the error state
	sm.stateManager.SetSessionInUseError(sess.ID, false)

	// Clear the old runner from cache so a fresh one will be created
	if oldRunner, exists := sm.runners[sess.ID]; exists {
		oldRunner.Stop()
		delete(sm.runners, sess.ID)
	}

	if killed > 0 {
		logger.Log("SessionManager: Killed %d orphaned processes for session %s", killed, sess.ID)
	} else {
		logger.Log("SessionManager: No orphaned processes found for session %s, cleared error state", sess.ID)
	}

	return ForceResumeResult{Killed: killed, Error: nil}
}

// SaveMessages saves the current messages from a runner to disk.
func (sm *SessionManager) SaveMessages(sessionID string) {
	runner, exists := sm.runners[sessionID]
	if !exists || runner == nil {
		return
	}

	msgs := runner.GetMessages()
	var configMsgs []config.Message
	for _, msg := range msgs {
		configMsgs = append(configMsgs, config.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	config.SaveSessionMessages(sessionID, configMsgs, config.MaxSessionMessageLines)
}

// SaveRunnerMessages saves messages for a specific runner (used when runner reference is already available).
func (sm *SessionManager) SaveRunnerMessages(sessionID string, runner claude.RunnerInterface) {
	if runner == nil {
		return
	}

	msgs := runner.GetMessages()
	var configMsgs []config.Message
	for _, msg := range msgs {
		configMsgs = append(configMsgs, config.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	config.SaveSessionMessages(sessionID, configMsgs, config.MaxSessionMessageLines)
}

// DeleteSession cleans up all resources for a deleted session.
// Returns the runner if it existed (so caller can check if it was active).
func (sm *SessionManager) DeleteSession(sessionID string) claude.RunnerInterface {
	// Stop and remove runner
	var runner claude.RunnerInterface
	if r, exists := sm.runners[sessionID]; exists {
		logger.Log("SessionManager: Stopping runner for deleted session %s", sessionID)
		r.Stop()
		runner = r
		delete(sm.runners, sessionID)
	}

	// Clean up all per-session state (this also cancels in-progress operations)
	sm.stateManager.Delete(sessionID)

	return runner
}

// AddAllowedTool adds a tool to the allowed list for a session's repo and updates the runner.
func (sm *SessionManager) AddAllowedTool(sessionID string, tool string) {
	sess := sm.GetSession(sessionID)
	if sess == nil {
		return
	}

	sm.config.AddRepoAllowedTool(sess.RepoPath, tool)
	sm.config.Save()

	if runner, exists := sm.runners[sessionID]; exists {
		runner.AddAllowedTool(tool)
	}

	logger.Log("SessionManager: Added tool %s to allowed list for repo %s", tool, sess.RepoPath)
}

// SetRunner sets a runner for a session (used when manually creating runners).
func (sm *SessionManager) SetRunner(sessionID string, runner claude.RunnerInterface) {
	sm.runners[sessionID] = runner
}

// Shutdown stops all runners gracefully. This should be called when the
// application is exiting to ensure all Claude CLI processes are terminated
// and resources are cleaned up.
func (sm *SessionManager) Shutdown() {
	logger.Log("SessionManager: Shutting down all runners (%d total)", len(sm.runners))
	for sessionID, runner := range sm.runners {
		logger.Log("SessionManager: Stopping runner for session %s", sessionID)
		runner.Stop()
	}
	sm.runners = make(map[string]claude.RunnerInterface)
	logger.Log("SessionManager: Shutdown complete")
}
