package modals

import (
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
)

// =============================================================================
// HelpState - State for the Help modal with keyboard shortcuts (bubbles list)
// =============================================================================

// helpShortcutItem wraps a HelpShortcut for use in a bubbles list.
type helpShortcutItem struct {
	shortcut HelpShortcut
}

func (i helpShortcutItem) FilterValue() string {
	return i.shortcut.Key + " " + i.shortcut.Desc
}

// helpSectionItem represents a section header in the list.
// It is not selectable and not filterable.
type helpSectionItem struct {
	title string
}

func (i helpSectionItem) FilterValue() string { return "" }

// helpDelegate renders help list items with the existing styling.
type helpDelegate struct{}

func (d helpDelegate) Height() int                              { return 1 }
func (d helpDelegate) Spacing() int                             { return 0 }
func (d helpDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d helpDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	switch i := item.(type) {
	case helpSectionItem:
		title := lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSecondary).
			Render(i.title)
		fmt.Fprint(w, title)

	case helpShortcutItem:
		isSelected := index == m.Index()
		var key, desc string
		if isSelected {
			key = lipgloss.NewStyle().
				Foreground(ColorTextInverse).
				Background(ColorPrimary).
				Bold(true).
				Width(16).
				Render(i.shortcut.Key)
			desc = lipgloss.NewStyle().
				Foreground(ColorTextInverse).
				Background(ColorPrimary).
				Render(i.shortcut.Desc)
			fmt.Fprint(w, "> "+key+desc)
		} else {
			key = lipgloss.NewStyle().
				Foreground(ColorPrimary).
				Bold(true).
				Width(16).
				Render(i.shortcut.Key)
			desc = lipgloss.NewStyle().
				Foreground(ColorText).
				Render(i.shortcut.Desc)
			fmt.Fprint(w, "  "+key+desc)
		}
	}
}

// HelpState wraps a bubbles list.Model for the help modal.
type HelpState struct {
	list list.Model
}

func (*HelpState) modalState() {}

func (s *HelpState) Title() string { return "Keyboard Shortcuts" }

func (s *HelpState) Help() string {
	if s.list.SettingFilter() {
		return "Type to filter  Enter: apply  Esc: cancel"
	}
	return "/: filter  up/down: navigate  Enter: trigger  Esc: close"
}

func (s *HelpState) Render() string {
	title := ModalTitleStyle.Render(s.Title())
	content := s.list.View()
	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, content, help)
}

func (s *HelpState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

// SetSize implements ModalWithSize so the modal framework passes dimensions.
func (s *HelpState) SetSize(width, height int) {
	// Reserve space for title (1 line + margin) and help text (1 line + margin)
	const titleAndHelpOverhead = 4
	listHeight := height - titleAndHelpOverhead
	if listHeight < 1 {
		listHeight = 1
	}
	s.list.SetSize(width, listHeight)
}

// GetSelectedShortcut returns the currently selected shortcut.
// Returns nil if a section header is selected or the list is empty.
func (s *HelpState) GetSelectedShortcut() *HelpShortcut {
	item := s.list.SelectedItem()
	if item == nil {
		return nil
	}
	if si, ok := item.(helpShortcutItem); ok {
		return &si.shortcut
	}
	return nil
}

// IsFiltering returns whether the user is currently typing in the filter.
func (s *HelpState) IsFiltering() bool {
	return s.list.SettingFilter()
}

// NewHelpStateFromSections creates a HelpState from pre-built sections.
// This allows the shortcut registry to generate sections programmatically.
func NewHelpStateFromSections(sections []HelpSection) *HelpState {
	// Build list items: interleave section headers with shortcuts
	var items []list.Item
	for _, section := range sections {
		items = append(items, helpSectionItem{title: section.Title})
		for _, shortcut := range section.Shortcuts {
			items = append(items, helpShortcutItem{shortcut: shortcut})
		}
	}

	l := list.New(items, helpDelegate{}, ModalWidth, HelpModalMaxVisible)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.DisableQuitKeybindings()
	l.SetFilteringEnabled(true)

	// Start selection on the first shortcut item (skip any leading section header)
	for i, item := range items {
		if _, ok := item.(helpShortcutItem); ok {
			l.Select(i)
			break
		}
	}

	return &HelpState{list: l}
}
