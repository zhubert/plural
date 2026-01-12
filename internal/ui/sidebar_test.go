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

	// Verify MergedToParent indicator is shown
	if !strings.Contains(view, "merged to parent") {
		t.Error("View should contain 'merged to parent' indicator for session-4")
	}
}

func TestSidebar_GetSelectedLine(t *testing.T) {
	sidebar := NewSidebar()

	sessions := []config.Session{
		{ID: "s1", RepoPath: "/repo1", Branch: "b1"},
		{ID: "s2", RepoPath: "/repo1", Branch: "b2"},
		{ID: "s3", RepoPath: "/repo2", Branch: "b3"},
	}
	sidebar.SetSessions(sessions)

	// First session
	sidebar.selectedIdx = 0
	line := sidebar.getSelectedLine()
	if line != 0 {
		t.Errorf("Expected line 0 for first session, got %d", line)
	}

	// Second session (same repo group)
	sidebar.selectedIdx = 1
	line = sidebar.getSelectedLine()
	if line != 1 {
		t.Errorf("Expected line 1 for second session, got %d", line)
	}

	// Third session (new repo group, has header between)
	sidebar.selectedIdx = 2
	line = sidebar.getSelectedLine()
	if line != 3 { // 2 sessions + 1 header
		t.Errorf("Expected line 3 for third session, got %d", line)
	}
}
