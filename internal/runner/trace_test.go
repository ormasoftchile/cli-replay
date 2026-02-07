package runner

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTraceOutput_Format(t *testing.T) {
	var buf bytes.Buffer

	WriteTraceOutput(&buf, 0, []string{"kubectl", "get", "pods"}, 0)

	output := buf.String()
	assert.Contains(t, output, "[cli-replay]")
	assert.Contains(t, output, "step=0")
	assert.Contains(t, output, "kubectl")
	assert.Contains(t, output, "exit=0")
}

func TestTraceOutput_NonZeroExit(t *testing.T) {
	var buf bytes.Buffer

	WriteTraceOutput(&buf, 2, []string{"cmd", "arg"}, 1)

	output := buf.String()
	assert.Contains(t, output, "step=2")
	assert.Contains(t, output, "exit=1")
}

func TestTraceOutput_MultipleArgs(t *testing.T) {
	var buf bytes.Buffer

	WriteTraceOutput(&buf, 0, []string{"kubectl", "get", "pods", "-n", "production", "-o", "json"}, 0)

	output := buf.String()
	assert.Contains(t, output, "production")
	assert.Contains(t, output, "json")
}

func TestIsTraceEnabled(t *testing.T) {
	// Test with various env values
	tests := []struct {
		name     string
		envValue string
		want     bool
	}{
		{"enabled with 1", "1", true},
		{"enabled with true", "true", true},
		{"disabled with 0", "0", false},
		{"disabled with false", "false", false},
		{"disabled when empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTraceEnabled(tt.envValue)
			assert.Equal(t, tt.want, got)
		})
	}
}

// T009: WriteDeniedEnvTrace tests

func TestWriteDeniedEnvTrace_Format(t *testing.T) {
	var buf bytes.Buffer
	WriteDeniedEnvTrace(&buf, "AWS_SECRET_ACCESS_KEY")

	output := buf.String()
	assert.Equal(t, "cli-replay[trace]: denied env var AWS_SECRET_ACCESS_KEY\n", output)
}

func TestWriteDeniedEnvTrace_MultipleVars(t *testing.T) {
	var buf bytes.Buffer
	WriteDeniedEnvTrace(&buf, "AWS_KEY")
	WriteDeniedEnvTrace(&buf, "GITHUB_TOKEN")

	output := buf.String()
	assert.Contains(t, output, "denied env var AWS_KEY")
	assert.Contains(t, output, "denied env var GITHUB_TOKEN")
}
