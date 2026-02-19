package workflow

import (
	"fmt"
	"strings"
)

// GenerateMermaid produces a mermaid stateDiagram-v2 string from a workflow config.
func GenerateMermaid(cfg *Config) string {
	var sb strings.Builder

	sb.WriteString("stateDiagram-v2\n")
	sb.WriteString(fmt.Sprintf("    [*] --> %s\n", cfg.Start))

	// Walk each state and emit edges
	for name, state := range cfg.States {
		switch state.Type {
		case StateTypeSucceed, StateTypeFail:
			// Terminal states transition to [*]
			sb.WriteString(fmt.Sprintf("    %s --> [*]\n", name))

		case StateTypeTask:
			label := state.Action
			if state.Next != "" {
				sb.WriteString(fmt.Sprintf("    %s --> %s : %s\n", name, state.Next, label))
			}
			if state.Error != "" {
				sb.WriteString(fmt.Sprintf("    %s --> %s : error\n", name, state.Error))
			}

			// Emit after-hooks
			if len(state.After) > 0 {
				hookName := name + "_hooks"
				sb.WriteString(fmt.Sprintf("    %s --> %s : after hooks\n", name, hookName))
			}

		case StateTypeWait:
			label := state.Event
			if state.Timeout != nil {
				label += fmt.Sprintf(" (timeout: %s)", state.Timeout.Duration)
			}
			if state.Next != "" {
				sb.WriteString(fmt.Sprintf("    %s --> %s : %s\n", name, state.Next, label))
			}
			if state.Error != "" {
				sb.WriteString(fmt.Sprintf("    %s --> %s : error\n", name, state.Error))
			}
		}

		// Add note for params
		note := formatStateNote(state)
		if note != "" {
			sb.WriteString(fmt.Sprintf("    note right of %s\n        %s\n    end note\n", name, note))
		}
	}

	return sb.String()
}

// formatStateNote creates a note string from a state's params.
func formatStateNote(state *State) string {
	if len(state.Params) == 0 {
		return ""
	}

	var parts []string
	for k, v := range state.Params {
		parts = append(parts, fmt.Sprintf("%s: %v", k, v))
	}
	return strings.Join(parts, ", ")
}
