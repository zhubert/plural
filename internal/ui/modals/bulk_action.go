package modals

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
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
	BulkActionCreatePRs
	BulkActionSendPrompt
)

// BulkActionState is the modal for choosing a bulk action
type BulkActionState struct {
	SessionIDs    []string
	SessionCount  int
	Action        BulkAction
	Workspaces    []config.Workspace
	SelectedWSIdx int
	PromptInput   textarea.Model
}

func (*BulkActionState) modalState() {}

func (s *BulkActionState) Title() string {
	return fmt.Sprintf("Bulk Action (%d sessions)", s.SessionCount)
}

func (s *BulkActionState) Help() string {
	if s.Action == BulkActionSendPrompt {
		return "tab/shift+tab: switch action  Enter: send  Esc: cancel"
	}
	return "left/right: switch action  Enter: confirm  Esc: cancel"
}

func (s *BulkActionState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Action selector (left/right)
	actions := []string{"Delete", "Move to Workspace", "Create PRs", "Send Prompt"}
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

	// Show prompt input when "Send Prompt" is selected
	if s.Action == BulkActionSendPrompt {
		promptLabel := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			MarginTop(1).
			Render("Enter prompt:")

		promptStyle := lipgloss.NewStyle().
			BorderLeft(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(ColorPrimary).
			PaddingLeft(1)
		promptView := promptStyle.Render(s.PromptInput.View())

		parts = append(parts, promptLabel, promptView)
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
	case BulkActionCreatePRs:
		confirmMsg = fmt.Sprintf("Create PRs for %d session(s). Sessions with existing PRs or that are already merged will be skipped.", s.SessionCount)
	case BulkActionSendPrompt:
		confirmMsg = fmt.Sprintf("Send prompt to %d session(s).", s.SessionCount)
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
		key := keyMsg.String()

		// When on Send Prompt action, use tab/shift+tab for navigation
		// to avoid conflicts with arrow keys used for text editing
		if s.Action == BulkActionSendPrompt {
			switch key {
			case keys.Tab:
				// Navigate right (wrap to beginning)
				s.PromptInput.Blur()
				if s.Action < BulkActionSendPrompt {
					s.Action++
				} else {
					s.Action = BulkActionDelete
				}
				if s.Action == BulkActionSendPrompt {
					s.PromptInput.Focus()
				}
				return s, nil
			case keys.ShiftTab:
				// Navigate left (wrap to end)
				s.PromptInput.Blur()
				if s.Action > 0 {
					s.Action--
				} else {
					s.Action = BulkActionSendPrompt
				}
				if s.Action == BulkActionSendPrompt {
					s.PromptInput.Focus()
				}
				return s, nil
			default:
				// Forward all other events to textarea (including arrow keys for editing)
				var cmd tea.Cmd
				s.PromptInput, cmd = s.PromptInput.Update(msg)
				return s, cmd
			}
		}

		// For other actions, handle navigation keys (arrow keys + vim shortcuts)
		switch key {
		case keys.Left, "h":
			if s.Action > 0 {
				s.Action--
				// Focus textarea if we just switched to Send Prompt
				if s.Action == BulkActionSendPrompt {
					s.PromptInput.Focus()
				}
				return s, nil
			}
		case keys.Right, "l", keys.Tab:
			if s.Action < BulkActionSendPrompt {
				s.Action++
				// Focus textarea if we just switched to Send Prompt
				if s.Action == BulkActionSendPrompt {
					s.PromptInput.Focus()
				}
				return s, nil
			}
		case keys.ShiftTab:
			if s.Action > 0 {
				s.Action--
				// Focus textarea if we just switched to Send Prompt
				if s.Action == BulkActionSendPrompt {
					s.PromptInput.Focus()
				}
				return s, nil
			}
		}

		// Handle workspace navigation when in Move action
		if s.Action == BulkActionMoveToWorkspace {
			switch key {
			case keys.Up, "k":
				if s.SelectedWSIdx > 0 {
					s.SelectedWSIdx--
				}
				return s, nil
			case keys.Down, "j":
				if s.SelectedWSIdx < len(s.Workspaces)-1 {
					s.SelectedWSIdx++
				}
				return s, nil
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

// GetPrompt returns the prompt text for send prompt action
func (s *BulkActionState) GetPrompt() string {
	return strings.TrimSpace(s.PromptInput.Value())
}

// NewBulkActionState creates a new BulkActionState
func NewBulkActionState(sessionIDs []string, workspaces []config.Workspace) *BulkActionState {
	promptInput := textarea.New()
	promptInput.Placeholder = "Enter your prompt here..."
	promptInput.CharLimit = 10000
	promptInput.ShowLineNumbers = false
	promptInput.SetWidth(ModalWidth - 6) // Account for padding/borders
	promptInput.SetHeight(4)
	promptInput.Prompt = "" // Remove default prompt to avoid double bar with focus border
	// Don't focus immediately - focus when user navigates to Send Prompt action

	// Apply transparent background styles
	ApplyTextareaStyles(&promptInput)

	return &BulkActionState{
		SessionIDs:   sessionIDs,
		SessionCount: len(sessionIDs),
		Action:       BulkActionDelete,
		Workspaces:   workspaces,
		PromptInput:  promptInput,
	}
}
