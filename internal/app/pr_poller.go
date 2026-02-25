package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/config"
	"github.com/zhubert/plural/internal/git"
	"github.com/zhubert/plural/internal/logger"
)

const prPollInterval = 30 * time.Second

// PRPollTickMsg triggers a PR status check cycle
type PRPollTickMsg time.Time

// PRStatusResult carries the result of checking a single session's PR state within a batch
type PRStatusResult struct {
	SessionID    string
	State        git.PRState
	CommentCount int // Total comments + reviews from gh pr list
}

// PRBatchStatusCheckMsg carries the results of checking all eligible sessions' PR states
type PRBatchStatusCheckMsg struct {
	Results []PRStatusResult
	Error   error
}

// PRPollTick returns a command that sends a PRPollTickMsg after the poll interval
func PRPollTick() tea.Cmd {
	return tea.Tick(prPollInterval, func(t time.Time) tea.Msg {
		return PRPollTickMsg(t)
	})
}

// eligibleSession holds the info needed to check a session's PR state
type eligibleSession struct {
	ID       string
	RepoPath string
	Branch   string
}

// getEligibleSessions filters sessions to those that need PR state checking.
// Eligible sessions are those with PRCreated=true and PRMerged=false and PRClosed=false and Merged=false.
func getEligibleSessions(sessions []config.Session) []eligibleSession {
	var eligible []eligibleSession
	for _, sess := range sessions {
		if !sess.PRCreated || sess.PRMerged || sess.PRClosed || sess.Merged {
			continue
		}
		eligible = append(eligible, eligibleSession{
			ID:       sess.ID,
			RepoPath: sess.RepoPath,
			Branch:   sess.Branch,
		})
	}
	return eligible
}

// checkPRStatuses returns a single command that checks PR state for all eligible sessions.
// Sessions are grouped by repo so only one gh CLI call is made per repo.
func checkPRStatuses(sessions []config.Session, gitSvc *git.GitService) tea.Cmd {
	eligible := getEligibleSessions(sessions)
	if len(eligible) == 0 {
		return nil
	}

	return func() tea.Msg {
		log := logger.WithComponent("pr-poller")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Group sessions by repo path
		repoSessions := make(map[string][]eligibleSession)
		for _, s := range eligible {
			repoSessions[s.RepoPath] = append(repoSessions[s.RepoPath], s)
		}

		var results []PRStatusResult

		// One gh call per repo
		for repoPath, sessions := range repoSessions {
			branches := make([]string, len(sessions))
			for i, s := range sessions {
				branches[i] = s.Branch
			}

			batchResults, err := gitSvc.GetBatchPRStatesWithComments(ctx, repoPath, branches)
			if err != nil {
				log.Debug("batch PR status check failed", "repo", repoPath, "error", err)
				continue
			}

			// Build a branch->sessionID lookup for this repo
			for _, s := range sessions {
				if br, ok := batchResults[s.Branch]; ok {
					results = append(results, PRStatusResult{
						SessionID:    s.ID,
						State:        br.State,
						CommentCount: br.CommentCount,
					})
				}
			}
		}

		return PRBatchStatusCheckMsg{Results: results}
	}
}
