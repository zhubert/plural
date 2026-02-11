package modals

import (
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
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

type SettingsState struct {
	// Theme selection (focus 0)
	Themes             []string // Theme keys
	ThemeDisplayNames  []string // Display names for themes
	SelectedThemeIndex int
	OriginalTheme      string // To detect if theme changed

	BranchPrefixInput    textinput.Model
	AsanaProjectInput    textinput.Model
	NotificationsEnabled bool
	AsanaPATSet          bool // Whether ASANA_PAT env var is set
	Focus                int  // 0 = theme, 1 = branch prefix, 2 = notifications, 3 = repo selector, [4 = asana if PAT set]

	// Multi-repo support
	Repos            []string          // All registered repos
	SelectedRepoIndex int              // Currently displayed repo
	AsanaProjects    map[string]string // Per-repo Asana GIDs (accumulated across switches)
}

func (*SettingsState) modalState() {}

func (s *SettingsState) PreferredWidth() int { return ModalWidthWide }

func (s *SettingsState) Title() string { return "Settings" }

func (s *SettingsState) Help() string {
	if s.Focus == 0 {
		return "Tab: next field  Left/Right: change theme  Enter: save  Esc: cancel"
	}
	if s.Focus == 3 && len(s.Repos) > 0 {
		return "Tab: next field  Left/Right: switch repo  Enter: save  Esc: cancel"
	}
	return "Tab: next field  Space: toggle  Enter: save  Esc: cancel"
}

func (s *SettingsState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Theme selector
	themeLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Theme:")

	themeSelectorStyle := lipgloss.NewStyle()
	if s.Focus == 0 {
		themeSelectorStyle = themeSelectorStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
	} else {
		themeSelectorStyle = themeSelectorStyle.PaddingLeft(2)
	}
	leftArrowTheme := " "
	rightArrowTheme := " "
	if s.SelectedThemeIndex > 0 {
		leftArrowTheme = lipgloss.NewStyle().Foreground(ColorPrimary).Render("<")
	}
	if s.SelectedThemeIndex < len(s.Themes)-1 {
		rightArrowTheme = lipgloss.NewStyle().Foreground(ColorPrimary).Render(">")
	}
	themeName := ""
	if s.SelectedThemeIndex < len(s.ThemeDisplayNames) {
		themeName = s.ThemeDisplayNames[s.SelectedThemeIndex]
	}
	themeDisplay := lipgloss.NewStyle().Foreground(ColorText).Bold(true).Render(themeName)
	themeView := themeSelectorStyle.Render(leftArrowTheme + " " + themeDisplay + " " + rightArrowTheme)

	// Branch prefix field
	prefixLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginTop(1).
		Render("Default branch prefix:")

	prefixDesc := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Italic(true).
		Width(70).
		Render("Applied to all new branches (e.g., \"zhubert/\" creates branches like \"zhubert/plural-...\")")

	prefixInputStyle := lipgloss.NewStyle()
	if s.Focus == 1 {
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

	notifCheckbox := "[ ]"
	if s.NotificationsEnabled {
		notifCheckbox = "[x]"
	}
	notifCheckboxStyle := lipgloss.NewStyle()
	if s.Focus == 2 {
		notifCheckboxStyle = notifCheckboxStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
	} else {
		notifCheckboxStyle = notifCheckboxStyle.PaddingLeft(2)
	}
	notifDesc := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Italic(true).
		Render("Notify when Claude finishes while app is in background")
	notifView := notifCheckboxStyle.Render(notifCheckbox + " " + notifDesc)

	// Per-repo settings (shown when repos exist)
	var repoSections []string
	if len(s.Repos) > 0 {
		// Section header
		sectionHeader := lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true).
			MarginTop(1).
			Render("Per-repo settings:")

		// Repo selector
		repoName := filepath.Base(s.selectedRepoPath())
		selectorStyle := lipgloss.NewStyle()
		if s.Focus == 3 {
			selectorStyle = selectorStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
		} else {
			selectorStyle = selectorStyle.PaddingLeft(2)
		}
		leftArrow := " "
		rightArrow := " "
		if s.SelectedRepoIndex > 0 {
			leftArrow = lipgloss.NewStyle().Foreground(ColorPrimary).Render("<")
		}
		if s.SelectedRepoIndex < len(s.Repos)-1 {
			rightArrow = lipgloss.NewStyle().Foreground(ColorPrimary).Render(">")
		}
		repoDisplay := lipgloss.NewStyle().Foreground(ColorText).Bold(true).Render(repoName)
		selectorView := selectorStyle.Render(leftArrow + " " + repoDisplay + " " + rightArrow)
		repoSections = append(repoSections, sectionHeader+"\n"+selectorView)

		// Asana project GID (only shown when ASANA_PAT is set)
		if s.AsanaPATSet {
			asanaLabel := lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				MarginTop(1).
				Render("Asana project GID:")

			asanaDesc := lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Italic(true).
				Width(70).
				Render("Links this repo to an Asana project for task import")

			asanaInputStyle := lipgloss.NewStyle()
			if s.Focus == s.asanaFocusIndex() {
				asanaInputStyle = asanaInputStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
			} else {
				asanaInputStyle = asanaInputStyle.PaddingLeft(2)
			}
			asanaView := asanaInputStyle.Render(s.AsanaProjectInput.View())
			repoSections = append(repoSections, asanaLabel+"\n"+asanaDesc+"\n"+asanaView)
		}
	}

	help := ModalHelpStyle.Render(s.Help())

	parts := []string{title, themeLabel, themeView, prefixLabel, prefixDesc, prefixView, notifLabel, notifView}
	for _, section := range repoSections {
		parts = append(parts, section)
	}
	parts = append(parts, help)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// numFields returns the number of focusable fields in the settings modal.
func (s *SettingsState) numFields() int {
	if len(s.Repos) == 0 {
		return 3 // theme, branch prefix, notifications
	}
	if s.AsanaPATSet {
		return 5 // theme, branch prefix, notifications, repo selector, asana
	}
	return 4 // theme, branch prefix, notifications, repo selector
}

// asanaFocusIndex returns the focus index for the Asana project field.
// Only meaningful when AsanaPATSet is true.
func (s *SettingsState) asanaFocusIndex() int {
	return 4
}

// selectedRepoPath returns the path of the currently selected repo.
func (s *SettingsState) selectedRepoPath() string {
	if len(s.Repos) == 0 || s.SelectedRepoIndex >= len(s.Repos) {
		return ""
	}
	return s.Repos[s.SelectedRepoIndex]
}

// flushCurrentToMaps saves the currently displayed per-repo values to the maps.
func (s *SettingsState) flushCurrentToMaps() {
	repo := s.selectedRepoPath()
	if repo == "" {
		return
	}
	s.AsanaProjects[repo] = s.AsanaProjectInput.Value()
}

// loadRepoValues loads per-repo values from the maps into the displayed fields.
func (s *SettingsState) loadRepoValues() {
	repo := s.selectedRepoPath()
	if repo == "" {
		return
	}
	s.AsanaProjectInput.SetValue(s.AsanaProjects[repo])
}

// switchRepo saves current values, changes index, loads new values.
func (s *SettingsState) switchRepo(delta int) {
	if len(s.Repos) == 0 {
		return
	}
	newIndex := s.SelectedRepoIndex + delta
	if newIndex < 0 || newIndex >= len(s.Repos) {
		return // clamp at bounds
	}
	s.flushCurrentToMaps()
	s.SelectedRepoIndex = newIndex
	s.loadRepoValues()
}

func (s *SettingsState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	numFields := s.numFields()

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case keys.Tab:
			s.Focus = (s.Focus + 1) % numFields
			s.updateInputFocus()
			return s, nil
		case keys.ShiftTab:
			s.Focus = (s.Focus - 1 + numFields) % numFields
			s.updateInputFocus()
			return s, nil
		case keys.Space:
			if s.Focus == 2 {
				s.NotificationsEnabled = !s.NotificationsEnabled
			}
			return s, nil
		case keys.Left, "h":
			if s.Focus == 0 && len(s.Themes) > 0 {
				if s.SelectedThemeIndex > 0 {
					s.SelectedThemeIndex--
				}
				return s, nil
			}
			if s.Focus == 3 && len(s.Repos) > 0 {
				s.switchRepo(-1)
				return s, nil
			}
		case keys.Right, "l":
			if s.Focus == 0 && len(s.Themes) > 0 {
				if s.SelectedThemeIndex < len(s.Themes)-1 {
					s.SelectedThemeIndex++
				}
				return s, nil
			}
			if s.Focus == 3 && len(s.Repos) > 0 {
				s.switchRepo(1)
				return s, nil
			}
		}
	}

	// Handle text input updates when focused on branch prefix
	if s.Focus == 1 {
		var cmd tea.Cmd
		s.BranchPrefixInput, cmd = s.BranchPrefixInput.Update(msg)
		return s, cmd
	}

	// Handle text input updates when focused on Asana project GID
	if s.AsanaPATSet && s.Focus == s.asanaFocusIndex() {
		var cmd tea.Cmd
		s.AsanaProjectInput, cmd = s.AsanaProjectInput.Update(msg)
		return s, cmd
	}

	return s, nil
}

// updateInputFocus manages focus state for text inputs based on current Focus index.
func (s *SettingsState) updateInputFocus() {
	if s.Focus == 1 {
		s.BranchPrefixInput.Focus()
		s.AsanaProjectInput.Blur()
	} else if s.Focus == s.asanaFocusIndex() {
		s.AsanaProjectInput.Focus()
		s.BranchPrefixInput.Blur()
	} else {
		s.BranchPrefixInput.Blur()
		s.AsanaProjectInput.Blur()
	}
}

// GetBranchPrefix returns the branch prefix value
func (s *SettingsState) GetBranchPrefix() string {
	return s.BranchPrefixInput.Value()
}

// GetNotificationsEnabled returns whether notifications are enabled
func (s *SettingsState) GetNotificationsEnabled() bool {
	return s.NotificationsEnabled
}

// GetRepoPath returns the currently selected repo path (for backward compat).
func (s *SettingsState) GetRepoPath() string {
	return s.selectedRepoPath()
}

// GetAsanaProject returns the Asana project GID value for the currently displayed repo.
func (s *SettingsState) GetAsanaProject() string {
	return s.AsanaProjectInput.Value()
}

// GetSelectedTheme returns the selected theme key.
func (s *SettingsState) GetSelectedTheme() string {
	if len(s.Themes) == 0 || s.SelectedThemeIndex >= len(s.Themes) {
		return ""
	}
	return s.Themes[s.SelectedThemeIndex]
}

// ThemeChanged returns true if the selected theme differs from the original.
func (s *SettingsState) ThemeChanged() bool {
	return s.GetSelectedTheme() != s.OriginalTheme
}

// GetAllAsanaProjects flushes the current display values and returns a copy of all per-repo Asana projects.
func (s *SettingsState) GetAllAsanaProjects() map[string]string {
	s.flushCurrentToMaps()
	result := make(map[string]string, len(s.AsanaProjects))
	for k, v := range s.AsanaProjects {
		result[k] = v
	}
	return result
}

// NewSettingsState creates a new SettingsState with the current settings values.
// themes and themeDisplayNames are parallel slices of theme keys and display names.
// currentTheme is the currently active theme key.
// repos is the list of all registered repos. asanaProjects maps repo paths to Asana
// project GIDs. defaultRepoIndex is the initially selected repo (e.g., the active
// session's repo), clamped to valid range. asanaPATSet indicates whether ASANA_PAT
// env var is set (controls visibility of the Asana project GID field).
func NewSettingsState(themes []string, themeDisplayNames []string, currentTheme string,
	currentBranchPrefix string, notificationsEnabled bool, repos []string,
	asanaProjects map[string]string,
	defaultRepoIndex int, asanaPATSet bool) *SettingsState {

	// Find the index of the current theme
	selectedThemeIndex := 0
	for i, t := range themes {
		if t == currentTheme {
			selectedThemeIndex = i
			break
		}
	}

	prefixInput := textinput.New()
	prefixInput.Placeholder = "e.g., zhubert/ (leave empty for no prefix)"
	prefixInput.CharLimit = BranchPrefixCharLimit
	prefixInput.SetWidth(ModalInputWidth)
	prefixInput.SetValue(currentBranchPrefix)

	// Clamp default repo index
	if defaultRepoIndex < 0 || (len(repos) > 0 && defaultRepoIndex >= len(repos)) {
		defaultRepoIndex = 0
	}

	// Copy map to avoid mutating caller's data
	ap := make(map[string]string, len(asanaProjects))
	for k, v := range asanaProjects {
		ap[k] = v
	}

	// Set up the initial Asana input for the default repo
	var initialAsana string
	if len(repos) > 0 && defaultRepoIndex < len(repos) {
		initialAsana = ap[repos[defaultRepoIndex]]
	}

	asanaInput := textinput.New()
	asanaInput.Placeholder = "e.g., 1234567890123 (leave empty to disable)"
	asanaInput.CharLimit = BranchPrefixCharLimit
	asanaInput.SetWidth(ModalInputWidth)
	asanaInput.SetValue(initialAsana)

	return &SettingsState{
		Themes:               themes,
		ThemeDisplayNames:    themeDisplayNames,
		SelectedThemeIndex:   selectedThemeIndex,
		OriginalTheme:        currentTheme,
		BranchPrefixInput:    prefixInput,
		AsanaProjectInput:    asanaInput,
		NotificationsEnabled: notificationsEnabled,
		AsanaPATSet:          asanaPATSet,
		Focus:                0,
		Repos:                repos,
		SelectedRepoIndex:    defaultRepoIndex,
		AsanaProjects:        ap,
	}
}
