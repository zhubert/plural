package modals

import (
	"fmt"
	"strings"

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

	optionList := RenderSelectableList(s.Options, s.SelectedIndex)

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

// unwrapCommitMessage unwraps the body of a commit message so it displays
// correctly in the textarea. Claude often wraps commit message bodies at
// 72 chars, but this creates awkward line breaks in the modal.
// This function:
// - Preserves the summary line (first line)
// - Preserves the blank line separator
// - Unwraps body paragraphs (replacing single newlines with spaces)
// - Preserves paragraph breaks (double newlines)
func unwrapCommitMessage(message string) string {
	lines := strings.Split(message, "\n")
	if len(lines) <= 1 {
		return message
	}

	// Find where the body starts (after summary and blank line)
	// A proper commit message has: summary, blank line, body
	bodyStart := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			bodyStart = i + 1
			break
		}
	}

	// If no blank line found, this isn't a properly formatted commit message
	// Return as-is to avoid mangling it
	if bodyStart == -1 || bodyStart >= len(lines) {
		return message
	}

	// Summary is just the first line
	summary := lines[0]

	// Process the body - unwrap paragraphs
	body := strings.Join(lines[bodyStart:], "\n")

	// Split by double newlines to preserve paragraph breaks
	paragraphs := strings.Split(body, "\n\n")
	var unwrappedParagraphs []string
	for _, para := range paragraphs {
		// Replace single newlines within paragraph with spaces
		unwrapped := strings.ReplaceAll(para, "\n", " ")
		// Clean up multiple spaces
		for strings.Contains(unwrapped, "  ") {
			unwrapped = strings.ReplaceAll(unwrapped, "  ", " ")
		}
		unwrappedParagraphs = append(unwrappedParagraphs, strings.TrimSpace(unwrapped))
	}

	// Reconstruct: summary + blank line + unwrapped body paragraphs
	return summary + "\n\n" + strings.Join(unwrappedParagraphs, "\n\n")
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
	ta.SetValue(unwrapCommitMessage(message))
	ta.Focus()

	// Apply transparent background styles
	ApplyTextareaStyles(&ta)

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
	optionList := RenderSelectableList(s.Options, s.SelectedIndex)

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

	optionList := RenderSelectableList(s.Options, s.SelectedIndex)

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

// =============================================================================
// ConfirmDeleteRepoState - State for the Confirm Delete Repository modal
// =============================================================================

type ConfirmDeleteRepoState struct {
	RepoPath string
}

func (*ConfirmDeleteRepoState) modalState() {}

func (s *ConfirmDeleteRepoState) Title() string { return "Delete Repository?" }

func (s *ConfirmDeleteRepoState) Help() string {
	return "Enter: confirm  Esc: cancel"
}

func (s *ConfirmDeleteRepoState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Show repo path prominently
	repoLabel := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true).
		MarginBottom(1).
		Render(s.RepoPath)

	message := lipgloss.NewStyle().
		Foreground(ColorText).
		MarginBottom(1).
		Render("This will remove the repository from Plural.\nExisting sessions for this repo will not be affected.")

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, repoLabel, message, help)
}

func (s *ConfirmDeleteRepoState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	return s, nil
}

// GetRepoPath returns the repository path to delete
func (s *ConfirmDeleteRepoState) GetRepoPath() string {
	return s.RepoPath
}

// NewConfirmDeleteRepoState creates a new ConfirmDeleteRepoState
func NewConfirmDeleteRepoState(repoPath string) *ConfirmDeleteRepoState {
	return &ConfirmDeleteRepoState{
		RepoPath: repoPath,
	}
}

// =============================================================================
// ConfirmExitState - State for the Confirm Exit modal
// =============================================================================

type ConfirmExitState struct {
	ActiveSessionCount int
	Options            []string
	SelectedIndex      int
}

func (*ConfirmExitState) modalState() {}

func (s *ConfirmExitState) Title() string { return "Exit Plural?" }

func (s *ConfirmExitState) Help() string {
	return "up/down to select, Enter to confirm, Esc to cancel"
}

func (s *ConfirmExitState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Show warning about active sessions
	var message string
	if s.ActiveSessionCount == 1 {
		message = "There is 1 active session running."
	} else {
		message = fmt.Sprintf("There are %d active sessions running.", s.ActiveSessionCount)
	}

	messageStyle := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true).
		MarginBottom(1)
	messageRendered := messageStyle.Render(message)

	warningStyle := lipgloss.NewStyle().
		Foreground(ColorText).
		MarginBottom(1)
	warning := warningStyle.Render("Exiting will terminate all Claude processes.")

	optionList := RenderSelectableList(s.Options, s.SelectedIndex)

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, messageRendered, warning, optionList, help)
}

func (s *ConfirmExitState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
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

// ShouldExit returns true if user selected to exit
func (s *ConfirmExitState) ShouldExit() bool {
	return s.SelectedIndex == 1 // "Exit" is index 1
}

// NewConfirmExitState creates a new ConfirmExitState
func NewConfirmExitState(activeSessionCount int) *ConfirmExitState {
	return &ConfirmExitState{
		ActiveSessionCount: activeSessionCount,
		Options:            []string{"Cancel", "Exit"},
		SelectedIndex:      0,
	}
}
