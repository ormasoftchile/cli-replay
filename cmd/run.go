package cmd

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cli-replay/cli-replay/internal/runner"
	"github.com/cli-replay/cli-replay/internal/scenario"
	"github.com/spf13/cobra"
)

var runShellFlag string

var runCmd = &cobra.Command{
	Use:   "run <scenario.yaml>",
	Short: "Start or resume a replay session",
	Long: `Start or resume a replay session for a scenario.

Loads the scenario file, validates it, creates intercept wrappers for every
command referenced in the scenario, and outputs shell commands that configure
PATH and CLI_REPLAY_SCENARIO in the calling shell.

Usage (PowerShell):
  cli-replay run scenario.yaml | Invoke-Expression

Usage (bash / zsh):
  eval "$(cli-replay run scenario.yaml)"

The --shell flag selects the output format. If omitted, the shell is auto-
detected from the PSModulePath (PowerShell) or SHELL environment variable.`,
	Args: cobra.ExactArgs(1),
	RunE: runRun,
}

func init() { //nolint:gochecknoinits // Standard cobra pattern
	runCmd.Flags().StringVar(&runShellFlag, "shell", "", "Output format: powershell, bash, cmd (auto-detected if omitted)")
	rootCmd.AddCommand(runCmd)
}

func runRun(_ *cobra.Command, args []string) error {
	scenarioPath := args[0]

	absPath, err := filepath.Abs(scenarioPath)
	if err != nil {
		return fmt.Errorf("failed to resolve scenario path: %w", err)
	}

	// Load and validate scenario
	scn, err := scenario.LoadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to load scenario: %w", err)
	}

	// Extract unique command names from scenario steps (argv[0])
	commands := extractCommands(scn)
	if len(commands) == 0 {
		return fmt.Errorf("scenario has no steps with a command name")
	}

	// Calculate scenario hash for state tracking
	data, readErr := os.ReadFile(absPath) //nolint:gosec // user-provided path is expected
	scenarioHash := ""
	if readErr == nil {
		h := sha256.Sum256(data)
		scenarioHash = hex.EncodeToString(h[:])
	}

	// Locate our own binary to create intercepts
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to locate cli-replay binary: %w", err)
	}

	// Create intercept directory
	interceptDir, err := os.MkdirTemp("", "cli-replay-intercept-")
	if err != nil {
		return fmt.Errorf("failed to create intercept directory: %w", err)
	}

	// Create intercept entries for each command
	for _, cmd := range commands {
		if err := createIntercept(self, interceptDir, cmd); err != nil {
			// Clean up on failure
			_ = os.RemoveAll(interceptDir)
			return fmt.Errorf("failed to create intercept for %q: %w", cmd, err)
		}
	}

	// Generate session ID for parallel isolation
	sessionID := generateSessionID()

	// Initialize state (resets to step 0) and store intercept dir
	// Use session-aware state path so parallel runs don't collide
	stateFile := runner.StateFilePathWithSession(absPath, sessionID)
	state := runner.NewState(absPath, scenarioHash, len(scn.Steps))
	state.InterceptDir = interceptDir
	if err := runner.WriteState(stateFile, state); err != nil {
		_ = os.RemoveAll(interceptDir)
		return fmt.Errorf("failed to initialize state: %w", err)
	}

	// Status to stderr (not piped to Invoke-Expression / eval)
	fmt.Fprintf(os.Stderr, "cli-replay: session initialized for %q (%d steps, %d commands)\n",
		scn.Meta.Name, len(scn.Steps), len(commands))
	fmt.Fprintf(os.Stderr, "  intercept dir: %s\n", interceptDir)
	fmt.Fprintf(os.Stderr, "  commands: %s\n", strings.Join(commands, ", "))

	// Detect shell and emit env-setting code to stdout
	shell := detectShell(runShellFlag)
	emitShellSetup(shell, interceptDir, absPath, sessionID)

	return nil
}

// extractCommands returns a de-duplicated, ordered list of command names
// from step[*].match.argv[0] in the scenario.
func extractCommands(scn *scenario.Scenario) []string {
	seen := make(map[string]bool)
	var cmds []string
	for _, step := range scn.Steps {
		if len(step.Match.Argv) == 0 {
			continue
		}
		name := step.Match.Argv[0]
		if !seen[name] {
			seen[name] = true
			cmds = append(cmds, name)
		}
	}
	return cmds
}

// createIntercept copies or symlinks the cli-replay binary under the target
// command name. On Windows, creates a .exe copy. On Unix, creates a symlink.
func createIntercept(self, interceptDir, command string) error {
	if runtime.GOOS == "windows" {
		dst := filepath.Join(interceptDir, command+".exe")
		src, err := os.ReadFile(self) //nolint:gosec // reading own binary
		if err != nil {
			return fmt.Errorf("failed to read binary: %w", err)
		}
		return os.WriteFile(dst, src, 0755) //nolint:gosec // intercept must be executable
	}
	// Unix: symlink
	dst := filepath.Join(interceptDir, command)
	return os.Symlink(self, dst)
}

// detectShell determines which shell output format to use.
// Priority: explicit --shell flag > PSModulePath (PowerShell) > SHELL env > platform default.
func detectShell(explicit string) string {
	if explicit != "" {
		switch strings.ToLower(explicit) {
		case "powershell", "pwsh", "ps":
			return "powershell"
		case "bash", "zsh", "sh":
			return "bash"
		case "cmd":
			return "cmd"
		default:
			return "bash"
		}
	}

	// Auto-detect: if PSModulePath is set, caller is likely PowerShell
	if os.Getenv("PSModulePath") != "" {
		return "powershell"
	}

	// Check SHELL env (Unix)
	if shell := os.Getenv("SHELL"); shell != "" {
		return "bash"
	}

	// Platform default
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return "bash"
}

// emitShellSetup writes shell-specific commands to stdout that set
// CLI_REPLAY_SESSION, CLI_REPLAY_SCENARIO, and prepend the intercept directory to PATH.
func emitShellSetup(shell, interceptDir, scenarioPath, sessionID string) {
	switch shell {
	case "powershell":
		fmt.Printf("$env:CLI_REPLAY_SESSION = '%s'\n", sessionID)
		fmt.Printf("$env:CLI_REPLAY_SCENARIO = '%s'\n", strings.ReplaceAll(scenarioPath, "'", "''"))
		fmt.Printf("$env:PATH = '%s' + ';' + $env:PATH\n", strings.ReplaceAll(interceptDir, "'", "''"))
	case "cmd":
		fmt.Printf("set \"CLI_REPLAY_SESSION=%s\"\n", sessionID)
		fmt.Printf("set \"CLI_REPLAY_SCENARIO=%s\"\n", scenarioPath)
		fmt.Printf("set \"PATH=%s;%%PATH%%\"\n", interceptDir)
	default: // bash / zsh / sh
		fmt.Printf("export CLI_REPLAY_SESSION='%s'\n", sessionID)
		fmt.Printf("export CLI_REPLAY_SCENARIO='%s'\n", strings.ReplaceAll(scenarioPath, "'", "'\\''"))
		fmt.Printf("export PATH='%s':\"$PATH\"\n", strings.ReplaceAll(interceptDir, "'", "'\\''"))
	}
}

// generateSessionID returns a random hex string for session isolation.
func generateSessionID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback: use PID + timestamp
		return fmt.Sprintf("%d-%d", os.Getpid(), os.Getpid())
	}
	return hex.EncodeToString(b)
}

