package app

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/zhubert/plural/internal/ui"
)

// View renders the app. This is the core Bubble Tea view function.
func (m *Model) View() tea.View {
	var v tea.View
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	if m.width == 0 || m.height == 0 {
		v.SetContent("Loading...")
		return v
	}

	// Update footer context for conditional bindings
	m.updateFooterContext()

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
	m.updateFooterContext()

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

// updateFooterContext updates the footer with current context for conditional bindings
func (m *Model) updateFooterContext() {
	hasSession := m.sidebar.SelectedSession() != nil
	sidebarFocused := m.focus == FocusSidebar
	hasPendingPermission := m.activeSession != nil && m.sessionState().GetPendingPermission(m.activeSession.ID) != nil
	hasPendingQuestion := m.activeSession != nil && m.sessionState().GetPendingQuestion(m.activeSession.ID) != nil
	isStreaming := m.activeSession != nil && m.sessionState().GetStreamCancel(m.activeSession.ID) != nil
	viewChangesMode := m.chat.IsInViewChangesMode()
	searchMode := m.sidebar.IsSearchMode()
	hasDetectedOptions := m.activeSession != nil && m.sessionState().HasDetectedOptions(m.activeSession.ID)
	m.footer.SetContext(hasSession, sidebarFocused, hasPendingPermission, hasPendingQuestion, isStreaming, viewChangesMode, searchMode, hasDetectedOptions)
}

// updateSizes updates component sizes based on terminal dimensions
func (m *Model) updateSizes() {
	ctx := ui.GetViewContext()
	ctx.UpdateTerminalSize(m.width, m.height)

	m.header.SetWidth(ctx.TerminalWidth)
	m.footer.SetWidth(ctx.TerminalWidth)
	m.sidebar.SetSize(ctx.SidebarWidth, ctx.ContentHeight)
	m.chat.SetSize(ctx.ChatWidth, ctx.ContentHeight)
}
