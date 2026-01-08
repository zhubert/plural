package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Configuration constants
const (
	// MaxSessionMessageLines is the maximum number of lines to keep in session message history
	MaxSessionMessageLines = 10000
)

// Session represents a Claude Code conversation session with its own worktree
type Session struct {
	ID        string    `json:"id"`
	RepoPath  string    `json:"repo_path"`
	WorkTree  string    `json:"worktree"`
	Branch    string    `json:"branch"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	Started   bool      `json:"started,omitempty"`    // Whether session has been started with Claude CLI
	Merged    bool      `json:"merged,omitempty"`     // Whether session has been merged to main
	PRCreated bool      `json:"pr_created,omitempty"` // Whether a PR has been created for this session
}

// MCPServer represents an MCP server configuration
type MCPServer struct {
	Name    string   `json:"name"`    // Unique identifier for the server
	Command string   `json:"command"` // Executable command (e.g., "npx", "node")
	Args    []string `json:"args"`    // Command arguments
}

// Config holds the application configuration
type Config struct {
	Repos            []string               `json:"repos"`
	Sessions         []Session              `json:"sessions"`
	MCPServers       []MCPServer            `json:"mcp_servers,omitempty"`        // Global MCP servers
	RepoMCP          map[string][]MCPServer `json:"repo_mcp,omitempty"`           // Per-repo MCP servers
	AllowedTools     []string               `json:"allowed_tools,omitempty"`      // Global allowed tools
	RepoAllowedTools map[string][]string    `json:"repo_allowed_tools,omitempty"` // Per-repo allowed tools
	WelcomeShown     bool                   `json:"welcome_shown,omitempty"`      // Whether welcome modal has been shown
	LastSeenVersion  string                 `json:"last_seen_version,omitempty"`  // Last version user has seen changelog for

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
// before Validate() since Validate() only reads. This method is NOT
// thread-safe and should only be called during initialization.
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

// AddSession adds a new session
func (c *Config) AddSession(session Session) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Sessions = append(c.Sessions, session)
}

// RemoveSession removes a session by ID
func (c *Config) RemoveSession(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, s := range c.Sessions {
		if s.ID == id {
			c.Sessions = append(c.Sessions[:i], c.Sessions[i+1:]...)
			return true
		}
	}
	return false
}

// ClearSessions removes all sessions
func (c *Config) ClearSessions() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Sessions = []Session{}
}

// GetSession returns a session by ID
func (c *Config) GetSession(id string) *Session {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := range c.Sessions {
		if c.Sessions[i].ID == id {
			return &c.Sessions[i]
		}
	}
	return nil
}

// GetRepos returns a copy of the repos slice
func (c *Config) GetRepos() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	repos := make([]string, len(c.Repos))
	copy(repos, c.Repos)
	return repos
}

// GetSessions returns a copy of the sessions slice
func (c *Config) GetSessions() []Session {
	c.mu.RLock()
	defer c.mu.RUnlock()

	sessions := make([]Session, len(c.Sessions))
	copy(sessions, c.Sessions)
	return sessions
}

// MarkSessionStarted marks a session as started with Claude CLI
func (c *Config) MarkSessionStarted(sessionID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.Sessions {
		if c.Sessions[i].ID == sessionID {
			c.Sessions[i].Started = true
			return true
		}
	}
	return false
}

// MarkSessionMerged marks a session as merged to main
func (c *Config) MarkSessionMerged(sessionID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.Sessions {
		if c.Sessions[i].ID == sessionID {
			c.Sessions[i].Merged = true
			return true
		}
	}
	return false
}

// MarkSessionPRCreated marks a session as having a PR created
func (c *Config) MarkSessionPRCreated(sessionID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.Sessions {
		if c.Sessions[i].ID == sessionID {
			c.Sessions[i].PRCreated = true
			return true
		}
	}
	return false
}

// AddGlobalMCPServer adds a global MCP server (returns false if name already exists)
func (c *Config) AddGlobalMCPServer(server MCPServer) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, s := range c.MCPServers {
		if s.Name == server.Name {
			return false
		}
	}
	c.MCPServers = append(c.MCPServers, server)
	return true
}

// RemoveGlobalMCPServer removes a global MCP server by name
func (c *Config) RemoveGlobalMCPServer(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, s := range c.MCPServers {
		if s.Name == name {
			c.MCPServers = append(c.MCPServers[:i], c.MCPServers[i+1:]...)
			return true
		}
	}
	return false
}

// GetGlobalMCPServers returns a copy of global MCP servers
func (c *Config) GetGlobalMCPServers() []MCPServer {
	c.mu.RLock()
	defer c.mu.RUnlock()

	servers := make([]MCPServer, len(c.MCPServers))
	copy(servers, c.MCPServers)
	return servers
}

// AddRepoMCPServer adds an MCP server for a specific repository
func (c *Config) AddRepoMCPServer(repoPath string, server MCPServer) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.RepoMCP == nil {
		c.RepoMCP = make(map[string][]MCPServer)
	}

	// Check for duplicate name in this repo
	for _, s := range c.RepoMCP[repoPath] {
		if s.Name == server.Name {
			return false
		}
	}
	c.RepoMCP[repoPath] = append(c.RepoMCP[repoPath], server)
	return true
}

// RemoveRepoMCPServer removes an MCP server from a specific repository
func (c *Config) RemoveRepoMCPServer(repoPath, name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	servers, exists := c.RepoMCP[repoPath]
	if !exists {
		return false
	}

	for i, s := range servers {
		if s.Name == name {
			c.RepoMCP[repoPath] = append(servers[:i], servers[i+1:]...)
			// Clean up empty map entries
			if len(c.RepoMCP[repoPath]) == 0 {
				delete(c.RepoMCP, repoPath)
			}
			return true
		}
	}
	return false
}

// GetRepoMCPServers returns MCP servers for a specific repository
func (c *Config) GetRepoMCPServers(repoPath string) []MCPServer {
	c.mu.RLock()
	defer c.mu.RUnlock()

	servers := c.RepoMCP[repoPath]
	result := make([]MCPServer, len(servers))
	copy(result, servers)
	return result
}

// GetMCPServersForRepo returns merged global + per-repo servers
// Per-repo servers with the same name override global ones
func (c *Config) GetMCPServersForRepo(repoPath string) []MCPServer {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Start with global servers
	serverMap := make(map[string]MCPServer)
	for _, s := range c.MCPServers {
		serverMap[s.Name] = s
	}

	// Override with per-repo servers
	for _, s := range c.RepoMCP[repoPath] {
		serverMap[s.Name] = s
	}

	// Convert map back to slice
	result := make([]MCPServer, 0, len(serverMap))
	for _, s := range serverMap {
		result = append(result, s)
	}
	return result
}

// GetGlobalAllowedTools returns a copy of global allowed tools
func (c *Config) GetGlobalAllowedTools() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tools := make([]string, len(c.AllowedTools))
	copy(tools, c.AllowedTools)
	return tools
}

// AddRepoAllowedTool adds a tool to a repository's allowed tools list
func (c *Config) AddRepoAllowedTool(repoPath, tool string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.RepoAllowedTools == nil {
		c.RepoAllowedTools = make(map[string][]string)
	}

	for _, t := range c.RepoAllowedTools[repoPath] {
		if t == tool {
			return false
		}
	}
	c.RepoAllowedTools[repoPath] = append(c.RepoAllowedTools[repoPath], tool)
	return true
}

// GetAllowedToolsForRepo returns merged global + per-repo allowed tools
func (c *Config) GetAllowedToolsForRepo(repoPath string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Use a map to deduplicate
	toolSet := make(map[string]bool)
	for _, t := range c.AllowedTools {
		toolSet[t] = true
	}
	for _, t := range c.RepoAllowedTools[repoPath] {
		toolSet[t] = true
	}

	result := make([]string, 0, len(toolSet))
	for t := range toolSet {
		result = append(result, t)
	}
	return result
}

// Message represents a chat message for persistence
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// sessionsDir returns the path to the sessions directory
func sessionsDir() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sessions"), nil
}

// SaveSessionMessages saves messages for a session (keeps last maxLines lines)
func SaveSessionMessages(sessionID string, messages []Message, maxLines int) error {
	dir, err := sessionsDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Keep only the last maxLines worth of content
	if maxLines > 0 && len(messages) > 0 {
		// Trim messages to approximately maxLines of content
		var totalLines int
		startIdx := len(messages)
		for i := len(messages) - 1; i >= 0; i-- {
			lines := countLines(messages[i].Content)
			if totalLines+lines > maxLines && startIdx < len(messages) {
				break
			}
			totalLines += lines
			startIdx = i
		}
		messages = messages[startIdx:]
	}

	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(dir, sessionID+".json")
	return os.WriteFile(path, data, 0644)
}

// LoadSessionMessages loads messages for a session
func LoadSessionMessages(sessionID string) ([]Message, error) {
	dir, err := sessionsDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, sessionID+".json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []Message{}, nil
	}
	if err != nil {
		return nil, err
	}

	var messages []Message
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, err
	}

	return messages, nil
}

// DeleteSessionMessages deletes the messages file for a session
func DeleteSessionMessages(sessionID string) error {
	dir, err := sessionsDir()
	if err != nil {
		return err
	}

	path := filepath.Join(dir, sessionID+".json")
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// countLines counts the number of lines in a string
func countLines(s string) int {
	if s == "" {
		return 0
	}
	count := 1
	for _, c := range s {
		if c == '\n' {
			count++
		}
	}
	return count
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
