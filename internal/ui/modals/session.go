package modals

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// =============================================================================
// NewSessionState - State for the New Session modal
// =============================================================================

type NewSessionState struct {
	RepoOptions []string
	RepoIndex   int
	BaseOptions []string // Options for base branch selection
	BaseIndex   int      // Selected base option index
	BranchInput textinput.Model
	Focus       int // 0=repo list, 1=base selection, 2=branch input
}

func (*NewSessionState) modalState() {}

func (s *NewSessionState) Title() string { return "New Session" }

func (s *NewSessionState) Help() string {
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

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, repoLabel, repoList, baseLabel, baseList, branchLabel, branchView, help)
}

func (s *NewSessionState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
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
		case "down", "j":
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
		case "tab":
			switch s.Focus {
			case 0:
				s.Focus = 1
			case 1:
				s.Focus = 2
				s.BranchInput.Focus()
			case 2:
				s.Focus = 0
				s.BranchInput.Blur()
			}
			return s, nil
		case "shift+tab":
			switch s.Focus {
			case 2:
				s.Focus = 1
				s.BranchInput.Blur()
			case 1:
				s.Focus = 0
			case 0:
				s.Focus = 2
				s.BranchInput.Focus()
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

// NewNewSessionState creates a new NewSessionState with proper initialization
func NewNewSessionState(repos []string) *NewSessionState {
	branchInput := textinput.New()
	branchInput.Placeholder = "optional branch name (leave empty for auto)"
	branchInput.CharLimit = 100
	branchInput.SetWidth(ModalInputWidth)

	return &NewSessionState{
		RepoOptions: repos,
		RepoIndex:   0,
		BaseOptions: []string{
			"From remote default branch (latest)",
			"From current local branch",
		},
		BaseIndex:   0,
		BranchInput: branchInput,
		Focus:       0,
	}
}

// =============================================================================
// ForkSessionState - State for the Fork Session modal
// =============================================================================

type ForkSessionState struct {
	ParentSessionName string
	ParentSessionID   string
	RepoPath          string
	BranchInput       textinput.Model
	CopyMessages      bool // Whether to copy conversation history
	Focus             int  // 0=copy messages toggle, 1=branch input
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

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		parentLabel,
		parentName,
		copyLabel,
		copyOption,
		branchLabel,
		branchView,
		help,
	)
}

func (s *ForkSessionState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "tab":
			if s.Focus == 0 {
				s.Focus = 1
				s.BranchInput.Focus()
			} else {
				s.Focus = 0
				s.BranchInput.Blur()
			}
			return s, nil
		case "shift+tab":
			if s.Focus == 1 {
				s.Focus = 0
				s.BranchInput.Blur()
			}
			return s, nil
		case "space":
			if s.Focus == 0 {
				s.CopyMessages = !s.CopyMessages
			}
			return s, nil
		case "up", "down", "j", "k":
			// Toggle focus between options
			if s.Focus == 0 {
				s.Focus = 1
				s.BranchInput.Focus()
			} else {
				s.Focus = 0
				s.BranchInput.Blur()
			}
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

// GetBranchName returns the custom branch name
func (s *ForkSessionState) GetBranchName() string {
	return s.BranchInput.Value()
}

// ShouldCopyMessages returns whether to copy conversation history
func (s *ForkSessionState) ShouldCopyMessages() bool {
	return s.CopyMessages
}

// NewForkSessionState creates a new ForkSessionState
func NewForkSessionState(parentSessionName, parentSessionID, repoPath string) *ForkSessionState {
	branchInput := textinput.New()
	branchInput.Placeholder = "optional branch name (leave empty for auto)"
	branchInput.CharLimit = 100
	branchInput.SetWidth(ModalInputWidth)

	return &ForkSessionState{
		ParentSessionName: parentSessionName,
		ParentSessionID:   parentSessionID,
		RepoPath:          repoPath,
		BranchInput:       branchInput,
		CopyMessages:      true, // Default to copying messages
		Focus:             0,
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
	nameInput.CharLimit = 100
	nameInput.SetWidth(ModalInputWidth)
	nameInput.SetValue(currentName)
	nameInput.Focus()

	return &RenameSessionState{
		SessionID:   sessionID,
		SessionName: currentName,
		NameInput:   nameInput,
	}
}
