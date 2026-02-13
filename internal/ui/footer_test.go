package ui

import (
	"strings"
	"testing"
	"time"
)

func TestNewFooter(t *testing.T) {
	footer := NewFooter()

	if footer == nil {
		t.Fatal("NewFooter() returned nil")
	}

	if len(footer.bindings) == 0 {
		t.Error("Expected default bindings to be set")
	}

	if footer.flashMessage != nil {
		t.Error("Expected no flash message initially")
	}
}

func TestFooter_SetWidth(t *testing.T) {
	footer := NewFooter()

	footer.SetWidth(120)

	if footer.width != 120 {
		t.Errorf("Expected width 120, got %d", footer.width)
	}
}

func TestFooter_SetFlash(t *testing.T) {
	footer := NewFooter()

	footer.SetFlash("Test error message", FlashError)

	if footer.flashMessage == nil {
		t.Fatal("Expected flash message to be set")
	}

	if footer.flashMessage.Text != "Test error message" {
		t.Errorf("Expected text 'Test error message', got %q", footer.flashMessage.Text)
	}

	if footer.flashMessage.Type != FlashError {
		t.Errorf("Expected type FlashError, got %v", footer.flashMessage.Type)
	}

	if footer.flashMessage.Duration != DefaultFlashDuration {
		t.Errorf("Expected duration %v, got %v", DefaultFlashDuration, footer.flashMessage.Duration)
	}
}

func TestFooter_SetFlashWithDuration(t *testing.T) {
	footer := NewFooter()
	customDuration := 10 * time.Second

	footer.SetFlashWithDuration("Custom duration", FlashInfo, customDuration)

	if footer.flashMessage == nil {
		t.Fatal("Expected flash message to be set")
	}

	if footer.flashMessage.Duration != customDuration {
		t.Errorf("Expected duration %v, got %v", customDuration, footer.flashMessage.Duration)
	}
}

func TestFooter_ClearFlash(t *testing.T) {
	footer := NewFooter()

	footer.SetFlash("Test message", FlashInfo)
	if !footer.HasFlash() {
		t.Error("Expected HasFlash() to return true")
	}

	footer.ClearFlash()
	if footer.HasFlash() {
		t.Error("Expected HasFlash() to return false after ClearFlash()")
	}
}

func TestFooter_HasFlash(t *testing.T) {
	footer := NewFooter()

	if footer.HasFlash() {
		t.Error("Expected HasFlash() to return false initially")
	}

	footer.SetFlash("Test", FlashInfo)

	if !footer.HasFlash() {
		t.Error("Expected HasFlash() to return true after SetFlash")
	}
}

func TestFlashMessage_IsExpired(t *testing.T) {
	// Test non-expired message
	msg := &FlashMessage{
		Text:      "Test",
		Type:      FlashInfo,
		CreatedAt: time.Now(),
		Duration:  5 * time.Second,
	}

	if msg.IsExpired() {
		t.Error("New message should not be expired")
	}

	// Test expired message
	expiredMsg := &FlashMessage{
		Text:      "Test",
		Type:      FlashInfo,
		CreatedAt: time.Now().Add(-10 * time.Second),
		Duration:  5 * time.Second,
	}

	if !expiredMsg.IsExpired() {
		t.Error("Old message should be expired")
	}
}

func TestFooter_ClearIfExpired(t *testing.T) {
	footer := NewFooter()

	// Set a message that's not expired
	footer.SetFlash("Not expired", FlashInfo)

	if footer.ClearIfExpired() {
		t.Error("Should not clear non-expired message")
	}

	if !footer.HasFlash() {
		t.Error("Flash should still be present")
	}

	// Set an expired message
	footer.flashMessage = &FlashMessage{
		Text:      "Expired",
		Type:      FlashInfo,
		CreatedAt: time.Now().Add(-10 * time.Second),
		Duration:  5 * time.Second,
	}

	if !footer.ClearIfExpired() {
		t.Error("Should clear expired message")
	}

	if footer.HasFlash() {
		t.Error("Flash should be cleared")
	}
}

func TestFooter_View_WithFlash(t *testing.T) {
	footer := NewFooter()
	footer.SetWidth(80)

	// Without flash, should show keybindings
	viewWithoutFlash := footer.View()
	if strings.Contains(viewWithoutFlash, "Test error") {
		t.Error("Should not contain flash message text when no flash is set")
	}

	// With flash, should show flash message instead of keybindings
	footer.SetFlash("Test error message", FlashError)
	viewWithFlash := footer.View()

	if !strings.Contains(viewWithFlash, "Test error message") {
		t.Error("Flash message should be visible in view")
	}

	// Should contain error icon
	if !strings.Contains(viewWithFlash, "✕") {
		t.Error("Error flash should contain error icon")
	}
}

func TestFooter_FlashTypes(t *testing.T) {
	tests := []struct {
		name         string
		flashType    FlashType
		expectedIcon string
	}{
		{"Error", FlashError, "✕"},
		{"Warning", FlashWarning, "⚠"},
		{"Info", FlashInfo, "ℹ"},
		{"Success", FlashSuccess, "✓"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			footer := NewFooter()
			footer.SetWidth(80)
			footer.SetFlash("Test message", tt.flashType)

			view := footer.View()
			if !strings.Contains(view, tt.expectedIcon) {
				t.Errorf("Expected %s flash to contain icon %q", tt.name, tt.expectedIcon)
			}
		})
	}
}

func TestFlashTick(t *testing.T) {
	cmd := FlashTick()

	if cmd == nil {
		t.Error("FlashTick() should return a command")
	}
}

func TestFooter_MultiSelectMode(t *testing.T) {
	footer := NewFooter()
	footer.SetWidth(120)

	// Default view should not show multi-select bindings
	footer.SetContext(true, true, false, false, false, false, false, false, false, false)
	defaultView := footer.View()
	if strings.Contains(defaultView, "toggle") {
		t.Error("Default view should not contain multi-select 'toggle' binding")
	}
	if strings.Contains(defaultView, "select all") {
		t.Error("Default view should not contain 'select all' binding")
	}

	// Multi-select mode should show multi-select-specific bindings
	footer.SetContext(true, true, false, false, false, false, false, true, false, false)
	multiSelectView := footer.View()

	expectedBindings := []string{"toggle", "select all", "deselect all", "bulk action", "navigate", "exit", "help"}
	for _, binding := range expectedBindings {
		if !strings.Contains(multiSelectView, binding) {
			t.Errorf("Multi-select view should contain %q binding", binding)
		}
	}

	// Multi-select bindings should NOT show standard sidebar bindings
	standardBindings := []string{"new session", "add repo", "merge/pr", "fork", "delete", "quit"}
	for _, binding := range standardBindings {
		if strings.Contains(multiSelectView, binding) {
			t.Errorf("Multi-select view should NOT contain standard binding %q", binding)
		}
	}
}

func TestFooter_MultiSelectMode_FlashTakesPriority(t *testing.T) {
	footer := NewFooter()
	footer.SetWidth(120)

	// Flash message should take priority over multi-select bindings
	footer.SetContext(true, true, false, false, false, false, false, true, false, false)
	footer.SetFlash("Error occurred", FlashError)

	view := footer.View()
	if !strings.Contains(view, "Error occurred") {
		t.Error("Flash message should take priority over multi-select bindings")
	}
	if strings.Contains(view, "toggle") {
		t.Error("Multi-select bindings should not show when flash is active")
	}
}

func TestFooter_NewlineShortcutDisplay(t *testing.T) {
	footer := NewFooter()
	footer.SetWidth(120)

	// Without kitty keyboard, should show opt+enter
	footer.SetContext(true, false, false, false, false, false, false, false, false, false)
	view := footer.View()
	if !strings.Contains(view, "opt+enter") {
		t.Error("Without kitty keyboard, should show opt+enter")
	}
	if strings.Contains(view, "shift+enter") {
		t.Error("Without kitty keyboard, should not show shift+enter")
	}

	// With kitty keyboard, should show shift+enter
	footer.SetContext(true, false, false, false, false, false, false, false, false, true)
	view = footer.View()
	if !strings.Contains(view, "shift+enter") {
		t.Error("With kitty keyboard, should show shift+enter")
	}
	if strings.Contains(view, "opt+enter") {
		t.Error("With kitty keyboard, should not show opt+enter")
	}
}
