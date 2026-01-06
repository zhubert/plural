package session

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/logger"
)

// Create creates a new session with a git worktree for the given repo path
func Create(repoPath string) (*config.Session, error) {
	startTime := time.Now()
	logger.Log("Session: Creating new session for repo=%s", repoPath)

	// Generate UUID for this session
	id := uuid.New().String()
	shortID := id[:8]

	// Get repo name from path
	repoName := filepath.Base(repoPath)

	// Branch name: plural-<UUID>
	branch := fmt.Sprintf("plural-%s", id)

	// Worktree path: sibling to repo in .plural-worktrees directory
	repoParent := filepath.Dir(repoPath)
	worktreePath := filepath.Join(repoParent, ".plural-worktrees", id)

	// Create the worktree with a new branch
	logger.Log("Session: Creating git worktree: branch=%s, path=%s", branch, worktreePath)
	worktreeStart := time.Now()
	cmd := exec.Command("git", "worktree", "add", "-b", branch, worktreePath)
	cmd.Dir = repoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Log("Session: Failed to create worktree after %v: %s", time.Since(worktreeStart), string(output))
		return nil, fmt.Errorf("failed to create worktree: %s: %w", string(output), err)
	}
	logger.Log("Session: Git worktree created in %v", time.Since(worktreeStart))

	session := &config.Session{
		ID:        id,
		RepoPath:  repoPath,
		WorkTree:  worktreePath,
		Branch:    branch,
		Name:      fmt.Sprintf("%s/%s", repoName, shortID),
		CreatedAt: time.Now(),
	}

	logger.Log("Session: Session created successfully: id=%s, name=%s, total_time=%v", id, session.Name, time.Since(startTime))
	return session, nil
}

// ValidateRepo checks if a path is a valid git repository
func ValidateRepo(path string) error {
	logger.Log("Session: Validating repo path=%s", path)
	startTime := time.Now()

	// Expand ~ to home directory
	if strings.HasPrefix(path, "~") {
		logger.Log("Session: Validation failed - path uses ~")
		return fmt.Errorf("please use absolute path instead of ~")
	}

	// Check if it's a git repo by running git rev-parse
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = path

	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Log("Session: Validation failed after %v - not a git repo: %s", time.Since(startTime), strings.TrimSpace(string(output)))
		return fmt.Errorf("not a git repository: %s", strings.TrimSpace(string(output)))
	}

	logger.Log("Session: Repo validated successfully in %v", time.Since(startTime))
	return nil
}

// GetWorktreeDir returns the worktrees directory path for a repo
func GetWorktreeDir(repoPath string) string {
	repoParent := filepath.Dir(repoPath)
	return filepath.Join(repoParent, ".plural-worktrees")
}

// GetGitRoot returns the git root directory for a path, or empty string if not a git repo
func GetGitRoot(path string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = path

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}

// GetCurrentDirGitRoot returns the git root of the current working directory
func GetCurrentDirGitRoot() string {
	return GetGitRoot(".")
}
