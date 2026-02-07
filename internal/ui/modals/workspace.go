package modals

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
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
	NameInput   textinput.Model
	IsRename    bool   // true if renaming an existing workspace
	WorkspaceID string // set when renaming
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

	label := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Name:")

	inputStyle := lipgloss.NewStyle().
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorPrimary).
		PaddingLeft(1)
	inputView := inputStyle.Render(s.NameInput.View())

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, label, inputView, help)
}

func (s *NewWorkspaceState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	var cmd tea.Cmd
	s.NameInput, cmd = s.NameInput.Update(msg)
	return s, cmd
}

// GetName returns the workspace name input value
func (s *NewWorkspaceState) GetName() string {
	return s.NameInput.Value()
}

// NewNewWorkspaceState creates a state for creating a new workspace
func NewNewWorkspaceState() *NewWorkspaceState {
	input := textinput.New()
	input.Placeholder = "e.g., Feature Work, Bug Fixes"
	input.CharLimit = SessionNameCharLimit
	input.SetWidth(ModalInputWidth)
	input.Focus()

	return &NewWorkspaceState{
		NameInput: input,
	}
}

// NewRenameWorkspaceState creates a state for renaming an existing workspace
func NewRenameWorkspaceState(workspaceID, currentName string) *NewWorkspaceState {
	input := textinput.New()
	input.Placeholder = "New name"
	input.CharLimit = SessionNameCharLimit
	input.SetWidth(ModalInputWidth)
	input.SetValue(currentName)
	input.Focus()

	return &NewWorkspaceState{
		NameInput:   input,
		IsRename:    true,
		WorkspaceID: workspaceID,
	}
}
