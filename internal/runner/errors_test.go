package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMismatchError_Error(t *testing.T) {
	err := &MismatchError{
		Scenario:  "test-scenario",
		StepIndex: 2,
		Expected:  []string{"kubectl", "get", "pods"},
		Received:  []string{"kubectl", "get", "services"},
	}

	errMsg := err.Error()
	assert.Contains(t, errMsg, "mismatch")
	assert.Contains(t, errMsg, "2")
}

func TestMismatchError_Format(t *testing.T) {
	err := &MismatchError{
		Scenario:  "my-scenario",
		StepIndex: 0,
		Expected:  []string{"cmd", "expected", "args"},
		Received:  []string{"cmd", "different", "args"},
	}

	formatted := FormatMismatchError(err)

	assert.Contains(t, formatted, "my-scenario")
	assert.Contains(t, formatted, "step 0")
	assert.Contains(t, formatted, "expected")
	assert.Contains(t, formatted, "received")
	assert.Contains(t, formatted, "cmd")
}

func TestMismatchError_FormatWithLongArgv(t *testing.T) {
	err := &MismatchError{
		Scenario:  "test",
		StepIndex: 5,
		Expected:  []string{"kubectl", "get", "pods", "-n", "production", "-o", "json", "--all-namespaces"},
		Received:  []string{"kubectl", "get", "pods", "-n", "staging"},
	}

	formatted := FormatMismatchError(err)

	// Should include all elements
	assert.Contains(t, formatted, "production")
	assert.Contains(t, formatted, "staging")
}
