package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

// DiffStats holds file change statistics for display in the header
type DiffStats struct {
	FilesChanged int
	Additions    int
	Deletions    int
}

// Header represents the top header bar
type Header struct {
	width           int
	sessionName     string
	baseBranch      string
	diffStats       *DiffStats
	previewActive   bool
	containerActive bool
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

// SetDiffStats sets the diff statistics to display
func (h *Header) SetDiffStats(stats *DiffStats) {
	h.diffStats = stats
}

// SetPreviewActive sets whether a preview is currently active
func (h *Header) SetPreviewActive(active bool) {
	h.previewActive = active
}

// SetContainerActive sets whether the current session is containerized
func (h *Header) SetContainerActive(active bool) {
	h.containerActive = active
}

// headerRegion represents a styled region in the header
type headerRegion struct {
	start int
	end   int
	style string // "normal", "muted", "added", "deleted", "preview", "container"
}

// View renders the header
func (h *Header) View() string {
	// Build the content string (without styling)
	titleText := " plural"

	// Build right side content and track regions for coloring
	var rightText string
	var regions []headerRegion

	if h.sessionName != "" {
		// Add container indicator if active
		if h.containerActive {
			containerStart := utf8.RuneCountInString(rightText)
			rightText += "[CONTAINER] "
			containerEnd := utf8.RuneCountInString(rightText)
			regions = append(regions, headerRegion{start: containerStart, end: containerEnd, style: "container"})
		}

		// Add preview indicator if active
		if h.previewActive {
			previewStart := utf8.RuneCountInString(rightText)
			rightText += "[PREVIEW] "
			previewEnd := utf8.RuneCountInString(rightText)
			regions = append(regions, headerRegion{start: previewStart, end: previewEnd, style: "preview"})
		}

		// Add diff stats before session name if available
		if h.diffStats != nil && h.diffStats.FilesChanged > 0 {
			// Format: "3 files, +157, -5 "
			filesText := fmt.Sprintf("%d file", h.diffStats.FilesChanged)
			if h.diffStats.FilesChanged != 1 {
				filesText += "s"
			}

			additionsText := fmt.Sprintf("+%d", h.diffStats.Additions)
			deletionsText := fmt.Sprintf("-%d", h.diffStats.Deletions)

			// Build the stats string and track regions
			rightText += filesText + ", "
			addStart := utf8.RuneCountInString(rightText)
			rightText += additionsText
			addEnd := utf8.RuneCountInString(rightText)
			regions = append(regions, headerRegion{start: addStart, end: addEnd, style: "added"})

			rightText += ", "
			delStart := utf8.RuneCountInString(rightText)
			rightText += deletionsText
			delEnd := utf8.RuneCountInString(rightText)
			regions = append(regions, headerRegion{start: delStart, end: delEnd, style: "deleted"})

			rightText += "  " // Spacing before session name
		}

		rightText += h.sessionName
		if h.baseBranch != "" {
			branchStart := utf8.RuneCountInString(rightText)
			rightText += " (" + h.baseBranch + ")"
			branchEnd := utf8.RuneCountInString(rightText)
			regions = append(regions, headerRegion{start: branchStart, end: branchEnd, style: "muted"})
		}
		rightText += " "
	}

	// Calculate padding using display width (accounts for double-width CJK characters)
	paddingLen := max(h.width-lipgloss.Width(titleText)-lipgloss.Width(rightText), 0)

	fullContent := titleText + strings.Repeat(" ", paddingLen) + rightText

	// Adjust region positions to account for the left side content
	leftOffset := utf8.RuneCountInString(titleText) + paddingLen
	for i := range regions {
		regions[i].start += leftOffset
		regions[i].end += leftOffset
	}

	// Render with gradient background
	return h.renderGradient(fullContent, regions)
}

// parseHexColor parses a hex color string (e.g., "#7C3AED") into RGB components
func parseHexColor(hex string) (r, g, b int) {
	if len(hex) == 7 && hex[0] == '#' {
		fmt.Sscanf(hex[1:], "%02x%02x%02x", &r, &g, &b)
	}
	return
}

// renderGradient renders the content with a theme-aware gradient background
// regions specifies which portions of the text should have special styling
func (h *Header) renderGradient(content string, regions []headerRegion) string {
	if len(content) == 0 {
		return ""
	}

	// Get colors from current theme
	theme := CurrentTheme()
	startR, startG, startB := parseHexColor(theme.Primary)
	// End color: fade to the main background
	endR, endG, endB := parseHexColor(theme.Bg)

	// Text colors from theme
	textColor := lipgloss.Color(theme.Text)
	mutedColor := lipgloss.Color(theme.TextMuted)
	addedColor := lipgloss.Color(theme.DiffAdded)
	deletedColor := lipgloss.Color(theme.DiffRemoved)
	previewColor := lipgloss.Color(theme.Warning)   // Use warning color (amber/yellow) for preview indicator
	containerColor := lipgloss.Color(theme.Success) // Use success color (green) for container indicator

	// Helper to get the style for a given position
	getStyleForPos := func(pos int) string {
		for _, region := range regions {
			if pos >= region.start && pos < region.end {
				return region.style
			}
		}
		return "normal"
	}

	runes := []rune(content)
	// Use display width for gradient interpolation so double-width characters
	// get proportionally sized gradient steps
	displayWidth := lipgloss.Width(content)
	var result strings.Builder

	col := 0 // current display column
	for i, r := range runes {
		// Calculate interpolation factor based on display column position
		t := float64(col) / float64(displayWidth)

		// Interpolate colors
		cr := int(float64(startR)*(1-t) + float64(endR)*t)
		cg := int(float64(startG)*(1-t) + float64(endG)*t)
		cb := int(float64(startB)*(1-t) + float64(endB)*t)

		// Create color string
		bgColor := lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", cr, cg, cb))

		// Style for this character
		style := lipgloss.NewStyle().
			Background(bgColor).
			Bold(i < 7) // Bold for "Plural" title

		// Apply foreground color based on region
		switch getStyleForPos(i) {
		case "muted":
			style = style.Foreground(mutedColor)
		case "added":
			style = style.Foreground(addedColor)
		case "deleted":
			style = style.Foreground(deletedColor)
		case "preview":
			style = style.Foreground(previewColor).Bold(true)
		case "container":
			style = style.Foreground(containerColor).Bold(true)
		default:
			style = style.Foreground(textColor)
		}

		result.WriteString(style.Render(string(r)))
		col += runewidth.RuneWidth(r)
	}

	return result.String()
}
