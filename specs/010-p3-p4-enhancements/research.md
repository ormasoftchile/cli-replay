# Research: P3/P4 Enhancements — Windows Job Objects, GitHub Action, Cobra Validate

**Date**: 2026-02-07  
**Status**: Complete  
**Scope**: Research only — no code changes

---

## Topic 1: Windows Job Objects via `golang.org/x/sys/windows`

### 1.1 Current State in cli-replay

The current Windows signal handling in `cmd/exec_windows.go` is minimal:

```go
func setupSignalForwarding(childCmd *exec.Cmd) func() {
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt)
    go func() {
        for range sigCh {
            if childCmd.Process != nil {
                _ = childCmd.Process.Kill() // Windows: no SIGTERM, use Kill()
            }
        }
    }()
    return func() {
        signal.Stop(sigCh)
        close(sigCh)
    }
}
```

**Problem**: `Process.Kill()` only terminates the direct child process. If that child spawns grandchild processes (e.g., `bash -c "kubectl get pods && kubectl apply"`), those grandchildren are **orphaned** and continue running, wasting CI agent resources and holding file locks.

### 1.2 Dependency Status

`golang.org/x/sys` is **already a dependency** at v0.19.0 (indirect, via `golang.org/x/term`):

```
golang.org/x/sys v0.19.0 // indirect
```

To use the Job Object APIs, the import will change from indirect to direct: `import "golang.org/x/sys/windows"`.

### 1.3 API Surface — Complete Reference

All APIs are in package `golang.org/x/sys/windows`. Source locations reference the `golang/sys` GitHub repository.

#### Core Functions

| Function | Signature | Source |
|----------|-----------|--------|
| `CreateJobObject` | `func CreateJobObject(jobAttr *SecurityAttributes, name *uint16) (handle Handle, err error)` | `syscall_windows.go` line 349 — `= kernel32.CreateJobObjectW` |
| `AssignProcessToJobObject` | `func AssignProcessToJobObject(job Handle, process Handle) (err error)` | `syscall_windows.go` line 350 — `= kernel32.AssignProcessToJobObject` |
| `TerminateJobObject` | `func TerminateJobObject(job Handle, exitCode uint32) (err error)` | `syscall_windows.go` line 351 — `= kernel32.TerminateJobObject` |
| `SetInformationJobObject` | `func SetInformationJobObject(job Handle, JobObjectInformationClass uint32, JobObjectInformation uintptr, JobObjectInformationLength uint32) (ret int, err error)` | `syscall_windows.go` line 357 |
| `QueryInformationJobObject` | `func QueryInformationJobObject(job Handle, JobObjectInformationClass int32, JobObjectInformation uintptr, JobObjectInformationLength uint32, retlen *uint32) (err error)` | `syscall_windows.go` line 356 |
| `CloseHandle` | `func CloseHandle(handle Handle) (err error)` | Standard Windows handle cleanup |

#### Key Types

**`JOBOBJECT_BASIC_LIMIT_INFORMATION`** (architecture-specific):

From `types_windows_amd64.go` (line 25):
```go
type JOBOBJECT_BASIC_LIMIT_INFORMATION struct {
    PerProcessUserTimeLimit int64
    PerJobUserTimeLimit     int64
    LimitFlags              uint32
    MinimumWorkingSetSize   uintptr
    MaximumWorkingSetSize   uintptr
    ActiveProcessLimit      uint32
    Affinity                uintptr
    PriorityClass           uint32
    SchedulingClass         uint32
}
```

From `types_windows_386.go` and `types_windows_arm.go` (32-bit variants have padding):
```go
type JOBOBJECT_BASIC_LIMIT_INFORMATION struct {
    PerProcessUserTimeLimit int64
    PerJobUserTimeLimit     int64
    LimitFlags              uint32
    MinimumWorkingSetSize   uintptr
    MaximumWorkingSetSize   uintptr
    ActiveProcessLimit      uint32
    Affinity                uintptr
    PriorityClass           uint32
    SchedulingClass         uint32
    _                       uint32 // pad to 8 byte boundary
}
```

**`JOBOBJECT_EXTENDED_LIMIT_INFORMATION`** from `types_windows.go` (line 2540):
```go
type JOBOBJECT_EXTENDED_LIMIT_INFORMATION struct {
    BasicLimitInformation JOBOBJECT_BASIC_LIMIT_INFORMATION
    IoInfo                IO_COUNTERS
    ProcessMemoryLimit    uintptr
    JobMemoryLimit        uintptr
    PeakProcessMemoryUsed uintptr
    PeakJobMemoryUsed     uintptr
}
```

#### Constants

From `types_windows.go` (lines 2513–2530):
```go
const (
    // flags for JOBOBJECT_BASIC_LIMIT_INFORMATION.LimitFlags
    JOB_OBJECT_LIMIT_ACTIVE_PROCESS             = 0x00000008
    JOB_OBJECT_LIMIT_AFFINITY                   = 0x00000010
    JOB_OBJECT_LIMIT_BREAKAWAY_OK               = 0x00000800
    JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION = 0x00000400
    JOB_OBJECT_LIMIT_JOB_MEMORY                 = 0x00000200
    JOB_OBJECT_LIMIT_JOB_TIME                   = 0x00000004
    JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE          = 0x00002000  // ← KEY FLAG
    JOB_OBJECT_LIMIT_PRESERVE_JOB_TIME          = 0x00000040
    JOB_OBJECT_LIMIT_PRIORITY_CLASS             = 0x00000020
    JOB_OBJECT_LIMIT_PROCESS_MEMORY             = 0x00000100
    JOB_OBJECT_LIMIT_PROCESS_TIME               = 0x00000002
    JOB_OBJECT_LIMIT_SCHEDULING_CLASS           = 0x00000080
    JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK        = 0x00001000
    JOB_OBJECT_LIMIT_SUBSET_AFFINITY            = 0x00004000
    JOB_OBJECT_LIMIT_WORKINGSET                 = 0x00000001
)
```

Job Object information class constants from `types_windows.go` (lines 2566–2582):
```go
const (
    JobObjectAssociateCompletionPortInformation = 7
    JobObjectBasicAccountingInformation         = 1
    JobObjectBasicAndIoAccountingInformation    = 8
    JobObjectBasicLimitInformation              = 2
    JobObjectBasicProcessIdList                 = 3
    JobObjectBasicUIRestrictions                = 4
    JobObjectCpuRateControlInformation          = 15
    JobObjectEndOfJobTimeInformation            = 6
    JobObjectExtendedLimitInformation           = 9   // ← USE THIS
    JobObjectGroupInformation                   = 11
    JobObjectGroupInformationEx                 = 14
    JobObjectLimitViolationInformation          = 13
    JobObjectLimitViolationInformation2         = 34
    JobObjectNetRateControlInformation          = 32
    JobObjectNotificationLimitInformation       = 12
    JobObjectNotificationLimitInformation2      = 33
    JobObjectSecurityLimitInformation           = 5
)
```

Process creation flags for `CREATE_BREAKAWAY_FROM_JOB` from `types_windows.go` (line 238):
```go
const CREATE_BREAKAWAY_FROM_JOB = 0x01000000
```

### 1.4 Canonical Usage Pattern (from golang/sys test suite)

From `syscall_windows_test.go` lines 588–621 (`TestJobObjectInfo`):

```go
// Create anonymous job object (nil name = anonymous)
jo, err := windows.CreateJobObject(nil, nil)
if err != nil {
    t.Fatalf("CreateJobObject failed: %v", err)
}
defer windows.CloseHandle(jo)

// Query current limits
var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
err = windows.QueryInformationJobObject(jo, windows.JobObjectExtendedLimitInformation,
    uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil)

// Set limits (e.g., memory limit)
info.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY
info.ProcessMemoryLimit = 4 * 1024
_, err = windows.SetInformationJobObject(jo, windows.JobObjectExtendedLimitInformation,
    uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
```

### 1.5 Recommended Implementation Pattern for cli-replay

```go
// Pseudocode — NOT production code

import "golang.org/x/sys/windows"

// Step 1: Create job object before starting child
job, err := windows.CreateJobObject(nil, nil)
// if err → log warning, fall back to Process.Kill()

// Step 2: Configure KILL_ON_JOB_CLOSE
var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
    uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))

// Step 3: Start child process (via os/exec or CreateProcess with CREATE_SUSPENDED)
childCmd.Start()

// Step 4: Assign child to job AFTER start (need process handle)
windows.AssignProcessToJobObject(job, windows.Handle(childCmd.Process.Pid))
// NOTE: Need the actual Windows handle, not the PID.
// Use: handle, _ := windows.OpenProcess(windows.PROCESS_ALL_ACCESS, false, uint32(cmd.Process.Pid))
// Or use cmd.Process.Handle (via reflect/unsafe, since it's unexported)

// Step 5: On termination signal, terminate the job
windows.TerminateJobObject(job, 1)

// Step 6: When job handle is closed (deferred), all remaining processes are killed
// (because of KILL_ON_JOB_CLOSE)
defer windows.CloseHandle(job)
```

### 1.6 Key Design Decisions

| Decision | Recommendation | Rationale |
|----------|---------------|-----------|
| Job handle acquisition | Use `windows.OpenProcess()` with child PID, or access `cmd.Process` handle via `syscall.Handle` | Go's `os.Process` doesn't expose the Windows handle directly; use `cmd.SysProcAttr.CreationFlags` with `CREATE_SUSPENDED` + `ResumeThread` pattern for race-free assignment |
| Race window | `CREATE_SUSPENDED` + assign + `ResumeThread` | Without this, child can spawn grandchildren between `Start()` and `AssignProcessToJobObject()` |
| Nested job objects | Supported on Windows 8+ / Server 2012+ | Older Windows doesn't allow processes already in a job to be assigned to another. All GitHub Actions and Azure DevOps runners are Windows 10+ |
| Fallback | Log warning, fall back to `Process.Kill()` | Ensures cli-replay works in restricted environments or containers where job object creation fails |
| Handle lifecycle | `defer windows.CloseHandle(job)` in `setupSignalForwarding` or the exec lifecycle | `KILL_ON_JOB_CLOSE` ensures cleanup even if parent crashes |

### 1.7 SysProcAttr Integration

Go's `os/exec` supports `SysProcAttr` on Windows:

```go
import "syscall"

childCmd.SysProcAttr = &syscall.SysProcAttr{
    CreationFlags: syscall.CREATE_SUSPENDED | syscall.CREATE_NEW_PROCESS_GROUP,
}
```

This allows:
1. Start child in suspended state
2. Assign to job object (no race)
3. Call `windows.ResumeThread()` to begin execution

The `ResumeThread` function is available:
```go
//sys ResumeThread(thread Handle) (ret uint32, err error) = kernel32.ResumeThread
```

To get the thread handle, use `ProcessInformation.Thread` from `CreateProcess`, or access `cmd.Process` internals.

### 1.8 Platform Compatibility

| Windows Version | Nested Jobs | Notes |
|----------------|-------------|-------|
| Windows 7 / Server 2008 R2 | ❌ | Process already in a job → `AssignProcessToJobObject` fails |
| Windows 8 / Server 2012 | ✅ | Full nested job support |
| Windows 10+ / Server 2016+ | ✅ | All modern CI runners |
| GitHub Actions `windows-latest` | ✅ | Windows Server 2022 |
| Azure DevOps Windows agents | ✅ | Windows Server 2019/2022 |

---

## Topic 2: Composite GitHub Actions for Binary Distribution

### 2.1 Composite Action Structure

A composite action is defined by an `action.yml` file with `runs.using: "composite"`:

```yaml
name: 'Setup cli-replay'
description: 'Install cli-replay and optionally run a scenario'
inputs:
  scenario:
    description: 'Path to scenario YAML file'
    required: true
  run:
    description: 'Command to run under interception'
    required: false
  version:
    description: 'cli-replay version to install (e.g., v1.2.3)'
    required: false
    default: 'latest'
  format:
    description: 'Output format for verification report (json, junit)'
    required: false
  report-file:
    description: 'Path to write verification report'
    required: false
  validate-only:
    description: 'Run validate instead of exec'
    required: false
    default: 'false'
  allowed-commands:
    description: 'Comma-separated allowlist of commands'
    required: false
outputs:
  cli-replay-version:
    description: 'Installed cli-replay version'
    value: ${{ steps.install.outputs.version }}
runs:
  using: "composite"
  steps:
    - name: Install cli-replay
      id: install
      shell: bash
      run: |
        # ... download and install logic
```

**Key references**:
- `${{ github.action_path }}` — resolves to the action's own directory
- `$GITHUB_PATH` — file to append directories to PATH
- `$GITHUB_OUTPUT` — file for step outputs
- Each step **must** declare `shell:` explicitly in composite actions

### 2.2 Platform Detection in Composite Actions

#### Context Variables

| Variable | Access Pattern | Values |
|----------|---------------|--------|
| Runner OS | `${{ runner.os }}` | `Linux`, `macOS`, `Windows` |
| Runner Arch | `${{ runner.arch }}` | `X64`, `X86`, `ARM`, `ARM64` |
| Runner OS (env) | `$RUNNER_OS` | `Linux`, `macOS`, `Windows` |
| Runner Arch (env) | `$RUNNER_ARCH` | `X64`, `X86`, `ARM`, `ARM64` |

#### Mapping to Go Build Targets

From `actions/setup-go` `system.ts`:

```typescript
// getArch() — maps runner.arch to Go GOARCH
function getArch(arch: string): string {
    const mappings: {[s: string]: string} = {
        x64: 'amd64',
        x32: '386',
        arm: 'armv6l',
    };
    return mappings[arch] || arch;  // ARM64 passes through as 'arm64'
}

// getPlatform() — maps process.platform to Go GOOS
function getPlatform(platform: string): string {
    const mappings: {[s: string]: string} = {
        win32: 'windows',
    };
    return mappings[platform] || platform;  // 'linux', 'darwin' pass through
}
```

**For a composite shell action**, the mapping logic must be done in bash/powershell:

```bash
# OS mapping (runner.os → GOOS)
case "${{ runner.os }}" in
  Linux)   GOOS="linux" ;;
  macOS)   GOOS="darwin" ;;
  Windows) GOOS="windows" ;;
esac

# Arch mapping (runner.arch → GOARCH)
case "${{ runner.arch }}" in
  X64)   GOARCH="amd64" ;;
  X86)   GOARCH="386" ;;
  ARM)   GOARCH="arm" ;;
  ARM64) GOARCH="arm64" ;;
esac
```

### 2.3 Binary Download Pattern

Based on analysis of `actions/setup-go` and `cli/cli`:

#### actions/setup-go Pattern (TypeScript/Node action)
1. Check tool cache → `tc.find('go', version, arch)`
2. Try manifest download → `tc.downloadToolAttempt(downloadUrl)`
3. Extract archive → `tc.extractTar()` / `tc.extractZip()`
4. Cache result → `tc.cacheDir(extPath, 'go', version, arch)`
5. Add to PATH → `core.addPath(binPath)`

#### cli/cli Pattern (Go binary distribution)

From `copilot.go` `downloadCopilot()`:
```go
// Platform + arch detection
platform := runtime.GOOS     // "linux", "darwin", "windows"
arch := runtime.GOARCH        // "amd64", "arm64"

// Archive naming convention
archiveName := fmt.Sprintf("copilot-%s-%s.tar.gz", platform, arch)

// Download + checksum verification
// SHA256 verification against a signed checksums file
```

#### Recommended Pattern for cli-replay-action

```bash
# Step: Download binary
VERSION="${{ inputs.version }}"
if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -sL https://api.github.com/repos/ormasoftchile/cli-replay/releases/latest | grep tag_name | cut -d '"' -f 4)
fi

# Construct archive name
GOOS=...  # (from platform detection above)
GOARCH=...
EXT="tar.gz"
if [ "$GOOS" = "windows" ]; then
  EXT="zip"
fi
ARCHIVE="cli-replay-${VERSION}-${GOOS}-${GOARCH}.${EXT}"

# Download
curl -sL "https://github.com/ormasoftchile/cli-replay/releases/download/${VERSION}/${ARCHIVE}" -o "$ARCHIVE"

# Extract
if [ "$GOOS" = "windows" ]; then
  unzip "$ARCHIVE" -d cli-replay-bin
else
  tar xzf "$ARCHIVE" -C cli-replay-bin
fi

# Add to PATH
echo "${{ github.action_path }}/cli-replay-bin" >> "$GITHUB_PATH"
```

### 2.4 Cross-Platform Shell Selection

Composite action steps must specify `shell` explicitly. For cross-platform:

| Runner OS | Recommended Shell | Notes |
|-----------|------------------|-------|
| Linux | `bash` | Default, always available |
| macOS | `bash` | Default, always available |
| Windows | `bash` | Git Bash available on all Windows runners |
| Windows | `pwsh` | PowerShell Core available on all runners |

**Recommendation**: Use `shell: bash` for all steps — Git Bash is pre-installed on all GitHub Actions Windows runners, providing a consistent cross-platform experience.

### 2.5 PATH Management

```yaml
- name: Add cli-replay to PATH
  shell: bash
  run: echo "${{ github.action_path }}/bin" >> "$GITHUB_PATH"
```

- `$GITHUB_PATH` appends to PATH for **all subsequent steps** (not just the current step)
- This is the standard mechanism used by `actions/setup-go`, `actions/setup-node`, etc.
- On Windows, both forward slashes and backslashes work in PATH entries

### 2.6 action.yml Input/Output Contract

**Inputs** (declared in `action.yml`):
- Accessed via `${{ inputs.scenario }}` in step `run:` blocks
- No type system — all values are strings
- `default:` provides fallback values
- `required: true` enforces presence at workflow author time (not runtime)

**Outputs** (declared in `action.yml`):
- Populated via `$GITHUB_OUTPUT` in composite steps
- Referenced by consuming workflow via `${{ steps.<step-id>.outputs.<name> }}`

### 2.7 Versioning and Publishing

- Publish in a separate repo: `ormasoftchile/cli-replay-action`
- Use semantic versioning tags: `v1.0.0`, `v1.0.1`, etc.
- Maintain a floating major version tag: `v1` → points to latest `v1.x.x`
- Users reference as `uses: ormasoftchile/cli-replay-action@v1`
- Update `v1` tag on each minor/patch release: `git tag -fa v1 -m "Update v1 tag"`

### 2.8 Key Constraints and Gotchas

1. **Composite actions cannot use `if:` at the action level** — only at step level within `runs.steps`
2. **Each step must declare `shell:`** — unlike regular workflow steps, there's no default
3. **`${{ github.action_path }}`** resolves at step execution time — safe for referencing bundled scripts
4. **Nested composite actions** are supported (since 2022)
5. **No `services:` or `container:`** support in composite actions
6. **Step errors** propagate up — a failed step fails the action unless `continue-on-error: true`

---

## Topic 3: Cobra Subcommand Patterns for `validate`

### 3.1 Existing Command Registration Pattern in cli-replay

All commands follow the same pattern:

```go
// Package-level variable for the command
var validateCmd = &cobra.Command{
    Use:   "validate <file>...",
    Short: "Validate scenario files",
    // ...
    RunE: runValidate,
}

// Package-level init() registers with root
func init() {
    validateCmd.Flags().StringVar(&formatFlag, "format", "text", "Output format: text, json")
    rootCmd.AddCommand(validateCmd)
}

// RunE implementation
func runValidate(cmd *cobra.Command, args []string) error {
    // ...
}
```

**Evidence from codebase**:
- `cmd/verify.go`: `rootCmd.AddCommand(verifyCmd)` in `init()`
- `cmd/exec.go`: `rootCmd.AddCommand(execCmd)` in `init()`
- `cmd/run.go`: `rootCmd.AddCommand(runCmd)` in `init()`
- `cmd/clean.go`: `rootCmd.AddCommand(cleanCmd)` in `init()`
- `cmd/root.go`: `rootCmd` with `SilenceUsage: true, SilenceErrors: true`

### 3.2 Argument Validation Patterns

| Command | Args Constraint | Multi-file? |
|---------|----------------|-------------|
| `verify` | `cobra.MaximumNArgs(1)` | No — single file or env var |
| `run` | `cobra.ExactArgs(1)` | No — single scenario |
| `exec` | Custom (uses `ArgsLenAtDash()`) | No — single scenario + child command |
| `clean` | `cobra.MaximumNArgs(1)` | No — single file or env var |
| **`validate`** (new) | `cobra.MinimumNArgs(1)` | **Yes — one or more files** |

For `validate`, the recommended constraint is:

```go
Args: cobra.MinimumNArgs(1),
```

This enforces at least one file while allowing `cli-replay validate a.yaml b.yaml c.yaml`.

### 3.3 Format Flag Pattern

From `cmd/verify.go`:

```go
var verifyFormatFlag string

var verifyCmd = &cobra.Command{
    // ...
    RunE: runVerify,
}

func init() {
    verifyCmd.Flags().StringVar(&verifyFormatFlag, "format", "text", "Output format: text, json, or junit")
    rootCmd.AddCommand(verifyCmd)
}

func runVerify(_ *cobra.Command, args []string) error {
    format := strings.ToLower(verifyFormatFlag)
    switch format {
    case "text", "json", "junit":
        // valid
    default:
        return fmt.Errorf("invalid format %q: valid values are text, json, junit", verifyFormatFlag)
    }
    // ...
}
```

For `validate`, the same pattern applies with `text` and `json` formats only (no `junit` — validation results aren't test executions):

```go
var validateFormatFlag string

func init() {
    validateCmd.Flags().StringVar(&validateFormatFlag, "format", "text", "Output format: text, json")
    rootCmd.AddCommand(validateCmd)
}
```

### 3.4 Multi-File Error Reporting Pattern

The `validate` command must report errors per-file in a single pass. Proposed approach:

```go
func runValidate(_ *cobra.Command, args []string) error {
    hasErrors := false
    var results []ValidationResult  // for JSON output

    for _, path := range args {
        result := validateFile(path)
        results = append(results, result)
        if !result.Valid {
            hasErrors = true
        }
    }

    // Output based on format
    switch format {
    case "text":
        for _, r := range results {
            if r.Valid {
                fmt.Fprintf(os.Stderr, "✓ %s: valid\n", r.File)
            } else {
                for _, e := range r.Errors {
                    fmt.Fprintf(os.Stderr, "✗ %s: %s\n", r.File, e)
                }
            }
        }
    case "json":
        json.NewEncoder(os.Stdout).Encode(results)
    }

    if hasErrors {
        os.Exit(1)
    }
    return nil
}
```

### 3.5 Existing Validation Logic Reuse

The `validate` command can directly reuse `scenario.LoadFile()` which calls `scenario.Load()` → `Scenario.Validate()`:

**Validation already implemented in `internal/scenario/model.go`**:

| Check | Location | Method |
|-------|----------|--------|
| Empty `meta.name` | `Meta.Validate()` | `strings.TrimSpace(m.Name) == ""` |
| No steps | `Scenario.Validate()` | `len(s.Steps) == 0` |
| Exit code range (0–255) | `Response.Validate()` | `r.Exit < 0 \|\| r.Exit > 255` |
| stdout/stdout_file mutual exclusivity | `Response.Validate()` | Both non-empty check |
| stderr/stderr_file mutual exclusivity | `Response.Validate()` | Both non-empty check |
| Capture identifier regex | `Response.Validate()` | `captureIdentifierRe.MatchString(key)` |
| Empty argv | `Match.Validate()` | `len(m.Argv) == 0` |
| `calls.min > calls.max` | `CallBounds.Validate()` | `cb.Min > cb.Max` |
| `calls.max < 1` | `CallBounds.Validate()` | `cb.Max < 1` |
| Nested groups | `StepGroup.Validate()` | `elem.Group != nil` → error |
| Empty groups | `StepGroup.Validate()` | `len(sg.Steps) == 0` |
| Unsupported group mode | `StepGroup.Validate()` | `sg.Mode != "unordered"` |
| Forward capture references | `Scenario.validateCaptures()` | Template AST walk |
| Capture-vs-vars conflicts | `Scenario.validateCaptures()` | Map key check |
| Unknown YAML fields | `loader.go` strict decode | `decoder.KnownFields(true)` |
| Invalid `deny_env_vars` | `Security.Validate()` | Empty pattern check |
| Invalid session TTL | `Session.Validate()` | Duration parse + positive check |

**What `validate` adds beyond `LoadFile()`**:
1. **Multi-file iteration** — validate all files, don't stop on first error
2. **`stdout_file`/`stderr_file` existence check** — resolve relative to scenario dir, warn if missing
3. **Structured output** — text/json formatting of validation results
4. **No side effects guarantee** — no state files, no intercept dirs, no env changes

### 3.6 Exit Code Convention

Consistent with existing commands:

| Condition | Exit Code | Behavior |
|-----------|-----------|----------|
| All files valid | 0 | Success message per file |
| Any file has errors | 1 | All errors reported, then exit 1 |
| Usage error (no args) | 1 | Via `cobra.MinimumNArgs(1)` automatic error |
| Invalid `--format` flag | 1 | Via `RunE` returning error |

### 3.7 Proposed Command Definition

```go
var validateFormatFlag string

var validateCmd = &cobra.Command{
    Use:   "validate <file>...",
    Short: "Validate scenario files for schema and semantic correctness",
    Long: `Validate one or more scenario YAML files without executing them.

Checks schema compliance (required fields, types, ranges) and semantic rules
(no forward capture references, no min > max, no duplicate capture keys,
allowlist consistency, group structure).

Does not create any files, directories, or modify any environment state.

Exit code 0 if all files are valid, 1 if any file has errors.

Formats:
  text   Human-readable output to stderr (default)
  json   Structured JSON to stdout

Examples:
  cli-replay validate scenario.yaml
  cli-replay validate a.yaml b.yaml c.yaml
  cli-replay validate --format json scenario.yaml`,
    Args: cobra.MinimumNArgs(1),
    RunE: runValidate,
}

func init() {
    validateCmd.Flags().StringVar(&validateFormatFlag, "format", "text",
        "Output format: text, json")
    rootCmd.AddCommand(validateCmd)
}
```

### 3.8 JSON Output Schema

```json
[
  {
    "file": "scenario.yaml",
    "valid": true,
    "errors": []
  },
  {
    "file": "invalid.yaml",
    "valid": false,
    "errors": [
      "meta: name must be non-empty",
      "step 2: calls: min (5) must be <= max (3)",
      "step 3: respond: stdout and stdout_file are mutually exclusive"
    ]
  }
]
```

---

## Cross-Cutting Findings

### Build Tags

The Windows Job Objects implementation should use `//go:build windows` consistently with the existing `exec_windows.go` pattern. The Unix `exec_unix.go` remains unchanged.

### No New Dependencies Required

| Feature | Dependencies |
|---------|-------------|
| Windows Job Objects | `golang.org/x/sys/windows` — already at v0.19.0 (promote from indirect to direct) |
| `validate` command | `encoding/json` (stdlib), reuses `internal/scenario` |
| GitHub Action | No Go dependencies — shell scripts only in separate repo |

### Testing Strategy Notes

| Feature | Test Approach |
|---------|--------------|
| Windows Job Objects | Integration test on Windows CI only (`//go:build windows`); test that `TerminateJobObject` kills grandchildren; unit test fallback path on all platforms |
| `validate` command | Table-driven tests with valid/invalid fixtures; test multi-file reporting; test JSON output schema; test `stdout_file` existence warning |
| GitHub Action | Workflow-level testing in the action repo (test matrix: ubuntu/macos/windows × latest/specific version) |

---

## References

- **golang/sys repo**: https://github.com/golang/sys — `windows/syscall_windows.go`, `windows/types_windows.go`, `windows/types_windows_amd64.go`
- **Job Objects test**: https://github.com/golang/sys/blob/main/windows/syscall_windows_test.go#L588-L621
- **Composite Actions docs**: https://docs.github.com/en/actions/creating-actions/creating-a-composite-action
- **GitHub Actions variables**: https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/store-information-in-variables
- **actions/setup-go source**: https://github.com/actions/setup-go — `src/system.ts`, `src/installer.ts`
- **cli/cli source**: https://github.com/cli/cli — `pkg/cmd/copilot/copilot.go` for binary download patterns
- **Cobra documentation**: https://github.com/spf13/cobra
