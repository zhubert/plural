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
