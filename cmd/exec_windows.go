//go:build windows

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"unsafe"

	"github.com/cli-replay/cli-replay/internal/platform"
	"golang.org/x/sys/windows"
)

// setupSignalForwarding creates a Windows Job Object and registers
// os.Interrupt (Ctrl+C). On signal receipt the entire process tree is
// terminated via TerminateJobObject. If job object creation fails, falls
// back to the legacy Process.Kill() behavior with a warning to stderr.
//
// The returned cleanup function:
//  1. Stops signal notification and closes the channel
//  2. Terminates the job (if active)
//  3. Closes the job handle (safety net via KILL_ON_JOB_CLOSE)
//
// The returned postStart hook must be called after childCmd.Start() to
// assign the child to the job and resume its suspended main thread.
func setupSignalForwarding(childCmd *exec.Cmd) (postStart func(), cleanup func()) {
	job, jobErr := platform.NewJobObject()
	if jobErr != nil {
		fmt.Fprintf(os.Stderr, "cli-replay: warning: job object unavailable, falling back to single-process kill: %v\n", jobErr)
		return func() {}, setupSignalForwardingFallback(childCmd)
	}

	// Set CREATE_SUSPENDED so the child is paused until we assign it to
	// the job — this prevents a race where the child spawns grandchildren
	// before the job assignment.
	if childCmd.SysProcAttr == nil {
		childCmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	childCmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go func() {
		for range sigCh {
			if job != nil {
				_ = job.Terminate(1) // kills entire process tree
			}
		}
	}()

	postStart = func() {
		resumeChildProcess(childCmd, job)
	}

	cleanup = func() {
		signal.Stop(sigCh)
		close(sigCh)
		if job != nil {
			_ = job.Terminate(1)
			_ = job.Close()
		}
	}

	return postStart, cleanup
}

// resumeChildProcess assigns the suspended child to the job object and
// resumes its main thread. This must be called after childCmd.Start()
// when CREATE_SUSPENDED is in effect. If assignment fails, the process
// is resumed anyway and a warning is emitted (falls back to best-effort
// single-process kill semantics).
func resumeChildProcess(childCmd *exec.Cmd, job *platform.JobObject) {
	if childCmd.Process == nil {
		return
	}

	pid := childCmd.Process.Pid
	if err := job.AssignProcess(pid); err != nil {
		fmt.Fprintf(os.Stderr, "cli-replay: warning: could not assign process %d to job object: %v\n", pid, err)
	}

	// Resume the main thread. The thread handle is the child's first thread;
	// we obtain it via NtQueryInformationProcess or by re-opening the
	// process. However, Go's os/exec with CREATE_SUSPENDED stores the
	// thread handle in the PROCESS_INFORMATION struct. We can access it
	// through the process's thread snapshot.
	resumeProcessThreads(uint32(pid))
}

// resumeProcessThreads enumerates and resumes all threads of the given
// process. This is needed after starting a child with CREATE_SUSPENDED.
func resumeProcessThreads(pid uint32) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cli-replay: warning: CreateToolhelp32Snapshot failed: %v\n", err)
		return
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()

	var te windows.ThreadEntry32
	te.Size = uint32(unsafe.Sizeof(te))

	err = windows.Thread32First(snapshot, &te)
	for err == nil {
		if te.OwnerProcessID == pid {
			th, openErr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, te.ThreadID)
			if openErr == nil {
				_, _ = windows.ResumeThread(th)
				_ = windows.CloseHandle(th)
			}
		}
		err = windows.Thread32Next(snapshot, &te)
	}
}

// setupSignalForwardingFallback is the legacy behavior: catch os.Interrupt
// and call Process.Kill(). Used when job object creation fails.
func setupSignalForwardingFallback(childCmd *exec.Cmd) func() {
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

// retryWithoutProcessGroup is a no-op on Windows. Windows uses Job Objects
// for process tree management (not Unix process groups). If the initial
// Start() fails on Windows, there is no Setpgid to clear.
func retryWithoutProcessGroup(_ *exec.Cmd) error {
	return fmt.Errorf("process start retry not supported on Windows")
}
