package scenario

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

//nolint:funlen // Table-driven test with comprehensive test cases
func TestScenario_Validate(t *testing.T) {
	tests := []struct {
		name        string
		scenario    Scenario
		wantErr     bool
		errContains string
	}{
		{
			name: "valid scenario with single step",
			scenario: Scenario{
				Meta: Meta{Name: "test-scenario"},
				Steps: []StepElement{
					{Step: &Step{
						Match:   Match{Argv: []string{"kubectl", "get", "pods"}},
						Respond: Response{Exit: 0, Stdout: "pod-output"},
					}},
				},
			},
			wantErr: false,
		},
		{
			name: "valid scenario with multiple steps",
			scenario: Scenario{
				Meta: Meta{
					Name:        "multi-step",
					Description: "A multi-step scenario",
					Vars:        map[string]string{"cluster": "prod"},
				},
				Steps: []StepElement{
					{Step: &Step{
						Match:   Match{Argv: []string{"cmd1"}},
						Respond: Response{Exit: 0},
					}},
					{Step: &Step{
						Match:   Match{Argv: []string{"cmd2", "arg"}},
						Respond: Response{Exit: 1, Stderr: "error"},
					}},
				},
			},
			wantErr: false,
		},
		{
			name: "empty steps",
			scenario: Scenario{
				Meta:  Meta{Name: "empty"},
				Steps: []StepElement{},
			},
			wantErr:     true,
			errContains: "steps must contain at least one step",
		},
		{
			name: "nil steps",
			scenario: Scenario{
				Meta:  Meta{Name: "nil-steps"},
				Steps: nil,
			},
			wantErr:     true,
			errContains: "steps must contain at least one step",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.scenario.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMeta_Validate(t *testing.T) {
	tests := []struct {
		name        string
		meta        Meta
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid meta with name only",
			meta:    Meta{Name: "test"},
			wantErr: false,
		},
		{
			name: "valid meta with all fields",
			meta: Meta{
				Name:        "test",
				Description: "description",
				Vars:        map[string]string{"key": "value"},
			},
			wantErr: false,
		},
		{
			name:        "empty name",
			meta:        Meta{Name: ""},
			wantErr:     true,
			errContains: "name must be non-empty",
		},
		{
			name:        "whitespace-only name",
			meta:        Meta{Name: "   "},
			wantErr:     true,
			errContains: "name must be non-empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.meta.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStep_Validate(t *testing.T) {
	tests := []struct {
		name        string
		step        Step
		wantErr     bool
		errContains string
	}{
		{
			name: "valid step",
			step: Step{
				Match:   Match{Argv: []string{"cmd"}},
				Respond: Response{Exit: 0},
			},
			wantErr: false,
		},
		{
			name: "valid step with all response fields",
			step: Step{
				Match: Match{Argv: []string{"cmd", "arg1", "arg2"}},
				Respond: Response{
					Exit:   1,
					Stdout: "out",
					Stderr: "err",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.step.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMatch_Validate(t *testing.T) {
	tests := []struct {
		name        string
		match       Match
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid match with single element",
			match:   Match{Argv: []string{"cmd"}},
			wantErr: false,
		},
		{
			name:    "valid match with multiple elements",
			match:   Match{Argv: []string{"cmd", "arg1", "arg2"}},
			wantErr: false,
		},
		{
			name:        "empty argv",
			match:       Match{Argv: []string{}},
			wantErr:     true,
			errContains: "argv must be non-empty",
		},
		{
			name:        "nil argv",
			match:       Match{Argv: nil},
			wantErr:     true,
			errContains: "argv must be non-empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.match.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCallBounds_Validate(t *testing.T) {
	tests := []struct {
		name        string
		calls       CallBounds
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid exactly once",
			calls:   CallBounds{Min: 1, Max: 1},
			wantErr: false,
		},
		{
			name:    "valid range",
			calls:   CallBounds{Min: 1, Max: 5},
			wantErr: false,
		},
		{
			name:    "valid optional step",
			calls:   CallBounds{Min: 0, Max: 1},
			wantErr: false,
		},
		{
			name:    "valid min equals max",
			calls:   CallBounds{Min: 3, Max: 3},
			wantErr: false,
		},
		{
			name:        "max zero rejected",
			calls:       CallBounds{Min: 0, Max: 0},
			wantErr:     true,
			errContains: "max must be >= 1",
		},
		{
			name:        "negative min rejected",
			calls:       CallBounds{Min: -1, Max: 1},
			wantErr:     true,
			errContains: "min must be >= 0",
		},
		{
			name:        "min greater than max rejected",
			calls:       CallBounds{Min: 5, Max: 3},
			wantErr:     true,
			errContains: "min (5) must be <= max (3)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.calls.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStep_EffectiveCalls(t *testing.T) {
	t.Run("nil calls returns default {1,1}", func(t *testing.T) {
		step := Step{
			Match:   Match{Argv: []string{"cmd"}},
			Respond: Response{Exit: 0},
		}
		ec := step.EffectiveCalls()
		assert.Equal(t, 1, ec.Min)
		assert.Equal(t, 1, ec.Max)
	})

	t.Run("explicit calls returned as-is", func(t *testing.T) {
		step := Step{
			Match:   Match{Argv: []string{"cmd"}},
			Respond: Response{Exit: 0},
			Calls:   &CallBounds{Min: 2, Max: 5},
		}
		ec := step.EffectiveCalls()
		assert.Equal(t, 2, ec.Min)
		assert.Equal(t, 5, ec.Max)
	})
}

func TestStep_Validate_WithCalls(t *testing.T) {
	tests := []struct {
		name        string
		step        Step
		wantErr     bool
		errContains string
	}{
		{
			name: "valid step with calls",
			step: Step{
				Match:   Match{Argv: []string{"cmd"}},
				Respond: Response{Exit: 0},
				Calls:   &CallBounds{Min: 1, Max: 5},
			},
			wantErr: false,
		},
		{
			name: "calls min only defaults max to min",
			step: Step{
				Match:   Match{Argv: []string{"cmd"}},
				Respond: Response{Exit: 0},
				Calls:   &CallBounds{Min: 3, Max: 0},
			},
			wantErr: false, // defaulting: max = min = 3
		},
		{
			name: "calls max:0 min:0 rejected",
			step: Step{
				Match:   Match{Argv: []string{"cmd"}},
				Respond: Response{Exit: 0},
				Calls:   &CallBounds{Min: 0, Max: 0},
			},
			wantErr:     true,
			errContains: "max must be >= 1",
		},
		{
			name: "calls min > max rejected",
			step: Step{
				Match:   Match{Argv: []string{"cmd"}},
				Respond: Response{Exit: 0},
				Calls:   &CallBounds{Min: 5, Max: 3},
			},
			wantErr:     true,
			errContains: "min (5) must be <= max (3)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.step.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSecurity_AllowedCommands(t *testing.T) {
	t.Run("security struct with allowed commands", func(t *testing.T) {
		sec := Security{AllowedCommands: []string{"kubectl", "az"}}
		assert.Equal(t, []string{"kubectl", "az"}, sec.AllowedCommands)
	})

	t.Run("empty allowed commands", func(t *testing.T) {
		sec := Security{}
		assert.Nil(t, sec.AllowedCommands)
	})
}

func TestMeta_WithSecurity(t *testing.T) {
	t.Run("meta with security section", func(t *testing.T) {
		meta := Meta{
			Name:     "test",
			Security: &Security{AllowedCommands: []string{"kubectl"}},
		}
		require.NoError(t, meta.Validate())
		assert.NotNil(t, meta.Security)
		assert.Equal(t, []string{"kubectl"}, meta.Security.AllowedCommands)
	})

	t.Run("meta without security section", func(t *testing.T) {
		meta := Meta{Name: "test"}
		require.NoError(t, meta.Validate())
		assert.Nil(t, meta.Security)
	})
}

func TestMatch_WithStdin(t *testing.T) {
	t.Run("match with stdin", func(t *testing.T) {
		m := Match{
			Argv:  []string{"kubectl", "apply", "-f", "-"},
			Stdin: "apiVersion: v1\nkind: Pod",
		}
		require.NoError(t, m.Validate())
		assert.Equal(t, "apiVersion: v1\nkind: Pod", m.Stdin)
	})

	t.Run("match without stdin", func(t *testing.T) {
		m := Match{Argv: []string{"cmd"}}
		require.NoError(t, m.Validate())
		assert.Empty(t, m.Stdin)
	})
}

func TestMatch_StdinYAMLParsing(t *testing.T) {
	// Test that stdin is correctly parsed from YAML
	yamlContent := `
meta:
  name: stdin-test
steps:
  - match:
      argv: ["kubectl", "apply", "-f", "-"]
      stdin: |
        apiVersion: v1
        kind: Pod
    respond:
      exit: 0
      stdout: "created"
`
	var scn Scenario
	err := yaml.Unmarshal([]byte(yamlContent), &scn)
	require.NoError(t, err)
	require.Len(t, scn.Steps, 1)
	assert.Contains(t, scn.Steps[0].Step.Match.Stdin, "apiVersion: v1")
	assert.Contains(t, scn.Steps[0].Step.Match.Stdin, "kind: Pod")
}

func TestCallBounds_YAMLParsing(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantMin int
		wantMax int
		wantNil bool
	}{
		{
			name:    "no calls field",
			yaml:    `match: {argv: ["cmd"]}` + "\nrespond: {exit: 0}",
			wantNil: true,
		},
		{
			name:    "min and max specified",
			yaml:    `match: {argv: ["cmd"]}` + "\nrespond: {exit: 0}\ncalls: {min: 1, max: 5}",
			wantMin: 1,
			wantMax: 5,
		},
		{
			name:    "min only",
			yaml:    `match: {argv: ["cmd"]}` + "\nrespond: {exit: 0}\ncalls: {min: 3}",
			wantMin: 3,
			wantMax: 0, // Go zero-value; defaulted during validation
		},
		{
			name:    "max only",
			yaml:    `match: {argv: ["cmd"]}` + "\nrespond: {exit: 0}\ncalls: {max: 5}",
			wantMin: 0,
			wantMax: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var step Step
			err := yaml.Unmarshal([]byte(tt.yaml), &step)
			require.NoError(t, err)

			if tt.wantNil {
				assert.Nil(t, step.Calls)
			} else {
				require.NotNil(t, step.Calls)
				assert.Equal(t, tt.wantMin, step.Calls.Min)
				assert.Equal(t, tt.wantMax, step.Calls.Max)
			}
		})
	}
}

func TestSecurityYAMLParsing(t *testing.T) {
	yamlContent := `
meta:
  name: security-test
  security:
    allowed_commands:
      - kubectl
      - az
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
`
	var scn Scenario
	err := yaml.Unmarshal([]byte(yamlContent), &scn)
	require.NoError(t, err)
	require.NotNil(t, scn.Meta.Security)
	assert.Equal(t, []string{"kubectl", "az"}, scn.Meta.Security.AllowedCommands)
}

// T004: DenyEnvVars field tests

func TestSecurity_DenyEnvVars(t *testing.T) {
	t.Run("security struct with deny_env_vars", func(t *testing.T) {
		sec := Security{DenyEnvVars: []string{"AWS_*", "GITHUB_TOKEN"}}
		assert.Equal(t, []string{"AWS_*", "GITHUB_TOKEN"}, sec.DenyEnvVars)
		assert.NoError(t, sec.Validate())
	})

	t.Run("empty deny_env_vars slice", func(t *testing.T) {
		sec := Security{DenyEnvVars: []string{}}
		assert.NoError(t, sec.Validate())
	})

	t.Run("nil deny_env_vars", func(t *testing.T) {
		sec := Security{}
		assert.Nil(t, sec.DenyEnvVars)
		assert.NoError(t, sec.Validate())
	})
}

func TestSecurity_DenyEnvVars_Validation(t *testing.T) {
	t.Run("empty string in deny_env_vars rejected", func(t *testing.T) {
		sec := Security{DenyEnvVars: []string{"AWS_*", ""}}
		err := sec.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "deny_env_vars[1]: must be non-empty")
	})

	t.Run("valid patterns pass validation", func(t *testing.T) {
		sec := Security{DenyEnvVars: []string{"*", "AWS_*", "GITHUB_TOKEN", "*_SECRET"}}
		assert.NoError(t, sec.Validate())
	})
}

func TestDenyEnvVarsYAMLParsing(t *testing.T) {
	yamlContent := `
meta:
  name: deny-test
  security:
    allowed_commands:
      - kubectl
    deny_env_vars:
      - "AWS_*"
      - "GITHUB_TOKEN"
      - "*_SECRET"
steps:
  - match:
      argv: ["kubectl", "get", "pods"]
    respond:
      exit: 0
`
	var scn Scenario
	err := yaml.Unmarshal([]byte(yamlContent), &scn)
	require.NoError(t, err)
	require.NotNil(t, scn.Meta.Security)
	assert.Equal(t, []string{"AWS_*", "GITHUB_TOKEN", "*_SECRET"}, scn.Meta.Security.DenyEnvVars)
	assert.Equal(t, []string{"kubectl"}, scn.Meta.Security.AllowedCommands)
}

func TestMeta_DenyEnvVarsValidation(t *testing.T) {
	t.Run("meta with empty deny_env_vars entry fails validation", func(t *testing.T) {
		meta := Meta{
			Name:     "test",
			Security: &Security{DenyEnvVars: []string{""}},
		}
		err := meta.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "security: deny_env_vars[0]: must be non-empty")
	})
}

// T005: Session struct tests

func TestSession_Validate(t *testing.T) {
	tests := []struct {
		name        string
		session     Session
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid TTL",
			session: Session{TTL: "5m"},
			wantErr: false,
		},
		{
			name:    "valid TTL hours",
			session: Session{TTL: "1h"},
			wantErr: false,
		},
		{
			name:    "valid TTL seconds",
			session: Session{TTL: "30s"},
			wantErr: false,
		},
		{
			name:    "empty TTL is valid",
			session: Session{TTL: ""},
			wantErr: false,
		},
		{
			name:        "invalid TTL format",
			session:     Session{TTL: "never"},
			wantErr:     true,
			errContains: "invalid ttl",
		},
		{
			name:        "negative TTL",
			session:     Session{TTL: "-5m"},
			wantErr:     true,
			errContains: "ttl must be positive",
		},
		{
			name:        "zero TTL",
			session:     Session{TTL: "0s"},
			wantErr:     true,
			errContains: "ttl must be positive",
		},
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

func TestMeta_WithSession(t *testing.T) {
	t.Run("meta with session TTL", func(t *testing.T) {
		meta := Meta{
			Name:    "test",
			Session: &Session{TTL: "10m"},
		}
		require.NoError(t, meta.Validate())
		assert.Equal(t, "10m", meta.Session.TTL)
	})

	t.Run("meta with invalid session TTL", func(t *testing.T) {
		meta := Meta{
			Name:    "test",
			Session: &Session{TTL: "invalid"},
		}
		err := meta.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "session: invalid ttl")
	})

	t.Run("meta without session", func(t *testing.T) {
		meta := Meta{Name: "test"}
		require.NoError(t, meta.Validate())
		assert.Nil(t, meta.Session)
	})
}

func TestSessionYAMLParsing(t *testing.T) {
	yamlContent := `
meta:
  name: session-test
  session:
    ttl: "5m"
steps:
  - match:
      argv: ["terraform", "plan"]
    respond:
      exit: 0
`
	var scn Scenario
	err := yaml.Unmarshal([]byte(yamlContent), &scn)
	require.NoError(t, err)
	require.NotNil(t, scn.Meta.Session)
	assert.Equal(t, "5m", scn.Meta.Session.TTL)
}

func TestBothDenyEnvVarsAndSessionYAML(t *testing.T) {
	yamlContent := `
meta:
  name: combined-test
  security:
    allowed_commands: [az]
    deny_env_vars: ["*"]
  session:
    ttl: "10m"
  vars:
    region: "eastus2"
steps:
  - match:
      argv: ["az", "account", "show"]
    respond:
      exit: 0
      stdout: "region={{ .region }}\n"
`
	var scn Scenario
	err := yaml.Unmarshal([]byte(yamlContent), &scn)
	require.NoError(t, err)
	require.NoError(t, scn.Validate())

	require.NotNil(t, scn.Meta.Security)
	assert.Equal(t, []string{"*"}, scn.Meta.Security.DenyEnvVars)
	assert.Equal(t, []string{"az"}, scn.Meta.Security.AllowedCommands)

	require.NotNil(t, scn.Meta.Session)
	assert.Equal(t, "10m", scn.Meta.Session.TTL)
	assert.Equal(t, "eastus2", scn.Meta.Vars["region"])
}

func TestResponse_Validate(t *testing.T) {
	tests := []struct {
		name        string
		response    Response
		wantErr     bool
		errContains string
	}{
		{
			name:     "valid response with exit only",
			response: Response{Exit: 0},
			wantErr:  false,
		},
		{
			name:     "valid response with stdout",
			response: Response{Exit: 0, Stdout: "output"},
			wantErr:  false,
		},
		{
			name:     "valid response with stderr",
			response: Response{Exit: 1, Stderr: "error"},
			wantErr:  false,
		},
		{
			name:     "valid response with stdout_file",
			response: Response{Exit: 0, StdoutFile: "file.txt"},
			wantErr:  false,
		},
		{
			name:     "valid response with stderr_file",
			response: Response{Exit: 1, StderrFile: "error.txt"},
			wantErr:  false,
		},
		{
			name:     "valid exit code 255",
			response: Response{Exit: 255},
			wantErr:  false,
		},
		{
			name:        "exit code negative",
			response:    Response{Exit: -1},
			wantErr:     true,
			errContains: "exit must be in range 0-255",
		},
		{
			name:        "exit code too large",
			response:    Response{Exit: 256},
			wantErr:     true,
			errContains: "exit must be in range 0-255",
		},
		{
			name:        "stdout and stdout_file mutually exclusive",
			response:    Response{Exit: 0, Stdout: "out", StdoutFile: "file.txt"},
			wantErr:     true,
			errContains: "stdout and stdout_file are mutually exclusive",
		},
		{
			name:        "stderr and stderr_file mutually exclusive",
			response:    Response{Exit: 0, Stderr: "err", StderrFile: "file.txt"},
			wantErr:     true,
			errContains: "stderr and stderr_file are mutually exclusive",
		},
		// T011: Capture identifier validation tests
		{
			name:     "valid capture identifiers",
			response: Response{Exit: 0, Capture: map[string]string{"rg_id": "val", "_underscore": "v", "CamelCase": "v"}},
			wantErr:  false,
		},
		{
			name:     "valid capture with empty map",
			response: Response{Exit: 0, Capture: map[string]string{}},
			wantErr:  false,
		},
		{
			name:        "capture identifier starting with digit rejected",
			response:    Response{Exit: 0, Capture: map[string]string{"1bad": "val"}},
			wantErr:     true,
			errContains: "capture identifier \"1bad\" must match",
		},
		{
			name:        "capture identifier with hyphen rejected",
			response:    Response{Exit: 0, Capture: map[string]string{"my-id": "val"}},
			wantErr:     true,
			errContains: "capture identifier \"my-id\" must match",
		},
		{
			name:        "capture identifier with spaces rejected",
			response:    Response{Exit: 0, Capture: map[string]string{"my id": "val"}},
			wantErr:     true,
			errContains: "capture identifier \"my id\" must match",
		},
		{
			name:        "capture empty key rejected",
			response:    Response{Exit: 0, Capture: map[string]string{"": "val"}},
			wantErr:     true,
			errContains: "must match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.response.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// T019: StepElement/StepGroup validation tests

func TestStepElement_Validate(t *testing.T) {
	tests := []struct {
		name        string
		elem        StepElement
		wantErr     bool
		errContains string
	}{
		{
			name: "valid leaf step",
			elem: StepElement{Step: &Step{
				Match: Match{Argv: []string{"cmd"}}, Respond: Response{Exit: 0},
			}},
		},
		{
			name: "valid group",
			elem: StepElement{Group: &StepGroup{
				Mode: "unordered",
				Name: "g1",
				Steps: []StepElement{
					{Step: &Step{Match: Match{Argv: []string{"a"}}, Respond: Response{Exit: 0}}},
				},
			}},
		},
		{
			name:        "neither step nor group",
			elem:        StepElement{},
			wantErr:     true,
			errContains: "must have either a step or a group",
		},
		{
			name: "both step and group",
			elem: StepElement{
				Step:  &Step{Match: Match{Argv: []string{"cmd"}}, Respond: Response{Exit: 0}},
				Group: &StepGroup{Mode: "unordered", Steps: []StepElement{{Step: &Step{Match: Match{Argv: []string{"a"}}, Respond: Response{Exit: 0}}}}},
			},
			wantErr:     true,
			errContains: "not both",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.elem.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStepGroup_Validate(t *testing.T) {
	tests := []struct {
		name        string
		group       StepGroup
		wantErr     bool
		errContains string
	}{
		{
			name: "valid unordered group",
			group: StepGroup{
				Mode: "unordered",
				Steps: []StepElement{
					{Step: &Step{Match: Match{Argv: []string{"a"}}, Respond: Response{Exit: 0}}},
					{Step: &Step{Match: Match{Argv: []string{"b"}}, Respond: Response{Exit: 0}}},
				},
			},
		},
		{
			name:        "empty group rejected",
			group:       StepGroup{Mode: "unordered", Steps: []StepElement{}},
			wantErr:     true,
			errContains: "must contain at least one step",
		},
		{
			name: "nested group rejected",
			group: StepGroup{
				Mode: "unordered",
				Steps: []StepElement{
					{Group: &StepGroup{Mode: "unordered", Steps: []StepElement{
						{Step: &Step{Match: Match{Argv: []string{"x"}}, Respond: Response{Exit: 0}}},
					}}},
				},
			},
			wantErr:     true,
			errContains: "nested groups are not allowed",
		},
		{
			name: "unknown mode rejected",
			group: StepGroup{
				Mode:  "ordered",
				Steps: []StepElement{{Step: &Step{Match: Match{Argv: []string{"a"}}, Respond: Response{Exit: 0}}}},
			},
			wantErr:     true,
			errContains: "unsupported group mode",
		},
		{
			name: "nil step child rejected",
			group: StepGroup{
				Mode:  "unordered",
				Steps: []StepElement{{}},
			},
			wantErr:     true,
			errContains: "group children must be leaf steps",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.group.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestScenario_AutoNamingGroups(t *testing.T) {
	scn := Scenario{
		Meta: Meta{Name: "group-test"},
		Steps: []StepElement{
			{Step: &Step{Match: Match{Argv: []string{"a"}}, Respond: Response{Exit: 0}}},
			{Group: &StepGroup{
				Mode:  "unordered",
				Steps: []StepElement{{Step: &Step{Match: Match{Argv: []string{"b"}}, Respond: Response{Exit: 0}}}},
			}},
			{Group: &StepGroup{
				Mode:  "unordered",
				Steps: []StepElement{{Step: &Step{Match: Match{Argv: []string{"c"}}, Respond: Response{Exit: 0}}}},
			}},
		},
	}
	require.NoError(t, scn.Validate())

	// Groups auto-named sequentially
	assert.Equal(t, "group-1", scn.Steps[1].Group.Name)
	assert.Equal(t, "group-2", scn.Steps[2].Group.Name)
}

func TestScenario_ExplicitNamePreserved(t *testing.T) {
	scn := Scenario{
		Meta: Meta{Name: "named-groups"},
		Steps: []StepElement{
			{Group: &StepGroup{
				Mode: "unordered", Name: "pre-flight",
				Steps: []StepElement{{Step: &Step{Match: Match{Argv: []string{"a"}}, Respond: Response{Exit: 0}}}},
			}},
			{Group: &StepGroup{
				Mode:  "unordered",
				Steps: []StepElement{{Step: &Step{Match: Match{Argv: []string{"b"}}, Respond: Response{Exit: 0}}}},
			}},
		},
	}
	require.NoError(t, scn.Validate())

	assert.Equal(t, "pre-flight", scn.Steps[0].Group.Name)
	assert.Equal(t, "group-2", scn.Steps[1].Group.Name)
}

func TestScenario_FlatSteps(t *testing.T) {
	scn := Scenario{
		Meta: Meta{Name: "flat-test"},
		Steps: []StepElement{
			{Step: &Step{Match: Match{Argv: []string{"a"}}, Respond: Response{Exit: 0}}},
			{Group: &StepGroup{
				Mode: "unordered", Name: "g1",
				Steps: []StepElement{
					{Step: &Step{Match: Match{Argv: []string{"b"}}, Respond: Response{Exit: 0}}},
					{Step: &Step{Match: Match{Argv: []string{"c"}}, Respond: Response{Exit: 0}}},
				},
			}},
			{Step: &Step{Match: Match{Argv: []string{"d"}}, Respond: Response{Exit: 0}}},
		},
	}

	flat := scn.FlatSteps()
	require.Len(t, flat, 4)
	assert.Equal(t, []string{"a"}, flat[0].Match.Argv)
	assert.Equal(t, []string{"b"}, flat[1].Match.Argv)
	assert.Equal(t, []string{"c"}, flat[2].Match.Argv)
	assert.Equal(t, []string{"d"}, flat[3].Match.Argv)
}

func TestScenario_GroupRanges(t *testing.T) {
	scn := Scenario{
		Meta: Meta{Name: "ranges-test"},
		Steps: []StepElement{
			{Step: &Step{Match: Match{Argv: []string{"a"}}, Respond: Response{Exit: 0}}},
			{Group: &StepGroup{
				Mode: "unordered", Name: "g1",
				Steps: []StepElement{
					{Step: &Step{Match: Match{Argv: []string{"b"}}, Respond: Response{Exit: 0}}},
					{Step: &Step{Match: Match{Argv: []string{"c"}}, Respond: Response{Exit: 0}}},
				},
			}},
			{Step: &Step{Match: Match{Argv: []string{"d"}}, Respond: Response{Exit: 0}}},
			{Group: &StepGroup{
				Mode: "unordered", Name: "g2",
				Steps: []StepElement{
					{Step: &Step{Match: Match{Argv: []string{"e"}}, Respond: Response{Exit: 0}}},
				},
			}},
		},
	}

	ranges := scn.GroupRanges()
	require.Len(t, ranges, 2)

	assert.Equal(t, GroupRange{Start: 1, End: 3, Name: "g1", TopIndex: 1}, ranges[0])
	assert.Equal(t, GroupRange{Start: 4, End: 5, Name: "g2", TopIndex: 3}, ranges[1])
}

// T012: Capture-vs-vars conflict and forward-reference detection tests

func TestScenario_Validate_CaptureVarsConflict(t *testing.T) {
	scn := Scenario{
		Meta: Meta{
			Name: "conflict-test",
			Vars: map[string]string{"region": "eastus"},
		},
		Steps: []StepElement{
			{Step: &Step{
				Match:   Match{Argv: []string{"cmd"}},
				Respond: Response{Exit: 0, Capture: map[string]string{"region": "westus"}},
			}},
		},
	}
	err := scn.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "capture identifier \"region\" conflicts with meta.vars key")
}

func TestScenario_Validate_CaptureNoConflict(t *testing.T) {
	scn := Scenario{
		Meta: Meta{
			Name: "no-conflict",
			Vars: map[string]string{"region": "eastus"},
		},
		Steps: []StepElement{
			{Step: &Step{
				Match:   Match{Argv: []string{"cmd"}},
				Respond: Response{Exit: 0, Capture: map[string]string{"rg_id": "val"}},
			}},
		},
	}
	assert.NoError(t, scn.Validate())
}

func TestScenario_Validate_ForwardReference(t *testing.T) {
	scn := Scenario{
		Meta: Meta{Name: "forward-ref"},
		Steps: []StepElement{
			{Step: &Step{
				Match:   Match{Argv: []string{"cmd1"}},
				Respond: Response{Exit: 0, Stdout: "val={{ .capture.vm_id }}"},
			}},
			{Step: &Step{
				Match:   Match{Argv: []string{"cmd2"}},
				Respond: Response{Exit: 0, Capture: map[string]string{"vm_id": "vm-1"}},
			}},
		},
	}
	err := scn.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "step 0 references capture \"vm_id\" first defined at step 1 (forward reference)")
}

func TestScenario_Validate_NoForwardReference(t *testing.T) {
	scn := Scenario{
		Meta: Meta{Name: "no-forward-ref"},
		Steps: []StepElement{
			{Step: &Step{
				Match:   Match{Argv: []string{"cmd1"}},
				Respond: Response{Exit: 0, Capture: map[string]string{"rg_id": "val"}},
			}},
			{Step: &Step{
				Match:   Match{Argv: []string{"cmd2"}},
				Respond: Response{Exit: 0, Stdout: "rg={{ .capture.rg_id }}"},
			}},
		},
	}
	assert.NoError(t, scn.Validate())
}

func TestScenario_Validate_CaptureInStderr(t *testing.T) {
	scn := Scenario{
		Meta: Meta{Name: "stderr-forward-ref"},
		Steps: []StepElement{
			{Step: &Step{
				Match:   Match{Argv: []string{"cmd1"}},
				Respond: Response{Exit: 0, Stderr: "err={{ .capture.x }}"},
			}},
			{Step: &Step{
				Match:   Match{Argv: []string{"cmd2"}},
				Respond: Response{Exit: 0, Capture: map[string]string{"x": "val"}},
			}},
		},
	}
	err := scn.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forward reference")
}

func TestScenario_Validate_UndefinedCaptureNotAnError(t *testing.T) {
	// Referencing a capture that is never defined is NOT a validation error
	// (it will resolve to empty string at runtime for unordered groups/optional steps)
	scn := Scenario{
		Meta: Meta{Name: "undefined-capture"},
		Steps: []StepElement{
			{Step: &Step{
				Match:   Match{Argv: []string{"cmd"}},
				Respond: Response{Exit: 0, Stdout: "val={{ .capture.nonexistent }}"},
			}},
		},
	}
	assert.NoError(t, scn.Validate())
}

func TestScenario_Validate_CaptureWithGroup(t *testing.T) {
	// Capture defined in a group step, referenced by a later ordered step — should pass
	scn := Scenario{
		Meta: Meta{Name: "group-capture"},
		Steps: []StepElement{
			{Group: &StepGroup{
				Mode: "unordered",
				Name: "setup",
				Steps: []StepElement{
					{Step: &Step{
						Match:   Match{Argv: []string{"cmd1"}},
						Respond: Response{Exit: 0, Capture: map[string]string{"id": "val"}},
					}},
				},
			}},
			{Step: &Step{
				Match:   Match{Argv: []string{"cmd2"}},
				Respond: Response{Exit: 0, Stdout: "id={{ .capture.id }}"},
			}},
		},
	}
	assert.NoError(t, scn.Validate())
}

func TestExtractCaptureRefs(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		want []string
	}{
		{"simple ref", "{{ .capture.rg_id }}", []string{"rg_id"}},
		{"multiple refs", "{{ .capture.a }} and {{ .capture.b }}", []string{"a", "b"}},
		{"no refs", "plain text {{ .name }}", nil},
		{"empty string", "", nil},
		{"nested in text", "prefix {{ .capture.x }} suffix", []string{"x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := extractCaptureRefs(tt.tmpl)
			assert.Equal(t, tt.want, refs)
		})
	}
}

func TestCaptureYAMLParsing(t *testing.T) {
	yamlContent := `
meta:
  name: capture-yaml-test
steps:
  - match:
      argv: ["az", "group", "create"]
    respond:
      exit: 0
      stdout: '{"id": "rg-1"}'
      capture:
        rg_id: "rg-1"
        rg_name: "demo-rg"
`
	var scn Scenario
	err := yaml.Unmarshal([]byte(yamlContent), &scn)
	require.NoError(t, err)
	require.Len(t, scn.Steps, 1)
	require.NotNil(t, scn.Steps[0].Step)
	assert.Equal(t, map[string]string{"rg_id": "rg-1", "rg_name": "demo-rg"}, scn.Steps[0].Step.Respond.Capture)
}

func TestCaptureYAMLParsing_NoCaptureField(t *testing.T) {
	yamlContent := `
meta:
  name: no-capture
steps:
  - match:
      argv: ["cmd"]
    respond:
      exit: 0
      stdout: "hello"
`
	var scn Scenario
	err := yaml.Unmarshal([]byte(yamlContent), &scn)
	require.NoError(t, err)
	assert.Nil(t, scn.Steps[0].Step.Respond.Capture)
}
