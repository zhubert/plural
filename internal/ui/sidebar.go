package ui

import (
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/zhubert/plural/internal/config"
)

// sidebarSpinnerFrames uses the same shimmering spinner as the chat panel
// Inspired by Claude Code's flower-like spinner
var sidebarSpinnerFrames = []string{"·", "✺", "✹", "✸", "✷", "✶", "✵", "✴", "✳", "✲", "✱", "✧", "✦", "·"}

// sidebarSpinnerHoldTimes defines how long each frame should be held (in ticks)
// First and last frames hold longer for a "breathing" effect
var sidebarSpinnerHoldTimes = []int{3, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 3}

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
	filteredSessions   []config.Session // sessions matching current search filter
	selectedIdx        int
	width              int
	height             int
	focused            bool
	scrollOffset       int
	streamingSessions  map[string]bool // Map of session IDs that are currently streaming
	pendingPermissions map[string]bool // Map of session IDs that have pending permission requests
	sessionsInUse      map[string]bool // Map of session IDs that have "session in use" errors
	spinnerFrame       int             // Current spinner animation frame
	spinnerTick        int             // Tick counter for frame hold timing

	// Search mode
	searchMode  bool
	searchInput textinput.Model
}

// NewSidebar creates a new sidebar
func NewSidebar() *Sidebar {
	ti := textinput.New()
	ti.Placeholder = "search..."
	ti.CharLimit = 50

	return &Sidebar{
		selectedIdx:        0,
		streamingSessions:  make(map[string]bool),
		pendingPermissions: make(map[string]bool),
		sessionsInUse:      make(map[string]bool),
		searchInput:        ti,
	}
}

// SetSize sets the sidebar dimensions
func (s *Sidebar) SetSize(width, height int) {
	s.width = width
	s.height = height

	ctx := GetViewContext()
	ctx.Log("Sidebar.SetSize: outer=%dx%d, inner=%dx%d", width, height, ctx.InnerWidth(width), ctx.InnerHeight(height))
}

// Width returns the sidebar width
func (s *Sidebar) Width() int {
	return s.width
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
	displaySessions := s.getDisplaySessions()
	if len(displaySessions) == 0 || s.selectedIdx >= len(displaySessions) {
		return nil
	}
	return &displaySessions[s.selectedIdx]
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

// SetSessionInUse sets whether a session has a "session in use" error
func (s *Sidebar) SetSessionInUse(sessionID string, inUse bool) {
	if inUse {
		s.sessionsInUse[sessionID] = true
	} else {
		delete(s.sessionsInUse, sessionID)
	}
}

// HasSessionInUse returns whether a session has a "session in use" error
func (s *Sidebar) HasSessionInUse(sessionID string) bool {
	return s.sessionsInUse[sessionID]
}

// SidebarTick returns a command that sends a tick message after a delay
func SidebarTick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
		return SidebarTickMsg(t)
	})
}

// EnterSearchMode activates search mode
func (s *Sidebar) EnterSearchMode() tea.Cmd {
	s.searchMode = true
	s.searchInput.SetValue("")
	s.searchInput.Focus()
	s.applyFilter("")
	return nil
}

// ExitSearchMode deactivates search mode and clears the filter
func (s *Sidebar) ExitSearchMode() {
	s.searchMode = false
	s.searchInput.Blur()
	s.searchInput.SetValue("")
	s.filteredSessions = nil
	// Reset selection to stay within bounds
	if s.selectedIdx >= len(s.sessions) {
		s.selectedIdx = len(s.sessions) - 1
	}
	if s.selectedIdx < 0 {
		s.selectedIdx = 0
	}
}

// IsSearchMode returns whether search mode is active
func (s *Sidebar) IsSearchMode() bool {
	return s.searchMode
}

// GetSearchQuery returns the current search query
func (s *Sidebar) GetSearchQuery() string {
	return s.searchInput.Value()
}

// applyFilter filters sessions based on the search query
func (s *Sidebar) applyFilter(query string) {
	if query == "" {
		s.filteredSessions = nil
		return
	}

	query = strings.ToLower(query)
	s.filteredSessions = nil

	for _, sess := range s.sessions {
		// Search in branch name
		if sess.Branch != "" && strings.Contains(strings.ToLower(sess.Branch), query) {
			s.filteredSessions = append(s.filteredSessions, sess)
			continue
		}
		// Search in session name
		if strings.Contains(strings.ToLower(sess.Name), query) {
			s.filteredSessions = append(s.filteredSessions, sess)
			continue
		}
		// Search in repo path (just the base name)
		repoName := filepath.Base(sess.RepoPath)
		if strings.Contains(strings.ToLower(repoName), query) {
			s.filteredSessions = append(s.filteredSessions, sess)
			continue
		}
	}

	// Reset selection to stay within bounds of filtered list
	if len(s.filteredSessions) > 0 {
		if s.selectedIdx >= len(s.filteredSessions) {
			s.selectedIdx = len(s.filteredSessions) - 1
		}
	} else {
		s.selectedIdx = 0
	}
	s.scrollOffset = 0
}

// getDisplaySessions returns the sessions to display (filtered or all)
func (s *Sidebar) getDisplaySessions() []config.Session {
	if s.searchMode && s.filteredSessions != nil {
		return s.filteredSessions
	}
	return s.sessions
}

// Update handles messages
func (s *Sidebar) Update(msg tea.Msg) (*Sidebar, tea.Cmd) {
	switch msg := msg.(type) {
	case SidebarTickMsg:
		if s.IsStreaming() {
			// Advance the spinner with easing (some frames hold longer)
			s.spinnerTick++
			holdTime := sidebarSpinnerHoldTimes[s.spinnerFrame%len(sidebarSpinnerHoldTimes)]
			if s.spinnerTick >= holdTime {
				s.spinnerTick = 0
				s.spinnerFrame = (s.spinnerFrame + 1) % len(sidebarSpinnerFrames)
			}
			return s, SidebarTick()
		}
		return s, nil

	case tea.KeyPressMsg:
		if !s.focused {
			return s, nil
		}

		// Handle search mode input
		if s.searchMode {
			switch msg.String() {
			case "esc":
				s.ExitSearchMode()
				return s, nil
			case "enter":
				// Exit search mode but keep filter applied (user selected)
				s.searchMode = false
				s.searchInput.Blur()
				return s, nil
			case "up", "ctrl+p":
				displaySessions := s.getDisplaySessions()
				if s.selectedIdx > 0 {
					s.selectedIdx--
					s.ensureVisibleFiltered(displaySessions)
				}
				return s, nil
			case "down", "ctrl+n":
				displaySessions := s.getDisplaySessions()
				if s.selectedIdx < len(displaySessions)-1 {
					s.selectedIdx++
					s.ensureVisibleFiltered(displaySessions)
				}
				return s, nil
			default:
				// Forward to text input
				var cmd tea.Cmd
				s.searchInput, cmd = s.searchInput.Update(msg)
				// Apply filter based on new query
				s.applyFilter(s.searchInput.Value())
				return s, cmd
			}
		}

		// Normal mode navigation
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

// ensureVisibleFiltered adjusts scroll offset for filtered list (no repo headers)
func (s *Sidebar) ensureVisibleFiltered(displaySessions []config.Session) {
	ctx := GetViewContext()
	// Account for search input line when in search mode
	visibleHeight := ctx.InnerHeight(s.height)
	if s.searchMode {
		visibleHeight-- // Reserve one line for search input
	}

	// In filtered mode, each session is one line (no repo headers)
	selectedLine := s.selectedIdx

	// Ensure we don't go past the end of the list
	if selectedLine >= len(displaySessions) {
		selectedLine = len(displaySessions) - 1
	}

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

	// Render search input if in search mode
	var searchLine string
	if s.searchMode {
		// Style the search input
		searchStyle := lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)
		s.searchInput.SetWidth(ctx.InnerWidth(s.width) - 3) // Leave room for "/ "
		searchLine = searchStyle.Render("/") + " " + s.searchInput.View()
		innerHeight-- // Reserve one line for search
	}

	displaySessions := s.getDisplaySessions()

	if len(displaySessions) == 0 {
		var emptyMsg string
		if s.searchMode && s.searchInput.Value() != "" {
			emptyMsg = lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Italic(true).
				Render("No matches.")
		} else {
			emptyMsg = lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Italic(true).
				Render("No sessions.")
		}
		content = emptyMsg
	} else if s.searchMode && s.filteredSessions != nil {
		// Render flat filtered list (no repo grouping)
		var lines []string
		for idx, sess := range s.filteredSessions {
			displayName := s.renderSessionName(sess, idx)
			itemStyle := SidebarItemStyle
			if idx == s.selectedIdx {
				itemStyle = SidebarSelectedStyle
				displayName = "> " + strings.TrimPrefix(displayName, "  ")
			}
			lines = append(lines, itemStyle.Render(displayName))
		}

		// Apply scrolling
		if len(lines) > innerHeight && s.scrollOffset > 0 {
			if s.scrollOffset < len(lines) {
				lines = lines[s.scrollOffset:]
			}
		}
		if len(lines) > innerHeight {
			lines = lines[:innerHeight]
		}
		content = strings.Join(lines, "\n")
	} else {
		// Build the grouped list (normal mode)
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
				displayName := s.renderSessionName(sess, sessionIdx)

				itemStyle := SidebarItemStyle
				if sessionIdx == s.selectedIdx {
					itemStyle = SidebarSelectedStyle
					displayName = "> " + strings.TrimPrefix(displayName, "  ")
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

	// Prepend search line if in search mode
	if s.searchMode {
		if content != "" {
			content = searchLine + "\n" + content
		} else {
			content = searchLine
		}
	}

	// In lipgloss v2, Width/Height include borders, so pass full panel size
	return style.Width(s.width).Height(s.height).Render(content)
}

// renderSessionName builds the display name for a session with all indicators
func (s *Sidebar) renderSessionName(sess config.Session, sessionIdx int) string {
	// Use branch name if it's a custom branch, otherwise use the short ID from name
	var displayName string
	if sess.Branch != "" && !strings.HasPrefix(sess.Branch, "plural-") {
		// Custom branch name - show it
		displayName = "  " + sess.Branch
	} else if parts := strings.Split(sess.Name, "/"); len(parts) > 1 {
		// Extract short ID from name
		displayName = "  " + parts[len(parts)-1]
	} else {
		displayName = "  " + sess.Name
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

	// Add "session in use" indicator
	if s.HasSessionInUse(sess.ID) {
		// Use white for selected (purple bg), error color for unselected
		indicatorColor := ColorError
		if sessionIdx == s.selectedIdx {
			indicatorColor = ColorText // White on purple background
		}
		indicatorStyle := lipgloss.NewStyle().Foreground(indicatorColor)
		displayName = displayName + " " + indicatorStyle.Render("⛔")
	}

	// Add merged/PR status labels
	if sess.Merged {
		labelColor := ColorSecondary // Green for merged
		if sessionIdx == s.selectedIdx {
			labelColor = ColorText // White on purple background
		}
		labelStyle := lipgloss.NewStyle().Foreground(labelColor)
		displayName = displayName + " " + labelStyle.Render("(merged)")
	} else if sess.PRCreated {
		labelColor := ColorUser // Blue for PR
		if sessionIdx == s.selectedIdx {
			labelColor = ColorText // White on purple background
		}
		labelStyle := lipgloss.NewStyle().Foreground(labelColor)
		displayName = displayName + " " + labelStyle.Render("(pr)")
	}

	return displayName
}
