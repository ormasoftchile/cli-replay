package runner

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_SingleStepScenario tests the full replay flow for a single-step scenario.
// This is an integration test that simulates what happens when the binary is invoked.
func TestIntegration_SingleStepScenario(t *testing.T) {
	tmpDir := t.TempDir()

	// Create scenario file
	scenarioContent := `
meta:
  name: "integration-single-step"
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: |
        NAME    READY   STATUS
        web-0   1/1     Running
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// Test the replay logic directly
	var stdout, stderr bytes.Buffer

	// Simulate the replay flow
	result, err := ExecuteReplay(scenarioPath, []string{"kubectl", "get", "pods"}, &stdout, &stderr)
	require.NoError(t, err)

	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, stdout.String(), "web-0")
	assert.Contains(t, stdout.String(), "Running")
}

// TestIntegration_SingleStepMismatch tests that mismatched argv returns an error.
func TestIntegration_SingleStepMismatch(t *testing.T) {
	tmpDir := t.TempDir()

	// Create scenario file
	scenarioContent := `
meta:
  name: "integration-mismatch"
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: "pods"
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	// Send wrong command
	result, err := ExecuteReplay(scenarioPath, []string{"kubectl", "get", "services"}, &stdout, &stderr)

	// Should return error
	require.Error(t, err)
	assert.NotEqual(t, 0, result.ExitCode)
}

// TestIntegration_StdoutFile tests loading response content from file.
func TestIntegration_StdoutFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create fixture file
	fixtureDir := filepath.Join(tmpDir, "fixtures")
	require.NoError(t, os.MkdirAll(fixtureDir, 0750))

	fixtureContent := `NAME    READY   STATUS    AGE
pod-1   1/1     Running   5m
pod-2   1/1     Running   3m
`
	fixtureFile := filepath.Join(fixtureDir, "pods.txt")
	err := os.WriteFile(fixtureFile, []byte(fixtureContent), 0600)
	require.NoError(t, err)

	// Create scenario file
	scenarioContent := `
meta:
  name: "file-test"
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout_file: fixtures/pods.txt
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err = os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	result, err := ExecuteReplay(scenarioPath, []string{"kubectl", "get", "pods"}, &stdout, &stderr)
	require.NoError(t, err)

	assert.Equal(t, 0, result.ExitCode)
	assert.Equal(t, fixtureContent, stdout.String())
}

// TestIntegration_ErrorExitCode tests that error exit codes are returned correctly.
func TestIntegration_ErrorExitCode(t *testing.T) {
	tmpDir := t.TempDir()

	scenarioContent := `
meta:
  name: "error-test"
steps:
  - match:
      argv: ["kubectl", "delete", "pod", "nonexistent"]
    respond:
      exit: 1
      stderr: "Error from server (NotFound): pods \"nonexistent\" not found"
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer

	result, err := ExecuteReplay(scenarioPath, []string{"kubectl", "delete", "pod", "nonexistent"}, &stdout, &stderr)
	require.NoError(t, err)

	assert.Equal(t, 1, result.ExitCode)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "NotFound")
}

// skipIfNoGo skips the test if go is not available (for exec tests).
func skipIfNoGo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available, skipping exec-based test")
	}
}

// TestIntegration_InterceptMode_EndToEnd is a full end-to-end test that builds
// the binary and tests symlink-based interception.
// This test is skipped if go is not available.
func TestIntegration_InterceptMode_EndToEnd(t *testing.T) {
	skipIfNoGo(t)

	// This test would build the binary and test symlink interception
	// For now, we test the logic directly above
	t.Skip("Full binary test - requires built binary")
}

// T027: Integration test for multi-step scenario in order
func TestIntegration_MultiStepInOrder(t *testing.T) {
	tmpDir := t.TempDir()

	scenarioContent := `
meta:
  name: "multi-step-ordered"
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: "pod-list"
  - match:
      argv: ["kubectl", "rollout", "restart", "deployment/web"]
    respond:
      exit: 0
      stdout: "deployment restarted"
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
      stdout: "pod-list-healthy"
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// Step 1
	var stdout1, stderr1 bytes.Buffer
	result1, err := ExecuteReplay(scenarioPath, []string{"kubectl", "get", "pods"}, &stdout1, &stderr1)
	require.NoError(t, err)
	assert.Equal(t, 0, result1.ExitCode)
	assert.Contains(t, stdout1.String(), "pod-list")

	// Step 2
	var stdout2, stderr2 bytes.Buffer
	result2, err := ExecuteReplay(scenarioPath, []string{"kubectl", "rollout", "restart", "deployment/web"}, &stdout2, &stderr2)
	require.NoError(t, err)
	assert.Equal(t, 0, result2.ExitCode)
	assert.Contains(t, stdout2.String(), "deployment restarted")

	// Step 3
	var stdout3, stderr3 bytes.Buffer
	result3, err := ExecuteReplay(scenarioPath, []string{"kubectl", "get", "pods"}, &stdout3, &stderr3)
	require.NoError(t, err)
	assert.Equal(t, 0, result3.ExitCode)
	assert.Contains(t, stdout3.String(), "pod-list-healthy")
}

// T028: Integration test for out-of-order rejection
func TestIntegration_OutOfOrderRejection(t *testing.T) {
	tmpDir := t.TempDir()

	scenarioContent := `
meta:
  name: "out-of-order-test"
steps:
  - match:
      argv: ["cmd", "first"]
    respond:
      exit: 0
      stdout: "first"
  - match:
      argv: ["cmd", "second"]
    respond:
      exit: 0
      stdout: "second"
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// Try to call second step first - should fail
	var stdout, stderr bytes.Buffer
	result, err := ExecuteReplay(scenarioPath, []string{"cmd", "second"}, &stdout, &stderr)

	// Should return error
	require.Error(t, err)
	assert.NotEqual(t, 0, result.ExitCode)

	// Check it's a mismatch error
	_, ok := err.(*MismatchError)
	assert.True(t, ok, "expected MismatchError")
}

// TestIntegration_ScenarioComplete tests that completed scenarios reject further commands
func TestIntegration_ScenarioComplete(t *testing.T) {
	tmpDir := t.TempDir()

	scenarioContent := `
meta:
  name: "complete-test"
steps:
  - match:
      argv: ["cmd", "only"]
    respond:
      exit: 0
      stdout: "done"
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	err := os.WriteFile(scenarioPath, []byte(scenarioContent), 0600)
	require.NoError(t, err)

	// Execute the only step
	var stdout1, stderr1 bytes.Buffer
	result1, err := ExecuteReplay(scenarioPath, []string{"cmd", "only"}, &stdout1, &stderr1)
	require.NoError(t, err)
	assert.Equal(t, 0, result1.ExitCode)

	// Try to execute again - should fail
	var stdout2, stderr2 bytes.Buffer
	result2, err := ExecuteReplay(scenarioPath, []string{"cmd", "only"}, &stdout2, &stderr2)

	require.Error(t, err)
	assert.NotEqual(t, 0, result2.ExitCode)
	assert.Contains(t, stderr2.String(), "complete")
}

// T020: Integration test — capture chain end-to-end.
// Load capture_chain.yaml, simulate 3-step replay, assert step 3 stdout
// contains captured values from steps 1 and 2.
func TestIntegration_CaptureChain(t *testing.T) {
	tmpDir := t.TempDir()

	scenarioContent := `
meta:
  name: capture-chain
steps:
  - match:
      argv: ["az", "group", "create", "--name", "demo-rg"]
    respond:
      exit: 0
      stdout: '{"id": "/subscriptions/abc/resourceGroups/demo-rg", "name": "demo-rg"}'
      capture:
        rg_id: "/subscriptions/abc/resourceGroups/demo-rg"

  - match:
      argv: ["az", "vm", "create", "--resource-group", "demo-rg"]
    respond:
      exit: 0
      stdout: '{"id": "/subscriptions/abc/vms/vm-1", "name": "vm-1"}'
      capture:
        vm_id: "/subscriptions/abc/vms/vm-1"

  - match:
      argv: ["az", "vm", "show", "--ids"]
    respond:
      exit: 0
      stdout: 'rg={{ .capture.rg_id }} vm={{ .capture.vm_id }}'
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	require.NoError(t, os.WriteFile(scenarioPath, []byte(scenarioContent), 0600))

	// Step 1: az group create → captures rg_id
	var stdout1, stderr1 bytes.Buffer
	result1, err := ExecuteReplay(scenarioPath, []string{"az", "group", "create", "--name", "demo-rg"}, &stdout1, &stderr1)
	require.NoError(t, err, "step 1 err: %s", stderr1.String())
	assert.Equal(t, 0, result1.ExitCode)
	assert.Contains(t, stdout1.String(), "demo-rg")

	// Step 2: az vm create → captures vm_id
	var stdout2, stderr2 bytes.Buffer
	result2, err := ExecuteReplay(scenarioPath, []string{"az", "vm", "create", "--resource-group", "demo-rg"}, &stdout2, &stderr2)
	require.NoError(t, err, "step 2 err: %s", stderr2.String())
	assert.Equal(t, 0, result2.ExitCode)
	assert.Contains(t, stdout2.String(), "vm-1")

	// Step 3: az vm show → template uses captured values
	var stdout3, stderr3 bytes.Buffer
	result3, err := ExecuteReplay(scenarioPath, []string{"az", "vm", "show", "--ids"}, &stdout3, &stderr3)
	require.NoError(t, err, "step 3 err: %s", stderr3.String())
	assert.Equal(t, 0, result3.ExitCode)
	assert.Equal(t, "rg=/subscriptions/abc/resourceGroups/demo-rg vm=/subscriptions/abc/vms/vm-1", stdout3.String())
}

// T021: Integration test — capture within unordered groups.
// The group has two steps; the second references a capture from the first.
// When pods step executes first, svc step can see its capture.
// When base_id from ordered step 0 is referenced, it should resolve.
func TestIntegration_CaptureGroup(t *testing.T) {
	tmpDir := t.TempDir()

	scenarioContent := `
meta:
  name: capture-group
steps:
  - match:
      argv: ["setup"]
    respond:
      exit: 0
      stdout: "ready"
      capture:
        base_id: "base-123"

  - group:
      mode: unordered
      name: monitoring
      steps:
        - match:
            argv: ["kubectl", "get", "pods"]
          respond:
            exit: 0
            stdout: "web-pod-1"
            capture:
              first_pod: "web-pod-1"

        - match:
            argv: ["kubectl", "get", "svc"]
          respond:
            exit: 0
            stdout: 'svc for {{ .capture.first_pod }} base={{ .capture.base_id }}'

  - match:
      argv: ["teardown"]
    respond:
      exit: 0
      stdout: 'done base={{ .capture.base_id }}'
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	require.NoError(t, os.WriteFile(scenarioPath, []byte(scenarioContent), 0600))

	// Step 0: setup → captures base_id
	var stdout0, stderr0 bytes.Buffer
	result0, err := ExecuteReplay(scenarioPath, []string{"setup"}, &stdout0, &stderr0)
	require.NoError(t, err, "setup err: %s", stderr0.String())
	assert.Equal(t, 0, result0.ExitCode)
	assert.Equal(t, "ready", stdout0.String())

	// Group step: kubectl get pods → captures first_pod
	var stdout1, stderr1 bytes.Buffer
	result1, err := ExecuteReplay(scenarioPath, []string{"kubectl", "get", "pods"}, &stdout1, &stderr1)
	require.NoError(t, err, "pods err: %s", stderr1.String())
	assert.Equal(t, 0, result1.ExitCode)
	assert.Equal(t, "web-pod-1", stdout1.String())

	// Group step: kubectl get svc → references first_pod + base_id
	var stdout2, stderr2 bytes.Buffer
	result2, err := ExecuteReplay(scenarioPath, []string{"kubectl", "get", "svc"}, &stdout2, &stderr2)
	require.NoError(t, err, "svc err: %s", stderr2.String())
	assert.Equal(t, 0, result2.ExitCode)
	assert.Equal(t, "svc for web-pod-1 base=base-123", stdout2.String())

	// Step after group: teardown → references base_id
	var stdout3, stderr3 bytes.Buffer
	result3, err := ExecuteReplay(scenarioPath, []string{"teardown"}, &stdout3, &stderr3)
	require.NoError(t, err, "teardown err: %s", stderr3.String())
	assert.Equal(t, 0, result3.ExitCode)
	assert.Equal(t, "done base=base-123", stdout3.String())
}

// Test capture within group when sibling capture is not yet available (best-effort).
// Execute svc before pods — first_pod hasn't been captured yet, resolves to empty.
func TestIntegration_CaptureGroup_BestEffort(t *testing.T) {
	tmpDir := t.TempDir()

	scenarioContent := `
meta:
  name: capture-group-besteffort
steps:
  - match:
      argv: ["setup"]
    respond:
      exit: 0
      stdout: "ready"
      capture:
        base_id: "base-123"

  - group:
      mode: unordered
      name: monitoring
      steps:
        - match:
            argv: ["kubectl", "get", "pods"]
          respond:
            exit: 0
            stdout: "web-pod-1"
            capture:
              first_pod: "web-pod-1"

        - match:
            argv: ["kubectl", "get", "svc"]
          respond:
            exit: 0
            stdout: 'svc for [{{ .capture.first_pod }}] base={{ .capture.base_id }}'
`
	scenarioPath := filepath.Join(tmpDir, "scenario.yaml")
	require.NoError(t, os.WriteFile(scenarioPath, []byte(scenarioContent), 0600))

	// Step 0: setup → captures base_id
	var stdout0, stderr0 bytes.Buffer
	_, err := ExecuteReplay(scenarioPath, []string{"setup"}, &stdout0, &stderr0)
	require.NoError(t, err)

	// Execute svc BEFORE pods — first_pod not yet captured, should resolve to empty
	var stdout1, stderr1 bytes.Buffer
	result1, err := ExecuteReplay(scenarioPath, []string{"kubectl", "get", "svc"}, &stdout1, &stderr1)
	require.NoError(t, err, "svc err: %s", stderr1.String())
	assert.Equal(t, 0, result1.ExitCode)
	// first_pod is empty (not captured yet), base_id is available from step 0
	assert.Equal(t, "svc for [] base=base-123", stdout1.String())
}
