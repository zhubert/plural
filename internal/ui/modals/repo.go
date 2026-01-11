package modals

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// =============================================================================
// AddRepoState - State for the Add Repository modal
// =============================================================================

type AddRepoState struct {
	Input         textinput.Model
	SuggestedRepo string
	UseSuggested  bool
}

func (*AddRepoState) modalState() {}

func (s *AddRepoState) Title() string { return "Add Repository" }

func (s *AddRepoState) Help() string {
	if s.SuggestedRepo != "" {
		return "up/down to switch, Enter to confirm, Esc to cancel"
	}
	return "Enter the full path to a git repository"
}

func (s *AddRepoState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	var content string

	if s.SuggestedRepo != "" {
		suggestionLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Render("Current directory:")

		style := SidebarItemStyle
		prefix := "  "
		if s.UseSuggested {
			style = SidebarSelectedStyle
			prefix = "> "
		}
		suggestionItem := style.Render(prefix + s.SuggestedRepo)

		otherLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			MarginTop(1).
			Render("Or enter a different path:")

		inputStyle := lipgloss.NewStyle()
		if !s.UseSuggested {
			inputStyle = inputStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
		} else {
			inputStyle = inputStyle.PaddingLeft(2)
		}
		inputView := inputStyle.Render(s.Input.View())

		content = lipgloss.JoinVertical(lipgloss.Left, suggestionLabel, suggestionItem, otherLabel, inputView)
	} else {
		content = s.Input.View()
	}

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, content, help)
}

func (s *AddRepoState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && s.SuggestedRepo != "" {
		switch keyMsg.String() {
		case "up", "down", "tab":
			s.UseSuggested = !s.UseSuggested
			if s.UseSuggested {
				s.Input.Blur()
			} else {
				s.Input.Focus()
			}
			return s, nil
		}
	}

	// Only update text input when it's focused
	if !s.UseSuggested {
		var cmd tea.Cmd
		s.Input, cmd = s.Input.Update(msg)
		return s, cmd
	}

	return s, nil
}

// GetPath returns the path to add (either suggested or from input)
func (s *AddRepoState) GetPath() string {
	if s.SuggestedRepo != "" && s.UseSuggested {
		return s.SuggestedRepo
	}
	return s.Input.Value()
}

// NewAddRepoState creates a new AddRepoState with proper initialization
func NewAddRepoState(suggestedRepo string) *AddRepoState {
	ti := textinput.New()
	ti.Placeholder = "/path/to/repo"
	ti.CharLimit = ModalInputCharLimit
	ti.SetWidth(ModalInputWidth)

	state := &AddRepoState{
		Input:         ti,
		SuggestedRepo: suggestedRepo,
		UseSuggested:  suggestedRepo != "",
	}

	if suggestedRepo == "" {
		state.Input.Focus()
	}

	return state
}

// =============================================================================
// SelectRepoForIssuesState - State for selecting a repo to import issues from
// =============================================================================

type SelectRepoForIssuesState struct {
	RepoOptions []string
	RepoIndex   int
}

func (*SelectRepoForIssuesState) modalState() {}

func (s *SelectRepoForIssuesState) Title() string { return "Select Repository" }

func (s *SelectRepoForIssuesState) Help() string {
	return "up/down select repo  Enter: import issues  Esc: cancel"
}

func (s *SelectRepoForIssuesState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Repository selection section
	repoLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Select a repository to import GitHub issues from:")

	var repoList string
	if len(s.RepoOptions) == 0 {
		repoList = lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("No repositories added. Press 'a' to add one first.")
	} else {
		for i, repo := range s.RepoOptions {
			style := SidebarItemStyle
			prefix := "  "
			if i == s.RepoIndex {
				style = SidebarSelectedStyle
				prefix = "> "
			}
			repoList += style.Render(prefix+repo) + "\n"
		}
	}

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, repoLabel, repoList, help)
}

func (s *SelectRepoForIssuesState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if s.RepoIndex > 0 {
				s.RepoIndex--
			}
		case "down", "j":
			if s.RepoIndex < len(s.RepoOptions)-1 {
				s.RepoIndex++
			}
		}
	}
	return s, nil
}

// GetSelectedRepo returns the selected repository path
func (s *SelectRepoForIssuesState) GetSelectedRepo() string {
	if len(s.RepoOptions) == 0 || s.RepoIndex >= len(s.RepoOptions) {
		return ""
	}
	return s.RepoOptions[s.RepoIndex]
}

// NewSelectRepoForIssuesState creates a new SelectRepoForIssuesState
func NewSelectRepoForIssuesState(repos []string) *SelectRepoForIssuesState {
	return &SelectRepoForIssuesState{
		RepoOptions: repos,
		RepoIndex:   0,
	}
}
