//go:build !windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// bashShimTemplate is the bash script that intercepts command executions.
// It uses explicit paths (/bin/cat, /bin/rm) for internal operations to avoid
// recursive interception when the shimmed command is one of these utilities.
const bashShimTemplate = `#!/usr/bin/env bash
# cli-replay shim for: %s
# This script intercepts the command and logs execution details to JSONL

# Prevent recursive shim execution by stripping shim dir from PATH
if [ "$CLI_REPLAY_IN_SHIM" = "1" ]; then
    export PATH="${PATH#%s:}"
    exec %s "$@"
fi
export CLI_REPLAY_IN_SHIM=1

LOGFILE="%s"
SHIM_DIR="%s"

# Find real command by removing shim directory from PATH
REAL_COMMAND=$(PATH="${PATH#${SHIM_DIR}:}" command -v %s 2>/dev/null)

# If real command not found, report error
if [ -z "$REAL_COMMAND" ] || [ "$REAL_COMMAND" = "${BASH_SOURCE[0]}" ]; then
    echo "cli-replay: command not found: %s" >&2
    exit 127
fi

# Capture start time (RFC3339 format)
TIMESTAMP=$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)

# Execute the real command and capture output
STDOUT_FILE=$(mktemp)
STDERR_FILE=$(mktemp)
EXIT_CODE=0

"$REAL_COMMAND" "$@" >"$STDOUT_FILE" 2>"$STDERR_FILE" || EXIT_CODE=$?

# Read captured output into variables using bash builtins to avoid
# depending on external 'cat' which might be shimmed
STDOUT_CONTENT=""
if [ -s "$STDOUT_FILE" ]; then
    STDOUT_CONTENT=$(< "$STDOUT_FILE")
fi
STDERR_CONTENT=""
if [ -s "$STDERR_FILE" ]; then
    STDERR_CONTENT=$(< "$STDERR_FILE")
fi

# Echo output to preserve command behavior
# Use /bin/cat with explicit path to avoid shimming recursion
if [ -s "$STDOUT_FILE" ]; then
    /bin/cat "$STDOUT_FILE"
fi
if [ -s "$STDERR_FILE" ]; then
    /bin/cat "$STDERR_FILE" >&2
fi

# Clean up temp files (explicit path to avoid shimming recursion)
/bin/rm -f "$STDOUT_FILE" "$STDERR_FILE"

# Build argv array for JSON
ARGV_JSON="[\"%s\""
for arg in "$@"; do
    # Escape quotes and backslashes
    ESCAPED=$(printf '%%s' "$arg" | sed 's/\\/\\\\/g; s/"/\\"/g')
    ARGV_JSON="$ARGV_JSON,\"$ESCAPED\""
done
ARGV_JSON="$ARGV_JSON]"

# Escape JSON strings using sed/awk (unlikely to be shimmed)
ESC_STDOUT=$(printf '%%s' "$STDOUT_CONTENT" | sed 's/\\/\\\\/g; s/"/\\"/g' | awk '{printf "%%s\\n", $0}' | sed 's/\\n$//')
ESC_STDERR=$(printf '%%s' "$STDERR_CONTENT" | sed 's/\\/\\\\/g; s/"/\\"/g' | awk '{printf "%%s\\n", $0}' | sed 's/\\n$//')

# Write JSONL entry
printf '{"timestamp":"%%s","argv":%%s,"exit":%%d,"stdout":"%%s","stderr":"%%s"}\n' \
    "$TIMESTAMP" "$ARGV_JSON" "$EXIT_CODE" "$ESC_STDOUT" "$ESC_STDERR" >> "$LOGFILE"

exit $EXIT_CODE
`

// unixPlatform implements Platform for Unix-like systems (Linux, macOS, FreeBSD, etc.).
type unixPlatform struct{}

// newPlatform returns the Unix platform implementation.
// This is the build-tagged factory called by New() on non-Windows systems.
func newPlatform() Platform {
	return &unixPlatform{}
}

// New returns the Platform for the current OS.
func New() Platform {
	return newPlatform()
}

// Name returns "unix".
func (u *unixPlatform) Name() string {
	return "unix"
}

// GenerateShim creates a bash shim script that intercepts command execution.
func (u *unixPlatform) GenerateShim(command, logPath, shimDir string) (*ShimFile, error) {
	if command == "" {
		return nil, fmt.Errorf("command must be non-empty")
	}
	if logPath == "" {
		return nil, fmt.Errorf("logPath must be non-empty")
	}
	if shimDir == "" {
		return nil, fmt.Errorf("shimDir must be non-empty")
	}

	script := fmt.Sprintf(bashShimTemplate,
		command, // Comment line: shim for
		shimDir, // Guard: PATH strip prefix
		command, // Guard: exec fallback
		logPath, // LOGFILE variable
		shimDir, // SHIM_DIR variable
		command, // command -v lookup
		command, // Error message
		command, // argv[0] in JSON
	)

	return &ShimFile{
		EntryPointPath: filepath.Join(shimDir, command),
		Command:        command,
		Content:        script,
		FileMode:       u.ShimFileMode(),
	}, nil
}

// ShimFileName returns the command name without extension (Unix convention).
func (u *unixPlatform) ShimFileName(command string) string {
	return command
}

// ShimFileMode returns 0755 (executable on Unix).
func (u *unixPlatform) ShimFileMode() os.FileMode {
	return 0755
}

// WrapCommand returns an exec.Cmd wrapping args in bash -c.
func (u *unixPlatform) WrapCommand(args []string, env []string) *exec.Cmd {
	cmdStr := strings.Join(args, " ")
	cmd := exec.Command("bash", "-c", cmdStr) //nolint:gosec // user command is intentionally executed
	if len(env) > 0 {
		cmd.Env = env
	}
	return cmd
}

// Resolve locates the real binary for command, excluding excludeDir.
// Uses exec.LookPath with PATH filtering, falling back to common Unix paths.
func (u *unixPlatform) Resolve(command string, excludeDir string) (string, error) {
	if command == "" {
		return "", fmt.Errorf("command must be non-empty")
	}

	// Filter excludeDir out of PATH
	originalPath := os.Getenv("PATH")
	filteredPath := filterPath(originalPath, excludeDir)

	// Temporarily set filtered PATH for LookPath
	os.Setenv("PATH", filteredPath)           //nolint:errcheck // temp set for LookPath
	defer os.Setenv("PATH", originalPath)     //nolint:errcheck // restore original PATH
	resolved, err := exec.LookPath(command)
	if err == nil {
		return resolved, nil
	}

	// Fallback: check common Unix paths directly
	commonPaths := []string{
		"/usr/bin/" + command,
		"/usr/local/bin/" + command,
		"/bin/" + command,
	}
	for _, path := range commonPaths {
		if _, statErr := os.Stat(path); statErr == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("command not found: %s", command)
}

// CreateIntercept creates a symlink at targetDir/command pointing to binaryPath.
func (u *unixPlatform) CreateIntercept(binaryPath, targetDir, command string) (string, error) {
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return "", fmt.Errorf("cli-replay binary not found: %s", binaryPath)
	}
	if info, err := os.Stat(targetDir); err != nil || !info.IsDir() {
		return "", fmt.Errorf("intercept directory does not exist: %s", targetDir)
	}

	linkPath := filepath.Join(targetDir, command)
	if err := os.Symlink(binaryPath, linkPath); err != nil {
		return "", fmt.Errorf("failed to create symlink: %w", err)
	}
	return linkPath, nil
}

// InterceptFileName returns the command name without extension.
func (u *unixPlatform) InterceptFileName(command string) string {
	return command
}

// filterPath removes excludeDir from a PATH string.
func filterPath(pathEnv, excludeDir string) string {
	if excludeDir == "" {
		return pathEnv
	}
	parts := filepath.SplitList(pathEnv)
	filtered := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != excludeDir {
			filtered = append(filtered, p)
		}
	}
	return strings.Join(filtered, string(os.PathListSeparator))
}
