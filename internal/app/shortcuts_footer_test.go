package app

import (
	"testing"

	"github.com/zhubert/plural-core/config"
	"github.com/zhubert/plural/internal/ui"
)

// TestGetApplicableFooterBindings verifies that footer bindings are correctly
// generated from the shortcut registry with proper guard checks.
func TestGetApplicableFooterBindings(t *testing.T) {
	tests := []struct {
		name              string
		hasSession        bool
		sidebarFocused    bool
		chatFocused       bool
		expectKeys        []string // Keys that should appear
		expectDescriptions []string // Descriptions that should match
		notExpectKeys     []string // Keys that should NOT appear
	}{
		{
			name:           "sidebar focused with no session",
			hasSession:     false,
			sidebarFocused: true,
			chatFocused:    false,
			expectKeys:     []string{"n", "a", "q", "?"},
			expectDescriptions: []string{"Create new session", "Add repository", "Quit application", "Show this help"},
			notExpectKeys:  []string{"v", "m", "f", "d"}, // Session-required shortcuts
		},
		{
			name:           "sidebar focused with session",
			hasSession:     true,
			sidebarFocused: true,
			chatFocused:    false,
			expectKeys:     []string{"Tab", "n", "a", "v", "m", "f", "d", "q", "?"},
			expectDescriptions: []string{
				"Switch between sidebar and chat",
				"Create new session",
				"Add repository",
				"View changes in worktree",
				"Merge to main / Create PR",
				"Fork selected session",
				"Delete selected session",
				"Quit application",
				"Show this help",
			},
			notExpectKeys: nil,
		},
		{
			name:           "chat focused with session",
			hasSession:     true,
			sidebarFocused: false,
			chatFocused:    true,
			expectKeys:     []string{"Tab", "ctrl-e"},
			notExpectKeys:  []string{"n", "a", "v", "m", "f", "d", "q", "?"}, // Sidebar-only shortcuts
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal config
			cfg := &config.Config{
				Repos:    []string{},
				Sessions: []config.Session{},
			}
			m := New(cfg, "test-version")

			// Set up test state
			if tt.hasSession {
				// Add a repo and session
				cfg.AddRepo("/test/repo")
				sess := &config.Session{
					ID:       "test-session-id",
					RepoPath: "/test/repo",
					WorkTree: "/test/worktree",
					Branch:   "test-branch",
					Name:     "test-session",
				}
				cfg.AddSession(*sess)
				m.activeSession = sess
				m.sidebar.SetSessions(cfg.GetSessions())
			}

			// Set focus state
			if tt.chatFocused {
				m.focus = FocusChat
				m.chat.SetFocused(true)
				m.sidebar.SetFocused(false)
			} else {
				m.focus = FocusSidebar
				m.chat.SetFocused(false)
				m.sidebar.SetFocused(true)
			}

			// Get footer bindings
			bindings := m.getApplicableFooterBindings()

			// Verify expected keys are present
			for i, expectedKey := range tt.expectKeys {
				found := false
				for _, binding := range bindings {
					if binding.Key == expectedKey {
						found = true
						// Also verify the description matches if provided
						if i < len(tt.expectDescriptions) {
							expectedDesc := tt.expectDescriptions[i]
							if binding.Desc != expectedDesc {
								t.Errorf("Key %q has wrong description: got %q, want %q", expectedKey, binding.Desc, expectedDesc)
							}
						}
						break
					}
				}
				if !found {
					t.Errorf("Expected key %q not found in footer bindings", expectedKey)
				}
			}

			// Verify keys that should NOT be present
			for _, notExpectedKey := range tt.notExpectKeys {
				for _, binding := range bindings {
					if binding.Key == notExpectedKey {
						t.Errorf("Key %q should NOT be in footer bindings but was found", notExpectedKey)
					}
				}
			}
		})
	}
}

// TestFooterBindingsMatchHelpModal verifies that the footer bindings are a subset
// of what appears in the help modal (both should use the same registry).
func TestFooterBindingsMatchHelpModal(t *testing.T) {
	cfg := &config.Config{
		Repos:    []string{"/test/repo"},
		Sessions: []config.Session{
			{
				ID:       "test-session-id",
				RepoPath: "/test/repo",
				WorkTree: "/test/worktree",
				Branch:   "test-branch",
				Name:     "test-session",
			},
		},
	}

	m := New(cfg, "test-version")
	m.activeSession = &cfg.Sessions[0]
	m.focus = FocusSidebar
	m.sidebar.SetFocused(true)
	m.sidebar.SetSessions(cfg.Sessions)

	// Get footer bindings
	footerBindings := m.getApplicableFooterBindings()

	// Get help sections (which also use the registry)
	helpSections := m.getApplicableHelpSections(
		append(ShortcutRegistry, helpShortcut),
		DisplayOnlyShortcuts,
	)

	// Build a map of all help shortcuts
	helpShortcuts := make(map[string]ui.HelpShortcut)
	for _, section := range helpSections {
		for _, shortcut := range section.Shortcuts {
			helpShortcuts[shortcut.Key] = shortcut
		}
	}

	// Verify each footer binding exists in help
	for _, footerBinding := range footerBindings {
		if _, exists := helpShortcuts[footerBinding.Key]; !exists {
			t.Errorf("Footer binding %q (%s) not found in help modal", footerBinding.Key, footerBinding.Desc)
		}
	}
}

// TestFooterBindingsInjection verifies that the bindings generator is properly
// injected into the footer during model initialization.
func TestFooterBindingsInjection(t *testing.T) {
	cfg := &config.Config{
		Repos:    []string{"/test/repo"},
		Sessions: []config.Session{},
	}

	m := New(cfg, "test-version")

	// Call the generator to make sure it doesn't panic
	bindings := m.footer.GetApplicableBindings()
	if bindings == nil {
		t.Error("Bindings generator returned nil")
	}

	// Should have at least a few basic bindings (n, a, q, ?)
	if len(bindings) < 3 {
		t.Errorf("Expected at least 3 bindings, got %d", len(bindings))
	}
}
