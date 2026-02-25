package issues

import "github.com/zhubert/plural/internal/config"

// Compile-time interface satisfaction checks.
var (
	_ AsanaConfigProvider  = (*config.Config)(nil)
	_ LinearConfigProvider = (*config.Config)(nil)
)

// AsanaConfigProvider defines the configuration interface required by AsanaProvider.
type AsanaConfigProvider interface {
	HasAsanaProject(repoPath string) bool
}

// LinearConfigProvider defines the configuration interface required by LinearProvider.
type LinearConfigProvider interface {
	HasLinearTeam(repoPath string) bool
}
