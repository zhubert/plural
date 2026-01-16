package app

import (
	"regexp"
	"strings"
)

// DetectedOption represents a numbered or lettered option found in Claude's response
type DetectedOption struct {
	Number     int    // The option number (1, 2, 3, etc.) or letter index (A=1, B=2, etc.)
	Letter     string // The option letter if letter-based (A, B, C, etc.), empty if numeric
	Text       string // The option text
	GroupIndex int    // Which group this option belongs to (0-indexed)
}

// optionsTagPattern matches <options>...</options> blocks in Claude's response.
// Claude is instructed via system prompt to wrap numbered choices in these tags.
var optionsTagPattern = regexp.MustCompile(`(?s)<options>\s*(.*?)\s*</options>`)

// optgroupTagPattern matches <optgroup>...</optgroup> blocks within options.
var optgroupTagPattern = regexp.MustCompile(`(?s)<optgroup>\s*(.*?)\s*</optgroup>`)

// numericOptionPatterns are regexes that match numbered lists in Claude responses.
// These are used as fallback when <options> tags are not present.
var numericOptionPatterns = []*regexp.Regexp{
	// Standard numbered list: "1. Option text" or "1) Option text"
	regexp.MustCompile(`(?m)^(\d+)[.)]\s+(.+)$`),
	// Markdown bold option: "**Option 1:** text" or "**1.** text"
	regexp.MustCompile(`(?m)^\*\*(?:Option\s+)?(\d+)[.:]?\*\*:?\s*(.+)$`),
	// Markdown heading option: "## Option 1: text" or "### Option 1: text"
	regexp.MustCompile(`(?m)^#{2,3}\s+Option\s+(\d+):?\s*(.+)$`),
}

// letterOptionPatterns are regexes that match letter-based lists (A, B, C, etc.)
var letterOptionPatterns = []*regexp.Regexp{
	// Markdown heading option: "## Option A: text" or "### Option A: text"
	regexp.MustCompile(`(?m)^#{2,3}\s+Option\s+([A-Z]):?\s*(.+)$`),
	// Markdown bold option: "**Option A:** text" or "**A.** text"
	regexp.MustCompile(`(?m)^\*\*(?:Option\s+)?([A-Z])[.:]?\*\*:?\s*(.+)$`),
	// Standard letter list: "A. Option text" or "A) Option text"
	regexp.MustCompile(`(?m)^([A-Z])[.)]\s+(.+)$`),
}

// DetectOptions scans a message for numbered or lettered options.
// It first looks for <options> tags (most reliable), then falls back to
// pattern matching on numbered lists, then letter-based lists.
// Returns nil if no valid option list is found.
func DetectOptions(message string) []DetectedOption {
	// First, try to find options within <options> tags (most reliable)
	if options := detectOptionsFromTags(message); len(options) >= 2 {
		return options
	}

	// Try letter-based patterns first (they're more specific, like "Option A:")
	for _, pattern := range letterOptionPatterns {
		matches := pattern.FindAllStringSubmatch(message, -1)
		if len(matches) >= 2 {
			options := extractSequentialLetterOptions(matches)
			if len(options) >= 2 {
				return options
			}
		}
	}

	// Fallback: try numeric pattern matching
	for _, pattern := range numericOptionPatterns {
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
// Supports both:
// 1. Multiple <options> blocks (each block becomes a group)
// 2. Single <options> with <optgroup> tags inside (each optgroup becomes a group)
func detectOptionsFromTags(message string) []DetectedOption {
	matches := optionsTagPattern.FindAllStringSubmatch(message, -1)
	if len(matches) == 0 {
		return nil
	}

	// If there are multiple <options> blocks, treat each as a group
	if len(matches) > 1 {
		var result []DetectedOption
		for groupIdx, match := range matches {
			if len(match) < 2 {
				continue
			}
			groupOptions := parseOptionsFromContent(match[1], groupIdx)
			result = append(result, groupOptions...)
		}
		if len(result) >= 2 {
			return result
		}
	}

	// Single <options> block - check for <optgroup> tags inside
	content := matches[0][1]

	optgroupMatches := optgroupTagPattern.FindAllStringSubmatch(content, -1)
	if len(optgroupMatches) > 0 {
		var result []DetectedOption
		for groupIdx, optgroupMatch := range optgroupMatches {
			if len(optgroupMatch) < 2 {
				continue
			}
			groupContent := optgroupMatch[1]
			groupOptions := parseOptionsFromContent(groupContent, groupIdx)
			result = append(result, groupOptions...)
		}
		if len(result) >= 2 {
			return result
		}
	}

	// No optgroups, parse the content directly
	for _, pattern := range numericOptionPatterns {
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

// parseOptionsFromContent parses numbered options from content, assigning the given group index.
func parseOptionsFromContent(content string, groupIndex int) []DetectedOption {
	for _, pattern := range numericOptionPatterns {
		lineMatches := pattern.FindAllStringSubmatch(content, -1)
		if len(lineMatches) >= 2 {
			var options []DetectedOption
			for _, match := range lineMatches {
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
				options = append(options, DetectedOption{
					Number:     num,
					Text:       text,
					GroupIndex: groupIndex,
				})
			}
			if len(options) >= 2 {
				return options
			}
		}
	}
	return nil
}

// extractSequentialOptions finds all sequential runs of numbered options
// starting from 1. Returns all groups flattened with GroupIndex set.
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
		switch num {
		case expectedNum:
			currentGroup = append(currentGroup, DetectedOption{
				Number: num,
				Text:   text,
			})
		case 1:
			// Start a new group
			if len(currentGroup) >= 2 {
				allGroups = append(allGroups, currentGroup)
			}
			currentGroup = []DetectedOption{{
				Number: 1,
				Text:   text,
			}}
		default:
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

	// Flatten all groups with GroupIndex set
	var result []DetectedOption
	for groupIdx, group := range allGroups {
		for _, opt := range group {
			opt.GroupIndex = groupIdx
			result = append(result, opt)
		}
	}

	return result
}

// extractSequentialLetterOptions finds all sequential runs of lettered options
// starting from A. Returns all groups flattened with GroupIndex set.
func extractSequentialLetterOptions(matches [][]string) []DetectedOption {
	if len(matches) == 0 {
		return nil
	}

	// Find all option groups (sequences starting from A)
	var allGroups [][]DetectedOption
	var currentGroup []DetectedOption

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		letter := strings.ToUpper(match[1])
		if len(letter) != 1 || letter[0] < 'A' || letter[0] > 'Z' {
			continue
		}
		letterIndex := int(letter[0] - 'A' + 1) // A=1, B=2, etc.

		text := strings.TrimSpace(match[2])
		if text == "" {
			continue
		}

		// Check if this continues the sequence or starts a new one
		expectedIndex := len(currentGroup) + 1
		expectedLetter := string('A' + byte(expectedIndex-1))

		if letter == expectedLetter {
			currentGroup = append(currentGroup, DetectedOption{
				Number: letterIndex,
				Letter: letter,
				Text:   text,
			})
		} else if letter == "A" {
			// Start a new group
			if len(currentGroup) >= 2 {
				allGroups = append(allGroups, currentGroup)
			}
			currentGroup = []DetectedOption{{
				Number: 1,
				Letter: "A",
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

	// Flatten all groups with GroupIndex set
	var result []DetectedOption
	for groupIdx, group := range allGroups {
		for _, opt := range group {
			opt.GroupIndex = groupIdx
			result = append(result, opt)
		}
	}

	return result
}
