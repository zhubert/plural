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

// NewSessionMaxVisibleRepos is the maximum number of repos visible before scrolling
const NewSessionMaxVisibleRepos = 10

// ContainerAuthHelp is the user-facing message explaining how to set up auth for container mode.
const ContainerAuthHelp = "Set ANTHROPIC_API_KEY env var, run 'claude setup-token', or add 'anthropic_api_key' to macOS keychain"

// =============================================================================
// NewSessionState - State for the New Session modal
// =============================================================================

type NewSessionState struct {
	RepoOptions            []string
	RepoIndex              int
	ScrollOffset           int      // For scrolling the repo list
	LockedRepo             string   // When set, skip repo selector and use this repo
	BaseOptions            []string // Options for base branch selection
	BaseIndex              int      // Selected base option index
	BranchInput            textinput.Model
	UseContainers          bool // Whether to run this session in a container
	ContainersSupported    bool // Whether Docker is available for container mode
	ContainerAuthAvailable bool // Whether API key credentials are available for container mode
	Autonomous             bool // Whether to run in autonomous mode (auto-enables containers)
	Focus                  int  // 0=repo list, 1=base selection, 2=autonomous (if supported), 3=branch input, 4=containers (if supported)
}

func (*NewSessionState) modalState() {}

func (s *NewSessionState) Title() string {
	if s.LockedRepo != "" {
		return "New Session in " + filepath.Base(s.LockedRepo)
	}
	return "New Session"
}

func (s *NewSessionState) Help() string {
	if s.LockedRepo == "" {
		if s.Focus == 0 && len(s.RepoOptions) == 0 {
			return "a: add repo  Esc: cancel"
		}
		if s.Focus == 0 && len(s.RepoOptions) > 0 {
			return "up/down: select  Tab: next field  a: add repo  d: delete repo  Enter: create"
		}
	}
	if s.ContainersSupported && (s.Focus == 2 || s.Focus == 4) {
		return "Space: toggle  Tab: next field  Enter: create"
	}
	return "up/down: select  Tab: next field  Enter: create"
}

func (s *NewSessionState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	var parts []string
	parts = append(parts, title)

	// Repository selection section (hidden when repo is locked)
	if s.LockedRepo == "" {
		repoLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Render("Repository:")

		var repoList string
		if len(s.RepoOptions) == 0 {
			repoList = lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Italic(true).
				Render("No repositories added. Press 'a' to add one.")
		} else {
			repoList = s.renderRepoList()
		}
		parts = append(parts, repoLabel, repoList)
	}

	// Base branch selection section
	baseLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginTop(1).
		Render("Base branch:")

	baseList := RenderSelectableListWithFocus(s.BaseOptions, s.BaseIndex, s.Focus == 1, "> ")

	parts = append(parts, baseLabel, baseList)

	// Autonomous mode checkbox (focus 2, only when containers supported)
	if s.ContainersSupported {
		autoLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			MarginTop(1).
			Render("Autonomous mode:")

		autoCheckbox := "[ ]"
		if s.Autonomous {
			autoCheckbox = "[x]"
		}
		autoCheckboxStyle := lipgloss.NewStyle()
		if s.Focus == 2 {
			autoCheckboxStyle = autoCheckboxStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
		} else {
			autoCheckboxStyle = autoCheckboxStyle.PaddingLeft(2)
		}
		autoDesc := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Width(50).
			Render("Orchestrator: delegates to children, can create PRs")
		autoView := autoCheckboxStyle.Render(autoCheckbox + " " + autoDesc)

		parts = append(parts, autoLabel, autoView)
	}

	// Branch name input section (focus 3 with containers, focus 2 without)
	branchLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginTop(1).
		Render("Branch name:")

	var branchView string
	if s.Autonomous {
		branchView = lipgloss.NewStyle().PaddingLeft(2).Italic(true).Foreground(ColorTextMuted).
			Render("(disabled in autonomous mode)")
	} else {
		branchInputStyle := lipgloss.NewStyle()
		if s.Focus == s.branchFocusIdx() {
			branchInputStyle = branchInputStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
		} else {
			branchInputStyle = branchInputStyle.PaddingLeft(2)
		}
		branchView = branchInputStyle.Render(s.BranchInput.View())
	}

	parts = append(parts, branchLabel, branchView)

	// Container mode checkbox (focus 4, only when containers supported)
	if s.ContainersSupported {
		containerLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			MarginTop(1).
			Render("Run in container:")

		containerCheckbox := "[ ]"
		if s.UseContainers {
			containerCheckbox = "[x]"
		}
		var containerView string
		if s.Autonomous {
			containerDesc := lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Italic(true).
				Width(50).
				Render("Run in a sandboxed container (enabled by autonomous mode)")
			containerView = lipgloss.NewStyle().PaddingLeft(2).Foreground(ColorTextMuted).
				Render(containerCheckbox + " " + containerDesc)
		} else {
			containerCheckboxStyle := lipgloss.NewStyle()
			if s.Focus == 4 {
				containerCheckboxStyle = containerCheckboxStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
			} else {
				containerCheckboxStyle = containerCheckboxStyle.PaddingLeft(2)
			}
			containerDesc := lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Italic(true).
				Width(50).
				Render("Run in a sandboxed container (no permission prompts)")
			containerView = containerCheckboxStyle.Render(containerCheckbox + " " + containerDesc)
		}

		containerWarning := lipgloss.NewStyle().
			Foreground(ColorWarning).
			Italic(true).
			Width(50).
			PaddingLeft(2).
			Render("Warning: Containers provide defense in depth but are not a complete security boundary.")

		parts = append(parts, containerLabel, containerView, containerWarning)

		if s.UseContainers && !s.ContainerAuthAvailable {
			authWarning := lipgloss.NewStyle().
				Foreground(ColorWarning).
				Bold(true).
				Width(55).
				PaddingLeft(2).
				Render(ContainerAuthHelp)
			parts = append(parts, authWarning)
		}
	} else {
		dockerHint := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			MarginTop(1).
			PaddingLeft(2).
			Render("Install Docker to enable container and autonomous modes")
		parts = append(parts, dockerHint)
	}

	help := ModalHelpStyle.Render(s.Help())
	parts = append(parts, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (s *NewSessionState) renderRepoList() string {
	var lines []string

	// Calculate visible range
	startIdx := s.ScrollOffset
	endIdx := startIdx + NewSessionMaxVisibleRepos
	if endIdx > len(s.RepoOptions) {
		endIdx = len(s.RepoOptions)
	}

	// Show scroll indicator at top if needed
	if startIdx > 0 {
		lines = append(lines, lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Render("  ... "+formatCount(startIdx, 0)+" more above"))
	}

	for i := startIdx; i < endIdx; i++ {
		style := SidebarItemStyle
		prefix := "  "
		if i == s.RepoIndex && s.Focus == 0 {
			style = SidebarSelectedStyle
			prefix = "> "
		} else if i == s.RepoIndex {
			prefix = "* "
		}

		lines = append(lines, style.Render(prefix+s.RepoOptions[i]))
	}

	// Show scroll indicator at bottom if needed
	if endIdx < len(s.RepoOptions) {
		remaining := len(s.RepoOptions) - endIdx
		lines = append(lines, lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Render("  ... "+formatCount(remaining, 0)+" more below"))
	}

	return strings.Join(lines, "\n")
}

// numFields returns the number of focusable fields.
// With containers:    0=repo, 1=base, 2=autonomous, 3=branch, 4=containers
// Without containers: 0=repo, 1=base, 2=branch
// When LockedRepo is set, focus 0 is skipped. When autonomous, focus 3+4 are skipped.
func (s *NewSessionState) numFields() int {
	if s.ContainersSupported {
		return 5
	}
	return 3
}

// branchFocusIdx returns the focus index for the branch input field.
func (s *NewSessionState) branchFocusIdx() int {
	if s.ContainersSupported {
		return 3
	}
	return 2
}

// isSkippedFocus returns true if this focus index should be skipped.
func (s *NewSessionState) isSkippedFocus(idx int) bool {
	if s.LockedRepo != "" && idx == 0 {
		return true
	}
	// When autonomous, skip branch and container (they're auto-set)
	if s.Autonomous && s.ContainersSupported && (idx == 3 || idx == 4) {
		return true
	}
	return false
}

// advanceFocus moves focus forward, skipping disabled fields.
func (s *NewSessionState) advanceFocus(delta int) {
	n := s.numFields()
	for i := 0; i < n; i++ {
		s.Focus = (s.Focus + delta + n) % n
		if !s.isSkippedFocus(s.Focus) {
			return
		}
	}
}

func (s *NewSessionState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case keys.Up, "k":
			switch s.Focus {
			case 0: // Repo list
				if s.RepoIndex > 0 {
					s.RepoIndex--
					if s.RepoIndex < s.ScrollOffset {
						s.ScrollOffset = s.RepoIndex
					}
				}
			case 1: // Base selection
				if s.BaseIndex > 0 {
					s.BaseIndex--
				}
			}
		case keys.Down, "j":
			switch s.Focus {
			case 0: // Repo list
				if s.RepoIndex < len(s.RepoOptions)-1 {
					s.RepoIndex++
					if s.RepoIndex >= s.ScrollOffset+NewSessionMaxVisibleRepos {
						s.ScrollOffset = s.RepoIndex - NewSessionMaxVisibleRepos + 1
					}
				}
			case 1: // Base selection
				if s.BaseIndex < len(s.BaseOptions)-1 {
					s.BaseIndex++
				}
			}
		case keys.Tab:
			oldFocus := s.Focus
			s.advanceFocus(1)
			s.updateInputFocus(oldFocus)
			return s, nil
		case keys.ShiftTab:
			oldFocus := s.Focus
			s.advanceFocus(-1)
			s.updateInputFocus(oldFocus)
			return s, nil
		case keys.Space:
			if s.Focus == 2 && s.ContainersSupported {
				s.Autonomous = !s.Autonomous
				// Autonomous requires containers
				if s.Autonomous {
					s.UseContainers = true
					// Clear branch input since it's not used in autonomous mode
					s.BranchInput.SetValue("")
					s.BranchInput.Blur()
				}
			} else if s.Focus == 4 && s.ContainersSupported && !s.Autonomous {
				s.UseContainers = !s.UseContainers
				// If disabling containers, also disable autonomous
				if !s.UseContainers {
					s.Autonomous = false
				}
			}
			return s, nil
		}
	}

	// Handle branch input updates when focused (disabled in autonomous mode)
	if s.Focus == s.branchFocusIdx() && !s.Autonomous {
		var cmd tea.Cmd
		s.BranchInput, cmd = s.BranchInput.Update(msg)
		return s, cmd
	}

	return s, nil
}

// updateInputFocus manages focus state for text inputs based on current Focus index.
func (s *NewSessionState) updateInputFocus(oldFocus int) {
	bfi := s.branchFocusIdx()
	if s.Focus == bfi {
		s.BranchInput.Focus()
	} else if oldFocus == bfi {
		s.BranchInput.Blur()
	}
}

// GetSelectedRepo returns the selected repository path
func (s *NewSessionState) GetSelectedRepo() string {
	if s.LockedRepo != "" {
		return s.LockedRepo
	}
	if len(s.RepoOptions) == 0 || s.RepoIndex >= len(s.RepoOptions) {
		return ""
	}
	return s.RepoOptions[s.RepoIndex]
}

// GetBranchName returns the custom branch name
func (s *NewSessionState) GetBranchName() string {
	return s.BranchInput.Value()
}

// GetBaseIndex returns the selected base option index
func (s *NewSessionState) GetBaseIndex() int {
	return s.BaseIndex
}

// GetUseContainers returns whether container mode is selected
func (s *NewSessionState) GetUseContainers() bool {
	return s.UseContainers
}

// GetAutonomous returns whether autonomous mode is selected
func (s *NewSessionState) GetAutonomous() bool {
	return s.Autonomous
}

// NewNewSessionState creates a new NewSessionState with proper initialization.
// containersSupported indicates whether the host supports Apple containers (darwin/arm64).
// containerAuthAvailable indicates whether API key credentials exist for container mode.
func NewNewSessionState(repos []string, containersSupported bool, containerAuthAvailable bool) *NewSessionState {
	branchInput := textinput.New()
	branchInput.Placeholder = "optional branch name (leave empty for auto)"
	branchInput.CharLimit = BranchNameCharLimit
	branchInput.SetWidth(ModalInputWidth)

	return &NewSessionState{
		RepoOptions:  repos,
		RepoIndex:    0,
		ScrollOffset: 0,
		BaseOptions: []string{
			"From current branch",
			"From local default branch",
			"From remote default branch (latest)",
		},
		BaseIndex:              0,
		BranchInput:            branchInput,
		ContainersSupported:    containersSupported,
		ContainerAuthAvailable: containerAuthAvailable,
		Focus:                  0,
	}
}

// =============================================================================
// ForkSessionState - State for the Fork Session modal
// =============================================================================

type ForkSessionState struct {
	ParentSessionName      string
	ParentSessionID        string
	RepoPath               string
	CopyMessages           bool   // Whether to copy conversation history
	UseContainers          bool   // Whether to run this session in a container
	ContainersSupported    bool   // Whether Docker is available for container mode
	ContainerAuthAvailable bool   // Whether API key credentials are available for container mode
	branchName             string // Bound form value
	enabledOptions         []string // MultiSelect binding

	form *huh.Form
}

const (
	optionCopyMessages  = "copy-messages"
	optionUseContainers = "use-containers"
)

func (*ForkSessionState) modalState() {}

func (s *ForkSessionState) Title() string { return "Fork Session" }

func (s *ForkSessionState) Help() string {
	return "Tab: next field  Enter: create fork  Esc: cancel"
}

func (s *ForkSessionState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Parent session info
	parentLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Forking from:")

	parentName := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true).
		MarginBottom(1).
		Render("  " + s.ParentSessionName)

	parts := []string{title, parentLabel, parentName, s.form.View()}

	if s.UseContainers && !s.ContainerAuthAvailable {
		authWarning := lipgloss.NewStyle().
			Foreground(ColorWarning).
			Bold(true).
			Width(55).
			PaddingLeft(2).
			Render(ContainerAuthHelp)
		parts = append(parts, authWarning)
	}

	if !s.ContainersSupported {
		dockerHint := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			MarginTop(1).
			PaddingLeft(2).
			Render("Install Docker to enable container and autonomous modes")
		parts = append(parts, dockerHint)
	}

	help := ModalHelpStyle.Render(s.Help())
	parts = append(parts, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (s *ForkSessionState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	var cmd tea.Cmd
	s.form, cmd = huhFormUpdate(s.form, msg)
	s.syncFromMultiSelect()
	return s, cmd
}

// syncFromMultiSelect updates boolean fields from the MultiSelect binding.
func (s *ForkSessionState) syncFromMultiSelect() {
	s.CopyMessages = slices.Contains(s.enabledOptions, optionCopyMessages)
	s.UseContainers = slices.Contains(s.enabledOptions, optionUseContainers)
}

// GetBranchName returns the custom branch name
func (s *ForkSessionState) GetBranchName() string {
	return s.branchName
}

// ShouldCopyMessages returns whether to copy conversation history
func (s *ForkSessionState) ShouldCopyMessages() bool {
	return s.CopyMessages
}

// GetUseContainers returns whether container mode is selected
func (s *ForkSessionState) GetUseContainers() bool {
	return s.UseContainers
}

// SetBranchName sets the branch name value (for testing).
func (s *ForkSessionState) SetBranchName(name string) {
	s.branchName = name
}

// NewForkSessionState creates a new ForkSessionState.
// parentContainerized is the parent session's container status (used as default for the checkbox).
// containersSupported indicates whether the host supports Apple containers (darwin/arm64).
// containerAuthAvailable indicates whether API key credentials exist for container mode.
func NewForkSessionState(parentSessionName, parentSessionID, repoPath string, parentContainerized bool, containersSupported bool, containerAuthAvailable bool) *ForkSessionState {
	s := &ForkSessionState{
		ParentSessionName:      parentSessionName,
		ParentSessionID:        parentSessionID,
		RepoPath:               repoPath,
		CopyMessages:           true, // Default to copying messages
		UseContainers:          parentContainerized,
		ContainersSupported:    containersSupported,
		ContainerAuthAvailable: containerAuthAvailable,
	}

	// Build MultiSelect options
	options := []huh.Option[string]{
		huh.NewOption("Copy conversation history", optionCopyMessages).
			Selected(true), // Default to copying
	}
	s.enabledOptions = append(s.enabledOptions, optionCopyMessages)

	if containersSupported {
		opt := huh.NewOption("Run in container — sandboxed (no permission prompts)", optionUseContainers).
			Selected(parentContainerized)
		options = append(options, opt)
		if parentContainerized {
			s.enabledOptions = append(s.enabledOptions, optionUseContainers)
		}
	}

	s.form = huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Options").
				Options(options...).
				Height(len(options)).
				Value(&s.enabledOptions),
			huh.NewInput().
				Title("Branch name").
				Placeholder("optional (leave empty for auto)").
				CharLimit(BranchNameCharLimit).
				Value(&s.branchName),
		),
	).WithTheme(ModalTheme()).
		WithShowHelp(false).
		WithWidth(ModalInputWidth)

	initHuhForm(s.form)
	return s
}

// =============================================================================
// RenameSessionState - State for the Rename Session modal
// =============================================================================

type RenameSessionState struct {
	SessionID   string
	SessionName string

	form    *huh.Form
	newName string
}

func (*RenameSessionState) modalState() {}

func (s *RenameSessionState) Title() string { return "Rename Session" }

func (s *RenameSessionState) Help() string {
	return "Enter: save  Esc: cancel"
}

func (s *RenameSessionState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Current name info
	currentLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Current name:")

	currentName := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true).
		MarginBottom(1).
		Render("  " + s.SessionName)

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		currentLabel,
		currentName,
		s.form.View(),
		help,
	)
}

func (s *RenameSessionState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	var cmd tea.Cmd
	s.form, cmd = huhFormUpdate(s.form, msg)
	return s, cmd
}

// GetNewName returns the new name entered by the user
func (s *RenameSessionState) GetNewName() string {
	return s.newName
}

// SetNewName sets the new name value (for testing).
func (s *RenameSessionState) SetNewName(name string) {
	s.newName = name
}

// NewRenameSessionState creates a new RenameSessionState
func NewRenameSessionState(sessionID, currentName string) *RenameSessionState {
	s := &RenameSessionState{
		SessionID:   sessionID,
		SessionName: currentName,
		newName:     currentName,
	}

	s.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("New name").
				Placeholder("enter new name").
				CharLimit(SessionNameCharLimit).
				Value(&s.newName),
		),
	).WithTheme(ModalTheme()).
		WithShowHelp(false).
		WithWidth(ModalInputWidth)

	initHuhForm(s.form)
	return s
}

// =============================================================================
// SessionSettingsState - State for the per-session Settings modal
// =============================================================================

// SessionSettingsState holds per-session settings shown when a session is selected in the sidebar.
type SessionSettingsState struct {
	SessionID   string
	SessionName string
	Branch      string
	BaseBranch  string

	// Read-only info
	Containerized bool

	// Bound form values
	name           string
	Autonomous     bool
	enabledOptions []string // MultiSelect binding

	form *huh.Form
}

const optionAutonomous = "autonomous"

func (*SessionSettingsState) modalState() {}

func (s *SessionSettingsState) Title() string {
	return "Session Settings"
}

func (s *SessionSettingsState) Help() string {
	return "Tab: next field  Enter: save  Esc: cancel"
}

func (s *SessionSettingsState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Info section
	infoHeader := renderSectionHeader("Info:")

	branchLabel := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Branch: ")
	branchValue := lipgloss.NewStyle().Foreground(ColorSecondary).Render(s.Branch)
	branchLine := lipgloss.NewStyle().PaddingLeft(2).Render(branchLabel + branchValue)

	baseLabel := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Base: ")
	baseValue := lipgloss.NewStyle().Foreground(ColorSecondary).Render(s.BaseBranch)
	baseLine := lipgloss.NewStyle().PaddingLeft(2).Render(baseLabel + baseValue)

	containerLabel := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Container: ")
	containerValue := "no"
	if s.Containerized {
		containerValue = "yes"
	}
	containerLine := lipgloss.NewStyle().PaddingLeft(2).Render(
		containerLabel + lipgloss.NewStyle().Foreground(ColorSecondary).Render(containerValue),
	)

	// Editable fields via huh form
	editHeader := renderSectionHeader("Settings:")

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		infoHeader,
		branchLine,
		baseLine,
		containerLine,
		editHeader,
		s.form.View(),
		help,
	)
}

func (s *SessionSettingsState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	var cmd tea.Cmd
	s.form, cmd = huhFormUpdate(s.form, msg)
	s.syncFromMultiSelect()
	return s, cmd
}

// syncFromMultiSelect updates boolean fields from the MultiSelect binding.
func (s *SessionSettingsState) syncFromMultiSelect() {
	s.Autonomous = slices.Contains(s.enabledOptions, optionAutonomous)
}

// GetNewName returns the new name entered by the user.
func (s *SessionSettingsState) GetNewName() string {
	return s.name
}

// NewSessionSettingsState creates a new SessionSettingsState.
func NewSessionSettingsState(sessionID, currentName, branch, baseBranch string, autonomous, containerized bool) *SessionSettingsState {
	s := &SessionSettingsState{
		SessionID:     sessionID,
		SessionName:   currentName,
		Branch:        branch,
		BaseBranch:    baseBranch,
		name:          currentName,
		Autonomous:    autonomous,
		Containerized: containerized,
	}

	if autonomous {
		s.enabledOptions = append(s.enabledOptions, optionAutonomous)
	}

	autonomousOpt := huh.NewOption("Autonomous — run without user prompts", optionAutonomous).
		Selected(autonomous)

	s.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Name").
				Placeholder("enter session name").
				CharLimit(SessionNameCharLimit).
				Value(&s.name),
			huh.NewMultiSelect[string]().
				Title("Options").
				Options(autonomousOpt).
				Height(1).
				Value(&s.enabledOptions),
		),
	).WithTheme(ModalTheme()).
		WithShowHelp(false).
		WithWidth(ModalInputWidth)

	initHuhForm(s.form)
	return s
}
