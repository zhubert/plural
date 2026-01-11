// Package modals provides modal dialog state types for the UI.
// Each modal type implements the ModalState interface with its own state struct,
// ensuring type-safe access to modal-specific fields.
package modals

import (
	tea "charm.land/bubbletea/v2"
)

// ModalState is a discriminated union interface for modal-specific state.
// Each modal type implements this interface with its own state struct,
// ensuring type-safe access to modal-specific fields.
type ModalState interface {
	modalState() // marker method to restrict implementations
	Title() string
	Help() string
	Render() string
	Update(msg tea.Msg) (ModalState, tea.Cmd)
}

// MCPServerDisplay represents an MCP server for display in the modal
type MCPServerDisplay struct {
	Name     string
	Command  string
	Args     string // Args joined as string for display
	IsGlobal bool
	RepoPath string // Only set if per-repo
}

// ChangelogEntry represents a single version's changelog for display
type ChangelogEntry struct {
	Version string
	Date    string
	Changes []string
}

// OptionItem represents a detected option for display
type OptionItem struct {
	Number     int
	Text       string
	Selected   bool
	GroupIndex int // Which group this option belongs to (for visual separation)
}

// IssueItem represents a GitHub issue for display in the modal
type IssueItem struct {
	Number   int
	Title    string
	Body     string
	URL      string
	Selected bool
}

// HelpShortcut represents a single keyboard shortcut for display
type HelpShortcut struct {
	Key  string
	Desc string
}

// HelpShortcutTriggeredMsg is sent when user selects a shortcut in the help modal
type HelpShortcutTriggeredMsg struct {
	Key string // The key string to simulate (e.g., "n", "tab", "q")
}

// HelpSection represents a group of related shortcuts
type HelpSection struct {
	Title     string
	Shortcuts []HelpShortcut
}

// SearchResult represents a single search match with context
type SearchResult struct {
	MessageIndex int    // Index in the messages array
	Role         string // "user" or "assistant"
	Content      string // The full message content
	MatchStart   int    // Start position of match in content
	MatchEnd     int    // End position of match in content
}
