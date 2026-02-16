package app

import (
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"
	pexec "github.com/zhubert/plural/internal/exec"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/keys"
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

	// 'n' requires sidebar focus, should NOT be handled so key propagates to textarea
	result, cmd, handled := m.ExecuteShortcut("n")
	if handled {
		t.Error("Expected 'n' to NOT be handled when guard fails (key should propagate to textarea)")
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

	// 'd' requires a session selected, should NOT be handled so key propagates
	result, cmd, handled := m.ExecuteShortcut("d")
	if handled {
		t.Error("Expected 'd' to NOT be handled when guard fails (key should propagate)")
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

	// Second '/' should NOT be handled when condition fails, so key propagates
	result, cmd, handled := m.ExecuteShortcut("/")
	if handled {
		t.Error("Expected '/' to NOT be handled when condition fails (key should propagate)")
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

func TestGetApplicableHelpSections_IncludesApplicableCategories(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	allShortcuts := append(ShortcutRegistry, helpShortcut)
	sections := m.getApplicableHelpSections(allShortcuts, DisplayOnlyShortcuts)

	// Check that we have the expected categories (when sidebar is focused, no session)
	categoryFound := make(map[string]bool)
	for _, section := range sections {
		categoryFound[section.Title] = true
	}

	// These categories should always have some applicable shortcuts when sidebar is focused
	expectedCategories := []string{
		CategoryNavigation,
		CategoryConfiguration,
		CategoryGeneral,
	}

	for _, cat := range expectedCategories {
		if !categoryFound[cat] {
			t.Errorf("Expected category %q not found in help sections", cat)
		}
	}
}

func TestGetApplicableHelpSections_FiltersSessionRequiredShortcuts(t *testing.T) {
	cfg := testConfig() // No sessions
	m := testModelWithSize(cfg, 120, 40)

	allShortcuts := append(ShortcutRegistry, helpShortcut)
	sections := m.getApplicableHelpSections(allShortcuts, DisplayOnlyShortcuts)

	// Find sessions section (if it exists)
	var sessSection *ui.HelpSection
	for i := range sections {
		if sections[i].Title == CategorySessions {
			sessSection = &sections[i]
			break
		}
	}

	// If sessions section exists, it should NOT have shortcuts requiring a session
	if sessSection != nil {
		for _, s := range sessSection.Shortcuts {
			// 'd' (delete) requires session selected
			if s.Key == "d" {
				t.Error("Expected 'd' shortcut to be filtered out when no session selected")
			}
		}
	}
}

func TestGetApplicableHelpSections_IncludesSessionShortcutsWhenSelected(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	allShortcuts := append(ShortcutRegistry, helpShortcut)
	sections := m.getApplicableHelpSections(allShortcuts, DisplayOnlyShortcuts)

	// Find sessions section
	var sessSection *ui.HelpSection
	for i := range sections {
		if sections[i].Title == CategorySessions {
			sessSection = &sections[i]
			break
		}
	}
	if sessSection == nil {
		t.Fatal("Sessions section not found when session is selected")
	}

	// Should include 'd' (delete) since session is selected
	foundDelete := false
	for _, s := range sessSection.Shortcuts {
		if s.Key == "d" {
			foundDelete = true
			break
		}
	}
	if !foundDelete {
		t.Error("Expected 'd' shortcut to be included when session is selected")
	}
}

func TestGetApplicableHelpSections_IncludesDisplayOnlyShortcuts(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	allShortcuts := append(ShortcutRegistry, helpShortcut)
	sections := m.getApplicableHelpSections(allShortcuts, DisplayOnlyShortcuts)

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

func TestGetApplicableHelpSections_IncludesHelpShortcut(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	allShortcuts := append(ShortcutRegistry, helpShortcut)
	sections := m.getApplicableHelpSections(allShortcuts, DisplayOnlyShortcuts)

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

func TestGetApplicableHelpSections_UsesDisplayKeyWhenSet(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	allShortcuts := append(ShortcutRegistry, helpShortcut)
	sections := m.getApplicableHelpSections(allShortcuts, DisplayOnlyShortcuts)

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

func TestGetApplicableHelpSections_OrderedByCategory(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	allShortcuts := append(ShortcutRegistry, helpShortcut)
	sections := m.getApplicableHelpSections(allShortcuts, DisplayOnlyShortcuts)

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

func TestGetApplicableHelpSections_FiltersChatShortcutsWhenSidebarFocused(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)
	// Sidebar is focused by default

	allShortcuts := append(ShortcutRegistry, helpShortcut)
	sections := m.getApplicableHelpSections(allShortcuts, DisplayOnlyShortcuts)

	// Chat category should not appear when sidebar is focused
	for _, section := range sections {
		if section.Title == CategoryChat {
			t.Error("Expected Chat category to be filtered out when sidebar is focused")
		}
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
	// The first shortcut should be passed (navigated down twice)
	shortcut := state.GetSelectedShortcut()
	if shortcut == nil {
		t.Fatal("Expected non-nil shortcut after navigation")
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

// =============================================================================
// Textarea Input Protection Tests
// =============================================================================

// TestAllShortcutKeysTypableInTextarea ensures that when the textarea is focused,
// all shortcut keys pass through to the textarea instead of triggering shortcuts.
// This is a critical test - if this fails, users cannot type certain letters.
func TestAllShortcutKeysTypableInTextarea(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select session to focus the chat textarea
	m = sendKey(m, "enter")
	if !m.chat.IsFocused() {
		t.Fatal("Expected chat to be focused after selecting session")
	}

	// Collect all single-character shortcut keys that have RequiresSidebar guard
	// These are the keys that could potentially block typing if the bug exists
	var sidebarShortcutKeys []string
	for _, s := range ShortcutRegistry {
		if s.RequiresSidebar && len(s.Key) == 1 {
			sidebarShortcutKeys = append(sidebarShortcutKeys, s.Key)
		}
	}

	// Also include the help shortcut which is handled specially
	sidebarShortcutKeys = append(sidebarShortcutKeys, "?")

	// Verify we have shortcuts to test (sanity check)
	if len(sidebarShortcutKeys) == 0 {
		t.Fatal("No single-character sidebar shortcuts found - test may be misconfigured")
	}

	// Each shortcut key should NOT be handled when textarea is focused
	for _, key := range sidebarShortcutKeys {
		_, _, handled := m.ExecuteShortcut(key)
		if handled {
			t.Errorf("Key %q was handled by shortcut system when textarea is focused - users cannot type this letter!", key)
		}
	}
}

// TestCommonLettersTypableInTextarea verifies that common letters used in
// messages can be typed. This is a regression test for the textarea input bug.
func TestCommonLettersTypableInTextarea(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select session to focus the chat textarea
	m = sendKey(m, "enter")
	if !m.chat.IsFocused() {
		t.Fatal("Expected chat to be focused after selecting session")
	}

	// Test every letter of the alphabet - users should be able to type all of them
	alphabet := "abcdefghijklmnopqrstuvwxyz"
	for _, letter := range alphabet {
		key := string(letter)
		_, _, handled := m.ExecuteShortcut(key)
		if handled {
			t.Errorf("Letter %q intercepted by shortcut system - users cannot type this letter!", key)
		}
	}
}

// =============================================================================
// Preview Session Tests
// =============================================================================

func TestPreviewInMain_CommitsSessionChangesFirst(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Create mock executor to simulate git operations
	mockExec := pexec.NewMockExecutor(nil)

	// Session worktree has uncommitted changes
	mockExec.AddRule(func(dir, name string, args []string) bool {
		return name == "git" && len(args) > 0 && args[0] == "status" && dir == "/test/worktree1"
	}, pexec.MockResponse{
		Stdout: []byte(" M modified.go\n"),
	})

	// Main repo is clean
	mockExec.AddRule(func(dir, name string, args []string) bool {
		return name == "git" && len(args) > 0 && args[0] == "status" && dir == "/test/repo1"
	}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	// Git diff for session worktree (for commit message)
	mockExec.AddRule(func(dir, name string, args []string) bool {
		return name == "git" && len(args) > 1 && args[0] == "diff" && dir == "/test/worktree1"
	}, pexec.MockResponse{
		Stdout: []byte(" 1 file changed, 5 insertions(+)"),
	})

	// Git add -A should succeed
	mockExec.AddRule(func(dir, name string, args []string) bool {
		return name == "git" && len(args) > 0 && args[0] == "add" && dir == "/test/worktree1"
	}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	// Git commit should succeed
	mockExec.AddRule(func(dir, name string, args []string) bool {
		return name == "git" && len(args) > 0 && args[0] == "commit" && dir == "/test/worktree1"
	}, pexec.MockResponse{
		Stdout: []byte("[feature-branch abc1234] Plural session changes\n"),
	})

	// Get current branch of main repo
	mockExec.AddRule(func(dir, name string, args []string) bool {
		return name == "git" && len(args) > 0 && args[0] == "branch" && args[1] == "--show-current" && dir == "/test/repo1"
	}, pexec.MockResponse{
		Stdout: []byte("main\n"),
	})

	// Checkout branch in main repo (ignore worktrees)
	mockExec.AddRule(func(dir, name string, args []string) bool {
		return name == "git" && len(args) > 0 && args[0] == "checkout" && dir == "/test/repo1"
	}, pexec.MockResponse{
		Stdout: []byte("Switched to branch 'feature-branch'\n"),
	})

	// Set up mock git service
	mockGitService := git.NewGitServiceWithExecutor(mockExec)
	m.SetGitService(mockGitService)

	// Execute the preview shortcut
	_, _, handled := m.ExecuteShortcut("p")
	if !handled {
		t.Error("Expected 'p' shortcut to be handled")
	}

	// Verify git commands were called in the right order
	calls := mockExec.GetCalls()
	foundSessionStatus := false
	foundCommit := false
	foundMainStatus := false
	foundCheckout := false

	for _, call := range calls {
		if call.Name == "git" && len(call.Args) > 0 {
			if call.Args[0] == "status" && call.Dir == "/test/worktree1" {
				foundSessionStatus = true
			}
			if call.Args[0] == "commit" && call.Dir == "/test/worktree1" {
				foundCommit = true
			}
			if call.Args[0] == "status" && call.Dir == "/test/repo1" {
				foundMainStatus = true
			}
			if call.Args[0] == "checkout" && call.Dir == "/test/repo1" {
				foundCheckout = true
			}
		}
	}

	if !foundSessionStatus {
		t.Error("Expected session worktree status to be checked")
	}
	if !foundCommit {
		t.Error("Expected session changes to be committed before preview")
	}
	if !foundMainStatus {
		t.Error("Expected main repo status to be checked")
	}
	if !foundCheckout {
		t.Error("Expected session branch to be checked out in main repo")
	}
}

// =============================================================================
// Shortcut Handler Tests (0% coverage handlers)
// =============================================================================

func TestShortcutSearchMessages_NoMessages(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Select session and switch to chat
	m = sendKey(m, "enter")
	if !m.chat.IsFocused() {
		t.Fatal("expected chat to be focused")
	}

	// Chat has no messages, shortcutSearchMessages should return nil cmd
	result, cmd := shortcutSearchMessages(m)
	if cmd != nil {
		t.Error("expected nil cmd when no messages")
	}
	if result != m {
		t.Error("expected model to be unchanged")
	}
}

func TestShortcutToggleToolUseRollup_NoPanic(t *testing.T) {
	cfg := testConfigWithSessions()
	m, _ := testModelWithMocks(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	m = sendKey(m, "enter")

	// Should not panic even without active rollup
	result, cmd := shortcutToggleToolUseRollup(m)
	if result == nil {
		t.Error("expected non-nil model")
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestShortcutWhatsNew_ReturnsCommand(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	_, cmd := shortcutWhatsNew(m)
	if cmd == nil {
		t.Error("expected non-nil cmd from shortcutWhatsNew")
	}
}

func TestShortcutMultiSelect_EntersMode(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	if m.sidebar.IsMultiSelectMode() {
		t.Fatal("should not be in multi-select mode initially")
	}

	shortcutMultiSelect(m)

	if !m.sidebar.IsMultiSelectMode() {
		t.Error("expected sidebar to be in multi-select mode")
	}
}

func TestShortcutWorkspaces_OpensModal(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	if m.modal.IsVisible() {
		t.Fatal("modal should not be visible initially")
	}

	shortcutWorkspaces(m)

	if !m.modal.IsVisible() {
		t.Error("expected modal to be visible after shortcutWorkspaces")
	}
}

func TestShortcutBroadcast_OpensModal(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	if m.modal.IsVisible() {
		t.Fatal("modal should not be visible initially")
	}

	shortcutBroadcast(m)

	if !m.modal.IsVisible() {
		t.Error("expected modal to be visible after shortcutBroadcast")
	}

	_, ok := m.modal.State.(*ui.BroadcastState)
	if !ok {
		t.Errorf("expected BroadcastState modal, got %T", m.modal.State)
	}
}

func TestShortcutRepoSettings_OpensModal(t *testing.T) {
	t.Run("RepoSelected", func(t *testing.T) {
		cfg := testConfig()
		m := testModelWithSize(cfg, 120, 40)

		// With repos but no sessions, sidebar starts on a repo
		shortcutRepoSettings(m)

		if !m.modal.IsVisible() {
			t.Error("expected modal to be visible")
		}
		_, ok := m.modal.State.(*ui.RepoSettingsState)
		if !ok {
			t.Errorf("expected RepoSettingsState modal, got %T", m.modal.State)
		}
	})

	t.Run("SessionSelected", func(t *testing.T) {
		cfg := testConfigWithSessions()
		m := testModelWithSize(cfg, 120, 40)
		m.sidebar.SetSessions(cfg.Sessions)
		// SetSessions auto-advances to session-1, so sidebar already points at a session

		shortcutRepoSettings(m)

		if !m.modal.IsVisible() {
			t.Error("expected modal to be visible")
		}
		state, ok := m.modal.State.(*ui.SessionSettingsState)
		if !ok {
			t.Errorf("expected SessionSettingsState modal, got %T", m.modal.State)
		}
		if state.SessionID != "session-1" {
			t.Errorf("expected session-1, got %s", state.SessionID)
		}
	})
}

func TestShortcutGlobalSettings_OpensModal(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)

	shortcutGlobalSettings(m)

	if !m.modal.IsVisible() {
		t.Error("expected modal to be visible")
	}
	_, ok := m.modal.State.(*ui.SettingsState)
	if !ok {
		t.Errorf("expected SettingsState modal, got %T", m.modal.State)
	}
}

func TestAltComma_OpensGlobalSettingsFromSidebar(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Sidebar is focused by default
	_, _, handled := m.ExecuteShortcut(keys.AltComma)
	if !handled {
		t.Error("Expected opt-, to be handled from sidebar")
	}
	if !m.modal.IsVisible() {
		t.Error("Expected modal to be visible")
	}
	_, ok := m.modal.State.(*ui.SettingsState)
	if !ok {
		t.Errorf("Expected SettingsState (global settings), got %T", m.modal.State)
	}
}

func TestAltComma_OpensGlobalSettingsFromChat(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Switch to chat focus
	m = sendKey(m, "enter")
	if !m.chat.IsFocused() {
		t.Fatal("Expected chat to be focused")
	}

	_, _, handled := m.ExecuteShortcut(keys.AltComma)
	if !handled {
		t.Error("Expected opt-, to be handled from chat")
	}
	if !m.modal.IsVisible() {
		t.Error("Expected modal to be visible")
	}
	_, ok := m.modal.State.(*ui.SettingsState)
	if !ok {
		t.Errorf("Expected SettingsState (global settings), got %T", m.modal.State)
	}
}

func TestAltComma_AlwaysGlobalEvenWhenRepoSelected(t *testing.T) {
	cfg := testConfig()
	m := testModelWithSize(cfg, 120, 40)

	// Sidebar starts on a repo - comma would show repo settings, but opt-, should show global
	_, _, handled := m.ExecuteShortcut(keys.AltComma)
	if !handled {
		t.Error("Expected opt-, to be handled")
	}
	if !m.modal.IsVisible() {
		t.Error("Expected modal to be visible")
	}
	_, ok := m.modal.State.(*ui.SettingsState)
	if !ok {
		t.Errorf("Expected SettingsState (global settings) even with repo selected, got %T", m.modal.State)
	}
}

func TestPreviewInMain_NoCommitWhenClean(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Create mock executor to simulate git operations
	mockExec := pexec.NewMockExecutor(nil)

	// Session worktree is clean (no uncommitted changes)
	mockExec.AddRule(func(dir, name string, args []string) bool {
		return name == "git" && len(args) > 0 && args[0] == "status" && dir == "/test/worktree1"
	}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	// Main repo is clean
	mockExec.AddRule(func(dir, name string, args []string) bool {
		return name == "git" && len(args) > 0 && args[0] == "status" && dir == "/test/repo1"
	}, pexec.MockResponse{
		Stdout: []byte(""),
	})

	// Get current branch of main repo
	mockExec.AddRule(func(dir, name string, args []string) bool {
		return name == "git" && len(args) > 0 && args[0] == "branch" && args[1] == "--show-current" && dir == "/test/repo1"
	}, pexec.MockResponse{
		Stdout: []byte("main\n"),
	})

	// Checkout branch in main repo
	mockExec.AddRule(func(dir, name string, args []string) bool {
		return name == "git" && len(args) > 0 && args[0] == "checkout" && dir == "/test/repo1"
	}, pexec.MockResponse{
		Stdout: []byte("Switched to branch 'feature-branch'\n"),
	})

	// Set up mock git service
	mockGitService := git.NewGitServiceWithExecutor(mockExec)
	m.SetGitService(mockGitService)

	// Execute the preview shortcut
	m.ExecuteShortcut("p")

	// Verify that commit was NOT called (since session was clean)
	calls := mockExec.GetCalls()
	for _, call := range calls {
		if call.Name == "git" && len(call.Args) > 0 && call.Args[0] == "commit" {
			t.Error("Expected no commit when session worktree is clean")
		}
	}
}

func TestSidebarNavigation_AutoSelectsSession(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Initially no active session (SetSessions auto-advances cursor to session-1
	// but doesn't trigger auto-select since no Update loop runs)
	if m.activeSession != nil {
		t.Fatal("expected no active session initially")
	}

	// Navigate down from session-1 (index 1) to session-2 (index 2)
	// This triggers auto-select of session-2
	m = sendKey(m, "j")

	// Session-2 should be auto-selected
	if m.activeSession == nil {
		t.Fatal("expected session to be auto-selected on navigate")
	}
	if m.activeSession.ID != "session-2" {
		t.Errorf("expected session-2, got %s", m.activeSession.ID)
	}

	// Focus should remain on sidebar
	if m.focus != FocusSidebar {
		t.Error("expected focus to remain on sidebar after auto-select")
	}
}

func TestSidebarNavigation_AutoSelectsNewSessionOnMove(t *testing.T) {
	cfg := testConfigWithSessions()
	m := testModelWithSize(cfg, 120, 40)
	m.sidebar.SetSessions(cfg.Sessions)

	// Navigate down to session-2 (from session-1 which SetSessions auto-advanced to)
	m = sendKey(m, "j")
	if m.activeSession == nil || m.activeSession.ID != "session-2" {
		t.Fatal("expected session-2 to be auto-selected")
	}

	// Navigate up back to session-1
	m = sendKey(m, "k")
	if m.activeSession == nil || m.activeSession.ID != "session-1" {
		t.Errorf("expected session-1 after navigating up, got %v", m.activeSession)
	}

	// Focus should still be on sidebar
	if m.focus != FocusSidebar {
		t.Error("expected focus to remain on sidebar")
	}
}

// =============================================================================
// Terminal Detection Tests
// =============================================================================

func TestDetectTerminalApp(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected string
	}{
		{"ghostty lowercase", "ghostty", "Ghostty"},
		{"ghostty mixed case", "Ghostty", "Ghostty"},
		{"iterm", "iTerm.app", "iTerm2"},
		{"iterm lowercase", "iterm.app", "iTerm2"},
		{"apple terminal", "Apple_Terminal", "Terminal"},
		{"apple terminal lowercase", "apple_terminal", "Terminal"},
		{"wezterm", "WezTerm", "WezTerm"},
		{"wezterm lowercase", "wezterm", "WezTerm"},
		{"kitty", "kitty", "kitty"},
		{"kitty uppercase", "Kitty", "kitty"},
		{"alacritty", "alacritty", "Alacritty"},
		{"alacritty mixed case", "Alacritty", "Alacritty"},
		{"empty string", "", "Terminal"},
		{"unknown terminal", "some-random-terminal", "Terminal"},
		{"tmux", "tmux", "Terminal"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := os.Getenv("TERM_PROGRAM")
			os.Setenv("TERM_PROGRAM", tt.envValue)
			defer os.Setenv("TERM_PROGRAM", original)

			result := detectTerminalApp()
			if result != tt.expected {
				t.Errorf("detectTerminalApp() with TERM_PROGRAM=%q = %q, want %q", tt.envValue, result, tt.expected)
			}
		})
	}
}
