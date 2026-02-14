package modals

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
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
	BaseOptions            []string // Options for base branch selection
	BaseIndex              int      // Selected base option index
	BranchInput            textinput.Model
	UseContainers          bool // Whether to run this session in a container
	ContainersSupported    bool // Whether Docker is available for container mode
	ContainerAuthAvailable bool // Whether API key credentials are available for container mode
	Autonomous             bool // Whether to run in autonomous mode (auto-enables containers)
	Focus                  int  // 0=repo list, 1=base selection, 2=branch input, 3=containers (if supported), 4=autonomous
}

func (*NewSessionState) modalState() {}

func (s *NewSessionState) Title() string { return "New Session" }

func (s *NewSessionState) Help() string {
	if s.Focus == 0 && len(s.RepoOptions) == 0 {
		return "a: add repo  Esc: cancel"
	}
	if s.Focus == 0 && len(s.RepoOptions) > 0 {
		return "up/down: select  Tab: next field  a: add repo  d: delete repo  Enter: create"
	}
	if s.Focus == 3 && s.ContainersSupported {
		return "Space: toggle  Tab: next field  Enter: create"
	}
	if s.Focus == 4 && s.ContainersSupported {
		return "Space: toggle  Tab: next field  Enter: create"
	}
	return "up/down: select  Tab: next field  Enter: create"
}

func (s *NewSessionState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Repository selection section
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

	// Base branch selection section
	baseLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginTop(1).
		Render("Base branch:")

	baseList := RenderSelectableListWithFocus(s.BaseOptions, s.BaseIndex, s.Focus == 1, "> ")

	// Branch name input section
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
		if s.Focus == 2 {
			branchInputStyle = branchInputStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
		} else {
			branchInputStyle = branchInputStyle.PaddingLeft(2)
		}
		branchView = branchInputStyle.Render(s.BranchInput.View())
	}

	parts := []string{title, repoLabel, repoList, baseLabel, baseList, branchLabel, branchView}

	// Container mode checkbox (only on Apple Silicon)
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
			if s.Focus == 3 {
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

		// Autonomous mode checkbox
		autoLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			MarginTop(1).
			Render("Autonomous mode:")

		autoCheckbox := "[ ]"
		if s.Autonomous {
			autoCheckbox = "[x]"
		}
		autoCheckboxStyle := lipgloss.NewStyle()
		if s.Focus == 4 {
			autoCheckboxStyle = autoCheckboxStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
		} else {
			autoCheckboxStyle = autoCheckboxStyle.PaddingLeft(2)
		}
		autoDesc := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Width(50).
			Render("Auto-respond to questions, no user interaction needed")
		autoView := autoCheckboxStyle.Render(autoCheckbox + " " + autoDesc)

		parts = append(parts, autoLabel, autoView)
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
func (s *NewSessionState) numFields() int {
	if s.ContainersSupported {
		return 5 // repo list, base selection, branch input, containers, autonomous
	}
	return 3 // repo list, base selection, branch input
}

func (s *NewSessionState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	numFields := s.numFields()

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
			s.Focus = (s.Focus + 1) % numFields
			// Skip disabled fields when autonomous is on (with safety bound)
			if s.Autonomous {
				for i := 0; (s.Focus == 2 || s.Focus == 3) && i < numFields; i++ {
					s.Focus = (s.Focus + 1) % numFields
				}
			}
			s.updateInputFocus(oldFocus)
			return s, nil
		case keys.ShiftTab:
			oldFocus := s.Focus
			s.Focus = (s.Focus - 1 + numFields) % numFields
			// Skip disabled fields when autonomous is on (with safety bound)
			if s.Autonomous {
				for i := 0; (s.Focus == 2 || s.Focus == 3) && i < numFields; i++ {
					s.Focus = (s.Focus - 1 + numFields) % numFields
				}
			}
			s.updateInputFocus(oldFocus)
			return s, nil
		case keys.Space:
			if s.Focus == 3 && s.ContainersSupported && !s.Autonomous {
				s.UseContainers = !s.UseContainers
				// If disabling containers, also disable autonomous
				if !s.UseContainers {
					s.Autonomous = false
				}
			} else if s.Focus == 4 && s.ContainersSupported {
				s.Autonomous = !s.Autonomous
				// Autonomous requires containers
				if s.Autonomous {
					s.UseContainers = true
					// Clear branch input since it's not used in autonomous mode
					s.BranchInput.SetValue("")
					s.BranchInput.Blur()
				}
			}
			return s, nil
		}
	}

	// Handle branch input updates when focused (disabled in autonomous mode)
	if s.Focus == 2 && !s.Autonomous {
		var cmd tea.Cmd
		s.BranchInput, cmd = s.BranchInput.Update(msg)
		return s, cmd
	}

	return s, nil
}

// updateInputFocus manages focus state for text inputs based on current Focus index.
func (s *NewSessionState) updateInputFocus(oldFocus int) {
	if s.Focus == 2 {
		s.BranchInput.Focus()
	} else if oldFocus == 2 {
		s.BranchInput.Blur()
	}
}

// GetSelectedRepo returns the selected repository path
func (s *NewSessionState) GetSelectedRepo() string {
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
			"From current local branch",
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
	BranchInput            textinput.Model
	CopyMessages           bool // Whether to copy conversation history
	UseContainers          bool // Whether to run this session in a container
	ContainersSupported    bool // Whether Docker is available for container mode
	ContainerAuthAvailable bool // Whether API key credentials are available for container mode
	Focus                  int  // 0=copy messages toggle, 1=branch input, 2=containers (if supported)
}

func (*ForkSessionState) modalState() {}

func (s *ForkSessionState) Title() string { return "Fork Session" }

func (s *ForkSessionState) Help() string {
	if s.Focus == 2 && s.ContainersSupported {
		return "Space: toggle  Tab: next field  Enter: create fork  Esc: cancel"
	}
	return "Tab: switch field  Space: toggle  Enter: create fork  Esc: cancel"
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

	// Copy messages toggle
	copyLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginTop(1).
		Render("Copy conversation history:")

	copyStyle := SidebarItemStyle
	copyPrefix := "  "
	if s.Focus == 0 {
		copyStyle = SidebarSelectedStyle
		copyPrefix = "> "
	}
	checkbox := "[ ]"
	if s.CopyMessages {
		checkbox = "[x]"
	}
	copyOption := copyStyle.Render(copyPrefix + checkbox + " Include messages from parent session")

	// Branch name input
	branchLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginTop(1).
		Render("Branch name:")

	branchInputStyle := lipgloss.NewStyle()
	if s.Focus == 1 {
		branchInputStyle = branchInputStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
	} else {
		branchInputStyle = branchInputStyle.PaddingLeft(2)
	}
	branchView := branchInputStyle.Render(s.BranchInput.View())

	parts := []string{title, parentLabel, parentName, copyLabel, copyOption, branchLabel, branchView}

	// Container mode checkbox (only on Apple Silicon)
	if s.ContainersSupported {
		containerLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			MarginTop(1).
			Render("Run in container:")

		containerCheckbox := "[ ]"
		if s.UseContainers {
			containerCheckbox = "[x]"
		}
		containerCheckboxStyle := lipgloss.NewStyle()
		if s.Focus == 2 {
			containerCheckboxStyle = containerCheckboxStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
		} else {
			containerCheckboxStyle = containerCheckboxStyle.PaddingLeft(2)
		}
		containerDesc := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Width(50).
			Render("Run in a sandboxed container (no permission prompts)")
		containerView := containerCheckboxStyle.Render(containerCheckbox + " " + containerDesc)

		parts = append(parts, containerLabel, containerView)

		if s.UseContainers && !s.ContainerAuthAvailable {
			authWarning := lipgloss.NewStyle().
				Foreground(ColorWarning).
				Bold(true).
				Width(55).
				PaddingLeft(2).
				Render(ContainerAuthHelp)
			parts = append(parts, authWarning)
		}
	}

	help := ModalHelpStyle.Render(s.Help())
	parts = append(parts, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// numFields returns the number of focusable fields.
func (s *ForkSessionState) numFields() int {
	if s.ContainersSupported {
		return 3 // copy messages, branch input, containers
	}
	return 2 // copy messages, branch input
}

func (s *ForkSessionState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	numFields := s.numFields()

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case keys.Tab:
			oldFocus := s.Focus
			s.Focus = (s.Focus + 1) % numFields
			s.updateForkInputFocus(oldFocus)
			return s, nil
		case keys.ShiftTab:
			oldFocus := s.Focus
			s.Focus = (s.Focus - 1 + numFields) % numFields
			s.updateForkInputFocus(oldFocus)
			return s, nil
		case keys.Space:
			if s.Focus == 0 {
				s.CopyMessages = !s.CopyMessages
			} else if s.Focus == 2 && s.ContainersSupported {
				s.UseContainers = !s.UseContainers
			}
			return s, nil
		case keys.Up, "k":
			oldFocus := s.Focus
			s.Focus = (s.Focus - 1 + numFields) % numFields
			s.updateForkInputFocus(oldFocus)
			return s, nil
		case keys.Down, "j":
			oldFocus := s.Focus
			s.Focus = (s.Focus + 1) % numFields
			s.updateForkInputFocus(oldFocus)
			return s, nil
		}
	}

	// Handle branch input updates when focused
	if s.Focus == 1 {
		var cmd tea.Cmd
		s.BranchInput, cmd = s.BranchInput.Update(msg)
		return s, cmd
	}

	return s, nil
}

// updateForkInputFocus manages focus state for text inputs based on current Focus index.
func (s *ForkSessionState) updateForkInputFocus(oldFocus int) {
	if s.Focus == 1 {
		s.BranchInput.Focus()
	} else if oldFocus == 1 {
		s.BranchInput.Blur()
	}
}

// GetBranchName returns the custom branch name
func (s *ForkSessionState) GetBranchName() string {
	return s.BranchInput.Value()
}

// ShouldCopyMessages returns whether to copy conversation history
func (s *ForkSessionState) ShouldCopyMessages() bool {
	return s.CopyMessages
}

// GetUseContainers returns whether container mode is selected
func (s *ForkSessionState) GetUseContainers() bool {
	return s.UseContainers
}

// NewForkSessionState creates a new ForkSessionState.
// parentContainerized is the parent session's container status (used as default for the checkbox).
// containersSupported indicates whether the host supports Apple containers (darwin/arm64).
// containerAuthAvailable indicates whether API key credentials exist for container mode.
func NewForkSessionState(parentSessionName, parentSessionID, repoPath string, parentContainerized bool, containersSupported bool, containerAuthAvailable bool) *ForkSessionState {
	branchInput := textinput.New()
	branchInput.Placeholder = "optional branch name (leave empty for auto)"
	branchInput.CharLimit = BranchNameCharLimit
	branchInput.SetWidth(ModalInputWidth)

	return &ForkSessionState{
		ParentSessionName:      parentSessionName,
		ParentSessionID:        parentSessionID,
		RepoPath:               repoPath,
		BranchInput:            branchInput,
		CopyMessages:           true, // Default to copying messages
		UseContainers:          parentContainerized,
		ContainersSupported:    containersSupported,
		ContainerAuthAvailable: containerAuthAvailable,
		Focus:                  0,
	}
}

// =============================================================================
// RenameSessionState - State for the Rename Session modal
// =============================================================================

type RenameSessionState struct {
	SessionID   string
	SessionName string
	NameInput   textinput.Model
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

	// New name input
	newLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginTop(1).
		Render("New name:")

	inputStyle := lipgloss.NewStyle().
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorPrimary).
		PaddingLeft(1)
	inputView := inputStyle.Render(s.NameInput.View())

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		currentLabel,
		currentName,
		newLabel,
		inputView,
		help,
	)
}

func (s *RenameSessionState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	var cmd tea.Cmd
	s.NameInput, cmd = s.NameInput.Update(msg)
	return s, cmd
}

// GetNewName returns the new name entered by the user
func (s *RenameSessionState) GetNewName() string {
	return s.NameInput.Value()
}

// NewRenameSessionState creates a new RenameSessionState
func NewRenameSessionState(sessionID, currentName string) *RenameSessionState {
	nameInput := textinput.New()
	nameInput.Placeholder = "enter new name"
	nameInput.CharLimit = SessionNameCharLimit
	nameInput.SetWidth(ModalInputWidth)
	nameInput.SetValue(currentName)
	nameInput.Focus()

	return &RenameSessionState{
		SessionID:   sessionID,
		SessionName: currentName,
		NameInput:   nameInput,
	}
}
