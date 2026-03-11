package claudeconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetClaudeConfigDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	tests := []struct {
		name    string
		envVal  string
		want    string
		wantErr bool
	}{
		{
			name:   "env var set returns env var value",
			envVal: "/custom/claude/config",
			want:   "/custom/claude/config",
		},
		{
			name:   "env var unset returns ~/.claude",
			envVal: "",
			want:   filepath.Join(home, ".claude"),
		},
		{
			name:   "env var with trailing slash is returned as-is",
			envVal: "/custom/dir/",
			want:   "/custom/dir/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CONFIG_DIR", tt.envVal)

			got, err := GetClaudeConfigDir()
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetClaudeConfigDir() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("GetClaudeConfigDir() = %q, want %q", got, tt.want)
			}
		})
	}
}
