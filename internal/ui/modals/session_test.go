package modals

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/keys"
)

func makeRepos(n int) []string {
	repos := make([]string, n)
	for i := 0; i < n; i++ {
		repos[i] = fmt.Sprintf("/path/to/repo%d", i)
	}
	return repos
}

func sendKey(state *NewSessionState, key string) {
	state.Update(tea.KeyPressMsg{Code: -1, Text: key})
}

func TestNewSessionState_ScrollOffsetAdjustsOnNavigateDown(t *testing.T) {
	repos := makeRepos(15)
	state := NewNewSessionState(repos, false, false)

	// Navigate down past visible area
	for i := 0; i < 12; i++ {
		state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Down})
	}

	if state.RepoIndex != 12 {
		t.Errorf("expected RepoIndex 12, got %d", state.RepoIndex)
	}
	// ScrollOffset should have adjusted so index 12 is visible
	// visible range: [ScrollOffset, ScrollOffset+10)
	// 12 >= ScrollOffset+10 triggers adjustment
	expectedOffset := 12 - NewSessionMaxVisibleRepos + 1 // 3
	if state.ScrollOffset != expectedOffset {
		t.Errorf("expected ScrollOffset %d, got %d", expectedOffset, state.ScrollOffset)
	}
}

func TestNewSessionState_ScrollOffsetAdjustsOnNavigateUp(t *testing.T) {
	repos := makeRepos(15)
	state := NewNewSessionState(repos, false, false)

	// Navigate down to index 12
	for i := 0; i < 12; i++ {
		state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Down})
	}

	// Now navigate back up past the visible area
	for i := 0; i < 12; i++ {
		state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Up})
	}

	if state.RepoIndex != 0 {
		t.Errorf("expected RepoIndex 0, got %d", state.RepoIndex)
	}
	if state.ScrollOffset != 0 {
		t.Errorf("expected ScrollOffset 0, got %d", state.ScrollOffset)
	}
}

func TestNewSessionState_NoScrollIndicatorsWhenFewRepos(t *testing.T) {
	initTestStyles()

	repos := makeRepos(5)
	state := NewNewSessionState(repos, false, false)

	rendered := state.Render()
	if strings.Contains(rendered, "more above") {
		t.Error("should not show 'more above' indicator with only 5 repos")
	}
	if strings.Contains(rendered, "more below") {
		t.Error("should not show 'more below' indicator with only 5 repos")
	}
}

func TestNewSessionState_ScrollIndicatorsWhenManyRepos(t *testing.T) {
	initTestStyles()

	repos := makeRepos(15)
	state := NewNewSessionState(repos, false, false)

	// At start, should have "more below" but not "more above"
	rendered := state.Render()
	if strings.Contains(rendered, "more above") {
		t.Error("should not show 'more above' at top of list")
	}
	if !strings.Contains(rendered, "more below") {
		t.Error("should show 'more below' when repos exceed visible area")
	}

	// Navigate to middle
	for i := 0; i < 12; i++ {
		state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Down})
	}

	rendered = state.Render()
	if !strings.Contains(rendered, "more above") {
		t.Error("should show 'more above' when scrolled down")
	}
	if !strings.Contains(rendered, "more below") {
		t.Error("should show 'more below' when not at bottom")
	}

	// Navigate to bottom
	for i := 12; i < 14; i++ {
		state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Down})
	}

	rendered = state.Render()
	if !strings.Contains(rendered, "more above") {
		t.Error("should show 'more above' when at bottom")
	}
	if strings.Contains(rendered, "more below") {
		t.Error("should not show 'more below' when at bottom of list")
	}
}

func TestNewSessionState_RenderContainsVisibleReposOnly(t *testing.T) {
	initTestStyles()

	repos := makeRepos(15)
	state := NewNewSessionState(repos, false, false)

	rendered := state.Render()

	// Visible repos (0-9) should be in rendered output
	for i := 0; i < NewSessionMaxVisibleRepos; i++ {
		if !strings.Contains(rendered, repos[i]) {
			t.Errorf("expected visible repo %q in rendered output", repos[i])
		}
	}
	// Off-screen repos (10-14) should NOT be in rendered output
	for i := NewSessionMaxVisibleRepos; i < 15; i++ {
		if strings.Contains(rendered, repos[i]) {
			t.Errorf("did not expect off-screen repo %q in rendered output", repos[i])
		}
	}
}

func TestNewSessionState_ExactlyMaxVisibleRepos(t *testing.T) {
	initTestStyles()

	repos := makeRepos(NewSessionMaxVisibleRepos)
	state := NewNewSessionState(repos, false, false)

	rendered := state.Render()

	// All repos should be visible
	for _, repo := range repos {
		if !strings.Contains(rendered, repo) {
			t.Errorf("expected repo %q in rendered output", repo)
		}
	}

	// No scroll indicators
	if strings.Contains(rendered, "more above") {
		t.Error("should not show 'more above' with exactly max visible repos")
	}
	if strings.Contains(rendered, "more below") {
		t.Error("should not show 'more below' with exactly max visible repos")
	}

	// ScrollOffset should stay 0
	if state.ScrollOffset != 0 {
		t.Errorf("expected ScrollOffset 0, got %d", state.ScrollOffset)
	}
}

func TestNewSessionState_ScrollOffsetInitializedToZero(t *testing.T) {
	state := NewNewSessionState(makeRepos(20), false, false)
	if state.ScrollOffset != 0 {
		t.Errorf("expected initial ScrollOffset 0, got %d", state.ScrollOffset)
	}
}

func TestNewSessionState_NavigateDownDoesNotExceedBounds(t *testing.T) {
	repos := makeRepos(3)
	state := NewNewSessionState(repos, false, false)

	// Navigate down more times than there are items
	for i := 0; i < 10; i++ {
		state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Down})
	}

	if state.RepoIndex != 2 {
		t.Errorf("expected RepoIndex 2 (last item), got %d", state.RepoIndex)
	}
	if state.ScrollOffset != 0 {
		t.Errorf("expected ScrollOffset 0 (no scrolling needed), got %d", state.ScrollOffset)
	}
}

func TestNewSessionState_NavigateUpDoesNotGoBelowZero(t *testing.T) {
	repos := makeRepos(3)
	state := NewNewSessionState(repos, false, false)

	// Navigate up from index 0
	for i := 0; i < 5; i++ {
		state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Up})
	}

	if state.RepoIndex != 0 {
		t.Errorf("expected RepoIndex 0, got %d", state.RepoIndex)
	}
	if state.ScrollOffset != 0 {
		t.Errorf("expected ScrollOffset 0, got %d", state.ScrollOffset)
	}
}

func TestNewSessionState_ScrollOnlyAffectsRepoFocus(t *testing.T) {
	repos := makeRepos(15)
	state := NewNewSessionState(repos, false, false)

	// Switch focus to base selection
	state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Tab})

	// Navigate down in base selection
	state.Update(tea.KeyPressMsg{Code: -1, Text: keys.Down})

	// ScrollOffset should not change for base selection
	if state.ScrollOffset != 0 {
		t.Errorf("expected ScrollOffset 0 when navigating base options, got %d", state.ScrollOffset)
	}
	if state.BaseIndex != 1 {
		t.Errorf("expected BaseIndex 1, got %d", state.BaseIndex)
	}
}
