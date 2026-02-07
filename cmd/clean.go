package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cli-replay/cli-replay/internal/runner"
	"github.com/cli-replay/cli-replay/internal/scenario"
	"github.com/spf13/cobra"
)

var cleanTTLFlag string
var cleanRecursiveFlag bool

var cleanCmd = &cobra.Command{
	Use:   "clean [scenario.yaml]",
	Short: "Clean up intercept session",
	Long: `Clean up a replay session: remove intercept wrappers and delete state.

If no scenario file is given, uses the CLI_REPLAY_SCENARIO environment
variable (which was set automatically by 'cli-replay run | Invoke-Expression').

This deletes the state file and removes the intercept directory, so the next
'cli-replay run' starts fresh.

Use --ttl to clean only sessions older than a given duration.
Use --recursive with --ttl to walk a directory tree and clean all expired
sessions under all .cli-replay/ directories found.

Examples:
  cli-replay clean                            # uses CLI_REPLAY_SCENARIO from env
  cli-replay clean scenario.yaml              # explicit path
  cli-replay clean scenario.yaml --ttl 10m    # only expired sessions
  cli-replay clean --ttl 10m --recursive .    # bulk cleanup under current dir
  cli-replay clean --ttl 1h --recursive /path # bulk cleanup under given path`,
	Args: cobra.MaximumNArgs(1),
	RunE: runClean,
}

func init() { //nolint:gochecknoinits // Standard cobra pattern
	cleanCmd.Flags().StringVar(&cleanTTLFlag, "ttl", "", "Only clean sessions older than this duration (e.g., 10m, 1h)")
	cleanCmd.Flags().BoolVar(&cleanRecursiveFlag, "recursive", false, "Walk directory tree for .cli-replay/ dirs (requires --ttl)")
	rootCmd.AddCommand(cleanCmd)
}

func runClean(_ *cobra.Command, args []string) error {
	// T025: --recursive requires --ttl
	if cleanRecursiveFlag && cleanTTLFlag == "" {
		return fmt.Errorf("--recursive requires --ttl to prevent accidental deletion of all sessions")
	}

	// T024: TTL mode
	if cleanTTLFlag != "" {
		ttl, err := time.ParseDuration(cleanTTLFlag)
		if err != nil {
			return fmt.Errorf("invalid --ttl value %q: %w", cleanTTLFlag, err)
		}
		if ttl <= 0 {
			return fmt.Errorf("--ttl must be positive, got %s", cleanTTLFlag)
		}

		if cleanRecursiveFlag {
			// T026: Recursive directory walk
			return runCleanRecursive(args, ttl)
		}

		// TTL mode for single scenario
		return runCleanTTL(args, ttl)
	}

	// Original behavior: clean specific scenario session
	return runCleanSession(args)
}

// runCleanSession is the original clean behavior: remove state + intercept for one session.
func runCleanSession(args []string) error {
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

// runCleanTTL cleans expired sessions for a single scenario.
func runCleanTTL(args []string, ttl time.Duration) error {
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

	cliReplayDir := filepath.Join(filepath.Dir(absPath), ".cli-replay")
	cleaned, err := runner.CleanExpiredSessions(cliReplayDir, ttl, os.Stderr)
	if err != nil {
		return fmt.Errorf("failed to clean expired sessions: %w", err)
	}

	if cleaned > 0 {
		fmt.Fprintf(os.Stderr, "cli-replay: cleaned %d expired sessions for %s\n", cleaned, scenarioPath)
	} else {
		fmt.Fprintf(os.Stderr, "cli-replay: no expired sessions found\n")
	}
	return nil
}

// runCleanRecursive walks a directory tree for .cli-replay/ dirs and cleans expired sessions.
func runCleanRecursive(args []string, ttl time.Duration) error {
	rootPath := "."
	if len(args) > 0 {
		rootPath = args[0]
	}

	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// T026: skip common non-scenario directories
	skipDirs := map[string]bool{
		".git":         true,
		"node_modules": true,
		"vendor":       true,
		".terraform":   true,
		"__pycache__":  true,
	}

	dirsScanned := 0
	totalCleaned := 0

	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Permission error on directory — skip it
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !d.IsDir() {
			return nil
		}

		name := d.Name()

		// Skip common non-scenario directories
		if skipDirs[name] {
			return filepath.SkipDir
		}

		// Process .cli-replay directories
		if name == ".cli-replay" {
			dirsScanned++
			cleaned, cleanErr := runner.CleanExpiredSessions(path, ttl, os.Stderr)
			if cleanErr != nil {
				fmt.Fprintf(os.Stderr, "cli-replay: warning: failed to clean %s: %v\n", path, cleanErr)
			} else {
				totalCleaned += cleaned
			}
			return filepath.SkipDir // don't recurse into .cli-replay/
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	// T027: Output
	if totalCleaned > 0 {
		fmt.Fprintf(os.Stderr, "cli-replay: scanned %d directories, cleaned %d expired sessions\n",
			dirsScanned, totalCleaned)
	} else {
		fmt.Fprintf(os.Stderr, "cli-replay: no expired sessions found\n")
	}

	return nil
}
