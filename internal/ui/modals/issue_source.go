package modals

import (
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zhubert/plural/internal/keys"
)

// IssueSource represents a source that can provide issues/tasks.
type IssueSource struct {
	Name   string // Display name (e.g., "GitHub Issues", "Asana Tasks")
	Source string // Source identifier ("github", "asana")
}

// =============================================================================
// SelectIssueSourceState - State for selecting an issue source
// =============================================================================

// SelectIssueSourceState holds state for the Select Issue Source modal.
// This modal is shown when multiple issue sources (GitHub, Asana) are available.
type SelectIssueSourceState struct {
	RepoPath      string
	RepoName      string
	Sources       []IssueSource
	SelectedIndex int
}

func (*SelectIssueSourceState) modalState() {}

func (s *SelectIssueSourceState) Title() string { return "Select Issue Source" }

func (s *SelectIssueSourceState) Help() string {
	return "up/down: navigate  Enter: select  Esc: cancel"
}

func (s *SelectIssueSourceState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Repo info
	repoLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Repository:")

	repoName := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true).
		MarginBottom(1).
		Render("  " + s.RepoName)

	// Source selection
	description := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginBottom(1).
		Render("Select where to import issues/tasks from:")

	// Build source list
	var sourceList strings.Builder
	for i, source := range s.Sources {
		style := SidebarItemStyle
		prefix := "  "

		if i == s.SelectedIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}

		sourceList.WriteString(style.Render(prefix+source.Name) + "\n")
	}

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, repoLabel, repoName, description, sourceList.String(), help)
}

func (s *SelectIssueSourceState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case keys.Up, "k":
			if s.SelectedIndex > 0 {
				s.SelectedIndex--
			}
		case keys.Down, "j":
			if s.SelectedIndex < len(s.Sources)-1 {
				s.SelectedIndex++
			}
		}
	}
	return s, nil
}

// GetSelectedSource returns the selected source identifier.
func (s *SelectIssueSourceState) GetSelectedSource() string {
	if s.SelectedIndex < len(s.Sources) {
		return s.Sources[s.SelectedIndex].Source
	}
	return ""
}

// NewSelectIssueSourceState creates a new SelectIssueSourceState.
func NewSelectIssueSourceState(repoPath string, sources []IssueSource) *SelectIssueSourceState {
	return &SelectIssueSourceState{
		RepoPath:      repoPath,
		RepoName:      filepath.Base(repoPath),
		Sources:       sources,
		SelectedIndex: 0,
	}
}
