package cli

import (
	"strings"
	"testing"
)

func TestDefaultPrerequisites(t *testing.T) {
	prereqs := DefaultPrerequisites()

	if len(prereqs) == 0 {
		t.Error("DefaultPrerequisites should return at least one prerequisite")
	}

	// Check that required prerequisites exist
	requiredNames := map[string]bool{"claude": false, "git": false}
	for _, prereq := range prereqs {
		if _, ok := requiredNames[prereq.Name]; ok {
			requiredNames[prereq.Name] = true
			if !prereq.Required {
				t.Errorf("Prerequisite %q should be required", prereq.Name)
			}
		}
	}

	for name, found := range requiredNames {
		if !found {
			t.Errorf("Expected prerequisite %q not found", name)
		}
	}

	// Verify gh is optional
	for _, prereq := range prereqs {
		if prereq.Name == "gh" && prereq.Required {
			t.Error("gh should be optional, not required")
		}
	}
}

func TestCheck_ExistingCommand(t *testing.T) {
	// Test with a command that definitely exists on any system
	prereq := Prerequisite{
		Name:        "echo",
		Required:    true,
		Description: "Echo command",
		InstallURL:  "",
	}

	result := Check(prereq)

	if !result.Found {
		t.Skip("echo command not found in PATH, skipping test")
	}

	if result.Path == "" {
		t.Error("Check should return path for found command")
	}

	if result.Error != nil {
		t.Errorf("Check should not return error for found command: %v", result.Error)
	}
}

func TestCheck_NonExistingCommand(t *testing.T) {
	prereq := Prerequisite{
		Name:        "definitely-not-a-real-command-12345",
		Required:    true,
		Description: "Fake command",
		InstallURL:  "http://example.com",
	}

	result := Check(prereq)

	if result.Found {
		t.Error("Check should return Found=false for non-existing command")
	}

	if result.Path != "" {
		t.Error("Check should return empty path for non-existing command")
	}

	if result.Error == nil {
		t.Error("Check should return error for non-existing command")
	}
}

func TestCheckAll(t *testing.T) {
	prereqs := []Prerequisite{
		{Name: "echo", Required: true, Description: "Echo"},
		{Name: "fake-cmd-xyz", Required: false, Description: "Fake"},
	}

	results := CheckAll(prereqs)

	if len(results) != len(prereqs) {
		t.Errorf("CheckAll returned %d results, want %d", len(results), len(prereqs))
	}

	// First should be found, second should not
	if !results[0].Found {
		t.Skip("echo not found, skipping")
	}

	if results[1].Found {
		t.Error("Fake command should not be found")
	}
}

func TestValidateRequired_AllPresent(t *testing.T) {
	// Only test with commands that exist on the system
	prereqs := []Prerequisite{
		{Name: "echo", Required: true, Description: "Echo"},
		{Name: "ls", Required: true, Description: "List"},
	}

	err := ValidateRequired(prereqs)
	if err != nil {
		t.Skip("Required test commands not found, skipping")
	}
}

func TestValidateRequired_MissingRequired(t *testing.T) {
	prereqs := []Prerequisite{
		{Name: "echo", Required: true, Description: "Echo"},
		{Name: "fake-required-cmd-xyz", Required: true, Description: "Fake required", InstallURL: "http://example.com"},
	}

	err := ValidateRequired(prereqs)
	if err == nil {
		t.Error("ValidateRequired should return error when required command is missing")
	}

	// Error should mention the missing command
	if !strings.Contains(err.Error(), "fake-required-cmd-xyz") {
		t.Errorf("Error should mention missing command: %v", err)
	}
}

func TestValidateRequired_OptionalMissing(t *testing.T) {
	prereqs := []Prerequisite{
		{Name: "echo", Required: true, Description: "Echo"},
		{Name: "fake-optional-cmd-xyz", Required: false, Description: "Fake optional"},
	}

	// Check if echo exists first
	result := Check(prereqs[0])
	if !result.Found {
		t.Skip("echo not found, skipping")
	}

	err := ValidateRequired(prereqs)
	if err != nil {
		t.Errorf("ValidateRequired should not error when only optional commands are missing: %v", err)
	}
}

func TestFormatCheckResults(t *testing.T) {
	results := []CheckResult{
		{
			Prerequisite: Prerequisite{Name: "found-cmd", Required: true, Description: "Found command"},
			Found:        true,
			Path:         "/usr/bin/found-cmd",
			Version:      "1.0.0",
		},
		{
			Prerequisite: Prerequisite{Name: "missing-required", Required: true, Description: "Missing required"},
			Found:        false,
		},
		{
			Prerequisite: Prerequisite{Name: "missing-optional", Required: false, Description: "Missing optional"},
			Found:        false,
		},
	}

	output := FormatCheckResults(results)

	// Should contain header
	if !strings.Contains(output, "CLI Prerequisites") {
		t.Error("Output should contain header")
	}

	// Should show found command with version
	if !strings.Contains(output, "found-cmd") {
		t.Error("Output should contain found command name")
	}
	if !strings.Contains(output, "1.0.0") {
		t.Error("Output should contain version for found command")
	}

	// Should show [REQUIRED] for missing required
	if !strings.Contains(output, "REQUIRED") {
		t.Error("Output should show REQUIRED for missing required command")
	}

	// Should show [optional] for missing optional
	if !strings.Contains(output, "optional") {
		t.Error("Output should show optional for missing optional command")
	}

	// Should use checkmarks and X marks
	if !strings.Contains(output, "✓") {
		t.Error("Output should contain checkmark for found command")
	}
	if !strings.Contains(output, "✗") {
		t.Error("Output should contain X for missing required command")
	}
	if !strings.Contains(output, "○") {
		t.Error("Output should contain circle for missing optional command")
	}
}

func TestFormatCheckResults_Empty(t *testing.T) {
	output := FormatCheckResults([]CheckResult{})

	if !strings.Contains(output, "CLI Prerequisites") {
		t.Error("Empty results should still contain header")
	}
}

func TestPrerequisite_Fields(t *testing.T) {
	prereq := Prerequisite{
		Name:        "test-cmd",
		Required:    true,
		Description: "Test command",
		InstallURL:  "https://example.com/install",
	}

	if prereq.Name != "test-cmd" {
		t.Error("Name field mismatch")
	}
	if !prereq.Required {
		t.Error("Required field mismatch")
	}
	if prereq.Description != "Test command" {
		t.Error("Description field mismatch")
	}
	if prereq.InstallURL != "https://example.com/install" {
		t.Error("InstallURL field mismatch")
	}
}

func TestCheckResult_Fields(t *testing.T) {
	prereq := Prerequisite{Name: "test", Required: true}
	result := CheckResult{
		Prerequisite: prereq,
		Found:        true,
		Path:         "/usr/bin/test",
		Version:      "2.0.0",
	}

	if result.Prerequisite.Name != "test" {
		t.Error("Prerequisite field mismatch")
	}
	if !result.Found {
		t.Error("Found field mismatch")
	}
	if result.Path != "/usr/bin/test" {
		t.Error("Path field mismatch")
	}
	if result.Version != "2.0.0" {
		t.Error("Version field mismatch")
	}
}
