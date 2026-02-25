package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSamePath_IdenticalStrings(t *testing.T) {
	// Fast path: exact string match, no stat needed
	// Use a path that definitely doesn't exist to prove stat isn't called
	if !SamePath("/nonexistent/identical/path", "/nonexistent/identical/path") {
		t.Error("SamePath should return true for identical strings")
	}
}

func TestSamePath_DifferentDirs(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()

	if SamePath(dirA, dirB) {
		t.Error("SamePath should return false for different directories")
	}
}

func TestSamePath_Symlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if !SamePath(target, link) {
		t.Error("SamePath should return true for symlink to same directory")
	}
}

func TestSamePath_NonExistent(t *testing.T) {
	dir := t.TempDir()

	// Both missing
	if SamePath("/no/such/pathA", "/no/such/pathB") {
		t.Error("SamePath should return false when both paths are missing")
	}

	// One missing
	if SamePath(dir, "/no/such/path") {
		t.Error("SamePath should return false when one path is missing")
	}
	if SamePath("/no/such/path", dir) {
		t.Error("SamePath should return false when one path is missing")
	}
}

func TestSamePath_CaseSensitivity(t *testing.T) {
	// Create a temp dir and test whether the FS is case-insensitive
	dir := t.TempDir()
	sub := filepath.Join(dir, "TestDir")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}

	variant := filepath.Join(dir, "testdir")

	// Detect FS behavior: if stat succeeds on the case-variant, FS is case-insensitive
	_, err := os.Stat(variant)
	caseInsensitive := err == nil

	got := SamePath(sub, variant)
	if got != caseInsensitive {
		t.Errorf("SamePath(%q, %q) = %v; expected %v (caseInsensitive=%v)",
			sub, variant, got, caseInsensitive, caseInsensitive)
	}
}

func TestResolveRepoPath_Match(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "repo")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(dir, "repo-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	repos := []string{target}
	got := resolveRepoPath(repos, link)
	if got != target {
		t.Errorf("resolveRepoPath returned %q, want stored path %q", got, target)
	}
}

func TestResolveRepoPath_NoMatch(t *testing.T) {
	repos := []string{"/stored/repo1", "/stored/repo2"}
	input := "/other/repo"
	got := resolveRepoPath(repos, input)
	if got != input {
		t.Errorf("resolveRepoPath returned %q, want input %q", got, input)
	}
}

func TestResolveRepoPath_CaseVariant(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "MyRepo")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}

	variant := filepath.Join(dir, "myrepo")

	// Only test on case-insensitive FS
	if _, err := os.Stat(variant); err != nil {
		t.Skip("filesystem is case-sensitive, skipping case-variant test")
	}

	repos := []string{sub}
	got := resolveRepoPath(repos, variant)
	if got != sub {
		t.Errorf("resolveRepoPath returned %q, want stored path %q", got, sub)
	}
}

func TestResolveRepoPath_EmptyRepos(t *testing.T) {
	input := "/some/path"
	got := resolveRepoPath(nil, input)
	if got != input {
		t.Errorf("resolveRepoPath with nil repos returned %q, want %q", got, input)
	}
	got = resolveRepoPath([]string{}, input)
	if got != input {
		t.Errorf("resolveRepoPath with empty repos returned %q, want %q", got, input)
	}
}

func TestSamePath_SameDir(t *testing.T) {
	dir := t.TempDir()
	if !SamePath(dir, dir) {
		t.Error("SamePath should return true for same directory")
	}
}

func TestSamePath_TrailingSlash(t *testing.T) {
	dir := t.TempDir()
	// filepath.Clean removes trailing slash; test that string mismatch still works
	withSlash := dir + "/"
	// Strings differ, but os.Stat resolves both to the same inode
	if !SamePath(dir, withSlash) {
		t.Error("SamePath should return true for path with trailing slash")
	}
}

func TestSamePath_DotDot(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "child")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}

	// dir/child/.. should resolve to dir
	dotdot := filepath.Join(sub, "..")
	if !SamePath(dir, dotdot) {
		t.Error("SamePath should return true for path with .. that resolves to same dir")
	}
}

func TestSamePath_EmptyStrings(t *testing.T) {
	// Both empty: fast path returns true (identical strings)
	if !SamePath("", "") {
		t.Error("SamePath should return true for two empty strings")
	}

	// One empty, one real path: stat("") fails, so should return false
	dir := t.TempDir()
	if SamePath("", dir) {
		t.Error("SamePath should return false when one path is empty and the other exists")
	}
}

func TestResolveRepoPath_ExactMatchPreferred(t *testing.T) {
	// When the exact string matches (fast path in SamePath), it should still
	// return the stored path from the repos slice.
	repos := []string{"/exact/path"}
	got := resolveRepoPath(repos, "/exact/path")
	if got != "/exact/path" {
		t.Errorf("resolveRepoPath returned %q, want %q", got, "/exact/path")
	}
}

func TestResolveRepoPath_FirstMatchWins(t *testing.T) {
	// If somehow multiple stored paths point to the same inode (shouldn't happen
	// with AddRepo dedup, but test behavior), the first match wins.
	dir := t.TempDir()
	target := filepath.Join(dir, "repo")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}

	link1 := filepath.Join(dir, "link1")
	link2 := filepath.Join(dir, "link2")
	if err := os.Symlink(target, link1); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link2); err != nil {
		t.Fatal(err)
	}

	repos := []string{link1, link2}
	got := resolveRepoPath(repos, target)
	if got != link1 {
		t.Errorf("resolveRepoPath returned %q, want first match %q", got, link1)
	}
}

func TestSamePath_NestedSymlinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}

	link1 := filepath.Join(dir, "link1")
	if err := os.Symlink(target, link1); err != nil {
		t.Fatal(err)
	}

	link2 := filepath.Join(dir, "link2")
	if err := os.Symlink(link1, link2); err != nil {
		t.Fatal(err)
	}

	// link2 -> link1 -> target: all same inode
	if !SamePath(target, link2) {
		t.Error("SamePath should return true for chained symlinks")
	}

	// Ensure strings are actually different
	if target == link2 {
		t.Fatal("test setup error: paths should differ")
	}
	if !strings.Contains(link2, "link2") {
		t.Fatal("test setup error")
	}
}
