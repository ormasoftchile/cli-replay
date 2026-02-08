//go:build !windows && integration

package cmd

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestProcessGroupCleanup verifies that when cli-replay's exec_unix
// setupSignalForwarding is used, killing the parent also kills all
// descendants in the process group (FR-001, FR-002, FR-003).
//
// Strategy: Build a small Go helper (this test binary with -run flag)
// that spawns a shell which spawns a background grandchild, then the
// test sends SIGTERM to the parent and verifies the grandchild also exits.
func TestProcessGroupCleanup(t *testing.T) {
	if os.Getenv("CLI_REPLAY_TEST_CHILD") == "1" {
		// We are the spawned child: exec a shell that creates a grandchild.
		// The grandchild writes its PID to a file, then sleeps forever.
		pidFile := os.Getenv("CLI_REPLAY_PIDFILE")
		if pidFile == "" {
			t.Fatal("CLI_REPLAY_PIDFILE not set")
		}
		child := exec.Command("bash", "-c",
			"echo $$ > "+pidFile+".grandchild && sleep 300 &\necho $$ > "+pidFile+" && sleep 300")
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr

		postStart, cleanup := setupSignalForwarding(child)
		if err := child.Start(); err != nil {
			t.Fatalf("child start: %v", err)
		}
		postStart()
		_ = child.Wait()
		cleanup()
		return
	}

	// Main test: spawn this binary as a child, then kill it, then check grandchild.
	tmpDir := t.TempDir()
	pidFile := tmpDir + "/pids"

	cmd := exec.Command(os.Args[0], "-test.run=^TestProcessGroupCleanup$", "-test.v")
	cmd.Env = append(os.Environ(),
		"CLI_REPLAY_TEST_CHILD=1",
		"CLI_REPLAY_PIDFILE="+pidFile,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start test child: %v", err)
	}

	// Wait for grandchild PID file to appear (up to 5s).
	grandchildPidFile := pidFile + ".grandchild"
	var grandchildPid int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(grandchildPidFile)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if parseErr == nil && pid > 0 {
				grandchildPid = pid
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if grandchildPid == 0 {
		_ = cmd.Process.Kill()
		t.Fatal("grandchild PID file never appeared")
	}

	// Send SIGTERM to the parent process. Since we started it normally
	// (not via setupSignalForwarding), we kill it directly. The parent's
	// own signal handler should clean up the group.
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("failed to send SIGINT to parent: %v", err)
	}

	// Wait for the parent to exit (up to 5s).
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		// Parent exited.
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("parent did not exit within 5 seconds after SIGTERM")
	}

	// Verify grandchild is also dead (up to 200ms).
	time.Sleep(200 * time.Millisecond)
	proc, err := os.FindProcess(grandchildPid)
	if err == nil {
		// On Unix, FindProcess always succeeds. Check if it's alive by sending signal 0.
		err = proc.Signal(syscall.Signal(0))
		if err == nil {
			t.Errorf("grandchild process %d is still alive after parent was killed", grandchildPid)
			_ = proc.Kill()
		}
		// If err != nil, process is dead — that's what we want.
	}
}
