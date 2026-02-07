// Package main is the entry point for cli-replay.
// When invoked as "cli-replay", it runs in management mode (cobra subcommands).
// When invoked via symlink/copy under a different name (e.g., "kubectl"),
// it runs in intercept mode — replaying canned responses from a scenario file.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cli-replay/cli-replay/cmd"
	"github.com/cli-replay/cli-replay/internal/runner"
)

func main() {
	// Determine invocation name (strip .exe on Windows, strip path)
	invoked := filepath.Base(os.Args[0])
	invoked = strings.TrimSuffix(invoked, ".exe")
	invoked = strings.TrimSuffix(invoked, ".cmd")

	if invoked != "cli-replay" {
		// Intercept mode: we were invoked as a symlink/wrapper (e.g., "kubectl")
		os.Exit(runIntercept())
	}

	// Management mode: run cobra command tree
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// runIntercept handles intercept mode where cli-replay was invoked
// via symlink or wrapper as another command name (e.g., kubectl, az).
// It reads CLI_REPLAY_SCENARIO, loads the scenario, matches os.Args
// against the next expected step, and returns the canned response.
func runIntercept() int {
	scenarioPath := os.Getenv("CLI_REPLAY_SCENARIO")
	if scenarioPath == "" {
		fmt.Fprintf(os.Stderr, "cli-replay: no scenario specified\n")
		fmt.Fprintf(os.Stderr, "  use: cli-replay run <scenario.yaml>\n")
		fmt.Fprintf(os.Stderr, "  or:  export CLI_REPLAY_SCENARIO=/path/to/scenario.yaml\n")
		return 1
	}

	// Build argv: replace os.Args[0] (full path to symlink) with just the base command name
	argv := make([]string, len(os.Args))
	copy(argv, os.Args)
	base := filepath.Base(argv[0])
	base = strings.TrimSuffix(base, ".exe")
	base = strings.TrimSuffix(base, ".cmd")
	argv[0] = base

	result, err := runner.ExecuteReplay(scenarioPath, argv, os.Stdout, os.Stderr)
	if err != nil {
		// Format and display typed errors with rich diagnostics
		switch e := err.(type) {
		case *runner.MismatchError:
			fmt.Fprint(os.Stderr, runner.FormatMismatchError(e))
		case *runner.StdinMismatchError:
			fmt.Fprint(os.Stderr, runner.FormatStdinMismatchError(e))
		default:
			fmt.Fprintf(os.Stderr, "cli-replay: %v\n", err)
		}
		if result != nil {
			return result.ExitCode
		}
		return 1
	}

	return result.ExitCode
}
