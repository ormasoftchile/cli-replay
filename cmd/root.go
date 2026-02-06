// Package cmd implements the cli-replay Cobra command tree.
package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "cli-replay",
	Short: "Record and replay CLI command scenarios for testing",
	Long: `cli-replay is a tool for recording real CLI command executions and 
generating YAML scenario files that can be replayed for deterministic testing.

Features:
- Record command executions with their exact outputs
- Generate portable YAML scenario files
- Replay scenarios with matching command detection
- Perfect for testing CLI tools and scripts`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() { //nolint:gochecknoinits
	// Global flags can be added here
	// rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
}
