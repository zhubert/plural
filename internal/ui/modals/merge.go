package modals

import (
	"fmt"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// =============================================================================
// MergeState - State for the Merge/PR modal
// =============================================================================

type MergeState struct {
	SessionName    string
	Options        []string
	SelectedIndex  int
	HasRemote      bool
	HasParent      bool   // Whether session has a parent it can merge to
	ParentName     string // Name of parent session (for display)
	ChangesSummary string
	PRCreated      bool // Whether a PR has already been created for this session
}

func (*MergeState) modalState() {}

func (s *MergeState) Title() string { return "Merge/PR" }

func (s *MergeState) Help() string {
	return "up/down to select, Enter to confirm, Esc to cancel"
}

func (s *MergeState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Content width for text wrapping (modal width minus padding)
	contentWidth := ModalWidth - 4

	// Show session name prominently
	sessionLabel := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true).
		MarginBottom(1).
		Width(contentWidth).
		Render(s.SessionName)

	// Show changes summary
	var summarySection string
	if s.ChangesSummary != "" {
		summaryStyle := lipgloss.NewStyle().
			Foreground(ColorSecondary).
			MarginBottom(1).
			Width(contentWidth)
		summarySection = summaryStyle.Render("Changes: " + s.ChangesSummary)
	} else {
		noChangesStyle := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			MarginBottom(1).
			Width(contentWidth)
		summarySection = noChangesStyle.Render("No uncommitted changes")
	}

	var optionList string
	for i, opt := range s.Options {
		style := SidebarItemStyle
		prefix := "  "
		if i == s.SelectedIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}
		optionList += style.Render(prefix+opt) + "\n"
	}

	if !s.HasRemote {
		note := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Width(contentWidth).
			Render("(No remote origin - PR option unavailable)")
		optionList += "\n" + note
	}

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, sessionLabel, summarySection, optionList, help)
}

func (s *MergeState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if s.SelectedIndex > 0 {
				s.SelectedIndex--
			}
		case "down", "j":
			if s.SelectedIndex < len(s.Options)-1 {
				s.SelectedIndex++
			}
		}
	}
	return s, nil
}

// GetSelectedOption returns the selected merge option
func (s *MergeState) GetSelectedOption() string {
	if len(s.Options) == 0 || s.SelectedIndex >= len(s.Options) {
		return ""
	}
	return s.Options[s.SelectedIndex]
}

// NewMergeState creates a new MergeState
// parentName should be non-empty if this session has a parent it can merge to
// prCreated should be true if a PR has already been created for this session
func NewMergeState(sessionName string, hasRemote bool, changesSummary string, parentName string, prCreated bool) *MergeState {
	var options []string

	// If session has a parent, offer merge to parent first
	hasParent := parentName != ""
	if hasParent {
		options = append(options, "Merge to parent")
	}

	options = append(options, "Merge to main")
	if hasRemote {
		if prCreated {
			// PR already exists - offer to push updates instead
			options = append(options, "Push updates to PR")
		} else {
			options = append(options, "Create PR")
		}
	}

	return &MergeState{
		SessionName:    sessionName,
		Options:        options,
		SelectedIndex:  0,
		HasRemote:      hasRemote,
		HasParent:      hasParent,
		ParentName:     parentName,
		ChangesSummary: changesSummary,
		PRCreated:      prCreated,
	}
}

// =============================================================================
// EditCommitState - State for the Edit Commit Message modal
// =============================================================================

type EditCommitState struct {
	Textarea  textarea.Model
	MergeType string // "merge" or "pr"
}

func (*EditCommitState) modalState() {}

func (s *EditCommitState) Title() string { return "Edit Commit Message" }

func (s *EditCommitState) Help() string {
	return "Ctrl+s: commit  Esc: cancel"
}

func (s *EditCommitState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Show what operation will follow
	var operationSection string
	if s.MergeType == "conflict" {
		operationStyle := lipgloss.NewStyle().
			Foreground(ColorSecondary).
			MarginBottom(1)
		operationSection = operationStyle.Render("Committing resolved merge conflicts")
	} else {
		operationLabel := "Merge to main"
		if s.MergeType == "pr" {
			operationLabel = "Create PR"
		}
		operationStyle := lipgloss.NewStyle().
			Foreground(ColorSecondary).
			MarginBottom(1)
		operationSection = operationStyle.Render("After commit: " + operationLabel)
	}

	textareaView := s.Textarea.View()

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, operationSection, textareaView, help)
}

func (s *EditCommitState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	var cmd tea.Cmd
	s.Textarea, cmd = s.Textarea.Update(msg)
	return s, cmd
}

// GetMessage returns the commit message
func (s *EditCommitState) GetMessage() string {
	return s.Textarea.Value()
}

// NewEditCommitState creates a new EditCommitState
func NewEditCommitState(message, mergeType string) *EditCommitState {
	ta := textarea.New()
	ta.Placeholder = "Enter commit message..."
	ta.CharLimit = 0
	ta.SetHeight(10)
	ta.SetWidth(ModalInputWidth)
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.SetValue(message)
	ta.Focus()

	return &EditCommitState{
		Textarea:  ta,
		MergeType: mergeType,
	}
}

// =============================================================================
// MergeConflictState - State for merge conflict resolution modal
// =============================================================================

type MergeConflictState struct {
	SessionID       string
	SessionName     string
	ConflictedFiles []string
	RepoPath        string
	Options         []string
	SelectedIndex   int
}

func (*MergeConflictState) modalState() {}

func (s *MergeConflictState) Title() string { return "Merge Conflict" }

func (s *MergeConflictState) Help() string {
	return "up/down to select, Enter to confirm, Esc to cancel"
}

func (s *MergeConflictState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Show session name
	sessionLabel := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true).
		MarginBottom(1).
		Render(s.SessionName)

	// Show conflicted files
	filesLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Conflicted files:")

	var filesList string
	maxFilesToShow := 5
	for i, file := range s.ConflictedFiles {
		if i >= maxFilesToShow {
			remaining := len(s.ConflictedFiles) - maxFilesToShow
			filesList += lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Italic(true).
				Render(fmt.Sprintf("  ... and %d more\n", remaining))
			break
		}
		filesList += lipgloss.NewStyle().
			Foreground(ColorText).
			Render("  " + file + "\n")
	}

	// Options
	var optionList string
	for i, opt := range s.Options {
		style := SidebarItemStyle
		prefix := "  "
		if i == s.SelectedIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}
		optionList += style.Render(prefix+opt) + "\n"
	}

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, sessionLabel, filesLabel, filesList, optionList, help)
}

func (s *MergeConflictState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if s.SelectedIndex > 0 {
				s.SelectedIndex--
			}
		case "down", "j":
			if s.SelectedIndex < len(s.Options)-1 {
				s.SelectedIndex++
			}
		}
	}
	return s, nil
}

// GetSelectedOption returns the index of the selected option
// 0 = Have Claude resolve, 1 = Abort merge, 2 = Resolve manually
func (s *MergeConflictState) GetSelectedOption() int {
	return s.SelectedIndex
}

// NewMergeConflictState creates a new MergeConflictState
func NewMergeConflictState(sessionID, sessionName string, conflictedFiles []string, repoPath string) *MergeConflictState {
	return &MergeConflictState{
		SessionID:       sessionID,
		SessionName:     sessionName,
		ConflictedFiles: conflictedFiles,
		RepoPath:        repoPath,
		Options:         []string{"Have Claude resolve", "Abort merge", "Resolve manually"},
		SelectedIndex:   0,
	}
}

// =============================================================================
// ConfirmDeleteState - State for the Confirm Delete modal
// =============================================================================

type ConfirmDeleteState struct {
	SessionName   string
	Options       []string
	SelectedIndex int
}

func (*ConfirmDeleteState) modalState() {}

func (s *ConfirmDeleteState) Title() string { return "Delete Session?" }

func (s *ConfirmDeleteState) Help() string {
	return "up/down to select, Enter to confirm, Esc to cancel"
}

func (s *ConfirmDeleteState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Show session name prominently
	sessionLabel := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true).
		MarginBottom(1).
		Render(s.SessionName)

	message := lipgloss.NewStyle().
		Foreground(ColorText).
		MarginBottom(1).
		Render("This will remove the session from the list.")

	var optionList string
	for i, opt := range s.Options {
		style := SidebarItemStyle
		prefix := "  "
		if i == s.SelectedIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}
		optionList += style.Render(prefix+opt) + "\n"
	}

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, sessionLabel, message, optionList, help)
}

func (s *ConfirmDeleteState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if s.SelectedIndex > 0 {
				s.SelectedIndex--
			}
		case "down", "j":
			if s.SelectedIndex < len(s.Options)-1 {
				s.SelectedIndex++
			}
		}
	}
	return s, nil
}

// ShouldDeleteWorktree returns true if user selected to delete the worktree
func (s *ConfirmDeleteState) ShouldDeleteWorktree() bool {
	return s.SelectedIndex == 1 // "Delete worktree" is index 1
}

// NewConfirmDeleteState creates a new ConfirmDeleteState
func NewConfirmDeleteState(sessionName string) *ConfirmDeleteState {
	return &ConfirmDeleteState{
		SessionName:   sessionName,
		Options:       []string{"Keep worktree", "Delete worktree"},
		SelectedIndex: 0,
	}
}
