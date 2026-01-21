package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	pclaude "github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/mcp"
)

// ToolUseInProgress is the white circle marker for tool use in progress
const ToolUseInProgress = "⏺"

// ToolUseComplete is the green circle marker for completed tool use
const ToolUseComplete = "●"

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
	messages    []pclaude.Message
	streaming   string // Current streaming response
	sessionName string
	hasSession  bool
	waiting     bool   // Waiting for Claude's response
	waitingVerb string // Random verb to display while waiting
	spinnerIdx  int    // Current spinner frame index
	spinnerTick int    // Tick counter for frame hold timing

	// Completion flash animation
	completionFlashFrame int // -1 = inactive, 0-2 = animation frames

	// Message rendering cache - avoids re-rendering unchanged messages
	messageCache []messageCache // Cache of rendered messages, indexed by message position

	// Track last tool use position for marking as complete
	lastToolUsePos int // Position in streaming content where last tool use marker starts

	// Pending permission prompt
	hasPendingPermission  bool
	pendingPermissionTool string
	pendingPermissionDesc string

	// Pending question prompt
	hasPendingQuestion bool
	pendingQuestions   []mcp.Question
	currentQuestionIdx int               // Index of current question being answered
	selectedOptionIdx  int               // Currently highlighted option
	questionAnswers    map[string]string // Collected answers (question text -> selected label)

	// Pending plan approval prompt
	hasPendingPlanApproval bool
	pendingPlan            string             // The plan content (markdown)
	pendingAllowedPrompts  []mcp.AllowedPrompt // Requested Bash permissions
	planScrollOffset       int                 // Scroll offset for viewing the plan

	// View changes mode - temporary overlay showing git diff with file navigation
	viewChangesMode      bool           // Whether we're showing the diff overlay
	viewChangesViewport  viewport.Model // Viewport for diff scrolling
	viewChangesFiles     []git.FileDiff // List of files with diffs
	viewChangesFileIndex int            // Currently selected file index

	// Pending image attachment
	pendingImageData []byte  // PNG encoded image data
	pendingImageType string  // MIME type
	pendingImageSize int     // Size in bytes

	// Queued message waiting to be sent after streaming completes
	queuedMessage string

	// Todo list display state
	hasTodoList     bool
	currentTodoList *pclaude.TodoList

	// Text selection state
	selectionStartCol  int
	selectionStartLine int
	selectionEndCol    int
	selectionEndLine   int
	selectionActive    bool // true during drag

	// Click tracking for double/triple click detection
	lastClickTime time.Time
	lastClickX    int
	lastClickY    int
	clickCount    int

	// Selection flash animation (brief highlight after copy, then clear)
	selectionFlashFrame int // -1 = inactive, 0 = flash visible, 1+ = done
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

	// Apply theme-aware styles to textarea
	applyTextareaStyles(&ti)

	// Create viewport for messages
	vp := viewport.New()
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 3
	// SoftWrap disabled - we handle wrapping manually in renderMarkdown
	// Having both causes issues with line spacing
	vp.SoftWrap = false

	c := &Chat{
		viewport:             vp,
		input:                ti,
		messages:             []pclaude.Message{},
		lastToolUsePos:       -1,
		completionFlashFrame: -1,
		selectionFlashFrame:  -1,
	}
	c.updateContent()
	return c
}

// applyTextareaStyles configures the textarea with theme-aware colors
func applyTextareaStyles(ti *textarea.Model) {
	theme := CurrentTheme()

	// Get current styles and modify them
	styles := ti.Styles()

	// Create style states for focused and blurred
	// Don't set background - let terminal's native background show through
	baseStyle := lipgloss.NewStyle()

	textStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.Text))

	placeholderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.TextMuted))

	// Configure focused state
	styles.Focused.Base = baseStyle
	styles.Focused.Text = textStyle
	styles.Focused.Placeholder = placeholderStyle
	styles.Focused.CursorLine = textStyle
	styles.Focused.Prompt = textStyle

	// Configure blurred state (same colors, just not focused)
	styles.Blurred.Base = baseStyle
	styles.Blurred.Text = textStyle
	styles.Blurred.Placeholder = placeholderStyle
	styles.Blurred.CursorLine = textStyle
	styles.Blurred.Prompt = textStyle

	ti.SetStyles(styles)
}

// SetSize sets the chat panel dimensions
func (c *Chat) SetSize(width, height int) {
	// Check if viewport width changed - if so, invalidate message cache
	// since messages are wrapped based on viewport width
	ctx := GetViewContext()
	newInnerWidth := ctx.InnerWidth(width)
	wasUninitialized := c.viewport.Width() <= 0
	widthChanged := c.viewport.Width() != newInnerWidth && c.viewport.Width() > 0
	if widthChanged {
		c.messageCache = nil // Clear cache to force re-render at new width
	}

	c.width = width
	c.height = height

	// Chat panel height (excluding input area which is separate)
	chatPanelHeight := height - InputTotalHeight

	// Calculate inner dimensions for the chat panel (accounting for borders)
	innerWidth := newInnerWidth
	viewportHeight := ctx.InnerHeight(chatPanelHeight)

	if viewportHeight < 1 {
		viewportHeight = 1
	}

	c.viewport.SetWidth(innerWidth)
	c.viewport.SetHeight(viewportHeight)

	// Input width accounts for its own border AND padding
	inputInnerWidth := ctx.InnerWidth(width) - InputPaddingWidth
	c.input.SetWidth(inputInnerWidth)

	// Re-render content if viewport was uninitialized or width changed
	// This ensures text is wrapped correctly for the new dimensions
	if (wasUninitialized || widthChanged) && innerWidth > 0 {
		c.updateContent()
	}

	ctx.Log("Chat.SetSize",
		"outerWidth", width,
		"outerHeight", height,
		"chatPanelHeight", chatPanelHeight,
		"inputTotalHeight", InputTotalHeight,
		"viewportWidth", c.viewport.Width(),
		"viewportHeight", c.viewport.Height(),
	)
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

// RefreshStyles updates the textarea styles after a theme change
func (c *Chat) RefreshStyles() {
	applyTextareaStyles(&c.input)
}

// SetSession sets the current session info
func (c *Chat) SetSession(name string, messages []pclaude.Message) {
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
	c.waiting = false
	c.completionFlashFrame = -1
	c.queuedMessage = ""
	c.hasTodoList = false
	c.currentTodoList = nil
	c.updateContent()
}

// AppendStreaming appends content to the current streaming response
func (c *Chat) AppendStreaming(content string) {
	// Add extra newline after tool use for visual separation
	if c.lastToolUsePos >= 0 && strings.HasSuffix(c.streaming, "\n") && !strings.HasSuffix(c.streaming, "\n\n") && !strings.HasPrefix(content, "\n") {
		c.streaming += "\n"
	}
	c.streaming += content
	c.updateContent()
}

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
		c.messages = append(c.messages, pclaude.Message{
			Role:    "assistant",
			Content: c.streaming,
		})
		c.streaming = ""
		c.lastToolUsePos = -1 // Reset tool tracking to prevent stale state affecting future streaming
		c.updateContent()
	}
}

// AddUserMessage adds a user message
func (c *Chat) AddUserMessage(content string) {
	c.messages = append(c.messages, pclaude.Message{
		Role:    "user",
		Content: content,
	})
	c.updateContent()
}

// AddSystemMessage adds a system/assistant message (for local command responses)
func (c *Chat) AddSystemMessage(content string) {
	c.messages = append(c.messages, pclaude.Message{
		Role:    "assistant",
		Content: content,
	})
	c.updateContent()
}

// GetInput returns the current input text
func (c *Chat) GetInput() string {
	val := strings.TrimSpace(c.input.Value())
	logger.Get().Debug("Chat.GetInput", "value", val, "len", len(val))
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

// GetStreaming returns the current streaming content
func (c *Chat) GetStreaming() string {
	return c.streaming
}

// GetMessages returns the conversation messages
func (c *Chat) GetMessages() []pclaude.Message {
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

// SetPendingPlanApproval sets the pending plan approval to display
func (c *Chat) SetPendingPlanApproval(plan string, allowedPrompts []mcp.AllowedPrompt) {
	c.hasPendingPlanApproval = true
	c.pendingPlan = plan
	c.pendingAllowedPrompts = allowedPrompts
	c.planScrollOffset = 0
	c.updateContent()
}

// ClearPendingPlanApproval clears the pending plan approval prompt
func (c *Chat) ClearPendingPlanApproval() {
	c.hasPendingPlanApproval = false
	c.pendingPlan = ""
	c.pendingAllowedPrompts = nil
	c.planScrollOffset = 0
	c.updateContent()
}

// HasPendingPlanApproval returns whether there's a pending plan approval prompt
func (c *Chat) HasPendingPlanApproval() bool {
	return c.hasPendingPlanApproval
}

// ScrollPlan scrolls the plan view by the given delta
func (c *Chat) ScrollPlan(delta int) {
	if !c.hasPendingPlanApproval {
		return
	}
	c.planScrollOffset += delta
	if c.planScrollOffset < 0 {
		c.planScrollOffset = 0
	}
	c.updateContent()
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

// SetTodoList sets the current todo list to display
// If the list is complete (all items done), it gets "baked" into the message
// history so it scrolls like normal messages instead of staying pinned at bottom
func (c *Chat) SetTodoList(list *pclaude.TodoList) {
	if list != nil && list.IsComplete() {
		// Bake the completed todo list into messages as rendered content
		wrapWidth := c.viewport.Width()
		if wrapWidth < TodoListMinWrapWidth {
			wrapWidth = TodoListFallbackWrapWidth
		}
		renderedTodo := renderTodoList(list, wrapWidth)
		c.messages = append(c.messages, pclaude.Message{
			Role:    "assistant",
			Content: renderedTodo,
		})
		// Clear the live todo list since it's now in history
		c.hasTodoList = false
		c.currentTodoList = nil
	} else {
		c.hasTodoList = list != nil && len(list.Items) > 0
		c.currentTodoList = list
	}
	c.updateContent()
}

// ClearTodoList clears the todo list display
func (c *Chat) ClearTodoList() {
	c.hasTodoList = false
	c.currentTodoList = nil
	c.updateContent()
}

// HasTodoList returns whether there's a todo list to display
func (c *Chat) HasTodoList() bool {
	return c.hasTodoList
}

// GetTodoList returns the current todo list
func (c *Chat) GetTodoList() *pclaude.TodoList {
	return c.currentTodoList
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
	sb.WriteString(headerStyle.Render("? " + q.Header + ":"))
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

	// Wrap in a box, capped at max width for readability
	boxWidth := wrapWidth
	if boxWidth > OverlayBoxMaxWidth {
		boxWidth = OverlayBoxMaxWidth
	}
	return QuestionBoxStyle.Width(boxWidth).Render(sb.String())
}

// renderPlanApprovalPrompt renders the inline plan approval prompt
func (c *Chat) renderPlanApprovalPrompt(wrapWidth int) string {
	if !c.hasPendingPlanApproval {
		return ""
	}

	var sb strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(ColorInfo).Bold(true)
	sb.WriteString(titleStyle.Render("📋 Plan Approval Required"))
	sb.WriteString("\n\n")

	// Render plan as markdown, accounting for box padding
	renderedPlan := renderMarkdown(c.pendingPlan, wrapWidth-OverlayBoxPadding)
	planLines := strings.Split(renderedPlan, "\n")
	maxVisibleLines := PlanApprovalMaxVisible

	// Calculate visible range
	startLine := c.planScrollOffset
	if startLine >= len(planLines) {
		startLine = len(planLines) - 1
		if startLine < 0 {
			startLine = 0
		}
	}
	endLine := startLine + maxVisibleLines
	if endLine > len(planLines) {
		endLine = len(planLines)
	}

	// Show scroll indicators if needed
	if startLine > 0 {
		scrollHint := lipgloss.NewStyle().Foreground(ColorTextMuted).Italic(true)
		sb.WriteString(scrollHint.Render(fmt.Sprintf("  ↑ %d more lines above", startLine)))
		sb.WriteString("\n")
	}

	// Render visible lines (already markdown-rendered)
	for i := startLine; i < endLine; i++ {
		sb.WriteString(planLines[i])
		sb.WriteString("\n")
	}

	if endLine < len(planLines) {
		scrollHint := lipgloss.NewStyle().Foreground(ColorTextMuted).Italic(true)
		sb.WriteString(scrollHint.Render(fmt.Sprintf("  ↓ %d more lines below", len(planLines)-endLine)))
		sb.WriteString("\n")
	}

	// Show allowed prompts if any
	if len(c.pendingAllowedPrompts) > 0 {
		sb.WriteString("\n")
		promptsHeader := lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
		sb.WriteString(promptsHeader.Render("Requested permissions:"))
		sb.WriteString("\n")

		promptStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		for _, prompt := range c.pendingAllowedPrompts {
			sb.WriteString(promptStyle.Render(fmt.Sprintf("  • %s: %s", prompt.Tool, prompt.Prompt)))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")

	// Keyboard hints
	keyStyle := lipgloss.NewStyle().Foreground(ColorInfo).Bold(true)
	hintStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)

	sb.WriteString(keyStyle.Render("[y]"))
	sb.WriteString(hintStyle.Render(" Approve  "))
	sb.WriteString(keyStyle.Render("[n]"))
	sb.WriteString(hintStyle.Render(" Reject  "))
	if len(planLines) > maxVisibleLines {
		sb.WriteString(keyStyle.Render("[↑/↓]"))
		sb.WriteString(hintStyle.Render(" Scroll"))
	}

	// Wrap in a box, capped at max width for readability
	// Plans use a wider max since they often contain code
	boxWidth := wrapWidth
	if boxWidth > PlanBoxMaxWidth {
		boxWidth = PlanBoxMaxWidth
	}
	return PlanApprovalBoxStyle.Width(boxWidth).Render(sb.String())
}

func (c *Chat) updateContent() {
	// Skip rendering if viewport not yet initialized - content will be rendered
	// when SetSize is called with actual dimensions. Rendering with wrong width
	// causes text to be wrapped incorrectly, then re-wrapped by viewport's SoftWrap.
	if c.viewport.Width() <= 0 {
		return
	}

	var sb strings.Builder

	// Get wrap width (use viewport width, fallback to reasonable default)
	// Subtract ContentPadding for the horizontal padding applied via Padding(0, 1)
	wrapWidth := c.viewport.Width() - ContentPadding
	if wrapWidth < MinWrapWidth {
		wrapWidth = DefaultWrapWidth
	}

	if !c.hasSession {
		sb.WriteString(renderNoSessionMessage())
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

		// Show todo list if present
		if c.hasTodoList && c.currentTodoList != nil {
			if len(c.messages) > 0 || c.streaming != "" || c.waiting {
				sb.WriteString("\n\n")
			}
			sb.WriteString(renderTodoList(c.currentTodoList, wrapWidth))
		}

		// Show pending permission prompt
		if c.hasPendingPermission {
			if len(c.messages) > 0 || c.streaming != "" || c.waiting {
				sb.WriteString("\n\n")
			}
			sb.WriteString(renderPermissionPrompt(c.pendingPermissionTool, c.pendingPermissionDesc, wrapWidth))
		}

		// Show pending question prompt
		if c.hasPendingQuestion {
			if len(c.messages) > 0 || c.streaming != "" || c.waiting || c.hasPendingPermission {
				sb.WriteString("\n\n")
			}
			sb.WriteString(c.renderQuestionPrompt(wrapWidth))
		}

		// Show pending plan approval prompt
		if c.hasPendingPlanApproval {
			if len(c.messages) > 0 || c.streaming != "" || c.waiting || c.hasPendingPermission || c.hasPendingQuestion {
				sb.WriteString("\n\n")
			}
			sb.WriteString(c.renderPlanApprovalPrompt(wrapWidth))
		}
	}

	// Add horizontal padding to content for visual breathing room
	paddedContent := lipgloss.NewStyle().Padding(0, 1).Render(sb.String())
	c.viewport.SetContent(paddedContent)
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
				// Navigate to previous file
				if c.viewChangesFileIndex > 0 {
					c.viewChangesFileIndex--
					c.updateViewChangesDiff()
				}
				return c, nil
			case "right", "l":
				// Navigate to next file
				if c.viewChangesFileIndex < len(c.viewChangesFiles)-1 {
					c.viewChangesFileIndex++
					c.updateViewChangesDiff()
				}
				return c, nil
			case "up", "k", "down", "j", "pgup", "pgdown", "ctrl+up", "ctrl+down",
				"home", "end", "page up", "page down", "ctrl+u", "ctrl+d":
				// Scroll diff viewport
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

	// Handle mouse events for text selection
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		if c.hasSession && msg.Button == tea.MouseLeft {
			// Adjust coordinates for panel border
			x := msg.X - 1
			y := msg.Y - 1
			if x >= 0 && y >= 0 {
				cmd := c.handleMouseClick(x, y)
				if cmd != nil {
					return c, cmd
				}
			}
		}
		return c, nil

	case tea.MouseMotionMsg:
		if c.hasSession && c.selectionActive && msg.Button == tea.MouseLeft {
			// Adjust coordinates for panel border
			x := msg.X - 1
			y := msg.Y - 1
			c.EndSelection(x, y)
		}
		return c, nil

	case tea.MouseReleaseMsg:
		// Note: Don't check msg.Button here - release events may not preserve the button that was released
		// We rely on selectionActive which was set when we started selection with left click
		if c.hasSession && c.selectionActive {
			// Adjust coordinates for panel border
			x := msg.X - 1
			y := msg.Y - 1

			// For drag selections, update the end position
			if c.selectionActive {
				c.EndSelection(x, y)
			}

			// Copy if we have a selection (either from drag or double/triple click)
			if c.HasTextSelection() {
				clickCount := c.clickCount

				// Schedule delayed copy to allow for multi-click detection
				tick := tea.Tick(doubleClickThreshold, func(time.Time) tea.Msg {
					return SelectionCopyMsg{
						clickCount:   clickCount,
						endSelection: true,
						x:            x,
						y:            y,
					}
				})
				return c, tick
			}
		}
		return c, nil

	case SelectionCopyMsg:
		if msg.clickCount == c.clickCount && time.Since(c.lastClickTime) >= doubleClickThreshold {
			// If the click count matches and threshold has passed, copy selected text
			c.SelectionStop()
			cmds = append(cmds, c.CopySelectedText())
			return c, tea.Batch(cmds...)
		}
		return c, nil

	case StopwatchTickMsg:
		cmd := c.handleStopwatchTick()
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return c, tea.Batch(cmds...)

	case CompletionFlashTickMsg:
		cmd := c.handleCompletionFlashTick()
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return c, tea.Batch(cmds...)

	case SelectionFlashTickMsg:
		if c.selectionFlashFrame >= 0 {
			c.selectionFlashFrame++
			if c.selectionFlashFrame >= 1 {
				// Flash complete - clear the selection
				c.SelectionClear()
				c.selectionFlashFrame = -1
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
			case "tab":
				// Don't let textarea consume Tab - let it bubble up for focus switching
				return c, tea.Batch(cmds...)
			case "esc":
				// Clear text selection if there is one
				if c.HasTextSelection() {
					c.SelectionClear()
					return c, nil
				}
				// Clear textarea if it has content
				if c.input.Value() != "" {
					c.input.Reset()
					return c, nil
				}
				// Otherwise let it bubble up for other handlers (stop streaming, etc.)
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
		viewportContent = lipgloss.NewStyle().Padding(0, 1).Render(renderNoSessionMessage())
	} else {
		viewportContent = c.viewport.View()
		// Apply selection highlighting if there's an active selection
		if c.HasTextSelection() {
			viewportContent = c.selectionView(viewportContent)
		}
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
		indicatorStyle := lipgloss.NewStyle().
			Foreground(ColorInfo).
			Padding(0, 1)
		indicator := indicatorStyle.Render(fmt.Sprintf("[Image attached: %dKB] (backspace to remove)", c.GetPendingImageSizeKB()))
		inputContent = indicator + "\n" + c.input.View()
	} else {
		inputContent = c.input.View()
	}
	inputArea := inputStyle.Width(c.width).Render(inputContent)

	return lipgloss.JoinVertical(lipgloss.Left, chatPanel, inputArea)
}
