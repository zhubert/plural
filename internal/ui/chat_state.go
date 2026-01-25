package ui

import (
	"time"

	"charm.land/bubbles/v2/viewport"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/mcp"
)

// PendingPermission tracks an awaited permission response from the user.
// Non-nil when a permission prompt is displayed.
type PendingPermission struct {
	Tool        string // Tool name requesting permission (e.g., "Bash")
	Description string // Description of what the tool wants to do
}

// PendingQuestion tracks an awaited question response from the user.
// Non-nil when a question prompt is displayed.
type PendingQuestion struct {
	Questions      []mcp.Question    // All questions to be answered
	CurrentIdx     int               // Index of question currently being answered
	SelectedOption int               // Currently highlighted option (0-indexed)
	Answers        map[string]string // Collected answers (question text -> selected label)
}

// NewPendingQuestion creates a new PendingQuestion for the given questions.
func NewPendingQuestion(questions []mcp.Question) *PendingQuestion {
	return &PendingQuestion{
		Questions:      questions,
		CurrentIdx:     0,
		SelectedOption: 0,
		Answers:        make(map[string]string),
	}
}

// CurrentQuestion returns the current question being asked, or nil if done.
func (p *PendingQuestion) CurrentQuestion() *mcp.Question {
	if p.CurrentIdx >= len(p.Questions) {
		return nil
	}
	return &p.Questions[p.CurrentIdx]
}

// IsComplete returns true if all questions have been answered.
func (p *PendingQuestion) IsComplete() bool {
	return p.CurrentIdx >= len(p.Questions)
}

// PendingPlanApproval tracks an awaited plan approval from the user.
// Non-nil when a plan approval prompt is displayed.
type PendingPlanApproval struct {
	Plan           string              // The plan content (markdown)
	AllowedPrompts []mcp.AllowedPrompt // Requested Bash permissions
	ScrollOffset   int                 // Scroll offset for viewing the plan
}

// TextSelection tracks mouse-based text selection state in the chat viewport.
type TextSelection struct {
	StartCol, StartLine int  // Start position (column, line in viewport)
	EndCol, EndLine     int  // End position (column, line in viewport)
	Active              bool // True during drag operation

	// Click tracking for double/triple click detection
	LastClickTime time.Time
	LastClickX    int
	LastClickY    int
	ClickCount    int

	// Selection flash animation (brief highlight after copy, then clear)
	FlashFrame int // -1 = inactive, 0 = flash visible, 1+ = done
}

// NewTextSelection creates a new TextSelection in inactive state.
func NewTextSelection() *TextSelection {
	return &TextSelection{
		FlashFrame: -1,
	}
}

// HasSelection returns true if there's a non-empty text selection.
func (s *TextSelection) HasSelection() bool {
	if s.StartLine != s.EndLine {
		return true
	}
	return s.StartCol != s.EndCol
}

// Clear resets the selection to empty state.
func (s *TextSelection) Clear() {
	s.StartCol = 0
	s.StartLine = 0
	s.EndCol = 0
	s.EndLine = 0
	s.Active = false
}

// ViewChangesState tracks the git diff overlay state.
// Non-nil when the diff overlay is displayed.
type ViewChangesState struct {
	Viewport  viewport.Model // Viewport for diff scrolling
	Files     []git.FileDiff // List of files with diffs
	FileIndex int            // Currently selected file index
}

// LogFile represents a log file for display in the log viewer.
type LogFile struct {
	Name    string // Display name (e.g., "Debug Log", "MCP (session-id)")
	Path    string // Full file path
	Content string // File content (loaded on demand)
}

// LogViewerState tracks the log viewer overlay state.
// Non-nil when the log viewer is displayed.
type LogViewerState struct {
	Viewport  viewport.Model // Viewport for log scrolling
	Files     []LogFile      // List of available log files
	FileIndex int            // Currently selected file index
	FollowTail bool          // Whether to auto-scroll to bottom on updates
}

// PendingImage tracks an attached image waiting to be sent.
// Non-nil when an image is attached.
type PendingImage struct {
	Data      []byte // PNG encoded image data
	MediaType string // MIME type (e.g., "image/png")
}

// SizeKB returns the size of the image in kilobytes.
func (p *PendingImage) SizeKB() int {
	return len(p.Data) / 1024
}

// SpinnerState tracks the waiting/streaming spinner animation.
type SpinnerState struct {
	Idx         int    // Current spinner frame index
	Tick        int    // Tick counter for frame hold timing
	Verb        string // Random verb to display while waiting (e.g., "Thinking")
	StartTime   time.Time
	FlashFrame  int // Completion flash animation: -1 = inactive, 0-2 = animation frames
}

// NewSpinnerState creates a new SpinnerState.
func NewSpinnerState() *SpinnerState {
	return &SpinnerState{
		FlashFrame: -1,
	}
}
