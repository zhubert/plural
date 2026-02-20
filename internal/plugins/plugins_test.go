package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestListMarketplaces_NoFile(t *testing.T) {
	// Override home dir to a temp dir with no files
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	marketplaces, err := ListMarketplaces()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(marketplaces) != 0 {
		t.Errorf("expected 0 marketplaces, got %d", len(marketplaces))
	}
}

func TestListMarketplaces_ValidFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	pluginsDir := filepath.Join(tmp, ".claude", "plugins")
	if err := os.MkdirAll(pluginsDir, 0700); err != nil {
		t.Fatal(err)
	}

	now := time.Now().Format(time.RFC3339)
	knownData := map[string]any{
		"claude-code-plugins": map[string]any{
			"source": map[string]string{
				"source": "github",
				"repo":   "anthropics/claude-code-plugins",
			},
			"installLocation": "/tmp/claude/plugins",
			"lastUpdated":     now,
		},
	}
	data, _ := json.Marshal(knownData)
	if err := os.WriteFile(filepath.Join(pluginsDir, "known_marketplaces.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	marketplaces, err := ListMarketplaces()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(marketplaces) != 1 {
		t.Fatalf("expected 1 marketplace, got %d", len(marketplaces))
	}

	mp := marketplaces[0]
	if mp.Name != "claude-code-plugins" {
		t.Errorf("expected name 'claude-code-plugins', got %q", mp.Name)
	}
	if mp.Source != "github" {
		t.Errorf("expected source 'github', got %q", mp.Source)
	}
	if mp.Repo != "anthropics/claude-code-plugins" {
		t.Errorf("expected repo 'anthropics/claude-code-plugins', got %q", mp.Repo)
	}
	if mp.LastUpdated.IsZero() {
		t.Error("expected LastUpdated to be set")
	}
}

func TestListMarketplaces_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	pluginsDir := filepath.Join(tmp, ".claude", "plugins")
	if err := os.MkdirAll(pluginsDir, 0700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(pluginsDir, "known_marketplaces.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ListMarketplaces()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestListPlugins_NoFiles(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	plugins, err := ListPlugins()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(plugins))
	}
}

func TestListPlugins_WithMarketplaceAndPlugins(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	claudeDir := filepath.Join(tmp, ".claude")
	pluginsDir := filepath.Join(claudeDir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Create marketplace install location with plugin directories
	mpInstallDir := filepath.Join(tmp, "marketplace-install")
	pluginDir := filepath.Join(mpInstallDir, "plugins", "test-plugin", ".claude-plugin")
	if err := os.MkdirAll(pluginDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Write plugin manifest
	manifest := map[string]string{
		"name":        "test-plugin",
		"version":     "1.0.0",
		"description": "A test plugin",
	}
	manifestData, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), manifestData, 0644); err != nil {
		t.Fatal(err)
	}

	// Write known_marketplaces.json
	knownData := map[string]any{
		"test-marketplace": map[string]any{
			"source": map[string]string{
				"source": "github",
				"repo":   "test/repo",
			},
			"installLocation": mpInstallDir,
			"lastUpdated":     time.Now().Format(time.RFC3339),
		},
	}
	data, _ := json.Marshal(knownData)
	if err := os.WriteFile(filepath.Join(pluginsDir, "known_marketplaces.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Write installed_plugins.json
	installedData := map[string]any{
		"version": 1,
		"plugins": map[string]any{
			"test-plugin@test-marketplace": []map[string]string{
				{
					"scope":       "global",
					"installPath": pluginDir,
					"version":     "1.0.0",
					"installedAt": time.Now().Format(time.RFC3339),
				},
			},
		},
	}
	installJSON, _ := json.Marshal(installedData)
	if err := os.WriteFile(filepath.Join(pluginsDir, "installed_plugins.json"), installJSON, 0644); err != nil {
		t.Fatal(err)
	}

	// Write settings.json with enabled plugin
	settingsData := map[string]any{
		"enabledPlugins": map[string]bool{
			"test-plugin@test-marketplace": true,
		},
	}
	settingsJSON, _ := json.Marshal(settingsData)
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), settingsJSON, 0644); err != nil {
		t.Fatal(err)
	}

	plugins, err := ListPlugins()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}

	p := plugins[0]
	if p.Name != "test-plugin" {
		t.Errorf("expected name 'test-plugin', got %q", p.Name)
	}
	if p.Marketplace != "test-marketplace" {
		t.Errorf("expected marketplace 'test-marketplace', got %q", p.Marketplace)
	}
	if p.FullName != "test-plugin@test-marketplace" {
		t.Errorf("expected fullName 'test-plugin@test-marketplace', got %q", p.FullName)
	}
	if p.Description != "A test plugin" {
		t.Errorf("expected description 'A test plugin', got %q", p.Description)
	}
	if !p.Installed {
		t.Error("expected Installed=true")
	}
	if !p.Enabled {
		t.Error("expected Enabled=true")
	}
	if p.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", p.Version)
	}
}

func TestListPlugins_NotInstalledNotEnabled(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	claudeDir := filepath.Join(tmp, ".claude")
	pluginsDir := filepath.Join(claudeDir, "plugins")
	if err := os.MkdirAll(pluginsDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Create marketplace with a plugin directory but no installed/enabled entries
	mpInstallDir := filepath.Join(tmp, "marketplace-install")
	pluginDir := filepath.Join(mpInstallDir, "plugins", "uninstalled-plugin")
	if err := os.MkdirAll(pluginDir, 0700); err != nil {
		t.Fatal(err)
	}

	knownData := map[string]any{
		"mp": map[string]any{
			"source":          map[string]string{"source": "github", "repo": "x/y"},
			"installLocation": mpInstallDir,
			"lastUpdated":     time.Now().Format(time.RFC3339),
		},
	}
	data, _ := json.Marshal(knownData)
	if err := os.WriteFile(filepath.Join(pluginsDir, "known_marketplaces.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	plugins, err := ListPlugins()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}

	p := plugins[0]
	if p.Installed {
		t.Error("expected Installed=false")
	}
	if p.Enabled {
		t.Error("expected Enabled=false")
	}
}

func TestListPlugins_SkipsNonDirectories(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	pluginsDir := filepath.Join(tmp, ".claude", "plugins")
	if err := os.MkdirAll(pluginsDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Create marketplace with a file (not dir) in the plugins folder
	mpInstallDir := filepath.Join(tmp, "marketplace-install")
	mpPluginsDir := filepath.Join(mpInstallDir, "plugins")
	if err := os.MkdirAll(mpPluginsDir, 0700); err != nil {
		t.Fatal(err)
	}
	// Write a regular file that should be skipped
	if err := os.WriteFile(filepath.Join(mpPluginsDir, "not-a-plugin.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	knownData := map[string]any{
		"mp": map[string]any{
			"source":          map[string]string{"source": "github", "repo": "x/y"},
			"installLocation": mpInstallDir,
			"lastUpdated":     time.Now().Format(time.RFC3339),
		},
	}
	data, _ := json.Marshal(knownData)
	if err := os.WriteFile(filepath.Join(pluginsDir, "known_marketplaces.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	plugins, err := ListPlugins()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("expected 0 plugins (only files, no dirs), got %d", len(plugins))
	}
}

func TestGetClaudeDir(t *testing.T) {
	dir := getClaudeDir()
	if dir == "" {
		t.Skip("could not determine home directory")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("expected absolute path, got %q", dir)
	}
	if filepath.Base(dir) != ".claude" {
		t.Errorf("expected .claude suffix, got %q", filepath.Base(dir))
	}
}
