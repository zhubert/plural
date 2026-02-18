package ui

import (
	"sync"
	"testing"
)

func TestGetViewContext_Singleton(t *testing.T) {
	// Reset the singleton for testing
	// Note: This test verifies the singleton pattern works
	ctx1 := GetViewContext()
	ctx2 := GetViewContext()

	if ctx1 != ctx2 {
		t.Error("GetViewContext should return the same instance")
	}
}

func TestViewContext_UpdateTerminalSize(t *testing.T) {
	ctx := GetViewContext()

	ctx.UpdateTerminalSize(120, 40)

	if ctx.TerminalWidth != 120 {
		t.Errorf("Expected TerminalWidth 120, got %d", ctx.TerminalWidth)
	}

	if ctx.TerminalHeight != 40 {
		t.Errorf("Expected TerminalHeight 40, got %d", ctx.TerminalHeight)
	}

	if ctx.HeaderHeight != HeaderHeight {
		t.Errorf("Expected HeaderHeight %d, got %d", HeaderHeight, ctx.HeaderHeight)
	}

	if ctx.FooterHeight != FooterHeight {
		t.Errorf("Expected FooterHeight %d, got %d", FooterHeight, ctx.FooterHeight)
	}

	expectedContent := 40 - HeaderHeight - FooterHeight
	if ctx.ContentHeight != expectedContent {
		t.Errorf("Expected ContentHeight %d, got %d", expectedContent, ctx.ContentHeight)
	}

	expectedSidebar := 120 / SidebarWidthRatio
	if ctx.SidebarWidth != expectedSidebar {
		t.Errorf("Expected SidebarWidth %d, got %d", expectedSidebar, ctx.SidebarWidth)
	}

	expectedChat := 120 - expectedSidebar
	if ctx.ChatWidth != expectedChat {
		t.Errorf("Expected ChatWidth %d, got %d", expectedChat, ctx.ChatWidth)
	}
}

func TestViewContext_InnerWidth(t *testing.T) {
	ctx := GetViewContext()

	tests := []struct {
		panelWidth int
		expected   int
	}{
		{40, 40 - BorderSize},
		{80, 80 - BorderSize},
		{10, 10 - BorderSize},
		{BorderSize, 0},
	}

	for _, tt := range tests {
		result := ctx.InnerWidth(tt.panelWidth)
		if result != tt.expected {
			t.Errorf("InnerWidth(%d) = %d, want %d", tt.panelWidth, result, tt.expected)
		}
	}
}

func TestViewContext_InnerHeight(t *testing.T) {
	ctx := GetViewContext()

	tests := []struct {
		panelHeight int
		expected    int
	}{
		{24, 24 - BorderSize},
		{40, 40 - BorderSize},
		{10, 10 - BorderSize},
		{BorderSize, 0},
	}

	for _, tt := range tests {
		result := ctx.InnerHeight(tt.panelHeight)
		if result != tt.expected {
			t.Errorf("InnerHeight(%d) = %d, want %d", tt.panelHeight, result, tt.expected)
		}
	}
}

func TestViewContext_Log(t *testing.T) {
	ctx := GetViewContext()

	// Should not panic when logging
	ctx.Log("Test message: %d", 42)
	ctx.Log("Another test: %s, %v", "hello", true)
}

func TestViewContext_ConcurrentAccess(t *testing.T) {
	ctx := GetViewContext()

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ctx.UpdateTerminalSize(80+n, 24+n)
			_ = ctx.InnerWidth(40)
			_ = ctx.InnerHeight(20)
		}(i)
	}
	wg.Wait()

	// Should not panic or deadlock
}

func TestLayoutConstants(t *testing.T) {
	// Verify constants are reasonable
	if HeaderHeight < 1 {
		t.Errorf("HeaderHeight should be at least 1, got %d", HeaderHeight)
	}

	if FooterHeight < 1 {
		t.Errorf("FooterHeight should be at least 1, got %d", FooterHeight)
	}

	if BorderSize < 0 {
		t.Errorf("BorderSize should be non-negative, got %d", BorderSize)
	}

	if SidebarWidthRatio < 2 {
		t.Errorf("SidebarWidthRatio should be at least 2, got %d", SidebarWidthRatio)
	}
}
