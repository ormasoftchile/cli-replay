package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cli-replay/cli-replay/internal/runner"
	"github.com/cli-replay/cli-replay/internal/scenario"
	"github.com/cli-replay/cli-replay/internal/verify"
	"github.com/spf13/cobra"
)

var execAllowedCommandsFlag string
var execFormatFlag string
var execReportFileFlag string
var execDryRunFlag bool

var execCmd = &cobra.Command{
	Use:   "exec [flags] <scenario.yaml> -- <command> [args...]",
	Short: "Run a command under replay interception",
	Long: `Run a command under cli-replay interception with automatic lifecycle management.

Sets up the intercept directory, spawns the command as a child process with
modified PATH, waits for completion, verifies scenario completion, and cleans
up — all in a single invocation.

This is the recommended approach for CI/CD pipelines where the three-step
eval/execute/verify pattern is cumbersome.

Exit codes:
  0     Child exited 0 AND all scenario steps satisfied
  1     Scenario verification failed (steps not consumed)
  N     Child's non-zero exit code (takes precedence)
  126   Child command found but not executable
  127   Child command not found
  128+N Child killed by signal N (e.g., 130 = SIGINT)

Examples:
  cli-replay exec scenario.yaml -- ./test-script.sh
  cli-replay exec --allowed-commands=kubectl scenario.yaml -- make test
  cli-replay exec scenario.yaml -- bash -c 'kubectl get pods'`,
	RunE:              runExec,
	SilenceUsage:      true,
	DisableFlagParsing: false,
}

func init() { //nolint:gochecknoinits // Standard cobra pattern
	execCmd.Flags().StringVar(&execAllowedCommandsFlag, "allowed-commands", "", "Comma-separated list of commands allowed to be intercepted")
	execCmd.Flags().StringVar(&execFormatFlag, "format", "", "Output format for verification report: json or junit")
	execCmd.Flags().StringVar(&execReportFileFlag, "report-file", "", "Write verification report to file instead of stderr")
	execCmd.Flags().BoolVar(&execDryRunFlag, "dry-run", false, "Preview the scenario without spawning a child process")
	rootCmd.AddCommand(execCmd)
}

// ExecExitCode is set by runExec to communicate the desired exit code
// to the caller. Because cobra uses RunE (returns error), we use this
// sentinel to pass exit codes through without printing a usage message.
var ExecExitCode int

// runExec implements the exec command lifecycle:
// Phase 1: pre-spawn validation
// Phase 2: setup (intercept dir, state file)
// Phase 3: spawn + wait (with signal forwarding)
// Phase 4: verify + cleanup
func runExec(cmd *cobra.Command, args []string) error {
	ExecExitCode = 0

	// --- Validate format flag ---
	execFormat := strings.ToLower(execFormatFlag)
	if execFormat != "" {
		switch execFormat {
		case "json", "junit":
			// valid
		default:
			return fmt.Errorf("invalid format %q: valid values are json, junit", execFormatFlag)
		}
	}

	// --- Phase 1: Pre-spawn validation ---

	// Parse args: everything before -- is exec args, everything after is the child command
	dashIdx := cmd.ArgsLenAtDash()
	if dashIdx < 0 {
		return fmt.Errorf("missing '--' separator: usage: cli-replay exec <scenario.yaml> -- <command> [args...]")
	}
	if dashIdx == 0 {
		return fmt.Errorf("missing scenario path before '--'")
	}
	if dashIdx > 1 {
		return fmt.Errorf("expected exactly one scenario path before '--', got %d args", dashIdx)
	}

	scenarioPath := args[0]
	childArgv := args[dashIdx:]
	if len(childArgv) == 0 {
		return fmt.Errorf("missing command after '--': usage: cli-replay exec <scenario.yaml> -- <command> [args...]")
	}

	absPath, err := filepath.Abs(scenarioPath)
	if err != nil {
		return fmt.Errorf("failed to resolve scenario path: %w", err)
	}

	// Load and validate scenario
	scn, err := scenario.LoadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to load scenario: %w", err)
	}

	// Validate delays (no max-delay flag in exec, so no cap)
	// If we add --max-delay later, pass it here
	for i, step := range scn.FlatSteps() {
		if err := step.Respond.ValidateDelay(0); err != nil {
			return fmt.Errorf("step %d: %w", i+1, err)
		}
	}

	// Extract commands and validate allowlist
	commands := extractCommands(scn)
	if len(commands) == 0 {
		return fmt.Errorf("scenario has no steps with a command name")
	}

	// Use exec-specific allowlist flag
	cliList := parseAllowedCommands(execAllowedCommandsFlag)
	var yamlList []string
	if scn.Meta.Security != nil {
		yamlList = scn.Meta.Security.AllowedCommands
	}
	if err := validateAllowlist(scn, yamlList, cliList); err != nil {
		return err
	}

	// Dry-run mode: preview scenario and exit without side effects
	if execDryRunFlag {
		report := runner.BuildDryRunReport(scn)
		return runner.FormatDryRunReport(report, cmd.OutOrStdout())
	}

	// T019: TTL cleanup at session startup
	if scn.Meta.Session != nil && scn.Meta.Session.TTL != "" {
		ttl, parseErr := time.ParseDuration(scn.Meta.Session.TTL)
		if parseErr == nil && ttl > 0 {
			cliReplayDir := filepath.Join(filepath.Dir(absPath), ".cli-replay")
			cleaned, _ := runner.CleanExpiredSessions(cliReplayDir, ttl, os.Stderr)
			if cleaned > 0 {
				fmt.Fprintf(os.Stderr, "cli-replay: cleaned %d expired sessions\n", cleaned)
			}
		}
	}

	// --- Phase 2: Setup ---

	scenarioHash := hashScenarioFile(absPath)

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to locate cli-replay binary: %w", err)
	}

	interceptDir, err := runner.InterceptDirPath(absPath)
	if err != nil {
		return fmt.Errorf("failed to create intercept directory: %w", err)
	}

	// Idempotent cleanup guard
	cleaned := false
	cleanup := func() {
		if cleaned {
			return
		}
		cleaned = true
		_ = os.RemoveAll(interceptDir)
		stateFile := runner.StateFilePathWithSession(absPath, "")
		// Try to compute the real state file path using session ID
		// (set below, but may not be reachable here if setup failed early)
		_ = runner.DeleteState(stateFile)
	}

	for _, c := range commands {
		if err := createIntercept(self, interceptDir, c); err != nil {
			cleanup()
			return fmt.Errorf("failed to create intercept for %q: %w", c, err)
		}
	}

	sessionID := generateSessionID()
	stateFile := runner.StateFilePathWithSession(absPath, sessionID)
	state := runner.NewState(absPath, scenarioHash, len(scn.FlatSteps()))
	state.InterceptDir = interceptDir
	if err := runner.WriteState(stateFile, state); err != nil {
		cleanup()
		return fmt.Errorf("failed to initialize state: %w", err)
	}

	// Fix cleanup to use the correct session-specific state file
	cleaned = false
	cleanup = func() {
		if cleaned {
			return
		}
		cleaned = true
		_ = os.RemoveAll(interceptDir)
		_ = runner.DeleteState(stateFile)
	}
	defer cleanup()

	// Status to stderr
	fmt.Fprintf(os.Stderr, "cli-replay: exec session initialized for %q (%d steps, %d commands)\n",
		scn.Meta.Name, len(scn.FlatSteps()), len(commands))
	fmt.Fprintf(os.Stderr, "  child command: %s\n", strings.Join(childArgv, " "))

	// --- Phase 3: Spawn + Wait ---

	childCmd := exec.Command(childArgv[0], childArgv[1:]...) //nolint:gosec // user-specified command
	childCmd.Env = runner.BuildChildEnv(interceptDir, sessionID, absPath)
	childCmd.Stdin = os.Stdin
	childCmd.Stdout = os.Stdout
	childCmd.Stderr = os.Stderr

	// Set up signal forwarding (platform-specific: see exec_unix.go / exec_windows.go)
	cleanupSignals := setupSignalForwarding(childCmd)

	if err := childCmd.Start(); err != nil {
		cleanupSignals()
		// Determine exit code: command not found = 127, not executable = 126
		ExecExitCode = exitCodeForStartError(err)
		return fmt.Errorf("failed to start child process: %w", err)
	}

	waitErr := childCmd.Wait()
	cleanupSignals()

	childExitCode := runner.ExitCodeFromError(waitErr)

	// --- Phase 4: Verify + Cleanup ---

	// Determine session for structured output
	session := sessionID
	if session == "" {
		session = "default"
	}

	// Reload state to check completion
	updatedState, readErr := runner.ReadState(stateFile)

	verificationPassed := false
	if readErr != nil {
		fmt.Fprintf(os.Stderr, "cli-replay: warning: could not read state for verification: %v\n", readErr)
		// Write error result if format is set
		if execFormat != "" {
			errResult := verify.BuildErrorResult(scn.Meta.Name, session, "could not read state")
			writeExecReport(errResult, execFormat, scenarioPath)
		}
	} else {
		verificationPassed = updatedState.AllStepsMetMin(scn.FlatSteps())

		// Build structured result for report
		if execFormat != "" {
			result := verify.BuildResult(scn.Meta.Name, session, scn.FlatSteps(), updatedState, scn.GroupRanges())
			writeExecReport(result, execFormat, scenarioPath)
		}

		if !verificationPassed {
			consumed := countConsumedSteps(updatedState)
			fmt.Fprintf(os.Stderr, "✗ Scenario %q incomplete\n", scn.Meta.Name)
			fmt.Fprintf(os.Stderr, "  consumed: %d/%d steps\n", consumed, updatedState.TotalSteps)
			printPerStepCounts(scn.FlatSteps(), updatedState)
		} else {
			consumed := countConsumedSteps(updatedState)
			fmt.Fprintf(os.Stderr, "✓ Scenario %q completed: %d/%d steps consumed\n",
				scn.Meta.Name, consumed, updatedState.TotalSteps)
		}
	}

	// Cleanup runs via defer

	// Determine final exit code
	if childExitCode != 0 {
		ExecExitCode = childExitCode
		return fmt.Errorf("child process exited with code %d", childExitCode)
	}
	if !verificationPassed {
		ExecExitCode = 1
		return fmt.Errorf("scenario verification failed")
	}

	ExecExitCode = 0
	return nil
}

// exitCodeForStartError returns the conventional exit code for a process
// start failure: 127 for "not found", 126 for "not executable".
func exitCodeForStartError(err error) int {
	if err == nil {
		return 0
	}
	msg := err.Error()
	if strings.Contains(msg, "not found") || strings.Contains(msg, "no such file") {
		return 127
	}
	if strings.Contains(msg, "permission denied") || strings.Contains(msg, "not executable") {
		return 126
	}
	return 1
}

// writeExecReport writes the structured verification result to the report
// destination. If --report-file is set, writes to that file. Otherwise,
// if --format is set, writes to stderr (stdout is reserved for child process).
func writeExecReport(result *verify.VerifyResult, format, scenarioFile string) {
	var w *os.File
	if execReportFileFlag != "" {
		f, err := os.Create(execReportFileFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cli-replay: warning: could not create report file %q: %v\n", execReportFileFlag, err)
			return
		}
		defer f.Close() //nolint:errcheck
		w = f
	} else {
		w = os.Stderr
	}

	var err error
	switch format {
	case "json":
		err = verify.FormatJSON(w, result)
	case "junit":
		err = verify.FormatJUnit(w, result, scenarioFile, time.Time{})
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "cli-replay: warning: failed to write report: %v\n", err)
	}
}
