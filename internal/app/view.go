package app

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/zhubert/plural/internal/ui"
)

// updateSizes recalculates and applies dimensions to all UI components
func (m *Model) updateSizes() {
	ctx := ui.GetViewContext()
	ctx.UpdateTerminalSize(m.width, m.height)

	m.header.SetWidth(ctx.TerminalWidth)
	m.footer.SetWidth(ctx.TerminalWidth)
	m.sidebar.SetSize(ctx.SidebarWidth, ctx.ContentHeight)
	m.chat.SetSize(ctx.ChatWidth, ctx.ContentHeight)
}

// View renders the app
func (m *Model) View() tea.View {
	var v tea.View
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.ReportFocus = true

	if m.width == 0 || m.height == 0 {
		v.SetContent("Loading...")
		return v
	}

	// Update footer context for conditional bindings
	hasSession := m.sidebar.SelectedSession() != nil
	sidebarFocused := m.focus == FocusSidebar
	var hasPendingPermission, hasPendingQuestion, isStreaming, hasDetectedOptions bool
	if m.activeSession != nil {
		if state := m.sessionState().GetIfExists(m.activeSession.ID); state != nil {
			hasPendingPermission = state.GetPendingPermission() != nil
			hasPendingQuestion = state.GetPendingQuestion() != nil
			isStreaming = state.GetStreamCancel() != nil
			hasDetectedOptions = state.HasDetectedOptions()
		}
	}
	viewChangesMode := m.chat.IsInViewChangesMode()
	searchMode := m.sidebar.IsSearchMode()
	multiSelectMode := m.sidebar.IsMultiSelectMode()
	m.footer.SetContext(hasSession, sidebarFocused, hasPendingPermission, hasPendingQuestion, isStreaming, viewChangesMode, searchMode, multiSelectMode, hasDetectedOptions, m.kittyKeyboard)

	header := m.header.View()
	footer := m.footer.View()

	// Render panels side by side
	sidebarView := m.sidebar.View()
	chatView := m.chat.View()

	panels := lipgloss.JoinHorizontal(
		lipgloss.Top,
		sidebarView,
		chatView,
	)

	view := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		panels,
		footer,
	)

	// Overlay modal if visible
	if m.modal.IsVisible() {
		modalView := m.modal.View(m.width, m.height)
		// Center modal over the view
		v.SetContent(lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			modalView,
		))
		return v
	}

	v.SetContent(view)
	return v
}

// RenderToString renders the current view as a string.
// This is useful for demos and testing.
func (m *Model) RenderToString() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// Update footer context for conditional bindings
	hasSession := m.sidebar.SelectedSession() != nil
	sidebarFocused := m.focus == FocusSidebar
	var hasPendingPermission, hasPendingQuestion, isStreaming, hasDetectedOptions bool
	if m.activeSession != nil {
		if state := m.sessionState().GetIfExists(m.activeSession.ID); state != nil {
			hasPendingPermission = state.GetPendingPermission() != nil
			hasPendingQuestion = state.GetPendingQuestion() != nil
			isStreaming = state.GetStreamCancel() != nil
			hasDetectedOptions = state.HasDetectedOptions()
		}
	}
	viewChangesMode := m.chat.IsInViewChangesMode()
	searchMode := m.sidebar.IsSearchMode()
	multiSelectMode := m.sidebar.IsMultiSelectMode()
	m.footer.SetContext(hasSession, sidebarFocused, hasPendingPermission, hasPendingQuestion, isStreaming, viewChangesMode, searchMode, multiSelectMode, hasDetectedOptions, m.kittyKeyboard)

	header := m.header.View()
	footer := m.footer.View()

	// Render panels side by side
	sidebarView := m.sidebar.View()
	chatView := m.chat.View()

	panels := lipgloss.JoinHorizontal(
		lipgloss.Top,
		sidebarView,
		chatView,
	)

	view := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		panels,
		footer,
	)

	// Overlay modal if visible
	if m.modal.IsVisible() {
		modalView := m.modal.View(m.width, m.height)
		return lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			modalView,
		)
	}

	return view
}

// adjustMouseForChat checks if a mouse event is in the chat panel area and adjusts
// coordinates relative to the chat panel. Returns the adjusted message and true if
// the event should be routed to chat, or nil and false otherwise.
func (m *Model) adjustMouseForChat(msg tea.Msg) (tea.Msg, bool) {
	sidebarWidth := m.sidebar.Width()

	switch mouseMsg := msg.(type) {
	case tea.MouseClickMsg:
		if mouseMsg.X > sidebarWidth {
			return tea.MouseClickMsg{
				X:      mouseMsg.X - sidebarWidth,
				Y:      mouseMsg.Y - ui.HeaderHeight,
				Button: mouseMsg.Button,
				Mod:    mouseMsg.Mod,
			}, true
		}
	case tea.MouseMotionMsg:
		if mouseMsg.X > sidebarWidth {
			return tea.MouseMotionMsg{
				X:      mouseMsg.X - sidebarWidth,
				Y:      mouseMsg.Y - ui.HeaderHeight,
				Button: mouseMsg.Button,
				Mod:    mouseMsg.Mod,
			}, true
		}
	case tea.MouseReleaseMsg:
		if mouseMsg.X > sidebarWidth {
			return tea.MouseReleaseMsg{
				X:      mouseMsg.X - sidebarWidth,
				Y:      mouseMsg.Y - ui.HeaderHeight,
				Button: mouseMsg.Button,
				Mod:    mouseMsg.Mod,
			}, true
		}
	}
	return nil, false
}

// routeMouseToChat adjusts mouse coordinates and routes the event to the chat panel.
// Returns the updated model and command if the event was handled, or nil cmd if not.
func (m *Model) routeMouseToChat(msg tea.Msg) (*Model, tea.Cmd, bool) {
	if adjustedMsg, ok := m.adjustMouseForChat(msg); ok {
		chat, cmd := m.chat.Update(adjustedMsg)
		m.chat = chat
		return m, cmd, true
	}
	return m, nil, false
}
