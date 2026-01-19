// Package ui provides constants for layout calculations and configuration.
package ui

// Layout constants for panel sizing
const (
	// HeaderHeight is the height of the header in lines
	HeaderHeight = 1

	// FooterHeight is the height of the footer in lines
	FooterHeight = 1

	// BorderSize is the total border width (1 on each side)
	BorderSize = 2

	// SidebarWidthRatio is the denominator for sidebar width (1/3 of total width)
	SidebarWidthRatio = 3

	// MinTerminalWidth is the minimum supported terminal width
	MinTerminalWidth = 40

	// MinTerminalHeight is the minimum supported terminal height
	MinTerminalHeight = 10

	// TextareaHeight is the number of lines for the chat input textarea
	TextareaHeight = 3

	// TextareaBorderHeight is the border size around the textarea
	TextareaBorderHeight = 2

	// InputPaddingWidth is the horizontal padding inside the input area (Padding(0, 1) = 1 left + 1 right)
	InputPaddingWidth = 2

	// InputTotalHeight is the total height of the input area (textarea + borders)
	InputTotalHeight = TextareaHeight + TextareaBorderHeight

	// TitleHeight is the height of panel titles
	TitleHeight = 1

	// SeparatorHeight is the height of separators between sections
	SeparatorHeight = 1

	// DefaultWrapWidth is the default width for text wrapping when viewport width is unknown
	DefaultWrapWidth = 80
)

// Session message limits
const (
	// MaxSessionMessageLines is the maximum number of lines to keep in session message history
	MaxSessionMessageLines = 10000

	// PermissionChannelBuffer is the buffer size for permission request/response channels
	PermissionChannelBuffer = 1

	// PermissionTimeoutSeconds is the timeout for waiting for permission responses
	PermissionTimeoutSeconds = 300 // 5 minutes
)

// Modal dimensions
const (
	// ModalWidth is the default width of modals
	ModalWidth = 80

	// ModalInputCharLimit is the character limit for modal text inputs
	ModalInputCharLimit = 256

	// ModalInputWidth is the width of modal text inputs
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
)
