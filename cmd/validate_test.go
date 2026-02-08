package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeValidateRoot creates a fresh root + validate command tree for testing.
func makeValidateRoot() *cobra.Command {
	// Reset global flag state
	validateFormatFlag = "text"

	root := &cobra.Command{
		Use:           "cli-replay",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	v := &cobra.Command{
		Use:  "validate <file>...",
		Args: cobra.MinimumNArgs(1),
		RunE: runValidate,
	}
	v.Flags().StringVar(&validateFormatFlag, "format", "text", "Output format: text, json")
	root.AddCommand(v)
	return root
}

func TestValidate_ValidFile_ExitZero(t *testing.T) {
	root := makeValidateRoot()
	root.SetArgs([]string{"validate", "../testdata/scenarios/validate-valid.yaml"})

	// runValidate calls os.Exit(1) on failure; for valid files it returns nil
	err := root.Execute()
	assert.NoError(t, err, "validate should succeed for valid scenario file")
}

func TestValidate_InvalidFile_Errors(t *testing.T) {
	// We can't easily test os.Exit(1) without subprocess, but we can verify
	// validateFile returns errors for the invalid fixture.
	result := validateFile("../testdata/scenarios/validate-invalid.yaml")

	assert.False(t, result.Valid, "invalid scenario should not be valid")
	assert.NotEmpty(t, result.Errors, "should have error messages")

	// Check that we get the expected error (empty meta.name is the first validation failure)
	foundNameError := false
	for _, e := range result.Errors {
		if contains(e, "name must be non-empty") || contains(e, "name") {
			foundNameError = true
			break
		}
	}
	assert.True(t, foundNameError, "should report empty meta.name error, got: %v", result.Errors)
}

func TestValidate_BadYAML_ParseError(t *testing.T) {
	result := validateFile("../testdata/scenarios/validate-bad-yaml.yaml")

	assert.False(t, result.Valid, "bad YAML should not be valid")
	assert.NotEmpty(t, result.Errors, "should have parse error")
}

func TestValidate_FileNotFound(t *testing.T) {
	result := validateFile("nonexistent-file-xyz.yaml")

	assert.False(t, result.Valid, "nonexistent file should not be valid")
	assert.NotEmpty(t, result.Errors, "should have file-not-found error")
	assert.Contains(t, result.Errors[0], "failed to open scenario file",
		"error should mention file open failure")
}

func TestValidate_MultipleFiles_MixedResults(t *testing.T) {
	validResult := validateFile("../testdata/scenarios/validate-valid.yaml")
	invalidResult := validateFile("../testdata/scenarios/validate-invalid.yaml")

	assert.True(t, validResult.Valid, "valid file should pass")
	assert.False(t, invalidResult.Valid, "invalid file should fail")
}

func TestValidate_FormatJSON_Output(t *testing.T) {
	// Capture stdout for JSON output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	results := []ValidationResult{
		{File: "test.yaml", Valid: true, Errors: []string{}},
		{File: "bad.yaml", Valid: false, Errors: []string{"meta: name must be non-empty"}},
	}

	err := formatValidateJSON(results)

	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	os.Stdout = oldStdout

	require.NoError(t, err)

	// Parse JSON output
	var parsed []ValidationResult
	jsonErr := json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, jsonErr, "output should be valid JSON: %s", buf.String())
	assert.Len(t, parsed, 2)
	assert.True(t, parsed[0].Valid)
	assert.Equal(t, "test.yaml", parsed[0].File)
	assert.False(t, parsed[1].Valid)
	assert.Equal(t, "bad.yaml", parsed[1].File)
	assert.Contains(t, parsed[1].Errors, "meta: name must be non-empty")
}

func TestValidate_FormatJSON_FieldNames(t *testing.T) {
	// Verify JSON uses correct field names: file, valid, errors
	result := ValidationResult{File: "test.yaml", Valid: true, Errors: []string{}}
	data, err := json.Marshal(result)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Contains(t, raw, "file")
	assert.Contains(t, raw, "valid")
	assert.Contains(t, raw, "errors")
}

func TestValidate_FormatInvalid_Error(t *testing.T) {
	root := makeValidateRoot()
	root.SetArgs([]string{"validate", "--format", "yaml", "../testdata/scenarios/validate-valid.yaml"})
	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid format")
	assert.Contains(t, err.Error(), "text, json")
}

func TestValidate_CommandRegistered(t *testing.T) {
	// Verify validate subcommand exists on the real root
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "validate" {
			found = true
			f := cmd.Flags().Lookup("format")
			assert.NotNil(t, f, "--format flag should be registered")
			assert.Equal(t, "text", f.DefValue, "default format should be text")
			break
		}
	}
	assert.True(t, found, "validate subcommand should be registered")
}

// T011: Test stdout_file existence check
func TestValidate_StdoutFile_NonExistent(t *testing.T) {
	// Create a scenario that references a non-existent stdout_file
	tmpDir := t.TempDir()
	scenarioContent := `meta:
  name: stdout-file-test
steps:
  - match:
      argv: [echo, hello]
    respond:
      exit: 0
      stdout_file: "nonexistent-output.txt"
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	require.NoError(t, os.WriteFile(scenarioPath, []byte(scenarioContent), 0644))

	result := validateFile(scenarioPath)
	assert.False(t, result.Valid, "scenario with missing stdout_file should be invalid")

	foundFileError := false
	for _, e := range result.Errors {
		if contains(e, "stdout_file") && contains(e, "not found") {
			foundFileError = true
			break
		}
	}
	assert.True(t, foundFileError, "should report missing stdout_file, got: %v", result.Errors)
}

func TestValidate_StdoutFile_Exists(t *testing.T) {
	// Create a scenario that references an existing stdout_file
	tmpDir := t.TempDir()

	// Create the referenced file
	outputContent := "hello world\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "output.txt"), []byte(outputContent), 0644))

	scenarioContent := `meta:
  name: stdout-file-exists-test
steps:
  - match:
      argv: [echo, hello]
    respond:
      exit: 0
      stdout_file: "output.txt"
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	require.NoError(t, os.WriteFile(scenarioPath, []byte(scenarioContent), 0644))

	result := validateFile(scenarioPath)
	assert.True(t, result.Valid, "scenario with existing stdout_file should be valid, errors: %v", result.Errors)
	assert.Empty(t, result.Errors)
}

func TestValidate_StderrFile_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioContent := `meta:
  name: stderr-file-test
steps:
  - match:
      argv: [echo, hello]
    respond:
      exit: 0
      stderr_file: "missing-errors.txt"
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	require.NoError(t, os.WriteFile(scenarioPath, []byte(scenarioContent), 0644))

	result := validateFile(scenarioPath)
	assert.False(t, result.Valid)

	foundFileError := false
	for _, e := range result.Errors {
		if contains(e, "stderr_file") && contains(e, "not found") {
			foundFileError = true
			break
		}
	}
	assert.True(t, foundFileError, "should report missing stderr_file, got: %v", result.Errors)
}

// contains checks if s contains substr (case-insensitive-friendly helper).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
