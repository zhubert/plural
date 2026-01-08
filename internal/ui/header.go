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

// View renders the header
func (h *Header) View() string {
	// Build the content string (without styling)
	titleText := " plural"
	var rightText string
	if h.sessionName != "" {
		rightText = h.sessionName + " "
	}

	// Calculate padding
	paddingLen := h.width - len(titleText) - len(rightText)
	if paddingLen < 0 {
		paddingLen = 0
	}

	fullContent := titleText + strings.Repeat(" ", paddingLen) + rightText

	// Render with gradient background
	return h.renderGradient(fullContent)
}

// parseHexColor parses a hex color string (e.g., "#7C3AED") into RGB components
func parseHexColor(hex string) (r, g, b int) {
	if len(hex) == 7 && hex[0] == '#' {
		fmt.Sscanf(hex[1:], "%02x%02x%02x", &r, &g, &b)
	}
	return
}

// renderGradient renders the content with a theme-aware gradient background
func (h *Header) renderGradient(content string) string {
	if len(content) == 0 {
		return ""
	}

	// Get colors from current theme
	theme := CurrentTheme()
	startR, startG, startB := parseHexColor(theme.Primary)
	// End color: fade to the dark background
	endR, endG, endB := parseHexColor(theme.BgDark)

	// Text color from theme
	textColor := lipgloss.Color(theme.Text)

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

		// Style for this character
		style := lipgloss.NewStyle().
			Background(bgColor).
			Foreground(textColor).
			Bold(i < 7) // Bold for "Plural" title

		result.WriteString(style.Render(string(r)))
	}

	return result.String()
}
