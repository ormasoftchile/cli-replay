// Package cmd implements the cli-replay Cobra command tree.
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version, Commit, and Date are set at build time via -ldflags.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "cli-replay",
	Short: "Scenario-driven CLI replay for testing",
	Long: `cli-replay - Scenario-driven CLI replay for testing

Record real CLI command executions and generate YAML scenario files,
then replay them for deterministic, reproducible testing.

Modes:
  Management mode: cli-replay <command> [flags]
  Intercept mode:  Symlink/copy cli-replay as another command (e.g. kubectl)
                   and set CLI_REPLAY_SCENARIO to replay canned responses.

Examples:
  # Record a command
  cli-replay record --output demo.yaml -- kubectl get pods

  # Initialize a replay session
  cli-replay run scenario.yaml

  # Verify all steps were consumed
  cli-replay verify scenario.yaml

  # Clean up intercept session
  cli-replay clean scenario.yaml`,
	Version:       Version,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() { //nolint:gochecknoinits
	rootCmd.SetVersionTemplate(fmt.Sprintf("cli-replay version {{.Version}} (commit: %s, built: %s)\n", Commit, Date))
}
