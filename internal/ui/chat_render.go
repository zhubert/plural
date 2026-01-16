package ui

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/muesli/reflow/wordwrap"
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

// renderNoSessionMessage renders the placeholder message when no session is selected
func renderNoSessionMessage() string {
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
	sb.WriteString(keyStyle.Render("a"))
	sb.WriteString(msgStyle.Render(" to add a repository first"))
	return sb.String()
}

// renderPermissionPrompt renders the inline permission prompt
func renderPermissionPrompt(tool, description string, wrapWidth int) string {
	var sb strings.Builder

	// Title with tool name on same line: "⚠ Permission Required: Edit"
	sb.WriteString(PermissionTitleStyle.Render("⚠ Permission Required: "))
	sb.WriteString(PermissionToolStyle.Render(tool))
	sb.WriteString("\n")

	// Description (wrapped)
	descStyle := PermissionDescStyle.Width(wrapWidth - 4) // Account for box padding
	sb.WriteString(descStyle.Render(description))
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
