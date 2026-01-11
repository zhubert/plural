package modals

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// =============================================================================
// ImportIssuesState - State for importing GitHub issues as sessions
// =============================================================================

// ImportIssuesState holds state for the Import Issues modal
type ImportIssuesState struct {
	RepoPath      string
	RepoName      string
	Issues        []IssueItem
	SelectedIndex int
	Loading       bool
	LoadError     string
	ScrollOffset  int
	maxVisible    int
}

func (*ImportIssuesState) modalState() {}

func (s *ImportIssuesState) Title() string { return "Import GitHub Issues" }

func (s *ImportIssuesState) Help() string {
	if s.Loading {
		return "Loading issues..."
	}
	if s.LoadError != "" {
		return "Esc: close"
	}
	return "up/down navigate  Space: toggle  Enter: import  Esc: cancel"
}

func (s *ImportIssuesState) Render() string {
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

	// Loading state
	if s.Loading {
		loadingText := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("Fetching issues from GitHub...")
		help := ModalHelpStyle.Render(s.Help())
		return lipgloss.JoinVertical(lipgloss.Left, title, repoLabel, repoName, loadingText, help)
	}

	// Error state
	if s.LoadError != "" {
		errorText := StatusErrorStyle.Render(s.LoadError)
		help := ModalHelpStyle.Render(s.Help())
		return lipgloss.JoinVertical(lipgloss.Left, title, repoLabel, repoName, errorText, help)
	}

	// No issues
	if len(s.Issues) == 0 {
		noIssues := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("No open issues found")
		help := ModalHelpStyle.Render(s.Help())
		return lipgloss.JoinVertical(lipgloss.Left, title, repoLabel, repoName, noIssues, help)
	}

	description := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginBottom(1).
		Render("Select issues to import as sessions:")

	// Build issue list with scrolling
	var issueList string
	visibleEnd := s.ScrollOffset + s.maxVisible
	if visibleEnd > len(s.Issues) {
		visibleEnd = len(s.Issues)
	}

	for i := s.ScrollOffset; i < visibleEnd; i++ {
		issue := s.Issues[i]
		style := SidebarItemStyle
		prefix := "  "
		checkbox := "[ ]"

		if i == s.SelectedIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}

		if issue.Selected {
			checkbox = "[x]"
		}

		// Truncate long titles
		titleText := issue.Title
		if len(titleText) > 45 {
			titleText = titleText[:42] + "..."
		}

		issueLine := fmt.Sprintf("%s #%d: %s", checkbox, issue.Number, titleText)
		issueList += style.Render(prefix+issueLine) + "\n"
	}

	// Scroll indicators
	if s.ScrollOffset > 0 {
		issueList = lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  up more above\n") + issueList
	}
	if visibleEnd < len(s.Issues) {
		issueList += lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  down more below\n")
	}

	// Show count of selected issues
	selectedCount := 0
	for _, issue := range s.Issues {
		if issue.Selected {
			selectedCount++
		}
	}

	countStyle := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		MarginTop(1)
	countText := fmt.Sprintf("%d issue(s) selected", selectedCount)
	if selectedCount > 0 {
		countText += fmt.Sprintf(" - will create %d session(s)", selectedCount)
	}
	countSection := countStyle.Render(countText)

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, repoLabel, repoName, description, issueList, countSection, help)
}

func (s *ImportIssuesState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if s.SelectedIndex > 0 {
				s.SelectedIndex--
				// Scroll up if needed
				if s.SelectedIndex < s.ScrollOffset {
					s.ScrollOffset = s.SelectedIndex
				}
			}
		case "down", "j":
			if s.SelectedIndex < len(s.Issues)-1 {
				s.SelectedIndex++
				// Scroll down if needed
				if s.SelectedIndex >= s.ScrollOffset+s.maxVisible {
					s.ScrollOffset = s.SelectedIndex - s.maxVisible + 1
				}
			}
		case "space":
			// Toggle selection
			if s.SelectedIndex < len(s.Issues) {
				s.Issues[s.SelectedIndex].Selected = !s.Issues[s.SelectedIndex].Selected
			}
		}
	}
	return s, nil
}

// GetSelectedIssues returns the issues that are selected
func (s *ImportIssuesState) GetSelectedIssues() []IssueItem {
	var selected []IssueItem
	for _, issue := range s.Issues {
		if issue.Selected {
			selected = append(selected, issue)
		}
	}
	return selected
}

// SetIssues sets the issues list and clears loading state
func (s *ImportIssuesState) SetIssues(issues []IssueItem) {
	s.Issues = issues
	s.Loading = false
	s.LoadError = ""
}

// SetError sets an error and clears loading state
func (s *ImportIssuesState) SetError(err string) {
	s.LoadError = err
	s.Loading = false
}

// NewImportIssuesState creates a new ImportIssuesState in loading state
func NewImportIssuesState(repoPath, repoName string) *ImportIssuesState {
	return &ImportIssuesState{
		RepoPath:      repoPath,
		RepoName:      repoName,
		Loading:       true,
		SelectedIndex: 0,
		ScrollOffset:  0,
		maxVisible:    10,
	}
}
