package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// Header represents the top header bar
type Header struct {
	width       int
	sessionName string
	baseBranch  string
}

// NewHeader creates a new header
func NewHeader() *Header {
	return &Header{}
}

// SetWidth sets the header width
func (h *Header) SetWidth(width int) {
	h.width = width
}

// SetSessionName sets the current session name to display
func (h *Header) SetSessionName(name string) {
	h.sessionName = name
}

// SetBaseBranch sets the base branch to display
func (h *Header) SetBaseBranch(branch string) {
	h.baseBranch = branch
}

// View renders the header
func (h *Header) View() string {
	// Build the content string (without styling)
	titleText := " plural"
	var rightText string
	if h.sessionName != "" {
		rightText = h.sessionName
		if h.baseBranch != "" {
			rightText += " (" + h.baseBranch + ")"
		}
		rightText += " "
	}

	// Calculate padding
	paddingLen := h.width - len(titleText) - len(rightText)
	if paddingLen < 0 {
		paddingLen = 0
	}

	fullContent := titleText + strings.Repeat(" ", paddingLen) + rightText

	// Render with gradient background
	return h.renderGradient(fullContent, h.baseBranch)
}

// parseHexColor parses a hex color string (e.g., "#7C3AED") into RGB components
func parseHexColor(hex string) (r, g, b int) {
	if len(hex) == 7 && hex[0] == '#' {
		fmt.Sscanf(hex[1:], "%02x%02x%02x", &r, &g, &b)
	}
	return
}

// renderGradient renders the content with a theme-aware gradient background
// baseBranch is used to identify and mute the base branch portion of the text
func (h *Header) renderGradient(content string, baseBranch string) string {
	if len(content) == 0 {
		return ""
	}

	// Get colors from current theme
	theme := CurrentTheme()
	startR, startG, startB := parseHexColor(theme.Primary)
	// End color: fade to the main background
	endR, endG, endB := parseHexColor(theme.Bg)

	// Text color from theme
	textColor := lipgloss.Color(theme.Text)
	mutedColor := lipgloss.Color(theme.TextMuted)

	// Find where the base branch portion starts (if present)
	baseBranchStart := -1
	if baseBranch != "" {
		baseBranchMarker := "(" + baseBranch + ")"
		baseBranchStart = strings.Index(content, baseBranchMarker)
	}

	runes := []rune(content)
	width := len(runes)
	var result strings.Builder

	for i, r := range runes {
		// Calculate interpolation factor (0.0 to 1.0)
		t := float64(i) / float64(width)

		// Interpolate colors
		cr := int(float64(startR)*(1-t) + float64(endR)*t)
		cg := int(float64(startG)*(1-t) + float64(endG)*t)
		cb := int(float64(startB)*(1-t) + float64(endB)*t)

		// Create color string
		bgColor := lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", cr, cg, cb))

		// Determine if this character is in the base branch portion
		inBaseBranch := baseBranchStart >= 0 && i >= baseBranchStart

		// Style for this character
		style := lipgloss.NewStyle().
			Background(bgColor).
			Bold(i < 7) // Bold for "Plural" title

		if inBaseBranch {
			style = style.Foreground(mutedColor)
		} else {
			style = style.Foreground(textColor)
		}

		result.WriteString(style.Render(string(r)))
	}

	return result.String()
}
