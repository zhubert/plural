package modals

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zhubert/plural/internal/keys"
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
	Source        string // "github" or "asana"
	ProjectID     string // Asana project GID (only used for Asana)
}

func (*ImportIssuesState) modalState() {}

// PreferredWidth returns the preferred width for this modal.
// Import issues modal uses a wider width to show more of the issue titles.
func (s *ImportIssuesState) PreferredWidth() int {
	return ModalWidthWide
}

func (s *ImportIssuesState) Title() string {
	switch s.Source {
	case "asana":
		return "Import Asana Tasks"
	default:
		return "Import GitHub Issues"
	}
}

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
		loadingMsg := "Fetching issues from GitHub..."
		if s.Source == "asana" {
			loadingMsg = "Fetching tasks from Asana..."
		}
		loadingText := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render(loadingMsg)
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
		noIssuesMsg := "No open issues found"
		if s.Source == "asana" {
			noIssuesMsg = "No incomplete tasks found"
		}
		noIssues := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render(noIssuesMsg)
		help := ModalHelpStyle.Render(s.Help())
		return lipgloss.JoinVertical(lipgloss.Left, title, repoLabel, repoName, noIssues, help)
	}

	descMsg := "Select issues to import as sessions:"
	if s.Source == "asana" {
		descMsg = "Select tasks to import as sessions:"
	}
	description := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginBottom(1).
		Render(descMsg)

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

		// Calculate available width for title based on modal width
		// ModalWidthWide (120) - padding (4) - prefix (2) - checkbox (4) - issue # (varies) = available for title
		// For GitHub: "  > [x] #123: " = 2 + 1 + 4 + 1 + up to 5 for issue # + 2 = ~15 chars overhead
		// For Asana: "  > [x] " = 2 + 1 + 4 + 1 = 8 chars overhead
		modalWidth := s.PreferredWidth()
		availableWidth := modalWidth - 4 // Account for modal padding/borders

		// Truncate long titles based on available width
		titleText := issue.Title
		var maxTitleLen int
		if issue.Source == "asana" {
			maxTitleLen = availableWidth - 8 // "  > [x] "
		} else {
			// Account for issue number (estimate ~7 chars for "#12345: ")
			maxTitleLen = availableWidth - 15
		}

		// Use rune-based truncation to safely handle multi-byte Unicode characters
		titleRunes := []rune(titleText)
		if len(titleRunes) > maxTitleLen && maxTitleLen > 3 {
			titleText = string(titleRunes[:maxTitleLen-3]) + "..."
		}

		// Format depends on source: GitHub uses "#123", Asana just shows title
		var issueLine string
		if issue.Source == "asana" {
			issueLine = fmt.Sprintf("%s %s", checkbox, titleText)
		} else {
			issueLine = fmt.Sprintf("%s #%s: %s", checkbox, issue.ID, titleText)
		}
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
		case keys.Up, "k":
			if s.SelectedIndex > 0 {
				s.SelectedIndex--
				// Scroll up if needed
				if s.SelectedIndex < s.ScrollOffset {
					s.ScrollOffset = s.SelectedIndex
				}
			}
		case keys.Down, "j":
			if s.SelectedIndex < len(s.Issues)-1 {
				s.SelectedIndex++
				// Scroll down if needed
				if s.SelectedIndex >= s.ScrollOffset+s.maxVisible {
					s.ScrollOffset = s.SelectedIndex - s.maxVisible + 1
				}
			}
		case keys.Space:
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

// NewImportIssuesState creates a new ImportIssuesState in loading state for GitHub issues.
func NewImportIssuesState(repoPath, repoName string) *ImportIssuesState {
	return &ImportIssuesState{
		RepoPath:      repoPath,
		RepoName:      repoName,
		Loading:       true,
		SelectedIndex: 0,
		ScrollOffset:  0,
		maxVisible:    IssuesModalMaxVisible,
		Source:        "github",
	}
}

// NewImportIssuesStateWithSource creates a new ImportIssuesState for a specific source.
func NewImportIssuesStateWithSource(repoPath, repoName, source, projectID string) *ImportIssuesState {
	return &ImportIssuesState{
		RepoPath:      repoPath,
		RepoName:      repoName,
		Loading:       true,
		SelectedIndex: 0,
		ScrollOffset:  0,
		maxVisible:    IssuesModalMaxVisible,
		Source:        source,
		ProjectID:     projectID,
	}
}
