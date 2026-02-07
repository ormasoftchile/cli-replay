package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cli-replay/cli-replay/internal/runner"
	"github.com/cli-replay/cli-replay/internal/scenario"
	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean [scenario.yaml]",
	Short: "Clean up intercept session",
	Long: `Clean up a replay session: remove intercept wrappers and delete state.

If no scenario file is given, uses the CLI_REPLAY_SCENARIO environment
variable (which was set automatically by 'cli-replay run | Invoke-Expression').

This deletes the state file and removes the intercept directory, so the next
'cli-replay run' starts fresh.

Examples:
  cli-replay clean                   # uses CLI_REPLAY_SCENARIO from env
  cli-replay clean scenario.yaml     # explicit path`,
	Args: cobra.MaximumNArgs(1),
	RunE: runClean,
}

func init() { //nolint:gochecknoinits // Standard cobra pattern
	rootCmd.AddCommand(cleanCmd)
}

func runClean(_ *cobra.Command, args []string) error {
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

	// Validate that the scenario file exists and is valid
	_, err = scenario.LoadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to load scenario: %w", err)
	}

	stateFile := runner.StateFilePath(absPath)

	// Try to read existing state to find intercept directory for cleanup
	if state, readErr := runner.ReadState(stateFile); readErr == nil && state.InterceptDir != "" {
		if err := os.RemoveAll(state.InterceptDir); err != nil {
			fmt.Fprintf(os.Stderr, "cli-replay: warning: failed to remove intercept dir %s: %v\n",
				state.InterceptDir, err)
		} else {
			fmt.Fprintf(os.Stderr, "cli-replay: removed intercept dir %s\n", state.InterceptDir)
		}
	}

	// Delete state file
	if err := runner.DeleteState(stateFile); err != nil {
		return fmt.Errorf("failed to reset state: %w", err)
	}

	fmt.Fprintf(os.Stderr, "cli-replay: state reset for %s\n", scenarioPath)

	return nil
}
