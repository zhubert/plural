package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
		Repos:    []string{"/path/to/repo1", "/path/to/repo2", "/path/to/repo3"},
		Sessions: []Session{},
	}

	// Test removing existing repo from middle
	if !cfg.RemoveRepo("/path/to/repo2") {
		t.Error("RemoveRepo should return true for existing repo")
	}

	if len(cfg.Repos) != 2 {
		t.Errorf("Expected 2 repos after removal, got %d", len(cfg.Repos))
	}

	// Verify correct repo was removed
	for _, r := range cfg.Repos {
		if r == "/path/to/repo2" {
			t.Error("repo2 should have been removed")
		}
	}

	// Test removing non-existent repo
	if cfg.RemoveRepo("/nonexistent") {
		t.Error("RemoveRepo should return false for non-existent repo")
	}

	if len(cfg.Repos) != 2 {
		t.Errorf("Expected 2 repos after failed removal, got %d", len(cfg.Repos))
	}

	// Test removing first repo
	if !cfg.RemoveRepo("/path/to/repo1") {
		t.Error("RemoveRepo should return true for first repo")
	}

	if len(cfg.Repos) != 1 {
		t.Errorf("Expected 1 repo after second removal, got %d", len(cfg.Repos))
	}

	// Test removing last remaining repo
	if !cfg.RemoveRepo("/path/to/repo3") {
		t.Error("RemoveRepo should return true for last repo")
	}

	if len(cfg.Repos) != 0 {
		t.Errorf("Expected 0 repos after removing all, got %d", len(cfg.Repos))
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

func TestConfig_GetSession_ReturnsCopy(t *testing.T) {
	cfg := &Config{
		Sessions: []Session{
			{ID: "session-1", RepoPath: "/path1", Branch: "original"},
		},
	}

	// Get session and modify the copy
	sess := cfg.GetSession("session-1")
	if sess == nil {
		t.Fatal("GetSession should return session")
	}
	sess.Branch = "modified"

	// Original in config should be unchanged
	sess2 := cfg.GetSession("session-1")
	if sess2.Branch != "original" {
		t.Errorf("GetSession should return a copy; modifying it should not affect config, got branch=%q", sess2.Branch)
	}
}

func TestConfig_AllowedTools(t *testing.T) {
	cfg := &Config{
		Repos:            []string{"/path/to/repo"},
		Sessions:         []Session{},
		AllowedTools:     []string{"Edit"},
		RepoAllowedTools: make(map[string][]string),
	}

	// Test getting global allowed tools
	globalTools := cfg.GetGlobalAllowedTools()
	if len(globalTools) != 1 {
		t.Errorf("Expected 1 global tool, got %d", len(globalTools))
	}
	if globalTools[0] != "Edit" {
		t.Errorf("Expected tool 'Edit', got '%s'", globalTools[0])
	}

	// Test adding per-repo allowed tool
	if !cfg.AddRepoAllowedTool("/path/to/repo", "Bash(git:*)") {
		t.Error("AddRepoAllowedTool should return true for new tool")
	}

	// Test adding duplicate per-repo tool
	if cfg.AddRepoAllowedTool("/path/to/repo", "Bash(git:*)") {
		t.Error("AddRepoAllowedTool should return false for duplicate tool")
	}

	// Test getting merged allowed tools
	mergedTools := cfg.GetAllowedToolsForRepo("/path/to/repo")
	if len(mergedTools) != 2 {
		t.Errorf("Expected 2 merged tools, got %d", len(mergedTools))
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

func TestMergeConversationHistory(t *testing.T) {
	// Test merging child session messages into parent
	parentID := "test-parent-merge"
	childID := "test-child-merge"

	// Setup parent messages
	parentMessages := []Message{
		{Role: "user", Content: "Parent message 1"},
		{Role: "assistant", Content: "Parent response 1"},
	}
	err := SaveSessionMessages(parentID, parentMessages, MaxSessionMessageLines)
	if err != nil {
		t.Fatalf("Failed to save parent messages: %v", err)
	}
	defer DeleteSessionMessages(parentID)

	// Setup child messages
	childMessages := []Message{
		{Role: "user", Content: "Child message 1"},
		{Role: "assistant", Content: "Child response 1"},
		{Role: "user", Content: "Child message 2"},
		{Role: "assistant", Content: "Child response 2"},
	}
	err = SaveSessionMessages(childID, childMessages, MaxSessionMessageLines)
	if err != nil {
		t.Fatalf("Failed to save child messages: %v", err)
	}
	defer DeleteSessionMessages(childID)

	// Simulate the merge operation (what app.mergeConversationHistory does)
	parentMsgs, err := LoadSessionMessages(parentID)
	if err != nil {
		t.Fatalf("Failed to load parent messages: %v", err)
	}

	childMsgs, err := LoadSessionMessages(childID)
	if err != nil {
		t.Fatalf("Failed to load child messages: %v", err)
	}

	// Add separator and combine
	separator := Message{Role: "assistant", Content: "\n---\n[Merged from child session]\n---\n"}
	combined := append(parentMsgs, separator)
	combined = append(combined, childMsgs...)

	err = SaveSessionMessages(parentID, combined, MaxSessionMessageLines)
	if err != nil {
		t.Fatalf("Failed to save merged messages: %v", err)
	}

	// Verify merged result
	merged, err := LoadSessionMessages(parentID)
	if err != nil {
		t.Fatalf("Failed to load merged messages: %v", err)
	}

	// Should have: 2 parent + 1 separator + 4 child = 7 messages
	expectedCount := 2 + 1 + 4
	if len(merged) != expectedCount {
		t.Errorf("Expected %d merged messages, got %d", expectedCount, len(merged))
	}

	// First two should be parent messages
	if merged[0].Content != "Parent message 1" {
		t.Errorf("First message should be parent message 1, got %q", merged[0].Content)
	}

	// Third should be separator
	if merged[2].Role != "assistant" || merged[2].Content != "\n---\n[Merged from child session]\n---\n" {
		t.Errorf("Third message should be separator, got %+v", merged[2])
	}

	// Fourth onwards should be child messages
	if merged[3].Content != "Child message 1" {
		t.Errorf("Fourth message should be child message 1, got %q", merged[3].Content)
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
			},
		},
		AllowedTools: []string{"Edit"},
		RepoAllowedTools: map[string][]string{
			"/path/to/repo1": {"Bash(git:*)"},
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

	if len(loaded.AllowedTools) != 1 {
		t.Errorf("Expected 1 global allowed tool, got %d", len(loaded.AllowedTools))
	}

	if len(loaded.RepoAllowedTools["/path/to/repo1"]) != 1 {
		t.Errorf("Expected 1 repo allowed tool, got %d", len(loaded.RepoAllowedTools["/path/to/repo1"]))
	}
}

func TestConfig_MarkSessionMerged(t *testing.T) {
	cfg := &Config{
		Repos: []string{},
		Sessions: []Session{
			{ID: "session-1", RepoPath: "/path", WorkTree: "/wt", Branch: "b1", Merged: false},
		},
	}

	// Test marking existing session as merged
	if !cfg.MarkSessionMerged("session-1") {
		t.Error("MarkSessionMerged should return true for existing session")
	}

	sess := cfg.GetSession("session-1")
	if !sess.Merged {
		t.Error("Session should be marked as merged")
	}

	// Test marking non-existent session
	if cfg.MarkSessionMerged("nonexistent") {
		t.Error("MarkSessionMerged should return false for non-existent session")
	}
}

func TestConfig_MarkSessionPRCreated(t *testing.T) {
	cfg := &Config{
		Repos: []string{},
		Sessions: []Session{
			{ID: "session-1", RepoPath: "/path", WorkTree: "/wt", Branch: "b1", PRCreated: false},
		},
	}

	// Test marking existing session as having PR created
	if !cfg.MarkSessionPRCreated("session-1") {
		t.Error("MarkSessionPRCreated should return true for existing session")
	}

	sess := cfg.GetSession("session-1")
	if !sess.PRCreated {
		t.Error("Session should be marked as having PR created")
	}

	// Test marking non-existent session
	if cfg.MarkSessionPRCreated("nonexistent") {
		t.Error("MarkSessionPRCreated should return false for non-existent session")
	}
}

func TestConfig_MarkSessionMergedToParent(t *testing.T) {
	cfg := &Config{
		Repos: []string{},
		Sessions: []Session{
			{ID: "child-1", RepoPath: "/path", WorkTree: "/wt", Branch: "b1", ParentID: "parent-1", MergedToParent: false},
			{ID: "parent-1", RepoPath: "/path", WorkTree: "/wt2", Branch: "b2"},
		},
	}

	// Test marking existing session as merged to parent
	if !cfg.MarkSessionMergedToParent("child-1") {
		t.Error("MarkSessionMergedToParent should return true for existing session")
	}

	sess := cfg.GetSession("child-1")
	if !sess.MergedToParent {
		t.Error("Session should be marked as merged to parent")
	}

	// Test marking non-existent session
	if cfg.MarkSessionMergedToParent("nonexistent") {
		t.Error("MarkSessionMergedToParent should return false for non-existent session")
	}
}

func TestConfig_GlobalMCPServers(t *testing.T) {
	cfg := &Config{
		Repos:      []string{"/path/to/repo"},
		Sessions:   []Session{},
		MCPServers: []MCPServer{},
	}

	server1 := MCPServer{
		Name:    "github",
		Command: "npx",
		Args:    []string{"@modelcontextprotocol/server-github"},
	}

	// Test adding a global MCP server
	if !cfg.AddGlobalMCPServer(server1) {
		t.Error("AddGlobalMCPServer should return true for new server")
	}

	servers := cfg.GetGlobalMCPServers()
	if len(servers) != 1 {
		t.Errorf("Expected 1 global MCP server, got %d", len(servers))
	}

	if servers[0].Name != "github" {
		t.Errorf("Expected server name 'github', got '%s'", servers[0].Name)
	}

	// Test adding duplicate server (same name)
	if cfg.AddGlobalMCPServer(server1) {
		t.Error("AddGlobalMCPServer should return false for duplicate server name")
	}

	// Test adding different server
	server2 := MCPServer{
		Name:    "postgres",
		Command: "npx",
		Args:    []string{"@modelcontextprotocol/server-postgres"},
	}
	if !cfg.AddGlobalMCPServer(server2) {
		t.Error("AddGlobalMCPServer should return true for new server")
	}

	servers = cfg.GetGlobalMCPServers()
	if len(servers) != 2 {
		t.Errorf("Expected 2 global MCP servers, got %d", len(servers))
	}

	// Verify copy is returned
	servers[0].Name = "modified"
	original := cfg.GetGlobalMCPServers()
	if original[0].Name == "modified" {
		t.Error("GetGlobalMCPServers should return a copy")
	}
}

func TestConfig_RemoveGlobalMCPServer(t *testing.T) {
	cfg := &Config{
		Repos:    []string{},
		Sessions: []Session{},
		MCPServers: []MCPServer{
			{Name: "github", Command: "npx", Args: []string{"@mcp/github"}},
			{Name: "postgres", Command: "npx", Args: []string{"@mcp/postgres"}},
		},
	}

	// Test removing existing server
	if !cfg.RemoveGlobalMCPServer("github") {
		t.Error("RemoveGlobalMCPServer should return true for existing server")
	}

	servers := cfg.GetGlobalMCPServers()
	if len(servers) != 1 {
		t.Errorf("Expected 1 server after removal, got %d", len(servers))
	}

	if servers[0].Name != "postgres" {
		t.Errorf("Expected remaining server 'postgres', got '%s'", servers[0].Name)
	}

	// Test removing non-existent server
	if cfg.RemoveGlobalMCPServer("nonexistent") {
		t.Error("RemoveGlobalMCPServer should return false for non-existent server")
	}
}

func TestConfig_RepoMCPServers(t *testing.T) {
	cfg := &Config{
		Repos:    []string{"/path/to/repo"},
		Sessions: []Session{},
		RepoMCP:  nil, // Start with nil to test initialization
	}

	repoPath := "/path/to/repo"
	server := MCPServer{
		Name:    "github",
		Command: "npx",
		Args:    []string{"@mcp/github"},
	}

	// Test adding repo-specific server (with nil map)
	if !cfg.AddRepoMCPServer(repoPath, server) {
		t.Error("AddRepoMCPServer should return true for new server")
	}

	servers := cfg.GetRepoMCPServers(repoPath)
	if len(servers) != 1 {
		t.Errorf("Expected 1 repo MCP server, got %d", len(servers))
	}

	// Test adding duplicate
	if cfg.AddRepoMCPServer(repoPath, server) {
		t.Error("AddRepoMCPServer should return false for duplicate server name")
	}

	// Test getting servers for non-existent repo
	noServers := cfg.GetRepoMCPServers("/different/repo")
	if len(noServers) != 0 {
		t.Errorf("Expected 0 servers for non-existent repo, got %d", len(noServers))
	}

	// Verify copy is returned
	servers[0].Name = "modified"
	original := cfg.GetRepoMCPServers(repoPath)
	if original[0].Name == "modified" {
		t.Error("GetRepoMCPServers should return a copy")
	}
}

func TestConfig_RemoveRepoMCPServer(t *testing.T) {
	repoPath := "/path/to/repo"
	cfg := &Config{
		Repos:    []string{repoPath},
		Sessions: []Session{},
		RepoMCP: map[string][]MCPServer{
			repoPath: {
				{Name: "github", Command: "npx", Args: []string{"@mcp/github"}},
				{Name: "postgres", Command: "npx", Args: []string{"@mcp/postgres"}},
			},
		},
	}

	// Test removing existing server
	if !cfg.RemoveRepoMCPServer(repoPath, "github") {
		t.Error("RemoveRepoMCPServer should return true for existing server")
	}

	servers := cfg.GetRepoMCPServers(repoPath)
	if len(servers) != 1 {
		t.Errorf("Expected 1 server after removal, got %d", len(servers))
	}

	// Test removing from non-existent repo
	if cfg.RemoveRepoMCPServer("/other/repo", "github") {
		t.Error("RemoveRepoMCPServer should return false for non-existent repo")
	}

	// Test removing non-existent server
	if cfg.RemoveRepoMCPServer(repoPath, "nonexistent") {
		t.Error("RemoveRepoMCPServer should return false for non-existent server")
	}

	// Remove last server - map entry should be cleaned up
	cfg.RemoveRepoMCPServer(repoPath, "postgres")
	if _, exists := cfg.RepoMCP[repoPath]; exists {
		t.Error("RepoMCP entry should be removed when empty")
	}
}

func TestConfig_GetMCPServersForRepo(t *testing.T) {
	repoPath := "/path/to/repo"
	cfg := &Config{
		Repos:    []string{repoPath},
		Sessions: []Session{},
		MCPServers: []MCPServer{
			{Name: "global-github", Command: "npx", Args: []string{"@mcp/github"}},
			{Name: "shared", Command: "npx", Args: []string{"@mcp/shared"}},
		},
		RepoMCP: map[string][]MCPServer{
			repoPath: {
				{Name: "repo-postgres", Command: "npx", Args: []string{"@mcp/postgres"}},
				{Name: "shared", Command: "custom", Args: []string{"--custom-args"}}, // Override global
			},
		},
	}

	servers := cfg.GetMCPServersForRepo(repoPath)

	// Should have 3 servers: global-github, repo-postgres, and shared (overridden)
	if len(servers) != 3 {
		t.Errorf("Expected 3 merged servers, got %d", len(servers))
	}

	// Build a map for easier checking
	serverMap := make(map[string]MCPServer)
	for _, s := range servers {
		serverMap[s.Name] = s
	}

	// Check global-github is present
	if _, ok := serverMap["global-github"]; !ok {
		t.Error("Expected global-github server to be present")
	}

	// Check repo-postgres is present
	if _, ok := serverMap["repo-postgres"]; !ok {
		t.Error("Expected repo-postgres server to be present")
	}

	// Check shared is overridden with repo-specific version
	if shared, ok := serverMap["shared"]; ok {
		if shared.Command != "custom" {
			t.Errorf("Expected 'shared' server to be overridden with repo version, got command=%s", shared.Command)
		}
	} else {
		t.Error("Expected shared server to be present")
	}
}

func TestConfig_GetMCPServersForRepo_EmptyRepo(t *testing.T) {
	cfg := &Config{
		Repos:    []string{},
		Sessions: []Session{},
		MCPServers: []MCPServer{
			{Name: "global-github", Command: "npx", Args: []string{"@mcp/github"}},
		},
		RepoMCP: make(map[string][]MCPServer),
	}

	// Should return only global servers for repo with no specific config
	servers := cfg.GetMCPServersForRepo("/some/repo")
	if len(servers) != 1 {
		t.Errorf("Expected 1 global server, got %d", len(servers))
	}
}

func TestConfig_EnsureInitialized(t *testing.T) {
	cfg := &Config{}

	// All fields should be nil initially
	if cfg.Repos != nil || cfg.Sessions != nil {
		t.Error("Expected nil slices initially")
	}

	cfg.ensureInitialized()

	// All fields should be initialized
	if cfg.Repos == nil {
		t.Error("Repos should be initialized")
	}
	if cfg.Sessions == nil {
		t.Error("Sessions should be initialized")
	}
	if cfg.MCPServers == nil {
		t.Error("MCPServers should be initialized")
	}
	if cfg.RepoMCP == nil {
		t.Error("RepoMCP should be initialized")
	}
	if cfg.AllowedTools == nil {
		t.Error("AllowedTools should be initialized")
	}
	if cfg.RepoAllowedTools == nil {
		t.Error("RepoAllowedTools should be initialized")
	}
	if cfg.RepoSquashOnMerge == nil {
		t.Error("RepoSquashOnMerge should be initialized")
	}
}

func TestLoad_NewConfig(t *testing.T) {
	// Create a temp directory to use as HOME
	tmpDir, err := os.MkdirTemp("", "plural-load-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Save original HOME and set temp dir
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Load should create a new config when none exists
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}

	// Verify defaults are set
	if cfg.Repos == nil {
		t.Error("Repos should be initialized")
	}
	if cfg.Sessions == nil {
		t.Error("Sessions should be initialized")
	}
	if cfg.AllowedTools == nil {
		t.Error("AllowedTools should be initialized")
	}
	if cfg.RepoAllowedTools == nil {
		t.Error("RepoAllowedTools should be initialized")
	}
}

func TestLoad_ExistingConfig(t *testing.T) {
	// Create a temp directory
	tmpDir, err := os.MkdirTemp("", "plural-load-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Save original HOME and set temp dir
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create config directory and file
	pluralDir := filepath.Join(tmpDir, ".plural")
	if err := os.MkdirAll(pluralDir, 0755); err != nil {
		t.Fatalf("Failed to create plural dir: %v", err)
	}

	configData := `{
		"repos": ["/path/to/repo"],
		"sessions": [{
			"id": "test-session",
			"repo_path": "/path/to/repo",
			"worktree": "/path/to/worktree",
			"branch": "plural-test",
			"name": "test/session"
		}],
		"allowed_tools": ["Edit", "Write"]
	}`

	configFile := filepath.Join(pluralDir, "config.json")
	if err := os.WriteFile(configFile, []byte(configData), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load the config
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify loaded data
	if len(cfg.Repos) != 1 {
		t.Errorf("Expected 1 repo, got %d", len(cfg.Repos))
	}
	if cfg.Repos[0] != "/path/to/repo" {
		t.Errorf("Expected repo '/path/to/repo', got %q", cfg.Repos[0])
	}

	if len(cfg.Sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(cfg.Sessions))
	}
	if cfg.Sessions[0].ID != "test-session" {
		t.Errorf("Expected session ID 'test-session', got %q", cfg.Sessions[0].ID)
	}

	if len(cfg.AllowedTools) != 2 {
		t.Errorf("Expected 2 allowed tools, got %d", len(cfg.AllowedTools))
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	// Create a temp directory
	tmpDir, err := os.MkdirTemp("", "plural-load-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Save original HOME and set temp dir
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create config directory and invalid file
	pluralDir := filepath.Join(tmpDir, ".plural")
	if err := os.MkdirAll(pluralDir, 0755); err != nil {
		t.Fatalf("Failed to create plural dir: %v", err)
	}

	configFile := filepath.Join(pluralDir, "config.json")
	if err := os.WriteFile(configFile, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load should fail
	_, err = Load()
	if err == nil {
		t.Error("Load() should fail with invalid JSON")
	}
}

func TestLoad_InvalidConfig(t *testing.T) {
	// Create a temp directory
	tmpDir, err := os.MkdirTemp("", "plural-load-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Save original HOME and set temp dir
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create config directory and file with duplicate session IDs
	pluralDir := filepath.Join(tmpDir, ".plural")
	if err := os.MkdirAll(pluralDir, 0755); err != nil {
		t.Fatalf("Failed to create plural dir: %v", err)
	}

	configData := `{
		"repos": [],
		"sessions": [
			{"id": "duplicate", "repo_path": "/path1", "worktree": "/wt1", "branch": "b1"},
			{"id": "duplicate", "repo_path": "/path2", "worktree": "/wt2", "branch": "b2"}
		]
	}`

	configFile := filepath.Join(pluralDir, "config.json")
	if err := os.WriteFile(configFile, []byte(configData), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load should fail validation
	_, err = Load()
	if err == nil {
		t.Error("Load() should fail with duplicate session IDs")
	}
}

func TestCountLines_EdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"\n", 2},
		{"\n\n", 3},
		{"no newline", 1},
		{"ends with newline\n", 2},
		{"multi\nline\nstring", 3},
	}

	for _, tt := range tests {
		result := countLines(tt.input)
		if result != tt.expected {
			t.Errorf("countLines(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestConfig_PreviewState(t *testing.T) {
	cfg := &Config{
		Repos:    []string{},
		Sessions: []Session{},
	}

	// Initially no preview should be active
	if cfg.IsPreviewActive() {
		t.Error("IsPreviewActive should return false initially")
	}

	if cfg.GetPreviewSessionID() != "" {
		t.Error("GetPreviewSessionID should return empty string initially")
	}

	sessionID, previousBranch, repoPath := cfg.GetPreviewState()
	if sessionID != "" || previousBranch != "" || repoPath != "" {
		t.Error("GetPreviewState should return empty strings initially")
	}

	// Start a preview
	cfg.StartPreview("session-123", "main", "/path/to/repo")

	if !cfg.IsPreviewActive() {
		t.Error("IsPreviewActive should return true after StartPreview")
	}

	if cfg.GetPreviewSessionID() != "session-123" {
		t.Errorf("GetPreviewSessionID = %q, want 'session-123'", cfg.GetPreviewSessionID())
	}

	sessionID, previousBranch, repoPath = cfg.GetPreviewState()
	if sessionID != "session-123" {
		t.Errorf("sessionID = %q, want 'session-123'", sessionID)
	}
	if previousBranch != "main" {
		t.Errorf("previousBranch = %q, want 'main'", previousBranch)
	}
	if repoPath != "/path/to/repo" {
		t.Errorf("repoPath = %q, want '/path/to/repo'", repoPath)
	}

	// End the preview
	cfg.EndPreview()

	if cfg.IsPreviewActive() {
		t.Error("IsPreviewActive should return false after EndPreview")
	}

	if cfg.GetPreviewSessionID() != "" {
		t.Error("GetPreviewSessionID should return empty string after EndPreview")
	}

	sessionID, previousBranch, repoPath = cfg.GetPreviewState()
	if sessionID != "" || previousBranch != "" || repoPath != "" {
		t.Error("GetPreviewState should return empty strings after EndPreview")
	}
}

func TestConfig_PreviewState_Persistence(t *testing.T) {
	// Create a temp directory for test config
	tmpDir, err := os.MkdirTemp("", "plural-preview-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.json")

	// Create config with preview state
	cfg := &Config{
		Repos:    []string{"/path/to/repo"},
		Sessions: []Session{},
		filePath: configPath,
	}

	cfg.StartPreview("session-abc", "develop", "/path/to/repo")

	// Save the config
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Read and verify JSON structure includes preview fields
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	if loaded.PreviewSessionID != "session-abc" {
		t.Errorf("PreviewSessionID = %q, want 'session-abc'", loaded.PreviewSessionID)
	}
	if loaded.PreviewPreviousBranch != "develop" {
		t.Errorf("PreviewPreviousBranch = %q, want 'develop'", loaded.PreviewPreviousBranch)
	}
	if loaded.PreviewRepoPath != "/path/to/repo" {
		t.Errorf("PreviewRepoPath = %q, want '/path/to/repo'", loaded.PreviewRepoPath)
	}
}

func TestConfig_SquashOnMerge(t *testing.T) {
	cfg := &Config{
		Repos:             []string{"/path/to/repo1", "/path/to/repo2"},
		Sessions:          []Session{},
		RepoSquashOnMerge: make(map[string]bool),
	}

	// Initially should return false for all repos
	if cfg.GetSquashOnMerge("/path/to/repo1") {
		t.Error("GetSquashOnMerge should return false initially")
	}

	// Enable for repo1
	cfg.SetSquashOnMerge("/path/to/repo1", true)

	if !cfg.GetSquashOnMerge("/path/to/repo1") {
		t.Error("GetSquashOnMerge should return true after enabling")
	}

	// repo2 should still be false
	if cfg.GetSquashOnMerge("/path/to/repo2") {
		t.Error("GetSquashOnMerge should return false for repo2")
	}

	// Disable for repo1
	cfg.SetSquashOnMerge("/path/to/repo1", false)

	if cfg.GetSquashOnMerge("/path/to/repo1") {
		t.Error("GetSquashOnMerge should return false after disabling")
	}

	// Map entry should be cleaned up
	if _, exists := cfg.RepoSquashOnMerge["/path/to/repo1"]; exists {
		t.Error("RepoSquashOnMerge entry should be removed when disabled")
	}
}

func TestConfig_SquashOnMerge_NilMap(t *testing.T) {
	cfg := &Config{
		Repos:             []string{"/path/to/repo"},
		Sessions:          []Session{},
		RepoSquashOnMerge: nil, // Start with nil map
	}

	// GetSquashOnMerge should handle nil map gracefully
	if cfg.GetSquashOnMerge("/path/to/repo") {
		t.Error("GetSquashOnMerge should return false for nil map")
	}

	// SetSquashOnMerge should initialize the map
	cfg.SetSquashOnMerge("/path/to/repo", true)

	if cfg.RepoSquashOnMerge == nil {
		t.Error("RepoSquashOnMerge should be initialized after SetSquashOnMerge")
	}

	if !cfg.GetSquashOnMerge("/path/to/repo") {
		t.Error("GetSquashOnMerge should return true after setting")
	}
}

func TestConfig_SquashOnMerge_Persistence(t *testing.T) {
	// Create a temp directory for test config
	tmpDir, err := os.MkdirTemp("", "plural-squash-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.json")

	// Create config with squash setting
	cfg := &Config{
		Repos:             []string{"/path/to/repo"},
		Sessions:          []Session{},
		RepoSquashOnMerge: make(map[string]bool),
		filePath:          configPath,
	}

	cfg.SetSquashOnMerge("/path/to/repo", true)

	// Save the config
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Read and verify JSON structure includes squash field
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	if !loaded.RepoSquashOnMerge["/path/to/repo"] {
		t.Error("RepoSquashOnMerge should be persisted")
	}
}

func TestConfig_GetSessionsByBroadcastGroup(t *testing.T) {
	cfg := &Config{
		Repos: []string{"/path/to/repo"},
		Sessions: []Session{
			{ID: "session-1", RepoPath: "/path/to/repo1", WorkTree: "/wt1", Branch: "b1", BroadcastGroupID: "group-abc"},
			{ID: "session-2", RepoPath: "/path/to/repo2", WorkTree: "/wt2", Branch: "b2", BroadcastGroupID: "group-abc"},
			{ID: "session-3", RepoPath: "/path/to/repo3", WorkTree: "/wt3", Branch: "b3", BroadcastGroupID: "group-def"},
			{ID: "session-4", RepoPath: "/path/to/repo4", WorkTree: "/wt4", Branch: "b4"}, // No group
		},
	}

	// Test getting sessions by group
	group1Sessions := cfg.GetSessionsByBroadcastGroup("group-abc")
	if len(group1Sessions) != 2 {
		t.Errorf("Expected 2 sessions in group-abc, got %d", len(group1Sessions))
	}

	// Verify the correct sessions are returned
	ids := make(map[string]bool)
	for _, s := range group1Sessions {
		ids[s.ID] = true
	}
	if !ids["session-1"] || !ids["session-2"] {
		t.Error("Expected session-1 and session-2 in group-abc")
	}

	// Test different group
	group2Sessions := cfg.GetSessionsByBroadcastGroup("group-def")
	if len(group2Sessions) != 1 {
		t.Errorf("Expected 1 session in group-def, got %d", len(group2Sessions))
	}
	if group2Sessions[0].ID != "session-3" {
		t.Errorf("Expected session-3 in group-def, got %s", group2Sessions[0].ID)
	}

	// Test non-existent group
	noSessions := cfg.GetSessionsByBroadcastGroup("nonexistent")
	if len(noSessions) != 0 {
		t.Errorf("Expected 0 sessions for nonexistent group, got %d", len(noSessions))
	}

	// Test empty group ID
	emptyGroup := cfg.GetSessionsByBroadcastGroup("")
	if len(emptyGroup) != 0 {
		t.Errorf("Expected 0 sessions for empty group ID, got %d", len(emptyGroup))
	}
}

func TestConfig_SetSessionBroadcastGroup(t *testing.T) {
	cfg := &Config{
		Repos: []string{},
		Sessions: []Session{
			{ID: "session-1", RepoPath: "/path", WorkTree: "/wt", Branch: "b1"},
		},
	}

	// Test setting broadcast group
	if !cfg.SetSessionBroadcastGroup("session-1", "group-xyz") {
		t.Error("SetSessionBroadcastGroup should return true for existing session")
	}

	sess := cfg.GetSession("session-1")
	if sess.BroadcastGroupID != "group-xyz" {
		t.Errorf("Expected BroadcastGroupID 'group-xyz', got '%s'", sess.BroadcastGroupID)
	}

	// Test setting for non-existent session
	if cfg.SetSessionBroadcastGroup("nonexistent", "group-xyz") {
		t.Error("SetSessionBroadcastGroup should return false for non-existent session")
	}
}

func TestConfig_BroadcastGroupID_Persistence(t *testing.T) {
	// Create a temp directory for test config
	tmpDir, err := os.MkdirTemp("", "plural-broadcast-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.json")

	// Create config with broadcast group
	cfg := &Config{
		Repos: []string{"/path/to/repo"},
		Sessions: []Session{
			{
				ID:               "session-1",
				RepoPath:         "/path/to/repo",
				WorkTree:         "/path/to/worktree",
				Branch:           "plural-session-1",
				Name:             "session1",
				BroadcastGroupID: "broadcast-group-123",
			},
		},
		filePath: configPath,
	}

	// Save the config
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Read and verify JSON structure includes broadcast_group_id field
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	if len(loaded.Sessions) != 1 {
		t.Fatalf("Expected 1 session, got %d", len(loaded.Sessions))
	}

	if loaded.Sessions[0].BroadcastGroupID != "broadcast-group-123" {
		t.Errorf("BroadcastGroupID = %q, want 'broadcast-group-123'", loaded.Sessions[0].BroadcastGroupID)
	}
}

func TestConfig_Workspaces(t *testing.T) {
	cfg := &Config{
		Repos:      []string{},
		Sessions:   []Session{},
		Workspaces: []Workspace{},
	}

	// Test adding a workspace
	ws1 := Workspace{ID: "ws-1", Name: "Backend"}
	if !cfg.AddWorkspace(ws1) {
		t.Error("AddWorkspace should return true for new workspace")
	}

	workspaces := cfg.GetWorkspaces()
	if len(workspaces) != 1 {
		t.Errorf("Expected 1 workspace, got %d", len(workspaces))
	}
	if workspaces[0].ID != "ws-1" || workspaces[0].Name != "Backend" {
		t.Errorf("Workspace mismatch: %+v", workspaces[0])
	}

	// Test adding duplicate name
	ws2 := Workspace{ID: "ws-2", Name: "Backend"}
	if cfg.AddWorkspace(ws2) {
		t.Error("AddWorkspace should return false for duplicate name")
	}

	// Test adding another workspace
	ws3 := Workspace{ID: "ws-3", Name: "Frontend"}
	if !cfg.AddWorkspace(ws3) {
		t.Error("AddWorkspace should return true for new name")
	}

	if len(cfg.GetWorkspaces()) != 2 {
		t.Errorf("Expected 2 workspaces, got %d", len(cfg.GetWorkspaces()))
	}

	// Test that GetWorkspaces returns a copy
	workspaces = cfg.GetWorkspaces()
	workspaces[0].Name = "Modified"
	if cfg.Workspaces[0].Name == "Modified" {
		t.Error("GetWorkspaces should return a copy")
	}
}

func TestConfig_RemoveWorkspace(t *testing.T) {
	cfg := &Config{
		Repos: []string{},
		Sessions: []Session{
			{ID: "s1", RepoPath: "/r", WorkTree: "/w", Branch: "b1", WorkspaceID: "ws-1"},
			{ID: "s2", RepoPath: "/r", WorkTree: "/w", Branch: "b2", WorkspaceID: "ws-1"},
			{ID: "s3", RepoPath: "/r", WorkTree: "/w", Branch: "b3", WorkspaceID: "ws-2"},
		},
		Workspaces: []Workspace{
			{ID: "ws-1", Name: "Backend"},
			{ID: "ws-2", Name: "Frontend"},
		},
		ActiveWorkspaceID: "ws-1",
	}

	// Remove ws-1 (the active workspace)
	if !cfg.RemoveWorkspace("ws-1") {
		t.Error("RemoveWorkspace should return true")
	}

	// Should have 1 workspace left
	if len(cfg.GetWorkspaces()) != 1 {
		t.Errorf("Expected 1 workspace, got %d", len(cfg.GetWorkspaces()))
	}

	// Sessions in ws-1 should have WorkspaceID cleared
	s1 := cfg.GetSession("s1")
	if s1.WorkspaceID != "" {
		t.Errorf("s1 WorkspaceID should be cleared, got %q", s1.WorkspaceID)
	}
	s2 := cfg.GetSession("s2")
	if s2.WorkspaceID != "" {
		t.Errorf("s2 WorkspaceID should be cleared, got %q", s2.WorkspaceID)
	}

	// s3 should be unchanged
	s3 := cfg.GetSession("s3")
	if s3.WorkspaceID != "ws-2" {
		t.Errorf("s3 WorkspaceID should be ws-2, got %q", s3.WorkspaceID)
	}

	// Active workspace should be cleared
	if cfg.GetActiveWorkspaceID() != "" {
		t.Errorf("ActiveWorkspaceID should be cleared, got %q", cfg.GetActiveWorkspaceID())
	}

	// Remove non-existent workspace
	if cfg.RemoveWorkspace("nonexistent") {
		t.Error("RemoveWorkspace should return false for non-existent workspace")
	}
}

func TestConfig_RenameWorkspace(t *testing.T) {
	cfg := &Config{
		Repos:    []string{},
		Sessions: []Session{},
		Workspaces: []Workspace{
			{ID: "ws-1", Name: "Backend"},
			{ID: "ws-2", Name: "Frontend"},
		},
	}

	// Rename ws-1
	if !cfg.RenameWorkspace("ws-1", "API") {
		t.Error("RenameWorkspace should return true")
	}

	workspaces := cfg.GetWorkspaces()
	if workspaces[0].Name != "API" {
		t.Errorf("Expected name 'API', got %q", workspaces[0].Name)
	}

	// Rename to conflicting name
	if cfg.RenameWorkspace("ws-1", "Frontend") {
		t.Error("RenameWorkspace should return false for conflicting name")
	}

	// Rename non-existent workspace
	if cfg.RenameWorkspace("nonexistent", "New") {
		t.Error("RenameWorkspace should return false for non-existent workspace")
	}

	// Rename to same name (should succeed)
	if !cfg.RenameWorkspace("ws-1", "API") {
		t.Error("RenameWorkspace should return true when renaming to same name")
	}
}

func TestConfig_ActiveWorkspace(t *testing.T) {
	cfg := &Config{
		Repos:    []string{},
		Sessions: []Session{},
	}

	// Initially no active workspace
	if cfg.GetActiveWorkspaceID() != "" {
		t.Error("ActiveWorkspaceID should be empty initially")
	}

	// Set active workspace
	cfg.SetActiveWorkspaceID("ws-1")
	if cfg.GetActiveWorkspaceID() != "ws-1" {
		t.Errorf("Expected active workspace 'ws-1', got %q", cfg.GetActiveWorkspaceID())
	}

	// Clear active workspace
	cfg.SetActiveWorkspaceID("")
	if cfg.GetActiveWorkspaceID() != "" {
		t.Error("ActiveWorkspaceID should be empty after clearing")
	}
}

func TestConfig_SetSessionWorkspace(t *testing.T) {
	cfg := &Config{
		Repos: []string{},
		Sessions: []Session{
			{ID: "s1", RepoPath: "/r", WorkTree: "/w", Branch: "b1"},
		},
	}

	// Assign to workspace
	if !cfg.SetSessionWorkspace("s1", "ws-1") {
		t.Error("SetSessionWorkspace should return true")
	}

	sess := cfg.GetSession("s1")
	if sess.WorkspaceID != "ws-1" {
		t.Errorf("Expected WorkspaceID 'ws-1', got %q", sess.WorkspaceID)
	}

	// Unassign
	if !cfg.SetSessionWorkspace("s1", "") {
		t.Error("SetSessionWorkspace should return true for unassign")
	}

	sess = cfg.GetSession("s1")
	if sess.WorkspaceID != "" {
		t.Errorf("Expected empty WorkspaceID, got %q", sess.WorkspaceID)
	}

	// Non-existent session
	if cfg.SetSessionWorkspace("nonexistent", "ws-1") {
		t.Error("SetSessionWorkspace should return false for non-existent session")
	}
}

func TestConfig_GetSessionsByWorkspace(t *testing.T) {
	cfg := &Config{
		Repos: []string{},
		Sessions: []Session{
			{ID: "s1", RepoPath: "/r", WorkTree: "/w", Branch: "b1", WorkspaceID: "ws-1"},
			{ID: "s2", RepoPath: "/r", WorkTree: "/w", Branch: "b2", WorkspaceID: "ws-1"},
			{ID: "s3", RepoPath: "/r", WorkTree: "/w", Branch: "b3", WorkspaceID: "ws-2"},
			{ID: "s4", RepoPath: "/r", WorkTree: "/w", Branch: "b4"},
		},
	}

	// Get sessions in ws-1
	sessions := cfg.GetSessionsByWorkspace("ws-1")
	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions in ws-1, got %d", len(sessions))
	}

	// Get sessions in ws-2
	sessions = cfg.GetSessionsByWorkspace("ws-2")
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session in ws-2, got %d", len(sessions))
	}

	// Empty workspace ID
	sessions = cfg.GetSessionsByWorkspace("")
	if sessions != nil {
		t.Error("Expected nil for empty workspace ID")
	}

	// Non-existent workspace
	sessions = cfg.GetSessionsByWorkspace("nonexistent")
	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions for nonexistent workspace, got %d", len(sessions))
	}
}

func TestConfig_RemoveSessions(t *testing.T) {
	cfg := &Config{
		Repos: []string{},
		Sessions: []Session{
			{ID: "s1", RepoPath: "/r", WorkTree: "/w", Branch: "b1"},
			{ID: "s2", RepoPath: "/r", WorkTree: "/w", Branch: "b2"},
			{ID: "s3", RepoPath: "/r", WorkTree: "/w", Branch: "b3"},
			{ID: "s4", RepoPath: "/r", WorkTree: "/w", Branch: "b4"},
		},
	}

	// Remove multiple sessions
	removed := cfg.RemoveSessions([]string{"s1", "s3"})
	if removed != 2 {
		t.Errorf("Expected 2 removed, got %d", removed)
	}

	sessions := cfg.GetSessions()
	if len(sessions) != 2 {
		t.Errorf("Expected 2 remaining sessions, got %d", len(sessions))
	}

	// Verify correct sessions remain
	ids := map[string]bool{}
	for _, s := range sessions {
		ids[s.ID] = true
	}
	if !ids["s2"] || !ids["s4"] {
		t.Error("Expected s2 and s4 to remain")
	}

	// Remove with non-existent IDs
	removed = cfg.RemoveSessions([]string{"nonexistent"})
	if removed != 0 {
		t.Errorf("Expected 0 removed for nonexistent IDs, got %d", removed)
	}

	// Remove empty list
	removed = cfg.RemoveSessions([]string{})
	if removed != 0 {
		t.Errorf("Expected 0 removed for empty list, got %d", removed)
	}
}

func TestConfig_SetSessionsWorkspace(t *testing.T) {
	cfg := &Config{
		Repos: []string{},
		Sessions: []Session{
			{ID: "s1", RepoPath: "/r", WorkTree: "/w", Branch: "b1"},
			{ID: "s2", RepoPath: "/r", WorkTree: "/w", Branch: "b2"},
			{ID: "s3", RepoPath: "/r", WorkTree: "/w", Branch: "b3"},
		},
	}

	// Assign multiple sessions to workspace
	updated := cfg.SetSessionsWorkspace([]string{"s1", "s3"}, "ws-1")
	if updated != 2 {
		t.Errorf("Expected 2 updated, got %d", updated)
	}

	s1 := cfg.GetSession("s1")
	if s1.WorkspaceID != "ws-1" {
		t.Errorf("s1 should be in ws-1, got %q", s1.WorkspaceID)
	}
	s2 := cfg.GetSession("s2")
	if s2.WorkspaceID != "" {
		t.Errorf("s2 should have no workspace, got %q", s2.WorkspaceID)
	}
	s3 := cfg.GetSession("s3")
	if s3.WorkspaceID != "ws-1" {
		t.Errorf("s3 should be in ws-1, got %q", s3.WorkspaceID)
	}
}

func TestConfig_Workspace_Persistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plural-workspace-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.json")

	cfg := &Config{
		Repos: []string{"/path/to/repo"},
		Sessions: []Session{
			{
				ID:          "s1",
				RepoPath:    "/path/to/repo",
				WorkTree:    "/path/to/worktree",
				Branch:      "b1",
				Name:        "session1",
				WorkspaceID: "ws-1",
			},
		},
		Workspaces: []Workspace{
			{ID: "ws-1", Name: "Backend"},
			{ID: "ws-2", Name: "Frontend"},
		},
		ActiveWorkspaceID: "ws-1",
		filePath:          configPath,
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	if len(loaded.Workspaces) != 2 {
		t.Errorf("Expected 2 workspaces, got %d", len(loaded.Workspaces))
	}
	if loaded.Workspaces[0].Name != "Backend" {
		t.Errorf("Expected first workspace name 'Backend', got %q", loaded.Workspaces[0].Name)
	}
	if loaded.ActiveWorkspaceID != "ws-1" {
		t.Errorf("Expected active workspace 'ws-1', got %q", loaded.ActiveWorkspaceID)
	}
	if loaded.Sessions[0].WorkspaceID != "ws-1" {
		t.Errorf("Expected session workspace 'ws-1', got %q", loaded.Sessions[0].WorkspaceID)
	}
}

func TestConfig_EnsureInitialized_Workspaces(t *testing.T) {
	cfg := &Config{}
	cfg.ensureInitialized()

	if cfg.Workspaces == nil {
		t.Error("Workspaces should be initialized")
	}
}

func TestConfig_ClearOrphanedParentIDs(t *testing.T) {
	cfg := &Config{
		Sessions: []Session{
			{ID: "child-1", ParentID: "parent-1", MergedToParent: true},
			{ID: "child-2", ParentID: "parent-2", MergedToParent: true},
			{ID: "child-3", ParentID: "parent-1"},
			{ID: "orphan", ParentID: ""},
			{ID: "unrelated", ParentID: "still-exists", MergedToParent: true},
		},
	}

	cfg.ClearOrphanedParentIDs([]string{"parent-1"})

	// child-1 and child-3 had ParentID=parent-1, should be cleared
	sess1 := cfg.GetSession("child-1")
	if sess1.ParentID != "" {
		t.Errorf("child-1 ParentID should be cleared, got %q", sess1.ParentID)
	}
	if sess1.MergedToParent {
		t.Error("child-1 MergedToParent should be cleared when parent is orphaned")
	}
	sess3 := cfg.GetSession("child-3")
	if sess3.ParentID != "" {
		t.Errorf("child-3 ParentID should be cleared, got %q", sess3.ParentID)
	}
	if sess3.MergedToParent {
		t.Error("child-3 MergedToParent should be cleared when parent is orphaned")
	}

	// child-2 had ParentID=parent-2, should be unchanged
	sess2 := cfg.GetSession("child-2")
	if sess2.ParentID != "parent-2" {
		t.Errorf("child-2 ParentID should be unchanged, got %q", sess2.ParentID)
	}
	if !sess2.MergedToParent {
		t.Error("child-2 MergedToParent should be unchanged")
	}

	// unrelated had ParentID=still-exists, should be unchanged
	unrelated := cfg.GetSession("unrelated")
	if unrelated.ParentID != "still-exists" {
		t.Errorf("unrelated ParentID should be unchanged, got %q", unrelated.ParentID)
	}
	if !unrelated.MergedToParent {
		t.Error("unrelated MergedToParent should be unchanged")
	}
}

func TestConfig_ClearOrphanedParentIDs_MultipleDeleted(t *testing.T) {
	cfg := &Config{
		Sessions: []Session{
			{ID: "a", ParentID: "del-1"},
			{ID: "b", ParentID: "del-2"},
			{ID: "c", ParentID: "keep"},
		},
	}

	cfg.ClearOrphanedParentIDs([]string{"del-1", "del-2"})

	a := cfg.GetSession("a")
	if a.ParentID != "" {
		t.Errorf("a ParentID should be cleared, got %q", a.ParentID)
	}
	b := cfg.GetSession("b")
	if b.ParentID != "" {
		t.Errorf("b ParentID should be cleared, got %q", b.ParentID)
	}
	c := cfg.GetSession("c")
	if c.ParentID != "keep" {
		t.Errorf("c ParentID should be unchanged, got %q", c.ParentID)
	}
}

func TestConfig_ClearOrphanedParentIDs_EmptyList(t *testing.T) {
	cfg := &Config{
		Sessions: []Session{
			{ID: "a", ParentID: "parent"},
		},
	}

	cfg.ClearOrphanedParentIDs([]string{})

	a := cfg.GetSession("a")
	if a.ParentID != "parent" {
		t.Errorf("ParentID should be unchanged when no deletions, got %q", a.ParentID)
	}
}

func TestConfig_ContainerImage(t *testing.T) {
	cfg := &Config{
		Repos:    []string{"/path/to/repo"},
		Sessions: []Session{},
	}

	// Default should be "plural-claude"
	if got := cfg.GetContainerImage(); got != "plural-claude" {
		t.Errorf("GetContainerImage default = %q, want 'plural-claude'", got)
	}

	// Set custom image
	cfg.SetContainerImage("my-custom-image")

	if got := cfg.GetContainerImage(); got != "my-custom-image" {
		t.Errorf("GetContainerImage = %q, want 'my-custom-image'", got)
	}

	// Set empty string should revert to default
	cfg.SetContainerImage("")

	if got := cfg.GetContainerImage(); got != "plural-claude" {
		t.Errorf("GetContainerImage after clearing = %q, want 'plural-claude'", got)
	}
}

func TestConfig_ContainerImage_Persistence(t *testing.T) {
	// Create a temp directory for test config
	tmpDir, err := os.MkdirTemp("", "plural-container-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.json")

	// Create config with container settings
	cfg := &Config{
		Repos:          []string{"/path/to/repo"},
		Sessions:       []Session{},
		ContainerImage: "my-image",
		filePath:       configPath,
	}

	// Save the config
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
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

	if loaded.ContainerImage != "my-image" {
		t.Errorf("ContainerImage = %q, want 'my-image'", loaded.ContainerImage)
	}
}

func TestSession_Containerized(t *testing.T) {
	// Test that Containerized field round-trips through JSON
	sess := Session{
		ID:            "test-id",
		RepoPath:      "/repo",
		WorkTree:      "/wt",
		Branch:        "main",
		Containerized: true,
	}

	data, err := json.Marshal(sess)
	if err != nil {
		t.Fatalf("Failed to marshal session: %v", err)
	}

	var loaded Session
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal session: %v", err)
	}

	if !loaded.Containerized {
		t.Error("Containerized should be true after round-trip")
	}

	// Test omitempty: false value should not be in JSON
	sess2 := Session{
		ID:            "test-id-2",
		RepoPath:      "/repo",
		WorkTree:      "/wt",
		Branch:        "main",
		Containerized: false,
	}

	data2, err := json.Marshal(sess2)
	if err != nil {
		t.Fatalf("Failed to marshal session: %v", err)
	}

	if strings.Contains(string(data2), "containerized") {
		t.Error("Containerized=false should be omitted from JSON (omitempty)")
	}
}

func TestConfig_ConcurrentSave(t *testing.T) {
	t.Parallel()

	// This test primarily detects data races when run with -race flag.
	// It verifies that concurrent Save() calls don't corrupt the config file,
	// but does not test atomicity of Save() with other operations like AddRepo.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := &Config{
		Repos:    []string{"/path/to/repo"},
		Sessions: []Session{},
		filePath: configPath,
	}

	// Launch multiple goroutines that all try to Save() concurrently
	const numGoroutines = 10
	errChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			// Just call Save() repeatedly to test concurrent file writes
			errChan <- cfg.Save()
		}()
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		if err := <-errChan; err != nil {
			t.Errorf("Save() failed in goroutine: %v", err)
		}
	}

	// Verify the config file is valid JSON and can be loaded
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Config file is corrupted (invalid JSON): %v", err)
	}

	// Verify the config matches what we expect
	if len(loaded.Repos) != 1 || loaded.Repos[0] != "/path/to/repo" {
		t.Errorf("Expected repos ['/path/to/repo'], got %v", loaded.Repos)
	}
}

func TestConfig_SaveRaceWithMutations(t *testing.T) {
	t.Parallel()

	// This test detects races between Save() and mutations (AddRepo, etc.)
	// when run with -race flag. It verifies Save() properly serializes with
	// write operations so concurrent file writes don't produce corrupt JSON.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := &Config{
		Repos:    []string{},
		Sessions: []Session{},
		filePath: configPath,
	}

	var wg sync.WaitGroup

	// Half the goroutines mutate the config
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				cfg.AddRepo(fmt.Sprintf("/repo/%d/%d", id, j))
			}
		}(i)
	}

	// Half the goroutines save the config
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				_ = cfg.Save()
			}
		}()
	}

	wg.Wait()

	// Final save and verify the file is valid JSON
	if err := cfg.Save(); err != nil {
		t.Fatalf("Final save failed: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Config file is corrupted (invalid JSON): %v", err)
	}
}
