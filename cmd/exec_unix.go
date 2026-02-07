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
// Process.Signal, and returns a cleanup function that stops notification
// and closes the channel.
func setupSignalForwarding(childCmd *exec.Cmd) func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for sig := range sigCh {
			if childCmd.Process != nil {
				_ = childCmd.Process.Signal(sig) // safe if already dead
			}
		}
	}()

	return func() {
		signal.Stop(sigCh)
		close(sigCh)
	}
}
