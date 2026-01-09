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

// optionsTagPattern matches <options>...</options> blocks in Claude's response.
// Claude is instructed via system prompt to wrap numbered choices in these tags.
var optionsTagPattern = regexp.MustCompile(`(?s)<options>\s*(.*?)\s*</options>`)

// optionPatterns are regexes that match numbered lists in Claude responses.
// These are used as fallback when <options> tags are not present.
var optionPatterns = []*regexp.Regexp{
	// Standard numbered list: "1. Option text" or "1) Option text"
	regexp.MustCompile(`(?m)^(\d+)[.)]\s+(.+)$`),
	// Markdown bold option: "**Option 1:** text" or "**1.** text"
	regexp.MustCompile(`(?m)^\*\*(?:Option\s+)?(\d+)[.:]?\*\*:?\s*(.+)$`),
	// Markdown heading option: "## Option 1: text" or "### Option 1: text"
	regexp.MustCompile(`(?m)^#{2,3}\s+Option\s+(\d+):?\s*(.+)$`),
}

// DetectOptions scans a message for numbered options.
// It first looks for <options> tags (most reliable), then falls back to
// pattern matching on numbered lists.
// Returns nil if no valid option list is found.
func DetectOptions(message string) []DetectedOption {
	// First, try to find options within <options> tags (most reliable)
	if options := detectOptionsFromTags(message); len(options) >= 2 {
		return options
	}

	// Fallback: try pattern matching
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

// detectOptionsFromTags extracts options from <options>...</options> blocks.
// Returns the options from the last block found (most recent).
func detectOptionsFromTags(message string) []DetectedOption {
	matches := optionsTagPattern.FindAllStringSubmatch(message, -1)
	if len(matches) == 0 {
		return nil
	}

	// Use the last match (most recent options block)
	lastMatch := matches[len(matches)-1]
	if len(lastMatch) < 2 {
		return nil
	}

	content := lastMatch[1]

	// Parse numbered options within the block
	for _, pattern := range optionPatterns {
		lineMatches := pattern.FindAllStringSubmatch(content, -1)
		if len(lineMatches) >= 2 {
			options := extractSequentialOptions(lineMatches)
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

