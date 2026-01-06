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
	MaxSessionMessageLines = 100
)

// Session represents a Claude Code conversation session with its own worktree
type Session struct {
	ID           string    `json:"id"`
	RepoPath     string    `json:"repo_path"`
	WorkTree     string    `json:"worktree"`
	Branch       string    `json:"branch"`
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"created_at"`
	Started      bool      `json:"started,omitempty"`        // Whether session has been started with Claude CLI
	AllowedTools []string  `json:"allowed_tools,omitempty"` // Persisted permission decisions (e.g., "Edit", "Bash(git:*)")
}

// Config holds the application configuration
type Config struct {
	Repos    []string  `json:"repos"`
	Sessions []Session `json:"sessions"`

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
		Repos:    []string{},
		Sessions: []Session{},
		filePath: path,
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

	// Validate loaded config
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks that the config is internally consistent
func (c *Config) Validate() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Ensure slices are initialized (not nil)
	if c.Repos == nil {
		c.Repos = []string{}
	}
	if c.Sessions == nil {
		c.Sessions = []Session{}
	}

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

// RemoveRepo removes a repository path
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

// AddAllowedTool adds a tool to a session's allowed tools list
func (c *Config) AddAllowedTool(sessionID, tool string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.Sessions {
		if c.Sessions[i].ID == sessionID {
			// Check if already exists
			for _, t := range c.Sessions[i].AllowedTools {
				if t == tool {
					return false
				}
			}
			c.Sessions[i].AllowedTools = append(c.Sessions[i].AllowedTools, tool)
			return true
		}
	}
	return false
}

// GetAllowedTools returns a session's allowed tools
func (c *Config) GetAllowedTools(sessionID string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := range c.Sessions {
		if c.Sessions[i].ID == sessionID {
			tools := make([]string, len(c.Sessions[i].AllowedTools))
			copy(tools, c.Sessions[i].AllowedTools)
			return tools
		}
	}
	return nil
}

// IsToolAllowed checks if a tool is in a session's allowed list
func (c *Config) IsToolAllowed(sessionID, tool string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := range c.Sessions {
		if c.Sessions[i].ID == sessionID {
			for _, t := range c.Sessions[i].AllowedTools {
				if t == tool {
					return true
				}
			}
			return false
		}
	}
	return false
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
