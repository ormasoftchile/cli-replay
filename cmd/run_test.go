package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cli-replay/cli-replay/internal/scenario"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAllowedCommands(t *testing.T) {
	// Empty string returns nil
	assert.Nil(t, parseAllowedCommands(""))

	// Single command
	assert.Equal(t, []string{"kubectl"}, parseAllowedCommands("kubectl"))

	// Multiple commands
	assert.Equal(t, []string{"kubectl", "az", "docker"}, parseAllowedCommands("kubectl,az,docker"))

	// With spaces
	assert.Equal(t, []string{"kubectl", "az"}, parseAllowedCommands("kubectl , az"))

	// Trailing comma ignored
	assert.Equal(t, []string{"kubectl"}, parseAllowedCommands("kubectl,"))
}

func TestEffectiveAllowlist(t *testing.T) {
	// Both empty → nil
	assert.Nil(t, effectiveAllowlist(nil, nil))
	assert.Nil(t, effectiveAllowlist([]string{}, []string{}))

	// Only YAML set
	assert.Equal(t, []string{"kubectl", "az"}, effectiveAllowlist([]string{"kubectl", "az"}, nil))

	// Only CLI set
	assert.Equal(t, []string{"docker"}, effectiveAllowlist(nil, []string{"docker"}))

	// Both set → intersection
	result := effectiveAllowlist([]string{"kubectl", "az", "docker"}, []string{"kubectl", "docker", "helm"})
	assert.Equal(t, []string{"kubectl", "docker"}, result)

	// Both set, no overlap → empty (not nil)
	result = effectiveAllowlist([]string{"kubectl"}, []string{"docker"})
	assert.Empty(t, result)
}

func TestValidateAllowlist_AllAllowed(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{Name: "test"},
		Steps: []scenario.StepElement{
			{Step: &scenario.Step{Match: scenario.Match{Argv: []string{"kubectl", "get", "pods"}}}},
			{Step: &scenario.Step{Match: scenario.Match{Argv: []string{"kubectl", "apply", "-f", "-"}}}},
		},
	}

	err := validateAllowlist(scn, []string{"kubectl"}, nil)
	require.NoError(t, err)
}

func TestValidateAllowlist_DisallowedRejected(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{Name: "deploy-test"},
		Steps: []scenario.StepElement{
			{Step: &scenario.Step{Match: scenario.Match{Argv: []string{"kubectl", "get", "pods"}}}},
			{Step: &scenario.Step{Match: scenario.Match{Argv: []string{"docker", "build", "-t", "myapp"}}}},
		},
	}

	err := validateAllowlist(scn, []string{"kubectl"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `command "docker" is not in the allowed commands list`)
	assert.Contains(t, err.Error(), "deploy-test")
	assert.Contains(t, err.Error(), "Step 2")
}

func TestValidateAllowlist_PathBasedArgv0(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{Name: "test"},
		Steps: []scenario.StepElement{
			{Step: &scenario.Step{Match: scenario.Match{Argv: []string{"/usr/local/bin/kubectl", "get", "pods"}}}},
		},
	}

	err := validateAllowlist(scn, []string{"kubectl"}, nil)
	require.NoError(t, err)
}

func TestValidateAllowlist_EmptyListAllowsAll(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{Name: "test"},
		Steps: []scenario.StepElement{
			{Step: &scenario.Step{Match: scenario.Match{Argv: []string{"any-command"}}}},
		},
	}

	// Both empty → no restrictions
	err := validateAllowlist(scn, nil, nil)
	require.NoError(t, err)

	err = validateAllowlist(scn, []string{}, []string{})
	require.NoError(t, err)
}

func TestValidateAllowlist_IntersectionLogic(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{Name: "test"},
		Steps: []scenario.StepElement{
			{Step: &scenario.Step{Match: scenario.Match{Argv: []string{"kubectl", "get", "pods"}}}},
		},
	}

	// kubectl is in both lists → allowed
	err := validateAllowlist(scn, []string{"kubectl", "docker"}, []string{"kubectl", "helm"})
	require.NoError(t, err)

	// kubectl is only in YAML but not in CLI intersection → rejected
	scnDocker := &scenario.Scenario{
		Meta: scenario.Meta{Name: "test"},
		Steps: []scenario.StepElement{
			{Step: &scenario.Step{Match: scenario.Match{Argv: []string{"docker", "build"}}}},
		},
	}
	err = validateAllowlist(scnDocker, []string{"kubectl", "docker"}, []string{"kubectl", "helm"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `command "docker" is not in the allowed commands list`)
}

func TestValidateAllowlist_WindowsCaseInsensitive(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}

	scn := &scenario.Scenario{
		Meta: scenario.Meta{Name: "test"},
		Steps: []scenario.StepElement{
			{Step: &scenario.Step{Match: scenario.Match{Argv: []string{"Kubectl.exe", "get", "pods"}}}},
		},
	}

	err := validateAllowlist(scn, []string{"kubectl.exe"}, nil)
	require.NoError(t, err)
}

// --- T024: Trap emission test for bash/zsh/sh ---

func TestEmitShellSetup_BashTrapEmission(t *testing.T) {
	var buf bytes.Buffer
	writeShellSetup(&buf, "bash", "/tmp/intercept-dir", "/path/to/scenario.yaml", "session-abc123")
	output := buf.String()

	// Should contain export statements
	assert.Contains(t, output, "export CLI_REPLAY_SESSION='session-abc123'")
	assert.Contains(t, output, "export CLI_REPLAY_SCENARIO=")
	assert.Contains(t, output, "export PATH=")

	// Should contain cleanup function
	assert.Contains(t, output, "_cli_replay_clean()")
	assert.Contains(t, output, "_cli_replay_cleaned")
	assert.Contains(t, output, "command cli-replay clean")

	// Should contain trap statement
	assert.Contains(t, output, "trap '_cli_replay_clean' EXIT INT TERM")
}

func TestEmitShellSetup_DefaultShellTrapEmission(t *testing.T) {
	// Default shell (empty string mapped to bash-like) should also emit traps
	var buf bytes.Buffer
	writeShellSetup(&buf, "", "/tmp/intercept", "/scenario.yaml", "s123")
	output := buf.String()

	assert.Contains(t, output, "_cli_replay_clean()")
	assert.Contains(t, output, "trap '_cli_replay_clean' EXIT INT TERM")
}

func TestEmitShellSetup_TrapGuardVariable(t *testing.T) {
	var buf bytes.Buffer
	writeShellSetup(&buf, "bash", "/tmp/int", "/s.yaml", "s1")
	output := buf.String()

	// Guard variable prevents double-fire
	assert.Contains(t, output, "${_cli_replay_cleaned:-}")
	assert.Contains(t, output, "_cli_replay_cleaned=1")
}

func TestEmitShellSetup_TrapUsesCommandPrefix(t *testing.T) {
	var buf bytes.Buffer
	writeShellSetup(&buf, "bash", "/tmp/int", "/s.yaml", "s1")
	output := buf.String()

	// 'command' prefix bypasses intercept shims
	assert.Contains(t, output, "command cli-replay clean")
}

func TestEmitShellSetup_TrapRedirectsStderr(t *testing.T) {
	var buf bytes.Buffer
	writeShellSetup(&buf, "bash", "/tmp/int", "/s.yaml", "s1")
	output := buf.String()

	// stderr suppressed in trap
	assert.Contains(t, output, "2>/dev/null")
}

// --- T025: Trap emission test for PowerShell ---

func TestEmitShellSetup_PowerShellNoTrap(t *testing.T) {
	var buf bytes.Buffer
	writeShellSetup(&buf, "powershell", "/tmp/intercept", "/scenario.yaml", "session-xyz")
	output := buf.String()

	// PowerShell should have env var exports
	assert.Contains(t, output, "$env:CLI_REPLAY_SESSION")
	assert.Contains(t, output, "$env:CLI_REPLAY_SCENARIO")
	assert.Contains(t, output, "$env:PATH")

	// PowerShell should NOT have bash-style trap
	assert.NotContains(t, output, "trap ")
	assert.NotContains(t, output, "_cli_replay_clean")
}

// --- T026: Trap emission test for cmd.exe ---

func TestEmitShellSetup_CmdNoTrap(t *testing.T) {
	var buf bytes.Buffer
	writeShellSetup(&buf, "cmd", "/tmp/intercept", "/scenario.yaml", "session-xyz")
	output := buf.String()

	// cmd should have set statements
	assert.Contains(t, output, "set \"CLI_REPLAY_SESSION=")
	assert.Contains(t, output, "set \"CLI_REPLAY_SCENARIO=")
	assert.Contains(t, output, "set \"PATH=")

	// cmd should NOT have bash-style trap
	assert.NotContains(t, output, "trap ")
	assert.NotContains(t, output, "_cli_replay_clean")
}

// T033: Dry-run tests for `run` command

func TestRunDryRun_ValidScenario(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioContent := `
meta:
  name: "dry-run-test"
  description: "Test scenario for dry-run"
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: "pod-1 Running"
  - match:
      argv: ["kubectl", "delete", "pod", "pod-1"]
    respond:
      exit: 1
      stderr: "pod deleted"
`
	scenarioPath := writeScenarioFile(t, tmpDir, scenarioContent)

	// Execute the run command with --dry-run
	rootCmd.SetArgs([]string{"run", "--dry-run", scenarioPath})
	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	err := rootCmd.Execute()
	// Reset for other tests
	runDryRunFlag = false

	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Scenario: dry-run-test")
	assert.Contains(t, output, "kubectl get pods")
	assert.Contains(t, output, "kubectl delete pod pod-1")
	assert.Contains(t, output, "Steps: 2")

	// Verify no state files or intercept dirs created
	assertNoSideEffects(t, tmpDir)
}

func TestRunDryRun_InvalidScenario(t *testing.T) {
	tmpDir := t.TempDir()
	scenarioContent := `
meta:
  name: ""
steps: []
`
	scenarioPath := writeScenarioFile(t, tmpDir, scenarioContent)

	rootCmd.SetArgs([]string{"run", "--dry-run", scenarioPath})
	var stderr bytes.Buffer
	rootCmd.SetErr(&stderr)

	err := rootCmd.Execute()
	runDryRunFlag = false

	require.Error(t, err)
}

// writeScenarioFile writes YAML content to a temp scenario file and returns the path.
func writeScenarioFile(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "scenario.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	return path
}

// assertNoSideEffects verifies no .cli-replay directory or state files were created.
func assertNoSideEffects(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // dir doesn't exist = no side effects
	}
	for _, e := range entries {
		assert.NotEqual(t, ".cli-replay", e.Name(), "dry-run should not create .cli-replay directory")
	}
}
