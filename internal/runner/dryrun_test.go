package runner

import (
	"bytes"
	"testing"

	"github.com/cli-replay/cli-replay/internal/scenario"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDryRunReport_LinearScenario(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{
			Name:        "linear-test",
			Description: "A simple linear test",
			Vars:        map[string]string{"region": "eastus"},
		},
		Steps: []scenario.StepElement{
			{Step: &scenario.Step{
				Match:   scenario.Match{Argv: []string{"az", "group", "create"}},
				Respond: scenario.Response{Exit: 0, Stdout: "created"},
			}},
			{Step: &scenario.Step{
				Match:   scenario.Match{Argv: []string{"az", "vm", "create"}},
				Respond: scenario.Response{Exit: 0, Stdout: "provisioned"},
			}},
		},
	}

	report := BuildDryRunReport(scn)

	assert.Equal(t, "linear-test", report.ScenarioName)
	assert.Equal(t, "A simple linear test", report.Description)
	assert.Equal(t, 2, report.TotalSteps)
	assert.Empty(t, report.Groups)
	require.Len(t, report.Steps, 2)
	assert.Equal(t, "az group create", report.Steps[0].MatchArgv)
	assert.Equal(t, "az vm create", report.Steps[1].MatchArgv)
	assert.Equal(t, 0, report.Steps[0].Exit)
	assert.Equal(t, "created", report.Steps[0].StdoutPreview)
	assert.Contains(t, report.TemplateVars, "region")
}

func TestBuildDryRunReport_GroupedScenario(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{Name: "grouped-test"},
		Steps: []scenario.StepElement{
			{Step: &scenario.Step{
				Match:   scenario.Match{Argv: []string{"setup"}},
				Respond: scenario.Response{Exit: 0, Stdout: "ready"},
			}},
			{Group: &scenario.StepGroup{
				Mode: "unordered",
				Name: "monitoring",
				Steps: []scenario.StepElement{
					{Step: &scenario.Step{
						Match:   scenario.Match{Argv: []string{"kubectl", "get", "pods"}},
						Respond: scenario.Response{Exit: 0, Stdout: "pod-1"},
					}},
					{Step: &scenario.Step{
						Match:   scenario.Match{Argv: []string{"kubectl", "get", "svc"}},
						Respond: scenario.Response{Exit: 0, Stdout: "svc-1"},
					}},
				},
			}},
		},
	}

	report := BuildDryRunReport(scn)

	assert.Equal(t, 3, report.TotalSteps)
	require.Len(t, report.Groups, 1)
	require.Len(t, report.Steps, 3)

	// Step 0 is plain
	assert.Empty(t, report.Steps[0].GroupName)

	// Steps 1 and 2 are in the group
	assert.Equal(t, "monitoring", report.Steps[1].GroupName)
	assert.Equal(t, "unordered", report.Steps[1].GroupMode)
	assert.Equal(t, "monitoring", report.Steps[2].GroupName)
}

func TestBuildDryRunReport_WithCaptures(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{Name: "capture-test"},
		Steps: []scenario.StepElement{
			{Step: &scenario.Step{
				Match: scenario.Match{Argv: []string{"cmd"}},
				Respond: scenario.Response{
					Exit:    0,
					Stdout:  "result",
					Capture: map[string]string{"rg_id": "rg-1", "vm_id": "vm-1"},
				},
			}},
		},
	}

	report := BuildDryRunReport(scn)

	require.Len(t, report.Steps, 1)
	assert.Len(t, report.Steps[0].Captures, 2)
	assert.Contains(t, report.Steps[0].Captures, "rg_id")
	assert.Contains(t, report.Steps[0].Captures, "vm_id")
}

func TestBuildDryRunReport_WithAllowlist(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{
			Name: "allowlist-test",
			Security: &scenario.Security{
				AllowedCommands: []string{"kubectl", "helm"},
			},
		},
		Steps: []scenario.StepElement{
			{Step: &scenario.Step{
				Match:   scenario.Match{Argv: []string{"kubectl", "get", "pods"}},
				Respond: scenario.Response{Exit: 0},
			}},
			{Step: &scenario.Step{
				Match:   scenario.Match{Argv: []string{"az", "group", "create"}},
				Respond: scenario.Response{Exit: 0},
			}},
		},
	}

	report := BuildDryRunReport(scn)

	assert.Equal(t, []string{"kubectl", "helm"}, report.Allowlist)
	require.Len(t, report.AllowlistIssues, 1)
	assert.Contains(t, report.AllowlistIssues[0], "az")
}

func TestBuildDryRunReport_StdoutFilePreview(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{Name: "file-test"},
		Steps: []scenario.StepElement{
			{Step: &scenario.Step{
				Match:   scenario.Match{Argv: []string{"cmd"}},
				Respond: scenario.Response{Exit: 0, StdoutFile: "fixtures/output.txt"},
			}},
		},
	}

	report := BuildDryRunReport(scn)

	assert.Equal(t, "[file: fixtures/output.txt]", report.Steps[0].StdoutPreview)
}

func TestBuildDryRunReport_SessionTTL(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{
			Name:    "ttl-test",
			Session: &scenario.Session{TTL: "30m"},
		},
		Steps: []scenario.StepElement{
			{Step: &scenario.Step{
				Match:   scenario.Match{Argv: []string{"cmd"}},
				Respond: scenario.Response{Exit: 0},
			}},
		},
	}

	report := BuildDryRunReport(scn)

	assert.Equal(t, "30m", report.SessionTTL)
}

// T027: FormatDryRunReport tests

func TestFormatDryRunReport_ContainsNumberedSteps(t *testing.T) {
	report := &DryRunReport{
		ScenarioName: "test-scenario",
		TotalSteps:   2,
		Commands:     []string{"kubectl"},
		Steps: []DryRunStep{
			{Index: 0, MatchArgv: "kubectl get pods", Exit: 0, StdoutPreview: "pod-1", CallsMin: 1, CallsMax: 1},
			{Index: 1, MatchArgv: "kubectl delete pod pod-1", Exit: 0, CallsMin: 1, CallsMax: 1},
		},
	}

	var buf bytes.Buffer
	err := FormatDryRunReport(report, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Scenario: test-scenario")
	assert.Contains(t, output, "kubectl get pods")
	assert.Contains(t, output, "kubectl delete pod pod-1")
	assert.Contains(t, output, "1 ")  // step number 1
	assert.Contains(t, output, "2 ")  // step number 2
	assert.Contains(t, output, "exit 0")
}

func TestFormatDryRunReport_GroupIndicators(t *testing.T) {
	report := &DryRunReport{
		ScenarioName: "group-test",
		TotalSteps:   2,
		Commands:     []string{"kubectl"},
		Groups:       []scenario.GroupRange{{Start: 0, End: 2, Name: "monitoring"}},
		Steps: []DryRunStep{
			{Index: 0, MatchArgv: "kubectl get pods", Exit: 0, CallsMin: 1, CallsMax: 0, GroupName: "monitoring", GroupMode: "unordered"},
			{Index: 1, MatchArgv: "kubectl get svc", Exit: 0, CallsMin: 1, CallsMax: 0, GroupName: "monitoring", GroupMode: "unordered"},
		},
	}

	var buf bytes.Buffer
	err := FormatDryRunReport(report, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "monitoring (unordered)")
	assert.Contains(t, output, "Groups: 1")
}

func TestFormatDryRunReport_TemplateVars(t *testing.T) {
	report := &DryRunReport{
		ScenarioName: "var-test",
		TotalSteps:   1,
		TemplateVars: []string{"cluster", "namespace"},
		Steps: []DryRunStep{
			{Index: 0, MatchArgv: "cmd", Exit: 0, CallsMin: 1, CallsMax: 1},
		},
	}

	var buf bytes.Buffer
	err := FormatDryRunReport(report, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Template Variables:")
	assert.Contains(t, output, "cluster")
	assert.Contains(t, output, "namespace")
}

func TestFormatDryRunReport_AllowlistPass(t *testing.T) {
	report := &DryRunReport{
		ScenarioName: "allow-test",
		TotalSteps:   1,
		Commands:     []string{"kubectl"},
		Allowlist:    []string{"kubectl"},
		Steps: []DryRunStep{
			{Index: 0, MatchArgv: "kubectl get pods", Exit: 0, CallsMin: 1, CallsMax: 1},
		},
	}

	var buf bytes.Buffer
	err := FormatDryRunReport(report, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "All commands match allowlist")
}

func TestFormatDryRunReport_AllowlistFail(t *testing.T) {
	report := &DryRunReport{
		ScenarioName:    "allow-fail",
		TotalSteps:      1,
		Commands:        []string{"az"},
		Allowlist:       []string{"kubectl"},
		AllowlistIssues: []string{`command "az" not in allowlist`},
		Steps: []DryRunStep{
			{Index: 0, MatchArgv: "az group create", Exit: 0, CallsMin: 1, CallsMax: 1},
		},
	}

	var buf bytes.Buffer
	err := FormatDryRunReport(report, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, `"az" not in allowlist`)
}

func TestFormatDryRunReport_CapturesShown(t *testing.T) {
	report := &DryRunReport{
		ScenarioName: "capture-test",
		TotalSteps:   1,
		Commands:     []string{"cmd"},
		Steps: []DryRunStep{
			{Index: 0, MatchArgv: "cmd", Exit: 0, CallsMin: 1, CallsMax: 1, Captures: []string{"rg_id", "vm_id"}},
		},
	}

	var buf bytes.Buffer
	err := FormatDryRunReport(report, &buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "captures:")
	assert.Contains(t, output, "rg_id")
	assert.Contains(t, output, "vm_id")
}
