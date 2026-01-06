package ui

import (
	"bytes"
	"fmt"
	"math/rand"
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
	"github.com/zhubert/plural/internal/claude"
	"github.com/zhubert/plural/internal/logger"
)

// StopwatchTickMsg is sent to update the stopwatch display
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
	viewport      viewport.Model
	input         textarea.Model
	width         int
	height        int
	focused       bool
	messages      []claude.Message
	streaming     string    // Current streaming response
	sessionName   string
	hasSession    bool
	waiting       bool      // Waiting for Claude's response
	waitStartTime time.Time // When waiting started (for stopwatch)
	waitingVerb   string    // Random verb to display while waiting

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

// SetWaiting sets the waiting state (before streaming starts)
func (c *Chat) SetWaiting(waiting bool) {
	c.waiting = waiting
	if waiting {
		c.waitStartTime = time.Now()
		c.waitingVerb = randomThinkingVerb()
	}
	c.updateContent()
}

// SetWaitingWithStart sets the waiting state with a specific start time (for session restoration)
func (c *Chat) SetWaitingWithStart(waiting bool, startTime time.Time) {
	c.waiting = waiting
	c.waitStartTime = startTime
	if waiting {
		c.waitingVerb = randomThinkingVerb()
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

// formatElapsed formats a duration as a stopwatch string (e.g., "1.2s", "1:23")
func formatElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	mins := int(d.Minutes())
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", mins, secs)
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
				result.WriteString(highlighted)
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
			result.WriteString(line)
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
		} else if c.waiting {
			if len(c.messages) > 0 {
				sb.WriteString("\n\n")
			}
			elapsed := time.Since(c.waitStartTime)
			stopwatchStyle := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
			sb.WriteString(ChatAssistantStyle.Render("Claude:"))
			sb.WriteString("\n")
			sb.WriteString(StatusLoadingStyle.Render(c.waitingVerb + "... "))
			sb.WriteString(stopwatchStyle.Render(formatElapsed(elapsed)))
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
