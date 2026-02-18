package workflow

import (
	"fmt"
	"os"
	"path/filepath"
)

// Template is the default workflow.yaml content with commented optional sections.
const Template = `# Plural Agent Workflow Configuration
# See: https://github.com/zhubert/plural for full documentation
#
# This file controls how the plural agent daemon processes issues.
# Only the source section is required — all other settings have sensible defaults.

source:
  provider: github          # github, asana, or linear
  filter:
    label: queued            # GitHub: issue label to poll
    # project: ""            # Asana: project GID (required for asana provider)
    # team: ""               # Linear: team ID (required for linear provider)

workflow:
  coding:
    max_turns: 50            # Max autonomous turns per session
    max_duration: 30m        # Max wall-clock time per session
    # containerized: true    # Run sessions in Docker containers
    # supervisor: true       # Use supervisor mode for coding
    # system_prompt: ""      # Custom system prompt (inline or file:path/to/prompt.md)
    # after:                 # Hooks to run after coding completes
    #   - run: "make lint"

  pr:
    # draft: false           # Create PRs as draft
    # link_issue: true       # Link PR to the source issue
    # template: ""           # PR body template (inline or file:path/to/template.md)
    # after:                 # Hooks to run after PR creation
    #   - run: "echo PR created"

  review:
    # auto_address: true     # Automatically address PR review comments
    # max_feedback_rounds: 3 # Max review/feedback cycles before giving up
    # system_prompt: ""      # Custom system prompt for review phase
    # after:                 # Hooks to run after review completes
    #   - run: "echo review done"

  ci:
    # timeout: 2h            # How long to wait for CI to complete
    # on_failure: retry      # What to do on CI failure: retry, abandon, or notify

  merge:
    # method: rebase         # Merge method: rebase, squash, or merge
    # cleanup: true          # Delete branch after merge
    # after:                 # Hooks to run after merge completes
    #   - run: "echo merged"
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
