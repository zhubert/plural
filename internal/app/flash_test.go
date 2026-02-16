package app

import (
	"path/filepath"
	"testing"
)

func TestSaveConfigOrFlash_Success(t *testing.T) {
	cfg := testConfig()
	// Use a temp file so Save() succeeds
	cfg.SetFilePath(filepath.Join(t.TempDir(), "config.json"))
	m := testModelWithSize(cfg, 120, 40)

	cmd := m.saveConfigOrFlash()
	if cmd != nil {
		t.Error("expected nil cmd on successful save, got non-nil")
	}
}

func TestSaveConfigOrFlash_Error(t *testing.T) {
	cfg := testConfig()
	// Use a path that will fail (directory doesn't exist)
	cfg.SetFilePath("/nonexistent/directory/config.json")
	m := testModelWithSize(cfg, 120, 40)

	cmd := m.saveConfigOrFlash()
	if cmd == nil {
		t.Error("expected non-nil cmd on failed save, got nil")
	}
}
