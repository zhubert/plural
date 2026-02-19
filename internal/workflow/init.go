package workflow

import (
	"fmt"
	"os"
	"path/filepath"
)

// Template is the default workflow.yaml content with the new step-functions format.
const Template = `# Plural Agent Workflow Configuration
# See: https://github.com/zhubert/plural for full documentation
#
# This file defines a state machine that controls how the plural agent
# daemon processes issues. States are nodes connected by next (success)
# and error (failure) edges.

workflow: issue-to-merge
start: coding

source:
  provider: github          # github, asana, or linear
  filter:
    label: queued            # GitHub: issue label to poll
    # project: ""            # Asana: project GID (required for asana provider)
    # team: ""               # Linear: team ID (required for linear provider)

states:
  coding:
    type: task
    action: ai.code
    params:
      max_turns: 50            # Max autonomous turns per session
      max_duration: 30m        # Max wall-clock time per session
      # containerized: true    # Run sessions in Docker containers
      # supervisor: true       # Use supervisor mode for coding
      # system_prompt: ""      # Custom system prompt (inline or file:path/to/prompt.md)
    # after:                   # Hooks to run after coding completes
    #   - run: "make lint"
    next: open_pr
    error: failed

  open_pr:
    type: task
    action: github.create_pr
    params:
      link_issue: true         # Link PR to the source issue
      # template: ""           # PR body template (inline or file:path/to/template.md)
    # after:                   # Hooks to run after PR creation
    #   - run: "echo PR created"
    next: await_review
    error: failed

  await_review:
    type: wait
    event: pr.reviewed
    params:
      auto_address: true       # Automatically address PR review comments
      max_feedback_rounds: 3   # Max review/feedback cycles
      # system_prompt: ""      # Custom system prompt for review phase
    # after:                   # Hooks to run after review completes
    #   - run: "echo review done"
    next: await_ci
    error: failed

  await_ci:
    type: wait
    event: ci.complete
    timeout: 2h                # How long to wait for CI to complete
    params:
      on_failure: retry        # What to do on CI failure: retry, abandon, or notify
    next: merge
    error: failed

  merge:
    type: task
    action: github.merge
    params:
      method: rebase           # Merge method: rebase, squash, or merge
      cleanup: true            # Delete branch after merge
    # after:                   # Hooks to run after merge completes
    #   - run: "echo merged"
    next: done

  done:
    type: succeed

  failed:
    type: fail
`

// WriteTemplate writes the default workflow.yaml template to repoPath/.plural/workflow.yaml.
// Returns an error if the file already exists.
func WriteTemplate(repoPath string) (string, error) {
	dir := filepath.Join(repoPath, workflowDir)
	fp := filepath.Join(dir, workflowFileName)

	if _, err := os.Stat(fp); err == nil {
		return fp, fmt.Errorf("%s already exists", fp)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fp, fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(fp, []byte(Template), 0o644); err != nil {
		return fp, fmt.Errorf("failed to write %s: %w", fp, err)
	}

	return fp, nil
}
