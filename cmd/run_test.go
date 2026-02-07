package cmd

import (
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
		Steps: []scenario.Step{
			{Match: scenario.Match{Argv: []string{"kubectl", "get", "pods"}}},
			{Match: scenario.Match{Argv: []string{"kubectl", "apply", "-f", "-"}}},
		},
	}

	err := validateAllowlist(scn, []string{"kubectl"}, nil)
	require.NoError(t, err)
}

func TestValidateAllowlist_DisallowedRejected(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{Name: "deploy-test"},
		Steps: []scenario.Step{
			{Match: scenario.Match{Argv: []string{"kubectl", "get", "pods"}}},
			{Match: scenario.Match{Argv: []string{"docker", "build", "-t", "myapp"}}},
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
		Steps: []scenario.Step{
			{Match: scenario.Match{Argv: []string{"/usr/local/bin/kubectl", "get", "pods"}}},
		},
	}

	err := validateAllowlist(scn, []string{"kubectl"}, nil)
	require.NoError(t, err)
}

func TestValidateAllowlist_EmptyListAllowsAll(t *testing.T) {
	scn := &scenario.Scenario{
		Meta: scenario.Meta{Name: "test"},
		Steps: []scenario.Step{
			{Match: scenario.Match{Argv: []string{"any-command"}}},
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
		Steps: []scenario.Step{
			{Match: scenario.Match{Argv: []string{"kubectl", "get", "pods"}}},
		},
	}

	// kubectl is in both lists → allowed
	err := validateAllowlist(scn, []string{"kubectl", "docker"}, []string{"kubectl", "helm"})
	require.NoError(t, err)

	// kubectl is only in YAML but not in CLI intersection → rejected
	scnDocker := &scenario.Scenario{
		Meta: scenario.Meta{Name: "test"},
		Steps: []scenario.Step{
			{Match: scenario.Match{Argv: []string{"docker", "build"}}},
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
		Steps: []scenario.Step{
			{Match: scenario.Match{Argv: []string{"Kubectl.exe", "get", "pods"}}},
		},
	}

	err := validateAllowlist(scn, []string{"kubectl.exe"}, nil)
	require.NoError(t, err)
}
