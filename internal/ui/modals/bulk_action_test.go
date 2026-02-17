package modals

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestNewBulkActionState(t *testing.T) {
	ids := []string{"s1", "s2", "s3"}

	state := NewBulkActionState(ids)

	if state.SessionCount != 3 {
		t.Errorf("expected session count 3, got %d", state.SessionCount)
	}
	if len(state.SessionIDs) != 3 {
		t.Errorf("expected 3 session IDs, got %d", len(state.SessionIDs))
	}
	if state.Action != BulkActionDelete {
		t.Errorf("expected default action to be Delete, got %d", state.Action)
	}

	// Check textarea has line numbers disabled
	if state.PromptInput.ShowLineNumbers {
		t.Error("expected ShowLineNumbers to be false")
	}
}

func TestBulkActionState_SwitchAction(t *testing.T) {
	state := NewBulkActionState([]string{"s1"})

	// Start at Delete
	if state.Action != BulkActionDelete {
		t.Fatal("should start at Delete")
	}

	// Switch right to Create PRs
	state.Update(tea.KeyPressMsg{Code: -1, Text: "l"})
	if state.Action != BulkActionCreatePRs {
		t.Errorf("expected CreatePRs, got %d", state.Action)
	}

	// Switch right to Send Prompt
	state.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if state.Action != BulkActionSendPrompt {
		t.Errorf("expected SendPrompt, got %d", state.Action)
	}

	// On SendPrompt, tab wraps to beginning
	state.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if state.Action != BulkActionDelete {
		t.Errorf("tab should wrap to Delete, got %d", state.Action)
	}

	// Navigate forward to SendPrompt again
	state.Action = BulkActionSendPrompt

	// Switch back left to CreatePRs (use shift+tab when on SendPrompt)
	state.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if state.Action != BulkActionCreatePRs {
		t.Errorf("expected CreatePRs, got %d", state.Action)
	}

	// Switch back left to Delete
	state.Update(tea.KeyPressMsg{Code: -1, Text: "h"})
	if state.Action != BulkActionDelete {
		t.Errorf("expected Delete, got %d", state.Action)
	}

	// Can't go further left
	state.Update(tea.KeyPressMsg{Code: -1, Text: "h"})
	if state.Action != BulkActionDelete {
		t.Errorf("should stay at Delete, got %d", state.Action)
	}
}





func TestBulkActionState_Render_Delete(t *testing.T) {
	state := NewBulkActionState([]string{"s1", "s2"})
	rendered := state.Render()

	if !strings.Contains(rendered, "Bulk Action (2 sessions)") {
		t.Error("should contain title with count")
	}
	if !strings.Contains(rendered, "Delete") {
		t.Error("should contain Delete action")
	}
	if !strings.Contains(rendered, "delete 2 session(s)") {
		t.Errorf("should contain delete confirmation message, got:\n%s", rendered)
	}
}



func TestBulkActionState_Render_CreatePRs(t *testing.T) {
	state := NewBulkActionState([]string{"s1", "s2", "s3"})
	state.Action = BulkActionCreatePRs

	rendered := state.Render()

	if !strings.Contains(rendered, "Create PRs") {
		t.Error("should contain 'Create PRs' action")
	}
	if !strings.Contains(rendered, "Create PRs for 3 session(s)") {
		t.Error("should show PR creation confirmation message")
	}
	if !strings.Contains(rendered, "skipped") {
		t.Error("should mention that sessions will be skipped")
	}
}

func TestBulkActionState_SwitchToSendPrompt(t *testing.T) {
	state := NewBulkActionState([]string{"s1"})

	// Navigate right to SendPrompt (Delete -> Move -> CreatePRs -> SendPrompt)
	state.Update(tea.KeyPressMsg{Code: -1, Text: "l"})
	state.Update(tea.KeyPressMsg{Code: -1, Text: "l"})
	state.Update(tea.KeyPressMsg{Code: -1, Text: "l"})

	if state.Action != BulkActionSendPrompt {
		t.Errorf("expected BulkActionSendPrompt, got %d", state.Action)
	}

	// When on SendPrompt, tab wraps to beginning (not boundary clamping)
	state.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if state.Action != BulkActionDelete {
		t.Errorf("tab should wrap to Delete, got %d", state.Action)
	}
}

func TestBulkActionState_Render_SendPrompt(t *testing.T) {
	state := NewBulkActionState([]string{"s1", "s2"})
	state.Action = BulkActionSendPrompt

	rendered := state.Render()

	if !strings.Contains(rendered, "Send Prompt") {
		t.Error("should contain 'Send Prompt' action")
	}
	if !strings.Contains(rendered, "Enter prompt:") {
		t.Error("should show prompt input label")
	}
	if !strings.Contains(rendered, "Send prompt to 2 session(s)") {
		t.Error("should show send prompt confirmation message")
	}
}

func TestBulkActionState_GetPrompt(t *testing.T) {
	state := NewBulkActionState([]string{"s1"})
	state.Action = BulkActionSendPrompt

	// Simulate typing text into the prompt
	state.PromptInput.SetValue("Test prompt message")

	prompt := state.GetPrompt()
	if prompt != "Test prompt message" {
		t.Errorf("expected 'Test prompt message', got %q", prompt)
	}
}

func TestBulkActionState_GetPrompt_Empty(t *testing.T) {
	state := NewBulkActionState([]string{"s1"})

	prompt := state.GetPrompt()
	if prompt != "" {
		t.Errorf("expected empty prompt, got %q", prompt)
	}
}

func TestBulkActionState_GetPrompt_Trimmed(t *testing.T) {
	state := NewBulkActionState([]string{"s1"})
	state.PromptInput.SetValue("  Test prompt  \n")

	prompt := state.GetPrompt()
	if prompt != "Test prompt" {
		t.Errorf("expected trimmed 'Test prompt', got %q", prompt)
	}
}

func TestBulkActionState_PromptInputInitialized(t *testing.T) {
	state := NewBulkActionState([]string{"s1"})

	// Check that PromptInput is initialized
	if state.PromptInput.Placeholder == "" {
		t.Error("PromptInput should have a placeholder")
	}
}

func TestBulkActionState_PromptInput_ArrowKeysForEditing(t *testing.T) {
	state := NewBulkActionState([]string{"s1"})
	state.Action = BulkActionSendPrompt
	state.PromptInput.Focus()

	// Type some text
	state.PromptInput.SetValue("hello")

	// Arrow keys should work for cursor movement within the textarea, not action switching
	// Just verify that arrow key messages are forwarded to the textarea
	_, cmd := state.Update(tea.KeyPressMsg{Code: tea.KeyLeft})

	// The action should remain on SendPrompt (arrow keys don't switch actions)
	if state.Action != BulkActionSendPrompt {
		t.Errorf("arrow keys should not switch actions when on SendPrompt, got action %d", state.Action)
	}

	// Cmd might be nil or a textarea command - either is fine
	_ = cmd
}

func TestBulkActionState_FocusManagement(t *testing.T) {
	state := NewBulkActionState([]string{"s1"})

	// Start on Delete - textarea should not be focused
	if state.PromptInput.Focused() {
		t.Error("textarea should not be focused initially")
	}

	// Navigate to SendPrompt
	state.Update(tea.KeyPressMsg{Code: tea.KeyRight}) // to Move
	state.Update(tea.KeyPressMsg{Code: tea.KeyRight}) // to CreatePRs
	state.Update(tea.KeyPressMsg{Code: tea.KeyRight}) // to SendPrompt

	// Textarea should be focused now
	if !state.PromptInput.Focused() {
		t.Error("textarea should be focused when on SendPrompt action")
	}

	// Navigate away using shift+tab
	state.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}) // to CreatePRs

	// Textarea should be blurred
	if state.PromptInput.Focused() {
		t.Error("textarea should be blurred when navigating away from SendPrompt")
	}
}

func TestBulkActionState_PromptInput_AcceptsTyping(t *testing.T) {
	state := NewBulkActionState([]string{"s1"})
	state.Action = BulkActionSendPrompt
	state.PromptInput.Focus() // Focus textarea since we're directly setting the action

	// Simulate typing
	state.Update(tea.KeyPressMsg{Code: -1, Text: "t"})
	state.Update(tea.KeyPressMsg{Code: -1, Text: "e"})
	state.Update(tea.KeyPressMsg{Code: -1, Text: "s"})
	state.Update(tea.KeyPressMsg{Code: -1, Text: "t"})

	prompt := state.GetPrompt()
	if prompt != "test" {
		t.Errorf("expected 'test', got %q", prompt)
	}
}

func TestBulkActionState_PromptInput_NavigationStillWorks(t *testing.T) {
	state := NewBulkActionState([]string{"s1"})
	state.Action = BulkActionSendPrompt
	state.PromptInput.Focus() // Focus since we're directly setting the action

	// Type something
	state.PromptInput.SetValue("test")

	// Navigate left using shift+tab (arrow keys are used for text editing when on SendPrompt)
	state.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})

	if state.Action != BulkActionCreatePRs {
		t.Errorf("expected to switch to CreatePRs, got %d", state.Action)
	}

	if state.GetPrompt() != "test" {
		t.Error("prompt should be preserved when navigating")
	}
}
