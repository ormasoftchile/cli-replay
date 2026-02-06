package recorder

import (
	"os"
	"testing"
	"time"

	"github.com/cli-replay/cli-replay/internal/scenario"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertToScenario(t *testing.T) {
	meta := SessionMetadata{
		Name:        "kubectl-workflow",
		Description: "Testing kubectl commands",
		RecordedAt:  mustParseTime("2024-01-15T10:00:00Z"),
	}

	commands := []RecordedCommand{
		{
			Timestamp: mustParseTime("2024-01-15T10:30:00Z"),
			Argv:      []string{"kubectl", "get", "pods"},
			ExitCode:  0,
			Stdout:    "NAME    READY   STATUS\npod1    1/1     Running\n",
			Stderr:    "",
		},
		{
			Timestamp: mustParseTime("2024-01-15T10:30:05Z"),
			Argv:      []string{"kubectl", "describe", "pod", "pod1"},
			ExitCode:  0,
			Stdout:    "Name: pod1\nNamespace: default\n",
			Stderr:    "",
		},
	}

	scenario, err := ConvertToScenario(meta, commands)
	require.NoError(t, err)

	// Verify scenario metadata
	assert.Equal(t, "kubectl-workflow", scenario.Meta.Name)
	assert.Equal(t, "Testing kubectl commands", scenario.Meta.Description)

	// Verify steps
	require.Len(t, scenario.Steps, 2)

	// Step 1
	assert.Equal(t, []string{"kubectl", "get", "pods"}, scenario.Steps[0].Match.Argv)
	assert.Equal(t, "NAME    READY   STATUS\npod1    1/1     Running\n", scenario.Steps[0].Respond.Stdout)
	assert.Equal(t, "", scenario.Steps[0].Respond.Stderr)
	assert.Equal(t, 0, scenario.Steps[0].Respond.Exit)

	// Step 2
	assert.Equal(t, []string{"kubectl", "describe", "pod", "pod1"}, scenario.Steps[1].Match.Argv)
	assert.Equal(t, "Name: pod1\nNamespace: default\n", scenario.Steps[1].Respond.Stdout)
	assert.Equal(t, 0, scenario.Steps[1].Respond.Exit)
}

func TestConvertToScenario_DuplicateCommands(t *testing.T) {
	meta := SessionMetadata{
		Name:        "duplicate-test",
		Description: "Test duplicate command handling",
		RecordedAt:  time.Now(),
	}

	// Same command executed three times with different outputs
	commands := []RecordedCommand{
		{
			Timestamp: mustParseTime("2024-01-15T10:00:00Z"),
			Argv:      []string{"kubectl", "get", "pods"},
			ExitCode:  0,
			Stdout:    "No pods\n",
			Stderr:    "",
		},
		{
			Timestamp: mustParseTime("2024-01-15T10:00:10Z"),
			Argv:      []string{"kubectl", "get", "pods"},
			ExitCode:  0,
			Stdout:    "pod1\n",
			Stderr:    "",
		},
		{
			Timestamp: mustParseTime("2024-01-15T10:00:20Z"),
			Argv:      []string{"kubectl", "get", "pods"},
			ExitCode:  0,
			Stdout:    "pod1\npod2\n",
			Stderr:    "",
		},
	}

	scenario, err := ConvertToScenario(meta, commands)
	require.NoError(t, err)

	// Should create 3 separate steps per FR-009b
	require.Len(t, scenario.Steps, 3)

	// All should have same argv (duplicate commands allowed)
	assert.Equal(t, []string{"kubectl", "get", "pods"}, scenario.Steps[0].Match.Argv)
	assert.Equal(t, []string{"kubectl", "get", "pods"}, scenario.Steps[1].Match.Argv)
	assert.Equal(t, []string{"kubectl", "get", "pods"}, scenario.Steps[2].Match.Argv)

	// Verify outputs are different
	assert.Equal(t, "No pods\n", scenario.Steps[0].Respond.Stdout)
	assert.Equal(t, "pod1\n", scenario.Steps[1].Respond.Stdout)
	assert.Equal(t, "pod1\npod2\n", scenario.Steps[2].Respond.Stdout)
}

func TestConvertToScenario_NonZeroExitCode(t *testing.T) {
	meta := SessionMetadata{
		Name:        "error-test",
		Description: "Test error handling",
		RecordedAt:  time.Now(),
	}

	commands := []RecordedCommand{
		{
			Timestamp: mustParseTime("2024-01-15T10:00:00Z"),
			Argv:      []string{"kubectl", "get", "pod", "nonexistent"},
			ExitCode:  1,
			Stdout:    "",
			Stderr:    "Error from server (NotFound): pods \"nonexistent\" not found\n",
		},
	}

	scenario, err := ConvertToScenario(meta, commands)
	require.NoError(t, err)

	require.Len(t, scenario.Steps, 1)
	assert.Equal(t, 1, scenario.Steps[0].Respond.Exit)
	assert.Equal(t, "Error from server (NotFound): pods \"nonexistent\" not found\n", scenario.Steps[0].Respond.Stderr)
	assert.Equal(t, "", scenario.Steps[0].Respond.Stdout)
}

func TestConvertToScenario_EmptyCommands(t *testing.T) {
	meta := SessionMetadata{
		Name:        "empty-test",
		Description: "No commands recorded",
		RecordedAt:  time.Now(),
	}

	scenario, err := ConvertToScenario(meta, []RecordedCommand{})
	require.NoError(t, err)

	assert.Equal(t, "empty-test", scenario.Meta.Name)
	assert.Len(t, scenario.Steps, 0)
}

func TestGenerateYAML(t *testing.T) {
	sc := &scenario.Scenario{
		Meta: scenario.Meta{
			Name:        "test-scenario",
			Description: "A test scenario",
		},
		Steps: []scenario.Step{
			{
				Match: scenario.Match{
					Argv: []string{"kubectl", "get", "pods"},
				},
				Respond: scenario.Response{
					Exit:   0,
					Stdout: "NAME    READY\npod1    1/1     Running\n",
					Stderr: "",
				},
			},
		},
	}

	yaml, err := GenerateYAML(sc)
	require.NoError(t, err)

	assert.Contains(t, yaml, "name: test-scenario")
	assert.Contains(t, yaml, "description: A test scenario")
	assert.Contains(t, yaml, "kubectl")
	assert.Contains(t, yaml, "get")
	assert.Contains(t, yaml, "pods")
	assert.Contains(t, yaml, "NAME    READY")
	assert.Contains(t, yaml, "exit: 0")
}

func TestWriteYAMLFile(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := tmpDir + "/output.yaml"

	sc := &scenario.Scenario{
		Meta: scenario.Meta{
			Name:        "file-test",
			Description: "Test file writing",
		},
		Steps: []scenario.Step{
			{
				Match: scenario.Match{
					Argv: []string{"echo", "hello"},
				},
				Respond: scenario.Response{
					Exit:   0,
					Stdout: "hello\n",
					Stderr: "",
				},
			},
		},
	}

	err := WriteYAMLFile(outputPath, sc)
	require.NoError(t, err)

	// Verify file exists
	assert.FileExists(t, outputPath)

	// Verify content
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "name: file-test")
	assert.Contains(t, string(content), "echo")
}

func TestWriteYAMLFile_InvalidPath(t *testing.T) {
	sc := &scenario.Scenario{
		Meta: scenario.Meta{
			Name: "invalid-path-test",
		},
	}

	err := WriteYAMLFile("/nonexistent/directory/output.yaml", sc)
	assert.Error(t, err)
}

// Helper function to parse time from RFC3339 string
func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
