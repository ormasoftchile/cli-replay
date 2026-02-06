package scenario

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				Steps: []Step{
					{
						Match:   Match{Argv: []string{"kubectl", "get", "pods"}},
						Respond: Response{Exit: 0, Stdout: "pod-output"},
					},
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
				Steps: []Step{
					{
						Match:   Match{Argv: []string{"cmd1"}},
						Respond: Response{Exit: 0},
					},
					{
						Match:   Match{Argv: []string{"cmd2", "arg"}},
						Respond: Response{Exit: 1, Stderr: "error"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty steps",
			scenario: Scenario{
				Meta:  Meta{Name: "empty"},
				Steps: []Step{},
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

//nolint:funlen // Table-driven test with comprehensive test cases
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
