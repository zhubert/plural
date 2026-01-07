package ui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ModalType represents the type of modal
type ModalType int

const (
	ModalNone ModalType = iota
	ModalAddRepo
	ModalNewSession
	ModalConfirmDelete
	ModalMerge
)

// Modal represents a popup dialog
type Modal struct {
	Type        ModalType
	input       textinput.Model
	title       string
	help        string
	error       string
	width       int
	height      int
	repoOptions []string // For session creation, list of repos
	repoIndex   int      // Selected repo index

	// Merge modal fields
	mergeOptions   []string // Available merge options
	mergeIndex     int      // Selected option index
	hasRemote      bool     // Whether remote origin exists
	changesSummary string   // Summary of uncommitted changes

	// Add repo modal fields
	suggestedRepo    string // Current directory if it's a git repo and not already added
	useSuggestedRepo bool   // Whether the suggestion is selected (vs text input)

	// Delete modal fields
	deleteOptions []string // Delete options (keep/delete worktree)
	deleteIndex   int      // Selected delete option index
}

// NewModal creates a new modal
func NewModal() *Modal {
	ti := textinput.New()
	ti.Placeholder = "Enter path..."
	ti.CharLimit = ModalInputCharLimit
	ti.SetWidth(ModalInputWidth)

	return &Modal{
		Type:  ModalNone,
		input: ti,
	}
}

// SetSuggestedRepo sets the suggested repo for the add repo modal
func (m *Modal) SetSuggestedRepo(path string) {
	m.suggestedRepo = path
	m.useSuggestedRepo = path != ""
}

// Show shows a modal of the specified type
func (m *Modal) Show(t ModalType) {
	m.Type = t
	m.error = ""
	m.input.Reset()

	switch t {
	case ModalAddRepo:
		m.title = "Add Repository"
		m.input.Placeholder = "/path/to/repo"
		if m.suggestedRepo != "" {
			m.useSuggestedRepo = true
			m.input.Blur()
			m.help = "↑/↓ to switch, Enter to confirm, Esc to cancel"
		} else {
			m.useSuggestedRepo = false
			m.input.Focus()
			m.help = "Enter the full path to a git repository"
		}
	case ModalNewSession:
		m.title = "New Session"
		m.help = "Select a repository for the new session"
		m.repoIndex = 0
	case ModalConfirmDelete:
		m.title = "Delete Session?"
		m.help = "↑/↓ to select, Enter to confirm, Esc to cancel"
		m.deleteOptions = []string{"Keep worktree", "Delete worktree"}
		m.deleteIndex = 0
	case ModalMerge:
		m.title = "Merge/PR"
		m.help = "↑/↓ to select, Enter to confirm, Esc to cancel"
		m.mergeIndex = 0
	}
}

// Hide hides the modal
func (m *Modal) Hide() {
	m.Type = ModalNone
	m.error = ""
	m.input.Blur()
}

// IsVisible returns whether the modal is visible
func (m *Modal) IsVisible() bool {
	return m.Type != ModalNone
}

// SetError sets an error message
func (m *Modal) SetError(err string) {
	m.error = err
}

// GetInput returns the current input value
func (m *Modal) GetInput() string {
	return m.input.Value()
}

// SetRepoOptions sets the available repos for session creation
func (m *Modal) SetRepoOptions(repos []string) {
	m.repoOptions = repos
	m.repoIndex = 0
}

// GetSelectedRepo returns the selected repo for session creation
func (m *Modal) GetSelectedRepo() string {
	if len(m.repoOptions) == 0 {
		return ""
	}
	if m.repoIndex >= len(m.repoOptions) {
		m.repoIndex = 0
	}
	return m.repoOptions[m.repoIndex]
}

// SetMergeOptions sets the available merge options based on remote availability
func (m *Modal) SetMergeOptions(hasRemote bool, changesSummary string) {
	m.hasRemote = hasRemote
	m.changesSummary = changesSummary
	m.mergeOptions = []string{"Merge to main"}
	if hasRemote {
		m.mergeOptions = append(m.mergeOptions, "Create PR")
	}
	m.mergeIndex = 0
}

// GetSelectedMergeOption returns the selected merge option
func (m *Modal) GetSelectedMergeOption() string {
	if len(m.mergeOptions) == 0 {
		return ""
	}
	if m.mergeIndex >= len(m.mergeOptions) {
		m.mergeIndex = 0
	}
	return m.mergeOptions[m.mergeIndex]
}

// ShouldDeleteWorktree returns true if user selected to delete the worktree
func (m *Modal) ShouldDeleteWorktree() bool {
	return m.deleteIndex == 1 // "Delete worktree" is index 1
}

// Update handles messages
func (m *Modal) Update(msg tea.Msg) (*Modal, tea.Cmd) {
	if !m.IsVisible() {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch m.Type {
		case ModalNewSession:
			switch msg.String() {
			case "up", "k":
				if m.repoIndex > 0 {
					m.repoIndex--
				}
			case "down", "j":
				if m.repoIndex < len(m.repoOptions)-1 {
					m.repoIndex++
				}
			}
		case ModalConfirmDelete:
			switch msg.String() {
			case "up", "k":
				if m.deleteIndex > 0 {
					m.deleteIndex--
				}
			case "down", "j":
				if m.deleteIndex < len(m.deleteOptions)-1 {
					m.deleteIndex++
				}
			}
		case ModalMerge:
			switch msg.String() {
			case "up", "k":
				if m.mergeIndex > 0 {
					m.mergeIndex--
				}
			case "down", "j":
				if m.mergeIndex < len(m.mergeOptions)-1 {
					m.mergeIndex++
				}
			}
		}
	}

	if m.Type == ModalAddRepo {
		// Handle navigation between suggestion and text input
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok && m.suggestedRepo != "" {
			switch keyMsg.String() {
			case "up", "down", "tab":
				m.useSuggestedRepo = !m.useSuggestedRepo
				if m.useSuggestedRepo {
					m.input.Blur()
				} else {
					m.input.Focus()
				}
				return m, nil
			}
		}

		// Only update text input when it's focused
		if !m.useSuggestedRepo {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// View renders the modal
func (m *Modal) View(screenWidth, screenHeight int) string {
	if !m.IsVisible() {
		return ""
	}

	var content string

	switch m.Type {
	case ModalAddRepo:
		content = m.renderAddRepo()
	case ModalNewSession:
		content = m.renderNewSession()
	case ModalConfirmDelete:
		content = m.renderConfirmDelete()
	case ModalMerge:
		content = m.renderMerge()
	}

	modal := ModalStyle.Render(content)

	// Center the modal
	modalWidth := lipgloss.Width(modal)
	modalHeight := lipgloss.Height(modal)

	x := (screenWidth - modalWidth) / 2
	y := (screenHeight - modalHeight) / 2

	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	return lipgloss.Place(
		screenWidth, screenHeight,
		lipgloss.Center, lipgloss.Center,
		modal,
	)
}

func (m *Modal) renderAddRepo() string {
	title := ModalTitleStyle.Render(m.title)

	var content string

	// Show suggested repo if available
	if m.suggestedRepo != "" {
		suggestionLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Render("Current directory:")

		style := SidebarItemStyle
		prefix := "  "
		if m.useSuggestedRepo {
			style = SidebarSelectedStyle
			prefix = "> "
		}
		suggestionItem := style.Render(prefix + m.suggestedRepo)

		otherLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			MarginTop(1).
			Render("Or enter a different path:")

		inputStyle := lipgloss.NewStyle()
		if !m.useSuggestedRepo {
			inputStyle = inputStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
		} else {
			inputStyle = inputStyle.PaddingLeft(2)
		}
		inputView := inputStyle.Render(m.input.View())

		content = lipgloss.JoinVertical(lipgloss.Left, suggestionLabel, suggestionItem, otherLabel, inputView)
	} else {
		content = m.input.View()
	}

	var errView string
	if m.error != "" {
		errView = "\n" + StatusErrorStyle.Render(m.error)
	}

	help := ModalHelpStyle.Render(m.help)

	return lipgloss.JoinVertical(lipgloss.Left, title, content, errView, help)
}

// GetAddRepoPath returns the path to add (either suggested or from input)
func (m *Modal) GetAddRepoPath() string {
	if m.suggestedRepo != "" && m.useSuggestedRepo {
		return m.suggestedRepo
	}
	return m.input.Value()
}

func (m *Modal) renderNewSession() string {
	title := ModalTitleStyle.Render(m.title)

	var repoList string
	if len(m.repoOptions) == 0 {
		repoList = lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("No repositories added. Press 'r' to add one first.")
	} else {
		for i, repo := range m.repoOptions {
			style := SidebarItemStyle
			prefix := "  "
			if i == m.repoIndex {
				style = SidebarSelectedStyle
				prefix = "> "
			}
			repoList += style.Render(prefix+repo) + "\n"
		}
	}

	help := ModalHelpStyle.Render("↑/↓ to select, Enter to confirm, Esc to cancel")

	return lipgloss.JoinVertical(lipgloss.Left, title, repoList, help)
}

func (m *Modal) renderConfirmDelete() string {
	title := ModalTitleStyle.Render(m.title)

	message := lipgloss.NewStyle().
		Foreground(ColorText).
		MarginBottom(1).
		Render("This will remove the session from the list.")

	var optionList string
	for i, opt := range m.deleteOptions {
		style := SidebarItemStyle
		prefix := "  "
		if i == m.deleteIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}
		optionList += style.Render(prefix+opt) + "\n"
	}

	help := ModalHelpStyle.Render(m.help)

	return lipgloss.JoinVertical(lipgloss.Left, title, message, optionList, help)
}

func (m *Modal) renderMerge() string {
	title := ModalTitleStyle.Render(m.title)

	// Show changes summary
	var summarySection string
	if m.changesSummary != "" {
		summaryStyle := lipgloss.NewStyle().
			Foreground(ColorSecondary).
			MarginBottom(1)
		summarySection = summaryStyle.Render("📝 " + m.changesSummary)
	} else {
		noChangesStyle := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			MarginBottom(1)
		summarySection = noChangesStyle.Render("No uncommitted changes")
	}

	var optionList string
	for i, opt := range m.mergeOptions {
		style := SidebarItemStyle
		prefix := "  "
		if i == m.mergeIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}
		optionList += style.Render(prefix+opt) + "\n"
	}

	if !m.hasRemote {
		note := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("(No remote origin - PR option unavailable)")
		optionList += "\n" + note
	}

	help := ModalHelpStyle.Render(m.help)

	return lipgloss.JoinVertical(lipgloss.Left, title, summarySection, optionList, help)
}
