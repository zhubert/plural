package workflow

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
)

// HookContext provides environment variables for hook execution.
type HookContext struct {
	RepoPath  string
	Branch    string
	SessionID string
	IssueID   string
	IssueTitle string
	IssueURL  string
	PRURL     string
	WorkTree  string
	Provider  string
}

// envVars returns the hook context as environment variable pairs.
func (hc HookContext) envVars() []string {
	return []string{
		fmt.Sprintf("PLURAL_REPO_PATH=%s", hc.RepoPath),
		fmt.Sprintf("PLURAL_BRANCH=%s", hc.Branch),
		fmt.Sprintf("PLURAL_SESSION_ID=%s", hc.SessionID),
		fmt.Sprintf("PLURAL_ISSUE_ID=%s", hc.IssueID),
		fmt.Sprintf("PLURAL_ISSUE_TITLE=%s", hc.IssueTitle),
		fmt.Sprintf("PLURAL_ISSUE_URL=%s", hc.IssueURL),
		fmt.Sprintf("PLURAL_PR_URL=%s", hc.PRURL),
		fmt.Sprintf("PLURAL_WORKTREE=%s", hc.WorkTree),
		fmt.Sprintf("PLURAL_PROVIDER=%s", hc.Provider),
	}
}

// RunHooks executes hooks sequentially. Errors are logged but do not block the workflow.
func RunHooks(ctx context.Context, hooks []HookConfig, hookCtx HookContext, logger *slog.Logger) {
	for _, hook := range hooks {
		if hook.Run == "" {
			continue
		}

		cmd := exec.CommandContext(ctx, "sh", "-c", hook.Run)
		cmd.Dir = hookCtx.RepoPath
		cmd.Env = append(cmd.Environ(), hookCtx.envVars()...)

		output, err := cmd.CombinedOutput()
		if err != nil {
			logger.Warn("hook failed",
				"command", hook.Run,
				"error", err,
				"output", string(output),
			)
			continue
		}

		logger.Debug("hook completed",
			"command", hook.Run,
			"output", string(output),
		)
	}
}
