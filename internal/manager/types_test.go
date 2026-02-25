package manager

import "testing"

func TestMergeType_Push(t *testing.T) {
	if MergeTypePush.String() != "push" {
		t.Errorf("MergeTypePush.String() = %q, want 'push'", MergeTypePush.String())
	}
}

func TestMergeTypeConstants(t *testing.T) {
	if MergeTypeNone != 0 {
		t.Errorf("MergeTypeNone = %d, want 0", MergeTypeNone)
	}
	if MergeTypeMerge != 1 {
		t.Errorf("MergeTypeMerge = %d, want 1", MergeTypeMerge)
	}
	if MergeTypePR != 2 {
		t.Errorf("MergeTypePR = %d, want 2", MergeTypePR)
	}
	if MergeTypeParent != 3 {
		t.Errorf("MergeTypeParent = %d, want 3", MergeTypeParent)
	}
	if MergeTypePush != 4 {
		t.Errorf("MergeTypePush = %d, want 4", MergeTypePush)
	}
}
