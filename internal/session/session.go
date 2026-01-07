package session

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/logger"
)

// validBranchNameRegex matches valid git branch name characters
// Git branch names cannot contain: space, ~, ^, :, ?, *, [, \, or control characters
// They also cannot start with - or end with .lock
var validBranchNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9/_.-]*$`)

// ValidateBranchName checks if a branch name is valid for git
func ValidateBranchName(branch string) error {
	if branch == "" {
		return nil // Empty is allowed (will use default)
	}

	if len(branch) > 100 {
		return fmt.Errorf("branch name too long (max 100 characters)")
	}

	if strings.HasPrefix(branch, "-") {
		return fmt.Errorf("branch name cannot start with '-'")
	}

	if strings.HasSuffix(branch, ".lock") {
		return fmt.Errorf("branch name cannot end with '.lock'")
	}

	if strings.Contains(branch, "..") {
		return fmt.Errorf("branch name cannot contain '..'")
	}

	if !validBranchNameRegex.MatchString(branch) {
		return fmt.Errorf("branch name contains invalid characters (use letters, numbers, /, _, ., -)")
	}

	return nil
}

// BranchExists checks if a branch already exists in the repo
func BranchExists(repoPath, branch string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", branch)
	cmd.Dir = repoPath
	return cmd.Run() == nil
}

// Create creates a new session with a git worktree for the given repo path.
// If customBranch is provided, it will be used as the branch name; otherwise
// a branch named "plural-<UUID>" will be created.
func Create(repoPath string, customBranch string) (*config.Session, error) {
	startTime := time.Now()
	logger.Log("Session: Creating new session for repo=%s, customBranch=%q", repoPath, customBranch)

	// Generate UUID for this session
	id := uuid.New().String()
	shortID := id[:8]

	// Get repo name from path
	repoName := filepath.Base(repoPath)

	// Branch name: use custom if provided, otherwise plural-<UUID>
	var branch string
	if customBranch != "" {
		branch = customBranch
	} else {
		branch = fmt.Sprintf("plural-%s", id)
	}

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

	// Display name: use custom branch if provided, otherwise use short UUID
	var displayName string
	if customBranch != "" {
		displayName = customBranch
	} else {
		displayName = shortID
	}

	session := &config.Session{
		ID:        id,
		RepoPath:  repoPath,
		WorkTree:  worktreePath,
		Branch:    branch,
		Name:      fmt.Sprintf("%s/%s", repoName, displayName),
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

// Delete removes a session's git worktree and branch
func Delete(sess *config.Session) error {
	logger.Log("Session: Deleting worktree for session=%s, worktree=%s, branch=%s", sess.ID, sess.WorkTree, sess.Branch)

	// Remove the worktree
	cmd := exec.Command("git", "worktree", "remove", sess.WorkTree, "--force")
	cmd.Dir = sess.RepoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Log("Session: Failed to remove worktree: %s", string(output))
		return fmt.Errorf("failed to remove worktree: %s: %w", string(output), err)
	}
	logger.Log("Session: Worktree removed successfully")

	// Prune worktree references
	pruneCmd := exec.Command("git", "worktree", "prune")
	pruneCmd.Dir = sess.RepoPath
	pruneCmd.Run() // Ignore errors, this is just cleanup

	// Delete the branch
	branchCmd := exec.Command("git", "branch", "-D", sess.Branch)
	branchCmd.Dir = sess.RepoPath

	branchOutput, err := branchCmd.CombinedOutput()
	if err != nil {
		logger.Log("Session: Failed to delete branch (may already be deleted): %s", string(branchOutput))
		// Don't return error - the worktree is already gone, branch deletion is best-effort
	} else {
		logger.Log("Session: Branch deleted successfully")
	}

	return nil
}

// OrphanedWorktree represents a worktree that has no matching session
type OrphanedWorktree struct {
	Path     string // Full path to the worktree
	RepoPath string // Parent repo path (derived from .plural-worktrees location)
	ID       string // Session ID (directory name)
}

// FindOrphanedWorktrees finds all worktrees in .plural-worktrees directories
// that don't have a matching session in config
func FindOrphanedWorktrees(cfg *config.Config) ([]OrphanedWorktree, error) {
	logger.Log("Session: Searching for orphaned worktrees")

	// Build a set of known session IDs
	knownSessions := make(map[string]bool)
	for _, sess := range cfg.GetSessions() {
		knownSessions[sess.ID] = true
	}

	var orphans []OrphanedWorktree

	// Get all repo paths from config
	repoPaths := cfg.GetRepos()
	if len(repoPaths) == 0 {
		logger.Log("Session: No repos in config, checking common locations")
	}

	// Check .plural-worktrees directories next to each repo
	checkedDirs := make(map[string]bool)
	for _, repoPath := range repoPaths {
		repoParent := filepath.Dir(repoPath)
		worktreesDir := filepath.Join(repoParent, ".plural-worktrees")

		if checkedDirs[worktreesDir] {
			continue
		}
		checkedDirs[worktreesDir] = true

		orphansInDir, err := findOrphansInDir(worktreesDir, repoPath, knownSessions)
		if err != nil {
			continue // Skip if directory doesn't exist or can't be read
		}
		orphans = append(orphans, orphansInDir...)
	}

	logger.Log("Session: Found %d orphaned worktrees", len(orphans))
	return orphans, nil
}

func findOrphansInDir(worktreesDir, repoPath string, knownSessions map[string]bool) ([]OrphanedWorktree, error) {
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return nil, err
	}

	var orphans []OrphanedWorktree
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		sessionID := entry.Name()
		if !knownSessions[sessionID] {
			orphans = append(orphans, OrphanedWorktree{
				Path:     filepath.Join(worktreesDir, sessionID),
				RepoPath: repoPath,
				ID:       sessionID,
			})
		}
	}

	return orphans, nil
}

// PruneOrphanedWorktrees removes all orphaned worktrees and their branches
func PruneOrphanedWorktrees(cfg *config.Config) (int, error) {
	orphans, err := FindOrphanedWorktrees(cfg)
	if err != nil {
		return 0, err
	}

	pruned := 0
	for _, orphan := range orphans {
		logger.Log("Session: Pruning orphaned worktree: %s", orphan.Path)

		// Try to remove via git worktree remove first
		cmd := exec.Command("git", "worktree", "remove", orphan.Path, "--force")
		cmd.Dir = orphan.RepoPath
		if err := cmd.Run(); err != nil {
			// If git command fails, try direct removal
			logger.Log("Session: git worktree remove failed, trying direct removal")
			if err := os.RemoveAll(orphan.Path); err != nil {
				logger.Log("Session: Failed to remove orphan %s: %v", orphan.Path, err)
				continue
			}
		}

		// Prune worktree references
		pruneCmd := exec.Command("git", "worktree", "prune")
		pruneCmd.Dir = orphan.RepoPath
		pruneCmd.Run()

		// Try to delete the branch
		branchName := fmt.Sprintf("plural-%s", orphan.ID)
		branchCmd := exec.Command("git", "branch", "-D", branchName)
		branchCmd.Dir = orphan.RepoPath
		branchCmd.Run() // Ignore errors

		pruned++
		logger.Log("Session: Pruned orphan: %s", orphan.Path)
	}

	return pruned, nil
}
