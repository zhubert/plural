package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/zhubert/plural/internal/paths"
)

// Config holds the application configuration
type Config struct {
	Repos             []string               `json:"repos"`
	Sessions          []Session              `json:"sessions"`
	MCPServers        []MCPServer            `json:"mcp_servers,omitempty"`          // Global MCP servers
	RepoMCP           map[string][]MCPServer `json:"repo_mcp,omitempty"`             // Per-repo MCP servers
	AllowedTools      []string               `json:"allowed_tools,omitempty"`        // Global allowed tools
	RepoAllowedTools  map[string][]string    `json:"repo_allowed_tools,omitempty"`   // Per-repo allowed tools
	RepoSquashOnMerge map[string]bool        `json:"repo_squash_on_merge,omitempty"` // Per-repo squash-on-merge setting
	RepoAsanaProject  map[string]string      `json:"repo_asana_project,omitempty"`   // Per-repo Asana project GID mapping
	RepoLinearTeam    map[string]string      `json:"repo_linear_team,omitempty"`    // Per-repo Linear team ID mapping
	ContainerImage    string                 `json:"container_image,omitempty"`      // Container image for containerized sessions

	WelcomeShown         bool   `json:"welcome_shown,omitempty"`         // Whether welcome modal has been shown
	LastSeenVersion      string `json:"last_seen_version,omitempty"`     // Last version user has seen changelog for
	Theme                string `json:"theme,omitempty"`                 // UI theme name (e.g., "dark-purple", "nord")
	DefaultBranchPrefix  string `json:"default_branch_prefix,omitempty"` // Prefix for auto-generated branch names (e.g., "zhubert/")
	NotificationsEnabled bool   `json:"notifications_enabled,omitempty"` // Desktop notifications when Claude completes

	// Automation settings
	AutoMaxTurns       int            `json:"auto_max_turns,omitempty"`        // Max autonomous turns before stopping (default 50)
	AutoMaxDurationMin int            `json:"auto_max_duration_min,omitempty"` // Max autonomous duration in minutes (default 30)
	AutoCleanupMerged  bool           `json:"auto_cleanup_merged,omitempty"`   // Auto-cleanup sessions when PR merged/closed
	AutoAddressPRComments bool         `json:"auto_address_pr_comments,omitempty"` // Auto-fetch and address new PR review comments
	AutoBroadcastPR    bool           `json:"auto_broadcast_pr,omitempty"`     // Auto-create PRs when all broadcast sessions complete
	RepoAutoMerge      map[string]bool  `json:"repo_auto_merge,omitempty"`      // Per-repo auto-merge after CI passes
	RepoIssuePolling   map[string]bool  `json:"repo_issue_polling,omitempty"`   // Per-repo issue polling enabled
	RepoIssueLabels    map[string]string `json:"repo_issue_labels,omitempty"`    // Per-repo issue filter label (e.g., "autonomous ready")
	IssueMaxConcurrent int            `json:"issue_max_concurrent,omitempty"`  // Max concurrent auto-sessions from issues (default 3)

	// Workspace organization
	Workspaces        []Workspace `json:"workspaces,omitempty"`
	ActiveWorkspaceID string      `json:"active_workspace_id,omitempty"`

	// Preview state - tracks when a session's branch is checked out in the main repo
	PreviewSessionID      string `json:"preview_session_id,omitempty"`      // Session ID currently being previewed (empty if none)
	PreviewPreviousBranch string `json:"preview_previous_branch,omitempty"` // Branch that was checked out before preview started
	PreviewRepoPath       string `json:"preview_repo_path,omitempty"`       // Path to the main repo where preview is active

	mu       sync.RWMutex
	filePath string
}

// Load reads the config from disk, or creates a new one if it doesn't exist
func Load() (*Config, error) {
	path, err := paths.ConfigFilePath()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Repos:            []string{},
		Sessions:         []Session{},
		MCPServers:       []MCPServer{},
		RepoMCP:          make(map[string][]MCPServer),
		AllowedTools:     []string{},
		RepoAllowedTools: make(map[string][]string),
		filePath:         path,
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Ensure slices and maps are initialized (not nil) after unmarshaling
	// This must happen before Validate() since Validate() only reads
	cfg.ensureInitialized()

	// Validate loaded config
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// ensureInitialized ensures all slices and maps are initialized (not nil).
// This is called during Load() after unmarshaling, and must be called
// before Validate() since Validate() only reads.
//
// Thread-safety: This method is NOT thread-safe and must only be called
// during single-threaded initialization (i.e., from Load() before the Config
// is shared across goroutines). This is safe because Load() is called once
// at application startup before any concurrent access is possible.
func (c *Config) ensureInitialized() {
	if c.Repos == nil {
		c.Repos = []string{}
	}
	if c.Sessions == nil {
		c.Sessions = []Session{}
	}
	if c.MCPServers == nil {
		c.MCPServers = []MCPServer{}
	}
	if c.RepoMCP == nil {
		c.RepoMCP = make(map[string][]MCPServer)
	}
	if c.AllowedTools == nil {
		c.AllowedTools = []string{}
	}
	if c.RepoAllowedTools == nil {
		c.RepoAllowedTools = make(map[string][]string)
	}
	if c.RepoSquashOnMerge == nil {
		c.RepoSquashOnMerge = make(map[string]bool)
	}
	if c.RepoAsanaProject == nil {
		c.RepoAsanaProject = make(map[string]string)
	}
	if c.RepoLinearTeam == nil {
		c.RepoLinearTeam = make(map[string]string)
	}
	if c.Workspaces == nil {
		c.Workspaces = []Workspace{}
	}
	if c.RepoAutoMerge == nil {
		c.RepoAutoMerge = make(map[string]bool)
	}
	if c.RepoIssuePolling == nil {
		c.RepoIssuePolling = make(map[string]bool)
	}
	if c.RepoIssueLabels == nil {
		c.RepoIssueLabels = make(map[string]string)
	}
}

// Validate checks that the config is internally consistent.
// This is a read-only operation - call ensureInitialized() first if needed.
func (c *Config) Validate() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check for duplicate session IDs
	seenIDs := make(map[string]bool)
	for _, sess := range c.Sessions {
		if sess.ID == "" {
			return fmt.Errorf("session with empty ID found")
		}
		if seenIDs[sess.ID] {
			return fmt.Errorf("duplicate session ID: %s", sess.ID)
		}
		seenIDs[sess.ID] = true

		// Validate session fields
		if sess.RepoPath == "" {
			return fmt.Errorf("session %s has empty repo path", sess.ID)
		}
		if sess.WorkTree == "" {
			return fmt.Errorf("session %s has empty worktree path", sess.ID)
		}
		if sess.Branch == "" {
			return fmt.Errorf("session %s has empty branch", sess.ID)
		}
	}

	// Check for duplicate repos
	seenRepos := make(map[string]bool)
	for _, repo := range c.Repos {
		if repo == "" {
			return fmt.Errorf("empty repo path found")
		}
		if seenRepos[repo] {
			return fmt.Errorf("duplicate repo: %s", repo)
		}
		seenRepos[repo] = true
	}

	return nil
}

// Save writes the config to disk
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	dir, err := paths.ConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.filePath, data, 0644)
}

// SetFilePath sets the config file path (for testing).
func (c *Config) SetFilePath(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.filePath = path
}

// AddRepo adds a repository path if it doesn't already exist
func (c *Config) AddRepo(path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already exists
	for _, r := range c.Repos {
		if r == path {
			return false
		}
	}

	c.Repos = append(c.Repos, path)
	return true
}

// RemoveRepo removes a repository from the config.
// Returns true if the repo was found and removed, false otherwise.
func (c *Config) RemoveRepo(path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, r := range c.Repos {
		if r == path {
			c.Repos = append(c.Repos[:i], c.Repos[i+1:]...)
			return true
		}
	}
	return false
}

// GetRepos returns a copy of the repos slice
func (c *Config) GetRepos() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	repos := make([]string, len(c.Repos))
	copy(repos, c.Repos)
	return repos
}

// HasSeenWelcome returns whether the welcome modal has been shown
func (c *Config) HasSeenWelcome() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.WelcomeShown
}

// MarkWelcomeShown marks the welcome modal as shown
func (c *Config) MarkWelcomeShown() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.WelcomeShown = true
}

// GetLastSeenVersion returns the last version the user has seen
func (c *Config) GetLastSeenVersion() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.LastSeenVersion
}

// SetLastSeenVersion sets the last version the user has seen
func (c *Config) SetLastSeenVersion(version string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastSeenVersion = version
}

// GetTheme returns the current theme name
func (c *Config) GetTheme() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Theme
}

// SetTheme sets the current theme name
func (c *Config) SetTheme(theme string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Theme = theme
}

// GetDefaultBranchPrefix returns the default branch prefix
func (c *Config) GetDefaultBranchPrefix() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.DefaultBranchPrefix
}

// SetDefaultBranchPrefix sets the default branch prefix
func (c *Config) SetDefaultBranchPrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.DefaultBranchPrefix = prefix
}

// GetNotificationsEnabled returns whether desktop notifications are enabled
func (c *Config) GetNotificationsEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.NotificationsEnabled
}

// SetNotificationsEnabled sets whether desktop notifications are enabled
func (c *Config) SetNotificationsEnabled(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.NotificationsEnabled = enabled
}

// GetPreviewState returns the current preview state (session ID, previous branch, repo path).
// Returns empty strings if no preview is active.
func (c *Config) GetPreviewState() (sessionID, previousBranch, repoPath string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.PreviewSessionID, c.PreviewPreviousBranch, c.PreviewRepoPath
}

// IsPreviewActive returns true if a preview is currently active
func (c *Config) IsPreviewActive() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.PreviewSessionID != ""
}

// GetPreviewSessionID returns the session ID being previewed, or empty string if none
func (c *Config) GetPreviewSessionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.PreviewSessionID
}

// StartPreview records that a session's branch is being previewed in the main repo.
// previousBranch is what was checked out before (to restore later).
func (c *Config) StartPreview(sessionID, previousBranch, repoPath string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.PreviewSessionID = sessionID
	c.PreviewPreviousBranch = previousBranch
	c.PreviewRepoPath = repoPath
}

// EndPreview clears the preview state
func (c *Config) EndPreview() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.PreviewSessionID = ""
	c.PreviewPreviousBranch = ""
	c.PreviewRepoPath = ""
}

// GetSquashOnMerge returns whether squash-on-merge is enabled for a repo
func (c *Config) GetSquashOnMerge(repoPath string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.RepoSquashOnMerge == nil {
		return false
	}
	return c.RepoSquashOnMerge[repoPath]
}

// SetSquashOnMerge sets whether squash-on-merge is enabled for a repo
func (c *Config) SetSquashOnMerge(repoPath string, enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.RepoSquashOnMerge == nil {
		c.RepoSquashOnMerge = make(map[string]bool)
	}
	if enabled {
		c.RepoSquashOnMerge[repoPath] = true
	} else {
		delete(c.RepoSquashOnMerge, repoPath)
	}
}

// GetAsanaProject returns the Asana project GID for a repo, or empty string if not configured
func (c *Config) GetAsanaProject(repoPath string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.RepoAsanaProject == nil {
		return ""
	}
	return c.RepoAsanaProject[repoPath]
}

// SetAsanaProject sets the Asana project GID for a repo
func (c *Config) SetAsanaProject(repoPath, projectGID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.RepoAsanaProject == nil {
		c.RepoAsanaProject = make(map[string]string)
	}
	if projectGID == "" {
		delete(c.RepoAsanaProject, repoPath)
	} else {
		c.RepoAsanaProject[repoPath] = projectGID
	}
}

// HasAsanaProject returns true if the repo has an Asana project configured
func (c *Config) HasAsanaProject(repoPath string) bool {
	return c.GetAsanaProject(repoPath) != ""
}

// GetLinearTeam returns the Linear team ID for a repo, or empty string if not configured
func (c *Config) GetLinearTeam(repoPath string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.RepoLinearTeam == nil {
		return ""
	}
	return c.RepoLinearTeam[repoPath]
}

// SetLinearTeam sets the Linear team ID for a repo
func (c *Config) SetLinearTeam(repoPath, teamID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.RepoLinearTeam == nil {
		c.RepoLinearTeam = make(map[string]string)
	}
	if teamID == "" {
		delete(c.RepoLinearTeam, repoPath)
	} else {
		c.RepoLinearTeam[repoPath] = teamID
	}
}

// HasLinearTeam returns true if the repo has a Linear team configured
func (c *Config) HasLinearTeam(repoPath string) bool {
	return c.GetLinearTeam(repoPath) != ""
}

// GetContainerImage returns the container image name, defaulting to "ghcr.io/zhubert/plural-claude"
func (c *Config) GetContainerImage() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.ContainerImage == "" {
		return "ghcr.io/zhubert/plural-claude"
	}
	return c.ContainerImage
}

// SetContainerImage sets the container image name
func (c *Config) SetContainerImage(image string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ContainerImage = image
}

// GetAutoMaxTurns returns the max autonomous turns, defaulting to 50
func (c *Config) GetAutoMaxTurns() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.AutoMaxTurns <= 0 {
		return 50
	}
	return c.AutoMaxTurns
}

// SetAutoMaxTurns sets the max autonomous turns
func (c *Config) SetAutoMaxTurns(turns int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.AutoMaxTurns = turns
}

// GetAutoMaxDurationMin returns the max autonomous duration in minutes, defaulting to 30
func (c *Config) GetAutoMaxDurationMin() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.AutoMaxDurationMin <= 0 {
		return 30
	}
	return c.AutoMaxDurationMin
}

// SetAutoMaxDurationMin sets the max autonomous duration in minutes
func (c *Config) SetAutoMaxDurationMin(min int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.AutoMaxDurationMin = min
}

// GetAutoCleanupMerged returns whether auto-cleanup of merged sessions is enabled
func (c *Config) GetAutoCleanupMerged() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.AutoCleanupMerged
}

// SetAutoCleanupMerged sets whether auto-cleanup of merged sessions is enabled
func (c *Config) SetAutoCleanupMerged(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.AutoCleanupMerged = enabled
}

// GetAutoAddressPRComments returns whether auto-addressing PR comments is enabled
func (c *Config) GetAutoAddressPRComments() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.AutoAddressPRComments
}

// SetAutoAddressPRComments sets whether auto-addressing PR comments is enabled
func (c *Config) SetAutoAddressPRComments(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.AutoAddressPRComments = enabled
}

// GetAutoBroadcastPR returns whether auto-creating PRs for broadcast groups is enabled
func (c *Config) GetAutoBroadcastPR() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.AutoBroadcastPR
}

// SetAutoBroadcastPR sets whether auto-creating PRs for broadcast groups is enabled
func (c *Config) SetAutoBroadcastPR(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.AutoBroadcastPR = enabled
}

// GetRepoAutoMerge returns whether auto-merge is enabled for a repo
func (c *Config) GetRepoAutoMerge(repoPath string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.RepoAutoMerge == nil {
		return false
	}
	return c.RepoAutoMerge[repoPath]
}

// SetRepoAutoMerge sets whether auto-merge is enabled for a repo
func (c *Config) SetRepoAutoMerge(repoPath string, enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.RepoAutoMerge == nil {
		c.RepoAutoMerge = make(map[string]bool)
	}
	if enabled {
		c.RepoAutoMerge[repoPath] = true
	} else {
		delete(c.RepoAutoMerge, repoPath)
	}
}

// GetRepoIssuePolling returns whether issue polling is enabled for a repo
func (c *Config) GetRepoIssuePolling(repoPath string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.RepoIssuePolling == nil {
		return false
	}
	return c.RepoIssuePolling[repoPath]
}

// SetRepoIssuePolling sets whether issue polling is enabled for a repo
func (c *Config) SetRepoIssuePolling(repoPath string, enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.RepoIssuePolling == nil {
		c.RepoIssuePolling = make(map[string]bool)
	}
	if enabled {
		c.RepoIssuePolling[repoPath] = true
	} else {
		delete(c.RepoIssuePolling, repoPath)
	}
}

// GetRepoIssueLabels returns the issue filter label for a repo
func (c *Config) GetRepoIssueLabels(repoPath string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.RepoIssueLabels == nil {
		return ""
	}
	return c.RepoIssueLabels[repoPath]
}

// SetRepoIssueLabels sets the issue filter label for a repo
func (c *Config) SetRepoIssueLabels(repoPath, label string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.RepoIssueLabels == nil {
		c.RepoIssueLabels = make(map[string]string)
	}
	if label == "" {
		delete(c.RepoIssueLabels, repoPath)
	} else {
		c.RepoIssueLabels[repoPath] = label
	}
}

// GetIssueMaxConcurrent returns the max concurrent auto-sessions, defaulting to 3
func (c *Config) GetIssueMaxConcurrent() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.IssueMaxConcurrent <= 0 {
		return 3
	}
	return c.IssueMaxConcurrent
}

// SetIssueMaxConcurrent sets the max concurrent auto-sessions
func (c *Config) SetIssueMaxConcurrent(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.IssueMaxConcurrent = n
}

// GetWorkspaces returns a copy of the workspaces slice
func (c *Config) GetWorkspaces() []Workspace {
	c.mu.RLock()
	defer c.mu.RUnlock()

	workspaces := make([]Workspace, len(c.Workspaces))
	copy(workspaces, c.Workspaces)
	return workspaces
}

// AddWorkspace adds a new workspace. Returns false if a workspace with the same name already exists.
func (c *Config) AddWorkspace(ws Workspace) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, existing := range c.Workspaces {
		if existing.Name == ws.Name {
			return false
		}
	}
	c.Workspaces = append(c.Workspaces, ws)
	return true
}

// RemoveWorkspace removes a workspace by ID. Also clears WorkspaceID on all sessions
// in that workspace and clears ActiveWorkspaceID if it was the active workspace.
// Returns false if the workspace was not found.
func (c *Config) RemoveWorkspace(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	found := false
	for i, ws := range c.Workspaces {
		if ws.ID == id {
			c.Workspaces = append(c.Workspaces[:i], c.Workspaces[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return false
	}

	// Clear workspace assignment from all sessions that were in this workspace
	for i := range c.Sessions {
		if c.Sessions[i].WorkspaceID == id {
			c.Sessions[i].WorkspaceID = ""
		}
	}

	// Clear active workspace if it was this one
	if c.ActiveWorkspaceID == id {
		c.ActiveWorkspaceID = ""
	}

	return true
}

// RenameWorkspace renames a workspace. Returns false if the workspace was not found
// or if a workspace with the new name already exists.
func (c *Config) RenameWorkspace(id, newName string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check for name conflict
	for _, ws := range c.Workspaces {
		if ws.Name == newName && ws.ID != id {
			return false
		}
	}

	for i := range c.Workspaces {
		if c.Workspaces[i].ID == id {
			c.Workspaces[i].Name = newName
			return true
		}
	}
	return false
}

// GetActiveWorkspaceID returns the active workspace ID, or empty string if none
func (c *Config) GetActiveWorkspaceID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ActiveWorkspaceID
}

// SetActiveWorkspaceID sets the active workspace ID. Pass empty string to clear.
func (c *Config) SetActiveWorkspaceID(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ActiveWorkspaceID = id
}

// SetSessionWorkspace assigns a session to a workspace. Pass empty workspaceID to unassign.
// Returns false if the session was not found.
func (c *Config) SetSessionWorkspace(sessionID, workspaceID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.Sessions {
		if c.Sessions[i].ID == sessionID {
			c.Sessions[i].WorkspaceID = workspaceID
			return true
		}
	}
	return false
}

// GetSessionsByWorkspace returns all sessions that belong to the given workspace.
func (c *Config) GetSessionsByWorkspace(workspaceID string) []Session {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if workspaceID == "" {
		return nil
	}

	var sessions []Session
	for _, s := range c.Sessions {
		if s.WorkspaceID == workspaceID {
			sessions = append(sessions, s)
		}
	}
	return sessions
}

// RemoveSessions removes multiple sessions by ID. Returns the count of sessions removed.
func (c *Config) RemoveSessions(ids []string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	removed := 0
	remaining := make([]Session, 0, len(c.Sessions))
	for _, s := range c.Sessions {
		if idSet[s.ID] {
			removed++
		} else {
			remaining = append(remaining, s)
		}
	}
	c.Sessions = remaining
	return removed
}

// SetSessionsWorkspace assigns multiple sessions to a workspace. Returns the count updated.
func (c *Config) SetSessionsWorkspace(ids []string, workspaceID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}

	updated := 0
	for i := range c.Sessions {
		if idSet[c.Sessions[i].ID] {
			c.Sessions[i].WorkspaceID = workspaceID
			updated++
		}
	}
	return updated
}
