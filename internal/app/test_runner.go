package app

import (
	"bytes"
	"os/exec"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/logger"
)

// runTestsForSession runs the test command for a session in its worktree directory.
// The command is run through a shell (sh -c) to support pipes, redirects, and other
// shell semantics that users would expect.
// Returns a tea.Cmd that produces a TestRunResultMsg.
func runTestsForSession(sessionID, worktreePath, testCmd string, iteration int) tea.Cmd {
	return func() tea.Msg {
		log := logger.WithSession(sessionID)
		log.Info("running tests", "cmd", testCmd, "iteration", iteration, "worktree", worktreePath)

		testCmd = strings.TrimSpace(testCmd)
		if testCmd == "" {
			return TestRunResultMsg{
				SessionID: sessionID,
				Output:    "Error: empty test command",
				ExitCode:  1,
				Iteration: iteration,
			}
		}

		// Run through shell to support pipes, redirects, and other shell semantics
		shell := "sh"
		if runtime.GOOS == "windows" {
			shell = "cmd"
		}
		cmd := exec.Command(shell, "-c", testCmd)
		cmd.Dir = worktreePath

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()

		// Combine stdout and stderr
		output := stdout.String()
		if errOutput := stderr.String(); errOutput != "" {
			if output != "" {
				output += "\n"
			}
			output += errOutput
		}

		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
				output += "\nError: " + err.Error()
			}
		}

		log.Info("test run complete", "exitCode", exitCode, "outputLen", len(output))

		return TestRunResultMsg{
			SessionID: sessionID,
			Output:    output,
			ExitCode:  exitCode,
			Iteration: iteration,
		}
	}
}

