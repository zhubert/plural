package modals

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/zhubert/plural/internal/keys"
)

// =============================================================================
// SearchMessagesState - State for searching within conversation messages
// =============================================================================

type SearchMessagesState struct {
	Query         string
	Input         textinput.Model
	AllMessages   []SearchResult // All messages (for context)
	Results       []SearchResult // Filtered search results
	SelectedIndex int            // Currently selected result
	ScrollOffset  int            // For scrolling through results
	maxVisible    int            // Maximum visible results
}

func (*SearchMessagesState) modalState() {}

func (s *SearchMessagesState) Title() string { return "Search Messages" }

func (s *SearchMessagesState) Help() string {
	if len(s.Results) == 0 && s.Query != "" {
		return "No matches found. Esc: close"
	}
	return "Type to search  up/down: navigate  Enter: go to message  Esc: close"
}

func (s *SearchMessagesState) Render() string {
	title := ModalTitleStyle.Render(s.Title())

	// Search input
	inputLabel := lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Render("Search:")

	inputStyle := lipgloss.NewStyle().
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorPrimary).
		PaddingLeft(1)
	inputView := inputStyle.Render(s.Input.View())

	// Results section
	var resultsSection string
	if s.Query == "" {
		resultsSection = lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			MarginTop(1).
			Render("Start typing to search through messages...")
	} else if len(s.Results) == 0 {
		resultsSection = lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			MarginTop(1).
			Render("No matches found")
	} else {
		// Show result count
		countStyle := lipgloss.NewStyle().
			Foreground(ColorSecondary).
			MarginTop(1).
			MarginBottom(1)
		resultsSection = countStyle.Render(fmt.Sprintf("%d match(es) found", len(s.Results)))

		// Build results list with scrolling
		visibleEnd := s.ScrollOffset + s.maxVisible
		if visibleEnd > len(s.Results) {
			visibleEnd = len(s.Results)
		}

		// Scroll indicators
		if s.ScrollOffset > 0 {
			resultsSection += "\n" + lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  up more above")
		}

		for i := s.ScrollOffset; i < visibleEnd; i++ {
			result := s.Results[i]
			isSelected := i == s.SelectedIndex

			// Role indicator
			roleStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
			if result.Role == "user" {
				roleStyle = lipgloss.NewStyle().Foreground(ColorUser).Bold(true)
			}
			roleText := "Claude"
			if result.Role == "user" {
				roleText = "You"
			}

			// Message number
			msgNum := fmt.Sprintf("[%d]", result.MessageIndex+1)

			// Extract snippet around the match
			snippet := s.extractSnippet(result, 60)

			// Build the line
			prefix := "  "
			style := SidebarItemStyle
			if isSelected {
				prefix = "> "
				style = SidebarSelectedStyle
			}

			line := fmt.Sprintf("%s %s %s: %s", prefix, msgNum, roleStyle.Render(roleText), snippet)
			resultsSection += "\n" + style.Render(line)
		}

		// More below indicator
		if visibleEnd < len(s.Results) {
			resultsSection += "\n" + lipgloss.NewStyle().Foreground(ColorTextMuted).Render("  down more below")
		}
	}

	help := ModalHelpStyle.Render(s.Help())

	return lipgloss.JoinVertical(lipgloss.Left, title, inputLabel, inputView, resultsSection, help)
}

// extractSnippet extracts a snippet of text around the match
func (s *SearchMessagesState) extractSnippet(result SearchResult, maxLen int) string {
	content := result.Content

	// Clean up the content - remove newlines and extra whitespace
	content = strings.ReplaceAll(content, "\n", " ")
	content = strings.ReplaceAll(content, "\t", " ")
	for strings.Contains(content, "  ") {
		content = strings.ReplaceAll(content, "  ", " ")
	}
	content = strings.TrimSpace(content)

	if len(content) <= maxLen {
		return s.highlightMatch(content, result.MatchStart, result.MatchEnd)
	}

	// Center the snippet around the match
	matchMid := (result.MatchStart + result.MatchEnd) / 2
	halfLen := maxLen / 2

	start := matchMid - halfLen
	end := matchMid + halfLen

	if start < 0 {
		start = 0
		end = maxLen
	}
	if end > len(content) {
		end = len(content)
		start = end - maxLen
		if start < 0 {
			start = 0
		}
	}

	snippet := content[start:end]

	// Adjust match positions for the snippet
	newMatchStart := result.MatchStart - start
	newMatchEnd := result.MatchEnd - start
	if newMatchStart < 0 {
		newMatchStart = 0
	}
	if newMatchEnd > len(snippet) {
		newMatchEnd = len(snippet)
	}

	// Add ellipsis if truncated
	prefix := ""
	suffix := ""
	if start > 0 {
		prefix = "..."
		newMatchStart += 3
		newMatchEnd += 3
	}
	if end < len(content) {
		suffix = "..."
	}

	return prefix + s.highlightMatch(snippet, newMatchStart-len(prefix), newMatchEnd-len(prefix)) + suffix
}

// highlightMatch highlights the matched portion of text
func (s *SearchMessagesState) highlightMatch(text string, start, end int) string {
	if start < 0 || end > len(text) || start >= end {
		return text
	}

	before := text[:start]
	match := text[start:end]
	after := text[end:]

	highlightStyle := lipgloss.NewStyle().
		Background(ColorWarning).
		Foreground(ColorTextInverse).
		Bold(true)

	return before + highlightStyle.Render(match) + after
}

func (s *SearchMessagesState) Update(msg tea.Msg) (ModalState, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case keys.Up, keys.CtrlP:
			if s.SelectedIndex > 0 {
				s.SelectedIndex--
				// Scroll up if needed
				if s.SelectedIndex < s.ScrollOffset {
					s.ScrollOffset = s.SelectedIndex
				}
			}
			return s, nil
		case keys.Down, keys.CtrlN:
			if s.SelectedIndex < len(s.Results)-1 {
				s.SelectedIndex++
				// Scroll down if needed
				if s.SelectedIndex >= s.ScrollOffset+s.maxVisible {
					s.ScrollOffset = s.SelectedIndex - s.maxVisible + 1
				}
			}
			return s, nil
		}
	}

	// Update text input
	var cmd tea.Cmd
	oldQuery := s.Input.Value()
	s.Input, cmd = s.Input.Update(msg)
	newQuery := s.Input.Value()

	// Re-filter if query changed
	if newQuery != oldQuery {
		s.Query = newQuery
		s.filterResults()
	}

	return s, cmd
}

// filterResults filters messages based on the current query
func (s *SearchMessagesState) filterResults() {
	s.Results = nil
	s.SelectedIndex = 0
	s.ScrollOffset = 0

	if s.Query == "" {
		return
	}

	query := strings.ToLower(s.Query)

	for _, msg := range s.AllMessages {
		content := strings.ToLower(msg.Content)
		idx := strings.Index(content, query)
		if idx != -1 {
			result := SearchResult{
				MessageIndex: msg.MessageIndex,
				Role:         msg.Role,
				Content:      msg.Content,
				MatchStart:   idx,
				MatchEnd:     idx + len(s.Query),
			}
			s.Results = append(s.Results, result)
		}
	}
}

// GetSelectedResult returns the currently selected search result
func (s *SearchMessagesState) GetSelectedResult() *SearchResult {
	if len(s.Results) == 0 || s.SelectedIndex >= len(s.Results) {
		return nil
	}
	return &s.Results[s.SelectedIndex]
}

// NewSearchMessagesState creates a new SearchMessagesState
// messages should be the conversation history to search through
func NewSearchMessagesState(messages []struct{ Role, Content string }) *SearchMessagesState {
	input := textinput.New()
	input.Placeholder = "Type to search..."
	input.CharLimit = SearchInputCharLimit
	input.SetWidth(ModalInputWidth)
	input.Focus()

	// Convert messages to SearchResult format for storage
	var allMessages []SearchResult
	for i, msg := range messages {
		allMessages = append(allMessages, SearchResult{
			MessageIndex: i,
			Role:         msg.Role,
			Content:      msg.Content,
		})
	}

	return &SearchMessagesState{
		Input:       input,
		AllMessages: allMessages,
		maxVisible:  SearchModalMaxVisible,
	}
}
