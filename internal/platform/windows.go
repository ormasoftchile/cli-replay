//go:build windows

package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// cmdShimTemplate is the .cmd entry-point that invokes the companion .ps1 script.
// It is discovered via PATHEXT (which includes .CMD by default on Windows 10+).
// The %~dp0 expands to the drive+path of the .cmd file itself.
// The %~n0 expands to the name without extension.
const cmdShimTemplate = "@echo off\r\npowershell.exe -ExecutionPolicy Bypass -NoProfile -File \"%%~dp0_%%~n0_shim.ps1\" %%*\r\nexit /B %%ERRORLEVEL%%\r\n"

// ps1ShimTemplate is the PowerShell companion script that does the actual
// interception work: finds the real command, captures output, and logs to JSONL.
// NOTE: PowerShell uses backtick (`) as escape character, but Go raw strings
// are delimited by backticks. We use [char]13/[char]10/[char]9 instead.
var ps1ShimTemplate = "# cli-replay shim for: %s\r\n" +
	"# This script intercepts the command and logs execution details to JSONL\r\n" +
	"param([Parameter(ValueFromRemainingArguments=$true)]$ShimArgs)\r\n" +
	"\r\n" +
	"# Prevent recursive shim execution\r\n" +
	"if ($env:CLI_REPLAY_IN_SHIM -eq \"1\") {\r\n" +
	"    $filteredPath = ($env:PATH -split ';' | Where-Object { $_ -ne '%s' }) -join ';'\r\n" +
	"    $env:PATH = $filteredPath\r\n" +
	"    $realCmd = (Get-Command -Name '%s' -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1).Source\r\n" +
	"    if ($realCmd) {\r\n" +
	"        & $realCmd @ShimArgs\r\n" +
	"        exit $LASTEXITCODE\r\n" +
	"    }\r\n" +
	"    Write-Error \"cli-replay: command not found: %s\"\r\n" +
	"    exit 127\r\n" +
	"}\r\n" +
	"$env:CLI_REPLAY_IN_SHIM = \"1\"\r\n" +
	"\r\n" +
	"$LogFile = '%s'\r\n" +
	"$ShimDir = '%s'\r\n" +
	"\r\n" +
	"# Find real command by filtering shim directory from PATH\r\n" +
	"$filteredPath = ($env:PATH -split ';' | Where-Object { $_ -ne $ShimDir }) -join ';'\r\n" +
	"$savedPath = $env:PATH\r\n" +
	"$env:PATH = $filteredPath\r\n" +
	"$realCommand = (Get-Command -Name '%s' -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1).Source\r\n" +
	"$env:PATH = $savedPath\r\n" +
	"\r\n" +
	"if (-not $realCommand) {\r\n" +
	"    Write-Error \"cli-replay: command not found: %s\"\r\n" +
	"    exit 127\r\n" +
	"}\r\n" +
	"\r\n" +
	"# Capture start time (RFC3339 format)\r\n" +
	"$timestamp = (Get-Date).ToUniversalTime().ToString(\"yyyy-MM-ddTHH:mm:ssZ\")\r\n" +
	"\r\n" +
	"# Execute the real command and capture output\r\n" +
	"$exitCode = 0\r\n" +
	"\r\n" +
	"try {\r\n" +
	"    $psi = New-Object System.Diagnostics.ProcessStartInfo\r\n" +
	"    $psi.FileName = $realCommand\r\n" +
	"    $psi.Arguments = ($ShimArgs | ForEach-Object {\r\n" +
	"        if ($_ -match '\\s') { '\"' + $_ + '\"' } else { $_ }\r\n" +
	"    }) -join ' '\r\n" +
	"    $psi.UseShellExecute = $false\r\n" +
	"    $psi.RedirectStandardOutput = $true\r\n" +
	"    $psi.RedirectStandardError = $true\r\n" +
	"    $psi.CreateNoWindow = $true\r\n" +
	"\r\n" +
	"    $process = [System.Diagnostics.Process]::Start($psi)\r\n" +
	"    $stdoutTask = $process.StandardOutput.ReadToEndAsync()\r\n" +
	"    $stderrTask = $process.StandardError.ReadToEndAsync()\r\n" +
	"    $process.WaitForExit()\r\n" +
	"\r\n" +
	"    $stdoutContent = $stdoutTask.Result\r\n" +
	"    $stderrContent = $stderrTask.Result\r\n" +
	"    $exitCode = $process.ExitCode\r\n" +
	"} catch {\r\n" +
	"    Write-Error \"cli-replay: failed to execute: $_\"\r\n" +
	"    exit 127\r\n" +
	"}\r\n" +
	"\r\n" +
	"# Echo output to preserve command behavior\r\n" +
	"if ($stdoutContent) { [Console]::Out.Write($stdoutContent) }\r\n" +
	"if ($stderrContent) { [Console]::Error.Write($stderrContent) }\r\n" +
	"\r\n" +
	"# Build argv JSON array\r\n" +
	"$argvParts = @('\"' + ('%s' -replace '\\\\','\\\\' -replace '\"','\\\"') + '\"')\r\n" +
	"foreach ($arg in $ShimArgs) {\r\n" +
	"    $escaped = ($arg -replace '\\\\','\\\\') -replace '\"','\\\"'\r\n" +
	"    $argvParts += ('\"' + $escaped + '\"')\r\n" +
	"}\r\n" +
	"$argvJson = '[' + ($argvParts -join ',') + ']'\r\n" +
	"\r\n" +
	"# Escape JSON strings\r\n" +
	"$escStdout = ($stdoutContent -replace '\\\\','\\\\' -replace '\"','\\\"' -replace [char]13+[char]10,'\\n' -replace [char]10,'\\n' -replace [char]13,'\\r' -replace [char]9,'\\t')\r\n" +
	"$escStderr = ($stderrContent -replace '\\\\','\\\\' -replace '\"','\\\"' -replace [char]13+[char]10,'\\n' -replace [char]10,'\\n' -replace [char]13,'\\r' -replace [char]9,'\\t')\r\n" +
	"\r\n" +
	"# Write JSONL entry\r\n" +
	"$jsonLine = '{\"timestamp\":\"' + $timestamp + '\",\"argv\":' + $argvJson + ',\"exit\":' + $exitCode + ',\"stdout\":\"' + $escStdout + '\",\"stderr\":\"' + $escStderr + '\"}'\r\n" +
	"Add-Content -Path $LogFile -Value $jsonLine -Encoding UTF8 -NoNewline\r\n" +
	"Add-Content -Path $LogFile -Value ([char]10) -NoNewline\r\n" +
	"\r\n" +
	"exit $exitCode\r\n"

// windowsPlatform implements Platform for Windows systems.
type windowsPlatform struct{}

// newPlatform returns the Windows platform implementation.
// This is the build-tagged factory called by New() on Windows.
func newPlatform() Platform {
	return &windowsPlatform{}
}

// New returns the Platform for the current OS.
func New() Platform {
	return newPlatform()
}

// Name returns "windows".
func (w *windowsPlatform) Name() string {
	return "windows"
}

// GenerateShim creates a .cmd entry-point + companion .ps1 script that
// intercepts command execution on Windows.
func (w *windowsPlatform) GenerateShim(command, logPath, shimDir string) (*ShimFile, error) {
	if command == "" {
		return nil, fmt.Errorf("command must be non-empty")
	}
	if logPath == "" {
		return nil, fmt.Errorf("logPath must be non-empty")
	}
	if shimDir == "" {
		return nil, fmt.Errorf("shimDir must be non-empty")
	}

	// Normalize paths to Windows separators
	logPath = filepath.FromSlash(logPath)
	shimDir = filepath.FromSlash(shimDir)

	// Generate .ps1 companion content
	ps1Content := fmt.Sprintf(ps1ShimTemplate,
		command, // Comment line: shim for
		shimDir, // Guard: PATH filter
		command, // Guard: Get-Command lookup
		command, // Guard: error message
		logPath, // $LogFile variable
		shimDir, // $ShimDir variable
		command, // Get-Command lookup
		command, // Error message
		command, // argv[0] in JSON
	)

	entryPointPath := filepath.Join(shimDir, command+".cmd")
	companionPath := filepath.Join(shimDir, "_"+command+"_shim.ps1")

	return &ShimFile{
		EntryPointPath:   entryPointPath,
		CompanionPath:    companionPath,
		Command:          command,
		Content:          cmdShimTemplate,
		CompanionContent: ps1Content,
		FileMode:         w.ShimFileMode(),
	}, nil
}

// ShimFileName returns the command name with .cmd extension (Windows convention).
func (w *windowsPlatform) ShimFileName(command string) string {
	return command + ".cmd"
}

// ShimFileMode returns 0644 (no execute bit needed on Windows).
func (w *windowsPlatform) ShimFileMode() os.FileMode {
	return 0644
}

// WrapCommand returns an exec.Cmd wrapping args in powershell -NoProfile -Command.
func (w *windowsPlatform) WrapCommand(args []string, env []string) *exec.Cmd {
	// Join args into a single command string for PowerShell
	// Each arg is quoted to handle spaces and special characters
	quotedArgs := make([]string, len(args))
	for i, arg := range args {
		if strings.ContainsAny(arg, " \t\"'&|<>()") {
			quotedArgs[i] = fmt.Sprintf("'%s'", strings.ReplaceAll(arg, "'", "''"))
		} else {
			quotedArgs[i] = arg
		}
	}
	cmdStr := "& " + strings.Join(quotedArgs, " ")
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", cmdStr) //nolint:gosec // user command is intentionally executed
	if len(env) > 0 {
		cmd.Env = env
	}
	return cmd
}

// Resolve locates the real binary for command, excluding excludeDir.
// Uses exec.LookPath with PATH filtering and PATHEXT awareness.
func (w *windowsPlatform) Resolve(command string, excludeDir string) (string, error) {
	if command == "" {
		return "", fmt.Errorf("command must be non-empty")
	}

	// Filter excludeDir out of PATH
	originalPath := os.Getenv("PATH")
	filteredPath := filterPath(originalPath, excludeDir)

	// Temporarily set filtered PATH for LookPath
	os.Setenv("PATH", filteredPath)       //nolint:errcheck // temp set for LookPath
	defer os.Setenv("PATH", originalPath) //nolint:errcheck // restore original PATH

	resolved, err := exec.LookPath(command)
	if err == nil {
		return resolved, nil
	}

	return "", fmt.Errorf("command not found: %s", command)
}

// CreateIntercept creates a .cmd wrapper at targetDir that delegates to binaryPath.
func (w *windowsPlatform) CreateIntercept(binaryPath, targetDir, command string) (string, error) {
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return "", fmt.Errorf("cli-replay binary not found: %s", binaryPath)
	}
	if info, err := os.Stat(targetDir); err != nil || !info.IsDir() {
		return "", fmt.Errorf("intercept directory does not exist: %s", targetDir)
	}

	cmdPath := filepath.Join(targetDir, command+".cmd")
	// The .cmd wrapper delegates all arguments to the cli-replay binary
	content := fmt.Sprintf("@echo off\r\n\"%s\" %%*\r\n", binaryPath)

	if err := os.WriteFile(cmdPath, []byte(content), 0644); err != nil { //nolint:gosec // intercept needs to be readable
		return "", fmt.Errorf("failed to write intercept: %w", err)
	}
	return cmdPath, nil
}

// InterceptFileName returns the command name with .cmd extension.
func (w *windowsPlatform) InterceptFileName(command string) string {
	return command + ".cmd"
}

// filterPath removes excludeDir from a PATH string.
// On Windows, PATH entries are separated by ';'.
func filterPath(pathEnv, excludeDir string) string {
	if excludeDir == "" {
		return pathEnv
	}
	// Normalize for case-insensitive comparison on Windows
	excludeNorm := strings.ToLower(filepath.Clean(excludeDir))
	parts := filepath.SplitList(pathEnv)
	filtered := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.ToLower(filepath.Clean(p)) != excludeNorm {
			filtered = append(filtered, p)
		}
	}
	return strings.Join(filtered, string(os.PathListSeparator))
}

// detectExecutionPolicyError checks if a PowerShell error indicates an
// execution policy restriction that prevents script execution.
func detectExecutionPolicyError(stderr string) string {
	indicators := []string{
		"is not digitally signed",
		"running scripts is disabled",
		"execution of scripts is disabled",
		"AuthorizationManager check failed",
		"PSSecurityException",
	}
	for _, indicator := range indicators {
		if strings.Contains(stderr, indicator) {
			return fmt.Sprintf("PowerShell execution policy is blocking script execution. "+
				"The shim uses -ExecutionPolicy Bypass, but a Group Policy may override this.\n"+
				"Remediation options:\n"+
				"  1. Run: Set-ExecutionPolicy RemoteSigned -Scope CurrentUser\n"+
				"  2. Contact your IT administrator to allow PowerShell script execution\n"+
				"  3. Check current policy: Get-ExecutionPolicy -List\n"+
				"Original error: %s", stderr)
		}
	}
	return ""
}
