package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfig_AddRepo(t *testing.T) {
	cfg := &Config{
		Repos:    []string{},
		Sessions: []Session{},
	}

	// Test adding a new repo
	if !cfg.AddRepo("/path/to/repo1") {
		t.Error("AddRepo should return true for new repo")
	}

	if len(cfg.Repos) != 1 {
		t.Errorf("Expected 1 repo, got %d", len(cfg.Repos))
	}

	// Test adding duplicate repo
	if cfg.AddRepo("/path/to/repo1") {
		t.Error("AddRepo should return false for duplicate repo")
	}

	if len(cfg.Repos) != 1 {
		t.Errorf("Expected 1 repo after duplicate add, got %d", len(cfg.Repos))
	}

	// Test adding another repo
	if !cfg.AddRepo("/path/to/repo2") {
		t.Error("AddRepo should return true for new repo")
	}

	if len(cfg.Repos) != 2 {
		t.Errorf("Expected 2 repos, got %d", len(cfg.Repos))
	}
}

func TestConfig_RemoveRepo(t *testing.T) {
	cfg := &Config{
		Repos:    []string{"/path/to/repo1", "/path/to/repo2"},
		Sessions: []Session{},
	}

	// Test removing existing repo
	if !cfg.RemoveRepo("/path/to/repo1") {
		t.Error("RemoveRepo should return true for existing repo")
	}

	if len(cfg.Repos) != 1 {
		t.Errorf("Expected 1 repo, got %d", len(cfg.Repos))
	}

	// Test removing non-existent repo
	if cfg.RemoveRepo("/path/to/nonexistent") {
		t.Error("RemoveRepo should return false for non-existent repo")
	}
}

func TestConfig_AddSession(t *testing.T) {
	cfg := &Config{
		Repos:    []string{},
		Sessions: []Session{},
	}

	sess := Session{
		ID:        "test-session-1",
		RepoPath:  "/path/to/repo",
		WorkTree:  "/path/to/worktree",
		Branch:    "plural-test",
		Name:      "test/session",
		CreatedAt: time.Now(),
	}

	cfg.AddSession(sess)

	if len(cfg.Sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(cfg.Sessions))
	}

	if cfg.Sessions[0].ID != "test-session-1" {
		t.Errorf("Expected session ID 'test-session-1', got '%s'", cfg.Sessions[0].ID)
	}
}

func TestConfig_RemoveSession(t *testing.T) {
	cfg := &Config{
		Repos: []string{},
		Sessions: []Session{
			{ID: "session-1", RepoPath: "/path", WorkTree: "/wt", Branch: "b1"},
			{ID: "session-2", RepoPath: "/path", WorkTree: "/wt", Branch: "b2"},
		},
	}

	// Test removing existing session
	if !cfg.RemoveSession("session-1") {
		t.Error("RemoveSession should return true for existing session")
	}

	if len(cfg.Sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(cfg.Sessions))
	}

	// Test removing non-existent session
	if cfg.RemoveSession("nonexistent") {
		t.Error("RemoveSession should return false for non-existent session")
	}
}

func TestConfig_GetSession(t *testing.T) {
	cfg := &Config{
		Repos: []string{},
		Sessions: []Session{
			{ID: "session-1", RepoPath: "/path1", WorkTree: "/wt1", Branch: "b1"},
			{ID: "session-2", RepoPath: "/path2", WorkTree: "/wt2", Branch: "b2"},
		},
	}

	// Test getting existing session
	sess := cfg.GetSession("session-1")
	if sess == nil {
		t.Error("GetSession should return session for existing ID")
	}
	if sess.RepoPath != "/path1" {
		t.Errorf("Expected repo path '/path1', got '%s'", sess.RepoPath)
	}

	// Test getting non-existent session
	sess = cfg.GetSession("nonexistent")
	if sess != nil {
		t.Error("GetSession should return nil for non-existent ID")
	}
}

func TestConfig_AllowedTools(t *testing.T) {
	cfg := &Config{
		Repos: []string{},
		Sessions: []Session{
			{ID: "session-1", RepoPath: "/path", WorkTree: "/wt", Branch: "b1"},
		},
	}

	// Test adding allowed tool
	if !cfg.AddAllowedTool("session-1", "Edit") {
		t.Error("AddAllowedTool should return true for new tool")
	}

	// Test adding duplicate tool
	if cfg.AddAllowedTool("session-1", "Edit") {
		t.Error("AddAllowedTool should return false for duplicate tool")
	}

	// Test getting allowed tools
	tools := cfg.GetAllowedTools("session-1")
	if len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tools))
	}
	if tools[0] != "Edit" {
		t.Errorf("Expected tool 'Edit', got '%s'", tools[0])
	}

	// Test IsToolAllowed
	if !cfg.IsToolAllowed("session-1", "Edit") {
		t.Error("IsToolAllowed should return true for allowed tool")
	}
	if cfg.IsToolAllowed("session-1", "Bash") {
		t.Error("IsToolAllowed should return false for non-allowed tool")
	}

	// Test adding tool to non-existent session
	if cfg.AddAllowedTool("nonexistent", "Edit") {
		t.Error("AddAllowedTool should return false for non-existent session")
	}
}

func TestConfig_MarkSessionStarted(t *testing.T) {
	cfg := &Config{
		Repos: []string{},
		Sessions: []Session{
			{ID: "session-1", RepoPath: "/path", WorkTree: "/wt", Branch: "b1", Started: false},
		},
	}

	// Test marking existing session as started
	if !cfg.MarkSessionStarted("session-1") {
		t.Error("MarkSessionStarted should return true for existing session")
	}

	sess := cfg.GetSession("session-1")
	if !sess.Started {
		t.Error("Session should be marked as started")
	}

	// Test marking non-existent session
	if cfg.MarkSessionStarted("nonexistent") {
		t.Error("MarkSessionStarted should return false for non-existent session")
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Repos: []string{"/path/to/repo"},
				Sessions: []Session{
					{ID: "session-1", RepoPath: "/path", WorkTree: "/wt", Branch: "b1"},
				},
			},
			wantErr: false,
		},
		{
			name: "empty config",
			config: &Config{
				Repos:    []string{},
				Sessions: []Session{},
			},
			wantErr: false,
		},
		{
			name: "nil slices",
			config: &Config{
				Repos:    nil,
				Sessions: nil,
			},
			wantErr: false, // Should auto-fix nil slices
		},
		{
			name: "duplicate session ID",
			config: &Config{
				Repos: []string{},
				Sessions: []Session{
					{ID: "session-1", RepoPath: "/path1", WorkTree: "/wt1", Branch: "b1"},
					{ID: "session-1", RepoPath: "/path2", WorkTree: "/wt2", Branch: "b2"},
				},
			},
			wantErr: true,
		},
		{
			name: "empty session ID",
			config: &Config{
				Repos: []string{},
				Sessions: []Session{
					{ID: "", RepoPath: "/path", WorkTree: "/wt", Branch: "b1"},
				},
			},
			wantErr: true,
		},
		{
			name: "empty repo path in session",
			config: &Config{
				Repos: []string{},
				Sessions: []Session{
					{ID: "session-1", RepoPath: "", WorkTree: "/wt", Branch: "b1"},
				},
			},
			wantErr: true,
		},
		{
			name: "empty worktree path",
			config: &Config{
				Repos: []string{},
				Sessions: []Session{
					{ID: "session-1", RepoPath: "/path", WorkTree: "", Branch: "b1"},
				},
			},
			wantErr: true,
		},
		{
			name: "empty branch",
			config: &Config{
				Repos: []string{},
				Sessions: []Session{
					{ID: "session-1", RepoPath: "/path", WorkTree: "/wt", Branch: ""},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate repos",
			config: &Config{
				Repos:    []string{"/path/to/repo", "/path/to/repo"},
				Sessions: []Session{},
			},
			wantErr: true,
		},
		{
			name: "empty repo path",
			config: &Config{
				Repos:    []string{"", "/path/to/repo"},
				Sessions: []Session{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_GetRepos(t *testing.T) {
	cfg := &Config{
		Repos:    []string{"/path/to/repo1", "/path/to/repo2"},
		Sessions: []Session{},
	}

	repos := cfg.GetRepos()

	// Verify we get a copy
	if len(repos) != 2 {
		t.Errorf("Expected 2 repos, got %d", len(repos))
	}

	// Modify the returned slice and verify original is unchanged
	repos[0] = "/modified"
	if cfg.Repos[0] == "/modified" {
		t.Error("GetRepos should return a copy, not the original slice")
	}
}

func TestConfig_GetSessions(t *testing.T) {
	cfg := &Config{
		Repos: []string{},
		Sessions: []Session{
			{ID: "session-1", RepoPath: "/path1", WorkTree: "/wt1", Branch: "b1"},
			{ID: "session-2", RepoPath: "/path2", WorkTree: "/wt2", Branch: "b2"},
		},
	}

	sessions := cfg.GetSessions()

	// Verify we get a copy
	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(sessions))
	}

	// Modify the returned slice and verify original is unchanged
	sessions[0].ID = "modified"
	if cfg.Sessions[0].ID == "modified" {
		t.Error("GetSessions should return a copy, not the original slice")
	}
}

func TestConfig_ClearSessions(t *testing.T) {
	cfg := &Config{
		Repos: []string{"/path"},
		Sessions: []Session{
			{ID: "session-1", RepoPath: "/path", WorkTree: "/wt", Branch: "b1"},
			{ID: "session-2", RepoPath: "/path", WorkTree: "/wt", Branch: "b2"},
		},
	}

	cfg.ClearSessions()

	if len(cfg.Sessions) != 0 {
		t.Errorf("Expected 0 sessions after ClearSessions, got %d", len(cfg.Sessions))
	}

	// Repos should be unchanged
	if len(cfg.Repos) != 1 {
		t.Errorf("Expected repos to be unchanged, got %d", len(cfg.Repos))
	}
}

func TestSessionMessages(t *testing.T) {
	// Create a temporary directory for test
	tmpDir, err := os.MkdirTemp("", "plural-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Override the sessions directory for testing
	// This is a bit hacky but necessary for testing without modifying home dir
	sessionID := "test-session-123"

	messages := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
	}

	// Test saving messages
	err = SaveSessionMessages(sessionID, messages, 100)
	if err != nil {
		t.Errorf("SaveSessionMessages failed: %v", err)
	}

	// Test loading messages
	loaded, err := LoadSessionMessages(sessionID)
	if err != nil {
		t.Errorf("LoadSessionMessages failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(loaded))
	}

	if loaded[0].Role != "user" || loaded[0].Content != "Hello" {
		t.Errorf("First message mismatch: %+v", loaded[0])
	}

	// Test loading non-existent session (should return empty, not error)
	loaded, err = LoadSessionMessages("nonexistent-session")
	if err != nil {
		t.Errorf("LoadSessionMessages should not error for non-existent session: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("Expected 0 messages for non-existent session, got %d", len(loaded))
	}

	// Test deleting messages
	err = DeleteSessionMessages(sessionID)
	if err != nil {
		t.Errorf("DeleteSessionMessages failed: %v", err)
	}

	// Verify deletion
	loaded, err = LoadSessionMessages(sessionID)
	if err != nil {
		t.Errorf("LoadSessionMessages after delete failed: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("Expected 0 messages after delete, got %d", len(loaded))
	}

	// Test deleting non-existent session (should not error)
	err = DeleteSessionMessages("nonexistent-session")
	if err != nil {
		t.Errorf("DeleteSessionMessages should not error for non-existent session: %v", err)
	}
}

func TestCountLines(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"hello", 1},
		{"hello\n", 2},
		{"hello\nworld", 2},
		{"hello\nworld\n", 3},
		{"a\nb\nc\nd", 4},
		{"\n\n\n", 4},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := countLines(tt.input)
			if result != tt.expected {
				t.Errorf("countLines(%q) = %d, expected %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSaveSessionMessages_MaxLines(t *testing.T) {
	sessionID := "test-maxlines-session"

	// Create messages that exceed max lines
	messages := []Message{}
	for i := 0; i < 20; i++ {
		// Each message is about 10 lines
		content := ""
		for j := 0; j < 10; j++ {
			content += "line content\n"
		}
		messages = append(messages, Message{
			Role:    "user",
			Content: content,
		})
	}

	// Save with max 50 lines
	err := SaveSessionMessages(sessionID, messages, 50)
	if err != nil {
		t.Fatalf("SaveSessionMessages failed: %v", err)
	}
	defer DeleteSessionMessages(sessionID)

	// Load and verify truncation happened
	loaded, err := LoadSessionMessages(sessionID)
	if err != nil {
		t.Fatalf("LoadSessionMessages failed: %v", err)
	}

	// Should have fewer messages than original due to truncation
	if len(loaded) >= len(messages) {
		t.Errorf("Expected truncated messages, got same or more: %d vs %d", len(loaded), len(messages))
	}
}

func TestConfig_SaveAndLoad(t *testing.T) {
	// Create a temporary directory for test
	tmpDir, err := os.MkdirTemp("", "plural-config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.json")

	// Create a config manually
	cfg := &Config{
		Repos: []string{"/path/to/repo1", "/path/to/repo2"},
		Sessions: []Session{
			{
				ID:        "session-1",
				RepoPath:  "/path/to/repo1",
				WorkTree:  "/path/to/worktree1",
				Branch:    "plural-session-1",
				Name:      "repo1/session1",
				CreatedAt: time.Now(),
				Started:   true,
				AllowedTools: []string{"Edit", "Bash(git:*)"},
			},
		},
		filePath: configPath,
	}

	// Save the config
	err = cfg.Save()
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	// Read and verify JSON structure
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	if len(loaded.Repos) != 2 {
		t.Errorf("Expected 2 repos, got %d", len(loaded.Repos))
	}

	if len(loaded.Sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(loaded.Sessions))
	}

	if loaded.Sessions[0].ID != "session-1" {
		t.Errorf("Expected session ID 'session-1', got '%s'", loaded.Sessions[0].ID)
	}

	if len(loaded.Sessions[0].AllowedTools) != 2 {
		t.Errorf("Expected 2 allowed tools, got %d", len(loaded.Sessions[0].AllowedTools))
	}
}
