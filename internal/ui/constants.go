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
	ModalWidth = 60

	// ModalInputCharLimit is the character limit for modal text inputs
	ModalInputCharLimit = 256

	// ModalInputWidth is the width of modal text inputs
	ModalInputWidth = 50
)
