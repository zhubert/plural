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

// renderGradient renders the content with a purple-to-transparent gradient background
func (h *Header) renderGradient(content string) string {
	if len(content) == 0 {
		return ""
	}

	// Purple RGB: #7C3AED = (124, 58, 237)
	startR, startG, startB := 124, 58, 237
	// End color (terminal default, we'll fade to black/transparent effect)
	endR, endG, endB := 0, 0, 0

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
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(i < 7) // Bold for "Plural" title

		result.WriteString(style.Render(string(r)))
	}

	return result.String()
}
