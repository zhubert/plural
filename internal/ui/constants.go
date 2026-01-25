// Package ui provides constants for layout calculations and configuration.
//
// # Layout System Overview
//
// The terminal is divided into a fixed structure:
//
//	┌──────────────────────────────────────────────────────────────────┐
//	│                        Header (1 line)                           │
//	├──────────────────────┬───────────────────────────────────────────┤
//	│                      │                                           │
//	│    Sidebar           │           Chat Panel                      │
//	│    (1/5 width)       │           (4/5 width)                     │
//	│                      │                                           │
//	│    - Session list    │    ┌─────────────────────────────────┐    │
//	│    - Repo grouping   │    │  Message viewport               │    │
//	│                      │    │  (scrollable)                   │    │
//	│                      │    │                                 │    │
//	│                      │    ├─────────────────────────────────┤    │
//	│                      │    │  Input textarea (3 lines)       │    │
//	│                      │    └─────────────────────────────────┘    │
//	│                      │                                           │
//	├──────────────────────┴───────────────────────────────────────────┤
//	│                        Footer (1 line)                           │
//	└──────────────────────────────────────────────────────────────────┘
//
// Key layout calculations:
//   - ContentHeight = TerminalHeight - HeaderHeight - FooterHeight
//   - SidebarWidth = TerminalWidth / SidebarWidthRatio (1/5)
//   - ChatWidth = TerminalWidth - SidebarWidth (4/5)
//   - ChatViewportHeight = ContentHeight - InputTotalHeight - BorderSize
//
// The ViewContext singleton (context.go) centralizes these calculations and provides
// helper methods for consistent sizing across components.
package ui

// Layout constants for panel sizing.
//
// These values define the fixed-size elements of the UI. The remaining space
// is distributed proportionally between the sidebar and chat panels.
const (
	// HeaderHeight is the height of the header bar showing the app title and session info.
	// A single line provides enough space for the gradient title and session name while
	// maximizing vertical space for content.
	HeaderHeight = 1

	// FooterHeight is the height of the footer showing keyboard shortcuts and flash messages.
	// A single line is sufficient since we show context-sensitive shortcuts based on focus.
	FooterHeight = 1

	// BorderSize is the total vertical border space (1px top + 1px bottom = 2).
	// Used when calculating inner content height for panels with borders.
	BorderSize = 2

	// SidebarWidthRatio determines sidebar width as TerminalWidth/SidebarWidthRatio.
	// Value of 5 means sidebar gets 1/5 of width, chat gets 4/5. This ratio provides
	// enough space for session names while maximizing space for conversation content.
	SidebarWidthRatio = 5

	// MinTerminalWidth is the minimum width required for the UI to function.
	// Below this, layout calculations could produce negative widths.
	MinTerminalWidth = 40

	// MinTerminalHeight is the minimum height required for the UI to function.
	// Below this, there's no room for header + footer + any content.
	MinTerminalHeight = 10

	// TextareaHeight is the input area height in lines.
	// 3 lines allows multi-line input for longer prompts while keeping
	// most vertical space for the conversation history.
	TextareaHeight = 3

	// TextareaBorderHeight is the border around the textarea (top + bottom).
	TextareaBorderHeight = 2

	// InputPaddingWidth is horizontal padding inside the input (left + right).
	// Used when calculating the actual text width available for typing.
	InputPaddingWidth = 2

	// InputTotalHeight is the total vertical space consumed by the input area.
	// This is subtracted from content height to determine viewport height.
	// Note: When an image is attached, add ImageIndicatorHeight to this value.
	InputTotalHeight = TextareaHeight + TextareaBorderHeight

	// ImageIndicatorHeight is the extra line used when an image is attached.
	// The indicator shows "[Image attached: NKB] (backspace to remove)".
	ImageIndicatorHeight = 1

	// TitleHeight is the height of panel title bars (currently unused but reserved).
	TitleHeight = 1

	// SeparatorHeight is the height of visual separators between sections.
	SeparatorHeight = 1

	// DefaultWrapWidth is the fallback text wrap width when viewport width is unknown.
	// 80 characters is a traditional terminal width that provides good readability.
	// This is mainly used during initialization before the terminal size is known.
	DefaultWrapWidth = 80
)

// Session message limits and IPC configuration.
const (
	// MaxSessionMessageLines limits the session message history file size.
	// 10,000 lines provides substantial history while preventing unbounded growth.
	// Messages are stored as JSON lines in ~/.plural/sessions/<session-id>.json.
	MaxSessionMessageLines = 10000

	// PermissionChannelBuffer is the buffer size for permission request/response channels.
	// A buffer of 1 prevents blocking while allowing at most one pending permission.
	// Larger buffers aren't needed since permissions are processed sequentially.
	PermissionChannelBuffer = 1

	// PermissionTimeoutSeconds is how long to wait for user response to permission prompts.
	// 5 minutes gives users time to review complex permissions without causing the
	// Claude process to hang indefinitely if the TUI is backgrounded.
	PermissionTimeoutSeconds = 300 // 5 minutes
)

// Modal dimensions.
//
// Modals are centered overlays for focused interactions (creating sessions,
// configuring settings, etc.). These constants ensure consistent sizing.
const (
	// ModalWidth is the default width of modal dialogs in characters.
	// 80 characters matches DefaultWrapWidth and traditional terminal width.
	ModalWidth = 80

	// ModalInputCharLimit is the maximum characters for modal text inputs.
	// 256 characters is sufficient for session names, branch names, and paths.
	ModalInputCharLimit = 256

	// ModalInputWidth is the width of text input fields inside modals.
	// 72 characters leaves room for padding and borders within the 80-char modal.
	ModalInputWidth = 72
)

// Modal visibility limits - maximum items shown at once before scrolling
const (
	// HelpModalMaxVisible is the max visible lines in the help modal
	HelpModalMaxVisible = 18

	// IssuesModalMaxVisible is the max visible issues in the GitHub issues modal
	IssuesModalMaxVisible = 10

	// SearchModalMaxVisible is the max visible search results
	SearchModalMaxVisible = 8

	// ChangelogModalMaxVisible is the max visible lines in the changelog modal
	ChangelogModalMaxVisible = 15

	// PlanApprovalMaxVisible is the max visible lines in the plan approval prompt
	PlanApprovalMaxVisible = 20
)

// Text input character limits for various inputs
const (
	// SidebarSearchCharLimit is the character limit for sidebar search
	SidebarSearchCharLimit = 50

	// BranchNameCharLimit is the character limit for branch name inputs
	BranchNameCharLimit = 100

	// SessionNameCharLimit is the character limit for session name inputs
	SessionNameCharLimit = 100

	// SearchInputCharLimit is the character limit for search filter inputs
	SearchInputCharLimit = 100

	// MCPServerNameCharLimit is the character limit for MCP server names
	MCPServerNameCharLimit = 50

	// MCPCommandCharLimit is the character limit for MCP server commands
	MCPCommandCharLimit = 100

	// MCPArgsCharLimit is the character limit for MCP server arguments
	MCPArgsCharLimit = 200

	// PluginSearchCharLimit is the character limit for plugin search
	PluginSearchCharLimit = 50

	// MarketplaceSourceCharLimit is the character limit for marketplace source URLs
	MarketplaceSourceCharLimit = 200

	// BranchPrefixCharLimit is the character limit for branch prefix settings
	BranchPrefixCharLimit = 50
)

// Logging preview lengths
const (
	// PasteContentPreviewLen is the max length for paste content in logs
	PasteContentPreviewLen = 100

	// InputMessagePreviewLen is the max length for input message previews in logs
	InputMessagePreviewLen = 50
)

// Syntax highlighting configuration
const (
	// DefaultSyntaxStyle is the default chroma style for syntax highlighting
	DefaultSyntaxStyle = "monokai"

	// DefaultTerminalFormatter is the default chroma formatter for terminal output
	DefaultTerminalFormatter = "terminal256"
)

// Todo list rendering
const (
	// TodoListMinWrapWidth is the minimum wrap width for todo lists
	TodoListMinWrapWidth = 20

	// TodoListFallbackWrapWidth is the fallback wrap width when viewport not initialized
	TodoListFallbackWrapWidth = 80

	// TodoSidebarWidthRatio determines sidebar width as ChatWidth/TodoSidebarWidthRatio.
	// Value of 4 means todo sidebar gets 1/4 of chat panel width.
	TodoSidebarWidthRatio = 4
)

// Text wrapping and indentation constants.
//
// These constants define the exact character widths used when wrapping text
// for different markdown elements. Each constant documents its calculation
// to prevent magic numbers and ensure continuation lines align properly.
//
// Visual structure for list items:
//
//	Unordered: "  • content here..."     (2 spaces + bullet + space = 4 chars)
//	           "    continuation..."     (4 spaces for continuation)
//
//	Numbered:  "  1. content here..."    (2 spaces + digit + dot + space = 5 chars for 1-9)
//	           "     continuation..."    (5 spaces for continuation)
//
//	Blockquote: "▎ content here..."      (bar + space = 2 chars visible, but styled)
const (
	// ContentPadding is the horizontal padding applied to viewport content.
	// Applied as Padding(0, 1) which adds 1 char on each side = 2 total.
	// This is subtracted from viewport width to get the usable wrap width.
	ContentPadding = 2

	// ListItemPrefixWidth is the width of the unordered list item prefix "  • ".
	// Breakdown: 2 leading spaces + 1 bullet char + 1 trailing space = 4 chars.
	ListItemPrefixWidth = 4

	// ListItemContinuationIndent is the indentation for wrapped list item lines.
	// Must match ListItemPrefixWidth so text aligns vertically.
	ListItemContinuationIndent = 4

	// NumberedListPrefixWidth is the width of numbered list prefixes "  N. ".
	// Breakdown: 2 leading spaces + 1-2 digit chars + 1 dot + 1 space = 5-6 chars.
	// We use 5 for single-digit numbers (1-9) as the common case.
	NumberedListPrefixWidth = 5

	// NumberedListContinuationIndent is the indentation for wrapped numbered list lines.
	// Must match NumberedListPrefixWidth so text aligns vertically.
	NumberedListContinuationIndent = 5

	// BlockquotePrefixWidth is the effective width consumed by blockquote styling.
	// The blockquote style adds a left border and padding. We account for 4 chars
	// to ensure content doesn't overflow: 1 border + 1 padding + 2 safety margin.
	BlockquotePrefixWidth = 4

	// TodoMarkerWidth is the width of todo item markers "✓ ", "▸ ", or "○ ".
	// Breakdown: 1 marker char + 1 space = 2 chars.
	TodoMarkerWidth = 2

	// TodoItemPadding is additional padding for todo items within the box.
	// Combined with marker: 2 (marker) + 4 (leading space) + 2 (trailing) = 8 total.
	TodoItemPadding = 6

	// OverlayBoxPadding is the padding inside overlay boxes (permission, question, plan).
	// The box style adds padding, so we subtract this from wrap width for content.
	OverlayBoxPadding = 4

	// OverlayBoxMaxWidth is the maximum width for overlay boxes.
	// Capped at 80 chars for readability, matching traditional terminal width.
	// This applies to permission prompts, question prompts, and todo lists.
	OverlayBoxMaxWidth = 80

	// PlanBoxMaxWidth is the maximum width for plan approval boxes.
	// Plans can contain code and complex content, so we allow a wider box (100 chars)
	// to reduce excessive wrapping while still fitting most terminals.
	PlanBoxMaxWidth = 100

	// MinWrapWidth is the minimum width below which wrapping degrades.
	// At very narrow widths, wrapping produces poor results. This provides
	// a floor for wrap width calculations.
	MinWrapWidth = 20

	// TableMinColumnWidth is the minimum characters per table column.
	// Columns narrower than this become unreadable.
	TableMinColumnWidth = 3

	// TableCellPadding is the padding around table cell content.
	// Each cell has 1 space padding on left and right = 2 chars per cell.
	TableCellPadding = 2

	// TableBorderWidth is the width of a single table border character "│".
	TableBorderWidth = 1
)
