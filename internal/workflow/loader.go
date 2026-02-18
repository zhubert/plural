package workflow

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const workflowFileName = "workflow.yaml"
const workflowDir = ".plural"

// Load reads and parses .plural/workflow.yaml from the given repo path.
// Returns nil, nil if the file does not exist.
func Load(repoPath string) (*Config, error) {
	fp := filepath.Join(repoPath, workflowDir, workflowFileName)

	data, err := os.ReadFile(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read workflow config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse workflow config: %w", err)
	}

	return &cfg, nil
}

// LoadAndMerge loads the workflow config and merges with defaults.
// If no workflow file exists, returns the default config.
func LoadAndMerge(repoPath string) (*Config, error) {
	cfg, err := Load(repoPath)
	if err != nil {
		return nil, err
	}

	defaults := DefaultConfig()
	if cfg == nil {
		return defaults, nil
	}

	return Merge(cfg, defaults), nil
}
