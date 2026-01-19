package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zhubert/plural/internal/config"
	pexec "github.com/zhubert/plural/internal/exec"
	"github.com/zhubert/plural/internal/logger"
)

// executor is the command executor used by this package.
// It can be swapped for testing/demos via SetExecutor.
var executor pexec.CommandExecutor = pexec.NewRealExecutor()

// SetExecutor sets the command executor used by this package.
// This is primarily used for testing and demo generation.
func SetExecutor(e pexec.CommandExecutor) {
	executor = e
}

// GetExecutor returns the current command executor.
func GetExecutor() pexec.CommandExecutor {
	return executor
}

// BasePoint specifies where to branch from when creating a new session
type BasePoint string

const (
	// BasePointOrigin branches from origin's default branch (after fetching)
	BasePointOrigin BasePoint = "origin"
	// BasePointHead branches from the current local HEAD
	BasePointHead BasePoint = "head"
)

// MaxBranchNameValidation is the maximum length for user-provided branch names.
// This is more permissive than git.MaxBranchNameLength which is for auto-generated names.
const MaxBranchNameValidation = 100

// validBranchNameRegex matches valid git branch name characters
// Git branch names cannot contain: space, ~, ^, :, ?, *, [, \, or control characters
// They also cannot start with - or end with .lock
var validBranchNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9/_.-]*$`)

// ValidateBranchName checks if a branch name is valid for git
func ValidateBranchName(branch string) error {
	if branch == "" {
		return nil // Empty is allowed (will use default)
	}

	if len(branch) > MaxBranchNameValidation {
		return fmt.Errorf("branch name too long (max %d characters)", MaxBranchNameValidation)
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
	_, _, err := executor.Run(context.Background(), repoPath, "git", "rev-parse", "--verify", branch)
	return err == nil
}

// getCurrentBranchName returns the current branch name for the repo
// Returns "HEAD" as fallback if it cannot be determined
func getCurrentBranchName(repoPath string) string {
	output, err := executor.Output(context.Background(), repoPath, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		branch := strings.TrimSpace(string(output))
		if branch != "" && branch != "HEAD" {
			return branch
		}
	}
	return "HEAD"
}

// GetDefaultBranch returns the default branch name for the remote (e.g., "main" or "master")
// Returns "main" as fallback if it cannot be determined
func GetDefaultBranch(repoPath string) string {
	ctx := context.Background()

	// Try to get the default branch from origin's HEAD reference
	output, err := executor.Output(ctx, repoPath, "git", "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		// Output is like "refs/remotes/origin/main"
		ref := strings.TrimSpace(string(output))
		if strings.HasPrefix(ref, "refs/remotes/origin/") {
			return strings.TrimPrefix(ref, "refs/remotes/origin/")
		}
	}

	// Fallback: check if origin/main exists
	_, _, err = executor.Run(ctx, repoPath, "git", "rev-parse", "--verify", "origin/main")
	if err == nil {
		return "main"
	}

	// Fallback: check if origin/master exists
	_, _, err = executor.Run(ctx, repoPath, "git", "rev-parse", "--verify", "origin/master")
	if err == nil {
		return "master"
	}

	// Last resort fallback
	return "main"
}

// FetchOrigin fetches the latest changes from origin
// Returns nil if successful, or if there's no remote (local-only repo)
func FetchOrigin(repoPath string) error {
	ctx := context.Background()

	// First check if origin remote exists
	_, _, err := executor.Run(ctx, repoPath, "git", "remote", "get-url", "origin")
	if err != nil {
		// No origin remote - this is a local-only repo, which is fine
		logger.Log("Session: No origin remote found, skipping fetch")
		return nil
	}

	logger.Log("Session: Fetching from origin")
	output, err := executor.CombinedOutput(ctx, repoPath, "git", "fetch", "origin")
	if err != nil {
		logger.Warn("Session: Failed to fetch from origin: %s", string(output))
		// Don't fail session creation if fetch fails - just log a warning
		// This allows offline usage and handles network issues gracefully
		return nil
	}
	logger.Log("Session: Fetch completed successfully")
	return nil
}

// Create creates a new session with a git worktree for the given repo path.
// If customBranch is provided, it will be used as the branch name; otherwise
// a branch named "plural-<UUID>" will be created.
// The branchPrefix is prepended to auto-generated branch names (e.g., "zhubert/").
// The basePoint specifies where to branch from:
//   - BasePointOrigin: fetches from origin and branches from origin's default branch
//   - BasePointHead: branches from the current local HEAD
func Create(repoPath string, customBranch string, branchPrefix string, basePoint BasePoint) (*config.Session, error) {
	startTime := time.Now()
	logger.Log("Session: Creating new session for repo=%s, customBranch=%q, branchPrefix=%q, basePoint=%v", repoPath, customBranch, branchPrefix, basePoint)

	// Generate UUID for this session
	id := uuid.New().String()
	shortID := id[:8]

	// Get repo name from path
	repoName := filepath.Base(repoPath)

	// Branch name: use custom if provided, otherwise plural-<UUID>
	// Apply branchPrefix to auto-generated branch names
	var branch string
	if customBranch != "" {
		branch = branchPrefix + customBranch
	} else {
		branch = branchPrefix + fmt.Sprintf("plural-%s", id)
	}

	// Worktree path: sibling to repo in .plural-worktrees directory
	repoParent := filepath.Dir(repoPath)
	worktreePath := filepath.Join(repoParent, ".plural-worktrees", id)

	// Determine the starting point for the new branch
	var startPoint string
	var baseBranch string // The branch name to display as the base
	switch basePoint {
	case BasePointOrigin:
		// Fetch from origin to ensure we have the latest commits
		FetchOrigin(repoPath)

		// Prefer origin's default branch if it exists, otherwise fall back to HEAD
		defaultBranch := GetDefaultBranch(repoPath)
		startPoint = fmt.Sprintf("origin/%s", defaultBranch)
		baseBranch = defaultBranch

		// Check if the remote branch exists
		_, _, err := executor.Run(context.Background(), repoPath, "git", "rev-parse", "--verify", startPoint)
		if err != nil {
			// Remote branch doesn't exist (local-only repo), fall back to HEAD
			logger.Log("Session: Remote branch %s not found, falling back to HEAD", startPoint)
			startPoint = "HEAD"
			baseBranch = getCurrentBranchName(repoPath)
		}
	case BasePointHead:
		fallthrough
	default:
		// Use current branch (HEAD)
		startPoint = "HEAD"
		baseBranch = getCurrentBranchName(repoPath)
		logger.Log("Session: Using current branch (HEAD) as base")
	}

	// Create the worktree with a new branch based on the start point
	logger.Log("Session: Creating git worktree: branch=%s, path=%s, from=%s", branch, worktreePath, startPoint)
	worktreeStart := time.Now()
	output, err := executor.CombinedOutput(context.Background(), repoPath, "git", "worktree", "add", "-b", branch, worktreePath, startPoint)
	if err != nil {
		logger.Error("Session: Failed to create worktree after %v: %s", time.Since(worktreeStart), string(output))
		return nil, fmt.Errorf("failed to create worktree: %s: %w", string(output), err)
	}
	logger.Debug("Session: Git worktree created in %v", time.Since(worktreeStart))

	// Display name: use the full branch name for clarity
	var displayName string
	if customBranch != "" {
		displayName = branchPrefix + customBranch
	} else {
		// For auto-generated branches, just show the short ID (prefix is visible in branch name)
		if branchPrefix != "" {
			displayName = branchPrefix + shortID
		} else {
			displayName = shortID
		}
	}

	session := &config.Session{
		ID:         id,
		RepoPath:   repoPath,
		WorkTree:   worktreePath,
		Branch:     branch,
		BaseBranch: baseBranch,
		Name:       fmt.Sprintf("%s/%s", repoName, displayName),
		CreatedAt:  time.Now(),
	}

	logger.Info("Session: Session created successfully: id=%s, name=%s, base=%s, total_time=%v", id, session.Name, baseBranch, time.Since(startTime))
	return session, nil
}

// CreateFromBranch creates a new session forked from a specific branch.
// This is used when forking an existing session - the new worktree is created
// from the source branch's current state rather than from origin/main.
// If customBranch is provided, it will be used as the new branch name; otherwise
// a branch named "plural-<UUID>" will be created.
func CreateFromBranch(repoPath string, sourceBranch string, customBranch string, branchPrefix string) (*config.Session, error) {
	startTime := time.Now()
	logger.Log("Session: Creating forked session for repo=%s, sourceBranch=%q, customBranch=%q, branchPrefix=%q",
		repoPath, sourceBranch, customBranch, branchPrefix)

	// Generate UUID for this session
	id := uuid.New().String()
	shortID := id[:8]

	// Get repo name from path
	repoName := filepath.Base(repoPath)

	// Branch name: use custom if provided, otherwise plural-<UUID>
	var branch string
	if customBranch != "" {
		branch = branchPrefix + customBranch
	} else {
		branch = branchPrefix + fmt.Sprintf("plural-%s", id)
	}

	// Worktree path: sibling to repo in .plural-worktrees directory
	repoParent := filepath.Dir(repoPath)
	worktreePath := filepath.Join(repoParent, ".plural-worktrees", id)

	// Create the worktree with a new branch based on the source branch
	logger.Log("Session: Creating git worktree: branch=%s, path=%s, from=%s", branch, worktreePath, sourceBranch)
	worktreeStart := time.Now()
	output, err := executor.CombinedOutput(context.Background(), repoPath, "git", "worktree", "add", "-b", branch, worktreePath, sourceBranch)
	if err != nil {
		logger.Error("Session: Failed to create forked worktree after %v: %s", time.Since(worktreeStart), string(output))
		return nil, fmt.Errorf("failed to create worktree: %s: %w", string(output), err)
	}
	logger.Debug("Session: Git worktree created in %v", time.Since(worktreeStart))

	// Display name: use the full branch name for clarity
	var displayName string
	if customBranch != "" {
		displayName = branchPrefix + customBranch
	} else {
		if branchPrefix != "" {
			displayName = branchPrefix + shortID
		} else {
			displayName = shortID
		}
	}

	session := &config.Session{
		ID:         id,
		RepoPath:   repoPath,
		WorkTree:   worktreePath,
		Branch:     branch,
		BaseBranch: sourceBranch,
		Name:       fmt.Sprintf("%s/%s", repoName, displayName),
		CreatedAt:  time.Now(),
	}

	logger.Info("Session: Forked session created successfully: id=%s, name=%s, base=%s, total_time=%v",
		id, session.Name, sourceBranch, time.Since(startTime))
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
	output, err := executor.CombinedOutput(context.Background(), path, "git", "rev-parse", "--git-dir")
	if err != nil {
		logger.Log("Session: Validation failed after %v - not a git repo: %s", time.Since(startTime), strings.TrimSpace(string(output)))
		return fmt.Errorf("not a git repository: %s", strings.TrimSpace(string(output)))
	}

	logger.Log("Session: Repo validated successfully in %v", time.Since(startTime))
	return nil
}

// GetGitRoot returns the git root directory for a path, or empty string if not a git repo
func GetGitRoot(path string) string {
	output, err := executor.Output(context.Background(), path, "git", "rev-parse", "--show-toplevel")
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

	ctx := context.Background()

	// Remove the worktree
	output, err := executor.CombinedOutput(ctx, sess.RepoPath, "git", "worktree", "remove", sess.WorkTree, "--force")
	if err != nil {
		logger.Error("Session: Failed to remove worktree: %s", string(output))
		return fmt.Errorf("failed to remove worktree: %s: %w", string(output), err)
	}
	logger.Info("Session: Worktree removed successfully")

	// Prune worktree references (best-effort cleanup)
	if output, err := executor.CombinedOutput(ctx, sess.RepoPath, "git", "worktree", "prune"); err != nil {
		logger.Warn("Session: Worktree prune failed (best-effort): %s - %v", string(output), err)
	}

	// Delete the branch
	branchOutput, err := executor.CombinedOutput(ctx, sess.RepoPath, "git", "branch", "-D", sess.Branch)
	if err != nil {
		logger.Warn("Session: Failed to delete branch (may already be deleted): %s", string(branchOutput))
		// Don't return error - the worktree is already gone, branch deletion is best-effort
	} else {
		logger.Debug("Session: Branch deleted successfully")
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

	ctx := context.Background()
	pruned := 0
	for _, orphan := range orphans {
		logger.Log("Session: Pruning orphaned worktree: %s", orphan.Path)

		// Try to remove via git worktree remove first
		_, _, err := executor.Run(ctx, orphan.RepoPath, "git", "worktree", "remove", orphan.Path, "--force")
		if err != nil {
			// If git command fails, try direct removal
			logger.Warn("Session: git worktree remove failed, trying direct removal")
			if err := os.RemoveAll(orphan.Path); err != nil {
				logger.Error("Session: Failed to remove orphan %s: %v", orphan.Path, err)
				continue
			}
		}

		// Prune worktree references
		executor.Run(ctx, orphan.RepoPath, "git", "worktree", "prune")

		// Try to delete the branch
		branchName := fmt.Sprintf("plural-%s", orphan.ID)
		executor.Run(ctx, orphan.RepoPath, "git", "branch", "-D", branchName)

		// Delete session messages file
		if err := config.DeleteSessionMessages(orphan.ID); err != nil {
			logger.Warn("Session: Failed to delete session messages for %s: %v", orphan.ID, err)
		} else {
			logger.Log("Session: Deleted session messages for: %s", orphan.ID)
		}

		pruned++
		logger.Log("Session: Pruned orphan: %s", orphan.Path)
	}

	return pruned, nil
}
