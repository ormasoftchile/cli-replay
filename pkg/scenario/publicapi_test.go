package scenario

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Public API surface tests for pkg/scenario — consumer-perspective tests
// documenting the contract that gert (and other consumers) will depend on.
// ═══════════════════════════════════════════════════════════════════════════════

// --- ValidateDelay -----------------------------------------------------------

func TestValidateDelay_Table(t *testing.T) {
	tests := []struct {
		name        string
		delay       string
		maxDelay    time.Duration
		wantErr     bool
		errContains string
	}{
		{
			name:     "no delay set",
			delay:    "",
			maxDelay: 5 * time.Second,
			wantErr:  false,
		},
		{
			name:     "zero max disables cap",
			delay:    "10h",
			maxDelay: 0,
			wantErr:  false,
		},
		{
			name:     "delay within limit",
			delay:    "2s",
			maxDelay: 5 * time.Second,
			wantErr:  false,
		},
		{
			name:     "delay equals max exactly",
			delay:    "5s",
			maxDelay: 5 * time.Second,
			wantErr:  false,
		},
		{
			name:        "delay exceeds max",
			delay:       "10s",
			maxDelay:    5 * time.Second,
			wantErr:     true,
			errContains: "exceeds max-delay",
		},
		{
			name:        "invalid duration string",
			delay:       "notaduration",
			maxDelay:    5 * time.Second,
			wantErr:     true,
			errContains: "invalid delay",
		},
		{
			name:     "sub-millisecond delay",
			delay:    "500us",
			maxDelay: 1 * time.Second,
			wantErr:  false,
		},
		{
			name:        "negative delay string",
			delay:       "-1s",
			maxDelay:    5 * time.Second,
			wantErr:     false, // ParseDuration accepts it; -1s < 5s
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Response{Delay: tt.delay}
			err := r.ValidateDelay(tt.maxDelay)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// --- CallBounds validation ---------------------------------------------------

func TestCallBounds_Validate_Table(t *testing.T) {
	tests := []struct {
		name        string
		bounds      CallBounds
		wantErr     bool
		errContains string
	}{
		{"valid min=1 max=1", CallBounds{Min: 1, Max: 1}, false, ""},
		{"valid min=0 max=5", CallBounds{Min: 0, Max: 5}, false, ""},
		{"valid min=2 max=10", CallBounds{Min: 2, Max: 10}, false, ""},
		{"min greater than max", CallBounds{Min: 5, Max: 3}, true, "min (5) must be <= max (3)"},
		{"max zero", CallBounds{Min: 0, Max: 0}, true, "max must be >= 1"},
		{"negative min", CallBounds{Min: -1, Max: 5}, true, "min must be >= 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.bounds.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// --- EffectiveCalls defaults -------------------------------------------------

func TestEffectiveCalls_Defaults(t *testing.T) {
	t.Run("nil calls defaults to min=1 max=1", func(t *testing.T) {
		step := Step{
			Match:   Match{Argv: []string{"cmd"}},
			Respond: Response{Exit: 0},
		}
		bounds := step.EffectiveCalls()
		assert.Equal(t, 1, bounds.Min)
		assert.Equal(t, 1, bounds.Max)
	})

	t.Run("explicit calls preserved", func(t *testing.T) {
		step := Step{
			Match:   Match{Argv: []string{"cmd"}},
			Respond: Response{Exit: 0},
			Calls:   &CallBounds{Min: 0, Max: 5},
		}
		bounds := step.EffectiveCalls()
		assert.Equal(t, 0, bounds.Min)
		assert.Equal(t, 5, bounds.Max)
	})
}

// --- FlatSteps ---------------------------------------------------------------

func TestFlatSteps_MixedStepsAndGroups(t *testing.T) {
	scn := Scenario{
		Meta: Meta{Name: "flat-test"},
		Steps: []StepElement{
			{Step: &Step{Match: Match{Argv: []string{"setup"}}, Respond: Response{Exit: 0}}},
			{Group: &StepGroup{
				Mode: "unordered",
				Name: "checks",
				Steps: []StepElement{
					{Step: &Step{Match: Match{Argv: []string{"check-a"}}, Respond: Response{Exit: 0}}},
					{Step: &Step{Match: Match{Argv: []string{"check-b"}}, Respond: Response{Exit: 0}}},
				},
			}},
			{Step: &Step{Match: Match{Argv: []string{"teardown"}}, Respond: Response{Exit: 0}}},
		},
	}

	flat := scn.FlatSteps()
	require.Len(t, flat, 4)
	assert.Equal(t, []string{"setup"}, flat[0].Match.Argv)
	assert.Equal(t, []string{"check-a"}, flat[1].Match.Argv)
	assert.Equal(t, []string{"check-b"}, flat[2].Match.Argv)
	assert.Equal(t, []string{"teardown"}, flat[3].Match.Argv)
}

func TestFlatSteps_NoGroups(t *testing.T) {
	scn := Scenario{
		Meta: Meta{Name: "linear"},
		Steps: []StepElement{
			{Step: &Step{Match: Match{Argv: []string{"a"}}, Respond: Response{Exit: 0}}},
			{Step: &Step{Match: Match{Argv: []string{"b"}}, Respond: Response{Exit: 0}}},
		},
	}
	flat := scn.FlatSteps()
	require.Len(t, flat, 2)
}

func TestFlatSteps_EmptyGroup(t *testing.T) {
	scn := Scenario{
		Meta: Meta{Name: "empty-group"},
		Steps: []StepElement{
			{Step: &Step{Match: Match{Argv: []string{"a"}}, Respond: Response{Exit: 0}}},
			{Group: &StepGroup{Mode: "unordered", Name: "empty", Steps: nil}},
		},
	}
	flat := scn.FlatSteps()
	require.Len(t, flat, 1)
}

// --- GroupRanges -------------------------------------------------------------

func TestGroupRanges_SingleGroup(t *testing.T) {
	scn := Scenario{
		Meta: Meta{Name: "group-range"},
		Steps: []StepElement{
			{Step: &Step{Match: Match{Argv: []string{"s1"}}, Respond: Response{Exit: 0}}},
			{Group: &StepGroup{
				Mode: "unordered",
				Name: "grp",
				Steps: []StepElement{
					{Step: &Step{Match: Match{Argv: []string{"g1"}}, Respond: Response{Exit: 0}}},
					{Step: &Step{Match: Match{Argv: []string{"g2"}}, Respond: Response{Exit: 0}}},
					{Step: &Step{Match: Match{Argv: []string{"g3"}}, Respond: Response{Exit: 0}}},
				},
			}},
			{Step: &Step{Match: Match{Argv: []string{"s2"}}, Respond: Response{Exit: 0}}},
		},
	}

	ranges := scn.GroupRanges()
	require.Len(t, ranges, 1)
	assert.Equal(t, 1, ranges[0].Start)
	assert.Equal(t, 4, ranges[0].End)
	assert.Equal(t, "grp", ranges[0].Name)
	assert.Equal(t, 1, ranges[0].TopIndex)
}

func TestGroupRanges_MultipleGroups(t *testing.T) {
	scn := Scenario{
		Meta: Meta{Name: "multi-group"},
		Steps: []StepElement{
			{Group: &StepGroup{
				Mode:  "unordered",
				Name:  "first",
				Steps: []StepElement{{Step: &Step{Match: Match{Argv: []string{"a"}}, Respond: Response{Exit: 0}}}},
			}},
			{Step: &Step{Match: Match{Argv: []string{"mid"}}, Respond: Response{Exit: 0}}},
			{Group: &StepGroup{
				Mode: "unordered",
				Name: "second",
				Steps: []StepElement{
					{Step: &Step{Match: Match{Argv: []string{"b"}}, Respond: Response{Exit: 0}}},
					{Step: &Step{Match: Match{Argv: []string{"c"}}, Respond: Response{Exit: 0}}},
				},
			}},
		},
	}

	ranges := scn.GroupRanges()
	require.Len(t, ranges, 2)

	assert.Equal(t, 0, ranges[0].Start)
	assert.Equal(t, 1, ranges[0].End)
	assert.Equal(t, "first", ranges[0].Name)

	assert.Equal(t, 2, ranges[1].Start)
	assert.Equal(t, 4, ranges[1].End)
	assert.Equal(t, "second", ranges[1].Name)
}

func TestGroupRanges_NoGroups(t *testing.T) {
	scn := Scenario{
		Meta: Meta{Name: "no-group"},
		Steps: []StepElement{
			{Step: &Step{Match: Match{Argv: []string{"a"}}, Respond: Response{Exit: 0}}},
		},
	}
	assert.Empty(t, scn.GroupRanges())
}

// --- YAML Round-Trip ---------------------------------------------------------

func TestStepElement_YAMLRoundTrip_LeafStep(t *testing.T) {
	original := StepElement{
		Step: &Step{
			Match:   Match{Argv: []string{"kubectl", "get", "pods"}},
			Respond: Response{Exit: 0, Stdout: "pod-1 Running"},
		},
	}

	data, err := yaml.Marshal(original)
	require.NoError(t, err)

	var decoded StepElement
	require.NoError(t, yaml.Unmarshal(data, &decoded))

	require.NotNil(t, decoded.Step)
	assert.Nil(t, decoded.Group)
	assert.Equal(t, original.Step.Match.Argv, decoded.Step.Match.Argv)
	assert.Equal(t, original.Step.Respond.Exit, decoded.Step.Respond.Exit)
	assert.Equal(t, original.Step.Respond.Stdout, decoded.Step.Respond.Stdout)
}

func TestStepElement_YAMLRoundTrip_Group(t *testing.T) {
	original := StepElement{
		Group: &StepGroup{
			Mode: "unordered",
			Name: "checks",
			Steps: []StepElement{
				{Step: &Step{Match: Match{Argv: []string{"check-a"}}, Respond: Response{Exit: 0}}},
				{Step: &Step{Match: Match{Argv: []string{"check-b"}}, Respond: Response{Exit: 0}}},
			},
		},
	}

	data, err := yaml.Marshal(original)
	require.NoError(t, err)

	var decoded StepElement
	require.NoError(t, yaml.Unmarshal(data, &decoded))

	require.NotNil(t, decoded.Group)
	assert.Nil(t, decoded.Step)
	assert.Equal(t, "unordered", decoded.Group.Mode)
	assert.Equal(t, "checks", decoded.Group.Name)
	require.Len(t, decoded.Group.Steps, 2)
}

// --- Scenario YAML round-trip ------------------------------------------------

func TestScenario_YAMLRoundTrip(t *testing.T) {
	original := Scenario{
		Meta: Meta{
			Name:        "roundtrip-test",
			Description: "Tests YAML serialization",
			Vars:        map[string]string{"ns": "default"},
		},
		Steps: []StepElement{
			{Step: &Step{
				Match:   Match{Argv: []string{"cmd", "arg"}},
				Respond: Response{Exit: 0, Stdout: "output"},
			}},
		},
	}

	data, err := yaml.Marshal(original)
	require.NoError(t, err)

	roundtripped, err := Load(strings.NewReader(string(data)))
	require.NoError(t, err)
	assert.Equal(t, original.Meta.Name, roundtripped.Meta.Name)
	assert.Equal(t, original.Meta.Description, roundtripped.Meta.Description)
	assert.Equal(t, original.Meta.Vars, roundtripped.Meta.Vars)
	require.Len(t, roundtripped.Steps, 1)
}

// --- Session validation ------------------------------------------------------

func TestSession_Validate_Table(t *testing.T) {
	tests := []struct {
		name        string
		session     Session
		wantErr     bool
		errContains string
	}{
		{"empty TTL is valid", Session{TTL: ""}, false, ""},
		{"valid TTL", Session{TTL: "10m"}, false, ""},
		{"valid TTL hours", Session{TTL: "2h"}, false, ""},
		{"invalid TTL format", Session{TTL: "notduration"}, true, "invalid ttl"},
		{"zero TTL", Session{TTL: "0s"}, true, "must be positive"},
		{"negative TTL", Session{TTL: "-5m"}, true, "must be positive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.session.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// --- Security validation -----------------------------------------------------

func TestSecurity_Validate_EmptyDenyPattern(t *testing.T) {
	s := &Security{DenyEnvVars: []string{"AWS_*", ""}}
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be non-empty")
}

func TestSecurity_Validate_AllValid(t *testing.T) {
	s := &Security{
		AllowedCommands: []string{"kubectl", "az"},
		DenyEnvVars:     []string{"AWS_*", "SECRET_*"},
	}
	assert.NoError(t, s.Validate())
}

// --- Capture identifier validation -------------------------------------------

func TestResponse_Validate_CaptureIdentifiers(t *testing.T) {
	tests := []struct {
		name    string
		capture map[string]string
		wantErr bool
	}{
		{"valid identifier", map[string]string{"rg_id": "val"}, false},
		{"valid underscore prefix", map[string]string{"_internal": "val"}, false},
		{"invalid starts with number", map[string]string{"1bad": "val"}, true},
		{"invalid contains dash", map[string]string{"bad-id": "val"}, true},
		{"invalid contains space", map[string]string{"bad id": "val"}, true},
		{"empty key", map[string]string{"": "val"}, true},
		{"multiple valid", map[string]string{"a": "1", "b_c": "2", "D": "3"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Response{Exit: 0, Capture: tt.capture}
			err := r.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "capture identifier")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// --- Nested group rejection --------------------------------------------------

func TestStepGroup_NestedGroupRejected(t *testing.T) {
	sg := &StepGroup{
		Mode: "unordered",
		Steps: []StepElement{
			{Group: &StepGroup{Mode: "unordered", Steps: []StepElement{
				{Step: &Step{Match: Match{Argv: []string{"inner"}}, Respond: Response{Exit: 0}}},
			}}},
		},
	}
	err := sg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nested groups")
}

// --- Load with call bounds ---------------------------------------------------

func TestLoad_WithCallBounds(t *testing.T) {
	yamlContent := `
meta:
  name: bounds-test
steps:
  - match:
      argv: ["cmd"]
    calls:
      min: 2
      max: 5
    respond:
      exit: 0
`
	scn, err := Load(strings.NewReader(yamlContent))
	require.NoError(t, err)
	require.Len(t, scn.Steps, 1)
	require.NotNil(t, scn.Steps[0].Step.Calls)
	assert.Equal(t, 2, scn.Steps[0].Step.Calls.Min)
	assert.Equal(t, 5, scn.Steps[0].Step.Calls.Max)
}

func TestLoad_CallBoundsMinOnlyDefaultsMaxToMin(t *testing.T) {
	yamlContent := `
meta:
  name: min-only
steps:
  - match:
      argv: ["cmd"]
    calls:
      min: 3
    respond:
      exit: 0
`
	scn, err := Load(strings.NewReader(yamlContent))
	require.NoError(t, err)
	require.NotNil(t, scn.Steps[0].Step.Calls)
	assert.Equal(t, 3, scn.Steps[0].Step.Calls.Min)
	assert.Equal(t, 3, scn.Steps[0].Step.Calls.Max)
}

// --- Load with security config -----------------------------------------------

func TestLoad_WithSecurityConfig(t *testing.T) {
	yamlContent := `
meta:
  name: secure
  security:
    allowed_commands: ["kubectl", "az"]
    deny_env_vars: ["SECRET_*"]
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
`
	scn, err := Load(strings.NewReader(yamlContent))
	require.NoError(t, err)
	require.NotNil(t, scn.Meta.Security)
	assert.Equal(t, []string{"kubectl", "az"}, scn.Meta.Security.AllowedCommands)
	assert.Equal(t, []string{"SECRET_*"}, scn.Meta.Security.DenyEnvVars)
}

// --- Load with session config ------------------------------------------------

func TestLoad_WithSessionTTL(t *testing.T) {
	yamlContent := `
meta:
  name: session-test
  session:
    ttl: 30m
steps:
  - match:
      argv: ["cmd"]
    respond:
      exit: 0
`
	scn, err := Load(strings.NewReader(yamlContent))
	require.NoError(t, err)
	require.NotNil(t, scn.Meta.Session)
	assert.Equal(t, "30m", scn.Meta.Session.TTL)
}

// --- Match validation --------------------------------------------------------

func TestMatch_Validate_EmptyArgvRejected(t *testing.T) {
	m := Match{Argv: []string{}}
	require.Error(t, m.Validate())
}

func TestMatch_Validate_WithStdin(t *testing.T) {
	m := Match{Argv: []string{"cmd"}, Stdin: "some input"}
	assert.NoError(t, m.Validate())
}

// --- Response stdout/stderr_file mutual exclusivity --------------------------

func TestResponse_Validate_MutualExclusivity(t *testing.T) {
	tests := []struct {
		name    string
		resp    Response
		wantErr bool
	}{
		{"stdout only", Response{Exit: 0, Stdout: "out"}, false},
		{"stdout_file only", Response{Exit: 0, StdoutFile: "f.txt"}, false},
		{"both stdout", Response{Exit: 0, Stdout: "out", StdoutFile: "f.txt"}, true},
		{"stderr only", Response{Exit: 0, Stderr: "err"}, false},
		{"stderr_file only", Response{Exit: 0, StderrFile: "f.txt"}, false},
		{"both stderr", Response{Exit: 0, Stderr: "err", StderrFile: "f.txt"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.resp.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "mutually exclusive")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
