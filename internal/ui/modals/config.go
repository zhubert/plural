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

// AsanaProjectOption represents a selectable Asana project.
type AsanaProjectOption struct {
	GID  string
	Name string
}

// AsanaProjectMaxVisible is the max number of projects shown in the scrollable list.
const AsanaProjectMaxVisible = 5

type SettingsState struct {
	// Theme selection (focus 0)
	Themes             []string // Theme keys
	ThemeDisplayNames  []string // Display names for themes
	SelectedThemeIndex int
	OriginalTheme      string // To detect if theme changed

	BranchPrefixInput    textinput.Model
	NotificationsEnabled bool

	// Container image (only shown when ContainersSupported)
	ContainerImageInput textinput.Model
	ContainersSupported bool // Whether Docker is available for container mode

	AsanaPATSet bool // Whether ASANA_PAT env var is set
	Focus       int  // 0=theme, 1=branch prefix, 2=notifications, [3=container image if supported], then repo selector, asana

	// Multi-repo support
	Repos             []string          // All registered repos
	SelectedRepoIndex int               // Currently displayed repo
	AsanaSelectedGIDs map[string]string // Per-repo selected Asana project GIDs

	// Asana project selector (replaces text input)
	AsanaProjectOptions []AsanaProjectOption // All fetched projects (cached for modal lifetime)
	AsanaSearchInput    textinput.Model      // Search/filter text input
	AsanaCursorIndex    int                  // Cursor position in filtered list
	AsanaScrollOffset   int                  // Scroll offset for filtered list
	AsanaLoading        bool                 // Whether projects are being fetched
	AsanaLoadError      string               // Error message from fetch

	// Size tracking
	availableWidth int // Actual width available after modal is clamped to screen
}

func (*SettingsState) modalState() {}

func (s *SettingsState) PreferredWidth() int { return ModalWidthWide }

// SetSize updates the available width for rendering content.
// Called by the modal container before Render() to notify the modal of its actual size.
func (s *SettingsState) SetSize(width, height int) {
	s.availableWidth = width
	// Update text input widths based on actual available width
	contentWidth := s.contentWidth()
	s.BranchPrefixInput.SetWidth(contentWidth)
	s.ContainerImageInput.SetWidth(contentWidth)
	// Search input is slightly narrower to account for extra padding
	s.AsanaSearchInput.SetWidth(contentWidth - 4)
}

// contentWidth returns the width available for content inside the modal.
// Falls back to ModalWidthWide if availableWidth is not set.
func (s *SettingsState) contentWidth() int {
	if s.availableWidth > 0 {
		return s.availableWidth - 10 // Leave room for padding
	}
	return ModalWidthWide - 10
}

func (s *SettingsState) Title() string { return "Settings" }

func (s *SettingsState) Help() string {
	if s.Focus == 0 {
		return "Tab: next field  Left/Right: change theme  Enter: save  Esc: cancel"
	}
	if s.Focus == s.repoSelectorFocusIndex() && len(s.Repos) > 0 && s.AsanaPATSet {
		return "Tab: next field  Left/Right: switch repo  Enter: save  Esc: cancel"
	}
	if s.Focus == s.asanaFocusIndex() && s.AsanaPATSet {
		return "Tab: next field  Up/Down: navigate  Enter: select  Esc: cancel"
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
		Width(s.contentWidth()).
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

	// Container image field (only on Apple Silicon) - collected for later append
	var containerParts []string
	if s.ContainersSupported {
		containerLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			MarginTop(1).
			Render("Container image:")

		containerDesc := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Width(s.contentWidth()).
			Render("Image name used for container mode sessions")

		containerInputStyle := lipgloss.NewStyle()
		if s.Focus == s.containerImageFocusIndex() {
			containerInputStyle = containerInputStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
		} else {
			containerInputStyle = containerInputStyle.PaddingLeft(2)
		}
		containerView := containerInputStyle.Render(s.ContainerImageInput.View())

		containerParts = []string{containerLabel, containerDesc, containerView}
	}

	// Per-repo settings (shown when repos exist and there's something to configure)
	var repoSections []string
	if len(s.Repos) > 0 && s.AsanaPATSet {
		// Section header
		sectionHeader := lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true).
			MarginTop(1).
			Render("Per-repo settings:")

		// Repo selector
		repoName := filepath.Base(s.selectedRepoPath())
		selectorStyle := lipgloss.NewStyle()
		if s.Focus == s.repoSelectorFocusIndex() {
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

		// Asana project selector
		asanaLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			MarginTop(1).
			Render("Asana project:")

		asanaDesc := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Width(s.contentWidth()).
			Render("Links this repo to an Asana project for task import")

		asanaContent := s.renderAsanaSelector()

		asanaStyle := lipgloss.NewStyle()
		if s.Focus == s.asanaFocusIndex() {
			asanaStyle = asanaStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
		} else {
			asanaStyle = asanaStyle.PaddingLeft(2)
		}
		asanaView := asanaStyle.Render(asanaContent)
		repoSections = append(repoSections, asanaLabel+"\n"+asanaDesc+"\n"+asanaView)
	}

	help := ModalHelpStyle.Render(s.Help())

	parts := []string{title, themeLabel, themeView, prefixLabel, prefixDesc, prefixView, notifLabel, notifView}
	parts = append(parts, containerParts...)
	for _, section := range repoSections {
		parts = append(parts, section)
	}
	parts = append(parts, help)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// renderAsanaSelector renders the Asana project search and selection UI.
func (s *SettingsState) renderAsanaSelector() string {
	if s.AsanaLoading {
		return lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("Fetching Asana projects...")
	}

	if s.AsanaLoadError != "" {
		return lipgloss.NewStyle().
			Foreground(ColorWarning).
			Render(s.AsanaLoadError)
	}

	var parts []string

	// Show current selection
	repo := s.selectedRepoPath()
	currentGID := s.AsanaSelectedGIDs[repo]
	currentLabel := "(none)"
	for _, opt := range s.AsanaProjectOptions {
		if opt.GID == currentGID {
			currentLabel = opt.Name
			break
		}
	}
	currentLine := lipgloss.NewStyle().
		Foreground(ColorText).
		Render("Current: " + currentLabel)
	parts = append(parts, currentLine)

	// Search input
	searchLabel := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Search: ")
	parts = append(parts, searchLabel+s.AsanaSearchInput.View())

	// Filtered list
	filtered := s.getFilteredAsanaProjects()
	if len(filtered) == 0 {
		msg := "No projects match your search."
		if len(s.AsanaProjectOptions) == 0 {
			msg = "No projects available."
		}
		parts = append(parts, lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render(msg))
	} else {
		startIdx := s.AsanaScrollOffset
		endIdx := startIdx + AsanaProjectMaxVisible
		if endIdx > len(filtered) {
			endIdx = len(filtered)
		}

		var listContent string

		if startIdx > 0 {
			listContent += lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Render("  ^ more above") + "\n"
		}

		for i := startIdx; i < endIdx; i++ {
			opt := filtered[i]
			style := SidebarItemStyle
			prefix := "  "
			if i == s.AsanaCursorIndex {
				style = SidebarSelectedStyle
				prefix = "> "
			}
			listContent += style.Render(prefix+opt.Name) + "\n"
		}

		if endIdx < len(filtered) {
			listContent += lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Render("  v more below")
		}

		parts = append(parts, listContent)
	}

	return strings.Join(parts, "\n")
}

// getFilteredAsanaProjects returns projects matching the current search query.
func (s *SettingsState) getFilteredAsanaProjects() []AsanaProjectOption {
	query := strings.ToLower(s.AsanaSearchInput.Value())
	if query == "" {
		return s.AsanaProjectOptions
	}

	var filtered []AsanaProjectOption
	for _, opt := range s.AsanaProjectOptions {
		if strings.Contains(strings.ToLower(opt.Name), query) {
			filtered = append(filtered, opt)
		}
	}
	return filtered
}

// numFields returns the number of focusable fields in the settings modal.
func (s *SettingsState) numFields() int {
	base := 3 // theme, branch prefix, notifications
	if s.ContainersSupported {
		base++ // container image
	}
	if len(s.Repos) > 0 && s.AsanaPATSet {
		base += 2 // repo selector, asana
	}
	return base
}

// containerImageFocusIndex returns the focus index for the container image field.
// Only meaningful when ContainersSupported is true.
func (s *SettingsState) containerImageFocusIndex() int {
	return 3 // theme=0, branch prefix=1, notifications=2, container image=3
}

// repoSelectorFocusIndex returns the focus index for the repo selector field.
func (s *SettingsState) repoSelectorFocusIndex() int {
	base := 3 // theme, branch prefix, notifications
	if s.ContainersSupported {
		base++ // container image shifts it up
	}
	return base
}

// asanaFocusIndex returns the focus index for the Asana project field.
// Only meaningful when AsanaPATSet is true.
func (s *SettingsState) asanaFocusIndex() int {
	return s.repoSelectorFocusIndex() + 1
}

// selectedRepoPath returns the path of the currently selected repo.
func (s *SettingsState) selectedRepoPath() string {
	if len(s.Repos) == 0 || s.SelectedRepoIndex >= len(s.Repos) {
		return ""
	}
	return s.Repos[s.SelectedRepoIndex]
}

// flushCurrentToMaps is a no-op for the new selector-based flow.
// Selections are stored immediately on Enter.
func (s *SettingsState) flushCurrentToMaps() {
	// No-op: AsanaSelectedGIDs is updated directly on selection
}

// loadRepoValues resets the search and cursor when switching repos.
func (s *SettingsState) loadRepoValues() {
	s.AsanaSearchInput.SetValue("")
	s.AsanaCursorIndex = 0
	s.AsanaScrollOffset = 0
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

	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}

	key := keyMsg.String()

	// When focused on Asana selector, handle search + list navigation
	if s.AsanaPATSet && s.Focus == s.asanaFocusIndex() {
		switch key {
		case keys.Tab:
			s.Focus = (s.Focus + 1) % numFields
			s.updateInputFocus()
			return s, nil
		case keys.ShiftTab:
			s.Focus = (s.Focus - 1 + numFields) % numFields
			s.updateInputFocus()
			return s, nil
		case keys.Up:
			s.asanaNavigate(-1)
			return s, nil
		case keys.Down:
			s.asanaNavigate(1)
			return s, nil
		case keys.Enter:
			// Select the highlighted project
			filtered := s.getFilteredAsanaProjects()
			if s.AsanaCursorIndex < len(filtered) {
				selected := filtered[s.AsanaCursorIndex]
				repo := s.selectedRepoPath()
				if repo != "" {
					s.AsanaSelectedGIDs[repo] = selected.GID
				}
			}
			return s, nil
		default:
			// Send all other keys to search input
			var cmd tea.Cmd
			oldQuery := s.AsanaSearchInput.Value()
			s.AsanaSearchInput, cmd = s.AsanaSearchInput.Update(msg)
			newQuery := s.AsanaSearchInput.Value()
			if newQuery != oldQuery {
				s.AsanaCursorIndex = 0
				s.AsanaScrollOffset = 0
			}
			return s, cmd
		}
	}

	switch key {
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
		if s.Focus == s.repoSelectorFocusIndex() && len(s.Repos) > 0 && s.AsanaPATSet {
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
		if s.Focus == s.repoSelectorFocusIndex() && len(s.Repos) > 0 && s.AsanaPATSet {
			s.switchRepo(1)
			return s, nil
		}
	}

	// Handle text input updates when focused on branch prefix
	if s.Focus == 1 {
		var cmd tea.Cmd
		s.BranchPrefixInput, cmd = s.BranchPrefixInput.Update(msg)
		return s, cmd
	}

	// Handle text input updates when focused on container image
	if s.ContainersSupported && s.Focus == s.containerImageFocusIndex() {
		var cmd tea.Cmd
		s.ContainerImageInput, cmd = s.ContainerImageInput.Update(msg)
		return s, cmd
	}

	return s, nil
}

// asanaNavigate moves the cursor up or down in the filtered project list.
func (s *SettingsState) asanaNavigate(delta int) {
	filtered := s.getFilteredAsanaProjects()
	if len(filtered) == 0 {
		return
	}

	newIndex := s.AsanaCursorIndex + delta
	if newIndex < 0 {
		newIndex = 0
	}
	if newIndex >= len(filtered) {
		newIndex = len(filtered) - 1
	}

	s.AsanaCursorIndex = newIndex

	// Adjust scroll offset
	if s.AsanaCursorIndex < s.AsanaScrollOffset {
		s.AsanaScrollOffset = s.AsanaCursorIndex
	}
	if s.AsanaCursorIndex >= s.AsanaScrollOffset+AsanaProjectMaxVisible {
		s.AsanaScrollOffset = s.AsanaCursorIndex - AsanaProjectMaxVisible + 1
	}
}

// updateInputFocus manages focus state for text inputs based on current Focus index.
func (s *SettingsState) updateInputFocus() {
	// Blur all first
	s.BranchPrefixInput.Blur()
	s.ContainerImageInput.Blur()
	s.AsanaSearchInput.Blur()

	// Focus the active one
	switch {
	case s.Focus == 1:
		s.BranchPrefixInput.Focus()
	case s.ContainersSupported && s.Focus == s.containerImageFocusIndex():
		s.ContainerImageInput.Focus()
	case s.AsanaPATSet && s.Focus == s.asanaFocusIndex():
		s.AsanaSearchInput.Focus()
	}
}

// IsAsanaFocused returns true when the Asana project selector is focused.
func (s *SettingsState) IsAsanaFocused() bool {
	return s.AsanaPATSet && s.Focus == s.asanaFocusIndex()
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

// GetAsanaProject returns the Asana project GID for the currently selected repo.
func (s *SettingsState) GetAsanaProject() string {
	repo := s.selectedRepoPath()
	return s.AsanaSelectedGIDs[repo]
}

// GetContainerImage returns the container image name, or empty string if unchanged/empty.
func (s *SettingsState) GetContainerImage() string {
	return strings.TrimSpace(s.ContainerImageInput.Value())
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

// GetAllAsanaProjects returns a copy of all per-repo Asana project GIDs.
func (s *SettingsState) GetAllAsanaProjects() map[string]string {
	s.flushCurrentToMaps()
	result := make(map[string]string, len(s.AsanaSelectedGIDs))
	for k, v := range s.AsanaSelectedGIDs {
		result[k] = v
	}
	return result
}

// SetAsanaProjects populates the project options and clears the loading state.
func (s *SettingsState) SetAsanaProjects(options []AsanaProjectOption) {
	s.AsanaProjectOptions = options
	s.AsanaLoading = false
	s.AsanaLoadError = ""
	s.AsanaCursorIndex = 0
	s.AsanaScrollOffset = 0
}

// SetAsanaProjectsError sets the error state and clears loading.
func (s *SettingsState) SetAsanaProjectsError(errMsg string) {
	s.AsanaLoading = false
	s.AsanaLoadError = errMsg
}

// NewSettingsState creates a new SettingsState with the current settings values.
func NewSettingsState(themes []string, themeDisplayNames []string, currentTheme string,
	currentBranchPrefix string, notificationsEnabled bool, repos []string,
	asanaProjects map[string]string,
	defaultRepoIndex int, asanaPATSet bool,
	containersSupported bool, containerImage string) *SettingsState {

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
	prefixInput.SetWidth(ModalWidthWide - 10) // Will be updated by SetSize()
	prefixInput.SetValue(currentBranchPrefix)

	containerImageInput := textinput.New()
	containerImageInput.Placeholder = "ghcr.io/zhubert/plural-claude"
	containerImageInput.CharLimit = 200
	containerImageInput.SetWidth(ModalWidthWide - 10) // Will be updated by SetSize()
	containerImageInput.SetValue(containerImage)

	// Clamp default repo index
	if defaultRepoIndex < 0 || (len(repos) > 0 && defaultRepoIndex >= len(repos)) {
		defaultRepoIndex = 0
	}

	// Copy map to avoid mutating caller's data
	ap := make(map[string]string, len(asanaProjects))
	for k, v := range asanaProjects {
		ap[k] = v
	}

	searchInput := textinput.New()
	searchInput.Placeholder = "Type to filter projects..."
	searchInput.CharLimit = 100
	searchInput.SetWidth(ModalWidthWide - 14) // Will be updated by SetSize()

	return &SettingsState{
		Themes:               themes,
		ThemeDisplayNames:    themeDisplayNames,
		SelectedThemeIndex:   selectedThemeIndex,
		OriginalTheme:        currentTheme,
		BranchPrefixInput:    prefixInput,
		NotificationsEnabled: notificationsEnabled,
		ContainerImageInput:  containerImageInput,
		ContainersSupported:  containersSupported,
		AsanaPATSet:          asanaPATSet,
		Focus:                0,
		Repos:                repos,
		SelectedRepoIndex:    defaultRepoIndex,
		AsanaSelectedGIDs:    ap,
		AsanaSearchInput:     searchInput,
		AsanaLoading:         asanaPATSet,
		availableWidth:       ModalWidthWide, // Default, will be updated by SetSize()
	}
}
