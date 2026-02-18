package modals

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zhubert/plural/internal/keys"
)

// ReviewCommentsState holds state for the PR Review Comments modal.
type ReviewCommentsState struct {
	SessionID     string
	Branch        string
	Comments      []ReviewCommentItem
	SelectedIndex int
	Loading       bool
	LoadError     string
	ScrollOffset  int
	maxVisible    int

	// Size tracking
	availableWidth int
}

func (*ReviewCommentsState) modalState() {}

// PreferredWidth returns the preferred width for this modal.
func (s *ReviewCommentsState) PreferredWidth() int {
	return ModalWidthWide
}

// SetSize updates the available width for rendering content.
func (s *ReviewCommentsState) SetSize(width, height int) {
	s.availableWidth = width
}

func (s *ReviewCommentsState) Title() string {
	return "PR Review Comments"
}

func (s *ReviewCommentsState) Help() string {
	if s.Loading {
		return "Loading review comments..."
	}
	if s.LoadError != "" {
		return "Esc: close"
	}
	return "up/down navigate  Space: toggle  a: select all  Enter: send to Claude  Esc: cancel"
}

func (s *ReviewCommentsState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	branchLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Branch:")

	branchName := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true).
		MarginBottom(1).
		Render("  " + s.Branch)

	// Loading state
	if s.Loading {
		loadingText := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("Fetching review comments...")
		help := ModalHelpStyle.Render(s.Help())
		return lipgloss.JoinVertical(lipgloss.Left, title, branchLabel, branchName, loadingText, help)
	}

	// Error state
	if s.LoadError != "" {
		errorText := StatusErrorStyle.Render(s.LoadError)
		help := ModalHelpStyle.Render(s.Help())
		return lipgloss.JoinVertical(lipgloss.Left, title, branchLabel, branchName, errorText, help)
	}

	// No comments
	if len(s.Comments) == 0 {
		noComments := lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("No review comments found")
		help := ModalHelpStyle.Render(s.Help())
		return lipgloss.JoinVertical(lipgloss.Left, title, branchLabel, branchName, noComments, help)
	}

	description := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		MarginBottom(1).
		Render("Select comments to address:")

	// Build comment list with scrolling
	modalWidth := s.availableWidth
	if modalWidth == 0 {
		modalWidth = s.PreferredWidth()
	}
	contentWidth := modalWidth - 4 // Account for modal padding/borders

	var commentList string
	visibleEnd := min(s.ScrollOffset+s.maxVisible, len(s.Comments))

	for i := s.ScrollOffset; i < visibleEnd; i++ {
		comment := s.Comments[i]
		style := SidebarItemStyle
		prefix := "  "
		checkbox := "[ ]"

		if i == s.SelectedIndex {
			style = SidebarSelectedStyle
			prefix = "> "
		}

		if comment.Selected {
			checkbox = "[x]"
		}

		// First line: checkbox + author + optional file path
		var headerParts []string
		if comment.Author != "" {
			headerParts = append(headerParts, "@"+comment.Author)
		}
		if comment.Path != "" {
			if comment.Line > 0 {
				headerParts = append(headerParts, fmt.Sprintf("%s:%d", comment.Path, comment.Line))
			} else {
				headerParts = append(headerParts, comment.Path)
			}
		}

		headerLine := fmt.Sprintf("%s %s", checkbox, strings.Join(headerParts, "  "))
		commentList += style.Render(prefix+headerLine) + "\n"

		// Body lines: wrap comment body to show more context (up to 3 lines)
		bodyText := strings.TrimSpace(comment.Body)
		bodyStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		indent := "      " // Align with text after checkbox
		// Overhead: "  " prefix + "      " indent = 8 chars
		maxLineLen := max(contentWidth-8, 10)
		bodyLines := wrapBodyText(bodyText, maxLineLen, 3)
		for _, line := range bodyLines {
			commentList += bodyStyle.Render(prefix+indent+line) + "\n"
		}
	}

	// Scroll indicators
	if s.ScrollOffset > 0 {
		commentList = lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  up more above\n") + commentList
	}
	if visibleEnd < len(s.Comments) {
		commentList += lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  down more below\n")
	}

	// Show count of selected comments
	selectedCount := 0
	for _, c := range s.Comments {
		if c.Selected {
			selectedCount++
		}
	}

	countStyle := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		MarginTop(1)
	countText := fmt.Sprintf("%d of %d comment(s) selected", selectedCount, len(s.Comments))
	countSection := countStyle.Render(countText)

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, branchLabel, branchName, description, commentList, countSection, help)
}

func (s *ReviewCommentsState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case keys.Up, "k":
			if s.SelectedIndex > 0 {
				s.SelectedIndex--
				if s.SelectedIndex < s.ScrollOffset {
					s.ScrollOffset = s.SelectedIndex
				}
			}
		case keys.Down, "j":
			if s.SelectedIndex < len(s.Comments)-1 {
				s.SelectedIndex++
				if s.SelectedIndex >= s.ScrollOffset+s.maxVisible {
					s.ScrollOffset = s.SelectedIndex - s.maxVisible + 1
				}
			}
		case keys.Space:
			if s.SelectedIndex < len(s.Comments) {
				s.Comments[s.SelectedIndex].Selected = !s.Comments[s.SelectedIndex].Selected
			}
		case "a":
			// Toggle select all: if all are selected, deselect all; otherwise select all
			allSelected := true
			for _, c := range s.Comments {
				if !c.Selected {
					allSelected = false
					break
				}
			}
			for i := range s.Comments {
				s.Comments[i].Selected = !allSelected
			}
		}
	}
	return s, nil
}

// GetSelectedComments returns the comments that are selected.
func (s *ReviewCommentsState) GetSelectedComments() []ReviewCommentItem {
	var selected []ReviewCommentItem
	for _, c := range s.Comments {
		if c.Selected {
			selected = append(selected, c)
		}
	}
	return selected
}

// SetComments sets the comments list and clears loading state.
func (s *ReviewCommentsState) SetComments(comments []ReviewCommentItem) {
	s.Comments = comments
	s.Loading = false
	s.LoadError = ""
}

// SetError sets an error and clears loading state.
func (s *ReviewCommentsState) SetError(err string) {
	s.LoadError = err
	s.Loading = false
}

// wrapBodyText wraps a comment body into lines of at most maxLen runes,
// returning up to maxLines lines. The last line is truncated with "..." if
// the full text doesn't fit.
func wrapBodyText(body string, maxLen, maxLines int) []string {
	// Normalize whitespace: collapse newlines and multiple spaces
	body = strings.Join(strings.Fields(body), " ")
	if body == "" {
		return nil
	}

	runes := []rune(body)
	var lines []string

	for len(runes) > 0 && len(lines) < maxLines {
		if len(runes) <= maxLen {
			lines = append(lines, string(runes))
			runes = nil
			break
		}

		isLastLine := len(lines) == maxLines-1
		if isLastLine {
			// Last allowed line: truncate with ellipsis
			if maxLen <= 3 {
				lines = append(lines, string(runes[:maxLen]))
			} else {
				lines = append(lines, string(runes[:maxLen-3])+"...")
			}
			break
		}

		// Find a word boundary to break at by scanning backwards within the
		// line. We know len(runes) > maxLen here, so indices 0..maxLen-1 are
		// the characters that would fit on this line.
		breakAt := -1
		for i := maxLen - 1; i > 0; i-- {
			if runes[i] == ' ' {
				breakAt = i
				break
			}
		}
		if breakAt <= 0 {
			// No space found within the line, hard break at maxLen
			breakAt = maxLen
		}

		lines = append(lines, string(runes[:breakAt]))
		runes = runes[breakAt:]
		// Skip leading space on next line
		if len(runes) > 0 && runes[0] == ' ' {
			runes = runes[1:]
		}
	}

	return lines
}

// NewReviewCommentsState creates a new ReviewCommentsState in loading state.
func NewReviewCommentsState(sessionID, branch string) *ReviewCommentsState {
	return &ReviewCommentsState{
		SessionID:      sessionID,
		Branch:         branch,
		Loading:        true,
		SelectedIndex:  0,
		ScrollOffset:   0,
		maxVisible:     IssuesModalMaxVisible,
		availableWidth: ModalWidthWide,
	}
}
