package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
)

const prPollInterval = 30 * time.Second

// PRPollTickMsg triggers a PR status check cycle
type PRPollTickMsg time.Time

// PRStatusCheckMsg carries the result of checking a single session's PR state
type PRStatusCheckMsg struct {
	SessionID string
	State     git.PRState
	Error     error
}

// PRPollTick returns a command that sends a PRPollTickMsg after the poll interval
func PRPollTick() tea.Cmd {
	return tea.Tick(prPollInterval, func(t time.Time) tea.Msg {
		return PRPollTickMsg(t)
	})
}

// checkPRStatuses returns a batch of commands that check PR state for eligible sessions.
// Eligible sessions are those with PRCreated=true and PRMerged=false and PRClosed=false and Merged=false.
func checkPRStatuses(sessions []config.Session, gitSvc *git.GitService) tea.Cmd {
	var cmds []tea.Cmd

	for _, sess := range sessions {
		if !sess.PRCreated || sess.PRMerged || sess.PRClosed || sess.Merged {
			continue
		}

		// Capture loop variable for goroutine
		sessionID := sess.ID
		repoPath := sess.RepoPath
		branch := sess.Branch

		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			state, err := gitSvc.GetPRState(ctx, repoPath, branch)
			return PRStatusCheckMsg{
				SessionID: sessionID,
				State:     state,
				Error:     err,
			}
		})
	}

	if len(cmds) == 0 {
		return nil
	}

	return tea.Batch(cmds...)
}
