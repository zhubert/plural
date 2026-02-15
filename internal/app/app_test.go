package app

import (
	"testing"

	"github.com/zhubert/plural/internal/config"
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
