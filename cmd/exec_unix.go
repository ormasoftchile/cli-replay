//go:build !windows

package cmd

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// setupSignalForwarding registers SIGINT and SIGTERM handlers, spawns a
// goroutine that forwards received signals to the child process via
// Process.Signal, and returns a postStart hook (no-op on Unix) and a
// cleanup function that stops notification and closes the channel.
func setupSignalForwarding(childCmd *exec.Cmd) (postStart func(), cleanup func()) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for sig := range sigCh {
			if childCmd.Process != nil {
				_ = childCmd.Process.Signal(sig) // safe if already dead
			}
		}
	}()

	postStart = func() {} // no-op on Unix

	cleanup = func() {
		signal.Stop(sigCh)
		close(sigCh)
	}

	return postStart, cleanup
}
