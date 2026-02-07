package keys

import "testing"

// TestKeyStringValues verifies that all key constants produce the expected
// string representations. This acts as a safety net if Bubble Tea ever changes
// its key string format.
func TestKeyStringValues(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		// Navigation
		{"Up", Up, "up"},
		{"Down", Down, "down"},
		{"Left", Left, "left"},
		{"Right", Right, "right"},
		{"Home", Home, "home"},
		{"End", End, "end"},
		{"PgUp", PgUp, "pgup"},
		{"PgDown", PgDown, "pgdown"},

		// Actions
		{"Enter", Enter, "enter"},
		{"Tab", Tab, "tab"},
		{"ShiftTab", ShiftTab, "shift+tab"},
		{"Space", Space, "space"},
		{"Backspace", Backspace, "backspace"},
		{"Delete", Delete, "delete"},
		{"Escape", Escape, "esc"},

		// Ctrl combos
		{"CtrlC", CtrlC, "ctrl+c"},
		{"CtrlV", CtrlV, "ctrl+v"},
		{"CtrlS", CtrlS, "ctrl+s"},
		{"CtrlO", CtrlO, "ctrl+o"},
		{"CtrlU", CtrlU, "ctrl+u"},
		{"CtrlD", CtrlD, "ctrl+d"},
		{"CtrlT", CtrlT, "ctrl+t"},
		{"CtrlL", CtrlL, "ctrl+l"},
		{"CtrlB", CtrlB, "ctrl+b"},
		{"CtrlN", CtrlN, "ctrl+n"},
		{"CtrlP", CtrlP, "ctrl+p"},
		{"CtrlE", CtrlE, "ctrl+e"},
		{"CtrlSlash", CtrlSlash, "ctrl+/"},
		{"CtrlShiftB", CtrlShiftB, "ctrl+shift+b"},
		{"CtrlUp", CtrlUp, "ctrl+up"},
		{"CtrlDown", CtrlDown, "ctrl+down"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("keys.%s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}
