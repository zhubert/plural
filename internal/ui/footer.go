package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// KeyBinding represents a keyboard shortcut
type KeyBinding struct {
	Key  string
	Desc string
}

// Footer represents the bottom footer bar with keybindings
type Footer struct {
	width             int
	bindings          []KeyBinding
	hasSession        bool // Whether a session is selected
	sidebarFocused    bool // Whether sidebar has focus
	pendingPermission bool // Whether chat has a pending permission prompt
	pendingQuestion   bool // Whether chat has a pending question prompt
	streaming         bool // Whether active session is streaming
	sessionInUse      bool // Whether selected session has "session in use" error
	viewChangesMode   bool // Whether showing view changes overlay
}

// NewFooter creates a new footer
func NewFooter() *Footer {
	return &Footer{
		bindings: []KeyBinding{
			{Key: "tab", Desc: "switch pane"},
			{Key: "n", Desc: "new session"},
			{Key: "r", Desc: "add repo"},
			{Key: "s", Desc: "mcp servers"},
			{Key: "v", Desc: "view changes"},
			{Key: "m", Desc: "merge/pr"},
			{Key: "d", Desc: "delete"},
			{Key: "pgup/dn", Desc: "scroll"},
			{Key: "q", Desc: "quit"},
		},
	}
}

// SetContext updates the footer's context for conditional bindings
func (f *Footer) SetContext(hasSession, sidebarFocused, pendingPermission, pendingQuestion, streaming, sessionInUse, viewChangesMode bool) {
	f.hasSession = hasSession
	f.sidebarFocused = sidebarFocused
	f.pendingPermission = pendingPermission
	f.pendingQuestion = pendingQuestion
	f.streaming = streaming
	f.sessionInUse = sessionInUse
	f.viewChangesMode = viewChangesMode
}

// SetWidth sets the footer width
func (f *Footer) SetWidth(width int) {
	f.width = width
}

// SetBindings allows custom keybindings
func (f *Footer) SetBindings(bindings []KeyBinding) {
	f.bindings = bindings
}

// View renders the footer
func (f *Footer) View() string {
	var parts []string

	// Show view-changes-specific shortcuts when in view changes mode
	if f.viewChangesMode {
		viewChangesBindings := []KeyBinding{
			{Key: "esc/q/v", Desc: "close"},
			{Key: "↑/↓/j/k", Desc: "scroll"},
			{Key: "pgup/dn", Desc: "page"},
		}
		for _, b := range viewChangesBindings {
			key := FooterKeyStyle.Render(b.Key)
			desc := FooterDescStyle.Render(": " + b.Desc)
			parts = append(parts, key+desc)
		}
		content := strings.Join(parts, "  "+lipgloss.NewStyle().Foreground(ColorBorder).Render("|")+"  ")
		return FooterStyle.Width(f.width).Render(content)
	}

	// Show permission-specific shortcuts when pending permission in chat
	if f.pendingPermission && !f.sidebarFocused {
		permBindings := []KeyBinding{
			{Key: "y", Desc: "allow"},
			{Key: "n", Desc: "deny"},
			{Key: "a", Desc: "always allow"},
			{Key: "tab", Desc: "switch pane"},
		}
		for _, b := range permBindings {
			key := FooterKeyStyle.Render(b.Key)
			desc := FooterDescStyle.Render(": " + b.Desc)
			parts = append(parts, key+desc)
		}
	} else if f.pendingQuestion && !f.sidebarFocused {
		// Show question-specific shortcuts when pending question in chat
		questBindings := []KeyBinding{
			{Key: "1-5", Desc: "select"},
			{Key: "↑/↓", Desc: "navigate"},
			{Key: "enter", Desc: "confirm"},
			{Key: "tab", Desc: "switch pane"},
		}
		for _, b := range questBindings {
			key := FooterKeyStyle.Render(b.Key)
			desc := FooterDescStyle.Render(": " + b.Desc)
			parts = append(parts, key+desc)
		}
	} else if f.streaming && !f.sidebarFocused {
		// Show streaming-specific shortcuts when streaming in chat
		streamBindings := []KeyBinding{
			{Key: "esc", Desc: "stop"},
			{Key: "tab", Desc: "switch pane"},
			{Key: "pgup/dn", Desc: "scroll"},
		}
		for _, b := range streamBindings {
			key := FooterKeyStyle.Render(b.Key)
			desc := FooterDescStyle.Render(": " + b.Desc)
			parts = append(parts, key+desc)
		}
	} else if f.sessionInUse && f.sidebarFocused {
		// Show force-resume option when session has "in use" error
		inUseBindings := []KeyBinding{
			{Key: "f", Desc: "force resume"},
			{Key: "tab", Desc: "switch pane"},
			{Key: "n", Desc: "new session"},
			{Key: "d", Desc: "delete"},
			{Key: "q", Desc: "quit"},
		}
		for _, b := range inUseBindings {
			key := FooterKeyStyle.Render(b.Key)
			desc := FooterDescStyle.Render(": " + b.Desc)
			parts = append(parts, key+desc)
		}
	} else if !f.sidebarFocused && f.hasSession {
		// Chat focused, not streaming - show enter and ctrl+v
		chatBindings := []KeyBinding{
			{Key: "enter", Desc: "send"},
			{Key: "ctrl+v", Desc: "paste image"},
			{Key: "tab", Desc: "switch pane"},
			{Key: "pgup/dn", Desc: "scroll"},
		}
		for _, b := range chatBindings {
			key := FooterKeyStyle.Render(b.Key)
			desc := FooterDescStyle.Render(": " + b.Desc)
			parts = append(parts, key+desc)
		}
	} else {
		for _, b := range f.bindings {
			// Skip tab when no session (can't switch to chat without one)
			if b.Key == "tab" && !f.hasSession {
				continue
			}
			// Skip sidebar-only bindings when chat is focused
			if (b.Key == "n" || b.Key == "r" || b.Key == "s" || b.Key == "v" || b.Key == "m" || b.Key == "d" || b.Key == "q") && !f.sidebarFocused {
				continue
			}
			// Skip session-specific bindings when no session selected
			if (b.Key == "v" || b.Key == "m" || b.Key == "d" || b.Key == "pgup/dn") && !f.hasSession {
				continue
			}

			key := FooterKeyStyle.Render(b.Key)
			desc := FooterDescStyle.Render(": " + b.Desc)
			parts = append(parts, key+desc)
		}
	}

	content := strings.Join(parts, "  "+lipgloss.NewStyle().Foreground(ColorBorder).Render("|")+"  ")

	return FooterStyle.Width(f.width).Render(content)
}
