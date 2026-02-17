package modals

import (
	"path/filepath"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
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
	selectedTheme         string
	OriginalTheme         string // To detect if theme changed
	branchPrefix          string
	NotificationsEnabled  bool
	AutoCleanupMerged     bool // Auto-cleanup sessions when PR merged/closed
	AutoBroadcastPR       bool // Auto-create PRs when broadcast group completes
	containerImage        string
	ContainersSupported   bool // Whether Docker is available for container mode
	AutoAddressPRComments bool
	autoMaxTurns          string
	autoMaxDuration       string
	issueMaxConcurrent    string

	// MultiSelect bindings
	generalOptions    []string
	autonomousOptions []string

	form *huh.Form

	// Size tracking
	availableWidth int
}

const (
	optionNotifications  = "notifications"
	optionAutoCleanup    = "auto-cleanup"
	optionAutoBroadcast  = "auto-broadcast"
	optionAutoAddressPR  = "auto-address-pr"
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
	s.AutoBroadcastPR = slices.Contains(s.generalOptions, optionAutoBroadcast)
	s.AutoAddressPRComments = slices.Contains(s.autonomousOptions, optionAutoAddressPR)
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

// GetAutoMaxTurns returns the max autonomous turns value.
func (s *SettingsState) GetAutoMaxTurns() string {
	return s.autoMaxTurns
}

// GetAutoMaxDuration returns the max autonomous duration value.
func (s *SettingsState) GetAutoMaxDuration() string {
	return s.autoMaxDuration
}

// GetIssueMaxConcurrent returns the max concurrent auto-sessions value.
func (s *SettingsState) GetIssueMaxConcurrent() string {
	return s.issueMaxConcurrent
}

// GetSelectedTheme returns the selected theme key.
func (s *SettingsState) GetSelectedTheme() string {
	return s.selectedTheme
}

// ThemeChanged returns true if the selected theme differs from the original.
func (s *SettingsState) ThemeChanged() bool {
	return s.selectedTheme != s.OriginalTheme
}

// SetAutoMaxTurns sets the initial value for max autonomous turns.
// Must be called before the form is displayed to the user. Works because
// huh binds via pointer, so mutations to the struct field reflect in the form.
func (s *SettingsState) SetAutoMaxTurns(v string) {
	s.autoMaxTurns = v
}

// SetAutoMaxDuration sets the initial value for max autonomous duration.
// Must be called before the form is displayed to the user. Works because
// huh binds via pointer, so mutations to the struct field reflect in the form.
func (s *SettingsState) SetAutoMaxDuration(v string) {
	s.autoMaxDuration = v
}

// SetIssueMaxConcurrent sets the initial value for max concurrent sessions.
// Must be called before the form is displayed to the user. Works because
// huh binds via pointer, so mutations to the struct field reflect in the form.
func (s *SettingsState) SetIssueMaxConcurrent(v string) {
	s.issueMaxConcurrent = v
}

// SetBranchPrefix sets the branch prefix value.
// Must be called before the form is displayed to the user. Works because
// huh binds via pointer, so mutations to the struct field reflect in the form.
func (s *SettingsState) SetBranchPrefix(v string) {
	s.branchPrefix = v
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
	if s.numFields() == 0 {
		return "Esc: close"
	}
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
func (s *RepoSettingsState) autoMergeFocusIndex() int    { return 1 }
func (s *RepoSettingsState) issueLabelFocusIndex() int   { return 2 }

func (s *RepoSettingsState) asanaFocusIndex() int {
	if s.ContainersSupported {
		return 3 // after issue polling, auto-merge, issue label
	}
	return 0 // first field if no containers
}

func (s *RepoSettingsState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	parts := []string{title}

	if s.numFields() == 0 {
		noSettings := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			MarginTop(1).
			Render("No per-repo settings available.\nEnable containers or configure Asana to see options here.")
		help := ModalHelpStyle.Render(s.Help())
		parts = append(parts, noSettings, help)
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}

	if s.ContainersSupported {
		autoHeader := renderSectionHeader("Autonomous:")

		issuePollingView := renderCheckboxField(
			"Issue polling",
			"Auto-poll for new issues and create autonomous supervisor sessions",
			s.IssuePolling, s.issuePollingFocusIndex(), s.Focus)
		autoMergeView := renderCheckboxField(
			"Auto-merge after CI",
			"Auto-merge PR when CI passes",
			s.AutoMerge, s.autoMergeFocusIndex(), s.Focus)
		issueLabelView := renderInputField(
			"Issue filter label", "",
			s.IssueLabelInput, s.issueLabelFocusIndex(), s.Focus, s.contentWidth())

		parts = append(parts, autoHeader, issuePollingView, autoMergeView, issueLabelView)
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
	issueLabelInput.Placeholder = "e.g., queued (leave empty for all issues)"
	issueLabelInput.CharLimit = 100
	issueLabelInput.SetWidth(ModalWidthWide - 10)
	issueLabelInput.SetValue(issueLabel)

	searchInput := textinput.New()
	searchInput.Placeholder = "Type to filter projects..."
	searchInput.CharLimit = 100
	searchInput.SetWidth(ModalWidthWide - 14)

	initialFocus := 0

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
	containersSupported bool, containerImage string,
	autoCleanupMerged bool, autoBroadcastPR bool, autoAddressPRComments bool) *SettingsState {

	s := &SettingsState{
		selectedTheme:         currentTheme,
		OriginalTheme:         currentTheme,
		branchPrefix:          currentBranchPrefix,
		NotificationsEnabled:  notificationsEnabled,
		AutoCleanupMerged:     autoCleanupMerged,
		AutoBroadcastPR:       autoBroadcastPR,
		AutoAddressPRComments: autoAddressPRComments,
		containerImage:        containerImage,
		ContainersSupported:   containersSupported,
		availableWidth:        ModalWidthWide,
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
		huh.NewOption("Auto-create broadcast PRs", optionAutoBroadcast).
			Selected(autoBroadcastPR),
	}
	// Initialize the enabledOptions slice to match
	if notificationsEnabled {
		s.generalOptions = append(s.generalOptions, optionNotifications)
	}
	if autoCleanupMerged {
		s.generalOptions = append(s.generalOptions, optionAutoCleanup)
	}
	if autoBroadcastPR {
		s.generalOptions = append(s.generalOptions, optionAutoBroadcast)
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

	// Build autonomous options MultiSelect
	autoOpts := []huh.Option[string]{
		huh.NewOption("Auto-address PR comments", optionAutoAddressPR).
			Selected(autoAddressPRComments),
	}
	if autoAddressPRComments {
		s.autonomousOptions = append(s.autonomousOptions, optionAutoAddressPR)
	}

	// Autonomous settings group (conditionally shown)
	autonomousGroup := huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Autonomous options").
			Options(autoOpts...).
			Height(len(autoOpts)).
			Value(&s.autonomousOptions),
		huh.NewInput().
			Title("Max autonomous turns").
			Placeholder("50").
			CharLimit(5).
			Value(&s.autoMaxTurns),
		huh.NewInput().
			Title("Max autonomous duration (min)").
			Placeholder("30").
			CharLimit(5).
			Value(&s.autoMaxDuration),
		huh.NewInput().
			Title("Max concurrent auto-sessions").
			Placeholder("3").
			CharLimit(3).
			Value(&s.issueMaxConcurrent),
	).WithHideFunc(func() bool { return !containersSupported })

	s.form = huh.NewForm(generalGroup, containerGroup, autonomousGroup).
		WithTheme(ModalTheme()).
		WithShowHelp(false).
		WithWidth(s.contentWidth()).
		WithLayout(huh.LayoutStack)

	initHuhForm(s.form)
	return s
}
