package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/logger"
)

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
func (s *SessionService) BranchExists(ctx context.Context, repoPath, branch string) bool {
	_, _, err := s.executor.Run(ctx, repoPath, "git", "rev-parse", "--verify", branch)
	return err == nil
}

// getCurrentBranchName returns the current branch name for the repo
// Returns "HEAD" as fallback if it cannot be determined
func (s *SessionService) getCurrentBranchName(ctx context.Context, repoPath string) string {
	output, err := s.executor.Output(ctx, repoPath, "git", "rev-parse", "--abbrev-ref", "HEAD")
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
func (s *SessionService) GetDefaultBranch(ctx context.Context, repoPath string) string {
	// Try to get the default branch from origin's HEAD reference
	output, err := s.executor.Output(ctx, repoPath, "git", "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		// Output is like "refs/remotes/origin/main"
		ref := strings.TrimSpace(string(output))
		if strings.HasPrefix(ref, "refs/remotes/origin/") {
			return strings.TrimPrefix(ref, "refs/remotes/origin/")
		}
	}

	// Fallback: check if origin/main exists
	_, _, err = s.executor.Run(ctx, repoPath, "git", "rev-parse", "--verify", "origin/main")
	if err == nil {
		return "main"
	}

	// Fallback: check if origin/master exists
	_, _, err = s.executor.Run(ctx, repoPath, "git", "rev-parse", "--verify", "origin/master")
	if err == nil {
		return "master"
	}

	// Last resort fallback
	return "main"
}

// FetchOrigin fetches the latest changes from origin
// Returns nil if successful, or if there's no remote (local-only repo)
func (s *SessionService) FetchOrigin(ctx context.Context, repoPath string) error {
	log := logger.WithComponent("session")

	// First check if origin remote exists
	_, _, err := s.executor.Run(ctx, repoPath, "git", "remote", "get-url", "origin")
	if err != nil {
		// No origin remote - this is a local-only repo, which is fine
		log.Info("no origin remote found, skipping fetch", "repoPath", repoPath)
		return nil
	}

	log.Info("fetching from origin", "repoPath", repoPath)
	output, err := s.executor.CombinedOutput(ctx, repoPath, "git", "fetch", "origin")
	if err != nil {
		log.Warn("failed to fetch from origin", "repoPath", repoPath, "output", string(output))
		// Don't fail session creation if fetch fails - just log a warning
		// This allows offline usage and handles network issues gracefully
		return nil
	}
	log.Info("fetch completed successfully", "repoPath", repoPath)
	return nil
}

// Create creates a new session with a git worktree for the given repo path.
// If customBranch is provided, it will be used as the branch name; otherwise
// a branch named "plural-<UUID>" will be created.
// The branchPrefix is prepended to auto-generated branch names (e.g., "zhubert/").
// The basePoint specifies where to branch from:
//   - BasePointOrigin: fetches from origin and branches from origin's default branch
//   - BasePointHead: branches from the current local HEAD
func (s *SessionService) Create(ctx context.Context, repoPath string, customBranch string, branchPrefix string, basePoint BasePoint) (*config.Session, error) {
	log := logger.WithComponent("session")
	startTime := time.Now()
	log.Info("creating new session",
		"repoPath", repoPath,
		"customBranch", customBranch,
		"branchPrefix", branchPrefix,
		"basePoint", string(basePoint))

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
		s.FetchOrigin(ctx, repoPath)

		// Prefer origin's default branch if it exists, otherwise fall back to HEAD
		defaultBranch := s.GetDefaultBranch(ctx, repoPath)
		startPoint = fmt.Sprintf("origin/%s", defaultBranch)
		baseBranch = defaultBranch

		// Check if the remote branch exists
		_, _, err := s.executor.Run(ctx, repoPath, "git", "rev-parse", "--verify", startPoint)
		if err != nil {
			// Remote branch doesn't exist (local-only repo), fall back to HEAD
			log.Info("remote branch not found, falling back to HEAD", "startPoint", startPoint)
			startPoint = "HEAD"
			baseBranch = s.getCurrentBranchName(ctx, repoPath)
		}
	case BasePointHead:
		fallthrough
	default:
		// Use current branch (HEAD)
		startPoint = "HEAD"
		baseBranch = s.getCurrentBranchName(ctx, repoPath)
		log.Info("using current branch as base", "baseBranch", baseBranch)
	}

	// Create the worktree with a new branch based on the start point
	log.Info("creating git worktree",
		"branch", branch,
		"worktreePath", worktreePath,
		"startPoint", startPoint)
	worktreeStart := time.Now()
	output, err := s.executor.CombinedOutput(ctx, repoPath, "git", "worktree", "add", "-b", branch, worktreePath, startPoint)
	if err != nil {
		log.Error("failed to create worktree",
			"duration", time.Since(worktreeStart),
			"output", string(output),
			"error", err)
		return nil, fmt.Errorf("failed to create worktree: %s: %w", string(output), err)
	}
	log.Debug("git worktree created", "duration", time.Since(worktreeStart))

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

	log.Info("session created successfully",
		"sessionID", id,
		"name", session.Name,
		"baseBranch", baseBranch,
		"duration", time.Since(startTime))
	return session, nil
}

// CreateFromBranch creates a new session forked from a specific branch.
// This is used when forking an existing session - the new worktree is created
// from the source branch's current state rather than from origin/main.
// If customBranch is provided, it will be used as the new branch name; otherwise
// a branch named "plural-<UUID>" will be created.
func (s *SessionService) CreateFromBranch(ctx context.Context, repoPath string, sourceBranch string, customBranch string, branchPrefix string) (*config.Session, error) {
	log := logger.WithComponent("session")
	startTime := time.Now()
	log.Info("creating forked session",
		"repoPath", repoPath,
		"sourceBranch", sourceBranch,
		"customBranch", customBranch,
		"branchPrefix", branchPrefix)

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
	log.Info("creating git worktree",
		"branch", branch,
		"worktreePath", worktreePath,
		"sourceBranch", sourceBranch)
	worktreeStart := time.Now()
	output, err := s.executor.CombinedOutput(ctx, repoPath, "git", "worktree", "add", "-b", branch, worktreePath, sourceBranch)
	if err != nil {
		log.Error("failed to create forked worktree",
			"duration", time.Since(worktreeStart),
			"output", string(output),
			"error", err)
		return nil, fmt.Errorf("failed to create worktree: %s: %w", string(output), err)
	}
	log.Debug("git worktree created", "duration", time.Since(worktreeStart))

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

	log.Info("forked session created successfully",
		"sessionID", id,
		"name", session.Name,
		"baseBranch", sourceBranch,
		"duration", time.Since(startTime))
	return session, nil
}

// ValidateRepo checks if a path is a valid git repository
func (s *SessionService) ValidateRepo(ctx context.Context, path string) error {
	log := logger.WithComponent("session")
	log.Info("validating repo", "path", path)
	startTime := time.Now()

	// Expand ~ to home directory
	if strings.HasPrefix(path, "~") {
		log.Info("validation failed - path uses tilde", "path", path)
		return fmt.Errorf("please use absolute path instead of ~")
	}

	// Check if it's a git repo by running git rev-parse
	output, err := s.executor.CombinedOutput(ctx, path, "git", "rev-parse", "--git-dir")
	if err != nil {
		log.Info("validation failed - not a git repo",
			"path", path,
			"duration", time.Since(startTime),
			"output", strings.TrimSpace(string(output)))
		return fmt.Errorf("not a git repository: %s", strings.TrimSpace(string(output)))
	}

	log.Info("repo validated successfully", "path", path, "duration", time.Since(startTime))
	return nil
}

// GetGitRoot returns the git root directory for a path, or empty string if not a git repo
func (s *SessionService) GetGitRoot(ctx context.Context, path string) string {
	output, err := s.executor.Output(ctx, path, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}

// GetCurrentDirGitRoot returns the git root of the current working directory
func (s *SessionService) GetCurrentDirGitRoot(ctx context.Context) string {
	return s.GetGitRoot(ctx, ".")
}

// Delete removes a session's git worktree and branch
func (s *SessionService) Delete(ctx context.Context, sess *config.Session) error {
	log := logger.WithComponent("session")
	log.Info("deleting worktree",
		"sessionID", sess.ID,
		"worktree", sess.WorkTree,
		"branch", sess.Branch)

	// Remove the worktree
	output, err := s.executor.CombinedOutput(ctx, sess.RepoPath, "git", "worktree", "remove", sess.WorkTree, "--force")
	if err != nil {
		log.Error("failed to remove worktree", "output", string(output), "error", err)
		return fmt.Errorf("failed to remove worktree: %s: %w", string(output), err)
	}
	log.Info("worktree removed successfully", "sessionID", sess.ID)

	// Prune worktree references (best-effort cleanup)
	if output, err := s.executor.CombinedOutput(ctx, sess.RepoPath, "git", "worktree", "prune"); err != nil {
		log.Warn("worktree prune failed (best-effort)", "output", string(output), "error", err)
	}

	// Delete the branch
	branchOutput, err := s.executor.CombinedOutput(ctx, sess.RepoPath, "git", "branch", "-D", sess.Branch)
	if err != nil {
		log.Warn("failed to delete branch (may already be deleted)", "output", string(branchOutput))
		// Don't return error - the worktree is already gone, branch deletion is best-effort
	} else {
		log.Debug("branch deleted successfully", "branch", sess.Branch)
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
// that don't have a matching session in config.
// Directory scans are parallelized for better performance with many repos.
func FindOrphanedWorktrees(cfg *config.Config) ([]OrphanedWorktree, error) {
	log := logger.WithComponent("session")
	log.Info("searching for orphaned worktrees")

	// Build a set of known session IDs
	knownSessions := make(map[string]bool)
	for _, sess := range cfg.GetSessions() {
		knownSessions[sess.ID] = true
	}

	// Get all repo paths from config
	repoPaths := cfg.GetRepos()
	if len(repoPaths) == 0 {
		log.Info("no repos in config, checking common locations")
	}

	// Build set of repo paths for filtering
	// Resolve symlinks for consistent path comparison (e.g., /tmp vs /private/tmp on macOS)
	repoPathsSet := make(map[string]bool)
	for _, repoPath := range repoPaths {
		// Add both the original and resolved paths for maximum compatibility
		repoPathsSet[repoPath] = true
		if resolved, err := filepath.EvalSymlinks(repoPath); err == nil {
			repoPathsSet[resolved] = true
		}
	}

	// Build list of unique .plural-worktrees directories to check
	// Note: Multiple repos can share the same parent directory and thus
	// the same .plural-worktrees directory. We deduplicate the directories
	// but let getWorktreeRepoPath() determine which repo each worktree belongs to.
	checkedDirs := make(map[string]bool)
	var dirsToCheck []string

	for _, repoPath := range repoPaths {
		repoParent := filepath.Dir(repoPath)
		worktreesDir := filepath.Join(repoParent, ".plural-worktrees")

		if checkedDirs[worktreesDir] {
			continue
		}
		checkedDirs[worktreesDir] = true
		dirsToCheck = append(dirsToCheck, worktreesDir)
	}

	if len(dirsToCheck) == 0 {
		return nil, nil
	}

	// Scan directories in parallel
	var mu sync.Mutex
	var orphans []OrphanedWorktree

	var wg sync.WaitGroup
	for _, worktreesDir := range dirsToCheck {
		wg.Add(1)
		go func(worktreesDir string) {
			defer wg.Done()

			orphansInDir, err := findOrphansInDir(worktreesDir, knownSessions, repoPathsSet)
			if err != nil {
				return // Skip if directory doesn't exist or can't be read
			}

			mu.Lock()
			orphans = append(orphans, orphansInDir...)
			mu.Unlock()
		}(worktreesDir)
	}

	wg.Wait()
	log.Info("orphaned worktree search complete", "count", len(orphans))
	return orphans, nil
}

// getWorktreeRepoPath determines which repository a worktree belongs to
// by reading the .git file in the worktree, which points to the main repo's
// .git/worktrees/<name> directory.
func getWorktreeRepoPath(worktreePath string) (string, error) {
	gitFile := filepath.Join(worktreePath, ".git")
	content, err := os.ReadFile(gitFile)
	if err != nil {
		return "", fmt.Errorf("failed to read .git file: %w", err)
	}

	// Content is like: "gitdir: /path/to/repo/.git/worktrees/uuid"
	line := strings.TrimSpace(string(content))
	if !strings.HasPrefix(line, "gitdir: ") {
		return "", fmt.Errorf("invalid .git file format: %s", line)
	}

	gitdir := strings.TrimPrefix(line, "gitdir: ")

	// Handle relative paths by resolving against the worktree path
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(worktreePath, gitdir)
	}

	// gitdir is like: /path/to/repo/.git/worktrees/uuid
	// We want: /path/to/repo
	parts := strings.Split(filepath.Clean(gitdir), string(filepath.Separator))

	// Find the .git component and take everything before it
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == ".git" {
			repoPath := filepath.Join(string(filepath.Separator), filepath.Join(parts[:i]...))

			// Resolve symlinks for consistent path comparison (e.g., /tmp vs /private/tmp on macOS)
			repoPath, err = filepath.EvalSymlinks(repoPath)
			if err != nil {
				// If we can't resolve symlinks, use the path as-is
				return filepath.Join(string(filepath.Separator), filepath.Join(parts[:i]...)), nil
			}
			return repoPath, nil
		}
	}

	return "", fmt.Errorf("could not find .git directory in path: %s", gitdir)
}

func findOrphansInDir(worktreesDir string, knownSessions map[string]bool, repoPathsSet map[string]bool) ([]OrphanedWorktree, error) {
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
			worktreePath := filepath.Join(worktreesDir, sessionID)

			// Determine which repo this worktree actually belongs to
			repoPath, err := getWorktreeRepoPath(worktreePath)
			if err != nil {
				// If we can't determine the repo, skip this worktree
				// (it might be corrupted or not a valid git worktree)
				continue
			}

			// Only include orphans that belong to repos in our config
			if repoPathsSet[repoPath] {
				orphans = append(orphans, OrphanedWorktree{
					Path:     worktreePath,
					RepoPath: repoPath,
					ID:       sessionID,
				})
			}
		}
	}

	return orphans, nil
}

// detectWorktreeBranch determines the actual branch name for a worktree by
// running "git rev-parse --abbrev-ref HEAD" inside it. This handles all branch
// naming patterns: default (plural-<UUID>), prefixed (user/plural-<UUID>), and
// renamed branches.
func detectWorktreeBranch(ctx context.Context, s *SessionService, orphan OrphanedWorktree) string {
	stdout, _, err := s.executor.Run(ctx, orphan.Path, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	branch := strings.TrimSpace(string(stdout))
	if branch == "" || branch == "HEAD" {
		// "HEAD" means detached HEAD state - no branch to delete
		return ""
	}
	return branch
}

// PruneOrphanedWorktrees removes all orphaned worktrees and their branches.
// Pruning operations are parallelized across repos, but serialized within each repo
// to avoid concurrent git operations on the same repository.
func (s *SessionService) PruneOrphanedWorktrees(ctx context.Context, cfg *config.Config) (int, error) {
	log := logger.WithComponent("session")

	orphans, err := FindOrphanedWorktrees(cfg)
	if err != nil {
		return 0, err
	}

	if len(orphans) == 0 {
		return 0, nil
	}

	// Group orphans by repo to avoid concurrent git operations on the same repo
	orphansByRepo := make(map[string][]OrphanedWorktree)
	for _, orphan := range orphans {
		orphansByRepo[orphan.RepoPath] = append(orphansByRepo[orphan.RepoPath], orphan)
	}

	var mu sync.Mutex
	pruned := 0

	// Process repos in parallel, but orphans within each repo sequentially
	var wg sync.WaitGroup
	for repoPath, repoOrphans := range orphansByRepo {
		wg.Add(1)
		go func(repoPath string, repoOrphans []OrphanedWorktree) {
			defer wg.Done()

			for _, orphan := range repoOrphans {
				log.Info("pruning orphaned worktree", "path", orphan.Path)

				// Detect the actual branch name before removing the worktree.
				// Sessions can have prefixed branches (e.g., "zhubert/plural-<UUID>")
				// or custom names after rename, so we can't assume "plural-<UUID>".
				branchName := detectWorktreeBranch(ctx, s, orphan)

				// Try to remove via git worktree remove first
				_, _, err := s.executor.Run(ctx, orphan.RepoPath, "git", "worktree", "remove", orphan.Path, "--force")
				if err != nil {
					// If git command fails, try direct removal
					log.Warn("git worktree remove failed, trying direct removal", "path", orphan.Path)
					if err := os.RemoveAll(orphan.Path); err != nil {
						log.Error("failed to remove orphan", "path", orphan.Path, "error", err)
						continue
					}
				}

				// Prune worktree references (best-effort cleanup)
				if _, _, pruneErr := s.executor.Run(ctx, orphan.RepoPath, "git", "worktree", "prune"); pruneErr != nil {
					log.Warn("worktree prune failed (best-effort)", "repoPath", orphan.RepoPath, "error", pruneErr)
				}

				// Try to delete the branch
				if branchName != "" {
					if _, _, branchErr := s.executor.Run(ctx, orphan.RepoPath, "git", "branch", "-D", branchName); branchErr != nil {
						log.Warn("failed to delete branch (may already be deleted)", "branch", branchName, "error", branchErr)
					}
				} else {
					log.Warn("could not detect branch name for orphan, skipping branch deletion", "sessionID", orphan.ID)
				}

				// Delete session messages file
				if err := config.DeleteSessionMessages(orphan.ID); err != nil {
					log.Warn("failed to delete session messages", "sessionID", orphan.ID, "error", err)
				} else {
					log.Info("deleted session messages", "sessionID", orphan.ID)
				}

				mu.Lock()
				pruned++
				mu.Unlock()
				log.Info("pruned orphan", "path", orphan.Path)
			}
		}(repoPath, repoOrphans)
	}

	wg.Wait()
	return pruned, nil
}
