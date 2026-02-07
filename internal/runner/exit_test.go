package runner

import (
	"fmt"
	"os/exec"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExitCodeFromError_NilReturnsZero(t *testing.T) {
	assert.Equal(t, 0, ExitCodeFromError(nil))
}

func TestExitCodeFromError_NormalNonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	cmd := exec.Command("sh", "-c", "exit 42")
	err := cmd.Run()
	assert.Equal(t, 42, ExitCodeFromError(err))
}

func TestExitCodeFromError_ExitCodeOne(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	cmd := exec.Command("sh", "-c", "exit 1")
	err := cmd.Run()
	assert.Equal(t, 1, ExitCodeFromError(err))
}

func TestExitCodeFromError_SignalKilled_SIGTERM(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	// Start a process and kill it with SIGTERM (signal 15)
	cmd := exec.Command("sh", "-c", "kill -TERM $$")
	err := cmd.Run()
	assert.Equal(t, 143, ExitCodeFromError(err), "SIGTERM should produce exit code 143 (128+15)")
}

func TestExitCodeFromError_SignalKilled_SIGINT(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	// Start a process that kills itself with SIGINT (signal 2)
	cmd := exec.Command("sh", "-c", "kill -INT $$")
	err := cmd.Run()
	assert.Equal(t, 130, ExitCodeFromError(err), "SIGINT should produce exit code 130 (128+2)")
}

func TestExitCodeFromError_NonExitErrorFallback(t *testing.T) {
	// A non-ExitError (e.g., command not found at exec level)
	err := fmt.Errorf("some random error")
	assert.Equal(t, 1, ExitCodeFromError(err))
}

func TestExitCodeFromError_ExitCodeZero(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	cmd := exec.Command("sh", "-c", "exit 0")
	err := cmd.Run()
	assert.Equal(t, 0, ExitCodeFromError(err))
}

func TestExitCodeFromError_HighExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-specific test")
	}

	cmd := exec.Command("sh", "-c", "exit 255")
	err := cmd.Run()
	assert.Equal(t, 255, ExitCodeFromError(err))
}
