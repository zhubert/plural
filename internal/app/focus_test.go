package app

import (
	"testing"
)

// =============================================================================
// toggleFocus Tests
// =============================================================================

func TestToggleFocus_SidebarToChat_WithActiveSession(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)

	// Select the first session to make it active
	m.activeSession = &cfg.Sessions[0]
	m.focus = FocusSidebar

	// Toggle focus from sidebar to chat
	cmd := m.toggleFocus()

	// Verify focus switched to chat
	if m.focus != FocusChat {
		t.Errorf("expected focus to be FocusChat, got %v", m.focus)
	}

	// Verify sidebar is unfocused
	if m.sidebar.IsFocused() {
		t.Error("expected sidebar to be unfocused")
	}

	// Verify chat is focused
	if !m.chat.IsFocused() {
		t.Error("expected chat to be focused")
	}

	// Verify no command is returned (regression test for issue #147)
	if cmd != nil {
		t.Errorf("expected nil command, got %v", cmd)
	}
}

func TestToggleFocus_SidebarToChat_WithoutActiveSession(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Ensure no active session
	m.activeSession = nil
	m.focus = FocusSidebar

	// Toggle focus from sidebar to chat (should fail)
	cmd := m.toggleFocus()

	// Verify focus stayed on sidebar
	if m.focus != FocusSidebar {
		t.Errorf("expected focus to stay on FocusSidebar, got %v", m.focus)
	}

	// Verify sidebar is still focused
	if !m.sidebar.IsFocused() {
		t.Error("expected sidebar to remain focused")
	}

	// Verify chat is not focused
	if m.chat.IsFocused() {
		t.Error("expected chat to remain unfocused")
	}

	// Verify no command is returned
	if cmd != nil {
		t.Errorf("expected nil command, got %v", cmd)
	}
}

func TestToggleFocus_ChatToSidebar(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)

	// Select the first session to make it active
	m.activeSession = &cfg.Sessions[0]
	m.focus = FocusChat

	// Set initial focus state
	m.sidebar.SetFocused(false)
	m.chat.SetFocused(true)

	// Toggle focus from chat to sidebar
	cmd := m.toggleFocus()

	// Verify focus switched to sidebar
	if m.focus != FocusSidebar {
		t.Errorf("expected focus to be FocusSidebar, got %v", m.focus)
	}

	// Verify sidebar is focused
	if !m.sidebar.IsFocused() {
		t.Error("expected sidebar to be focused")
	}

	// Verify chat is unfocused
	if m.chat.IsFocused() {
		t.Error("expected chat to be unfocused")
	}

	// Verify no command is returned (regression test for issue #147)
	if cmd != nil {
		t.Errorf("expected nil command, got %v", cmd)
	}
}
