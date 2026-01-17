package ui

import (
	"math/rand"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
// First and last frames hold longer for a "breathing" effect
var spinnerFrameHoldTimes = []int{3, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 3}

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
	c.completionFlashFrame = 0
	c.updateContent()
	return CompletionFlashTick()
}

// IsCompletionFlashing returns whether the completion flash animation is active
func (c *Chat) IsCompletionFlashing() bool {
	return c.completionFlashFrame >= 0
}

// IsSelectionFlashing returns whether the selection flash animation is active
func (c *Chat) IsSelectionFlashing() bool {
	return c.selectionFlashFrame >= 0
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

// renderCompletionFlash renders the checkmark completion flash
func renderCompletionFlash(frame int) string {
	checkmark := "✓"

	// Frame 0: bright checkmark (using theme's diff added color which is green)
	// Frame 1: normal checkmark
	// Frame 2+: fade out (empty)
	switch frame {
	case 0:
		// Bright checkmark using theme's diff added color
		style := lipgloss.NewStyle().
			Foreground(DiffAddedStyle.GetForeground()).
			Bold(true)
		return style.Render(checkmark) + " " + lipgloss.NewStyle().Foreground(ColorSecondary).Italic(true).Render("Done")
	case 1:
		// Normal checkmark (using theme's secondary color)
		style := lipgloss.NewStyle().
			Foreground(ColorSecondary)
		return style.Render(checkmark)
	default:
		return ""
	}
}

// SetWaiting sets the waiting state (before streaming starts)
func (c *Chat) SetWaiting(waiting bool) {
	c.waiting = waiting
	if waiting {
		c.waitingVerb = randomThinkingVerb()
		c.spinnerIdx = 0
		c.spinnerTick = 0
	}
	c.updateContent()
}

// SetWaitingWithStart sets the waiting state with a specific start time (for session restoration)
// Note: startTime parameter is kept for API compatibility but no longer used
func (c *Chat) SetWaitingWithStart(waiting bool, startTime time.Time) {
	c.waiting = waiting
	if waiting {
		c.waitingVerb = randomThinkingVerb()
		c.spinnerIdx = 0
		c.spinnerTick = 0
	}
	c.updateContent()
}

// IsWaiting returns whether we're waiting for a response
func (c *Chat) IsWaiting() bool {
	return c.waiting
}

// handleStopwatchTick handles the spinner animation tick
func (c *Chat) handleStopwatchTick() tea.Cmd {
	if !c.waiting {
		return nil
	}

	// Advance the spinner with easing (some frames hold longer)
	c.spinnerTick++
	holdTime := spinnerFrameHoldTimes[c.spinnerIdx%len(spinnerFrameHoldTimes)]
	if c.spinnerTick >= holdTime {
		c.spinnerTick = 0
		c.spinnerIdx++
		if c.spinnerIdx >= len(spinnerFrames) {
			c.spinnerIdx = 0
		}
	}
	c.updateContent()
	return StopwatchTick()
}

// handleCompletionFlashTick handles the completion flash animation tick
func (c *Chat) handleCompletionFlashTick() tea.Cmd {
	if c.completionFlashFrame < 0 {
		return nil
	}

	c.completionFlashFrame++
	if c.completionFlashFrame >= 3 {
		// Animation complete
		c.completionFlashFrame = -1
	}
	c.updateContent()
	if c.completionFlashFrame >= 0 {
		return CompletionFlashTick()
	}
	return nil
}
