package app

import (
	"bytes"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/zhubert/plural/internal/logger"
)

// runTestsForSession runs the test command for a session in its worktree directory.
// Returns a tea.Cmd that produces a TestRunResultMsg.
func runTestsForSession(sessionID, worktreePath, testCmd string, iteration int) tea.Cmd {
	return func() tea.Msg {
		log := logger.WithSession(sessionID)
		log.Info("running tests", "cmd", testCmd, "iteration", iteration, "worktree", worktreePath)

		// Parse the command - split on spaces but respect basic quoting
		parts := parseCommand(testCmd)
		if len(parts) == 0 {
			return TestRunResultMsg{
				SessionID: sessionID,
				Output:    "Error: empty test command",
				ExitCode:  1,
				Iteration: iteration,
			}
		}

		cmd := exec.Command(parts[0], parts[1:]...)
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

// parseCommand splits a command string into parts, handling basic shell quoting.
func parseCommand(cmd string) []string {
	// For simple cases, just split on whitespace
	// For more complex cases with quotes, do basic parsing
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return nil
	}

	var parts []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case c == '\'' && !inDoubleQuote:
			inSingleQuote = !inSingleQuote
		case c == '"' && !inSingleQuote:
			inDoubleQuote = !inDoubleQuote
		case c == ' ' && !inSingleQuote && !inDoubleQuote:
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(c)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}
