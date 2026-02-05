package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Config holds the application configuration
type Config struct {
	Repos            []string               `json:"repos"`
	Sessions         []Session              `json:"sessions"`
	MCPServers       []MCPServer            `json:"mcp_servers,omitempty"`        // Global MCP servers
	RepoMCP          map[string][]MCPServer `json:"repo_mcp,omitempty"`           // Per-repo MCP servers
	AllowedTools      []string               `json:"allowed_tools,omitempty"`       // Global allowed tools
	RepoAllowedTools  map[string][]string    `json:"repo_allowed_tools,omitempty"`  // Per-repo allowed tools
	RepoSquashOnMerge map[string]bool        `json:"repo_squash_on_merge,omitempty"` // Per-repo squash-on-merge setting
	RepoAsanaProject  map[string]string      `json:"repo_asana_project,omitempty"`  // Per-repo Asana project GID mapping

	WelcomeShown         bool   `json:"welcome_shown,omitempty"`         // Whether welcome modal has been shown
	LastSeenVersion      string `json:"last_seen_version,omitempty"`     // Last version user has seen changelog for
	Theme                string `json:"theme,omitempty"`                 // UI theme name (e.g., "dark-purple", "nord")
	DefaultBranchPrefix  string `json:"default_branch_prefix,omitempty"` // Prefix for auto-generated branch names (e.g., "zhubert/")
	NotificationsEnabled bool   `json:"notifications_enabled,omitempty"` // Desktop notifications when Claude completes

	// Preview state - tracks when a session's branch is checked out in the main repo
	PreviewSessionID      string `json:"preview_session_id,omitempty"`      // Session ID currently being previewed (empty if none)
	PreviewPreviousBranch string `json:"preview_previous_branch,omitempty"` // Branch that was checked out before preview started
	PreviewRepoPath       string `json:"preview_repo_path,omitempty"`       // Path to the main repo where preview is active

	mu       sync.RWMutex
	filePath string
}

// configDir returns the path to the config directory
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".plural"), nil
}

// configPath returns the path to the config file
func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the config from disk, or creates a new one if it doesn't exist
func Load() (*Config, error) {
	path, err := configPath()
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
	c.mu.RLock()
	defer c.mu.RUnlock()

	dir, err := configDir()
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
