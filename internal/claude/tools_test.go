package claude

import (
	"slices"
	"testing"
)

func TestComposeTools_Empty(t *testing.T) {
	result := ComposeTools()
	if len(result) != 0 {
		t.Errorf("ComposeTools() with no args should return empty, got %d items", len(result))
	}
}

func TestComposeTools_SingleSet(t *testing.T) {
	result := ComposeTools(ToolSetBase)
	if len(result) != len(ToolSetBase) {
		t.Errorf("expected %d tools, got %d", len(ToolSetBase), len(result))
	}
	for _, tool := range ToolSetBase {
		if !slices.Contains(result, tool) {
			t.Errorf("missing tool %q", tool)
		}
	}
}

func TestComposeTools_Dedup(t *testing.T) {
	// Both sets contain "Read" — it should appear only once
	set1 := []string{"Read", "Write"}
	set2 := []string{"Read", "Bash"}
	result := ComposeTools(set1, set2)

	if len(result) != 3 {
		t.Errorf("expected 3 tools after dedup, got %d: %v", len(result), result)
	}

	// Verify order: first occurrence wins
	if result[0] != "Read" || result[1] != "Write" || result[2] != "Bash" {
		t.Errorf("unexpected order: %v", result)
	}
}

func TestComposeTools_EmptySets(t *testing.T) {
	result := ComposeTools([]string{}, []string{}, ToolSetBase)
	if len(result) != len(ToolSetBase) {
		t.Errorf("expected %d tools, got %d", len(ToolSetBase), len(result))
	}
}

func TestDefaultAllowedTools_EquivalentToComposition(t *testing.T) {
	composed := ComposeTools(ToolSetBase, ToolSetSafeShell)
	if len(DefaultAllowedTools) != len(composed) {
		t.Errorf("DefaultAllowedTools has %d tools, ComposeTools(Base, SafeShell) has %d",
			len(DefaultAllowedTools), len(composed))
	}
	for _, tool := range DefaultAllowedTools {
		if !slices.Contains(composed, tool) {
			t.Errorf("DefaultAllowedTools contains %q but ComposeTools(Base, SafeShell) does not", tool)
		}
	}
}

func TestContainerAllowedTools_EquivalentToComposition(t *testing.T) {
	composed := ComposeTools(ToolSetBase, ToolSetContainerShell, ToolSetWeb, ToolSetProductivity)
	if len(containerAllowedTools) != len(composed) {
		t.Errorf("containerAllowedTools has %d tools, composed has %d",
			len(containerAllowedTools), len(composed))
	}
	for _, tool := range containerAllowedTools {
		if !slices.Contains(composed, tool) {
			t.Errorf("containerAllowedTools contains %q but composed does not", tool)
		}
	}
}

func TestToolSets_NoOverlap_BaseAndContainerShell(t *testing.T) {
	// Base has restricted Bash patterns, ContainerShell has unrestricted Bash.
	// They should not overlap.
	for _, tool := range ToolSetBase {
		if slices.Contains(ToolSetContainerShell, tool) {
			t.Errorf("ToolSetBase and ToolSetContainerShell overlap on %q", tool)
		}
	}
}

func TestToolSets_ContainerShell_HasBash(t *testing.T) {
	if !slices.Contains(ToolSetContainerShell, "Bash") {
		t.Error("ToolSetContainerShell should contain unrestricted Bash")
	}
}

func TestToolSets_SafeShell_NoUnrestrictedBash(t *testing.T) {
	for _, tool := range ToolSetSafeShell {
		if tool == "Bash" {
			t.Error("ToolSetSafeShell should not contain unrestricted Bash")
		}
	}
}
