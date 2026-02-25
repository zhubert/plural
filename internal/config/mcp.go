package config

import "slices"

// MCPServer represents an MCP server configuration
type MCPServer struct {
	Name    string   `json:"name"`    // Unique identifier for the server
	Command string   `json:"command"` // Executable command (e.g., "npx", "node")
	Args    []string `json:"args"`    // Command arguments
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

	resolved := resolveRepoPath(c.Repos, repoPath)

	// Check for duplicate name in this repo
	for _, s := range c.RepoMCP[resolved] {
		if s.Name == server.Name {
			return false
		}
	}
	c.RepoMCP[resolved] = append(c.RepoMCP[resolved], server)
	return true
}

// RemoveRepoMCPServer removes an MCP server from a specific repository
func (c *Config) RemoveRepoMCPServer(repoPath, name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	resolved := resolveRepoPath(c.Repos, repoPath)

	servers, exists := c.RepoMCP[resolved]
	if !exists {
		return false
	}

	for i, s := range servers {
		if s.Name == name {
			c.RepoMCP[resolved] = append(servers[:i], servers[i+1:]...)
			// Clean up empty map entries
			if len(c.RepoMCP[resolved]) == 0 {
				delete(c.RepoMCP, resolved)
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

	resolved := resolveRepoPath(c.Repos, repoPath)
	servers := c.RepoMCP[resolved]
	result := make([]MCPServer, len(servers))
	copy(result, servers)
	return result
}

// GetMCPServersForRepo returns merged global + per-repo servers
// Per-repo servers with the same name override global ones
func (c *Config) GetMCPServersForRepo(repoPath string) []MCPServer {
	c.mu.RLock()
	defer c.mu.RUnlock()

	resolved := resolveRepoPath(c.Repos, repoPath)

	// Start with global servers
	serverMap := make(map[string]MCPServer)
	for _, s := range c.MCPServers {
		serverMap[s.Name] = s
	}

	// Override with per-repo servers
	for _, s := range c.RepoMCP[resolved] {
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

	resolved := resolveRepoPath(c.Repos, repoPath)

	if slices.Contains(c.RepoAllowedTools[resolved], tool) {
		return false
	}
	c.RepoAllowedTools[resolved] = append(c.RepoAllowedTools[resolved], tool)
	return true
}

// GetAllowedToolsForRepo returns merged global + per-repo allowed tools
func (c *Config) GetAllowedToolsForRepo(repoPath string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	resolved := resolveRepoPath(c.Repos, repoPath)

	// Use a map to deduplicate
	toolSet := make(map[string]bool)
	for _, t := range c.AllowedTools {
		toolSet[t] = true
	}
	for _, t := range c.RepoAllowedTools[resolved] {
		toolSet[t] = true
	}

	result := make([]string, 0, len(toolSet))
	for t := range toolSet {
		result = append(result, t)
	}
	return result
}
