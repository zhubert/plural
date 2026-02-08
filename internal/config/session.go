package config

import (
	"strconv"
	"time"
)

// IssueRef represents a reference to an issue/task from any supported source.
// This is the generic replacement for the deprecated IssueNumber field.
type IssueRef struct {
	Source string `json:"source"` // "github" or "asana"
	ID     string `json:"id"`     // Issue/task ID (number for GitHub, GID for Asana)
	Title  string `json:"title"`  // Issue/task title for display
	URL    string `json:"url"`    // Link to the issue/task
}

// Session represents a Claude Code conversation session with its own worktree
type Session struct {
	ID         string    `json:"id"`
	RepoPath   string    `json:"repo_path"`
	WorkTree   string    `json:"worktree"`
	Branch     string    `json:"branch"`
	BaseBranch string    `json:"base_branch,omitempty"` // Branch this session was created from (e.g., "main", parent branch)
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"created_at"`
	Started    bool      `json:"started,omitempty"` // Whether session has been started with Claude CLI

	Merged           bool      `json:"merged,omitempty"`             // Whether session has been merged to main
	PRCreated        bool      `json:"pr_created,omitempty"`         // Whether a PR has been created for this session
	ParentID         string    `json:"parent_id,omitempty"`          // ID of parent session if this is a fork
	MergedToParent   bool      `json:"merged_to_parent,omitempty"`   // Whether session has been merged back to its parent (locks the session)
	IssueNumber      int       `json:"issue_number,omitempty"`       // Deprecated: use IssueRef instead. Kept for backwards compatibility.
	IssueRef         *IssueRef `json:"issue_ref,omitempty"`          // Generic issue/task reference (GitHub, Asana, etc.)
	BroadcastGroupID string    `json:"broadcast_group_id,omitempty"` // Links sessions created from the same broadcast
	WorkspaceID      string    `json:"workspace_id,omitempty"`       // Workspace this session belongs to
}

// GetIssueRef returns the IssueRef for this session, converting from legacy IssueNumber if needed.
// Returns nil if no issue is associated with this session.
// Migration: older sessions only have IssueNumber (GitHub-specific int). New sessions use IssueRef
// which supports any provider. Once all persisted sessions have been re-saved with IssueRef,
// the IssueNumber field and this fallback can be removed.
func (s *Session) GetIssueRef() *IssueRef {
	// Prefer new IssueRef if set
	if s.IssueRef != nil {
		return s.IssueRef
	}
	// Fall back to legacy IssueNumber for backwards compatibility
	if s.IssueNumber > 0 {
		return &IssueRef{
			Source: "github",
			ID:     strconv.Itoa(s.IssueNumber),
			Title:  "", // Title not stored in legacy format
			URL:    "", // URL not stored in legacy format
		}
	}
	return nil
}

// HasIssue returns true if this session was created from an issue/task.
func (s *Session) HasIssue() bool {
	return s.GetIssueRef() != nil
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

// ClearOrphanedParentIDs clears ParentID references that point to any of the deleted session IDs.
// This prevents child sessions from referencing non-existent parents.
func (c *Config) ClearOrphanedParentIDs(deletedIDs []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	idSet := make(map[string]bool, len(deletedIDs))
	for _, id := range deletedIDs {
		idSet[id] = true
	}

	for i := range c.Sessions {
		if c.Sessions[i].ParentID != "" && idSet[c.Sessions[i].ParentID] {
			c.Sessions[i].ParentID = ""
			c.Sessions[i].MergedToParent = false
		}
	}
}

// ClearSessions removes all sessions
func (c *Config) ClearSessions() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Sessions = []Session{}
}

// GetSession returns a copy of a session by ID.
// Returns nil if no session with the given ID exists.
func (c *Config) GetSession(id string) *Session {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := range c.Sessions {
		if c.Sessions[i].ID == id {
			sess := c.Sessions[i] // copy
			return &sess
		}
	}
	return nil
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

// MarkSessionMergedToParent marks a session as merged to its parent (locks the session)
func (c *Config) MarkSessionMergedToParent(sessionID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.Sessions {
		if c.Sessions[i].ID == sessionID {
			c.Sessions[i].MergedToParent = true
			return true
		}
	}
	return false
}

// RenameSession updates the name and branch of a session
func (c *Config) RenameSession(sessionID, newName, newBranch string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.Sessions {
		if c.Sessions[i].ID == sessionID {
			c.Sessions[i].Name = newName
			c.Sessions[i].Branch = newBranch
			return true
		}
	}
	return false
}

// GetSessionsByBroadcastGroup returns all sessions that belong to the given broadcast group
func (c *Config) GetSessionsByBroadcastGroup(groupID string) []Session {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if groupID == "" {
		return nil
	}

	var sessions []Session
	for _, s := range c.Sessions {
		if s.BroadcastGroupID == groupID {
			sessions = append(sessions, s)
		}
	}
	return sessions
}

// SetSessionBroadcastGroup sets the broadcast group ID for a session
func (c *Config) SetSessionBroadcastGroup(sessionID, groupID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.Sessions {
		if c.Sessions[i].ID == sessionID {
			c.Sessions[i].BroadcastGroupID = groupID
			return true
		}
	}
	return false
}
