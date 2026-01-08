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
			name: "multiple lists - returns last",
			message: `First set:
1. A
2. B

Second set:
1. X
2. Y
3. Z`,
			wantLen:  3,
			wantNums: []int{1, 2, 3},
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
