package ui

import (
	"bytes"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/muesli/reflow/wordwrap"
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/mcp"
)

// optionsTagStripPattern matches <options>...</options> blocks for stripping from display.
var optionsTagStripPattern = regexp.MustCompile(`(?s)<options>\s*\n?(.*?)\n?\s*</options>`)

// optgroupTagStripPattern matches <optgroup>...</optgroup> blocks for stripping from display.
var optgroupTagStripPattern = regexp.MustCompile(`(?s)<optgroup>\s*\n?(.*?)\n?\s*</optgroup>`)

// stripOptionsTags removes <options>, </options>, <optgroup>, and </optgroup> tags
// from content for display, leaving only the numbered options inside.
func stripOptionsTags(content string) string {
	result := optionsTagStripPattern.ReplaceAllString(content, "$1")
	result = optgroupTagStripPattern.ReplaceAllString(result, "$1")
	return result
}

// Compiled regex patterns for markdown parsing
var (
	boldPattern       = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	underscoreItalic  = regexp.MustCompile(`(?:^|[^a-zA-Z0-9_])_([^_]+)_(?:[^a-zA-Z0-9_]|$)`)
	inlineCodePattern = regexp.MustCompile("`([^`]+)`")
	linkPattern       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
)

// StopwatchTickMsg is sent to update the animated waiting display
type StopwatchTickMsg time.Time

// CompletionFlashTickMsg is sent to animate the completion checkmark flash
type CompletionFlashTickMsg time.Time

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

// messageCache stores pre-rendered message content to avoid expensive re-rendering
type messageCache struct {
	content   string // The original message content
	rendered  string // The rendered output
	wrapWidth int    // The width used for wrapping
}

// Chat represents the right panel with conversation view
type Chat struct {
	viewport    viewport.Model
	input       textarea.Model
	width       int
	height      int
	focused     bool
	messages    []claude.Message
	streaming   string // Current streaming response
	sessionName string
	hasSession  bool
	waiting     bool   // Waiting for Claude's response
	waitingVerb string // Random verb to display while waiting
	spinnerIdx  int // Current spinner frame index
	spinnerTick int // Tick counter for frame hold timing

	// Completion flash animation
	completionFlashFrame int // -1 = inactive, 0-2 = animation frames

	// Message rendering cache - avoids re-rendering unchanged messages
	messageCache []messageCache // Cache of rendered messages, indexed by message position

	// Track last tool use position for marking as complete
	lastToolUsePos int // Position in streaming content where last tool use marker starts

	// Pending permission prompt
	hasPendingPermission   bool
	pendingPermissionTool  string
	pendingPermissionDesc  string

	// Pending question prompt
	hasPendingQuestion    bool
	pendingQuestions      []mcp.Question
	currentQuestionIdx    int                // Index of current question being answered
	selectedOptionIdx     int                // Currently highlighted option
	questionAnswers       map[string]string  // Collected answers (question text -> selected label)

	// View changes mode - temporary overlay showing git diff with file-by-file navigation
	viewChangesMode      bool             // Whether we're showing the diff overlay
	viewChangesViewport  viewport.Model   // Viewport for diff scrolling
	viewChangesFiles     []git.FileDiff   // List of files with diffs
	viewChangesFileIndex int              // Currently selected file index
	viewChangesFilePane  bool             // true = file list focused, false = diff pane focused

	// Pending image attachment
	pendingImageData []byte  // PNG encoded image data
	pendingImageType string  // MIME type
	pendingImageSize int     // Size in bytes

	// Queued message waiting to be sent after streaming completes
	queuedMessage string
}

// NewChat creates a new chat panel
func NewChat() *Chat {
	// Create text input
	ti := textarea.New()
	ti.Placeholder = "Type your message..."
	ti.CharLimit = 0
	ti.SetHeight(3)
	ti.ShowLineNumbers = false
	ti.Prompt = ""

	// Create viewport for messages
	vp := viewport.New()
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 3
	vp.SoftWrap = true // Wrap text instead of allowing horizontal scrolling

	c := &Chat{
		viewport:             vp,
		input:                ti,
		messages:             []claude.Message{},
		lastToolUsePos:       -1,
		completionFlashFrame: -1,
	}
	c.updateContent()
	return c
}

// SetSize sets the chat panel dimensions
func (c *Chat) SetSize(width, height int) {
	c.width = width
	c.height = height

	ctx := GetViewContext()

	// Chat panel height (excluding input area which is separate)
	chatPanelHeight := height - InputTotalHeight

	// Calculate inner dimensions for the chat panel (accounting for borders)
	innerWidth := ctx.InnerWidth(width)
	viewportHeight := ctx.InnerHeight(chatPanelHeight)

	if viewportHeight < 1 {
		viewportHeight = 1
	}

	c.viewport.SetWidth(innerWidth)
	c.viewport.SetHeight(viewportHeight)

	// Input width accounts for its own border AND padding
	inputInnerWidth := ctx.InnerWidth(width) - InputPaddingWidth
	c.input.SetWidth(inputInnerWidth)

	ctx.Log("Chat.SetSize: outer=%dx%d, chatPanel=%d, input=%d", width, height, chatPanelHeight, InputTotalHeight)
	ctx.Log("  Chat viewport: w=%d, h=%d", c.viewport.Width(), c.viewport.Height())
}

// SetFocused sets the focus state
func (c *Chat) SetFocused(focused bool) {
	c.focused = focused
	if focused {
		c.input.Focus()
	} else {
		c.input.Blur()
	}
}

// IsFocused returns the focus state
func (c *Chat) IsFocused() bool {
	return c.focused
}

// SetSession sets the current session info
func (c *Chat) SetSession(name string, messages []claude.Message) {
	c.sessionName = name
	c.messages = messages
	c.hasSession = true
	c.streaming = ""
	c.messageCache = nil // Clear cache on session change
	c.updateContent()
}

// ClearSession clears the current session
func (c *Chat) ClearSession() {
	c.sessionName = ""
	c.messages = nil
	c.hasSession = false
	c.streaming = ""
	c.lastToolUsePos = -1
	c.messageCache = nil // Clear cache on session clear
	c.hasPendingPermission = false
	c.pendingPermissionTool = ""
	c.pendingPermissionDesc = ""
	c.hasPendingQuestion = false
	c.pendingQuestions = nil
	c.currentQuestionIdx = 0
	c.selectedOptionIdx = 0
	c.questionAnswers = nil
	c.updateContent()
}

// AppendStreaming appends content to the current streaming response
func (c *Chat) AppendStreaming(content string) {
	// Add extra newline after tool use for visual separation
	if c.lastToolUsePos >= 0 && strings.HasSuffix(c.streaming, "\n") && !strings.HasSuffix(c.streaming, "\n\n") {
		c.streaming += "\n"
	}
	c.streaming += content
	c.updateContent()
}

// ToolUseInProgress is the white circle marker for tool use in progress
const ToolUseInProgress = "⏺"

// ToolUseComplete is the green circle marker for completed tool use
const ToolUseComplete = "●"

// AppendToolUse appends a formatted tool use line to the streaming content
func (c *Chat) AppendToolUse(toolName, toolInput string) {
	icon := GetToolIcon(toolName)
	line := ToolUseInProgress + " " + icon + "(" + toolName
	if toolInput != "" {
		line += ": " + toolInput
	}
	line += ")\n"

	// Add newline before if there's existing content that doesn't end with newline
	if c.streaming != "" && !strings.HasSuffix(c.streaming, "\n") {
		c.streaming += "\n"
	}
	// Track position where the marker starts
	c.lastToolUsePos = len(c.streaming)
	c.streaming += line
	c.updateContent()
}

// MarkLastToolUseComplete changes the last tool use marker from white to green
func (c *Chat) MarkLastToolUseComplete() {
	if c.lastToolUsePos >= 0 && c.lastToolUsePos < len(c.streaming) {
		// Check if the marker is at the expected position
		markerLen := len(ToolUseInProgress)
		if c.lastToolUsePos+markerLen <= len(c.streaming) {
			prefix := c.streaming[:c.lastToolUsePos]
			suffix := c.streaming[c.lastToolUsePos+markerLen:]
			c.streaming = prefix + ToolUseComplete + suffix
			c.updateContent()
		}
	}
	// Reset position after marking
	c.lastToolUsePos = -1
}

// FinishStreaming completes the streaming and adds to messages
func (c *Chat) FinishStreaming() {
	if c.streaming != "" {
		c.messages = append(c.messages, claude.Message{
			Role:    "assistant",
			Content: c.streaming,
		})
		c.streaming = ""
		c.updateContent()
	}
}

// AddUserMessage adds a user message
func (c *Chat) AddUserMessage(content string) {
	c.messages = append(c.messages, claude.Message{
		Role:    "user",
		Content: content,
	})
	c.updateContent()
}

// GetInput returns the current input text
func (c *Chat) GetInput() string {
	val := strings.TrimSpace(c.input.Value())
	logger.Log("Chat.GetInput: value=%q, len=%d", val, len(val))
	return val
}

// ClearInput clears the input field
func (c *Chat) ClearInput() {
	c.input.Reset()
}

// SetInput sets the input field value
func (c *Chat) SetInput(value string) {
	c.input.SetValue(value)
}

// SetQueuedMessage sets a message that is queued to be sent after streaming completes
func (c *Chat) SetQueuedMessage(msg string) {
	c.queuedMessage = msg
	c.updateContent()
}

// ClearQueuedMessage clears the queued message display
func (c *Chat) ClearQueuedMessage() {
	c.queuedMessage = ""
	c.updateContent()
}

// IsStreaming returns whether we're currently streaming a response
func (c *Chat) IsStreaming() bool {
	return c.streaming != ""
}

// EnterViewChangesMode enters the temporary diff view overlay with file-by-file navigation
func (c *Chat) EnterViewChangesMode(files []git.FileDiff) {
	c.viewChangesMode = true
	c.viewChangesFiles = files
	c.viewChangesFileIndex = 0
	c.viewChangesFilePane = false // Start with diff pane focused

	// Create a fresh viewport for the diff content
	c.viewChangesViewport = viewport.New()
	c.viewChangesViewport.MouseWheelEnabled = true
	c.viewChangesViewport.MouseWheelDelta = 3
	c.viewChangesViewport.SoftWrap = true

	// Size it - will be adjusted in render, but set initial size
	c.viewChangesViewport.SetWidth(c.viewport.Width() * 2 / 3)
	c.viewChangesViewport.SetHeight(c.viewport.Height())

	// Load the first file's diff
	c.updateViewChangesDiff()
}

// updateViewChangesDiff updates the diff viewport with the currently selected file's diff
func (c *Chat) updateViewChangesDiff() {
	if len(c.viewChangesFiles) == 0 {
		c.viewChangesViewport.SetContent("No files to display")
		return
	}
	if c.viewChangesFileIndex >= len(c.viewChangesFiles) {
		c.viewChangesFileIndex = len(c.viewChangesFiles) - 1
	}
	file := c.viewChangesFiles[c.viewChangesFileIndex]
	content := HighlightDiff(file.Diff)
	c.viewChangesViewport.SetContent(content)
	c.viewChangesViewport.GotoTop()
}

// ExitViewChangesMode exits the diff view overlay and returns to chat
func (c *Chat) ExitViewChangesMode() {
	c.viewChangesMode = false
	c.viewChangesFiles = nil
	c.viewChangesFileIndex = 0
	c.viewChangesFilePane = false
}

// IsInViewChangesMode returns whether we're currently showing the diff overlay
func (c *Chat) IsInViewChangesMode() bool {
	return c.viewChangesMode
}

// GetSelectedFileIndex returns the currently selected file index in view changes mode.
// Used for testing navigation.
func (c *Chat) GetSelectedFileIndex() int {
	return c.viewChangesFileIndex
}

// GetViewChangesFocus returns "files" if file list pane is focused, "diff" if diff pane is focused.
// Used for testing pane switching.
func (c *Chat) GetViewChangesFocus() string {
	if c.viewChangesFilePane {
		return "files"
	}
	return "diff"
}

// GetStreaming returns the current streaming content
func (c *Chat) GetStreaming() string {
	return c.streaming
}

// GetMessages returns the conversation messages
func (c *Chat) GetMessages() []claude.Message {
	return c.messages
}

// SetStreaming sets the streaming content (used when restoring session state)
func (c *Chat) SetStreaming(content string) {
	c.streaming = content
	c.updateContent()
}

// SetPendingPermission sets the pending permission prompt to display
func (c *Chat) SetPendingPermission(tool, description string) {
	c.hasPendingPermission = true
	c.pendingPermissionTool = tool
	c.pendingPermissionDesc = description
	c.updateContent()
}

// ClearPendingPermission clears the pending permission prompt
func (c *Chat) ClearPendingPermission() {
	c.hasPendingPermission = false
	c.pendingPermissionTool = ""
	c.pendingPermissionDesc = ""
	c.updateContent()
}

// HasPendingPermission returns whether there's a pending permission prompt
func (c *Chat) HasPendingPermission() bool {
	return c.hasPendingPermission
}

// SetPendingQuestion sets the pending question prompt to display
func (c *Chat) SetPendingQuestion(questions []mcp.Question) {
	c.hasPendingQuestion = true
	c.pendingQuestions = questions
	c.currentQuestionIdx = 0
	c.selectedOptionIdx = 0
	c.questionAnswers = make(map[string]string)
	c.updateContent()
}

// ClearPendingQuestion clears the pending question prompt
func (c *Chat) ClearPendingQuestion() {
	c.hasPendingQuestion = false
	c.pendingQuestions = nil
	c.currentQuestionIdx = 0
	c.selectedOptionIdx = 0
	c.questionAnswers = nil
	c.updateContent()
}

// HasPendingQuestion returns whether there's a pending question prompt
func (c *Chat) HasPendingQuestion() bool {
	return c.hasPendingQuestion
}

// GetQuestionAnswers returns the collected question answers
func (c *Chat) GetQuestionAnswers() map[string]string {
	return c.questionAnswers
}

// MoveQuestionSelection moves the selection up or down
func (c *Chat) MoveQuestionSelection(delta int) {
	if !c.hasPendingQuestion || c.currentQuestionIdx >= len(c.pendingQuestions) {
		return
	}
	q := c.pendingQuestions[c.currentQuestionIdx]
	numOptions := len(q.Options) + 1 // +1 for "Other" option
	c.selectedOptionIdx = (c.selectedOptionIdx + delta + numOptions) % numOptions
	c.updateContent()
}

// SelectCurrentOption selects the current option and moves to next question or completes
// Returns true if all questions are answered
func (c *Chat) SelectCurrentOption() bool {
	if !c.hasPendingQuestion || c.currentQuestionIdx >= len(c.pendingQuestions) {
		return true
	}
	q := c.pendingQuestions[c.currentQuestionIdx]

	// Determine the selected answer
	var answer string
	if c.selectedOptionIdx < len(q.Options) {
		answer = q.Options[c.selectedOptionIdx].Label
	} else {
		// "Other" selected - for now, just use empty string
		// A full implementation would allow text input
		answer = ""
	}

	c.questionAnswers[q.Question] = answer

	// Move to next question or complete
	c.currentQuestionIdx++
	c.selectedOptionIdx = 0

	if c.currentQuestionIdx >= len(c.pendingQuestions) {
		// All questions answered
		return true
	}

	c.updateContent()
	return false
}

// SelectOptionByNumber selects an option by its number (1-based)
// Returns true if all questions are answered after this selection
func (c *Chat) SelectOptionByNumber(num int) bool {
	if !c.hasPendingQuestion || c.currentQuestionIdx >= len(c.pendingQuestions) {
		return true
	}
	q := c.pendingQuestions[c.currentQuestionIdx]
	numOptions := len(q.Options) + 1 // +1 for "Other"

	// Convert 1-based to 0-based index
	idx := num - 1
	if idx < 0 || idx >= numOptions {
		return false
	}

	c.selectedOptionIdx = idx
	return c.SelectCurrentOption()
}

// AttachImage attaches an image to the pending message
func (c *Chat) AttachImage(data []byte, mediaType string) {
	c.pendingImageData = data
	c.pendingImageType = mediaType
	c.pendingImageSize = len(data)
	c.updateContent()
}

// ClearImage removes the pending image attachment
func (c *Chat) ClearImage() {
	c.pendingImageData = nil
	c.pendingImageType = ""
	c.pendingImageSize = 0
	c.updateContent()
}

// HasPendingImage returns whether there's a pending image attachment
func (c *Chat) HasPendingImage() bool {
	return len(c.pendingImageData) > 0
}

// GetPendingImage returns the pending image data and clears it
func (c *Chat) GetPendingImage() (data []byte, mediaType string) {
	data = c.pendingImageData
	mediaType = c.pendingImageType
	c.pendingImageData = nil
	c.pendingImageType = ""
	c.pendingImageSize = 0
	return data, mediaType
}

// GetPendingImageSizeKB returns the pending image size in KB
func (c *Chat) GetPendingImageSizeKB() int {
	return c.pendingImageSize / 1024
}

// renderNoSessionMessage renders the placeholder message when no session is selected
func (c *Chat) renderNoSessionMessage() string {
	msgStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	keyStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)

	var sb strings.Builder
	sb.WriteString(msgStyle.Italic(true).Render("No session selected"))
	sb.WriteString("\n\n")
	sb.WriteString(msgStyle.Render("To get started:"))
	sb.WriteString("\n")
	sb.WriteString(msgStyle.Render("  • Press "))
	sb.WriteString(keyStyle.Render("n"))
	sb.WriteString(msgStyle.Render(" to create a new session"))
	sb.WriteString("\n")
	sb.WriteString(msgStyle.Render("  • Press "))
	sb.WriteString(keyStyle.Render("r"))
	sb.WriteString(msgStyle.Render(" to add a repository first"))
	return sb.String()
}

// renderPermissionPrompt renders the inline permission prompt
func (c *Chat) renderPermissionPrompt(wrapWidth int) string {
	var sb strings.Builder

	// Title with tool name on same line: "⚠ Permission Required: Edit"
	sb.WriteString(PermissionTitleStyle.Render("⚠ Permission Required: "))
	sb.WriteString(PermissionToolStyle.Render(c.pendingPermissionTool))
	sb.WriteString("\n")

	// Description (wrapped)
	descStyle := PermissionDescStyle.Width(wrapWidth - 4) // Account for box padding
	sb.WriteString(descStyle.Render(c.pendingPermissionDesc))
	sb.WriteString("\n\n")

	// Keyboard hints - compact horizontal layout
	keyStyle := lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
	hintStyle := PermissionHintStyle

	sb.WriteString(keyStyle.Render("[y]"))
	sb.WriteString(hintStyle.Render(" Allow  "))
	sb.WriteString(keyStyle.Render("[n]"))
	sb.WriteString(hintStyle.Render(" Deny  "))
	sb.WriteString(keyStyle.Render("[a]"))
	sb.WriteString(hintStyle.Render(" Always"))

	// Wrap in a box - allow wider for horizontal content
	boxWidth := wrapWidth
	if boxWidth > 80 {
		boxWidth = 80
	}
	return PermissionBoxStyle.Width(boxWidth).Render(sb.String())
}

// renderQuestionPrompt renders the inline question prompt
func (c *Chat) renderQuestionPrompt(wrapWidth int) string {
	if !c.hasPendingQuestion || c.currentQuestionIdx >= len(c.pendingQuestions) {
		return ""
	}

	q := c.pendingQuestions[c.currentQuestionIdx]
	var sb strings.Builder

	// Question progress indicator (if multiple questions)
	if len(c.pendingQuestions) > 1 {
		progressStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		sb.WriteString(progressStyle.Render(fmt.Sprintf("Question %d of %d", c.currentQuestionIdx+1, len(c.pendingQuestions))))
		sb.WriteString("\n\n")
	}

	// Header/label
	headerStyle := lipgloss.NewStyle().Foreground(ColorInfo).Bold(true)
	sb.WriteString(headerStyle.Render("? "+q.Header+":"))
	sb.WriteString(" ")

	// Question text
	questionStyle := lipgloss.NewStyle().Foreground(ColorText)
	sb.WriteString(questionStyle.Render(q.Question))
	sb.WriteString("\n\n")

	// Render options
	for i, opt := range q.Options {
		isSelected := i == c.selectedOptionIdx

		// Number indicator
		numStyle := lipgloss.NewStyle().Foreground(ColorInfo).Bold(true)
		if isSelected {
			sb.WriteString(numStyle.Render(fmt.Sprintf("[%d]", i+1)))
		} else {
			sb.WriteString(numStyle.Render(fmt.Sprintf(" %d.", i+1)))
		}
		sb.WriteString(" ")

		// Option label
		labelStyle := lipgloss.NewStyle().Foreground(ColorText)
		if isSelected {
			labelStyle = labelStyle.Bold(true).Background(ColorPrimary).Foreground(ColorTextInverse)
		}
		sb.WriteString(labelStyle.Render(opt.Label))

		// Description if present
		if opt.Description != "" {
			descStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
			sb.WriteString(" ")
			sb.WriteString(descStyle.Render("- " + opt.Description))
		}
		sb.WriteString("\n")
	}

	// "Other" option (always last)
	otherIdx := len(q.Options)
	isOtherSelected := c.selectedOptionIdx == otherIdx
	numStyle := lipgloss.NewStyle().Foreground(ColorInfo).Bold(true)
	if isOtherSelected {
		sb.WriteString(numStyle.Render(fmt.Sprintf("[%d]", otherIdx+1)))
	} else {
		sb.WriteString(numStyle.Render(fmt.Sprintf(" %d.", otherIdx+1)))
	}
	sb.WriteString(" ")
	labelStyle := lipgloss.NewStyle().Foreground(ColorText)
	if isOtherSelected {
		labelStyle = labelStyle.Bold(true).Background(ColorPrimary).Foreground(ColorTextInverse)
	}
	sb.WriteString(labelStyle.Render("Other"))
	sb.WriteString("\n\n")

	// Keyboard hints
	hintStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	keyStyle := lipgloss.NewStyle().Foreground(ColorInfo).Bold(true)
	sb.WriteString(hintStyle.Render("Press "))
	sb.WriteString(keyStyle.Render("1-" + fmt.Sprintf("%d", len(q.Options)+1)))
	sb.WriteString(hintStyle.Render(" to select, or "))
	sb.WriteString(keyStyle.Render("↑/↓"))
	sb.WriteString(hintStyle.Render(" + "))
	sb.WriteString(keyStyle.Render("enter"))

	// Wrap in a box
	boxWidth := wrapWidth
	if boxWidth > 80 {
		boxWidth = 80
	}
	return QuestionBoxStyle.Width(boxWidth).Render(sb.String())
}

// GetToolIcon returns an appropriate icon for the tool type
func GetToolIcon(toolName string) string {
	switch toolName {
	case "Read":
		return "Reading"
	case "Edit":
		return "Editing"
	case "Write":
		return "Writing"
	case "Glob":
		return "Searching"
	case "Grep":
		return "Searching"
	case "Bash":
		return "Running"
	case "Task":
		return "Delegating"
	case "WebFetch":
		return "Fetching"
	case "WebSearch":
		return "Searching"
	case "TodoWrite":
		return "Planning"
	default:
		return "Using"
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

	// Frame 0: bright green checkmark
	// Frame 1: normal green checkmark
	// Frame 2+: fade out (empty)
	switch frame {
	case 0:
		// Bright green
		style := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#22C55E")).
			Bold(true)
		return style.Render(checkmark) + " " + lipgloss.NewStyle().Foreground(ColorSecondary).Italic(true).Render("Done")
	case 1:
		// Normal green (using theme's secondary color which is cyan/teal)
		style := lipgloss.NewStyle().
			Foreground(ColorSecondary)
		return style.Render(checkmark)
	default:
		return ""
	}
}

// highlightCode applies syntax highlighting to code using chroma
func highlightCode(code, language string) string {
	lexer := lexers.Get(language)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	style := styles.Get("monokai")
	if style == nil {
		style = styles.Fallback
	}

	formatter := formatters.Get("terminal256")
	if formatter == nil {
		formatter = formatters.Fallback
	}

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, iterator); err != nil {
		return code
	}

	return buf.String()
}

// HighlightDiff applies coloring to git diff output
func HighlightDiff(diff string) string {
	if diff == "" {
		return diff
	}

	var result strings.Builder
	lines := strings.Split(diff, "\n")

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			// File headers
			result.WriteString(DiffHeaderStyle.Render(line))
		case strings.HasPrefix(line, "@@"):
			// Hunk markers
			result.WriteString(DiffHunkStyle.Render(line))
		case strings.HasPrefix(line, "+"):
			// Added lines
			result.WriteString(DiffAddedStyle.Render(line))
		case strings.HasPrefix(line, "-"):
			// Removed lines
			result.WriteString(DiffRemovedStyle.Render(line))
		case strings.HasPrefix(line, "diff --git"):
			// Diff command header
			result.WriteString(DiffHeaderStyle.Render(line))
		case strings.HasPrefix(line, "index "):
			// Index line
			result.WriteString(DiffHeaderStyle.Render(line))
		case strings.HasPrefix(line, "new file mode") || strings.HasPrefix(line, "deleted file mode"):
			// File mode changes
			result.WriteString(DiffHeaderStyle.Render(line))
		default:
			// Context lines (unchanged)
			result.WriteString(line)
		}
		result.WriteString("\n")
	}

	return strings.TrimRight(result.String(), "\n")
}

// renderInlineMarkdown applies inline formatting (bold, italic, code, links) to a line
func renderInlineMarkdown(line string) string {
	// Apply tool use marker coloring first
	// White circle for in-progress tools
	line = strings.ReplaceAll(line, ToolUseInProgress, ToolUseInProgressStyle.Render(ToolUseInProgress))
	// Green circle for completed tools
	line = strings.ReplaceAll(line, ToolUseComplete, ToolUseCompleteStyle.Render(ToolUseComplete))

	// Process inline code first (to avoid formatting inside code)
	// We need to protect code spans from other formatting
	type codeSpan struct {
		placeholder string
		original    string
		rendered    string
	}
	var codeSpans []codeSpan
	codeIdx := 0

	// Extract and replace inline code with placeholders
	line = inlineCodePattern.ReplaceAllStringFunc(line, func(match string) string {
		code := inlineCodePattern.FindStringSubmatch(match)[1]
		placeholder := fmt.Sprintf("\x00CODE%d\x00", codeIdx)
		codeSpans = append(codeSpans, codeSpan{
			placeholder: placeholder,
			original:    match,
			rendered:    MarkdownInlineCodeStyle.Render(code),
		})
		codeIdx++
		return placeholder
	})

	// Process bold (**text**)
	line = boldPattern.ReplaceAllStringFunc(line, func(match string) string {
		text := boldPattern.FindStringSubmatch(match)[1]
		return MarkdownBoldStyle.Render(text)
	})

	// Process italic with underscores (_text_)
	// Only match underscores at word boundaries (not in identifiers like foo_bar_baz)
	line = underscoreItalic.ReplaceAllStringFunc(line, func(match string) string {
		submatch := underscoreItalic.FindStringSubmatch(match)
		text := submatch[1]
		// Preserve any prefix/suffix boundary characters that were matched
		prefix := ""
		suffix := ""
		// The regex may have matched a leading non-word character
		if len(match) > 0 && len(text)+2 < len(match) {
			// Find where _text_ starts and ends within the match
			start := strings.Index(match, "_"+text+"_")
			if start > 0 {
				prefix = match[:start]
			}
			end := start + len("_"+text+"_")
			if end < len(match) {
				suffix = match[end:]
			}
		}
		return prefix + MarkdownItalicStyle.Render(text) + suffix
	})

	// Process links [text](url)
	line = linkPattern.ReplaceAllStringFunc(line, func(match string) string {
		parts := linkPattern.FindStringSubmatch(match)
		text := parts[1]
		url := parts[2]
		return MarkdownLinkStyle.Render(text) + " (" + MarkdownLinkStyle.Render(url) + ")"
	})

	// Restore code spans
	for _, cs := range codeSpans {
		line = strings.Replace(line, cs.placeholder, cs.rendered, 1)
	}

	return line
}

// wrapText wraps text to the specified width, handling ANSI escape codes
func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	return wordwrap.String(text, width)
}

// renderMarkdownLine renders a single line with markdown formatting
func renderMarkdownLine(line string, width int) string {
	trimmed := strings.TrimSpace(line)

	// Headers - don't wrap, they should be concise
	if strings.HasPrefix(trimmed, "#### ") {
		return MarkdownH4Style.Render(strings.TrimPrefix(trimmed, "#### "))
	}
	if strings.HasPrefix(trimmed, "### ") {
		return MarkdownH3Style.Render(strings.TrimPrefix(trimmed, "### "))
	}
	if strings.HasPrefix(trimmed, "## ") {
		return MarkdownH2Style.Render(strings.TrimPrefix(trimmed, "## "))
	}
	if strings.HasPrefix(trimmed, "# ") {
		return MarkdownH1Style.Render(strings.TrimPrefix(trimmed, "# "))
	}

	// Horizontal rule
	if trimmed == "---" || trimmed == "***" || trimmed == "___" {
		return MarkdownHRStyle.Render("────────────────────────────────")
	}

	// Blockquote
	if strings.HasPrefix(trimmed, "> ") {
		content := strings.TrimPrefix(trimmed, "> ")
		return MarkdownBlockquoteStyle.Render(wrapText(renderInlineMarkdown(content), width-4))
	}

	// Unordered list items
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
		content := trimmed[2:]
		bullet := MarkdownListBulletStyle.Render("•")
		// Wrap list item content, accounting for indent and bullet
		wrapped := wrapText(renderInlineMarkdown(content), width-6)
		// Indent continuation lines
		lines := strings.Split(wrapped, "\n")
		if len(lines) > 1 {
			for i := 1; i < len(lines); i++ {
				lines[i] = "    " + lines[i]
			}
			wrapped = strings.Join(lines, "\n")
		}
		return "  " + bullet + " " + wrapped
	}

	// Numbered list items
	for i := 1; i <= 99; i++ {
		prefix := fmt.Sprintf("%d. ", i)
		if strings.HasPrefix(trimmed, prefix) {
			content := strings.TrimPrefix(trimmed, prefix)
			number := MarkdownListBulletStyle.Render(fmt.Sprintf("%d.", i))
			// Wrap list item content, accounting for indent and number
			wrapped := wrapText(renderInlineMarkdown(content), width-6)
			// Indent continuation lines
			lines := strings.Split(wrapped, "\n")
			if len(lines) > 1 {
				for j := 1; j < len(lines); j++ {
					lines[j] = "     " + lines[j]
				}
				wrapped = strings.Join(lines, "\n")
			}
			return "  " + number + " " + wrapped
		}
	}

	// Regular line with inline formatting and wrapping
	return wrapText(renderInlineMarkdown(line), width)
}

// renderMarkdown renders markdown content with syntax-highlighted code blocks
func renderMarkdown(content string, width int) string {
	if width <= 0 {
		width = DefaultWrapWidth
	}

	var result strings.Builder
	lines := strings.Split(content, "\n")
	inCodeBlock := false
	codeBlockLang := ""
	var codeBlockContent strings.Builder

	for _, line := range lines {
		// Check for code block start/end
		if strings.HasPrefix(line, "```") {
			if !inCodeBlock {
				// Starting a code block
				inCodeBlock = true
				codeBlockLang = strings.TrimPrefix(line, "```")
				codeBlockLang = strings.TrimSpace(codeBlockLang)
				codeBlockContent.Reset()
			} else {
				// Ending a code block - render with syntax highlighting
				inCodeBlock = false
				highlighted := highlightCode(codeBlockContent.String(), codeBlockLang)
				// Add a newline before and after code blocks for spacing
				if result.Len() > 0 {
					result.WriteString("\n")
				}
				result.WriteString(highlighted)
				result.WriteString("\n")
				codeBlockLang = ""
			}
			continue
		}

		if inCodeBlock {
			if codeBlockContent.Len() > 0 {
				codeBlockContent.WriteString("\n")
			}
			codeBlockContent.WriteString(line)
		} else {
			// Render markdown line with wrapping
			result.WriteString(renderMarkdownLine(line, width))
			result.WriteString("\n")
		}
	}

	// If we ended while still in a code block, output whatever we have
	if inCodeBlock {
		highlighted := highlightCode(codeBlockContent.String(), codeBlockLang)
		result.WriteString(highlighted)
	}

	return strings.TrimRight(result.String(), "\n")
}

func (c *Chat) updateContent() {
	var sb strings.Builder

	// Get wrap width (use viewport width, fallback to reasonable default)
	wrapWidth := c.viewport.Width()
	if wrapWidth <= 0 {
		wrapWidth = DefaultWrapWidth
	}

	if !c.hasSession {
		sb.WriteString(c.renderNoSessionMessage())
	} else if len(c.messages) == 0 && c.streaming == "" {
		sb.WriteString(lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			Render("Start a conversation with Claude..."))
	} else {
		// Ensure cache is properly sized
		if len(c.messageCache) > len(c.messages) {
			// Messages were removed (session change), truncate cache
			c.messageCache = c.messageCache[:len(c.messages)]
		}

		for i, msg := range c.messages {
			if i > 0 {
				sb.WriteString("\n\n")
			}

			var roleStyle lipgloss.Style
			var roleName string
			if msg.Role == "user" {
				roleStyle = ChatUserStyle
				roleName = "You"
			} else {
				roleStyle = ChatAssistantStyle
				roleName = "Claude"
			}

			sb.WriteString(roleStyle.Render(roleName + ":"))
			sb.WriteString("\n")

			// Check cache for this message
			content := stripOptionsTags(strings.TrimSpace(msg.Content))
			var renderedContent string

			if i < len(c.messageCache) {
				cached := c.messageCache[i]
				if cached.content == content && cached.wrapWidth == wrapWidth {
					// Cache hit - use pre-rendered content
					renderedContent = cached.rendered
				} else {
					// Cache miss - content or width changed, re-render
					renderedContent = renderMarkdown(content, wrapWidth)
					c.messageCache[i] = messageCache{
						content:   content,
						rendered:  renderedContent,
						wrapWidth: wrapWidth,
					}
				}
			} else {
				// New message - render and add to cache
				renderedContent = renderMarkdown(content, wrapWidth)
				c.messageCache = append(c.messageCache, messageCache{
					content:   content,
					rendered:  renderedContent,
					wrapWidth: wrapWidth,
				})
			}

			sb.WriteString(renderedContent)
		}

		// Show streaming content or waiting indicator with stopwatch
		if c.streaming != "" {
			if len(c.messages) > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(ChatAssistantStyle.Render("Claude:"))
			sb.WriteString("\n")
			// Render markdown for streaming content, stripping <options> tags
			// Tool use lines are already included in streaming content with circle markers
			streamContent := stripOptionsTags(strings.TrimSpace(c.streaming))
			sb.WriteString(renderMarkdown(streamContent, wrapWidth))
		} else if c.waiting {
			if len(c.messages) > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(ChatAssistantStyle.Render("Claude:"))
			sb.WriteString("\n")
			sb.WriteString(renderSpinner(c.waitingVerb, c.spinnerIdx))
		} else if c.completionFlashFrame >= 0 {
			// Show completion flash animation
			if len(c.messages) > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(ChatAssistantStyle.Render("Claude:"))
			sb.WriteString("\n")
			sb.WriteString(renderCompletionFlash(c.completionFlashFrame))
		}

		// Show queued message waiting to be sent
		if c.queuedMessage != "" {
			sb.WriteString("\n\n")
			queuedStyle := lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Italic(true)
			sb.WriteString(queuedStyle.Render("You (queued):"))
			sb.WriteString("\n")
			sb.WriteString(queuedStyle.Render(c.queuedMessage))
		}

		// Show pending permission prompt
		if c.hasPendingPermission {
			if len(c.messages) > 0 || c.streaming != "" || c.waiting {
				sb.WriteString("\n\n")
			}
			sb.WriteString(c.renderPermissionPrompt(wrapWidth))
		}

		// Show pending question prompt
		if c.hasPendingQuestion {
			if len(c.messages) > 0 || c.streaming != "" || c.waiting || c.hasPendingPermission {
				sb.WriteString("\n\n")
			}
			sb.WriteString(c.renderQuestionPrompt(wrapWidth))
		}
	}

	c.viewport.SetContent(sb.String())
	c.viewport.GotoBottom()
}

// Update handles messages
func (c *Chat) Update(msg tea.Msg) (*Chat, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle view changes mode first - it intercepts all input
	if c.viewChangesMode {
		if keyMsg, isKey := msg.(tea.KeyPressMsg); isKey {
			key := keyMsg.String()
			switch key {
			case "esc", "q":
				// Exit view changes mode
				c.ExitViewChangesMode()
				return c, nil
			case "left", "h":
				// Focus file list pane
				c.viewChangesFilePane = true
				return c, nil
			case "right", "l", "enter":
				// Focus diff pane
				c.viewChangesFilePane = false
				return c, nil
			case "up", "k":
				if c.viewChangesFilePane {
					// Navigate file list up
					if c.viewChangesFileIndex > 0 {
						c.viewChangesFileIndex--
						c.updateViewChangesDiff()
					}
				} else {
					// Scroll diff viewport
					var cmd tea.Cmd
					c.viewChangesViewport, cmd = c.viewChangesViewport.Update(msg)
					cmds = append(cmds, cmd)
				}
				return c, tea.Batch(cmds...)
			case "down", "j":
				if c.viewChangesFilePane {
					// Navigate file list down
					if c.viewChangesFileIndex < len(c.viewChangesFiles)-1 {
						c.viewChangesFileIndex++
						c.updateViewChangesDiff()
					}
				} else {
					// Scroll diff viewport
					var cmd tea.Cmd
					c.viewChangesViewport, cmd = c.viewChangesViewport.Update(msg)
					cmds = append(cmds, cmd)
				}
				return c, tea.Batch(cmds...)
			case "pgup", "pgdown", "ctrl+up", "ctrl+down", "home", "end",
				"page up", "page down", "ctrl+u", "ctrl+d":
				// Page scroll keys always go to diff viewport
				var cmd tea.Cmd
				c.viewChangesViewport, cmd = c.viewChangesViewport.Update(msg)
				cmds = append(cmds, cmd)
				return c, tea.Batch(cmds...)
			}
			// Ignore other keys in view changes mode
			return c, nil
		}
		// Pass non-key events (like mouse wheel) to viewport
		var cmd tea.Cmd
		c.viewChangesViewport, cmd = c.viewChangesViewport.Update(msg)
		cmds = append(cmds, cmd)
		return c, tea.Batch(cmds...)
	}

	switch msg.(type) {
	case StopwatchTickMsg:
		if c.waiting {
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
			cmds = append(cmds, StopwatchTick())
		}
		return c, tea.Batch(cmds...)

	case CompletionFlashTickMsg:
		if c.completionFlashFrame >= 0 {
			c.completionFlashFrame++
			if c.completionFlashFrame >= 3 {
				// Animation complete
				c.completionFlashFrame = -1
			}
			c.updateContent()
			if c.completionFlashFrame >= 0 {
				cmds = append(cmds, CompletionFlashTick())
			}
		}
		return c, tea.Batch(cmds...)
	}

	if c.focused && c.hasSession {
		// Check if this is a scroll key before sending to input
		if keyMsg, isKey := msg.(tea.KeyPressMsg); isKey {
			key := keyMsg.String()
			// Allow scroll keys to pass through to viewport
			switch key {
			case "pgup", "pgdown", "ctrl+up", "ctrl+down", "home", "end",
				"page up", "page down", "ctrl+u", "ctrl+d":
				// Pass to viewport for scrolling
				var cmd tea.Cmd
				c.viewport, cmd = c.viewport.Update(msg)
				cmds = append(cmds, cmd)
				return c, tea.Batch(cmds...)
			}
		}

		var cmd tea.Cmd
		c.input, cmd = c.input.Update(msg)
		cmds = append(cmds, cmd)

		// Don't pass other key events to viewport when input is focused
		// This prevents spacebar/arrows from scrolling while typing
		if _, isKey := msg.(tea.KeyPressMsg); isKey {
			return c, tea.Batch(cmds...)
		}
	}

	// Update viewport for scrolling (non-key events, or when not focused)
	var cmd tea.Cmd
	c.viewport, cmd = c.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return c, tea.Batch(cmds...)
}

// View renders the chat panel
func (c *Chat) View() string {
	panelStyle := PanelStyle
	if c.focused {
		panelStyle = PanelFocusedStyle
	}

	// View changes mode: show diff overlay instead of chat
	if c.viewChangesMode {
		return c.renderViewChangesMode(panelStyle)
	}

	// Viewport content - render placeholder directly if no session
	var viewportContent string
	if !c.hasSession {
		viewportContent = c.renderNoSessionMessage()
	} else {
		viewportContent = c.viewport.View()
	}

	if !c.hasSession {
		// No session: just show the panel with placeholder
		return panelStyle.Width(c.width).Height(c.height).Render(viewportContent)
	}

	// With session: chat history panel + input area below it
	// Calculate heights: chat panel gets remaining space after input
	chatPanelHeight := c.height - InputTotalHeight

	// Render chat history in its own bordered panel
	chatPanel := panelStyle.Width(c.width).Height(chatPanelHeight).Render(viewportContent)

	// Input area with its own border
	inputStyle := ChatInputStyle
	if c.focused {
		inputStyle = ChatInputFocusedStyle
	}

	// Build input area content with optional image indicator
	var inputContent string
	if c.HasPendingImage() {
		// Show image attachment indicator above the textarea
		theme := CurrentTheme()
		indicatorStyle := lipgloss.NewStyle().
			Foreground(ColorInfo).
			Background(lipgloss.Color(theme.BgDark)).
			Padding(0, 1)
		indicator := indicatorStyle.Render(fmt.Sprintf("[Image attached: %dKB] (backspace to remove)", c.GetPendingImageSizeKB()))
		inputContent = indicator + "\n" + c.input.View()
	} else {
		inputContent = c.input.View()
	}
	inputArea := inputStyle.Width(c.width).Render(inputContent)

	return lipgloss.JoinVertical(lipgloss.Left, chatPanel, inputArea)
}

// renderViewChangesMode renders the diff overlay view
func (c *Chat) renderViewChangesMode(panelStyle lipgloss.Style) string {
	// Calculate dimensions for split-pane layout
	innerWidth := c.width - 2 // Account for panel border
	innerHeight := c.height - 2
	fileListWidth := innerWidth / 3
	dividerWidth := 1
	diffWidth := innerWidth - fileListWidth - dividerWidth

	// Build file list with selection indicator
	var fileLines []string
	for i, f := range c.viewChangesFiles {
		indicator := "  "
		if i == c.viewChangesFileIndex {
			indicator = "> "
		}
		line := fmt.Sprintf("%s[%s] %s", indicator, f.Status, f.Filename)
		// Truncate if too long for the file list width
		maxLen := fileListWidth - 2
		if maxLen > 0 && len(line) > maxLen {
			line = line[:maxLen-1] + "…"
		}
		// Highlight selected file
		if i == c.viewChangesFileIndex {
			line = ViewChangesSelectedStyle.Render(line)
		}
		fileLines = append(fileLines, line)
	}

	// Pad file list to fill height
	for len(fileLines) < innerHeight {
		fileLines = append(fileLines, "")
	}
	fileListContent := strings.Join(fileLines[:innerHeight], "\n")

	// Build divider
	dividerLines := make([]string, innerHeight)
	for i := range dividerLines {
		dividerLines[i] = "│"
	}
	dividerContent := strings.Join(dividerLines, "\n")

	// Update diff viewport size
	c.viewChangesViewport.SetWidth(diffWidth)
	c.viewChangesViewport.SetHeight(innerHeight)

	// Style for file list pane - highlight border if focused
	fileListStyle := lipgloss.NewStyle().Width(fileListWidth)
	if c.viewChangesFilePane {
		fileListStyle = fileListStyle.Foreground(lipgloss.Color("#60A5FA")) // Blue when focused
	}

	// Join the panes horizontally
	content := lipgloss.JoinHorizontal(
		lipgloss.Top,
		fileListStyle.Render(fileListContent),
		lipgloss.NewStyle().Width(dividerWidth).Foreground(lipgloss.Color("#4B5563")).Render(dividerContent),
		lipgloss.NewStyle().Width(diffWidth).Render(c.viewChangesViewport.View()),
	)

	return panelStyle.Width(c.width).Height(c.height).Render(content)
}
