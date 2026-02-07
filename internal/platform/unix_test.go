//go:build !windows

package platform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnixPlatform_Name(t *testing.T) {
	p := New()
	assert.Equal(t, "unix", p.Name())
}

func TestUnixPlatform_GenerateShim(t *testing.T) {
	p := New()
	shimDir := t.TempDir()
	logPath := filepath.Join(shimDir, "recording.jsonl")

	shim, err := p.GenerateShim("kubectl", logPath, shimDir)
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(shimDir, "kubectl"), shim.EntryPointPath)
	assert.Equal(t, "kubectl", shim.Command)
	assert.Contains(t, shim.Content, "#!/usr/bin/env bash")
	assert.Contains(t, shim.Content, "kubectl")
	assert.Contains(t, shim.Content, logPath)
	assert.Contains(t, shim.Content, shimDir)
	assert.Empty(t, shim.CompanionPath, "Unix shims have no companion")
	assert.Empty(t, shim.CompanionContent, "Unix shims have no companion content")
	assert.Equal(t, os.FileMode(0755), shim.FileMode)
}

func TestUnixPlatform_GenerateShim_Errors(t *testing.T) {
	p := New()
	tests := []struct {
		name    string
		cmd     string
		log     string
		dir     string
		wantErr string
	}{
		{"empty command", "", "/tmp/log.jsonl", "/tmp/shims", "command must be non-empty"},
		{"empty logPath", "kubectl", "", "/tmp/shims", "logPath must be non-empty"},
		{"empty shimDir", "kubectl", "/tmp/log.jsonl", "", "shimDir must be non-empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.GenerateShim(tt.cmd, tt.log, tt.dir)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestUnixPlatform_ShimFileName(t *testing.T) {
	p := New()
	assert.Equal(t, "kubectl", p.ShimFileName("kubectl"))
	assert.Equal(t, "docker-compose", p.ShimFileName("docker-compose"))
}

func TestUnixPlatform_ShimFileMode(t *testing.T) {
	p := New()
	assert.Equal(t, os.FileMode(0755), p.ShimFileMode())
}

func TestUnixPlatform_WrapCommand(t *testing.T) {
	p := New()
	cmd := p.WrapCommand([]string{"echo", "hello"}, nil)

	assert.Equal(t, "bash", filepath.Base(cmd.Path))
	assert.Contains(t, cmd.Args, "-c")
	assert.Contains(t, cmd.Args, "echo hello")
}

func TestUnixPlatform_WrapCommand_WithEnv(t *testing.T) {
	p := New()
	env := []string{"FOO=bar", "PATH=/usr/bin"}
	cmd := p.WrapCommand([]string{"echo"}, env)

	assert.Equal(t, env, cmd.Env)
}

func TestUnixPlatform_Resolve(t *testing.T) {
	p := New()
	// 'sh' should be resolvable on any Unix system
	resolved, err := p.Resolve("sh", "")
	require.NoError(t, err)
	assert.NotEmpty(t, resolved)
	assert.True(t, filepath.IsAbs(resolved))
}

func TestUnixPlatform_Resolve_ExcludeDir(t *testing.T) {
	p := New()
	// Create a shim dir with a fake 'sh', then resolve excluding it
	shimDir := t.TempDir()
	fakeShim := filepath.Join(shimDir, "sh")
	require.NoError(t, os.WriteFile(fakeShim, []byte("#!/bin/sh\n"), 0755))

	// Resolve should find the real 'sh', not the one in shimDir
	resolved, err := p.Resolve("sh", shimDir)
	require.NoError(t, err)
	assert.NotEqual(t, fakeShim, resolved, "should not resolve to the excluded directory")
}

func TestUnixPlatform_Resolve_NotFound(t *testing.T) {
	p := New()
	_, err := p.Resolve("nonexistent-command-xyz-12345", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command not found")
}

func TestUnixPlatform_Resolve_EmptyCommand(t *testing.T) {
	p := New()
	_, err := p.Resolve("", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command must be non-empty")
}

func TestUnixPlatform_CreateIntercept(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fake binary
	binaryPath := filepath.Join(tmpDir, "cli-replay")
	require.NoError(t, os.WriteFile(binaryPath, []byte("#!/bin/sh\n"), 0755))

	targetDir := filepath.Join(tmpDir, "intercepts")
	require.NoError(t, os.MkdirAll(targetDir, 0755))

	p := New()
	linkPath, err := p.CreateIntercept(binaryPath, targetDir, "kubectl")
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(targetDir, "kubectl"), linkPath)

	// Verify it's a symlink
	info, err := os.Lstat(linkPath)
	require.NoError(t, err)
	assert.True(t, info.Mode()&os.ModeSymlink != 0, "should be a symlink")

	// Verify symlink target
	target, err := os.Readlink(linkPath)
	require.NoError(t, err)
	assert.Equal(t, binaryPath, target)
}

func TestUnixPlatform_CreateIntercept_BinaryNotFound(t *testing.T) {
	p := New()
	_, err := p.CreateIntercept("/nonexistent/cli-replay", t.TempDir(), "kubectl")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cli-replay binary not found")
}

func TestUnixPlatform_CreateIntercept_DirNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "cli-replay")
	require.NoError(t, os.WriteFile(binaryPath, []byte("#!/bin/sh\n"), 0755))

	p := New()
	_, err := p.CreateIntercept(binaryPath, "/nonexistent/dir", "kubectl")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "intercept directory does not exist")
}

func TestUnixPlatform_InterceptFileName(t *testing.T) {
	p := New()
	assert.Equal(t, "kubectl", p.InterceptFileName("kubectl"))
}

func TestUnixPlatform_ShimWriteAndExecute(t *testing.T) {
	p := New()
	shimDir := t.TempDir()
	logPath := filepath.Join(shimDir, "recording.jsonl")

	shim, err := p.GenerateShim("echo", logPath, shimDir)
	require.NoError(t, err)

	// Write the shim to disk
	err = os.WriteFile(shim.EntryPointPath, []byte(shim.Content), shim.FileMode)
	require.NoError(t, err)

	// Verify it's executable
	info, err := os.Stat(shim.EntryPointPath)
	require.NoError(t, err)
	assert.NotEqual(t, os.FileMode(0), info.Mode()&0111, "shim should be executable")
}
