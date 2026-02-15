package modals

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/keys"
)

// =============================================================================
// WorkspaceListState - State for the Workspace manager modal
// =============================================================================

type WorkspaceListState struct {
	Workspaces        []config.Workspace
	SessionCounts     map[string]int // workspace ID -> session count
	SelectedIndex     int
	ActiveWorkspaceID string
}

func (*WorkspaceListState) modalState() {}

func (s *WorkspaceListState) Title() string { return "Workspaces" }

func (s *WorkspaceListState) Help() string {
	return "n: new  d: delete  r: rename  Enter: switch  Esc: close"
}

func (s *WorkspaceListState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	var content strings.Builder

	// "All Sessions" entry always at top
	allStyle := SidebarItemStyle
	allPrefix := "  "
	allSuffix := ""
	if s.SelectedIndex == 0 {
		allStyle = SidebarSelectedStyle
		allPrefix = "> "
	}
	if s.ActiveWorkspaceID == "" {
		allSuffix = " (active)"
	}
	content.WriteString(allStyle.Render(allPrefix+"All Sessions"+allSuffix) + "\n")

	// Workspace entries
	for i, ws := range s.Workspaces {
		idx := i + 1 // offset by 1 for "All Sessions"
		style := SidebarItemStyle
		prefix := "  "
		suffix := ""

		if idx == s.SelectedIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}

		if ws.ID == s.ActiveWorkspaceID {
			suffix = " (active)"
		}

		count := s.SessionCounts[ws.ID]
		label := fmt.Sprintf("%s%s  %d sessions%s", prefix, ws.Name, count, suffix)
		content.WriteString(style.Render(label) + "\n")
	}

	if len(s.Workspaces) == 0 {
		empty := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("  No workspaces. Press 'n' to create one.")
		content.WriteString(empty + "\n")
	}

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, content.String(), help)
}

func (s *WorkspaceListState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		totalItems := 1 + len(s.Workspaces) // "All" + workspaces
		switch keyMsg.String() {
		case keys.Up, "k":
			if s.SelectedIndex > 0 {
				s.SelectedIndex--
			}
		case keys.Down, "j":
			if s.SelectedIndex < totalItems-1 {
				s.SelectedIndex++
			}
		}
	}
	return s, nil
}

// GetSelectedWorkspaceID returns the ID of the selected workspace,
// or empty string if "All Sessions" is selected.
func (s *WorkspaceListState) GetSelectedWorkspaceID() string {
	if s.SelectedIndex == 0 {
		return ""
	}
	wsIdx := s.SelectedIndex - 1
	if wsIdx >= 0 && wsIdx < len(s.Workspaces) {
		return s.Workspaces[wsIdx].ID
	}
	return ""
}

// GetSelectedWorkspaceName returns the name of the selected workspace.
func (s *WorkspaceListState) GetSelectedWorkspaceName() string {
	if s.SelectedIndex == 0 {
		return ""
	}
	wsIdx := s.SelectedIndex - 1
	if wsIdx >= 0 && wsIdx < len(s.Workspaces) {
		return s.Workspaces[wsIdx].Name
	}
	return ""
}

// IsAllSessionsSelected returns true if "All Sessions" is the current selection.
func (s *WorkspaceListState) IsAllSessionsSelected() bool {
	return s.SelectedIndex == 0
}

// NewWorkspaceListState creates a new WorkspaceListState
func NewWorkspaceListState(workspaces []config.Workspace, sessionCounts map[string]int, activeWorkspaceID string) *WorkspaceListState {
	// Find the selected index that corresponds to the active workspace
	selectedIndex := 0 // default to "All Sessions"
	if activeWorkspaceID != "" {
		for i, ws := range workspaces {
			if ws.ID == activeWorkspaceID {
				selectedIndex = i + 1 // offset by 1 for "All Sessions"
				break
			}
		}
	}

	return &WorkspaceListState{
		Workspaces:        workspaces,
		SessionCounts:     sessionCounts,
		SelectedIndex:     selectedIndex,
		ActiveWorkspaceID: activeWorkspaceID,
	}
}

// =============================================================================
// NewWorkspaceState - State for creating/renaming a workspace
// =============================================================================

type NewWorkspaceState struct {
	IsRename    bool   // true if renaming an existing workspace
	WorkspaceID string // set when renaming

	form        *huh.Form
	initialized bool
	name        string
}

func (*NewWorkspaceState) modalState() {}

func (s *NewWorkspaceState) Title() string {
	if s.IsRename {
		return "Rename Workspace"
	}
	return "New Workspace"
}

func (s *NewWorkspaceState) Help() string {
	return "Enter: save  Esc: cancel"
}

func (s *NewWorkspaceState) Render() string {
	title := ModalTitleStyle.Render(s.Title())
	help := ModalHelpStyle.Render(s.Help())
	return lipgloss.JoinVertical(lipgloss.Left, title, s.form.View(), help)
}

func (s *NewWorkspaceState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	var cmd tea.Cmd
	s.form, cmd = huhFormUpdate(s.form, &s.initialized, msg)
	return s, cmd
}

// GetName returns the workspace name input value
func (s *NewWorkspaceState) GetName() string {
	return s.name
}

func newWorkspaceForm(s *NewWorkspaceState, placeholder string) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Name").
				Placeholder(placeholder).
				CharLimit(SessionNameCharLimit).
				Value(&s.name),
		),
	).WithTheme(ModalTheme()).
		WithShowHelp(false).
		WithWidth(ModalInputWidth)
}

// NewNewWorkspaceState creates a state for creating a new workspace
func NewNewWorkspaceState() *NewWorkspaceState {
	s := &NewWorkspaceState{}
	s.form = newWorkspaceForm(s, "e.g., Feature Work, Bug Fixes")
	s.initialized = true
	initHuhForm(s.form)
	return s
}

// NewRenameWorkspaceState creates a state for renaming an existing workspace
func NewRenameWorkspaceState(workspaceID, currentName string) *NewWorkspaceState {
	s := &NewWorkspaceState{
		IsRename:    true,
		WorkspaceID: workspaceID,
		name:        currentName,
	}
	s.form = newWorkspaceForm(s, "New name")
	s.initialized = true
	initHuhForm(s.form)
	return s
}
