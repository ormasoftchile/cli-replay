package runner

import (
	"os"
	"runtime"
	"strings"
)

// BuildChildEnv returns a copy of the current process's environment with:
//   - PATH prepended with interceptDir
//   - CLI_REPLAY_SESSION set to sessionID
//   - CLI_REPLAY_SCENARIO set to scenarioPath
//
// The returned slice is suitable for use as exec.Cmd.Env.
func BuildChildEnv(interceptDir, sessionID, scenarioPath string) []string {
	base := os.Environ()
	result := make([]string, 0, len(base)+3)

	pathSep := ":"
	pathKey := "PATH"
	if runtime.GOOS == "windows" {
		pathSep = ";"
	}

	foundPath := false
	foundSession := false
	foundScenario := false

	for _, env := range base {
		key, _, ok := splitEnvVar(env)
		if !ok {
			result = append(result, env)
			continue
		}

		upperKey := strings.ToUpper(key)

		switch upperKey {
		case pathKey:
			// Prepend intercept dir to existing PATH
			_, val, _ := splitEnvVar(env)
			result = append(result, key+"="+interceptDir+pathSep+val)
			foundPath = true
		case "CLI_REPLAY_SESSION":
			result = append(result, "CLI_REPLAY_SESSION="+sessionID)
			foundSession = true
		case "CLI_REPLAY_SCENARIO":
			result = append(result, "CLI_REPLAY_SCENARIO="+scenarioPath)
			foundScenario = true
		default:
			result = append(result, env)
		}
	}

	// Add any vars that weren't already in the environment
	if !foundPath {
		result = append(result, pathKey+"="+interceptDir)
	}
	if !foundSession {
		result = append(result, "CLI_REPLAY_SESSION="+sessionID)
	}
	if !foundScenario {
		result = append(result, "CLI_REPLAY_SCENARIO="+scenarioPath)
	}

	return result
}

// splitEnvVar splits an environment variable string "KEY=VALUE" into key and value.
// Returns false if the string doesn't contain '='.
func splitEnvVar(env string) (key, value string, ok bool) {
	idx := strings.IndexByte(env, '=')
	if idx < 0 {
		return "", "", false
	}
	return env[:idx], env[idx+1:], true
}
