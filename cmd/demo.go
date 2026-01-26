package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zhubert/plural/internal/demo"
	"github.com/zhubert/plural/internal/demo/scenarios"
)

var (
	demoOutput     string
	demoWidth      int
	demoHeight     int
	demoCaptureAll bool
)

var demoCmd = &cobra.Command{
	Use:   "demo",
	Short: "Generate demo recordings of Plural",
	Long: `Generate demo recordings of Plural for documentation and presentations.

Available subcommands:
  list      - List available demo scenarios
  run       - Run a scenario and output to stdout (for testing)
  generate  - Generate a VHS tape file for rendering
  cast      - Generate an asciinema cast file`,
}

var demoListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available demo scenarios",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Available demo scenarios:")
		fmt.Println()
		for _, s := range scenarios.All() {
			fmt.Printf("  %-15s %s\n", s.Name, s.Description)
		}
	},
}

var demoRunCmd = &cobra.Command{
	Use:   "run <scenario>",
	Short: "Run a scenario and output to stdout (for testing)",
	Args:  cobra.ExactArgs(1),
	RunE:  runDemoRun,
}

var demoGenerateCmd = &cobra.Command{
	Use:   "generate <scenario>",
	Short: "Generate a VHS tape file for rendering",
	Args:  cobra.ExactArgs(1),
	RunE:  runDemoGenerate,
}

var demoCastCmd = &cobra.Command{
	Use:   "cast <scenario>",
	Short: "Generate an asciinema cast file",
	Args:  cobra.ExactArgs(1),
	RunE:  runDemoCast,
}

func init() {
	// Add flags to subcommands that need them
	for _, cmd := range []*cobra.Command{demoRunCmd, demoGenerateCmd, demoCastCmd} {
		cmd.Flags().StringVarP(&demoOutput, "output", "o", "", "Output file")
		cmd.Flags().IntVarP(&demoWidth, "width", "w", 120, "Terminal width")
		cmd.Flags().IntVarP(&demoHeight, "height", "H", 40, "Terminal height")
		cmd.Flags().BoolVar(&demoCaptureAll, "capture-all", false, "Capture frame after every step (for debugging)")
	}

	demoCmd.AddCommand(demoListCmd)
	demoCmd.AddCommand(demoRunCmd)
	demoCmd.AddCommand(demoGenerateCmd)
	demoCmd.AddCommand(demoCastCmd)
	rootCmd.AddCommand(demoCmd)
}

func getScenario(name string) (*demo.Scenario, error) {
	scenario := scenarios.Get(name)
	if scenario == nil {
		return nil, fmt.Errorf("unknown scenario %q\nRun 'plural demo list' to see available scenarios", name)
	}

	// Override dimensions if specified
	if demoWidth > 0 {
		scenario.Width = demoWidth
	}
	if demoHeight > 0 {
		scenario.Height = demoHeight
	}

	return scenario, nil
}

func executeScenario(scenario *demo.Scenario) ([]demo.Frame, error) {
	execCfg := demo.DefaultExecutorConfig()
	execCfg.CaptureEveryStep = demoCaptureAll

	executor := demo.NewExecutor(execCfg)
	return executor.Run(scenario)
}

func runDemoRun(cmd *cobra.Command, args []string) error {
	scenario, err := getScenario(args[0])
	if err != nil {
		return err
	}

	frames, err := executeScenario(scenario)
	if err != nil {
		return fmt.Errorf("error running scenario: %w", err)
	}

	// Print frames to stdout for testing
	fmt.Printf("Captured %d frames\n", len(frames))
	for i, f := range frames {
		fmt.Printf("\n=== Frame %d (delay: %v) ===\n", i, f.Delay)
		if f.Annotation != "" {
			fmt.Printf("Annotation: %s\n", f.Annotation)
		}
		fmt.Println(f.Content)
	}

	return nil
}

func runDemoGenerate(cmd *cobra.Command, args []string) error {
	scenarioName := args[0]
	scenario, err := getScenario(scenarioName)
	if err != nil {
		return err
	}

	frames, err := executeScenario(scenario)
	if err != nil {
		return fmt.Errorf("error running scenario: %w", err)
	}

	// Determine output file
	outputFile := demoOutput
	if outputFile == "" {
		outputFile = scenarioName + ".tape"
	}

	f, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("error creating output file: %w", err)
	}
	defer f.Close()

	vhsCfg := demo.DefaultVHSConfig()
	vhsCfg.Output = strings.TrimSuffix(outputFile, ".tape") + ".gif"
	vhsCfg.Width = scenario.Width
	vhsCfg.Height = scenario.Height

	if err := demo.GenerateVHSTape(f, frames, vhsCfg); err != nil {
		return fmt.Errorf("error generating VHS tape: %w", err)
	}

	fmt.Printf("Generated %s (%d frames)\n", outputFile, len(frames))
	fmt.Printf("Render with: vhs %s\n", outputFile)

	return nil
}

func runDemoCast(cmd *cobra.Command, args []string) error {
	scenarioName := args[0]
	scenario, err := getScenario(scenarioName)
	if err != nil {
		return err
	}

	frames, err := executeScenario(scenario)
	if err != nil {
		return fmt.Errorf("error running scenario: %w", err)
	}

	// Determine output file
	outputFile := demoOutput
	if outputFile == "" {
		outputFile = scenarioName + ".cast"
	}

	f, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("error creating output file: %w", err)
	}
	defer f.Close()

	if err := demo.GenerateASCIICast(f, frames, scenario.Width, scenario.Height); err != nil {
		return fmt.Errorf("error generating cast file: %w", err)
	}

	fmt.Printf("Generated %s (%d frames)\n", outputFile, len(frames))
	fmt.Printf("Play with: asciinema play %s\n", outputFile)

	return nil
}
