package ui

import (
	"hash/fnv"
	"image/color"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/keys"
	"github.com/zhubert/plural/internal/logger"
)

// sidebarSpinnerFrames uses the same shimmering spinner as the chat panel
// Inspired by Claude Code's flower-like spinner
var sidebarSpinnerFrames = []string{"·", "✺", "✹", "✸", "✷", "✶", "✵", "✴", "✳", "✲", "✱", "✧", "✦", "·"}

// sidebarSpinnerHoldTimes defines how long each frame should be held (in ticks)
// First and last frames hold longer for a "breathing" effect
var sidebarSpinnerHoldTimes = []int{3, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 3}

// SidebarTickMsg is sent to advance the spinner animation
type SidebarTickMsg time.Time

// sidebarItemKind distinguishes between repo headers and sessions in the sidebar.
type sidebarItemKind int

const (
	itemKindRepo       sidebarItemKind = iota // A repo header (selectable)
	itemKindSession                           // A session within a repo
	itemKindNewSession                        // A "+ New Session" action under a repo
)

// sidebarItem represents a selectable item in the sidebar (either a repo or a session).
type sidebarItem struct {
	Kind     sidebarItemKind
	Session  config.Session // Only valid when Kind == itemKindSession
	RepoPath string         // Set for both kinds
}

// sessionNode represents a session with its children (forks)
type sessionNode struct {
	Session  config.Session
	Children []sessionNode
}

// repoGroup represents a group of sessions for a single repo
type repoGroup struct {
	RepoPath string
	RepoName string
	Sessions []config.Session
	// Tree structure for hierarchical display
	RootNodes []sessionNode
}

// Sidebar represents the left panel with session list
type Sidebar struct {
	groups             []repoGroup
	sessions           []config.Session // flat list for index tracking
	items              []sidebarItem    // flat list of all selectable items (repos + sessions)
	filteredSessions   []config.Session // sessions matching current search filter
	selectedIdx        int
	width              int
	height             int
	focused            bool
	scrollOffset       int
	streamingSessions  map[string]bool // Map of session IDs that are currently streaming
	pendingPermissions map[string]bool // Map of session IDs that have pending permission requests
	pendingQuestions   map[string]bool // Map of session IDs that have pending questions
	idleWithResponse   map[string]bool // Map of session IDs that finished streaming (user hasn't responded)
	uncommittedChanges map[string]bool // Map of session IDs that have uncommitted changes
	hasNewComments     map[string]bool // Map of session IDs that have new PR review comments
	spinnerFrame       int             // Current spinner animation frame
	spinnerTick        int             // Tick counter for frame hold timing

	// Multi-select mode
	multiSelectMode  bool
	selectedSessions map[string]bool

	// Cache for incremental updates
	lastHash     uint64 // Hash of last session list for change detection
	lastAttnHash uint64 // Hash of attention state for re-ordering detection

	// Registered repos (may have no sessions)
	registeredRepos []string

	// Search mode
	searchMode  bool
	searchInput textinput.Model
}

// NewSidebar creates a new sidebar
func NewSidebar() *Sidebar {
	ti := textinput.New()
	ti.Placeholder = "search..."
	ti.CharLimit = SidebarSearchCharLimit

	return &Sidebar{
		selectedIdx:        0,
		streamingSessions:  make(map[string]bool),
		pendingPermissions: make(map[string]bool),
		pendingQuestions:   make(map[string]bool),
		idleWithResponse:   make(map[string]bool),
		uncommittedChanges: make(map[string]bool),
		hasNewComments:     make(map[string]bool),
		selectedSessions:   make(map[string]bool),
		searchInput:        ti,
	}
}

// SetSize sets the sidebar dimensions
func (s *Sidebar) SetSize(width, height int) {
	s.width = width
	s.height = height

	ctx := GetViewContext()
	ctx.Log("Sidebar.SetSize",
		"outerWidth", width,
		"outerHeight", height,
		"innerWidth", ctx.InnerWidth(width),
		"innerHeight", ctx.InnerHeight(height),
	)
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

// hashSessions computes a fast hash of the session list to detect changes
func hashSessions(sessions []config.Session) uint64 {
	h := fnv.New64a()
	for _, sess := range sessions {
		h.Write([]byte(sess.ID))
		h.Write([]byte{0})
		h.Write([]byte(sess.RepoPath))
		h.Write([]byte{0})
		h.Write([]byte(sess.ParentID))
		h.Write([]byte{0})
		h.Write([]byte(sess.Branch))
		h.Write([]byte{0})
		h.Write([]byte(sess.Name))
		h.Write([]byte{0})
		// Include status flags in hash
		if sess.Merged {
			h.Write([]byte{1})
		} else {
			h.Write([]byte{0})
		}
		if sess.PRCreated {
			h.Write([]byte{1})
		} else {
			h.Write([]byte{0})
		}
		if sess.MergedToParent {
			h.Write([]byte{1})
		} else {
			h.Write([]byte{0})
		}
		if sess.PRMerged {
			h.Write([]byte{1})
		} else {
			h.Write([]byte{0})
		}
		if sess.PRClosed {
			h.Write([]byte{1})
		} else {
			h.Write([]byte{0})
		}
	}
	return h.Sum64()
}

// hashAttention computes a hash of the attention state maps to detect changes.
// Map keys are sorted before hashing to ensure deterministic output.
func (s *Sidebar) hashAttention() uint64 {
	h := fnv.New64a()

	hashMap := func(prefix byte, m map[string]bool) {
		keys := make([]string, 0, len(m))
		for id := range m {
			keys = append(keys, id)
		}
		sort.Strings(keys)
		for _, id := range keys {
			h.Write([]byte{prefix})
			h.Write([]byte(id))
		}
	}

	hashMap('P', s.pendingPermissions)
	hashMap('Q', s.pendingQuestions)
	hashMap('S', s.streamingSessions)
	hashMap('I', s.idleWithResponse)
	hashMap('U', s.uncommittedChanges)
	hashMap('C', s.hasNewComments)
	return h.Sum64()
}

// SetRepos sets the list of registered repos so they appear in the sidebar
// even when they have no sessions.
func (s *Sidebar) SetRepos(repos []string) {
	s.registeredRepos = repos
	s.lastHash = 0 // Invalidate cache so next SetSessions rebuilds items
}

// SetSessions updates the session list, grouping by repo
func (s *Sidebar) SetSessions(sessions []config.Session) {
	// Fast path: check if sessions or attention state have changed
	newHash := hashSessions(sessions)
	newAttnHash := s.hashAttention()
	if newHash == s.lastHash && newAttnHash == s.lastAttnHash && len(sessions) == len(s.sessions) {
		// No structural or attention changes - skip expensive tree rebuild
		return
	}
	s.lastHash = newHash
	s.lastAttnHash = newAttnHash

	// Group sessions by repo path
	groupMap := make(map[string]*repoGroup)
	var groupOrder []string

	// Add registered repos first so they always appear (even without sessions)
	for _, repo := range s.registeredRepos {
		if _, exists := groupMap[repo]; !exists {
			groupMap[repo] = &repoGroup{
				RepoPath: repo,
				RepoName: filepath.Base(repo),
				Sessions: []config.Session{},
			}
			groupOrder = append(groupOrder, repo)
		}
	}

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

	// Build ordered groups with tree structure and priority sorting
	s.groups = make([]repoGroup, 0, len(groupOrder))
	for _, path := range groupOrder {
		group := groupMap[path]
		group.RootNodes = buildSessionTree(group.Sessions)
		s.sortNodesByPriority(group.RootNodes)
		s.groups = append(s.groups, *group)
	}

	// Rebuild flat sessions list in tree order (parents before children)
	s.sessions = make([]config.Session, 0, len(sessions))
	for _, group := range s.groups {
		flattenSessionTree(group.RootNodes, &s.sessions)
	}

	// Build flat items list: repo header + sessions + new session action for each group
	s.items = make([]sidebarItem, 0, len(s.sessions)+len(s.groups)*2)
	for _, group := range s.groups {
		s.items = append(s.items, sidebarItem{
			Kind:     itemKindRepo,
			RepoPath: group.RepoPath,
		})
		for _, node := range group.RootNodes {
			flattenSessionTreeToItems(node, group.RepoPath, &s.items)
		}
		s.items = append(s.items, sidebarItem{
			Kind:     itemKindNewSession,
			RepoPath: group.RepoPath,
		})
	}

	// Adjust selection if needed
	if s.selectedIdx >= len(s.items) {
		s.selectedIdx = len(s.items) - 1
	}
	if s.selectedIdx < 0 {
		s.selectedIdx = 0
	}

	// If the current selection is a repo header and there are sessions,
	// advance to the first session to preserve expected default behavior.
	if s.selectedIdx == 0 && len(s.items) > 0 && s.items[0].Kind == itemKindRepo {
		if len(s.items) > 1 && s.items[1].Kind == itemKindSession {
			s.selectedIdx = 1
		}
	}
}

// buildSessionTree builds a tree structure from a flat list of sessions
func buildSessionTree(sessions []config.Session) []sessionNode {
	// Create a map of session ID to session for quick lookup
	sessionMap := make(map[string]config.Session)
	for _, sess := range sessions {
		sessionMap[sess.ID] = sess
	}

	// Create a map of parent ID to children
	childrenMap := make(map[string][]config.Session)
	var rootSessions []config.Session

	for _, sess := range sessions {
		if sess.ParentID == "" {
			// No parent - this is a root session
			rootSessions = append(rootSessions, sess)
		} else if _, parentExists := sessionMap[sess.ParentID]; parentExists {
			// Parent exists in this repo group - add as child
			childrenMap[sess.ParentID] = append(childrenMap[sess.ParentID], sess)
		} else {
			// Parent doesn't exist (deleted?) - treat as root
			rootSessions = append(rootSessions, sess)
		}
	}

	// Build tree recursively
	var buildNode func(sess config.Session) sessionNode
	buildNode = func(sess config.Session) sessionNode {
		node := sessionNode{Session: sess}
		if children, hasChildren := childrenMap[sess.ID]; hasChildren {
			for _, child := range children {
				node.Children = append(node.Children, buildNode(child))
			}
		}
		return node
	}

	var roots []sessionNode
	for _, sess := range rootSessions {
		roots = append(roots, buildNode(sess))
	}
	return roots
}

// flattenSessionTree flattens a tree into a slice (depth-first, parent before children)
func flattenSessionTree(nodes []sessionNode, result *[]config.Session) {
	for _, node := range nodes {
		*result = append(*result, node.Session)
		flattenSessionTree(node.Children, result)
	}
}

// flattenSessionTreeToItems flattens a session tree into sidebarItem entries.
func flattenSessionTreeToItems(node sessionNode, repoPath string, items *[]sidebarItem) {
	*items = append(*items, sidebarItem{
		Kind:     itemKindSession,
		Session:  node.Session,
		RepoPath: repoPath,
	})
	for _, child := range node.Children {
		flattenSessionTreeToItems(child, repoPath, items)
	}
}

// SelectedSession returns the currently selected session, or nil if a repo is selected.
func (s *Sidebar) SelectedSession() *config.Session {
	// In search mode, use filtered sessions (flat, no repo items)
	if s.searchMode && s.filteredSessions != nil {
		if s.selectedIdx >= 0 && s.selectedIdx < len(s.filteredSessions) {
			return &s.filteredSessions[s.selectedIdx]
		}
		return nil
	}
	if s.selectedIdx < 0 || s.selectedIdx >= len(s.items) {
		return nil
	}
	item := &s.items[s.selectedIdx]
	if item.Kind != itemKindSession {
		return nil
	}
	return &item.Session
}

// SelectedRepo returns the repo path of the currently selected repo header,
// or empty string if a session (or nothing) is selected.
func (s *Sidebar) SelectedRepo() string {
	if s.searchMode {
		return ""
	}
	if s.selectedIdx < 0 || s.selectedIdx >= len(s.items) {
		return ""
	}
	item := &s.items[s.selectedIdx]
	if item.Kind != itemKindRepo {
		return ""
	}
	return item.RepoPath
}

// IsRepoSelected returns true when a repo header is currently selected.
func (s *Sidebar) IsRepoSelected() bool {
	return s.SelectedRepo() != ""
}

// SelectedNewSessionRepo returns the repo path if the "+ New Session" action is selected,
// or empty string otherwise.
func (s *Sidebar) SelectedNewSessionRepo() string {
	if s.searchMode {
		return ""
	}
	if s.selectedIdx < 0 || s.selectedIdx >= len(s.items) {
		return ""
	}
	item := &s.items[s.selectedIdx]
	if item.Kind != itemKindNewSession {
		return ""
	}
	return item.RepoPath
}

// IsNewSessionSelected returns true when a "+ New Session" action is selected.
func (s *Sidebar) IsNewSessionSelected() bool {
	return s.SelectedNewSessionRepo() != ""
}

// SelectSession selects a session by ID
func (s *Sidebar) SelectSession(id string) {
	if s.searchMode && s.filteredSessions != nil {
		for i, sess := range s.filteredSessions {
			if sess.ID == id {
				s.selectedIdx = i
				return
			}
		}
		return
	}
	for i, item := range s.items {
		if item.Kind == itemKindSession && item.Session.ID == id {
			s.selectedIdx = i
			return
		}
	}
}

// SetStreaming sets the streaming state for a session
func (s *Sidebar) SetStreaming(sessionID string, streaming bool) {
	log := logger.WithComponent("sidebar")
	if streaming {
		s.streamingSessions[sessionID] = true
		log.Info("SetStreaming", "sessionID", sessionID, "streaming", true, "totalStreaming", len(s.streamingSessions))
	} else {
		delete(s.streamingSessions, sessionID)
		log.Info("SetStreaming", "sessionID", sessionID, "streaming", false, "totalStreaming", len(s.streamingSessions))
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

// SetPendingQuestion sets whether a session has a pending question
func (s *Sidebar) SetPendingQuestion(sessionID string, pending bool) {
	if pending {
		s.pendingQuestions[sessionID] = true
	} else {
		delete(s.pendingQuestions, sessionID)
	}
}

// SetIdleWithResponse marks that a session has finished streaming and awaits user response
func (s *Sidebar) SetIdleWithResponse(sessionID string, idle bool) {
	if idle {
		s.idleWithResponse[sessionID] = true
	} else {
		delete(s.idleWithResponse, sessionID)
	}
}

// SetUncommittedChanges sets whether a session has uncommitted changes
func (s *Sidebar) SetUncommittedChanges(sessionID string, has bool) {
	if has {
		s.uncommittedChanges[sessionID] = true
	} else {
		delete(s.uncommittedChanges, sessionID)
	}
}

// SetHasNewComments sets whether a session has new PR review comments
func (s *Sidebar) SetHasNewComments(sessionID string, has bool) {
	if has {
		s.hasNewComments[sessionID] = true
	} else {
		delete(s.hasNewComments, sessionID)
	}
}

// HasNewComments returns whether a session has new PR review comments
func (s *Sidebar) HasNewComments(sessionID string) bool {
	return s.hasNewComments[sessionID]
}

// Attention priority levels (lower = higher priority, needs attention sooner)
const (
	priorityPermission  = 0 // Pending permission/question/plan approval
	priorityStreaming   = 1 // Actively streaming
	priorityIdle        = 2 // Idle with response (streaming finished, user hasn't responded)
	priorityUncommitted = 3 // Has uncommitted changes to review
	priorityNewComments = 4 // Has unread PR review comments
	priorityNormal      = 5 // Normal session
)

// sessionPriority returns the attention priority for a given session ID.
func (s *Sidebar) sessionPriority(sessionID string) int {
	if s.pendingPermissions[sessionID] || s.pendingQuestions[sessionID] {
		return priorityPermission
	}
	if s.streamingSessions[sessionID] {
		return priorityStreaming
	}
	if s.idleWithResponse[sessionID] {
		return priorityIdle
	}
	if s.uncommittedChanges[sessionID] {
		return priorityUncommitted
	}
	if s.hasNewComments[sessionID] {
		return priorityNewComments
	}
	return priorityNormal
}

// effectivePriority returns the best (lowest) priority across a node and all its descendants.
func (s *Sidebar) effectivePriority(node sessionNode) int {
	best := s.sessionPriority(node.Session.ID)
	for _, child := range node.Children {
		childPriority := s.effectivePriority(child)
		if childPriority < best {
			best = childPriority
		}
	}
	return best
}

// sortNodesByPriority sorts root nodes and their children by attention priority.
// Uses stable sort to preserve original order for sessions with the same priority.
func (s *Sidebar) sortNodesByPriority(nodes []sessionNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		return s.effectivePriority(nodes[i]) < s.effectivePriority(nodes[j])
	})
	// Recursively sort children within each parent
	for i := range nodes {
		if len(nodes[i].Children) > 1 {
			s.sortNodesByPriority(nodes[i].Children)
		}
	}
}

// =============================================================================
// Multi-select mode
// =============================================================================

// EnterMultiSelect enters multi-select mode, pre-selecting the current item
func (s *Sidebar) EnterMultiSelect() {
	s.multiSelectMode = true
	s.selectedSessions = make(map[string]bool)
	// Pre-select the currently highlighted session
	sessions := s.visibleSessions()
	if s.selectedIdx >= 0 && s.selectedIdx < len(sessions) {
		s.selectedSessions[sessions[s.selectedIdx].ID] = true
	}
}

// ExitMultiSelect exits multi-select mode and clears selections
func (s *Sidebar) ExitMultiSelect() {
	s.multiSelectMode = false
	s.selectedSessions = make(map[string]bool)
}

// IsMultiSelectMode returns whether multi-select mode is active
func (s *Sidebar) IsMultiSelectMode() bool {
	return s.multiSelectMode
}

// GetSelectedSessionIDs returns the IDs of all selected sessions
func (s *Sidebar) GetSelectedSessionIDs() []string {
	var ids []string
	for id := range s.selectedSessions {
		ids = append(ids, id)
	}
	return ids
}

// ToggleSelected toggles the selection of the currently highlighted session
func (s *Sidebar) ToggleSelected() {
	sessions := s.visibleSessions()
	if s.selectedIdx < 0 || s.selectedIdx >= len(sessions) {
		return
	}
	id := sessions[s.selectedIdx].ID
	if s.selectedSessions[id] {
		delete(s.selectedSessions, id)
	} else {
		s.selectedSessions[id] = true
	}
}

// SelectAll selects all visible sessions
func (s *Sidebar) SelectAll() {
	sessions := s.visibleSessions()
	for _, sess := range sessions {
		s.selectedSessions[sess.ID] = true
	}
}

// DeselectAll deselects all sessions
func (s *Sidebar) DeselectAll() {
	s.selectedSessions = make(map[string]bool)
}

// SelectedCount returns the number of selected sessions
func (s *Sidebar) SelectedCount() int {
	return len(s.selectedSessions)
}

// visibleSessions returns the sessions currently visible (filtered or all)
func (s *Sidebar) visibleSessions() []config.Session {
	if s.searchMode {
		return s.filteredSessions
	}
	return s.sessions
}

// SidebarTick returns a command that sends a tick message after a delay
func SidebarTick() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(t time.Time) tea.Msg {
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
	if s.selectedIdx >= len(s.items) {
		s.selectedIdx = len(s.items) - 1
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
		var cmds []tea.Cmd
		if s.IsStreaming() {
			// Advance the spinner with easing (some frames hold longer)
			s.spinnerTick++
			holdTime := sidebarSpinnerHoldTimes[s.spinnerFrame%len(sidebarSpinnerHoldTimes)]
			if s.spinnerTick >= holdTime {
				s.spinnerTick = 0
				s.spinnerFrame = (s.spinnerFrame + 1) % len(sidebarSpinnerFrames)
			}
			cmds = append(cmds, SidebarTick())
		}
		if len(cmds) > 0 {
			return s, tea.Batch(cmds...)
		}
		return s, nil

	case tea.KeyPressMsg:
		if !s.focused {
			return s, nil
		}

		// Handle search mode input
		if s.searchMode {
			switch msg.String() {
			case keys.Escape:
				s.ExitSearchMode()
				return s, nil
			case keys.Enter:
				// Exit search mode but keep filter applied (user selected)
				s.searchMode = false
				s.searchInput.Blur()
				return s, nil
			case keys.Up, keys.CtrlP:
				displaySessions := s.getDisplaySessions()
				if s.selectedIdx > 0 {
					s.selectedIdx--
					s.ensureVisibleFiltered(displaySessions)
				}
				return s, nil
			case keys.Down, keys.CtrlN:
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
		case keys.Up, "k":
			if s.selectedIdx > 0 {
				s.selectedIdx--
				s.ensureVisible()
			}
		case keys.Down, "j":
			if s.selectedIdx < len(s.items)-1 {
				s.selectedIdx++
				s.ensureVisible()
			}
		}
	}

	return s, nil
}

// ensureVisible is now handled in View() where we have accurate line counts
// after text wrapping. This is kept as a no-op for API compatibility.
func (s *Sidebar) ensureVisible() {
	// Scroll adjustment happens in View() with actual rendered line counts
}

// ensureVisibleFiltered is now handled in View() where we have accurate line counts
// after text wrapping. This is kept as a no-op for API compatibility.
func (s *Sidebar) ensureVisibleFiltered(displaySessions []config.Session) {
	// Scroll adjustment happens in View() with actual rendered line counts
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

	if len(displaySessions) == 0 && len(s.items) == 0 {
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
		// Use actual lines to handle text wrapping correctly
		var allLines []string
		selectedStartLine := 0
		innerWidth := ctx.InnerWidth(s.width)

		for idx, sess := range s.filteredSessions {
			displayName := s.renderSessionName(sess, idx)
			itemStyle := SidebarItemStyle.Width(innerWidth)
			if idx == s.selectedIdx {
				itemStyle = SidebarSelectedStyle.Width(innerWidth)
				displayName = "> " + strings.TrimPrefix(displayName, "  ")
				selectedStartLine = len(allLines)
			}
			// Render and split into actual lines
			rendered := itemStyle.Render(displayName)
			for _, line := range strings.Split(rendered, "\n") {
				allLines = append(allLines, line)
			}
		}

		// Adjust scroll to keep selected session visible
		visibleHeight := innerHeight
		if selectedStartLine < s.scrollOffset {
			s.scrollOffset = selectedStartLine
		} else if selectedStartLine >= s.scrollOffset+visibleHeight {
			s.scrollOffset = selectedStartLine - visibleHeight + 1
		}

		// Ensure scrollOffset is valid
		if s.scrollOffset < 0 {
			s.scrollOffset = 0
		}
		maxScroll := len(allLines) - visibleHeight
		if maxScroll < 0 {
			maxScroll = 0
		}
		if s.scrollOffset > maxScroll {
			s.scrollOffset = maxScroll
		}

		// Apply scrolling and truncate
		if s.scrollOffset > 0 && s.scrollOffset < len(allLines) {
			allLines = allLines[s.scrollOffset:]
		}
		if len(allLines) > visibleHeight {
			allLines = allLines[:visibleHeight]
		}
		content = strings.Join(allLines, "\n")
	} else {
		// Build the grouped list (normal mode) with tree structure
		// Use actual lines (not items) to handle text wrapping correctly
		var allLines []string
		selectedStartLine := 0 // Line where selected item starts

		itemIdx := 0
		innerWidth := ctx.InnerWidth(s.width)

		for i, group := range s.groups {
			// Add blank line between repos (not before first one)
			if i > 0 {
				allLines = append(allLines, "")
			}

			// Repo header (selectable)
			isRepoSelected := itemIdx == s.selectedIdx
			repoName := group.RepoName
			if isRepoSelected {
				repoStyle := SidebarSelectedStyle.Width(innerWidth).Bold(true)
				selectedStartLine = len(allLines)
				rendered := repoStyle.Render("> " + repoName)
				for _, line := range strings.Split(rendered, "\n") {
					allLines = append(allLines, line)
				}
			} else {
				repoStyle := lipgloss.NewStyle().
					Foreground(ColorTextMuted).
					Bold(true)
				allLines = append(allLines, repoStyle.Render(repoName))
			}
			itemIdx++

			// Render sessions in tree order with indentation
			var renderNode func(node sessionNode, depth int, isLastChild bool)
			renderNode = func(node sessionNode, depth int, isLastChild bool) {
				isSelected := itemIdx == s.selectedIdx
				hasChildren := len(node.Children) > 0
				displayName := s.renderSessionNode(node.Session, depth, isSelected, hasChildren, isLastChild)

				itemStyle := SidebarItemStyle.Width(innerWidth)
				if isSelected {
					itemStyle = SidebarSelectedStyle.Width(innerWidth)
					selectedStartLine = len(allLines) // Record where selected session starts
				}

				// Render and split into actual lines (handles text wrapping)
				rendered := itemStyle.Render(displayName)
				for _, line := range strings.Split(rendered, "\n") {
					allLines = append(allLines, line)
				}
				itemIdx++

				// Render children with increased depth
				for i, child := range node.Children {
					childIsLast := i == len(node.Children)-1
					renderNode(child, depth+1, childIsLast)
				}
			}

			for i, node := range group.RootNodes {
				isLast := i == len(group.RootNodes)-1
				renderNode(node, 0, isLast)
			}

			// "+ New Session" action item
			isNewSelected := itemIdx == s.selectedIdx
			newLabel := "  + New Session"
			if isNewSelected {
				newStyle := SidebarSelectedStyle.Width(innerWidth)
				selectedStartLine = len(allLines)
				rendered := newStyle.Render("> + New Session")
				for _, line := range strings.Split(rendered, "\n") {
					allLines = append(allLines, line)
				}
			} else {
				newStyle := lipgloss.NewStyle().
					Foreground(ColorTextMuted).
					Italic(true)
				allLines = append(allLines, newStyle.Render(newLabel))
			}
			itemIdx++
		}

		// Adjust scroll to keep selected session visible
		visibleHeight := innerHeight
		if selectedStartLine < s.scrollOffset {
			// Selected is above visible area - scroll up
			s.scrollOffset = selectedStartLine
		} else if selectedStartLine >= s.scrollOffset+visibleHeight {
			// Selected is below visible area - scroll down
			s.scrollOffset = selectedStartLine - visibleHeight + 1
		}

		// Ensure scrollOffset is valid
		if s.scrollOffset < 0 {
			s.scrollOffset = 0
		}
		maxScroll := len(allLines) - visibleHeight
		if maxScroll < 0 {
			maxScroll = 0
		}
		if s.scrollOffset > maxScroll {
			s.scrollOffset = maxScroll
		}

		// Apply scrolling and truncate
		if s.scrollOffset > 0 && s.scrollOffset < len(allLines) {
			allLines = allLines[s.scrollOffset:]
		}
		if len(allLines) > visibleHeight {
			allLines = allLines[:visibleHeight]
		}

		content = strings.Join(allLines, "\n")
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
	isSelected := sessionIdx == s.selectedIdx
	return s.renderSessionNameWithDepth(sess, 0, isSelected)
}

// renderSessionNameWithDepth builds the display name for a session with indentation based on depth
// This is a compatibility wrapper - use renderSessionNode for full tree rendering
func (s *Sidebar) renderSessionNameWithDepth(sess config.Session, depth int, isSelected bool) string {
	return s.renderSessionNode(sess, depth, isSelected, false, true)
}

// renderSessionNode builds the display name for a session with git-log style tree visualization
// hasChildren: whether this session has child sessions (forks)
// isLastChild: whether this is the last child of its parent (for connector style)
func (s *Sidebar) renderSessionNode(sess config.Session, depth int, isSelected bool, hasChildren bool, isLastChild bool) string {
	// Determine the node symbol based on state priority:
	// 1. Pending permission (⚠) - highest priority, needs attention
	// 2. Streaming (spinner) - active work happening
	// 3. Merged status (✓) - completed state
	// 4. PR created - has PR but not merged
	// 5. Default node type (◆/◇) - base state
	var nodeSymbol string
	var symbolColor color.Color

	if s.HasPendingPermission(sess.ID) {
		// Pending permission - needs attention
		nodeSymbol = "⚠"
		symbolColor = ColorWarning
	} else if s.IsSessionStreaming(sess.ID) {
		// Streaming - use animated spinner
		nodeSymbol = sidebarSpinnerFrames[s.spinnerFrame]
		symbolColor = ColorPrimary
	} else if sess.MergedToParent || sess.Merged {
		// Merged to parent or main branch
		nodeSymbol = "✓"
		symbolColor = ColorSecondary
	} else if sess.PRMerged {
		// PR merged on GitHub
		nodeSymbol = "✓"
		symbolColor = ColorSuccess
	} else if sess.PRClosed {
		// PR closed without merging
		nodeSymbol = "✕"
		symbolColor = ColorError
	} else if sess.PRCreated {
		// PR created but still open
		nodeSymbol = "⬡" // hexagon to indicate PR
		symbolColor = ColorUser
	} else if hasChildren {
		// Has children - parent node
		nodeSymbol = "◆"
		symbolColor = ColorPrimary
	} else {
		// No children - regular session
		nodeSymbol = "◇"
		symbolColor = ColorTextMuted
	}

	// Build the prefix with tree structure
	// Symbol positions by depth:
	//   depth 0: " ◆ name"       symbol at column 1
	//   depth 1: " ╰─◆ name"     symbol at column 3, connector at column 1 (under parent)
	//   depth 2: "   ╰─◇ name"   symbol at column 5, connector at column 3 (under parent)
	// Pattern: symbol at column (1 + 2*depth), connector 2 columns before symbol
	var prefix string
	if depth == 0 {
		// Root level - just the node symbol with padding
		prefix = " " + nodeSymbol + " "
	} else {
		// Child level - use tree connectors
		// Indent puts connector under parent's symbol (at column 1 + 2*(depth-1))
		indent := strings.Repeat("  ", depth-1)
		if isLastChild {
			prefix = " " + indent + "╰─" + nodeSymbol + " "
		} else {
			prefix = " " + indent + "├─" + nodeSymbol + " "
		}
	}

	// Apply styling to prefix (node symbol color)
	var styledPrefix string
	if isSelected {
		// Selected - let parent style handle colors
		styledPrefix = prefix
	} else {
		symbolStyle := lipgloss.NewStyle().Foreground(symbolColor)

		// Style just the node symbol, keep connectors muted
		if depth == 0 {
			styledPrefix = " " + symbolStyle.Render(nodeSymbol) + " "
		} else {
			connectorStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
			indent := strings.Repeat("  ", depth-1)
			connector := "├─"
			if isLastChild {
				connector = "╰─"
			}
			styledPrefix = " " + connectorStyle.Render(indent+connector) + symbolStyle.Render(nodeSymbol) + " "
		}
	}

	// Display the session name (extracts last part for old-style names)
	var name string
	if parts := strings.Split(sess.Name, "/"); len(parts) > 1 {
		name = parts[len(parts)-1]
	} else {
		name = sess.Name
	}

	displayName := styledPrefix + name

	// Show autonomous mode indicator
	if sess.Autonomous {
		if isSelected {
			displayName += " [AUTO]"
		} else {
			autoStyle := lipgloss.NewStyle().Foreground(ColorInfo)
			displayName += autoStyle.Render(" [AUTO]")
		}
	}

	// Show new comments indicator
	if s.hasNewComments[sess.ID] {
		if isSelected {
			displayName += " *"
		} else {
			commentStyle := lipgloss.NewStyle().Foreground(ColorInfo)
			displayName += commentStyle.Render(" *")
		}
	}

	// In multi-select mode, prepend a checkbox
	if s.multiSelectMode {
		checkbox := "[ ] "
		if s.selectedSessions[sess.ID] {
			checkbox = "[x] "
		}
		if isSelected {
			displayName = checkbox + displayName
		} else {
			checkStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
			if s.selectedSessions[sess.ID] {
				checkStyle = checkStyle.Foreground(ColorSecondary)
			}
			displayName = checkStyle.Render(checkbox) + displayName
		}
	}

	return displayName
}
