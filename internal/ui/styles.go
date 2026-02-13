package ui

import "charm.land/lipgloss/v2"

// Color palette - Purple + Cyan/Teal theme
var (
	ColorPrimary     = lipgloss.Color("#7C3AED") // Purple
	ColorSecondary   = lipgloss.Color("#06B6D4") // Cyan
	ColorMuted       = lipgloss.Color("#6B7280") // Gray
	ColorBorder      = lipgloss.Color("#374151") // Dark gray
	ColorBorderFocus = lipgloss.Color("#7C3AED") // Purple when focused
	ColorBg          = lipgloss.Color("#1F2937") // Dark background
	ColorText        = lipgloss.Color("#F9FAFB") // Light text
	ColorTextMuted   = lipgloss.Color("#B0B8C4") // Muted text
	ColorTextInverse = lipgloss.Color("#1F2937") // Dark text for light backgrounds
	ColorUser        = lipgloss.Color("#A78BFA") // Light purple for user messages
	ColorAssistant   = lipgloss.Color("#22D3EE") // Bright cyan for assistant messages
	ColorWarning     = lipgloss.Color("#F59E0B") // Amber for permission prompts
	ColorInfo        = lipgloss.Color("#06B6D4") // Cyan for info/questions
	ColorError       = lipgloss.Color("#EF4444") // Red for errors
	ColorSuccess     = lipgloss.Color("#10B981") // Green for success
)

// Header styles
var (
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorText).
			Background(ColorPrimary).
			Padding(0, 1)

	HeaderTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorText)
)

// Footer styles
var (
	FooterStyle = lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Padding(0, 1)

	FooterKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSecondary)

	FooterDescStyle = lipgloss.NewStyle().
			Foreground(ColorTextMuted)
)

// Panel styles
var (
	PanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder)

	PanelFocusedStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorderFocus)

	PanelTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			Padding(0, 1)
)

// Sidebar styles
var (
	SidebarItemStyle = lipgloss.NewStyle().
				Padding(0, 1)

	// SidebarSelectedStyle uses theme's BgSelected color - initialized properly in regenerateStyles()
	SidebarSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(BuiltinThemes[DefaultTheme].GetBgSelected())).
				Foreground(lipgloss.Color(BuiltinThemes[DefaultTheme].Text)).
				Bold(true).
				Padding(0, 1)

	SidebarRepoStyle = lipgloss.NewStyle().
				Foreground(ColorMuted).
				Italic(true)
)

// Chat styles
var (
	ChatUserStyle = lipgloss.NewStyle().
			Foreground(ColorUser).
			Bold(true)

	ChatAssistantStyle = lipgloss.NewStyle().
				Foreground(ColorAssistant).
				Bold(true)

	ChatMessageStyle = lipgloss.NewStyle().
				Foreground(ColorText)

	ChatInputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1)

	ChatInputFocusedStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorderFocus).
				Padding(0, 1)
)

// Modal styles
var (
	ModalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 2).
			Width(ModalWidth)

	ModalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			MarginBottom(1)

	ModalHelpStyle = lipgloss.NewStyle().
			Foreground(ColorTextMuted).
			Italic(true).
			MarginTop(1)
)

// Status styles
var (
	StatusLoadingStyle = lipgloss.NewStyle().
				Foreground(ColorSecondary).
				Italic(true)

	StatusErrorStyle = lipgloss.NewStyle().
				Foreground(ColorError).
				Bold(true)
)

// Tool use marker styles
var (
	ToolUseInProgressStyle = lipgloss.NewStyle().
				Foreground(ColorText) // White circle for in-progress

	ToolUseCompleteStyle = lipgloss.NewStyle().
				Foreground(ColorSecondary) // Green circle for completed
)

// Permission prompt styles
var (
	PermissionBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorWarning).
				Padding(0, 1)

	PermissionTitleStyle = lipgloss.NewStyle().
				Foreground(ColorWarning).
				Bold(true)

	PermissionToolStyle = lipgloss.NewStyle().
				Foreground(ColorText).
				Bold(true)

	PermissionDescStyle = lipgloss.NewStyle().
				Foreground(ColorTextMuted)

	PermissionHintStyle = lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Italic(true)

	PermissionIndicatorStyle = lipgloss.NewStyle().
					Foreground(ColorWarning).
					Bold(true)
)

// Question prompt styles
var (
	QuestionBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorInfo).
		Padding(0, 1)
)

// Plan approval prompt styles
var (
	PlanApprovalBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorInfo).
		Padding(1, 2)
)

// Todo list styles
var (
	// Box style for the todo list container (used when inline)
	TodoListBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorInfo).
				Padding(0, 1)

	// TodoSidebarStyle is for the todo list when shown as a sidebar panel.
	// Uses left border only to connect visually with the chat panel.
	TodoSidebarStyle = lipgloss.NewStyle().
				Border(lipgloss.Border{
			Top:         "─",
			Bottom:      "─",
			Left:        "│",
			Right:       "│",
			TopLeft:     "┬",
			TopRight:    "╮",
			BottomLeft:  "┴",
			BottomRight: "╯",
		}).
		BorderForeground(ColorBorder)

	// Marker styles for different states (updated by regenerateStyles)
	TodoCompletedMarkerStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color(BuiltinThemes[DefaultTheme].DiffAdded))

	TodoInProgressMarkerStyle = lipgloss.NewStyle().
					Foreground(ColorSecondary) // Cyan hourglass

	TodoPendingMarkerStyle = lipgloss.NewStyle().
				Foreground(ColorMuted) // Gray circle

	// Content styles for different states
	TodoCompletedContentStyle = lipgloss.NewStyle().
					Foreground(ColorTextMuted).
					Strikethrough(true)

	TodoInProgressContentStyle = lipgloss.NewStyle().
					Foreground(ColorText).
					Bold(true)

	TodoPendingContentStyle = lipgloss.NewStyle().
				Foreground(ColorTextMuted)
)

// Markdown rendering styles (updated by regenerateStyles)
var (
	// Headers
	MarkdownH1Style = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(BuiltinThemes[DefaultTheme].MarkdownH1)).
			MarginTop(1)

	MarkdownH2Style = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(BuiltinThemes[DefaultTheme].MarkdownH2)).
			MarginTop(1)

	MarkdownH3Style = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(BuiltinThemes[DefaultTheme].MarkdownH3))

	MarkdownH4Style = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorTextMuted)

	// Inline styles
	MarkdownBoldStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorText)

	MarkdownItalicStyle = lipgloss.NewStyle().
				Italic(true).
				Foreground(ColorText)

	MarkdownInlineCodeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(BuiltinThemes[DefaultTheme].MarkdownCode)).
				Background(lipgloss.Color(BuiltinThemes[DefaultTheme].MarkdownCodeBg))

	// Code block
	MarkdownCodeBlockStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(BuiltinThemes[DefaultTheme].MarkdownCodeBg))

	// List
	MarkdownListBulletStyle = lipgloss.NewStyle().
				Foreground(ColorSecondary)

	// Blockquote
	MarkdownBlockquoteStyle = lipgloss.NewStyle().
				Foreground(ColorTextMuted).
				Italic(true).
				BorderLeft(true).
				BorderStyle(lipgloss.ThickBorder()).
				BorderForeground(ColorMuted).
				PaddingLeft(1)

	// Horizontal rule
	MarkdownHRStyle = lipgloss.NewStyle().
			Foreground(ColorBorder)

	// Link
	MarkdownLinkStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(BuiltinThemes[DefaultTheme].MarkdownLink)).
				Underline(true)

	// Table
	MarkdownTableBorderStyle = lipgloss.NewStyle().
					Foreground(ColorBorder)

	MarkdownTableHeaderStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(ColorSecondary)

	MarkdownTableCellStyle = lipgloss.NewStyle().
				Foreground(ColorText)
)

// Diff coloring styles (updated by regenerateStyles)
var (
	DiffAddedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(BuiltinThemes[DefaultTheme].DiffAdded))

	DiffRemovedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(BuiltinThemes[DefaultTheme].DiffRemoved))

	DiffHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(BuiltinThemes[DefaultTheme].DiffHeader)).
			Bold(true)

	DiffHunkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(BuiltinThemes[DefaultTheme].DiffHunk))

	// View changes file list selection style (updated by regenerateStyles)
	ViewChangesSelectedStyle = lipgloss.NewStyle().
					Background(lipgloss.Color(BuiltinThemes[DefaultTheme].GetBgSelected())).
					Foreground(lipgloss.Color(BuiltinThemes[DefaultTheme].Text))
)

// Text selection style (updated by regenerateStyles)
var (
	TextSelectionStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(BuiltinThemes[DefaultTheme].TextSelectionBg)).
				Foreground(lipgloss.Color(BuiltinThemes[DefaultTheme].TextSelectionFg))

	// TextSelectionFlashStyle is used briefly when text is copied to indicate success
	// (updated by regenerateStyles)
	TextSelectionFlashStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(BuiltinThemes[DefaultTheme].Success)).
				Foreground(lipgloss.Color(BuiltinThemes[DefaultTheme].TextInverse))
)
