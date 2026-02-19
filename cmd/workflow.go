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

var workflowInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate a .plural/workflow.yaml template",
	Long: `Creates a .plural/workflow.yaml file with sensible defaults and commented
optional sections. Run this in your repository root (or use --repo) to get started
with a customizable agent workflow.

Examples:
  plural workflow init                   # Initialize in current directory
  plural workflow init --repo /path/to/repo`,
	RunE: runWorkflowInit,
}

var workflowVisualizeCmd = &cobra.Command{
	Use:   "visualize",
	Short: "Generate mermaid diagram of workflow",
	Long:  `Generates a mermaid stateDiagram-v2 from the workflow configuration and prints it to stdout.`,
	RunE:  runWorkflowVisualize,
}

func init() {
	workflowCmd.PersistentFlags().StringVar(&workflowRepoPath, "repo", ".", "Path to the repository")
	workflowCmd.AddCommand(workflowInitCmd)
	workflowCmd.AddCommand(workflowValidateCmd)
	workflowCmd.AddCommand(workflowVisualizeCmd)
	rootCmd.AddCommand(workflowCmd)
}

func runWorkflowInit(_ *cobra.Command, _ []string) error {
	fp, err := workflow.WriteTemplate(workflowRepoPath)
	if err != nil {
		return err
	}
	fmt.Printf("Created %s\n", fp)
	return nil
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
		fmt.Printf("  Workflow: %s\n", cfg.Workflow)
		fmt.Printf("  Start: %s\n", cfg.Start)
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
		fmt.Printf("  States: %d\n", len(cfg.States))
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
