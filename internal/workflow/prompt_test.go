package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSystemPrompt(t *testing.T) {
	// Create a temp dir with a prompt file
	dir := t.TempDir()
	promptDir := filepath.Join(dir, "prompts")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(promptDir, "coding.md"), []byte("Be careful with tests"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		prompt   string
		repoPath string
		want     string
		wantErr  bool
	}{
		{
			name:     "empty prompt",
			prompt:   "",
			repoPath: dir,
			want:     "",
		},
		{
			name:     "inline prompt",
			prompt:   "Be careful with tests",
			repoPath: dir,
			want:     "Be careful with tests",
		},
		{
			name:     "file prompt",
			prompt:   "file:prompts/coding.md",
			repoPath: dir,
			want:     "Be careful with tests",
		},
		{
			name:     "file prompt with dot slash",
			prompt:   "file:./prompts/coding.md",
			repoPath: dir,
			want:     "Be careful with tests",
		},
		{
			name:     "file not found",
			prompt:   "file:./nonexistent.md",
			repoPath: dir,
			wantErr:  true,
		},
		{
			name:     "path traversal blocked",
			prompt:   "file:../../etc/passwd",
			repoPath: dir,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveSystemPrompt(tt.prompt, tt.repoPath)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
