// Package platform defines the OS abstraction layer for cli-replay.
// All platform-specific behavior (shim generation, shell execution, command
// resolution, intercept creation) is encapsulated behind the Platform interface.
// Concrete implementations are selected at compile time via Go build tags.
package platform

import (
	"os"
	"os/exec"
)

// ShimFile represents a generated shim file (or file pair on Windows).
// On Unix, only EntryPointPath and Content are populated.
// On Windows, CompanionPath and CompanionContent are also populated
// for the .ps1 companion script.
type ShimFile struct {
	EntryPointPath   string      // Absolute path to the discoverable shim (e.g., <shimDir>/kubectl or <shimDir>/kubectl.cmd)
	CompanionPath    string      // Absolute path to companion script (Windows only: <shimDir>/_kubectl_shim.ps1)
	Command          string      // Target command name (e.g., "kubectl")
	Content          string      // Script content for the entry-point
	CompanionContent string      // Script content for the companion (Windows only)
	FileMode         os.FileMode // Permission bits (0755 Unix, 0644 Windows)
}

// ShimGenerator produces platform-native interception scripts.
type ShimGenerator interface {
	// GenerateShim returns a ShimFile with the entry-point content (and optional
	// companion content on Windows) for intercepting the given command.
	GenerateShim(command, logPath, shimDir string) (*ShimFile, error)

	// ShimFileName returns the platform-appropriate filename for a shim
	// (e.g., "kubectl" on Unix, "kubectl.cmd" on Windows).
	ShimFileName(command string) string

	// ShimFileMode returns the OS file permissions for generated shims.
	ShimFileMode() os.FileMode
}

// ShellExecutor wraps command execution in the native shell.
type ShellExecutor interface {
	// WrapCommand returns an exec.Cmd that runs args through the native shell
	// (bash -c on Unix, powershell -NoProfile -Command on Windows).
	WrapCommand(args []string, env []string) *exec.Cmd
}

// CommandResolver locates real binaries on PATH.
type CommandResolver interface {
	// Resolve returns the absolute path to the real binary for command,
	// excluding any binary found inside excludeDir (the shim directory).
	Resolve(command string, excludeDir string) (string, error)
}

// InterceptFactory creates command intercept entries for replay.
type InterceptFactory interface {
	// CreateIntercept creates a wrapper or symlink at targetDir that delegates
	// to binaryPath when command is invoked.
	CreateIntercept(binaryPath, targetDir, command string) (string, error)

	// InterceptFileName returns the platform-appropriate filename for an
	// intercept entry (e.g., "kubectl" on Unix, "kubectl.cmd" on Windows).
	InterceptFileName(command string) string
}

// Platform is the composite interface grouping all OS-specific strategies.
// Obtained via New() which is defined in build-tagged files.
type Platform interface {
	ShimGenerator
	ShellExecutor
	CommandResolver
	InterceptFactory

	// Name returns a human-readable platform identifier ("unix" or "windows").
	Name() string
}
