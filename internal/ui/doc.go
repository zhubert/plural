// Package ui provides the user interface components for the Plural TUI.
//
// # Overview
//
// The ui package implements the visual components of Plural using the Bubble Tea
// framework and Lipgloss styling library. It follows the Model-Update-View pattern
// established by Bubble Tea.
//
// # Layout System
//
// The layout is organized as follows:
//
//	┌─────────────────────────────────────────────────────┐
//	│ Header (1 line)                                     │
//	├─────────────────┬───────────────────────────────────┤
//	│                 │                                   │
//	│   Sidebar       │         Chat Panel                │
//	│   (1/3 width)   │         (2/3 width)               │
//	│                 │                                   │
//	├─────────────────┴───────────────────────────────────┤
//	│ Footer (1 line)                                     │
//	└─────────────────────────────────────────────────────┘
//
// # Components
//
// ViewContext: Singleton that manages centralized layout calculations.
// All size calculations should go through ViewContext to ensure consistency.
//
// Header: Displays the application title and optionally the current session name.
// Uses a gradient background with the primary color.
//
// Footer: Shows context-aware keyboard shortcuts. The displayed shortcuts
// change based on focus state and whether a session is selected.
//
// Sidebar: Lists all sessions grouped by repository. Supports selection
// with keyboard navigation (j/k or arrow keys).
//
// Chat: The main conversation panel showing message history and input.
// Includes a viewport for scrolling through messages and a textarea for input.
//
// Modal: Popup dialogs for various operations:
//   - ModalAddRepo: Add a new repository
//   - ModalNewSession: Create a new session
//   - ModalConfirmDelete: Confirm session deletion
//   - ModalMerge: Merge/PR options
//
// # Focus System
//
// The application has two focus states:
//   - FocusSidebar: Session list is focused, keyboard controls navigation
//   - FocusChat: Chat panel is focused, keyboard input goes to textarea
//
// Tab key toggles between focus states. The 'q' key only quits when
// the sidebar is focused (to allow typing 'q' in chat).
//
// # Constants
//
// Layout constants are defined in constants.go:
//   - HeaderHeight, FooterHeight: Fixed at 1 line each
//   - BorderSize: 2 (1 on each side)
//   - SidebarWidthRatio: 3 (sidebar gets 1/3 of width)
//   - TextareaHeight: 3 lines for input
//   - MaxSessionMessageLines: 100 lines kept in history
//
// # Styles
//
// All styles are defined in styles.go using Lipgloss. The color palette uses:
//   - ColorPrimary (#7C3AED): Purple, used for highlights and focused elements
//   - ColorSecondary (#10B981): Green, used for assistant messages
//   - ColorBg (#1F2937): Dark background
//   - ColorText (#F9FAFB): Light text
//   - ColorTextMuted (#9CA3AF): Muted text for secondary content
package ui
