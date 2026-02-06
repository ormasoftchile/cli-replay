package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cli-replay/cli-replay/internal/recorder"
	"github.com/spf13/cobra"
)

var (
	outputPath  string
	scenarioName string
	description string
	commands    []string
)

var recordCmd = &cobra.Command{
	Use:   "record [flags] -- <command> [args...]",
	Short: "Record a command execution and generate a YAML scenario file",
	Long: `Record captures the execution of a CLI command including its exact output
and exit code, then generates a YAML scenario file that can be replayed later.

Examples:
  # Record a simple command
  cli-replay record --output demo.yaml -- kubectl get pods

  # Record with custom metadata
  cli-replay record --name "my-test" --description "Test scenario" --output test.yaml -- echo "hello"

  # Record a multi-command script
  cli-replay record --output workflow.yaml -- bash -c "kubectl get pods && kubectl get services"

The generated YAML file can be used with 'cli-replay replay' for deterministic testing.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRecord,
}

func init() {
	rootCmd.AddCommand(recordCmd)

	recordCmd.Flags().StringVarP(&outputPath, "output", "o", "", "output YAML file path (required)")
	recordCmd.Flags().StringVarP(&scenarioName, "name", "n", "", "scenario name (default: auto-generated)")
	recordCmd.Flags().StringVarP(&description, "description", "d", "", "scenario description")
	recordCmd.Flags().StringSliceVarP(&commands, "command", "c", []string{}, "commands to intercept (e.g., kubectl,docker)")

	recordCmd.MarkFlagRequired("output")
}

func runRecord(cmd *cobra.Command, args []string) error {
	// Validate output path
	if outputPath == "" {
		return fmt.Errorf("--output flag is required")
	}

	// Validate output directory exists
	outputDir := filepath.Dir(outputPath)
	if outputDir != "." && outputDir != "" {
		if _, err := os.Stat(outputDir); os.IsNotExist(err) {
			return fmt.Errorf("output directory does not exist: %s", outputDir)
		}
	}

	// Create session metadata
	meta := recorder.SessionMetadata{
		Name:        scenarioName,
		Description: description,
		RecordedAt:  time.Now().UTC(),
	}

	// Create recording session
	session, err := recorder.New(meta, commands)
	if err != nil {
		return fmt.Errorf("failed to create recording session: %w", err)
	}
	defer session.Cleanup()

	// For MVP: Direct execution recording (no shims for single command)
	// Shim-based interception will be added in future iterations for multi-step workflows
	exitCode, stdout, stderr, err := executeAndCapture(args)
	if err != nil {
		return fmt.Errorf("failed to execute command: %w", err)
	}

	// Record the command execution
	recordedCmd := recorder.RecordedCommand{
		Timestamp: time.Now().UTC(),
		Argv:      args,
		ExitCode:  exitCode,
		Stdout:    stdout,
		Stderr:    stderr,
	}

	// Populate session commands
	session.Commands = []recorder.RecordedCommand{recordedCmd}

	// Convert recorded commands to scenario
	scenario, err := recorder.ConvertToScenario(session.Metadata, session.Commands)
	if err != nil {
		return fmt.Errorf("failed to convert to scenario: %w", err)
	}

	// Write YAML file
	if err := recorder.WriteYAMLFile(outputPath, scenario); err != nil {
		return fmt.Errorf("failed to write YAML file: %w", err)
	}

	// Print success message
	fmt.Fprintf(os.Stderr, "✓ Recorded %d command(s) to %s\n", len(session.Commands), outputPath)
	fmt.Fprintf(os.Stderr, "  Scenario: %s\n", scenario.Meta.Name)
	if scenario.Meta.Description != "" {
		fmt.Fprintf(os.Stderr, "  Description: %s\n", scenario.Meta.Description)
	}
	fmt.Fprintf(os.Stderr, "  Exit code: %d\n", exitCode)

	// Exit with the same code as the recorded command
	if exitCode != 0 {
		os.Exit(exitCode)
	}

	return nil
}

// executeWithShims runs the command with shims prepended to PATH.
func executeWithShims(session *recorder.RecordingSession, args []string) (int, error) {
	if len(args) == 0 {
		return 0, fmt.Errorf("no command specified")
	}

	// Build the command to execute through a shell to ensure PATH is used
	// This is necessary for shim interception to work
	var command *exec.Cmd
	
	// Join args into a shell command string
	cmdString := strings.Join(args, " ")
	
	// Execute through bash to ensure PATH lookup
	command = exec.Command("bash", "-c", cmdString)

	// Modify PATH to include shim directory first
	originalPath := os.Getenv("PATH")
	modifiedPath := session.ShimDir + string(os.PathListSeparator) + originalPath
	command.Env = append(os.Environ(), "PATH="+modifiedPath)

	// Set up stdout/stderr to pass through
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin

	// Run the command
	err := command.Run()

	// Get exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Command not found or execution failed
			return 127, fmt.Errorf("command execution failed: %w", err)
		}
	}

	return exitCode, nil
}

// executeAndCapture runs a command directly and captures its output.
func executeAndCapture(args []string) (exitCode int, stdout string, stderr string, err error) {
	if len(args) == 0 {
		return 0, "", "", fmt.Errorf("no command specified")
	}

	// Build command
	command := exec.Command(args[0], args[1:]...)

	// Capture stdout and stderr
	var outBuf, errBuf strings.Builder
	command.Stdout = &outBuf
	command.Stderr = &errBuf
	command.Stdin = os.Stdin

	// Also write to actual stdout/stderr for user visibility
	command.Stdout = io.MultiWriter(os.Stdout, &outBuf)
	command.Stderr = io.MultiWriter(os.Stderr, &errBuf)

	// Run the command
	runErr := command.Run()

	// Get exit code
	exitCode = 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Command not found or execution failed
			return 127, "", "", fmt.Errorf("command execution failed: %w", runErr)
		}
	}

	return exitCode, outBuf.String(), errBuf.String(), nil
}

// validateOutputPath checks if the output path is valid and writable.
func validateOutputPath(path string) error {
	// Check if parent directory exists
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		info, err := os.Stat(dir)
		if os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", dir)
		}
		if err != nil {
			return fmt.Errorf("cannot access directory: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("not a directory: %s", dir)
		}
	}

	// Check if file already exists (warn but allow overwrite)
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(os.Stderr, "Warning: %s already exists and will be overwritten\n", path)
	}

	// Try to create/write a test file to verify permissions
	testFile, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("cannot write to %s: %w", path, err)
	}
	testFile.Close()

	return nil
}

// extractCommandName returns a human-readable command name from argv.
func extractCommandName(argv []string) string {
	if len(argv) == 0 {
		return "unknown"
	}

	// Get base command name
	cmd := filepath.Base(argv[0])

	// If there are subcommands, include them
	if len(argv) > 1 && !strings.HasPrefix(argv[1], "-") {
		return fmt.Sprintf("%s %s", cmd, argv[1])
	}

	return cmd
}
