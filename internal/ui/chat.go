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
	"github.com/zhubert/plural/internal/logger"
	"github.com/zhubert/plural/internal/mcp"
)

// ToolUseInProgress is the empty circle marker for tool use in progress
const ToolUseInProgress = "○"

// ToolUseComplete is the green circle marker for completed tool use
const ToolUseComplete = "●"

// ToolUseItem represents a single tool use for rollup tracking
type ToolUseItem struct {
	ToolName  string // e.g., "Read", "Edit", "Bash"
	ToolInput string // Brief description of tool parameters
	ToolUseID string // Unique ID for matching tool_use to tool_result
	Complete  bool   // Whether the tool has completed
}

// ToolUseRollup tracks consecutive tool uses for collapsible display
type ToolUseRollup struct {
	Items    []ToolUseItem // All tool uses in this group
	Expanded bool          // Whether the rollup is expanded (show all) or collapsed (show summary)
}

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
	waiting     bool // Waiting for Claude's response

	// Spinner and completion animation state
	spinner *SpinnerState

	// Message rendering cache - avoids re-rendering unchanged messages
	messageCache []messageCache // Cache of rendered messages, indexed by message position

	// Track last tool use position for marking as complete
	lastToolUsePos int // Position in streaming content where last tool use marker starts

	// Tool use rollup - tracks consecutive tool uses for collapsible display
	toolUseRollup *ToolUseRollup // Current rollup group (nil when no tool uses yet)

	// Pending prompts (nil when not active)
	permission   *PendingPermission   // Permission prompt state
	question     *PendingQuestion     // Question prompt state
	planApproval *PendingPlanApproval // Plan approval state

	// View changes mode - temporary overlay showing git diff (nil when not active)
	viewChanges *ViewChangesState

	// Pending image attachment (nil when no image attached)
	pendingImage *PendingImage

	// Queued message waiting to be sent after streaming completes
	queuedMessage string

	// Todo list display state
	currentTodoList *pclaude.TodoList
	todoWidth       int            // Width of todo sidebar when visible (0 when hidden)
	todoViewport    viewport.Model // Viewport for scrollable todo list

	// Text selection state
	selection *TextSelection

	// Streaming statistics display
	streamStartTime time.Time            // When waiting/streaming started
	streamStats     *pclaude.StreamStats // Latest stats from Claude (nil until result received)
	finalStats      *pclaude.StreamStats // Final stats from last completed response (persists for display)
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

	// Create viewport for todo sidebar (scrollable task list)
	todoVp := viewport.New()
	todoVp.MouseWheelEnabled = true
	todoVp.MouseWheelDelta = 3
	todoVp.SoftWrap = false

	c := &Chat{
		viewport:       vp,
		todoViewport:   todoVp,
		input:          ti,
		messages:       []pclaude.Message{},
		lastToolUsePos: -1,
		spinner:        NewSpinnerState(),
		selection:      NewTextSelection(),
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
	ctx := GetViewContext()

	c.width = width
	c.height = height

	// Calculate todo sidebar width if we have a todo list
	var mainPanelWidth int
	if c.HasTodoList() {
		// Todo sidebar gets 1/4 of the total chat panel width
		c.todoWidth = width / TodoSidebarWidthRatio
		if c.todoWidth < TodoListMinWrapWidth+BorderSize {
			c.todoWidth = TodoListMinWrapWidth + BorderSize
		}
		mainPanelWidth = width - c.todoWidth

		// Chat panel height (excluding input area which is separate)
		chatPanelHeight := height - InputTotalHeight
		// Set todo viewport dimensions (accounting for border)
		todoInnerWidth := c.todoWidth - BorderSize
		todoInnerHeight := ctx.InnerHeight(chatPanelHeight)
		if todoInnerWidth < TodoListMinWrapWidth {
			todoInnerWidth = TodoListMinWrapWidth
		}
		if todoInnerHeight < 1 {
			todoInnerHeight = 1
		}
		c.todoViewport.SetWidth(todoInnerWidth)
		c.todoViewport.SetHeight(todoInnerHeight)
		// Update todo viewport content with new dimensions
		c.updateTodoViewportContent()
	} else {
		c.todoWidth = 0
		mainPanelWidth = width
	}

	// Check if viewport width changed - if so, invalidate message cache
	// since messages are wrapped based on viewport width
	newInnerWidth := ctx.InnerWidth(mainPanelWidth)
	wasUninitialized := c.viewport.Width() <= 0
	widthChanged := c.viewport.Width() != newInnerWidth && c.viewport.Width() > 0
	if widthChanged {
		c.messageCache = nil // Clear cache to force re-render at new width
	}

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

	// Input width accounts for its own border AND padding (spans full width below both panels)
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
		"mainPanelWidth", mainPanelWidth,
		"todoWidth", c.todoWidth,
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
	c.toolUseRollup = nil // Clear rollup from any previous session
	c.messageCache = nil  // Clear cache on session change
	c.updateContent()
}

// ClearSession clears the current session
func (c *Chat) ClearSession() {
	c.sessionName = ""
	c.messages = nil
	c.hasSession = false
	c.streaming = ""
	c.lastToolUsePos = -1
	c.toolUseRollup = nil // Clear tool use rollup
	c.messageCache = nil  // Clear cache on session clear
	c.permission = nil
	c.question = nil
	c.waiting = false
	c.spinner.FlashFrame = -1
	c.queuedMessage = ""
	c.currentTodoList = nil
	c.updateContent()
}

// AppendStreaming appends content to the current streaming response
func (c *Chat) AppendStreaming(content string) {
	// When text content arrives, flush any pending tool uses to streaming first
	// (flushToolUseRollup adds a trailing newline for visual separation)
	c.flushToolUseRollup()

	c.streaming += content
	c.updateContent()
}

// AppendToolUse adds a tool use to the current rollup group
func (c *Chat) AppendToolUse(toolName, toolInput, toolUseID string) {
	// Initialize rollup if needed
	if c.toolUseRollup == nil {
		c.toolUseRollup = &ToolUseRollup{
			Items:    []ToolUseItem{},
			Expanded: false,
		}
	}

	// Add the new tool use to the rollup
	c.toolUseRollup.Items = append(c.toolUseRollup.Items, ToolUseItem{
		ToolName:  toolName,
		ToolInput: toolInput,
		ToolUseID: toolUseID,
		Complete:  false,
	})

	c.updateContent()
}

// MarkToolUseComplete marks the tool use with the given ID as complete.
// If the ID is empty or not found, falls back to marking the last incomplete tool use.
func (c *Chat) MarkToolUseComplete(toolUseID string) {
	if c.toolUseRollup == nil || len(c.toolUseRollup.Items) == 0 {
		return
	}

	// If we have a tool use ID, find and mark the matching item
	if toolUseID != "" {
		for i := range c.toolUseRollup.Items {
			if c.toolUseRollup.Items[i].ToolUseID == toolUseID {
				c.toolUseRollup.Items[i].Complete = true
				c.updateContent()
				return
			}
		}
	}

	// Fallback: mark the first incomplete tool use as complete
	// This handles cases where we don't have a matching ID
	for i := range c.toolUseRollup.Items {
		if !c.toolUseRollup.Items[i].Complete {
			c.toolUseRollup.Items[i].Complete = true
			c.updateContent()
			return
		}
	}
}

// flushToolUseRollup writes the current rollup to streaming content and clears it
func (c *Chat) flushToolUseRollup() {
	if c.toolUseRollup == nil || len(c.toolUseRollup.Items) == 0 {
		return
	}

	// Add newline before if there's existing content that doesn't end with newline
	if c.streaming != "" && !strings.HasSuffix(c.streaming, "\n") {
		c.streaming += "\n"
	}

	// Render all tool uses in the rollup to streaming content
	for _, item := range c.toolUseRollup.Items {
		marker := ToolUseInProgress
		if item.Complete {
			marker = ToolUseComplete
		}
		icon := GetToolIcon(item.ToolName)
		line := marker + " " + icon + "(" + item.ToolName
		if item.ToolInput != "" {
			line += ": " + item.ToolInput
		}
		line += ")\n"
		c.streaming += line
	}

	// Add extra newline after tool uses for visual separation from following text
	// This is called from AppendStreaming, so there will be text content after
	c.streaming += "\n"

	// Clear the rollup - tool uses are now in streaming content
	c.toolUseRollup = nil
	c.lastToolUsePos = -1
}

// FinishStreaming completes the streaming and adds to messages
func (c *Chat) FinishStreaming() {
	// Flush any remaining tool uses before finishing
	c.flushToolUseRollup()

	if c.streaming != "" {
		c.messages = append(c.messages, pclaude.Message{
			Role:    "assistant",
			Content: c.streaming,
		})
		c.streaming = ""
		c.lastToolUsePos = -1 // Reset tool tracking to prevent stale state affecting future streaming
		c.toolUseRollup = nil // Ensure rollup is cleared
		// Preserve final stats for display after streaming ends
		if c.streamStats != nil {
			c.finalStats = c.streamStats
		}
		c.updateContent()
	}
}

// ToggleToolUseRollup toggles between expanded and collapsed view of tool uses
func (c *Chat) ToggleToolUseRollup() {
	if c.toolUseRollup != nil {
		c.toolUseRollup.Expanded = !c.toolUseRollup.Expanded
		c.updateContent()
	}
}

// HasActiveToolUseRollup returns true if there's an active rollup with multiple items
func (c *Chat) HasActiveToolUseRollup() bool {
	return c.toolUseRollup != nil && len(c.toolUseRollup.Items) > 1
}

// GetToolUseRollup returns the current tool use rollup (for rendering)
func (c *Chat) GetToolUseRollup() *ToolUseRollup {
	return c.toolUseRollup
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
	c.permission = &PendingPermission{
		Tool:        tool,
		Description: description,
	}
	c.updateContent()
}

// ClearPendingPermission clears the pending permission prompt
func (c *Chat) ClearPendingPermission() {
	c.permission = nil
	c.updateContent()
}

// HasPendingPermission returns whether there's a pending permission prompt
func (c *Chat) HasPendingPermission() bool {
	return c.permission != nil
}

// SetPendingQuestion sets the pending question prompt to display
func (c *Chat) SetPendingQuestion(questions []mcp.Question) {
	c.question = NewPendingQuestion(questions)
	c.updateContent()
}

// ClearPendingQuestion clears the pending question prompt
func (c *Chat) ClearPendingQuestion() {
	c.question = nil
	c.updateContent()
}

// HasPendingQuestion returns whether there's a pending question prompt
func (c *Chat) HasPendingQuestion() bool {
	return c.question != nil
}

// GetQuestionAnswers returns the collected question answers
func (c *Chat) GetQuestionAnswers() map[string]string {
	if c.question == nil {
		return nil
	}
	return c.question.Answers
}

// SetPendingPlanApproval sets the pending plan approval to display
func (c *Chat) SetPendingPlanApproval(plan string, allowedPrompts []mcp.AllowedPrompt) {
	c.planApproval = &PendingPlanApproval{
		Plan:           plan,
		AllowedPrompts: allowedPrompts,
		ScrollOffset:   0,
	}
	c.updateContent()
}

// ClearPendingPlanApproval clears the pending plan approval prompt
func (c *Chat) ClearPendingPlanApproval() {
	c.planApproval = nil
	c.updateContent()
}

// HasPendingPlanApproval returns whether there's a pending plan approval prompt
func (c *Chat) HasPendingPlanApproval() bool {
	return c.planApproval != nil
}

// ScrollPlan scrolls the plan view by the given delta
func (c *Chat) ScrollPlan(delta int) {
	if c.planApproval == nil {
		return
	}
	c.planApproval.ScrollOffset += delta
	if c.planApproval.ScrollOffset < 0 {
		c.planApproval.ScrollOffset = 0
	}
	c.updateContent()
}

// MoveQuestionSelection moves the selection up or down
func (c *Chat) MoveQuestionSelection(delta int) {
	if c.question == nil || c.question.CurrentIdx >= len(c.question.Questions) {
		return
	}
	q := c.question.Questions[c.question.CurrentIdx]
	numOptions := len(q.Options) + 1 // +1 for "Other" option
	c.question.SelectedOption = (c.question.SelectedOption + delta + numOptions) % numOptions
	c.updateContent()
}

// SelectCurrentOption selects the current option and moves to next question or completes
// Returns true if all questions are answered
func (c *Chat) SelectCurrentOption() bool {
	if c.question == nil || c.question.CurrentIdx >= len(c.question.Questions) {
		return true
	}
	q := c.question.Questions[c.question.CurrentIdx]

	// Determine the selected answer
	var answer string
	if c.question.SelectedOption < len(q.Options) {
		answer = q.Options[c.question.SelectedOption].Label
	} else {
		// "Other" selected - for now, just use empty string
		// A full implementation would allow text input
		answer = ""
	}

	c.question.Answers[q.Question] = answer

	// Move to next question or complete
	c.question.CurrentIdx++
	c.question.SelectedOption = 0

	if c.question.CurrentIdx >= len(c.question.Questions) {
		// All questions answered
		return true
	}

	c.updateContent()
	return false
}

// SelectOptionByNumber selects an option by its number (1-based)
// Returns true if all questions are answered after this selection
func (c *Chat) SelectOptionByNumber(num int) bool {
	if c.question == nil || c.question.CurrentIdx >= len(c.question.Questions) {
		return true
	}
	q := c.question.Questions[c.question.CurrentIdx]
	numOptions := len(q.Options) + 1 // +1 for "Other"

	// Convert 1-based to 0-based index
	idx := num - 1
	if idx < 0 || idx >= numOptions {
		return false
	}

	c.question.SelectedOption = idx
	return c.SelectCurrentOption()
}

// AttachImage attaches an image to the pending message
func (c *Chat) AttachImage(data []byte, mediaType string) {
	c.pendingImage = &PendingImage{
		Data:      data,
		MediaType: mediaType,
	}
	c.updateContent()
}

// ClearImage removes the pending image attachment
func (c *Chat) ClearImage() {
	c.pendingImage = nil
	c.updateContent()
}

// HasPendingImage returns whether there's a pending image attachment
func (c *Chat) HasPendingImage() bool {
	return c.pendingImage != nil && len(c.pendingImage.Data) > 0
}

// GetPendingImage returns the pending image data and clears it
func (c *Chat) GetPendingImage() (data []byte, mediaType string) {
	if c.pendingImage == nil {
		return nil, ""
	}
	data = c.pendingImage.Data
	mediaType = c.pendingImage.MediaType
	c.pendingImage = nil
	return data, mediaType
}

// GetPendingImageSizeKB returns the pending image size in KB
func (c *Chat) GetPendingImageSizeKB() int {
	if c.pendingImage == nil {
		return 0
	}
	return c.pendingImage.SizeKB()
}

// SetTodoList sets the current todo list to display
// If the list is complete (all items done), it gets "baked" into the message
// history so it scrolls like normal messages instead of staying pinned at bottom
func (c *Chat) SetTodoList(list *pclaude.TodoList) {
	hadTodoList := c.HasTodoList()

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
		c.currentTodoList = nil
	} else if list != nil && len(list.Items) > 0 {
		c.currentTodoList = list
	} else {
		c.currentTodoList = nil
	}

	// If todo list visibility changed, recalculate layout
	hasTodoList := c.HasTodoList()
	if hadTodoList != hasTodoList && c.width > 0 && c.height > 0 {
		c.SetSize(c.width, c.height)
	}

	// Update todo viewport content
	c.updateTodoViewportContent()

	c.updateContent()
}

// ClearTodoList clears the todo list display
func (c *Chat) ClearTodoList() {
	hadTodoList := c.HasTodoList()
	c.currentTodoList = nil

	// If we had a todo list, recalculate layout to reclaim the sidebar space
	if hadTodoList && c.width > 0 && c.height > 0 {
		c.SetSize(c.width, c.height)
	}

	// Clear todo viewport content
	c.updateTodoViewportContent()

	c.updateContent()
}

// HasTodoList returns whether there's a todo list to display
func (c *Chat) HasTodoList() bool {
	return c.currentTodoList != nil && len(c.currentTodoList.Items) > 0
}

// GetTodoList returns the current todo list
func (c *Chat) GetTodoList() *pclaude.TodoList {
	return c.currentTodoList
}

// updateTodoViewportContent updates the todo viewport's content.
// Call this when the todo list changes or when the viewport is resized.
func (c *Chat) updateTodoViewportContent() {
	if c.currentTodoList == nil || len(c.currentTodoList.Items) == 0 {
		c.todoViewport.SetContent("")
		return
	}

	// Get inner width for content wrapping
	width := c.todoViewport.Width()
	if width < TodoListMinWrapWidth {
		width = TodoListMinWrapWidth
	}

	// Use renderTodoListForSidebar which renders without the box border
	// since the sidebar panel already has borders
	content := renderTodoListForSidebar(c.currentTodoList, width)
	c.todoViewport.SetContent(content)
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

// renderToolUseRollup renders the tool use rollup as either expanded or collapsed
func (c *Chat) renderToolUseRollup() string {
	if c.toolUseRollup == nil || len(c.toolUseRollup.Items) == 0 {
		return ""
	}

	var sb strings.Builder

	// Always show the most recent (last) tool use
	lastItem := c.toolUseRollup.Items[len(c.toolUseRollup.Items)-1]
	marker := ToolUseInProgress
	if lastItem.Complete {
		marker = ToolUseComplete
	}
	icon := GetToolIcon(lastItem.ToolName)
	line := marker + " " + icon + "(" + lastItem.ToolName
	if lastItem.ToolInput != "" {
		line += ": " + lastItem.ToolInput
	}
	line += ")"

	// Apply styling to tool use markers in the line
	line = strings.ReplaceAll(line, ToolUseInProgress, ToolUseInProgressStyle.Render(ToolUseInProgress))
	line = strings.ReplaceAll(line, ToolUseComplete, ToolUseCompleteStyle.Render(ToolUseComplete))

	sb.WriteString(line)
	sb.WriteString("\n")

	// If there are multiple items and not expanded, show the rollup summary
	if len(c.toolUseRollup.Items) > 1 {
		if c.toolUseRollup.Expanded {
			// Show all previous tool uses (oldest first, excluding the last one already shown)
			for i := 0; i < len(c.toolUseRollup.Items)-1; i++ {
				item := c.toolUseRollup.Items[i]
				itemMarker := ToolUseInProgress
				if item.Complete {
					itemMarker = ToolUseComplete
				}
				itemIcon := GetToolIcon(item.ToolName)
				itemLine := "  " + itemMarker + " " + itemIcon + "(" + item.ToolName
				if item.ToolInput != "" {
					itemLine += ": " + item.ToolInput
				}
				itemLine += ")"
				// Apply styling
				itemLine = strings.ReplaceAll(itemLine, ToolUseInProgress, ToolUseInProgressStyle.Render(ToolUseInProgress))
				itemLine = strings.ReplaceAll(itemLine, ToolUseComplete, ToolUseCompleteStyle.Render(ToolUseComplete))
				sb.WriteString(itemLine)
				sb.WriteString("\n")
			}
		} else {
			// Show collapsed summary
			moreCount := len(c.toolUseRollup.Items) - 1
			rollupStyle := lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Italic(true)
			keyStyle := lipgloss.NewStyle().
				Foreground(ColorInfo)
			summaryText := fmt.Sprintf("  +%d more tool use", moreCount)
			if moreCount > 1 {
				summaryText += "s"
			}
			summaryText += " ("
			sb.WriteString(rollupStyle.Render(summaryText))
			sb.WriteString(keyStyle.Render("ctrl-t"))
			sb.WriteString(rollupStyle.Render(" to expand)"))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// renderQuestionPrompt renders the inline question prompt
func (c *Chat) renderQuestionPrompt(wrapWidth int) string {
	if c.question == nil || c.question.CurrentIdx >= len(c.question.Questions) {
		return ""
	}

	q := c.question.Questions[c.question.CurrentIdx]
	var sb strings.Builder

	// Question progress indicator (if multiple questions)
	if len(c.question.Questions) > 1 {
		progressStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		sb.WriteString(progressStyle.Render(fmt.Sprintf("Question %d of %d", c.question.CurrentIdx+1, len(c.question.Questions))))
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
		isSelected := i == c.question.SelectedOption

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
	isOtherSelected := c.question.SelectedOption == otherIdx
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
	if c.planApproval == nil {
		return ""
	}

	var sb strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(ColorInfo).Bold(true)
	sb.WriteString(titleStyle.Render("Plan Approval Required"))
	sb.WriteString("\n\n")

	// Render plan as markdown, accounting for box padding
	renderedPlan := renderMarkdown(c.planApproval.Plan, wrapWidth-OverlayBoxPadding)
	planLines := strings.Split(renderedPlan, "\n")
	maxVisibleLines := PlanApprovalMaxVisible

	// Calculate visible range
	startLine := c.planApproval.ScrollOffset
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
	if len(c.planApproval.AllowedPrompts) > 0 {
		sb.WriteString("\n")
		promptsHeader := lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
		sb.WriteString(promptsHeader.Render("Requested permissions:"))
		sb.WriteString("\n")

		promptStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
		for _, prompt := range c.planApproval.AllowedPrompts {
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
		if c.streaming != "" || c.toolUseRollup != nil {
			if len(c.messages) > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(ChatAssistantStyle.Render("Claude:"))
			sb.WriteString("\n")
			// Render markdown for streaming content, stripping <options> tags
			// Tool use lines are already included in streaming content with circle markers
			if c.streaming != "" {
				streamContent := stripOptionsTags(strings.TrimSpace(c.streaming))
				sb.WriteString(renderMarkdown(streamContent, wrapWidth))
			}
			// Render active tool use rollup
			if c.toolUseRollup != nil && len(c.toolUseRollup.Items) > 0 {
				// Add newline separator if there's streaming content before the rollup
				if c.streaming != "" {
					sb.WriteString("\n")
				}
				sb.WriteString(c.renderToolUseRollup())
			}
			// Add status line below streaming content
			sb.WriteString("\n")
			var elapsed time.Duration
			if !c.streamStartTime.IsZero() {
				elapsed = time.Since(c.streamStartTime)
			}
			sb.WriteString(renderStreamingStatus(c.spinner.Verb, c.spinner.Idx, elapsed, c.streamStats))
		} else if c.waiting {
			if len(c.messages) > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(ChatAssistantStyle.Render("Claude:"))
			sb.WriteString("\n")
			var elapsed time.Duration
			if !c.streamStartTime.IsZero() {
				elapsed = time.Since(c.streamStartTime)
			}
			sb.WriteString(renderStreamingStatus(c.spinner.Verb, c.spinner.Idx, elapsed, c.streamStats))
		} else if c.spinner.FlashFrame >= 0 {
			// Show completion flash animation with final stats
			if len(c.messages) > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(ChatAssistantStyle.Render("Claude:"))
			sb.WriteString("\n")
			sb.WriteString(renderCompletionFlash(c.spinner.FlashFrame, c.finalStats))
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

		// Note: Todo list is now rendered as a sidebar in View(), not inline here

		// Show pending permission prompt
		if c.permission != nil {
			if len(c.messages) > 0 || c.streaming != "" || c.waiting {
				sb.WriteString("\n\n")
			}
			sb.WriteString(renderPermissionPrompt(c.permission.Tool, c.permission.Description, wrapWidth))
		}

		// Show pending question prompt
		if c.question != nil {
			if len(c.messages) > 0 || c.streaming != "" || c.waiting || c.permission != nil {
				sb.WriteString("\n\n")
			}
			sb.WriteString(c.renderQuestionPrompt(wrapWidth))
		}

		// Show pending plan approval prompt
		if c.planApproval != nil {
			if len(c.messages) > 0 || c.streaming != "" || c.waiting || c.permission != nil || c.question != nil {
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
	if c.viewChanges != nil {
		if keyMsg, isKey := msg.(tea.KeyPressMsg); isKey {
			key := keyMsg.String()
			switch key {
			case "esc", "q":
				// Exit view changes mode
				c.ExitViewChangesMode()
				return c, nil
			case "left", "h":
				// Navigate to previous file
				if c.viewChanges.FileIndex > 0 {
					c.viewChanges.FileIndex--
					c.updateViewChangesDiff()
				}
				return c, nil
			case "right", "l":
				// Navigate to next file
				if c.viewChanges.FileIndex < len(c.viewChanges.Files)-1 {
					c.viewChanges.FileIndex++
					c.updateViewChangesDiff()
				}
				return c, nil
			case "up", "k", "down", "j", "pgup", "pgdown", "ctrl+up", "ctrl+down",
				"home", "end", "page up", "page down", "ctrl+u", "ctrl+d":
				// Scroll diff viewport
				var cmd tea.Cmd
				c.viewChanges.Viewport, cmd = c.viewChanges.Viewport.Update(msg)
				cmds = append(cmds, cmd)
				return c, tea.Batch(cmds...)
			}
			// Ignore other keys in view changes mode
			return c, nil
		}
		// Pass non-key events (like mouse wheel) to viewport
		var cmd tea.Cmd
		c.viewChanges.Viewport, cmd = c.viewChanges.Viewport.Update(msg)
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
		if c.hasSession && c.selection.Active && msg.Button == tea.MouseLeft {
			// Adjust coordinates for panel border
			x := msg.X - 1
			y := msg.Y - 1
			c.EndSelection(x, y)
		}
		return c, nil

	case tea.MouseReleaseMsg:
		// Note: Don't check msg.Button here - release events may not preserve the button that was released
		// We rely on selection.Active which was set when we started selection with left click
		if c.hasSession && c.selection.Active {
			// Adjust coordinates for panel border
			x := msg.X - 1
			y := msg.Y - 1

			// For drag selections, update the end position
			if c.selection.Active {
				c.EndSelection(x, y)
			}

			// Copy if we have a selection (either from drag or double/triple click)
			if c.HasTextSelection() {
				clickCount := c.selection.ClickCount

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
		if msg.clickCount == c.selection.ClickCount && time.Since(c.selection.LastClickTime) >= doubleClickThreshold {
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
		if c.selection.FlashFrame >= 0 {
			c.selection.FlashFrame++
			if c.selection.FlashFrame >= 1 {
				// Flash complete - clear the selection
				c.SelectionClear()
				c.selection.FlashFrame = -1
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
	// Route mouse wheel events to the appropriate viewport based on X coordinate
	if mouseMsg, isMouse := msg.(tea.MouseWheelMsg); isMouse && c.HasTodoList() && c.todoWidth > 0 {
		// Calculate the boundary between chat and todo sidebar
		mainWidth := c.width - c.todoWidth
		if mouseMsg.X >= mainWidth {
			// Mouse is over the todo sidebar - route to todo viewport
			var cmd tea.Cmd
			c.todoViewport, cmd = c.todoViewport.Update(msg)
			cmds = append(cmds, cmd)
			return c, tea.Batch(cmds...)
		}
	}

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
	if c.viewChanges != nil {
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

	// Check if we need to show todo sidebar
	if c.HasTodoList() && c.todoWidth > 0 {
		// Split layout: chat viewport on left, todo sidebar on right
		mainWidth := c.width - c.todoWidth

		// Render main chat viewport (left side)
		mainPanel := panelStyle.Width(mainWidth).Height(chatPanelHeight).Render(viewportContent)

		// Render todo sidebar (right side) - use scrollable viewport
		todoContent := c.todoViewport.View()
		todoPanel := TodoSidebarStyle.Width(c.todoWidth).Height(chatPanelHeight).Render(todoContent)

		// Join horizontally
		chatPanel := lipgloss.JoinHorizontal(lipgloss.Top, mainPanel, todoPanel)

		// Input spans full width below both panels
		inputArea := inputStyle.Width(c.width).Render(inputContent)

		return lipgloss.JoinVertical(lipgloss.Left, chatPanel, inputArea)
	}

	// No todo list: full-width chat (original behavior)
	chatPanel := panelStyle.Width(c.width).Height(chatPanelHeight).Render(viewportContent)
	inputArea := inputStyle.Width(c.width).Render(inputContent)

	return lipgloss.JoinVertical(lipgloss.Left, chatPanel, inputArea)
}
