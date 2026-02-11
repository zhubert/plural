package issues

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/zhubert/plural/internal/config"
)

const (
	asanaAPIBase     = "https://app.asana.com/api/1.0"
	asanaPATEnvVar   = "ASANA_PAT"
	asanaHTTPTimeout = 30 * time.Second
)

// AsanaProject represents an Asana project with its GID and name.
type AsanaProject struct {
	GID  string
	Name string
}

// AsanaProvider implements Provider for Asana Tasks using the Asana REST API.
type AsanaProvider struct {
	config     *config.Config
	httpClient *http.Client
	apiBase    string // Override for testing; defaults to asanaAPIBase
}

// NewAsanaProvider creates a new Asana task provider.
func NewAsanaProvider(cfg *config.Config) *AsanaProvider {
	return &AsanaProvider{
		config: cfg,
		httpClient: &http.Client{
			Timeout: asanaHTTPTimeout,
		},
		apiBase: asanaAPIBase,
	}
}

// NewAsanaProviderWithClient creates a new Asana task provider with a custom HTTP client and API base URL (for testing).
func NewAsanaProviderWithClient(cfg *config.Config, client *http.Client, apiBase string) *AsanaProvider {
	if apiBase == "" {
		apiBase = asanaAPIBase
	}
	return &AsanaProvider{
		config:     cfg,
		httpClient: client,
		apiBase:    apiBase,
	}
}

// Name returns the human-readable name of this provider.
func (p *AsanaProvider) Name() string {
	return "Asana Tasks"
}

// Source returns the source type for this provider.
func (p *AsanaProvider) Source() Source {
	return SourceAsana
}

// asanaTask represents a task from the Asana API response.
type asanaTask struct {
	GID       string `json:"gid"`
	Name      string `json:"name"`
	Notes     string `json:"notes"`
	Permalink string `json:"permalink_url"`
}

// asanaTasksResponse represents the Asana API response for listing tasks.
type asanaTasksResponse struct {
	Data []asanaTask `json:"data"`
}

// FetchIssues retrieves incomplete tasks from the Asana project.
// The projectID should be the Asana project GID.
func (p *AsanaProvider) FetchIssues(ctx context.Context, repoPath, projectID string) ([]Issue, error) {
	pat := os.Getenv(asanaPATEnvVar)
	if pat == "" {
		return nil, fmt.Errorf("ASANA_PAT environment variable not set")
	}

	if projectID == "" {
		return nil, fmt.Errorf("Asana project GID not configured for this repository")
	}

	// Fetch incomplete tasks from the project
	url := fmt.Sprintf("%s/projects/%s/tasks?opt_fields=gid,name,notes,permalink_url&completed_since=now", p.apiBase, projectID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+pat)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tasks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("Asana API returned 403 Forbidden - check that your ASANA_PAT has access to this project")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Asana API returned status %d", resp.StatusCode)
	}

	var tasksResp asanaTasksResponse
	if err := json.NewDecoder(resp.Body).Decode(&tasksResp); err != nil {
		return nil, fmt.Errorf("failed to parse Asana response: %w", err)
	}

	issues := make([]Issue, len(tasksResp.Data))
	for i, task := range tasksResp.Data {
		issues[i] = Issue{
			ID:     task.GID,
			Title:  task.Name,
			Body:   task.Notes,
			URL:    task.Permalink,
			Source: SourceAsana,
		}
	}

	return issues, nil
}

// IsConfigured returns true if Asana is configured for the given repo.
// Requires both ASANA_PAT env var and a project GID mapped to the repo.
func (p *AsanaProvider) IsConfigured(repoPath string) bool {
	// Check if PAT is set
	if os.Getenv(asanaPATEnvVar) == "" {
		return false
	}
	// Check if repo has a project mapped
	return p.config.HasAsanaProject(repoPath)
}

// slugifyRegex is used to generate URL-safe slugs from task names.
var slugifyRegex = regexp.MustCompile(`[^a-z0-9]+`)

// GenerateBranchName returns a branch name for the given Asana task.
// Format: "task-{slug}" where slug is derived from the task name.
func (p *AsanaProvider) GenerateBranchName(issue Issue) string {
	// Convert to lowercase and replace non-alphanumeric chars with hyphens
	slug := strings.ToLower(issue.Title)
	slug = slugifyRegex.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")

	// Limit length to keep branch names reasonable
	const maxSlugLen = 40
	if len(slug) > maxSlugLen {
		slug = slug[:maxSlugLen]
		// Don't end on a hyphen
		slug = strings.TrimRight(slug, "-")
	}

	// Fallback if slug is empty
	if slug == "" {
		return fmt.Sprintf("task-%s", issue.ID)
	}

	return fmt.Sprintf("task-%s", slug)
}

// asanaWorkspace represents a workspace from the Asana API.
type asanaWorkspace struct {
	GID  string `json:"gid"`
	Name string `json:"name"`
}

// asanaWorkspacesResponse represents the Asana API response for listing workspaces.
type asanaWorkspacesResponse struct {
	Data []asanaWorkspace `json:"data"`
}

// asanaProject represents a project from the Asana API.
type asanaProject struct {
	GID  string `json:"gid"`
	Name string `json:"name"`
}

// asanaProjectsResponse represents the Asana API response for listing projects.
type asanaProjectsResponse struct {
	Data []asanaProject `json:"data"`
}

// FetchProjects retrieves all projects accessible to the user.
// If the user belongs to a single workspace, project names are returned directly.
// If multiple workspaces exist, names are prefixed with "WorkspaceName / ProjectName".
func (p *AsanaProvider) FetchProjects(ctx context.Context) ([]AsanaProject, error) {
	pat := os.Getenv(asanaPATEnvVar)
	if pat == "" {
		return nil, fmt.Errorf("ASANA_PAT environment variable not set")
	}

	workspaces, err := p.fetchWorkspaces(ctx, pat)
	if err != nil {
		return nil, err
	}

	if len(workspaces) == 0 {
		return nil, nil
	}

	multiWorkspace := len(workspaces) > 1

	var allProjects []AsanaProject
	for _, ws := range workspaces {
		projects, err := p.fetchWorkspaceProjects(ctx, pat, ws.GID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch projects for workspace %q: %w", ws.Name, err)
		}
		for _, proj := range projects {
			name := proj.Name
			if multiWorkspace {
				name = ws.Name + " / " + proj.Name
			}
			allProjects = append(allProjects, AsanaProject{
				GID:  proj.GID,
				Name: name,
			})
		}
	}

	return allProjects, nil
}

// fetchWorkspaces retrieves all workspaces for the authenticated user.
func (p *AsanaProvider) fetchWorkspaces(ctx context.Context, pat string) ([]asanaWorkspace, error) {
	url := fmt.Sprintf("%s/workspaces", p.apiBase)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+pat)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch workspaces: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Asana API returned status %d for workspaces", resp.StatusCode)
	}

	var wsResp asanaWorkspacesResponse
	if err := json.NewDecoder(resp.Body).Decode(&wsResp); err != nil {
		return nil, fmt.Errorf("failed to parse workspaces response: %w", err)
	}

	return wsResp.Data, nil
}

// fetchWorkspaceProjects retrieves all projects in a workspace.
func (p *AsanaProvider) fetchWorkspaceProjects(ctx context.Context, pat, workspaceGID string) ([]asanaProject, error) {
	url := fmt.Sprintf("%s/workspaces/%s/projects?opt_fields=gid,name&limit=100", p.apiBase, workspaceGID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+pat)
	req.Header.Set("Accept", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch projects: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Asana API returned status %d for projects", resp.StatusCode)
	}

	var projResp asanaProjectsResponse
	if err := json.NewDecoder(resp.Body).Decode(&projResp); err != nil {
		return nil, fmt.Errorf("failed to parse projects response: %w", err)
	}

	return projResp.Data, nil
}

// GetPRLinkText returns empty string for Asana tasks.
// Asana doesn't support auto-closing tasks via PR merge.
func (p *AsanaProvider) GetPRLinkText(issue Issue) string {
	// Asana doesn't have auto-close support via commit messages.
	// Users can manually link PRs in Asana or use the Asana GitHub integration.
	return ""
}
