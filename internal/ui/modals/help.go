package modals

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// =============================================================================
// HelpState - State for the Help modal with keyboard shortcuts
// =============================================================================

type HelpState struct {
	Sections      []HelpSection
	ScrollOffset  int
	SelectedIndex int          // Currently selected shortcut index (flattened across all sections)
	FlatShortcuts []HelpShortcut // Flattened list of all shortcuts for selection
	totalLines    int
	maxVisible    int
}

func (*HelpState) modalState() {}

func (s *HelpState) Title() string { return "Keyboard Shortcuts" }

func (s *HelpState) Help() string {
	return "up/down navigate  Enter: trigger  Esc: close"
}

func (s *HelpState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Build all lines first to enable scrolling
	// Track which flattened shortcut index each line corresponds to (-1 for non-shortcut lines)
	var allLines []string
	var lineToShortcutIndex []int
	flatIdx := 0

	for i, section := range s.Sections {
		if i > 0 {
			allLines = append(allLines, "") // Blank line between sections
			lineToShortcutIndex = append(lineToShortcutIndex, -1)
		}

		sectionTitle := lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSecondary).
			Render(section.Title)
		allLines = append(allLines, sectionTitle)
		lineToShortcutIndex = append(lineToShortcutIndex, -1)

		for _, shortcut := range section.Shortcuts {
			isSelected := flatIdx == s.SelectedIndex

			var key, desc string
			if isSelected {
				// Highlight the selected shortcut
				key = lipgloss.NewStyle().
					Foreground(ColorTextInverse).
					Background(ColorPrimary).
					Bold(true).
					Width(12).
					Render(shortcut.Key)
				desc = lipgloss.NewStyle().
					Foreground(ColorTextInverse).
					Background(ColorPrimary).
					Render(shortcut.Desc)
				allLines = append(allLines, "> "+key+desc)
			} else {
				key = lipgloss.NewStyle().
					Foreground(ColorPrimary).
					Bold(true).
					Width(12).
					Render(shortcut.Key)
				desc = lipgloss.NewStyle().
					Foreground(ColorText).
					Render(shortcut.Desc)
				allLines = append(allLines, "  "+key+desc)
			}
			lineToShortcutIndex = append(lineToShortcutIndex, flatIdx)
			flatIdx++
		}
	}

	s.totalLines = len(allLines)

	// Find which line contains the selected shortcut
	selectedLineIndex := 0
	for i, idx := range lineToShortcutIndex {
		if idx == s.SelectedIndex {
			selectedLineIndex = i
			break
		}
	}

	// Auto-scroll to keep selected item visible
	if selectedLineIndex < s.ScrollOffset {
		s.ScrollOffset = selectedLineIndex
	} else if selectedLineIndex >= s.ScrollOffset+s.maxVisible {
		s.ScrollOffset = selectedLineIndex - s.maxVisible + 1
	}

	// Apply scroll offset and limit visible lines
	var visibleLines []string
	for i, line := range allLines {
		if i < s.ScrollOffset {
			continue
		}
		if len(visibleLines) >= s.maxVisible {
			break
		}
		visibleLines = append(visibleLines, line)
	}

	content := strings.Join(visibleLines, "\n")

	// Scroll indicator
	if s.totalLines > s.maxVisible {
		scrollInfo := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			MarginTop(1).
			Render("(scroll for more)")
		content += "\n" + scrollInfo
	}

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, content, help)
}

func (s *HelpState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if s.SelectedIndex > 0 {
				s.SelectedIndex--
			}
		case "down", "j":
			if s.SelectedIndex < len(s.FlatShortcuts)-1 {
				s.SelectedIndex++
			}
		}
	}
	return s, nil
}

// GetSelectedShortcut returns the currently selected shortcut
func (s *HelpState) GetSelectedShortcut() *HelpShortcut {
	if s.SelectedIndex >= 0 && s.SelectedIndex < len(s.FlatShortcuts) {
		return &s.FlatShortcuts[s.SelectedIndex]
	}
	return nil
}

// NewHelpState creates a new HelpState with all keyboard shortcuts
func NewHelpState() *HelpState {
	sections := []HelpSection{
		{
			Title: "Navigation",
			Shortcuts: []HelpShortcut{
				{Key: "Tab", Desc: "Switch between sidebar and chat"},
				{Key: "up/down or j/k", Desc: "Navigate session list"},
				{Key: "PgUp/PgDn", Desc: "Scroll chat or session list"},
				{Key: "Enter", Desc: "Select session / Send message"},
				{Key: "/", Desc: "Search sessions"},
				{Key: "Esc", Desc: "Cancel search / Stop streaming"},
			},
		},
		{
			Title: "Sessions",
			Shortcuts: []HelpShortcut{
				{Key: "n", Desc: "Create new session"},
				{Key: "d", Desc: "Delete selected session"},
				{Key: "f", Desc: "Fork selected session"},
				{Key: "i", Desc: "Import GitHub issues"},
				{Key: "ctrl+f", Desc: "Force resume (if session in use)"},
			},
		},
		{
			Title: "Git Operations",
			Shortcuts: []HelpShortcut{
				{Key: "v", Desc: "View changes in worktree"},
				{Key: "m", Desc: "Merge to main / Create PR"},
				{Key: "c", Desc: "Commit resolved conflicts"},
			},
		},
		{
			Title: "Configuration",
			Shortcuts: []HelpShortcut{
				{Key: "a", Desc: "Add repository"},
				{Key: "s", Desc: "Manage MCP servers"},
				{Key: "t", Desc: "Change theme"},
			},
		},
		{
			Title: "Chat (when focused)",
			Shortcuts: []HelpShortcut{
				{Key: "Ctrl+V", Desc: "Paste image"},
				{Key: "Ctrl+P", Desc: "Fork detected options"},
			},
		},
		{
			Title: "Permissions (when prompted)",
			Shortcuts: []HelpShortcut{
				{Key: "y", Desc: "Allow action"},
				{Key: "n", Desc: "Deny action"},
				{Key: "a", Desc: "Always allow this tool"},
			},
		},
		{
			Title: "General",
			Shortcuts: []HelpShortcut{
				{Key: "?", Desc: "Show this help"},
				{Key: "q", Desc: "Quit application"},
			},
		},
	}

	// Build flattened list of shortcuts for navigation
	var flatShortcuts []HelpShortcut
	for _, section := range sections {
		flatShortcuts = append(flatShortcuts, section.Shortcuts...)
	}

	return &HelpState{
		Sections:      sections,
		FlatShortcuts: flatShortcuts,
		ScrollOffset:  0,
		SelectedIndex: 0,
		maxVisible:    18,
	}
}

// NewHelpStateFromSections creates a HelpState from pre-built sections.
// This allows the shortcut registry to generate sections programmatically.
func NewHelpStateFromSections(sections []HelpSection) *HelpState {
	// Build flattened list of shortcuts for navigation
	var flatShortcuts []HelpShortcut
	for _, section := range sections {
		flatShortcuts = append(flatShortcuts, section.Shortcuts...)
	}

	return &HelpState{
		Sections:      sections,
		FlatShortcuts: flatShortcuts,
		ScrollOffset:  0,
		SelectedIndex: 0,
		maxVisible:    18,
	}
}
