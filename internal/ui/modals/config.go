package modals

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/zhubert/plural/internal/keys"
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
		Render("  a   Add a git repository\n  n   Create a new session\n  Tab Switch between sidebar and chat")

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
	Entries         []ChangelogEntry
	ScrollOffset    int
	maxVisibleLines int
	totalLines      int
}

func (*ChangelogState) modalState() {}

func (s *ChangelogState) Title() string { return "What's New" }

func (s *ChangelogState) Help() string {
	if s.totalLines > s.maxVisibleLines {
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

	// Build all lines first to enable line-based scrolling
	// Each entry in allLines represents one visual line (after text wrapping)
	var allLines []string

	for i, entry := range s.Entries {
		if i > 0 {
			allLines = append(allLines, "") // Blank line between versions
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
		allLines = append(allLines, versionLine)

		// Changes - handle text wrapping by splitting into individual visual lines
		for _, change := range entry.Changes {
			changeText := lipgloss.NewStyle().
				Foreground(ColorText).
				Width(45).
				Render(change)
			// Split wrapped text into individual lines
			wrappedLines := strings.Split(changeText, "\n")
			for j, line := range wrappedLines {
				bullet := lipgloss.NewStyle().
					Foreground(ColorSecondary).
					Render("  - ")
				if j == 0 {
					allLines = append(allLines, bullet+line)
				} else {
					// Continuation lines get padding to align with first line
					allLines = append(allLines, "    "+line)
				}
			}
		}
	}

	s.totalLines = len(allLines)

	// Apply scroll offset and limit visible lines
	var visibleLines []string
	for i, line := range allLines {
		if i < s.ScrollOffset {
			continue
		}
		if len(visibleLines) >= s.maxVisibleLines {
			break
		}
		visibleLines = append(visibleLines, line)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, visibleLines...)

	// Scroll indicator
	if s.totalLines > s.maxVisibleLines {
		scrollInfo := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			MarginTop(1).
			Render("(scroll for more)")
		content += "\n" + scrollInfo
	}

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, content, help)
}

func (s *ChangelogState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case keys.Up, "k":
			if s.ScrollOffset > 0 {
				s.ScrollOffset--
			}
		case keys.Down, "j":
			maxOffset := max(0, s.totalLines-s.maxVisibleLines)
			if s.ScrollOffset < maxOffset {
				s.ScrollOffset++
			}
		}
	case tea.MouseWheelMsg:
		maxOffset := max(0, s.totalLines-s.maxVisibleLines)
		if msg.Y < 0 {
			// Scroll up
			if s.ScrollOffset > 0 {
				s.ScrollOffset--
			}
		} else if msg.Y > 0 {
			// Scroll down
			if s.ScrollOffset < maxOffset {
				s.ScrollOffset++
			}
		}
	}
	return s, nil
}

// NewChangelogState creates a new ChangelogState
func NewChangelogState(entries []ChangelogEntry) *ChangelogState {
	return &ChangelogState{
		Entries:         entries,
		ScrollOffset:    0,
		maxVisibleLines: ChangelogModalMaxVisible,
	}
}

// =============================================================================
// SettingsState - State for the Settings modal
// =============================================================================

// AsanaProjectOption represents a selectable Asana project.
type AsanaProjectOption struct {
	GID  string
	Name string
}

// AsanaProjectMaxVisible is the max number of projects shown in the scrollable list.
const AsanaProjectMaxVisible = 5

type SettingsState struct {
	// Bound form values
	selectedTheme        string
	OriginalTheme        string // To detect if theme changed
	branchPrefix         string
	NotificationsEnabled bool
	AutoCleanupMerged    bool // Auto-cleanup sessions when PR merged/closed
	containerImage       string
	ContainersSupported  bool // Whether Docker is available for container mode

	// MultiSelect bindings
	generalOptions []string

	form *huh.Form

	// Size tracking
	availableWidth int
}

const (
	optionNotifications = "notifications"
	optionAutoCleanup   = "auto-cleanup"
)

func (*SettingsState) modalState() {}

func (s *SettingsState) PreferredWidth() int { return ModalWidthWide }

// SetSize updates the available width for rendering content.
func (s *SettingsState) SetSize(width, height int) {
	s.availableWidth = width
	s.form.WithWidth(s.contentWidth())
}

func (s *SettingsState) contentWidth() int {
	if s.availableWidth > 0 {
		return s.availableWidth - 10
	}
	return ModalWidthWide - 10
}

func (s *SettingsState) Title() string { return "Settings" }

func (s *SettingsState) Help() string {
	return "Tab: next field  Enter: save  Esc: cancel"
}

func (s *SettingsState) Render() string {
	title := ModalTitleStyle.Render(s.Title())
	help := ModalHelpStyle.Render(s.Help())
	return lipgloss.JoinVertical(lipgloss.Left, title, s.form.View(), help)
}

func (s *SettingsState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	var cmd tea.Cmd
	s.form, cmd = huhFormUpdate(s.form, msg)
	s.syncFromMultiSelect()
	return s, cmd
}

// syncFromMultiSelect updates boolean fields from the MultiSelect bindings.
func (s *SettingsState) syncFromMultiSelect() {
	s.NotificationsEnabled = slices.Contains(s.generalOptions, optionNotifications)
	s.AutoCleanupMerged = slices.Contains(s.generalOptions, optionAutoCleanup)
}

// GetBranchPrefix returns the branch prefix value
func (s *SettingsState) GetBranchPrefix() string {
	return s.branchPrefix
}

// GetNotificationsEnabled returns whether notifications are enabled
func (s *SettingsState) GetNotificationsEnabled() bool {
	return s.NotificationsEnabled
}

// GetContainerImage returns the container image name, or empty string if unchanged/empty.
func (s *SettingsState) GetContainerImage() string {
	return strings.TrimSpace(s.containerImage)
}

// GetSelectedTheme returns the selected theme key.
func (s *SettingsState) GetSelectedTheme() string {
	return s.selectedTheme
}

// ThemeChanged returns true if the selected theme differs from the original.
func (s *SettingsState) ThemeChanged() bool {
	return s.selectedTheme != s.OriginalTheme
}

// SetBranchPrefix sets the branch prefix value.
// Must be called before the form is displayed to the user. Works because
// huh binds via pointer, so mutations to the struct field reflect in the form.
func (s *SettingsState) SetBranchPrefix(v string) {
	s.branchPrefix = v
}

// LinearTeamOption represents a selectable Linear team.
type LinearTeamOption struct {
	ID   string
	Name string
}

// Shared render helpers

func renderSectionHeader(title string) string {
	return lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true).
		MarginTop(1).
		Render(title)
}

// NewSettingsState creates a new SettingsState with the current settings values.
func NewSettingsState(themes []string, themeDisplayNames []string, currentTheme string,
	currentBranchPrefix string, notificationsEnabled bool,
	containersSupported bool, containerImage string,
	autoCleanupMerged bool) *SettingsState {

	s := &SettingsState{
		selectedTheme:        currentTheme,
		OriginalTheme:        currentTheme,
		branchPrefix:         currentBranchPrefix,
		NotificationsEnabled: notificationsEnabled,
		AutoCleanupMerged:    autoCleanupMerged,
		containerImage:       containerImage,
		ContainersSupported:  containersSupported,
		availableWidth:       ModalWidthWide,
	}

	// Build theme options
	themeOptions := make([]huh.Option[string], len(themes))
	for i := range themes {
		themeOptions[i] = huh.NewOption(themeDisplayNames[i], themes[i])
	}

	// Build general options MultiSelect
	generalOpts := []huh.Option[string]{
		huh.NewOption("Desktop notifications", optionNotifications).
			Selected(notificationsEnabled),
		huh.NewOption("Auto-cleanup merged sessions", optionAutoCleanup).
			Selected(autoCleanupMerged),
	}
	// Initialize the enabledOptions slice to match
	if notificationsEnabled {
		s.generalOptions = append(s.generalOptions, optionNotifications)
	}
	if autoCleanupMerged {
		s.generalOptions = append(s.generalOptions, optionAutoCleanup)
	}

	// General settings group
	generalGroup := huh.NewGroup(
		huh.NewSelect[string]().
			Title("Theme").
			Options(themeOptions...).
			Value(&s.selectedTheme),
		huh.NewInput().
			Title("Default branch prefix").
			Description("Applied to all new branches").
			Placeholder("e.g., zhubert/").
			CharLimit(BranchPrefixCharLimit).
			Value(&s.branchPrefix),
		huh.NewMultiSelect[string]().
			Title("Options").
			Options(generalOpts...).
			Height(len(generalOpts)).
			Value(&s.generalOptions),
	)

	// Container settings group (conditionally shown)
	containerGroup := huh.NewGroup(
		huh.NewInput().
			Title("Container image").
			Description("Image name used for container mode sessions").
			Placeholder("ghcr.io/zhubert/plural-claude").
			CharLimit(200).
			Value(&s.containerImage),
	).WithHideFunc(func() bool { return !containersSupported })

	s.form = huh.NewForm(generalGroup, containerGroup).
		WithTheme(ModalTheme()).
		WithShowHelp(false).
		WithWidth(s.contentWidth()).
		WithLayout(huh.LayoutStack)

	initHuhForm(s.form)
	return s
}
