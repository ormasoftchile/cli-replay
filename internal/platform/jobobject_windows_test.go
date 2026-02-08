//go:build windows

package platform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewJobObject_ReturnsValidHandle(t *testing.T) {
	job, err := NewJobObject()
	require.NoError(t, err, "NewJobObject should succeed on Windows 8+")
	require.NotNil(t, job)
	defer func() { _ = job.Close() }()

	assert.False(t, job.IsAssigned(), "newly created job should not have assigned processes")
}

func TestJobObject_Close_ReleasesHandle(t *testing.T) {
	job, err := NewJobObject()
	require.NoError(t, err)
	require.NotNil(t, job)

	err = job.Close()
	assert.NoError(t, err, "Close should succeed")

	// Double close should be safe (no-op)
	err = job.Close()
	assert.NoError(t, err, "double Close should be safe")
	assert.False(t, job.IsAssigned(), "after close, assigned should be false")
}

func TestJobObject_Terminate_EmptyJobSucceeds(t *testing.T) {
	job, err := NewJobObject()
	require.NoError(t, err)
	defer func() { _ = job.Close() }()

	// Terminating an empty job should not error
	err = job.Terminate(1)
	assert.NoError(t, err, "Terminate on empty job should succeed")
}

func TestJobObject_DoubleClose_Safe(t *testing.T) {
	job, err := NewJobObject()
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		_ = job.Close()
		_ = job.Close()
	}, "double close must not panic")
}

func TestJobObject_AssignProcess_InvalidPID(t *testing.T) {
	job, err := NewJobObject()
	require.NoError(t, err)
	defer func() { _ = job.Close() }()

	// PID 0 or a non-existent PID should fail gracefully
	err = job.AssignProcess(0)
	assert.Error(t, err, "AssignProcess with PID 0 should fail")
	assert.False(t, job.IsAssigned())
}

func TestJobObject_Terminate_AfterClose(t *testing.T) {
	job, err := NewJobObject()
	require.NoError(t, err)

	_ = job.Close()

	// Terminate after close should be a no-op (handle == 0)
	err = job.Terminate(1)
	assert.NoError(t, err, "Terminate after Close should be a no-op")
}
