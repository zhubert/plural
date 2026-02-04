package modals

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// BroadcastMaxVisibleRepos is the maximum number of repos visible before scrolling
const BroadcastMaxVisibleRepos = 6

// RepoItem represents a repository for selection in the broadcast modal
type RepoItem struct {
	Path     string
	Name     string
	Selected bool
}

// BroadcastState is the state for the broadcast modal
type BroadcastState struct {
	Repos         []RepoItem
	SelectedIndex int              // Currently highlighted repo
	NameInput     textinput.Model  // Session name input (optional)
	PromptInput   textarea.Model   // Multi-line prompt input
	Focus         int              // 0=repo list, 1=name input, 2=prompt textarea
	ScrollOffset  int              // For scrolling the repo list
}

func (*BroadcastState) modalState() {}

func (s *BroadcastState) Title() string { return "Broadcast to Repositories" }

func (s *BroadcastState) Help() string {
	switch s.Focus {
	case 0:
		return "Space: toggle  Tab: name  a: all  n: none  Enter: send  Esc: cancel"
	case 1:
		return "Tab: prompt  Shift+Tab: repos  Enter: send  Esc: cancel"
	default:
		return "Tab: repos  Shift+Tab: name  Enter: send  Esc: cancel"
	}
}

func (s *BroadcastState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Repository selection section
	repoLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Select repositories:")

	var repoList string
	if len(s.Repos) == 0 {
		repoList = lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("No repositories added. Press 'a' to add one first.")
	} else {
		repoList = s.renderRepoList()
	}

	// Count selected repos
	selectedCount := s.GetSelectedCount()
	countLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Italic(true).
		Render("(" + formatCount(selectedCount, len(s.Repos)) + " selected)")

	// Session name input section
	nameLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginTop(1).
		Render("Session name (optional):")

	nameStyle := lipgloss.NewStyle()
	if s.Focus == 1 {
		nameStyle = nameStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
	} else {
		nameStyle = nameStyle.PaddingLeft(2)
	}
	nameView := nameStyle.Render(s.NameInput.View())

	// Prompt input section
	promptLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginTop(1).
		Render("Prompt:")

	promptStyle := lipgloss.NewStyle()
	if s.Focus == 2 {
		promptStyle = promptStyle.BorderLeft(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(ColorPrimary).PaddingLeft(1)
	} else {
		promptStyle = promptStyle.PaddingLeft(2)
	}
	promptView := promptStyle.Render(s.PromptInput.View())

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		repoLabel,
		countLabel,
		repoList,
		nameLabel,
		nameView,
		promptLabel,
		promptView,
		help,
	)
}

func (s *BroadcastState) renderRepoList() string {
	var lines []string

	// Calculate visible range
	startIdx := s.ScrollOffset
	endIdx := startIdx + BroadcastMaxVisibleRepos
	if endIdx > len(s.Repos) {
		endIdx = len(s.Repos)
	}

	// Show scroll indicator at top if needed
	if startIdx > 0 {
		lines = append(lines, lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Render("  ... "+formatCount(startIdx, 0)+" more above"))
	}

	for i := startIdx; i < endIdx; i++ {
		repo := s.Repos[i]
		style := SidebarItemStyle
		prefix := "  "
		if i == s.SelectedIndex && s.Focus == 0 {
			style = SidebarSelectedStyle
			prefix = "> "
		}

		checkbox := "[ ]"
		if repo.Selected {
			checkbox = "[x]"
		}

		lines = append(lines, style.Render(prefix+checkbox+" "+repo.Name))
	}

	// Show scroll indicator at bottom if needed
	if endIdx < len(s.Repos) {
		remaining := len(s.Repos) - endIdx
		lines = append(lines, lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Render("  ... "+formatCount(remaining, 0)+" more below"))
	}

	return strings.Join(lines, "\n")
}

func formatCount(count, total int) string {
	if total > 0 {
		return lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("%d", count)) + fmt.Sprintf("/%d", total)
	}
	return fmt.Sprintf("%d", count)
}

func (s *BroadcastState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		key := keyMsg.String()

		// Handle focus-specific keys
		switch s.Focus {
		case 0:
			// Repo list focused
			switch key {
			case "up", "k":
				if s.SelectedIndex > 0 {
					s.SelectedIndex--
					// Adjust scroll if selection is above visible area
					if s.SelectedIndex < s.ScrollOffset {
						s.ScrollOffset = s.SelectedIndex
					}
				}
				return s, nil
			case "down", "j":
				if s.SelectedIndex < len(s.Repos)-1 {
					s.SelectedIndex++
					// Adjust scroll if selection is below visible area
					if s.SelectedIndex >= s.ScrollOffset+BroadcastMaxVisibleRepos {
						s.ScrollOffset = s.SelectedIndex - BroadcastMaxVisibleRepos + 1
					}
				}
				return s, nil
			case "space":
				if len(s.Repos) > 0 && s.SelectedIndex < len(s.Repos) {
					s.Repos[s.SelectedIndex].Selected = !s.Repos[s.SelectedIndex].Selected
				}
				return s, nil
			case "a":
				// Select all
				for i := range s.Repos {
					s.Repos[i].Selected = true
				}
				return s, nil
			case "n":
				// Select none
				for i := range s.Repos {
					s.Repos[i].Selected = false
				}
				return s, nil
			case "tab":
				s.Focus = 1
				s.NameInput.Focus()
				return s, nil
			}
		case 1:
			// Name input focused
			switch key {
			case "tab":
				s.Focus = 2
				s.NameInput.Blur()
				s.PromptInput.Focus()
				return s, nil
			case "shift+tab":
				s.Focus = 0
				s.NameInput.Blur()
				return s, nil
			}
		case 2:
			// Prompt textarea focused
			switch key {
			case "tab":
				s.Focus = 0
				s.PromptInput.Blur()
				return s, nil
			case "shift+tab":
				s.Focus = 1
				s.PromptInput.Blur()
				s.NameInput.Focus()
				return s, nil
			}
		}
	}

	// Forward to name input if focused
	if s.Focus == 1 {
		var cmd tea.Cmd
		s.NameInput, cmd = s.NameInput.Update(msg)
		return s, cmd
	}

	// Forward to textarea if focused
	if s.Focus == 2 {
		var cmd tea.Cmd
		s.PromptInput, cmd = s.PromptInput.Update(msg)
		return s, cmd
	}

	return s, nil
}

// GetSelectedRepos returns the paths of all selected repositories
func (s *BroadcastState) GetSelectedRepos() []string {
	var selected []string
	for _, repo := range s.Repos {
		if repo.Selected {
			selected = append(selected, repo.Path)
		}
	}
	return selected
}

// GetSelectedCount returns the number of selected repositories
func (s *BroadcastState) GetSelectedCount() int {
	count := 0
	for _, repo := range s.Repos {
		if repo.Selected {
			count++
		}
	}
	return count
}

// GetName returns the session name (may be empty)
func (s *BroadcastState) GetName() string {
	return s.NameInput.Value()
}

// GetPrompt returns the prompt text
func (s *BroadcastState) GetPrompt() string {
	return s.PromptInput.Value()
}

// NewBroadcastState creates a new BroadcastState
func NewBroadcastState(repoPaths []string) *BroadcastState {
	repos := make([]RepoItem, len(repoPaths))
	for i, path := range repoPaths {
		repos[i] = RepoItem{
			Path:     path,
			Name:     filepath.Base(path),
			Selected: false,
		}
	}

	nameInput := textinput.New()
	nameInput.Placeholder = "Leave empty for auto-generated name"
	nameInput.CharLimit = 100
	nameInput.SetWidth(ModalWidth - 6) // Account for padding/borders

	promptInput := textarea.New()
	promptInput.Placeholder = "Enter prompt to send to all selected repos..."
	promptInput.CharLimit = 10000
	promptInput.SetWidth(ModalWidth - 6) // Account for padding/borders
	promptInput.SetHeight(4)
	promptInput.Prompt = "" // Remove default prompt to avoid double bar with focus border

	// Apply transparent background styles
	ApplyTextareaStyles(&promptInput)

	return &BroadcastState{
		Repos:         repos,
		SelectedIndex: 0,
		NameInput:     nameInput,
		PromptInput:   promptInput,
		Focus:         0, // Start focused on repo list
		ScrollOffset:  0,
	}
}
