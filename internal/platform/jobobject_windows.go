//go:build windows

package platform

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// JobObject wraps a Windows Job Object for process tree lifecycle management.
// When the job object is configured with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
// closing the handle automatically terminates all processes assigned to it.
type JobObject struct {
	handle   windows.Handle
	assigned bool
}

// NewJobObject creates an anonymous Windows Job Object with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE configured. This ensures that when
// the job handle is closed, all processes in the job are terminated.
func NewJobObject() (*JobObject, error) {
	handle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateJobObject failed: %w", err)
	}

	// Configure KILL_ON_JOB_CLOSE so closing the handle terminates all processes
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE

	_, err = windows.SetInformationJobObject(
		handle,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		// Best-effort: close the handle if configuration fails
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("SetInformationJobObject failed: %w", err)
	}

	return &JobObject{handle: handle}, nil
}

// AssignProcess assigns a running process (identified by PID) to the job object.
// All child processes spawned by the assigned process will also be tracked by
// the job. This should be called after the process is created but before it
// begins execution (CREATE_SUSPENDED + ResumeThread pattern).
func (j *JobObject) AssignProcess(pid int) error {
	if j.handle == 0 {
		return fmt.Errorf("job object handle is invalid")
	}

	// Open the process with sufficient access for job assignment
	const access = windows.PROCESS_SET_QUOTA | windows.PROCESS_TERMINATE
	processHandle, err := windows.OpenProcess(access, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("OpenProcess(%d) failed: %w", pid, err)
	}
	defer func() { _ = windows.CloseHandle(processHandle) }()

	if err := windows.AssignProcessToJobObject(j.handle, processHandle); err != nil {
		return fmt.Errorf("AssignProcessToJobObject failed: %w", err)
	}

	j.assigned = true
	return nil
}

// Terminate explicitly kills all processes in the job with the given exit code.
// Errors are returned but should generally be ignored — the processes may
// already have exited.
//
// Not goroutine-safe — callers must synchronize access if Terminate and Close
// may be called concurrently from different goroutines.
func (j *JobObject) Terminate(exitCode uint32) error {
	if j.handle == 0 {
		return nil
	}
	if err := windows.TerminateJobObject(j.handle, exitCode); err != nil {
		return fmt.Errorf("TerminateJobObject failed: %w", err)
	}
	return nil
}

// Close releases the job handle. If JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE is set
// and processes are still running, they will be terminated. Close is safe to
// call multiple times.
func (j *JobObject) Close() error {
	if j.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(j.handle)
	j.handle = 0
	j.assigned = false
	return err
}

// IsAssigned returns true if a process has been successfully assigned to the job.
func (j *JobObject) IsAssigned() bool {
	return j.assigned
}
