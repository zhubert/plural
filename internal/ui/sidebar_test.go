package ui

import (
	"strings"
	"testing"

	"github.com/zhubert/plural/internal/config"
)

func TestNewSidebar(t *testing.T) {
	sidebar := NewSidebar()

	if sidebar == nil {
		t.Fatal("NewSidebar() returned nil")
	}

	if sidebar.selectedIdx != 0 {
		t.Errorf("Expected selectedIdx 0, got %d", sidebar.selectedIdx)
	}

	if sidebar.streamingSessions == nil {
		t.Error("streamingSessions map should be initialized")
	}

	if sidebar.pendingPermissions == nil {
		t.Error("pendingPermissions map should be initialized")
	}
}

func TestSidebar_SetSize(t *testing.T) {
	sidebar := NewSidebar()

	sidebar.SetSize(40, 24)

	if sidebar.width != 40 {
		t.Errorf("Expected width 40, got %d", sidebar.width)
	}

	if sidebar.height != 24 {
		t.Errorf("Expected height 24, got %d", sidebar.height)
	}

	if sidebar.Width() != 40 {
		t.Errorf("Width() should return 40, got %d", sidebar.Width())
	}
}

func TestSidebar_FocusState(t *testing.T) {
	sidebar := NewSidebar()

	// Initially not focused
	if sidebar.IsFocused() {
		t.Error("Should not be focused initially")
	}

	// Set focused
	sidebar.SetFocused(true)
	if !sidebar.IsFocused() {
		t.Error("Should be focused after SetFocused(true)")
	}

	// Unfocus
	sidebar.SetFocused(false)
	if sidebar.IsFocused() {
		t.Error("Should not be focused after SetFocused(false)")
	}
}

func TestSidebar_SetSessions(t *testing.T) {
	sidebar := NewSidebar()

	sessions := []config.Session{
		{ID: "session-1", RepoPath: "/repo1", Branch: "plural-abc"},
		{ID: "session-2", RepoPath: "/repo1", Branch: "custom-branch"},
		{ID: "session-3", RepoPath: "/repo2", Branch: "plural-xyz"},
	}

	sidebar.SetSessions(sessions)

	if len(sidebar.sessions) != 3 {
		t.Errorf("Expected 3 sessions, got %d", len(sidebar.sessions))
	}

	// Should be grouped by repo
	if len(sidebar.groups) != 2 {
		t.Errorf("Expected 2 groups (repos), got %d", len(sidebar.groups))
	}

	// First group should have 2 sessions
	if len(sidebar.groups[0].Sessions) != 2 {
		t.Errorf("Expected 2 sessions in first group, got %d", len(sidebar.groups[0].Sessions))
	}

	// Second group should have 1 session
	if len(sidebar.groups[1].Sessions) != 1 {
		t.Errorf("Expected 1 session in second group, got %d", len(sidebar.groups[1].Sessions))
	}
}

func TestSidebar_SetSessions_Empty(t *testing.T) {
	sidebar := NewSidebar()
	sidebar.selectedIdx = 5 // Set an invalid index

	sidebar.SetSessions([]config.Session{})

	if len(sidebar.sessions) != 0 {
		t.Errorf("Expected 0 sessions, got %d", len(sidebar.sessions))
	}

	if sidebar.selectedIdx != 0 {
		t.Errorf("Selected index should reset to 0, got %d", sidebar.selectedIdx)
	}
}

func TestSidebar_SetSessions_AdjustsSelection(t *testing.T) {
	sidebar := NewSidebar()

	// Set many sessions
	sessions := make([]config.Session, 10)
	for i := range sessions {
		sessions[i] = config.Session{
			ID:       string(rune('a' + i)),
			RepoPath: "/repo",
			Branch:   "branch",
		}
	}
	sidebar.SetSessions(sessions)
	sidebar.selectedIdx = 9

	// Now set fewer sessions
	sidebar.SetSessions(sessions[:3])

	if sidebar.selectedIdx >= 3 {
		t.Errorf("Selection should be adjusted, got %d", sidebar.selectedIdx)
	}
}

func TestSidebar_SelectedSession(t *testing.T) {
	sidebar := NewSidebar()

	// No sessions - should return nil
	sess := sidebar.SelectedSession()
	if sess != nil {
		t.Error("SelectedSession should return nil when no sessions")
	}

	// Add sessions
	sessions := []config.Session{
		{ID: "session-1", RepoPath: "/repo1", Branch: "b1"},
		{ID: "session-2", RepoPath: "/repo1", Branch: "b2"},
	}
	sidebar.SetSessions(sessions)

	sess = sidebar.SelectedSession()
	if sess == nil {
		t.Fatal("SelectedSession should return a session")
	}

	if sess.ID != "session-1" {
		t.Errorf("Expected first session, got %s", sess.ID)
	}
}

func TestSidebar_SelectSession(t *testing.T) {
	sidebar := NewSidebar()

	sessions := []config.Session{
		{ID: "session-1", RepoPath: "/repo", Branch: "b1"},
		{ID: "session-2", RepoPath: "/repo", Branch: "b2"},
		{ID: "session-3", RepoPath: "/repo", Branch: "b3"},
	}
	sidebar.SetSessions(sessions)

	// Select by ID
	sidebar.SelectSession("session-2")
	if sidebar.selectedIdx != 1 {
		t.Errorf("Expected selectedIdx 1, got %d", sidebar.selectedIdx)
	}

	// Select non-existent session (should not change)
	sidebar.SelectSession("nonexistent")
	if sidebar.selectedIdx != 1 {
		t.Errorf("Selection should not change, got %d", sidebar.selectedIdx)
	}
}

func TestSidebar_Streaming(t *testing.T) {
	sidebar := NewSidebar()

	// Initially not streaming
	if sidebar.IsStreaming() {
		t.Error("Should not be streaming initially")
	}

	if sidebar.IsSessionStreaming("session-1") {
		t.Error("Session should not be streaming initially")
	}

	// Set streaming
	sidebar.SetStreaming("session-1", true)

	if !sidebar.IsStreaming() {
		t.Error("Should be streaming after SetStreaming")
	}

	if !sidebar.IsSessionStreaming("session-1") {
		t.Error("Session-1 should be streaming")
	}

	if sidebar.IsSessionStreaming("session-2") {
		t.Error("Session-2 should not be streaming")
	}

	// Multiple sessions streaming
	sidebar.SetStreaming("session-2", true)
	if !sidebar.IsStreaming() {
		t.Error("Should still be streaming")
	}

	// Stop streaming for one
	sidebar.SetStreaming("session-1", false)
	if !sidebar.IsStreaming() {
		t.Error("Should still be streaming (session-2)")
	}

	if sidebar.IsSessionStreaming("session-1") {
		t.Error("Session-1 should not be streaming after disable")
	}

	// Stop all streaming
	sidebar.SetStreaming("session-2", false)
	if sidebar.IsStreaming() {
		t.Error("Should not be streaming after all disabled")
	}
}

func TestSidebar_PendingPermission(t *testing.T) {
	sidebar := NewSidebar()

	// Initially no pending permissions
	if sidebar.HasPendingPermission("session-1") {
		t.Error("Should not have pending permission initially")
	}

	// Set pending permission
	sidebar.SetPendingPermission("session-1", true)

	if !sidebar.HasPendingPermission("session-1") {
		t.Error("Should have pending permission after set")
	}

	if sidebar.HasPendingPermission("session-2") {
		t.Error("Session-2 should not have pending permission")
	}

	// Clear pending permission
	sidebar.SetPendingPermission("session-1", false)

	if sidebar.HasPendingPermission("session-1") {
		t.Error("Should not have pending permission after clear")
	}
}

func TestSidebar_SpinnerFrames(t *testing.T) {
	if len(sidebarSpinnerFrames) == 0 {
		t.Error("sidebarSpinnerFrames should not be empty")
	}

	if len(sidebarSpinnerHoldTimes) != len(sidebarSpinnerFrames) {
		t.Errorf("sidebarSpinnerHoldTimes length (%d) should match sidebarSpinnerFrames (%d)",
			len(sidebarSpinnerHoldTimes), len(sidebarSpinnerFrames))
	}

	// Verify all hold times are positive
	for i, holdTime := range sidebarSpinnerHoldTimes {
		if holdTime < 1 {
			t.Errorf("sidebarSpinnerHoldTimes[%d] = %d, should be >= 1", i, holdTime)
		}
	}
}

func TestSidebar_View_NoSessions(t *testing.T) {
	sidebar := NewSidebar()
	sidebar.SetSize(40, 24)

	view := sidebar.View()

	if view == "" {
		t.Error("View should not be empty even with no sessions")
	}

	// Should contain "No sessions" message
	// (We can't easily check the styled content, but it shouldn't panic)
}

func TestSidebar_View_WithSessions(t *testing.T) {
	sidebar := NewSidebar()
	sidebar.SetSize(40, 24)

	sessions := []config.Session{
		{ID: "session-1", Name: "repo/session1", RepoPath: "/repo", Branch: "plural-abc"},
		{ID: "session-2", Name: "repo/session2", RepoPath: "/repo", Branch: "custom-branch"},
	}
	sidebar.SetSessions(sessions)

	view := sidebar.View()

	if view == "" {
		t.Error("View should not be empty")
	}
}

func TestSidebar_View_WithIndicators(t *testing.T) {
	sidebar := NewSidebar()
	sidebar.SetSize(40, 24)

	sessions := []config.Session{
		{ID: "session-1", Name: "repo/session1", RepoPath: "/repo", Branch: "b1"},
		{ID: "session-2", Name: "repo/session2", RepoPath: "/repo", Branch: "b2", Merged: true},
		{ID: "session-3", Name: "repo/session3", RepoPath: "/repo", Branch: "b3", PRCreated: true},
		{ID: "session-4", Name: "repo/session4", RepoPath: "/repo", Branch: "b4", ParentID: "session-1", MergedToParent: true},
	}
	sidebar.SetSessions(sessions)
	sidebar.SetStreaming("session-1", true)
	sidebar.SetPendingPermission("session-2", true)

	// Should not panic when rendering with all indicators
	view := sidebar.View()
	if view == "" {
		t.Error("View should not be empty")
	}

	// Verify MergedToParent indicator is shown (uses ✓ as node symbol)
	if !strings.Contains(view, "✓") {
		t.Error("View should contain '✓' node symbol for merged session-4")
	}
}

func TestSidebar_View_MultipleSpinners(t *testing.T) {
	sidebar := NewSidebar()
	sidebar.SetSize(60, 24) // Wide enough to show spinners

	sessions := []config.Session{
		{ID: "session-1", Name: "repo/session1", RepoPath: "/repo", Branch: "b1"},
		{ID: "session-2", Name: "repo/session2", RepoPath: "/repo", Branch: "b2"},
		{ID: "session-3", Name: "repo/session3", RepoPath: "/repo", Branch: "b3"},
	}
	sidebar.SetSessions(sessions)

	// Set multiple sessions as streaming
	sidebar.SetStreaming("session-1", true)
	sidebar.SetStreaming("session-2", true)
	sidebar.SetStreaming("session-3", true)

	// Verify state
	if !sidebar.IsSessionStreaming("session-1") {
		t.Error("session-1 should be streaming")
	}
	if !sidebar.IsSessionStreaming("session-2") {
		t.Error("session-2 should be streaming")
	}
	if !sidebar.IsSessionStreaming("session-3") {
		t.Error("session-3 should be streaming")
	}

	// Get the view and check for spinners
	view := sidebar.View()

	// Count spinner occurrences - the first spinner frame is "·"
	spinnerCount := strings.Count(view, sidebarSpinnerFrames[0])

	if spinnerCount < 3 {
		t.Errorf("Expected at least 3 spinners in view, got %d. View:\n%s", spinnerCount, view)
	}
}

func TestSidebar_View_MultipleSpinners_ForkedSessions(t *testing.T) {
	sidebar := NewSidebar()
	sidebar.SetSize(60, 24) // Wide enough to show spinners

	// Parent session and forked children (mimics createParallelSessions)
	sessions := []config.Session{
		{ID: "parent", Name: "repo/parent", RepoPath: "/repo", Branch: "main"},
		{ID: "child-1", Name: "repo/option-1", RepoPath: "/repo", Branch: "opt1", ParentID: "parent"},
		{ID: "child-2", Name: "repo/option-2", RepoPath: "/repo", Branch: "opt2", ParentID: "parent"},
		{ID: "child-3", Name: "repo/option-3", RepoPath: "/repo", Branch: "opt3", ParentID: "parent"},
	}
	sidebar.SetSessions(sessions)

	// Set all children as streaming (mimics what happens during parallel session creation)
	sidebar.SetStreaming("child-1", true)
	sidebar.SetStreaming("child-2", true)
	sidebar.SetStreaming("child-3", true)

	// Verify state
	if len(sidebar.streamingSessions) != 3 {
		t.Errorf("Expected 3 streaming sessions, got %d", len(sidebar.streamingSessions))
	}

	// Get the view and check for spinners
	view := sidebar.View()

	// Count spinner occurrences - the first spinner frame is "·"
	spinnerCount := strings.Count(view, sidebarSpinnerFrames[0])

	if spinnerCount < 3 {
		t.Errorf("Expected at least 3 spinners in view for forked sessions, got %d. View:\n%s", spinnerCount, view)
	}
}

func TestSidebar_TreeOrder(t *testing.T) {
	// This test ensures sessions are ordered correctly when they have
	// parent-child relationships (tree order differs from input order)
	sidebar := NewSidebar()

	// Create sessions where input order differs from tree order
	// Input order: parent, sibling, child (of parent)
	// Tree order:  parent, child, sibling
	sessions := []config.Session{
		{ID: "parent", RepoPath: "/repo1", Branch: "b1", Name: "parent"},
		{ID: "sibling", RepoPath: "/repo1", Branch: "b2", Name: "sibling"},
		{ID: "child", RepoPath: "/repo1", Branch: "b3", Name: "child", ParentID: "parent"},
	}
	sidebar.SetSessions(sessions)

	// After SetSessions, s.sessions should be in tree order: parent, child, sibling
	if len(sidebar.sessions) != 3 {
		t.Fatalf("Expected 3 sessions, got %d", len(sidebar.sessions))
	}
	if sidebar.sessions[0].ID != "parent" {
		t.Errorf("Expected sessions[0]=parent, got %s", sidebar.sessions[0].ID)
	}
	if sidebar.sessions[1].ID != "child" {
		t.Errorf("Expected sessions[1]=child, got %s", sidebar.sessions[1].ID)
	}
	if sidebar.sessions[2].ID != "sibling" {
		t.Errorf("Expected sessions[2]=sibling, got %s", sidebar.sessions[2].ID)
	}
}

func TestSidebar_RenderSessionNode_NodeSymbols(t *testing.T) {
	sidebar := NewSidebar()

	tests := []struct {
		name           string
		session        config.Session
		hasChildren    bool
		streaming      bool
		expectedSymbol string
		description    string
	}{
		{
			name:           "regular session",
			session:        config.Session{ID: "s1", Name: "test"},
			hasChildren:    false,
			streaming:      false,
			expectedSymbol: "◇",
			description:    "Regular session without children should use ◇",
		},
		{
			name:           "parent session",
			session:        config.Session{ID: "s2", Name: "parent"},
			hasChildren:    true,
			streaming:      false,
			expectedSymbol: "◆",
			description:    "Session with children should use ◆",
		},
		{
			name:           "merged to parent",
			session:        config.Session{ID: "s3", Name: "merged", MergedToParent: true},
			hasChildren:    false,
			streaming:      false,
			expectedSymbol: "✓",
			description:    "Merged to parent session should use ✓",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.streaming {
				sidebar.SetStreaming(tt.session.ID, true)
				defer sidebar.SetStreaming(tt.session.ID, false)
			}

			result := sidebar.renderSessionNode(tt.session, 0, false, tt.hasChildren, true)

			if !strings.Contains(result, tt.expectedSymbol) {
				t.Errorf("%s: expected symbol %s in result %q", tt.description, tt.expectedSymbol, result)
			}
		})
	}
}

func TestSidebar_RenderSessionNode_StreamingSymbol(t *testing.T) {
	sidebar := NewSidebar()
	session := config.Session{ID: "streaming-session", Name: "test"}

	sidebar.SetStreaming(session.ID, true)
	defer sidebar.SetStreaming(session.ID, false)

	result := sidebar.renderSessionNode(session, 0, false, false, true)

	// Should contain the current spinner frame (first frame is "·")
	spinnerFrame := sidebarSpinnerFrames[sidebar.spinnerFrame]
	if !strings.Contains(result, spinnerFrame) {
		t.Errorf("Streaming session should show spinner frame %q, got %q", spinnerFrame, result)
	}
}

func TestSidebar_RenderSessionNode_TreeConnectors(t *testing.T) {
	sidebar := NewSidebar()

	tests := []struct {
		name              string
		depth             int
		isLastChild       bool
		expectedConnector string
	}{
		{
			name:              "root level",
			depth:             0,
			isLastChild:       true,
			expectedConnector: "", // No connector at root
		},
		{
			name:              "middle child",
			depth:             1,
			isLastChild:       false,
			expectedConnector: "├─",
		},
		{
			name:              "last child",
			depth:             1,
			isLastChild:       true,
			expectedConnector: "╰─",
		},
	}

	session := config.Session{ID: "test", Name: "test-session"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sidebar.renderSessionNode(session, tt.depth, false, false, tt.isLastChild)

			if tt.depth == 0 {
				// Root level should not have tree connectors
				if strings.Contains(result, "├─") || strings.Contains(result, "╰─") {
					t.Errorf("Root level should not have tree connectors, got %q", result)
				}
			} else {
				if !strings.Contains(result, tt.expectedConnector) {
					t.Errorf("Expected connector %q in result %q", tt.expectedConnector, result)
				}
			}
		})
	}
}

func TestSidebar_RenderSessionNode_NodeSymbolStatus(t *testing.T) {
	// Tests that status is shown as the node symbol (left side), not right side
	sidebar := NewSidebar()

	tests := []struct {
		name           string
		session        config.Session
		expectedSymbol string
	}{
		{
			name:           "merged session",
			session:        config.Session{ID: "s1", Name: "test", Merged: true},
			expectedSymbol: "✓",
		},
		{
			name:           "merged to parent",
			session:        config.Session{ID: "s2", Name: "test", MergedToParent: true},
			expectedSymbol: "✓",
		},
		{
			name:           "PR created",
			session:        config.Session{ID: "s3", Name: "test", PRCreated: true},
			expectedSymbol: "⬡", // hexagon for PR
		},
		{
			name:           "PR with issue number",
			session:        config.Session{ID: "s4", Name: "test", PRCreated: true, IssueNumber: 123},
			expectedSymbol: "⬡", // hexagon for PR (issue number no longer shown)
		},
		{
			name:           "no status - regular session",
			session:        config.Session{ID: "s5", Name: "test"},
			expectedSymbol: "◇", // empty diamond for regular session
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sidebar.renderSessionNode(tt.session, 0, false, false, true)

			if !strings.Contains(result, tt.expectedSymbol) {
				t.Errorf("Expected symbol %q in result %q", tt.expectedSymbol, result)
			}
		})
	}
}

func TestSidebar_View_TreeStructure(t *testing.T) {
	sidebar := NewSidebar()
	sidebar.SetSize(60, 30)

	// Create a parent with multiple children
	sessions := []config.Session{
		{ID: "parent", Name: "Parent Session", RepoPath: "/repo", Branch: "main"},
		{ID: "child-1", Name: "First Child", RepoPath: "/repo", Branch: "c1", ParentID: "parent"},
		{ID: "child-2", Name: "Second Child", RepoPath: "/repo", Branch: "c2", ParentID: "parent"},
		{ID: "child-3", Name: "Third Child", RepoPath: "/repo", Branch: "c3", ParentID: "parent"},
	}
	sidebar.SetSessions(sessions)

	view := sidebar.View()

	// Parent should have ◆ symbol (has children)
	if !strings.Contains(view, "◆") {
		t.Errorf("Parent session should show ◆ symbol, view:\n%s", view)
	}

	// Children should have tree connectors
	// First two children should have ├─
	if !strings.Contains(view, "├─") {
		t.Errorf("Middle children should have ├─ connector, view:\n%s", view)
	}

	// Last child should have ╰─
	if !strings.Contains(view, "╰─") {
		t.Errorf("Last child should have ╰─ connector, view:\n%s", view)
	}

	// Children without children of their own should have ◇
	if !strings.Contains(view, "◇") {
		t.Errorf("Leaf children should show ◇ symbol, view:\n%s", view)
	}
}
