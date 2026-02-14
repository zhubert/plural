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
	AutoCleanupMerged    bool // Auto-cleanup sessions when PR merged/closed
	AutoBroadcastPR      bool // Auto-create PRs when broadcast group completes

	// Container image (only shown when ContainersSupported)
	ContainerImageInput textinput.Model
	ContainersSupported bool // Whether Docker is available for container mode

	// Autonomous settings (only shown when ContainersSupported)
	AutoAddressPRComments    bool
	AutoMaxTurnsInput        textinput.Model
	AutoMaxDurationInput     textinput.Model
	IssueMaxConcurrentInput  textinput.Model

	Focus int // 0=theme, 1=branch prefix, 2=notifications, 3=auto-cleanup, 4=auto-broadcast, [5+=container fields]

	// Size tracking
	availableWidth int // Actual width available after modal is clamped to screen
}

func (*SettingsState) modalState() {}

func (s *SettingsState) PreferredWidth() int { return ModalWidthWide }

// SetSize updates the available width for rendering content.
// Called by the modal container before Render() to notify the modal of its actual size.
func (s *SettingsState) SetSize(width, height int) {
	s.availableWidth = width
	contentWidth := s.contentWidth()
	s.BranchPrefixInput.SetWidth(contentWidth)
	s.ContainerImageInput.SetWidth(contentWidth)
	s.AutoMaxTurnsInput.SetWidth(contentWidth)
	s.AutoMaxDurationInput.SetWidth(contentWidth)
	s.IssueMaxConcurrentInput.SetWidth(contentWidth)
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
	return "Tab: next field  Space: toggle  Enter: save  Esc: cancel"
}

func (s *SettingsState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// --- General section ---
	generalHeader := renderSectionHeader("General")

	themeLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Theme:")
	themeName := ""
	if s.SelectedThemeIndex < len(s.ThemeDisplayNames) {
		themeName = s.ThemeDisplayNames[s.SelectedThemeIndex]
	}
	themeView := s.renderSelectorField(themeName, s.SelectedThemeIndex, len(s.Themes), 0)

	prefixView := renderInputField(
		"Default branch prefix",
		"Applied to all new branches (e.g., \"zhubert/\" creates branches like \"zhubert/plural-...\")",
		s.BranchPrefixInput, 1, s.Focus, s.contentWidth())

	notifView := renderCheckboxField(
		"Desktop notifications",
		"Notify when Claude finishes while app is in background",
		s.NotificationsEnabled, 2, s.Focus)

	cleanupView := renderCheckboxField(
		"Auto-cleanup merged sessions",
		"Automatically delete sessions when their PR is merged or closed",
		s.AutoCleanupMerged, 3, s.Focus)

	broadcastPRView := renderCheckboxField(
		"Auto-create broadcast PRs",
		"Auto-create PRs when all broadcast group sessions complete",
		s.AutoBroadcastPR, 4, s.Focus)

	parts := []string{title, generalHeader, themeLabel, themeView, prefixView, notifView, cleanupView, broadcastPRView}

	// Container image field (only on Apple Silicon)
	if s.ContainersSupported {
		containerView := renderInputField(
			"Container image",
			"Image name used for container mode sessions",
			s.ContainerImageInput, s.containerImageFocusIndex(), s.Focus, s.contentWidth())
		parts = append(parts, containerView)
	}

	// --- Autonomous section ---
	if s.ContainersSupported {
		autoHeader := renderSectionHeader("Autonomous:")

		autoAddressView := renderCheckboxField(
			"Auto-address PR comments",
			"Auto-fetch and address new PR review comments",
			s.AutoAddressPRComments, s.autoAddressFocusIndex(), s.Focus)

		maxTurnsView := renderInputField("Max autonomous turns", "",
			s.AutoMaxTurnsInput, s.autoMaxTurnsFocusIndex(), s.Focus, s.contentWidth())
		maxDurationView := renderInputField("Max autonomous duration (min)", "",
			s.AutoMaxDurationInput, s.autoMaxDurationFocusIndex(), s.Focus, s.contentWidth())
		maxConcurrentView := renderInputField("Max concurrent auto-sessions", "",
			s.IssueMaxConcurrentInput, s.issueMaxConcurrentFocusIndex(), s.Focus, s.contentWidth())

		parts = append(parts, autoHeader, autoAddressView, maxTurnsView, maxDurationView, maxConcurrentView)
	}

	help := ModalHelpStyle.Render(s.Help())
	parts = append(parts, help)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// renderSelectorField renders a left/right arrow selector with focus border.
func (s *SettingsState) renderSelectorField(displayName string, index, total, focusIdx int) string {
	style := lipgloss.NewStyle()
	if s.Focus == focusIdx {
		style = style.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
	} else {
		style = style.PaddingLeft(2)
	}
	leftArrow := " "
	rightArrow := " "
	if index > 0 {
		leftArrow = lipgloss.NewStyle().Foreground(ColorPrimary).Render("<")
	}
	if index < total-1 {
		rightArrow = lipgloss.NewStyle().Foreground(ColorPrimary).Render(">")
	}
	display := lipgloss.NewStyle().Foreground(ColorText).Bold(true).Render(displayName)
	return style.Render(leftArrow + " " + display + " " + rightArrow)
}

// numFields returns the number of focusable fields in the settings modal.
func (s *SettingsState) numFields() int {
	base := 5 // theme, branch prefix, notifications, auto-cleanup, auto-broadcast-PR
	if s.ContainersSupported {
		base++ // container image
		base += 4 // auto-address PR, max turns, max duration, max concurrent
	}
	return base
}

// containerImageFocusIndex returns the focus index for the container image field.
// Only meaningful when ContainersSupported is true.
func (s *SettingsState) containerImageFocusIndex() int {
	return 5 // theme=0, prefix=1, notifications=2, auto-cleanup=3, auto-broadcast=4, container image=5
}

// autoAddressFocusIndex returns the focus index for auto-address PR comments checkbox.
func (s *SettingsState) autoAddressFocusIndex() int {
	return 6 // after container image
}

// autoMaxTurnsFocusIndex returns the focus index for max autonomous turns input.
func (s *SettingsState) autoMaxTurnsFocusIndex() int {
	return 7
}

// autoMaxDurationFocusIndex returns the focus index for max autonomous duration input.
func (s *SettingsState) autoMaxDurationFocusIndex() int {
	return 8
}

// issueMaxConcurrentFocusIndex returns the focus index for max concurrent sessions input.
func (s *SettingsState) issueMaxConcurrentFocusIndex() int {
	return 9
}



func (s *SettingsState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	numFields := s.numFields()

	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}

	key := keyMsg.String()

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
		switch {
		case s.Focus == 2:
			s.NotificationsEnabled = !s.NotificationsEnabled
		case s.Focus == 3:
			s.AutoCleanupMerged = !s.AutoCleanupMerged
		case s.Focus == 4:
			s.AutoBroadcastPR = !s.AutoBroadcastPR
		case s.ContainersSupported && s.Focus == s.autoAddressFocusIndex():
			s.AutoAddressPRComments = !s.AutoAddressPRComments
		}
		return s, nil
	case keys.Left, "h":
		if s.Focus == 0 && len(s.Themes) > 0 {
			if s.SelectedThemeIndex > 0 {
				s.SelectedThemeIndex--
			}
			return s, nil
		}
	case keys.Right, "l":
		if s.Focus == 0 && len(s.Themes) > 0 {
			if s.SelectedThemeIndex < len(s.Themes)-1 {
				s.SelectedThemeIndex++
			}
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

	// Handle text input updates for autonomous global fields
	if s.ContainersSupported {
		switch s.Focus {
		case s.autoMaxTurnsFocusIndex():
			var cmd tea.Cmd
			s.AutoMaxTurnsInput, cmd = s.AutoMaxTurnsInput.Update(msg)
			return s, cmd
		case s.autoMaxDurationFocusIndex():
			var cmd tea.Cmd
			s.AutoMaxDurationInput, cmd = s.AutoMaxDurationInput.Update(msg)
			return s, cmd
		case s.issueMaxConcurrentFocusIndex():
			var cmd tea.Cmd
			s.IssueMaxConcurrentInput, cmd = s.IssueMaxConcurrentInput.Update(msg)
			return s, cmd
		}
	}

	return s, nil
}

// updateInputFocus manages focus state for text inputs based on current Focus index.
func (s *SettingsState) updateInputFocus() {
	// Blur all first
	s.BranchPrefixInput.Blur()
	s.ContainerImageInput.Blur()
	s.AutoMaxTurnsInput.Blur()
	s.AutoMaxDurationInput.Blur()
	s.IssueMaxConcurrentInput.Blur()

	// Focus the active one
	switch {
	case s.Focus == 1:
		s.BranchPrefixInput.Focus()
	case s.ContainersSupported && s.Focus == s.containerImageFocusIndex():
		s.ContainerImageInput.Focus()
	case s.ContainersSupported && s.Focus == s.autoMaxTurnsFocusIndex():
		s.AutoMaxTurnsInput.Focus()
	case s.ContainersSupported && s.Focus == s.autoMaxDurationFocusIndex():
		s.AutoMaxDurationInput.Focus()
	case s.ContainersSupported && s.Focus == s.issueMaxConcurrentFocusIndex():
		s.IssueMaxConcurrentInput.Focus()
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

// =============================================================================
// RepoSettingsState - State for the per-repo Settings modal
// =============================================================================

// RepoSettingsState holds per-repo settings shown when a repo is selected in the sidebar.
type RepoSettingsState struct {
	RepoPath string // The repo this settings modal is for
	RepoName string // Display name (basename of path)

	// Per-repo autonomous settings (only shown when ContainersSupported)
	ContainersSupported bool
	IssuePolling        bool
	IssueLabelInput     textinput.Model
	AutoMerge           bool

	// Asana project selector
	AsanaPATSet         bool
	AsanaSelectedGID    string               // Selected Asana project GID for this repo
	AsanaProjectOptions []AsanaProjectOption  // All fetched projects
	AsanaSearchInput    textinput.Model
	AsanaCursorIndex    int
	AsanaScrollOffset   int
	AsanaLoading        bool
	AsanaLoadError      string

	Focus          int
	availableWidth int
}

func (*RepoSettingsState) modalState() {}

func (s *RepoSettingsState) PreferredWidth() int { return ModalWidthWide }

func (s *RepoSettingsState) Title() string { return "Repo Settings: " + s.RepoName }

func (s *RepoSettingsState) SetSize(width, height int) {
	s.availableWidth = width
	contentWidth := s.contentWidth()
	s.IssueLabelInput.SetWidth(contentWidth)
	s.AsanaSearchInput.SetWidth(contentWidth - 4)
}

func (s *RepoSettingsState) contentWidth() int {
	if s.availableWidth > 0 {
		return s.availableWidth - 10
	}
	return ModalWidthWide - 10
}

func (s *RepoSettingsState) Help() string {
	if s.Focus == s.asanaFocusIndex() && s.AsanaPATSet {
		return "Tab: next field  Up/Down: navigate  Enter: select  Esc: cancel"
	}
	return "Tab: next field  Space: toggle  Enter: save  Esc: cancel"
}

func (s *RepoSettingsState) numFields() int {
	n := 0
	if s.ContainersSupported {
		n += 3 // issue polling, issue label, auto-merge
	}
	if s.AsanaPATSet {
		n++ // asana
	}
	return n
}

// Focus indices for repo settings fields
func (s *RepoSettingsState) issuePollingFocusIndex() int { return 0 }
func (s *RepoSettingsState) issueLabelFocusIndex() int   { return 1 }
func (s *RepoSettingsState) autoMergeFocusIndex() int    { return 2 }

func (s *RepoSettingsState) asanaFocusIndex() int {
	if s.ContainersSupported {
		return 3 // after issue polling, issue label, auto-merge
	}
	return 0 // first field if no containers
}

func (s *RepoSettingsState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	parts := []string{title}

	if s.ContainersSupported {
		autoHeader := renderSectionHeader("Autonomous:")

		issuePollingView := renderCheckboxField(
			"Issue polling",
			"Auto-poll for new issues and create sessions",
			s.IssuePolling, s.issuePollingFocusIndex(), s.Focus)
		issueLabelView := renderInputField(
			"Issue filter label", "",
			s.IssueLabelInput, s.issueLabelFocusIndex(), s.Focus, s.contentWidth())
		autoMergeView := renderCheckboxField(
			"Auto-merge after CI",
			"Auto-merge PR when CI passes",
			s.AutoMerge, s.autoMergeFocusIndex(), s.Focus)

		parts = append(parts, autoHeader, issuePollingView, issueLabelView, autoMergeView)
	}

	// Asana project selector
	if s.AsanaPATSet {
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
		parts = append(parts, asanaLabel+"\n"+asanaDesc+"\n"+asanaView)
	}

	help := ModalHelpStyle.Render(s.Help())
	parts = append(parts, help)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (s *RepoSettingsState) renderAsanaSelector() string {
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
	currentLabel := "(none)"
	for _, opt := range s.AsanaProjectOptions {
		if opt.GID == s.AsanaSelectedGID {
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

func (s *RepoSettingsState) getFilteredAsanaProjects() []AsanaProjectOption {
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

func (s *RepoSettingsState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	numFields := s.numFields()
	if numFields == 0 {
		return s, nil
	}

	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}

	key := keyMsg.String()

	// Asana selector handling
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
			filtered := s.getFilteredAsanaProjects()
			if s.AsanaCursorIndex < len(filtered) {
				s.AsanaSelectedGID = filtered[s.AsanaCursorIndex].GID
			}
			return s, nil
		default:
			var cmd tea.Cmd
			oldQuery := s.AsanaSearchInput.Value()
			s.AsanaSearchInput, cmd = s.AsanaSearchInput.Update(msg)
			if s.AsanaSearchInput.Value() != oldQuery {
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
		if s.ContainersSupported {
			switch s.Focus {
			case s.issuePollingFocusIndex():
				s.IssuePolling = !s.IssuePolling
			case s.autoMergeFocusIndex():
				s.AutoMerge = !s.AutoMerge
			}
		}
		return s, nil
	}

	// Text input for issue label
	if s.ContainersSupported && s.Focus == s.issueLabelFocusIndex() {
		var cmd tea.Cmd
		s.IssueLabelInput, cmd = s.IssueLabelInput.Update(msg)
		return s, cmd
	}

	return s, nil
}

func (s *RepoSettingsState) asanaNavigate(delta int) {
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

	if s.AsanaCursorIndex < s.AsanaScrollOffset {
		s.AsanaScrollOffset = s.AsanaCursorIndex
	}
	if s.AsanaCursorIndex >= s.AsanaScrollOffset+AsanaProjectMaxVisible {
		s.AsanaScrollOffset = s.AsanaCursorIndex - AsanaProjectMaxVisible + 1
	}
}

func (s *RepoSettingsState) updateInputFocus() {
	s.IssueLabelInput.Blur()
	s.AsanaSearchInput.Blur()

	switch {
	case s.ContainersSupported && s.Focus == s.issueLabelFocusIndex():
		s.IssueLabelInput.Focus()
	case s.AsanaPATSet && s.Focus == s.asanaFocusIndex():
		s.AsanaSearchInput.Focus()
	}
}

// IsAsanaFocused returns true when the Asana project selector is focused.
func (s *RepoSettingsState) IsAsanaFocused() bool {
	return s.AsanaPATSet && s.Focus == s.asanaFocusIndex()
}

// GetIssueLabel returns the issue label value.
func (s *RepoSettingsState) GetIssueLabel() string {
	return s.IssueLabelInput.Value()
}

// GetAsanaProject returns the Asana project GID.
func (s *RepoSettingsState) GetAsanaProject() string {
	return s.AsanaSelectedGID
}

// SetAsanaProjects populates the project options and clears the loading state.
func (s *RepoSettingsState) SetAsanaProjects(options []AsanaProjectOption) {
	s.AsanaProjectOptions = options
	s.AsanaLoading = false
	s.AsanaLoadError = ""
	s.AsanaCursorIndex = 0
	s.AsanaScrollOffset = 0
}

// SetAsanaProjectsError sets the error state and clears loading.
func (s *RepoSettingsState) SetAsanaProjectsError(errMsg string) {
	s.AsanaLoading = false
	s.AsanaLoadError = errMsg
}

// NewRepoSettingsState creates a new RepoSettingsState for the given repo.
func NewRepoSettingsState(repoPath string, containersSupported bool, asanaPATSet bool,
	issuePolling bool, issueLabel string, autoMerge bool, asanaGID string) *RepoSettingsState {

	issueLabelInput := textinput.New()
	issueLabelInput.Placeholder = "e.g., plural-auto (leave empty for all issues)"
	issueLabelInput.CharLimit = 100
	issueLabelInput.SetWidth(ModalWidthWide - 10)
	issueLabelInput.SetValue(issueLabel)

	searchInput := textinput.New()
	searchInput.Placeholder = "Type to filter projects..."
	searchInput.CharLimit = 100
	searchInput.SetWidth(ModalWidthWide - 14)

	// Default focus to first available field
	initialFocus := 0
	if !containersSupported && asanaPATSet {
		initialFocus = 0 // asana is the only field
	}

	return &RepoSettingsState{
		RepoPath:            repoPath,
		RepoName:            filepath.Base(repoPath),
		ContainersSupported: containersSupported,
		IssuePolling:        issuePolling,
		IssueLabelInput:     issueLabelInput,
		AutoMerge:           autoMerge,
		AsanaPATSet:         asanaPATSet,
		AsanaSelectedGID:    asanaGID,
		AsanaSearchInput:    searchInput,
		AsanaLoading:        asanaPATSet,
		Focus:               initialFocus,
		availableWidth:      ModalWidthWide,
	}
}

// Shared render helpers for both SettingsState and RepoSettingsState

func renderSectionHeader(title string) string {
	return lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true).
		MarginTop(1).
		Render(title)
}

func renderCheckboxField(label, desc string, checked bool, focusIdx, currentFocus int) string {
	checkbox := "[ ]"
	if checked {
		checkbox = "[x]"
	}

	labelText := lipgloss.NewStyle().Bold(true).Foreground(ColorText).Render(label)
	descText := lipgloss.NewStyle().Foreground(ColorTextMuted).Italic(true).Render(desc)
	content := checkbox + " " + labelText + " — " + descText

	style := lipgloss.NewStyle()
	if currentFocus == focusIdx {
		style = style.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
	} else {
		style = style.PaddingLeft(2)
	}
	return style.Render(content)
}

func renderInputField(label, desc string, input textinput.Model, focusIdx, currentFocus, contentWidth int) string {
	labelText := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(label)
	var headerLine string
	if desc != "" {
		descText := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render(desc)
		headerLine = labelText + " — " + descText
	} else {
		headerLine = labelText
	}

	inputStyle := lipgloss.NewStyle()
	if currentFocus == focusIdx {
		inputStyle = inputStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
	} else {
		inputStyle = inputStyle.PaddingLeft(2)
	}
	return headerLine + "\n" + inputStyle.Render(input.View())
}

// NewSettingsState creates a new SettingsState with the current settings values.
func NewSettingsState(themes []string, themeDisplayNames []string, currentTheme string,
	currentBranchPrefix string, notificationsEnabled bool,
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
	prefixInput.SetWidth(ModalWidthWide - 10)
	prefixInput.SetValue(currentBranchPrefix)

	containerImageInput := textinput.New()
	containerImageInput.Placeholder = "ghcr.io/zhubert/plural-claude"
	containerImageInput.CharLimit = 200
	containerImageInput.SetWidth(ModalWidthWide - 10)
	containerImageInput.SetValue(containerImage)

	autoMaxTurnsInput := textinput.New()
	autoMaxTurnsInput.Placeholder = "50"
	autoMaxTurnsInput.CharLimit = 5
	autoMaxTurnsInput.SetWidth(ModalWidthWide - 10)

	autoMaxDurationInput := textinput.New()
	autoMaxDurationInput.Placeholder = "30"
	autoMaxDurationInput.CharLimit = 5
	autoMaxDurationInput.SetWidth(ModalWidthWide - 10)

	issueMaxConcurrentInput := textinput.New()
	issueMaxConcurrentInput.Placeholder = "3"
	issueMaxConcurrentInput.CharLimit = 3
	issueMaxConcurrentInput.SetWidth(ModalWidthWide - 10)

	return &SettingsState{
		Themes:                  themes,
		ThemeDisplayNames:       themeDisplayNames,
		SelectedThemeIndex:      selectedThemeIndex,
		OriginalTheme:           currentTheme,
		BranchPrefixInput:       prefixInput,
		NotificationsEnabled:    notificationsEnabled,
		ContainerImageInput:     containerImageInput,
		ContainersSupported:     containersSupported,
		AutoMaxTurnsInput:       autoMaxTurnsInput,
		AutoMaxDurationInput:    autoMaxDurationInput,
		IssueMaxConcurrentInput: issueMaxConcurrentInput,
		Focus:                   0,
		availableWidth:          ModalWidthWide,
	}
}
