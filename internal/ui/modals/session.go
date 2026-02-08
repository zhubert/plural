package modals

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/zhubert/plural/internal/keys"
)

// =============================================================================
// NewSessionState - State for the New Session modal
// =============================================================================

type NewSessionState struct {
	RepoOptions         []string
	RepoIndex           int
	BaseOptions         []string // Options for base branch selection
	BaseIndex           int      // Selected base option index
	BranchInput         textinput.Model
	UseContainers       bool // Whether to run this session in a container
	ContainersSupported bool // Whether the host supports Apple containers (darwin/arm64)
	Focus               int  // 0=repo list, 1=base selection, 2=branch input, 3=containers (if supported)
}

func (*NewSessionState) modalState() {}

func (s *NewSessionState) Title() string { return "New Session" }

func (s *NewSessionState) Help() string {
	if s.Focus == 0 && len(s.RepoOptions) > 0 {
		return "up/down: select  Tab: next field  d: delete repo  Enter: create"
	}
	if s.Focus == 3 && s.ContainersSupported {
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
			Render("No repositories added. Press 'r' to add one first.")
	} else {
		repoList = RenderSelectableListWithFocus(s.RepoOptions, s.RepoIndex, s.Focus == 0, "* ")
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

	branchInputStyle := lipgloss.NewStyle()
	if s.Focus == 2 {
		branchInputStyle = branchInputStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
	} else {
		branchInputStyle = branchInputStyle.PaddingLeft(2)
	}
	branchView := branchInputStyle.Render(s.BranchInput.View())

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
			Render("Run Claude CLI inside an Apple container with --dangerously-skip-permissions")
		containerView := containerCheckboxStyle.Render(containerCheckbox + " " + containerDesc)

		containerWarning := lipgloss.NewStyle().
			Foreground(ColorWarning).
			Italic(true).
			Width(50).
			PaddingLeft(2).
			Render("Warning: Containers provide defense in depth but are not a complete security boundary.")

		parts = append(parts, containerLabel, containerView, containerWarning)
	}

	help := ModalHelpStyle.Render(s.Help())
	parts = append(parts, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// numFields returns the number of focusable fields.
func (s *NewSessionState) numFields() int {
	if s.ContainersSupported {
		return 4 // repo list, base selection, branch input, containers
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
				}
			case 1: // Base selection
				if s.BaseIndex < len(s.BaseOptions)-1 {
					s.BaseIndex++
				}
			}
		case keys.Tab:
			oldFocus := s.Focus
			s.Focus = (s.Focus + 1) % numFields
			s.updateInputFocus(oldFocus)
			return s, nil
		case keys.ShiftTab:
			oldFocus := s.Focus
			s.Focus = (s.Focus - 1 + numFields) % numFields
			s.updateInputFocus(oldFocus)
			return s, nil
		case keys.Space:
			if s.Focus == 3 && s.ContainersSupported {
				s.UseContainers = !s.UseContainers
			}
			return s, nil
		}
	}

	// Handle branch input updates when focused
	if s.Focus == 2 {
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

// NewNewSessionState creates a new NewSessionState with proper initialization.
// containersSupported indicates whether the host supports Apple containers (darwin/arm64).
func NewNewSessionState(repos []string, containersSupported bool) *NewSessionState {
	branchInput := textinput.New()
	branchInput.Placeholder = "optional branch name (leave empty for auto)"
	branchInput.CharLimit = BranchNameCharLimit
	branchInput.SetWidth(ModalInputWidth)

	return &NewSessionState{
		RepoOptions:         repos,
		RepoIndex:           0,
		BaseOptions: []string{
			"From current local branch",
			"From remote default branch (latest)",
		},
		BaseIndex:           0,
		BranchInput:         branchInput,
		ContainersSupported: containersSupported,
		Focus:               0,
	}
}

// =============================================================================
// ForkSessionState - State for the Fork Session modal
// =============================================================================

type ForkSessionState struct {
	ParentSessionName   string
	ParentSessionID     string
	RepoPath            string
	BranchInput         textinput.Model
	CopyMessages        bool // Whether to copy conversation history
	UseContainers       bool // Whether to run this session in a container
	ContainersSupported bool // Whether the host supports Apple containers (darwin/arm64)
	Focus               int  // 0=copy messages toggle, 1=branch input, 2=containers (if supported)
}

func (*ForkSessionState) modalState() {}

func (s *ForkSessionState) Title() string { return "Fork Session" }

func (s *ForkSessionState) Help() string {
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
			Render("Run Claude CLI inside an Apple container with --dangerously-skip-permissions")
		containerView := containerCheckboxStyle.Render(containerCheckbox + " " + containerDesc)

		parts = append(parts, containerLabel, containerView)
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
		case keys.Up, keys.Down, "j", "k":
			// Toggle focus between options
			oldFocus := s.Focus
			if s.Focus == 0 {
				s.Focus = 1
			} else {
				s.Focus = 0
			}
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
func NewForkSessionState(parentSessionName, parentSessionID, repoPath string, parentContainerized bool, containersSupported bool) *ForkSessionState {
	branchInput := textinput.New()
	branchInput.Placeholder = "optional branch name (leave empty for auto)"
	branchInput.CharLimit = BranchNameCharLimit
	branchInput.SetWidth(ModalInputWidth)

	return &ForkSessionState{
		ParentSessionName:   parentSessionName,
		ParentSessionID:     parentSessionID,
		RepoPath:            repoPath,
		BranchInput:         branchInput,
		CopyMessages:        true, // Default to copying messages
		UseContainers:       parentContainerized,
		ContainersSupported: containersSupported,
		Focus:               0,
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
