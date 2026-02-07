package runner

import (
	"errors"
	"os/exec"
	"syscall"
)

// ExitCodeFromError extracts the exit code from an exec.Cmd.Wait() error.
//
// Returns:
//   - 0 if err is nil (child exited successfully)
//   - The child's exit code if it exited normally with non-zero status
//   - 128+signum if the child was killed by a signal (POSIX convention)
//   - 1 for any other error (e.g., command not found wraps a non-ExitError)
func ExitCodeFromError(err error) int {
	if err == nil {
		return 0
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 1
	}

	// Try to get signal information via syscall.WaitStatus (Unix)
	if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok {
		if ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return ws.ExitStatus()
	}

	// Fallback: use ExitCode() (returns -1 for signal-killed, but we
	// already handled that above via WaitStatus)
	if code := exitErr.ExitCode(); code >= 0 {
		return code
	}
	return 1
}
