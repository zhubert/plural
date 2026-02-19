package agent

import "github.com/zhubert/plural/internal/config"

// AgentConfig defines the configuration interface required by the agent package.
// This decouples the agent from the concrete config.Config struct, allowing it
// to depend only on the methods it actually uses.
//
// *config.Config satisfies this interface implicitly.
type AgentConfig interface {
	// Session CRUD
	GetSession(id string) *config.Session
	GetSessions() []config.Session
	AddSession(session config.Session)
	RemoveSession(id string) bool
	RemoveSessions(ids []string) int
	ClearOrphanedParentIDs(deletedIDs []string)
	MarkSessionStarted(sessionID string) bool
	MarkSessionPRCreated(sessionID string) bool
	MarkSessionPRMerged(sessionID string) bool
	MarkSessionPRClosed(sessionID string) bool
	MarkSessionMergedToParent(sessionID string) bool
	SetSessionAutonomous(sessionID string, autonomous bool) bool
	AddChildSession(supervisorID, childID string) bool
	GetChildSessions(supervisorID string) []config.Session
	GetSessionsByBroadcastGroup(groupID string) []config.Session
	UpdateSessionPRCommentCount(sessionID string, count int) bool
	UpdateSessionPRCommentsAddressedCount(sessionID string, count int) bool

	// Repo settings
	GetRepos() []string
	GetDefaultBranchPrefix() string
	GetContainerImage() string
	GetAllowedToolsForRepo(repoPath string) []string
	GetMCPServersForRepo(repoPath string) []config.MCPServer
	AddRepoAllowedTool(repoPath, tool string) bool

	// Automation settings
	GetAutoMaxTurns() int
	GetAutoMaxDurationMin() int
	GetAutoCleanupMerged() bool
	GetAutoAddressPRComments() bool
	GetAutoBroadcastPR() bool
	GetAutoMergeMethod() string
	GetIssueMaxConcurrent() int

	// Persistence
	Save() error
}
