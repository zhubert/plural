package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/mcp"
)

// DiffStats holds file change statistics for the header display
type DiffStats struct {
	FilesChanged int
	Additions    int
	Deletions    int
}

// SelectResult contains all the state needed by the UI after selecting a session.
// This allows SessionManager to handle data operations while app.go handles UI updates.
type SelectResult struct {
	Runner     claude.RunnerInterface
	Messages   []claude.Message
	HeaderName string     // Branch name if custom, otherwise session name
	BaseBranch string     // Base branch this session was created from
	DiffStats  *DiffStats // Git diff statistics for the worktree

	// State to restore
	WaitStart     time.Time
	IsWaiting     bool
	Permission    *mcp.PermissionRequest
	Question      *mcp.QuestionRequest
	PlanApproval  *mcp.PlanApprovalRequest
	TodoList      *claude.TodoList
	Streaming     string
	SavedInput    string
	SubagentModel string // Active subagent model (empty if none)
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
	config           *config.Config
	stateManager     *SessionStateManager
	runners          map[string]claude.RunnerInterface
	runnerFactory    RunnerFactory
	skipMessageLoad  bool // Skip loading messages from disk (for demos/tests)
	gitService       *git.GitService
	mu               sync.RWMutex // Protects runners map
}

// NewSessionManager creates a new session manager.
func NewSessionManager(cfg *config.Config, gitSvc *git.GitService) *SessionManager {
	return &SessionManager{
		config:        cfg,
		stateManager:  NewSessionStateManager(),
		runners:       make(map[string]claude.RunnerInterface),
		runnerFactory: defaultRunnerFactory,
		gitService:    gitSvc,
	}
}

// SetGitService sets the git service (for testing/demos).
func (sm *SessionManager) SetGitService(svc *git.GitService) {
	sm.gitService = svc
}

// SetRunnerFactory sets a custom runner factory (for testing).
func (sm *SessionManager) SetRunnerFactory(factory RunnerFactory) {
	sm.runnerFactory = factory
}

// SetSkipMessageLoad configures whether to skip loading messages from disk.
// This is useful for demos and tests where clean state is needed.
func (sm *SessionManager) SetSkipMessageLoad(skip bool) {
	sm.skipMessageLoad = skip
}

// StateManager returns the underlying session state manager for direct state access.
// This is needed for operations that don't warrant a full SessionManager method.
func (sm *SessionManager) StateManager() *SessionStateManager {
	return sm.stateManager
}

// GetRunner returns the runner for a session, or nil if none exists.
func (sm *SessionManager) GetRunner(sessionID string) claude.RunnerInterface {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.runners[sessionID]
}

// GetRunners returns a copy of all runners (for safe iteration).
// The returned map is a snapshot - concurrent modifications to the original
// will not affect it.
func (sm *SessionManager) GetRunners() map[string]claude.RunnerInterface {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	copy := make(map[string]claude.RunnerInterface, len(sm.runners))
	for k, v := range sm.runners {
		copy[k] = v
	}
	return copy
}

// HasActiveStreaming returns true if any session is currently streaming.
func (sm *SessionManager) HasActiveStreaming() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
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
		if previousInput != "" || previousStreaming != "" {
			prevState := sm.stateManager.GetOrCreate(previousSessionID)
			prevLog := logger.WithSession(previousSessionID)
			if previousInput != "" {
				prevState.InputText = previousInput
				prevLog.Debug("saved input for session")
			}
			if previousStreaming != "" {
				prevState.StreamingContent = previousStreaming
				prevLog.Debug("saved streaming content for session")
			}
		}
	}

	log := logger.WithSession(sess.ID)
	log.Debug("selecting session", "name", sess.Name)

	// Get or create runner
	runner := sm.GetOrCreateRunner(sess)

	// Determine header name (branch if custom, otherwise session name)
	headerName := sess.Name
	if sess.Branch != "" && len(sess.Branch) > 7 && sess.Branch[:7] != "plural-" {
		headerName = sess.Branch
	}

	// Get diff stats for the worktree
	var diffStats *DiffStats
	if sess.WorkTree != "" && sm.gitService != nil {
		ctx := context.Background()
		if gitStats, err := sm.gitService.GetDiffStats(ctx, sess.WorkTree); err == nil {
			diffStats = &DiffStats{
				FilesChanged: gitStats.FilesChanged,
				Additions:    gitStats.Additions,
				Deletions:    gitStats.Deletions,
			}
		} else {
			log.Debug("failed to get diff stats", "workTree", sess.WorkTree, "error", err)
		}
	}

	// Build result with all state needed for UI
	result := &SelectResult{
		Runner:     runner,
		Messages:   runner.GetMessages(),
		HeaderName: headerName,
		BaseBranch: sess.BaseBranch,
		DiffStats:  diffStats,
	}

	// Get state for all fields - use WithLock to get streaming state atomically
	if state := sm.stateManager.GetIfExists(sess.ID); state != nil {
		// Get pending permission
		result.Permission = state.GetPendingPermission()

		// Get pending question
		result.Question = state.GetPendingQuestion()

		// Get pending plan approval
		result.PlanApproval = state.GetPendingPlanApproval()

		// Get todo list
		result.TodoList = state.GetCurrentTodoList()

		// Get streaming state atomically - this ensures IsWaiting, WaitStart,
		// StreamingContent, SubagentModel, and StreamingStartTime are all read consistently
		state.WithLock(func(s *SessionState) {
			result.IsWaiting = s.IsWaiting
			result.SubagentModel = s.SubagentModel
			// Always use StreamingStartTime for elapsed time display - it's set when
			// streaming starts and preserved throughout (WaitStart gets cleared when
			// first chunk arrives, but we still need elapsed time for the UI)
			if s.IsWaiting || s.StreamingContent != "" {
				result.WaitStart = s.StreamingStartTime
			}
			if s.StreamingContent != "" {
				result.Streaming = s.StreamingContent
				s.StreamingContent = ""
				log.Debug("retrieved streaming content for session")
			}
		})

		// Get saved input
		result.SavedInput = state.GetInputText()
	}

	log.Debug("session selected")
	return result
}

// GetOrCreateRunner returns an existing runner or creates a new one for the session.
// Uses double-checked locking to prevent race conditions where multiple goroutines
// could create duplicate runners for the same session.
// This is safe to call concurrently from multiple goroutines.
func (sm *SessionManager) GetOrCreateRunner(sess *config.Session) claude.RunnerInterface {
	log := logger.WithSession(sess.ID)

	// Fast path: check with read lock
	sm.mu.RLock()
	if runner, exists := sm.runners[sess.ID]; exists {
		sm.mu.RUnlock()
		log.Debug("reusing existing runner")
		return runner
	}
	sm.mu.RUnlock()

	// Slow path: acquire write lock and double-check before creating
	sm.mu.Lock()

	// Double-check: another goroutine may have created the runner while we waited for the lock
	if runner, exists := sm.runners[sess.ID]; exists {
		sm.mu.Unlock()
		log.Debug("reusing existing runner (created by another goroutine)")
		return runner
	}

	log.Debug("creating new runner")

	var initialMsgs []claude.Message

	// Load saved messages from disk (unless skipped for demos/tests)
	// Note: This is done while holding the lock to ensure only one goroutine loads messages
	if !sm.skipMessageLoad {
		savedMsgs, err := config.LoadSessionMessages(sess.ID)
		if err != nil {
			log.Warn("failed to load session messages", "error", err)
		} else {
			log.Debug("loaded saved messages", "count", len(savedMsgs))
			for _, msg := range savedMsgs {
				initialMsgs = append(initialMsgs, claude.Message{
					Role:    msg.Role,
					Content: msg.Content,
				})
			}
		}
	} else {
		log.Debug("skipping message load (demo/test mode)")
	}

	runner := sm.runnerFactory(sess.ID, sess.WorkTree, sess.Started, initialMsgs)
	sm.runners[sess.ID] = runner
	sm.mu.Unlock()

	// If this is a forked session that hasn't started yet, set up to fork from parent
	// to inherit the parent's conversation history in Claude.
	// We only fork if the parent session was actually started (has a Claude session to fork from).
	// If the parent was never started, there's no Claude session file to fork from.
	if !sess.Started && sess.ParentID != "" {
		parentSess := sm.config.GetSession(sess.ParentID)
		if parentSess != nil && parentSess.Started {
			// Copy Claude's session JSONL file from parent's project dir to child's project dir.
			// This is required because Claude CLI stores sessions by project path (worktree),
			// so the child can't find the parent session unless we copy it.
			if err := copyClaudeSessionForFork(sess.ParentID, parentSess.WorkTree, sess.WorkTree); err != nil {
				log.Warn("failed to copy Claude session for fork, starting as new session", "error", err)
			} else {
				runner.SetForkFromSession(sess.ParentID)
				log.Debug("session will fork from parent", "parentID", sess.ParentID)
			}
		} else if parentSess == nil {
			log.Debug("parent session not found, starting as new session", "parentID", sess.ParentID)
		} else {
			log.Debug("parent session not started yet, starting as new session", "parentID", sess.ParentID)
		}
	}

	// Load allowed tools from config (global + per-repo)
	allowedTools := sm.config.GetAllowedToolsForRepo(sess.RepoPath)
	if len(allowedTools) > 0 {
		log.Debug("loaded allowed tools", "count", len(allowedTools), "repo", sess.RepoPath)
		runner.SetAllowedTools(allowedTools)
	}

	// Load MCP servers for this session's repo
	mcpServers := sm.config.GetMCPServersForRepo(sess.RepoPath)
	if len(mcpServers) > 0 {
		log.Debug("loaded MCP servers", "count", len(mcpServers), "repo", sess.RepoPath)
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

// SaveMessages saves the current messages from a runner to disk.
func (sm *SessionManager) SaveMessages(sessionID string) {
	sm.mu.RLock()
	runner, exists := sm.runners[sessionID]
	sm.mu.RUnlock()
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
	log := logger.WithSession(sessionID)
	// Stop and remove runner
	sm.mu.Lock()
	var runner claude.RunnerInterface
	if r, exists := sm.runners[sessionID]; exists {
		log.Debug("stopping runner for deleted session")
		r.Stop()
		runner = r
		delete(sm.runners, sessionID)
	}
	sm.mu.Unlock()

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

	sm.mu.RLock()
	runner, exists := sm.runners[sessionID]
	sm.mu.RUnlock()
	if exists {
		runner.AddAllowedTool(tool)
	}

	logger.WithSession(sessionID).Debug("added tool to allowed list", "tool", tool, "repo", sess.RepoPath)
}

// SetRunner sets a runner for a session (used when manually creating runners).
func (sm *SessionManager) SetRunner(sessionID string, runner claude.RunnerInterface) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.runners[sessionID] = runner
}

// Shutdown stops all runners gracefully. This should be called when the
// application is exiting to ensure all Claude CLI processes are terminated
// and resources are cleaned up.
func (sm *SessionManager) Shutdown() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	log := logger.WithComponent("SessionManager")
	log.Info("shutting down all runners", "count", len(sm.runners))
	for sessionID, runner := range sm.runners {
		logger.WithSession(sessionID).Debug("stopping runner")
		runner.Stop()
	}
	sm.runners = make(map[string]claude.RunnerInterface)
	log.Info("shutdown complete")
}

// copyClaudeSessionForFork copies Claude's session JSONL file from the parent's
// project directory to the child's project directory so that --fork-session works.
// Claude CLI stores sessions in ~/.claude/projects/<escaped-path>/<session-id>.jsonl
// and when forking with --resume <parent-id> --fork-session, it looks for the parent
// session in the CURRENT working directory's project path, not the parent's.
func copyClaudeSessionForFork(parentSessionID, parentWorktree, childWorktree string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// Claude escapes paths by replacing "/" and "." with "-"
	escapePath := func(path string) string {
		escaped := strings.ReplaceAll(path, "/", "-")
		return strings.ReplaceAll(escaped, ".", "-")
	}

	parentEscaped := escapePath(parentWorktree)
	childEscaped := escapePath(childWorktree)

	claudeProjectsDir := filepath.Join(homeDir, ".claude", "projects")
	parentProjectDir := filepath.Join(claudeProjectsDir, parentEscaped)
	childProjectDir := filepath.Join(claudeProjectsDir, childEscaped)

	srcFile := filepath.Join(parentProjectDir, parentSessionID+".jsonl")
	dstFile := filepath.Join(childProjectDir, parentSessionID+".jsonl")

	// Check if source file exists
	if _, err := os.Stat(srcFile); os.IsNotExist(err) {
		return err
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(childProjectDir, 0700); err != nil {
		return err
	}

	// Copy the file
	src, err := os.Open(srcFile)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstFile)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	logger.WithSession(parentSessionID).Debug("copied Claude session for fork", "from", parentProjectDir, "to", childProjectDir)
	return nil
}
