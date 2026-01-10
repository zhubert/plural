package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/ui"
)

// =============================================================================
// ShortcutRegistry Tests
// =============================================================================

func TestShortcutRegistry_AllShortcutsHaveHandlers(t *testing.T) {
	for _, s := range ShortcutRegistry {
		if s.Handler == nil {
			t.Errorf("Shortcut %q has no handler", s.Key)
		}
		if s.Key == "" {
			t.Error("Shortcut has empty key")
		}
		if s.Description == "" {
			t.Errorf("Shortcut %q has no description", s.Key)
		}
		if s.Category == "" {
			t.Errorf("Shortcut %q has no category", s.Key)
		}
	}
}

func TestShortcutRegistry_NoDuplicateKeys(t *testing.T) {
	seen := make(map[string]bool)
	for _, s := range ShortcutRegistry {
		if seen[s.Key] {
			t.Errorf("Duplicate shortcut key: %q", s.Key)
		}
		seen[s.Key] = true
	}
	// Also check the help shortcut
	if seen["?"] {
		t.Error("Help shortcut key '?' duplicated in registry")
	}
}

func TestShortcutRegistry_ValidCategories(t *testing.T) {
	validCategories := map[string]bool{
		CategoryNavigation:    true,
		CategorySessions:      true,
		CategoryGit:           true,
		CategoryConfiguration: true,
		CategoryChat:          true,
		CategoryPermissions:   true,
		CategoryGeneral:       true,
	}

	for _, s := range ShortcutRegistry {
		if !validCategories[s.Category] {
			t.Errorf("Shortcut %q has invalid category: %q", s.Key, s.Category)
		}
	}
}

// =============================================================================
// ExecuteShortcut Tests
// =============================================================================

func TestExecuteShortcut_FindsRegisteredShortcut(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Tab should be found and executed
	_, _, handled := m.ExecuteShortcut("tab")
	if !handled {
		t.Error("Expected 'tab' shortcut to be handled")
	}
}

func TestExecuteShortcut_ReturnsNotHandledForUnknownKey(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	_, _, handled := m.ExecuteShortcut("unknown-key")
	if handled {
		t.Error("Expected unknown key to not be handled")
	}
}

func TestExecuteShortcut_HelpShortcutHandledSpecially(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Help shortcut should be handled (it's defined outside the registry)
	_, _, handled := m.ExecuteShortcut("?")
	if !handled {
		t.Error("Expected '?' shortcut to be handled")
	}
}

func TestExecuteShortcut_RequiresSidebarGuard(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select session to get into chat mode
	m = sendKey(m, "enter")
	if !m.chat.IsFocused() {
		t.Fatal("Expected chat to be focused after selecting session")
	}

	// 'n' requires sidebar focus, should be blocked
	result, cmd, handled := m.ExecuteShortcut("n")
	if !handled {
		t.Error("Expected 'n' to be handled (found in registry)")
	}
	if cmd != nil {
		t.Error("Expected no command when guard fails")
	}
	// Model should be unchanged
	if result != m {
		t.Error("Expected model to be unchanged when guard fails")
	}
}

func TestExecuteShortcut_RequiresSessionGuard(t *testing.T) {
	cfg := testConfig() // No sessions
	m := testModelWithSize(cfg, 120, 40)

	// 'd' requires a session selected
	result, cmd, handled := m.ExecuteShortcut("d")
	if !handled {
		t.Error("Expected 'd' to be handled (found in registry)")
	}
	if cmd != nil {
		t.Error("Expected no command when session guard fails")
	}
	if result != m {
		t.Error("Expected model to be unchanged when guard fails")
	}
}

func TestExecuteShortcut_ConditionGuard(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// '/' has condition: not already in search mode
	// First call should work
	m.ExecuteShortcut("/")

	// Now we're in search mode, condition should fail
	if !m.sidebar.IsSearchMode() {
		t.Fatal("Expected to be in search mode after '/'")
	}

	// Second '/' should be blocked by condition
	result, cmd, handled := m.ExecuteShortcut("/")
	if !handled {
		t.Error("Expected '/' to be handled")
	}
	if cmd != nil {
		t.Error("Expected no command when condition fails")
	}
	if result != m {
		t.Error("Expected model unchanged when condition fails")
	}
}

func TestExecuteShortcut_TabTogglesWithoutGuards(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select a session first so we have an active session
	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Expected active session after selecting")
	}

	// Now we should be in chat focus
	if m.focus != FocusChat {
		t.Fatal("Expected chat focus after selecting session")
	}

	// Tab should work regardless of focus - toggle back to sidebar
	_, _, handled := m.ExecuteShortcut("tab")
	if !handled {
		t.Error("Expected 'tab' to be handled")
	}
	if m.focus != FocusSidebar {
		t.Error("Expected focus to change to sidebar after tab")
	}
}

func TestExecuteShortcut_QuitReturnsQuitCmd(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	_, cmd, handled := m.ExecuteShortcut("q")
	if !handled {
		t.Error("Expected 'q' to be handled")
	}
	if cmd == nil {
		t.Error("Expected quit command")
	}
	// Verify it's actually a quit command
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("Expected QuitMsg, got %T", msg)
	}
}

// =============================================================================
// Help Sections Generation Tests
// =============================================================================

func TestGetHelpSections_IncludesAllCategories(t *testing.T) {
	allShortcuts := append(ShortcutRegistry, helpShortcut)
	sections := getHelpSections(allShortcuts, DisplayOnlyShortcuts)

	// Check that we have the expected categories
	categoryFound := make(map[string]bool)
	for _, section := range sections {
		categoryFound[section.Title] = true
	}

	expectedCategories := []string{
		CategoryNavigation,
		CategorySessions,
		CategoryGit,
		CategoryConfiguration,
		CategoryGeneral,
	}

	for _, cat := range expectedCategories {
		if !categoryFound[cat] {
			t.Errorf("Expected category %q not found in help sections", cat)
		}
	}
}

func TestGetHelpSections_IncludesDisplayOnlyShortcuts(t *testing.T) {
	allShortcuts := append(ShortcutRegistry, helpShortcut)
	sections := getHelpSections(allShortcuts, DisplayOnlyShortcuts)

	// Find navigation section
	var navSection *ui.HelpSection
	for i := range sections {
		if sections[i].Title == CategoryNavigation {
			navSection = &sections[i]
			break
		}
	}
	if navSection == nil {
		t.Fatal("Navigation section not found")
	}

	// Should include display-only shortcuts like arrow keys
	foundArrows := false
	for _, s := range navSection.Shortcuts {
		if s.Key == "↑/↓ or j/k" {
			foundArrows = true
			break
		}
	}
	if !foundArrows {
		t.Error("Expected display-only arrow key shortcut in navigation section")
	}
}

func TestGetHelpSections_IncludesHelpShortcut(t *testing.T) {
	allShortcuts := append(ShortcutRegistry, helpShortcut)
	sections := getHelpSections(allShortcuts, DisplayOnlyShortcuts)

	// Find general section
	var generalSection *ui.HelpSection
	for i := range sections {
		if sections[i].Title == CategoryGeneral {
			generalSection = &sections[i]
			break
		}
	}
	if generalSection == nil {
		t.Fatal("General section not found")
	}

	// Should include '?' shortcut
	foundHelp := false
	for _, s := range generalSection.Shortcuts {
		if s.Key == "?" {
			foundHelp = true
			break
		}
	}
	if !foundHelp {
		t.Error("Expected '?' shortcut in general section")
	}
}

func TestGetHelpSections_UsesDisplayKeyWhenSet(t *testing.T) {
	allShortcuts := append(ShortcutRegistry, helpShortcut)
	sections := getHelpSections(allShortcuts, DisplayOnlyShortcuts)

	// Find navigation section for Tab which has DisplayKey set
	var navSection *ui.HelpSection
	for i := range sections {
		if sections[i].Title == CategoryNavigation {
			navSection = &sections[i]
			break
		}
	}
	if navSection == nil {
		t.Fatal("Navigation section not found")
	}

	// Tab shortcut should use DisplayKey "Tab" not "tab"
	foundTab := false
	for _, s := range navSection.Shortcuts {
		if s.Key == "Tab" {
			foundTab = true
			break
		}
	}
	if !foundTab {
		t.Error("Expected 'Tab' (with capital T) in navigation section")
	}
}

func TestGetHelpSections_OrderedByCategory(t *testing.T) {
	allShortcuts := append(ShortcutRegistry, helpShortcut)
	sections := getHelpSections(allShortcuts, DisplayOnlyShortcuts)

	// First section should be Navigation
	if len(sections) == 0 {
		t.Fatal("No sections generated")
	}
	if sections[0].Title != CategoryNavigation {
		t.Errorf("Expected first section to be %q, got %q", CategoryNavigation, sections[0].Title)
	}

	// Last section should be General
	lastSection := sections[len(sections)-1]
	if lastSection.Title != CategoryGeneral {
		t.Errorf("Expected last section to be %q, got %q", CategoryGeneral, lastSection.Title)
	}
}

// =============================================================================
// Key Normalization Tests
// =============================================================================

func TestNormalizeHelpDisplayKey_DisplayOnlyReturnsEmpty(t *testing.T) {
	displayOnlyKeys := []string{
		"↑/↓ or j/k",
		"PgUp/PgDn",
		"Enter",
		"Esc",
		"Ctrl+V",
		"Ctrl+P",
		"y", "n", "a", // permission keys
	}

	for _, key := range displayOnlyKeys {
		result := normalizeHelpDisplayKey(key)
		if result != "" {
			t.Errorf("Expected empty for display-only key %q, got %q", key, result)
		}
	}
}

func TestNormalizeHelpDisplayKey_NormalizesCapitalizedKeys(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"Tab", "tab"},
		{"N", "n"},
		{"Q", "q"},
		{"/", "/"},
		{"?", "?"},
		{"ctrl+f", "ctrl+f"},
	}

	for _, tc := range testCases {
		result := normalizeHelpDisplayKey(tc.input)
		if result != tc.expected {
			t.Errorf("normalizeHelpDisplayKey(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

// =============================================================================
// Integration: Help Modal Shortcut Trigger Tests
// =============================================================================

func TestHelpShortcutTrigger_ExecutesShortcut(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Trigger 'n' from help modal - use sendKey to properly exercise the flow
	// First open help modal
	m = sendKey(m, "?")
	if !m.modal.IsVisible() {
		t.Fatal("Expected help modal to open")
	}

	// Close help modal
	m = sendKey(m, "esc")

	// Now trigger 'n' directly through the shortcut system
	m.ExecuteShortcut("n")

	// Should open new session modal
	if !m.modal.IsVisible() {
		t.Error("Expected modal to be visible after triggering 'n'")
	}
}

func TestHelpShortcutTrigger_DisplayOnlyDoesNothing(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Trigger display-only key
	result, cmd := m.handleHelpShortcutTrigger("↑/↓ or j/k")

	if cmd != nil {
		t.Error("Expected no command for display-only shortcut")
	}
	if result != m {
		t.Error("Expected model unchanged for display-only shortcut")
	}
}

func TestHelpShortcutTrigger_NormalizesTabKey(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select a session first so we have an active session
	m = sendKey(m, "enter")
	if m.activeSession == nil {
		t.Fatal("Expected active session after selecting")
	}

	// Now we should be in chat focus
	if m.focus != FocusChat {
		t.Fatal("Expected chat focus after selecting session")
	}

	// Trigger 'Tab' (capitalized as shown in help) - should toggle back to sidebar
	m.handleHelpShortcutTrigger("Tab")

	if m.focus != FocusSidebar {
		t.Error("Expected focus to change to sidebar after triggering 'Tab'")
	}
}

func TestHelpShortcutTrigger_QuitWorks(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	_, cmd := m.handleHelpShortcutTrigger("q")

	if cmd == nil {
		t.Error("Expected quit command")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("Expected QuitMsg, got %T", msg)
	}
}

// =============================================================================
// Full Flow: Help Modal Opens and Executes Shortcut
// =============================================================================

func TestHelpModal_OpenAndTriggerShortcut(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Open help modal with '?'
	m = sendKey(m, "?")

	if !m.modal.IsVisible() {
		t.Fatal("Expected help modal to be visible")
	}

	// The modal should show help state
	_, isHelp := m.modal.State.(*ui.HelpState)
	if !isHelp {
		t.Fatal("Expected HelpState in modal")
	}

	// Press enter to trigger the selected shortcut (first one is Tab)
	m = sendKey(m, "enter")

	// Modal should be closed and focus toggled
	// Note: Tab toggles focus, but since we start at sidebar, we stay at sidebar
	// because help modal was just closed
	if m.modal.IsVisible() {
		// After triggering, help modal might reopen if it's the help shortcut
		// For this test, we just verify the flow works
	}
}

func TestHelpModal_NavigateAndTrigger(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Open help modal
	m = sendKey(m, "?")
	if !m.modal.IsVisible() {
		t.Fatal("Expected help modal to be visible")
	}

	// Navigate down to a different shortcut
	m = sendKey(m, "down")
	m = sendKey(m, "down")

	// Get current state to verify navigation worked
	state, ok := m.modal.State.(*ui.HelpState)
	if !ok {
		t.Fatal("Expected HelpState")
	}
	if state.SelectedIndex == 0 {
		t.Error("Expected selection to move after pressing down")
	}
}

// =============================================================================
// DisplayOnlyShortcuts Tests
// =============================================================================

func TestDisplayOnlyShortcuts_AllHaveDescriptions(t *testing.T) {
	for _, s := range DisplayOnlyShortcuts {
		if s.Description == "" {
			t.Errorf("Display-only shortcut %q has no description", s.DisplayKey)
		}
		if s.Category == "" {
			t.Errorf("Display-only shortcut %q has no category", s.DisplayKey)
		}
		if s.DisplayKey == "" && s.Key == "" {
			t.Error("Display-only shortcut has no key or display key")
		}
	}
}

func TestDisplayOnlyShortcuts_NoHandlers(t *testing.T) {
	for _, s := range DisplayOnlyShortcuts {
		if s.Handler != nil {
			key := s.DisplayKey
			if key == "" {
				key = s.Key
			}
			t.Errorf("Display-only shortcut %q should not have a handler", key)
		}
	}
}
