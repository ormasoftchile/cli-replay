//go:build windows

package cmd

import (
	"os"
	"os/exec"
	"os/signal"
)

// setupSignalForwarding registers os.Interrupt (Ctrl+C) on Windows. On
// signal receipt the child process is killed via Process.Kill() because
// Windows does not support SIGTERM. Returns a cleanup function that stops
// notification and closes the channel.
func setupSignalForwarding(childCmd *exec.Cmd) func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		for range sigCh {
			if childCmd.Process != nil {
				_ = childCmd.Process.Kill() // Windows: no SIGTERM, use Kill()
			}
		}
	}()

	return func() {
		signal.Stop(sigCh)
		close(sigCh)
	}
}
