package config

import "os"

// SamePath returns true if a and b refer to the same filesystem entry.
// It handles case-insensitive filesystems (e.g. macOS APFS) and symlinks
// by comparing device+inode via os.SameFile. Falls back to exact string
// comparison when either path cannot be stat'd.
func SamePath(a, b string) bool {
	if a == b {
		return true
	}
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return false
	}
	return os.SameFile(infoA, infoB)
}

// resolveRepoPath returns the stored repo path that refers to the same
// filesystem entry as path. If no match is found, path is returned unchanged.
// Callers must manage their own locking before calling this function.
func resolveRepoPath(repos []string, path string) string {
	for _, r := range repos {
		if SamePath(r, path) {
			return r
		}
	}
	return path
}
