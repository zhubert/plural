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

func TestSidebar_AttentionStateMaps(t *testing.T) {
	sidebar := NewSidebar()

	// Initially all maps are empty
	if sidebar.sessionPriority("s1") != priorityNormal {
		t.Error("Session should have normal priority initially")
	}

	// Set and verify pending question
	sidebar.SetPendingQuestion("s1", true)
	if sidebar.sessionPriority("s1") != priorityPermission {
		t.Errorf("Expected priorityPermission, got %d", sidebar.sessionPriority("s1"))
	}
	sidebar.SetPendingQuestion("s1", false)
	if sidebar.sessionPriority("s1") != priorityNormal {
		t.Error("Should return to normal after clearing question")
	}

	// Set and verify idle with response
	sidebar.SetIdleWithResponse("s1", true)
	if sidebar.sessionPriority("s1") != priorityIdle {
		t.Errorf("Expected priorityIdle, got %d", sidebar.sessionPriority("s1"))
	}
	sidebar.SetIdleWithResponse("s1", false)
	if sidebar.sessionPriority("s1") != priorityNormal {
		t.Error("Should return to normal after clearing idle")
	}

	// Set and verify uncommitted changes
	sidebar.SetUncommittedChanges("s1", true)
	if sidebar.sessionPriority("s1") != priorityUncommitted {
		t.Errorf("Expected priorityUncommitted, got %d", sidebar.sessionPriority("s1"))
	}
	sidebar.SetUncommittedChanges("s1", false)
	if sidebar.sessionPriority("s1") != priorityNormal {
		t.Error("Should return to normal after clearing uncommitted")
	}
}

func TestSidebar_SessionPriority_Ordering(t *testing.T) {
	sidebar := NewSidebar()

	// Test priority precedence: permission > streaming > idle > uncommitted > normal
	// When multiple states are set, highest priority wins

	// Permission + streaming -> permission wins
	sidebar.SetPendingPermission("s1", true)
	sidebar.SetStreaming("s1", true)
	if sidebar.sessionPriority("s1") != priorityPermission {
		t.Errorf("Permission should take priority over streaming, got %d", sidebar.sessionPriority("s1"))
	}

	// Question also maps to permission priority
	sidebar.SetPendingPermission("s1", false)
	sidebar.SetStreaming("s1", false)
	sidebar.SetPendingQuestion("s2", true)
	if sidebar.sessionPriority("s2") != priorityPermission {
		t.Errorf("Question should be priorityPermission, got %d", sidebar.sessionPriority("s2"))
	}

	// Streaming > idle
	sidebar.SetStreaming("s3", true)
	sidebar.SetIdleWithResponse("s3", true)
	if sidebar.sessionPriority("s3") != priorityStreaming {
		t.Errorf("Streaming should take priority over idle, got %d", sidebar.sessionPriority("s3"))
	}

	// Idle > uncommitted
	sidebar.SetIdleWithResponse("s4", true)
	sidebar.SetUncommittedChanges("s4", true)
	if sidebar.sessionPriority("s4") != priorityIdle {
		t.Errorf("Idle should take priority over uncommitted, got %d", sidebar.sessionPriority("s4"))
	}
}

func TestSidebar_EffectivePriority(t *testing.T) {
	sidebar := NewSidebar()

	// Parent with no attention, child with permission
	sidebar.SetPendingPermission("child1", true)

	node := sessionNode{
		Session: config.Session{ID: "parent"},
		Children: []sessionNode{
			{Session: config.Session{ID: "child1"}},
		},
	}

	ep := sidebar.effectivePriority(node)
	if ep != priorityPermission {
		t.Errorf("Effective priority should be permission (from child), got %d", ep)
	}

	// Parent with streaming, child with normal -> parent priority wins
	sidebar.SetPendingPermission("child1", false)
	sidebar.SetStreaming("parent", true)

	ep = sidebar.effectivePriority(node)
	if ep != priorityStreaming {
		t.Errorf("Effective priority should be streaming (from parent), got %d", ep)
	}
}

func TestSidebar_PrioritySorting_RootNodes(t *testing.T) {
	sidebar := NewSidebar()

	// Set up attention states before setting sessions
	sidebar.SetPendingPermission("s-perm", true)
	sidebar.SetStreaming("s-stream", true)
	sidebar.SetIdleWithResponse("s-idle", true)
	sidebar.SetUncommittedChanges("s-uncommit", true)

	sessions := []config.Session{
		{ID: "s-normal", RepoPath: "/repo", Branch: "b1", Name: "normal"},
		{ID: "s-uncommit", RepoPath: "/repo", Branch: "b2", Name: "uncommitted"},
		{ID: "s-idle", RepoPath: "/repo", Branch: "b3", Name: "idle"},
		{ID: "s-stream", RepoPath: "/repo", Branch: "b4", Name: "streaming"},
		{ID: "s-perm", RepoPath: "/repo", Branch: "b5", Name: "permission"},
	}
	sidebar.SetSessions(sessions)

	// After SetSessions, sessions should be sorted by priority
	// Expected order: permission, streaming, idle, uncommitted, normal
	expected := []string{"s-perm", "s-stream", "s-idle", "s-uncommit", "s-normal"}
	for i, id := range expected {
		if sidebar.sessions[i].ID != id {
			t.Errorf("sessions[%d]: expected %s, got %s", i, id, sidebar.sessions[i].ID)
		}
	}
}

func TestSidebar_PrioritySorting_StableOrder(t *testing.T) {
	sidebar := NewSidebar()

	// All sessions have normal priority - original order should be preserved
	sessions := []config.Session{
		{ID: "s1", RepoPath: "/repo", Branch: "b1", Name: "first"},
		{ID: "s2", RepoPath: "/repo", Branch: "b2", Name: "second"},
		{ID: "s3", RepoPath: "/repo", Branch: "b3", Name: "third"},
	}
	sidebar.SetSessions(sessions)

	for i, sess := range sessions {
		if sidebar.sessions[i].ID != sess.ID {
			t.Errorf("sessions[%d]: expected %s, got %s (stable sort failed)", i, sess.ID, sidebar.sessions[i].ID)
		}
	}
}

func TestSidebar_PrioritySorting_ParentChildPreserved(t *testing.T) {
	sidebar := NewSidebar()

	// Child has a pending permission - should drag parent subtree to top
	sidebar.SetPendingPermission("child", true)

	sessions := []config.Session{
		{ID: "other", RepoPath: "/repo", Branch: "b1", Name: "other"},
		{ID: "parent", RepoPath: "/repo", Branch: "b2", Name: "parent"},
		{ID: "child", RepoPath: "/repo", Branch: "b3", Name: "child", ParentID: "parent"},
	}
	sidebar.SetSessions(sessions)

	// Parent subtree should sort before "other" because child has permission
	// Expected flat order: parent, child, other
	if sidebar.sessions[0].ID != "parent" {
		t.Errorf("sessions[0]: expected parent, got %s", sidebar.sessions[0].ID)
	}
	if sidebar.sessions[1].ID != "child" {
		t.Errorf("sessions[1]: expected child, got %s", sidebar.sessions[1].ID)
	}
	if sidebar.sessions[2].ID != "other" {
		t.Errorf("sessions[2]: expected other, got %s", sidebar.sessions[2].ID)
	}
}

func TestSidebar_PrioritySorting_MultipleRepoGroups(t *testing.T) {
	sidebar := NewSidebar()

	// Permission in repo2, streaming in repo1
	sidebar.SetPendingPermission("s2", true)
	sidebar.SetStreaming("s1", true)

	sessions := []config.Session{
		{ID: "s1", RepoPath: "/repo1", Branch: "b1", Name: "repo1-streaming"},
		{ID: "s3", RepoPath: "/repo1", Branch: "b2", Name: "repo1-normal"},
		{ID: "s2", RepoPath: "/repo2", Branch: "b3", Name: "repo2-permission"},
		{ID: "s4", RepoPath: "/repo2", Branch: "b4", Name: "repo2-normal"},
	}
	sidebar.SetSessions(sessions)

	// Each repo group sorts independently
	// Repo1: s1 (streaming) before s3 (normal)
	// Repo2: s2 (permission) before s4 (normal)
	if sidebar.sessions[0].ID != "s1" {
		t.Errorf("sessions[0]: expected s1 (streaming), got %s", sidebar.sessions[0].ID)
	}
	if sidebar.sessions[1].ID != "s3" {
		t.Errorf("sessions[1]: expected s3 (normal), got %s", sidebar.sessions[1].ID)
	}
	if sidebar.sessions[2].ID != "s2" {
		t.Errorf("sessions[2]: expected s2 (permission), got %s", sidebar.sessions[2].ID)
	}
	if sidebar.sessions[3].ID != "s4" {
		t.Errorf("sessions[3]: expected s4 (normal), got %s", sidebar.sessions[3].ID)
	}
}

func TestSidebar_PrioritySorting_ChildrenWithinParent(t *testing.T) {
	sidebar := NewSidebar()

	// Two children: one streaming, one with permission
	sidebar.SetPendingPermission("child-perm", true)
	sidebar.SetStreaming("child-stream", true)

	sessions := []config.Session{
		{ID: "parent", RepoPath: "/repo", Branch: "b1", Name: "parent"},
		{ID: "child-stream", RepoPath: "/repo", Branch: "b2", Name: "streaming child", ParentID: "parent"},
		{ID: "child-perm", RepoPath: "/repo", Branch: "b3", Name: "permission child", ParentID: "parent"},
		{ID: "child-normal", RepoPath: "/repo", Branch: "b4", Name: "normal child", ParentID: "parent"},
	}
	sidebar.SetSessions(sessions)

	// Flat order: parent, then children sorted by priority
	// Children order: child-perm (permission), child-stream (streaming), child-normal (normal)
	if sidebar.sessions[0].ID != "parent" {
		t.Errorf("sessions[0]: expected parent, got %s", sidebar.sessions[0].ID)
	}
	if sidebar.sessions[1].ID != "child-perm" {
		t.Errorf("sessions[1]: expected child-perm, got %s", sidebar.sessions[1].ID)
	}
	if sidebar.sessions[2].ID != "child-stream" {
		t.Errorf("sessions[2]: expected child-stream, got %s", sidebar.sessions[2].ID)
	}
	if sidebar.sessions[3].ID != "child-normal" {
		t.Errorf("sessions[3]: expected child-normal, got %s", sidebar.sessions[3].ID)
	}
}

func TestSidebar_HashAttention_ChangeDetection(t *testing.T) {
	sidebar := NewSidebar()

	h1 := sidebar.hashAttention()

	// Change attention state
	sidebar.SetPendingPermission("s1", true)
	h2 := sidebar.hashAttention()

	if h1 == h2 {
		t.Error("Hash should change when attention state changes")
	}

	// Same state should give same hash
	h3 := sidebar.hashAttention()
	if h2 != h3 {
		t.Error("Hash should be stable for the same state")
	}

	// Different attention type should give different hash
	sidebar.SetPendingPermission("s1", false)
	sidebar.SetStreaming("s1", true)
	h4 := sidebar.hashAttention()
	if h2 == h4 {
		t.Error("Different attention types should produce different hashes")
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

// =============================================================================
// Multi-select mode tests
// =============================================================================

func TestSidebar_MultiSelect_EnterExit(t *testing.T) {
	sidebar := NewSidebar()
	sessions := []config.Session{
		{ID: "s1", RepoPath: "/repo", Branch: "b1", Name: "session1"},
		{ID: "s2", RepoPath: "/repo", Branch: "b2", Name: "session2"},
	}
	sidebar.SetSessions(sessions)

	if sidebar.IsMultiSelectMode() {
		t.Error("should not be in multi-select mode initially")
	}

	sidebar.EnterMultiSelect()
	if !sidebar.IsMultiSelectMode() {
		t.Error("should be in multi-select mode after enter")
	}

	// Current item should be pre-selected
	if sidebar.SelectedCount() != 1 {
		t.Errorf("expected 1 pre-selected session, got %d", sidebar.SelectedCount())
	}

	sidebar.ExitMultiSelect()
	if sidebar.IsMultiSelectMode() {
		t.Error("should not be in multi-select mode after exit")
	}
	if sidebar.SelectedCount() != 0 {
		t.Error("selections should be cleared after exit")
	}
}

func TestSidebar_MultiSelect_Toggle(t *testing.T) {
	sidebar := NewSidebar()
	sessions := []config.Session{
		{ID: "s1", RepoPath: "/repo", Branch: "b1", Name: "session1"},
		{ID: "s2", RepoPath: "/repo", Branch: "b2", Name: "session2"},
	}
	sidebar.SetSessions(sessions)
	sidebar.EnterMultiSelect()

	// s1 is pre-selected (index 0)
	if sidebar.SelectedCount() != 1 {
		t.Fatalf("expected 1 selected, got %d", sidebar.SelectedCount())
	}

	// Toggle s1 off
	sidebar.ToggleSelected()
	if sidebar.SelectedCount() != 0 {
		t.Errorf("expected 0 selected after toggle, got %d", sidebar.SelectedCount())
	}

	// Toggle s1 back on
	sidebar.ToggleSelected()
	if sidebar.SelectedCount() != 1 {
		t.Errorf("expected 1 selected after re-toggle, got %d", sidebar.SelectedCount())
	}
}

func TestSidebar_MultiSelect_SelectAllDeselectAll(t *testing.T) {
	sidebar := NewSidebar()
	sessions := []config.Session{
		{ID: "s1", RepoPath: "/repo", Branch: "b1", Name: "session1"},
		{ID: "s2", RepoPath: "/repo", Branch: "b2", Name: "session2"},
		{ID: "s3", RepoPath: "/repo", Branch: "b3", Name: "session3"},
	}
	sidebar.SetSessions(sessions)
	sidebar.EnterMultiSelect()

	sidebar.SelectAll()
	if sidebar.SelectedCount() != 3 {
		t.Errorf("expected 3 selected after SelectAll, got %d", sidebar.SelectedCount())
	}

	sidebar.DeselectAll()
	if sidebar.SelectedCount() != 0 {
		t.Errorf("expected 0 selected after DeselectAll, got %d", sidebar.SelectedCount())
	}
}

func TestSidebar_MultiSelect_GetSelectedSessionIDs(t *testing.T) {
	sidebar := NewSidebar()
	sessions := []config.Session{
		{ID: "s1", RepoPath: "/repo", Branch: "b1", Name: "session1"},
		{ID: "s2", RepoPath: "/repo", Branch: "b2", Name: "session2"},
	}
	sidebar.SetSessions(sessions)
	sidebar.EnterMultiSelect()
	sidebar.SelectAll()

	ids := sidebar.GetSelectedSessionIDs()
	if len(ids) != 2 {
		t.Errorf("expected 2 IDs, got %d", len(ids))
	}

	// Check both IDs are present (order is not guaranteed for maps)
	idMap := make(map[string]bool)
	for _, id := range ids {
		idMap[id] = true
	}
	if !idMap["s1"] || !idMap["s2"] {
		t.Errorf("expected s1 and s2, got %v", ids)
	}
}

func TestSidebar_MultiSelect_RenderCheckboxes(t *testing.T) {
	sidebar := NewSidebar()
	sidebar.SetSize(60, 24)

	sessions := []config.Session{
		{ID: "s1", RepoPath: "/repo", Branch: "b1", Name: "session1"},
		{ID: "s2", RepoPath: "/repo", Branch: "b2", Name: "session2"},
	}
	sidebar.SetSessions(sessions)
	sidebar.EnterMultiSelect()

	// s1 is pre-selected, s2 is not
	view := sidebar.View()

	if !strings.Contains(view, "[x]") {
		t.Errorf("should show [x] for selected session, view:\n%s", view)
	}
	if !strings.Contains(view, "[ ]") {
		t.Errorf("should show [ ] for unselected session, view:\n%s", view)
	}
}

func TestSidebar_MultiSelect_ExitClearsMode(t *testing.T) {
	sidebar := NewSidebar()
	sessions := []config.Session{
		{ID: "s1", RepoPath: "/repo", Branch: "b1", Name: "session1"},
	}
	sidebar.SetSessions(sessions)
	sidebar.EnterMultiSelect()
	sidebar.SelectAll()

	// Exit clears everything
	sidebar.ExitMultiSelect()

	if sidebar.IsMultiSelectMode() {
		t.Error("should not be in multi-select mode")
	}
	if sidebar.SelectedCount() != 0 {
		t.Error("selections should be cleared")
	}

	// View should not have checkboxes
	sidebar.SetSize(60, 24)
	view := sidebar.View()
	if strings.Contains(view, "[x]") || strings.Contains(view, "[ ]") {
		t.Error("checkboxes should not be visible after exiting multi-select mode")
	}
}

// =============================================================================
// Search mode tests
// =============================================================================

func TestSidebar_SelectSession_SearchMode(t *testing.T) {
	sidebar := NewSidebar()

	sessions := []config.Session{
		{ID: "session-1", RepoPath: "/repo", Branch: "b1", Name: "apple"},
		{ID: "session-2", RepoPath: "/repo", Branch: "b2", Name: "banana"},
		{ID: "session-3", RepoPath: "/repo", Branch: "b3", Name: "cherry"},
		{ID: "session-4", RepoPath: "/repo", Branch: "b4", Name: "apricot"},
	}
	sidebar.SetSessions(sessions)

	// Enable search mode and filter for "ap" (should match "apple" and "apricot")
	sidebar.searchMode = true
	sidebar.searchInput.SetValue("ap")
	sidebar.filteredSessions = []config.Session{
		sessions[0], // apple
		sessions[3], // apricot
	}

	// Select "apricot" (session-4)
	sidebar.SelectSession("session-4")

	// In filtered view, apricot should be at index 1
	// Without the fix, this would incorrectly use index 3 from the full list
	if sidebar.selectedIdx != 1 {
		t.Errorf("Expected selectedIdx 1 in filtered view, got %d", sidebar.selectedIdx)
	}

	// Verify SelectedSession returns the correct session
	selected := sidebar.SelectedSession()
	if selected == nil {
		t.Fatal("SelectedSession should not be nil")
	}
	if selected.ID != "session-4" {
		t.Errorf("Expected selected session-4, got %s", selected.ID)
	}
}

func TestSidebar_SelectSession_SearchMode_NotInFilteredList(t *testing.T) {
	sidebar := NewSidebar()

	sessions := []config.Session{
		{ID: "session-1", RepoPath: "/repo", Branch: "b1", Name: "apple"},
		{ID: "session-2", RepoPath: "/repo", Branch: "b2", Name: "banana"},
		{ID: "session-3", RepoPath: "/repo", Branch: "b3", Name: "cherry"},
	}
	sidebar.SetSessions(sessions)

	// Set initial selection to session-1
	sidebar.selectedIdx = 0

	// Enable search mode and filter for "ban" (should match only "banana")
	sidebar.searchMode = true
	sidebar.searchInput.SetValue("ban")
	sidebar.filteredSessions = []config.Session{
		sessions[1], // banana
	}

	// Try to select "cherry" which is not in the filtered list
	sidebar.SelectSession("session-3")

	// Selection should not change (should remain 0)
	if sidebar.selectedIdx != 0 {
		t.Errorf("Selection should not change when session not in filtered list, got %d", sidebar.selectedIdx)
	}
}

func TestSidebar_HashSessions_PRMergedPRClosed(t *testing.T) {
	sessBase := []config.Session{
		{ID: "s1", RepoPath: "/repo", Branch: "b1", PRCreated: true},
	}
	sessPRMerged := []config.Session{
		{ID: "s1", RepoPath: "/repo", Branch: "b1", PRCreated: true, PRMerged: true},
	}
	sessPRClosed := []config.Session{
		{ID: "s1", RepoPath: "/repo", Branch: "b1", PRCreated: true, PRClosed: true},
	}

	hashBase := hashSessions(sessBase)
	hashMerged := hashSessions(sessPRMerged)
	hashClosed := hashSessions(sessPRClosed)

	if hashBase == hashMerged {
		t.Error("Hash should differ when PRMerged changes")
	}
	if hashBase == hashClosed {
		t.Error("Hash should differ when PRClosed changes")
	}
	if hashMerged == hashClosed {
		t.Error("Hash should differ between PRMerged and PRClosed")
	}
}

func TestSidebar_RenderSessionNode_PRMergedSymbol(t *testing.T) {
	sidebar := NewSidebar()
	session := config.Session{ID: "s1", Name: "test", PRCreated: true, PRMerged: true}

	result := sidebar.renderSessionNode(session, 0, false, false, true)

	if !strings.Contains(result, "✓") {
		t.Errorf("PR merged session should show ✓ symbol, got %q", result)
	}
}

func TestSidebar_RenderSessionNode_PRClosedSymbol(t *testing.T) {
	sidebar := NewSidebar()
	session := config.Session{ID: "s1", Name: "test", PRCreated: true, PRClosed: true}

	result := sidebar.renderSessionNode(session, 0, false, false, true)

	if !strings.Contains(result, "✕") {
		t.Errorf("PR closed session should show ✕ symbol, got %q", result)
	}
}

func TestSidebar_HasNewComments(t *testing.T) {
	sidebar := NewSidebar()

	// Initially no new comments
	if sidebar.HasNewComments("s1") {
		t.Error("Should not have new comments initially")
	}

	// Set new comments
	sidebar.SetHasNewComments("s1", true)
	if !sidebar.HasNewComments("s1") {
		t.Error("Should have new comments after set")
	}
	if sidebar.HasNewComments("s2") {
		t.Error("s2 should not have new comments")
	}

	// Clear new comments
	sidebar.SetHasNewComments("s1", false)
	if sidebar.HasNewComments("s1") {
		t.Error("Should not have new comments after clear")
	}
}

func TestSidebar_HasNewComments_Priority(t *testing.T) {
	sidebar := NewSidebar()

	// New comments priority should be between uncommitted and normal
	sidebar.SetHasNewComments("s1", true)
	if sidebar.sessionPriority("s1") != priorityNewComments {
		t.Errorf("Expected priorityNewComments (%d), got %d", priorityNewComments, sidebar.sessionPriority("s1"))
	}

	// Uncommitted should take priority over new comments
	sidebar.SetUncommittedChanges("s1", true)
	if sidebar.sessionPriority("s1") != priorityUncommitted {
		t.Errorf("Uncommitted should take priority over new comments, got %d", sidebar.sessionPriority("s1"))
	}

	// Clear uncommitted, new comments should still be there
	sidebar.SetUncommittedChanges("s1", false)
	if sidebar.sessionPriority("s1") != priorityNewComments {
		t.Errorf("Expected priorityNewComments after clearing uncommitted, got %d", sidebar.sessionPriority("s1"))
	}

	// Clear new comments, should return to normal
	sidebar.SetHasNewComments("s1", false)
	if sidebar.sessionPriority("s1") != priorityNormal {
		t.Errorf("Should return to normal after clearing new comments, got %d", sidebar.sessionPriority("s1"))
	}
}

func TestSidebar_HasNewComments_Sorting(t *testing.T) {
	sidebar := NewSidebar()

	sidebar.SetHasNewComments("s-comments", true)

	sessions := []config.Session{
		{ID: "s-normal", RepoPath: "/repo", Branch: "b1", Name: "normal"},
		{ID: "s-comments", RepoPath: "/repo", Branch: "b2", Name: "comments"},
	}
	sidebar.SetSessions(sessions)

	// Session with new comments should sort before normal
	if sidebar.sessions[0].ID != "s-comments" {
		t.Errorf("sessions[0]: expected s-comments, got %s", sidebar.sessions[0].ID)
	}
	if sidebar.sessions[1].ID != "s-normal" {
		t.Errorf("sessions[1]: expected s-normal, got %s", sidebar.sessions[1].ID)
	}
}

func TestSidebar_HasNewComments_HashChange(t *testing.T) {
	sidebar := NewSidebar()

	h1 := sidebar.hashAttention()

	sidebar.SetHasNewComments("s1", true)
	h2 := sidebar.hashAttention()

	if h1 == h2 {
		t.Error("Hash should change when new comments state changes")
	}

	sidebar.SetHasNewComments("s1", false)
	h3 := sidebar.hashAttention()

	if h2 == h3 {
		t.Error("Hash should change when new comments state is cleared")
	}

	if h1 != h3 {
		t.Error("Hash should be same after setting and clearing new comments")
	}
}

func TestSidebar_RenderSessionNode_NewCommentsIndicator(t *testing.T) {
	sidebar := NewSidebar()
	session := config.Session{ID: "s1", Name: "test", PRCreated: true}

	// Without new comments - should not have *
	result := sidebar.renderSessionNode(session, 0, false, false, true)
	if strings.Contains(result, " *") {
		t.Errorf("Should not show * without new comments, got %q", result)
	}

	// With new comments - should have *
	sidebar.SetHasNewComments("s1", true)
	result = sidebar.renderSessionNode(session, 0, false, false, true)
	if !strings.Contains(result, "*") {
		t.Errorf("Should show * for new comments, got %q", result)
	}
}

func TestSidebar_SelectSession_NormalMode(t *testing.T) {
	sidebar := NewSidebar()

	sessions := []config.Session{
		{ID: "session-1", RepoPath: "/repo", Branch: "b1", Name: "apple"},
		{ID: "session-2", RepoPath: "/repo", Branch: "b2", Name: "banana"},
		{ID: "session-3", RepoPath: "/repo", Branch: "b3", Name: "cherry"},
	}
	sidebar.SetSessions(sessions)

	// Search mode is off (normal mode)
	sidebar.searchMode = false

	// Select session-2
	sidebar.SelectSession("session-2")

	// Should use index from full list
	if sidebar.selectedIdx != 1 {
		t.Errorf("Expected selectedIdx 1, got %d", sidebar.selectedIdx)
	}

	// Verify SelectedSession returns the correct session
	selected := sidebar.SelectedSession()
	if selected == nil {
		t.Fatal("SelectedSession should not be nil")
	}
	if selected.ID != "session-2" {
		t.Errorf("Expected selected session-2, got %s", selected.ID)
	}
}
