package modals

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/keys"
)

// BulkAction represents a bulk operation type
type BulkAction int

const (
	BulkActionDelete          BulkAction = iota
	BulkActionMoveToWorkspace
)

// BulkActionState is the modal for choosing a bulk action
type BulkActionState struct {
	SessionIDs    []string
	SessionCount  int
	Action        BulkAction
	Workspaces    []config.Workspace
	SelectedWSIdx int
}

func (*BulkActionState) modalState() {}

func (s *BulkActionState) Title() string {
	return fmt.Sprintf("Bulk Action (%d sessions)", s.SessionCount)
}

func (s *BulkActionState) Help() string {
	return "left/right: switch action  Enter: confirm  Esc: cancel"
}

func (s *BulkActionState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Action selector (left/right)
	actions := []string{"Delete", "Move to Workspace"}
	var actionLine strings.Builder
	for i, action := range actions {
		style := SidebarItemStyle
		if i == int(s.Action) {
			style = SidebarSelectedStyle
		}
		actionLine.WriteString(style.Render(" " + action + " "))
		if i < len(actions)-1 {
			actionLine.WriteString("  ")
		}
	}

	parts := []string{title, "", actionLine.String()}

	// Show workspace list when "Move to Workspace" is selected
	if s.Action == BulkActionMoveToWorkspace {
		wsLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			MarginTop(1).
			Render("Select workspace:")

		if len(s.Workspaces) == 0 {
			empty := lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Italic(true).
				Render("  No workspaces. Create one first (W).")
			parts = append(parts, wsLabel, empty)
		} else {
			var wsList strings.Builder
			for i, ws := range s.Workspaces {
				style := SidebarItemStyle
				prefix := "  "
				if i == s.SelectedWSIdx {
					style = SidebarSelectedStyle
					prefix = "> "
				}
				wsList.WriteString(style.Render(prefix+ws.Name) + "\n")
			}
			parts = append(parts, wsLabel, wsList.String())
		}
	}

	// Confirm info
	var confirmMsg string
	switch s.Action {
	case BulkActionDelete:
		confirmMsg = fmt.Sprintf("This will delete %d session(s) and their worktrees.", s.SessionCount)
	case BulkActionMoveToWorkspace:
		if len(s.Workspaces) > 0 && s.SelectedWSIdx < len(s.Workspaces) {
			confirmMsg = fmt.Sprintf("Move %d session(s) to \"%s\".", s.SessionCount, s.Workspaces[s.SelectedWSIdx].Name)
		}
	}
	if confirmMsg != "" {
		confirmStyle := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			MarginTop(1)
		parts = append(parts, confirmStyle.Render(confirmMsg))
	}

	help := ModalHelpStyle.Render(s.Help())
	parts = append(parts, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (s *BulkActionState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case keys.Left, "h":
			if s.Action > 0 {
				s.Action--
			}
		case keys.Right, "l":
			if s.Action < BulkActionMoveToWorkspace {
				s.Action++
			}
		case keys.Up, "k":
			if s.Action == BulkActionMoveToWorkspace && s.SelectedWSIdx > 0 {
				s.SelectedWSIdx--
			}
		case keys.Down, "j":
			if s.Action == BulkActionMoveToWorkspace && s.SelectedWSIdx < len(s.Workspaces)-1 {
				s.SelectedWSIdx++
			}
		}
	}
	return s, nil
}

// GetAction returns the selected bulk action
func (s *BulkActionState) GetAction() BulkAction {
	return s.Action
}

// GetSelectedWorkspaceID returns the workspace ID for move action
func (s *BulkActionState) GetSelectedWorkspaceID() string {
	if s.SelectedWSIdx >= 0 && s.SelectedWSIdx < len(s.Workspaces) {
		return s.Workspaces[s.SelectedWSIdx].ID
	}
	return ""
}

// NewBulkActionState creates a new BulkActionState
func NewBulkActionState(sessionIDs []string, workspaces []config.Workspace) *BulkActionState {
	return &BulkActionState{
		SessionIDs:   sessionIDs,
		SessionCount: len(sessionIDs),
		Action:       BulkActionDelete,
		Workspaces:   workspaces,
	}
}
