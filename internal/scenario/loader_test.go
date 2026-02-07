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

	assert.Equal(t, []string{"kubectl", "get", "pods"}, scenario.Steps[0].Step.Match.Argv)
	assert.Equal(t, 0, scenario.Steps[0].Step.Respond.Exit)
	assert.Equal(t, "pod-1 Running", scenario.Steps[0].Step.Respond.Stdout)

	assert.Equal(t, []string{"kubectl", "delete", "pod", "pod-1"}, scenario.Steps[1].Step.Match.Argv)
	assert.Equal(t, 0, scenario.Steps[1].Step.Respond.Exit)
	assert.Equal(t, "pod deleted", scenario.Steps[1].Step.Respond.Stderr)
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

	assert.Equal(t, "fixtures/output.txt", scenario.Steps[0].Step.Respond.StdoutFile)
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

	assert.Equal(t, "fixtures/error.txt", scenario.Steps[0].Step.Respond.StderrFile)
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

// T021: Group YAML loading tests

func TestLoad_GroupDecodesCorrectly(t *testing.T) {
	yamlContent := `
meta:
  name: group-load-test
steps:
  - group:
      mode: unordered
      name: pre-flight
      steps:
        - match:
            argv: ["kubectl", "get", "nodes"]
          respond:
            exit: 0
            stdout: "node-1 Ready"
        - match:
            argv: ["kubectl", "get", "ns"]
          respond:
            exit: 0
            stdout: "default"
`
	scn, err := Load(strings.NewReader(yamlContent))
	require.NoError(t, err)

	require.Len(t, scn.Steps, 1)
	require.NotNil(t, scn.Steps[0].Group)
	assert.Nil(t, scn.Steps[0].Step)

	g := scn.Steps[0].Group
	assert.Equal(t, "unordered", g.Mode)
	assert.Equal(t, "pre-flight", g.Name)
	require.Len(t, g.Steps, 2)
	assert.Equal(t, []string{"kubectl", "get", "nodes"}, g.Steps[0].Step.Match.Argv)
	assert.Equal(t, []string{"kubectl", "get", "ns"}, g.Steps[1].Step.Match.Argv)
}

func TestLoad_PlainStepStillDecodes(t *testing.T) {
	yamlContent := `
meta:
  name: plain-step
steps:
  - match:
      argv: ["echo", "hi"]
    respond:
      exit: 0
      stdout: "hi"
`
	scn, err := Load(strings.NewReader(yamlContent))
	require.NoError(t, err)

	require.Len(t, scn.Steps, 1)
	require.NotNil(t, scn.Steps[0].Step)
	assert.Nil(t, scn.Steps[0].Group)
	assert.Equal(t, []string{"echo", "hi"}, scn.Steps[0].Step.Match.Argv)
}

func TestLoad_MixedStepsAndGroups(t *testing.T) {
	yamlContent := `
meta:
  name: mixed
steps:
  - match:
      argv: ["setup"]
    respond:
      exit: 0
  - group:
      mode: unordered
      steps:
        - match:
            argv: ["a"]
          respond:
            exit: 0
        - match:
            argv: ["b"]
          respond:
            exit: 0
  - match:
      argv: ["teardown"]
    respond:
      exit: 0
`
	scn, err := Load(strings.NewReader(yamlContent))
	require.NoError(t, err)

	require.Len(t, scn.Steps, 3)
	assert.NotNil(t, scn.Steps[0].Step)
	assert.NotNil(t, scn.Steps[1].Group)
	assert.NotNil(t, scn.Steps[2].Step)

	assert.Equal(t, "group-1", scn.Steps[1].Group.Name) // auto-named
	require.Len(t, scn.Steps[1].Group.Steps, 2)
}

func TestLoad_GroupWithExplicitName(t *testing.T) {
	yamlContent := `
meta:
  name: named
steps:
  - group:
      mode: unordered
      name: health-checks
      steps:
        - match:
            argv: ["check"]
          respond:
            exit: 0
`
	scn, err := Load(strings.NewReader(yamlContent))
	require.NoError(t, err)

	assert.Equal(t, "health-checks", scn.Steps[0].Group.Name)
}

func TestLoad_GroupWithoutName(t *testing.T) {
	yamlContent := `
meta:
  name: unnamed
steps:
  - group:
      mode: unordered
      steps:
        - match:
            argv: ["cmd"]
          respond:
            exit: 0
`
	scn, err := Load(strings.NewReader(yamlContent))
	require.NoError(t, err)

	// Name auto-assigned during Validate
	assert.Equal(t, "group-1", scn.Steps[0].Group.Name)
}

func TestLoad_GroupValidationErrorsSurface(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		errContains string
	}{
		{
			name: "empty group steps",
			yaml: `
meta:
  name: bad
steps:
  - group:
      mode: unordered
      steps: []
`,
			errContains: "must contain at least one step",
		},
		{
			name: "unsupported group mode",
			yaml: `
meta:
  name: bad
steps:
  - group:
      mode: ordered
      steps:
        - match:
            argv: ["cmd"]
          respond:
            exit: 0
`,
			errContains: "unsupported group mode",
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

// T022: Validation loading tests for capture conflict and forward reference scenarios.

func TestLoad_CaptureConflict_ValidationError(t *testing.T) {
	yamlContent := `
meta:
  name: capture-conflict
  vars:
    region: "eastus"
steps:
  - match:
      argv: ["cmd"]
    respond:
      exit: 0
      stdout: "done"
      capture:
        region: "westus"
`
	_, err := Load(strings.NewReader(yamlContent))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "region")
	assert.Contains(t, err.Error(), "conflicts")
}

func TestLoad_CaptureForwardRef_ValidationError(t *testing.T) {
	yamlContent := `
meta:
  name: capture-forward-ref
steps:
  - match:
      argv: ["cmd"]
    respond:
      exit: 0
      stdout: 'hello {{ .capture.vm_id }}'
  - match:
      argv: ["cmd2"]
    respond:
      exit: 0
      stdout: "world"
      capture:
        vm_id: "vm-123"
`
	_, err := Load(strings.NewReader(yamlContent))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vm_id")
	assert.Contains(t, err.Error(), "forward")
}

func TestLoad_CaptureValid_NoError(t *testing.T) {
	yamlContent := `
meta:
  name: capture-valid
steps:
  - match:
      argv: ["cmd"]
    respond:
      exit: 0
      stdout: "first"
      capture:
        rg_id: "rg-123"
  - match:
      argv: ["cmd2"]
    respond:
      exit: 0
      stdout: 'rg={{ .capture.rg_id }}'
`
	scn, err := Load(strings.NewReader(yamlContent))
	require.NoError(t, err)
	assert.Equal(t, "capture-valid", scn.Meta.Name)
	assert.Equal(t, "rg-123", scn.Steps[0].Step.Respond.Capture["rg_id"])
}

func TestLoad_CaptureInGroup_Valid(t *testing.T) {
	yamlContent := `
meta:
  name: capture-group-valid
steps:
  - match:
      argv: ["setup"]
    respond:
      exit: 0
      capture:
        base_id: "base-123"
  - group:
      mode: unordered
      name: work
      steps:
        - match:
            argv: ["a"]
          respond:
            exit: 0
            capture:
              x: "value-x"
        - match:
            argv: ["b"]
          respond:
            exit: 0
            stdout: 'base={{ .capture.base_id }}'
`
	scn, err := Load(strings.NewReader(yamlContent))
	require.NoError(t, err)
	assert.Equal(t, "capture-group-valid", scn.Meta.Name)
}
