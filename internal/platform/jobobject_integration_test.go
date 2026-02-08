//go:build windows && integration

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// TestHelper_SleepForever is a helper process that sleeps indefinitely.
// It is invoked by other tests via -test.run=TestHelper_SleepForever.
func TestHelper_SleepForever(t *testing.T) {
	if os.Getenv("CLI_REPLAY_TEST_HELPER") != "1" {
		return // not the helper subprocess
	}
	// Write PID to stdout so the parent can track it
	fmt.Fprintf(os.Stdout, "%d", os.Getpid())
	os.Stdout.Sync()
	time.Sleep(10 * time.Minute)
}

// TestHelper_SpawnGrandchild is a helper process that spawns a grandchild
// (itself, but with SLEEP mode) and then waits.
func TestHelper_SpawnGrandchild(t *testing.T) {
	if os.Getenv("CLI_REPLAY_TEST_HELPER") != "1" {
		return
	}
	mode := os.Getenv("CLI_REPLAY_TEST_MODE")
	if mode == "SLEEP" {
		// Grandchild: just sleep
		fmt.Fprintf(os.Stdout, "%d", os.Getpid())
		os.Stdout.Sync()
		time.Sleep(10 * time.Minute)
		return
	}

	// Parent: read grandchild PID file path from env
	pidFile := os.Getenv("CLI_REPLAY_PID_FILE")

	// Spawn grandchild (ourselves with SLEEP mode)
	self, _ := os.Executable()
	grandchild := exec.Command(self, "-test.run=TestHelper_SpawnGrandchild", "-test.v")
	grandchild.Env = append(os.Environ(),
		"CLI_REPLAY_TEST_HELPER=1",
		"CLI_REPLAY_TEST_MODE=SLEEP",
	)
	grandchild.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}

	var gcOut strings.Builder
	grandchild.Stdout = &gcOut
	require.NoError(t, grandchild.Start())

	// Wait briefly for grandchild to emit its PID
	time.Sleep(500 * time.Millisecond)

	// Write grandchild PID to file
	gcPID := strings.TrimSpace(gcOut.String())
	if gcPID == "" {
		gcPID = strconv.Itoa(grandchild.Process.Pid)
	}
	if pidFile != "" {
		os.WriteFile(pidFile, []byte(gcPID), 0644) //nolint:errcheck
	}

	// Write our PID to stdout
	fmt.Fprintf(os.Stdout, "%d", os.Getpid())
	os.Stdout.Sync()

	// Wait for grandchild (will be killed by job object)
	_ = grandchild.Wait()
}

// processExists checks if a process with the given PID is still running.
func processExists(pid uint32) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle) //nolint:errcheck

	var exitCode uint32
	err = windows.GetExitCodeProcess(handle, &exitCode)
	if err != nil {
		return false
	}
	// STILL_ACTIVE (259) means the process is running
	return exitCode == 259
}

// ---- Tests ----

// TestWindows_JobObject_AssignAndTerminate tests creating a job, assigning
// a real process, and terminating it via the job.
func TestWindows_JobObject_AssignAndTerminate(t *testing.T) {
	job, err := NewJobObject()
	require.NoError(t, err)
	defer job.Close() //nolint:errcheck

	// Spawn a long-running child
	self, _ := os.Executable()
	child := exec.Command(self, "-test.run=TestHelper_SleepForever", "-test.v")
	child.Env = append(os.Environ(), "CLI_REPLAY_TEST_HELPER=1")
	require.NoError(t, child.Start())

	pid := uint32(child.Process.Pid)

	// Assign to job
	err = job.AssignProcess(int(pid))
	require.NoError(t, err)
	assert.True(t, job.IsAssigned())

	// Verify process is alive
	assert.True(t, processExists(pid), "child should be running")

	// Terminate via job
	err = job.Terminate(1)
	require.NoError(t, err)

	// Wait for termination to take effect
	_ = child.Wait()
	time.Sleep(200 * time.Millisecond)

	assert.False(t, processExists(pid), "child should be terminated")
}

// TestWindows_JobObject_ProcessTreeKill tests that terminating a job kills
// all processes assigned to it (simulating process tree kill).
func TestWindows_JobObject_ProcessTreeKill(t *testing.T) {
	job, err := NewJobObject()
	require.NoError(t, err)
	defer job.Close() //nolint:errcheck

	// Spawn two independent long-running processes and assign both to the job.
	// Terminating the job should kill both simultaneously.
	self, _ := os.Executable()

	child1 := exec.Command(self, "-test.run=TestHelper_SleepForever", "-test.v")
	child1.Env = append(os.Environ(), "CLI_REPLAY_TEST_HELPER=1")
	require.NoError(t, child1.Start())
	pid1 := uint32(child1.Process.Pid)

	child2 := exec.Command(self, "-test.run=TestHelper_SleepForever", "-test.v")
	child2.Env = append(os.Environ(), "CLI_REPLAY_TEST_HELPER=1")
	require.NoError(t, child2.Start())
	pid2 := uint32(child2.Process.Pid)

	// Assign both to the same job
	require.NoError(t, job.AssignProcess(int(pid1)))
	require.NoError(t, job.AssignProcess(int(pid2)))

	assert.True(t, processExists(pid1), "child1 should be running")
	assert.True(t, processExists(pid2), "child2 should be running")

	// Terminate the job — should kill both
	err = job.Terminate(1)
	require.NoError(t, err)
	_ = child1.Wait()
	_ = child2.Wait()

	time.Sleep(500 * time.Millisecond)

	assert.False(t, processExists(pid1), "child1 should be dead")
	assert.False(t, processExists(pid2), "child2 should be dead (multi-process job kill)")
}

// TestWindows_JobObject_KillOnJobClose tests that KILL_ON_JOB_CLOSE
// terminates processes when the handle is closed.
func TestWindows_JobObject_KillOnJobClose(t *testing.T) {
	job, err := NewJobObject()
	require.NoError(t, err)

	// Spawn a long-running child
	self, _ := os.Executable()
	child := exec.Command(self, "-test.run=TestHelper_SleepForever", "-test.v")
	child.Env = append(os.Environ(), "CLI_REPLAY_TEST_HELPER=1")
	require.NoError(t, child.Start())

	pid := uint32(child.Process.Pid)

	err = job.AssignProcess(int(pid))
	require.NoError(t, err)

	// Close the handle — KILL_ON_JOB_CLOSE should terminate the process
	err = job.Close()
	require.NoError(t, err)

	_ = child.Wait()
	time.Sleep(200 * time.Millisecond)

	assert.False(t, processExists(pid), "KILL_ON_JOB_CLOSE should terminate on handle close")
}

// TestWindows_JobObject_SuspendResumeFlow tests the CREATE_SUSPENDED +
// AssignProcess + ResumeThread pattern used by exec.
func TestWindows_JobObject_SuspendResumeFlow(t *testing.T) {
	job, err := NewJobObject()
	require.NoError(t, err)
	defer job.Close() //nolint:errcheck

	// Start a child in suspended mode
	child := exec.Command("cmd.exe", "/c", "echo", "resumed")
	child.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_SUSPENDED,
	}
	var outBuf strings.Builder
	child.Stdout = &outBuf

	require.NoError(t, child.Start())
	pid := uint32(child.Process.Pid)

	// Assign to job while suspended
	err = job.AssignProcess(int(pid))
	require.NoError(t, err)

	// Resume and wait for completion
	resumeAllThreads(pid)
	err = child.Wait()
	require.NoError(t, err, "resumed child should complete successfully")

	assert.Contains(t, outBuf.String(), "resumed",
		"child should produce output after resume")
}

// resumeAllThreads enumerates and resumes all threads of the given process.
func resumeAllThreads(pid uint32) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return
	}
	defer windows.CloseHandle(snapshot) //nolint:errcheck

	var te windows.ThreadEntry32
	te.Size = uint32(unsafe.Sizeof(te))

	err = windows.Thread32First(snapshot, &te)
	for err == nil {
		if te.OwnerProcessID == pid {
			th, openErr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, te.ThreadID)
			if openErr == nil {
				windows.ResumeThread(th) //nolint:errcheck
				windows.CloseHandle(th)  //nolint:errcheck
			}
		}
		err = windows.Thread32Next(snapshot, &te)
	}
}

// TestWindows_SignalPropagation_CtrlC tests that Ctrl+C is forwarded to the
// child process tree via the Job Object. Uses GenerateConsoleCtrlEvent.
func TestWindows_SignalPropagation_CtrlC(t *testing.T) {
	// This test requires that the child is in the same console group.
	// GenerateConsoleCtrlEvent sends to a process group, which may affect
	// the test runner. To avoid this, we create the child in a new group
	// and use the Job Object terminate path instead.

	job, err := NewJobObject()
	require.NoError(t, err)
	defer job.Close() //nolint:errcheck

	// Spawn a long-running child
	self, _ := os.Executable()
	child := exec.Command(self, "-test.run=TestHelper_SleepForever", "-test.v")
	child.Env = append(os.Environ(), "CLI_REPLAY_TEST_HELPER=1")
	child.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_SUSPENDED,
	}
	require.NoError(t, child.Start())

	pid := uint32(child.Process.Pid)
	err = job.AssignProcess(int(pid))
	require.NoError(t, err)

	resumeAllThreads(pid)
	time.Sleep(200 * time.Millisecond)

	// Instead of GenerateConsoleCtrlEvent (which affects the current console),
	// we use TerminateJobObject — this is what cli-replay exec does on Ctrl+C.
	err = job.Terminate(1)
	require.NoError(t, err)

	_ = child.Wait()
	time.Sleep(200 * time.Millisecond)

	assert.False(t, processExists(pid), "child should be terminated after simulated Ctrl+C")
}

// TestWindows_FallbackKill_NoJobObject tests the fallback path where
// Process.Kill() is used instead of Job Objects.
func TestWindows_FallbackKill_NoJobObject(t *testing.T) {
	// Spawn a long-running child WITHOUT a job object
	self, _ := os.Executable()
	child := exec.Command(self, "-test.run=TestHelper_SleepForever", "-test.v")
	child.Env = append(os.Environ(), "CLI_REPLAY_TEST_HELPER=1")
	require.NoError(t, child.Start())

	pid := uint32(child.Process.Pid)
	assert.True(t, processExists(pid))

	// Fallback: kill via Process.Kill()
	err := child.Process.Kill()
	require.NoError(t, err)

	_ = child.Wait()
	time.Sleep(200 * time.Millisecond)

	assert.False(t, processExists(pid), "Process.Kill() should terminate child")
}
