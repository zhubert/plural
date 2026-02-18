package workflow

import (
	"fmt"
	"strings"
)

// GenerateMermaid produces a mermaid stateDiagram-v2 string from a workflow config.
func GenerateMermaid(cfg *Config) string {
	var sb strings.Builder

	sb.WriteString("stateDiagram-v2\n")
	sb.WriteString("    [*] --> Polling\n")

	// Source
	provider := cfg.Source.Provider
	if provider == "" {
		provider = "github"
	}
	sb.WriteString(fmt.Sprintf("    Polling --> Queued : %s issue found\n", provider))

	// Coding
	sb.WriteString("    Queued --> Coding\n")
	codingNote := formatCodingNote(cfg)
	if codingNote != "" {
		sb.WriteString(fmt.Sprintf("    note right of Coding\n        %s\n    end note\n", codingNote))
	}
	if len(cfg.Workflow.Coding.After) > 0 {
		sb.WriteString("    Coding --> CodingHooks : after hooks\n")
		sb.WriteString("    CodingHooks --> PRCreation\n")
	} else {
		sb.WriteString("    Coding --> PRCreation\n")
	}

	// PR
	if len(cfg.Workflow.PR.After) > 0 {
		sb.WriteString("    PRCreation --> PRHooks : after hooks\n")
		sb.WriteString("    PRHooks --> AwaitingReview\n")
	} else {
		sb.WriteString("    PRCreation --> AwaitingReview\n")
	}

	// Review
	maxRounds := 3
	if cfg.Workflow.Review.MaxFeedbackRounds != nil {
		maxRounds = *cfg.Workflow.Review.MaxFeedbackRounds
	}
	sb.WriteString(fmt.Sprintf("    AwaitingReview --> AddressingFeedback : new comments (max %d rounds)\n", maxRounds))
	sb.WriteString("    AddressingFeedback --> AwaitingReview : push changes\n")
	if len(cfg.Workflow.Review.After) > 0 {
		sb.WriteString("    AddressingFeedback --> ReviewHooks : after hooks\n")
		sb.WriteString("    ReviewHooks --> AwaitingReview\n")
	}
	sb.WriteString("    AwaitingReview --> AwaitingCI : approved\n")
	sb.WriteString("    AwaitingReview --> Abandoned : PR closed\n")

	// CI
	onFailure := cfg.Workflow.CI.OnFailure
	if onFailure == "" {
		onFailure = "retry"
	}
	sb.WriteString("    AwaitingCI --> Merging : CI passed\n")
	switch onFailure {
	case "retry":
		sb.WriteString("    AwaitingCI --> AwaitingReview : CI failed (retry)\n")
	case "abandon":
		sb.WriteString("    AwaitingCI --> Abandoned : CI failed\n")
	case "notify":
		sb.WriteString("    AwaitingCI --> Failed : CI failed (notify)\n")
	}

	// Merge
	method := cfg.Workflow.Merge.Method
	if method == "" {
		method = "rebase"
	}
	sb.WriteString(fmt.Sprintf("    Merging --> Completed : %s merge\n", method))
	if len(cfg.Workflow.Merge.After) > 0 {
		sb.WriteString("    Completed --> MergeHooks : after hooks\n")
		sb.WriteString("    MergeHooks --> [*]\n")
	} else {
		sb.WriteString("    Completed --> [*]\n")
	}

	return sb.String()
}

func formatCodingNote(cfg *Config) string {
	var parts []string

	if cfg.Workflow.Coding.MaxTurns != nil {
		parts = append(parts, fmt.Sprintf("max_turns: %d", *cfg.Workflow.Coding.MaxTurns))
	}
	if cfg.Workflow.Coding.MaxDuration != nil {
		parts = append(parts, fmt.Sprintf("max_duration: %s", cfg.Workflow.Coding.MaxDuration.Duration))
	}
	if cfg.Workflow.Coding.Containerized != nil && *cfg.Workflow.Coding.Containerized {
		parts = append(parts, "containerized")
	}
	if cfg.Workflow.Coding.Supervisor != nil && *cfg.Workflow.Coding.Supervisor {
		parts = append(parts, "supervisor")
	}

	return strings.Join(parts, ", ")
}
