package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/zhubert/plural/internal/logger"
)

// WorktreeStatus represents the status of changes in a worktree
type WorktreeStatus struct {
	HasChanges bool
	Summary    string     // Short summary like "3 files changed"
	Files      []string   // List of changed files
	Diff       string     // Full diff output
	FileDiffs  []FileDiff // Per-file diffs for detailed viewing
}

// FileDiff represents a single file's diff with its status
type FileDiff struct {
	Filename string // File path relative to repo root
	Status   string // Status code: M (modified), A (added), D (deleted), etc.
	Diff     string // Diff content for this file only
}

// DiffStats represents the statistics of changes in a worktree
type DiffStats struct {
	FilesChanged int // Number of files changed
	Additions    int // Number of lines added
	Deletions    int // Number of lines deleted
}

// GetWorktreeStatus returns the status of uncommitted changes in a worktree
func (s *GitService) GetWorktreeStatus(ctx context.Context, worktreePath string) (*WorktreeStatus, error) {
	status := &WorktreeStatus{}

	// Get list of changed files using git status --porcelain
	output, err := s.executor.Output(ctx, worktreePath, "git", "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}

	// Only trim trailing whitespace - leading space is significant in porcelain format
	// (e.g., " M file.go" means modified in worktree, the leading space is part of status)
	lines := strings.Split(strings.TrimRight(string(output), "\n\r\t "), "\n")
	if len(lines) == 1 && lines[0] == "" {
		// No changes
		status.HasChanges = false
		status.Summary = "No changes"
		return status, nil
	}

	status.HasChanges = true
	// Map to store status codes for each file
	fileStatuses := make(map[string]string)
	for _, line := range lines {
		if len(line) > 3 {
			filename := strings.TrimSpace(line[3:])
			status.Files = append(status.Files, filename)
			// Extract status code (first 2 chars: index status + worktree status)
			// Use the more significant status (prefer non-space)
			statusCode := strings.TrimSpace(line[:2])
			if statusCode == "" {
				statusCode = "M" // Default to modified
			} else if len(statusCode) == 2 {
				// Take the first non-space character
				if statusCode[0] != ' ' {
					statusCode = string(statusCode[0])
				} else {
					statusCode = string(statusCode[1])
				}
			}
			fileStatuses[filename] = statusCode
		}
	}

	// Generate summary
	fileCount := len(status.Files)
	if fileCount == 1 {
		status.Summary = "1 file changed"
	} else {
		status.Summary = fmt.Sprintf("%d files changed", fileCount)
	}

	log := logger.WithComponent("git")

	// Get diff (use --no-ext-diff to ensure output goes to stdout even if external diff is configured)
	diffOutput, err := s.executor.Output(ctx, worktreePath, "git", "diff", "--no-ext-diff", "HEAD")
	if err != nil {
		// If HEAD doesn't exist (new repo), try diff without HEAD
		log.Debug("diff HEAD failed, trying without HEAD", "error", err, "worktree", worktreePath)
		diffOutput, err = s.executor.Output(ctx, worktreePath, "git", "diff", "--no-ext-diff")
		if err != nil {
			log.Warn("git diff failed", "error", err, "worktree", worktreePath)
		}
	}

	// Also include staged changes in diff-like format
	cachedDiff, err := s.executor.Output(ctx, worktreePath, "git", "diff", "--no-ext-diff", "--cached")
	if err != nil {
		log.Warn("git diff --cached failed", "error", err, "worktree", worktreePath)
	}

	status.Diff = string(diffOutput) + string(cachedDiff)

	// Parse per-file diffs for detailed viewing
	status.FileDiffs = s.parseFileDiffs(ctx, worktreePath, status.Diff, status.Files, fileStatuses)

	return status, nil
}

// parseFileDiffs splits a combined diff into per-file chunks
func (s *GitService) parseFileDiffs(ctx context.Context, worktreePath, diff string, files []string, fileStatuses map[string]string) []FileDiff {
	if diff == "" {
		// No diff content from git diff - but untracked files need special handling
		result := make([]FileDiff, 0, len(files))
		for _, file := range files {
			status := fileStatuses[file]
			if status == "" {
				status = "M"
			}
			var diffContent string
			if status == "?" {
				diffContent = s.generateUntrackedFileDiff(ctx, worktreePath, file)
			} else {
				diffContent = "(no diff available)"
			}
			result = append(result, FileDiff{
				Filename: file,
				Status:   status,
				Diff:     diffContent,
			})
		}
		return result
	}

	// Split diff on "diff --git" markers
	chunks := strings.Split(diff, "diff --git ")
	fileDiffMap := make(map[string]string)

	for _, chunk := range chunks {
		if chunk == "" {
			continue
		}
		// Re-add the marker for proper formatting
		chunk = "diff --git " + chunk

		// Extract filename from "diff --git a/path b/path" line
		firstLine := strings.SplitN(chunk, "\n", 2)[0]
		// Parse "diff --git a/foo/bar.go b/foo/bar.go"
		parts := strings.Split(firstLine, " ")
		if len(parts) >= 4 {
			// Get the b/path part and strip the "b/" prefix
			bPath := parts[len(parts)-1]
			if strings.HasPrefix(bPath, "b/") {
				filename := bPath[2:]
				fileDiffMap[filename] = strings.TrimRight(chunk, "\n")
			}
		}
	}

	// Build result in the same order as files list
	result := make([]FileDiff, 0, len(files))
	for _, file := range files {
		status := fileStatuses[file]
		if status == "" {
			status = "M"
		}
		diffContent := fileDiffMap[file]
		if diffContent == "" {
			// For untracked files (status "?"), generate a diff showing the new file content
			if status == "?" {
				diffContent = s.generateUntrackedFileDiff(ctx, worktreePath, file)
			} else {
				diffContent = "(no diff available - file may be binary)"
			}
		}
		result = append(result, FileDiff{
			Filename: file,
			Status:   status,
			Diff:     diffContent,
		})
	}

	return result
}

// generateUntrackedFileDiff creates a diff-like output for an untracked file
func (s *GitService) generateUntrackedFileDiff(ctx context.Context, worktreePath, file string) string {
	// Use git diff --no-index to compare /dev/null with the new file
	// This produces a proper diff format showing the file as new
	output, err := s.executor.Output(ctx, worktreePath, "git", "diff", "--no-ext-diff", "--no-index", "/dev/null", file)
	if err != nil {
		// git diff --no-index returns exit code 1 when files differ, which is expected
		// Only treat it as an error if there's no output
		if len(output) == 0 {
			logger.WithComponent("git").Warn("failed to generate diff for untracked file", "file", file, "error", err)
			return "(no diff available - file may be binary)"
		}
	}
	return strings.TrimRight(string(output), "\n")
}

// GetDiffStats returns the diff statistics (files changed, additions, deletions)
// for uncommitted changes in the given worktree.
func (s *GitService) GetDiffStats(ctx context.Context, worktreePath string) (*DiffStats, error) {
	stats := &DiffStats{}

	// Get list of changed files using git status --porcelain
	output, err := s.executor.Output(ctx, worktreePath, "git", "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}

	// Count files from status output
	statusOutput := strings.TrimRight(string(output), "\n\r\t ")
	if statusOutput == "" {
		// No changes
		return stats, nil
	}

	// Track untracked files to count their lines separately
	var untrackedFiles []string

	lines := strings.Split(statusOutput, "\n")
	for _, line := range lines {
		if len(line) > 2 {
			stats.FilesChanged++
			// Check if this is an untracked file (status "??")
			if len(line) >= 2 && line[0] == '?' && line[1] == '?' {
				filename := strings.TrimSpace(line[3:])
				untrackedFiles = append(untrackedFiles, filename)
			}
		}
	}

	log := logger.WithComponent("git")

	// Get diff stats using git diff --numstat for unstaged changes
	// (without HEAD to get only working tree changes vs staged)
	numstatOutput, err := s.executor.Output(ctx, worktreePath, "git", "diff", "--numstat")
	if err != nil {
		log.Warn("git diff --numstat failed", "error", err, "worktree", worktreePath)
	}

	// Get staged changes separately
	cachedOutput, err := s.executor.Output(ctx, worktreePath, "git", "diff", "--numstat", "--cached")
	if err != nil {
		log.Warn("git diff --numstat --cached failed", "error", err, "worktree", worktreePath)
	}

	// Parse numstat output: each line is "additions<tab>deletions<tab>filename"
	parseNumstat := func(data []byte) {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Split(line, "\t")
			if len(parts) >= 2 {
				// Binary files show "-" for additions/deletions
				if parts[0] != "-" {
					var add int
					fmt.Sscanf(parts[0], "%d", &add)
					stats.Additions += add
				}
				if parts[1] != "-" {
					var del int
					fmt.Sscanf(parts[1], "%d", &del)
					stats.Deletions += del
				}
			}
		}
	}

	parseNumstat(numstatOutput)
	parseNumstat(cachedOutput)

	// Count lines in untracked files (all lines are additions)
	// git diff --numstat doesn't include untracked files
	for _, filename := range untrackedFiles {
		lineCount, err := s.countFileLines(ctx, worktreePath, filename)
		if err != nil {
			log.Warn("failed to count lines in untracked file", "file", filename, "error", err)
			continue
		}
		stats.Additions += lineCount
	}

	return stats, nil
}

// countFileLines counts the number of lines in a file using git diff --no-index.
// For binary files, returns 0.
func (s *GitService) countFileLines(ctx context.Context, worktreePath, filename string) (int, error) {
	// Use git diff --no-index --numstat to get line count for untracked file
	// This compares /dev/null with the file, giving us the additions count
	output, err := s.executor.Output(ctx, worktreePath, "git", "diff", "--no-index", "--numstat", "/dev/null", filename)
	if err != nil {
		// git diff --no-index returns exit code 1 when files differ, which is expected
		// Only treat it as an error if there's no output
		if len(output) == 0 {
			return 0, err
		}
	}

	// Parse numstat output: "additions<tab>deletions<tab>filename"
	line := strings.TrimSpace(string(output))
	if line == "" {
		return 0, nil
	}

	parts := strings.Split(line, "\t")
	if len(parts) >= 1 {
		// Binary files show "-" for additions
		if parts[0] == "-" {
			return 0, nil // Binary file
		}
		var count int
		fmt.Sscanf(parts[0], "%d", &count)
		return count, nil
	}

	return 0, nil
}

// GetConflictedFiles returns the list of files with merge conflicts in a repo
func (s *GitService) GetConflictedFiles(ctx context.Context, repoPath string) ([]string, error) {
	output, err := s.executor.Output(ctx, repoPath, "git", "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, fmt.Errorf("failed to get conflicted files: %w", err)
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr == "" {
		return nil, nil
	}

	files := strings.Split(outputStr, "\n")
	return files, nil
}

// IsMergeInProgress checks if a merge is currently in progress in the repo.
// It returns true if MERGE_HEAD exists (meaning there's an ongoing merge).
func (s *GitService) IsMergeInProgress(ctx context.Context, repoPath string) (bool, error) {
	_, _, err := s.executor.Run(ctx, repoPath, "git", "rev-parse", "--verify", "MERGE_HEAD")
	if err != nil {
		// MERGE_HEAD doesn't exist - no merge in progress
		return false, nil
	}
	return true, nil
}
