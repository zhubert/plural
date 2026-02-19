package agent

import "github.com/zhubert/plural/internal/config"

// Compile-time interface satisfaction check.
var _ AgentConfig = (*config.Config)(nil)
