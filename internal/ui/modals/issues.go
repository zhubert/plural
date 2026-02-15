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

	// Container and autonomous mode options
	UseContainers          bool // Whether to run sessions in containers
	ContainersSupported    bool // Whether Docker is available
	ContainerAuthAvailable bool // Whether API key credentials are available
	Autonomous             bool // Whether to run in autonomous mode (auto-enables containers)

	// Focus: 0 = issue list, 1 = autonomous checkbox (if containers supported), 2 = container checkbox (if containers supported)
	Focus int

	// Size tracking
	availableWidth int // Actual width available after modal is clamped to screen
}

func (*ImportIssuesState) modalState() {}

// PreferredWidth returns the preferred width for this modal.
// Import issues modal uses a wider width to show more of the issue titles.
func (s *ImportIssuesState) PreferredWidth() int {
	return ModalWidthWide
}

// SetSize updates the available width for rendering content.
// Called by the modal container before Render() to notify the modal of its actual size.
func (s *ImportIssuesState) SetSize(width, height int) {
	s.availableWidth = width
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
	if s.ContainersSupported && (s.Focus == 1 || s.Focus == 2) {
		return "up/down navigate  Space: toggle  Tab: next field  Enter: import  Esc: cancel"
	}
	return "up/down navigate  Space: toggle  Tab: next field  Enter: import  Esc: cancel"
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

		// Calculate available width for title based on actual modal width
		// Account for modal padding/borders (4 chars)
		// For GitHub: "  > [x] #123: " = 2 + 1 + 4 + 1 + up to 5 for issue # + 2 = ~15 chars overhead
		// For Asana: "  > [x] " = 2 + 1 + 4 + 1 = 8 chars overhead
		modalWidth := s.availableWidth
		if modalWidth == 0 {
			modalWidth = s.PreferredWidth() // Fallback if SetSize() wasn't called
		}
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

	var parts []string
	parts = append(parts, title, repoLabel, repoName, description, issueList, countSection)

	// Autonomous mode checkbox (only when containers supported)
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
		if s.Focus == 1 {
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

		// Container mode checkbox (only when not autonomous - autonomous forces containers)
		containerLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			MarginTop(1).
			Render("Container mode:")

		containerCheckbox := "[ ]"
		if s.UseContainers || s.Autonomous {
			containerCheckbox = "[x]"
		}
		containerCheckboxStyle := lipgloss.NewStyle()
		if s.Focus == 2 && !s.Autonomous {
			containerCheckboxStyle = containerCheckboxStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
		} else {
			containerCheckboxStyle = containerCheckboxStyle.PaddingLeft(2)
		}

		var containerDesc string
		if s.Autonomous {
			containerDesc = "(required for autonomous mode)"
		} else {
			containerDesc = "Sandbox: isolated environment with Docker"
		}
		containerDescStyle := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Width(50).
			Render(containerDesc)
		containerView := containerCheckboxStyle.Render(containerCheckbox + " " + containerDescStyle)

		parts = append(parts, containerLabel, containerView)

		// Show auth warning if containers enabled but no auth
		if (s.UseContainers || s.Autonomous) && !s.ContainerAuthAvailable {
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

func (s *ImportIssuesState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case keys.Tab:
			if s.ContainersSupported {
				// Cycle through: 0 (issue list) -> 1 (autonomous) -> 2 (containers) -> 0
				s.Focus = (s.Focus + 1) % 3
			}
		case keys.Up, "k":
			if s.ContainersSupported {
				// Navigate between focus areas when containers are supported
				if s.Focus == 1 {
					// From autonomous checkbox, move up to issue list
					s.Focus = 0
				} else if s.Focus == 2 {
					// From container checkbox, move up to autonomous checkbox
					s.Focus = 1
				} else if s.Focus == 0 && s.SelectedIndex > 0 {
					// Navigate issue list
					s.SelectedIndex--
					// Scroll up if needed
					if s.SelectedIndex < s.ScrollOffset {
						s.ScrollOffset = s.SelectedIndex
					}
				}
			} else {
				// Without container support, only navigate issue list
				if s.Focus == 0 && s.SelectedIndex > 0 {
					s.SelectedIndex--
					if s.SelectedIndex < s.ScrollOffset {
						s.ScrollOffset = s.SelectedIndex
					}
				}
			}
		case keys.Down, "j":
			if s.ContainersSupported {
				// Navigate between focus areas when containers are supported
				if s.Focus == 1 {
					// From autonomous checkbox, move down to container checkbox
					s.Focus = 2
				} else if s.Focus == 2 {
					// From container checkbox, wrap to issue list
					s.Focus = 0
				} else if s.Focus == 0 && s.SelectedIndex < len(s.Issues)-1 {
					// Navigate issue list
					s.SelectedIndex++
					// Scroll down if needed
					if s.SelectedIndex >= s.ScrollOffset+s.maxVisible {
						s.ScrollOffset = s.SelectedIndex - s.maxVisible + 1
					}
				}
			} else {
				// Without container support, only navigate issue list
				if s.Focus == 0 && s.SelectedIndex < len(s.Issues)-1 {
					s.SelectedIndex++
					if s.SelectedIndex >= s.ScrollOffset+s.maxVisible {
						s.ScrollOffset = s.SelectedIndex - s.maxVisible + 1
					}
				}
			}
		case keys.Space:
			// Toggle selection based on focus
			if s.Focus == 0 {
				// Toggle issue selection
				if s.SelectedIndex < len(s.Issues) {
					s.Issues[s.SelectedIndex].Selected = !s.Issues[s.SelectedIndex].Selected
				}
			} else if s.Focus == 1 && s.ContainersSupported {
				// Toggle autonomous mode
				s.Autonomous = !s.Autonomous
				// Autonomous mode requires containers
				if s.Autonomous {
					s.UseContainers = true
				}
			} else if s.Focus == 2 && s.ContainersSupported && !s.Autonomous {
				// Toggle container mode (only if not autonomous)
				s.UseContainers = !s.UseContainers
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

// GetUseContainers returns whether container mode is selected
func (s *ImportIssuesState) GetUseContainers() bool {
	return s.UseContainers || s.Autonomous
}

// GetAutonomous returns whether autonomous mode is selected
func (s *ImportIssuesState) GetAutonomous() bool {
	return s.Autonomous
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
func NewImportIssuesState(repoPath, repoName string, containersSupported, containerAuthAvailable bool) *ImportIssuesState {
	return &ImportIssuesState{
		RepoPath:               repoPath,
		RepoName:               repoName,
		Loading:                true,
		SelectedIndex:          0,
		ScrollOffset:           0,
		maxVisible:             IssuesModalMaxVisible,
		Source:                 "github",
		ContainersSupported:    containersSupported,
		ContainerAuthAvailable: containerAuthAvailable,
		Focus:                  0,
		availableWidth:         ModalWidthWide, // Default, will be updated by SetSize()
	}
}

// NewImportIssuesStateWithSource creates a new ImportIssuesState for a specific source.
func NewImportIssuesStateWithSource(repoPath, repoName, source, projectID string, containersSupported, containerAuthAvailable bool) *ImportIssuesState {
	return &ImportIssuesState{
		RepoPath:               repoPath,
		RepoName:               repoName,
		Loading:                true,
		SelectedIndex:          0,
		ScrollOffset:           0,
		maxVisible:             IssuesModalMaxVisible,
		Source:                 source,
		ProjectID:              projectID,
		ContainersSupported:    containersSupported,
		ContainerAuthAvailable: containerAuthAvailable,
		Focus:                  0,
		availableWidth:         ModalWidthWide, // Default, will be updated by SetSize()
	}
}
