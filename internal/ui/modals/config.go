package modals

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// =============================================================================
// WelcomeState - State for the first-time user welcome modal
// =============================================================================

type WelcomeState struct{}

func (*WelcomeState) modalState() {}

func (s *WelcomeState) Title() string { return "Welcome to Plural!" }

func (s *WelcomeState) Help() string {
	return "Press Enter or Esc to continue"
}

func (s *WelcomeState) Render() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorSecondary).
		MarginBottom(1).
		Render(s.Title())

	intro := lipgloss.NewStyle().
		Foreground(ColorText).
		Width(50).
		Render("Plural helps you manage multiple concurrent Claude Code sessions, each in its own git worktree for complete isolation.")

	gettingStarted := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginTop(1).
		Render("Getting started:")

	shortcuts := lipgloss.NewStyle().
		Foreground(ColorText).
		Render("  r   Add a git repository\n  n   Create a new session\n  Tab Switch between sidebar and chat")

	issuesLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginTop(1).
		Render("Need help or found a bug?")

	issuesLink := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Render("  github.com/zhubert/plural/issues")

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		intro,
		gettingStarted,
		shortcuts,
		issuesLabel,
		issuesLink,
		help,
	)
}

func (s *WelcomeState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	return s, nil
}

// NewWelcomeState creates a new WelcomeState
func NewWelcomeState() *WelcomeState {
	return &WelcomeState{}
}

// =============================================================================
// ChangelogState - State for the "What's New" changelog modal
// =============================================================================

type ChangelogState struct {
	Entries      []ChangelogEntry
	ScrollOffset int
	MaxVisible   int
}

func (*ChangelogState) modalState() {}

func (s *ChangelogState) Title() string { return "What's New" }

func (s *ChangelogState) Help() string {
	if len(s.Entries) > s.MaxVisible {
		return "up/down scroll  Enter/Esc: dismiss"
	}
	return "Press Enter or Esc to dismiss"
}

func (s *ChangelogState) Render() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorSecondary).
		MarginBottom(1).
		Render(s.Title())

	// Build changelog content
	var content string
	for i, entry := range s.Entries {
		if i < s.ScrollOffset {
			continue
		}
		if i >= s.ScrollOffset+s.MaxVisible {
			break
		}

		// Version header
		versionStr := "v" + entry.Version
		if entry.Date != "" {
			versionStr += " (" + entry.Date + ")"
		}
		versionLine := lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			Render(versionStr)

		// Changes
		var changes string
		for _, change := range entry.Changes {
			bullet := lipgloss.NewStyle().
				Foreground(ColorSecondary).
				Render("  - ")
			changeText := lipgloss.NewStyle().
				Foreground(ColorText).
				Width(45).
				Render(change)
			changes += bullet + changeText + "\n"
		}

		content += versionLine + "\n" + changes
		if i < len(s.Entries)-1 && i < s.ScrollOffset+s.MaxVisible-1 {
			content += "\n"
		}
	}

	// Scroll indicator
	if len(s.Entries) > s.MaxVisible {
		scrollInfo := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("(scroll for more)")
		content += "\n" + scrollInfo
	}

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, content, help)
}

func (s *ChangelogState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if s.ScrollOffset > 0 {
				s.ScrollOffset--
			}
		case "down", "j":
			if s.ScrollOffset < len(s.Entries)-s.MaxVisible {
				s.ScrollOffset++
			}
		}
	}
	return s, nil
}

// NewChangelogState creates a new ChangelogState
func NewChangelogState(entries []ChangelogEntry) *ChangelogState {
	return &ChangelogState{
		Entries:      entries,
		ScrollOffset: 0,
		MaxVisible:   5,
	}
}

// =============================================================================
// ThemeState - State for the Theme picker modal
// =============================================================================

// ThemeName type and related functions must be provided by the parent package
// We use string here and let the parent package handle the conversion
type ThemeState struct {
	Themes        []string
	SelectedIndex int
	CurrentTheme  string
	ThemeNames    []string // Display names for themes
}

func (*ThemeState) modalState() {}

func (s *ThemeState) Title() string { return "Select Theme" }

func (s *ThemeState) Help() string {
	return "up/down to select, Enter to apply, Esc to cancel"
}

func (s *ThemeState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	var content string
	for i, themeKey := range s.Themes {
		style := SidebarItemStyle
		prefix := "  "
		suffix := ""

		if i == s.SelectedIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}

		if themeKey == s.CurrentTheme {
			suffix = " (current)"
		}

		displayName := themeKey
		if i < len(s.ThemeNames) {
			displayName = s.ThemeNames[i]
		}

		content += style.Render(prefix+displayName+suffix) + "\n"
	}

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, content, help)
}

func (s *ThemeState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if s.SelectedIndex > 0 {
				s.SelectedIndex--
			}
		case "down", "j":
			if s.SelectedIndex < len(s.Themes)-1 {
				s.SelectedIndex++
			}
		}
	}
	return s, nil
}

// GetSelectedTheme returns the selected theme key
func (s *ThemeState) GetSelectedTheme() string {
	if len(s.Themes) == 0 || s.SelectedIndex >= len(s.Themes) {
		return ""
	}
	return s.Themes[s.SelectedIndex]
}

// NewThemeState creates a new ThemeState
func NewThemeState(themes []string, themeNames []string, currentTheme string) *ThemeState {
	// Find the index of the current theme
	selectedIndex := 0
	for i, t := range themes {
		if t == currentTheme {
			selectedIndex = i
			break
		}
	}

	return &ThemeState{
		Themes:        themes,
		ThemeNames:    themeNames,
		SelectedIndex: selectedIndex,
		CurrentTheme:  currentTheme,
	}
}

// =============================================================================
// SettingsState - State for the Settings modal
// =============================================================================

type SettingsState struct {
	BranchPrefixInput    textinput.Model
	NotificationsEnabled bool
	Focus                int // 0 = branch prefix, 1 = notifications checkbox
}

func (*SettingsState) modalState() {}

func (s *SettingsState) Title() string { return "Settings" }

func (s *SettingsState) Help() string {
	return "Tab: next field  Space: toggle checkbox  Enter: save  Esc: cancel"
}

func (s *SettingsState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Branch prefix field
	prefixLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Default branch prefix:")

	prefixDesc := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Italic(true).
		Width(50).
		Render("Applied to all new branches (e.g., \"zhubert/\" creates branches like \"zhubert/plural-...\")")

	prefixInputStyle := lipgloss.NewStyle()
	if s.Focus == 0 {
		prefixInputStyle = prefixInputStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
	} else {
		prefixInputStyle = prefixInputStyle.PaddingLeft(2)
	}
	prefixView := prefixInputStyle.Render(s.BranchPrefixInput.View())

	// Notifications checkbox
	notifLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginTop(1).
		Render("Desktop notifications:")

	checkbox := "[ ]"
	if s.NotificationsEnabled {
		checkbox = "[x]"
	}
	notifCheckboxStyle := lipgloss.NewStyle()
	if s.Focus == 1 {
		notifCheckboxStyle = notifCheckboxStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
	} else {
		notifCheckboxStyle = notifCheckboxStyle.PaddingLeft(2)
	}
	notifDesc := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Italic(true).
		Render("Notify when Claude finishes while app is in background")
	notifView := notifCheckboxStyle.Render(checkbox + " " + notifDesc)

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, prefixLabel, prefixDesc, prefixView, notifLabel, notifView, help)
}

func (s *SettingsState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "tab":
			s.Focus = (s.Focus + 1) % 2
			if s.Focus == 0 {
				s.BranchPrefixInput.Focus()
			} else {
				s.BranchPrefixInput.Blur()
			}
			return s, nil
		case "shift+tab":
			s.Focus = (s.Focus + 1) % 2
			if s.Focus == 0 {
				s.BranchPrefixInput.Focus()
			} else {
				s.BranchPrefixInput.Blur()
			}
			return s, nil
		case "space":
			// Toggle checkbox when focused on notifications
			if s.Focus == 1 {
				s.NotificationsEnabled = !s.NotificationsEnabled
			}
			return s, nil
		}
	}

	// Handle text input updates when focused on branch prefix
	if s.Focus == 0 {
		var cmd tea.Cmd
		s.BranchPrefixInput, cmd = s.BranchPrefixInput.Update(msg)
		return s, cmd
	}

	return s, nil
}

// GetBranchPrefix returns the branch prefix value
func (s *SettingsState) GetBranchPrefix() string {
	return s.BranchPrefixInput.Value()
}

// GetNotificationsEnabled returns whether notifications are enabled
func (s *SettingsState) GetNotificationsEnabled() bool {
	return s.NotificationsEnabled
}

// NewSettingsState creates a new SettingsState with the current settings values
func NewSettingsState(currentBranchPrefix string, notificationsEnabled bool) *SettingsState {
	prefixInput := textinput.New()
	prefixInput.Placeholder = "e.g., zhubert/ (leave empty for no prefix)"
	prefixInput.CharLimit = 50
	prefixInput.SetWidth(ModalInputWidth)
	prefixInput.SetValue(currentBranchPrefix)
	prefixInput.Focus()

	return &SettingsState{
		BranchPrefixInput:    prefixInput,
		NotificationsEnabled: notificationsEnabled,
		Focus:                0,
	}
}
