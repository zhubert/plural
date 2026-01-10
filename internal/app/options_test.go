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
			name: "no options",
			message: `This is just a regular message without any numbered list.`,
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

func TestDetectOptions_WithOptionsTags(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		wantLen  int
		wantNums []int
	}{
		{
			name: "options in tags",
			message: `Here are some approaches I'd suggest:

<options>
1. Use a webhook-based architecture
2. Poll the API periodically
3. Use websockets for real-time updates
</options>

Let me know which one you'd like to explore.`,
			wantLen:  3,
			wantNums: []int{1, 2, 3},
		},
		{
			name: "options tags with surrounding whitespace",
			message: `<options>

1. First option
2. Second option

</options>`,
			wantLen:  2,
			wantNums: []int{1, 2},
		},
		{
			name: "multiple options blocks - returns all groups",
			message: `Earlier I mentioned:
<options>
1. Old option A
2. Old option B
</options>

But now I think these are better:
<options>
1. New option X
2. New option Y
3. New option Z
</options>`,
			wantLen:  5,
			wantNums: []int{1, 2, 1, 2, 3},
		},
		{
			name: "options tags with parentheses style",
			message: `<options>
1) First approach
2) Second approach
</options>`,
			wantLen:  2,
			wantNums: []int{1, 2},
		},
		{
			name: "empty options tags (fallback to pattern matching)",
			message: `<options>
</options>

But here are actual options:
1. Real option A
2. Real option B`,
			wantLen:  2,
			wantNums: []int{1, 2},
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

func TestDetectOptions_TagsPriorityOverFallback(t *testing.T) {
	// When both tagged and untagged options exist, tagged should be used
	message := `Some numbered list:
1. Untagged A
2. Untagged B

<options>
1. Tagged X
2. Tagged Y
</options>`

	options := DetectOptions(message)
	if len(options) != 2 {
		t.Fatalf("Expected 2 options, got %d", len(options))
	}

	if options[0].Text != "Tagged X" {
		t.Errorf("Option 1 text = %q, want %q", options[0].Text, "Tagged X")
	}
	if options[1].Text != "Tagged Y" {
		t.Errorf("Option 2 text = %q, want %q", options[1].Text, "Tagged Y")
	}
}

func TestDetectOptions_Optgroups(t *testing.T) {
	message := `<options>
<optgroup>
1. High priority A
2. High priority B
3. High priority C
</optgroup>
<optgroup>
1. Medium priority X
2. Medium priority Y
</optgroup>
<optgroup>
1. Low priority Z
2. Low priority W
</optgroup>
</options>`

	options := DetectOptions(message)
	if len(options) != 7 {
		t.Fatalf("Expected 7 options, got %d", len(options))
	}

	// Check first group (indices 0-2)
	for i := 0; i < 3; i++ {
		if options[i].GroupIndex != 0 {
			t.Errorf("Option %d GroupIndex = %d, want 0", i, options[i].GroupIndex)
		}
	}

	// Check second group (indices 3-4)
	for i := 3; i < 5; i++ {
		if options[i].GroupIndex != 1 {
			t.Errorf("Option %d GroupIndex = %d, want 1", i, options[i].GroupIndex)
		}
	}

	// Check third group (indices 5-6)
	for i := 5; i < 7; i++ {
		if options[i].GroupIndex != 2 {
			t.Errorf("Option %d GroupIndex = %d, want 2", i, options[i].GroupIndex)
		}
	}

	// Verify option text
	if options[0].Text != "High priority A" {
		t.Errorf("Option 0 text = %q, want %q", options[0].Text, "High priority A")
	}
	if options[3].Text != "Medium priority X" {
		t.Errorf("Option 3 text = %q, want %q", options[3].Text, "Medium priority X")
	}
	if options[5].Text != "Low priority Z" {
		t.Errorf("Option 5 text = %q, want %q", options[5].Text, "Low priority Z")
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

func TestDetectOptions_MultipleOptionsBlocks(t *testing.T) {
	// Test from actual production logs: multiple <options> blocks should all be included
	// Previously only the last block was returned
	message := `### Missing Features That Would Matter to Professional Developers

**High Value / Lower Effort:**
<options>
1. Session search within conversations
2. Session export to markdown
3. Desktop notifications
4. Session pinning/favorites
5. Token/cost tracking (if Claude CLI exposes it)
</options>

**High Value / Higher Effort:**
<options>
1. Session templates
2. Team sharing/collaboration
3. External tool integrations (webhooks)
4. Session archiving with restore
</options>

**Nice to Have:**
<options>
1. Custom shortcuts
2. Session tagging
3. Batch operations
4. Analytics dashboard
</options>`

	options := DetectOptions(message)
	if len(options) != 13 {
		t.Fatalf("Expected 13 options (5+4+4), got %d", len(options))
	}

	// First group (5 options, group index 0)
	for i := 0; i < 5; i++ {
		if options[i].GroupIndex != 0 {
			t.Errorf("Option %d GroupIndex = %d, want 0", i, options[i].GroupIndex)
		}
	}
	if options[0].Text != "Session search within conversations" {
		t.Errorf("Option 0 text = %q, want %q", options[0].Text, "Session search within conversations")
	}

	// Second group (4 options, group index 1)
	for i := 5; i < 9; i++ {
		if options[i].GroupIndex != 1 {
			t.Errorf("Option %d GroupIndex = %d, want 1", i, options[i].GroupIndex)
		}
	}
	if options[5].Text != "Session templates" {
		t.Errorf("Option 5 text = %q, want %q", options[5].Text, "Session templates")
	}

	// Third group (4 options, group index 2)
	for i := 9; i < 13; i++ {
		if options[i].GroupIndex != 2 {
			t.Errorf("Option %d GroupIndex = %d, want 2", i, options[i].GroupIndex)
		}
	}
	if options[9].Text != "Custom shortcuts" {
		t.Errorf("Option 9 text = %q, want %q", options[9].Text, "Custom shortcuts")
	}
}
