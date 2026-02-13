// Package keys provides string constants for Bubble Tea v2 key press events.
//
// These constants are derived from tea.KeyPressMsg{Code: tea.KeyXxx}.String()
// and are guaranteed to match the actual runtime values. Using these constants
// instead of hardcoded strings prevents typo bugs (e.g., "escape" vs "esc").
//
// Single-character keys like "a", "y", "?" are not included here because they
// are unambiguous and cannot be misspelled in a meaningful way.
package keys

import tea "charm.land/bubbletea/v2"

// Navigation keys
var (
	Up     = tea.KeyPressMsg{Code: tea.KeyUp}.String()     // "up"
	Down   = tea.KeyPressMsg{Code: tea.KeyDown}.String()   // "down"
	Left   = tea.KeyPressMsg{Code: tea.KeyLeft}.String()   // "left"
	Right  = tea.KeyPressMsg{Code: tea.KeyRight}.String()  // "right"
	Home   = tea.KeyPressMsg{Code: tea.KeyHome}.String()   // "home"
	End    = tea.KeyPressMsg{Code: tea.KeyEnd}.String()    // "end"
	PgUp   = tea.KeyPressMsg{Code: tea.KeyPgUp}.String()   // "pgup"
	PgDown = tea.KeyPressMsg{Code: tea.KeyPgDown}.String() // "pgdown"
)

// Action keys
var (
	Enter      = tea.KeyPressMsg{Code: tea.KeyEnter}.String()                      // "enter"
	ShiftEnter = (tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}).String() // "shift+enter"
	AltEnter   = (tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt}).String()   // "alt+enter"
	Tab        = tea.KeyPressMsg{Code: tea.KeyTab}.String()                        // "tab"
	ShiftTab   = (tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}).String()   // "shift+tab"
	Space      = tea.KeyPressMsg{Code: tea.KeySpace}.String()                      // "space"
	Backspace  = tea.KeyPressMsg{Code: tea.KeyBackspace}.String()                  // "backspace"
	Delete     = tea.KeyPressMsg{Code: tea.KeyDelete}.String()                     // "delete"
	Escape     = tea.KeyPressMsg{Code: tea.KeyEscape}.String()                     // "esc"
)

// Ctrl combinations
var (
	CtrlC      = (tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}).String()                // "ctrl+c"
	CtrlV      = (tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl}).String()                // "ctrl+v"
	CtrlS      = (tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}).String()                // "ctrl+s"
	CtrlO      = (tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl}).String()                // "ctrl+o"
	CtrlU      = (tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}).String()                // "ctrl+u"
	CtrlD      = (tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}).String()                // "ctrl+d"
	CtrlT      = (tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl}).String()                // "ctrl+t"
	CtrlL      = (tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl}).String()                // "ctrl+l"
	CtrlB      = (tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl}).String()                // "ctrl+b"
	CtrlN      = (tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}).String()                // "ctrl+n"
	CtrlP      = (tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}).String()                // "ctrl+p"
	CtrlE      = (tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl}).String()                // "ctrl+e"
	CtrlR      = (tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}).String()                // "ctrl+r"
	CtrlSlash  = (tea.KeyPressMsg{Code: '/', Mod: tea.ModCtrl}).String()                // "ctrl+/"
	CtrlShiftB = (tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl | tea.ModShift}).String() // "ctrl+shift+b"
	CtrlUp     = (tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl}).String()          // "ctrl+up"
	CtrlDown   = (tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl}).String()        // "ctrl+down"
)
