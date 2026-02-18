package modals

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zhubert/plural/internal/keys"
)

// =============================================================================
// ExploreOptionsState - State for the Explore Options modal (parallel sessions)
// =============================================================================

type ExploreOptionsState struct {
	ParentSessionName string
	Options           []OptionItem
	SelectedIndex     int // Currently highlighted option
}

func (*ExploreOptionsState) modalState() {}

func (s *ExploreOptionsState) Title() string { return "Fork Options" }

func (s *ExploreOptionsState) Help() string {
	return "up/down navigate  Space: toggle  Enter: create forks  Esc: cancel"
}

func (s *ExploreOptionsState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Parent session info (consistent with ForkSessionState)
	parentLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Forking from:")

	parentName := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true).
		MarginBottom(1).
		Render("  " + s.ParentSessionName)

	description := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginBottom(1).
		Render("Select options to explore in parallel forks:")

	var optionList strings.Builder
	lastGroupIndex := -1
	for i, opt := range s.Options {
		// Add separator between groups
		if lastGroupIndex != -1 && opt.GroupIndex != lastGroupIndex {
			separatorStyle := lipgloss.NewStyle().
				Foreground(ColorTextMuted)
			optionList.WriteString(separatorStyle.Render("    ───────────────────────────────────────") + "\n")
		}
		lastGroupIndex = opt.GroupIndex

		style := SidebarItemStyle
		prefix := "  "
		checkbox := "[ ]"

		if i == s.SelectedIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}

		if opt.Selected {
			checkbox = "[x]"
		}

		// Truncate long option text
		text := opt.Text
		if len(text) > 50 {
			text = text[:47] + "..."
		}

		// Use letter label if present, otherwise use number
		var label string
		if opt.Letter != "" {
			label = opt.Letter
		} else {
			label = fmt.Sprintf("%d", opt.Number)
		}
		optionLine := fmt.Sprintf("%s %s. %s", checkbox, label, text)
		optionList.WriteString(style.Render(prefix+optionLine) + "\n")
	}

	// Show count of selected options
	selectedCount := 0
	for _, opt := range s.Options {
		if opt.Selected {
			selectedCount++
		}
	}

	countStyle := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		MarginTop(1)
	countText := fmt.Sprintf("%d option(s) selected", selectedCount)
	if selectedCount > 0 {
		countText += " - will create " + fmt.Sprintf("%d", selectedCount) + " fork(s)"
	}
	countSection := countStyle.Render(countText)

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, parentLabel, parentName, description, optionList.String(), countSection, help)
}

func (s *ExploreOptionsState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case keys.Up, "k":
			if s.SelectedIndex > 0 {
				s.SelectedIndex--
			}
		case keys.Down, "j":
			if s.SelectedIndex < len(s.Options)-1 {
				s.SelectedIndex++
			}
		case keys.Space:
			// Toggle selection
			if s.SelectedIndex < len(s.Options) {
				s.Options[s.SelectedIndex].Selected = !s.Options[s.SelectedIndex].Selected
			}
		}
	}
	return s, nil
}

// GetSelectedOptions returns the options that are selected
func (s *ExploreOptionsState) GetSelectedOptions() []OptionItem {
	var selected []OptionItem
	for _, opt := range s.Options {
		if opt.Selected {
			selected = append(selected, opt)
		}
	}
	return selected
}

// NewExploreOptionsState creates a new ExploreOptionsState
func NewExploreOptionsState(parentSessionName string, options []OptionItem) *ExploreOptionsState {
	return &ExploreOptionsState{
		ParentSessionName: parentSessionName,
		Options:           options,
		SelectedIndex:     0,
	}
}
