package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveSystemPrompt resolves a system prompt value.
// If it starts with "file:", the path is read relative to repoPath.
// Otherwise, the string is returned as-is.
func ResolveSystemPrompt(prompt, repoPath string) (string, error) {
	if prompt == "" {
		return "", nil
	}

	if !strings.HasPrefix(prompt, "file:") {
		return prompt, nil
	}

	relPath := strings.TrimPrefix(prompt, "file:")
	absPath := filepath.Join(repoPath, relPath)

	// Ensure the resolved path is within the repo
	absPath, err := filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path %q: %w", relPath, err)
	}

	repoAbs, err := filepath.Abs(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve repo path: %w", err)
	}

	if !strings.HasPrefix(absPath, repoAbs+string(filepath.Separator)) && absPath != repoAbs {
		return "", fmt.Errorf("prompt file %q escapes repository root", relPath)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to read prompt file %q: %w", relPath, err)
	}

	return string(data), nil
}
