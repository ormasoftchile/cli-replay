// Package testutil provides test helpers for the platform package.
package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cli-replay/cli-replay/internal/platform"
)

// FakePlatform is a configurable test double implementing platform.Platform.
// Test authors set the function fields to control behavior per test case.
type FakePlatform struct {
	// NameValue is returned by Name(). Default: "fake".
	NameValue string

	// GenerateShimFunc overrides GenerateShim. If nil, returns a minimal ShimFile.
	GenerateShimFunc func(command, logPath, shimDir string) (*platform.ShimFile, error)

	// ShimFileNameFunc overrides ShimFileName. If nil, returns command as-is.
	ShimFileNameFunc func(command string) string

	// ShimFileModeValue is returned by ShimFileMode(). Default: 0755.
	ShimFileModeValue os.FileMode

	// WrapCommandFunc overrides WrapCommand. If nil, returns exec.Command(args[0], args[1:]...).
	WrapCommandFunc func(args []string, env []string) *exec.Cmd

	// ResolveFunc overrides Resolve. If nil, returns "/fake/bin/<command>".
	ResolveFunc func(command string, excludeDir string) (string, error)

	// CreateInterceptFunc overrides CreateIntercept. If nil, returns targetDir/command.
	CreateInterceptFunc func(binaryPath, targetDir, command string) (string, error)

	// InterceptFileNameFunc overrides InterceptFileName. If nil, returns command as-is.
	InterceptFileNameFunc func(command string) string

	// Calls tracks method invocations for assertion.
	Calls []Call
}

// Call records a single method invocation on FakePlatform.
type Call struct {
	Method string
	Args   []string
}

// NewFakePlatform returns a FakePlatform with sensible defaults.
func NewFakePlatform() *FakePlatform {
	return &FakePlatform{
		NameValue:         "fake",
		ShimFileModeValue: 0755,
	}
}

// Name returns the configured platform name.
func (f *FakePlatform) Name() string {
	return f.NameValue
}

// GenerateShim creates a minimal shim or delegates to GenerateShimFunc.
func (f *FakePlatform) GenerateShim(command, logPath, shimDir string) (*platform.ShimFile, error) {
	f.Calls = append(f.Calls, Call{Method: "GenerateShim", Args: []string{command, logPath, shimDir}})
	if f.GenerateShimFunc != nil {
		return f.GenerateShimFunc(command, logPath, shimDir)
	}
	if command == "" {
		return nil, fmt.Errorf("command must be non-empty")
	}
	return &platform.ShimFile{
		EntryPointPath: filepath.Join(shimDir, f.ShimFileName(command)),
		Command:        command,
		Content:        fmt.Sprintf("#!/bin/sh\n# fake shim for %s\n", command),
		FileMode:       f.ShimFileModeValue,
	}, nil
}

// ShimFileName returns the shim filename or delegates to ShimFileNameFunc.
func (f *FakePlatform) ShimFileName(command string) string {
	if f.ShimFileNameFunc != nil {
		return f.ShimFileNameFunc(command)
	}
	return command
}

// ShimFileMode returns the configured file mode.
func (f *FakePlatform) ShimFileMode() os.FileMode {
	return f.ShimFileModeValue
}

// WrapCommand returns an exec.Cmd or delegates to WrapCommandFunc.
func (f *FakePlatform) WrapCommand(args []string, env []string) *exec.Cmd {
	f.Calls = append(f.Calls, Call{Method: "WrapCommand", Args: args})
	if f.WrapCommandFunc != nil {
		return f.WrapCommandFunc(args, env)
	}
	// Default: run args[0] directly (cross-platform safe for tests)
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec // test helper
	if len(env) > 0 {
		cmd.Env = env
	}
	return cmd
}

// Resolve returns the real binary path or delegates to ResolveFunc.
func (f *FakePlatform) Resolve(command string, excludeDir string) (string, error) {
	f.Calls = append(f.Calls, Call{Method: "Resolve", Args: []string{command, excludeDir}})
	if f.ResolveFunc != nil {
		return f.ResolveFunc(command, excludeDir)
	}
	return filepath.Join("/fake/bin", command), nil
}

// CreateIntercept creates an intercept or delegates to CreateInterceptFunc.
func (f *FakePlatform) CreateIntercept(binaryPath, targetDir, command string) (string, error) {
	f.Calls = append(f.Calls, Call{Method: "CreateIntercept", Args: []string{binaryPath, targetDir, command}})
	if f.CreateInterceptFunc != nil {
		return f.CreateInterceptFunc(binaryPath, targetDir, command)
	}
	return filepath.Join(targetDir, command), nil
}

// InterceptFileName returns the intercept filename or delegates to InterceptFileNameFunc.
func (f *FakePlatform) InterceptFileName(command string) string {
	if f.InterceptFileNameFunc != nil {
		return f.InterceptFileNameFunc(command)
	}
	return command
}

// CallCount returns the number of times a method was called.
func (f *FakePlatform) CallCount(method string) int {
	count := 0
	for _, c := range f.Calls {
		if c.Method == method {
			count++
		}
	}
	return count
}

// CalledWith returns true if the method was called with the given args (prefix match).
func (f *FakePlatform) CalledWith(method string, args ...string) bool {
	for _, c := range f.Calls {
		if c.Method != method {
			continue
		}
		if len(args) > len(c.Args) {
			continue
		}
		match := true
		for i, a := range args {
			if !strings.Contains(c.Args[i], a) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// Verify compile-time interface compliance.
var _ platform.Platform = (*FakePlatform)(nil)
