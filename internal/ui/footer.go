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
func (f *Footer) SetContext(hasSession, sidebarFocused, pendingPermission bool) {
	f.hasSession = hasSession
	f.sidebarFocused = sidebarFocused
	f.pendingPermission = pendingPermission
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
