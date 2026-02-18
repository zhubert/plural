package app

import (
	"testing"
)

func TestDetectOptions(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		wantLen  int
		wantNums []int
	}{
		{
			name: "standard numbered list",
			message: `I see a few approaches here:
1. Use a webhook-based architecture
2. Poll the API periodically
3. Use websockets for real-time updates`,
			wantLen:  3,
			wantNums: []int{1, 2, 3},
		},
		{
			name: "parentheses style",
			message: `Here are some options:
1) First approach
2) Second approach
3) Third approach`,
			wantLen:  3,
			wantNums: []int{1, 2, 3},
		},
		{
			name: "markdown bold options",
			message: `**Option 1:** Use React
**Option 2:** Use Vue
**Option 3:** Use Svelte`,
			wantLen:  3,
			wantNums: []int{1, 2, 3},
		},
		{
			name: "markdown heading options",
			message: `## Option 1: Add an Animated Demo

**What:** Add a demo section.

---

## Option 2: Add Feature Comparison

**What:** Show before/after.

---

## Option 3: Add Interactive Preview

**What:** Make it interactive.`,
			wantLen:  3,
			wantNums: []int{1, 2, 3},
		},
		{
			name: "markdown h3 heading options",
			message: `### Option 1: First approach
Some details here.

### Option 2: Second approach
More details.`,
			wantLen:  2,
			wantNums: []int{1, 2},
		},
		{
			name: "only one option (not enough)",
			message: `Here's my suggestion:
1. Just do this one thing`,
			wantLen:  0,
			wantNums: nil,
		},
		{
			name:     "no options",
			message:  `This is just a regular message without any numbered list.`,
			wantLen:  0,
			wantNums: nil,
		},
		{
			name: "mixed content with options",
			message: `Let me explain the situation.

After analyzing your code, I think we have these options:

1. Refactor the existing module
2. Create a new module from scratch
3. Use a third-party library

Each has its pros and cons.`,
			wantLen:  3,
			wantNums: []int{1, 2, 3},
		},
		{
			name: "non-sequential numbers ignored",
			message: `Here are some items:
1. First
3. Third (skipped 2)
4. Fourth`,
			wantLen:  0, // Not sequential, so ignored
			wantNums: nil,
		},
		{
			name: "multiple lists - returns all groups",
			message: `First set:
1. A
2. B

Second set:
1. X
2. Y
3. Z`,
			wantLen:  5,
			wantNums: []int{1, 2, 1, 2, 3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := DetectOptions(tt.message)
			if len(options) != tt.wantLen {
				t.Errorf("DetectOptions() returned %d options, want %d", len(options), tt.wantLen)
				return
			}
			for i, opt := range options {
				if i < len(tt.wantNums) && opt.Number != tt.wantNums[i] {
					t.Errorf("Option %d has number %d, want %d", i, opt.Number, tt.wantNums[i])
				}
			}
		})
	}
}

func TestDetectOptions_ExtractsText(t *testing.T) {
	message := `Choose one:
1. Build a REST API
2. Build a GraphQL API`

	options := DetectOptions(message)
	if len(options) != 2 {
		t.Fatalf("Expected 2 options, got %d", len(options))
	}

	if options[0].Text != "Build a REST API" {
		t.Errorf("Option 1 text = %q, want %q", options[0].Text, "Build a REST API")
	}
	if options[1].Text != "Build a GraphQL API" {
		t.Errorf("Option 2 text = %q, want %q", options[1].Text, "Build a GraphQL API")
	}
}

func TestDetectOptions_MultipleGroupsWithGroupIndex(t *testing.T) {
	// Test that multiple lists without optgroup tags get proper GroupIndex values
	message := `Priority features:
1. Feature A
2. Feature B
3. Feature C

Nice to have:
1. Extra X
2. Extra Y`

	options := DetectOptions(message)
	if len(options) != 5 {
		t.Fatalf("Expected 5 options, got %d", len(options))
	}

	// First 3 should be group 0
	for i := 0; i < 3; i++ {
		if options[i].GroupIndex != 0 {
			t.Errorf("Option %d GroupIndex = %d, want 0", i, options[i].GroupIndex)
		}
	}

	// Last 2 should be group 1
	for i := 3; i < 5; i++ {
		if options[i].GroupIndex != 1 {
			t.Errorf("Option %d GroupIndex = %d, want 1", i, options[i].GroupIndex)
		}
	}
}

func TestDetectOptions_LetterBased(t *testing.T) {
	tests := []struct {
		name        string
		message     string
		wantLen     int
		wantLetters []string
		wantNums    []int
	}{
		{
			name: "markdown heading option letters",
			message: `## Option A: Rounded Nodes with Smooth Lines

Some description here.

---

## Option B: Bullet Style with Arcs

Another description.

---

## Option C: Metro/Subway Style

More details.`,
			wantLen:     3,
			wantLetters: []string{"A", "B", "C"},
			wantNums:    []int{1, 2, 3},
		},
		{
			name: "six letter options A-F",
			message: `## Option A: Minimal - Second Line Metadata

Details for A.

## Option B: Inline Compact Badges

Details for B.

## Option C: Visual Flow Lines

Details for C.

## Option D: Grouped Sections

Details for D.

## Option E: Vertical Timeline

Details for E.

## Option F: Bracket Style

Details for F.`,
			wantLen:     6,
			wantLetters: []string{"A", "B", "C", "D", "E", "F"},
			wantNums:    []int{1, 2, 3, 4, 5, 6},
		},
		{
			name: "markdown bold letter options",
			message: `**Option A:** Use React
**Option B:** Use Vue
**Option C:** Use Svelte`,
			wantLen:     3,
			wantLetters: []string{"A", "B", "C"},
			wantNums:    []int{1, 2, 3},
		},
		{
			name: "standard letter list",
			message: `Here are the approaches:
A. First approach
B. Second approach
C. Third approach`,
			wantLen:     3,
			wantLetters: []string{"A", "B", "C"},
			wantNums:    []int{1, 2, 3},
		},
		{
			name: "letter list with parentheses",
			message: `A) Option one
B) Option two`,
			wantLen:     2,
			wantLetters: []string{"A", "B"},
			wantNums:    []int{1, 2},
		},
		{
			name: "non-sequential letters ignored",
			message: `A. First
C. Third (skipped B)
D. Fourth`,
			wantLen:     0, // Not sequential, so ignored
			wantLetters: nil,
			wantNums:    nil,
		},
		{
			name:        "only one letter option (not enough)",
			message:     `## Option A: Just one option`,
			wantLen:     0,
			wantLetters: nil,
			wantNums:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := DetectOptions(tt.message)
			if len(options) != tt.wantLen {
				t.Errorf("DetectOptions() returned %d options, want %d", len(options), tt.wantLen)
				for i, opt := range options {
					t.Logf("  Option %d: Letter=%q Number=%d Text=%q", i, opt.Letter, opt.Number, opt.Text)
				}
				return
			}
			for i, opt := range options {
				if i < len(tt.wantLetters) && opt.Letter != tt.wantLetters[i] {
					t.Errorf("Option %d has letter %q, want %q", i, opt.Letter, tt.wantLetters[i])
				}
				if i < len(tt.wantNums) && opt.Number != tt.wantNums[i] {
					t.Errorf("Option %d has number %d, want %d", i, opt.Number, tt.wantNums[i])
				}
			}
		})
	}
}

func TestDetectOptions_LetterOptionsPreferredOverNumericFallback(t *testing.T) {
	// When a message has both letter-based options (like "## Option A:")
	// and a numeric list (like "1. 2. 3."), the letter options should be detected
	// because they're more specific (contain "Option" keyword)
	message := `## Option A: Rounded Nodes with Smooth Lines

The differences:

1. **Vertical spacing**: F has empty lines
2. **Node symbols**: B uses different shapes
3. **Line style**: B uses curved corner
4. **Connector length**: B has longer lines

## Option B: Bullet Style with Arcs

More details here.

## Option C: Metro Style

Final option.`

	options := DetectOptions(message)
	if len(options) != 3 {
		t.Fatalf("Expected 3 letter options, got %d", len(options))
	}

	// Should detect A, B, C letter options, not the 1, 2, 3, 4 numeric list
	if options[0].Letter != "A" {
		t.Errorf("Option 0 letter = %q, want %q", options[0].Letter, "A")
	}
	if options[1].Letter != "B" {
		t.Errorf("Option 1 letter = %q, want %q", options[1].Letter, "B")
	}
	if options[2].Letter != "C" {
		t.Errorf("Option 2 letter = %q, want %q", options[2].Letter, "C")
	}
}

