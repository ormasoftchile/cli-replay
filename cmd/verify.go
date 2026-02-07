package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	if state.IsComplete() {
		fmt.Fprintf(os.Stderr, "cli-replay: scenario %q complete (%d/%d steps)\n",
			scn.Meta.Name, state.TotalSteps, state.TotalSteps)
		return nil
	}

	// Incomplete — show remaining steps
	fmt.Fprintf(os.Stderr, "cli-replay: scenario %q incomplete\n", scn.Meta.Name)
	fmt.Fprintf(os.Stderr, "  consumed: %d/%d steps\n", state.CurrentStep, state.TotalSteps)
	fmt.Fprintf(os.Stderr, "  remaining:\n")
	for i := state.CurrentStep; i < len(scn.Steps); i++ {
		argv := scn.Steps[i].Match.Argv
		fmt.Fprintf(os.Stderr, "    step %d: [%s]\n", i+1, formatArgv(argv))
	}
	os.Exit(1)

	return nil // unreachable but satisfies compiler
}

// formatArgv formats an argv slice as a quoted, comma-separated string.
func formatArgv(argv []string) string {
	quoted := make([]string, len(argv))
	for i, a := range argv {
		quoted[i] = fmt.Sprintf("%q", a)
	}
	return strings.Join(quoted, ", ")
}
