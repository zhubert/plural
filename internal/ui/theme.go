// Package ui provides theme management for the application.
// Themes define the color palette used throughout the UI, allowing users
// to customize the visual appearance of Plural.
package ui

import "charm.land/lipgloss/v2"

// Theme defines a complete color palette for the application.
// Each theme provides colors for all UI elements, ensuring visual consistency.
type Theme struct {
	// Name is the display name of the theme
	Name string

	// Primary is the main accent color (used for focus, highlights, headers)
	Primary string
	// Secondary is the secondary accent color (used for assistant messages, info)
	Secondary string

	// Background colors
	Bg         string // Main background
	BgSelected string // Selected item background (defaults to Primary if empty)

	// Text colors
	Text        string // Primary text
	TextMuted   string // Secondary/muted text
	TextInverse string // Text on colored backgrounds

	// Semantic colors
	User      string // User message labels
	Assistant string // Assistant message labels
	Warning   string // Permission prompts, warnings
	Error     string // Error messages
	Info      string // Information, questions

	// Border colors
	Border      string // Default borders
	BorderFocus string // Focused element borders (defaults to Primary if empty)

	// Diff colors (for viewing changes)
	DiffAdded   string // Added lines
	DiffRemoved string // Removed lines
	DiffHeader  string // Diff headers
	DiffHunk    string // Hunk markers

	// Markdown colors
	MarkdownH1       string // H1 headers
	MarkdownH2       string // H2 headers
	MarkdownH3       string // H3 headers
	MarkdownCode     string // Inline code
	MarkdownCodeBg   string // Code background
	MarkdownLink     string // Links
	MarkdownListItem string // List bullets
}

// GetBgSelected returns the selected background color, defaulting to Primary
func (t Theme) GetBgSelected() string {
	if t.BgSelected != "" {
		return t.BgSelected
	}
	return t.Primary
}

// GetBorderFocus returns the focused border color, defaulting to Primary
func (t Theme) GetBorderFocus() string {
	if t.BorderFocus != "" {
		return t.BorderFocus
	}
	return t.Primary
}

// ThemeName is a type for theme identifiers
type ThemeName string

// Available theme names
const (
	ThemeDarkPurple    ThemeName = "dark-purple"
	ThemeNord          ThemeName = "nord"
	ThemeDracula       ThemeName = "dracula"
	ThemeGruvbox       ThemeName = "gruvbox"
	ThemeTokyoNight    ThemeName = "tokyo-night"
	ThemeCatppuccin    ThemeName = "catppuccin"
	ThemeScienceFiction ThemeName = "science-fiction"
	ThemeLight         ThemeName = "light"
)

// DefaultTheme is the default theme name
const DefaultTheme = ThemeDarkPurple

// BuiltinThemes contains all built-in themes
var BuiltinThemes = map[ThemeName]Theme{
	ThemeDarkPurple: {
		Name:             "Dark Purple",
		Primary:          "#7C3AED",
		Secondary:        "#06B6D4",
		Bg:               "#1F2937",
		Text:             "#F9FAFB",
		TextMuted:        "#9CA3AF",
		TextInverse:      "#1F2937",
		User:             "#A78BFA",
		Assistant:        "#22D3EE",
		Warning:          "#F59E0B",
		Error:            "#EF4444",
		Info:             "#06B6D4",
		Border:           "#374151",
		DiffAdded:        "#4ADE80",
		DiffRemoved:      "#F87171",
		DiffHeader:       "#60A5FA",
		DiffHunk:         "#C084FC",
		MarkdownH1:       "#A78BFA",
		MarkdownH2:       "#C4B5FD",
		MarkdownH3:       "#22D3EE",
		MarkdownCode:     "#67E8F9",
		MarkdownCodeBg:   "#1E1E2E",
		MarkdownLink:     "#67E8F9",
		MarkdownListItem: "#06B6D4",
	},
	ThemeNord: {
		Name:             "Nord",
		Primary:          "#88C0D0",
		Secondary:        "#81A1C1",
		Bg:               "#2E3440",
		Text:             "#ECEFF4",
		TextMuted:        "#D8DEE9",
		TextInverse:      "#2E3440",
		User:             "#A3BE8C",
		Assistant:        "#88C0D0",
		Warning:          "#EBCB8B",
		Error:            "#BF616A",
		Info:             "#81A1C1",
		Border:           "#4C566A",
		DiffAdded:        "#A3BE8C",
		DiffRemoved:      "#BF616A",
		DiffHeader:       "#81A1C1",
		DiffHunk:         "#B48EAD",
		MarkdownH1:       "#88C0D0",
		MarkdownH2:       "#81A1C1",
		MarkdownH3:       "#5E81AC",
		MarkdownCode:     "#A3BE8C",
		MarkdownCodeBg:   "#242933",
		MarkdownLink:     "#88C0D0",
		MarkdownListItem: "#81A1C1",
	},
	ThemeDracula: {
		Name:             "Dracula",
		Primary:          "#BD93F9",
		Secondary:        "#8BE9FD",
		Bg:               "#282A36",
		Text:             "#F8F8F2",
		TextMuted:        "#6272A4",
		TextInverse:      "#282A36",
		User:             "#FF79C6",
		Assistant:        "#8BE9FD",
		Warning:          "#FFB86C",
		Error:            "#FF5555",
		Info:             "#8BE9FD",
		Border:           "#44475A",
		DiffAdded:        "#50FA7B",
		DiffRemoved:      "#FF5555",
		DiffHeader:       "#8BE9FD",
		DiffHunk:         "#BD93F9",
		MarkdownH1:       "#BD93F9",
		MarkdownH2:       "#FF79C6",
		MarkdownH3:       "#8BE9FD",
		MarkdownCode:     "#50FA7B",
		MarkdownCodeBg:   "#21222C",
		MarkdownLink:     "#8BE9FD",
		MarkdownListItem: "#BD93F9",
	},
	ThemeGruvbox: {
		Name:             "Gruvbox Dark",
		Primary:          "#FE8019",
		Secondary:        "#83A598",
		Bg:               "#282828",
		Text:             "#EBDBB2",
		TextMuted:        "#A89984",
		TextInverse:      "#282828",
		User:             "#FABD2F",
		Assistant:        "#83A598",
		Warning:          "#FE8019",
		Error:            "#FB4934",
		Info:             "#83A598",
		Border:           "#504945",
		DiffAdded:        "#B8BB26",
		DiffRemoved:      "#FB4934",
		DiffHeader:       "#83A598",
		DiffHunk:         "#D3869B",
		MarkdownH1:       "#FE8019",
		MarkdownH2:       "#FABD2F",
		MarkdownH3:       "#83A598",
		MarkdownCode:     "#B8BB26",
		MarkdownCodeBg:   "#1D2021",
		MarkdownLink:     "#83A598",
		MarkdownListItem: "#FE8019",
	},
	ThemeTokyoNight: {
		Name:             "Tokyo Night",
		Primary:          "#7AA2F7",
		Secondary:        "#BB9AF7",
		Bg:               "#1A1B26",
		Text:             "#C0CAF5",
		TextMuted:        "#565F89",
		TextInverse:      "#1A1B26",
		User:             "#9ECE6A",
		Assistant:        "#7AA2F7",
		Warning:          "#E0AF68",
		Error:            "#F7768E",
		Info:             "#7DCFFF",
		Border:           "#3B4261",
		DiffAdded:        "#9ECE6A",
		DiffRemoved:      "#F7768E",
		DiffHeader:       "#7AA2F7",
		DiffHunk:         "#BB9AF7",
		MarkdownH1:       "#7AA2F7",
		MarkdownH2:       "#BB9AF7",
		MarkdownH3:       "#7DCFFF",
		MarkdownCode:     "#9ECE6A",
		MarkdownCodeBg:   "#16161E",
		MarkdownLink:     "#7DCFFF",
		MarkdownListItem: "#BB9AF7",
	},
	ThemeCatppuccin: {
		Name:             "Catppuccin Mocha",
		Primary:          "#CBA6F7",
		Secondary:        "#89DCEB",
		Bg:               "#1E1E2E",
		Text:             "#CDD6F4",
		TextMuted:        "#6C7086",
		TextInverse:      "#1E1E2E",
		User:             "#F5C2E7",
		Assistant:        "#89DCEB",
		Warning:          "#FAB387",
		Error:            "#F38BA8",
		Info:             "#89DCEB",
		Border:           "#313244",
		DiffAdded:        "#A6E3A1",
		DiffRemoved:      "#F38BA8",
		DiffHeader:       "#89DCEB",
		DiffHunk:         "#CBA6F7",
		MarkdownH1:       "#CBA6F7",
		MarkdownH2:       "#F5C2E7",
		MarkdownH3:       "#89DCEB",
		MarkdownCode:     "#A6E3A1",
		MarkdownCodeBg:   "#181825",
		MarkdownLink:     "#89DCEB",
		MarkdownListItem: "#CBA6F7",
	},
	ThemeScienceFiction: {
		Name:             "Science Fiction",
		Primary:          "#E50914",
		Secondary:        "#8B0000",
		Bg:               "#0A0A0A",
		BgSelected:       "#2D0A0A",
		Text:             "#E8E8E8",
		TextMuted:        "#666666",
		TextInverse:      "#0A0A0A",
		User:             "#FF4444",
		Assistant:        "#CC0000",
		Warning:          "#FF6600",
		Error:            "#FF0000",
		Info:             "#AA0000",
		Border:           "#330000",
		BorderFocus:      "#E50914",
		DiffAdded:        "#00AA00",
		DiffRemoved:      "#FF4444",
		DiffHeader:       "#E50914",
		DiffHunk:         "#8B0000",
		MarkdownH1:       "#E50914",
		MarkdownH2:       "#CC0000",
		MarkdownH3:       "#AA0000",
		MarkdownCode:     "#FF6666",
		MarkdownCodeBg:   "#1A0000",
		MarkdownLink:     "#FF4444",
		MarkdownListItem: "#E50914",
	},
	ThemeLight: {
		Name:             "Light",
		Primary:          "#6366F1",
		Secondary:        "#0891B2",
		Bg:               "#FFFFFF",
		BgSelected:       "#E0E7FF",
		Text:             "#1F2937",
		TextMuted:        "#6B7280",
		TextInverse:      "#FFFFFF",
		User:             "#7C3AED",
		Assistant:        "#0891B2",
		Warning:          "#D97706",
		Error:            "#DC2626",
		Info:             "#0891B2",
		Border:           "#D1D5DB",
		BorderFocus:      "#6366F1",
		DiffAdded:        "#16A34A",
		DiffRemoved:      "#DC2626",
		DiffHeader:       "#2563EB",
		DiffHunk:         "#7C3AED",
		MarkdownH1:       "#6366F1",
		MarkdownH2:       "#7C3AED",
		MarkdownH3:       "#0891B2",
		MarkdownCode:     "#059669",
		MarkdownCodeBg:   "#F3F4F6",
		MarkdownLink:     "#0891B2",
		MarkdownListItem: "#6366F1",
	},
}

// ThemeNames returns a list of all available theme names in display order
func ThemeNames() []ThemeName {
	return []ThemeName{
		ThemeDarkPurple,
		ThemeNord,
		ThemeDracula,
		ThemeGruvbox,
		ThemeTokyoNight,
		ThemeCatppuccin,
		ThemeScienceFiction,
		ThemeLight,
	}
}

// GetTheme returns a theme by name, defaulting to DarkPurple if not found
func GetTheme(name ThemeName) Theme {
	if theme, ok := BuiltinThemes[name]; ok {
		return theme
	}
	return BuiltinThemes[DefaultTheme]
}

// currentTheme holds the active theme
var currentTheme = BuiltinThemes[DefaultTheme]

// CurrentTheme returns the currently active theme
func CurrentTheme() Theme {
	return currentTheme
}

// SetTheme sets the active theme and regenerates all styles
func SetTheme(name ThemeName) {
	currentTheme = GetTheme(name)
	regenerateStyles()
	RefreshModalStyles()
}

// SetThemeByName sets the active theme by string name
func SetThemeByName(name string) {
	SetTheme(ThemeName(name))
}

// CurrentThemeName returns the name of the current theme
func CurrentThemeName() ThemeName {
	for name, theme := range BuiltinThemes {
		if theme.Name == currentTheme.Name {
			return name
		}
	}
	return DefaultTheme
}

// regenerateStyles updates all style variables based on the current theme
func regenerateStyles() {
	t := currentTheme

	// Update color variables
	ColorPrimary = lipgloss.Color(t.Primary)
	ColorSecondary = lipgloss.Color(t.Secondary)
	ColorMuted = lipgloss.Color(t.TextMuted)
	ColorBorder = lipgloss.Color(t.Border)
	ColorBorderFocus = lipgloss.Color(t.GetBorderFocus())
	ColorBg = lipgloss.Color(t.Bg)
	ColorText = lipgloss.Color(t.Text)
	ColorTextMuted = lipgloss.Color(t.TextMuted)
	ColorTextInverse = lipgloss.Color(t.TextInverse)
	ColorUser = lipgloss.Color(t.User)
	ColorAssistant = lipgloss.Color(t.Assistant)
	ColorWarning = lipgloss.Color(t.Warning)
	ColorInfo = lipgloss.Color(t.Info)
	ColorError = lipgloss.Color(t.Error)

	// Update header styles
	HeaderStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorText).
		Background(ColorPrimary).
		Padding(0, 1)

	HeaderTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorText)

	// Update footer styles
	FooterStyle = lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Padding(0, 1)

	FooterKeyStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorSecondary)

	FooterDescStyle = lipgloss.NewStyle().
		Foreground(ColorTextMuted)

	// Update panel styles
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

	// Update sidebar styles
	SidebarItemStyle = lipgloss.NewStyle().
		Padding(0, 1)

	SidebarSelectedStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(t.GetBgSelected())).
		Foreground(lipgloss.Color(t.Text)).
		Bold(true).
		Padding(0, 1)

	SidebarRepoStyle = lipgloss.NewStyle().
		Foreground(ColorMuted).
		Italic(true)

	// Update chat styles
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

	// Update modal styles
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

	// Update status styles
	StatusLoadingStyle = lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Italic(true)

	StatusErrorStyle = lipgloss.NewStyle().
		Foreground(ColorError).
		Bold(true)

	// Update tool use marker styles
	ToolUseInProgressStyle = lipgloss.NewStyle().
		Foreground(ColorText)

	ToolUseCompleteStyle = lipgloss.NewStyle().
		Foreground(ColorSecondary)

	// Update permission prompt styles
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

	// Update question prompt styles
	QuestionBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorInfo).
		Padding(0, 1)

	// Update markdown styles
	MarkdownH1Style = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(t.MarkdownH1)).
		MarginTop(1)

	MarkdownH2Style = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(t.MarkdownH2)).
		MarginTop(1)

	MarkdownH3Style = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(t.MarkdownH3))

	MarkdownH4Style = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorTextMuted)

	MarkdownBoldStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorText)

	MarkdownItalicStyle = lipgloss.NewStyle().
		Italic(true).
		Foreground(ColorText)

	MarkdownInlineCodeStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.MarkdownCode)).
		Background(lipgloss.Color(t.MarkdownCodeBg))

	MarkdownCodeBlockStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(t.MarkdownCodeBg))

	MarkdownListBulletStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.MarkdownListItem))

	MarkdownBlockquoteStyle = lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Italic(true).
		BorderLeft(true).
		BorderStyle(lipgloss.ThickBorder()).
		BorderForeground(ColorMuted).
		PaddingLeft(1)

	MarkdownHRStyle = lipgloss.NewStyle().
		Foreground(ColorBorder)

	MarkdownLinkStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.MarkdownLink)).
		Underline(true)

	// Update diff styles
	DiffAddedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.DiffAdded))

	DiffRemovedStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.DiffRemoved))

	DiffHeaderStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.DiffHeader)).
		Bold(true)

	DiffHunkStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.DiffHunk))

	// Update view changes styles
	ViewChangesSelectedStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(t.GetBgSelected())).
		Foreground(lipgloss.Color(t.Text))
}
