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
	pclaude "github.com/zhubert/plural/internal/claude"
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
	// Table separator pattern matches lines like |---|---|---| or |:---|:---:|---:|
	tableSeparatorPattern = regexp.MustCompile(`^\s*\|[\s\-:]+\|[\s\-:|]*$`)
)

// highlightCode applies syntax highlighting to code using chroma.
// The syntax style is determined by the current theme's SyntaxStyle field.
func highlightCode(code, language string) string {
	lexer := lexers.Get(language)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	// Use the current theme's syntax style
	syntaxStyle := CurrentTheme().GetSyntaxStyle()
	style := styles.Get(syntaxStyle)
	if style == nil {
		style = styles.Fallback
	}

	formatter := formatters.Get(DefaultTerminalFormatter)
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
	// Use lipgloss.Wrap which properly preserves ANSI escape codes
	// The third parameter specifies breakpoint characters (space for word boundaries)
	return lipgloss.Wrap(text, width, " ")
}

// isTableRow checks if a line looks like a markdown table row
func isTableRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	// Must start and end with |, and have at least 3 pipes (meaning at least 1 cell with content)
	// |a|b| has 3 pipes, || only has 2
	return strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") && strings.Count(trimmed, "|") >= 3
}

// isTableSeparator checks if a line is a table separator (e.g., |---|---|)
func isTableSeparator(line string) bool {
	return tableSeparatorPattern.MatchString(line)
}

// parseTableRow parses a table row into cells
func parseTableRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	// Remove leading and trailing pipes
	if strings.HasPrefix(trimmed, "|") {
		trimmed = trimmed[1:]
	}
	if strings.HasSuffix(trimmed, "|") {
		trimmed = trimmed[:len(trimmed)-1]
	}
	// Split by pipe and trim each cell
	parts := strings.Split(trimmed, "|")
	cells := make([]string, len(parts))
	for i, part := range parts {
		cells[i] = strings.TrimSpace(part)
	}
	return cells
}

// renderTable renders a complete markdown table with width constraints
func renderTable(rows [][]string, hasHeader bool, width int) string {
	if len(rows) == 0 {
		return ""
	}

	// Calculate number of columns
	numCols := 0
	for _, row := range rows {
		if len(row) > numCols {
			numCols = len(row)
		}
	}

	if numCols == 0 {
		return ""
	}

	// Calculate natural column widths (based on content)
	naturalWidths := make([]int, numCols)
	for _, row := range rows {
		for i, cell := range row {
			cellWidth := lipgloss.Width(cell)
			if cellWidth > naturalWidths[i] {
				naturalWidths[i] = cellWidth
			}
		}
	}

	// Ensure minimum column width
	for i := range naturalWidths {
		if naturalWidths[i] < TableMinColumnWidth {
			naturalWidths[i] = TableMinColumnWidth
		}
	}

	// Calculate available width for content
	// Each column has: padding (2) + content + border (1) = 3 chars overhead per column
	// Plus 1 for the leftmost border
	// Formula: 1 + numCols * (TableCellPadding + TableBorderWidth) + sum(colWidths) = total
	borderOverhead := TableBorderWidth + numCols*(TableCellPadding+TableBorderWidth)
	availableWidth := width - borderOverhead
	if availableWidth < numCols*TableMinColumnWidth {
		availableWidth = numCols * TableMinColumnWidth
	}

	// Calculate final column widths
	colWidths := calculateTableColumnWidths(naturalWidths, availableWidth)

	// Wrap cell content to fit column widths and convert to multi-line cells
	wrappedRows := make([][][]string, len(rows))
	for rowIdx, row := range rows {
		wrappedCells := make([][]string, numCols)
		maxLines := 1
		for i := 0; i < numCols; i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			// Wrap the cell content to fit the column width
			wrapped := wrapText(cell, colWidths[i])
			lines := strings.Split(wrapped, "\n")
			wrappedCells[i] = lines
			if len(lines) > maxLines {
				maxLines = len(lines)
			}
		}
		// Pad all cells to have the same number of lines
		for i := range wrappedCells {
			for len(wrappedCells[i]) < maxLines {
				wrappedCells[i] = append(wrappedCells[i], "")
			}
		}
		wrappedRows[rowIdx] = wrappedCells
	}

	var result strings.Builder
	borderStyle := MarkdownTableBorderStyle
	headerStyle := MarkdownTableHeaderStyle
	cellStyle := MarkdownTableCellStyle

	// Render top border
	result.WriteString(borderStyle.Render("┌"))
	for i, w := range colWidths {
		result.WriteString(borderStyle.Render(strings.Repeat("─", w+2)))
		if i < len(colWidths)-1 {
			result.WriteString(borderStyle.Render("┬"))
		}
	}
	result.WriteString(borderStyle.Render("┐"))
	result.WriteString("\n")

	// Render rows
	for rowIdx, wrappedCells := range wrappedRows {
		numLines := len(wrappedCells[0]) // All cells have same number of lines
		for lineIdx := 0; lineIdx < numLines; lineIdx++ {
			result.WriteString(borderStyle.Render("│"))
			for i := 0; i < numCols; i++ {
				cellLine := wrappedCells[i][lineIdx]
				// Pad cell line to column width using visual width
				cellVisualWidth := lipgloss.Width(cellLine)
				padding := colWidths[i] - cellVisualWidth
				if padding < 0 {
					padding = 0
				}
				padded := cellLine + strings.Repeat(" ", padding)
				// Apply style based on whether it's a header row
				if rowIdx == 0 && hasHeader {
					result.WriteString(" " + headerStyle.Render(padded) + " ")
				} else {
					result.WriteString(" " + cellStyle.Render(padded) + " ")
				}
				result.WriteString(borderStyle.Render("│"))
			}
			result.WriteString("\n")
		}

		// Render header separator after first row if it's a header
		if rowIdx == 0 && hasHeader && len(wrappedRows) > 1 {
			result.WriteString(borderStyle.Render("├"))
			for i, w := range colWidths {
				result.WriteString(borderStyle.Render(strings.Repeat("─", w+2)))
				if i < len(colWidths)-1 {
					result.WriteString(borderStyle.Render("┼"))
				}
			}
			result.WriteString(borderStyle.Render("┤"))
			result.WriteString("\n")
		}
	}

	// Render bottom border
	result.WriteString(borderStyle.Render("└"))
	for i, w := range colWidths {
		result.WriteString(borderStyle.Render(strings.Repeat("─", w+2)))
		if i < len(colWidths)-1 {
			result.WriteString(borderStyle.Render("┴"))
		}
	}
	result.WriteString(borderStyle.Render("┘"))

	return result.String()
}

// calculateTableColumnWidths distributes available width among columns.
// When content fits naturally, columns use their natural width.
// When content is too wide, larger columns are shrunk proportionally
// while smaller columns keep their natural width.
func calculateTableColumnWidths(naturalWidths []int, availableWidth int) []int {
	numCols := len(naturalWidths)
	colWidths := make([]int, numCols)

	// Calculate total natural width
	totalNatural := 0
	for _, w := range naturalWidths {
		totalNatural += w
	}

	// If everything fits, use natural widths
	if totalNatural <= availableWidth {
		copy(colWidths, naturalWidths)
		return colWidths
	}

	// Need to shrink columns. Strategy:
	// 1. Columns that are already small keep their width
	// 2. Larger columns share the remaining space proportionally

	avgWidth := availableWidth / numCols
	if avgWidth < TableMinColumnWidth {
		avgWidth = TableMinColumnWidth
	}

	// First pass: assign width to small columns (those at or below average)
	remaining := availableWidth
	flexibleCols := 0
	for i, natural := range naturalWidths {
		if natural <= avgWidth {
			colWidths[i] = natural
			remaining -= natural
		} else {
			flexibleCols++
		}
	}

	// Second pass: distribute remaining width among flexible (wider) columns
	if flexibleCols > 0 && remaining > 0 {
		perFlexible := remaining / flexibleCols
		if perFlexible < TableMinColumnWidth {
			perFlexible = TableMinColumnWidth
		}
		for i, natural := range naturalWidths {
			if natural > avgWidth {
				colWidths[i] = perFlexible
			}
		}
	}

	// Ensure minimum width for all columns
	for i := range colWidths {
		if colWidths[i] < TableMinColumnWidth {
			colWidths[i] = TableMinColumnWidth
		}
	}

	return colWidths
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
		return MarkdownBlockquoteStyle.Render(wrapText(renderInlineMarkdown(content), width-BlockquotePrefixWidth))
	}

	// Unordered list items
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
		content := trimmed[2:]
		bullet := MarkdownListBulletStyle.Render("•")
		// Wrap list item content, accounting for the prefix width "  • " = 4 chars
		wrapped := wrapText(renderInlineMarkdown(content), width-ListItemPrefixWidth)
		// Indent continuation lines to align with first line content
		lines := strings.Split(wrapped, "\n")
		if len(lines) > 1 {
			indent := strings.Repeat(" ", ListItemContinuationIndent)
			for i := 1; i < len(lines); i++ {
				lines[i] = indent + lines[i]
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
			// Calculate prefix width: "  " + number + ". " (2 + len(number) + 2)
			// For numbers 1-9: 5 chars, for 10-99: 6 chars
			prefixWidth := NumberedListPrefixWidth
			if i >= 10 {
				prefixWidth = NumberedListPrefixWidth + 1
			}
			// Wrap list item content, accounting for the prefix width
			wrapped := wrapText(renderInlineMarkdown(content), width-prefixWidth)
			// Indent continuation lines to align with first line content
			lines := strings.Split(wrapped, "\n")
			if len(lines) > 1 {
				indent := strings.Repeat(" ", prefixWidth)
				for j := 1; j < len(lines); j++ {
					lines[j] = indent + lines[j]
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

	// Table state
	inTable := false
	var tableRows [][]string
	tableHasHeader := false

	// Helper function to flush table
	flushTable := func() {
		if len(tableRows) > 0 {
			if result.Len() > 0 {
				result.WriteString("\n")
			}
			result.WriteString(renderTable(tableRows, tableHasHeader, width))
			result.WriteString("\n")
			tableRows = nil
			tableHasHeader = false
		}
		inTable = false
	}

	for i, line := range lines {
		// Check for code block start/end
		if strings.HasPrefix(line, "```") {
			// If we were in a table, flush it first
			if inTable {
				flushTable()
			}

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
			continue
		}

		// Check for table rows
		if isTableRow(line) {
			// Check if this is a separator row (marks that previous row was header)
			if isTableSeparator(line) {
				// This is a separator, so previous row was a header
				if len(tableRows) == 1 {
					tableHasHeader = true
				}
				// Skip the separator row, we render our own borders
				continue
			}

			// This is a data row
			if !inTable {
				inTable = true
				tableRows = nil
			}
			cells := parseTableRow(line)
			tableRows = append(tableRows, cells)
			continue
		}

		// If we were in a table but this line isn't a table row, flush the table
		if inTable {
			flushTable()
		}

		// Check if next line might be a table separator (lookahead for header detection)
		// This handles the case where we see a row that looks like a table row
		// but we haven't entered table mode yet
		if i+1 < len(lines) && isTableRow(line) && isTableSeparator(lines[i+1]) {
			inTable = true
			tableRows = [][]string{parseTableRow(line)}
			continue
		}

		// Render markdown line with wrapping
		result.WriteString(renderMarkdownLine(line, width))
		result.WriteString("\n")
	}

	// If we ended while still in a code block, output whatever we have
	if inCodeBlock {
		highlighted := highlightCode(codeBlockContent.String(), codeBlockLang)
		result.WriteString(highlighted)
	}

	// Flush any remaining table
	if inTable && len(tableRows) > 0 {
		flushTable()
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

	// Description (wrapped to fit within box padding)
	descStyle := PermissionDescStyle.Width(wrapWidth - OverlayBoxPadding)
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

	// Wrap in a box, capped at max width for readability
	boxWidth := wrapWidth
	if boxWidth > OverlayBoxMaxWidth {
		boxWidth = OverlayBoxMaxWidth
	}
	return PermissionBoxStyle.Width(boxWidth).Render(sb.String())
}

// renderTodoList renders the todo list from a TodoWrite tool call
func renderTodoList(list *pclaude.TodoList, wrapWidth int) string {
	if list == nil || len(list.Items) == 0 {
		return ""
	}

	var sb strings.Builder

	// Title with progress summary
	pending, inProgress, completed := list.CountByStatus()
	total := len(list.Items)

	titleStyle := lipgloss.NewStyle().Foreground(ColorInfo).Bold(true)
	sb.WriteString(titleStyle.Render("📋 Task Progress"))

	// Progress indicator
	progressStyle := lipgloss.NewStyle().Foreground(ColorTextMuted)
	sb.WriteString(progressStyle.Render(fmt.Sprintf(" (%d/%d)", completed, total)))
	sb.WriteString("\n\n")

	// Render each todo item
	for _, item := range list.Items {
		var marker string
		var contentStyle lipgloss.Style

		switch item.Status {
		case pclaude.TodoStatusCompleted:
			marker = TodoCompletedMarkerStyle.Render("✓")
			contentStyle = TodoCompletedContentStyle
		case pclaude.TodoStatusInProgress:
			// Use a single-width character for consistent alignment
			marker = TodoInProgressMarkerStyle.Render("▸")
			contentStyle = TodoInProgressContentStyle
		default: // pending
			marker = TodoPendingMarkerStyle.Render("○")
			contentStyle = TodoPendingContentStyle
		}

		sb.WriteString(marker)
		sb.WriteString(" ")

		// For in-progress items, show the activeForm if available
		content := item.Content
		if item.Status == pclaude.TodoStatusInProgress && item.ActiveForm != "" {
			content = item.ActiveForm
		}

		// Wrap long content, accounting for marker width and box padding
		// Total prefix: marker (2) + box padding (6) = 8 chars
		maxContentWidth := wrapWidth - TodoMarkerWidth - TodoItemPadding
		if maxContentWidth < MinWrapWidth {
			maxContentWidth = MinWrapWidth
		}
		wrappedContent := wrapText(content, maxContentWidth)

		// Handle multi-line wrapped content - indent continuation lines
		// Indent should match the marker width so text aligns
		continuationIndent := strings.Repeat(" ", TodoMarkerWidth+1) // +1 for visual alignment
		lines := strings.Split(wrappedContent, "\n")
		for i, line := range lines {
			if i > 0 {
				sb.WriteString(continuationIndent)
			}
			sb.WriteString(contentStyle.Render(line))
			if i < len(lines)-1 {
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
	}

	// Show summary hint if tasks are in progress
	if inProgress > 0 || pending > 0 {
		summaryStyle := lipgloss.NewStyle().Foreground(ColorTextMuted).Italic(true)
		if inProgress > 0 {
			sb.WriteString(summaryStyle.Render(fmt.Sprintf("Working on %d task(s)...", inProgress)))
		}
	}

	// Wrap in a box, capped at max width for readability
	boxWidth := wrapWidth
	if boxWidth > OverlayBoxMaxWidth {
		boxWidth = OverlayBoxMaxWidth
	}
	return TodoListBoxStyle.Width(boxWidth).Render(sb.String())
}
