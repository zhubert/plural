package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/ui"
)

// routeScrollAndMouseEvents routes scroll keys and mouse events to the appropriate panel.
// Returns a command if the event was handled, nil otherwise.
func (m *Model) routeScrollAndMouseEvents(msg tea.Msg) tea.Cmd {
	// Route scroll keys to chat panel even when sidebar is focused
	if m.focus == FocusSidebar && m.activeSession != nil {
		if cmd := m.routeSidebarFocusedEvents(msg); cmd != nil {
			return cmd
		}
	}

	// Handle mouse events when chat is focused
	if m.focus == FocusChat && m.activeSession != nil {
		if cmd := m.routeChatFocusedMouseEvents(msg); cmd != nil {
			return cmd
		}
	}

	return nil
}

// routeSidebarFocusedEvents routes events to chat when sidebar is focused
func (m *Model) routeSidebarFocusedEvents(msg tea.Msg) tea.Cmd {
	// Route scroll keys to chat panel
	if keyMsg, isKey := msg.(tea.KeyPressMsg); isKey {
		switch keyMsg.String() {
		case "pgup", "pgdown", "page up", "page down", "ctrl+u", "ctrl+d", "home", "end":
			chat, cmd := m.chat.Update(msg)
			m.chat = chat
			return cmd
		}
	}

	// Route mouse wheel events to chat panel for scrolling
	if mouseMsg, isMouse := msg.(tea.MouseWheelMsg); isMouse {
		if mouseMsg.X > m.sidebar.Width() {
			chat, cmd := m.chat.Update(msg)
			m.chat = chat
			return cmd
		}
	}

	// Route mouse click/motion/release events to chat panel for text selection
	return m.routeMouseEventsToChat(msg)
}

// routeChatFocusedMouseEvents routes mouse events when chat is focused
func (m *Model) routeChatFocusedMouseEvents(msg tea.Msg) tea.Cmd {
	return m.routeMouseEventsToChat(msg)
}

// routeMouseEventsToChat routes mouse events to the chat panel with coordinate adjustment.
// This handles click, motion, and release events, adjusting coordinates for sidebar and header.
func (m *Model) routeMouseEventsToChat(msg tea.Msg) tea.Cmd {
	sidebarWidth := m.sidebar.Width()

	switch mouseMsg := msg.(type) {
	case tea.MouseClickMsg:
		if mouseMsg.X > sidebarWidth {
			adjustedMsg := m.adjustMouseClickMsg(mouseMsg, sidebarWidth)
			chat, cmd := m.chat.Update(adjustedMsg)
			m.chat = chat
			return cmd
		}

	case tea.MouseMotionMsg:
		if mouseMsg.X > sidebarWidth {
			adjustedMsg := m.adjustMouseMotionMsg(mouseMsg, sidebarWidth)
			chat, cmd := m.chat.Update(adjustedMsg)
			m.chat = chat
			return cmd
		}

	case tea.MouseReleaseMsg:
		if mouseMsg.X > sidebarWidth {
			adjustedMsg := m.adjustMouseReleaseMsg(mouseMsg, sidebarWidth)
			chat, cmd := m.chat.Update(adjustedMsg)
			m.chat = chat
			return cmd
		}
	}

	return nil
}

// adjustMouseClickMsg adjusts mouse click coordinates for the chat panel.
// X is adjusted by subtracting sidebar width, Y by subtracting header height.
func (m *Model) adjustMouseClickMsg(msg tea.MouseClickMsg, sidebarWidth int) tea.MouseClickMsg {
	return tea.MouseClickMsg{
		X:      msg.X - sidebarWidth,
		Y:      msg.Y - ui.HeaderHeight,
		Button: msg.Button,
		Mod:    msg.Mod,
	}
}

// adjustMouseMotionMsg adjusts mouse motion coordinates for the chat panel.
// X is adjusted by subtracting sidebar width, Y by subtracting header height.
func (m *Model) adjustMouseMotionMsg(msg tea.MouseMotionMsg, sidebarWidth int) tea.MouseMotionMsg {
	return tea.MouseMotionMsg{
		X:      msg.X - sidebarWidth,
		Y:      msg.Y - ui.HeaderHeight,
		Button: msg.Button,
		Mod:    msg.Mod,
	}
}

// adjustMouseReleaseMsg adjusts mouse release coordinates for the chat panel.
// X is adjusted by subtracting sidebar width, Y by subtracting header height.
func (m *Model) adjustMouseReleaseMsg(msg tea.MouseReleaseMsg, sidebarWidth int) tea.MouseReleaseMsg {
	return tea.MouseReleaseMsg{
		X:      msg.X - sidebarWidth,
		Y:      msg.Y - ui.HeaderHeight,
		Button: msg.Button,
		Mod:    msg.Mod,
	}
}
