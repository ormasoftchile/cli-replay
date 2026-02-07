//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWindowsPlatform_Name(t *testing.T) {
	p := New()
	assert.Equal(t, "windows", p.Name())
}

func TestWindowsPlatform_GenerateShim(t *testing.T) {
	p := New()
	shimDir := t.TempDir()
	logPath := filepath.Join(shimDir, "recording.jsonl")

	shim, err := p.GenerateShim("kubectl", logPath, shimDir)
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(shimDir, "kubectl.cmd"), shim.EntryPointPath)
	assert.Equal(t, filepath.Join(shimDir, "_kubectl_shim.ps1"), shim.CompanionPath)
	assert.Equal(t, "kubectl", shim.Command)
	assert.Contains(t, shim.Content, "@echo off")
	assert.Contains(t, shim.Content, "powershell.exe")
	assert.NotEmpty(t, shim.CompanionContent, "Windows shims must have companion content")
	assert.Contains(t, shim.CompanionContent, "kubectl")
	assert.Contains(t, shim.CompanionContent, logPath)
	assert.Equal(t, os.FileMode(0644), shim.FileMode)
}

func TestWindowsPlatform_GenerateShim_Errors(t *testing.T) {
	p := New()
	tests := []struct {
		name    string
		cmd     string
		log     string
		dir     string
		wantErr string
	}{
		{"empty command", "", "C:\\tmp\\log.jsonl", "C:\\tmp\\shims", "command must be non-empty"},
		{"empty logPath", "kubectl", "", "C:\\tmp\\shims", "logPath must be non-empty"},
		{"empty shimDir", "kubectl", "C:\\tmp\\log.jsonl", "", "shimDir must be non-empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.GenerateShim(tt.cmd, tt.log, tt.dir)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestWindowsPlatform_GenerateShim_PathSeparators(t *testing.T) {
	p := New()
	shimDir := t.TempDir()
	// Use forward slashes to verify normalization
	logPath := shimDir + "/recording.jsonl"

	shim, err := p.GenerateShim("kubectl", logPath, shimDir)
	require.NoError(t, err)

	// Entry-point and companion should use backslashes
	assert.Contains(t, shim.EntryPointPath, string(os.PathSeparator))
	assert.Contains(t, shim.CompanionPath, string(os.PathSeparator))
}

func TestWindowsPlatform_ShimFileName(t *testing.T) {
	p := New()
	assert.Equal(t, "kubectl.cmd", p.ShimFileName("kubectl"))
	assert.Equal(t, "docker-compose.cmd", p.ShimFileName("docker-compose"))
}

func TestWindowsPlatform_ShimFileMode(t *testing.T) {
	p := New()
	assert.Equal(t, os.FileMode(0644), p.ShimFileMode())
}

func TestWindowsPlatform_WrapCommand(t *testing.T) {
	p := New()
	cmd := p.WrapCommand([]string{"echo", "hello"}, nil)

	assert.Contains(t, strings.ToLower(filepath.Base(cmd.Path)), "powershell")
	assert.Contains(t, cmd.Args, "-NoProfile")
	assert.Contains(t, cmd.Args, "-ExecutionPolicy")
	assert.Contains(t, cmd.Args, "Bypass")
}

func TestWindowsPlatform_WrapCommand_WithEnv(t *testing.T) {
	p := New()
	env := []string{"FOO=bar", "PATH=C:\\Windows\\system32"}
	cmd := p.WrapCommand([]string{"echo"}, env)

	assert.Equal(t, env, cmd.Env)
}

func TestWindowsPlatform_WrapCommand_SpecialChars(t *testing.T) {
	p := New()
	cmd := p.WrapCommand([]string{"echo", "hello world", "foo&bar"}, nil)

	// The -Command argument should contain properly quoted args
	found := false
	for _, arg := range cmd.Args {
		if strings.Contains(arg, "hello world") {
			found = true
		}
	}
	assert.True(t, found, "WrapCommand should handle args with spaces")
}

func TestWindowsPlatform_Resolve(t *testing.T) {
	p := New()
	// 'cmd.exe' should be resolvable on any Windows system
	resolved, err := p.Resolve("cmd", "")
	require.NoError(t, err)
	assert.NotEmpty(t, resolved)
	assert.True(t, filepath.IsAbs(resolved))
}

func TestWindowsPlatform_Resolve_ExcludeDir(t *testing.T) {
	p := New()
	shimDir := t.TempDir()

	// Create a fake cmd.exe shim
	fakeShim := filepath.Join(shimDir, "cmd.exe")
	require.NoError(t, os.WriteFile(fakeShim, []byte("fake"), 0644))

	// Resolve should find the real cmd.exe, not the fake one
	resolved, err := p.Resolve("cmd", shimDir)
	require.NoError(t, err)
	assert.NotEqual(t, fakeShim, resolved, "should not resolve to the excluded directory")
}

func TestWindowsPlatform_Resolve_NotFound(t *testing.T) {
	p := New()
	_, err := p.Resolve("nonexistent-command-xyz-12345", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command not found")
}

func TestWindowsPlatform_Resolve_EmptyCommand(t *testing.T) {
	p := New()
	_, err := p.Resolve("", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command must be non-empty")
}

func TestWindowsPlatform_CreateIntercept(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fake binary
	binaryPath := filepath.Join(tmpDir, "cli-replay.exe")
	require.NoError(t, os.WriteFile(binaryPath, []byte("fake binary"), 0644))

	targetDir := filepath.Join(tmpDir, "intercepts")
	require.NoError(t, os.MkdirAll(targetDir, 0755))

	p := New()
	cmdPath, err := p.CreateIntercept(binaryPath, targetDir, "kubectl")
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(targetDir, "kubectl.cmd"), cmdPath)

	// Verify .cmd file content
	content, err := os.ReadFile(cmdPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "@echo off")
	assert.Contains(t, string(content), binaryPath)
	assert.Contains(t, string(content), "%*")
}

func TestWindowsPlatform_CreateIntercept_BinaryNotFound(t *testing.T) {
	p := New()
	_, err := p.CreateIntercept("C:\\nonexistent\\cli-replay.exe", t.TempDir(), "kubectl")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cli-replay binary not found")
}

func TestWindowsPlatform_CreateIntercept_DirNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "cli-replay.exe")
	require.NoError(t, os.WriteFile(binaryPath, []byte("fake binary"), 0644))

	p := New()
	_, err := p.CreateIntercept(binaryPath, "C:\\nonexistent\\dir", "kubectl")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "intercept directory does not exist")
}

func TestWindowsPlatform_InterceptFileName(t *testing.T) {
	p := New()
	assert.Equal(t, "kubectl.cmd", p.InterceptFileName("kubectl"))
}

func TestWindowsPlatform_ShimDualWrite(t *testing.T) {
	p := New()
	shimDir := t.TempDir()
	logPath := filepath.Join(shimDir, "recording.jsonl")

	shim, err := p.GenerateShim("kubectl", logPath, shimDir)
	require.NoError(t, err)

	// Write both entry-point and companion
	err = os.WriteFile(shim.EntryPointPath, []byte(shim.Content), shim.FileMode)
	require.NoError(t, err)

	err = os.WriteFile(shim.CompanionPath, []byte(shim.CompanionContent), shim.FileMode)
	require.NoError(t, err)

	// Verify both files exist
	assert.FileExists(t, shim.EntryPointPath)
	assert.FileExists(t, shim.CompanionPath)

	// Verify .cmd references the .ps1
	cmdContent, _ := os.ReadFile(shim.EntryPointPath)
	assert.Contains(t, string(cmdContent), "_shim.ps1")
}

func TestWindowsPlatform_DetectExecutionPolicyError(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		want   bool
	}{
		{"no error", "some normal output", false},
		{"scripts disabled", "running scripts is disabled on this system", true},
		{"not digitally signed", "the file is not digitally signed", true},
		{"security exception", "PSSecurityException: blah", true},
		{"unrelated error", "file not found", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectExecutionPolicyError(tt.stderr)
			if tt.want {
				assert.NotEmpty(t, result, "should detect execution policy error")
				assert.Contains(t, result, "Remediation")
			} else {
				assert.Empty(t, result, "should not detect execution policy error")
			}
		})
	}
}

func TestWindowsPlatform_FilterPath_CaseInsensitive(t *testing.T) {
	// Windows paths are case-insensitive
	pathEnv := `C:\Users\dev\shims;C:\Windows\system32;C:\Go\bin`
	filtered := filterPath(pathEnv, `c:\users\dev\shims`)
	assert.NotContains(t, filtered, "shims")
	assert.Contains(t, filtered, "system32")
	assert.Contains(t, filtered, "Go")
}
