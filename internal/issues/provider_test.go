package issues

import (
	"context"
	"testing"
)

func TestProviderRegistry_GetConfiguredProviders(t *testing.T) {
	// Create a mock provider that's always configured
	mockConfigured := &mockProvider{
		name:       "Mock Configured",
		source:     "mock-configured",
		configured: true,
	}
	// Create a mock provider that's never configured
	mockNotConfigured := &mockProvider{
		name:       "Mock Not Configured",
		source:     "mock-not-configured",
		configured: false,
	}

	registry := NewProviderRegistry(mockConfigured, mockNotConfigured)

	providers := registry.GetConfiguredProviders("/some/repo")
	if len(providers) != 1 {
		t.Errorf("expected 1 configured provider, got %d", len(providers))
	}
	if len(providers) > 0 && providers[0].Source() != "mock-configured" {
		t.Errorf("expected mock-configured provider, got %s", providers[0].Source())
	}
}

func TestProviderRegistry_GetProvider(t *testing.T) {
	mockGitHub := &mockProvider{
		name:   "GitHub",
		source: SourceGitHub,
	}
	mockAsana := &mockProvider{
		name:   "Asana",
		source: SourceAsana,
	}

	registry := NewProviderRegistry(mockGitHub, mockAsana)

	// Test finding existing providers
	p := registry.GetProvider(SourceGitHub)
	if p == nil {
		t.Error("expected to find GitHub provider")
	}
	if p != nil && p.Source() != SourceGitHub {
		t.Errorf("expected SourceGitHub, got %s", p.Source())
	}

	p = registry.GetProvider(SourceAsana)
	if p == nil {
		t.Error("expected to find Asana provider")
	}
	if p != nil && p.Source() != SourceAsana {
		t.Errorf("expected SourceAsana, got %s", p.Source())
	}

	// Test unknown provider
	p = registry.GetProvider("unknown")
	if p != nil {
		t.Error("expected nil for unknown provider")
	}
}

func TestProviderRegistry_AllProviders(t *testing.T) {
	mockGitHub := &mockProvider{name: "GitHub", source: SourceGitHub}
	mockAsana := &mockProvider{name: "Asana", source: SourceAsana}

	registry := NewProviderRegistry(mockGitHub, mockAsana)

	providers := registry.AllProviders()
	if len(providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(providers))
	}
}

// mockProvider implements Provider for testing
type mockProvider struct {
	name       string
	source     Source
	configured bool
	issues     []Issue
	err        error
}

func (m *mockProvider) Name() string   { return m.name }
func (m *mockProvider) Source() Source { return m.source }

func (m *mockProvider) FetchIssues(_ context.Context, _, _ string) ([]Issue, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.issues, nil
}

func (m *mockProvider) IsConfigured(_ string) bool {
	return m.configured
}

func (m *mockProvider) GenerateBranchName(issue Issue) string {
	return "branch-" + issue.ID
}

func (m *mockProvider) GetPRLinkText(_ Issue) string {
	return ""
}
