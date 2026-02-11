package issues

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/zhubert/plural/internal/config"
)

func TestAsanaProvider_Name(t *testing.T) {
	p := NewAsanaProvider(nil)
	if p.Name() != "Asana Tasks" {
		t.Errorf("expected 'Asana Tasks', got '%s'", p.Name())
	}
}

func TestAsanaProvider_Source(t *testing.T) {
	p := NewAsanaProvider(nil)
	if p.Source() != SourceAsana {
		t.Errorf("expected SourceAsana, got '%s'", p.Source())
	}
}

func TestAsanaProvider_IsConfigured(t *testing.T) {
	// Create a temporary config
	cfg := &config.Config{}
	cfg.SetAsanaProject("/test/repo", "12345")

	p := NewAsanaProvider(cfg)

	// Save and restore env var
	origPAT := os.Getenv(asanaPATEnvVar)
	defer os.Setenv(asanaPATEnvVar, origPAT)

	// Test without PAT
	os.Setenv(asanaPATEnvVar, "")
	if p.IsConfigured("/test/repo") {
		t.Error("expected IsConfigured=false without PAT")
	}

	// Test with PAT but without project mapping
	os.Setenv(asanaPATEnvVar, "test-pat")
	if p.IsConfigured("/other/repo") {
		t.Error("expected IsConfigured=false without project mapping")
	}

	// Test with both PAT and project mapping
	if !p.IsConfigured("/test/repo") {
		t.Error("expected IsConfigured=true with PAT and project mapping")
	}
}

func TestAsanaProvider_GenerateBranchName(t *testing.T) {
	p := NewAsanaProvider(nil)

	tests := []struct {
		name     string
		issue    Issue
		expected string
	}{
		{"simple title", Issue{ID: "123", Title: "Fix login bug"}, "task-fix-login-bug"},
		{"uppercase", Issue{ID: "123", Title: "URGENT Fix"}, "task-urgent-fix"},
		{"special chars", Issue{ID: "123", Title: "Fix bug #42"}, "task-fix-bug-42"},
		{"long title", Issue{ID: "123", Title: "This is a very long task title that should be truncated to keep branch names reasonable"}, "task-this-is-a-very-long-task-title-that-shou"},
		{"only special chars", Issue{ID: "123", Title: "!@#$%"}, "task-123"},
		{"empty title", Issue{ID: "123", Title: ""}, "task-123"},
		{"trailing hyphen", Issue{ID: "123", Title: "Fix bug - "}, "task-fix-bug"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := p.GenerateBranchName(tc.issue)
			if result != tc.expected {
				t.Errorf("GenerateBranchName(%q) = %s, expected %s", tc.issue.Title, result, tc.expected)
			}
		})
	}
}

func TestAsanaProvider_GetPRLinkText(t *testing.T) {
	p := NewAsanaProvider(nil)

	// Asana doesn't support auto-close
	result := p.GetPRLinkText(Issue{ID: "123", Source: SourceAsana})
	if result != "" {
		t.Errorf("expected empty string, got '%s'", result)
	}
}

func TestAsanaProvider_FetchIssues_NoPAT(t *testing.T) {
	origPAT := os.Getenv(asanaPATEnvVar)
	defer os.Setenv(asanaPATEnvVar, origPAT)

	os.Setenv(asanaPATEnvVar, "")

	cfg := &config.Config{}
	p := NewAsanaProvider(cfg)

	ctx := context.Background()
	_, err := p.FetchIssues(ctx, "/test/repo", "12345")
	if err == nil {
		t.Error("expected error without PAT")
	}
}

func TestAsanaProvider_FetchIssues_NoProjectID(t *testing.T) {
	origPAT := os.Getenv(asanaPATEnvVar)
	defer os.Setenv(asanaPATEnvVar, origPAT)

	os.Setenv(asanaPATEnvVar, "test-pat")

	cfg := &config.Config{}
	p := NewAsanaProvider(cfg)

	ctx := context.Background()
	_, err := p.FetchIssues(ctx, "/test/repo", "")
	if err == nil {
		t.Error("expected error without project ID")
	}
}

func TestAsanaProvider_FetchIssues_MockServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-pat" {
			t.Errorf("expected 'Bearer test-pat', got '%s'", auth)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		response := asanaTasksResponse{
			Data: []asanaTask{
				{GID: "1234567890", Name: "Task 1", Notes: "Description 1", Permalink: "https://app.asana.com/0/123/1234567890"},
				{GID: "0987654321", Name: "Task 2", Notes: "Description 2", Permalink: "https://app.asana.com/0/123/0987654321"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	origPAT := os.Getenv(asanaPATEnvVar)
	defer os.Setenv(asanaPATEnvVar, origPAT)
	os.Setenv(asanaPATEnvVar, "test-pat")

	cfg := &config.Config{}
	p := NewAsanaProviderWithClient(cfg, server.Client(), server.URL)

	ctx := context.Background()
	issues, err := p.FetchIssues(ctx, "/test/repo", "12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}
	if issues[0].Title != "Task 1" {
		t.Errorf("expected title 'Task 1', got %q", issues[0].Title)
	}
	if issues[0].Source != SourceAsana {
		t.Errorf("expected source SourceAsana, got %q", issues[0].Source)
	}
}

func TestAsanaProvider_FetchIssues_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer server.Close()

	origPAT := os.Getenv(asanaPATEnvVar)
	defer os.Setenv(asanaPATEnvVar, origPAT)
	os.Setenv(asanaPATEnvVar, "test-pat")

	cfg := &config.Config{}
	p := NewAsanaProviderWithClient(cfg, server.Client(), server.URL)

	ctx := context.Background()
	_, err := p.FetchIssues(ctx, "/test/repo", "12345")
	if err == nil {
		t.Error("expected error from API error response")
	}
}

func TestAsanaProvider_FetchProjects_NoPAT(t *testing.T) {
	origPAT := os.Getenv(asanaPATEnvVar)
	defer os.Setenv(asanaPATEnvVar, origPAT)

	os.Setenv(asanaPATEnvVar, "")

	p := NewAsanaProvider(nil)
	ctx := context.Background()
	_, err := p.FetchProjects(ctx)
	if err == nil {
		t.Error("expected error without PAT")
	}
}

func TestAsanaProvider_FetchProjects_SingleWorkspace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/workspaces":
			json.NewEncoder(w).Encode(asanaWorkspacesResponse{
				Data: []asanaWorkspace{
					{GID: "ws1", Name: "My Workspace"},
				},
			})
		case "/workspaces/ws1/projects":
			json.NewEncoder(w).Encode(asanaProjectsResponse{
				Data: []asanaProject{
					{GID: "p1", Name: "Project Alpha"},
					{GID: "p2", Name: "Project Beta"},
				},
			})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	origPAT := os.Getenv(asanaPATEnvVar)
	defer os.Setenv(asanaPATEnvVar, origPAT)
	os.Setenv(asanaPATEnvVar, "test-pat")

	p := NewAsanaProviderWithClient(nil, server.Client(), server.URL)

	ctx := context.Background()
	projects, err := p.FetchProjects(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	// Single workspace: names should NOT be prefixed
	if projects[0].Name != "Project Alpha" {
		t.Errorf("expected name 'Project Alpha', got %q", projects[0].Name)
	}
	if projects[0].GID != "p1" {
		t.Errorf("expected GID 'p1', got %q", projects[0].GID)
	}
	if projects[1].Name != "Project Beta" {
		t.Errorf("expected name 'Project Beta', got %q", projects[1].Name)
	}
}

func TestAsanaProvider_FetchProjects_MultipleWorkspaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/workspaces":
			json.NewEncoder(w).Encode(asanaWorkspacesResponse{
				Data: []asanaWorkspace{
					{GID: "ws1", Name: "Workspace A"},
					{GID: "ws2", Name: "Workspace B"},
				},
			})
		case "/workspaces/ws1/projects":
			json.NewEncoder(w).Encode(asanaProjectsResponse{
				Data: []asanaProject{
					{GID: "p1", Name: "Alpha"},
				},
			})
		case "/workspaces/ws2/projects":
			json.NewEncoder(w).Encode(asanaProjectsResponse{
				Data: []asanaProject{
					{GID: "p2", Name: "Beta"},
				},
			})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	origPAT := os.Getenv(asanaPATEnvVar)
	defer os.Setenv(asanaPATEnvVar, origPAT)
	os.Setenv(asanaPATEnvVar, "test-pat")

	p := NewAsanaProviderWithClient(nil, server.Client(), server.URL)

	ctx := context.Background()
	projects, err := p.FetchProjects(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}
	// Multiple workspaces: names should be prefixed
	if projects[0].Name != "Workspace A / Alpha" {
		t.Errorf("expected name 'Workspace A / Alpha', got %q", projects[0].Name)
	}
	if projects[1].Name != "Workspace B / Beta" {
		t.Errorf("expected name 'Workspace B / Beta', got %q", projects[1].Name)
	}
}

func TestAsanaProvider_FetchProjects_EmptyWorkspaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(asanaWorkspacesResponse{Data: []asanaWorkspace{}})
	}))
	defer server.Close()

	origPAT := os.Getenv(asanaPATEnvVar)
	defer os.Setenv(asanaPATEnvVar, origPAT)
	os.Setenv(asanaPATEnvVar, "test-pat")

	p := NewAsanaProviderWithClient(nil, server.Client(), server.URL)

	ctx := context.Background()
	projects, err := p.FetchProjects(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if projects != nil {
		t.Errorf("expected nil projects for empty workspaces, got %v", projects)
	}
}

func TestAsanaProvider_FetchProjects_WorkspacesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer server.Close()

	origPAT := os.Getenv(asanaPATEnvVar)
	defer os.Setenv(asanaPATEnvVar, origPAT)
	os.Setenv(asanaPATEnvVar, "test-pat")

	p := NewAsanaProviderWithClient(nil, server.Client(), server.URL)

	ctx := context.Background()
	_, err := p.FetchProjects(ctx)
	if err == nil {
		t.Error("expected error from API error response")
	}
}

func TestAsanaProvider_FetchProjects_ProjectsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/workspaces":
			json.NewEncoder(w).Encode(asanaWorkspacesResponse{
				Data: []asanaWorkspace{{GID: "ws1", Name: "WS"}},
			})
		default:
			http.Error(w, "Forbidden", http.StatusForbidden)
		}
	}))
	defer server.Close()

	origPAT := os.Getenv(asanaPATEnvVar)
	defer os.Setenv(asanaPATEnvVar, origPAT)
	os.Setenv(asanaPATEnvVar, "test-pat")

	p := NewAsanaProviderWithClient(nil, server.Client(), server.URL)

	ctx := context.Background()
	_, err := p.FetchProjects(ctx)
	if err == nil {
		t.Error("expected error from projects API error")
	}
}
