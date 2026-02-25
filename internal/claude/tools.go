package claude

// Tool sets are composable building blocks for allowed-tool lists.
// Consumers (TUI, daemon, agent) compose them explicitly via ComposeTools
// rather than relying on SessionManager to make policy decisions.

// ToolSetBase contains core file-operation tools safe for any environment.
var ToolSetBase = []string{
	"Read",
	"Glob",
	"Grep",
	"Edit",
	"Write",
	"ExitPlanMode",
}

// ToolSetSafeShell contains read-only shell commands safe for non-sandboxed environments.
var ToolSetSafeShell = []string{
	"Bash(ls:*)",
	"Bash(cat:*)",
	"Bash(head:*)",
	"Bash(tail:*)",
	"Bash(wc:*)",
	"Bash(pwd:*)",
}

// ToolSetContainerShell contains unrestricted Bash for container-sandboxed environments.
var ToolSetContainerShell = []string{
	"Bash",
}

// ToolSetWeb contains web access tools.
var ToolSetWeb = []string{
	"WebFetch",
	"WebSearch",
}

// ToolSetProductivity contains productivity and notebook tools.
var ToolSetProductivity = []string{
	"TodoRead",
	"TodoWrite",
	"NotebookEdit",
	"Task",
}

// ComposeTools merges multiple tool sets into a single deduplicated slice.
// Order is preserved (first occurrence wins).
func ComposeTools(sets ...[]string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, set := range sets {
		for _, tool := range set {
			if _, exists := seen[tool]; !exists {
				seen[tool] = struct{}{}
				result = append(result, tool)
			}
		}
	}
	return result
}
