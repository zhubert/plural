package app

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/config"
)

// testConfig creates a minimal config for testing.
func testConfig() *config.Config {
	return &config.Config{
		Repos:            []string{"/test/repo1", "/test/repo2"},
		Sessions:         []config.Session{},
		AllowedTools:     []string{},
		RepoAllowedTools: make(map[string][]string),
		MCPServers:       []config.MCPServer{},
		RepoMCP:          make(map[string][]config.MCPServer),
		WelcomeShown:     true, // Skip welcome modal in tests
	}
}

// testConfigWithSessions creates a config with test sessions.
func testConfigWithSessions() *config.Config {
	cfg := testConfig()
	cfg.Sessions = []config.Session{
		{
			ID:        "session-1",
			RepoPath:  "/test/repo1",
			WorkTree:  "/test/worktree1",
			Branch:    "feature-branch",
			Name:      "repo1/session1",
			CreatedAt: time.Now(),
			Started:   true,
		},
		{
			ID:        "session-2",
			RepoPath:  "/test/repo1",
			WorkTree:  "/test/worktree2",
			Branch:    "plural-abc123",
			Name:      "repo1/abc123",
			CreatedAt: time.Now(),
			Started:   false,
		},
		{
			ID:        "session-3",
			RepoPath:  "/test/repo2",
			WorkTree:  "/test/worktree3",
			Branch:    "bugfix",
			Name:      "repo2/bugfix",
			CreatedAt: time.Now(),
			Started:   true,
		},
	}
	return cfg
}

// testModel creates a test Model with the given config.
func testModel(cfg *config.Config) *Model {
	return New(cfg, "0.0.0-test")
}

// testModelWithSize creates a test Model and sets its size.
func testModelWithSize(cfg *config.Config, width, height int) *Model {
	m := testModel(cfg)
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

// keyPress creates a tea.KeyPressMsg for the given key string.
// Examples: "a", "enter", "tab", "esc", "ctrl+c", "up", "down"
func keyPress(key string) tea.KeyPressMsg {
	switch key {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "ctrl+v":
		return tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl}
	case "ctrl+s":
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	default:
		// Regular character - for single characters, set both Code and Text
		if len(key) == 1 {
			return tea.KeyPressMsg{Code: rune(key[0]), Text: key}
		}
		// Fallback for unknown keys
		return tea.KeyPressMsg{Text: key}
	}
}

// sendKey sends a key press to the model and returns the updated model.
func sendKey(m *Model, key string) *Model {
	result, _ := m.Update(keyPress(key))
	return result.(*Model)
}

// typeText simulates typing a string by sending individual character key presses.
func typeText(m *Model, text string) *Model {
	for _, ch := range text {
		m = sendKey(m, string(ch))
	}
	return m
}

// setSize sends a window size message to the model.
func setSize(m *Model, width, height int) *Model {
	result, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return result.(*Model)
}
