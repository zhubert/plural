package ui

import (
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/zhubert/plural/internal/config"
)

// Braille spinner frames for sidebar
var sidebarSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// SidebarTickMsg is sent to advance the spinner animation
type SidebarTickMsg time.Time

// repoGroup represents a group of sessions for a single repo
type repoGroup struct {
	RepoPath string
	RepoName string
	Sessions []config.Session
}

// Sidebar represents the left panel with session list
type Sidebar struct {
	groups             []repoGroup
	sessions           []config.Session // flat list for index tracking
	selectedIdx        int
	width              int
	height             int
	focused            bool
	scrollOffset       int
	streamingSessions  map[string]bool // Map of session IDs that are currently streaming
	pendingPermissions map[string]bool // Map of session IDs that have pending permission requests
	spinnerFrame       int             // Current spinner animation frame
}

// NewSidebar creates a new sidebar
func NewSidebar() *Sidebar {
	return &Sidebar{
		selectedIdx:        0,
		streamingSessions:  make(map[string]bool),
		pendingPermissions: make(map[string]bool),
	}
}

// SetSize sets the sidebar dimensions
func (s *Sidebar) SetSize(width, height int) {
	s.width = width
	s.height = height

	ctx := GetViewContext()
	ctx.Log("Sidebar.SetSize: outer=%dx%d, inner=%dx%d", width, height, ctx.InnerWidth(width), ctx.InnerHeight(height))
}

// SetFocused sets the focus state
func (s *Sidebar) SetFocused(focused bool) {
	s.focused = focused
}

// IsFocused returns the focus state
func (s *Sidebar) IsFocused() bool {
	return s.focused
}

// SetSessions updates the session list, grouping by repo
func (s *Sidebar) SetSessions(sessions []config.Session) {
	// Group sessions by repo path
	groupMap := make(map[string]*repoGroup)
	var groupOrder []string

	for _, sess := range sessions {
		if _, exists := groupMap[sess.RepoPath]; !exists {
			groupMap[sess.RepoPath] = &repoGroup{
				RepoPath: sess.RepoPath,
				RepoName: filepath.Base(sess.RepoPath),
				Sessions: []config.Session{},
			}
			groupOrder = append(groupOrder, sess.RepoPath)
		}
		groupMap[sess.RepoPath].Sessions = append(groupMap[sess.RepoPath].Sessions, sess)
	}

	// Build ordered groups
	s.groups = make([]repoGroup, 0, len(groupOrder))
	for _, path := range groupOrder {
		s.groups = append(s.groups, *groupMap[path])
	}

	// Rebuild flat sessions list to match grouped order
	s.sessions = make([]config.Session, 0, len(sessions))
	for _, group := range s.groups {
		s.sessions = append(s.sessions, group.Sessions...)
	}

	// Adjust selection if needed
	if s.selectedIdx >= len(s.sessions) {
		s.selectedIdx = len(s.sessions) - 1
	}
	if s.selectedIdx < 0 {
		s.selectedIdx = 0
	}
}

// SelectedSession returns the currently selected session
func (s *Sidebar) SelectedSession() *config.Session {
	if len(s.sessions) == 0 || s.selectedIdx >= len(s.sessions) {
		return nil
	}
	return &s.sessions[s.selectedIdx]
}

// SelectSession selects a session by ID
func (s *Sidebar) SelectSession(id string) {
	for i, sess := range s.sessions {
		if sess.ID == id {
			s.selectedIdx = i
			return
		}
	}
}

// SetStreaming sets the streaming state for a session
func (s *Sidebar) SetStreaming(sessionID string, streaming bool) {
	if streaming {
		s.streamingSessions[sessionID] = true
	} else {
		delete(s.streamingSessions, sessionID)
	}
}

// IsStreaming returns whether any session is currently streaming
func (s *Sidebar) IsStreaming() bool {
	return len(s.streamingSessions) > 0
}

// IsSessionStreaming returns whether a specific session is streaming
func (s *Sidebar) IsSessionStreaming(sessionID string) bool {
	return s.streamingSessions[sessionID]
}

// SetPendingPermission sets whether a session has a pending permission request
func (s *Sidebar) SetPendingPermission(sessionID string, pending bool) {
	if pending {
		s.pendingPermissions[sessionID] = true
	} else {
		delete(s.pendingPermissions, sessionID)
	}
}

// HasPendingPermission returns whether a session has a pending permission request
func (s *Sidebar) HasPendingPermission(sessionID string) bool {
	return s.pendingPermissions[sessionID]
}

// SidebarTick returns a command that sends a tick message after a delay
func SidebarTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return SidebarTickMsg(t)
	})
}

// Update handles messages
func (s *Sidebar) Update(msg tea.Msg) (*Sidebar, tea.Cmd) {
	switch msg := msg.(type) {
	case SidebarTickMsg:
		if s.IsStreaming() {
			s.spinnerFrame = (s.spinnerFrame + 1) % len(sidebarSpinnerFrames)
			return s, SidebarTick()
		}
		return s, nil

	case tea.KeyPressMsg:
		if !s.focused {
			return s, nil
		}
		switch msg.String() {
		case "up", "k":
			if s.selectedIdx > 0 {
				s.selectedIdx--
				s.ensureVisible()
			}
		case "down", "j":
			if s.selectedIdx < len(s.sessions)-1 {
				s.selectedIdx++
				s.ensureVisible()
			}
		}
	}

	return s, nil
}

// ensureVisible adjusts scroll offset to keep selection visible
func (s *Sidebar) ensureVisible() {
	ctx := GetViewContext()
	visibleHeight := ctx.InnerHeight(s.height)

	// Calculate the line number of the selected session in the rendered view
	selectedLine := s.getSelectedLine()

	if selectedLine < s.scrollOffset {
		s.scrollOffset = selectedLine
	} else if selectedLine >= s.scrollOffset+visibleHeight {
		s.scrollOffset = selectedLine - visibleHeight + 1
	}
}

// getSelectedLine returns the line number of the selected session in the rendered list
func (s *Sidebar) getSelectedLine() int {
	line := 0
	sessionIdx := 0
	for i, group := range s.groups {
		if i > 0 {
			line++ // repo header (not for first group since no title above it)
		}
		for range group.Sessions {
			if sessionIdx == s.selectedIdx {
				return line
			}
			line++
			sessionIdx++
		}
	}
	return line
}

// View renders the sidebar
func (s *Sidebar) View() string {
	ctx := GetViewContext()

	style := PanelStyle
	if s.focused {
		style = PanelFocusedStyle
	}

	innerHeight := ctx.InnerHeight(s.height)

	var content string
	if len(s.sessions) == 0 {
		emptyMsg := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("No sessions.")
		content = emptyMsg
	} else {
		// Build the grouped list
		var lines []string

		sessionIdx := 0
		for _, group := range s.groups {
			// Repo header
			repoStyle := lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Bold(true)
			lines = append(lines, repoStyle.Render(group.RepoName))

			// Sessions in this group
			for _, sess := range group.Sessions {
				// Extract just the short ID part from the name (after the /)
				displayName := sess.Name
				if parts := strings.Split(sess.Name, "/"); len(parts) > 1 {
					displayName = "  " + parts[len(parts)-1]
				} else {
					displayName = "  " + displayName
				}

				itemStyle := SidebarItemStyle
				if sessionIdx == s.selectedIdx {
					itemStyle = SidebarSelectedStyle
					displayName = "> " + strings.TrimPrefix(displayName, "  ")
				}

				// Add indicators for streaming and pending permissions
				if s.IsSessionStreaming(sess.ID) {
					spinner := sidebarSpinnerFrames[s.spinnerFrame]
					// Use white for selected (purple bg), purple for unselected
					spinnerColor := ColorPrimary
					if sessionIdx == s.selectedIdx {
						spinnerColor = ColorText // White on purple background
					}
					spinnerStyle := lipgloss.NewStyle().Foreground(spinnerColor)
					displayName = displayName + " " + spinnerStyle.Render(spinner)
				}

				// Add permission indicator
				if s.HasPendingPermission(sess.ID) {
					// Use white for selected (purple bg), warning color for unselected
					indicatorColor := ColorWarning
					if sessionIdx == s.selectedIdx {
						indicatorColor = ColorText // White on purple background
					}
					indicatorStyle := lipgloss.NewStyle().Foreground(indicatorColor)
					displayName = displayName + " " + indicatorStyle.Render("⚠")
				}

				lines = append(lines, itemStyle.Render(displayName))
				sessionIdx++
			}
		}

		// Apply scrolling
		visibleHeight := innerHeight

		if len(lines) > visibleHeight && s.scrollOffset > 0 {
			// Show scroll indicator at top
			if s.scrollOffset < len(lines) {
				lines = lines[s.scrollOffset:]
			}
		}

		// Truncate to fit
		if len(lines) > visibleHeight {
			lines = lines[:visibleHeight]
		}

		content = strings.Join(lines, "\n")
	}

	// Ensure content fits
	lines := strings.Split(content, "\n")
	if len(lines) > innerHeight && innerHeight > 0 {
		lines = lines[:innerHeight]
		content = strings.Join(lines, "\n")
	}

	// In lipgloss v2, Width/Height include borders, so pass full panel size
	return style.Width(s.width).Height(s.height).Render(content)
}
