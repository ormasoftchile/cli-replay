package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cli-replay/cli-replay/internal/recorder"
	"github.com/spf13/cobra"
)

var (
	recordOutputPath  string
	recordName        string
	recordDescription string
	recordCommands    []string
)

var recordCmd = &cobra.Command{
	Use:   "record [flags] -- <command> [args...]",
	Short: "Record a command execution and generate a YAML scenario file",
	Long: `Record captures the execution of a CLI command including its exact output
and exit code, then generates a YAML scenario file that can be replayed later.

The record subcommand intercepts CLI command executions, captures their
arguments, exit codes, and output, then generates a scenario YAML file
that can be replayed using 'cli-replay run'.

Examples:
  # Record a simple command
  cli-replay record --output demo.yaml -- echo "hello world"

  # Record with custom metadata
  cli-replay record --name "my-test" --description "Test scenario" --output test.yaml -- echo "hello"

  # Record only specific commands from a shell script
  cli-replay record --output workflow.yaml --command kubectl --command docker -- bash deploy.sh

  # Record a multi-command script
  cli-replay record --output workflow.yaml -- bash -c "echo step1 && echo step2"

The generated YAML file can be used with 'cli-replay run' for deterministic testing.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRecord,
}

func init() { //nolint:gochecknoinits // Standard cobra pattern
	rootCmd.AddCommand(recordCmd)

	recordCmd.Flags().StringVarP(&recordOutputPath, "output", "o", "", "output YAML file path (required)")
	recordCmd.Flags().StringVarP(&recordName, "name", "n", "", "scenario name (default: auto-generated)")
	recordCmd.Flags().StringVarP(&recordDescription, "description", "d", "", "scenario description")
	recordCmd.Flags().StringSliceVarP(&recordCommands, "command", "c", []string{}, "commands to intercept (can be repeated)")

	_ = recordCmd.MarkFlagRequired("output")
}

// runRecord is the main handler for the record subcommand.
// Exit codes per CLI contract:
//
//	0 = success
//	1 = setup failure
//	2 = user command failed (still generates YAML)
//	3 = YAML generation/validation failed
func runRecord(_ *cobra.Command, args []string) error {
	// Validate output path
	if err := validateRecordOutputPath(recordOutputPath); err != nil {
		return fmt.Errorf("output path not writable: %w", err)
	}

	// Create session metadata
	meta := recorder.SessionMetadata{
		Name:        recordName,
		Description: recordDescription,
		RecordedAt:  time.Now().UTC(),
	}

	// Create recording session
	session, err := recorder.New(meta, recordCommands)
	if err != nil {
		return fmt.Errorf("failed to create recording session: %w", err)
	}
	defer session.Cleanup() //nolint:errcheck // best-effort cleanup

	// Setup shims if command filters are specified
	if err := session.SetupShims(); err != nil {
		return fmt.Errorf("failed to setup shims: %w", err)
	}

	// Execute the user command and record it
	exitCode, err := session.Execute(args, os.Stdout, os.Stderr)
	if err != nil {
		return fmt.Errorf("failed to execute user command: %w", err)
	}

	// For shim mode, finalize the session by parsing the JSONL log
	if len(recordCommands) > 0 {
		if err := session.Finalize(); err != nil {
			return fmt.Errorf("failed to finalize recording: %w", err)
		}
	}

	// Convert recorded commands to scenario
	sc, err := recorder.ConvertToScenario(session.Metadata, session.Commands)
	if err != nil {
		return fmt.Errorf("failed to convert to scenario: %w", err)
	}

	// Write YAML file
	if err := recorder.WriteYAMLFile(recordOutputPath, sc); err != nil {
		return fmt.Errorf("failed to write YAML file: %w", err)
	}

	// Print success message to stderr (stdout is reserved for command output)
	fmt.Fprintf(os.Stderr, "✓ Recorded %d command(s) to %s\n", len(session.Commands), recordOutputPath)
	fmt.Fprintf(os.Stderr, "  Scenario: %s\n", sc.Meta.Name)
	if sc.Meta.Description != "" {
		fmt.Fprintf(os.Stderr, "  Description: %s\n", sc.Meta.Description)
	}

	// If user command had non-zero exit, still succeed (we captured it) but inform
	if exitCode != 0 {
		fmt.Fprintf(os.Stderr, "  Command exit code: %d (captured in scenario)\n", exitCode)
	}

	return nil
}

// validateRecordOutputPath checks if the output path is valid and writable.
func validateRecordOutputPath(path string) error {
	if path == "" {
		return fmt.Errorf("--output flag is required")
	}

	// Check if parent directory exists
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		info, err := os.Stat(dir)
		if os.IsNotExist(err) {
			return fmt.Errorf("output directory does not exist: %s", dir)
		}
		if err != nil {
			return fmt.Errorf("cannot access output directory: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("not a directory: %s", dir)
		}
	}

	// Check file extension
	if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
		fmt.Fprintf(os.Stderr, "Warning: output file does not have .yaml or .yml extension\n")
	}

	return nil
}

// extractCommandName returns a human-readable command name from argv.
func extractCommandName(argv []string) string {
	if len(argv) == 0 {
		return "unknown"
	}

	// Get base command name
	cmdName := filepath.Base(argv[0])

	// If there are subcommands, include them
	if len(argv) > 1 && !strings.HasPrefix(argv[1], "-") {
		return fmt.Sprintf("%s %s", cmdName, argv[1])
	}

	return cmdName
}
