package manager

// MergeType represents the type of merge/PR operation.
// Using a typed enum instead of strings like "merge" or "pr"
// provides compile-time safety and clearer code.
type MergeType int

const (
	// MergeTypeNone indicates no merge operation is in progress.
	MergeTypeNone MergeType = iota

	// MergeTypeMerge indicates a direct merge to main branch.
	MergeTypeMerge

	// MergeTypePR indicates creating a pull request.
	MergeTypePR

	// MergeTypeParent indicates merging a child session back to its parent.
	MergeTypeParent

	// MergeTypePush indicates pushing updates to an existing PR.
	MergeTypePush
)

// String returns a human-readable name for the merge type.
func (t MergeType) String() string {
	switch t {
	case MergeTypeNone:
		return "none"
	case MergeTypeMerge:
		return "merge"
	case MergeTypePR:
		return "pr"
	case MergeTypeParent:
		return "parent"
	case MergeTypePush:
		return "push"
	default:
		return "unknown"
	}
}

