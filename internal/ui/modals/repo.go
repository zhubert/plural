package modals

import (
	"path/filepath"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zhubert/plural/internal/keys"
)

// =============================================================================
// AddRepoState - State for the Add Repository modal
// =============================================================================

type AddRepoState struct {
	Input              textinput.Model
	SuggestedRepo      string
	UseSuggested       bool
	ReturnToNewSession bool           // When true, return to new session modal after adding repo
	completer          *PathCompleter // Path auto-completion
	lastValue          string         // Track input changes to reset completer
	showingOptions     bool           // Whether we're showing completion options
	completionIndex    int            // Currently selected completion option
}

func (*AddRepoState) modalState() {}

func (s *AddRepoState) Title() string { return "Add Repository" }

func (s *AddRepoState) Help() string {
	if s.showingOptions {
		return "up/down to select, Tab/Enter to confirm, Esc to cancel"
	}
	if s.SuggestedRepo != "" {
		return "up/down to switch, Tab to complete path, Enter to confirm, Esc to cancel"
	}
	return "Tab to complete path, Enter to confirm, Esc to cancel"
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

	// Show completion options if available
	if s.showingOptions {
		completions := s.completer.GetCompletions()
		if len(completions) > 0 {
			optionsLabel := lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				MarginTop(1).
				Render("Completions:")

			// Render completion options (show basename for cleaner display)
			options := s.renderCompletionOptions(completions)
			content = lipgloss.JoinVertical(lipgloss.Left, content, optionsLabel, options)
		}
	}

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, content, help)
}

// renderCompletionOptions renders the list of completion options with selection indicator
func (s *AddRepoState) renderCompletionOptions(completions []string) string {
	maxDisplay := 5 // Show at most 5 options to keep modal compact
	start := 0
	if s.completionIndex >= maxDisplay {
		start = s.completionIndex - maxDisplay + 1
	}
	end := min(start+maxDisplay, len(completions))

	var lines []string
	for i := start; i < end; i++ {
		c := completions[i]
		// Show just the basename for cleaner display
		display := filepath.Base(c)
		if c[len(c)-1] == '/' {
			display = filepath.Base(c[:len(c)-1]) + "/"
		}

		style := SidebarItemStyle
		prefix := "  "
		if i == s.completionIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}
		lines = append(lines, style.Render(prefix+display))
	}

	// Show scroll indicator if there are more options
	if len(completions) > maxDisplay {
		indicator := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("  (" + string(rune('0'+len(completions))) + " total, scroll with up/down)")
		if len(completions) >= 10 {
			indicator = lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Italic(true).
				Render("  (" + formatInt(len(completions)) + " total, scroll with up/down)")
		}
		lines = append(lines, indicator)
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// formatInt converts an int to string (simple helper to avoid strconv import for small numbers)
func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func (s *AddRepoState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		key := keyMsg.String()

		// When showing completion options, handle navigation differently
		if s.showingOptions {
			completions := s.completer.GetCompletions()
			switch key {
			case keys.Up, "k":
				if s.completionIndex > 0 {
					s.completionIndex--
				}
				return s, nil
			case keys.Down, "j":
				if s.completionIndex < len(completions)-1 {
					s.completionIndex++
				}
				return s, nil
			case keys.Tab, keys.Enter:
				// Select the current completion
				if s.completionIndex < len(completions) {
					selected := completions[s.completionIndex]
					s.Input.SetValue(selected)
					s.Input.CursorEnd()
					s.lastValue = selected
					s.showingOptions = false
					s.completer.Reset()
				}
				return s, nil
			case keys.Escape:
				// Hide options but don't close modal
				s.showingOptions = false
				s.completer.Reset()
				return s, nil
			default:
				// Any other key exits option selection and updates input
				s.showingOptions = false
				s.completer.Reset()
				// Fall through to normal input handling
			}
		}

		// Handle up/down/tab to switch between suggested and custom input (when not showing options)
		if !s.showingOptions && s.SuggestedRepo != "" && (key == keys.Up || key == keys.Down || key == keys.Tab) {
			// Tab when on suggested switches to input; tab when on input triggers completion (handled below)
			if key == keys.Tab && !s.UseSuggested {
				// Fall through to completion handling below
			} else {
				s.UseSuggested = !s.UseSuggested
				if s.UseSuggested {
					s.Input.Blur()
				} else {
					s.Input.Focus()
				}
				return s, nil
			}
		}

		// Handle Tab for path completion (only when in custom input mode)
		if key == keys.Tab && !s.UseSuggested && !s.showingOptions {
			currentValue := s.Input.Value()
			s.completer.GenerateCompletions(currentValue)
			completions := s.completer.GetCompletions()

			if len(completions) == 0 {
				// No matches
				return s, nil
			} else if len(completions) == 1 {
				// Single match - complete directly
				s.Input.SetValue(completions[0])
				s.Input.CursorEnd()
				s.lastValue = completions[0]
				s.completer.Reset()
			} else {
				// Multiple matches - try common prefix first
				common := s.completer.GetCommonPrefix()
				if common != currentValue && common != "" {
					// Complete to common prefix
					s.Input.SetValue(common)
					s.Input.CursorEnd()
					s.lastValue = common
					// Regenerate completions for the new prefix
					s.completer.GenerateCompletions(common)
				}
				// Show options if still multiple matches
				if len(s.completer.GetCompletions()) > 1 {
					s.showingOptions = true
					s.completionIndex = 0
				}
			}
			return s, nil
		}
	}

	// Only update text input when it's focused and not showing options
	if !s.UseSuggested && !s.showingOptions {
		var cmd tea.Cmd
		s.Input, cmd = s.Input.Update(msg)

		// Reset completer if input changed (not via tab completion)
		if s.Input.Value() != s.lastValue {
			s.completer.Reset()
			s.showingOptions = false
			s.lastValue = s.Input.Value()
		}

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

// IsShowingOptions returns true if completion options are being displayed
func (s *AddRepoState) IsShowingOptions() bool {
	return s.showingOptions
}

// NewAddRepoState creates a new AddRepoState with proper initialization
func NewAddRepoState(suggestedRepo string) *AddRepoState {
	ti := textinput.New()
	ti.Placeholder = "/path/to/repo or /path/to/* (glob)"
	ti.CharLimit = ModalInputCharLimit
	ti.SetWidth(ModalInputWidth)

	state := &AddRepoState{
		Input:           ti,
		SuggestedRepo:   suggestedRepo,
		UseSuggested:    suggestedRepo != "",
		completer:       NewPathCompleter(),
		lastValue:       "",
		showingOptions:  false,
		completionIndex: 0,
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
		repoList = RenderSelectableList(s.RepoOptions, s.RepoIndex)
	}

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, repoLabel, repoList, help)
}

func (s *SelectRepoForIssuesState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case keys.Up, "k":
			if s.RepoIndex > 0 {
				s.RepoIndex--
			}
		case keys.Down, "j":
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
