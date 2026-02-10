package modals

import (
	"image/color"

	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
)

// Style variables - these will be set by the parent ui package via SetStyles
var (
	ModalTitleStyle      lipgloss.Style
	ModalHelpStyle       lipgloss.Style
	SidebarItemStyle     lipgloss.Style
	SidebarSelectedStyle lipgloss.Style
	StatusErrorStyle     lipgloss.Style

	ColorPrimary     color.Color
	ColorSecondary   color.Color
	ColorText        color.Color
	ColorTextMuted   color.Color
	ColorTextInverse color.Color
	ColorUser        color.Color
	ColorWarning     color.Color

	ModalInputWidth     int
	ModalInputCharLimit int
	ModalWidth          int
	ModalWidthWide      int
)

// Modal visibility limits - maximum items shown at once before scrolling
var (
	HelpModalMaxVisible      int
	IssuesModalMaxVisible    int
	SearchModalMaxVisible    int
	ChangelogModalMaxVisible int
)

// Text input character limits for various modal inputs
var (
	BranchNameCharLimit        int
	SessionNameCharLimit       int
	SearchInputCharLimit       int
	MCPServerNameCharLimit     int
	MCPCommandCharLimit        int
	MCPArgsCharLimit           int
	PluginSearchCharLimit      int
	MarketplaceSourceCharLimit int
	BranchPrefixCharLimit      int
)

// SetStyles sets the style variables from the parent ui package.
// This must be called before rendering any modals.
func SetStyles(
	modalTitle, modalHelp, sidebarItem, sidebarSelected, statusError lipgloss.Style,
	primary, secondary, text, textMuted, textInverse, user, warning color.Color,
	inputWidth, inputCharLimit, modalWidth, modalWidthWide int,
) {
	ModalTitleStyle = modalTitle
	ModalHelpStyle = modalHelp
	SidebarItemStyle = sidebarItem
	SidebarSelectedStyle = sidebarSelected
	StatusErrorStyle = statusError

	ColorPrimary = primary
	ColorSecondary = secondary
	ColorText = text
	ColorTextMuted = textMuted
	ColorTextInverse = textInverse
	ColorUser = user
	ColorWarning = warning

	ModalInputWidth = inputWidth
	ModalInputCharLimit = inputCharLimit
	ModalWidth = modalWidth
	ModalWidthWide = modalWidthWide
}

// ModalConstants holds all the constant values needed by modals
type ModalConstants struct {
	// Visibility limits
	HelpModalMaxVisible      int
	IssuesModalMaxVisible    int
	SearchModalMaxVisible    int
	ChangelogModalMaxVisible int

	// Text input character limits
	BranchNameCharLimit        int
	SessionNameCharLimit       int
	SearchInputCharLimit       int
	MCPServerNameCharLimit     int
	MCPCommandCharLimit        int
	MCPArgsCharLimit           int
	PluginSearchCharLimit      int
	MarketplaceSourceCharLimit int
	BranchPrefixCharLimit      int
}

// SetConstants sets the constant values from the parent ui package.
// This must be called before rendering any modals.
func SetConstants(c ModalConstants) {
	HelpModalMaxVisible = c.HelpModalMaxVisible
	IssuesModalMaxVisible = c.IssuesModalMaxVisible
	SearchModalMaxVisible = c.SearchModalMaxVisible
	ChangelogModalMaxVisible = c.ChangelogModalMaxVisible

	BranchNameCharLimit = c.BranchNameCharLimit
	SessionNameCharLimit = c.SessionNameCharLimit
	SearchInputCharLimit = c.SearchInputCharLimit
	MCPServerNameCharLimit = c.MCPServerNameCharLimit
	MCPCommandCharLimit = c.MCPCommandCharLimit
	MCPArgsCharLimit = c.MCPArgsCharLimit
	PluginSearchCharLimit = c.PluginSearchCharLimit
	MarketplaceSourceCharLimit = c.MarketplaceSourceCharLimit
	BranchPrefixCharLimit = c.BranchPrefixCharLimit
}

// ApplyTextareaStyles configures a textarea with transparent background styles.
// This ensures the textarea background matches the terminal background instead
// of using the default black background.
func ApplyTextareaStyles(ta *textarea.Model) {
	styles := ta.Styles()

	// Create base style without background - let terminal's native background show through
	baseStyle := lipgloss.NewStyle()

	textStyle := lipgloss.NewStyle().
		Foreground(ColorText)

	placeholderStyle := lipgloss.NewStyle().
		Foreground(ColorTextMuted)

	// Configure focused state - no background colors
	styles.Focused.Base = baseStyle
	styles.Focused.Text = textStyle
	styles.Focused.Placeholder = placeholderStyle
	styles.Focused.CursorLine = textStyle // Remove background from cursor line
	styles.Focused.Prompt = textStyle

	// Configure blurred state (same colors, just not focused)
	styles.Blurred.Base = baseStyle
	styles.Blurred.Text = textStyle
	styles.Blurred.Placeholder = placeholderStyle
	styles.Blurred.CursorLine = textStyle
	styles.Blurred.Prompt = textStyle

	ta.SetStyles(styles)
}
