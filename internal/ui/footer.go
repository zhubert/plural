package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// FlashType represents the type of flash message
type FlashType int

const (
	FlashError FlashType = iota
	FlashWarning
	FlashInfo
	FlashSuccess
)

// DefaultFlashDuration is how long flash messages are shown before auto-dismissing
const DefaultFlashDuration = 5 * time.Second

// FlashMessage represents a temporary message shown in the footer
type FlashMessage struct {
	Text      string
	Type      FlashType
	CreatedAt time.Time
	Duration  time.Duration
}

// IsExpired returns true if the flash message should be dismissed
func (f *FlashMessage) IsExpired() bool {
	return time.Since(f.CreatedAt) >= f.Duration
}

// FlashTickMsg is sent periodically to check for expired flash messages
type FlashTickMsg struct{}

// FlashTick returns a command that sends a FlashTickMsg after a delay
func FlashTick() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
		return FlashTickMsg{}
	})
}

// KeyBinding represents a keyboard shortcut
type KeyBinding struct {
	Key  string
	Desc string
}

// Footer represents the bottom footer bar with keybindings
type Footer struct {
	width              int
	bindings           []KeyBinding
	hasSession         bool          // Whether a session is selected
	sidebarFocused     bool          // Whether sidebar has focus
	pendingPermission  bool          // Whether chat has a pending permission prompt
	pendingQuestion    bool          // Whether chat has a pending question prompt
	streaming          bool          // Whether active session is streaming
	viewChangesMode    bool          // Whether showing view changes overlay
	searchMode         bool          // Whether sidebar is in search mode
	multiSelectMode    bool          // Whether sidebar is in multi-select mode
	hasDetectedOptions bool          // Whether chat has detected options for parallel exploration
	kittyKeyboard      bool          // Terminal supports Kitty keyboard protocol
	flashMessage       *FlashMessage // Current flash message, if any
}

// NewFooter creates a new footer
func NewFooter() *Footer {
	return &Footer{
		bindings: []KeyBinding{
			{Key: "tab", Desc: "switch pane"},
			{Key: "n", Desc: "new session"},
			{Key: "a", Desc: "add repo"},
			{Key: "v", Desc: "view changes"},
			{Key: "m", Desc: "merge/pr"},
			{Key: "f", Desc: "fork"},
			{Key: "d", Desc: "delete"},
			{Key: "q", Desc: "quit"},
			{Key: "?", Desc: "help"},
		},
	}
}

// SetContext updates the footer's context for conditional bindings
func (f *Footer) SetContext(hasSession, sidebarFocused, pendingPermission, pendingQuestion, streaming, viewChangesMode, searchMode, multiSelectMode, hasDetectedOptions, kittyKeyboard bool) {
	f.hasSession = hasSession
	f.sidebarFocused = sidebarFocused
	f.pendingPermission = pendingPermission
	f.pendingQuestion = pendingQuestion
	f.streaming = streaming
	f.viewChangesMode = viewChangesMode
	f.searchMode = searchMode
	f.multiSelectMode = multiSelectMode
	f.hasDetectedOptions = hasDetectedOptions
	f.kittyKeyboard = kittyKeyboard
}

// SetWidth sets the footer width
func (f *Footer) SetWidth(width int) {
	f.width = width
}

// SetBindings allows custom keybindings
func (f *Footer) SetBindings(bindings []KeyBinding) {
	f.bindings = bindings
}

// SetFlash sets a flash message to display in the footer
func (f *Footer) SetFlash(text string, flashType FlashType) {
	f.flashMessage = &FlashMessage{
		Text:      text,
		Type:      flashType,
		CreatedAt: time.Now(),
		Duration:  DefaultFlashDuration,
	}
}

// SetFlashWithDuration sets a flash message with a custom duration
func (f *Footer) SetFlashWithDuration(text string, flashType FlashType, duration time.Duration) {
	f.flashMessage = &FlashMessage{
		Text:      text,
		Type:      flashType,
		CreatedAt: time.Now(),
		Duration:  duration,
	}
}

// ClearFlash removes the current flash message
func (f *Footer) ClearFlash() {
	f.flashMessage = nil
}

// HasFlash returns true if there is an active flash message
func (f *Footer) HasFlash() bool {
	return f.flashMessage != nil
}

// ClearIfExpired clears the flash message if it has expired
// Returns true if the flash was cleared
func (f *Footer) ClearIfExpired() bool {
	if f.flashMessage != nil && f.flashMessage.IsExpired() {
		f.flashMessage = nil
		return true
	}
	return false
}

// flashStyle returns the appropriate style for the flash message type
func (f *Footer) flashStyle() lipgloss.Style {
	baseStyle := lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1).
		Width(f.width).
		MaxHeight(1)

	switch f.flashMessage.Type {
	case FlashError:
		return baseStyle.
			Foreground(ColorTextInverse).
			Background(ColorError)
	case FlashWarning:
		return baseStyle.
			Foreground(ColorTextInverse).
			Background(ColorWarning)
	case FlashSuccess:
		return baseStyle.
			Foreground(ColorTextInverse).
			Background(ColorSuccess)
	case FlashInfo:
		fallthrough
	default:
		return baseStyle.
			Foreground(ColorTextInverse).
			Background(ColorInfo)
	}
}

// flashIcon returns an icon prefix for the flash message type
func (f *Footer) flashIcon() string {
	switch f.flashMessage.Type {
	case FlashError:
		return "✕ "
	case FlashWarning:
		return "⚠ "
	case FlashSuccess:
		return "✓ "
	case FlashInfo:
		fallthrough
	default:
		return "ℹ "
	}
}

// footerSeparator returns the separator string between footer key bindings
func footerSeparator() string {
	sepStyle := lipgloss.NewStyle().
		Foreground(ColorBorder)
	return "  " + sepStyle.Render("|") + "  "
}

// View renders the footer
func (f *Footer) View() string {
	// If there's a flash message, show it instead of keybindings
	if f.flashMessage != nil {
		return f.flashStyle().Render(f.flashIcon() + f.flashMessage.Text)
	}

	var parts []string

	// Show view-changes-specific shortcuts when in view changes mode
	if f.viewChangesMode {
		viewChangesBindings := []KeyBinding{
			{Key: "←/→", Desc: "switch pane"},
			{Key: "↑/↓", Desc: "select file"},
			{Key: "j/k", Desc: "scroll diff"},
			{Key: "esc/q", Desc: "close"},
		}
		for _, b := range viewChangesBindings {
			key := FooterKeyStyle.Render(b.Key)
			desc := FooterDescStyle.Render(": " + b.Desc)
			parts = append(parts, key+desc)
		}
		content := strings.Join(parts, footerSeparator())
		return FooterStyle.Width(f.width).MaxHeight(1).Render(content)
	}

	// Show search-specific shortcuts when in search mode
	if f.searchMode {
		searchBindings := []KeyBinding{
			{Key: "esc", Desc: "cancel"},
			{Key: "enter", Desc: "select"},
			{Key: "↑/↓", Desc: "navigate"},
		}
		for _, b := range searchBindings {
			key := FooterKeyStyle.Render(b.Key)
			desc := FooterDescStyle.Render(": " + b.Desc)
			parts = append(parts, key+desc)
		}
		content := strings.Join(parts, footerSeparator())
		return FooterStyle.Width(f.width).MaxHeight(1).Render(content)
	}

	// Show multi-select-specific shortcuts when in multi-select mode
	if f.multiSelectMode {
		multiSelectBindings := []KeyBinding{
			{Key: "space", Desc: "toggle"},
			{Key: "a", Desc: "select all"},
			{Key: "n", Desc: "deselect all"},
			{Key: "enter", Desc: "bulk action"},
			{Key: "↑/↓", Desc: "navigate"},
			{Key: "esc", Desc: "exit"},
			{Key: "?", Desc: "help"},
		}
		for _, b := range multiSelectBindings {
			key := FooterKeyStyle.Render(b.Key)
			desc := FooterDescStyle.Render(": " + b.Desc)
			parts = append(parts, key+desc)
		}
		content := strings.Join(parts, footerSeparator())
		return FooterStyle.Width(f.width).MaxHeight(1).Render(content)
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
	} else if !f.sidebarFocused && f.hasSession {
		// Chat focused, not streaming - show enter and newline shortcut
		newlineKey := "opt+enter"
		if f.kittyKeyboard {
			newlineKey = "shift+enter"
		}
		chatBindings := []KeyBinding{
			{Key: "enter", Desc: "send"},
			{Key: newlineKey, Desc: "newline"},
		}
		// Show ctrl+o when options are detected
		if f.hasDetectedOptions {
			chatBindings = append(chatBindings, KeyBinding{Key: "ctrl+o", Desc: "fork options"})
		}
		chatBindings = append(chatBindings,
			KeyBinding{Key: "ctrl+v", Desc: "paste image"},
			KeyBinding{Key: "tab", Desc: "switch pane"},
			KeyBinding{Key: "pgup/dn", Desc: "scroll"},
		)
		for _, b := range chatBindings {
			key := FooterKeyStyle.Render(b.Key)
			desc := FooterDescStyle.Render(": " + b.Desc)
			parts = append(parts, key+desc)
		}
	} else {
		for _, b := range f.bindings {
			// Skip "?" here - it will be added at the end when sidebar is focused
			if b.Key == "?" {
				continue
			}
			// Skip tab when no session (can't switch to chat without one)
			if b.Key == "tab" && !f.hasSession {
				continue
			}
			// Skip sidebar-only bindings when chat is focused
			if (b.Key == "n" || b.Key == "a" || b.Key == "v" || b.Key == "m" || b.Key == "f" || b.Key == "d" || b.Key == "q") && !f.sidebarFocused {
				continue
			}
			// Skip session-specific bindings when no session selected
			if (b.Key == "v" || b.Key == "m" || b.Key == "f" || b.Key == "d") && !f.hasSession {
				continue
			}

			key := FooterKeyStyle.Render(b.Key)
			desc := FooterDescStyle.Render(": " + b.Desc)
			parts = append(parts, key+desc)
		}
		// Add "?" at the end only when sidebar is focused (can't trigger from chat textarea)
		if f.sidebarFocused {
			helpKey := FooterKeyStyle.Render("?")
			helpDesc := FooterDescStyle.Render(": help")
			parts = append(parts, helpKey+helpDesc)
		}
	}

	content := strings.Join(parts, footerSeparator())

	// Use MaxHeight(1) to ensure footer never wraps to multiple lines
	return FooterStyle.Width(f.width).MaxHeight(1).Render(content)
}
