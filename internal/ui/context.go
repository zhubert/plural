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
	HeaderHeight  int
	FooterHeight  int
	ContentHeight int
	SidebarWidth  int
	ChatWidth     int

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
		logger.WithComponent("ui").Debug("ViewContext initialized")
	})
	return ctx
}

// Log writes a debug message to the log file using slog structured logging.
// For new code, prefer using logger.WithComponent("ui").Debug() directly.
func (v *ViewContext) Log(msg string, args ...interface{}) {
	logger.WithComponent("ui").Debug(msg, args...)
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

	// Sidebar is 1/5 of width, chat gets the rest
	v.SidebarWidth = width / SidebarWidthRatio
	v.ChatWidth = width - v.SidebarWidth

	log := logger.WithComponent("ui")
	log.Debug("Terminal size updated",
		"width", width,
		"height", height,
		"headerHeight", v.HeaderHeight,
		"footerHeight", v.FooterHeight,
		"contentHeight", v.ContentHeight,
		"sidebarWidth", v.SidebarWidth,
		"chatWidth", v.ChatWidth,
	)
}

// InnerWidth returns the usable width inside a panel with borders
func (v *ViewContext) InnerWidth(panelWidth int) int {
	return panelWidth - BorderSize
}

// InnerHeight returns the usable height inside a panel with borders
func (v *ViewContext) InnerHeight(panelHeight int) int {
	return panelHeight - BorderSize
}
