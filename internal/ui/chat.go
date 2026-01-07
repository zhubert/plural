package ui

import (
	"bytes"
	"fmt"
	"image/color"
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
	"github.com/zhubert/plural/internal/logger"
)

// Compiled regex patterns for markdown parsing
var (
	boldPattern       = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicPattern     = regexp.MustCompile(`(?:^|[^*])\*([^*]+)\*(?:[^*]|$)`)
	underscoreItalic  = regexp.MustCompile(`_([^_]+)_`)
	inlineCodePattern = regexp.MustCompile("`([^`]+)`")
	linkPattern       = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
)

// StopwatchTickMsg is sent to update the animated waiting display
type StopwatchTickMsg time.Time

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
	colorOffset int    // Animation offset for flowing color effect

	// Current tool status (shown while Claude is using a tool)
	toolName  string // Name of the tool being used (e.g., "Read", "Edit", "Bash")
	toolInput string // Brief description of tool input (e.g., filename, command)

	// Pending permission prompt
	hasPendingPermission   bool
	pendingPermissionTool  string
	pendingPermissionDesc  string
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

	c := &Chat{
		viewport: vp,
		input:    ti,
		messages: []claude.Message{},
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
	c.updateContent()
}

// ClearSession clears the current session
func (c *Chat) ClearSession() {
	c.sessionName = ""
	c.messages = nil
	c.hasSession = false
	c.streaming = ""
	c.toolName = ""
	c.toolInput = ""
	c.hasPendingPermission = false
	c.pendingPermissionTool = ""
	c.pendingPermissionDesc = ""
	c.updateContent()
}

// AppendStreaming appends content to the current streaming response
func (c *Chat) AppendStreaming(content string) {
	c.streaming += content
	c.updateContent()
}

// AppendToolUse appends a formatted tool use line to the streaming content
func (c *Chat) AppendToolUse(toolName, toolInput string) {
	icon := GetToolIcon(toolName)
	line := "⏺ " + icon + "(" + toolName
	if toolInput != "" {
		line += ": " + toolInput
	}
	line += ")\n"

	// Add newline before if there's existing content that doesn't end with newline
	if c.streaming != "" && !strings.HasSuffix(c.streaming, "\n") {
		c.streaming += "\n"
	}
	c.streaming += line
	c.updateContent()
}

// FinishStreaming completes the streaming and adds to messages
func (c *Chat) FinishStreaming() {
	// Clear tool status
	c.toolName = ""
	c.toolInput = ""
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

// IsStreaming returns whether we're currently streaming a response
func (c *Chat) IsStreaming() bool {
	return c.streaming != ""
}

// GetStreaming returns the current streaming content
func (c *Chat) GetStreaming() string {
	return c.streaming
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

// SetToolStatus sets the current tool being used (shown as status indicator)
func (c *Chat) SetToolStatus(toolName, toolInput string) {
	c.toolName = toolName
	c.toolInput = toolInput
	c.updateContent()
}

// ClearToolStatus clears the current tool status
func (c *Chat) ClearToolStatus() {
	if c.toolName != "" {
		c.toolName = ""
		c.toolInput = ""
		c.updateContent()
	}
}

// HasToolStatus returns whether there's a tool status to display
func (c *Chat) HasToolStatus() bool {
	return c.toolName != ""
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

	// Title
	sb.WriteString(PermissionTitleStyle.Render("⚠ Permission Required"))
	sb.WriteString("\n")

	// Tool name
	sb.WriteString(PermissionToolStyle.Render(c.pendingPermissionTool))
	sb.WriteString("\n")

	// Description (wrapped)
	descStyle := PermissionDescStyle.Width(wrapWidth - 4) // Account for box padding
	sb.WriteString(descStyle.Render(c.pendingPermissionDesc))
	sb.WriteString("\n\n")

	// Keyboard hints
	keyStyle := lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
	hintStyle := PermissionHintStyle

	sb.WriteString(keyStyle.Render("[y]"))
	sb.WriteString(hintStyle.Render(" Allow  "))
	sb.WriteString(keyStyle.Render("[n]"))
	sb.WriteString(hintStyle.Render(" Deny  "))
	sb.WriteString(keyStyle.Render("[a]"))
	sb.WriteString(hintStyle.Render(" Always Allow"))

	// Wrap in a box
	boxWidth := wrapWidth
	if boxWidth > 60 {
		boxWidth = 60 // Cap the width for readability
	}
	return PermissionBoxStyle.Width(boxWidth).Render(sb.String())
}

// renderToolStatus renders the current tool activity indicator
func (c *Chat) renderToolStatus() string {
	if c.toolName == "" {
		return ""
	}

	// Tool icon based on tool type
	icon := GetToolIcon(c.toolName)

	// Style for the tool status
	toolStyle := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Italic(true)

	inputStyle := lipgloss.NewStyle().
		Foreground(ColorTextMuted)

	if c.toolInput != "" {
		return toolStyle.Render(icon+" "+c.toolName) + " " + inputStyle.Render(c.toolInput)
	}
	return toolStyle.Render(icon + " " + c.toolName + "...")
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
		c.colorOffset = 0
	}
	c.updateContent()
}

// SetWaitingWithStart sets the waiting state with a specific start time (for session restoration)
// Note: startTime parameter is kept for API compatibility but no longer used
func (c *Chat) SetWaitingWithStart(waiting bool, startTime time.Time) {
	c.waiting = waiting
	if waiting {
		c.waitingVerb = randomThinkingVerb()
		c.colorOffset = 0
	}
	c.updateContent()
}

// IsWaiting returns whether we're waiting for a response
func (c *Chat) IsWaiting() bool {
	return c.waiting
}

// StopwatchTick returns a command that sends a tick message after a delay
func StopwatchTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return StopwatchTickMsg(t)
	})
}

// renderAnimatedText renders text with a flowing color gradient effect.
// The offset parameter controls which position in the text is at peak brightness.
func renderAnimatedText(text string, offset int) string {
	// Gradient colors from dim to bright and back
	// Using a purple-based gradient that flows left to right
	colors := []color.Color{
		lipgloss.Color("#4C1D95"), // Very dark purple
		lipgloss.Color("#5B21B6"), // Dark purple
		lipgloss.Color("#6D28D9"), // Purple
		lipgloss.Color("#7C3AED"), // Medium purple
		lipgloss.Color("#8B5CF6"), // Light purple
		lipgloss.Color("#A78BFA"), // Lighter purple
		lipgloss.Color("#C4B5FD"), // Very light purple (peak)
		lipgloss.Color("#A78BFA"), // Lighter purple
		lipgloss.Color("#8B5CF6"), // Light purple
		lipgloss.Color("#7C3AED"), // Medium purple
		lipgloss.Color("#6D28D9"), // Purple
		lipgloss.Color("#5B21B6"), // Dark purple
	}

	runes := []rune(text + "...")
	var result strings.Builder
	gradientLen := len(colors)

	for i, r := range runes {
		// Calculate which color to use based on position and offset
		// The offset shifts the "bright" point across the text
		colorIdx := (i + offset) % gradientLen
		style := lipgloss.NewStyle().Foreground(colors[colorIdx]).Italic(true)
		result.WriteString(style.Render(string(r)))
	}

	return result.String()
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

// renderInlineMarkdown applies inline formatting (bold, italic, code, links) to a line
func renderInlineMarkdown(line string) string {
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
	line = underscoreItalic.ReplaceAllStringFunc(line, func(match string) string {
		text := underscoreItalic.FindStringSubmatch(match)[1]
		return MarkdownItalicStyle.Render(text)
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
			// Render markdown content
			sb.WriteString(renderMarkdown(strings.TrimSpace(msg.Content), wrapWidth))
		}

		// Show streaming content or waiting indicator with stopwatch
		if c.streaming != "" {
			if len(c.messages) > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(ChatAssistantStyle.Render("Claude:"))
			sb.WriteString("\n")
			// Render markdown for streaming content
			sb.WriteString(renderMarkdown(strings.TrimSpace(c.streaming), wrapWidth))
			// Show tool status after streaming content if a tool is active
			if c.toolName != "" {
				sb.WriteString("\n")
				sb.WriteString(c.renderToolStatus())
			}
		} else if c.toolName != "" {
			// Tool is active but no text content yet
			if len(c.messages) > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(ChatAssistantStyle.Render("Claude:"))
			sb.WriteString("\n")
			sb.WriteString(c.renderToolStatus())
		} else if c.waiting {
			if len(c.messages) > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(ChatAssistantStyle.Render("Claude:"))
			sb.WriteString("\n")
			sb.WriteString(renderAnimatedText(c.waitingVerb, c.colorOffset))
		}

		// Show pending permission prompt
		if c.hasPendingPermission {
			if len(c.messages) > 0 || c.streaming != "" || c.waiting {
				sb.WriteString("\n\n")
			}
			sb.WriteString(c.renderPermissionPrompt(wrapWidth))
		}
	}

	c.viewport.SetContent(sb.String())
	c.viewport.GotoBottom()
}

// Update handles messages
func (c *Chat) Update(msg tea.Msg) (*Chat, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg.(type) {
	case StopwatchTickMsg:
		if c.waiting {
			c.colorOffset++ // Advance the color animation
			c.updateContent()
			cmds = append(cmds, StopwatchTick())
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
	inputArea := inputStyle.Width(c.width).Render(c.input.View())

	return lipgloss.JoinVertical(lipgloss.Left, chatPanel, inputArea)
}
