package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zhubert/plural/internal/workflow"
)

var workflowRepoPath string

var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Manage workflow configuration",
	Long:  `Commands for validating and visualizing .plural/workflow.yaml configuration files.`,
}

var workflowValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate .plural/workflow.yaml",
	Long:  `Loads and validates the workflow configuration file in the specified repository.`,
	RunE:  runWorkflowValidate,
}

var workflowVisualizeCmd = &cobra.Command{
	Use:   "visualize",
	Short: "Generate mermaid diagram of workflow",
	Long:  `Generates a mermaid stateDiagram-v2 from the workflow configuration and prints it to stdout.`,
	RunE:  runWorkflowVisualize,
}

func init() {
	workflowCmd.PersistentFlags().StringVar(&workflowRepoPath, "repo", ".", "Path to the repository")
	workflowCmd.AddCommand(workflowValidateCmd)
	workflowCmd.AddCommand(workflowVisualizeCmd)
	rootCmd.AddCommand(workflowCmd)
}

func runWorkflowValidate(cmd *cobra.Command, args []string) error {
	cfg, err := workflow.Load(workflowRepoPath)
	if err != nil {
		return fmt.Errorf("failed to load workflow config: %w", err)
	}

	if cfg == nil {
		fmt.Fprintln(os.Stderr, "No .plural/workflow.yaml found, using defaults.")
		cfg = workflow.DefaultConfig()
	}

	errs := workflow.Validate(cfg)
	if len(errs) == 0 {
		fmt.Println("Workflow configuration is valid.")
		fmt.Printf("  Provider: %s\n", cfg.Source.Provider)
		if cfg.Source.Filter.Label != "" {
			fmt.Printf("  Label: %s\n", cfg.Source.Filter.Label)
		}
		if cfg.Source.Filter.Project != "" {
			fmt.Printf("  Project: %s\n", cfg.Source.Filter.Project)
		}
		if cfg.Source.Filter.Team != "" {
			fmt.Printf("  Team: %s\n", cfg.Source.Filter.Team)
		}
		if cfg.Workflow.Merge.Method != "" {
			fmt.Printf("  Merge method: %s\n", cfg.Workflow.Merge.Method)
		}
		if cfg.Workflow.CI.OnFailure != "" {
			fmt.Printf("  CI on failure: %s\n", cfg.Workflow.CI.OnFailure)
		}
		return nil
	}

	var sb strings.Builder
	sb.WriteString("Workflow configuration has errors:\n")
	for _, e := range errs {
		sb.WriteString(fmt.Sprintf("  - %s: %s\n", e.Field, e.Message))
	}
	return fmt.Errorf("%s", sb.String())
}

func runWorkflowVisualize(cmd *cobra.Command, args []string) error {
	cfg, err := workflow.LoadAndMerge(workflowRepoPath)
	if err != nil {
		return fmt.Errorf("failed to load workflow config: %w", err)
	}

	mermaid := workflow.GenerateMermaid(cfg)
	fmt.Fprintln(cmd.OutOrStdout(), mermaid)
	return nil
}
