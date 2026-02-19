package app

import (
	"testing"
	"time"

	"github.com/zhubert/plural-core/config"
	"github.com/zhubert/plural/internal/ui"
)

func TestNew_DefaultThemeInitialization(t *testing.T) {
	// Create a config with no theme set
	cfg := &config.Config{}

	// Create a new app model
	_ = New(cfg, "test-version")

	// Verify that the default theme (Tokyo Night) is applied
	currentTheme := ui.CurrentTheme()
	if currentTheme.Name != "Tokyo Night" {
		t.Errorf("Expected default theme to be Tokyo Night, got %s", currentTheme.Name)
	}

}

func TestNew_SavedThemeInitialization(t *testing.T) {
	// Create a config with Nord theme saved
	cfg := &config.Config{}
	cfg.SetTheme(string(ui.ThemeNord))

	// Create a new app model
	_ = New(cfg, "test-version")

	// Verify that Nord theme is applied
	currentTheme := ui.CurrentTheme()
	if currentTheme.Name != "Nord" {
		t.Errorf("Expected theme to be Nord, got %s", currentTheme.Name)
	}
}

func TestNew_ThemeStylesMatchThemeColors(t *testing.T) {
	tests := []struct {
		themeName ui.ThemeName
	}{
		{ui.ThemeTokyoNight},
		{ui.ThemeNord},
		{ui.ThemeDracula},
		{ui.ThemeGruvbox},
		{ui.ThemeCatppuccin},
	}

	for _, tt := range tests {
		t.Run(string(tt.themeName), func(t *testing.T) {
			cfg := &config.Config{}
			cfg.SetTheme(string(tt.themeName))

			_ = New(cfg, "test-version")

			currentTheme := ui.CurrentTheme()
			expectedTheme := ui.GetTheme(tt.themeName)

			if currentTheme.Name != expectedTheme.Name {
				t.Errorf("Theme %s: expected current theme to be %s, got %s",
					tt.themeName, expectedTheme.Name, currentTheme.Name)
			}
		})
	}
}

func TestSelectSession_ContainerInitProgressBar_DoesNotShowAfterStarted(t *testing.T) {
	// This tests the fix for GitHub issue #232
	// The container progress bar should NOT show when sending messages to an already-started container

	cfg := testConfig()
	cfg.Sessions = []config.Session{
		{
			ID:            "container-session",
			RepoPath:      "/test",
			WorkTree:      "/test/.plural-worktrees/abc",
			Branch:        "test-branch",
			Name:          "Container Session",
			Containerized: true,
			Started:       true, // Session already started
		},
	}

	m := testModelWithSize(cfg, 120, 40)

	// Simulate the session state manager having stale ContainerInitializing state
	// (this could happen due to a race condition or timing issue)
	m.sessionState().StartContainerInit("container-session")

	// Verify the state manager thinks it's initializing
	_, initializing := m.sessionState().GetContainerInitStart("container-session")
	if !initializing {
		t.Fatal("Expected session state to be initializing for this test")
	}

	// Now select the session
	sess := &cfg.Sessions[0]
	m.selectSession(sess)

	// CRITICAL: Even though SessionStateManager has ContainerInitializing=true,
	// the progress bar should NOT be shown because sess.Started=true
	if m.chat.IsContainerInitializing() {
		t.Error("Container progress bar should NOT show for already-started session, even if SessionStateManager has stale state")
	}
}

func TestSelectSession_ContainerInitProgressBar_ShowsWhenNotStarted(t *testing.T) {
	// Verify the progress bar DOES show when the session hasn't started yet

	cfg := testConfig()
	cfg.Sessions = []config.Session{
		{
			ID:            "new-container-session",
			RepoPath:      "/test",
			WorkTree:      "/test/.plural-worktrees/xyz",
			Branch:        "test-branch",
			Name:          "New Container Session",
			Containerized: true,
			Started:       false, // Session NOT started yet
		},
	}

	m := testModelWithSize(cfg, 120, 40)

	// Mark as initializing
	m.sessionState().StartContainerInit("new-container-session")

	// Select the session
	sess := &cfg.Sessions[0]
	m.selectSession(sess)

	// Progress bar SHOULD show because session hasn't started yet
	if !m.chat.IsContainerInitializing() {
		t.Error("Container progress bar SHOULD show for not-yet-started containerized session")
	}
}

func TestSelectSession_ContainerInitProgressBar_ClearsWhenNotInitializing(t *testing.T) {
	// Verify the progress bar is cleared when not initializing

	cfg := testConfig()
	cfg.Sessions = []config.Session{
		{
			ID:            "container-session",
			RepoPath:      "/test",
			WorkTree:      "/test/.plural-worktrees/def",
			Branch:        "test-branch",
			Name:          "Container Session",
			Containerized: true,
			Started:       false,
		},
	}

	m := testModelWithSize(cfg, 120, 40)

	// Manually set container initializing state in chat
	m.chat.SetContainerInitializing(true, time.Now())

	// Verify it's set
	if !m.chat.IsContainerInitializing() {
		t.Fatal("Expected container initializing to be set")
	}

	// Select the session (but DON'T mark as initializing in state manager)
	sess := &cfg.Sessions[0]
	m.selectSession(sess)

	// Progress bar should be cleared
	if m.chat.IsContainerInitializing() {
		t.Error("Container progress bar should be cleared when not initializing in state manager")
	}
}
