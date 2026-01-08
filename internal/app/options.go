package app

import (
	"regexp"
	"strings"
)

// DetectedOption represents a numbered option found in Claude's response
type DetectedOption struct {
	Number int    // The option number (1, 2, 3, etc.)
	Text   string // The option text
}

// optionPatterns are regexes that match numbered lists in Claude responses
var optionPatterns = []*regexp.Regexp{
	// Standard numbered list: "1. Option text" or "1) Option text"
	regexp.MustCompile(`(?m)^(\d+)[.)]\s+(.+)$`),
	// Markdown bold option: "**Option 1:** text" or "**1.** text"
	regexp.MustCompile(`(?m)^\*\*(?:Option\s+)?(\d+)[.:]?\*\*:?\s*(.+)$`),
}

// DetectOptions scans a message for numbered options.
// It looks for the most recent numbered list (1, 2, 3...) in the message.
// Returns nil if no valid option list is found.
func DetectOptions(message string) []DetectedOption {
	// Try each pattern
	for _, pattern := range optionPatterns {
		matches := pattern.FindAllStringSubmatch(message, -1)
		if len(matches) >= 2 {
			options := extractSequentialOptions(matches)
			if len(options) >= 2 {
				return options
			}
		}
	}

	return nil
}

// extractSequentialOptions finds the last sequential run of numbered options
// starting from 1 (or continuing a sequence).
func extractSequentialOptions(matches [][]string) []DetectedOption {
	if len(matches) == 0 {
		return nil
	}

	// Find all option groups (sequences starting from 1)
	var allGroups [][]DetectedOption
	var currentGroup []DetectedOption

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		num := 0
		for _, c := range match[1] {
			if c >= '0' && c <= '9' {
				num = num*10 + int(c-'0')
			}
		}

		text := strings.TrimSpace(match[2])
		if text == "" {
			continue
		}

		// Check if this continues the sequence or starts a new one
		expectedNum := len(currentGroup) + 1
		if num == expectedNum {
			currentGroup = append(currentGroup, DetectedOption{
				Number: num,
				Text:   text,
			})
		} else if num == 1 {
			// Start a new group
			if len(currentGroup) >= 2 {
				allGroups = append(allGroups, currentGroup)
			}
			currentGroup = []DetectedOption{{
				Number: 1,
				Text:   text,
			}}
		} else {
			// Break in sequence, save current group if valid
			if len(currentGroup) >= 2 {
				allGroups = append(allGroups, currentGroup)
			}
			currentGroup = nil
		}
	}

	// Don't forget the last group
	if len(currentGroup) >= 2 {
		allGroups = append(allGroups, currentGroup)
	}

	// Return the last valid group (most recent in the message)
	if len(allGroups) > 0 {
		return allGroups[len(allGroups)-1]
	}

	return nil
}

// FindLastAssistantMessageWithOptions scans messages backwards to find
// the most recent assistant message containing numbered options.
// Returns the options and the index of the message containing them.
func FindLastAssistantMessageWithOptions(messages []struct{ Role, Content string }) ([]DetectedOption, int) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			if options := DetectOptions(messages[i].Content); len(options) >= 2 {
				return options, i
			}
		}
	}
	return nil, -1
}
