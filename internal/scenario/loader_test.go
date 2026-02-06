package scenario

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ValidScenario(t *testing.T) {
	yaml := `
meta:
  name: "test-scenario"
  description: "A test scenario"
  vars:
    cluster: "prod"
    namespace: "default"
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: "pod-1 Running"
  - match:
      argv: ["kubectl", "delete", "pod", "pod-1"]
    respond:
      exit: 0
      stderr: "pod deleted"
`
	scenario, err := Load(strings.NewReader(yaml))
	require.NoError(t, err)

	assert.Equal(t, "test-scenario", scenario.Meta.Name)
	assert.Equal(t, "A test scenario", scenario.Meta.Description)
	assert.Equal(t, "prod", scenario.Meta.Vars["cluster"])
	assert.Equal(t, "default", scenario.Meta.Vars["namespace"])

	require.Len(t, scenario.Steps, 2)

	assert.Equal(t, []string{"kubectl", "get", "pods"}, scenario.Steps[0].Match.Argv)
	assert.Equal(t, 0, scenario.Steps[0].Respond.Exit)
	assert.Equal(t, "pod-1 Running", scenario.Steps[0].Respond.Stdout)

	assert.Equal(t, []string{"kubectl", "delete", "pod", "pod-1"}, scenario.Steps[1].Match.Argv)
	assert.Equal(t, 0, scenario.Steps[1].Respond.Exit)
	assert.Equal(t, "pod deleted", scenario.Steps[1].Respond.Stderr)
}

func TestLoad_MinimalScenario(t *testing.T) {
	yaml := `
meta:
  name: "minimal"
steps:
  - match:
      argv: ["cmd"]
    respond:
      exit: 0
`
	scenario, err := Load(strings.NewReader(yaml))
	require.NoError(t, err)

	assert.Equal(t, "minimal", scenario.Meta.Name)
	assert.Empty(t, scenario.Meta.Description)
	assert.Empty(t, scenario.Meta.Vars)
	require.Len(t, scenario.Steps, 1)
}

func TestLoad_WithStdoutFile(t *testing.T) {
	yaml := `
meta:
  name: "file-test"
steps:
  - match:
      argv: ["cmd"]
    respond:
      exit: 0
      stdout_file: "fixtures/output.txt"
`
	scenario, err := Load(strings.NewReader(yaml))
	require.NoError(t, err)

	assert.Equal(t, "fixtures/output.txt", scenario.Steps[0].Respond.StdoutFile)
}

func TestLoad_WithStderrFile(t *testing.T) {
	yaml := `
meta:
  name: "file-test"
steps:
  - match:
      argv: ["cmd"]
    respond:
      exit: 1
      stderr_file: "fixtures/error.txt"
`
	scenario, err := Load(strings.NewReader(yaml))
	require.NoError(t, err)

	assert.Equal(t, "fixtures/error.txt", scenario.Steps[0].Respond.StderrFile)
}

//nolint:funlen // Table-driven test with comprehensive test cases
func TestLoad_UnknownFieldRejected(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "unknown field at root",
			yaml: `
meta:
  name: "test"
unknown_field: "value"
steps:
  - match:
      argv: ["cmd"]
    respond:
      exit: 0
`,
		},
		{
			name: "unknown field in meta",
			yaml: `
meta:
  name: "test"
  unknown_meta: "value"
steps:
  - match:
      argv: ["cmd"]
    respond:
      exit: 0
`,
		},
		{
			name: "unknown field in step",
			yaml: `
meta:
  name: "test"
steps:
  - match:
      argv: ["cmd"]
    respond:
      exit: 0
    unknown_step: "value"
`,
		},
		{
			name: "unknown field in match",
			yaml: `
meta:
  name: "test"
steps:
  - match:
      argv: ["cmd"]
      unknown_match: "value"
    respond:
      exit: 0
`,
		},
		{
			name: "unknown field in respond",
			yaml: `
meta:
  name: "test"
steps:
  - match:
      argv: ["cmd"]
    respond:
      exit: 0
      unknown_respond: "value"
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(strings.NewReader(tt.yaml))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unknown")
		})
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "malformed yaml",
			yaml: `
meta:
  name: test
  [invalid
`,
		},
		{
			name: "empty input",
			yaml: "",
		},
		{
			name: "wrong type for argv",
			yaml: `
meta:
  name: "test"
steps:
  - match:
      argv: "not-an-array"
    respond:
      exit: 0
`,
		},
		{
			name: "wrong type for exit",
			yaml: `
meta:
  name: "test"
steps:
  - match:
      argv: ["cmd"]
    respond:
      exit: "not-an-int"
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(strings.NewReader(tt.yaml))
			require.Error(t, err)
		})
	}
}

//nolint:funlen // Table-driven test with comprehensive test cases
func TestLoad_ValidationFailures(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		errContains string
	}{
		{
			name: "empty name",
			yaml: `
meta:
  name: ""
steps:
  - match:
      argv: ["cmd"]
    respond:
      exit: 0
`,
			errContains: "name must be non-empty",
		},
		{
			name: "empty steps",
			yaml: `
meta:
  name: "test"
steps: []
`,
			errContains: "steps must contain at least one step",
		},
		{
			name: "empty argv",
			yaml: `
meta:
  name: "test"
steps:
  - match:
      argv: []
    respond:
      exit: 0
`,
			errContains: "argv must be non-empty",
		},
		{
			name: "exit out of range",
			yaml: `
meta:
  name: "test"
steps:
  - match:
      argv: ["cmd"]
    respond:
      exit: 300
`,
			errContains: "exit must be in range 0-255",
		},
		{
			name: "stdout and stdout_file both set",
			yaml: `
meta:
  name: "test"
steps:
  - match:
      argv: ["cmd"]
    respond:
      exit: 0
      stdout: "inline"
      stdout_file: "file.txt"
`,
			errContains: "stdout and stdout_file are mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(strings.NewReader(tt.yaml))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestLoadFile_NotFound(t *testing.T) {
	_, err := LoadFile("/nonexistent/path/scenario.yaml")
	require.Error(t, err)
}
