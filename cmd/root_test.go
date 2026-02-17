package cmd

import (
	"testing"
)

func TestDebugFlagDefaultTrue(t *testing.T) {
	flag := rootCmd.PersistentFlags().Lookup("debug")
	if flag == nil {
		t.Fatal("--debug flag not found")
	}
	if flag.DefValue != "true" {
		t.Errorf("--debug default = %q, want %q", flag.DefValue, "true")
	}
}

func TestQuietFlagExists(t *testing.T) {
	flag := rootCmd.PersistentFlags().Lookup("quiet")
	if flag == nil {
		t.Fatal("--quiet flag not found")
	}
	if flag.DefValue != "false" {
		t.Errorf("--quiet default = %q, want %q", flag.DefValue, "false")
	}
	if flag.Shorthand != "q" {
		t.Errorf("--quiet shorthand = %q, want %q", flag.Shorthand, "q")
	}
}

func TestInitConfig_DefaultDebugEnabled(t *testing.T) {
	// Save and restore package state
	origDebug, origQuiet := debugMode, quietMode
	defer func() { debugMode, origQuiet = origDebug, origQuiet }()

	debugMode = true
	quietMode = false

	// Should not panic
	initConfig()
}

func TestInitConfig_QuietOverridesDebug(t *testing.T) {
	origDebug, origQuiet := debugMode, quietMode
	defer func() { debugMode, quietMode = origDebug, origQuiet }()

	debugMode = true
	quietMode = true

	// Should not panic - quiet should take precedence
	initConfig()
}
