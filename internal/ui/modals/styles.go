package modals

import (
	"image/color"

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
)

// SetStyles sets the style variables from the parent ui package.
// This must be called before rendering any modals.
func SetStyles(
	modalTitle, modalHelp, sidebarItem, sidebarSelected, statusError lipgloss.Style,
	primary, secondary, text, textMuted, textInverse, user, warning color.Color,
	inputWidth, inputCharLimit, modalWidth int,
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
}
