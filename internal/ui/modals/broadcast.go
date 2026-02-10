package modals

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/zhubert/plural/internal/keys"
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
	Repos                  []RepoItem
	SelectedIndex          int              // Currently highlighted repo
	NameInput              textinput.Model  // Session name input (optional)
	PromptInput            textarea.Model   // Multi-line prompt input
	UseContainers          bool             // Whether to run sessions in containers
	ContainersSupported    bool             // Whether the host supports Apple containers (darwin/arm64)
	ContainerAuthAvailable bool             // Whether API key credentials are available for container mode
	Focus                  int              // 0=repo list, 1=name input, 2=prompt textarea, 3=containers (if supported)
	ScrollOffset           int              // For scrolling the repo list
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

	parts := []string{title, repoLabel, countLabel, repoList, nameLabel, nameView, promptLabel, promptView}

	// Container mode checkbox (only on Apple Silicon)
	if s.ContainersSupported {
		containerLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			MarginTop(1).
			Render("Run in containers:")

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
			Render("Run Claude CLI inside Apple containers with --dangerously-skip-permissions")
		containerView := containerCheckboxStyle.Render(containerCheckbox + " " + containerDesc)

		parts = append(parts, containerLabel, containerView)

		if s.UseContainers && !s.ContainerAuthAvailable {
			authWarning := lipgloss.NewStyle().
				Foreground(ColorWarning).
				Bold(true).
				Width(50).
				PaddingLeft(2).
				Render("Requires ANTHROPIC_API_KEY or CLAUDE_CODE_OAUTH_TOKEN env var")
			parts = append(parts, authWarning)
		}
	}

	help := ModalHelpStyle.Render(s.Help())
	parts = append(parts, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
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
			case keys.Up, "k":
				if s.SelectedIndex > 0 {
					s.SelectedIndex--
					// Adjust scroll if selection is above visible area
					if s.SelectedIndex < s.ScrollOffset {
						s.ScrollOffset = s.SelectedIndex
					}
				}
				return s, nil
			case keys.Down, "j":
				if s.SelectedIndex < len(s.Repos)-1 {
					s.SelectedIndex++
					// Adjust scroll if selection is below visible area
					if s.SelectedIndex >= s.ScrollOffset+BroadcastMaxVisibleRepos {
						s.ScrollOffset = s.SelectedIndex - BroadcastMaxVisibleRepos + 1
					}
				}
				return s, nil
			case keys.Space:
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
			case keys.Tab:
				s.Focus = 1
				s.NameInput.Focus()
				return s, nil
			}
		case 1:
			// Name input focused
			switch key {
			case keys.Tab:
				s.Focus = 2
				s.NameInput.Blur()
				s.PromptInput.Focus()
				return s, nil
			case keys.ShiftTab:
				s.Focus = 0
				s.NameInput.Blur()
				return s, nil
			}
		case 2:
			// Prompt textarea focused
			switch key {
			case keys.Tab:
				if s.ContainersSupported {
					s.Focus = 3
				} else {
					s.Focus = 0
				}
				s.PromptInput.Blur()
				return s, nil
			case keys.ShiftTab:
				s.Focus = 1
				s.PromptInput.Blur()
				s.NameInput.Focus()
				return s, nil
			}
		case 3:
			// Container checkbox focused (only when supported)
			switch key {
			case keys.Space:
				s.UseContainers = !s.UseContainers
				return s, nil
			case keys.Tab:
				s.Focus = 0
				return s, nil
			case keys.ShiftTab:
				s.Focus = 2
				s.PromptInput.Focus()
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

// GetUseContainers returns whether container mode is selected
func (s *BroadcastState) GetUseContainers() bool {
	return s.UseContainers
}

// NewBroadcastState creates a new BroadcastState.
// containersSupported indicates whether the host supports Apple containers (darwin/arm64).
// containerAuthAvailable indicates whether API key credentials exist for container mode.
func NewBroadcastState(repoPaths []string, containersSupported bool, containerAuthAvailable bool) *BroadcastState {
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
		Repos:                  repos,
		SelectedIndex:          0,
		NameInput:              nameInput,
		PromptInput:            promptInput,
		ContainersSupported:    containersSupported,
		ContainerAuthAvailable: containerAuthAvailable,
		Focus:                  0, // Start focused on repo list
		ScrollOffset:           0,
	}
}

// =============================================================================
// BroadcastGroupState - State for sending to existing broadcast group sessions
// =============================================================================

// BroadcastGroupAction represents the action to perform on the broadcast group
type BroadcastGroupAction int

const (
	BroadcastActionSendPrompt BroadcastGroupAction = iota
	BroadcastActionCreatePRs
)

// SessionItem represents a session for display in the broadcast group modal
type SessionItem struct {
	ID       string
	Name     string
	RepoName string
	Selected bool
}

// BroadcastGroupState is the state for the broadcast group modal
type BroadcastGroupState struct {
	GroupID       string
	Sessions      []SessionItem
	SelectedIndex int             // Currently highlighted session
	Action        BroadcastGroupAction
	PromptInput   textarea.Model  // Multi-line prompt input (only for SendPrompt action)
	Focus         int             // 0=action selector, 1=session list, 2=prompt textarea
	ScrollOffset  int             // For scrolling the session list
}

func (*BroadcastGroupState) modalState() {}

func (s *BroadcastGroupState) Title() string { return "Broadcast Group" }

func (s *BroadcastGroupState) Help() string {
	switch s.Focus {
	case 0:
		return "left/right: action  Tab: sessions  Enter: execute  Esc: cancel"
	case 1:
		if s.Action == BroadcastActionSendPrompt {
			return "Space: toggle  Tab: prompt  a: all  n: none  Enter: execute  Esc: cancel"
		}
		return "Space: toggle  a: all  n: none  Enter: execute  Esc: cancel"
	default:
		return "Tab: action  Shift+Tab: sessions  Enter: execute  Esc: cancel"
	}
}

func (s *BroadcastGroupState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Action selector section
	actionLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Action:")

	actionOptions := s.renderActionSelector()

	// Session list section
	sessionLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginTop(1).
		Render("Sessions:")

	var sessionList string
	if len(s.Sessions) == 0 {
		sessionList = lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("No sessions in this broadcast group.")
	} else {
		sessionList = s.renderSessionList()
	}

	// Count selected sessions
	selectedCount := s.GetSelectedCount()
	countLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Italic(true).
		Render("(" + formatCount(selectedCount, len(s.Sessions)) + " selected)")

	// Build the content based on action
	parts := []string{
		title,
		actionLabel,
		actionOptions,
		sessionLabel,
		countLabel,
		sessionList,
	}

	// Only show prompt input for SendPrompt action
	if s.Action == BroadcastActionSendPrompt {
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

		parts = append(parts, promptLabel, promptView)
	}

	help := ModalHelpStyle.Render(s.Help())
	parts = append(parts, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (s *BroadcastGroupState) renderActionSelector() string {
	sendStyle := lipgloss.NewStyle().Padding(0, 1)
	prStyle := lipgloss.NewStyle().Padding(0, 1)

	if s.Focus == 0 {
		if s.Action == BroadcastActionSendPrompt {
			sendStyle = sendStyle.Background(ColorPrimary).Foreground(ColorTextInverse)
			prStyle = prStyle.Foreground(ColorTextMuted)
		} else {
			sendStyle = sendStyle.Foreground(ColorTextMuted)
			prStyle = prStyle.Background(ColorPrimary).Foreground(ColorTextInverse)
		}
	} else {
		if s.Action == BroadcastActionSendPrompt {
			sendStyle = sendStyle.Bold(true).Foreground(ColorSecondary)
			prStyle = prStyle.Foreground(ColorTextMuted)
		} else {
			sendStyle = sendStyle.Foreground(ColorTextMuted)
			prStyle = prStyle.Bold(true).Foreground(ColorSecondary)
		}
	}

	sendOption := sendStyle.Render("Send Prompt")
	prOption := prStyle.Render("Create PRs")

	return lipgloss.JoinHorizontal(lipgloss.Top, "  ", sendOption, "  ", prOption)
}

func (s *BroadcastGroupState) renderSessionList() string {
	var lines []string

	// Calculate visible range
	startIdx := s.ScrollOffset
	endIdx := startIdx + BroadcastMaxVisibleRepos
	if endIdx > len(s.Sessions) {
		endIdx = len(s.Sessions)
	}

	// Show scroll indicator at top if needed
	if startIdx > 0 {
		lines = append(lines, lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Render("  ... "+formatCount(startIdx, 0)+" more above"))
	}

	for i := startIdx; i < endIdx; i++ {
		sess := s.Sessions[i]
		style := SidebarItemStyle
		prefix := "  "
		if i == s.SelectedIndex && s.Focus == 1 {
			style = SidebarSelectedStyle
			prefix = "> "
		}

		checkbox := "[ ]"
		if sess.Selected {
			checkbox = "[x]"
		}

		// Show session name and repo name
		displayName := sess.Name
		if sess.RepoName != "" {
			displayName = fmt.Sprintf("%s (%s)", sess.Name, sess.RepoName)
		}

		lines = append(lines, style.Render(prefix+checkbox+" "+displayName))
	}

	// Show scroll indicator at bottom if needed
	if endIdx < len(s.Sessions) {
		remaining := len(s.Sessions) - endIdx
		lines = append(lines, lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Render("  ... "+formatCount(remaining, 0)+" more below"))
	}

	return strings.Join(lines, "\n")
}

func (s *BroadcastGroupState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		key := keyMsg.String()

		// Handle focus-specific keys
		switch s.Focus {
		case 0:
			// Action selector focused
			switch key {
			case keys.Left, "h":
				s.Action = BroadcastActionSendPrompt
				return s, nil
			case keys.Right, "l":
				s.Action = BroadcastActionCreatePRs
				return s, nil
			case keys.Tab:
				s.Focus = 1
				return s, nil
			}
		case 1:
			// Session list focused
			switch key {
			case keys.Up, "k":
				if s.SelectedIndex > 0 {
					s.SelectedIndex--
					if s.SelectedIndex < s.ScrollOffset {
						s.ScrollOffset = s.SelectedIndex
					}
				}
				return s, nil
			case keys.Down, "j":
				if s.SelectedIndex < len(s.Sessions)-1 {
					s.SelectedIndex++
					if s.SelectedIndex >= s.ScrollOffset+BroadcastMaxVisibleRepos {
						s.ScrollOffset = s.SelectedIndex - BroadcastMaxVisibleRepos + 1
					}
				}
				return s, nil
			case keys.Space:
				if len(s.Sessions) > 0 && s.SelectedIndex < len(s.Sessions) {
					s.Sessions[s.SelectedIndex].Selected = !s.Sessions[s.SelectedIndex].Selected
				}
				return s, nil
			case "a":
				for i := range s.Sessions {
					s.Sessions[i].Selected = true
				}
				return s, nil
			case "n":
				for i := range s.Sessions {
					s.Sessions[i].Selected = false
				}
				return s, nil
			case keys.Tab:
				if s.Action == BroadcastActionSendPrompt {
					s.Focus = 2
					s.PromptInput.Focus()
				} else {
					s.Focus = 0
				}
				return s, nil
			case keys.ShiftTab:
				s.Focus = 0
				return s, nil
			}
		case 2:
			// Prompt textarea focused
			switch key {
			case keys.Tab:
				s.Focus = 0
				s.PromptInput.Blur()
				return s, nil
			case keys.ShiftTab:
				s.Focus = 1
				s.PromptInput.Blur()
				return s, nil
			}
		}
	}

	// Forward to textarea if focused
	if s.Focus == 2 {
		var cmd tea.Cmd
		s.PromptInput, cmd = s.PromptInput.Update(msg)
		return s, cmd
	}

	return s, nil
}

// GetSelectedSessions returns the IDs of all selected sessions
func (s *BroadcastGroupState) GetSelectedSessions() []string {
	var selected []string
	for _, sess := range s.Sessions {
		if sess.Selected {
			selected = append(selected, sess.ID)
		}
	}
	return selected
}

// GetSelectedCount returns the number of selected sessions
func (s *BroadcastGroupState) GetSelectedCount() int {
	count := 0
	for _, sess := range s.Sessions {
		if sess.Selected {
			count++
		}
	}
	return count
}

// GetPrompt returns the prompt text
func (s *BroadcastGroupState) GetPrompt() string {
	return s.PromptInput.Value()
}

// GetAction returns the selected action
func (s *BroadcastGroupState) GetAction() BroadcastGroupAction {
	return s.Action
}

// NewBroadcastGroupState creates a new BroadcastGroupState
func NewBroadcastGroupState(groupID string, sessions []SessionItem) *BroadcastGroupState {
	// Select all sessions by default
	for i := range sessions {
		sessions[i].Selected = true
	}

	promptInput := textarea.New()
	promptInput.Placeholder = "Enter prompt to send to selected sessions..."
	promptInput.CharLimit = 10000
	promptInput.SetWidth(ModalWidth - 6)
	promptInput.SetHeight(4)
	promptInput.Prompt = ""

	ApplyTextareaStyles(&promptInput)

	return &BroadcastGroupState{
		GroupID:       groupID,
		Sessions:      sessions,
		SelectedIndex: 0,
		Action:        BroadcastActionSendPrompt,
		PromptInput:   promptInput,
		Focus:         0, // Start focused on action selector
		ScrollOffset:  0,
	}
}
