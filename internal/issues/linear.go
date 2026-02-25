package issues

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	linearAPIBase      = "https://api.linear.app"
	linearAPIKeyEnvVar = "LINEAR_API_KEY"
	linearHTTPTimeout  = 30 * time.Second
)

// LinearTeam represents a Linear team with its ID and name.
type LinearTeam struct {
	ID   string
	Name string
}

// LinearProvider implements Provider for Linear Issues using the Linear GraphQL API.
type LinearProvider struct {
	config     LinearConfigProvider
	httpClient *http.Client
	apiBase    string // Override for testing; defaults to linearAPIBase
}

// NewLinearProvider creates a new Linear issue provider.
func NewLinearProvider(cfg LinearConfigProvider) *LinearProvider {
	return &LinearProvider{
		config: cfg,
		httpClient: &http.Client{
			Timeout: linearHTTPTimeout,
		},
		apiBase: linearAPIBase,
	}
}

// NewLinearProviderWithClient creates a new Linear issue provider with a custom HTTP client and API base URL (for testing).
func NewLinearProviderWithClient(cfg LinearConfigProvider, client *http.Client, apiBase string) *LinearProvider {
	if apiBase == "" {
		apiBase = linearAPIBase
	}
	return &LinearProvider{
		config:     cfg,
		httpClient: client,
		apiBase:    apiBase,
	}
}

// Name returns the human-readable name of this provider.
func (p *LinearProvider) Name() string {
	return "Linear Issues"
}

// Source returns the source type for this provider.
func (p *LinearProvider) Source() Source {
	return SourceLinear
}

// linearGraphQLRequest represents a GraphQL request body.
type linearGraphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// linearIssue represents an issue from the Linear GraphQL API response.
type linearIssue struct {
	ID          string `json:"id"`
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

// linearTeamIssuesResponse represents the Linear GraphQL response for team issues.
type linearTeamIssuesResponse struct {
	Data struct {
		Team struct {
			Issues struct {
				Nodes []linearIssue `json:"nodes"`
			} `json:"issues"`
		} `json:"team"`
	} `json:"data"`
}

// linearTeam represents a team from the Linear GraphQL API response.
type linearTeam struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// linearTeamsResponse represents the Linear GraphQL response for listing teams.
type linearTeamsResponse struct {
	Data struct {
		Teams struct {
			Nodes []linearTeam `json:"nodes"`
		} `json:"teams"`
	} `json:"data"`
}

// FetchIssues retrieves active issues from the Linear team.
// The projectID should be the Linear team ID.
func (p *LinearProvider) FetchIssues(ctx context.Context, repoPath, projectID string) ([]Issue, error) {
	apiKey := os.Getenv(linearAPIKeyEnvVar)
	if apiKey == "" {
		return nil, fmt.Errorf("LINEAR_API_KEY environment variable not set")
	}

	if projectID == "" {
		return nil, fmt.Errorf("Linear team ID not configured for this repository")
	}

	query := `query($teamId: String!) {
  team(id: $teamId) {
    issues(filter: { state: { type: { nin: ["completed", "canceled"] } } }) {
      nodes {
        id
        identifier
        title
        description
        url
      }
    }
  }
}`

	gqlReq := linearGraphQLRequest{
		Query: query,
		Variables: map[string]any{
			"teamId": projectID,
		},
	}

	body, err := json.Marshal(gqlReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GraphQL request: %w", err)
	}

	url := fmt.Sprintf("%s/graphql", p.apiBase)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch issues: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("Linear API returned 403 Forbidden - check that your LINEAR_API_KEY has access to this team")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Linear API returned status %d", resp.StatusCode)
	}

	var gqlResp linearTeamIssuesResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return nil, fmt.Errorf("failed to parse Linear response: %w", err)
	}

	nodes := gqlResp.Data.Team.Issues.Nodes
	issues := make([]Issue, len(nodes))
	for i, issue := range nodes {
		issues[i] = Issue{
			ID:     issue.Identifier,
			Title:  issue.Title,
			Body:   issue.Description,
			URL:    issue.URL,
			Source: SourceLinear,
		}
	}

	return issues, nil
}

// IsConfigured returns true if Linear is configured for the given repo.
// Requires both LINEAR_API_KEY env var and a team ID mapped to the repo.
func (p *LinearProvider) IsConfigured(repoPath string) bool {
	if os.Getenv(linearAPIKeyEnvVar) == "" {
		return false
	}
	return p.config.HasLinearTeam(repoPath)
}

// GenerateBranchName returns a branch name for the given Linear issue.
// Format: "linear-{identifier}" where identifier is lowercased (e.g., "linear-eng-123").
func (p *LinearProvider) GenerateBranchName(issue Issue) string {
	return fmt.Sprintf("linear-%s", strings.ToLower(issue.ID))
}

// GetPRLinkText returns the text to add to PR body to link/close the Linear issue.
// Linear supports auto-close via identifier mentions (e.g., "Fixes ENG-123").
func (p *LinearProvider) GetPRLinkText(issue Issue) string {
	return fmt.Sprintf("Fixes %s", issue.ID)
}

// FetchTeams retrieves all teams accessible to the user.
func (p *LinearProvider) FetchTeams(ctx context.Context) ([]LinearTeam, error) {
	apiKey := os.Getenv(linearAPIKeyEnvVar)
	if apiKey == "" {
		return nil, fmt.Errorf("LINEAR_API_KEY environment variable not set")
	}

	gqlReq := linearGraphQLRequest{
		Query: `{ teams { nodes { id name } } }`,
	}

	body, err := json.Marshal(gqlReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GraphQL request: %w", err)
	}

	url := fmt.Sprintf("%s/graphql", p.apiBase)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch teams: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Linear API returned status %d", resp.StatusCode)
	}

	var gqlResp linearTeamsResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return nil, fmt.Errorf("failed to parse Linear teams response: %w", err)
	}

	nodes := gqlResp.Data.Teams.Nodes
	teams := make([]LinearTeam, len(nodes))
	for i, team := range nodes {
		teams[i] = LinearTeam{
			ID:   team.ID,
			Name: team.Name,
		}
	}

	return teams, nil
}
