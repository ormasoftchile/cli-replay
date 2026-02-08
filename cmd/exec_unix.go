//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// useProcessGroup indicates whether the child was started with Setpgid: true.
// When true, signal forwarding and cleanup target the entire process group.
// When false (fallback mode), only the direct child is signalled.
var useProcessGroup bool

// setupSignalForwarding configures process-group-based signal handling on Unix.
//
// FR-001: Sets Setpgid: true so the child gets its own process group.
// FR-002: Forwards SIGINT/SIGTERM to the entire process group via Kill(-pgid, sig).
// FR-003: Cleanup terminates the group (SIGTERM → 100ms → SIGKILL).
// FR-004: If cmd.Start() fails due to Setpgid, the caller (exec.go) should call
//
//	retryWithoutProcessGroup to clear SysProcAttr and retry.
//
// Returns a postStart hook (no-op on Unix) and a cleanup function.
func setupSignalForwarding(childCmd *exec.Cmd) (postStart func(), cleanup func()) {
	// FR-001: Create a new process group for the child and all descendants.
	childCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	useProcessGroup = true

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for sig := range sigCh {
			if childCmd.Process == nil {
				continue
			}
			sysSig, ok := sig.(syscall.Signal)
			if !ok {
				continue
			}
			if useProcessGroup {
				// FR-002: Signal the entire process group.
				pgid := childCmd.Process.Pid
				_ = syscall.Kill(-pgid, sysSig) // ESRCH if group already gone
			} else {
				// Fallback: signal only the direct child.
				_ = childCmd.Process.Signal(sig)
			}
		}
	}()

	postStart = func() {} // no-op on Unix

	cleanup = func() {
		signal.Stop(sigCh)
		close(sigCh)

		if childCmd.Process == nil {
			return
		}

		if useProcessGroup {
			// FR-003: Best-effort cleanup of entire process group.
			pgid := childCmd.Process.Pid
			// Send SIGTERM to group — ignore ESRCH (already gone).
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
			time.Sleep(100 * time.Millisecond)
			// Escalate to SIGKILL for any survivors.
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		}
	}

	return postStart, cleanup
}

// retryWithoutProcessGroup clears SysProcAttr so cmd.Start() can be retried
// in single-process mode. It emits a warning to stderr. Called by exec.go
// when the initial Start() fails with Setpgid: true (FR-004).
func retryWithoutProcessGroup(childCmd *exec.Cmd) error {
	fmt.Fprintf(os.Stderr, "cli-replay: warning: process group creation failed, falling back to single-process signal forwarding\n")
	childCmd.SysProcAttr = nil
	useProcessGroup = false
	return childCmd.Start()
}
