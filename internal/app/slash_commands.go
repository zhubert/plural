package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zhubert/plural/internal/logger"
)

// SlashCommandAction represents a UI action to perform after handling a slash command.
type SlashCommandAction int

const (
	ActionNone        SlashCommandAction = iota
	ActionOpenMCP                        // Open MCP servers modal
	ActionOpenPlugins                    // Open plugins modal
)

// SlashCommandResult represents the result of handling a slash command.
type SlashCommandResult struct {
	Handled  bool               // Whether the command was recognized and handled
	Response string             // The response to display to the user
	Action   SlashCommandAction // Optional UI action to trigger
}

// slashCommandDef defines a slash command with its handler and help text.
type slashCommandDef struct {
	name        string
	description string
}

// getSlashCommands returns the registry of available slash commands.
// Using a function instead of a var avoids initialization cycles.
func getSlashCommands() []slashCommandDef {
	return []slashCommandDef{
		{
			name:        "cost",
			description: "Show token usage and cost for the current session",
		},
		{
			name:        "help",
			description: "Show available slash commands",
		},
		{
			name:        "mcp",
			description: "Manage MCP servers",
		},
		{
			name:        "plugins",
			description: "Manage plugin directories",
		},
	}
}

// handleSlashCommand checks if the input is a slash command and handles it.
// Returns whether the command was handled and any response to display.
func (m *Model) handleSlashCommand(input string) SlashCommandResult {
	if !strings.HasPrefix(input, "/") {
		return SlashCommandResult{Handled: false}
	}

	// Parse command and arguments
	parts := strings.SplitN(strings.TrimPrefix(input, "/"), " ", 2)
	cmdName := strings.ToLower(parts[0])
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}

	logger.Get().Debug("slash command detected", "command", cmdName, "args", args)

	// Dispatch to the appropriate handler
	switch cmdName {
	case "cost":
		return handleCostCommand(m, args)
	case "help":
		return handleHelpCommand(m, args)
	case "mcp":
		return handleMCPCommand(m, args)
	case "plugin", "plugins":
		return handlePluginsCommand(m, args)
	default:
		// Unknown slash command - let Claude handle it (might be a custom command)
		logger.Get().Debug("unknown slash command, passing to Claude", "command", cmdName)
		return SlashCommandResult{Handled: false}
	}
}

// handleMCPCommand opens the MCP servers configuration modal.
func handleMCPCommand(_ *Model, _ string) SlashCommandResult {
	return SlashCommandResult{
		Handled: true,
		Action:  ActionOpenMCP,
	}
}

// handlePluginsCommand opens the plugins configuration modal.
func handlePluginsCommand(_ *Model, _ string) SlashCommandResult {
	return SlashCommandResult{
		Handled: true,
		Action:  ActionOpenPlugins,
	}
}

// handleHelpCommand shows available slash commands.
func handleHelpCommand(_ *Model, _ string) SlashCommandResult {
	var sb strings.Builder
	sb.WriteString("**Plural Slash Commands**\n\n")
	sb.WriteString("These commands are handled locally by Plural:\n\n")

	for _, cmd := range getSlashCommands() {
		fmt.Fprintf(&sb, "  **/%s** - %s\n", cmd.name, cmd.description)
	}

	sb.WriteString("\nOther slash commands (like /compact, /clear) are passed to Claude CLI.\n")
	sb.WriteString("Note: Built-in Claude CLI commands may have limited functionality in Plural.\n")

	return SlashCommandResult{
		Handled:  true,
		Response: sb.String(),
	}
}

// UsageStats represents token usage statistics from a Claude session.
type UsageStats struct {
	InputTokens              int64   `json:"input_tokens"`
	OutputTokens             int64   `json:"output_tokens"`
	CacheCreationInputTokens int64   `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64   `json:"cache_read_input_tokens"`
	TotalTokens              int64   `json:"total_tokens"`
	EstimatedCostUSD         float64 `json:"estimated_cost_usd"`
	MessageCount             int     `json:"message_count"`
}

// sessionJSONLEntry represents a single entry from Claude's session JSONL file.
type sessionJSONLEntry struct {
	Type    string `json:"type"`
	Message struct {
		ID    string `json:"id"` // Message ID - used to deduplicate streaming chunks
		Usage struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			CacheCreation            struct {
				Ephemeral5mInputTokens int64 `json:"ephemeral_5m_input_tokens"`
				Ephemeral1hInputTokens int64 `json:"ephemeral_1h_input_tokens"`
			} `json:"cache_creation"`
		} `json:"usage"`
		Model string `json:"model"`
	} `json:"message"`
}

// messageUsage tracks the maximum usage values seen for a single message ID.
// Claude's JSONL contains multiple streaming chunks per API call, each with cumulative
// token counts. We track the maximum to get the final (accurate) value.
type messageUsage struct {
	InputTokens              int64
	OutputTokens             int64
	CacheCreationInputTokens int64
	CacheReadInputTokens     int64
}

// handleCostCommand shows token usage and estimated cost for the current session.
func handleCostCommand(m *Model, _ string) SlashCommandResult {
	if m.activeSession == nil {
		return SlashCommandResult{
			Handled:  true,
			Response: "No active session. Create or select a session first.",
		}
	}

	sessionID := m.activeSession.ID
	workingDir := m.activeSession.WorkTree

	// Find the Claude session JSONL file
	stats, err := getSessionUsageStats(sessionID, workingDir)
	if err != nil {
		logger.WithSession(sessionID).Warn("failed to get session usage stats", "error", err)
		return SlashCommandResult{
			Handled:  true,
			Response: fmt.Sprintf("Could not retrieve usage data: %v", err),
		}
	}

	// Format the response
	var sb strings.Builder
	sb.WriteString("**Session Token Usage**\n\n")
	fmt.Fprintf(&sb, "  Session ID: %s\n\n", sessionID)
	fmt.Fprintf(&sb, "  Input tokens:  %s\n", formatNumber(stats.InputTokens))
	fmt.Fprintf(&sb, "  Output tokens: %s\n", formatNumber(stats.OutputTokens))
	if stats.CacheCreationInputTokens > 0 {
		fmt.Fprintf(&sb, "  Cache write:   %s\n", formatNumber(stats.CacheCreationInputTokens))
	}
	if stats.CacheReadInputTokens > 0 {
		fmt.Fprintf(&sb, "  Cache read:    %s\n", formatNumber(stats.CacheReadInputTokens))
	}
	fmt.Fprintf(&sb, "  **Total:       %s**\n\n", formatNumber(stats.TotalTokens))
	fmt.Fprintf(&sb, "  Messages: %d\n", stats.MessageCount)

	if stats.EstimatedCostUSD > 0 {
		fmt.Fprintf(&sb, "  Estimated cost: $%.4f\n", stats.EstimatedCostUSD)
	}

	return SlashCommandResult{
		Handled:  true,
		Response: sb.String(),
	}
}

// getSessionUsageStats reads the Claude session JSONL file and calculates usage statistics.
func getSessionUsageStats(sessionID string, workingDir string) (*UsageStats, error) {
	// Build the path to Claude's project directory
	// Claude stores session data in ~/.claude/projects/<escaped-path>/<session-id>.jsonl
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not get home directory: %w", err)
	}

	// Escape the working directory path to match Claude's convention
	// Claude replaces both "/" and "." with "-", keeping the leading dash
	escapedPath := strings.ReplaceAll(workingDir, "/", "-")
	escapedPath = strings.ReplaceAll(escapedPath, ".", "-")

	jsonlPath := filepath.Join(homeDir, ".claude", "projects", escapedPath, sessionID+".jsonl")

	logger.Get().Debug("looking for session JSONL", "path", jsonlPath)

	data, err := os.ReadFile(jsonlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session data not found (session may be new)")
		}
		return nil, fmt.Errorf("could not read session data: %w", err)
	}

	stats := &UsageStats{}
	lines := strings.Split(string(data), "\n")

	// Track max usage per message ID to deduplicate streaming chunks.
	// Claude's JSONL contains multiple entries per API call, each with cumulative
	// token counts. We only want the final (maximum) value for each message.
	messageUsages := make(map[string]*messageUsage)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry sessionJSONLEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // Skip malformed lines
		}

		// Only process assistant messages which have usage data
		if entry.Type == "assistant" && entry.Message.Usage.InputTokens > 0 {
			msgID := entry.Message.ID
			if msgID == "" {
				// Fallback for entries without message ID - generate a unique key
				// This shouldn't happen in practice but handles edge cases
				msgID = fmt.Sprintf("unknown-%d", len(messageUsages))
			}

			usage, exists := messageUsages[msgID]
			if !exists {
				usage = &messageUsage{}
				messageUsages[msgID] = usage
			}

			// Update to maximum values (token counts are cumulative within each API call)
			if entry.Message.Usage.InputTokens > usage.InputTokens {
				usage.InputTokens = entry.Message.Usage.InputTokens
			}
			if entry.Message.Usage.OutputTokens > usage.OutputTokens {
				usage.OutputTokens = entry.Message.Usage.OutputTokens
			}
			if entry.Message.Usage.CacheCreationInputTokens > usage.CacheCreationInputTokens {
				usage.CacheCreationInputTokens = entry.Message.Usage.CacheCreationInputTokens
			}
			if entry.Message.Usage.CacheReadInputTokens > usage.CacheReadInputTokens {
				usage.CacheReadInputTokens = entry.Message.Usage.CacheReadInputTokens
			}
		}
	}

	// Sum up the deduplicated usage values
	for _, usage := range messageUsages {
		stats.InputTokens += usage.InputTokens
		stats.OutputTokens += usage.OutputTokens
		stats.CacheCreationInputTokens += usage.CacheCreationInputTokens
		stats.CacheReadInputTokens += usage.CacheReadInputTokens
	}
	stats.MessageCount = len(messageUsages)

	stats.TotalTokens = stats.InputTokens + stats.OutputTokens +
		stats.CacheCreationInputTokens + stats.CacheReadInputTokens

	// Estimate cost based on Claude Opus 4 pricing (approximate)
	// Input: $15/1M tokens, Output: $75/1M tokens
	// Cache write: $18.75/1M tokens, Cache read: $1.50/1M tokens
	inputCost := float64(stats.InputTokens) / 1_000_000 * 15.0
	outputCost := float64(stats.OutputTokens) / 1_000_000 * 75.0
	cacheWriteCost := float64(stats.CacheCreationInputTokens) / 1_000_000 * 18.75
	cacheReadCost := float64(stats.CacheReadInputTokens) / 1_000_000 * 1.50
	stats.EstimatedCostUSD = inputCost + outputCost + cacheWriteCost + cacheReadCost

	return stats, nil
}

// formatNumber formats a number with thousand separators.
func formatNumber(n int64) string {
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}

	var result strings.Builder
	start := len(str) % 3
	if start == 0 {
		start = 3
	}
	result.WriteString(str[:start])

	for i := start; i < len(str); i += 3 {
		result.WriteString(",")
		result.WriteString(str[i : i+3])
	}

	return result.String()
}
