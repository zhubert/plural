package ui

import (
	"sync"

	"github.com/zhubert/plural/internal/logger"
)

// ViewContext holds centralized layout calculations and provides debug logging.
// All size calculations should go through this to avoid duplication.
type ViewContext struct {
	// Terminal dimensions
	TerminalWidth  int
	TerminalHeight int

	// Calculated dimensions
	HeaderHeight   int
	FooterHeight   int
	ContentHeight  int
	SidebarWidth   int
	ChatWidth      int

	mu sync.Mutex
}

// Global view context instance
var ctx *ViewContext
var ctxOnce sync.Once

// GetViewContext returns the singleton ViewContext instance
func GetViewContext() *ViewContext {
	ctxOnce.Do(func() {
		ctx = &ViewContext{
			HeaderHeight: HeaderHeight,
			FooterHeight: FooterHeight,
		}
		ctx.Log("ViewContext initialized")
	})
	return ctx
}

// Log writes a debug message to the log file
func (v *ViewContext) Log(format string, args ...interface{}) {
	logger.Log(format, args...)
}

// UpdateTerminalSize recalculates all dimensions when terminal size changes.
// This method is thread-safe and should be called from the main event loop
// when the terminal is resized.
func (v *ViewContext) UpdateTerminalSize(width, height int) {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Validate dimensions to prevent negative layout values
	if width < MinTerminalWidth {
		width = MinTerminalWidth
	}
	if height < MinTerminalHeight {
		height = MinTerminalHeight
	}

	v.TerminalWidth = width
	v.TerminalHeight = height

	// Header and footer each take exactly 1 line of content
	// The styles add padding but lipgloss Width() handles the total
	v.HeaderHeight = HeaderHeight
	v.FooterHeight = FooterHeight

	// Content area is everything between header and footer
	v.ContentHeight = height - v.HeaderHeight - v.FooterHeight

	// Sidebar is 1/3 of width, chat gets the rest
	v.SidebarWidth = width / SidebarWidthRatio
	v.ChatWidth = width - v.SidebarWidth

	v.Log("Terminal size updated: %dx%d", width, height)
	v.Log("  HeaderHeight: %d", v.HeaderHeight)
	v.Log("  FooterHeight: %d", v.FooterHeight)
	v.Log("  ContentHeight: %d (terminal %d - header %d - footer %d)",
		v.ContentHeight, height, v.HeaderHeight, v.FooterHeight)
	v.Log("  SidebarWidth: %d", v.SidebarWidth)
	v.Log("  ChatWidth: %d", v.ChatWidth)
}

// InnerWidth returns the usable width inside a panel with borders
func (v *ViewContext) InnerWidth(panelWidth int) int {
	return panelWidth - BorderSize
}

// InnerHeight returns the usable height inside a panel with borders
func (v *ViewContext) InnerHeight(panelHeight int) int {
	return panelHeight - BorderSize
}
