package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cli-replay/cli-replay/internal/runner"
	"github.com/cli-replay/cli-replay/internal/scenario"
	"github.com/spf13/cobra"
)

var verifyCmd = &cobra.Command{
	Use:   "verify [scenario.yaml]",
	Short: "Verify all scenario steps consumed",
	Long: `Verify that all steps in a scenario have been consumed during replay.

If no scenario file is given, uses the CLI_REPLAY_SCENARIO environment
variable (which was set automatically by 'cli-replay run | Invoke-Expression').

Exit code 0 if all steps are consumed, 1 if steps remain or state is missing.

Examples:
  cli-replay verify                 # uses CLI_REPLAY_SCENARIO from env
  cli-replay verify scenario.yaml   # explicit path`,
	Args: cobra.MaximumNArgs(1),
	RunE: runVerify,
}

func init() { //nolint:gochecknoinits // Standard cobra pattern
	rootCmd.AddCommand(verifyCmd)
}

func runVerify(_ *cobra.Command, args []string) error {
	var scenarioPath string
	if len(args) > 0 {
		scenarioPath = args[0]
	} else {
		scenarioPath = os.Getenv("CLI_REPLAY_SCENARIO")
		if scenarioPath == "" {
			return fmt.Errorf("no scenario specified — pass a file or set CLI_REPLAY_SCENARIO")
		}
	}

	absPath, err := filepath.Abs(scenarioPath)
	if err != nil {
		return fmt.Errorf("failed to resolve scenario path: %w", err)
	}

	// Load scenario for metadata and step count
	scn, err := scenario.LoadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to load scenario: %w", err)
	}

	// Load state
	stateFile := runner.StateFilePath(absPath)
	state, err := runner.ReadState(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "cli-replay: no state found for %q\n", scn.Meta.Name)
			fmt.Fprintf(os.Stderr, "  run 'cli-replay run %s' first\n", scenarioPath)
			os.Exit(1)
		}
		return fmt.Errorf("failed to read state: %w", err)
	}

	// Check completion: AllStepsMetMin covers both simple (min=1 default) and bounded cases.
	hasCallBounds := hasAnyCallBounds(scn.Steps)
	allMetMin := state.AllStepsMetMin(scn.Steps)

	if allMetMin {
		consumed := countConsumedSteps(state)
		fmt.Fprintf(os.Stderr, "✓ Scenario %q completed: %d/%d steps consumed\n",
			scn.Meta.Name, consumed, state.TotalSteps)
		if hasCallBounds {
			printPerStepCounts(scn.Steps, state)
		}
		return nil
	}

	// Incomplete — show per-step detail
	consumed := countConsumedSteps(state)
	fmt.Fprintf(os.Stderr, "✗ Scenario %q incomplete\n", scn.Meta.Name)
	fmt.Fprintf(os.Stderr, "  consumed: %d/%d steps\n", consumed, state.TotalSteps)
	printPerStepCounts(scn.Steps, state)
	os.Exit(1)

	return nil // unreachable but satisfies compiler
}

// hasAnyCallBounds returns true if any step has explicit call bounds.
func hasAnyCallBounds(steps []scenario.Step) bool {
	for _, step := range steps {
		if step.Calls != nil {
			return true
		}
	}
	return false
}

// countConsumedSteps counts how many steps have been invoked at least once.
func countConsumedSteps(state *runner.State) int {
	count := 0
	if state.StepCounts != nil {
		for _, c := range state.StepCounts {
			if c >= 1 {
				count++
			}
		}
	}
	return count
}

// printPerStepCounts prints per-step invocation counts with call bounds info.
func printPerStepCounts(steps []scenario.Step, state *runner.State) {
	for i, step := range steps {
		bounds := step.EffectiveCalls()
		callCount := 0
		if state.StepCounts != nil && i < len(state.StepCounts) {
			callCount = state.StepCounts[i]
		}

		// Build step label from first argv elements
		label := ""
		if len(step.Match.Argv) > 0 {
			label = step.Match.Argv[0]
			if len(step.Match.Argv) > 1 {
				label += " " + step.Match.Argv[1]
			}
		}

		callWord := "calls"
		if callCount == 1 {
			callWord = "call"
		}

		status := "✓"
		suffix := ""
		if callCount < bounds.Min {
			status = "✗"
			needed := bounds.Min - callCount
			suffix = fmt.Sprintf(" needs %d more", needed)
		}

		if step.Calls != nil {
			fmt.Fprintf(os.Stderr, "  Step %d: %s — %d %s (min: %d, max: %d) %s%s\n",
				i+1, label, callCount, callWord, bounds.Min, bounds.Max, status, suffix)
		} else {
			fmt.Fprintf(os.Stderr, "  Step %d: %s — %d %s %s%s\n",
				i+1, label, callCount, callWord, status, suffix)
		}
	}
}
