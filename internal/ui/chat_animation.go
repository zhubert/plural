package ui

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	pclaude "github.com/zhubert/plural/internal/claude"
)

// StopwatchTickMsg is sent to update the animated waiting display
type StopwatchTickMsg time.Time

// CompletionFlashTickMsg is sent to animate the completion checkmark flash
type CompletionFlashTickMsg time.Time

// SelectionFlashTickMsg is sent to animate the selection copy flash
type SelectionFlashTickMsg time.Time

// thinkingVerbs are playful status messages that cycle while waiting for Claude
var thinkingVerbs = []string{
	"Thinking",
	"Reasoning",
	"Pondering",
	"Contemplating",
	"Musing",
	"Cogitating",
	"Ruminating",
	"Deliberating",
	"Reflecting",
	"Considering",
	"Analyzing",
	"Processing",
	"Computing",
	"Synthesizing",
	"Formulating",
	"Brainstorming",
	"Noodling",
	"Percolating",
	"Brewing",
	"Marinating",
}

// randomThinkingVerb returns a random verb from the list
func randomThinkingVerb() string {
	return thinkingVerbs[rand.Intn(len(thinkingVerbs))]
}

// spinnerFrames are the characters used for the shimmering spinner animation
// Inspired by Claude Code's flower-like spinner
var spinnerFrames = []string{"·", "✺", "✹", "✸", "✷", "✶", "✵", "✴", "✳", "✲", "✱", "✧", "✦", "·"}

// spinnerFrameHoldTimes defines how long each frame should be held (in ticks)
// All frames have equal duration for smooth animation
var spinnerFrameHoldTimes = []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}

// StopwatchTick returns a command that sends a tick message after a delay
func StopwatchTick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return StopwatchTickMsg(t)
	})
}

// CompletionFlashTick returns a command that sends a completion flash tick
func CompletionFlashTick() tea.Cmd {
	return tea.Tick(160*time.Millisecond, func(t time.Time) tea.Msg {
		return CompletionFlashTickMsg(t)
	})
}

// SelectionFlashTick returns a command that sends a selection flash tick
func SelectionFlashTick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
		return SelectionFlashTickMsg(t)
	})
}

// StartCompletionFlash starts the completion checkmark flash animation
func (c *Chat) StartCompletionFlash() tea.Cmd {
	c.spinner.FlashFrame = 0
	c.updateContent()
	return CompletionFlashTick()
}

// IsCompletionFlashing returns whether the completion flash animation is active
func (c *Chat) IsCompletionFlashing() bool {
	return c.spinner.FlashFrame >= 0
}

// IsSelectionFlashing returns whether the selection flash animation is active
func (c *Chat) IsSelectionFlashing() bool {
	return c.selection.FlashFrame >= 0
}

// renderSpinner renders the shimmering spinner with the thinking verb.
// Returns the spinner character followed by the verb text.
func renderSpinner(verb string, frameIdx int) string {
	// Get the current spinner frame
	frame := spinnerFrames[frameIdx%len(spinnerFrames)]

	// Style for the spinner character - uses theme's user color
	spinnerStyle := lipgloss.NewStyle().
		Foreground(ColorUser).
		Bold(true)

	// Style for the verb text - uses theme's primary color, italic
	verbStyle := lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Italic(true)

	return spinnerStyle.Render(frame) + " " + verbStyle.Render(verb+"...")
}

// renderStreamingStatus renders the full status line during streaming.
// Format: ✺ Thinking... (esc to interrupt • 12s • ↓ 342 tokens • cache: 138k)
// Or with subagent: ✺ Thinking... [haiku working] (esc to interrupt • 12s • ↓ 342 tokens • cache: 138k)
func renderStreamingStatus(verb string, frameIdx int, elapsed time.Duration, stats *pclaude.StreamStats, subagentModel string) string {
	// Get the current spinner frame
	frame := spinnerFrames[frameIdx%len(spinnerFrames)]

	// Style for the spinner character - uses theme's user color
	spinnerStyle := lipgloss.NewStyle().
		Foreground(ColorUser).
		Bold(true)

	// Style for the verb text - uses theme's primary color, italic
	verbStyle := lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Italic(true)

	// Style for the metadata - muted color
	metaStyle := lipgloss.NewStyle().
		Foreground(ColorTextMuted)

	// Build the verb portion with optional subagent indicator
	verbPart := verbStyle.Render(verb + "...")
	if subagentModel != "" {
		subagentStyle := lipgloss.NewStyle().
			Foreground(ColorWarning).
			Italic(true)
		shortName := shortModelName(subagentModel)
		verbPart += " " + subagentStyle.Render("["+shortName+" working]")
	}

	// Build metadata parts: (esc to interrupt • 12s • ↓ 342 tokens • cache: 138k)
	var parts []string
	parts = append(parts, "esc to interrupt")
	parts = append(parts, formatElapsed(elapsed))

	if stats != nil && stats.OutputTokens > 0 {
		parts = append(parts, fmt.Sprintf("↓ %s tokens", formatTokenCount(stats.OutputTokens)))
	}

	// Show cache read tokens if significant cache usage (indicates cache hits)
	if stats != nil && stats.CacheReadTokens > 0 {
		parts = append(parts, fmt.Sprintf("cache: %s", formatTokenCount(stats.CacheReadTokens)))
	}

	meta := metaStyle.Render("(" + strings.Join(parts, " • ") + ")")
	return spinnerStyle.Render(frame) + " " + verbPart + " " + meta
}

// renderContainerInitStatus renders the container initialization status line
func renderContainerInitStatus(frameIdx int, elapsed time.Duration) string {
	// Get the current spinner frame
	frame := spinnerFrames[frameIdx%len(spinnerFrames)]

	// Style for the spinner character - uses theme's user color
	spinnerStyle := lipgloss.NewStyle().
		Foreground(ColorUser).
		Bold(true)

	// Style for the message text - uses theme's primary color, italic
	messageStyle := lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Italic(true)

	// Style for the metadata - muted color
	metaStyle := lipgloss.NewStyle().
		Foreground(ColorTextMuted)

	// Build message
	message := messageStyle.Render("Starting container...")

	// Build metadata: (usually takes 2-5s • 3s)
	var parts []string
	parts = append(parts, "usually takes 2-5s")
	parts = append(parts, formatElapsed(elapsed))

	meta := metaStyle.Render("(" + strings.Join(parts, " • ") + ")")
	return spinnerStyle.Render(frame) + " " + message + " " + meta
}

// formatElapsed formats a duration for display (e.g., "12s", "1m30s")
func formatElapsed(d time.Duration) string {
	secs := int(d.Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	return fmt.Sprintf("%dm%ds", secs/60, secs%60)
}

// formatTokenCount formats a token count for display (e.g., "342", "1.4k")
func formatTokenCount(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// SetStreamStats updates the streaming statistics for display
func (c *Chat) SetStreamStats(stats *pclaude.StreamStats) {
	c.streamStats = stats
	c.updateContent()
}

// SetSubagentModel sets the current subagent model (empty string clears it)
func (c *Chat) SetSubagentModel(model string) {
	c.subagentModel = model
	c.updateContent()
}

// GetSubagentModel returns the current subagent model (empty if none active)
func (c *Chat) GetSubagentModel() string {
	return c.subagentModel
}

// ClearSubagentModel clears the subagent indicator
func (c *Chat) ClearSubagentModel() {
	c.subagentModel = ""
	c.updateContent()
}

// renderCompletionFlash renders the checkmark completion flash with final stats
func renderCompletionFlash(frame int, stats *pclaude.StreamStats) string {
	checkmark := "✓"

	// Check if we have any stats to display (tokens or timing)
	hasStats := stats != nil && (stats.OutputTokens > 0 || stats.DurationMs > 0)

	// Frame 0: bright checkmark (using theme's diff added color which is green)
	// Frame 1: normal checkmark
	// Frame 2+: fade out (empty)
	switch frame {
	case 0:
		// Bright checkmark using theme's diff added color
		style := lipgloss.NewStyle().
			Foreground(DiffAddedStyle.GetForeground()).
			Bold(true)
		result := style.Render(checkmark) + " " + lipgloss.NewStyle().Foreground(ColorSecondary).Italic(true).Render("Done")
		// Add stats if available (tokens and/or timing)
		if hasStats {
			result += " " + renderFinalStats(stats)
		}
		return result
	case 1:
		// Normal checkmark (using theme's secondary color) with stats
		style := lipgloss.NewStyle().
			Foreground(ColorSecondary)
		result := style.Render(checkmark)
		if hasStats {
			result += " " + renderFinalStats(stats)
		}
		return result
	default:
		return ""
	}
}

// renderFinalStats renders the final token statistics with model breakdown, cache efficiency, and timing
func renderFinalStats(stats *pclaude.StreamStats) string {
	if stats == nil || (stats.OutputTokens == 0 && stats.DurationMs == 0) {
		return ""
	}

	metaStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)

	var parts []string

	// Add token stats if available
	if stats.OutputTokens > 0 {
		parts = append(parts, fmt.Sprintf("↓ %s", formatTokenCount(stats.OutputTokens)))
	}

	// Cache efficiency percentage if cache was used
	if stats.CacheReadTokens > 0 || stats.CacheCreationTokens > 0 {
		totalInput := stats.InputTokens + stats.CacheReadTokens + stats.CacheCreationTokens
		if totalInput > 0 {
			hitRate := float64(stats.CacheReadTokens) / float64(totalInput) * 100
			parts = append(parts, fmt.Sprintf("cache: %.0f%%", hitRate))
		}
	}

	// If we have a breakdown by model and more than one model was used, show it
	if len(stats.ByModel) > 1 {
		var modelParts []string
		for _, m := range stats.ByModel {
			// Extract short model name (e.g., "opus" from "claude-opus-4-5-20251101")
			shortName := shortModelName(m.Model)
			modelParts = append(modelParts, fmt.Sprintf("%s: %s", shortName, formatTokenCount(m.OutputTokens)))
		}
		parts = append(parts, strings.Join(modelParts, ", "))
	}

	// Add timing if available
	if stats.DurationMs > 0 {
		parts = append(parts, formatDuration(stats.DurationMs))
	}

	if len(parts) == 0 {
		return ""
	}

	return metaStyle.Render("(" + strings.Join(parts, " • ") + ")")
}

// formatDuration formats milliseconds into a human-readable duration (e.g., "12s", "1m30s")
func formatDuration(ms int) string {
	secs := ms / 1000
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	return fmt.Sprintf("%dm%ds", secs/60, secs%60)
}

// shortModelName extracts a readable short name from a full model ID
func shortModelName(model string) string {
	// Map known model patterns to short names
	switch {
	case strings.Contains(model, "opus"):
		return "opus"
	case strings.Contains(model, "sonnet"):
		return "sonnet"
	case strings.Contains(model, "haiku"):
		return "haiku"
	default:
		// For unknown models, try to extract a meaningful part
		// e.g., "claude-3-5-sonnet-20241022" -> "sonnet"
		parts := strings.Split(model, "-")
		if len(parts) >= 3 {
			return parts[len(parts)-2] // Second to last part often has the name
		}
		return model
	}
}

// SetWaiting sets the waiting state (before streaming starts)
func (c *Chat) SetWaiting(waiting bool) {
	c.waiting = waiting
	if waiting {
		c.spinner.Verb = randomThinkingVerb()
		c.spinner.Idx = 0
		c.spinner.Tick = 0
		c.streamStartTime = time.Now()
		c.streamStats = nil  // Reset stats for new request
		c.finalStats = nil   // Clear previous final stats
	}
	c.updateContent()
}

// SetWaitingWithStart sets the waiting state with a specific start time (for session restoration)
func (c *Chat) SetWaitingWithStart(waiting bool, startTime time.Time) {
	c.waiting = waiting
	if waiting {
		c.spinner.Verb = randomThinkingVerb()
		c.spinner.Idx = 0
		c.spinner.Tick = 0
		c.streamStartTime = startTime
		c.streamStats = nil  // Reset stats for new request
		c.finalStats = nil   // Clear previous final stats
	}
	c.updateContent()
}

// IsWaiting returns whether we're waiting for a response
func (c *Chat) IsWaiting() bool {
	return c.waiting
}

// SetContainerInitializing sets the container initialization state
func (c *Chat) SetContainerInitializing(initializing bool, startTime time.Time) {
	c.containerInitializing = initializing
	if initializing {
		c.containerInitStart = startTime
	} else {
		c.containerInitStart = time.Time{}
	}
	c.updateContent()
}

// IsContainerInitializing returns whether a container is initializing
func (c *Chat) IsContainerInitializing() bool {
	return c.containerInitializing
}

// handleStopwatchTick handles the spinner animation tick
func (c *Chat) handleStopwatchTick() tea.Cmd {
	// Continue ticking while waiting for response OR actively streaming
	if !c.waiting && c.streaming == "" {
		return nil
	}

	// Advance the spinner with easing (some frames hold longer)
	c.spinner.Tick++
	holdTime := spinnerFrameHoldTimes[c.spinner.Idx%len(spinnerFrameHoldTimes)]
	if c.spinner.Tick >= holdTime {
		c.spinner.Tick = 0
		c.spinner.Idx++
		if c.spinner.Idx >= len(spinnerFrames) {
			c.spinner.Idx = 0
		}
	}
	c.updateContent()
	return StopwatchTick()
}

// handleCompletionFlashTick handles the completion flash animation tick
func (c *Chat) handleCompletionFlashTick() tea.Cmd {
	if c.spinner.FlashFrame < 0 {
		return nil
	}

	c.spinner.FlashFrame++
	if c.spinner.FlashFrame >= 3 {
		// Animation complete
		c.spinner.FlashFrame = -1
	}
	c.updateContent()
	if c.spinner.FlashFrame >= 0 {
		return CompletionFlashTick()
	}
	return nil
}
