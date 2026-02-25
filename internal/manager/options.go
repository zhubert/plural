package manager

// DetectedOption represents a numbered or lettered option found in Claude's response
type DetectedOption struct {
	Number     int    // The option number (1, 2, 3, etc.) or letter index (A=1, B=2, etc.)
	Letter     string // The option letter if letter-based (A, B, C, etc.), empty if numeric
	Text       string // The option text
	GroupIndex int    // Which group this option belongs to (0-indexed)
}
