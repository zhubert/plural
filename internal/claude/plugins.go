package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zhubert/plural/internal/logger"
)

// Marketplace represents a plugin marketplace
type Marketplace struct {
	Name        string
	Source      string // "github" or "url"
	Repo        string // e.g., "anthropics/claude-code-plugins"
	LastUpdated time.Time
}

// Plugin represents a plugin with its status
type Plugin struct {
	Name        string // e.g., "frontend-design"
	Marketplace string // e.g., "claude-code-plugins"
	FullName    string // e.g., "frontend-design@claude-code-plugins"
	Description string
	Installed   bool
	Enabled     bool
	Version     string
}

// getClaudeDir returns the Claude config directory path
func getClaudeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// knownMarketplacesFile is the structure of ~/.claude/plugins/known_marketplaces.json
type knownMarketplacesFile map[string]struct {
	Source struct {
		Source string `json:"source"` // "github" or "url"
		Repo   string `json:"repo"`   // e.g., "anthropics/claude-code"
	} `json:"source"`
	InstallLocation string `json:"installLocation"`
	LastUpdated     string `json:"lastUpdated"`
}

// installedPluginsFile is the structure of ~/.claude/plugins/installed_plugins.json
type installedPluginsFile struct {
	Version int `json:"version"`
	Plugins map[string][]struct {
		Scope       string `json:"scope"`
		InstallPath string `json:"installPath"`
		Version     string `json:"version"`
		InstalledAt string `json:"installedAt"`
	} `json:"plugins"`
}

// settingsFile is the structure of ~/.claude/settings.json (partial)
type settingsFile struct {
	EnabledPlugins map[string]bool `json:"enabledPlugins"`
}

// pluginManifest is the structure of .claude-plugin/plugin.json
type pluginManifest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

// ListMarketplaces returns configured marketplaces by reading the config files
func ListMarketplaces() ([]Marketplace, error) {
	log := logger.WithComponent("plugins")
	log.Debug("listing marketplaces")

	claudeDir := getClaudeDir()
	if claudeDir == "" {
		return nil, fmt.Errorf("could not find home directory")
	}

	knownPath := filepath.Join(claudeDir, "plugins", "known_marketplaces.json")
	data, err := os.ReadFile(knownPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug("no marketplaces file found")
			return []Marketplace{}, nil
		}
		return nil, fmt.Errorf("failed to read marketplaces: %w", err)
	}

	var known knownMarketplacesFile
	if err := json.Unmarshal(data, &known); err != nil {
		return nil, fmt.Errorf("failed to parse marketplaces: %w", err)
	}

	var marketplaces []Marketplace
	for name, info := range known {
		mp := Marketplace{
			Name:   name,
			Source: info.Source.Source,
			Repo:   info.Source.Repo,
		}
		if info.LastUpdated != "" {
			if t, err := time.Parse(time.RFC3339, info.LastUpdated); err == nil {
				mp.LastUpdated = t
			}
		}
		marketplaces = append(marketplaces, mp)
	}

	log.Debug("found marketplaces", "count", len(marketplaces))
	return marketplaces, nil
}

// ListPlugins returns all plugins with their status by reading config files and marketplace directories
func ListPlugins() ([]Plugin, error) {
	log := logger.WithComponent("plugins")
	log.Debug("listing plugins")

	claudeDir := getClaudeDir()
	if claudeDir == "" {
		return nil, fmt.Errorf("could not find home directory")
	}

	// Load installed plugins
	installedPath := filepath.Join(claudeDir, "plugins", "installed_plugins.json")
	installed := make(map[string]string) // fullName -> version
	if data, err := os.ReadFile(installedPath); err == nil {
		var installFile installedPluginsFile
		if err := json.Unmarshal(data, &installFile); err == nil {
			for fullName, installs := range installFile.Plugins {
				if len(installs) > 0 {
					installed[fullName] = installs[0].Version
				}
			}
		}
	}
	log.Debug("found installed plugins", "count", len(installed))

	// Load enabled plugins from settings
	settingsPath := filepath.Join(claudeDir, "settings.json")
	enabled := make(map[string]bool)
	if data, err := os.ReadFile(settingsPath); err == nil {
		var settings settingsFile
		if err := json.Unmarshal(data, &settings); err == nil {
			enabled = settings.EnabledPlugins
		}
	}
	log.Debug("found enabled plugins", "count", len(enabled))

	// Load marketplace locations
	knownPath := filepath.Join(claudeDir, "plugins", "known_marketplaces.json")
	marketplaces := make(map[string]string) // name -> installLocation
	if data, err := os.ReadFile(knownPath); err == nil {
		var known knownMarketplacesFile
		if err := json.Unmarshal(data, &known); err == nil {
			for name, info := range known {
				marketplaces[name] = info.InstallLocation
			}
		}
	}

	// Build plugin list from marketplace directories
	var plugins []Plugin
	seenPlugins := make(map[string]bool) // track which plugins we've already added

	for mpName, mpPath := range marketplaces {
		pluginsDir := filepath.Join(mpPath, "plugins")
		entries, err := os.ReadDir(pluginsDir)
		if err != nil {
			log.Debug("could not read marketplace plugins dir", "path", pluginsDir, "error", err)
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			pluginName := entry.Name()
			fullName := pluginName + "@" + mpName

			// Skip if we've already seen this plugin
			if seenPlugins[fullName] {
				continue
			}
			seenPlugins[fullName] = true

			// Try to read plugin manifest
			manifestPath := filepath.Join(pluginsDir, pluginName, ".claude-plugin", "plugin.json")
			var description, version string
			if data, err := os.ReadFile(manifestPath); err == nil {
				var manifest pluginManifest
				if err := json.Unmarshal(data, &manifest); err == nil {
					description = manifest.Description
					version = manifest.Version
				}
			}

			// Determine plugin status
			isInstalled := false
			isEnabled := false
			if _, ok := installed[fullName]; ok {
				isInstalled = true
				version = installed[fullName]
			}
			if enabled[fullName] {
				isEnabled = true
			}

			plugins = append(plugins, Plugin{
				Name:        pluginName,
				Marketplace: mpName,
				FullName:    fullName,
				Description: description,
				Installed:   isInstalled,
				Enabled:     isEnabled,
				Version:     version,
			})
		}
	}

	log.Debug("found total plugins", "count", len(plugins))
	return plugins, nil
}

// AddMarketplace adds a marketplace from GitHub repo or URL
func AddMarketplace(source string) error {
	log := logger.WithComponent("plugins")
	log.Info("adding marketplace", "source", source)

	cmd := exec.Command("claude", "plugin", "marketplace", "add", source)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add marketplace: %s", strings.TrimSpace(string(output)))
	}

	log.Info("successfully added marketplace", "source", source)
	return nil
}

// RemoveMarketplace removes a marketplace by name
func RemoveMarketplace(name string) error {
	log := logger.WithComponent("plugins")
	log.Info("removing marketplace", "name", name)

	cmd := exec.Command("claude", "plugin", "marketplace", "remove", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove marketplace: %s", strings.TrimSpace(string(output)))
	}

	log.Info("successfully removed marketplace", "name", name)
	return nil
}

// UpdateMarketplace updates a marketplace (or all if name is empty)
func UpdateMarketplace(name string) error {
	log := logger.WithComponent("plugins")

	var cmd *exec.Cmd
	if name == "" {
		log.Info("updating all marketplaces")
		cmd = exec.Command("claude", "plugin", "marketplace", "update")
	} else {
		log.Info("updating marketplace", "name", name)
		cmd = exec.Command("claude", "plugin", "marketplace", "update", name)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to update marketplace: %s", strings.TrimSpace(string(output)))
	}

	log.Info("successfully updated marketplace(s)")
	return nil
}

// InstallPlugin installs a plugin
func InstallPlugin(fullName string) error {
	log := logger.WithComponent("plugins")
	log.Info("installing plugin", "plugin", fullName)

	cmd := exec.Command("claude", "plugin", "install", fullName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install plugin: %s", strings.TrimSpace(string(output)))
	}

	log.Info("successfully installed plugin", "plugin", fullName)
	return nil
}

// UninstallPlugin removes a plugin
func UninstallPlugin(fullName string) error {
	log := logger.WithComponent("plugins")
	log.Info("uninstalling plugin", "plugin", fullName)

	cmd := exec.Command("claude", "plugin", "uninstall", fullName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to uninstall plugin: %s", strings.TrimSpace(string(output)))
	}

	log.Info("successfully uninstalled plugin", "plugin", fullName)
	return nil
}

// EnablePlugin enables a plugin
func EnablePlugin(fullName string) error {
	log := logger.WithComponent("plugins")
	log.Info("enabling plugin", "plugin", fullName)

	cmd := exec.Command("claude", "plugin", "enable", fullName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable plugin: %s", strings.TrimSpace(string(output)))
	}

	log.Info("successfully enabled plugin", "plugin", fullName)
	return nil
}

// DisablePlugin disables a plugin
func DisablePlugin(fullName string) error {
	log := logger.WithComponent("plugins")
	log.Info("disabling plugin", "plugin", fullName)

	cmd := exec.Command("claude", "plugin", "disable", fullName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to disable plugin: %s", strings.TrimSpace(string(output)))
	}

	log.Info("successfully disabled plugin", "plugin", fullName)
	return nil
}
